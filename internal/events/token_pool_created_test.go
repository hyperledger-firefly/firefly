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

package events

import (
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/mocks/tokenmocks"
	"github.com/hyperledger-firefly/firefly/pkg/blockchain"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/hyperledger-firefly/firefly/pkg/tokens"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestTokenPoolCreatedIgnore(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	mti := &tokenmocks.Plugin{}

	txID := fftypes.NewUUID()
	info := fftypes.JSONObject{"some": "info"}
	pool := &tokens.TokenPool{
		Type:        core.TokenTypeFungible,
		PoolLocator: "123",
		TX: core.TransactionRef{
			ID:   txID,
			Type: core.TransactionTypeTokenPool,
		},
		Connector: "erc1155",
		Event: &blockchain.Event{
			BlockchainTXID: "0xffffeeee",
			ProtocolID:     "tx1",
			Info:           info,
		},
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "123").Return(nil, nil, nil)
	em.mth.On("FindOperationInTransaction", em.ctx, txID, core.OpTypeTokenCreatePool).Return(nil, nil)

	err := em.TokenPoolCreated(em.ctx, mti, pool)
	assert.NoError(t, err)

}

func TestTokenPoolCreatedIgnoreNoTX(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	mti := &tokenmocks.Plugin{}

	info := fftypes.JSONObject{"some": "info"}
	pool := &tokens.TokenPool{
		Type:        core.TokenTypeFungible,
		PoolLocator: "123",
		Connector:   "erc1155",
		Event: &blockchain.Event{
			BlockchainTXID: "0xffffeeee",
			ProtocolID:     "tx1",
			Info:           info,
		},
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "123").Return(nil, nil, nil)

	err := em.TokenPoolCreated(em.ctx, mti, pool)
	assert.NoError(t, err)
}

func TestTokenPoolCreatedConfirm(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	mti := &tokenmocks.Plugin{}

	txID := fftypes.NewUUID()
	info1 := fftypes.JSONObject{"pool": "info"}
	info2 := fftypes.JSONObject{"block": "info"}
	chainPool := &tokens.TokenPool{
		Type:        core.TokenTypeFungible,
		PoolLocator: "123",
		Connector:   "erc1155",
		TX: core.TransactionRef{
			ID:   txID,
			Type: core.TransactionTypeTokenPool,
		},
		Standard: "ERC1155",
		Symbol:   "FFT",
		Info:     info1,
		Event: &blockchain.Event{
			BlockchainTXID: "0xffffeeee",
			Name:           "TokenPool",
			ProtocolID:     "tx1",
			Info:           info2,
		},
	}
	storedPool := &core.TokenPool{
		Namespace: "ns1",
		ID:        fftypes.NewUUID(),
		Active:    false,
		Message:   fftypes.NewUUID(),
		TX: core.TransactionRef{
			Type: core.TransactionTypeTokenPool,
			ID:   txID,
		},
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "123").Return(nil, fmt.Errorf("pop")).Once()
	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "123").Return(storedPool, nil).Once()
	em.mth.On("InsertOrGetBlockchainEvent", em.ctx, mock.MatchedBy(func(e *core.BlockchainEvent) bool {
		return e.Name == chainPool.Event.Name
	})).Return(nil, nil)
	em.mdi.On("InsertEvent", em.ctx, mock.MatchedBy(func(e *core.Event) bool {
		return e.Type == core.EventTypeBlockchainEventReceived
	})).Return(nil).Once()
	em.mth.On("PersistTransaction", mock.Anything, txID, core.TransactionTypeTokenPool, "0xffffeeee").Return(true, nil).Once()
	em.mdi.On("UpsertTokenPool", em.ctx, storedPool, database.UpsertOptimizationExisting).Return(nil).Once()
	em.mdi.On("InsertEvent", em.ctx, mock.MatchedBy(func(e *core.Event) bool {
		return e.Type == core.EventTypePoolConfirmed && *e.Reference == *storedPool.ID
	})).Return(nil).Once()

	err := em.TokenPoolCreated(em.ctx, mti, chainPool)
	assert.NoError(t, err)

	assert.Equal(t, "ERC1155", storedPool.Standard)
	assert.Equal(t, "FFT", storedPool.Symbol)
	assert.Equal(t, info1, storedPool.Info)

}

