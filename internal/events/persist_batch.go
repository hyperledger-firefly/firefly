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
	"database/sql/driver"
	"errors"

	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

type messageAndData struct {
	message *core.Message
	data    core.DataArray
}

// persistBatch performs very simple validation on each message/data element (hashes) and either persists
// or discards them. Errors are returned only in the case of database failures, which should be retried.
func (em *eventManager) persistBatch(ctx context.Context, batch *core.Batch) (persistedBatch *core.BatchPersisted, valid bool, err error) {
	l := log.L(ctx)

	if batch.ID == nil || batch.Payload.TX.ID == nil || batch.Hash == nil {
		l.Errorf("Invalid batch. Missing ID (%v), transaction ID (%s) or hash (%s)", batch.ID, batch.Payload.TX.ID, batch.Hash)
		return nil, false, nil // This is not retryable. skip this batch
	}

	if len(batch.Payload.Messages) == 0 {
		l.Errorf("Invalid batch '%s'. No messages in batch.", batch.ID)
		return nil, false, nil // This is not retryable. skip this batch
	}

	switch batch.Payload.TX.Type {
	case core.TransactionTypeBatchPin,
		core.TransactionTypeUnpinned,
		core.TransactionTypeContractInvokePin:
	default:
		l.Errorf("Invalid batch '%s'. Invalid transaction type: %s", batch.ID, batch.Payload.TX.Type)
		return nil, false, nil // This is not retryable. skip this batch
	}

	// Set confirmed on the batch (the messages should not be confirmed at this point - that's the aggregator's job)
	persistedBatch, manifest := batch.Confirmed()
	manifestHash := fftypes.HashString(persistedBatch.Manifest.String())

	// Verify the hash calculation.
	if !manifestHash.Equals(batch.Hash) {
		// To cope with existing batches written by v0.13 and older environments, we have to do a more expensive
		// hashing of the whole payload before we reject.
		if batch.Payload.Hash().Equals(batch.Hash) {
			l.Infof("Persisting migrated batch '%s'. Hash is a payload hash: %s", batch.ID, batch.Hash)
		} else {
			l.Errorf("Invalid batch '%s'. Hash does not match payload. Found=%s Expected=%s", batch.ID, manifestHash, batch.Hash)
			return nil, false, nil // This is not retryable. skip this batch
		}
	}

	// Insert the batch
	existing, err := em.database.InsertOrGetBatch(ctx, persistedBatch)
	if err != nil {
		l.Errorf("Failed to insert batch '%s': %s", batch.ID, err)
		return nil, false, err // a persistence failure here is considered retryable (so returned)
	}

	valid, err = em.validateAndPersistBatchContent(ctx, batch)
	if err != nil || !valid {
		return nil, valid, err
	}

	if existing != nil {
		l.Infof("Skipped insert of batch '%s' (already exists)", batch.ID)
		return existing, true, nil
	}
	em.aggregator.cacheBatch(em.aggregator.getBatchCacheKey(persistedBatch.ID, persistedBatch.Hash), persistedBatch, manifest)
	return persistedBatch, true, err
}

func (em *eventManager) validateAndPersistBatchContent(ctx context.Context, batch *core.Batch) (valid bool, err error) {

	// Insert the data entries
	dataByID := make(map[fftypes.UUID]*core.Data)
	for i, data := range batch.Payload.Data {
		if valid = em.validateBatchData(ctx, batch, i, data); !valid {
			return false, nil
		}
		if valid, err = em.checkAndInitiateBlobDownloads(ctx, batch, i, data); !valid || err != nil {
			return false, err
		}
		data.Namespace = em.namespace.Name
		dataByID[*data.ID] = data
	}

	// Insert the message entries
	for i, msg := range batch.Payload.Messages {
		if valid = em.validateBatchMessage(ctx, batch, i, msg); !valid {
			return false, nil
		}
	}

	// We require that the batch contains exactly the set of data that is in the messages - no more or less.
	// While this means an edge case inefficiency of re-transmission of data when sent in multiple messages,
	// that is outweighed by the efficiency it allows in the insertion logic in the majority case.
	matchedData := make(map[fftypes.UUID]bool)
	matchedMsgs := make([]*messageAndData, len(batch.Payload.Messages))
	for iMsg, msg := range batch.Payload.Messages {
		msgData := make(core.DataArray, len(msg.Data))
		for di, dataRef := range msg.Data {
			msgData[di] = dataByID[*dataRef.ID]
			if msgData[di] == nil || !msgData[di].Hash.Equals(dataRef.Hash) {
				log.L(ctx).Errorf("Message '%s' in batch '%s' - data not in-line in batch id='%s' hash='%s'", msg.Header.ID, batch.ID, dataRef.ID, dataRef.Hash)
				return false, nil
			}
			matchedData[*dataRef.ID] = true
		}
		matchedMsgs[iMsg] = &messageAndData{
			message: msg,
			data:    msgData,
		}
	}
	if len(matchedData) != len(dataByID) {
		log.L(ctx).Errorf("Batch '%s' contains %d unique data, but %d are referenced from messages", batch.ID, len(dataByID), len(matchedData))
		return false, nil
	}

	return em.persistBatchContent(ctx, batch, matchedMsgs)
}

