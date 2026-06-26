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

package shareddownload

import (
	"context"
	"database/sql/driver"
	"math"
	"sync/atomic"
	"time"

	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly/internal/coreconfig"
	"github.com/hyperledger/firefly/internal/coremsgs"
	"github.com/hyperledger/firefly/internal/operations"
	"github.com/hyperledger/firefly/pkg/core"
	"github.com/hyperledger/firefly/pkg/database"
	"github.com/hyperledger/firefly/pkg/dataexchange"
	"github.com/hyperledger/firefly/pkg/sharedstorage"
)

type Manager interface {
	Start() error
	WaitStop()

	InitiateDownloadBatch(ctx context.Context, tx *fftypes.UUID, payloadRef string, idempotentSubmit bool) error
	InitiateDownloadBlob(ctx context.Context, tx *fftypes.UUID, dataID *fftypes.UUID, payloadRef string, idempotentSubmit bool) error
}

// downloadManager operates a number of workers that can perform downloads/retries. Each download
// will stay in pending state until a number of retries has been executed against, but each retry
// will be dispatched individually to the workers. So a retrying downloads do not block new
// downloads from getting a chance to use the workers.
// Pending download operations are recovered on startup, and start a new retry loop.
type downloadManager struct {
	ctx                        context.Context
	cancelFunc                 func()
	namespace                  *core.Namespace
	database                   database.Plugin
	sharedstorage              sharedstorage.Plugin // optional
	dataexchange               dataexchange.Plugin
	operations                 operations.Manager
	callbacks                  Callbacks
	workerCount                int
	workers                    []*downloadWorker
	work                       chan *downloadWork
	recoveryComplete           chan struct{}
	broadcastBatchPayloadLimit int64
	retryMaxAttempts           int
	retryInitDelay             time.Duration
	retryMaxDelay              time.Duration
	retryFactor                float64
}

type downloadWork struct {
	dispatchedAt     time.Time
	preparedOp       *core.PreparedOperation
	attempts         atomic.Int32
	idempotentSubmit bool
}

type Callbacks interface {
	SharedStorageBatchDownloaded(payloadRef string, data []byte) (batchID *fftypes.UUID, err error)
	SharedStorageBlobDownloaded(hash fftypes.Bytes32, size int64, payloadRef string, dataID *fftypes.UUID) error
}

func NewDownloadManager(ctx context.Context, ns *core.Namespace, di database.Plugin, ss sharedstorage.Plugin, dx dataexchange.Plugin, om operations.Manager, cb Callbacks) (Manager, error) {
	if di == nil || dx == nil || ss == nil || cb == nil {
		return nil, i18n.NewError(ctx, coremsgs.MsgInitializationNilDepError, "DownloadManager")
	}

	dmCtx, cancelFunc := context.WithCancel(ctx)
	dm := &downloadManager{
		ctx:                        dmCtx,
		cancelFunc:                 cancelFunc,
		namespace:                  ns,
		database:                   di,
		sharedstorage:              ss,
		dataexchange:               dx,
		operations:                 om,
		callbacks:                  cb,
		broadcastBatchPayloadLimit: config.GetByteSize(coreconfig.BroadcastBatchPayloadLimit),
		workerCount:                config.GetInt(coreconfig.DownloadWorkerCount),
		retryMaxAttempts:           config.GetInt(coreconfig.DownloadRetryMaxAttempts),
		retryInitDelay:             config.GetDuration(coreconfig.DownloadRetryInitDelay),
		retryMaxDelay:              config.GetDuration(coreconfig.DownloadRetryMaxDelay),
		retryFactor:                config.GetFloat64(coreconfig.DownloadRetryFactor),
	}
	// Work queue is twice the size of the worker count
	workQueueLength := config.GetInt(coreconfig.DownloadWorkerQueueLength)
	if workQueueLength <= 0 {
		workQueueLength = 2 * dm.workerCount
	}
	if dm.retryMaxAttempts <= 0 {
		dm.retryMaxAttempts = 1
	}
	dm.work = make(chan *downloadWork, workQueueLength)

	dm.operations.RegisterHandler(ctx, dm, []core.OpType{
		core.OpTypeSharedStorageDownloadBatch,
		core.OpTypeSharedStorageDownloadBlob,
	})

	return dm, nil
}

func (dm *downloadManager) Start() error {
	dm.workers = make([]*downloadWorker, dm.workerCount)
	for i := 0; i < dm.workerCount; i++ {
		dm.workers[i] = newDownloadWorker(dm, i)
	}
	dm.recoveryComplete = make(chan struct{})
	go dm.recoverDownloads(fftypes.Now())
	return nil
}

func (dm *downloadManager) Name() string {
	return "SharedStorageDownloadManager"
}

func (dm *downloadManager) WaitStop() {
	dm.cancelFunc()
	for _, w := range dm.workers {
		<-w.done
	}
}