func TestTokenPoolCreatedAlreadyConfirmed(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	mti := &tokenmocks.Plugin{}

	txID := fftypes.NewUUID()
	info := fftypes.JSONObject{"some": "info"}
	chainPool := &tokens.TokenPool{
		Type:        core.TokenTypeFungible,
		PoolLocator: "123",
		Connector:   "erc1155",
		TX: core.TransactionRef{
			ID:   txID,
			Type: core.TransactionTypeTokenPool,
		},
		Event: &blockchain.Event{
			BlockchainTXID: "0xffffeeee",
			ProtocolID:     "tx1",
			Info:           info,
		},
	}
	storedPool := &core.TokenPool{
		Namespace: "ns1",
		ID:        fftypes.NewUUID(),
		Active:    true,
		TX: core.TransactionRef{
			Type: core.TransactionTypeTokenPool,
			ID:   txID,
		},
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "123").Return(storedPool, nil)

	err := em.TokenPoolCreated(em.ctx, mti, chainPool)
	assert.NoError(t, err)

}

func TestTokenPoolCreatedConfirmFailBadSymbol(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	mti := &tokenmocks.Plugin{}

	opID := fftypes.NewUUID()
	txID := fftypes.NewUUID()
	info := fftypes.JSONObject{"some": "info"}
	chainPool := &tokens.TokenPool{
		Type:        core.TokenTypeFungible,
		PoolLocator: "123",
		Connector:   "erc1155",
		TX: core.TransactionRef{
			ID:   txID,
			Type: core.TransactionTypeTokenPool,
		},
		Symbol: "ETH",
		Event: &blockchain.Event{
			BlockchainTXID: "0xffffeeee",
			ProtocolID:     "tx1",
			Info:           info,
		},
	}
	storedPool := &core.TokenPool{
		Namespace: "ns1",
		ID:        fftypes.NewUUID(),
		Active:    false,
		Symbol:    "FFT",
		TX: core.TransactionRef{
			Type: core.TransactionTypeTokenPool,
			ID:   txID,
		},
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "123").Return(storedPool, nil)
	em.mth.On("FindOperationInTransaction", em.ctx, txID, core.OpTypeTokenCreatePool).Return(&core.Operation{
		ID: opID,
	}, nil)

	err := em.TokenPoolCreated(em.ctx, mti, chainPool)
	assert.NoError(t, err)

}

func TestConfirmPoolBlockchainEventFail(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	txID := fftypes.NewUUID()
	storedPool := &core.TokenPool{
		Namespace: "ns1",
		ID:        fftypes.NewUUID(),
		Key:       "0x0",
		Active:    false,
		TX: core.TransactionRef{
			Type: core.TransactionTypeTokenPool,
			ID:   txID,
		},
	}
	event := &blockchain.Event{
		BlockchainTXID: "0xffffeeee",
		Name:           "TokenPool",
		ProtocolID:     "tx1",
	}

	em.mth.On("InsertOrGetBlockchainEvent", em.ctx, mock.MatchedBy(func(e *core.BlockchainEvent) bool {
		return e.Name == event.Name
	})).Return(nil, fmt.Errorf("pop"))

	err := em.confirmPool(em.ctx, storedPool, event)
	assert.EqualError(t, err, "pop")

}

