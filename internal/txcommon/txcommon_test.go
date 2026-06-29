// Copyright © 2022 Kaleido, Inc.
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

package txcommon

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/internal/cache"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/mocks/cachemocks"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/mocks/datamocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type testTransactionHelper struct {
	transactionHelper

	mdi *databasemocks.Plugin
	mdm *datamocks.Manager
}

func (tth *testTransactionHelper) cleanup(t *testing.T) {
	tth.mdi.AssertExpectations(t)
	tth.mdm.AssertExpectations(t)
}

func NewTestTransactionHelper() (*testTransactionHelper, cache.CInterface, cache.CInterface) {
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	t := transactionHelper{
		namespace: "ns1",
		database:  mdi,
		data:      mdm,
	}
	t.transactionCache = cache.NewUmanagedCache(context.Background(), config.GetByteSize(coreconfig.CacheTransactionSize), config.GetDuration(coreconfig.CacheTransactionTTL))
	t.blockchainEventCache = cache.NewUmanagedCache(context.Background(), config.GetByteSize(coreconfig.CacheBlockchainEventLimit), config.GetDuration(coreconfig.CacheBlockchainEventTTL))
	return &testTransactionHelper{
		transactionHelper: t,
		mdi:               mdi,
		mdm:               mdm,
	}, t.transactionCache, t.blockchainEventCache
}

func TestSubmitNewTransactionOK(t *testing.T) {

	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, err := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	var txidInserted *fftypes.UUID
	mdi.On("InsertTransaction", ctx, mock.MatchedBy(func(transaction *core.Transaction) bool {
		txidInserted = transaction.ID
		assert.NotNil(t, transaction.ID)
		assert.Equal(t, "ns1", transaction.Namespace)
		assert.Equal(t, core.TransactionTypeBatchPin, transaction.Type)
		assert.Empty(t, transaction.BlockchainIDs)
		return true
	})).Return(nil)
	mdi.On("InsertEvent", ctx, mock.MatchedBy(func(e *core.Event) bool {
		return e.Type == core.EventTypeTransactionSubmitted && e.Reference.Equals(txidInserted)
	})).Return(nil)

	txidReturned, err := txHelper.SubmitNewTransaction(ctx, core.TransactionTypeBatchPin, "idem1")
	assert.NoError(t, err)
	assert.Equal(t, *txidInserted, *txidReturned)

	mdi.AssertExpectations(t)

}

func TestSubmitNewTransactionFail(t *testing.T) {

	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	mdi.On("InsertTransaction", ctx, mock.Anything).Return(fmt.Errorf("pop"))

	_, err := txHelper.SubmitNewTransaction(ctx, core.TransactionTypeBatchPin, "idem1")
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)

}

func TestSubmitNewTransactionEventFail(t *testing.T) {

	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	mdi.On("InsertTransaction", ctx, mock.Anything).Return(nil)
	mdi.On("InsertEvent", ctx, mock.Anything).Return(fmt.Errorf("pop"))

	_, err := txHelper.SubmitNewTransaction(ctx, core.TransactionTypeBatchPin, "idem1")
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)

}

func TestCacheInitFail(t *testing.T) {
	cacheInitErr := errors.New("Initialization error.")
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	ns := "ns1"
	tErrcmi := &cachemocks.Manager{}
	tErrcmi.On("GetCache", cache.NewCacheConfig(
		ctx,
		coreconfig.CacheTransactionSize,
		coreconfig.CacheTransactionTTL,
		ns,
	)).Return(nil, cacheInitErr).Once()
	defer tErrcmi.AssertExpectations(t)
	_, err := NewTransactionHelper(ctx, ns, mdi, mdm, tErrcmi)
	assert.Equal(t, cacheInitErr, err)

	bErrcmi := &cachemocks.Manager{}
	bErrcmi.On("GetCache", cache.NewCacheConfig(
		ctx,
		coreconfig.CacheTransactionSize,
		coreconfig.CacheTransactionTTL,
		ns,
	)).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil).Once()
	bErrcmi.On("GetCache", cache.NewCacheConfig(
		ctx,
		coreconfig.CacheBlockchainEventLimit,
		coreconfig.CacheBlockchainEventTTL,
		ns,
	)).Return(nil, cacheInitErr).Once()
	defer bErrcmi.AssertExpectations(t)
	_, err = NewTransactionHelper(ctx, ns, mdi, mdm, bErrcmi)
	assert.Equal(t, cacheInitErr, err)
}

