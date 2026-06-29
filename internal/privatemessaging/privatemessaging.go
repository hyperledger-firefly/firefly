// Copyright © 2025 Kaleido, Inc.
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

package privatemessaging

import (
	"context"
	"sync"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/common/pkg/retry"
	"github.com/hyperledger-firefly/firefly/internal/batch"
	"github.com/hyperledger-firefly/firefly/internal/cache"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/internal/data"
	"github.com/hyperledger-firefly/firefly/internal/identity"
	"github.com/hyperledger-firefly/firefly/internal/metrics"
	"github.com/hyperledger-firefly/firefly/internal/multiparty"
	"github.com/hyperledger-firefly/firefly/internal/operations"
	"github.com/hyperledger-firefly/firefly/internal/syncasync"
	"github.com/hyperledger-firefly/firefly/pkg/blockchain"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/hyperledger-firefly/firefly/pkg/dataexchange"
)

const pinnedPrivateDispatcherName = "pinned_private"
const unpinnedPrivateDispatcherName = "unpinned_private"

type Manager interface {
	core.Named
	GroupManager

	NewMessage(msg *core.MessageInOut) syncasync.Sender
	SendMessage(ctx context.Context, in *core.MessageInOut, waitConfirm bool) (out *core.Message, err error)
	RequestReply(ctx context.Context, request *core.MessageInOut) (reply *core.MessageInOut, err error)

	// From operations.OperationHandler
	PrepareOperation(ctx context.Context, op *core.Operation) (*core.PreparedOperation, error)
	RunOperation(ctx context.Context, op *core.PreparedOperation) (outputs fftypes.JSONObject, phase core.OpPhase, err error)
}

type privateMessaging struct {
	groupManager

	ctx                   context.Context
	namespace             *core.Namespace
	database              database.Plugin
	identity              identity.Manager
	exchange              dataexchange.Plugin
	blockchain            blockchain.Plugin
	data                  data.Manager
	syncasync             syncasync.Bridge
	multiparty            multiparty.Manager
	retry                 retry.Retry
	maxBatchPayloadLength int64
	metrics               metrics.Manager
	operations            operations.Manager
	orgFirstNodes         map[string]*core.Identity
}

type blobTransferTracker struct {
	dataID   *fftypes.UUID
	blobHash *fftypes.Bytes32
	op       *core.PreparedOperation
}

func NewPrivateMessaging(ctx context.Context, ns *core.Namespace, di database.Plugin, dx dataexchange.Plugin, bi blockchain.Plugin, im identity.Manager, ba batch.Manager, dm data.Manager, sa syncasync.Bridge, mult multiparty.Manager, mm metrics.Manager, om operations.Manager, cacheManager cache.Manager) (Manager, error) {
	if di == nil || im == nil || dx == nil || bi == nil || ba == nil || dm == nil || mm == nil || om == nil || mult == nil {
		return nil, i18n.NewError(ctx, coremsgs.MsgInitializationNilDepError, "PrivateMessaging")
	}

	pm := &privateMessaging{
		ctx:        ctx,
		namespace:  ns,
		database:   di,
		identity:   im,
		exchange:   dx,
		blockchain: bi,
		data:       dm,
		syncasync:  sa,
		multiparty: mult,
		groupManager: groupManager{
			namespace: ns,
			database:  di,
			identity:  im,
			data:      dm,
		},
		retry: retry.Retry{
			InitialDelay: config.GetDuration(coreconfig.PrivateMessagingRetryInitDelay),
			MaximumDelay: config.GetDuration(coreconfig.PrivateMessagingRetryMaxDelay),
			Factor:       config.GetFloat64(coreconfig.PrivateMessagingRetryFactor),
		},
		maxBatchPayloadLength: config.GetByteSize(coreconfig.PrivateMessagingBatchPayloadLimit),
		metrics:               mm,
		operations:            om,
		orgFirstNodes:         make(map[string]*core.Identity),
	}

	groupCache, err := cacheManager.GetCache(
		cache.NewCacheConfig(
			ctx,
			coreconfig.CacheGroupLimit,
			coreconfig.CacheGroupTTL,
			ns.Name,
		),
	)
	if err != nil {
		return nil, err
	}

	pm.groupManager.groupCache = groupCache

	bo := batch.DispatcherOptions{
		BatchType:      core.BatchTypePrivate,
		BatchMaxSize:   config.GetInt(coreconfig.PrivateMessagingBatchSize),
		BatchMaxBytes:  pm.maxBatchPayloadLength,
		BatchTimeout:   config.GetDuration(coreconfig.PrivateMessagingBatchTimeout),
		DisposeTimeout: config.GetDuration(coreconfig.PrivateMessagingBatchAgentTimeout),
	}

	ba.RegisterDispatcher(pinnedPrivateDispatcherName,
		true,
		[]core.MessageType{
			core.MessageTypeGroupInit,
			core.MessageTypePrivate,
			core.MessageTypeDeprecatedTransferPrivate,
			core.MessageTypeDeprecatedApprovalPrivate,
		},
		pm.dispatchPinnedBatch, bo)

	ba.RegisterDispatcher(unpinnedPrivateDispatcherName,
		false,
		[]core.MessageType{
			core.MessageTypePrivate,
		},
		pm.dispatchUnpinnedBatch, bo)

	om.RegisterHandler(ctx, pm, []core.OpType{
		core.OpTypeDataExchangeSendBlob,
		core.OpTypeDataExchangeSendBatch,
	})

	return pm, nil
}

