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

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/common/pkg/retry"
	"github.com/hyperledger-firefly/firefly/internal/broadcast"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/internal/data"
	"github.com/hyperledger-firefly/firefly/internal/privatemessaging"
	"github.com/hyperledger-firefly/firefly/internal/txcommon"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/hyperledger-firefly/firefly/pkg/events"
)

const (
	maxReadAhead        = 65536
	defaultBatchTimeout = time.Duration(2) * time.Second
)

type ackNack struct {
	id     fftypes.UUID
	isNack bool
	offset int64
}

type eventDispatcher struct {
	acksNacks     chan ackNack
	cancelCtx     func()
	closed        chan struct{}
	connID        string
	ctx           context.Context
	enricher      *eventEnricher
	data          data.Manager
	database      database.Plugin
	transport     events.Plugin
	broadcast     broadcast.Manager        // optional
	messaging     privatemessaging.Manager // optional
	elected       bool
	eventPoller   *eventPoller
	inflight      map[fftypes.UUID]*core.Event
	eventDelivery chan []*core.EventDelivery
	mux           sync.Mutex
	namespace     string
	readAhead     uint64
	batch         bool
	subscription  *subscription
	txHelper      txcommon.Helper
}

func newEventDispatcher(ctx context.Context, enricher *eventEnricher, ei events.Plugin, di database.Plugin, dm data.Manager, bm broadcast.Manager, pm privatemessaging.Manager, connID string, sub *subscription, en *eventNotifier, txHelper txcommon.Helper) *eventDispatcher {
	ctx, cancelCtx := context.WithCancel(ctx)
	readAhead := uint64(0)
	if sub.definition.Options.ReadAhead != nil {
		readAhead = uint64(*sub.definition.Options.ReadAhead)
	}

	batchTimeout := defaultBatchTimeout
	if sub.definition.Options.BatchTimeout != nil && *sub.definition.Options.BatchTimeout != "" {
		batchTimeout = fftypes.ParseToDuration(*sub.definition.Options.BatchTimeout)
	}
	batch := false
	if sub.definition.Options.Batch != nil {
		batch = *sub.definition.Options.Batch
	}
	ed := &eventDispatcher{
		ctx: log.WithLogFields(log.WithLogFields(ctx,
			"role", fmt.Sprintf("ed[%s]", connID)),
			"sub", fmt.Sprintf("%s/%s:%s", sub.definition.ID, sub.definition.Namespace, sub.definition.Name)),
		enricher:      enricher,
		database:      di,
		transport:     ei,
		broadcast:     bm,
		messaging:     pm,
		data:          dm,
		connID:        connID,
		cancelCtx:     cancelCtx,
		subscription:  sub,
		namespace:     sub.definition.Namespace,
		inflight:      make(map[fftypes.UUID]*core.Event),
		eventDelivery: make(chan []*core.EventDelivery, readAhead+1),
		readAhead:     readAhead,
		acksNacks:     make(chan ackNack),
		closed:        make(chan struct{}),
		txHelper:      txHelper,
		batch:         batch,
	}

	pollerConf := &eventPollerConf{
		eventBatchSize:             config.GetUint64(coreconfig.EventDispatcherBufferLength),
		eventBatchTimeout:          config.GetDuration(coreconfig.EventDispatcherBatchTimeout),
		eventPollTimeout:           config.GetDuration(coreconfig.EventDispatcherPollTimeout),
		startupOffsetRetryAttempts: 0, // We need to keep trying to start indefinitely
		retry: retry.Retry{
			InitialDelay: config.GetDuration(coreconfig.EventDispatcherRetryInitDelay),
			MaximumDelay: config.GetDuration(coreconfig.EventDispatcherRetryMaxDelay),
			Factor:       config.GetFloat64(coreconfig.EventDispatcherRetryFactor),
		},
		namespace:        sub.definition.Namespace,
		offsetType:       core.OffsetTypeSubscription,
		offsetName:       sub.definition.ID.String(),
		addCriteria:      func(af ffapi.AndFilter) ffapi.AndFilter { return af },
		queryFactory:     database.EventQueryFactory,
		getItems:         ed.getEvents,
		newEventsHandler: ed.bufferedDelivery,
		ephemeral:        sub.definition.Ephemeral,
		firstEvent:       sub.definition.Options.FirstEvent,
	}

	// Users can tune the batch related settings.
	// This is always true in batch:true cases, and optionally you can use the batchTimeout setting
	// to tweak how we optimize ourselves for readahead / latency detection without batching
	// (most likely users with this requirement would be best to just move to batch:true).
	if batchTimeout > 0 {
		pollerConf.eventBatchTimeout = batchTimeout
		if batchTimeout > pollerConf.eventPollTimeout {
			pollerConf.eventPollTimeout = batchTimeout
		}
	}
	if batch || pollerConf.eventBatchSize < readAhead {
		pollerConf.eventBatchSize = ed.readAhead
	}

	ed.eventPoller = newEventPoller(ctx, di, en, pollerConf)
	return ed
}

func (ed *eventDispatcher) start() {
	go ed.electAndStart()
}

