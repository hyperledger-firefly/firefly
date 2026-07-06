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

package operations

import (
	"context"
	"fmt"
	"time"

	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-common/pkg/ffapi"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly-common/pkg/retry"
	"github.com/hyperledger/firefly/internal/coreconfig"
	"github.com/hyperledger/firefly/internal/coremsgs"
	"github.com/hyperledger/firefly/internal/txcommon"
	"github.com/hyperledger/firefly/pkg/core"
	"github.com/hyperledger/firefly/pkg/database"
	"github.com/hyperledger/firefly/pkg/utils"
)

type operationUpdaterBatch struct {
	updates        []*core.OperationUpdateAsync
	timeoutContext context.Context
	timeoutCancel  func()
}

// operationUpdater
type operationUpdater struct {
	ctx         context.Context
	cancelFunc  func()
	manager     *operationsManager
	database    database.Plugin
	txHelper    txcommon.Helper
	workQueues  []chan *core.OperationUpdateAsync
	workersDone []chan struct{}
	conf        operationUpdaterConf
	closed      bool
	retry       *retry.Retry
}

type operationUpdaterConf struct {
	workerCount  int
	batchTimeout time.Duration
	maxInserts   int
	queueLength  int
}

func newOperationUpdater(ctx context.Context, om *operationsManager, di database.Plugin, txHelper txcommon.Helper) *operationUpdater {
	ou := &operationUpdater{
		manager:  om,
		database: di,
		txHelper: txHelper,
		conf: operationUpdaterConf{
			workerCount:  config.GetInt(coreconfig.OpUpdateWorkerCount),
			batchTimeout: config.GetDuration(coreconfig.OpUpdateWorkerBatchTimeout),
			maxInserts:   config.GetInt(coreconfig.OpUpdateWorkerBatchMaxInserts),
			queueLength:  config.GetInt(coreconfig.OpUpdateWorkerQueueLength),
		},
		retry: &retry.Retry{
			InitialDelay: config.GetDuration(coreconfig.OpUpdateRetryInitDelay),
			MaximumDelay: config.GetDuration(coreconfig.OpUpdateRetryMaxDelay),
			Factor:       config.GetFloat64(coreconfig.OpUpdateRetryFactor),
		},
	}
	ou.ctx, ou.cancelFunc = context.WithCancel(ctx)
	if !di.Capabilities().Concurrency {
		log.L(ctx).Infof("Database plugin not configured for concurrency. Batched operation updates disabled")
		ou.conf.workerCount = 0
	}
	return ou
}

// pickWorker ensures multiple updates for the same ID go to the same worker
func (ou *operationUpdater) pickWorker(ctx context.Context, id *fftypes.UUID, update *core.OperationUpdateAsync) chan *core.OperationUpdateAsync {
	worker := id.HashBucket(ou.conf.workerCount)
	log.L(ctx).Debugf("Submitting operation update id=%s status=%s blockchainTX=%s worker=opu_%.3d", id, update.Status, update.BlockchainTXID, worker)
	return ou.workQueues[worker]
}

// SubmitBulkOperationUpdates is a synchronous write of batch of operation updates
func (ou *operationUpdater) SubmitBulkOperationUpdates(ctx context.Context, updates []*core.OperationUpdate) error {
	validUpdates := []*core.OperationUpdate{}
	for _, update := range updates {
		ns, _, err := core.ParseNamespacedOpID(ctx, update.NamespacedOpID)
		if err != nil {
			log.L(ctx).Warnf("Unable to update operation '%s' due to invalid ID: %s", update.NamespacedOpID, err)
			return err
		}

		if ns != ou.manager.namespace {
			log.L(ou.ctx).Errorf("Received operation update from different namespace '%s'", ns)
			return i18n.NewError(ctx, coremsgs.MsgInvalidNamespaceForOperationUpdate, ns, ou.manager.namespace)
		}

		if update.Plugin == "" {
			log.L(ou.ctx).Errorf("Cannot supply empty plugin on operation update '%s'", update.NamespacedOpID)
			return i18n.NewError(ctx, coremsgs.MsgEmptyPluginForOperationUpdate, update.NamespacedOpID)
		}

		validUpdates = append(validUpdates, update)
	}

	// Notice how this is not using the workers and is synchronous
	// The reason is because we want for all updates to be stored at once in this order
	// If offloaded into workers the updates would be processed in parallel, in different DB TX and in a different order
	// Up to the caller to retry if this fails
	err := ou.doBatchUpdateAsGroup(ctx, validUpdates)
	if err != nil {
		log.L(ctx).Warnf("Exiting while updating operations: %s", err)
		return err
	}

	return nil
}

