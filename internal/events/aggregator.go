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
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/common/pkg/retry"
	"github.com/hyperledger-firefly/firefly/internal/cache"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/internal/data"
	"github.com/hyperledger-firefly/firefly/internal/definitions"
	"github.com/hyperledger-firefly/firefly/internal/identity"
	"github.com/hyperledger-firefly/firefly/internal/metrics"
	"github.com/hyperledger-firefly/firefly/internal/privatemessaging"
	"github.com/hyperledger-firefly/firefly/pkg/blockchain"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

const (
	aggregatorOffsetName = "ff_aggregator"
)

type aggregator struct {
	ctx          context.Context
	namespace    string
	database     database.Plugin
	messaging    privatemessaging.Manager
	definitions  definitions.Handler
	identity     identity.Manager
	data         data.Manager
	eventPoller  *eventPoller
	verifierType core.VerifierType
	retry        *retry.Retry
	metrics      metrics.Manager
	batchCache   cache.CInterface
	rewinder     *rewinder
}

type batchCacheEntry struct {
	batch    *core.BatchPersisted
	manifest *core.BatchManifest
}

func broadcastContext(topic string) *fftypes.Bytes32 {
	h := sha256.New()
	h.Write([]byte(topic))
	return fftypes.HashResult(h)
}

func privateContext(topic string, group *fftypes.Bytes32) *fftypes.Bytes32 {
	h := sha256.New()
	h.Write([]byte(topic))
	h.Write((*group)[:])
	return fftypes.HashResult(h)
}

func privatePinHash(topic string, group *fftypes.Bytes32, identity string, nonce int64) *fftypes.Bytes32 {
	h := sha256.New()
	h.Write([]byte(topic))
	h.Write((*group)[:])
	h.Write([]byte(identity))
	nonceBytes := make([]byte, 8)
	n := uint64(0)
	if nonce > 0 {
		n = uint64(nonce)
	}
	binary.BigEndian.PutUint64(nonceBytes, n)
	h.Write(nonceBytes)
	return fftypes.HashResult(h)
}

func newAggregator(ctx context.Context, ns string, di database.Plugin, bi blockchain.Plugin, pm privatemessaging.Manager, sh definitions.Handler, im identity.Manager, dm data.Manager, en *eventNotifier, mm metrics.Manager, cacheManager cache.Manager) (*aggregator, error) {
	batchSize := config.GetUint64(coreconfig.EventAggregatorBatchSize)
	ag := &aggregator{
		ctx:          log.WithLogFields(ctx, "role", "aggregator"),
		namespace:    ns,
		database:     di,
		messaging:    pm,
		definitions:  sh,
		identity:     im,
		data:         dm,
		verifierType: bi.VerifierType(),
		metrics:      mm,
	}

	batchCache, err := cacheManager.GetCache(
		cache.NewCacheConfig(
			ctx,
			coreconfig.CacheBatchLimit,
			coreconfig.CacheBatchTTL,
			ns,
		),
	)
	if err != nil {
		return nil, err
	}
	ag.batchCache = batchCache
	firstEvent := core.SubOptsFirstEvent(config.GetString(coreconfig.EventAggregatorFirstEvent))
	ag.eventPoller = newEventPoller(ctx, di, en, &eventPollerConf{
		eventBatchSize:             batchSize,
		eventBatchTimeout:          config.GetDuration(coreconfig.EventAggregatorBatchTimeout),
		eventPollTimeout:           config.GetDuration(coreconfig.EventAggregatorPollTimeout),
		startupOffsetRetryAttempts: config.GetInt(coreconfig.OrchestratorStartupAttempts),
		retry: retry.Retry{
			InitialDelay: config.GetDuration(coreconfig.EventAggregatorRetryInitDelay),
			MaximumDelay: config.GetDuration(coreconfig.EventAggregatorRetryMaxDelay),
			Factor:       config.GetFloat64(coreconfig.EventAggregatorRetryFactor),
		},
		firstEvent:       &firstEvent,
		namespace:        ns,
		offsetType:       core.OffsetTypeAggregator,
		offsetName:       aggregatorOffsetName,
		newEventsHandler: ag.processPinsEventsHandler,
		getItems:         ag.getPins,
		queryFactory:     database.PinQueryFactory,
		addCriteria: func(af ffapi.AndFilter) ffapi.AndFilter {
			fb := af.Builder()
			return af.Condition(fb.Eq("dispatched", false))
		},
		maybeRewind: ag.rewindOffchainBatches,
	})
	ag.retry = &ag.eventPoller.conf.retry
	ag.rewinder = newRewinder(ag)
	return ag, nil
}

