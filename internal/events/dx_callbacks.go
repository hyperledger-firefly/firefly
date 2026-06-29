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
	"database/sql/driver"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/hyperledger-firefly/firefly/pkg/dataexchange"
)

// Check that the sending data exchange peer corresponds to the node listed in the batch,
// and that the node has been registered to the org listed in the batch.
func (em *eventManager) checkReceivedOffchainIdentity(ctx context.Context, peerID, author string, nodeID *fftypes.UUID) (valid bool, err error) {
	l := log.L(ctx)

	// Resolve the node for the peer ID and check against the node specified on the batch
	node, err := em.identity.FindIdentityForVerifier(ctx, []core.IdentityType{core.IdentityTypeNode}, &core.VerifierRef{
		Type:  core.VerifierTypeFFDXPeerID,
		Value: peerID,
	})
	if err != nil {
		return false, err
	}
	if node == nil {
		l.Errorf("Peer '%s' could not be resolved", peerID)
		return false, nil
	}
	if !node.ID.Equals(nodeID) {
		l.Errorf("Peer '%s' resolved to node '%s', which does not match expected '%s'", peerID, node.ID, nodeID)
		return false, nil
	}

	// Look up the identity specified on the batch
	org, retryable, err := em.identity.CachedIdentityLookupMustExist(ctx, author)
	if err != nil && retryable {
		l.Errorf("Failed to retrieve org: %v", err)
		return false, err // retryable error
	}
	if org == nil || err != nil {
		l.Errorf("Identity %s not found", author)
		return false, nil
	}

	// One of the orgs in the hierarchy of the author must be the owner of the peer node
	return em.identity.ValidateNodeOwner(ctx, node, org)
}

func (em *eventManager) privateBatchReceived(peerID string, batch *core.Batch, wrapperGroup *core.Group) (manifest string, err error) {
	if em.multiparty == nil {
		log.L(em.ctx).Errorf("Ignoring private batch from non-multiparty network!")
		return "", nil
	}
	if batch.Namespace != em.namespace.NetworkName {
		log.L(em.ctx).Debugf("Ignoring private batch from different namespace '%s'", batch.Namespace)
		return "", nil
	}
	batch.Namespace = em.namespace.Name

	sender := &core.Member{
		Identity: batch.Author,
		Node:     batch.Node,
	}

	// Retry for persistence errors (not validation errors)
	err = em.retry.Do(em.ctx, "private batch received", func(attempt int) (bool, error) {
		return true, em.database.RunAsGroup(em.ctx, func(ctx context.Context) error {
			l := log.L(ctx)

			if !core.IsPinned(batch.Payload.TX.Type) {
				if wrapperGroup != nil {
					if valid, err := em.messaging.EnsureLocalGroup(ctx, wrapperGroup, sender); err != nil {
						return err
					} else if !valid {
						l.Errorf("Invalid transmission: invalid group: %+v", wrapperGroup)
						return nil
					}
				}
			}

			if valid, err := em.checkReceivedOffchainIdentity(ctx, peerID, batch.Author, batch.Node); err != nil {
				return err
			} else if !valid {
				l.Errorf("Batch '%s' received from invalid author '%s' for peer '%s'", batch.ID, batch.Author, peerID)
				return nil
			}

			persistedBatch, valid, err := em.persistBatch(ctx, batch)
			if err != nil || !valid {
				l.Errorf("Batch '%s' from %s processing failed valid=%t: %s", batch.ID, peerID, valid, err)
				return err // retry - persistBatch only returns retryable errors
			}

			if !core.IsPinned(batch.Payload.TX.Type) {
				// We need to confirm all these messages immediately.
				if err := em.markUnpinnedMessagesConfirmed(ctx, batch); err != nil {
					return err
				}
			}

			manifest = persistedBatch.Manifest.String()
			return nil
		})
	})
	if err != nil {
		return "", err
	}
	// Poke the aggregator to do its stuff - after we have committed the transaction so the pins are visible
	if core.IsPinned(batch.Payload.TX.Type) {
		em.aggregator.queueBatchRewind(batch.ID)
	}
	return manifest, err
}