func TestConfirmPoolTxFail(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	txID := fftypes.NewUUID()
	storedPool := &core.TokenPool{
		Namespace: "ns1",
		ID:        fftypes.NewUUID(),
		Key:       "0x0",
		Active:    false,
		TX: core.TransactionRef{
			Type: core.TransactionTypeTokenPool,
			ID:   txID,
		},
	}
	event := &blockchain.Event{
		BlockchainTXID: "0xffffeeee",
		Name:           "TokenPool",
		ProtocolID:     "tx1",
	}

	em.mth.On("InsertOrGetBlockchainEvent", em.ctx, mock.MatchedBy(func(e *core.BlockchainEvent) bool {
		return e.Name == event.Name
	})).Return(nil, nil)
	em.mdi.On("InsertEvent", em.ctx, mock.MatchedBy(func(e *core.Event) bool {
		return e.Type == core.EventTypeBlockchainEventReceived
	})).Return(nil)
	em.mth.On("PersistTransaction", mock.Anything, txID, core.TransactionTypeTokenPool, "0xffffeeee").Return(false, fmt.Errorf("pop"))

	err := em.confirmPool(em.ctx, storedPool, event)
	assert.EqualError(t, err, "pop")

}

func TestConfirmPoolUpsertFail(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	txID := fftypes.NewUUID()
	storedPool := &core.TokenPool{
		Namespace: "ns1",
		ID:        fftypes.NewUUID(),
		Key:       "0x0",
		Active:    false,
		TX: core.TransactionRef{
			Type: core.TransactionTypeTokenPool,
			ID:   txID,
		},
	}
	event := &blockchain.Event{
		BlockchainTXID: "0xffffeeee",
		Name:           "TokenPool",
		ProtocolID:     "tx1",
	}

	em.mth.On("InsertOrGetBlockchainEvent", em.ctx, mock.MatchedBy(func(e *core.BlockchainEvent) bool {
		return e.Name == event.Name
	})).Return(nil, nil)
	em.mdi.On("InsertEvent", em.ctx, mock.MatchedBy(func(e *core.Event) bool {
		return e.Type == core.EventTypeBlockchainEventReceived
	})).Return(nil)
	em.mth.On("PersistTransaction", mock.Anything, txID, core.TransactionTypeTokenPool, "0xffffeeee").Return(true, nil).Once()
	em.mdi.On("UpsertTokenPool", em.ctx, storedPool, database.UpsertOptimizationExisting).Return(fmt.Errorf("pop"))

	err := em.confirmPool(em.ctx, storedPool, event)
	assert.EqualError(t, err, "pop")

}

func TestTokenPoolCreatedPublish(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	mti := &tokenmocks.Plugin{}

	poolID := fftypes.NewUUID()
	txID := fftypes.NewUUID()
	operation := &core.Operation{
		ID: fftypes.NewUUID(),
		Input: fftypes.JSONObject{
			"id":        poolID.String(),
			"namespace": "ns1",
			"name":      "my-pool",
		},
	}
	info := fftypes.JSONObject{"some": "info"}
	pool := &tokens.TokenPool{
		Type:        core.TokenTypeFungible,
		PoolLocator: "123",
		TX: core.TransactionRef{
			ID:   txID,
			Type: core.TransactionTypeTokenPool,
		},
		Connector: "erc1155",
		Event: &blockchain.Event{
			BlockchainTXID: "0xffffeeee",
			ProtocolID:     "tx1",
			Info:           info,
		},
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "123").Return(nil, nil).Times(2)
	em.mth.On("FindOperationInTransaction", em.ctx, txID, core.OpTypeTokenCreatePool).Return(nil, fmt.Errorf("pop")).Once()
	em.mth.On("FindOperationInTransaction", em.ctx, txID, core.OpTypeTokenCreatePool).Return(operation, nil).Once()
	em.mds.On("DefineTokenPool", em.ctx, mock.MatchedBy(func(pool *core.TokenPool) bool {
		return pool.Namespace == "ns1" && pool.Name == "my-pool" && *pool.ID == *poolID
	}), false).Return(nil, nil)

	err := em.TokenPoolCreated(em.ctx, mti, pool)
	assert.NoError(t, err)

	mti.AssertExpectations(t)
}

