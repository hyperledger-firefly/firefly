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

package events

import (
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/mocks/tokenmocks"
	"github.com/hyperledger-firefly/firefly/pkg/blockchain"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/tokens"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func newTransfer() *tokens.TokenTransfer {
	return &tokens.TokenTransfer{
		PoolLocator: "F1",
		TokenTransfer: core.TokenTransfer{
			Type:       core.TokenTransferTypeTransfer,
			TokenIndex: "0",
			Connector:  "erc1155",
			Key:        "0x12345",
			From:       "0x1",
			To:         "0x2",
			ProtocolID: "123",
			URI:        "firefly://token/1",
			Amount:     *fftypes.NewFFBigInt(1),
			TX: core.TransactionRef{
				ID:   fftypes.NewUUID(),
				Type: core.TransactionTypeTokenTransfer,
			},
		},
		Event: &blockchain.Event{
			BlockchainTXID: "0xffffeeee",
			Name:           "Transfer",
			ProtocolID:     "0000/0000/0000",
			Info:           fftypes.JSONObject{"some": "info"},
		},
	}
}

func TestTokensTransferredSucceedWithRetries(t *testing.T) {
	em := newTestEventManagerWithMetrics(t)
	defer em.cleanup(t)

	mti := &tokenmocks.Plugin{}

	transfer := newTransfer()
	transfer.TX = core.TransactionRef{}
	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "F1").Return(nil, fmt.Errorf("pop")).Once()
	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "F1").Return(pool, nil).Once()
	em.mam.On("GetTokenPoolByID", em.ctx, pool.ID).Return(pool, nil).Times(2)
	em.mth.On("InsertOrGetBlockchainEvent", em.ctx, mock.MatchedBy(func(e *core.BlockchainEvent) bool {
		return e.Namespace == pool.Namespace && e.Name == transfer.Event.Name
	})).Return(nil, nil).Times(3)
	em.mdi.On("InsertEvent", em.ctx, mock.MatchedBy(func(ev *core.Event) bool {
		return ev.Type == core.EventTypeBlockchainEventReceived && ev.Namespace == pool.Namespace
	})).Return(nil).Times(3)
	em.mdi.On("InsertOrGetTokenTransfer", em.ctx, &transfer.TokenTransfer).Return(nil, fmt.Errorf("pop")).Once()
	em.mdi.On("InsertOrGetTokenTransfer", em.ctx, &transfer.TokenTransfer).Return(nil, nil).Times(2)
	em.mdi.On("UpdateTokenBalances", em.ctx, &transfer.TokenTransfer).Return(fmt.Errorf("pop")).Once()
	em.mdi.On("UpdateTokenBalances", em.ctx, &transfer.TokenTransfer).Return(nil).Once()
	em.mdi.On("InsertEvent", em.ctx, mock.MatchedBy(func(ev *core.Event) bool {
		return ev.Type == core.EventTypeTransferConfirmed && ev.Reference == transfer.LocalID && ev.Namespace == pool.Namespace
	})).Return(nil).Once()

	err := em.TokensTransferred(mti, transfer)
	assert.NoError(t, err)

	mti.AssertExpectations(t)
}

func TestPersistTransferOpFail(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	transfer := newTransfer()
	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "F1").Return(pool, nil)
	em.mth.On("FindOperationInTransaction", em.ctx, transfer.TX.ID, core.OpTypeTokenTransfer).Return(nil, fmt.Errorf("pop"))

	valid, err := em.persistTokenTransfer(em.ctx, transfer)
	assert.False(t, valid)
	assert.EqualError(t, err, "pop")
}

func TestPersistTransferBadOp(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	transfer := newTransfer()
	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	op := &core.Operation{
		Input: fftypes.JSONObject{
			"localId": "bad",
		},
		Transaction: fftypes.NewUUID(),
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "F1").Return(pool, nil)
	em.mth.On("FindOperationInTransaction", em.ctx, transfer.TX.ID, core.OpTypeTokenTransfer).Return(op, nil)
	em.mth.On("PersistTransaction", mock.Anything, transfer.TX.ID, core.TransactionTypeTokenTransfer, "0xffffeeee").Return(false, fmt.Errorf("pop"))

	valid, err := em.persistTokenTransfer(em.ctx, transfer)
	assert.False(t, valid)
	assert.EqualError(t, err, "pop")

}