func (ou *operationUpdater) SubmitOperationUpdate(ctx context.Context, update *core.OperationUpdateAsync) {
	ns, id, err := core.ParseNamespacedOpID(ctx, update.NamespacedOpID)
	if err != nil {
		log.L(ctx).Warnf("Unable to update operation '%s' due to invalid ID: %s", update.NamespacedOpID, err)
		return
	}
	if ns != ou.manager.namespace {
		log.L(ou.ctx).Debugf("Ignoring operation update from different namespace '%s'", ns)
		return
	}

	if ou.conf.workerCount > 0 {
		if update.Status == core.OpStatusFailed {
			// We do a cache update pre-emptively, as for idempotency checking on an error status we want to
			// see the update immediately - even though it's being asynchronously flushed to the storage
			ou.manager.updateCachedOperation(id, update.Status, &update.ErrorMessage, update.Output, nil)
		}

		select {
		case ou.pickWorker(ctx, id, update) <- update:
		case <-ou.ctx.Done():
			log.L(ctx).Debugf("Not submitting operation update due to cancelled context")
		}
		return
	}
	// Otherwise do it in-line on this context
	err = ou.doBatchUpdateWithRetry(ctx, []*core.OperationUpdateAsync{update})
	if err != nil {
		log.L(ctx).Warnf("Exiting while updating operation: %s", err)
	}
}

func (ou *operationUpdater) initQueues() {
	ou.workQueues = make([]chan *core.OperationUpdateAsync, ou.conf.workerCount)
	ou.workersDone = make([]chan struct{}, ou.conf.workerCount)
	for i := 0; i < ou.conf.workerCount; i++ {
		ou.workQueues[i] = make(chan *core.OperationUpdateAsync, ou.conf.queueLength)
		ou.workersDone[i] = make(chan struct{})
	}
}

func (ou *operationUpdater) start() {
	if ou.conf.workerCount > 0 {
		ou.initQueues()
		for i := 0; i < ou.conf.workerCount; i++ {
			go ou.updaterLoop(i)
		}
	}
}

func (ou *operationUpdater) updaterLoop(index int) {
	defer close(ou.workersDone[index])
	workQueue := ou.workQueues[index]

	ctx := log.WithLogField(ou.ctx, "opupdater", fmt.Sprintf("opu_%.3d", index))

	var batch *operationUpdaterBatch
	for !ou.closed {
		var timeoutContext context.Context
		var timedOut bool
		if batch != nil {
			timeoutContext = batch.timeoutContext
		} else {
			timeoutContext = ctx
		}
		select {
		case work := <-workQueue:
			if batch == nil {
				batch = &operationUpdaterBatch{}
				batch.timeoutContext, batch.timeoutCancel = context.WithTimeout(ctx, ou.conf.batchTimeout)
			}
			batch.updates = append(batch.updates, work)
		case <-timeoutContext.Done():
			timedOut = true
		}

		if batch != nil && (timedOut || len(batch.updates) >= ou.conf.maxInserts) {
			batch.timeoutCancel()
			err := ou.doBatchUpdateWithRetry(ctx, batch.updates)
			if err != nil {
				log.L(ctx).Debugf("Operation update worker exiting: %s", err)
				return
			}
			batch = nil
		}
	}
}

