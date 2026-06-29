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

func newApproval() *tokens.TokenApproval {
	return &tokens.TokenApproval{
		PoolLocator: "F1",
		TokenApproval: core.TokenApproval{
			LocalID:    fftypes.NewUUID(),
			Connector:  "erc1155",
			Namespace:  "ns1",
			Key:        "0x01",
			Operator:   "0x02",
			Approved:   true,
			ProtocolID: "0001/01/01",
			Subject:    "123",
			TX: core.TransactionRef{
				Type: core.TransactionTypeTokenApproval,
				ID:   fftypes.NewUUID(),
			},
		},
		Event: &blockchain.Event{
			BlockchainTXID: "0xffffeeee",
			Name:           "TokenApproval",
			ProtocolID:     "0000/0000/0000",
			Info:           fftypes.JSONObject{"some": "info"},
		},
	}
}

func TestTokensApprovedSucceedWithRetries(t *testing.T) {
	em := newTestEventManagerWithMetrics(t)
	defer em.cleanup(t)

	mti := &tokenmocks.Plugin{}

	approval := newApproval()
	approval.TX = core.TransactionRef{}
	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "F1").Return(nil, fmt.Errorf("pop")).Once()
	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "F1").Return(pool, nil).Once()
	em.mam.On("GetTokenPoolByID", em.ctx, pool.ID).Return(pool, nil).Times(3)
	em.mdi.On("GetTokenApprovalByProtocolID", em.ctx, "ns1", pool.ID, approval.ProtocolID).Return(nil, fmt.Errorf("pop")).Once()
	em.mdi.On("GetTokenApprovalByProtocolID", em.ctx, "ns1", pool.ID, approval.ProtocolID).Return(nil, nil).Times(3)
	em.mth.On("InsertOrGetBlockchainEvent", em.ctx, mock.MatchedBy(func(e *core.BlockchainEvent) bool {
		return e.Namespace == pool.Namespace && e.Name == approval.Event.Name
	})).Return(nil, nil).Times(3)
	em.mdi.On("InsertEvent", em.ctx, mock.MatchedBy(func(ev *core.Event) bool {
		return ev.Type == core.EventTypeBlockchainEventReceived && ev.Namespace == pool.Namespace
	})).Return(nil).Times(3)
	em.mdi.On("UpdateTokenApprovals", em.ctx, mock.Anything, mock.Anything).Return(fmt.Errorf("pop")).Once()
	em.mdi.On("UpdateTokenApprovals", em.ctx, mock.Anything, mock.Anything).Return(nil).Times(2)
	em.mdi.On("UpsertTokenApproval", em.ctx, &approval.TokenApproval).Return(fmt.Errorf("pop")).Once()
	em.mdi.On("UpsertTokenApproval", em.ctx, &approval.TokenApproval).Return(nil).Times(1)
	em.mdi.On("InsertEvent", em.ctx, mock.MatchedBy(func(ev *core.Event) bool {
		return ev.Type == core.EventTypeApprovalConfirmed && ev.Reference == approval.LocalID && ev.Namespace == pool.Namespace
	})).Return(nil).Once()

	err := em.TokensApproved(mti, approval)
	assert.NoError(t, err)

	mti.AssertExpectations(t)
}

func TestPersistApprovalDuplicate(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	approval := newApproval()
	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "F1").Return(pool, nil)
	em.mdi.On("GetTokenApprovalByProtocolID", em.ctx, "ns1", pool.ID, approval.ProtocolID).Return(&core.TokenApproval{}, nil)

	valid, err := em.persistTokenApproval(em.ctx, approval)
	assert.False(t, valid)
	assert.NoError(t, err)

}

func TestPersistApprovalOpFail(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	approval := newApproval()
	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "F1").Return(pool, nil)
	em.mdi.On("GetTokenApprovalByProtocolID", em.ctx, "ns1", pool.ID, approval.ProtocolID).Return(nil, nil)
	em.mth.On("FindOperationInTransaction", em.ctx, approval.TX.ID, core.OpTypeTokenApproval).Return(nil, fmt.Errorf("pop"))

	valid, err := em.persistTokenApproval(em.ctx, approval)
	assert.False(t, valid)
	assert.EqualError(t, err, "pop")

}

func TestPersistApprovalBadOp(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	approval := newApproval()
	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	op := &core.Operation{
		Input: fftypes.JSONObject{
			"localId": "realbad",
		},
		Transaction: fftypes.NewUUID(),
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "F1").Return(pool, nil)
	em.mdi.On("GetTokenApprovalByProtocolID", em.ctx, "ns1", pool.ID, approval.ProtocolID).Return(nil, nil)
	em.mth.On("FindOperationInTransaction", em.ctx, approval.TX.ID, core.OpTypeTokenApproval).Return(op, nil)
	em.mth.On("PersistTransaction", mock.Anything, approval.TX.ID, core.TransactionTypeTokenApproval, "0xffffeeee").Return(false, fmt.Errorf("pop"))

	valid, err := em.persistTokenApproval(em.ctx, approval)
	assert.False(t, valid)
	assert.EqualError(t, err, "pop")

}