func TestPersistTransferTxFail(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	transfer := newTransfer()
	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	localID := fftypes.NewUUID()
	op := &core.Operation{
		Input: fftypes.JSONObject{
			"localId": localID.String(),
		},
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "F1").Return(pool, nil)
	em.mth.On("FindOperationInTransaction", em.ctx, transfer.TX.ID, core.OpTypeTokenTransfer).Return(op, nil)
	em.mth.On("PersistTransaction", mock.Anything, transfer.TX.ID, core.TransactionTypeTokenTransfer, "0xffffeeee").Return(false, fmt.Errorf("pop"))

	valid, err := em.persistTokenTransfer(em.ctx, transfer)
	assert.False(t, valid)
	assert.EqualError(t, err, "pop")

}

func TestPersistTransferGetTransferFail(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	transfer := newTransfer()
	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	localID := fftypes.NewUUID()
	op := &core.Operation{
		Input: fftypes.JSONObject{
			"localId":   localID.String(),
			"connector": transfer.Connector,
			"pool":      pool.ID.String(),
		},
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "F1").Return(pool, nil)
	em.mth.On("FindOperationInTransaction", em.ctx, transfer.TX.ID, core.OpTypeTokenTransfer).Return(op, nil)
	em.mdi.On("GetTokenTransferByID", em.ctx, "ns1", localID).Return(nil, fmt.Errorf("pop"))

	valid, err := em.persistTokenTransfer(em.ctx, transfer)
	assert.False(t, valid)
	assert.EqualError(t, err, "pop")

}

func TestPersistTransferBlockchainEventFail(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	transfer := newTransfer()
	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	localID := fftypes.NewUUID()
	op := &core.Operation{
		Input: fftypes.JSONObject{
			"localId":   localID.String(),
			"connector": transfer.Connector,
			"pool":      pool.ID.String(),
		},
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "F1").Return(pool, nil)
	em.mth.On("FindOperationInTransaction", em.ctx, transfer.TX.ID, core.OpTypeTokenTransfer).Return(op, nil)
	em.mth.On("PersistTransaction", mock.Anything, transfer.TX.ID, core.TransactionTypeTokenTransfer, "0xffffeeee").Return(true, nil)
	em.mdi.On("GetTokenTransferByID", em.ctx, "ns1", localID).Return(nil, nil)
	em.mth.On("InsertOrGetBlockchainEvent", em.ctx, mock.MatchedBy(func(e *core.BlockchainEvent) bool {
		return e.Namespace == pool.Namespace && e.Name == transfer.Event.Name
	})).Return(nil, fmt.Errorf("pop"))

	valid, err := em.persistTokenTransfer(em.ctx, transfer)
	assert.False(t, valid)
	assert.EqualError(t, err, "pop")

}

func TestTokensTransferredWithTransactionRegenerateLocalID(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	mti := &tokenmocks.Plugin{}

	transfer := newTransfer()
	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	localID := fftypes.NewUUID()
	op := &core.Operation{
		Input: fftypes.JSONObject{
			"localId":   localID.String(),
			"connector": transfer.Connector,
			"pool":      pool.ID.String(),
		},
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "F1").Return(pool, nil)
	em.mth.On("FindOperationInTransaction", em.ctx, transfer.TX.ID, core.OpTypeTokenTransfer).Return(op, nil)
	em.mth.On("PersistTransaction", mock.Anything, transfer.TX.ID, core.TransactionTypeTokenTransfer, "0xffffeeee").Return(true, nil)
	em.mdi.On("GetTokenTransferByID", em.ctx, "ns1", localID).Return(&core.TokenTransfer{}, nil)
	em.mth.On("InsertOrGetBlockchainEvent", em.ctx, mock.MatchedBy(func(e *core.BlockchainEvent) bool {
		return e.Namespace == pool.Namespace && e.Name == transfer.Event.Name
	})).Return(nil, nil)
	em.mdi.On("InsertEvent", em.ctx, mock.MatchedBy(func(ev *core.Event) bool {
		return ev.Type == core.EventTypeBlockchainEventReceived && ev.Namespace == pool.Namespace
	})).Return(nil)
	em.mdi.On("InsertOrGetTokenTransfer", em.ctx, &transfer.TokenTransfer).Return(nil, nil)
	em.mdi.On("UpdateTokenBalances", em.ctx, &transfer.TokenTransfer).Return(nil)

	valid, err := em.persistTokenTransfer(em.ctx, transfer)
	assert.True(t, valid)
	assert.NoError(t, err)

	assert.NotEqual(t, *localID, *transfer.LocalID)

	mti.AssertExpectations(t)
}