func TestPersistTransactionNew(t *testing.T) {

	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	txid := fftypes.NewUUID()
	mdi.On("GetTransactionByID", ctx, "ns1", txid).Return(nil, nil)
	mdi.On("InsertTransaction", ctx, mock.MatchedBy(func(transaction *core.Transaction) bool {
		assert.Equal(t, txid, transaction.ID)
		assert.Equal(t, "ns1", transaction.Namespace)
		assert.Equal(t, core.TransactionTypeBatchPin, transaction.Type)
		assert.Equal(t, fftypes.FFStringArray{"0x222222"}, transaction.BlockchainIDs)
		return true
	})).Return(nil)

	valid, err := txHelper.PersistTransaction(ctx, txid, core.TransactionTypeBatchPin, "0x222222")
	assert.NoError(t, err)
	assert.True(t, valid)

	mdi.AssertExpectations(t)

}

func TestPersistTransactionNewInsertFail(t *testing.T) {

	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	txid := fftypes.NewUUID()
	mdi.On("GetTransactionByID", ctx, "ns1", txid).Return(nil, nil)
	mdi.On("InsertTransaction", ctx, mock.Anything).Return(fmt.Errorf("pop"))

	valid, err := txHelper.PersistTransaction(ctx, txid, core.TransactionTypeBatchPin, "0x222222")
	assert.Regexp(t, "pop", err)
	assert.False(t, valid)

	mdi.AssertExpectations(t)

}