func (ed *eventDispatcher) electAndStart() {
	defer close(ed.closed)
	l := log.L(ed.ctx)
	l.Debugf("Dispatcher attempting to become leader")
	select {
	case ed.subscription.dispatcherElection <- true:
		l.Debugf("Dispatcher became leader")
		defer func() {
			// Unelect ourselves on close, to let another dispatcher in
			<-ed.subscription.dispatcherElection
		}()
	case <-ed.ctx.Done():
		l.Debugf("Closed before we became leader")
		return
	}
	// We're ready to go
	ed.elected = true
	ed.eventPoller.start()

	go ed.deliverEvents()

	// Wait until the event poller closes
	<-ed.eventPoller.closed
}

func (ed *eventDispatcher) getEvents(ctx context.Context, filter ffapi.Filter, offset int64) ([]core.LocallySequenced, error) {
	log.L(ctx).Tracef("Reading page of events > %d (first events would be %d)", offset, offset+1)
	events, _, err := ed.database.GetEvents(ctx, ed.namespace, filter)
	ls := make([]core.LocallySequenced, len(events))
	for i, e := range events {
		ls[i] = e
	}
	return ls, err
}

func (ed *eventDispatcher) enrichEvents(events []core.LocallySequenced) ([]*core.EventDelivery, error) {
	enriched := make([]*core.EventDelivery, len(events))
	for i, ls := range events {
		e := ls.(*core.Event)
		enrichedEvent, err := ed.enricher.enrichEvent(ed.ctx, e)
		if err != nil {
			return nil, err
		}
		enriched[i] = &core.EventDelivery{
			EnrichedEvent: *enrichedEvent,
			Subscription:  ed.subscription.definition.SubscriptionRef,
		}
	}
	return enriched, nil
}

func (ed *eventDispatcher) filterEvents(candidates []*core.EventDelivery) []*core.EventDelivery {
	matchingEvents := make([]*core.EventDelivery, 0, len(candidates))
	for _, event := range candidates {
		if ed.subscription.MatchesEvent(&event.EnrichedEvent) {
			matchingEvents = append(matchingEvents, event)
		}
	}
	return matchingEvents
}

func (ed *eventDispatcher) bufferedDelivery(events []core.LocallySequenced) (bool, error) {
	// At this point, the page of messages we've been given are loaded from the DB into memory,
	// but we can only make them in-flight and push them to the client up to the maximum
	// readahead (which is likely lower than our page size - 1 by default)

	if len(events) == 0 {
		return false, nil
	}
	highestOffset := events[len(events)-1].LocalSequence()
	var lastAck int64
	var nacks int

	l := log.L(ed.ctx)
	candidates, err := ed.enrichEvents(events)
	if err != nil {
		return false, err
	}

	matching := ed.filterEvents(candidates)
	matchCount := len(matching)
	dispatched := 0

	// We stay here blocked until we've consumed all the messages in the buffer,
	// or a reset event happens
	for {
		ed.mux.Lock()
		var dispatchable []*core.EventDelivery
		inflightCount := uint64(len(ed.inflight))
		maxDispatch := 1 + ed.readAhead - inflightCount
		if maxDispatch >= uint64(len(matching)) {
			dispatchable = matching
			matching = nil
		} else if maxDispatch > 0 {
			dispatchable = matching[0:maxDispatch]
			matching = matching[maxDispatch:]
		}
		ed.mux.Unlock()

		l.Debugf("Dispatcher event state: readahead=%d candidates=%d matched=%d inflight=%d queued=%d dispatched=%d dispatchable=%d lastAck=%d nacks=%d highest=%d",
			ed.readAhead, len(candidates), matchCount, inflightCount, len(matching), dispatched, len(dispatchable), lastAck, nacks, highestOffset)

		for _, event := range dispatchable {
			ed.mux.Lock()
			ed.inflight[*event.ID] = &event.Event
			inflightCount = uint64(len(ed.inflight))
			ed.mux.Unlock()

			dispatched++
			if !ed.batch {
				// dispatch individually
				ed.eventDelivery <- []*core.EventDelivery{event}
			}
		}

		if ed.batch && len(dispatchable) > 0 {
			// Dispatch the whole batch now marked in-flight
			ed.eventDelivery <- dispatchable
		}

		if inflightCount == 0 {
			// We've cleared the decks. Time to look for more messages
			break
		}

		// Block until we're closed, or woken due to a delivery response
		select {
		case <-ed.ctx.Done():
			return false, i18n.NewError(ed.ctx, coremsgs.MsgDispatcherClosing)
		case an := <-ed.acksNacks:
			if an.isNack {
				nacks++
				ed.handleNackOffsetUpdate(an)
			} else if nacks == 0 {
				ed.handleAckOffsetUpdate(an)
				lastAck = an.offset
			}
		}
	}
	if nacks == 0 && lastAck != highestOffset {
		ed.eventPoller.commitOffset(highestOffset)
	}
	return true, nil // poll again straight away for more messages
}

