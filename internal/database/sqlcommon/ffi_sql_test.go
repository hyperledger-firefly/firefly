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
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/stretchr/testify/assert"
)

func TestFFIE2EWithDB(t *testing.T) {
	log.SetLevel("trace")

	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new FFI
	id := fftypes.NewUUID()

	ffi := &fftypes.FFI{
		ID:          id,
		Namespace:   "ns1",
		Name:        "math",
		NetworkName: "math",
		Version:     "v1.0.0",
		Description: "Does things and stuff",
		Message:     fftypes.NewUUID(),
		Methods: []*fftypes.FFIMethod{
			{
				Name: "sum",
				Params: fftypes.FFIParams{
					{
						Name:   "x",
						Schema: fftypes.JSONAnyPtr(`{"type": "integer""}`),
					},
					{
						Name:   "y",
						Schema: fftypes.JSONAnyPtr(`{"type": "integer""}`),
					},
				},
				Returns: fftypes.FFIParams{
					{
						Name:   "result",
						Schema: fftypes.JSONAnyPtr(`{"type": "integer""}`),
					},
				},
			},
		},
	}

	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionFFIs, core.ChangeEventTypeCreated, "ns1", ffi.ID).Return()
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionFFIs, core.ChangeEventTypeUpdated, "ns1", ffi.ID).Return()
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionFFIs, core.ChangeEventTypeDeleted, "ns1", ffi.ID).Return()

	_, err := s.InsertOrGetFFI(ctx, ffi)
	assert.NoError(t, err)

	// Check we get the correct fields back
	dataRead, err := s.GetFFIByID(ctx, "ns1", id)
	assert.NoError(t, err)
	assert.NotNil(t, dataRead)
	assert.Equal(t, ffi.ID, dataRead.ID)
	assert.Equal(t, ffi.Namespace, dataRead.Namespace)
	assert.Equal(t, ffi.Name, dataRead.Name)
	assert.Equal(t, ffi.Version, dataRead.Version)
	assert.Equal(t, ffi.Message, dataRead.Message)

	ffi.Version = "v1.1.0"
	err = s.UpsertFFI(ctx, ffi, database.UpsertOptimizationExisting)
	assert.NoError(t, err)

	// Check we get the correct fields back
	dataRead, err = s.GetFFIByID(ctx, "ns1", id)
	assert.NoError(t, err)
	assert.NotNil(t, dataRead)
	assert.Equal(t, ffi.ID, dataRead.ID)
	assert.Equal(t, ffi.Namespace, dataRead.Namespace)
	assert.Equal(t, ffi.Name, dataRead.Name)
	assert.Equal(t, ffi.Version, dataRead.Version)
	assert.Equal(t, ffi.Message, dataRead.Message)

	dataRead, err = s.GetFFIByNetworkName(ctx, "ns1", "math", "v1.1.0")
	assert.NoError(t, err)
	assert.NotNil(t, dataRead)
	assert.Equal(t, ffi.ID, dataRead.ID)
	assert.Equal(t, ffi.Namespace, dataRead.Namespace)
	assert.Equal(t, ffi.Name, dataRead.Name)
	assert.Equal(t, ffi.Version, dataRead.Version)
	assert.Equal(t, ffi.Message, dataRead.Message)

	// Cannot insert again with same name or network name
	existing, err := s.InsertOrGetFFI(ctx, &fftypes.FFI{
		ID:        fftypes.NewUUID(),
		Name:      "math",
		Version:   "v1.1.0",
		Namespace: "ns1",
	})
	assert.NoError(t, err)
	assert.Equal(t, ffi.ID, existing.ID)
	existing, err = s.InsertOrGetFFI(ctx, &fftypes.FFI{
		ID:          fftypes.NewUUID(),
		NetworkName: "math",
		Version:     "v1.1.0",
		Namespace:   "ns1",
	})
	assert.NoError(t, err)
	assert.Equal(t, ffi.ID, existing.ID)

	// Delete the FFI
	err = s.DeleteFFI(ctx, "ns1", ffi.ID)
	assert.NoError(t, err)
}

