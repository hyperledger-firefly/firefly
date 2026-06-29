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
	"database/sql/driver"
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

func TestTransactionE2EWithDB(t *testing.T) {

	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new transaction entry
	transactionID := fftypes.NewUUID()
	transaction := &core.Transaction{
		ID:            transactionID,
		Type:          core.TransactionTypeBatchPin,
		Namespace:     "ns1",
		BlockchainIDs: fftypes.FFStringArray{"tx1"},
	}

	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionTransactions, core.ChangeEventTypeCreated, "ns1", transactionID, mock.Anything).Return()
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionTransactions, core.ChangeEventTypeUpdated, "ns1", transactionID, mock.Anything).Return()

	err := s.InsertTransaction(ctx, transaction)
	assert.NoError(t, err)

	// Check we get the exact same transaction back
	transactionRead, err := s.GetTransactionByID(ctx, "ns1", transactionID)
	assert.NoError(t, err)
	assert.NotNil(t, transactionRead)
	transactionJson, _ := json.Marshal(&transaction)
	transactionReadJson, _ := json.Marshal(&transactionRead)
	assert.Equal(t, string(transactionJson), string(transactionReadJson))

	// Query back the transaction
	fb := database.TransactionQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("id", transaction.ID.String()),
		fb.Gt("created", "0"),
	)
	transactions, res, err := s.GetTransactions(ctx, "ns1", filter.Count(true))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(transactions))
	assert.Equal(t, int64(1), *res.TotalCount)
	transactionReadJson, _ = json.Marshal(transactions[0])
	assert.Equal(t, string(transactionJson), string(transactionReadJson))

	// Negative test on filter
	filter = fb.And(
		fb.Eq("id", transaction.ID.String()),
		fb.Eq("created", "0"),
	)
	transactions, _, err = s.GetTransactions(ctx, "ns1", filter)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(transactions))

	// Update
	up := database.TransactionQueryFactory.NewUpdate(ctx).
		Set("blockchainids", fftypes.FFStringArray{"0x12345", "0x23456"}).
		Set("idempotencykey", "testKey")
	err = s.UpdateTransaction(ctx, "ns1", transaction.ID, up)
	assert.NoError(t, err)

	// Check we cannot insert a duplicate idempotency key
	err = s.InsertTransaction(ctx, &core.Transaction{
		Namespace:      "ns1",
		ID:             fftypes.NewUUID(),
		IdempotencyKey: "testKey",
	})
	assert.Regexp(t, "FF10431", err)

	// Test find updated value
	filter = fb.And(
		fb.Eq("id", transaction.ID.String()),
		fb.Eq("blockchainids", "0x12345,0x23456"),
		fb.Eq("idempotencykey", "testKey"),
	)
	transactions, _, err = s.GetTransactions(ctx, "ns1", filter)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(transactions))
	assert.Equal(t, (core.IdempotencyKey)("testKey"), transactions[0].IdempotencyKey)
}

func TestTransactionE2EInsertManyIdempotency(t *testing.T) {

	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new transaction entry
	txns := make([]*core.Transaction, 5)
	txnsByIdem := make(map[string]*core.Transaction)
	for i := 0; i < len(txns); i++ {
		idem := fmt.Sprintf("idem_%d", i)
		txns[i] = &core.Transaction{
			ID:             fftypes.NewUUID(),
			Type:           core.TransactionTypeBatchPin,
			Namespace:      "ns1",
			BlockchainIDs:  fftypes.FFStringArray{"tx1"},
			IdempotencyKey: core.IdempotencyKey(idem),
		}
		txnsByIdem[idem] = txns[i]
	}

	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionTransactions, core.ChangeEventTypeCreated, "ns1", mock.Anything, mock.Anything).Return()

	// Insert one transaction directly, with a conflicting idempotency key (different UUID)
	existingTxn := &core.Transaction{
		ID:             fftypes.NewUUID(),
		Type:           core.TransactionTypeBatchPin,
		Namespace:      "ns1",
		BlockchainIDs:  fftypes.FFStringArray{"tx1"},
		IdempotencyKey: core.IdempotencyKey("idem_2"),
	}
	err := s.InsertTransaction(ctx, existingTxn)
	assert.NoError(t, err)

	// Insert the whole set
	err = s.RunAsGroup(ctx, func(ctx context.Context) error {
		err := s.InsertTransactions(ctx, txns)
		assert.Regexp(t, "FF10431.*idem_2", err)
		return nil
	})
	assert.NoError(t, err)

	// Check we find every transaction
	fb := database.TransactionQueryFactory.NewFilter(ctx)
	idempotencyKeys := make([]driver.Value, len(txns))
	for i := 0; i < len(txns); i++ {
		idempotencyKeys[i] = fmt.Sprintf("idem_%d", i)
	}
	resolvedTX, _, err := s.GetTransactions(ctx, "ns1", fb.In("idempotencykey", idempotencyKeys))
	assert.NoError(t, err)
	assert.Len(t, resolvedTX, len(txns))
	for _, txn := range resolvedTX {
		if txn.IdempotencyKey == "idem_2" {
			assert.Equal(t, existingTxn.ID, txn.ID)
		} else {
			assert.Equal(t, txnsByIdem[string(txn.IdempotencyKey)].ID, txn.ID)
		}
	}

}

func TestInsertTransactionFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.InsertTransaction(context.Background(), &core.Transaction{})
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertTransactionFailInsert(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	transactionID := fftypes.NewUUID()
	err := s.InsertTransaction(context.Background(), &core.Transaction{ID: transactionID})
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestEmptyResultInsertIdempotencyKey(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT .*").WillReturnResult(sqlmock.NewResult(-1, 0))
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{}))
	mock.ExpectRollback()
	transactionID := fftypes.NewUUID()
	err := s.InsertTransaction(context.Background(), &core.Transaction{ID: transactionID, IdempotencyKey: "idem1"})
	assert.Regexp(t, "FF10432", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertTransactionFailCommit(t *testing.T) {
	s, mock := newMockProvider().init()
	transactionID := fftypes.NewUUID()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT .*").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(fmt.Errorf("pop"))
	err := s.InsertTransaction(context.Background(), &core.Transaction{ID: transactionID})
	assert.Regexp(t, "FF00180", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTransactionByIDSelectFail(t *testing.T) {
	s, mock := newMockProvider().init()
	transactionID := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetTransactionByID(context.Background(), "ns1", transactionID)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTransactionByIDNotFound(t *testing.T) {
	s, mock := newMockProvider().init()
	transactionID := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	msg, err := s.GetTransactionByID(context.Background(), "ns1", transactionID)
	assert.NoError(t, err)
	assert.Nil(t, msg)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTransactionByIDScanFail(t *testing.T) {
	s, mock := newMockProvider().init()
	transactionID := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("only one"))
	_, err := s.GetTransactionByID(context.Background(), "ns1", transactionID)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTransactionsQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	f := database.TransactionQueryFactory.NewFilter(context.Background()).Eq("id", "")
	_, _, err := s.GetTransactions(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTransactionsBuildQueryFail(t *testing.T) {
	s, _ := newMockProvider().init()
	f := database.TransactionQueryFactory.NewFilter(context.Background()).Eq("id", map[bool]bool{true: false})
	_, _, err := s.GetTransactions(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00143.*id", err)
}

func TestGetTransactionsReadMessageFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("only one"))
	f := database.TransactionQueryFactory.NewFilter(context.Background()).Eq("id", "")
	_, _, err := s.GetTransactions(context.Background(), "ns1", f)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestTransactionUpdateBeginFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	u := database.TransactionQueryFactory.NewUpdate(context.Background()).Set("id", "anything")
	err := s.UpdateTransaction(context.Background(), "ns1", fftypes.NewUUID(), u)
	assert.Regexp(t, "FF00175", err)
}

func TestTransactionUpdateBuildQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	u := database.TransactionQueryFactory.NewUpdate(context.Background()).Set("id", map[bool]bool{true: false})
	err := s.UpdateTransaction(context.Background(), "ns1", fftypes.NewUUID(), u)
	assert.Regexp(t, "FF00143.*id", err)
}

func TestTransactionUpdateFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	u := database.TransactionQueryFactory.NewUpdate(context.Background()).Set("id", fftypes.NewUUID())
	err := s.UpdateTransaction(context.Background(), "ns1", fftypes.NewUUID(), u)
	assert.Regexp(t, "FF00178", err)
}

func TestInsertTransactionsBeginFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.InsertTransactions(context.Background(), []*core.Transaction{})
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

func TestInsertTransactionsMultiRowOK(t *testing.T) {
	s := newMockProvider()
	s.multiRowInsert = true
	s.fakePSQLInsert = true
	s, mock := s.init()

	tx1 := &core.Transaction{ID: fftypes.NewUUID(), Namespace: "ns1"}
	tx2 := &core.Transaction{ID: fftypes.NewUUID(), Namespace: "ns1"}
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionTransactions, core.ChangeEventTypeCreated, "ns1", tx1.ID)
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionTransactions, core.ChangeEventTypeCreated, "ns1", tx2.ID)

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT.*transactions").WillReturnRows(sqlmock.NewRows([]string{s.SequenceColumn()}).
		AddRow(int64(1001)).
		AddRow(int64(1002)),
	)
	mock.ExpectCommit()
	err := s.RunAsGroup(context.Background(), func(ctx context.Context) error {
		return s.InsertTransactions(ctx, []*core.Transaction{tx1, tx2})
	})
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

func TestInsertTransactionsOutsideTXFail(t *testing.T) {
	s := newMockProvider()
	s, mock := s.init()
	mock.ExpectBegin()
	tx1 := &core.Transaction{ID: fftypes.NewUUID(), Namespace: "ns1"}
	err := s.InsertTransactions(context.Background(), []*core.Transaction{tx1})
	assert.Regexp(t, "FF10456", err)
}

func TestInsertTransactionsMultiRowFail(t *testing.T) {
	s := newMockProvider()
	s.multiRowInsert = true
	s.fakePSQLInsert = true
	s, mock := s.init()
	tx1 := &core.Transaction{ID: fftypes.NewUUID(), Namespace: "ns1"}
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT.*").WillReturnError(fmt.Errorf("pop"))
	err := s.RunAsGroup(context.Background(), func(ctx context.Context) error {
		return s.InsertTransactions(ctx, []*core.Transaction{tx1})
	})
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

func TestInsertTransactionsSingleRowFail(t *testing.T) {
	s, mock := newMockProvider().init()
	tx1 := &core.Transaction{ID: fftypes.NewUUID(), Namespace: "ns1"}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT.*").WillReturnError(fmt.Errorf("pop"))
	err := s.RunAsGroup(context.Background(), func(ctx context.Context) error {
		return s.InsertTransactions(ctx, []*core.Transaction{tx1})
	})
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}
