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
	"fmt"
	"time"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/common/pkg/retry"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

type blobNotification struct {
	blob       *core.Blob
	onComplete func()
}

type blobReceiverBatch struct {
	notifications  []*blobNotification
	timeoutContext context.Context
	timeoutCancel  func()
}

// blobReceiver
type blobReceiver struct {
	ctx         context.Context
	aggregator  *aggregator
	cancelFunc  func()
	database    database.Plugin
	workQueue   chan *blobNotification
	workersDone []chan struct{}
	conf        blobReceiverConf
	closed      bool
	retry       *retry.Retry
}

type blobReceiverConf struct {
	workerCount  int
	batchTimeout time.Duration
	maxInserts   int
}

func newBlobReceiver(ctx context.Context, ag *aggregator) *blobReceiver {
	br := &blobReceiver{
		aggregator: ag,
		database:   ag.database,
		conf: blobReceiverConf{
			workerCount:  config.GetInt(coreconfig.BlobReceiverWorkerCount),
			batchTimeout: config.GetDuration(coreconfig.BlobReceiverWorkerBatchTimeout),
			maxInserts:   config.GetInt(coreconfig.BlobReceiverWorkerBatchMaxInserts),
		},
		retry: &retry.Retry{
			InitialDelay: config.GetDuration(coreconfig.BlobReceiverRetryInitDelay),
			MaximumDelay: config.GetDuration(coreconfig.BlobReceiverRetryMaxDelay),
			Factor:       config.GetFloat64(coreconfig.BlobReceiverRetryFactor),
		},
	}
	br.ctx, br.cancelFunc = context.WithCancel(ctx)
	if !ag.database.Capabilities().Concurrency {
		log.L(ctx).Infof("Database plugin not configured for concurrency. Batched blob receiver updates disabled")
		br.conf.workerCount = 0
	}
	return br
}

func (br *blobReceiver) blobReceived(ctx context.Context, notification *blobNotification) {
	if br.conf.workerCount > 0 {
		select {
		case br.workQueue <- notification:
			log.L(ctx).Debugf("Dispatched blob notification %s", notification.blob.Hash)
		case <-br.ctx.Done():
			log.L(ctx).Debugf("Not submitting received blob due to cancelled context")
		}
		return
	}
	// Otherwise do it in-line on this context
	err := br.handleBlobNotificationsRetry(ctx, []*blobNotification{notification})
	if err != nil {
		log.L(ctx).Warnf("Exiting while updating operation: %s", err)
	}
}

func (br *blobReceiver) initQueues() {
	br.workQueue = make(chan *blobNotification)
	br.workersDone = make([]chan struct{}, br.conf.workerCount)
	for i := 0; i < br.conf.workerCount; i++ {
		br.workersDone[i] = make(chan struct{})
	}
}

func (br *blobReceiver) start() {
	if br.conf.workerCount > 0 {
		br.initQueues()
		for i := 0; i < br.conf.workerCount; i++ {
			go br.blobReceiverLoop(i)
		}
	}
}

func (br *blobReceiver) stop() {
	br.closed = true
	br.cancelFunc()
	for _, workerDone := range br.workersDone {
		<-workerDone
	}
}

func (br *blobReceiver) blobReceiverLoop(index int) {
	defer close(br.workersDone[index])

	ctx := log.WithLogFields(br.ctx, "blobreceiver", fmt.Sprintf("brcvr_%.3d", index))

	var batch *blobReceiverBatch
	for !br.closed {
		var timeoutContext context.Context
		var timedOut bool
		if batch != nil {
			timeoutContext = batch.timeoutContext
		} else {
			timeoutContext = ctx
		}
		select {
		case work := <-br.workQueue:
			if batch == nil {
				batch = &blobReceiverBatch{}
				batch.timeoutContext, batch.timeoutCancel = context.WithTimeout(ctx, br.conf.batchTimeout)
			}
			batch.notifications = append(batch.notifications, work)
		case <-timeoutContext.Done():
			timedOut = true
		}

		if batch != nil && (timedOut || len(batch.notifications) >= br.conf.maxInserts) {
			batch.timeoutCancel()
			err := br.handleBlobNotificationsRetry(ctx, batch.notifications)
			if err != nil {
				log.L(ctx).Debugf("Blob receiver worker exiting: %s", err)
				return
			}
			batch = nil
		}
	}
}

func (br *blobReceiver) handleBlobNotificationsRetry(ctx context.Context, notifications []*blobNotification) error {
	// We process the event in a retry loop (which will break only if the context is closed), so that
	// we only confirm consumption of the event to the plugin once we've processed it.
	var newHashes []*fftypes.Bytes32
	err := br.retry.Do(ctx, "blob reference insert", func(attempt int) (retry bool, err error) {
		return true, br.database.RunAsGroup(ctx, func(ctx context.Context) (err error) {
			newHashes, err = br.insertNewBlobs(ctx, notifications)
			return err
		})
	})
	// We only get an error here if we're exiting
	if err != nil {
		return err
	}
	// Notify all callbacks we completed
	for _, notification := range notifications {
		if notification.onComplete != nil {
			notification.onComplete()
		}
	}
	// Notify the aggregator for any rewinds
	for _, hash := range newHashes {
		br.aggregator.queueBlobRewind(hash)
	}
	return nil
}

func (br *blobReceiver) insertNewBlobs(ctx context.Context, notifications []*blobNotification) ([]*fftypes.Bytes32, error) {

	allHashes := make([]driver.Value, len(notifications))
	for i, n := range notifications {
		allHashes[i] = n.blob.Hash
	}

	// We want just one record in our DB for each entry in DX, so make the logic idempotent.
	// Note that we do create a record for each separate receipt of data on a new payload ref,
	// even if the hash of that data is the same.
	fb := database.BlobQueryFactory.NewFilter(ctx)
	filter := fb.In("hash", allHashes)
	existingBlobs, _, err := br.database.GetBlobs(ctx, br.aggregator.namespace, filter)
	if err != nil {
		return nil, err
	}
	newBlobs := make([]*core.Blob, 0, len(existingBlobs))
	newHashes := make([]*fftypes.Bytes32, 0, len(existingBlobs))
	for _, notification := range notifications {
		foundExisting := false
		// Check for duplicates in the DB
		for _, existing := range existingBlobs {
			if existing.Hash.Equals(notification.blob.Hash) && existing.PayloadRef == notification.blob.PayloadRef {
				foundExisting = true
				break
			}
		}
		// Check for duplicates in the notifications
		for _, inBatch := range newBlobs {
			if inBatch.Hash.Equals(notification.blob.Hash) && inBatch.PayloadRef == notification.blob.PayloadRef {
				foundExisting = true
				break
			}
		}
		if !foundExisting {
			newBlobs = append(newBlobs, notification.blob)
			newHashes = append(newHashes, notification.blob.Hash)
		}
	}

	// Insert the new blobs
	if len(newBlobs) > 0 {
		err = br.database.InsertBlobs(ctx, newBlobs)
		if err != nil {
			return nil, err
		}
	}
	return newHashes, nil

}
