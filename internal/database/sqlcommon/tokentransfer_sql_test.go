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
	"github.com/stretchr/testify/mock"
)

func newTestTransfer() *core.TokenTransfer {
	transfer := &core.TokenTransfer{
		LocalID:     fftypes.NewUUID(),
		Type:        core.TokenTransferTypeTransfer,
		Pool:        fftypes.NewUUID(),
		TokenIndex:  "1",
		URI:         "firefly://token/1",
		Connector:   "erc1155",
		Namespace:   "ns1",
		From:        "0x01",
		To:          "0x02",
		ProtocolID:  "12345",
		Message:     fftypes.NewUUID(),
		MessageHash: fftypes.NewRandB32(),
		TX: core.TransactionRef{
			Type: core.TransactionTypeTokenTransfer,
			ID:   fftypes.NewUUID(),
		},
		BlockchainEvent: fftypes.NewUUID(),
	}
	transfer.Amount.Int().SetInt64(10)
	return transfer
}

func TestTokenTransferE2EWithDB(t *testing.T) {
	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new token transfer entry
	transfer := newTestTransfer()

	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionTokenTransfers, core.ChangeEventTypeCreated, transfer.Namespace, transfer.LocalID, mock.Anything).
		Return().Once()
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionTokenTransfers, core.ChangeEventTypeUpdated, transfer.Namespace, transfer.LocalID, mock.Anything).
		Return().Once()

	existing, err := s.InsertOrGetTokenTransfer(ctx, transfer)
	assert.NoError(t, err)
	assert.Nil(t, existing)

	assert.NotNil(t, transfer.Created)
	transferJson, _ := json.Marshal(&transfer)

	// Query back the token transfer (by ID)
	transferRead, err := s.GetTokenTransferByID(ctx, "ns1", transfer.LocalID)
	assert.NoError(t, err)
	assert.NotNil(t, transferRead)
	transferReadJson, _ := json.Marshal(&transferRead)
	assert.Equal(t, string(transferJson), string(transferReadJson))

	// Query back the token transfer (by protocol ID)
	transferRead, err = s.GetTokenTransferByProtocolID(ctx, "ns1", transfer.Pool, transfer.ProtocolID)
	assert.NoError(t, err)
	assert.NotNil(t, transferRead)
	transferReadJson, _ = json.Marshal(&transferRead)
	assert.Equal(t, string(transferJson), string(transferReadJson))

	// Query back the token transfer (by query filter)
	fb := database.TokenTransferQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("pool", transfer.Pool),
		fb.Eq("tokenindex", transfer.TokenIndex),
		fb.Eq("from", transfer.From),
		fb.Eq("to", transfer.To),
		fb.Eq("protocolid", transfer.ProtocolID),
		fb.Eq("created", transfer.Created),
	)
	transfers, res, err := s.GetTokenTransfers(ctx, "ns1", filter.Count(true))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(transfers))
	assert.Equal(t, int64(1), *res.TotalCount)
	transferReadJson, _ = json.Marshal(transfers[0])
	assert.Equal(t, string(transferJson), string(transferReadJson))

	// Delete the token transfer
	err = s.DeleteTokenTransfers(ctx, "ns1", transfer.Pool)
	assert.NoError(t, err)
}

func TestInsertOrGetTokenTransferFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	existing, err := s.InsertOrGetTokenTransfer(context.Background(), &core.TokenTransfer{})
	assert.Regexp(t, "FF00175", err)
	assert.Nil(t, existing)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertOrGetTokenTransferFailSelect(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{}))
	mock.ExpectRollback()
	existing, err := s.InsertOrGetTokenTransfer(context.Background(), &core.TokenTransfer{})
	assert.Regexp(t, "FF00177", err)
	assert.Nil(t, existing)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertOrGetTokenTransferExisting(t *testing.T) {
	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new token transfer entry
	transfer := newTestTransfer()

	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionTokenTransfers, core.ChangeEventTypeCreated, transfer.Namespace, transfer.LocalID, mock.Anything).
		Return().Once()
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionTokenTransfers, core.ChangeEventTypeUpdated, transfer.Namespace, transfer.LocalID, mock.Anything).
		Return().Once()

	existing, err := s.InsertOrGetTokenTransfer(ctx, transfer)
	assert.NoError(t, err)
	assert.Nil(t, existing)

	existing, err = s.InsertOrGetTokenTransfer(ctx, transfer)
	assert.NoError(t, err)
	assert.NotNil(t, existing)
}

func TestGetTokenTransferByIDSelectFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetTokenTransferByID(context.Background(), "ns1", fftypes.NewUUID())
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTokenTransferByIDNotFound(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"protocolid"}))
	msg, err := s.GetTokenTransferByID(context.Background(), "ns1", fftypes.NewUUID())
	assert.NoError(t, err)
	assert.Nil(t, msg)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTokenTransferByIDScanFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"protocolid"}).AddRow("only one"))
	_, err := s.GetTokenTransferByID(context.Background(), "ns1", fftypes.NewUUID())
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTokenTransfersQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	f := database.TokenTransferQueryFactory.NewFilter(context.Background()).Eq("protocolid", "")
	_, _, err := s.GetTokenTransfers(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTokenTransfersBuildQueryFail(t *testing.T) {
	s, _ := newMockProvider().init()
	f := database.TokenTransferQueryFactory.NewFilter(context.Background()).Eq("protocolid", map[bool]bool{true: false})
	_, _, err := s.GetTokenTransfers(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00143.*id", err)
}

func TestGetTokenTransfersScanFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"protocolid"}).AddRow("only one"))
	f := database.TokenTransferQueryFactory.NewFilter(context.Background()).Eq("protocolid", "")
	_, _, err := s.GetTokenTransfers(context.Background(), "ns1", f)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteTokenTransfersFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.DeleteTokenTransfers(context.Background(), "ns1", fftypes.NewUUID())
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteTokenTransfersFailDelete(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("DELETE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.DeleteTokenTransfers(context.Background(), "ns1", fftypes.NewUUID())
	assert.Regexp(t, "FF00179", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