func (pm *privateMessaging) Name() string {
	return "PrivateMessaging"
}

func (pm *privateMessaging) dispatchPinnedBatch(ctx context.Context, payload *batch.DispatchPayload) error {
	err := pm.dispatchBatchCommon(ctx, payload)
	if err != nil {
		return err
	}

	log.L(ctx).Infof("Pinning private batch %s with author=%s key=%s group=%s", payload.Batch.ID, payload.Batch.Author, payload.Batch.Key, payload.Batch.Group)
	return pm.multiparty.SubmitBatchPin(ctx, &payload.Batch, payload.Pins, "" /* no payloadRef for private */, false /* batch processing does not currently use idempotency keys */)
}

func (pm *privateMessaging) dispatchUnpinnedBatch(ctx context.Context, payload *batch.DispatchPayload) error {
	return pm.dispatchBatchCommon(ctx, payload)
}

// prepareBatchForNetworkTransport is mainly for documentation purposes, we need to "normalize" a batch before sending
// it over the network to other parties, so that all nodes see the same batch, regardless of what they call their local
// namespace. This is used when we dispatch a batch per the regular messaging flow, and when we retry a private messaging
// send operation.
func (pm *privateMessaging) prepareBatchForNetworkTransport(ctx context.Context, tw *core.TransportWrapper) {
	tw.Batch.Namespace = pm.namespace.NetworkName
}

func (pm *privateMessaging) dispatchBatchCommon(ctx context.Context, payload *batch.DispatchPayload) error {
	batch := payload.Batch.GenInflight(payload.Messages, payload.Data)
	tw := &core.TransportWrapper{
		Batch: batch,
	}
	pm.prepareBatchForNetworkTransport(ctx, tw)

	// Retrieve the group
	group, nodes, err := pm.groupManager.getGroupNodes(ctx, batch.Group, false /* fail if not found */)
	if err != nil {
		return err
	}

	if !core.IsPinned(batch.Payload.TX.Type) {
		// In the case of an un-pinned message we cannot be sure the group has been broadcast via the blockchain.
		// So we have to take the hit of sending it along with every message.
		tw.Group = group
	}

	return pm.sendData(ctx, tw, nodes)
}

