// Copyright © 2024 Kaleido, Inc.
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
	"context"
	"fmt"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/pkg/blockchain"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

type eventBatchContext struct {
	contractListenerResults map[string]*core.ContractListener
	topicsByEventID         map[string]string
	chainEventsToInsert     []*core.BlockchainEvent
	postInsert              []func() error
}

func (bc *eventBatchContext) addEventToInsert(event *core.BlockchainEvent, topic string) {
	bc.chainEventsToInsert = append(bc.chainEventsToInsert, event)
	bc.topicsByEventID[event.ID.String()] = topic
}

func buildBlockchainEvent(ns string, subID *fftypes.UUID, event *blockchain.Event, tx *core.BlockchainTransactionRef) *core.BlockchainEvent {
	ev := &core.BlockchainEvent{
		ID:         fftypes.NewUUID(),
		Namespace:  ns,
		Listener:   subID,
		Source:     event.Source,
		ProtocolID: event.ProtocolID,
		Name:       event.Name,
		Output:     event.Output,
		Info:       event.Info,
		Timestamp:  event.Timestamp,
	}
	if tx != nil {
		ev.TX = *tx
	}
	return ev
}

func (em *eventManager) getChainListenerByProtocolIDCached(ctx context.Context, protocolID string, bc *eventBatchContext) (*core.ContractListener, error) {
	// Even a negative result is cached in the scope of the event batch (so we don't spam the DB hundreds of times in one tight loop to get not-found)
	if l, batchResult := bc.contractListenerResults[protocolID]; batchResult {
		return l, nil
	}
	l, err := em.getChainListenerCached(fmt.Sprintf("pid:%s", protocolID), func() (*core.ContractListener, error) {
		return em.database.GetContractListenerByBackendID(ctx, em.namespace.Name, protocolID)
	})
	if err != nil {
		return nil, err
	}
	bc.contractListenerResults[protocolID] = l // includes nil results
	return l, nil
}

// handleBlockchainBatchPinEvent handles a blockchain event, returning true if the event was created, false if it was a duplicate along with an error if any failures occur
func (em *eventManager) maybePersistBlockchainEvent(ctx context.Context, chainEvent *core.BlockchainEvent, listener *core.ContractListener) (bool, error) {
	existing, err := em.txHelper.InsertOrGetBlockchainEvent(ctx, chainEvent)
	if err != nil {
		return false, err
	}
	if existing != nil {
		log.L(ctx).Debugf("Ignoring duplicate blockchain event %s", chainEvent.ProtocolID)
		// Return the ID of the existing event
		chainEvent.ID = existing.ID
		return false, nil
	}
	topic := em.getTopicForChainListener(listener)
	ffEvent := core.NewEvent(core.EventTypeBlockchainEventReceived, chainEvent.Namespace, chainEvent.ID, chainEvent.TX.ID, topic)
	return true, em.database.InsertEvent(ctx, ffEvent)
}

func (em *eventManager) getChainListenerCached(cacheKey string, getter func() (*core.ContractListener, error)) (*core.ContractListener, error) {

	if cachedValue := em.chainListenerCache.Get(cacheKey); cachedValue != nil {
		return cachedValue.(*core.ContractListener), nil
	}
	listener, err := getter()
	if listener == nil || err != nil {
		return nil, err
	}
	em.chainListenerCache.Set(cacheKey, listener)
	return listener, err
}

func (em *eventManager) getTopicForChainListener(listener *core.ContractListener) string {
	if listener == nil {
		return core.SystemBatchPinTopic
	}
	var topic string
	if listener.Topic != "" {
		topic = listener.Topic
	} else {
		topic = listener.ID.String()
	}
	return topic
}

func (em *eventManager) maybePersistBlockchainEvents(ctx context.Context, bc *eventBatchContext) error {
	// Attempt to insert all the events in one go, using efficient InsertMany semantics
	inserted, err := em.txHelper.InsertNewBlockchainEvents(ctx, bc.chainEventsToInsert)
	if err != nil {
		return err
	}
	// Only the ones newly inserted need events emitting
	for _, chainEvent := range inserted {
		topic := bc.topicsByEventID[chainEvent.ID.String()] // bc.addEvent() ensures this is there
		ffEvent := core.NewEvent(core.EventTypeBlockchainEventReceived, chainEvent.Namespace, chainEvent.ID, chainEvent.TX.ID, topic)
		if err := em.database.InsertEvent(ctx, ffEvent); err != nil {
			return err
		}
	}
	return nil
}

func (em *eventManager) emitBlockchainEventMetric(event *blockchain.Event) {
	if em.metrics.IsMetricsEnabled() && event.Location != "" && event.Signature != "" {
		em.metrics.BlockchainEvent(event.Location, event.Signature)
	}
}

func (em *eventManager) BlockchainEventBatch(batch []*blockchain.EventToDispatch) error {
	return em.retry.Do(em.ctx, "persist blockchain event", func(attempt int) (bool, error) {
		bc := &eventBatchContext{
			contractListenerResults: make(map[string]*core.ContractListener),
			topicsByEventID:         make(map[string]string),
		}
		return true, em.database.RunAsGroup(em.ctx, func(ctx context.Context) error {
			// Process the events, generating the optimized list of event inserts
			for _, event := range batch {
				switch event.Type {
				case blockchain.EventTypeForListener:
					if err := em.handleBlockchainEventForListener(ctx, event.ForListener, bc); err != nil {
						return err
					}
				case blockchain.EventTypeBatchPinComplete:
					if err := em.handleBlockchainBatchPinEvent(ctx, event.BatchPinComplete, bc); err != nil {
						return err
					}
				case blockchain.EventTypeNetworkAction:
					if err := em.handleBlockchainNetworkAction(ctx, event.NetworkAction, bc); err != nil {
						return err
					}
				}
			}
			// Do the optimized inserts
			if len(bc.chainEventsToInsert) > 0 {
				if err := em.maybePersistBlockchainEvents(ctx, bc); err != nil {
					return err
				}
			}
			// Batch pins require processing after the event is inserted
			for _, postEvent := range bc.postInsert {
				if err := postEvent(); err != nil {
					return err
				}
			}
			return nil
		})
	})
}

func (em *eventManager) handleBlockchainEventForListener(ctx context.Context, event *blockchain.EventForListener, bc *eventBatchContext) error {
	listener, err := em.getChainListenerByProtocolIDCached(ctx, event.ListenerID, bc)
	if err != nil {
		return err
	}
	if listener == nil {
		log.L(ctx).Warnf("Event received from unknown subscription %s", event.ListenerID)
		return nil // no retry
	}
	if listener.Namespace != em.namespace.Name {
		log.L(ctx).Debugf("Ignoring blockchain event from different namespace '%s'", listener.Namespace)
		return nil
	}
	listener.Namespace = em.namespace.Name

	chainEvent := buildBlockchainEvent(listener.Namespace, listener.ID, event.Event, &core.BlockchainTransactionRef{
		BlockchainID: event.BlockchainTXID,
	})
	bc.addEventToInsert(chainEvent, em.getTopicForChainListener(listener))
	em.emitBlockchainEventMetric(event.Event)
	return nil
}