func (em *eventManager) validateBatchData(ctx context.Context, batch *core.Batch, i int, data *core.Data) bool {

	l := log.L(ctx)
	l.Tracef("Batch '%s' data %d: %+v", batch.ID, i, data)

	if data == nil {
		l.Errorf("null data entry %d in batch '%s'", i, batch.ID)
		return false
	}

	hash, err := data.CalcHash(ctx)
	if err != nil {
		log.L(ctx).Errorf("Invalid data entry %d in batch '%s': %s", i, batch.ID, err)
		return false
	}
	if data.Hash == nil || *data.Hash != *hash {
		log.L(ctx).Errorf("Invalid data entry %d in batch '%s': Hash=%v Expected=%v", i, batch.ID, data.Hash, hash)
		return false
	}

	return true
}

func (em *eventManager) checkAndInitiateBlobDownloads(ctx context.Context, batch *core.Batch, i int, data *core.Data) (bool, error) {

	if data.Blob != nil && batch.Type == core.BatchTypeBroadcast {
		// Need to check if we need to initiate a download
		fb := database.BlobQueryFactory.NewFilter(ctx)
		blobs, _, err := em.database.GetBlobs(ctx, em.namespace.Name, fb.And(fb.Eq("data_id", data.ID), fb.Eq("hash", data.Blob.Hash)))
		if err != nil {
			return false, err
		}
		if len(blobs) == 0 || blobs[0] == nil {
			if data.Blob.Public == "" {
				log.L(ctx).Errorf("Invalid data entry %d id=%s in batch '%s' - missing public blob reference", i, data.ID, batch.ID)
				return false, nil
			}
			if err = em.sharedDownload.InitiateDownloadBlob(ctx, batch.Payload.TX.ID, data.ID, data.Blob.Public, false /* batch processing does not currently use idempotency keys */); err != nil {
				return false, err
			}
		}

	}

	return true, nil
}

func (em *eventManager) validateBatchMessage(ctx context.Context, batch *core.Batch, i int, msg *core.Message) bool {

	l := log.L(ctx)
	if msg == nil {
		l.Errorf("null message entry %d in batch '%s'", i, batch.ID)
		return false
	}

	if msg.Header.Author != batch.Author || msg.Header.Key != batch.Key {
		log.L(ctx).Errorf("Mismatched key/author '%s'/'%s' on message entry %d in batch '%s'", msg.Header.Key, msg.Header.Author, i, batch.ID)
		return false
	}
	msg.LocalNamespace = em.namespace.Name
	msg.BatchID = batch.ID
	msg.TransactionID = batch.Payload.TX.ID

	l.Tracef("Batch '%s' message %d: %+v", batch.ID, i, msg)

	err := msg.Verify(ctx)
	if err != nil {
		l.Errorf("Invalid message entry %d in batch '%s': %s", i, batch.ID, err)
		return false
	}
	// Set the state to pending, for the insertion stage
	msg.State = core.MessageStatePending
	// Remove any idempotency key
	msg.IdempotencyKey = ""

	return true
}

func (em *eventManager) sentByUs(ctx context.Context, batch *core.Batch) bool {
	localNode, err := em.identity.GetLocalNode(ctx)
	if localNode == nil || err != nil {
		// This is from a node that hasn't yet completed registration, so we can't optimize
		return false
	} else if batch.Node.Equals(localNode.ID) {
		// We sent the batch, so we should already have all the messages and data locally
		return true
	}
	// We didn't send the batch, so all the data should be new - optimize the DB operations for that
	return false
}

func (em *eventManager) verifyAlreadyStored(ctx context.Context, batch *core.Batch) (valid bool, err error) {
	for _, msg := range batch.Payload.Messages {
		msgLocal, _, _, err := em.data.GetMessageWithDataCached(ctx, msg.Header.ID)
		if err != nil {
			return false, err
		}
		if msgLocal == nil {
			log.L(ctx).Errorf("Message entry %s in batch sent by this node, was not found", msg.Header.ID)
			return false, nil
		}
		if !msgLocal.Hash.Equals(msg.Hash) {
			log.L(ctx).Errorf("Message entry %s hash mismatch with already stored. Local=%s BatchMsg=%s", msg.Header.ID, msgLocal.Hash, msg.Hash)
			return false, nil
		}
	}
	return true, nil
}

