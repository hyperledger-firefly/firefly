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

package txwriter

import (
	"context"
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/internal/cache"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/database/sqlcommon"
	"github.com/hyperledger-firefly/firefly/internal/operations"
	"github.com/hyperledger-firefly/firefly/internal/txcommon"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/mocks/datamocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func newTestTransactionWriter(t *testing.T, dbCaps *database.Capabilities, mods ...func()) (context.Context, *txWriter, func()) {
	ctx, cancelCtx := context.WithCancel(context.Background())

	coreconfig.Reset()
	config.Set(coreconfig.TransactionWriterCount, 1)
	mdi := &databasemocks.Plugin{}
	mdi.On("Capabilities").Return(dbCaps)
	mrag := mdi.On("RunAsGroup", mock.Anything, mock.Anything)
	mrag.Run(func(args mock.Arguments) {
		ctx := args[0].(context.Context)
		fn := args[1].(func(context.Context) error)
		mrag.Return(fn(ctx))
	}).Maybe()
	mdm := &datamocks.Manager{}
	cm := cache.NewCacheManager(ctx)
	for _, mod := range mods {
		mod()
	}

	txh, err := txcommon.NewTransactionHelper(ctx, "ns1", mdi, mdm, cm)
	assert.NoError(t, err)
	ops, err := operations.NewOperationsManager(ctx, "ns1", mdi, txh, cm)
	assert.NoError(t, err)
	txw := NewTransactionWriter(ctx, "ns1", mdi, txh, ops).(*txWriter)
	return ctx, txw, func() {
		cancelCtx()
		mdi.AssertExpectations(t)
		mdm.AssertExpectations(t)
		txw.Close()
	}
}

func TestWorkerCountForcedToZeroIfNoDBConcurrency(t *testing.T) {
	_, txw, done := newTestTransactionWriter(t, &database.Capabilities{Concurrency: false})
	defer done()
	assert.Zero(t, txw.workerCount)
	txw.Start() // no op
}

func TestWriteNewTransactionClosed(t *testing.T) {
	_, txw, done := newTestTransactionWriter(t, &database.Capabilities{Concurrency: true})
	done()
	// Write under background context, but the write context is closed
	_, err := txw.WriteTransactionAndOps(context.Background(), core.TransactionTypeContractInvoke, "")
	assert.Regexp(t, "FF00154", err)
}

func TestWriteNewTransactionInputContextClosed(t *testing.T) {
	_, txw, done := newTestTransactionWriter(t, &database.Capabilities{Concurrency: true})
	defer done()
	// Write under background context, but the write context is closed
	cancelledCtx, cancelContext := context.WithCancel(context.Background())
	cancelContext()
	_, err := txw.WriteTransactionAndOps(cancelledCtx, core.TransactionTypeContractInvoke, "")
	assert.Regexp(t, "FF00154", err)
}

func TestBatchOfOneSequentialSuccess(t *testing.T) {
	ctx, txw, done := newTestTransactionWriter(t, &database.Capabilities{
		Concurrency: false, // will run inline
	})
	defer done()

	inputOpID := fftypes.NewUUID()
	mdi := txw.database.(*databasemocks.Plugin)
	mdi.On("InsertTransactions", mock.Anything, mock.MatchedBy(func(txns []*core.Transaction) bool {
		return len(txns) == 1
	})).Return(nil)
	mdi.On("InsertEvent", mock.Anything, mock.MatchedBy(func(event *core.Event) bool {
		return event.Type.Equals(core.EventTypeTransactionSubmitted)
	})).Return(nil)
	mdi.On("InsertOperations", mock.Anything, mock.MatchedBy(func(ops []*core.Operation) bool {
		return len(ops) == 1 && ops[0].ID.Equals(inputOpID)
	})).Return(nil)

	tx, err := txw.WriteTransactionAndOps(ctx, core.TransactionTypeContractInvoke, "", &core.Operation{
		ID: inputOpID,
	})
	assert.NoError(t, err)
	assert.NotNil(t, tx)
	assert.NotNil(t, tx.ID) // generated for us

}

func TestBatchOfOneAsyncSuccess(t *testing.T) {
	ctx, txw, done := newTestTransactionWriter(t, &database.Capabilities{
		Concurrency: true,
	})
	defer done()
	txw.Start()

	inputOpID := fftypes.NewUUID()
	mdi := txw.database.(*databasemocks.Plugin)
	mdi.On("InsertTransactions", mock.Anything, mock.MatchedBy(func(txns []*core.Transaction) bool {
		return len(txns) == 1
	})).Return(nil)
	mdi.On("InsertEvent", mock.Anything, mock.MatchedBy(func(event *core.Event) bool {
		return event.Type.Equals(core.EventTypeTransactionSubmitted)
	})).Return(nil)
	mdi.On("InsertOperations", mock.Anything, mock.MatchedBy(func(ops []*core.Operation) bool {
		return len(ops) == 1 && ops[0].ID.Equals(inputOpID)
	})).Return(nil)

	op := &core.Operation{
		ID: inputOpID,
	}
	tx, err := txw.WriteTransactionAndOps(ctx, core.TransactionTypeContractInvoke, "", op)
	assert.NoError(t, err)
	assert.NotNil(t, tx)
	assert.NotNil(t, tx.ID)                // generated for us
	assert.Equal(t, op.Transaction, tx.ID) // assigned for us

}

func TestBatchOfOneInsertOpFail(t *testing.T) {
	ctx, txw, done := newTestTransactionWriter(t, &database.Capabilities{
		Concurrency: false, // will run inline
	})
	defer done()

	inputOpID := fftypes.NewUUID()
	mdi := txw.database.(*databasemocks.Plugin)
	mdi.On("InsertTransactions", mock.Anything, mock.MatchedBy(func(txns []*core.Transaction) bool {
		return len(txns) == 1
	})).Return(nil)
	mdi.On("InsertEvent", mock.Anything, mock.MatchedBy(func(event *core.Event) bool {
		return event.Type.Equals(core.EventTypeTransactionSubmitted)
	})).Return(nil)
	mdi.On("InsertOperations", mock.Anything, mock.MatchedBy(func(ops []*core.Operation) bool {
		return len(ops) == 1 && ops[0].ID.Equals(inputOpID)
	})).Return(fmt.Errorf("pop"))

	_, err := txw.WriteTransactionAndOps(ctx, core.TransactionTypeContractInvoke, "", &core.Operation{
		ID: inputOpID,
	})
	assert.Regexp(t, "pop", err)

}

func TestInsertTxNonIdempotentFail(t *testing.T) {
	ctx, txw, done := newTestTransactionWriter(t, &database.Capabilities{
		Concurrency: false, // will run inline
	})
	defer done()

	inputOpID := fftypes.NewUUID()
	mdi := txw.database.(*databasemocks.Plugin)
	mdi.On("InsertTransactions", mock.Anything, mock.MatchedBy(func(txns []*core.Transaction) bool {
		return len(txns) == 1
	})).Return(fmt.Errorf("pop"))

	_, err := txw.WriteTransactionAndOps(ctx, core.TransactionTypeContractInvoke, "", &core.Operation{
		ID: inputOpID,
	})
	assert.Regexp(t, "pop", err)

}

func TestMixedIdempotencyResult(t *testing.T) {
	ctx, txw, done := newTestTransactionWriter(t, &database.Capabilities{
		Concurrency: false, // will run inline
	})
	defer done()

	mdi := txw.database.(*databasemocks.Plugin)
	var firstTXID *fftypes.UUID
	existingTXID := fftypes.NewUUID()
	mdi.On("InsertTransactions", mock.Anything, mock.MatchedBy(func(txns []*core.Transaction) bool {
		return len(txns) == 2
	})).Run(func(args mock.Arguments) {
		txns := args[1].([]*core.Transaction)
		firstTXID = txns[0].ID // capture this to provide a mixed result
	}).Return(fmt.Errorf("mixed result"))
	mockGet := mdi.On("GetTransactions", mock.Anything, "ns1", mock.Anything)
	mockGet.Run(func(args mock.Arguments) {
		mockGet.Return(
			[]*core.Transaction{
				{ID: firstTXID, IdempotencyKey: "idem1"},
				{ID: existingTXID /* existing */, IdempotencyKey: "idem2"},
			},
			nil, nil)
	})
	mdi.On("InsertEvent", mock.Anything, mock.MatchedBy(func(event *core.Event) bool {
		return event.Type.Equals(core.EventTypeTransactionSubmitted)
	})).Return(nil).Once()

	done1 := make(chan *result, 1)
	done2 := make(chan *result, 1)
	txw.executeBatch(ctx, &txWriterBatch{
		requests: []*request{
			{
				txType:         core.TransactionTypeContractInvoke,
				idempotencyKey: "idem1",
				result:         done1,
			},
			{
				txType:         core.TransactionTypeContractInvoke,
				idempotencyKey: "idem2",
				result:         done2,
			},
		},
	})
	res1 := <-done1
	res2 := <-done2
	assert.NoError(t, res1.err)
	assert.Equal(t, firstTXID, res1.transaction.ID)
	assert.Regexp(t, "FF10431.*idem2", res2.err)
	idemErr, ok := res2.err.(*sqlcommon.IdempotencyError)
	assert.True(t, ok)
	assert.Equal(t, existingTXID, idemErr.ExistingTXID)

}
