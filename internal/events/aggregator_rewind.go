// Copyright © 2022 Kaleido, Inc.
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
	"sync"
	"time"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/common/pkg/retry"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/data"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

type rewindType int

const (
	rewindBatch rewindType = iota
	rewindMessage
	rewindBlob
	rewindDIDConfirmed
)

type rewind struct {
	rewindType rewindType
	uuid       fftypes.UUID
	hash       fftypes.Bytes32
	did        string
}

type rewinder struct {
	ctx              context.Context
	aggregator       *aggregator
	database         database.Plugin
	data             data.Manager
	retry            *retry.Retry
	loop1Done        chan struct{}
	loop2Done        chan struct{}
	minRewindTimeout time.Duration
	mux              sync.Mutex
	rewindRequests   chan rewind
	queuedRewinds    []*rewind
	stagedRewinds    []*rewind
	loop2ShoulderTap chan bool
	readyRewinds     map[fftypes.UUID]bool
	querySafetyLimit uint64
}

func newRewinder(ag *aggregator) *rewinder {
	return &rewinder{
		ctx:              log.WithLogFields(ag.ctx, "role", "aggregator-rewind"),
		aggregator:       ag,
		database:         ag.database,
		data:             ag.data,
		retry:            ag.retry,
		loop1Done:        make(chan struct{}),
		loop2Done:        make(chan struct{}),
		rewindRequests:   make(chan rewind, config.GetInt(coreconfig.EventAggregatorRewindQueueLength)),
		loop2ShoulderTap: make(chan bool, 1),
		minRewindTimeout: config.GetDuration(coreconfig.EventAggregatorRewindTimeout),
		querySafetyLimit: uint64(config.GetUint((coreconfig.EventAggregatorRewindQueryLimit))),
		readyRewinds:     make(map[fftypes.UUID]bool),
	}
}

func (rw *rewinder) start() {
	go rw.rewindReceiveLoop()
	go rw.rewindProcessLoop()
}

// rewindReceiveLoop is responsible for taking requests to the aggregator for rewinds, from other
// event loops that execute completely independently.
// It takes them in as fast as possible, and puts them in a staging area ready for the aggregator
// itself to mark it's finished processing anything in-flight
func (rw *rewinder) rewindReceiveLoop() {
	defer close(rw.loop1Done)

	for {
		select {
		case rewind := <-rw.rewindRequests:
			rw.mux.Lock()
			rw.queuedRewinds = append(rw.queuedRewinds, &rewind)
			rw.mux.Unlock()

			// Shoulder tap at this point, to get the event loop to pop and tell us
			// we can move the queued rewinds to staged
			rw.aggregator.eventPoller.shoulderTap()
		case <-rw.ctx.Done():
			log.L(rw.ctx).Debugf("Rewind Receiver Loop stopping")
			return
		}
	}
}

// rewindProcessLoop does the heavy lifting of taking rewinds that the aggregator has marked staged,
// and doing the DB queries required to find out what BatchIDs need to be resolved.
// These then go to the readyRewinds map, ready for the aggregator to pop them
func (rw *rewinder) rewindProcessLoop() {
	defer close(rw.loop2Done)

	var timerCtx context.Context
	var timerCancel context.CancelFunc
	for {
		var popCtx context.Context
		if timerCtx != nil {
			// Pop when we've timed out
			popCtx = timerCtx
		} else {
			// Only pop for new work, or shutdown
			popCtx = rw.ctx
		}
		select {
		case <-popCtx.Done():
			if timerCtx == nil && timerCancel == nil {
				log.L(rw.ctx).Debugf("Rewind Processor Loop stopping")
				return
			}
			timerCancel()
			timerCtx = nil
			timerCancel = nil
			if rw.processStagedRewinds() {
				// Shoulder tap at this point, to get the event loop to pop and retrieve
				// the rewinds we have just derived batch IDs from
				rw.aggregator.eventPoller.shoulderTap()
			}
		case <-rw.loop2ShoulderTap:
			if timerCtx == nil {
				// We've been told at least one rewind is available, so start a timer
				// to wait for more to accumulate before we un-stage them
				timerCtx, timerCancel = context.WithTimeout(rw.ctx, rw.minRewindTimeout)
			}
		}
	}
}

