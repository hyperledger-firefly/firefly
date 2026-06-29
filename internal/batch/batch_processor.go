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

package batch

import (
	"context"
	"database/sql/driver"
	"math"
	"sync"
	"time"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/common/pkg/retry"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/internal/data"
	"github.com/hyperledger-firefly/firefly/internal/operations"
	"github.com/hyperledger-firefly/firefly/internal/txcommon"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

type batchWork struct {
	msg  *core.Message
	data core.DataArray
}

type batchProcessorConf struct {
	DispatcherOptions
	name           string
	dispatcherName string
	pinned         bool
	author         string
	group          *fftypes.Bytes32
	dispatch       DispatchHandler
}

// FlushStatus is an object that can be returned on REST queries to understand the status
// of the batch processor
type FlushStatus struct {
	LastFlushTime        *fftypes.FFTime `ffstruct:"BatchFlushStatus" json:"lastFlushStartTime"`
	Flushing             *fftypes.UUID   `ffstruct:"BatchFlushStatus" json:"flushing,omitempty"`
	Cancelled            bool            `ffstruct:"BatchFlushStatus" json:"cancelled"`
	Blocked              bool            `ffstruct:"BatchFlushStatus" json:"blocked"`
	LastFlushError       string          `ffstruct:"BatchFlushStatus" json:"lastFlushError,omitempty"`
	LastFlushErrorTime   *fftypes.FFTime `ffstruct:"BatchFlushStatus" json:"lastFlushErrorTime,omitempty"`
	AverageBatchBytes    int64           `ffstruct:"BatchFlushStatus" json:"averageBatchBytes"`
	AverageBatchMessages float64         `ffstruct:"BatchFlushStatus" json:"averageBatchMessages"`
	AverageBatchData     float64         `ffstruct:"BatchFlushStatus" json:"averageBatchData"`
	AverageFlushTimeMS   int64           `ffstruct:"BatchFlushStatus" json:"averageFlushTimeMS"`
	TotalBatches         int64           `ffstruct:"BatchFlushStatus" json:"totalBatches"`
	TotalErrors          int64           `ffstruct:"BatchFlushStatus" json:"totalErrors"`

	totalBytesFlushed    int64
	totalMessagesFlushed int64
	totalDataFlushed     int64
	totalFlushDuration   time.Duration
}

type batchProcessor struct {
	ctx                context.Context
	bm                 *batchManager
	data               data.Manager
	database           database.Plugin
	txHelper           txcommon.Helper
	cancelCtx          func()
	done               chan struct{}
	quiescing          chan bool
	newWork            chan *batchWork
	assemblyID         *fftypes.UUID
	assemblyQueue      []*batchWork
	assemblyQueueBytes int64
	statusMux          sync.Mutex
	flushStatus        FlushStatus
	retry              *retry.Retry
	conf               *batchProcessorConf
}

type nonceState struct {
	latest int64
	new    bool
}

type dispatchState struct {
	msgPins        map[fftypes.UUID]fftypes.FFStringArray
	noncesAssigned map[fftypes.Bytes32]*nonceState
}

type MessageUpdate struct {
	messages  []*core.Message
	fromState core.MessageState
	toState   core.MessageState
}

type DispatchPayload struct {
	Batch          core.BatchPersisted
	Messages       []*core.Message
	Data           core.DataArray
	Pins           []*fftypes.Bytes32
	MessageUpdates map[string]*MessageUpdate
}

func (dp *DispatchPayload) addMessageUpdate(messages []*core.Message, fromState core.MessageState, toState core.MessageState) {
	key := string(fromState + ":" + toState)
	if dp.MessageUpdates == nil {
		dp.MessageUpdates = make(map[string]*MessageUpdate)
	}
	entry, found := dp.MessageUpdates[key]
	if !found {
		entry = &MessageUpdate{fromState: fromState, toState: toState}
		dp.MessageUpdates[key] = entry
	}
	entry.messages = append(entry.messages, messages...)
}

const batchSizeEstimateBase = int64(512)

func newBatchProcessor(bm *batchManager, conf *batchProcessorConf, baseRetryConf *retry.Retry, txHelper txcommon.Helper) *batchProcessor {
	pCtx := log.WithLogField(log.WithLogField(bm.ctx, "d", conf.dispatcherName), "p", conf.name)
	pCtx, cancelCtx := context.WithCancel(pCtx)
	bp := &batchProcessor{
		ctx:       pCtx,
		cancelCtx: cancelCtx,
		bm:        bm,
		database:  bm.database,
		data:      bm.data,
		txHelper:  txHelper,
		newWork:   make(chan *batchWork, conf.BatchMaxSize),
		quiescing: make(chan bool, 1),
		done:      make(chan struct{}),
		retry: &retry.Retry{
			InitialDelay: baseRetryConf.InitialDelay,
			MaximumDelay: baseRetryConf.MaximumDelay,
			Factor:       baseRetryConf.Factor,
		},
		conf: conf,
		flushStatus: FlushStatus{
			LastFlushTime: fftypes.Now(),
		},
	}
	// Capture flush errors for our status
	bp.retry.ErrCallback = bp.captureFlushError
	bp.newAssembly()
	go bp.assemblyLoop()
	log.L(pCtx).Infof("Batch processor created")
	return bp
}

func (bw *batchWork) estimateSize() int64 {
	sizeEstimate := bw.msg.EstimateSize(false /* we calculate data size separately, as we have the full data objects */)
	for _, d := range bw.data {
		sizeEstimate += d.EstimateSize()
	}
	return sizeEstimate
}

func (bp *batchProcessor) status() *ProcessorStatus {
	bp.statusMux.Lock()
	defer bp.statusMux.Unlock()
	return &ProcessorStatus{
		Dispatcher: bp.conf.dispatcherName,
		Name:       bp.conf.name,
		Status:     bp.flushStatus, // copy
	}
}

func (bp *batchProcessor) newAssembly(initialWork ...*batchWork) {
	bp.assemblyID = fftypes.NewUUID()
	bp.assemblyQueue = append([]*batchWork{}, initialWork...)
	bp.assemblyQueueBytes = batchSizeEstimateBase
}

// addWork adds the work to the assemblyQueue, and calculates if we have overflowed with this work.
// We check for duplicates, and add the work in sequence order.
// This helps in the case for parallel REST APIs all committing to the DB at a similar time.
// With a sufficient batch size and batch timeout, the batch will still dispatch the messages
// in DB sequence order (although this is not guaranteed).
func (bp *batchProcessor) addWork(newWork *batchWork) (full, overflow bool) {
	newQueue := make([]*batchWork, 0, len(bp.assemblyQueue)+1)
	added := false

	if newWork.msg.BatchID != nil {
		log.L(bp.ctx).Warnf("Adding message to a new batch when one was already assigned. Old batch %s is likely abandoned.", newWork.msg.BatchID)
	}

	// Check for conditions that prevent this piece of work from going into the current batch
	// (i.e. the new work is specifically assigned a separate transaction or signing key)
	batchOfOne := newWork.msg.Header.TxType == core.TransactionTypeContractInvokePin
	if batchOfOne {
		full = true
		overflow = len(bp.assemblyQueue) > 0
	} else if len(bp.assemblyQueue) > 0 {
		full = newWork.msg.Header.TxType != bp.assemblyQueue[0].msg.Header.TxType ||
			newWork.msg.Header.Key != bp.assemblyQueue[0].msg.Header.Key
		overflow = true
	}

	// Build the new sorted work list
	if full {
		bp.assemblyQueue = append(bp.assemblyQueue, newWork)
	} else {
		for _, work := range bp.assemblyQueue {
			if !added && newWork.msg.Sequence < work.msg.Sequence {
				newQueue = append(newQueue, newWork)
				added = true
			}
			newQueue = append(newQueue, work)
		}
		if !added {
			newQueue = append(newQueue, newWork)
		}

		bp.assemblyQueueBytes += newWork.estimateSize()
		bp.assemblyQueue = newQueue

		full = len(bp.assemblyQueue) >= bp.conf.BatchMaxSize || bp.assemblyQueueBytes >= bp.conf.BatchMaxBytes
		overflow = len(bp.assemblyQueue) > 1 && (batchOfOne || bp.assemblyQueueBytes > bp.conf.BatchMaxBytes)
	}

	log.L(bp.ctx).Debugf("Added message %s sequence=%d to in-flight batch assembly %s", newWork.msg.Header.ID, newWork.msg.Sequence, bp.assemblyID)
	return full, overflow
}

func (bp *batchProcessor) startFlush(overflow bool) (id *fftypes.UUID, flushAssembly []*batchWork, byteSize int64) {
	bp.statusMux.Lock()
	defer bp.statusMux.Unlock()
	// Start the clock
	bp.flushStatus.LastFlushTime = fftypes.Now()
	// Split the current work if required for overflow
	overflowWork := make([]*batchWork, 0)
	if overflow {
		lastElem := len(bp.assemblyQueue) - 1
		flushAssembly = append(flushAssembly, bp.assemblyQueue[:lastElem]...)
		overflowWork = append(overflowWork, bp.assemblyQueue[lastElem])
	} else {
		flushAssembly = bp.assemblyQueue
	}
	// Cycle to the next assembly
	id = bp.assemblyID
	byteSize = bp.assemblyQueueBytes
	bp.flushStatus.Flushing = id
	bp.newAssembly(overflowWork...)
	return id, flushAssembly, byteSize
}

func (bp *batchProcessor) notifyFlushComplete(flushWork []*batchWork) {
	sequences := make([]int64, len(flushWork))
	for i, work := range flushWork {
		sequences[i] = work.msg.Sequence
	}
	bp.bm.notifyFlushed(sequences)
}

func (bp *batchProcessor) updateFlushStats(payload *DispatchPayload, byteSize int64) {
	bp.statusMux.Lock()
	defer bp.statusMux.Unlock()
	fs := &bp.flushStatus

	duration := time.Since(*fs.LastFlushTime.Time())
	fs.Flushing = nil
	fs.Blocked = false
	fs.Cancelled = false

	fs.TotalBatches++

	fs.totalFlushDuration += duration
	fs.AverageFlushTimeMS = (fs.totalFlushDuration / time.Duration(fs.TotalBatches)).Milliseconds()

	fs.totalBytesFlushed += byteSize
	fs.AverageBatchBytes = (fs.totalBytesFlushed / fs.TotalBatches)

	fs.totalMessagesFlushed += int64(len(payload.Messages))
	fs.AverageBatchMessages = math.Round((float64(fs.totalMessagesFlushed)/float64(fs.TotalBatches))*100) / 100

	fs.totalDataFlushed += int64(len(payload.Data))
	fs.AverageBatchData = math.Round((float64(fs.totalDataFlushed)/float64(fs.TotalBatches))*100) / 100
}

func (bp *batchProcessor) captureFlushError(err error) {
	bp.statusMux.Lock()
	defer bp.statusMux.Unlock()
	fs := &bp.flushStatus

	fs.TotalErrors++
	fs.Blocked = true
	fs.LastFlushErrorTime = fftypes.Now()
	fs.LastFlushError = err.Error()
}

func (bp *batchProcessor) cancelFlush(ctx context.Context, id *fftypes.UUID) error {
	bp.statusMux.Lock()
	defer bp.statusMux.Unlock()
	fs := &bp.flushStatus
	if !id.Equals(fs.Flushing) {
		return i18n.NewError(ctx, coremsgs.MsgBatchNotDispatching, id, fs.Flushing)
	}
	fs.Cancelled = true
	return nil
}

func (bp *batchProcessor) isCancelled() bool {
	bp.statusMux.Lock()
	defer bp.statusMux.Unlock()
	fs := &bp.flushStatus

	return fs.Cancelled
}

func (bp *batchProcessor) startQuiesce() {
	// We are ready to quiesce, but we can't safely close our input channel.
	// We just do a non-blocking pass (queue length is 1) to the manager to
	// ask them to close our channel before their next read.
	// One more item of work might get through the pipe in an edge case here.
	select {
	case bp.quiescing <- true:
	default:
	}
}

// The assemblyLoop receives new work, sorts it, and waits for the size/timer to pop before
// flushing the batch. The newWork channel has up to one batch of slots queue length,
// so that we can have one batch of work queuing for assembly, while we have one batch flushing.
func (bp *batchProcessor) assemblyLoop() {
	defer close(bp.done)
	l := log.L(bp.ctx)

	var batchTimeout = time.NewTimer(bp.conf.DisposeTimeout)
	idle := true
	quiescing := false
	for !quiescing {

		var timedout, full, overflow bool
		select {
		case <-bp.ctx.Done():
			l.Tracef("Batch processor shutting down")
			_ = batchTimeout.Stop()
			return
		case <-batchTimeout.C:
			l.Debugf("Batch timer popped")
			if len(bp.assemblyQueue) == 0 {
				bp.startQuiesce()
			} else {
				// We need to flush
				timedout = true
			}
		case work, ok := <-bp.newWork:
			if !ok {
				quiescing = true
			} else {
				full, overflow = bp.addWork(work)
				if idle {
					// We've hit a message while we were idle - we now need to wait for the batch to time out.
					_ = batchTimeout.Stop()
					batchTimeout = time.NewTimer(bp.conf.BatchTimeout)
					idle = false
				}
			}
		}
		if (full || timedout || quiescing) && len(bp.assemblyQueue) > 0 {
			// Let Go GC the old timer
			_ = batchTimeout.Stop()

			// If we are in overflow, start the clock for the next batch to start before we do the flush
			// (even though we won't check it until after).
			if overflow {
				batchTimeout = time.NewTimer(bp.conf.BatchTimeout)
			}

			err := bp.flush(overflow)
			if err != nil {
				l.Warnf("Batch processor shutting down: %s", err)
				_ = batchTimeout.Stop()
				return
			}

			// If we didn't overflow, then just go back to idle - we don't know if we have more work to come, so
			// either we'll pop straight away (and move to the batch timeout) or wait for the dispose timeout
			if !overflow && !quiescing {
				batchTimeout = time.NewTimer(bp.conf.DisposeTimeout)
				idle = true
			}
		}
	}
}

func (bp *batchProcessor) flush(overflow bool) error {
	id, flushWork, byteSize := bp.startFlush(overflow)

	log.L(bp.ctx).Debugf("Flushing batch %s", id)
	state := bp.initPayload(id, flushWork)

	// Sealing phase: assigns persisted pins to messages, and finalizes the manifest
	err := bp.sealBatch(state)
	if err != nil {
		return err
	}
	log.L(bp.ctx).Debugf("Sealed batch %s", id)

	// Dispatch phase: the heavy lifting work - calling plugins to do the hard work of the batch.
	//   The dispatcher can update the state, such as appending to the BlobsPublished array,
	//   to affect DB updates as part of the finalization phase.
	err = bp.dispatchBatch(state)
	if err != nil {
		return err
	}
	log.L(bp.ctx).Debugf("Dispatched batch %s", id)

	// Finalization phase: Writes back the changes to the DB, so that these messages
	//   are all tagged as part of this batch, and won't be included in any future batches.
	err = bp.markPayloadDispatched(state)
	if err != nil {
		return err
	}
	log.L(bp.ctx).Debugf("Finalized batch %s", id)

	// Notify the manager that we've flushed these sequences
	bp.notifyFlushComplete(flushWork)

	// Update our stats
	bp.updateFlushStats(state, byteSize)
	return nil
}

func (bp *batchProcessor) initPayload(id *fftypes.UUID, flushWork []*batchWork) *DispatchPayload {
	payload := &DispatchPayload{
		Batch: core.BatchPersisted{
			BatchHeader: core.BatchHeader{
				ID:        id,
				Type:      bp.conf.DispatcherOptions.BatchType,
				Namespace: bp.bm.namespace,
				SignerRef: core.SignerRef{
					Author: bp.conf.author,
					Key:    flushWork[0].msg.Header.Key,
				},
				Group:   bp.conf.group,
				Created: fftypes.Now(),
			},
			TX: core.TransactionRef{
				Type: flushWork[0].msg.Header.TxType,
			},
		},
	}
	localNode, err := bp.bm.identity.GetLocalNode(bp.ctx)
	if err == nil && localNode != nil {
		payload.Batch.BatchHeader.Node = localNode.ID
	}
	for _, w := range flushWork {
		if w.msg != nil {
			payload.Messages = append(payload.Messages, w.msg.BatchMessage())
		}
		for _, d := range w.data {
			log.L(bp.ctx).Debugf("Adding data '%s' to batch '%s' for message '%s'", d.ID, id, w.msg.Header.ID)
			payload.Data = append(payload.Data, d.BatchData(payload.Batch.Type))
		}
	}
	return payload
}

// Calculate the contexts/pins for this batch payload
func (bp *batchProcessor) calculateContexts(ctx context.Context, payload *DispatchPayload, state *dispatchState) error {
	payload.Pins = make([]*fftypes.Bytes32, 0)
	for _, msg := range payload.Messages {
		isPrivate := msg.Header.Group != nil
		if isPrivate && len(msg.Pins) > 0 {
			// We have already allocated pins to this message, we cannot re-allocate.
			log.L(ctx).Debugf("Message %s already has %d pins allocated", msg.Header.ID, len(msg.Pins))
			pins, err := bp.bm.loadContext(ctx, msg)
			if err != nil {
				return err
			}
			payload.Pins = append(payload.Pins, pins...)
			continue
		}
		var pins fftypes.FFStringArray
		if isPrivate {
			pins = make(fftypes.FFStringArray, len(msg.Header.Topics))
			state.msgPins[*msg.Header.ID] = pins
		}
		for i, topic := range msg.Header.Topics {
			pinString, contextOrPin, err := bp.bm.maskContext(ctx, state, msg, topic)
			if err != nil {
				return err
			}
			payload.Pins = append(payload.Pins, contextOrPin)
			if isPrivate {
				pins[i] = pinString
			}
		}
	}
	return nil
}

func (bp *batchProcessor) flushNonceState(ctx context.Context, state *dispatchState) error {

	// Flush all the assigned nonces to the DB
	for hash, nonceState := range state.noncesAssigned {
		dbNonce := &core.Nonce{
			Hash:  &hash,
			Nonce: nonceState.latest,
		}
		if nonceState.new {
			if err := bp.database.InsertNonce(ctx, dbNonce); err != nil {
				return err
			}
		} else {
			if err := bp.database.UpdateNonce(ctx, dbNonce); err != nil {
				return err
			}
		}
	}

	return nil
}

func (bp *batchProcessor) updateMessagePins(ctx context.Context, messages []*core.Message, state *dispatchState) error {
	// It's important we update the message pins at this phase, as we have "spent" a nonce
	// on this topic from the database. So this message has grabbed a slot in our queue.
	// If we fail the dispatch, and redo the batch sealing process, we must not allocate
	// a second nonce to it (and as such modifiy the batch payload).
	//
	// Note to consider database transactions, we do not update the msg.Pins array, or the
	// message cache, until after the Retry/RunAsGroup has ended in sealBatch.
	for _, msg := range messages {
		if pins, ok := state.msgPins[*msg.Header.ID]; ok {
			if err := bp.database.UpdateMessage(ctx, bp.bm.namespace, msg.Header.ID,
				database.MessageQueryFactory.NewUpdate(ctx).Set("pins", pins),
			); err != nil {
				return err
			}
		}
	}
	return nil
}

func (bp *batchProcessor) sealBatch(payload *DispatchPayload) (err error) {
	var state *dispatchState
	txType := payload.Batch.TX.Type

	err = bp.retry.Do(bp.ctx, "batch persist", func(attempt int) (retry bool, err error) {
		return true, bp.database.RunAsGroup(bp.ctx, func(ctx context.Context) (err error) {

			// Clear state from any previous retry. We need to do fresh queries against the DB for nonces.
			state = &dispatchState{
				noncesAssigned: make(map[fftypes.Bytes32]*nonceState),
				msgPins:        make(map[fftypes.UUID]fftypes.FFStringArray),
			}

			// Assign nonces and update nonces/messages in the database
			if core.IsPinned(txType) {
				if err = bp.calculateContexts(ctx, payload, state); err != nil {
					return err
				}
				if err = bp.flushNonceState(ctx, state); err != nil {
					return err
				}
				if err = bp.updateMessagePins(ctx, payload.Messages, state); err != nil {
					return err
				}
			}

			batchOfOne := txType == core.TransactionTypeContractInvokePin
			if batchOfOne && payload.Messages[0].TransactionID != nil {
				// For a batch-of-one with a pre-assigned transaction ID, propagate it to the batch
				payload.Batch.TX.ID = payload.Messages[0].TransactionID
			} else {
				// For all others, generate a new transaction
				payload.Batch.TX.ID, err = bp.txHelper.SubmitNewTransaction(ctx, txType, "" /* no idempotency key */)
				if err != nil {
					return err
				}
			}

			// The hash of the batch, is the hash of the manifest to minimize the compute cost.
			// Note in v0.13 and before, it was the hash of the payload - so the inbound route has a fallback to accepting the full payload hash
			manifest := payload.Batch.GenManifest(payload.Messages, payload.Data)
			manifestString := manifest.String()
			payload.Batch.Manifest = fftypes.JSONAnyPtr(manifestString)
			payload.Batch.Hash = fftypes.HashString(manifestString)
			log.L(ctx).Debugf("Batch %s sealed. Hash=%s", payload.Batch.ID, payload.Batch.Hash)

			// At this point the manifest of the batch is finalized. We write it to the database
			_, err = bp.database.InsertOrGetBatch(ctx, &payload.Batch)
			return err
		})
	})
	if err != nil {
		return err
	}

	// Once the DB transaction is done, we need to update the messages with the pins.
	// We do this at this point, so the logic is re-entrant in a way that avoids re-allocating Pins to messages, but
	// means the retry loops above using the batch state function correctly.
	if state != nil && core.IsPinned(txType) {
		for _, msg := range payload.Messages {
			if pins, ok := state.msgPins[*msg.Header.ID]; ok {
				msg.Pins = pins
				bp.data.UpdateMessageIfCached(bp.ctx, msg)
			}
		}
	}
	return nil
}

func (bp *batchProcessor) dispatchBatch(payload *DispatchPayload) error {
	// Call the dispatcher to do the heavy lifting - will only exit if we're closed
	return operations.RunWithOperationContext(bp.ctx, func(ctx context.Context) error {
		return bp.retry.Do(ctx, "batch dispatch", func(attempt int) (retry bool, err error) {
			err = bp.conf.dispatch(ctx, payload)
			if err != nil {
				if bp.isCancelled() {
					var gapFillPayload *DispatchPayload
					gapFillPayload, err = bp.prepareGapFill(ctx, payload)
					if err == nil {
						payload.addMessageUpdate(payload.Messages, core.MessageStateReady, core.MessageStateCancelled)
						if gapFillPayload != nil {
							payload.addMessageUpdate(gapFillPayload.Messages, core.MessageStateStaged, core.MessageStateSent)
							err = bp.dispatchBatch(gapFillPayload)
						}
					}
				}
				conflictErr, conflictTestOk := err.(operations.ConflictError)
				if conflictTestOk && conflictErr.IsConflictError() {
					// We know that the connector has received our batch, so we shouldn't need to retry
					payload.addMessageUpdate(payload.Messages, core.MessageStateReady, core.MessageStateSent)
					return true, nil
				}
			} else {
				if core.IsPinned(payload.Batch.TX.Type) {
					payload.addMessageUpdate(payload.Messages, core.MessageStateReady, core.MessageStateSent)
				} else {
					payload.addMessageUpdate(payload.Messages, core.MessageStateReady, core.MessageStateConfirmed)
				}
			}
			return true, err
		})
	})
}

func (bp *batchProcessor) prepareGapFill(ctx context.Context, payload *DispatchPayload) (*DispatchPayload, error) {
	// Gap fill is only needed for private custom pinned messages
	if payload.Batch.Type != core.MessageTypePrivate || payload.Batch.TX.Type != core.TransactionTypeContractInvokePin {
		return nil, nil
	}
	log.L(ctx).Warnf("Batch %s was cancelled - replacing with gap fill", payload.Batch.ID)

	gapFills := make([]*core.Message, len(payload.Messages))
	for i, msg := range payload.Messages {
		gapFill := &core.MessageInOut{
			Message: core.Message{
				Header:         msg.Header,
				LocalNamespace: bp.bm.namespace,
				State:          core.MessageStateStaged, // "staged" not "ready" to avoid the normal message dispatch loop
				Pins:           msg.Pins,                // reuse any assigned pins to fill the nonce gap
			},
		}
		gapFill.Header.ID = fftypes.NewUUID()
		gapFill.Header.CID = msg.Header.ID
		gapFill.Header.Tag = core.SystemTagGapFill
		gapFill.Header.TxType = core.TransactionTypeBatchPin
		err := gapFill.Seal(ctx)
		if err == nil {
			err = bp.data.WriteNewMessage(ctx, &data.NewMessage{Message: gapFill})
		}
		if err != nil {
			return nil, err
		}
		gapFills[i] = &gapFill.Message
	}

	gapFillPayload := &DispatchPayload{
		Batch: core.BatchPersisted{
			BatchHeader: payload.Batch.BatchHeader,
			TX: core.TransactionRef{
				Type: core.TransactionTypeBatchPin,
			},
		},
		Messages: gapFills,
	}
	gapFillPayload.Batch.ID = fftypes.NewUUID()
	log.L(ctx).Infof("Prepared gap fill batch %s", gapFillPayload.Batch.ID)

	err := bp.sealBatch(gapFillPayload)
	if err == nil {
		log.L(ctx).Infof("Sealed gap fill batch %s", gapFillPayload.Batch.ID)
	}

	return gapFillPayload, err
}

func (bp *batchProcessor) markPayloadDispatched(payload *DispatchPayload) error {
	return bp.retry.Do(bp.ctx, "mark dispatched messages", func(attempt int) (retry bool, err error) {
		return true, bp.database.RunAsGroup(bp.ctx, func(ctx context.Context) (err error) {
			confirmTime := fftypes.Now()
			for _, state := range payload.MessageUpdates {
				// Update the message state in the cache
				msgIDs := make([]driver.Value, len(state.messages))
				for i, msg := range state.messages {
					msgIDs[i] = msg.Header.ID
					msg.BatchID = payload.Batch.ID
					msg.TransactionID = payload.Batch.TX.ID
					if state.toState == core.MessageStateConfirmed {
						msg.Confirmed = confirmTime
					}
					bp.data.UpdateMessageIfCached(ctx, msg)
				}

				// Update the message state in the database
				fb := database.MessageQueryFactory.NewFilter(ctx)
				filter := fb.And(
					fb.In("id", msgIDs),
					fb.Eq("state", state.fromState), // In the outside chance the next state transition happens first (which supersedes this)
				)
				allMsgsUpdate := database.MessageQueryFactory.NewUpdate(ctx).
					Set("batch", payload.Batch.ID).
					Set("txid", payload.Batch.TX.ID).
					Set("state", state.toState)
				if state.toState == core.MessageStateConfirmed {
					allMsgsUpdate.Set("confirmed", confirmTime)
				}
				if err = bp.database.UpdateMessages(ctx, bp.bm.namespace, filter, allMsgsUpdate); err != nil {
					return err
				}

				if state.toState == core.MessageStateConfirmed {
					for _, msg := range state.messages {
						// Emit a confirmation event locally immediately
						for _, topic := range msg.Header.Topics {
							// One event per topic
							event := core.NewEvent(core.EventTypeMessageConfirmed, payload.Batch.Namespace, msg.Header.ID, payload.Batch.TX.ID, topic)
							event.Correlator = msg.Header.CID
							if err := bp.database.InsertEvent(ctx, event); err != nil {
								return err
							}
						}
					}
				}
			}
			return nil
		})
	})
}