func TestPersistTransactionExistingAddBlockchainID(t *testing.T) {

	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cache := cache.NewUmanagedCache(ctx, 1024, 5*time.Minute)
	cmi.On("GetCache", mock.Anything).Return(cache, nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	txid := fftypes.NewUUID()
	mdi.On("GetTransactionByID", ctx, "ns1", txid).Return(&core.Transaction{
		ID:            txid,
		Namespace:     "ns1",
		Type:          core.TransactionTypeBatchPin,
		Created:       fftypes.Now(),
		BlockchainIDs: fftypes.FFStringArray{"0x111111"},
	}, nil)
	mdi.On("UpdateTransaction", ctx, "ns1", txid, mock.Anything).Return(nil)

	valid, err := txHelper.PersistTransaction(ctx, txid, core.TransactionTypeBatchPin, "0x222222")
	assert.NoError(t, err)
	assert.True(t, valid)

	cachedTX := cache.Get(txid.String()).(*core.Transaction)
	assert.Equal(t, fftypes.FFStringArray{"0x111111", "0x222222"}, cachedTX.BlockchainIDs)

	mdi.AssertExpectations(t)

}

func TestPersistTransactionExistingUpdateFail(t *testing.T) {

	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	txid := fftypes.NewUUID()
	mdi.On("GetTransactionByID", ctx, "ns1", txid).Return(&core.Transaction{
		ID:            txid,
		Namespace:     "ns1",
		Type:          core.TransactionTypeBatchPin,
		Created:       fftypes.Now(),
		BlockchainIDs: fftypes.FFStringArray{"0x111111"},
	}, nil)
	mdi.On("UpdateTransaction", ctx, "ns1", txid, mock.Anything).Return(fmt.Errorf("pop"))

	valid, err := txHelper.PersistTransaction(ctx, txid, core.TransactionTypeBatchPin, "0x222222")
	assert.Regexp(t, "pop", err)
	assert.False(t, valid)

	mdi.AssertExpectations(t)

}

func TestPersistTransactionExistingNoChange(t *testing.T) {

	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	ns := "ns1"
	txHelper, _ := NewTransactionHelper(ctx, ns, mdi, mdm, cmi)
	cmi.AssertCalled(t, "GetCache", cache.NewCacheConfig(
		ctx,
		coreconfig.CacheTransactionSize,
		coreconfig.CacheTransactionTTL,
		ns,
	))
	cmi.AssertCalled(t, "GetCache", cache.NewCacheConfig(
		ctx,
		coreconfig.CacheBlockchainEventLimit,
		coreconfig.CacheBlockchainEventTTL,
		ns,
	))

	txid := fftypes.NewUUID()
	mdi.On("GetTransactionByID", ctx, "ns1", txid).Return(&core.Transaction{
		ID:            txid,
		Namespace:     "ns1",
		Type:          core.TransactionTypeBatchPin,
		Created:       fftypes.Now(),
		BlockchainIDs: fftypes.FFStringArray{"0x111111"},
	}, nil)

	valid, err := txHelper.PersistTransaction(ctx, txid, core.TransactionTypeBatchPin, "0x111111")
	assert.NoError(t, err)
	assert.True(t, valid)

	mdi.AssertExpectations(t)

}

func TestPersistTransactionExistingNoBlockchainID(t *testing.T) {

	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	txid := fftypes.NewUUID()
	mdi.On("GetTransactionByID", ctx, "ns1", txid).Return(&core.Transaction{
		ID:            txid,
		Namespace:     "ns1",
		Type:          core.TransactionTypeBatchPin,
		Created:       fftypes.Now(),
		BlockchainIDs: fftypes.FFStringArray{"0x111111"},
	}, nil)

	valid, err := txHelper.PersistTransaction(ctx, txid, core.TransactionTypeBatchPin, "")
	assert.NoError(t, err)
	assert.True(t, valid)

	mdi.AssertExpectations(t)

}

func TestPersistTransactionExistingLookupFail(t *testing.T) {

	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	txid := fftypes.NewUUID()
	mdi.On("GetTransactionByID", ctx, "ns1", txid).Return(nil, fmt.Errorf("pop"))

	valid, err := txHelper.PersistTransaction(ctx, txid, core.TransactionTypeBatchPin, "")
	assert.Regexp(t, "pop", err)
	assert.False(t, valid)

	mdi.AssertExpectations(t)

}

func TestPersistTransactionExistingMismatchType(t *testing.T) {

	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	txid := fftypes.NewUUID()
	mdi.On("GetTransactionByID", ctx, "ns1", txid).Return(&core.Transaction{
		ID:            txid,
		Namespace:     "ns1",
		Type:          core.TransactionTypeContractInvoke,
		Created:       fftypes.Now(),
		BlockchainIDs: fftypes.FFStringArray{"0x111111"},
	}, nil)

	valid, err := txHelper.PersistTransaction(ctx, txid, core.TransactionTypeBatchPin, "")
	assert.NoError(t, err)
	assert.False(t, valid)

	mdi.AssertExpectations(t)

}

func TestAddBlockchainTX(t *testing.T) {

	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	tx := &core.Transaction{
		ID:            fftypes.NewUUID(),
		Namespace:     "ns1",
		Type:          core.TransactionTypeContractInvoke,
		Created:       fftypes.Now(),
		BlockchainIDs: fftypes.FFStringArray{"0x111111"},
	}
	mdi.On("UpdateTransaction", ctx, "ns1", tx.ID, mock.MatchedBy(func(u ffapi.Update) bool {
		info, _ := u.Finalize()
		assert.Equal(t, 1, len(info.SetOperations))
		assert.Equal(t, "blockchainids", info.SetOperations[0].Field)
		val, _ := info.SetOperations[0].Value.Value()
		assert.Equal(t, "0x111111,abc", val)
		return true
	})).Return(nil)

	err := txHelper.AddBlockchainTX(ctx, tx, "abc")
	assert.NoError(t, err)

	mdi.AssertExpectations(t)

}

func TestAddBlockchainTXUpdateFail(t *testing.T) {

	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	tx := &core.Transaction{
		ID:            fftypes.NewUUID(),
		Namespace:     "ns1",
		Type:          core.TransactionTypeContractInvoke,
		Created:       fftypes.Now(),
		BlockchainIDs: fftypes.FFStringArray{"0x111111"},
	}
	mdi.On("UpdateTransaction", ctx, "ns1", tx.ID, mock.Anything).Return(fmt.Errorf("pop"))

	err := txHelper.AddBlockchainTX(ctx, tx, "abc")
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)

}