func (ag *aggregator) start() {
	ag.rewinder.start()
	ag.eventPoller.start()
}

func (ag *aggregator) queueBatchRewind(batchID *fftypes.UUID) {
	log.L(ag.ctx).Debugf("Queuing rewind for batch %s", batchID)
	ag.rewinder.rewindRequests <- rewind{
		rewindType: rewindBatch,
		uuid:       *batchID,
	}
}

func (ag *aggregator) queueMessageRewind(msgID *fftypes.UUID) {
	log.L(ag.ctx).Debugf("Queuing rewind for message %s", msgID)
	ag.rewinder.rewindRequests <- rewind{
		rewindType: rewindMessage,
		uuid:       *msgID,
	}
}

func (ag *aggregator) queueBlobRewind(hash *fftypes.Bytes32) {
	log.L(ag.ctx).Debugf("Queuing rewind for blob %s", hash)
	ag.rewinder.rewindRequests <- rewind{
		rewindType: rewindBlob,
		hash:       *hash,
	}
}

func (ag *aggregator) queueDIDRewind(did string) {
	log.L(ag.ctx).Debugf("Queuing rewind for author DID %s", did)
	ag.rewinder.rewindRequests <- rewind{
		rewindType: rewindDIDConfirmed,
		did:        did,
	}
}

func (ag *aggregator) rewindOffchainBatches() (bool, int64) {

	batchIDs := ag.rewinder.popRewinds()
	if len(batchIDs) == 0 {
		return false, 0
	}

	// Retry indefinitely for database errors (until the context closes)
	var rewindBatch *fftypes.UUID
	var offset int64
	_ = ag.retry.Do(ag.ctx, "check for off-chain batch deliveries", func(attempt int) (retry bool, err error) {
		pfb := database.PinQueryFactory.NewFilter(ag.ctx)
		pinFilter := pfb.And(
			pfb.In("batch", batchIDs),
			pfb.Eq("dispatched", false),
		).Sort("sequence").Limit(1) // only need the one oldest sequence
		sequences, _, err := ag.database.GetPins(ag.ctx, ag.namespace, pinFilter)
		if err != nil {
			return true, err
		}
		if len(sequences) > 0 {
			rewindBatch = sequences[0].Batch
			offset = sequences[0].Sequence - 1 // offset is set to the last event we saw (so one behind the next pin we want)
		}
		return false, nil
	})
	if rewindBatch != nil {
		log.L(ag.ctx).Debugf("Aggregator popped rewind to pin %d for batch %s", offset, rewindBatch)
	}
	return rewindBatch != nil, offset
}

func (ag *aggregator) processWithBatchState(callback func(ctx context.Context, state *batchState) error) error {
	state := newBatchState(ag)

	err := ag.database.RunAsGroup(ag.ctx, func(ctx context.Context) (err error) {
		if err := callback(ctx, state); err != nil {
			return err
		}
		if len(state.PreFinalize) == 0 {
			return state.RunFinalize(ctx)
		}
		return nil
	})
	if err != nil {
		return err
	}

	if len(state.PreFinalize) > 0 {
		if err := state.RunPreFinalize(ag.ctx); err != nil {
			return err
		}
		err = ag.database.RunAsGroup(ag.ctx, func(ctx context.Context) error {
			return state.RunFinalize(ctx)
		})
		if err != nil {
			return err
		}
	}
	state.queueRewinds(ag)
	return nil
}