func TestPersistApprovalTxFail(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	approval := newApproval()
	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	localID := fftypes.NewUUID()
	op := &core.Operation{
		Input: fftypes.JSONObject{
			"localId":   localID.String(),
			"connector": approval.Connector,
			"pool":      pool.ID.String(),
		},
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "F1").Return(pool, nil)
	em.mdi.On("GetTokenApprovalByProtocolID", em.ctx, "ns1", pool.ID, approval.ProtocolID).Return(nil, nil)
	em.mth.On("FindOperationInTransaction", em.ctx, approval.TX.ID, core.OpTypeTokenApproval).Return(op, nil)
	em.mdi.On("GetTokenApprovalByID", em.ctx, "ns1", localID).Return(nil, nil)
	em.mth.On("PersistTransaction", mock.Anything, approval.TX.ID, core.TransactionTypeTokenApproval, "0xffffeeee").Return(false, fmt.Errorf("pop"))

	valid, err := em.persistTokenApproval(em.ctx, approval)
	assert.False(t, valid)
	assert.EqualError(t, err, "pop")

}

func TestPersistApprovalGetApprovalFail(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	approval := newApproval()
	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	localID := fftypes.NewUUID()
	op := &core.Operation{
		Input: fftypes.JSONObject{
			"localId":   localID.String(),
			"connector": approval.Connector,
			"pool":      pool.ID.String(),
		},
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "F1").Return(pool, nil)
	em.mdi.On("GetTokenApprovalByProtocolID", em.ctx, "ns1", pool.ID, approval.ProtocolID).Return(nil, nil)
	em.mth.On("FindOperationInTransaction", em.ctx, approval.TX.ID, core.OpTypeTokenApproval).Return(op, nil)
	em.mdi.On("GetTokenApprovalByID", em.ctx, "ns1", localID).Return(nil, fmt.Errorf("pop"))

	valid, err := em.persistTokenApproval(em.ctx, approval)
	assert.False(t, valid)
	assert.EqualError(t, err, "pop")

}

func TestApprovedBadPool(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	mti := &tokenmocks.Plugin{}

	approval := newApproval()
	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "F1").Return(nil, nil)

	err := em.TokensApproved(mti, approval)
	assert.NoError(t, err)

	mti.AssertExpectations(t)
}

func TestApprovedWithTransactionRegenerateLocalID(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	mti := &tokenmocks.Plugin{}

	approval := newApproval()
	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	localID := fftypes.NewUUID()
	op := &core.Operation{
		Input: fftypes.JSONObject{
			"localId":   localID.String(),
			"connector": approval.Connector,
			"pool":      pool.ID.String(),
		},
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "F1").Return(pool, nil)
	em.mdi.On("GetTokenApprovalByProtocolID", em.ctx, "ns1", pool.ID, approval.ProtocolID).Return(nil, nil)
	em.mth.On("FindOperationInTransaction", em.ctx, approval.TX.ID, core.OpTypeTokenApproval).Return(op, nil)
	em.mth.On("PersistTransaction", mock.Anything, approval.TX.ID, core.TransactionTypeTokenApproval, "0xffffeeee").Return(true, nil)
	em.mdi.On("GetTokenApprovalByID", em.ctx, "ns1", localID).Return(&core.TokenApproval{}, nil)
	em.mth.On("InsertOrGetBlockchainEvent", em.ctx, mock.MatchedBy(func(e *core.BlockchainEvent) bool {
		return e.Namespace == pool.Namespace && e.Name == approval.Event.Name
	})).Return(nil, nil)
	em.mdi.On("InsertEvent", em.ctx, mock.MatchedBy(func(ev *core.Event) bool {
		return ev.Type == core.EventTypeBlockchainEventReceived && ev.Namespace == pool.Namespace
	})).Return(nil)
	em.mdi.On("UpdateTokenApprovals", em.ctx, mock.Anything, mock.Anything).Return(nil)
	em.mdi.On("UpsertTokenApproval", em.ctx, &approval.TokenApproval).Return(nil)

	valid, err := em.persistTokenApproval(em.ctx, approval)
	assert.True(t, valid)
	assert.NoError(t, err)

	assert.NotEqual(t, *localID, *approval.LocalID)

	mti.AssertExpectations(t)
}

func TestApprovedBlockchainEventFail(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	approval := newApproval()
	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	localID := fftypes.NewUUID()
	op := &core.Operation{
		Input: fftypes.JSONObject{
			"localId":   localID.String(),
			"connector": approval.Connector,
			"pool":      pool.ID.String(),
		},
	}

	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "F1").Return(pool, nil)
	em.mdi.On("GetTokenApprovalByProtocolID", em.ctx, "ns1", pool.ID, approval.ProtocolID).Return(nil, nil)
	em.mth.On("FindOperationInTransaction", em.ctx, approval.TX.ID, core.OpTypeTokenApproval).Return(op, nil)
	em.mth.On("PersistTransaction", mock.Anything, approval.TX.ID, core.TransactionTypeTokenApproval, "0xffffeeee").Return(true, nil)
	em.mdi.On("GetTokenApprovalByID", em.ctx, "ns1", localID).Return(&core.TokenApproval{}, nil)
	em.mth.On("InsertOrGetBlockchainEvent", em.ctx, mock.MatchedBy(func(e *core.BlockchainEvent) bool {
		return e.Namespace == pool.Namespace && e.Name == approval.Event.Name
	})).Return(nil, fmt.Errorf("pop"))

	valid, err := em.persistTokenApproval(em.ctx, approval)
	assert.False(t, valid)
	assert.EqualError(t, err, "pop")

}