func TestAddBlockchainTXUnchanged(t *testing.T) {

	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	tx := &core.Transaction{
		ID:            fftypes.NewUUID(),
		Namespace:     "ns1",
		Type:          core.TransactionTypeContractInvoke,
		Created:       fftypes.Now(),
		BlockchainIDs: fftypes.FFStringArray{"0x111111"},
	}

	err := txHelper.AddBlockchainTX(ctx, tx, "0x111111")
	assert.NoError(t, err)

}

func TestGetTransactionByIDCached(t *testing.T) {

	txHelper, _, _ := NewTestTransactionHelper()
	defer txHelper.cleanup(t)
	ctx := context.Background()

	txid := fftypes.NewUUID()
	txHelper.mdi.On("GetTransactionByID", ctx, "ns1", txid).Return(&core.Transaction{
		ID:            txid,
		Namespace:     "ns1",
		Type:          core.TransactionTypeContractInvoke,
		Created:       fftypes.Now(),
		BlockchainIDs: fftypes.FFStringArray{"0x111111"},
	}, nil).Once()

	tx, err := txHelper.GetTransactionByIDCached(ctx, txid)
	assert.NoError(t, err)
	assert.Equal(t, txid, tx.ID)

	// second attempt is cached
	tx, err = txHelper.GetTransactionByIDCached(ctx, txid)
	assert.NoError(t, err)
	assert.Equal(t, txid, tx.ID)

}

func TestGetTransactionByIDCachedFail(t *testing.T) {

	txHelper, _, _ := NewTestTransactionHelper()
	defer txHelper.cleanup(t)
	ctx := context.Background()

	txid := fftypes.NewUUID()
	txHelper.mdi.On("GetTransactionByID", ctx, "ns1", txid).Return(nil, fmt.Errorf("pop"))

	_, err := txHelper.GetTransactionByIDCached(ctx, txid)
	assert.EqualError(t, err, "pop")

}

func TestGetBlockchainEventByIDCached(t *testing.T) {
	config.Set(coreconfig.CacheEnabled, true)
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	evID := fftypes.NewUUID()
	mdi.On("GetBlockchainEventByID", ctx, "ns1", evID).Return(&core.BlockchainEvent{
		ID:        evID,
		Namespace: "ns1",
	}, nil).Once()

	chainEvent, err := txHelper.GetBlockchainEventByIDCached(ctx, evID)
	assert.NoError(t, err)
	assert.Equal(t, evID, chainEvent.ID)

	chainEvent, err = txHelper.GetBlockchainEventByIDCached(ctx, evID)
	assert.NoError(t, err)
	assert.Equal(t, evID, chainEvent.ID)

	mdi.AssertExpectations(t)

}

func TestGetBlockchainEventByIDNil(t *testing.T) {

	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	evID := fftypes.NewUUID()
	mdi.On("GetBlockchainEventByID", ctx, "ns1", evID).Return(nil, nil)

	chainEvent, err := txHelper.GetBlockchainEventByIDCached(ctx, evID)
	assert.NoError(t, err)
	assert.Nil(t, chainEvent)

	mdi.AssertExpectations(t)

}

func TestGetBlockchainEventByIDErr(t *testing.T) {

	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	evID := fftypes.NewUUID()
	mdi.On("GetBlockchainEventByID", ctx, "ns1", evID).Return(nil, fmt.Errorf("pop"))

	_, err := txHelper.GetBlockchainEventByIDCached(ctx, evID)
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)

}