func (ag *aggregator) processPinsEventsHandler(items []core.LocallySequenced) (repoll bool, err error) {
	pins := make([]*core.Pin, len(items))
	for i, item := range items {
		pins[i] = item.(*core.Pin)
	}

	return false, ag.processWithBatchState(func(ctx context.Context, state *batchState) error {
		return ag.processPins(ctx, pins, state)
	})
}

func (ag *aggregator) getPins(ctx context.Context, filter ffapi.Filter, offset int64) ([]core.LocallySequenced, error) {
	log.L(ctx).Tracef("Reading page of pins > %d (first pin would be %d)", offset, offset+1)
	pins, _, err := ag.database.GetPins(ctx, ag.namespace, filter)
	ls := make([]core.LocallySequenced, len(pins))
	for i, p := range pins {
		ls[i] = p
	}
	return ls, err
}

func (ag *aggregator) extractBatchMessagePin(manifest *core.BatchManifest, requiredIndex int64) (totalBatchPins int64, msgEntry *core.MessageManifestEntry, msgBaseIndex int64) {
	for _, batchMsg := range manifest.Messages {
		batchMsgBaseIdx := totalBatchPins
		for i := 0; i < batchMsg.Topics; i++ {
			if totalBatchPins == requiredIndex {
				msgEntry = batchMsg
				msgBaseIndex = batchMsgBaseIdx
			}
			totalBatchPins++
		}
	}
	return totalBatchPins, msgEntry, msgBaseIndex
}

func (ag *aggregator) migrateManifest(ctx context.Context, persistedBatch *core.BatchPersisted) *core.BatchManifest {
	// In version v0.13.x and earlier, we stored the full batch
	var fullPayload core.BatchPayload
	err := persistedBatch.Manifest.Unmarshal(ctx, &fullPayload)
	if err != nil {
		log.L(ctx).Errorf("Invalid migration persisted batch: %s", err)
		return nil
	}
	if len(fullPayload.Messages) == 0 {
		log.L(ctx).Errorf("Invalid migration persisted batch: no payload")
		return nil
	}

	return persistedBatch.GenManifest(fullPayload.Messages, fullPayload.Data)
}

func (ag *aggregator) extractManifest(ctx context.Context, batch *core.BatchPersisted) *core.BatchManifest {

	var manifest core.BatchManifest
	err := batch.Manifest.Unmarshal(ctx, &manifest)
	if err != nil {
		log.L(ctx).Errorf("Invalid manifest: %s", err)
		return nil
	}
	switch manifest.Version {
	case core.ManifestVersionUnset:
		return ag.migrateManifest(ctx, batch)
	case core.ManifestVersion1:
		return &manifest
	default:
		log.L(ctx).Errorf("Invalid manifest version: %d", manifest.Version)
		return nil
	}
}

func (ag *aggregator) getBatchCacheKey(id *fftypes.UUID, hash *fftypes.Bytes32) string {
	return fmt.Sprintf("%s/%s", id, hash)
}

func (ag *aggregator) GetBatchForPin(ctx context.Context, pin *core.Pin) (*core.BatchPersisted, *core.BatchManifest, error) {
	cacheKey := ag.getBatchCacheKey(pin.Batch, pin.BatchHash)
	cachedValue := ag.batchCache.Get(cacheKey)
	if cachedValue != nil {
		bce := cachedValue.(*batchCacheEntry)
		log.L(ag.ctx).Debugf("Batch cache hit %s", cacheKey)
		return bce.batch, bce.manifest, nil
	}
	batch, err := ag.database.GetBatchByID(ctx, ag.namespace, pin.Batch)
	if err != nil {
		return nil, nil, err
	}
	if batch == nil {
		return nil, nil, nil
	}
	if !batch.Hash.Equals(pin.BatchHash) {
		log.L(ctx).Errorf("Batch %s hash does not match the pin. OffChain=%s OnChain=%s", pin.Batch, batch.Hash, pin.Hash)
		return nil, nil, nil
	}
	manifest := ag.extractManifest(ctx, batch)
	if manifest == nil {
		log.L(ctx).Errorf("Batch %s manifest could not be extracted - pin %s is parked", pin.Batch, pin.Hash)
		return nil, nil, nil
	}
	ag.cacheBatch(cacheKey, batch, manifest)
	return batch, manifest, nil
}

