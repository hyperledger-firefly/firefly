// Copyright © 2021 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package sqlcommon

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/stretchr/testify/assert"
)

func TestFFIErrorsE2EWithDB(t *testing.T) {

	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new ffiErr entry
	interfaceID := fftypes.NewUUID()
	ffiErrID := fftypes.NewUUID()
	ffiErr := &fftypes.FFIError{
		ID:        ffiErrID,
		Interface: interfaceID,
		Namespace: "ns",
		Pathname:  "Changed_1",
		FFIErrorDefinition: fftypes.FFIErrorDefinition{
			Name:        "Changed",
			Description: "Things changed",
			Params: fftypes.FFIParams{
				{
					Name:   "value",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
				},
			},
		},
	}

	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionFFIErrors, core.ChangeEventTypeCreated, "ns", ffiErrID).Return()
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionFFIErrors, core.ChangeEventTypeUpdated, "ns", ffiErrID).Return()

	err := s.UpsertFFIError(ctx, ffiErr)
	assert.NoError(t, err)

	// Query back the ffiErr (by query filter)
	fb := database.FFIErrorQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("name", ffiErr.Name),
	)
	errors, res, err := s.GetFFIErrors(ctx, "ns", filter.Count(true))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(errors))
	assert.Equal(t, int64(1), *res.TotalCount)
	ffiErrJson, _ := json.Marshal(&ffiErr)
	ffiErrReadJson, _ := json.Marshal(errors[0])
	assert.Equal(t, string(ffiErrJson), string(ffiErrReadJson))

	// Update ffiErr
	ffiErr.Params = fftypes.FFIParams{}
	err = s.UpsertFFIError(ctx, ffiErr)
	assert.NoError(t, err)

	s.callbacks.AssertExpectations(t)
}

func TestFFIErrorDBFailBeginTransaction(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertFFIError(context.Background(), &fftypes.FFIError{})
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFFIErrorDBFailSelect(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertFFIError(context.Background(), &fftypes.FFIError{})
	assert.Regexp(t, "pop", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFFIErrorDBFailInsert(t *testing.T) {
	rows := sqlmock.NewRows([]string{"id", "namespace", "name", "version"})
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(rows)
	ffiErr := &fftypes.FFIError{
		ID: fftypes.NewUUID(),
	}
	err := s.UpsertFFIError(context.Background(), ffiErr)
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFFIErrorDBFailUpdate(t *testing.T) {
	rows := sqlmock.NewRows([]string{"id", "namespace", "name", "version"}).
		AddRow("7e2c001c-e270-4fd7-9e82-9dacee843dc2", "ns1", "math", "v1.0.0")
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(rows)
	mock.ExpectQuery("UPDATE .*").WillReturnError(fmt.Errorf("pop"))
	ffiErr := &fftypes.FFIError{
		ID: fftypes.NewUUID(),
	}
	err := s.UpsertFFIError(context.Background(), ffiErr)
	assert.Regexp(t, "pop", err)
}

func TestGetFFIErrors(t *testing.T) {
	// filter := database.FFIErrorQueryFactory.NewFilter(context.Background()).In("", []driver.Value{})

	fb := database.FFIErrorQueryFactory.NewFilter(context.Background())
	filter := fb.And(
		fb.Eq("name", "sum"),
	)
	s, mock := newMockProvider().init()
	rows := sqlmock.NewRows(ffiErrorsColumns).
		AddRow(fftypes.NewUUID().String(), fftypes.NewUUID().String(), "ns1", "sum", "sum", "", []byte(`[]`))
	mock.ExpectQuery("SELECT .*").WillReturnRows(rows)
	_, _, err := s.GetFFIErrors(context.Background(), "ns1", filter)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetFFIErrorsFilterSelectFail(t *testing.T) {
	fb := database.FFIErrorQueryFactory.NewFilter(context.Background())
	s, _ := newMockProvider().init()
	_, _, err := s.GetFFIErrors(context.Background(), "ns1", fb.And(fb.Eq("id", map[bool]bool{true: false})))
	assert.Error(t, err)
}

func TestGetFFIErrorsQueryFail(t *testing.T) {
	fb := database.FFIErrorQueryFactory.NewFilter(context.Background())
	filter := fb.And(
		fb.Eq("id", fftypes.NewUUID()),
	)
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, _, err := s.GetFFIErrors(context.Background(), "ns1", filter)
	assert.Regexp(t, "pop", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetFFIErrorsQueryResultFail(t *testing.T) {
	fb := database.FFIErrorQueryFactory.NewFilter(context.Background())
	filter := fb.And(
		fb.Eq("id", fftypes.NewUUID()),
	)
	s, mock := newMockProvider().init()
	rows := sqlmock.NewRows([]string{"id", "namespace", "name", "version"}).
		AddRow("7e2c001c-e270-4fd7-9e82-9dacee843dc2", "ns1", "math", "v1.0.0").
		AddRow("7e2c001c-e270-4fd7-9e82-9dacee843dc2", nil, "math", "v1.0.0")
	mock.ExpectQuery("SELECT .*").WillReturnRows(rows)
	_, _, err := s.GetFFIErrors(context.Background(), "ns1", filter)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
