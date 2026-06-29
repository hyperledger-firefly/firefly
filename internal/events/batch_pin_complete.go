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

package events

import (
	"context"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/pkg/blockchain"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

// BatchPinComplete is called in-line with a particular ledger's stream of events, so while we
// block here this blockchain event remains un-acknowledged, and no further events will arrive from this
// particular ledger.
//
// We must block here long enough to get the payload from the sharedstorage, persist the messages in the correct
// sequence, and also persist all the data.
func (em *eventManager) handleBlockchainBatchPinEvent(ctx context.Context, event *blockchain.BatchPinCompleteEvent, bc *eventBatchContext) error {
	batchPin := event.Batch

	if em.multiparty == nil {
		log.L(ctx).Errorf("Ignoring batch pin from non-multiparty network!")
		return nil
	}
	if batchPin.TransactionID == nil {
		log.L(ctx).Errorf("Invalid BatchPin transaction - ID is nil")
		return nil // move on
	}
	if event.Namespace != em.namespace.Name {
		log.L(ctx).Debugf("Ignoring batch pin from different namespace '%s'", event.Namespace)
		return nil // move on
	}

	if batchPin.TransactionType == "" {
		batchPin.TransactionType = core.TransactionTypeBatchPin
	}

	log.L(ctx).Infof("-> BatchPinComplete batch=%s txn=%s signingIdentity=%s", batchPin.BatchID, batchPin.Event.ProtocolID, event.SigningKey.Value)
	defer func() {
		log.L(ctx).Infof("<- BatchPinComplete batch=%s txn=%s signingIdentity=%s", batchPin.BatchID, batchPin.Event.ProtocolID, event.SigningKey.Value)
	}()
	log.L(ctx).Tracef("BatchPinComplete batch=%s info: %+v", batchPin.BatchID, batchPin.Event.Info)

	if err := em.persistBatchTransaction(ctx, batchPin); err != nil {
		return err
	}
	chainEvent := buildBlockchainEvent(em.namespace.Name, nil, &batchPin.Event, &core.BlockchainTransactionRef{
		Type:         batchPin.TransactionType,
		ID:           batchPin.TransactionID,
		BlockchainID: batchPin.Event.BlockchainTXID,
	})
	// Defer the event insert itself to the end
	bc.addEventToInsert(chainEvent, em.getTopicForChainListener(nil))
	bc.postInsert = append(bc.postInsert, func() error {
		em.emitBlockchainEventMetric(&batchPin.Event)
		return em.postBlockchainBatchPinEventInsert(ctx, event)
	})
	return nil
}

func (em *eventManager) postBlockchainBatchPinEventInsert(ctx context.Context, event *blockchain.BatchPinCompleteEvent) error {
	batchPin := event.Batch
	private := batchPin.BatchPayloadRef == ""
	if err := em.persistContexts(ctx, batchPin, event.SigningKey, private); err != nil {
		return err
	}

	batch, _, err := em.aggregator.GetBatchForPin(ctx, &core.Pin{
		Batch:     batchPin.BatchID,
		BatchHash: batchPin.BatchHash,
	})
	if err != nil {
		return err
	}
	// Kick off a download for broadcast batches if the batch isn't already persisted
	if !private && batch == nil {
		if err := em.sharedDownload.InitiateDownloadBatch(ctx, batchPin.TransactionID, batchPin.BatchPayloadRef, false /* batch processing does not currently use idempotency keys */); err != nil {
			return err
		}
	}
	return nil
}

func (em *eventManager) persistBatchTransaction(ctx context.Context, batchPin *blockchain.BatchPin) error {
	_, err := em.txHelper.PersistTransaction(ctx, batchPin.TransactionID, batchPin.TransactionType, batchPin.Event.BlockchainTXID)
	return err
}

func (em *eventManager) persistContexts(ctx context.Context, batchPin *blockchain.BatchPin, signingKey *core.VerifierRef, private bool) error {
	pins := make([]*core.Pin, len(batchPin.Contexts))
	for idx, hash := range batchPin.Contexts {
		pins[idx] = &core.Pin{
			Namespace: em.namespace.Name,
			Masked:    private,
			Hash:      hash,
			Batch:     batchPin.BatchID,
			BatchHash: batchPin.BatchHash,
			Index:     int64(idx),
			Signer:    signingKey.Value, // We don't store the type as we can infer that from the blockchain
			Created:   fftypes.Now(),
		}
	}

	// First attempt a single batch insert
	err := em.database.InsertPins(ctx, pins)
	if err == nil {
		return nil
	}
	log.L(ctx).Warnf("Batch insert of pins failed - assuming replay and performing upserts: %s", err)

	// Fall back to an upsert
	for _, pin := range pins {
		if err := em.database.UpsertPin(ctx, pin); err != nil {
			return err
		}
	}
	return nil
}