func (ag *aggregator) cacheBatch(cacheKey string, batch *core.BatchPersisted, manifest *core.BatchManifest) {
	bce := &batchCacheEntry{
		batch:    batch,
		manifest: manifest,
	}
	ag.batchCache.Set(cacheKey, bce)
	log.L(ag.ctx).Debugf("Cached batch %s", cacheKey)
}

func (ag *aggregator) processPins(ctx context.Context, pins []*core.Pin, state *batchState) (err error) {
	l := log.L(ctx)

	localCache := make(map[fftypes.UUID]*batchCacheEntry)
	var manifest *core.BatchManifest
	var batch *core.BatchPersisted

	// As messages can have multiple topics, we need to avoid processing the message twice in the same poll loop.
	// We must check all the contexts in the message, and mark them dispatched together.
	dupMsgCheck := make(map[fftypes.UUID]bool)
	for _, pin := range pins {
		found, ok := localCache[*pin.Batch] // avoid trying to fetch the same batch repeatedly (mainly for cache misses)
		if ok {
			manifest = found.manifest
			batch = found.batch
		} else {
			batch, manifest, err = ag.GetBatchForPin(ctx, pin)
			if err != nil {
				return err
			}
			localCache[*pin.Batch] = &batchCacheEntry{manifest: manifest, batch: batch}
		}
		if manifest == nil {
			l.Debugf("Pin %.10d batch unavailable: batch=%s pinIndex=%d hash=%s masked=%t", pin.Sequence, pin.Batch, pin.Index, pin.Hash, pin.Masked)
			continue
		}

		// Extract the message from the batch - where the index is of a topic within a message
		batchPinCount, msgEntry, msgBaseIndex := ag.extractBatchMessagePin(manifest, pin.Index)
		if msgEntry == nil {
			l.Errorf("Pin %.10d outside of range: batch=%s pinCount=%d pinIndex=%d hash=%s masked=%t", pin.Sequence, pin.Batch, batchPinCount, pin.Index, pin.Hash, pin.Masked)
			continue
		}

		l.Debugf("Aggregating pin %.10d batch=%s msg=%s pinIndex=%d msgBaseIndex=%d hash=%s masked=%t", pin.Sequence, pin.Batch, msgEntry.ID, pin.Index, msgBaseIndex, pin.Hash, pin.Masked)
		if dupMsgCheck[*msgEntry.ID] {
			continue
		}
		dupMsgCheck[*msgEntry.ID] = true

		// Attempt to process the message (only returns errors for database persistence issues)
		err := ag.processMessage(ctx, manifest, pin, msgBaseIndex, msgEntry, batch, state)
		if err != nil {
			return err
		}
	}

	ag.eventPoller.commitOffset(pins[len(pins)-1].Sequence)
	return nil
}

func (ag *aggregator) checkOnchainConsistency(ctx context.Context, msg *core.Message, pin *core.Pin) (action core.MessageAction, err error) {
	l := log.L(ctx)

	verifierRef := &core.VerifierRef{
		Type:  ag.verifierType,
		Value: pin.Signer,
	}

	if msg.Header.Key == "" || msg.Header.Key != pin.Signer {
		return core.ActionReject, i18n.NewError(ctx, coremsgs.MsgInvalidMessageSigner, msg.Header.ID, msg.Header.Key, pin.Signer)
	}

	// Verify that we can resolve the signing key back to the identity that is claimed in the batch
	resolvedAuthor, err := ag.identity.FindIdentityForVerifier(ctx, []core.IdentityType{
		core.IdentityTypeOrg,
		core.IdentityTypeCustom,
	}, verifierRef)
	if err != nil {
		return core.ActionRetry, err
	}

	if resolvedAuthor == nil {
		switch {
		case msg.Header.Type == core.MessageTypeDefinition &&
			(msg.Header.Tag == core.SystemTagIdentityClaim ||
				msg.Header.Tag == core.DeprecatedSystemTagDefineNode ||
				msg.Header.Tag == core.DeprecatedSystemTagDefineOrganization):
			// Identity claims can have an unregistered identity at this point
			// We defer detailed checking of the identity to the system handler
			return core.ActionConfirm, nil

		case msg.Header.Type == core.MessageTypePrivate || msg.Header.Type == core.MessageTypeGroupInit:
			// Private messages (and their associated group init) can always use an unregistered verifier
			return core.ActionConfirm, nil

		default:
			// Everything else (broadcasts, non-identity definitions) will not be processed from an unregistered identity
			l.Warnf("Skipping message '%s'. Author '%s' could not be resolved: %s", msg.Header.ID, msg.Header.Author, err)
			return core.ActionWait, nil // Wait in case the identity is resolved later
		}
	}
	if msg.Header.Author == "" || resolvedAuthor.DID != msg.Header.Author {
		return core.ActionReject, i18n.NewError(ctx, coremsgs.MsgInvalidMessageIdentity, msg.Header.ID, msg.Header.Author, verifierRef.Value, resolvedAuthor.DID, resolvedAuthor.ID)
	}
	return core.ActionConfirm, nil
}