func (ed *eventDispatcher) handleNackOffsetUpdate(nack ackNack) {
	ed.mux.Lock()
	defer ed.mux.Unlock()
	// If we're rejected, we need to redeliver all messages from this offset onwards,
	// even if we've delivered messages after that.
	// That means resetting the polling offest, and clearing out all our state
	delete(ed.inflight, nack.id)
	if ed.eventPoller.pollingOffset > nack.offset {
		ed.eventPoller.rewindPollingOffset(nack.offset - 1)
	}
	ed.inflight = map[fftypes.UUID]*core.Event{}
}

func (ed *eventDispatcher) handleAckOffsetUpdate(ack ackNack) {
	oldOffset := ed.eventPoller.getPollingOffset()
	ed.mux.Lock()
	delete(ed.inflight, ack.id)
	lowestInflight := int64(-1)
	for _, inflight := range ed.inflight {
		if lowestInflight < 0 || inflight.Sequence < lowestInflight {
			lowestInflight = inflight.Sequence
		}
	}
	ed.mux.Unlock()
	if (lowestInflight == -1 || lowestInflight > ack.offset) && ack.offset > oldOffset {
		// This was the lowest in flight, and we can move the offset forwards
		ed.eventPoller.commitOffset(ack.offset)
	}
}

func (ed *eventDispatcher) deliverEvents() {
	withData := ed.subscription.definition.Options.WithData != nil && *ed.subscription.definition.Options.WithData
	for {
		select {
		case events, ok := <-ed.eventDelivery:
			if !ok {
				return
			}

			// As soon as we hit an error, we need to trigger into nack mode
			var err error

			// Loop through the events enriching them, and dispatching individually in non-batch mode
			eventsWithData := make([]*core.CombinedEventDataDelivery, len(events))
			for i := 0; i < len(events); i++ {
				e := &core.CombinedEventDataDelivery{
					Event: events[i],
				}
				eventsWithData[i] = e
				// The first error we encounter stops us attempting to enrich or dispatch any more events
				if err == nil {
					log.L(ed.ctx).Debugf("Dispatching %s event: %.10d/%s [%s]: ref=%s/%s", ed.transport.Name(), e.Event.Sequence, e.Event.ID, e.Event.Type, e.Event.Namespace, e.Event.Reference)
					if withData && e.Event.Message != nil {
						e.Data, _, err = ed.data.GetMessageDataCached(ed.ctx, e.Event.Message)
					}
				}
				// If we are non-batched, we have to deliver each event individually...
				if !ed.batch {
					// .. only attempt to deliver if we've not triggered into an error scenario for one of the events already
					if err == nil {
						err = ed.transport.DeliveryRequest(ed.ctx, ed.connID, ed.subscription.definition, e.Event, e.Data)
					}
					// ... if we've triggered into an error scenario, we need to nack immediately for this and all the rest of the events
					if err != nil {
						ed.deliveryResponse(&core.EventDeliveryResponse{ID: e.Event.ID, Rejected: true})
					}
				}
			}

			// In batch mode we do one dispatch of the whole set as one
			if ed.batch {
				// Only attempt to deliver if we're in a non error case (enrich might have failed above)
				if err == nil {
					err = ed.transport.BatchDeliveryRequest(ed.ctx, ed.connID, ed.subscription.definition, eventsWithData)
				}
				// If we're in an error case we have to nack everything immediately
				if err != nil {
					for _, e := range events {
						ed.deliveryResponse(&core.EventDeliveryResponse{ID: e.Event.ID, Rejected: true})
					}
				}
			}

		case <-ed.ctx.Done():
			return
		}
	}
}

func (ed *eventDispatcher) deliveryResponse(response *core.EventDeliveryResponse) {
	l := log.L(ed.ctx)

	ed.mux.Lock()
	var an ackNack
	event, found := ed.inflight[*response.ID]
	if found {
		an.id = *response.ID
		an.offset = event.Sequence
		an.isNack = response.Rejected
	}
	ed.mux.Unlock()

	// Do some extra logging and persistent actions now we're out of lock
	if !found {
		l.Warnf("Response for event not in flight: %s rejected=%t info='%s' (likely previous reject)", response.ID, response.Rejected, response.Info)
		return
	}

	// We might have a message to send, do that before we dispatch the ack
	// Note a failure to send the reply does not invalidate the ack
	if response.Reply != nil {
		ed.sendReply(ed.ctx, event, response.Reply)
	}

	l.Debugf("Response for %s event: %.10d/%s [%s]: ref=%s/%s rejected=%t info='%s'", ed.transport.Name(), event.Sequence, event.ID, event.Type, event.Namespace, event.Reference, response.Rejected, response.Info)
	// We don't do any meaningful work in this call, we just set things up so the right thing
	// will happen when the poller wakes up. So we need to pass it over
	select {
	case ed.acksNacks <- an:
	case <-ed.ctx.Done():
		l.Debugf("Delivery response will not be delivered: closing")
		return
	}
}

func (ed *eventDispatcher) close() {
	log.L(ed.ctx).Infof("Dispatcher closing for conn=%s subscription=%s", ed.connID, ed.subscription.definition.ID)
	ed.cancelCtx()
	<-ed.closed
	if ed.elected {
		close(ed.eventDelivery)
		ed.elected = false
	}
}
