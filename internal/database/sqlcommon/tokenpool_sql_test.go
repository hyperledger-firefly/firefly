// Copyright © 2023 Kaleido, Inc.
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
	"github.com/stretchr/testify/mock"
)

func TestTokenPoolE2EWithDB(t *testing.T) {
	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new token pool entry
	poolID := fftypes.NewUUID()
	pool := &core.TokenPool{
		ID:          poolID,
		Namespace:   "ns1",
		Name:        "my-pool",
		NetworkName: "my-pool",
		Standard:    "ERC1155",
		Type:        core.TokenTypeFungible,
		Locator:     "12345",
		Connector:   "erc1155",
		Symbol:      "COIN",
		Decimals:    18,
		Message:     fftypes.NewUUID(),
		Active:      true,
		TX: core.TransactionRef{
			Type: core.TransactionTypeTokenPool,
			ID:   fftypes.NewUUID(),
		},
		Info: fftypes.JSONObject{
			"pool": "info",
		},
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
		InterfaceFormat: "abi",
	}

	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionTokenPools, core.ChangeEventTypeCreated, "ns1", poolID, mock.Anything).
		Return().Once()
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionTokenPools, core.ChangeEventTypeUpdated, "ns1", poolID, mock.Anything).
		Return().Once()
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionTokenPools, core.ChangeEventTypeDeleted, "ns1", poolID, mock.Anything).
		Return().Once()

	// Insert the pool
	_, err := s.InsertOrGetTokenPool(ctx, pool)
	assert.NoError(t, err)
	assert.NotNil(t, pool.Created)
	poolJson, _ := json.Marshal(&pool)

	// Query back the token pool (by ID)
	poolRead, err := s.GetTokenPoolByID(ctx, "ns1", pool.ID)
	assert.NoError(t, err)
	assert.NotNil(t, poolRead)
	poolReadJson, _ := json.Marshal(&poolRead)
	assert.Equal(t, string(poolJson), string(poolReadJson))

	// Query back the token pool (by name)
	poolRead, err = s.GetTokenPool(ctx, pool.Namespace, pool.Name)
	assert.NoError(t, err)
	assert.NotNil(t, poolRead)
	poolReadJson, _ = json.Marshal(&poolRead)
	assert.Equal(t, string(poolJson), string(poolReadJson))

	// Query back the token pool (by network name)
	poolRead, err = s.GetTokenPoolByNetworkName(ctx, pool.Namespace, pool.NetworkName)
	assert.NoError(t, err)
	assert.NotNil(t, poolRead)
	poolReadJson, _ = json.Marshal(&poolRead)
	assert.Equal(t, string(poolJson), string(poolReadJson))

	// Query back the token pool (by query filter)
	fb := database.TokenPoolQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("id", pool.ID.String()),
		fb.Eq("name", pool.Name),
		fb.Eq("locator", pool.Locator),
		fb.Eq("message", pool.Message),
		fb.Eq("created", pool.Created),
	)
	pools, res, err := s.GetTokenPools(ctx, "ns1", filter.Count(true))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(pools))
	assert.Equal(t, int64(1), *res.TotalCount)
	poolReadJson, _ = json.Marshal(pools[0])
	assert.Equal(t, string(poolJson), string(poolReadJson))

	// Cannot insert again with same ID, name, or network name
	existing, err := s.InsertOrGetTokenPool(ctx, &core.TokenPool{
		ID:        pool.ID,
		Namespace: "ns1",
	})
	assert.NoError(t, err)
	assert.Equal(t, pool.ID, existing.ID)
	existing, err = s.InsertOrGetTokenPool(ctx, &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Name:      "my-pool",
		Namespace: "ns1",
	})
	assert.NoError(t, err)
	assert.Equal(t, pool.ID, existing.ID)
	existing, err = s.InsertOrGetTokenPool(ctx, &core.TokenPool{
		ID:          fftypes.NewUUID(),
		NetworkName: "my-pool",
		Namespace:   "ns1",
	})
	assert.NoError(t, err)
	assert.Equal(t, pool.ID, existing.ID)

	// Update the token pool
	pool.Locator = "67890"
	pool.Type = core.TokenTypeNonFungible
	err = s.UpsertTokenPool(ctx, pool, database.UpsertOptimizationExisting)
	assert.NoError(t, err)

	// Query back the token pool (by ID)
	poolRead, err = s.GetTokenPoolByID(ctx, "ns1", pool.ID)
	assert.NoError(t, err)
	assert.NotNil(t, poolRead)
	poolJson, _ = json.Marshal(&pool)
	poolReadJson, _ = json.Marshal(&poolRead)
	assert.Equal(t, string(poolJson), string(poolReadJson))

	// Delete the token pool
	err = s.DeleteTokenPool(ctx, "ns1", pool.ID)
	assert.NoError(t, err)
}

func TestUpsertTokenPoolFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertTokenPool(context.Background(), &core.TokenPool{}, database.UpsertOptimizationNew)
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertOrGetTokenPoolFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	_, err := s.InsertOrGetTokenPool(context.Background(), &core.TokenPool{})
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertTokenPoolFailSelect(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpsertTokenPool(context.Background(), &core.TokenPool{}, database.UpsertOptimizationNew)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertOrGetTokenPoolFailSelectName(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	_, err := s.InsertOrGetTokenPool(context.Background(), &core.TokenPool{})
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertOrGetTokenPoolFailInsert(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{}))
	mock.ExpectRollback()
	_, err := s.InsertOrGetTokenPool(context.Background(), &core.TokenPool{})
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertTokenPoolFailInsert(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{}))
	mock.ExpectRollback()
	err := s.UpsertTokenPool(context.Background(), &core.TokenPool{}, database.UpsertOptimizationNew)
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertTokenPoolFailUpdate(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("1"))
	mock.ExpectExec("UPDATE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpsertTokenPool(context.Background(), &core.TokenPool{}, database.UpsertOptimizationNew)
	assert.Regexp(t, "FF00178", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertTokenPoolFailCommit(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT .*").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertTokenPool(context.Background(), &core.TokenPool{}, database.UpsertOptimizationNew)
	assert.Regexp(t, "FF00180", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTokenPoolByIDSelectFail(t *testing.T) {
	s, mock := newMockProvider().init()
	poolID := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetTokenPoolByID(context.Background(), "ns1", poolID)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTokenPoolByIDNotFound(t *testing.T) {
	s, mock := newMockProvider().init()
	poolID := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	msg, err := s.GetTokenPoolByID(context.Background(), "ns1", poolID)
	assert.NoError(t, err)
	assert.Nil(t, msg)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTokenPoolByIDScanFail(t *testing.T) {
	s, mock := newMockProvider().init()
	poolID := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("only one"))
	_, err := s.GetTokenPoolByID(context.Background(), "ns1", poolID)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTokenPoolsQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	f := database.TokenPoolQueryFactory.NewFilter(context.Background()).Eq("id", "")
	_, _, err := s.GetTokenPools(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTokenPoolsBuildQueryFail(t *testing.T) {
	s, _ := newMockProvider().init()
	f := database.TokenPoolQueryFactory.NewFilter(context.Background()).Eq("id", map[bool]bool{true: false})
	_, _, err := s.GetTokenPools(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00143.*id", err)
}

func TestGetTokenPoolsScanFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("only one"))
	f := database.TokenPoolQueryFactory.NewFilter(context.Background()).Eq("id", "")
	_, _, err := s.GetTokenPools(context.Background(), "ns1", f)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteTokenPoolFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.DeleteTokenPool(context.Background(), "ns1", fftypes.NewUUID())
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteTokenPoolFailDelete(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("DELETE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.DeleteTokenPool(context.Background(), "ns1", fftypes.NewUUID())
	assert.Regexp(t, "FF00179", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