func (ou *operationUpdater) doBatchUpdateAsGroup(ctx context.Context, updates []*core.OperationUpdate) error {
	return ou.database.RunAsGroup(ctx, func(ctx context.Context) error {
		return ou.doBatchUpdate(ctx, updates)
	})
}

func (ou *operationUpdater) doBatchUpdateWithRetry(ctx context.Context, updates []*core.OperationUpdateAsync) error {
	return ou.retry.Do(ctx, "operation update", func(attempt int) (retry bool, err error) {
		syncUpdates := []*core.OperationUpdate{}
		for _, update := range updates {
			syncUpdates = append(syncUpdates, &update.OperationUpdate)
		}
		err = ou.doBatchUpdateAsGroup(ctx, syncUpdates)
		if err != nil {
			return true, err
		}

		for _, update := range updates {
			if update.OnComplete != nil {
				update.OnComplete()
			}
		}
		return false, nil
	})
}

func (ou *operationUpdater) doBatchUpdate(ctx context.Context, updates []*core.OperationUpdate) error {
	// Get all the operations that match
	opIDs := make([]*fftypes.UUID, 0, len(updates))
	for _, update := range updates {
		_, id, err := core.ParseNamespacedOpID(ctx, update.NamespacedOpID)
		if err != nil {
			log.L(ctx).Warnf("Unable to update operation '%s' due to invalid ID: %s", update.NamespacedOpID, err)
			continue
		}
		opIDs = append(opIDs, id)
	}
	if len(opIDs) == 0 {
		return nil
	}
	ops, err := ou.manager.getOperationsCached(ctx, opIDs)
	if err != nil {
		return err
	}

	// Get all the transactions for these operations
	var transactions []*core.Transaction
	for _, op := range ops {
		if op.Transaction != nil {
			transaction, err := ou.txHelper.GetTransactionByIDCached(ctx, op.Transaction)
			if err != nil {
				return err
			}
			if transaction != nil {
				transactions = append(transactions, transaction)
			}
		}
	}

	// Spin through each update seeing what DB updates we need to do
	for _, update := range updates {
		if err := ou.doUpdate(ctx, update, ops, transactions); err != nil {
			return err
		}
	}

	return nil
}

func (ou *operationUpdater) doUpdate(ctx context.Context, update *core.OperationUpdate, ops []*core.Operation, transactions []*core.Transaction) error {

	_, updateID, err := core.ParseNamespacedOpID(ctx, update.NamespacedOpID)
	if err != nil {
		log.L(ctx).Warnf("Unable to update operation '%s' due to invalid ID: %s", update.NamespacedOpID, err)
		return nil
	}

	// Find the operation we already retrieved, and do the update
	var op *core.Operation
	for _, candidate := range ops {
		if updateID.Equals(candidate.ID) {
			if update.Plugin != candidate.Plugin {
				log.L(ctx).Debugf("Operation update '%s' from '%s' ignored, as it does not match operation source '%s'", update.NamespacedOpID, update.Plugin, candidate.Plugin)
				return nil
			}
			op = candidate
			break
		}
	}
	if op == nil {
		log.L(ctx).Warnf("Operation update '%s' ignored, as it was not submitted by this node", update.NamespacedOpID)
		return nil
	}

	// Match a TX we already retrieved, if found add a specified Blockchain Transaction ID to it
	var txnIDStr string
	var idempotencyKeyStr string
	var tx *core.Transaction
	if op.Transaction != nil {
		for _, candidate := range transactions {
			if op.Transaction.Equals(candidate.ID) {
				tx = candidate
				txnIDStr = candidate.ID.String()
				idempotencyKeyStr = string(candidate.IdempotencyKey)
				break
			}
		}
	}
	if tx != nil && update.BlockchainTXID != "" {
		if err := ou.txHelper.AddBlockchainTX(ctx, tx, update.BlockchainTXID); err != nil {
			return err
		}
	}

	// This is a key log line, where we can provide all pieces of correlation data a user needs:
	// - The type of the operation
	// - The plugin/connector
	// - The idempotencyKey
	// - The FF Transaction ID
	// - The Operation ID
	log.L(ctx).Infof("FF_OPERATION_UPDATE: namespace=%s plugin=%s type=%s status=%s operationId=%s transactionId=%s idempotencyKey='%s'", op.Namespace, op.Plugin, op.Type, update.Status, op.ID, txnIDStr, idempotencyKeyStr)

	if handler, ok := ou.manager.handlers[op.Type]; ok {
		if err := handler.OnOperationUpdate(ctx, op, update); err != nil {
			return err
		}
	}

	// Special handling for data exchange manifests
	if update.VerifyManifest {
		if err := ou.verifyManifest(ctx, update, op); err != nil {
			return err
		}
	}

	if err := ou.resolveOperation(ctx, op.Namespace, op.ID, update.Status, &update.ErrorMessage, update.Output); err != nil {
		return err
	}

	return nil
}

