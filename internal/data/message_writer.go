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

package data

import (
	"context"
	"time"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

type NewMessage struct {
	Message *core.MessageInOut
	AllData core.DataArray
	NewData core.DataArray
}

// writeRequest is a combination of a message and a list of data that is new and needs to be
// inserted into the database.
type writeRequest struct {
	id         *fftypes.UUID
	newMessage *core.Message
	newData    core.DataArray
	result     chan error
}

type messageWriterBatch struct {
	messages       []*core.Message
	data           core.DataArray
	listeners      map[fftypes.UUID]chan error
	timeoutContext context.Context
	timeoutCancel  func()
}

// messageWriter manages writing messages to the database.
//
// Where supported, it starts background workers to perform batch commits against the database,
// to allow high throughput insertion of messages + data.
//
// Multiple message writers can be started, to combine
// concurrency with batching and tune for maximum throughput.
type messageWriter struct {
	ctx         context.Context
	cancelFunc  func()
	database    database.Plugin
	workQueue   chan *writeRequest
	workersDone []chan struct{}
	conf        *messageWriterConf
	closed      bool
}

type messageWriterConf struct {
	workerCount  int
	batchTimeout time.Duration
	maxInserts   int
}

func newMessageWriter(ctx context.Context, di database.Plugin, conf *messageWriterConf) *messageWriter {
	if !di.Capabilities().Concurrency {
		log.L(ctx).Infof("Database plugin not configured for concurrency. Batched message writing disabled")
		conf.workerCount = 0
	}
	mw := &messageWriter{
		conf:     conf,
		database: di,
	}
	mw.ctx, mw.cancelFunc = context.WithCancel(ctx)
	return mw
}

// WriteNewMessage is the external interface, which depending on whether we have a non-zero
// worker count will dispatch the work to the pool and wait for it to complete on a background
// transaction, or just run it in-line on the context passed ini.
func (mw *messageWriter) WriteNewMessage(ctx context.Context, newMsg *NewMessage) error {
	log.L(ctx).Debugf("Writing message type=%s id=%s hash=%s idempotencyKey=%s concurrency=%d", newMsg.Message.Header.Type, newMsg.Message.Header.ID, newMsg.Message.Hash, newMsg.Message.IdempotencyKey, mw.conf.workerCount)
	if mw.conf.workerCount > 0 {
		// Dispatch to background worker
		nmi := &writeRequest{
			id:         newMsg.Message.Message.Header.ID,
			newMessage: &newMsg.Message.Message,
			newData:    newMsg.NewData,
			result:     make(chan error),
		}
		select {
		case mw.workQueue <- nmi:
		case <-mw.ctx.Done():
			return i18n.NewError(ctx, coremsgs.MsgContextCanceled)
		}
		return <-nmi.result
	}
	// Otherwise do it in-line on this context
	err := mw.database.RunAsGroup(ctx, func(ctx context.Context) error {
		return mw.writeMessages(ctx, []*core.Message{&newMsg.Message.Message}, newMsg.NewData)
	})
	if err != nil {
		if idempotencyErr := mw.checkIdempotencyDuplicate(ctx, &newMsg.Message.Message); idempotencyErr != nil {
			return idempotencyErr
		}
	}
	return err
}

// WriteData writes a piece of data independently of a message
func (mw *messageWriter) WriteData(ctx context.Context, data *core.Data) error {
	if mw.conf.workerCount > 0 {
		// Dispatch to background worker
		nmi := &writeRequest{
			id:      data.ID,
			newData: core.DataArray{data},
			result:  make(chan error),
		}
		select {
		case mw.workQueue <- nmi:
		case <-mw.ctx.Done():
			return i18n.NewError(ctx, coremsgs.MsgContextCanceled)
		}
		return <-nmi.result
	}
	// Otherwise do it in-line on this context
	return mw.database.UpsertData(ctx, data, database.UpsertOptimizationNew)
}

func (mw *messageWriter) start() {
	if mw.conf.workerCount > 0 {
		mw.workQueue = make(chan *writeRequest)
		mw.workersDone = make([]chan struct{}, mw.conf.workerCount)
		for i := 0; i < mw.conf.workerCount; i++ {
			mw.workersDone[i] = make(chan struct{})
			go mw.writerLoop(i)
		}
	}
}

func (mw *messageWriter) writerLoop(index int) {
	defer close(mw.workersDone[index])

	var batch *messageWriterBatch
	for !mw.closed {
		var timeoutContext context.Context
		var timedOut bool
		if batch != nil {
			timeoutContext = batch.timeoutContext
		} else {
			timeoutContext = mw.ctx
		}
		select {
		case work := <-mw.workQueue:
			if batch == nil {
				batch = &messageWriterBatch{
					listeners: make(map[fftypes.UUID]chan error),
				}
				batch.timeoutContext, batch.timeoutCancel = context.WithTimeout(mw.ctx, mw.conf.batchTimeout)
			}
			if work.newMessage != nil {
				batch.messages = append(batch.messages, work.newMessage)
			}
			batch.data = append(batch.data, work.newData...)
			batch.listeners[*work.id] = work.result
		case <-timeoutContext.Done():
			timedOut = true
		}

		if batch != nil && (timedOut || (len(batch.messages)+len(batch.data) >= mw.conf.maxInserts)) {
			batch.timeoutCancel()
			mw.persistMWBatch(batch)
			batch = nil
		}
	}
}

func (mw *messageWriter) persistMWBatch(batch *messageWriterBatch) {
	err := mw.database.RunAsGroup(mw.ctx, func(ctx context.Context) error {
		return mw.writeMessages(ctx, batch.messages, batch.data)
	})
	if err != nil {
		log.L(mw.ctx).Errorf("Failed batch message insert (pre-idempotency check): %s", err)
		duplicatesRemoved := mw.removeIdempotencyDuplicates(mw.ctx, batch)
		if duplicatesRemoved > 0 && (len(batch.messages) > 0 || len(batch.data) > 0) {
			// We have a reduced scope batch to retry, with some duplicates removed
			log.L(mw.ctx).Infof("Retrying batch insert after removing %d idempotency duplicates", duplicatesRemoved)
			err = mw.database.RunAsGroup(mw.ctx, func(ctx context.Context) error {
				return mw.writeMessages(ctx, batch.messages, batch.data)
			})
		}
	}
	for _, l := range batch.listeners {
		l <- err
	}
}

func (mw *messageWriter) checkIdempotencyDuplicate(ctx context.Context, m *core.Message) error {
	if m.IdempotencyKey != "" {
		fb := database.MessageQueryFactory.NewFilter(ctx)
		existing, _, err := mw.database.GetMessages(ctx, m.Header.Namespace, fb.Eq("idempotencykey", (string)(m.IdempotencyKey)))
		if err != nil {
			// Don't overwrite the original error for this - return -1 to the caller, who will return the previous error
			log.L(mw.ctx).Errorf("Failed checking for idempotency errors: %s", err)
			return nil
		}
		if len(existing) > 0 {
			return i18n.NewError(ctx, coremsgs.MsgIdempotencyKeyDuplicateMessage, m.IdempotencyKey, existing[0].Header.ID)
		}
	}
	return nil
}

func (mw *messageWriter) removeIdempotencyDuplicates(ctx context.Context, batch *messageWriterBatch) int {
	duplicatesRemoved := 0
	newMessageList := make([]*core.Message, 0, len(batch.messages))

	// Spin through the messages in the batch, looking for existing messages that
	for _, m := range batch.messages {
		dupRemoved := false
		idempotencyErr := mw.checkIdempotencyDuplicate(ctx, m)
		if idempotencyErr != nil {
			// We have an idempotency duplicate - we need to remove it from the batch so we can retry the rest
			dupRemoved = true
			if listener := batch.listeners[*m.Header.ID]; listener != nil {
				// Notify the listener for this message,
				listener <- idempotencyErr
			}
			delete(batch.listeners, *m.Header.ID)
			// Remove all the data associated with this message from the batch
			newData := make([]*core.Data, 0, len(batch.data))
			for _, d := range batch.data {
				isRef := false
				for _, dr := range m.Data {
					if dr.ID.Equals(d.ID) {
						isRef = true
						break
					}
				}
				if !isRef {
					newData = append(newData, d)
				}
			}
			batch.data = newData
		}
		if dupRemoved {
			duplicatesRemoved++
		} else {
			newMessageList = append(newMessageList, m)
		}
	}
	batch.messages = newMessageList
	return duplicatesRemoved
}

func (mw *messageWriter) writeMessages(ctx context.Context, msgs []*core.Message, data core.DataArray) error {
	if len(data) > 0 {
		if err := mw.database.InsertDataArray(ctx, data); err != nil {
			return err
		}
	}
	if len(msgs) > 0 {
		if err := mw.database.InsertMessages(ctx, msgs); err != nil {
			return err
		}
	}
	return nil
}

func (mw *messageWriter) close() {
	if !mw.closed {
		mw.closed = true
		mw.cancelFunc()
		for _, workerDone := range mw.workersDone {
			<-workerDone
		}
	}
}
