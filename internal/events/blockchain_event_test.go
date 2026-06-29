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
	"github.com/hyperledger-firefly/firefly/pkg/blockchain"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestContractEventWithRetries(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	ev := &blockchain.EventForListener{
		ListenerID: "sb-1",
		Event: &blockchain.Event{
			BlockchainTXID: "0xabcd1234",
			ProtocolID:     "10/20/30",
			Name:           "Changed",
			Output: fftypes.JSONObject{
				"value": "1",
			},
			Info: fftypes.JSONObject{
				"blockNumber": "10",
			},
		},
	}
	sub := &core.ContractListener{
		Namespace: "ns1",
		ID:        fftypes.NewUUID(),
		Topic:     "topic1",
	}
	var eventID *fftypes.UUID

	em.mdi.On("GetContractListenerByBackendID", mock.Anything, "ns1", "sb-1").Return(nil, fmt.Errorf("snap")).Once()
	em.mdi.On("GetContractListenerByBackendID", mock.Anything, "ns1", "sb-1").Return(sub, nil).Times(1) // cached
	em.mth.On("InsertNewBlockchainEvents", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("crackle")).Once()
	mInsert := em.mth.On("InsertNewBlockchainEvents", mock.Anything, mock.MatchedBy(func(events []*core.BlockchainEvent) bool {
		if len(events) != 1 {
			return false
		}
		e := events[0]
		eventID = e.ID
		return *e.Listener == *sub.ID && e.Name == "Changed" && e.Namespace == "ns1"
	})).Times(2)
	mInsert.Run(func(args mock.Arguments) {
		// Mock return for all-new events
		mInsert.Return(args[1].([]*core.BlockchainEvent), nil)
	})
	em.mdi.On("InsertEvent", mock.Anything, mock.Anything).Return(fmt.Errorf("pop")).Once()
	em.mdi.On("InsertEvent", mock.Anything, mock.MatchedBy(func(e *core.Event) bool {
		return e.Type == core.EventTypeBlockchainEventReceived && e.Reference != nil && e.Reference.Equals(eventID) && e.Topic == "topic1"
	})).Return(nil).Once()

	err := em.BlockchainEventBatch([]*blockchain.EventToDispatch{
		{
			Type:        blockchain.EventTypeForListener,
			ForListener: ev,
		},
	})
	assert.NoError(t, err)

}

func TestContractEventUnknownSubscription(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	ev := &blockchain.EventForListener{
		ListenerID: "sb-1",
		Event: &blockchain.Event{
			BlockchainTXID: "0xabcd1234",
			Name:           "Changed",
			Output: fftypes.JSONObject{
				"value": "1",
			},
			Info: fftypes.JSONObject{
				"blockNumber": "10",
			},
		},
	}

	em.mdi.On("GetContractListenerByBackendID", mock.Anything, "ns1", "sb-1").Once().Return(nil, nil)

	err := em.BlockchainEventBatch([]*blockchain.EventToDispatch{
		{
			Type:        blockchain.EventTypeForListener,
			ForListener: ev,
		},
		{
			Type:        blockchain.EventTypeForListener,
			ForListener: ev, // second event should read from cache (even though listener is not found)
		},
	})
	assert.NoError(t, err)

}

func TestContractEventWrongNS(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	ev := &blockchain.EventForListener{
		ListenerID: "sb-1",
		Event: &blockchain.Event{
			BlockchainTXID: "0xabcd1234",
			Name:           "Changed",
			Output: fftypes.JSONObject{
				"value": "1",
			},
			Info: fftypes.JSONObject{
				"blockNumber": "10",
			},
		},
	}
	sub := &core.ContractListener{
		Namespace: "ns2",
		ID:        fftypes.NewUUID(),
		Topic:     "topic1",
	}

	em.mdi.On("GetContractListenerByBackendID", mock.Anything, "ns1", "sb-1").Return(sub, nil)

	err := em.BlockchainEventBatch([]*blockchain.EventToDispatch{
		{
			Type:        blockchain.EventTypeForListener,
			ForListener: ev,
		},
	})
	assert.NoError(t, err)

}

// TODO: Add test case for event not existing
func TestPersistBlockchainEventDuplicate(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	ev := &core.BlockchainEvent{
		ID:         fftypes.NewUUID(),
		Name:       "Changed",
		Namespace:  "ns1",
		ProtocolID: "10/20/30",
		Output: fftypes.JSONObject{
			"value": "1",
		},
		Info: fftypes.JSONObject{
			"blockNumber": "10",
		},
		Listener: fftypes.NewUUID(),
	}
	existingID := fftypes.NewUUID()

	em.mth.On("InsertOrGetBlockchainEvent", mock.Anything, ev).
		Return(&core.BlockchainEvent{ID: existingID}, nil)

	created, err := em.maybePersistBlockchainEvent(em.ctx, ev, nil)
	assert.NoError(t, err)
	assert.Equal(t, existingID, ev.ID)
	assert.False(t, created)

}

func TestGetTopicForChainListenerFallback(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	sub := &core.ContractListener{
		Namespace: "ns1",
		ID:        fftypes.NewUUID(),
		Topic:     "",
	}

	topic := em.getTopicForChainListener(sub)
	assert.Equal(t, sub.ID.String(), topic)
}

func TestBlockchainEventMetric(t *testing.T) {
	em := newTestEventManagerWithMetrics(t)
	defer em.cleanup(t)
	em.mmi.On("BlockchainEvent", mock.Anything, mock.Anything).Return()

	event := blockchain.Event{
		BlockchainTXID: "0xabcd1234",
		Name:           "Changed",
		Output: fftypes.JSONObject{
			"value": "1",
		},
		Info: fftypes.JSONObject{
			"blockNumber": "10",
		},
		Location:  "0x12345",
		Signature: "John Hancock",
	}

	em.emitBlockchainEventMetric(&event)
}
