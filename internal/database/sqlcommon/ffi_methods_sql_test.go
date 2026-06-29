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

func TestFFIMethodsE2EWithDB(t *testing.T) {

	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new method entry
	interfaceID := fftypes.NewUUID()
	methodID := fftypes.NewUUID()
	method := &fftypes.FFIMethod{
		ID:          methodID,
		Interface:   interfaceID,
		Name:        "Set",
		Namespace:   "ns",
		Pathname:    "Set_1",
		Description: "Sets things",
		Params: fftypes.FFIParams{
			{
				Name:   "value",
				Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
			},
		},
		Returns: fftypes.FFIParams{
			{
				Name:   "value",
				Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
			},
		},
	}

	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionFFIMethods, core.ChangeEventTypeCreated, "ns", methodID).Return()
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionFFIMethods, core.ChangeEventTypeUpdated, "ns", methodID).Return()

	err := s.UpsertFFIMethod(ctx, method)
	assert.NoError(t, err)

	// Query back the method (by name)
	methodRead, err := s.GetFFIMethod(ctx, "ns", interfaceID, "Set_1")
	assert.NoError(t, err)
	assert.NotNil(t, methodRead)
	methodJson, _ := json.Marshal(&method)
	methodReadJson, _ := json.Marshal(&methodRead)
	assert.Equal(t, string(methodJson), string(methodReadJson))

	// Query back the method (by query filter)
	fb := database.FFIMethodQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("id", methodRead.ID.String()),
		fb.Eq("name", methodRead.Name),
	)
	methods, res, err := s.GetFFIMethods(ctx, "ns", filter.Count(true))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(methods))
	assert.Equal(t, int64(1), *res.TotalCount)
	methodReadJson, _ = json.Marshal(methods[0])
	assert.Equal(t, string(methodJson), string(methodReadJson))

	// Update method
	method.Params = fftypes.FFIParams{}
	err = s.UpsertFFIMethod(ctx, method)
	assert.NoError(t, err)

	// Query back the method (by name)
	methodRead, err = s.GetFFIMethod(ctx, "ns", interfaceID, "Set_1")
	assert.NoError(t, err)
	assert.NotNil(t, methodRead)
	methodJson, _ = json.Marshal(&method)
	methodReadJson, _ = json.Marshal(&methodRead)
	assert.Equal(t, string(methodJson), string(methodReadJson))

	s.callbacks.AssertExpectations(t)
}

func TestFFIMethodDBFailBeginTransaction(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertFFIMethod(context.Background(), &fftypes.FFIMethod{})
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFFIMethodDBFailSelect(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertFFIMethod(context.Background(), &fftypes.FFIMethod{})
	assert.Regexp(t, "pop", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFFIMethodDBFailInsert(t *testing.T) {
	rows := sqlmock.NewRows([]string{"id", "namespace", "name", "version"})
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(rows)
	event := &fftypes.FFIMethod{
		ID: fftypes.NewUUID(),
	}
	err := s.UpsertFFIMethod(context.Background(), event)
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFFIMethodDBFailUpdate(t *testing.T) {
	rows := sqlmock.NewRows([]string{"id", "namespace", "name", "version"}).
		AddRow("7e2c001c-e270-4fd7-9e82-9dacee843dc2", "ns1", "math", "v1.0.0")
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(rows)
	mock.ExpectQuery("UPDATE .*").WillReturnError(fmt.Errorf("pop"))
	event := &fftypes.FFIMethod{
		ID: fftypes.NewUUID(),
	}
	err := s.UpsertFFIMethod(context.Background(), event)
	assert.Regexp(t, "pop", err)
}

func TestFFIMethodDBFailScan(t *testing.T) {
	s, mock := newMockProvider().init()
	id := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("only one"))
	_, err := s.GetFFIMethod(context.Background(), id.String(), fftypes.NewUUID(), "sum")
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFFIMethodDBSelectFail(t *testing.T) {
	s, mock := newMockProvider().init()
	id := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetFFIMethod(context.Background(), id.String(), fftypes.NewUUID(), "sum")
	assert.Regexp(t, "pop", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFFIMethodDBNoRows(t *testing.T) {
	s, mock := newMockProvider().init()
	id := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id", "name", "params", "returns"}))
	_, err := s.GetFFIMethod(context.Background(), id.String(), fftypes.NewUUID(), "sum")
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetFFIMethods(t *testing.T) {
	// filter := database.FFIMethodQueryFactory.NewFilter(context.Background()).In("", []driver.Value{})

	fb := database.FFIMethodQueryFactory.NewFilter(context.Background())
	filter := fb.And(
		fb.Eq("name", "sum"),
	)
	s, mock := newMockProvider().init()
	rows := sqlmock.NewRows(ffiMethodsColumns).
		AddRow(fftypes.NewUUID().String(), fftypes.NewUUID().String(), "ns1", "sum", "sum", "", []byte(`[]`), []byte(`[]`), []byte(`{}`))
	mock.ExpectQuery("SELECT .*").WillReturnRows(rows)
	_, _, err := s.GetFFIMethods(context.Background(), "ns1", filter)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetFFIMethodsFilterSelectFail(t *testing.T) {
	fb := database.FFIMethodQueryFactory.NewFilter(context.Background())
	s, _ := newMockProvider().init()
	_, _, err := s.GetFFIMethods(context.Background(), "ns1", fb.And(fb.Eq("id", map[bool]bool{true: false})))
	assert.Error(t, err)
}

func TestGetFFIMethodsQueryFail(t *testing.T) {
	fb := database.FFIMethodQueryFactory.NewFilter(context.Background())
	filter := fb.And(
		fb.Eq("id", fftypes.NewUUID()),
	)
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, _, err := s.GetFFIMethods(context.Background(), "ns1", filter)
	assert.Regexp(t, "pop", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetFFIMethodsQueryResultFail(t *testing.T) {
	fb := database.FFIMethodQueryFactory.NewFilter(context.Background())
	filter := fb.And(
		fb.Eq("id", fftypes.NewUUID()),
	)
	s, mock := newMockProvider().init()
	rows := sqlmock.NewRows([]string{"id", "namespace", "name", "version"}).
		AddRow("7e2c001c-e270-4fd7-9e82-9dacee843dc2", "ns1", "math", "v1.0.0").
		AddRow("7e2c001c-e270-4fd7-9e82-9dacee843dc2", nil, "math", "v1.0.0")
	mock.ExpectQuery("SELECT .*").WillReturnRows(rows)
	_, _, err := s.GetFFIMethods(context.Background(), "ns1", filter)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetFFIMethod(t *testing.T) {
	s, mock := newMockProvider().init()
	rows := sqlmock.NewRows(ffiMethodsColumns).
		AddRow(fftypes.NewUUID().String(), fftypes.NewUUID().String(), "ns1", "sum", "sum", "", []byte(`[]`), []byte(`[]`), []byte(`{}`))
	mock.ExpectQuery("SELECT .*").WillReturnRows(rows)
	FFIMethod, err := s.GetFFIMethod(context.Background(), "ns1", fftypes.NewUUID(), "math")
	assert.NoError(t, err)
	assert.Equal(t, "sum", FFIMethod.Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}