func TestInsertGetBlockchainEventCached(t *testing.T) {

	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)
	evID := fftypes.NewUUID()
	chainEvent := &core.BlockchainEvent{
		ID:        evID,
		Namespace: "ns1",
	}
	mdi.On("InsertOrGetBlockchainEvent", ctx, chainEvent).Return(nil, nil)

	_, err := txHelper.InsertOrGetBlockchainEvent(ctx, chainEvent)
	assert.NoError(t, err)

	cached, err := txHelper.GetBlockchainEventByIDCached(ctx, evID)
	assert.NoError(t, err)
	assert.Equal(t, chainEvent, cached)
	mdi.AssertExpectations(t)

}

func TestInsertGetBlockchainEventDuplicate(t *testing.T) {

	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)
	evID := fftypes.NewUUID()
	chainEvent := &core.BlockchainEvent{
		ID:        evID,
		Namespace: "ns1",
	}
	existingEvent := &core.BlockchainEvent{}
	mdi.On("InsertOrGetBlockchainEvent", ctx, chainEvent).Return(existingEvent, nil)

	result, err := txHelper.InsertOrGetBlockchainEvent(ctx, chainEvent)
	assert.NoError(t, err)
	assert.Equal(t, existingEvent, result)

	mdi.AssertExpectations(t)

}

func TestInsertGetBlockchainEventErr(t *testing.T) {

	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)
	evID := fftypes.NewUUID()
	chainEvent := &core.BlockchainEvent{
		ID:        evID,
		Namespace: "ns1",
	}
	mdi.On("InsertOrGetBlockchainEvent", ctx, chainEvent).Return(nil, fmt.Errorf("pop"))

	_, err := txHelper.InsertOrGetBlockchainEvent(ctx, chainEvent)
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)

}

func TestInsertNewBlockchainEventsOptimized(t *testing.T) {

	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	evID := fftypes.NewUUID()
	chainEvent := &core.BlockchainEvent{
		ID:        evID,
		Namespace: "ns1",
	}
	mdi.On("InsertBlockchainEvents", ctx, []*core.BlockchainEvent{chainEvent}, mock.Anything).
		Run(func(args mock.Arguments) {
			cb := args[2].(database.PostCompletionHook)
			cb()
		}).
		Return(nil)

	_, err := txHelper.InsertNewBlockchainEvents(ctx, []*core.BlockchainEvent{chainEvent})
	assert.NoError(t, err)

	cached, err := txHelper.GetBlockchainEventByIDCached(ctx, evID)
	assert.NoError(t, err)
	assert.Equal(t, chainEvent, cached)

	mdi.AssertExpectations(t)

}

func TestInsertNewBlockchainEventsEventCached(t *testing.T) {

	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	evID := fftypes.NewUUID()
	chainEvent := &core.BlockchainEvent{
		ID:        evID,
		Namespace: "ns1",
	}
	mdi.On("InsertBlockchainEvents", ctx, []*core.BlockchainEvent{chainEvent}, mock.Anything).Return(fmt.Errorf("optimization bypass"))
	mdi.On("InsertOrGetBlockchainEvent", ctx, chainEvent).Return(nil, nil)

	_, err := txHelper.InsertNewBlockchainEvents(ctx, []*core.BlockchainEvent{chainEvent})
	assert.NoError(t, err)

	cached, err := txHelper.GetBlockchainEventByIDCached(ctx, evID)
	assert.NoError(t, err)
	assert.Equal(t, chainEvent, cached)

	mdi.AssertExpectations(t)

}

