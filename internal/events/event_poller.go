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
	"fmt"
	"sync"
	"time"

	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/common/pkg/retry"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

type eventPoller struct {
	ctx             context.Context
	database        database.Plugin
	shoulderTaps    chan bool
	eventNotifier   *eventNotifier
	closed          chan struct{}
	offsetCommitted chan int64
	offsetID        int64
	pollingOffset   int64
	mux             sync.Mutex
	conf            *eventPollerConf
}

type newEventsHandler func(events []core.LocallySequenced) (bool, error)

type eventPollerConf struct {
	ephemeral                  bool
	eventBatchSize             uint64
	eventBatchTimeout          time.Duration
	eventPollTimeout           time.Duration
	firstEvent                 *core.SubOptsFirstEvent
	queryFactory               ffapi.QueryFactory
	addCriteria                func(ffapi.AndFilter) ffapi.AndFilter
	getItems                   func(context.Context, ffapi.Filter, int64) ([]core.LocallySequenced, error)
	maybeRewind                func() (bool, int64)
	newEventsHandler           newEventsHandler
	namespace                  string
	offsetName                 string
	offsetType                 core.OffsetType
	retry                      retry.Retry
	startupOffsetRetryAttempts int
}

func newEventPoller(ctx context.Context, di database.Plugin, en *eventNotifier, conf *eventPollerConf) *eventPoller {
	ep := &eventPoller{
		ctx:             log.WithLogFields(ctx, "role", fmt.Sprintf("ep[%s:%s]", conf.namespace, conf.offsetName)),
		database:        di,
		shoulderTaps:    make(chan bool, 1),
		offsetCommitted: make(chan int64, 1),
		eventNotifier:   en,
		closed:          make(chan struct{}),
		conf:            conf,
	}
	if ep.conf.maybeRewind == nil {
		ep.conf.maybeRewind = func() (bool, int64) { return false, -1 }
	}
	return ep
}

func (ep *eventPoller) restoreOffset() error {
	return ep.conf.retry.Do(ep.ctx, "restore offset", func(attempt int) (retry bool, err error) {
		retry = ep.conf.startupOffsetRetryAttempts == 0 || attempt <= ep.conf.startupOffsetRetryAttempts
		var offset *core.Offset
		if ep.conf.ephemeral {
			ep.pollingOffset, err = calcFirstOffset(ep.ctx, ep.conf.namespace, ep.database, ep.conf.firstEvent)
			return retry, err
		}
		for offset == nil {
			offset, err = ep.database.GetOffset(ep.ctx, ep.conf.offsetType, ep.conf.offsetName)
			if err != nil {
				return retry, err
			}
			if offset == nil {
				firstOffset, err := calcFirstOffset(ep.ctx, ep.conf.namespace, ep.database, ep.conf.firstEvent)
				if err != nil {
					return retry, err
				}
				err = ep.database.UpsertOffset(ep.ctx, &core.Offset{
					Type:    ep.conf.offsetType,
					Name:    ep.conf.offsetName,
					Current: firstOffset,
				}, false)
				if err != nil {
					return retry, err
				}
			}
		}
		ep.offsetID = offset.RowID
		ep.pollingOffset = offset.Current
		log.L(ep.ctx).Infof("Event offset restored %d", ep.pollingOffset)
		return false, nil
	})
}

func (ep *eventPoller) start() {
	err := ep.conf.retry.Do(ep.ctx, "restore offset", func(attempt int) (retry bool, err error) {
		return true, ep.restoreOffset()
	})
	if err != nil {
		log.L(ep.ctx).Errorf("Event poller context closed before we successfully restored offset: %s", err)
		close(ep.closed)
		return
	}
	go ep.newEventNotifications()
	go ep.eventLoop()
	go ep.offsetCommitLoop()
}

func (ep *eventPoller) rewindPollingOffset(offset int64) int64 {
	log.L(ep.ctx).Infof("Event polling rewind to: %d", offset)
	ep.mux.Lock()
	defer ep.mux.Unlock()
	if offset < ep.pollingOffset {
		ep.pollingOffset = offset
	}
	return ep.pollingOffset
}

func (ep *eventPoller) getPollingOffset() int64 {
	ep.mux.Lock()
	defer ep.mux.Unlock()
	return ep.pollingOffset
}

func (ep *eventPoller) commitOffset(offset int64) {
	// Next polling cycle should start one higher than this offset
	ep.mux.Lock()
	ep.pollingOffset = offset
	ep.mux.Unlock()

	// No persistence for ephemeral (non-durable) subscriptions
	if !ep.conf.ephemeral {
		// We do this in the background, as it is an expensive full DB commit
		select {
		case ep.offsetCommitted <- offset:
		default:
		}
	}
}