func (rw *rewinder) processStagedRewinds() bool {
	batchIDs := make(map[fftypes.UUID]bool)
	var msgRewinds []*fftypes.UUID
	var newBlobHashes []driver.Value
	var identityRewinds []driver.Value

	// Pop the current batch of rewinds out of the staging area
	rw.mux.Lock()
	for _, rewind := range rw.stagedRewinds {
		switch rewind.rewindType {
		case rewindBatch:
			batchIDs[rewind.uuid] = true
		case rewindBlob:
			newBlobHashes = append(newBlobHashes, &rewind.hash)
		case rewindMessage:
			msgRewinds = append(msgRewinds, &rewind.uuid)
		case rewindDIDConfirmed:
			identityRewinds = append(identityRewinds, rewind.did)
		}
	}
	rw.stagedRewinds = rw.stagedRewinds[:0] // truncate
	rw.mux.Unlock()

	// We need to resolve these, so execute in a retry loop
	err := rw.retry.Do(rw.ctx, "process staged rewinds", func(attempt int) (retry bool, err error) {
		return true, rw.database.RunAsGroup(rw.ctx, func(ctx context.Context) error {
			if len(msgRewinds) > 0 {
				if err := rw.getRewindsForMessages(ctx, msgRewinds, batchIDs); err != nil {
					return err
				}
			}
			if len(newBlobHashes) > 0 {
				if err := rw.getRewindsForBlobs(ctx, newBlobHashes, batchIDs); err != nil {
					return err
				}
			}
			if len(identityRewinds) > 0 {
				if err := rw.getRewindsForDIDs(ctx, identityRewinds, batchIDs); err != nil {
					return err
				}
			}
			return nil
		})
	})
	if err != nil {
		log.L(rw.ctx).Debugf("closed context found while processing rewinds: %s", err)
	}

	// Store back what we found for the next popRewinds call
	rw.mux.Lock()
	for batchID := range batchIDs {
		rw.readyRewinds[batchID] = true
	}
	rw.mux.Unlock()

	return len(batchIDs) > 0
}

// popRewinds is the function called by aggregator each time round its loop
func (rw *rewinder) popRewinds() []driver.Value {
	rw.mux.Lock()

	// We can move our queued rewinds to be staged rewinds
	rw.stagedRewinds = append(rw.stagedRewinds, rw.queuedRewinds...)
	rw.queuedRewinds = rw.queuedRewinds[:0] // truncate
	if len(rw.stagedRewinds) > 0 {
		select {
		case rw.loop2ShoulderTap <- true:
		default:
		}
	}

	// Add pop out all the ready rewinds for the aggregator to execute
	batchIDs := make([]driver.Value, 0, len(rw.readyRewinds))
	for batchID := range rw.readyRewinds {
		batchIDs = append(batchIDs, batchID)
	}
	rw.readyRewinds = make(map[fftypes.UUID]bool) // clear the map

	rw.mux.Unlock()

	return batchIDs
}

func (rw *rewinder) getRewindsForMessages(ctx context.Context, msgRewinds []*fftypes.UUID, batchIDs map[fftypes.UUID]bool) error {

	// We use the data manager cache to look up each message, specifying that we need there to be a batch ID in order
	// for us to consider it a cache hit. For any misses we do a single targeted query against the DB for just the batch IDs
	var cacheMisses []*fftypes.UUID
	for _, msgID := range msgRewinds {
		msg, _ := rw.data.PeekMessageCache(ctx, msgID, data.CRORequireBatchID)
		if msg != nil && msg.BatchID != nil {
			batchIDs[*msg.BatchID] = true
		} else {
			cacheMisses = append(cacheMisses, msgID)
		}
	}

	if len(cacheMisses) > 0 {
		log.L(ctx).Debugf("Cache misses for message rewinds: %v", cacheMisses)
		msgBatchIDs, err := rw.database.GetBatchIDsForMessages(ctx, rw.aggregator.namespace, cacheMisses)
		if err != nil {
			return err
		}
		for _, batchID := range msgBatchIDs {
			log.L(ctx).Debugf("Messages %v caused rewind for batch %s", msgBatchIDs, batchID)
			batchIDs[*batchID] = true
		}
	}

	return nil
}

func (rw *rewinder) getRewindsForBlobs(ctx context.Context, newHashes []driver.Value, batchIDs map[fftypes.UUID]bool) error {

	// Find any data associated with this blob
	var data []*core.DataRef
	filter := database.DataQueryFactory.NewFilterLimit(ctx, rw.querySafetyLimit).In("blob.hash", newHashes)
	data, _, err := rw.database.GetDataRefs(ctx, rw.aggregator.namespace, filter)
	if err != nil {
		return err
	}

	// Find the batch IDs of all messages with these data IDs as attachments
	if len(data) > 0 {

		dataIDs := make([]*fftypes.UUID, len(data))
		for i, d := range data {
			dataIDs[i] = d.ID
		}
		msgBatchIDs, err := rw.database.GetBatchIDsForDataAttachments(ctx, rw.aggregator.namespace, dataIDs)
		if err != nil {
			return err
		}
		for _, batchID := range msgBatchIDs {
			log.L(ctx).Debugf("Data %v caused rewind for batch %s", dataIDs, batchID)
			batchIDs[*batchID] = true
		}

	}

	return nil
}

func (rw *rewinder) getRewindsForDIDs(ctx context.Context, dids []driver.Value, batchIDs map[fftypes.UUID]bool) error {

	// We need to find all pending messages, that are authored by this DID
	fb := database.MessageQueryFactory.NewFilterLimit(ctx, rw.querySafetyLimit)
	filter := fb.And(
		fb.Eq("state", core.MessageStatePending),
		fb.In("author", dids),
	)
	records, err := rw.database.GetMessageIDs(ctx, rw.aggregator.namespace, filter)
	if err != nil {
		return err
	}
	if len(records) == 0 {
		return nil
	}
	msgIDs := make([]*fftypes.UUID, len(records))
	for i, record := range records {
		msgIDs[i] = &record.ID
	}

	// We can treat the message level rewinds just like any other message rewind
	return rw.getRewindsForMessages(ctx, msgIDs, batchIDs)
}