func TestTokensTransferredWithExistingTransfer(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	mti := &tokenmocks.Plugin{}

	transfer := newTransfer()
	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	localID := fftypes.NewUUID()
	op := &core.Operation{
		Input: fftypes.JSONObject{
			"localId":   localID.String(),
			"connector": transfer.Connector,
			"pool":      pool.ID.String(),
		},
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "F1").Return(pool, nil)
	em.mth.On("InsertOrGetBlockchainEvent", em.ctx, mock.MatchedBy(func(e *core.BlockchainEvent) bool {
		return e.Namespace == pool.Namespace && e.Name == transfer.Event.Name
	})).Return(nil, nil)
	em.mdi.On("InsertEvent", em.ctx, mock.MatchedBy(func(ev *core.Event) bool {
		return ev.Type == core.EventTypeBlockchainEventReceived && ev.Namespace == pool.Namespace
	})).Return(nil)
	em.mth.On("FindOperationInTransaction", em.ctx, transfer.TX.ID, core.OpTypeTokenTransfer).Return(op, nil)
	em.mth.On("PersistTransaction", mock.Anything, transfer.TX.ID, core.TransactionTypeTokenTransfer, "0xffffeeee").Return(true, nil)
	em.mdi.On("GetTokenTransferByID", em.ctx, "ns1", localID).Return(&core.TokenTransfer{}, nil)
	em.mdi.On("InsertOrGetTokenTransfer", em.ctx, &transfer.TokenTransfer).Return(&core.TokenTransfer{Type: core.TokenTransferTypeMint}, nil)

	valid, err := em.persistTokenTransfer(em.ctx, transfer)
	assert.False(t, valid)
	assert.NoError(t, err)

	assert.NotEqual(t, *localID, *transfer.LocalID)

	mti.AssertExpectations(t)
}

func TestTokensTransferredBadPool(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	mti := &tokenmocks.Plugin{}

	transfer := newTransfer()

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "F1").Return(nil, nil)

	err := em.TokensTransferred(mti, transfer)
	assert.NoError(t, err)

	mti.AssertExpectations(t)
}