func (pm *privateMessaging) prepareBlobTransfers(ctx context.Context, data core.DataArray, txid *fftypes.UUID, node *core.Identity) ([]*blobTransferTracker, error) {

	operations := make([]*blobTransferTracker, 0)

	// Build all the operations needed to send the blobs in a single DB transaction
	err := pm.database.RunAsGroup(ctx, func(ctx context.Context) error {
		for _, d := range data {
			if d.Blob != nil {
				if d.Blob.Hash == nil {
					return i18n.NewError(ctx, coremsgs.MsgDataMissingBlobHash, d.ID)
				}
				fb := database.BlobQueryFactory.NewFilter(ctx)
				blobs, _, err := pm.database.GetBlobs(ctx, pm.namespace.Name, fb.And(fb.Eq("data_id", d.ID), fb.Eq("hash", d.Blob.Hash)))
				if err != nil {
					return err
				}
				if len(blobs) == 0 || blobs[0] == nil {
					return i18n.NewError(ctx, coremsgs.MsgBlobNotFound, d.Blob.Hash)
				}
				blob := blobs[0]

				op := core.NewOperation(
					pm.exchange,
					pm.namespace.Name,
					txid,
					core.OpTypeDataExchangeSendBlob)
				addTransferBlobInputs(op, node.ID, blob.Hash, d.ID)
				if err = pm.operations.AddOrReuseOperation(ctx, op); err != nil {
					return err
				}

				operations = append(operations, &blobTransferTracker{
					dataID:   d.ID,
					blobHash: blob.Hash,
					op:       opSendBlob(op, node, blob),
				})
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return operations, err
}

func (pm *privateMessaging) submitBlobTransfersToDX(ctx context.Context, trackers []*blobTransferTracker) error {
	// Initiate all the sends. We use parallel go routines here as these are blocking API calls
	wg := sync.WaitGroup{}
	wg.Add(len(trackers))
	var firstError error
	for _, tracker := range trackers {
		go func(tracker *blobTransferTracker) {
			defer wg.Done()
			log.L(ctx).Debugf("Initiating DX transfer blob=%s data=%s operation=%s", tracker.blobHash, tracker.dataID, tracker.op.ID)
			if _, err := pm.operations.RunOperation(ctx, tracker.op, false /* batch processing does not currently use idempotency keys */); err != nil {
				log.L(ctx).Errorf("Failed to initiate DX transfer blob=%s data=%s operation=%s", tracker.blobHash, tracker.dataID, tracker.op.ID)
				if firstError == nil {
					firstError = err
				}
			}
		}(tracker)
	}
	wg.Wait()
	return firstError
}

func (pm *privateMessaging) sendData(ctx context.Context, tw *core.TransportWrapper, nodes []*core.Identity) (err error) {
	l := log.L(ctx)
	batch := tw.Batch

	// Lookup the local node
	localNode, err := pm.identity.GetLocalNode(ctx)
	if err != nil {
		return err
	}

	// Write it to the dataexchange for each member
	for i, node := range nodes {

		if node.ID.Equals(localNode.ID) {
			l.Debugf("Skipping send of batch for local node %s for group=%s node=%s (%d/%d)", batch.ID, batch.Group, node.ID, i+1, len(nodes))
			continue
		}

		l.Debugf("Sending batch %s to group=%s node=%s (%d/%d)", batch.ID, batch.Group, node.ID, i+1, len(nodes))

		var blobTrackers []*blobTransferTracker
		var sendBatchOp *core.PreparedOperation

		// Use a DB group for preparing all the operations needed for this batch
		err := pm.database.RunAsGroup(ctx, func(ctx context.Context) (err error) {
			blobTrackers, err = pm.prepareBlobTransfers(ctx, batch.Payload.Data, batch.Payload.TX.ID, node)
			if err != nil {
				return err
			}

			op := core.NewOperation(
				pm.exchange,
				pm.namespace.Name,
				batch.Payload.TX.ID,
				core.OpTypeDataExchangeSendBatch)
			addBatchSendInputs(op, node.ID, batch.Group, batch.ID)
			if err = pm.operations.AddOrReuseOperation(ctx, op); err != nil {
				return err
			}
			sendBatchOp = opSendBatch(op, node, tw)
			return nil
		})
		if err != nil {
			return err
		}

		// Initiate transfer of any blobs first
		if len(blobTrackers) > 0 {
			if err = pm.submitBlobTransfersToDX(ctx, blobTrackers); err != nil {
				return err
			}
		}

		// Then initiate the batch transfer
		if _, err = pm.operations.RunOperation(ctx, sendBatchOp, false /* batch processing does not currently use idempotency keys */); err != nil {
			return err
		}
	}

	return nil
}