func (ag *aggregator) processMessage(ctx context.Context, manifest *core.BatchManifest, pin *core.Pin, msgBaseIndex int64, msgEntry *core.MessageManifestEntry, batch *core.BatchPersisted, state *batchState) (err error) {
	l := log.L(ctx)

	unmaskedContexts := make([]*fftypes.Bytes32, 0)
	nextPins := make([]*nextPinState, 0)
	action := core.ActionWait
	var correlator *fftypes.UUID

	var cro data.CacheReadOption
	if pin.Masked {
		cro = data.CRORequirePins
	} else {
		cro = data.CRORequirePublicBlobRefs
	}
	msg, data, dataAvailable, err := ag.data.GetMessageWithDataCached(ctx, msgEntry.ID, cro)
	switch {
	case err != nil:
		return err
	case msg == nil:
		l.Debugf("Message '%s' in batch '%s' is not yet available", msgEntry.ID, manifest.ID)
	case !dataAvailable:
		l.Errorf("Message '%s' in batch '%s' is missing data", msgEntry.ID, manifest.ID)
	default:
		// Check the pin signer is valid for the message
		action, err = ag.checkOnchainConsistency(ctx, msg, pin)
		if action == core.ActionWait || action == core.ActionRetry {
			break
		}

		// Check if it's ready to be processed
		if pin.Masked {
			// Private messages have one or more masked "pin" hashes that allow us to work
			// out if it's the next message in the sequence, given the previous messages
			if msg.Header.Group == nil || len(msg.Pins) == 0 || len(msg.Header.Topics) != len(msg.Pins) {
				l.Errorf("Message '%s' in batch '%s' has invalid pin data pins=%v topics=%v", msg.Header.ID, manifest.ID, msg.Pins, msg.Header.Topics)
				return nil
			}
			for i, pinStr := range msg.Pins {
				var msgContext fftypes.Bytes32
				pinSplit := strings.Split(pinStr, ":")
				nonceStr := ""
				if len(pinSplit) > 1 {
					// We introduced a "HASH:NONCE" syntax into the pin strings, to aid debug, but the inclusion of the
					// nonce after the hash is not necessary.
					nonceStr = pinSplit[1]
				}
				err := msgContext.UnmarshalText([]byte(pinSplit[0]))
				if err != nil {
					l.Errorf("Message '%s' in batch '%s' has invalid pin at index %d: '%s'", msg.Header.ID, manifest.ID, i, pinStr)
					return nil
				}
				nextPin, err := state.checkMaskedContextReady(ctx, msg, batch, msg.Header.Topics[i], pin.Sequence, &msgContext, nonceStr)
				if err != nil || nextPin == nil {
					return err
				}
				nextPins = append(nextPins, nextPin)
			}
		} else {
			for _, topic := range msg.Header.Topics {
				msgContext := broadcastContext(topic)
				unmaskedContexts = append(unmaskedContexts, msgContext)
				ready, err := state.checkUnmaskedContextReady(ctx, msgContext, msg, pin.Sequence)
				if err != nil || !ready {
					return err
				}
			}
		}

		if action == core.ActionConfirm {
			l.Debugf("Attempt dispatch msg=%s broadcastContexts=%v privatePins=%v", msg.Header.ID, unmaskedContexts, msg.Pins)
			action, correlator, err = ag.readyForDispatch(ctx, msg, data, manifest.TX.ID, state)
		}
	}

	if action == core.ActionRetry {
		return err
	} else if action == core.ActionWait {
		// We need to prevent dispatch of any subsequent messages on the same topic in the batch
		for _, unmaskedContext := range unmaskedContexts {
			state.SetContextBlockedBy(ctx, *unmaskedContext, pin.Sequence)
		}
		return nil
	}

	if action == core.ActionReject && err != nil {
		log.L(ctx).Warnf("Message '%s' rejected: %s", msg.Header.ID, err)
		msg.RejectReason = err.Error()
	}

	newState := ag.completeDispatch(action, correlator, msg, manifest.TX.ID, state)

	// Mark all message pins dispatched, and increment all nextPins
	for _, np := range nextPins {
		np.IncrementNextPin(ctx, ag.namespace)
	}
	state.markMessageDispatched(manifest.ID, msg, msgBaseIndex, newState)

	// For gap fill messages, mark the original message cancelled
	// This is only applicable if the original message was already received
	// (only for private messages where batch content was delivered via data exchange)
	if msg.Header.Tag == core.SystemTagGapFill {
		state.markMessageDispatched(manifest.ID, &core.Message{
			Header: core.MessageHeader{ID: msg.Header.CID},
		}, 0, core.MessageStateCancelled)
	}
	return nil
}