func TestInsertBlockchainEventDuplicate(t *testing.T) {

	txHelper, _, _ := NewTestTransactionHelper()
	defer txHelper.cleanup(t)
	ctx := context.Background()

	evID := fftypes.NewUUID()
	chainEvent := &core.BlockchainEvent{
		ID:        evID,
		Namespace: "ns1",
	}
	existingEvent := &core.BlockchainEvent{}
	txHelper.mdi.On("InsertBlockchainEvents", ctx, []*core.BlockchainEvent{chainEvent}, mock.Anything).Return(fmt.Errorf("optimization bypass"))
	txHelper.mdi.On("InsertOrGetBlockchainEvent", ctx, chainEvent).Return(existingEvent, nil)
	txHelper.mdi.On("GetEvents", ctx, "ns1", mock.Anything).Return([]*core.Event{{}}, nil, nil)

	result, err := txHelper.InsertNewBlockchainEvents(ctx, []*core.BlockchainEvent{chainEvent})
	assert.NoError(t, err)
	assert.Empty(t, result)

}

func TestInsertBlockchainEventFailEventQuery(t *testing.T) {

	txHelper, _, _ := NewTestTransactionHelper()
	defer txHelper.cleanup(t)
	ctx := context.Background()

	chainEvent := &core.BlockchainEvent{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	existingEvent := &core.BlockchainEvent{}
	txHelper.mdi.On("InsertBlockchainEvents", ctx, []*core.BlockchainEvent{chainEvent}, mock.Anything).Return(fmt.Errorf("optimization bypass"))
	txHelper.mdi.On("InsertOrGetBlockchainEvent", ctx, chainEvent).Return(existingEvent, nil)
	txHelper.mdi.On("GetEvents", ctx, "ns1", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	_, err := txHelper.InsertNewBlockchainEvents(ctx, []*core.BlockchainEvent{chainEvent})
	assert.EqualError(t, err, "pop")

}

func TestInsertBlockchainEventPartialBatch(t *testing.T) {

	txHelper, _, _ := NewTestTransactionHelper()
	defer txHelper.cleanup(t)
	ctx := context.Background()

	chainEvent := &core.BlockchainEvent{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	existingEvent := &core.BlockchainEvent{}
	txHelper.mdi.On("InsertBlockchainEvents", ctx, []*core.BlockchainEvent{chainEvent}, mock.Anything).Return(fmt.Errorf("optimization bypass"))
	txHelper.mdi.On("InsertOrGetBlockchainEvent", ctx, chainEvent).Return(existingEvent, nil)
	txHelper.mdi.On("GetEvents", ctx, "ns1", mock.Anything).Return([]*core.Event{}, nil, nil)

	result, err := txHelper.InsertNewBlockchainEvents(ctx, []*core.BlockchainEvent{chainEvent})
	assert.NoError(t, err)
	assert.Len(t, result, 1)
	assert.Equal(t, existingEvent, result[0])

}

func TestInsertBlockchainEventErr(t *testing.T) {

	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	evID := fftypes.NewUUID()
	chainEvent := &core.BlockchainEvent{
		ID:        evID,
		Namespace: "ns1",
	}
	mdi.On("InsertBlockchainEvents", ctx, []*core.BlockchainEvent{chainEvent}, mock.Anything).Return(fmt.Errorf("optimization bypass"))
	mdi.On("InsertOrGetBlockchainEvent", ctx, chainEvent).Return(nil, fmt.Errorf("pop"))

	_, err := txHelper.InsertNewBlockchainEvents(ctx, []*core.BlockchainEvent{chainEvent})
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)

}

func TestFindOperationInTransaction(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	txID := fftypes.NewUUID()
	ops := []*core.Operation{{
		ID: fftypes.NewUUID(),
	}}
	mdi.On("GetOperations", ctx, "ns1", mock.Anything).Return(ops, nil, nil)

	result, err := txHelper.FindOperationInTransaction(ctx, txID, core.OpTypeTokenTransfer)

	assert.NoError(t, err)
	assert.Equal(t, ops[0].ID, result.ID)

	mdi.AssertExpectations(t)
}

func TestFindOperationInTransactionFail(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	txID := fftypes.NewUUID()
	mdi.On("GetOperations", ctx, "ns1", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	_, err := txHelper.FindOperationInTransaction(ctx, txID, core.OpTypeTokenTransfer)

	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
}

func TestSubmitNewTransactionBatchAllPlainOk(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	batch := make([]*BatchedTransactionInsert, 5)
	for i := 0; i < len(batch); i++ {
		batch[i] = &BatchedTransactionInsert{
			Input: TransactionInsertInput{
				Type:           core.BatchTypePrivate,
				IdempotencyKey: "", // this makes them all plain
			},
		}
	}
	mdi.On("InsertTransactions", ctx, mock.MatchedBy(func(transactions []*core.Transaction) bool {
		return len(transactions) == len(batch)
	})).Return(nil).Once()
	mdi.On("InsertEvent", ctx, mock.MatchedBy(func(e *core.Event) bool {
		return e.Type == core.EventTypeTransactionSubmitted
	})).Return(nil).Times(len(batch))

	err := txHelper.SubmitNewTransactionBatch(ctx, "ns1", batch)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestSubmitNewTransactionBatchAllPlainFail(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	batch := make([]*BatchedTransactionInsert, 5)
	for i := 0; i < len(batch); i++ {
		batch[i] = &BatchedTransactionInsert{
			Input: TransactionInsertInput{
				Type:           core.BatchTypePrivate,
				IdempotencyKey: "", // this makes them all plain
			},
		}
	}
	mdi.On("InsertTransactions", ctx, mock.MatchedBy(func(transactions []*core.Transaction) bool {
		return len(transactions) == len(batch)
	})).Return(fmt.Errorf("pop")).Once()

	err := txHelper.SubmitNewTransactionBatch(ctx, "ns1", batch)
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
}

func TestSubmitNewTransactionBatchMixSucceedOptimized(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	batch := make([]*BatchedTransactionInsert, 6)
	for i := 0; i < len(batch); i++ {
		batch[i] = &BatchedTransactionInsert{
			Input: TransactionInsertInput{
				Type: core.BatchTypePrivate,
			},
		}
		if i%2 == 0 {
			batch[i].Input.IdempotencyKey = core.IdempotencyKey(fmt.Sprintf("idem_%.3d", i))
		}
	}
	mdi.On("InsertTransactions", ctx, mock.MatchedBy(func(transactions []*core.Transaction) bool {
		return len(transactions) == len(batch)/2
	})).Return(nil).Twice() // once for non-idempotent, once for idempotent
	mdi.On("InsertEvent", ctx, mock.MatchedBy(func(e *core.Event) bool {
		return e.Type == core.EventTypeTransactionSubmitted
	})).Return(nil).Times(len(batch))

	err := txHelper.SubmitNewTransactionBatch(ctx, "ns1", batch)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestSubmitNewTransactionBatchAllIdempotentDup(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	batch := make([]*BatchedTransactionInsert, 3)
	for i := 0; i < len(batch); i++ {
		batch[i] = &BatchedTransactionInsert{
			Input: TransactionInsertInput{
				Type:           core.BatchTypePrivate,
				IdempotencyKey: core.IdempotencyKey(fmt.Sprintf("idem_%.3d", i)),
			},
		}
	}
	mdi.On("InsertTransactions", ctx, mock.MatchedBy(func(transactions []*core.Transaction) bool {
		return len(transactions) == len(batch)
	})).Return(fmt.Errorf("go check for dups")).Once()
	mdi.On("GetTransactions", ctx, "ns1", mock.Anything).Return(
		[]*core.Transaction{
			{ID: fftypes.NewUUID(), IdempotencyKey: "idem_002"},
			{ID: fftypes.NewUUID(), IdempotencyKey: "idem_000"},
			{ID: fftypes.NewUUID(), IdempotencyKey: "idem_001"},
		},
		nil, nil,
	).Once()

	err := txHelper.SubmitNewTransactionBatch(ctx, "ns1", batch)
	assert.NoError(t, err)

	for i := 0; i < len(batch); i++ {
		assert.Regexp(t, "FF10431.*"+batch[i].Input.IdempotencyKey, batch[i].Output.IdempotencyError)
	}

	mdi.AssertExpectations(t)
}

func TestSubmitNewTransactionBatchQueryFailForIdempotencyCheck(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	batch := make([]*BatchedTransactionInsert, 3)
	for i := 0; i < len(batch); i++ {
		batch[i] = &BatchedTransactionInsert{
			Input: TransactionInsertInput{
				Type:           core.BatchTypePrivate,
				IdempotencyKey: core.IdempotencyKey(fmt.Sprintf("idem_%.3d", i)),
			},
		}
	}
	mdi.On("InsertTransactions", ctx, mock.MatchedBy(func(transactions []*core.Transaction) bool {
		return len(transactions) == len(batch)
	})).Return(fmt.Errorf("fallback to throw this err")).Once()
	mdi.On("GetTransactions", ctx, "ns1", mock.Anything).Return(nil, nil, fmt.Errorf("do not throw this error")).Once()

	err := txHelper.SubmitNewTransactionBatch(ctx, "ns1", batch)
	assert.Regexp(t, "fallback to throw this err", err)

	mdi.AssertExpectations(t)
}

func TestSubmitNewTransactionBatchFindWrongNumberOfRecords(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	batch := make([]*BatchedTransactionInsert, 3)
	for i := 0; i < len(batch); i++ {
		batch[i] = &BatchedTransactionInsert{
			Input: TransactionInsertInput{
				Type:           core.BatchTypePrivate,
				IdempotencyKey: core.IdempotencyKey(fmt.Sprintf("idem_%.3d", i)),
			},
		}
	}
	mdi.On("InsertTransactions", ctx, mock.MatchedBy(func(transactions []*core.Transaction) bool {
		return len(transactions) == len(batch)
	})).Return(fmt.Errorf("fallback to throw this err")).Once()
	mdi.On("GetTransactions", ctx, "ns1", mock.Anything).Return(
		[]*core.Transaction{
			{ID: fftypes.NewUUID(), IdempotencyKey: "idem_002"}, // only one came back
		},
		nil, nil,
	).Once()

	err := txHelper.SubmitNewTransactionBatch(ctx, "ns1", batch)
	assert.Regexp(t, "fallback to throw this err", err)

	mdi.AssertExpectations(t)
}

func TestSubmitNewTransactionBatchIdempotentDupInBatch(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	batch := make([]*BatchedTransactionInsert, 3)
	for i := 0; i < len(batch); i++ {
		batch[i] = &BatchedTransactionInsert{
			Input: TransactionInsertInput{
				Type:           core.BatchTypePrivate,
				IdempotencyKey: "duplicated_in_all",
			},
		}
	}
	mdi.On("InsertTransactions", ctx, mock.MatchedBy(func(transactions []*core.Transaction) bool {
		return len(transactions) == 1
	})).Return(nil).Once()
	mdi.On("InsertEvent", ctx, mock.MatchedBy(func(e *core.Event) bool {
		return e.Type == core.EventTypeTransactionSubmitted
	})).Return(nil).Once()

	err := txHelper.SubmitNewTransactionBatch(ctx, "ns1", batch)
	assert.NoError(t, err)

	for i := 0; i < len(batch); i++ {
		if i == 0 {
			assert.NoError(t, batch[i].Output.IdempotencyError)
		} else {
			assert.Regexp(t, "FF10431.*duplicated_in_all", batch[i].Output.IdempotencyError)
		}
	}

	mdi.AssertExpectations(t)
}

func TestSubmitNewTransactionBatchInsertEventFail(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	batch := []*BatchedTransactionInsert{
		{
			Input: TransactionInsertInput{
				Type: core.BatchTypePrivate,
			},
		},
	}
	mdi.On("InsertTransactions", ctx, mock.MatchedBy(func(transactions []*core.Transaction) bool {
		return len(transactions) == 1
	})).Return(nil).Once()
	mdi.On("InsertEvent", ctx, mock.MatchedBy(func(e *core.Event) bool {
		return e.Type == core.EventTypeTransactionSubmitted
	})).Return(fmt.Errorf("pop")).Once()

	err := txHelper.SubmitNewTransactionBatch(ctx, "ns1", batch)
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
}