func TestTokensTransferredWithMessageReceived(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	mti := &tokenmocks.Plugin{}

	uri := "firefly://token/1"
	info := fftypes.JSONObject{"some": "info"}
	transfer := &tokens.TokenTransfer{
		PoolLocator: "F1",
		TokenTransfer: core.TokenTransfer{
			Type:       core.TokenTransferTypeTransfer,
			TokenIndex: "0",
			URI:        uri,
			Connector:  "erc1155",
			Key:        "0x12345",
			From:       "0x1",
			To:         "0x2",
			ProtocolID: "123",
			Message:    fftypes.NewUUID(),
			Amount:     *fftypes.NewFFBigInt(1),
		},
		Event: &blockchain.Event{
			BlockchainTXID: "0xffffeeee",
			ProtocolID:     "0000/0000/0000",
			Info:           info,
		},
	}
	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	message := &core.Message{
		BatchID: fftypes.NewUUID(),
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "F1").Return(pool, nil).Once()
	em.mam.On("GetTokenPoolByID", em.ctx, pool.ID).Return(pool, nil).Once()
	em.mth.On("InsertOrGetBlockchainEvent", em.ctx, mock.MatchedBy(func(e *core.BlockchainEvent) bool {
		return e.Namespace == pool.Namespace && e.Name == transfer.Event.Name
	})).Return(nil, nil).Times(2)
	em.mdi.On("InsertEvent", em.ctx, mock.MatchedBy(func(ev *core.Event) bool {
		return ev.Type == core.EventTypeBlockchainEventReceived && ev.Namespace == pool.Namespace
	})).Return(nil).Times(2)
	em.mdi.On("InsertOrGetTokenTransfer", em.ctx, &transfer.TokenTransfer).Return(nil, nil).Times(2)
	em.mdi.On("UpdateTokenBalances", em.ctx, &transfer.TokenTransfer).Return(nil).Times(2)
	em.mdi.On("GetMessageByID", em.ctx, "ns1", transfer.Message).Return(nil, fmt.Errorf("pop")).Once()
	em.mdi.On("GetMessageByID", em.ctx, "ns1", transfer.Message).Return(message, nil).Once()
	em.mdi.On("InsertEvent", em.ctx, mock.MatchedBy(func(ev *core.Event) bool {
		return ev.Type == core.EventTypeTransferConfirmed && ev.Reference == transfer.LocalID && ev.Namespace == pool.Namespace
	})).Return(nil).Once()

	err := em.TokensTransferred(mti, transfer)
	assert.NoError(t, err)

	mti.AssertExpectations(t)
}

func TestTokensTransferredWithMessageSend(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	mti := &tokenmocks.Plugin{}

	uri := "firefly://token/1"
	info := fftypes.JSONObject{"some": "info"}
	transfer := &tokens.TokenTransfer{
		PoolLocator: "F1",
		TokenTransfer: core.TokenTransfer{
			Type:       core.TokenTransferTypeTransfer,
			TokenIndex: "0",
			URI:        uri,
			Connector:  "erc1155",
			Key:        "0x12345",
			From:       "0x1",
			To:         "0x2",
			ProtocolID: "123",
			Message:    fftypes.NewUUID(),
			Amount:     *fftypes.NewFFBigInt(1),
		},
		Event: &blockchain.Event{
			BlockchainTXID: "0xffffeeee",
			ProtocolID:     "0000/0000/0000",
			Info:           info,
		},
	}
	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	message := &core.Message{
		BatchID: fftypes.NewUUID(),
		State:   core.MessageStateStaged,
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "F1").Return(pool, nil).Once()
	em.mam.On("GetTokenPoolByID", em.ctx, pool.ID).Return(pool, nil).Once()
	em.mth.On("InsertOrGetBlockchainEvent", em.ctx, mock.MatchedBy(func(e *core.BlockchainEvent) bool {
		return e.Namespace == pool.Namespace && e.Name == transfer.Event.Name
	})).Return(nil, nil).Times(2)
	em.mdi.On("InsertEvent", em.ctx, mock.MatchedBy(func(ev *core.Event) bool {
		return ev.Type == core.EventTypeBlockchainEventReceived && ev.Namespace == pool.Namespace
	})).Return(nil).Times(2)
	em.mdi.On("InsertOrGetTokenTransfer", em.ctx, &transfer.TokenTransfer).Return(nil, nil).Times(2)
	em.mdi.On("UpdateTokenBalances", em.ctx, &transfer.TokenTransfer).Return(nil).Times(2)
	em.mdi.On("GetMessageByID", em.ctx, "ns1", mock.Anything).Return(message, nil).Times(2)
	em.mdi.On("ReplaceMessage", em.ctx, mock.MatchedBy(func(msg *core.Message) bool {
		return msg.State == core.MessageStateReady
	})).Return(fmt.Errorf("pop"))
	em.mdi.On("InsertEvent", em.ctx, mock.MatchedBy(func(ev *core.Event) bool {
		return ev.Type == core.EventTypeTransferConfirmed && ev.Reference == transfer.LocalID && ev.Namespace == pool.Namespace
	})).Return(nil).Once()

	err := em.TokensTransferred(mti, transfer)
	assert.NoError(t, err)

	mti.AssertExpectations(t)
}