func needsTokenTransfer(msg *core.Message) bool {
	return (msg.Header.TxParent != nil && msg.Header.TxParent.Type == core.TransactionTypeTokenTransfer) ||
		msg.Header.Type == core.MessageTypeDeprecatedTransferBroadcast ||
		msg.Header.Type == core.MessageTypeDeprecatedTransferPrivate
}

func needsTokenApproval(msg *core.Message) bool {
	return (msg.Header.TxParent != nil && msg.Header.TxParent.Type == core.TransactionTypeTokenApproval) ||
		msg.Header.Type == core.MessageTypeDeprecatedApprovalBroadcast ||
		msg.Header.Type == core.MessageTypeDeprecatedApprovalPrivate
}

func (ag *aggregator) readyForDispatch(ctx context.Context, msg *core.Message, data core.DataArray, tx *fftypes.UUID, state *batchState) (action core.MessageAction, correlator *fftypes.UUID, err error) {
	// Verify we have all the blobs for the data
	if resolved, err := ag.resolveBlobs(ctx, data); err != nil {
		return core.ActionRetry, nil, err
	} else if !resolved {
		return core.ActionWait, nil, nil
	}

	// For transfers, verify the transfer has come through
	if needsTokenTransfer(msg) {
		fb := database.TokenTransferQueryFactory.NewFilter(ctx)
		filter := fb.And(
			fb.Eq("message", msg.Header.ID),
		)
		transfers, _, err := ag.database.GetTokenTransfers(ctx, ag.namespace, filter)
		if err != nil {
			return core.ActionRetry, nil, err
		}
		if len(transfers) == 0 {
			log.L(ctx).Debugf("Transfer for message %s not yet available", msg.Header.ID)
			return core.ActionWait, nil, nil
		}
		if !msg.Hash.Equals(transfers[0].MessageHash) {
			log.L(ctx).Errorf("Message hash %s does not match hash recorded in transfer: %s", msg.Hash, transfers[0].MessageHash)
			return core.ActionWait, nil, nil
		}
	}

	// For approvals, verify the approval has come through
	if needsTokenApproval(msg) {
		fb := database.TokenApprovalQueryFactory.NewFilter(ctx)
		filter := fb.And(
			fb.Eq("message", msg.Header.ID),
		)
		approvals, _, err := ag.database.GetTokenApprovals(ctx, ag.namespace, filter)
		if err != nil {
			return core.ActionRetry, nil, err
		}
		if len(approvals) == 0 {
			log.L(ctx).Debugf("Approval for message %s not yet available", msg.Header.ID)
			return core.ActionWait, nil, nil
		}
		if !msg.Hash.Equals(approvals[0].MessageHash) {
			log.L(ctx).Errorf("Message hash %s does not match hash recorded in approval: %s", msg.Hash, approvals[0].MessageHash)
			return core.ActionWait, nil, nil
		}
	}

	// Validate the message data
	switch {
	case msg.Header.Type == core.MessageTypeDefinition:
		// We handle definition events in-line on the aggregator, as it would be confusing for apps to be
		// dispatched subsequent events before we have processed the definition events they depend on.
		var handlerResult definitions.HandlerResult
		handlerResult, err = ag.definitions.HandleDefinitionBroadcast(ctx, &state.BatchState, msg, data, tx)
		log.L(ctx).Infof("Result of definition broadcast '%s' [%s]: %s", msg.Header.Tag, msg.Header.ID, handlerResult.Action)
		correlator = handlerResult.CustomCorrelator
		action = handlerResult.Action

	case msg.Header.Type == core.MessageTypeGroupInit:
		// Already handled as part of resolving the context
		action = core.ActionConfirm

	case len(msg.Data) > 0:
		var valid bool
		valid, err = ag.data.ValidateAll(ctx, data)
		action = core.ActionReject
		if err != nil {
			action = core.ActionRetry
		} else if valid {
			action = core.ActionConfirm
		}

	default:
		action = core.ActionConfirm
	}

	return action, correlator, err
}