func (dm *downloadManager) calcDelay(attempts int) time.Duration {
	delay := dm.retryInitDelay
	for i := 0; i < attempts; i++ {
		delay = time.Duration(math.Ceil(float64(delay) * dm.retryFactor))
	}
	if delay > dm.retryMaxDelay {
		delay = dm.retryMaxDelay
	}
	return delay
}

// recoverDownloads grabs all pending operations on startup, to restart them
func (dm *downloadManager) recoverDownloads(startupTime *fftypes.FFTime) {

	defer close(dm.recoveryComplete)
	recovered := 0
	pageSize := uint64(25)
	page := uint64(0)
	errorAttempts := 0
	for {
		fb := database.OperationQueryFactory.NewFilter(dm.ctx)
		filter := fb.And(
			fb.In("type", []driver.Value{
				core.OpTypeSharedStorageDownloadBatch,
				core.OpTypeSharedStorageDownloadBlob,
			}),
			fb.Or(
				fb.Eq("status", core.OpStatusPending),
				fb.Eq("status", core.OpStatusInitialized),
			),
			fb.Lt("created", startupTime), // retry is handled completely separately
		).
			Sort("created").
			Skip(page * pageSize).
			Limit(pageSize)
		pendingOps, _, err := dm.database.GetOperations(dm.ctx, dm.namespace.Name, filter)
		if err != nil {
			log.L(dm.ctx).Errorf("Error while recovering pending downloads (retries=%d): %s", errorAttempts, err)
			errorAttempts++
			time.Sleep(dm.calcDelay(errorAttempts))
			continue
		}
		errorAttempts = 0 // reset on success
		page++
		if len(pendingOps) == 0 {
			log.L(dm.ctx).Infof("Download manager completed startup after recovering %d pending downloads", recovered)
			return
		}
		for _, op := range pendingOps {
			preparedOp, err := dm.PrepareOperation(dm.ctx, op)
			if err != nil {
				log.L(dm.ctx).Errorf("Failed to recover pending download %s/%s: %s", op.Type, op.ID, err)
				continue
			}
			recovered++
			log.L(dm.ctx).Infof("Recovering pending download %s/%s", op.Type, op.ID)
			dm.dispatchWork(&downloadWork{
				dispatchedAt: time.Now(),
				preparedOp:   preparedOp,
			})
		}
	}

}

func (dm *downloadManager) dispatchWork(work *downloadWork) {
	dm.work <- work
	// Log after dispatching so we can see the dispatch delay if the queue got full
	log.L(dm.ctx).Debugf("Dispatched download operation %s/%s (attempts=%d) to worker pool", work.preparedOp.Type, work.preparedOp.ID, work.attempts.Load())
}

// waitAndRetryDownload is a go routine to wait and re-dispatch a retrying download.
// Note this go routine is short lived and completely separate to the workers.
func (dm *downloadManager) waitAndRetryDownload(work *downloadWork) {
	startedWaiting := time.Now()
	attempts := int(work.attempts.Load())
	delay := dm.calcDelay(attempts)
	<-time.After(delay)
	delayTimeMS := time.Since(startedWaiting).Milliseconds()
	totalTimeMS := time.Since(work.dispatchedAt).Milliseconds()
	log.L(dm.ctx).Infof("Retrying download operation %s/%s after %dms (total=%dms,attempts=%d)",
		work.preparedOp.Type, work.preparedOp.ID, delayTimeMS, totalTimeMS, attempts)
	dm.dispatchWork(work)
}

func (dm *downloadManager) InitiateDownloadBatch(ctx context.Context, tx *fftypes.UUID, payloadRef string, idempotentSubmit bool) error {
	op := core.NewOperation(dm.sharedstorage, dm.namespace.Name, tx, core.OpTypeSharedStorageDownloadBatch)
	addDownloadBatchInputs(op, payloadRef)
	return dm.createAndDispatchOp(ctx, op, opDownloadBatch(op, payloadRef), idempotentSubmit)
}

func (dm *downloadManager) InitiateDownloadBlob(ctx context.Context, tx *fftypes.UUID, dataID *fftypes.UUID, payloadRef string, idempotentSubmit bool) error {
	op := core.NewOperation(dm.sharedstorage, dm.namespace.Name, tx, core.OpTypeSharedStorageDownloadBlob)
	addDownloadBlobInputs(op, dataID, payloadRef)
	return dm.createAndDispatchOp(ctx, op, opDownloadBlob(op, dataID, payloadRef), idempotentSubmit)
}

func (dm *downloadManager) createAndDispatchOp(ctx context.Context, op *core.Operation, preparedOp *core.PreparedOperation, idempotentSubmit bool) error {
	err := dm.operations.AddOrReuseOperation(ctx, op, func() {
		// Use a closure hook to dispatch the work once the operation is successfully in the DB.
		// Note we have crash recovery of pending operations on startup.
		dm.dispatchWork(&downloadWork{
			dispatchedAt:     time.Now(),
			preparedOp:       preparedOp,
			idempotentSubmit: idempotentSubmit,
		})
	})
	if err != nil {
		return err
	}
	return nil
}