func TestFFIDBFailBeginTransaction(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertFFI(context.Background(), &fftypes.FFI{}, database.UpsertOptimizationNew)
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFFIDBFailSelect(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertFFI(context.Background(), &fftypes.FFI{}, database.UpsertOptimizationNew)
	assert.Regexp(t, "pop", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFFIDBFailUpsert(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{}))
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertFFI(context.Background(), &fftypes.FFI{}, database.UpsertOptimizationNew)
	assert.Regexp(t, "pop", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFFIDBInsertFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	_, err := s.InsertOrGetFFI(context.Background(), &fftypes.FFI{})
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFFIDBFailInsert(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{}))
	mock.ExpectRollback()
	ffi := &fftypes.FFI{
		ID: fftypes.NewUUID(),
	}
	_, err := s.InsertOrGetFFI(context.Background(), ffi)
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFFIDBFailUpdate(t *testing.T) {
	rows := sqlmock.NewRows([]string{"id", "namespace", "name", "version"}).
		AddRow("7e2c001c-e270-4fd7-9e82-9dacee843dc2", "ns1", "math", "v1.0.0")
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(rows)
	mock.ExpectQuery("UPDATE .*").WillReturnError(fmt.Errorf("pop"))
	ffi := &fftypes.FFI{
		ID: fftypes.NewUUID(),
	}
	err := s.UpsertFFI(context.Background(), ffi, database.UpsertOptimizationNew)
	assert.Regexp(t, "pop", err)
}

func TestFFIDBFailScan(t *testing.T) {
	s, mock := newMockProvider().init()
	id := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("only one"))
	_, err := s.GetFFIByID(context.Background(), "ns1", id)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFFIDBSelectFail(t *testing.T) {
	s, mock := newMockProvider().init()
	id := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetFFIByID(context.Background(), "ns1", id)
	assert.Regexp(t, "pop", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFFIDBNoRows(t *testing.T) {
	s, mock := newMockProvider().init()
	id := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id", "namespace", "name", "version"}))
	_, err := s.GetFFIByID(context.Background(), "ns1", id)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetFFIs(t *testing.T) {
	fb := database.FFIQueryFactory.NewFilter(context.Background())
	s, mock := newMockProvider().init()
	rows := sqlmock.NewRows(ffiColumns).
		AddRow("7e2c001c-e270-4fd7-9e82-9dacee843dc2", "ns1", "math", "math", "v1.0.0", "super mathy things", "acfe07a2-117f-46b7-8d47-e3beb7cc382f", false)
	mock.ExpectQuery("SELECT .*").WillReturnRows(rows)
	_, _, err := s.GetFFIs(context.Background(), "ns1", fb.And())
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetFFIsFilterSelectFail(t *testing.T) {
	fb := database.FFIQueryFactory.NewFilter(context.Background())
	s, _ := newMockProvider().init()
	_, _, err := s.GetFFIs(context.Background(), "ns1", fb.And(fb.Eq("id", map[bool]bool{true: false})))
	assert.Error(t, err)
}

func TestGetFFIsQueryFail(t *testing.T) {
	fb := database.FFIQueryFactory.NewFilter(context.Background())
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, _, err := s.GetFFIs(context.Background(), "ns1", fb.And())
	assert.Regexp(t, "pop", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetFFIsQueryResultFail(t *testing.T) {
	fb := database.FFIQueryFactory.NewFilter(context.Background())
	s, mock := newMockProvider().init()
	rows := sqlmock.NewRows([]string{"id", "namespace", "name", "version"}).
		AddRow("7e2c001c-e270-4fd7-9e82-9dacee843dc2", "ns1", "math", "v1.0.0").
		AddRow("7e2c001c-e270-4fd7-9e82-9dacee843dc2", nil, "math", "v1.0.0")
	mock.ExpectQuery("SELECT .*").WillReturnRows(rows)
	_, _, err := s.GetFFIs(context.Background(), "ns1", fb.And())
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetFFI(t *testing.T) {
	s, mock := newMockProvider().init()
	rows := sqlmock.NewRows(ffiColumns).
		AddRow("7e2c001c-e270-4fd7-9e82-9dacee843dc2", "ns1", "math", "math", "v1.0.0", "super mathy things", "acfe07a2-117f-46b7-8d47-e3beb7cc382f", false)
	mock.ExpectQuery("SELECT .*").WillReturnRows(rows)
	ffi, err := s.GetFFI(context.Background(), "ns1", "math", "v1.0.0")
	assert.NoError(t, err)
	assert.Equal(t, "ns1", ffi.Namespace)
	assert.Equal(t, "math", ffi.Name)
	assert.Equal(t, "v1.0.0", ffi.Version)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteFFIFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.DeleteFFI(context.Background(), "ns1", fftypes.NewUUID())
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteFFIFailDelete(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("DELETE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.DeleteFFI(context.Background(), "ns1", fftypes.NewUUID())
	assert.Regexp(t, "FF00179", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