func (ou *operationUpdater) verifyManifest(ctx context.Context, update *core.OperationUpdate, op *core.Operation) error {

	if op.Type == core.OpTypeDataExchangeSendBatch && update.Status == core.OpStatusSucceeded {
		batchID, _ := fftypes.ParseUUID(ctx, op.Input.GetString("batch"))
		expectedManifest := ""
		if batchID != nil {
			batch, err := ou.database.GetBatchByID(ctx, ou.manager.namespace, batchID)
			if err != nil {
				return err
			}
			if batch != nil {
				expectedManifest = batch.Manifest.String()
			}
		}
		if update.DXManifest != expectedManifest {
			// Log and map to failure for user to see that the receiver did not provide a matching acknowledgement
			mismatchErr := i18n.NewError(ctx, coremsgs.MsgManifestMismatch, core.OpStatusSucceeded, update.DXManifest)
			log.L(ctx).Errorf("DX transfer %s: %s", op.ID, mismatchErr.Error())
			update.ErrorMessage = mismatchErr.Error()
			update.Status = core.OpStatusFailed
		}
	}

	if op.Type == core.OpTypeDataExchangeSendBlob && update.Status == core.OpStatusSucceeded {
		expectedHash := op.Input.GetString("hash")
		if update.DXHash != expectedHash {
			// Log and map to failure for user to see that the receiver did not provide a matching hash
			mismatchErr := i18n.NewError(ctx, coremsgs.MsgBlobHashMismatch, expectedHash, update.DXHash)
			log.L(ctx).Errorf("DX transfer %s: %s", op.ID, mismatchErr.Error())
			update.ErrorMessage = mismatchErr.Error()
			update.Status = core.OpStatusFailed
		}
	}

	return nil
}

func (ou *operationUpdater) close() {
	if !ou.closed {
		ou.closed = true
		ou.cancelFunc()
		for _, workerDone := range ou.workersDone {
			<-workerDone
		}
	}
}

func (ou *operationUpdater) resolveOperation(ctx context.Context, ns string, id *fftypes.UUID, status core.OpStatus, errorMsg *string, output fftypes.JSONObject) (err error) {
	// Never move an operation from Succeeded/Failed back to Pending
	fb := database.OperationQueryFactory.NewFilter(ctx)
	var filter ffapi.AndFilter
	if status == core.OpStatusPending {
		filter = fb.And(
			fb.Neq("status", core.OpStatusSucceeded),
			fb.Neq("status", core.OpStatusFailed),
		)
	}

	update := database.OperationQueryFactory.NewUpdate(ctx).S()
	if status != "" {
		update = update.Set("status", status)
	}
	if errorMsg != nil {
		update = update.Set("error", utils.DBSafeUTF8StringFromPtr(ctx, errorMsg))
	}
	if output != nil {
		update = update.Set("output", output)
	}
	ok, err := ou.database.UpdateOperation(ctx, ns, id, filter, update)
	if ok && err == nil {
		ou.manager.updateCachedOperation(id, status, errorMsg, output, nil)
	}
	return err
}