func TestTokensApprovedWithMessageReceived(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	mti := &tokenmocks.Plugin{}

	info := fftypes.JSONObject{"some": "info"}
	approval := &tokens.TokenApproval{
		PoolLocator: "F1",
		TokenApproval: core.TokenApproval{
			Connector:  "erc1155",
			Key:        "0x12345",
			ProtocolID: "123",
			Message:    fftypes.NewUUID(),
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

	em.mdi.On("GetTokenApprovalByProtocolID", em.ctx, "ns1", pool.ID, "123").Return(nil, nil).Times(2)
	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "F1").Return(pool, nil).Once()
	em.mam.On("GetTokenPoolByID", em.ctx, pool.ID).Return(pool, nil).Once()
	em.mth.On("InsertOrGetBlockchainEvent", em.ctx, mock.MatchedBy(func(e *core.BlockchainEvent) bool {
		return e.Namespace == pool.Namespace && e.Name == approval.Event.Name
	})).Return(nil, nil).Times(2)
	em.mdi.On("InsertEvent", em.ctx, mock.MatchedBy(func(ev *core.Event) bool {
		return ev.Type == core.EventTypeBlockchainEventReceived && ev.Namespace == pool.Namespace
	})).Return(nil).Times(2)
	em.mdi.On("UpsertTokenApproval", em.ctx, &approval.TokenApproval).Return(nil).Times(2)
	em.mdi.On("UpdateTokenApprovals", em.ctx, mock.Anything, mock.Anything).Return(nil).Times(2)
	em.mdi.On("GetMessageByID", em.ctx, "ns1", approval.Message).Return(nil, fmt.Errorf("pop")).Once()
	em.mdi.On("GetMessageByID", em.ctx, "ns1", approval.Message).Return(message, nil).Once()
	em.mdi.On("InsertEvent", em.ctx, mock.MatchedBy(func(ev *core.Event) bool {
		return ev.Type == core.EventTypeApprovalConfirmed && ev.Reference == approval.LocalID && ev.Namespace == pool.Namespace
	})).Return(nil).Once()

	err := em.TokensApproved(mti, approval)
	assert.NoError(t, err)

	mti.AssertExpectations(t)
}

func TestTokensApprovedWithMessageSend(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	mti := &tokenmocks.Plugin{}

	info := fftypes.JSONObject{"some": "info"}
	approval := &tokens.TokenApproval{
		PoolLocator: "F1",
		TokenApproval: core.TokenApproval{
			Connector:  "erc1155",
			Key:        "0x12345",
			ProtocolID: "123",
			Message:    fftypes.NewUUID(),
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

	em.mdi.On("GetTokenApprovalByProtocolID", em.ctx, "ns1", pool.ID, "123").Return(nil, nil).Times(2)
	em.mam.On("GetTokenPoolByLocator", em.ctx, "erc1155", "F1").Return(pool, nil).Once()
	em.mam.On("GetTokenPoolByID", em.ctx, pool.ID).Return(pool, nil).Once()
	em.mth.On("InsertOrGetBlockchainEvent", em.ctx, mock.MatchedBy(func(e *core.BlockchainEvent) bool {
		return e.Namespace == pool.Namespace && e.Name == approval.Event.Name
	})).Return(nil, nil).Times(2)
	em.mdi.On("InsertEvent", em.ctx, mock.MatchedBy(func(ev *core.Event) bool {
		return ev.Type == core.EventTypeBlockchainEventReceived && ev.Namespace == pool.Namespace
	})).Return(nil).Times(2)
	em.mdi.On("UpsertTokenApproval", em.ctx, &approval.TokenApproval).Return(nil).Times(2)
	em.mdi.On("UpdateTokenApprovals", em.ctx, mock.Anything, mock.Anything).Return(nil).Times(2)
	em.mdi.On("GetMessageByID", em.ctx, "ns1", mock.Anything).Return(message, nil).Times(2)
	em.mdi.On("ReplaceMessage", em.ctx, mock.MatchedBy(func(msg *core.Message) bool {
		return msg.State == core.MessageStateReady
	})).Return(fmt.Errorf("pop"))
	em.mdi.On("InsertEvent", em.ctx, mock.MatchedBy(func(ev *core.Event) bool {
		return ev.Type == core.EventTypeApprovalConfirmed && ev.Reference == approval.LocalID && ev.Namespace == pool.Namespace
	})).Return(nil).Once()

	err := em.TokensApproved(mti, approval)
	assert.NoError(t, err)

	mti.AssertExpectations(t)
}