func TestTokenPoolCreatedPublishBadInterface(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	mti := &tokenmocks.Plugin{}

	poolID := fftypes.NewUUID()
	txID := fftypes.NewUUID()
	interfaceID := fftypes.NewUUID()
	operation := &core.Operation{
		ID: fftypes.NewUUID(),
		Input: fftypes.JSONObject{
			"id":        poolID.String(),
			"namespace": "ns1",
			"name":      "my-pool",
			"interface": fftypes.JSONObject{
				"id": interfaceID.String(),
			},
		},
	}
	info := fftypes.JSONObject{"some": "info"}
	pool := &tokens.TokenPool{
		Type:        core.TokenTypeFungible,
		PoolLocator: "123",
		TX: core.TransactionRef{
			ID:   txID,
			Type: core.TransactionTypeTokenPool,
		},
		Connector: "erc1155",
		Event: &blockchain.Event{
			BlockchainTXID: "0xffffeeee",
			ProtocolID:     "tx1",
			Info:           info,
		},
		InterfaceFormat: "abi",
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "123").Return(nil, nil).Times(2)
	em.mth.On("FindOperationInTransaction", em.ctx, txID, core.OpTypeTokenCreatePool).Return(nil, fmt.Errorf("pop")).Once()
	em.mth.On("FindOperationInTransaction", em.ctx, txID, core.OpTypeTokenCreatePool).Return(operation, nil).Once()
	em.mam.On("ResolvePoolMethods", em.ctx, mock.MatchedBy(func(pool *core.TokenPool) bool {
		return pool.Locator == "123" && pool.Name == "my-pool"
	})).Return(fmt.Errorf("pop"))

	err := em.TokenPoolCreated(em.ctx, mti, pool)
	assert.EqualError(t, err, "pop")

	mti.AssertExpectations(t)
}

func TestTokenPoolCreatedPublishBadOpInputID(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	mti := &tokenmocks.Plugin{}

	txID := fftypes.NewUUID()
	operation := &core.Operation{
		ID:    fftypes.NewUUID(),
		Type:  core.OpTypeTokenCreatePool,
		Input: fftypes.JSONObject{},
	}
	info := fftypes.JSONObject{"some": "info"}
	pool := &tokens.TokenPool{
		Type:        core.TokenTypeFungible,
		PoolLocator: "123",
		TX: core.TransactionRef{
			ID:   txID,
			Type: core.TransactionTypeTokenPool,
		},
		Connector: "erc1155",
		Event: &blockchain.Event{
			BlockchainTXID: "0xffffeeee",
			ProtocolID:     "tx1",
			Info:           info,
		},
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "123").Return(nil, nil)
	em.mth.On("FindOperationInTransaction", em.ctx, txID, core.OpTypeTokenCreatePool).Return(operation, nil)

	err := em.TokenPoolCreated(em.ctx, mti, pool)
	assert.NoError(t, err)

}

func TestTokenPoolCreatedPublishBadOpInputNS(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	mti := &tokenmocks.Plugin{}

	txID := fftypes.NewUUID()
	operation := &core.Operation{
		ID:   fftypes.NewUUID(),
		Type: core.OpTypeTokenCreatePool,
		Input: fftypes.JSONObject{
			"id": fftypes.NewUUID().String(),
		},
	}
	info := fftypes.JSONObject{"some": "info"}
	pool := &tokens.TokenPool{
		Type:        core.TokenTypeFungible,
		PoolLocator: "123",
		TX: core.TransactionRef{
			ID:   txID,
			Type: core.TransactionTypeTokenPool,
		},
		Connector: "erc1155",
		Event: &blockchain.Event{
			BlockchainTXID: "0xffffeeee",
			ProtocolID:     "tx1",
			Info:           info,
		},
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "123").Return(nil, nil)
	em.mth.On("FindOperationInTransaction", em.ctx, txID, core.OpTypeTokenCreatePool).Return(operation, nil)

	err := em.TokenPoolCreated(em.ctx, mti, pool)
	assert.NoError(t, err)

}