func (ag *aggregator) completeDispatch(action core.MessageAction, correlator *fftypes.UUID, msg *core.Message, tx *fftypes.UUID, state *batchState) core.MessageState {
	newState := core.MessageStateConfirmed
	eventType := core.EventTypeMessageConfirmed
	if action == core.ActionConfirm {
		state.AddPendingConfirm(msg.Header.ID, msg)
	} else {
		newState = core.MessageStateRejected
		eventType = core.EventTypeMessageRejected
	}

	state.AddFinalize(func(ctx context.Context) error {
		// Generate the appropriate event - one per topic (events cover a single topic)
		for _, topic := range msg.Header.Topics {
			event := core.NewEvent(eventType, ag.namespace, msg.Header.ID, tx, topic)
			event.Correlator = msg.Header.CID
			if correlator != nil {
				// Definition handlers can set a custom event correlator (such as a token pool ID)
				event.Correlator = correlator
			}
			if err := ag.database.InsertEvent(ctx, event); err != nil {
				return err
			}
		}
		return nil
	})
	if ag.metrics.IsMetricsEnabled() {
		ag.metrics.MessageConfirmed(msg, eventType)
	}
	return newState
}

// resolveBlobs ensures that the blobs for all the attachments in the data array, have been received into the
// local data exchange blob store. Either because of a private transfer, or by downloading them from the shared storage
func (ag *aggregator) resolveBlobs(ctx context.Context, data core.DataArray) (resolved bool, err error) {
	l := log.L(ctx)

	for _, d := range data {
		if d.Blob == nil || d.Blob.Hash == nil {
			continue
		}

		// See if we already have the data
		fb := database.BlobQueryFactory.NewFilter(ctx)
		blobs, _, err := ag.database.GetBlobs(ctx, ag.namespace, fb.And(fb.Eq("data_id", d.ID), fb.Eq("hash", d.Blob.Hash)))
		if err != nil {
			return false, err
		}
		if len(blobs) > 0 && blobs[0] != nil {
			l.Debugf("Blob '%s' found in local DX with ref '%s'", blobs[0].Hash, blobs[0].PayloadRef)
			continue
		}

		// If we've reached here, the data isn't available yet.
		// This isn't an error, we just need to wait for it to arrive.
		l.Debugf("Blob '%s' not available for data %s", d.Blob.Hash, d.ID)
		return false, nil

	}

	return true, nil

}