func (ep *eventPoller) readPage() ([]core.LocallySequenced, error) {

	var items []core.LocallySequenced

	// We have a hook here to allow a safe to do operations that check pin state, and perform
	// a rewind based on it.
	rewind, pollingOffset := ep.conf.maybeRewind()
	if rewind {
		pollingOffset = ep.rewindPollingOffset(pollingOffset)
	} else {
		// Ensure we go through the mutex to pickup rewinds that happened elsewhere
		pollingOffset = ep.getPollingOffset()
	}

	err := ep.conf.retry.Do(ep.ctx, "retrieve events", func(attempt int) (retry bool, err error) {
		fb := ep.conf.queryFactory.NewFilter(ep.ctx)
		filter := fb.And(
			fb.Gt("sequence", pollingOffset),
		)
		filter = ep.conf.addCriteria(filter)
		items, err = ep.conf.getItems(ep.ctx, filter.Sort("sequence").Limit(ep.conf.eventBatchSize), pollingOffset)
		if err != nil {
			return true, err // Retry indefinitely, until context cancelled
		}
		return false, nil
	})
	return items, err
}

func (ep *eventPoller) eventLoop() {
	l := log.L(ep.ctx)
	l.Debugf("Started event detector")
	defer func() {
		close(ep.closed)
		close(ep.offsetCommitted)
	}()

	doBatchDelay := false
	for {
		if doBatchDelay {
			ep.waitForBatchTimeout()
		}

		// Read messages from the DB - in an error condition we retry until success, or a closed context
		events, err := ep.readPage()
		if err != nil {
			l.Debugf("Exiting: %s", err)
			return
		}

		eventCount := uint64(len(events))

		// We might want to wait for the batch to fill - so we delay and re-poll
		if ep.conf.eventBatchTimeout > 0 && !doBatchDelay && eventCount < ep.conf.eventBatchSize {
			doBatchDelay = true
			l.Tracef("Batch delay: detected=%d, batchSize=%d batchTimeout=%s", eventCount, ep.conf.eventBatchSize, ep.conf.eventBatchTimeout)
			continue
		}
		doBatchDelay = false // clear any batch delay for next iteration

		repoll := false
		if eventCount > 0 {
			// We process all the events in the page in a single database run group, and
			// keep retrying on all retryable errors, indefinitely ().
			var err error
			repoll, err = ep.dispatchEventsRetry(events)
			if err != nil {
				l.Debugf("Exiting: %s", err)
				return
			}
		}

		// Once we run out of events, wait to be woken
		if !repoll {
			if ok := ep.waitForShoulderTapOrPollTimeout(eventCount); !ok {
				return
			}
		}
	}
}

func (ep *eventPoller) offsetCommitLoop() {
	l := log.L(ep.ctx)
	for range ep.offsetCommitted {
		_ = ep.conf.retry.Do(ep.ctx, "process events", func(attempt int) (retry bool, err error) {
			ep.mux.Lock()
			pollingOffset := ep.pollingOffset
			ep.mux.Unlock()
			u := database.OffsetQueryFactory.NewUpdate(ep.ctx).Set("current", pollingOffset)
			if err := ep.database.UpdateOffset(ep.ctx, ep.offsetID, u); err != nil {
				return true, err
			}
			l.Debugf("Event polling offset committed %d", pollingOffset)
			return false, nil
		})
	}
}

func (ep *eventPoller) dispatchEventsRetry(events []core.LocallySequenced) (repoll bool, err error) {
	err = ep.conf.retry.Do(ep.ctx, "process events", func(attempt int) (retry bool, err error) {
		repoll, err = ep.conf.newEventsHandler(events)
		return err != nil, err // always retry (retry will end on cancelled context)
	})
	return repoll, err
}

// newEventNotifications just consumes new events, logs them, then ensures there's a shoulderTap
// in the channel - without blocking. This is important as we must not block the notifier
// - which might be our own eventLoop
func (ep *eventPoller) newEventNotifications() {
	defer close(ep.shoulderTaps)
	var lastNotified int64 = -1
	for {
		latestSequence := ep.getPollingOffset()
		if latestSequence <= lastNotified {
			latestSequence = lastNotified + 1
		}
		err := ep.eventNotifier.waitNext(latestSequence)
		if err != nil {
			log.L(ep.ctx).Debugf("event notifier closing")
			return
		}
		ep.shoulderTap()
		lastNotified = latestSequence
	}
}

func (ep *eventPoller) shoulderTap() {
	// Do not block sending to the shoulderTap - as it can only contain one
	select {
	case ep.shoulderTaps <- true:
	default:
	}
}

func (ep *eventPoller) waitForBatchTimeout() {
	// For throughput optimized environments, we can set an eventBatchingTimeout to allow
	// dispatching of incomplete batches at a shorter timeout than the
	// long timeout between polling cycles (at the cost of some dispatch latency)
	select {
	case <-time.After(ep.conf.eventBatchTimeout):
	case <-ep.ctx.Done():
	}
}

func (ep *eventPoller) waitForShoulderTapOrPollTimeout(lastEventCount uint64) bool {
	l := log.L(ep.ctx)
	longTimeoutDuration := ep.conf.eventPollTimeout

	if lastEventCount >= ep.conf.eventBatchSize {
		l.Tracef("Polling immediately due to full previous event batch")
		return true
	}

	longTimeout := time.NewTimer(longTimeoutDuration)
	select {
	case <-longTimeout.C:
		l.Debugf("Woken after poll timeout")
	case <-ep.shoulderTaps:
		l.Debug("Woken for trigger on event")
	case <-ep.ctx.Done():
		l.Debugf("Exiting due to cancelled context")
		return false
	}
	return true
}
