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
	"github.com/stretchr/testify/mock"
)

func TestContractAPIE2EWithDB(t *testing.T) {
	log.SetLevel("trace")

	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new contract API
	apiID := fftypes.NewUUID()
	interfaceID := fftypes.NewUUID()

	contractAPI := &core.ContractAPI{
		ID:          apiID,
		Namespace:   "ns1",
		Name:        "banana",
		NetworkName: "banana-net",
		Interface: &fftypes.FFIReference{
			ID:      interfaceID,
			Name:    "banana",
			Version: "v1.0.0",
		},
		Message: fftypes.NewUUID(),
	}

	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionContractAPIs, core.ChangeEventTypeCreated, "ns1", apiID, mock.Anything).Return()
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionContractAPIs, core.ChangeEventTypeUpdated, "ns1", apiID, mock.Anything).Return()
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionContractAPIs, core.ChangeEventTypeDeleted, "ns1", apiID, mock.Anything).Return()

	existing, err := s.InsertOrGetContractAPI(ctx, contractAPI)
	assert.NoError(t, err)
	assert.Nil(t, existing)

	// Check we get the exact same ContractAPI back
	dataRead, err := s.GetContractAPIByID(ctx, "ns1", apiID)
	assert.NoError(t, err)
	assert.NotNil(t, dataRead)
	assert.Equal(t, *apiID, *dataRead.ID)

	contractAPI.Interface.Version = "v1.1.0"

	err = s.UpsertContractAPI(ctx, contractAPI, database.UpsertOptimizationExisting)
	assert.NoError(t, err)

	// Check we get the exact same ContractAPI back
	dataRead, err = s.GetContractAPIByID(ctx, "ns1", apiID)
	assert.NoError(t, err)
	assert.NotNil(t, dataRead)
	assert.Equal(t, *apiID, *dataRead.ID)

	dataRead, err = s.GetContractAPIByName(ctx, "ns1", "banana")
	assert.NoError(t, err)
	assert.NotNil(t, dataRead)
	assert.Equal(t, *apiID, *dataRead.ID)

	dataRead, err = s.GetContractAPIByNetworkName(ctx, "ns1", "banana-net")
	assert.NoError(t, err)
	assert.NotNil(t, dataRead)
	assert.Equal(t, *apiID, *dataRead.ID)

	// Cannot insert again with same name or network name
	existing, err = s.InsertOrGetContractAPI(ctx, &core.ContractAPI{
		ID:        fftypes.NewUUID(),
		Name:      "banana",
		Namespace: "ns1",
		Interface: &fftypes.FFIReference{
			ID: interfaceID,
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, contractAPI.ID, existing.ID)
	existing, err = s.InsertOrGetContractAPI(ctx, &core.ContractAPI{
		ID:          fftypes.NewUUID(),
		NetworkName: "banana-net",
		Namespace:   "ns1",
		Interface: &fftypes.FFIReference{
			ID: interfaceID,
		},
	})
	assert.NoError(t, err)
	assert.Equal(t, contractAPI.ID, existing.ID)

	// Delete the API
	err = s.DeleteContractAPI(ctx, "ns1", contractAPI.ID)
	assert.NoError(t, err)
}

func TestContractAPIInsertOrGetFailBeginTransaction(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	_, err := s.InsertOrGetContractAPI(context.Background(), &core.ContractAPI{})
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContractAPIInsertOrGetFailInsert(t *testing.T) {
	rows := sqlmock.NewRows([]string{"id", "interface_id", "ledger", "location", "name", "network_name", "namespace", "message_id", "published"})
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectQuery("SELECT .*").WillReturnRows(rows)
	api := &core.ContractAPI{
		Interface: &fftypes.FFIReference{},
	}
	_, err := s.InsertOrGetContractAPI(context.Background(), api)
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContractAPIDBFailBeginTransaction(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertContractAPI(context.Background(), &core.ContractAPI{}, database.UpsertOptimizationNew)
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContractAPIDBFailSelect(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertContractAPI(context.Background(), &core.ContractAPI{}, database.UpsertOptimizationNew)
	assert.Regexp(t, "pop", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContractAPIDBFailInsert(t *testing.T) {
	rows := sqlmock.NewRows([]string{"id", "interface_id", "ledger", "location", "name", "network_name", "namespace", "message_id", "published"})
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectQuery("SELECT .*").WillReturnRows(rows)
	api := &core.ContractAPI{
		Interface: &fftypes.FFIReference{},
	}
	err := s.UpsertContractAPI(context.Background(), api, database.UpsertOptimizationNew)
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContractAPIDBFailUpdate(t *testing.T) {
	rows := sqlmock.NewRows([]string{"id", "interface_id", "ledger", "location", "name", "namespace", "message_id"}).
		AddRow("7e2c001c-e270-4fd7-9e82-9dacee843dc2", "8fcc4938-7d8b-4c00-a71b-1b46837c8ab1", nil, nil, "banana", "ns1", "acfe07a2-117f-46b7-8d47-e3beb7cc382f")
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(rows)
	mock.ExpectQuery("UPDATE .*").WillReturnError(fmt.Errorf("pop"))
	api := &core.ContractAPI{
		Interface: &fftypes.FFIReference{},
	}
	err := s.UpsertContractAPI(context.Background(), api, database.UpsertOptimizationNew)
	assert.Regexp(t, "pop", err)
}

func TestContractAPIDBFailScan(t *testing.T) {
	s, mock := newMockProvider().init()
	apiID := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("only one"))
	_, err := s.GetContractAPIByID(context.Background(), "ns1", apiID)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContractAPIDBSelectFail(t *testing.T) {
	s, mock := newMockProvider().init()
	apiID := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetContractAPIByID(context.Background(), "ns1", apiID)
	assert.Regexp(t, "pop", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContractAPIDBNoRows(t *testing.T) {
	s, mock := newMockProvider().init()
	apiID := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id", "interface_id", "ledger", "location", "name", "network_name", "namespace", "message_id", "published"}))
	_, err := s.GetContractAPIByID(context.Background(), "ns1", apiID)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetContractAPIs(t *testing.T) {
	fb := database.ContractAPIQueryFactory.NewFilter(context.Background())
	s, mock := newMockProvider().init()
	rows := sqlmock.NewRows([]string{"id", "interface_id", "location", "name", "network_name", "namespace", "message_id", "published"}).
		AddRow("7e2c001c-e270-4fd7-9e82-9dacee843dc2", "8fcc4938-7d8b-4c00-a71b-1b46837c8ab1", nil, "banana", "banana", "ns1", "acfe07a2-117f-46b7-8d47-e3beb7cc382f", true)
	mock.ExpectQuery("SELECT .*").WillReturnRows(rows)
	_, _, err := s.GetContractAPIs(context.Background(), "ns1", fb.And())
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetContractAPIsFilterSelectFail(t *testing.T) {
	fb := database.ContractAPIQueryFactory.NewFilter(context.Background())
	s, _ := newMockProvider().init()
	_, _, err := s.GetContractAPIs(context.Background(), "ns1", fb.And(fb.Eq("id", map[bool]bool{true: false})))
	assert.Error(t, err)
}

func TestGetContractAPIsQueryFail(t *testing.T) {
	fb := database.ContractAPIQueryFactory.NewFilter(context.Background())
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, _, err := s.GetContractAPIs(context.Background(), "ns1", fb.And())
	assert.Regexp(t, "pop", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetContractAPIsQueryResultFail(t *testing.T) {
	fb := database.ContractAPIQueryFactory.NewFilter(context.Background())
	s, mock := newMockProvider().init()
	rows := sqlmock.NewRows([]string{"id", "interface_id", "location", "name", "network_name", "namespace", "message_id", "published"}).
		AddRow("7e2c001c-e270-4fd7-9e82-9dacee843dc2", "8fcc4938-7d8b-4c00-a71b-1b46837c8ab1", nil, "apple", "apple", "ns1", "acfe07a2-117f-46b7-8d47-e3beb7cc382f", false).
		AddRow("69851ca3-e9f9-489b-8731-dc6a7d990291", "4db4952e-4669-4243-a387-8f0f609e92bd", nil, "orange", "orange", nil, "acfe07a2-117f-46b7-8d47-e3beb7cc382f", false)
	mock.ExpectQuery("SELECT .*").WillReturnRows(rows)
	_, _, err := s.GetContractAPIs(context.Background(), "ns1", fb.And())
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetContractAPIByName(t *testing.T) {
	s, mock := newMockProvider().init()
	rows := sqlmock.NewRows([]string{"id", "interface_id", "location", "name", "network_name", "namespace", "message_id", "published"}).
		AddRow("7e2c001c-e270-4fd7-9e82-9dacee843dc2", "8fcc4938-7d8b-4c00-a71b-1b46837c8ab1", nil, "banana", "banana", "ns1", "acfe07a2-117f-46b7-8d47-e3beb7cc382f", true)
	mock.ExpectQuery("SELECT .*").WillReturnRows(rows)
	api, err := s.GetContractAPIByName(context.Background(), "ns1", "banana")
	assert.NotNil(t, api)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteContractFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.DeleteContractAPI(context.Background(), "ns1", fftypes.NewUUID())
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteContractAPIFailDelete(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("DELETE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.DeleteContractAPI(context.Background(), "ns1", fftypes.NewUUID())
	assert.Regexp(t, "FF00179", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