func (em *eventManager) markUnpinnedMessagesConfirmed(ctx context.Context, batch *core.Batch) error {

	// Update all the messages in the batch with the batch ID
	msgIDs := make([]driver.Value, len(batch.Payload.Messages))
	for i, msg := range batch.Payload.Messages {
		msgIDs[i] = msg.Header.ID
	}
	fb := database.MessageQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.In("id", msgIDs),
		fb.Eq("state", core.MessageStatePending), // In the outside chance another state transition happens first (which supersedes this)
	)

	// Immediate confirmation if no transaction
	update := database.MessageQueryFactory.NewUpdate(ctx).
		Set("batch", batch.ID).
		Set("state", core.MessageStateConfirmed).
		Set("confirmed", fftypes.Now())

	if err := em.database.UpdateMessages(ctx, em.namespace.Name, filter, update); err != nil {
		return err
	}

	for _, msg := range batch.Payload.Messages {
		for _, topic := range msg.Header.Topics {
			// One event per topic
			event := core.NewEvent(core.EventTypeMessageConfirmed, batch.Namespace, msg.Header.ID, batch.Payload.TX.ID, topic)
			event.Correlator = msg.Header.CID
			if err := em.database.InsertEvent(ctx, event); err != nil {
				return err
			}
		}
	}

	return nil
}

func (em *eventManager) DXEvent(dx dataexchange.Plugin, event dataexchange.DXEvent) error {
	switch event.Type() {
	case dataexchange.DXEventTypePrivateBlobReceived:
		em.privateBlobReceived(dx, event)
	case dataexchange.DXEventTypeMessageReceived:
		// Batches are significant items of work in their own right, so get dispatched to their own routines
		go em.messageReceived(dx, event)
	default:
		log.L(em.ctx).Errorf("Invalid data exchange event type from %s: %d", dx.Name(), event.Type())
		event.Ack() // still ack
	}
	return nil
}

func (em *eventManager) messageReceived(dx dataexchange.Plugin, event dataexchange.DXEvent) {
	l := log.L(em.ctx)

	mr := event.MessageReceived()
	l.Infof("Private batch received from %s peer '%s'", dx.Name(), mr.PeerID)

	manifestString, err := em.privateBatchReceived(mr.PeerID, mr.Transport.Batch, mr.Transport.Group)
	if err != nil {
		l.Warnf("Exited while persisting batch: %s", err)
		// We do NOT ack here as we broke out of the retry
		return
	}
	event.AckWithManifest(manifestString)
}

func (em *eventManager) privateBlobReceived(dx dataexchange.Plugin, event dataexchange.DXEvent) {
	br := event.PrivateBlobReceived()
	log.L(em.ctx).Infof("Blob received event from data exchange %s: Peer='%s' Hash='%v' PayloadRef='%s'", dx.Name(), br.PeerID, &br.Hash, br.PayloadRef)

	if br.PeerID == "" || len(br.PeerID) > 256 || br.PayloadRef == "" || len(br.PayloadRef) > 1024 {
		log.L(em.ctx).Errorf("Invalid blob received event from data exchange: Peer='%s' Hash='%v' PayloadRef='%s'", br.PeerID, &br.Hash, br.PayloadRef)
		event.Ack() // Still confirm the event
		return
	}
	if br.Namespace != em.namespace.NetworkName {
		log.L(em.ctx).Debugf("Ignoring blob from different namespace '%s'", br.Namespace)
		event.Ack() // Still confirm the event
		return
	}

	dataID, err := fftypes.ParseUUID(em.ctx, br.DataID)
	if err != nil {
		log.L(em.ctx).Warnf("Ignoring blob with invalid data ID '%s'", br.DataID)
		return
	}

	// Dispatch to the blob receiver for efficient batch DB operations
	em.blobReceiver.blobReceived(em.ctx, &blobNotification{
		blob: &core.Blob{
			Namespace:  em.namespace.Name,
			Peer:       br.PeerID,
			PayloadRef: br.PayloadRef,
			Hash:       &br.Hash,
			Size:       br.Size,
			Created:    fftypes.Now(),
			DataID:     dataID,
		},
		onComplete: func() {
			event.Ack()
		},
	})
}