func (em *eventManager) persistBatchContent(ctx context.Context, batch *core.Batch, matchedMsgs []*messageAndData) (valid bool, err error) {

	// We want to insert the messages and data in the most efficient way we can.
	// If we are sure we wrote the batch, then we do a cached lookup of each in turn - which is efficient
	// because all of those should be in the cache as we wrote them recently.
	if em.sentByUs(ctx, batch) {
		log.L(ctx).Debugf("Batch %s sent by us", batch.ID)
		allStored, err := em.verifyAlreadyStored(ctx, batch)
		if err != nil {
			return false, err
		}
		if allStored {
			return true, nil
		}
		// Fall through otherwise
		log.L(ctx).Warnf("Batch %s was sent by our UUID, but the content was not already stored. Assuming node has been reset", batch.ID)
	}

	// Otherwise try a one-shot insert of all the data, on the basis it's likely unique
	err = em.database.InsertDataArray(ctx, batch.Payload.Data)
	if err != nil {
		log.L(ctx).Debugf("Batch data insert optimization failed for batch '%s': %s", batch.ID, err)
		// Fall back to individual upserts
		for i, data := range batch.Payload.Data {
			if err := em.database.UpsertData(ctx, data, database.UpsertOptimizationExisting); err != nil {
				if errors.Is(err, database.HashMismatch) {
					log.L(ctx).Errorf("Invalid data entry %d in batch '%s'. Hash mismatch with existing record with same UUID '%s' Hash=%s", i, batch.ID, data.ID, data.Hash)
					return false, nil
				}
				log.L(ctx).Errorf("Failed to insert data entry %d in batch '%s': %s", i, batch.ID, err)
				return false, err
			}
		}
	}

	// Then the same one-shot insert of all the messages, on the basis they are likely unique (even if
	// one of the data elements wasn't unique). Likely reasons for exceptions here are idempotent replay,
	// or a root broadcast where "em.sentByUs" returned false, but we actually sent it.
	err = em.database.InsertMessages(ctx, batch.Payload.Messages, func() {
		// If all is well, update the cache when the runAsGroup closes out.
		// It isn't safe to do this before the messages themselves are safely in the database, because the aggregator
		// might wake up and notice the cache before we're written the messages. Meaning we'll clash and override the
		// confirmed updates with un-confirmed batch messages.
		for _, mm := range matchedMsgs {
			em.data.UpdateMessageCache(mm.message, mm.data)
		}
	})
	if err != nil {
		log.L(ctx).Debugf("Batch message insert optimization failed for batch '%s': %s", batch.ID, err)

		// Messages are immutable in their contents, and it's entirely possible we're being sent
		// messages we've already been sent in a previous batch, and subsequently modified th
		// state of (they've been confirmed etc.).
		// So we find a list of those that aren't in the DB and so and insert just those.
		var foundIDs []*core.IDAndSequence
		foundIDs, err = em.database.GetMessageIDs(ctx, batch.Namespace, messageIDFilter(ctx, batch.Payload.Messages))
		if err == nil {
			remainingInserts := make([]*core.Message, 0, len(batch.Payload.Messages))
			for _, m := range batch.Payload.Messages {
				isFound := false
				for _, foundID := range foundIDs {
					if foundID.ID.Equals(m.Header.ID) {
						isFound = true
						log.L(ctx).Warnf("Message %s in batch '%s' is a duplicate", m.Header.ID, batch.ID)
						break
					}
				}
				if !isFound {
					remainingInserts = append(remainingInserts, m)
				}
			}
			if len(remainingInserts) > 0 {
				// Only the remaining ones get updates
				err = em.database.InsertMessages(ctx, batch.Payload.Messages, func() {
					for _, mm := range matchedMsgs {
						for _, m := range remainingInserts {
							if mm.message.Header.ID.Equals(m.Header.ID) {
								em.data.UpdateMessageCache(mm.message, mm.data)
							}
						}
					}
				})
			}
		}
		// If we have an error at this point, we cannot insert (must not be a duplicate)
		if err != nil {
			log.L(ctx).Errorf("Failed to insert messages: %s", err)
			return false, err // a persistence failure here is considered retryable (so returned)
		}
	}

	return true, nil
}

func messageIDFilter(ctx context.Context, msgs []*core.Message) ffapi.Filter {
	fb := database.MessageQueryFactory.NewFilter(ctx)
	ids := make([]driver.Value, len(msgs))
	for i, msg := range msgs {
		ids[i] = msg.Header.ID
	}
	return fb.In("id", ids)
}