func TestTokenPoolCreatedPublishBadSymbol(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	mti := &tokenmocks.Plugin{}

	poolID := fftypes.NewUUID()
	txID := fftypes.NewUUID()
	operation := &core.Operation{
		ID: fftypes.NewUUID(),
		Input: fftypes.JSONObject{
			"id":        poolID.String(),
			"namespace": "test-ns",
			"name":      "my-pool",
			"symbol":    "FFT",
		},
	}
	info := fftypes.JSONObject{"some": "info"}
	pool := &tokens.TokenPool{
		Type:        core.TokenTypeFungible,
		PoolLocator: "123",
		TX: core.TransactionRef{
			ID:   txID,
			Type: core.TransactionTypeTokenPool,
		},
		Connector: "erc1155",
		Symbol:    "ETH",
		Event: &blockchain.Event{
			BlockchainTXID: "0xffffeeee",
			ProtocolID:     "tx1",
			Info:           info,
		},
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "123").Return(nil, nil).Times(2)
	em.mth.On("FindOperationInTransaction", em.ctx, txID, core.OpTypeTokenCreatePool).Return(nil, fmt.Errorf("pop")).Once()
	em.mth.On("FindOperationInTransaction", em.ctx, txID, core.OpTypeTokenCreatePool).Return(operation, nil).Once()

	err := em.TokenPoolCreated(em.ctx, mti, pool)
	assert.NoError(t, err)

	mti.AssertExpectations(t)
}

func TestLoadExistingAlternateLocator(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	existingPool := &core.TokenPool{
		Type:      core.TokenTypeFungible,
		Locator:   "123",
		Connector: "erc1155",
		Symbol:    "ETH",
	}
	updatedPool := &tokens.TokenPool{
		Type:              core.TokenTypeFungible,
		PoolLocator:       "456",
		AlternateLocators: []string{"123"},
		Connector:         "erc1155",
		Symbol:            "ETH",
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "456").Return(nil, nil).Once()
	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "123").Return(existingPool, nil).Once()
	em.mdi.On("UpsertTokenPool", em.ctx, existingPool, database.UpsertOptimizationExisting).Return(nil).Once()

	p, err := em.loadExisting(em.ctx, updatedPool)
	assert.NoError(t, err)
	assert.Equal(t, p, existingPool)
}

func TestLoadExistingAlternateLocatorError(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	updatedPool := &tokens.TokenPool{
		Type:              core.TokenTypeFungible,
		PoolLocator:       "456",
		AlternateLocators: []string{"123"},
		Connector:         "erc1155",
		Symbol:            "ETH",
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "456").Return(nil, nil).Once()
	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "123").Return(nil, fmt.Errorf("pop")).Once()

	p, err := em.loadExisting(em.ctx, updatedPool)
	assert.Equal(t, err.Error(), "pop")
	assert.Nil(t, p)
}

func TestLoadExistingAlternateLocatorUpsertError(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	existingPool := &core.TokenPool{
		Type:      core.TokenTypeFungible,
		Locator:   "123",
		Connector: "erc1155",
		Symbol:    "ETH",
	}
	updatedPool := &tokens.TokenPool{
		Type:              core.TokenTypeFungible,
		PoolLocator:       "456",
		AlternateLocators: []string{"123"},
		Connector:         "erc1155",
		Symbol:            "ETH",
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "456").Return(nil, nil).Once()
	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "123").Return(existingPool, nil).Once()
	em.mdi.On("UpsertTokenPool", em.ctx, existingPool, database.UpsertOptimizationExisting).Return(fmt.Errorf("pop")).Once()

	p, err := em.loadExisting(em.ctx, updatedPool)
	assert.Equal(t, err.Error(), "pop")
	assert.Equal(t, p, existingPool)
}
