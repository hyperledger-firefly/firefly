// Copyright © 2021 Kaleido, Inc.
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
	"regexp"
	"testing"
	"time"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/cache"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/txcommon"
	"github.com/hyperledger-firefly/firefly/mocks/broadcastmocks"
	"github.com/hyperledger-firefly/firefly/mocks/cachemocks"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/mocks/datamocks"
	"github.com/hyperledger-firefly/firefly/mocks/eventsmocks"
	"github.com/hyperledger-firefly/firefly/mocks/operationmocks"
	"github.com/hyperledger-firefly/firefly/mocks/privatemessagingmocks"
	"github.com/hyperledger-firefly/firefly/mocks/syncasyncmocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/hyperledger-firefly/firefly/pkg/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func newTestEventDispatcher(sub *subscription) (*eventDispatcher, func()) {
	mdi := &databasemocks.Plugin{}
	mei := &eventsmocks.Plugin{}
	mei.On("Capabilities").Return(&events.Capabilities{}).Maybe()
	mei.On("Name").Return("ut").Maybe()
	mdm := &datamocks.Manager{}
	mbm := &broadcastmocks.Manager{}
	mpm := &privatemessagingmocks.Manager{}
	mom := &operationmocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := txcommon.NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)
	enricher := newEventEnricher("ns1", mdi, mdm, mom, txHelper)
	ctx, cancel := context.WithCancel(context.Background())
	return newEventDispatcher(ctx, enricher, mei, mdi, mdm, mbm, mpm, fftypes.NewUUID().String(), sub, newEventNotifier(ctx, "ut"), txHelper), func() {
		cancel()
		coreconfig.Reset()
	}
}

func TestEventDispatcherStartStop(t *testing.T) {
	ten := uint(10)
	oldest := core.SubOptsFirstEventOldest
	ed, cancel := newTestEventDispatcher(&subscription{
		dispatcherElection: make(chan bool, 1),
		definition: &core.Subscription{
			SubscriptionRef: core.SubscriptionRef{Namespace: "ns1", Name: "sub1"},
			Ephemeral:       true,
			Options: core.SubscriptionOptions{
				SubscriptionCoreOptions: core.SubscriptionCoreOptions{
					ReadAhead:  &ten,
					FirstEvent: &oldest,
				},
			},
		},
	})
	defer cancel()
	mdi := ed.database.(*databasemocks.Plugin)
	ge := mdi.On("GetEvents", mock.Anything, mock.Anything, mock.Anything).Return([]*core.Event{}, nil, nil)
	confirmedElected := make(chan bool)
	ge.RunFn = func(a mock.Arguments) {
		<-confirmedElected
	}

	assert.Equal(t, uint64(10), ed.readAhead)
	ed.start()
	confirmedElected <- true
	close(confirmedElected)
	ed.eventPoller.eventNotifier.newEvents <- 12345
	ed.close()
}

func TestEventDispatcherStartStopBatched(t *testing.T) {
	ten := uint(10)
	oldest := core.SubOptsFirstEventOldest
	truthy := true
	ed, cancel := newTestEventDispatcher(&subscription{
		dispatcherElection: make(chan bool, 1),
		definition: &core.Subscription{
			SubscriptionRef: core.SubscriptionRef{Namespace: "ns1", Name: "sub1"},
			Ephemeral:       true,
			Options: core.SubscriptionOptions{
				SubscriptionCoreOptions: core.SubscriptionCoreOptions{
					ReadAhead:  &ten,
					FirstEvent: &oldest,
					Batch:      &truthy,
				},
			},
		},
	})
	defer cancel()
	mdi := ed.database.(*databasemocks.Plugin)
	ge := mdi.On("GetEvents", mock.Anything, mock.Anything, mock.Anything).Return([]*core.Event{}, nil, fmt.Errorf("context closed"))
	confirmedElected := make(chan bool)
	ge.RunFn = func(a mock.Arguments) {
		<-confirmedElected
	}

	assert.Equal(t, uint64(10), ed.readAhead)
	ed.start()
	confirmedElected <- true
	close(confirmedElected)
	ed.eventPoller.eventNotifier.newEvents <- 12345
	ed.close()
}

func TestEventDispatcherLeaderElection(t *testing.T) {
	log.SetLevel("debug")

	subID := fftypes.NewUUID()
	sub := &subscription{
		dispatcherElection: make(chan bool, 1),
		definition: &core.Subscription{
			SubscriptionRef: core.SubscriptionRef{Namespace: "ns1", Name: "sub1", ID: subID},
		},
	}

	ed1, cancel1 := newTestEventDispatcher(sub)
	ed2, cancel2 := newTestEventDispatcher(sub /* same sub */)

	gev1Wait := make(chan bool)
	gev1Done := make(chan struct{})
	mdi1 := ed1.database.(*databasemocks.Plugin)
	gev1 := mdi1.On("GetEvents", mock.Anything, mock.Anything, mock.Anything).Return([]*core.Event{}, nil, nil)
	mdi1.On("GetOffset", mock.Anything, core.OffsetTypeSubscription, subID.String()).Return(&core.Offset{
		Type:    core.OffsetTypeSubscription,
		Name:    subID.String(),
		Current: 12345,
		RowID:   333333,
	}, nil)
	gev1.RunFn = func(a mock.Arguments) {
		gev1Wait <- true
		<-gev1Done
	}

	ed1.start()
	<-gev1Wait
	ed2.start()

	cancel2()
	ed2.close() // while ed1 is active
	close(gev1Done)
	cancel1()

}

func TestEventDispatcherReadAheadOutOfOrderAcks(t *testing.T) {
	log.SetLevel("debug")
	var five = uint(5)
	subID := fftypes.NewUUID()
	sub := &subscription{
		dispatcherElection: make(chan bool, 1),
		definition: &core.Subscription{
			SubscriptionRef: core.SubscriptionRef{ID: subID, Namespace: "ns1", Name: "sub1"},
			Options: core.SubscriptionOptions{
				SubscriptionCoreOptions: core.SubscriptionCoreOptions{
					ReadAhead: &five,
				},
			},
		},
		eventMatcher: regexp.MustCompile(fmt.Sprintf("^%s|%s$", core.EventTypeMessageConfirmed, core.EventTypeMessageConfirmed)),
	}

	ed, cancel := newTestEventDispatcher(sub)
	defer cancel()
	go ed.deliverEvents()
	ed.eventPoller.offsetCommitted = make(chan int64, 3)
	mdi := ed.database.(*databasemocks.Plugin)
	mei := ed.transport.(*eventsmocks.Plugin)
	mdm := ed.data.(*datamocks.Manager)

	eventDeliveries := make(chan *core.EventDelivery)
	deliveryRequestMock := mei.On("DeliveryRequest", ed.ctx, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	deliveryRequestMock.RunFn = func(a mock.Arguments) {
		eventDeliveries <- a.Get(3).(*core.EventDelivery)
	}

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()
	ref2 := fftypes.NewUUID()
	ev2 := fftypes.NewUUID()
	ref3 := fftypes.NewUUID()
	ev3 := fftypes.NewUUID()
	ref4 := fftypes.NewUUID()
	ev4 := fftypes.NewUUID()

	// Setup enrichment
	mdm.On("GetMessageWithDataCached", mock.Anything, ref1).Return(&core.Message{
		Header: core.MessageHeader{ID: ref1},
	}, nil, true, nil)
	mdm.On("GetMessageWithDataCached", mock.Anything, ref2).Return(&core.Message{
		Header: core.MessageHeader{ID: ref2},
	}, nil, true, nil)
	mdm.On("GetMessageWithDataCached", mock.Anything, ref3).Return(&core.Message{
		Header: core.MessageHeader{ID: ref3},
	}, nil, true, nil)
	mdm.On("GetMessageWithDataCached", mock.Anything, ref4).Return(&core.Message{
		Header: core.MessageHeader{ID: ref4},
	}, nil, true, nil)

	// Deliver a batch of messages
	batch1Done := make(chan struct{})
	go func() {
		repoll, err := ed.bufferedDelivery([]core.LocallySequenced{
			&core.Event{ID: ev1, Sequence: 10000001, Reference: ref1, Type: core.EventTypeMessageConfirmed}, // match
			&core.Event{ID: ev2, Sequence: 10000002, Reference: ref2, Type: core.EventTypeMessageRejected},
			&core.Event{ID: ev3, Sequence: 10000003, Reference: ref3, Type: core.EventTypeMessageConfirmed}, // match
			&core.Event{ID: ev4, Sequence: 10000004, Reference: ref4, Type: core.EventTypeMessageConfirmed}, // match
		})
		assert.NoError(t, err)
		assert.True(t, repoll)
		close(batch1Done)
	}()

	// Wait for the two calls to deliver the matching messages to the client (read ahead allows this)
	event1 := <-eventDeliveries
	assert.Equal(t, *ev1, *event1.ID)
	assert.Equal(t, *ref1, *event1.Message.Header.ID)
	event3 := <-eventDeliveries
	assert.Equal(t, *ev3, *event3.ID)
	assert.Equal(t, *ref3, *event3.Message.Header.ID)
	event4 := <-eventDeliveries
	assert.Equal(t, *ev4, *event4.ID)
	assert.Equal(t, *ref4, *event4.Message.Header.ID)

	// Send back the two acks - out of order to validate the read-ahead logic
	go func() {
		ed.deliveryResponse(&core.EventDeliveryResponse{ID: event4.ID})
		ed.deliveryResponse(&core.EventDeliveryResponse{ID: event1.ID})
		ed.deliveryResponse(&core.EventDeliveryResponse{ID: event3.ID})
	}()

	// Confirm we get the offset updates in the correct order, even though the confirmations
	// came in a different order from the app.
	assert.Equal(t, int64(10000001), <-ed.eventPoller.offsetCommitted)
	assert.Equal(t, int64(10000003), <-ed.eventPoller.offsetCommitted)
	assert.Equal(t, int64(10000004), <-ed.eventPoller.offsetCommitted)

	// This should complete the batch
	<-batch1Done

	mdi.AssertExpectations(t)
	mei.AssertExpectations(t)
	mdm.AssertExpectations(t)
}

func TestEventDispatcherNoReadAheadInOrder(t *testing.T) {
	log.SetLevel("debug")
	sub := &subscription{
		dispatcherElection: make(chan bool, 1),
		definition: &core.Subscription{
			SubscriptionRef: core.SubscriptionRef{ID: fftypes.NewUUID(), Namespace: "ns1", Name: "sub1"},
			Ephemeral:       true,
			Options:         core.SubscriptionOptions{},
		},
	}

	ed, cancel := newTestEventDispatcher(sub)
	defer cancel()
	go ed.deliverEvents()

	mdi := ed.database.(*databasemocks.Plugin)
	mdm := ed.data.(*datamocks.Manager)
	mei := ed.transport.(*eventsmocks.Plugin)

	eventDeliveries := make(chan *core.EventDelivery)
	deliveryRequestMock := mei.On("DeliveryRequest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	deliveryRequestMock.RunFn = func(a mock.Arguments) {
		eventDeliveries <- a.Get(3).(*core.EventDelivery)
	}

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()
	ref2 := fftypes.NewUUID()
	ev2 := fftypes.NewUUID()
	ref3 := fftypes.NewUUID()
	ev3 := fftypes.NewUUID()
	ref4 := fftypes.NewUUID()
	ev4 := fftypes.NewUUID()

	// Setup enrichment
	mdm.On("GetMessageWithDataCached", mock.Anything, ref1).Return(&core.Message{
		Header: core.MessageHeader{ID: ref1},
	}, nil, true, nil)
	mdm.On("GetMessageWithDataCached", mock.Anything, ref2).Return(&core.Message{
		Header: core.MessageHeader{ID: ref2},
	}, nil, true, nil)
	mdm.On("GetMessageWithDataCached", mock.Anything, ref3).Return(&core.Message{
		Header: core.MessageHeader{ID: ref3},
	}, nil, true, nil)
	mdm.On("GetMessageWithDataCached", mock.Anything, ref4).Return(&core.Message{
		Header: core.MessageHeader{ID: ref4},
	}, nil, true, nil)

	// Deliver a batch of messages
	batch1Done := make(chan struct{})
	go func() {
		repoll, err := ed.bufferedDelivery([]core.LocallySequenced{
			&core.Event{ID: ev1, Sequence: 10000001, Reference: ref1, Type: core.EventTypeMessageConfirmed}, // match
			&core.Event{ID: ev2, Sequence: 10000002, Reference: ref2, Type: core.EventTypeMessageConfirmed}, // match
			&core.Event{ID: ev3, Sequence: 10000003, Reference: ref3, Type: core.EventTypeMessageConfirmed}, // match
			&core.Event{ID: ev4, Sequence: 10000004, Reference: ref4, Type: core.EventTypeMessageConfirmed}, // match
		})
		assert.NoError(t, err)
		assert.True(t, repoll)
		close(batch1Done)
	}()

	// Wait for the two calls to deliver the matching messages to the client (read ahead allows this)
	event1 := <-eventDeliveries
	assert.Equal(t, *ev1, *event1.ID)
	assert.Equal(t, *ref1, *event1.Message.Header.ID)
	select {
	case <-eventDeliveries:
		assert.Fail(t, "should not have read ahead")
	default:
	}
	ed.deliveryResponse(&core.EventDeliveryResponse{ID: event1.ID})

	event2 := <-eventDeliveries
	ed.deliveryResponse(&core.EventDeliveryResponse{ID: event2.ID})

	event3 := <-eventDeliveries
	ed.deliveryResponse(&core.EventDeliveryResponse{ID: event3.ID})

	event4 := <-eventDeliveries
	ed.deliveryResponse(&core.EventDeliveryResponse{ID: event4.ID})

	// This should complete the batch
	<-batch1Done

	mdi.AssertExpectations(t)
	mei.AssertExpectations(t)
	mdm.AssertExpectations(t)
}

func TestEventDispatcherBatchBased(t *testing.T) {
	log.SetLevel("debug")
	three := uint(3)
	longTime := "1m"
	subID := fftypes.NewUUID()
	truthy := true
	sub := &subscription{
		dispatcherElection: make(chan bool, 1),
		definition: &core.Subscription{
			SubscriptionRef: core.SubscriptionRef{ID: subID, Namespace: "ns1", Name: "sub1"},
			Options: core.SubscriptionOptions{
				SubscriptionCoreOptions: core.SubscriptionCoreOptions{
					Batch:        &truthy,
					ReadAhead:    &three,
					BatchTimeout: &longTime, // because the batch should fill
				},
			},
		},
		eventMatcher: regexp.MustCompile(fmt.Sprintf("^%s|%s$", core.EventTypeMessageConfirmed, core.EventTypeMessageConfirmed)),
	}

	ed, cancel := newTestEventDispatcher(sub)
	defer cancel()
	go ed.deliverEvents()
	ed.eventPoller.offsetCommitted = make(chan int64, 3)
	mdi := ed.database.(*databasemocks.Plugin)
	mei := ed.transport.(*eventsmocks.Plugin)
	mdm := ed.data.(*datamocks.Manager)

	eventDeliveries := make(chan []*core.CombinedEventDataDelivery)
	deliveryRequestMock := mei.On("BatchDeliveryRequest", ed.ctx, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	deliveryRequestMock.RunFn = func(a mock.Arguments) {
		eventDeliveries <- a.Get(3).([]*core.CombinedEventDataDelivery)
	}

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()
	ref2 := fftypes.NewUUID()
	ev2 := fftypes.NewUUID()
	ref3 := fftypes.NewUUID()
	ev3 := fftypes.NewUUID()
	ref4 := fftypes.NewUUID()
	ev4 := fftypes.NewUUID()

	// Setup enrichment
	mdm.On("GetMessageWithDataCached", mock.Anything, ref1).Return(&core.Message{
		Header: core.MessageHeader{ID: ref1},
	}, nil, true, nil)
	mdm.On("GetMessageWithDataCached", mock.Anything, ref2).Return(&core.Message{
		Header: core.MessageHeader{ID: ref2},
	}, nil, true, nil)
	mdm.On("GetMessageWithDataCached", mock.Anything, ref3).Return(&core.Message{
		Header: core.MessageHeader{ID: ref3},
	}, nil, true, nil)
	mdm.On("GetMessageWithDataCached", mock.Anything, ref4).Return(&core.Message{
		Header: core.MessageHeader{ID: ref4},
	}, nil, true, nil)

	// Deliver a batch of messages
	batch1Done := make(chan struct{})
	go func() {
		repoll, err := ed.bufferedDelivery([]core.LocallySequenced{
			&core.Event{ID: ev1, Sequence: 10000001, Reference: ref1, Type: core.EventTypeMessageConfirmed}, // match
			&core.Event{ID: ev2, Sequence: 10000002, Reference: ref2, Type: core.EventTypeMessageRejected},
			&core.Event{ID: ev3, Sequence: 10000003, Reference: ref3, Type: core.EventTypeMessageConfirmed}, // match
			&core.Event{ID: ev4, Sequence: 10000004, Reference: ref4, Type: core.EventTypeMessageConfirmed}, // match
		})
		assert.NoError(t, err)
		assert.True(t, repoll)
		close(batch1Done)
	}()

	// Expect to get the batch dispatched - with the three matching events
	events := <-eventDeliveries
	assert.Len(t, events, 3)
	assert.Equal(t, *ev1, *events[0].Event.ID)
	assert.Equal(t, *ref1, *events[0].Event.Message.Header.ID)
	assert.Equal(t, *ev3, *events[1].Event.ID)
	assert.Equal(t, *ref3, *events[1].Event.Message.Header.ID)
	assert.Equal(t, *ev4, *events[2].Event.ID)
	assert.Equal(t, *ref4, *events[2].Event.Message.Header.ID)

	// Ack the batch
	go func() {
		ed.deliveryResponse(&core.EventDeliveryResponse{ID: events[0].Event.ID})
		ed.deliveryResponse(&core.EventDeliveryResponse{ID: events[1].Event.ID})
		ed.deliveryResponse(&core.EventDeliveryResponse{ID: events[2].Event.ID})
	}()

	assert.Equal(t, int64(10000001), <-ed.eventPoller.offsetCommitted)
	assert.Equal(t, int64(10000003), <-ed.eventPoller.offsetCommitted)
	assert.Equal(t, int64(10000004), <-ed.eventPoller.offsetCommitted)

	// This should complete the batch
	<-batch1Done

	mdi.AssertExpectations(t)
	mei.AssertExpectations(t)
	mdm.AssertExpectations(t)
}

func TestEventDispatcherBatchDispatchFail(t *testing.T) {
	log.SetLevel("debug")
	two := uint(2)
	longTime := "1m"
	subID := fftypes.NewUUID()
	truthy := true
	sub := &subscription{
		dispatcherElection: make(chan bool, 1),
		definition: &core.Subscription{
			SubscriptionRef: core.SubscriptionRef{ID: subID, Namespace: "ns1", Name: "sub1"},
			Options: core.SubscriptionOptions{
				SubscriptionCoreOptions: core.SubscriptionCoreOptions{
					Batch:        &truthy,
					ReadAhead:    &two,
					BatchTimeout: &longTime, // because the batch should fill
				},
			},
		},
		eventMatcher: regexp.MustCompile(fmt.Sprintf("^%s|%s$", core.EventTypeMessageConfirmed, core.EventTypeMessageConfirmed)),
	}

	ed, cancel := newTestEventDispatcher(sub)
	defer cancel()
	go ed.deliverEvents()
	ed.eventPoller.offsetCommitted = make(chan int64, 3)
	mdi := ed.database.(*databasemocks.Plugin)
	mei := ed.transport.(*eventsmocks.Plugin)
	mdm := ed.data.(*datamocks.Manager)

	eventDeliveries := make(chan []*core.CombinedEventDataDelivery)
	deliveryRequestMock := mei.On("BatchDeliveryRequest", ed.ctx, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
	deliveryRequestMock.RunFn = func(a mock.Arguments) {
		eventDeliveries <- a.Get(3).([]*core.CombinedEventDataDelivery)
	}

	// Deliver a batch of messages
	batch1Done := make(chan struct{})
	go func() {
		repoll, err := ed.bufferedDelivery([]core.LocallySequenced{
			&core.Event{ID: fftypes.NewUUID(), Sequence: 10000001, Type: core.EventTypeMessageConfirmed},
			&core.Event{ID: fftypes.NewUUID(), Sequence: 10000002, Type: core.EventTypeMessageConfirmed},
		})
		assert.NoError(t, err)
		assert.True(t, repoll)
		close(batch1Done)
	}()

	mdm.On("GetMessageWithDataCached", mock.Anything, mock.Anything).Return(&core.Message{
		Header: core.MessageHeader{ID: fftypes.NewUUID()},
	}, nil, true, nil)

	// Expect to get the batch dispatched - with the three matching events
	events := <-eventDeliveries
	assert.Len(t, events, 2)

	// This should complete the batch
	<-batch1Done

	mdi.AssertExpectations(t)
	mei.AssertExpectations(t)
	mdm.AssertExpectations(t)
}

func TestEnrichEventsFailGetMessages(t *testing.T) {

	sub := &subscription{
		definition: &core.Subscription{},
	}
	ed, cancel := newTestEventDispatcher(sub)
	defer cancel()

	mdm := ed.data.(*datamocks.Manager)
	mdm.On("GetMessageWithDataCached", mock.Anything, mock.Anything).Return(nil, nil, false, fmt.Errorf("pop"))

	id1 := fftypes.NewUUID()
	_, err := ed.enrichEvents([]core.LocallySequenced{&core.Event{ID: id1, Type: core.EventTypeMessageConfirmed}})

	assert.EqualError(t, err, "pop")
}

func TestEnrichEventsFailGetTransactions(t *testing.T) {

	sub := &subscription{
		definition: &core.Subscription{},
	}
	ed, cancel := newTestEventDispatcher(sub)
	defer cancel()

	mdi := ed.database.(*databasemocks.Plugin)
	mdi.On("GetTransactionByID", mock.Anything, "ns1", mock.Anything).Return(nil, fmt.Errorf("pop"))

	id1 := fftypes.NewUUID()
	_, err := ed.enrichEvents([]core.LocallySequenced{&core.Event{ID: id1, Type: core.EventTypeTransactionSubmitted}})

	assert.EqualError(t, err, "pop")
}

func TestEnrichEventsFailGetBlockchainEvents(t *testing.T) {

	sub := &subscription{
		definition: &core.Subscription{},
	}
	ed, cancel := newTestEventDispatcher(sub)
	defer cancel()

	mdi := ed.database.(*databasemocks.Plugin)
	mdi.On("GetBlockchainEventByID", mock.Anything, "ns1", mock.Anything).Return(nil, fmt.Errorf("pop"))

	id1 := fftypes.NewUUID()
	_, err := ed.enrichEvents([]core.LocallySequenced{&core.Event{ID: id1, Type: core.EventTypeBlockchainEventReceived}})

	assert.EqualError(t, err, "pop")
}

func TestFilterEventsMatch(t *testing.T) {

	sub := &subscription{
		definition:        &core.Subscription{},
		messageFilter:     &messageFilter{},
		transactionFilter: &transactionFilter{},
		blockchainFilter:  &blockchainFilter{},
	}
	ed, cancel := newTestEventDispatcher(sub)
	defer cancel()

	gid1 := fftypes.NewRandB32()
	id1 := fftypes.NewUUID()
	id2 := fftypes.NewUUID()
	id3 := fftypes.NewUUID()
	id4 := fftypes.NewUUID()
	id5 := fftypes.NewUUID()
	id6 := fftypes.NewUUID()
	lid := fftypes.NewUUID()
	events := ed.filterEvents([]*core.EventDelivery{
		{
			EnrichedEvent: core.EnrichedEvent{
				Event: core.Event{
					ID:    id1,
					Type:  core.EventTypeMessageConfirmed,
					Topic: "topic1",
				},
				Message: &core.Message{
					Header: core.MessageHeader{
						Topics: fftypes.FFStringArray{"topic1"},
						Tag:    "tag1",
						Group:  nil,
						SignerRef: core.SignerRef{
							Author: "signingOrg",
							Key:    "0x12345",
						},
					},
				},
			},
		},
		{
			EnrichedEvent: core.EnrichedEvent{
				Event: core.Event{
					ID:    id2,
					Type:  core.EventTypeMessageConfirmed,
					Topic: "topic1",
				},
				Message: &core.Message{
					Header: core.MessageHeader{
						Topics: fftypes.FFStringArray{"topic1"},
						Tag:    "tag2",
						Group:  gid1,
						SignerRef: core.SignerRef{
							Author: "org2",
							Key:    "0x23456",
						},
					},
				},
			},
		},
		{
			EnrichedEvent: core.EnrichedEvent{
				Event: core.Event{
					ID:    id3,
					Type:  core.EventTypeMessageRejected,
					Topic: "topic2",
				},
				Message: &core.Message{
					Header: core.MessageHeader{
						Topics: fftypes.FFStringArray{"topic2"},
						Tag:    "tag1",
						Group:  nil,
						SignerRef: core.SignerRef{
							Author: "signingOrg",
							Key:    "0x12345",
						},
					},
				},
			},
		},
		{
			EnrichedEvent: core.EnrichedEvent{
				Event: core.Event{
					ID:   id4,
					Type: core.EventTypeBlockchainEventReceived,
				},
				BlockchainEvent: &core.BlockchainEvent{
					Name: "flapflip",
				},
			},
		},
		{
			EnrichedEvent: core.EnrichedEvent{
				Event: core.Event{
					ID:   id5,
					Type: core.EventTypeTransactionSubmitted,
				},
				Transaction: &core.Transaction{
					Type: core.TransactionTypeBatchPin,
				},
			},
		},
		{
			EnrichedEvent: core.EnrichedEvent{
				Event: core.Event{
					ID:   id6,
					Type: core.EventTypeBlockchainEventReceived,
				},
				BlockchainEvent: &core.BlockchainEvent{
					Listener: lid,
				},
			},
		},
	})

	ed.subscription.eventMatcher = regexp.MustCompile(fmt.Sprintf("^%s$", core.EventTypeMessageConfirmed))
	ed.subscription.topicFilter = regexp.MustCompile(".*")
	ed.subscription.messageFilter.tagFilter = regexp.MustCompile(".*")
	ed.subscription.messageFilter.groupFilter = regexp.MustCompile(".*")
	matched := ed.filterEvents(events)
	assert.Equal(t, 2, len(matched))
	assert.Equal(t, *id1, *matched[0].ID)
	assert.Equal(t, *id2, *matched[1].ID)
	// id three has the wrong event type

	ed.subscription.eventMatcher = nil
	ed.subscription.topicFilter = nil
	ed.subscription.messageFilter.tagFilter = nil
	ed.subscription.messageFilter.groupFilter = nil
	matched = ed.filterEvents(events)
	assert.Equal(t, 6, len(matched))
	assert.Equal(t, *id1, *matched[0].ID)
	assert.Equal(t, *id2, *matched[1].ID)
	assert.Equal(t, *id3, *matched[2].ID)
	assert.Equal(t, *id4, *matched[3].ID)
	assert.Equal(t, *id5, *matched[4].ID)

	ed.subscription.topicFilter = regexp.MustCompile("topic1")
	matched = ed.filterEvents(events)
	assert.Equal(t, 2, len(matched))
	assert.Equal(t, *id1, *matched[0].ID)
	assert.Equal(t, *id2, *matched[1].ID)

	ed.subscription.topicFilter = nil
	ed.subscription.messageFilter.tagFilter = regexp.MustCompile("tag2")
	matched = ed.filterEvents(events)
	assert.Equal(t, 1, len(matched))
	assert.Equal(t, *id2, *matched[0].ID)

	ed.subscription.topicFilter = nil
	ed.subscription.messageFilter.authorFilter = nil
	ed.subscription.messageFilter.groupFilter = regexp.MustCompile(gid1.String())
	matched = ed.filterEvents(events)
	assert.Equal(t, 1, len(matched))
	assert.Equal(t, *id2, *matched[0].ID)

	ed.subscription.messageFilter.groupFilter = regexp.MustCompile("^$")
	matched = ed.filterEvents(events)
	assert.Equal(t, 0, len(matched))

	ed.subscription.messageFilter.groupFilter = nil
	ed.subscription.topicFilter = nil
	ed.subscription.messageFilter.tagFilter = nil
	ed.subscription.messageFilter.authorFilter = regexp.MustCompile("org2")
	matched = ed.filterEvents(events)
	assert.Equal(t, 1, len(matched))
	assert.Equal(t, *id2, *matched[0].ID)

	ed.subscription.messageFilter = nil
	ed.subscription.transactionFilter.typeFilter = regexp.MustCompile(fmt.Sprintf("^%s$", core.TransactionTypeBatchPin))
	matched = ed.filterEvents(events)
	assert.Equal(t, 1, len(matched))
	assert.Equal(t, *id5, *matched[0].ID)

	ed.subscription.messageFilter = nil
	ed.subscription.transactionFilter = nil
	ed.subscription.blockchainFilter.nameFilter = regexp.MustCompile("flapflip")
	matched = ed.filterEvents(events)
	assert.Equal(t, 1, len(matched))
	assert.Equal(t, *id4, *matched[0].ID)

	ed.subscription.messageFilter = nil
	ed.subscription.transactionFilter = nil
	ed.subscription.blockchainFilter.nameFilter = nil
	ed.subscription.blockchainFilter.listenerFilter = regexp.MustCompile(lid.String())
	matched = ed.filterEvents(events)
	assert.Equal(t, 1, len(matched))
	assert.Equal(t, *id6, *matched[0].ID)
}

func TestEnrichTransactionEvents(t *testing.T) {
	log.SetLevel("debug")
	sub := &subscription{
		dispatcherElection: make(chan bool, 1),
		definition: &core.Subscription{
			SubscriptionRef: core.SubscriptionRef{ID: fftypes.NewUUID(), Namespace: "ns1", Name: "sub1"},
			Ephemeral:       true,
			Options:         core.SubscriptionOptions{},
		},
	}

	ed, cancel := newTestEventDispatcher(sub)
	defer cancel()
	go ed.deliverEvents()

	mdi := ed.database.(*databasemocks.Plugin)
	mei := ed.transport.(*eventsmocks.Plugin)

	eventDeliveries := make(chan *core.EventDelivery)
	deliveryRequestMock := mei.On("DeliveryRequest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	deliveryRequestMock.RunFn = func(a mock.Arguments) {
		eventDeliveries <- a.Get(3).(*core.EventDelivery)
	}

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()
	ref2 := fftypes.NewUUID()
	ev2 := fftypes.NewUUID()
	ref3 := fftypes.NewUUID()
	ev3 := fftypes.NewUUID()
	ref4 := fftypes.NewUUID()
	ev4 := fftypes.NewUUID()

	// Setup enrichment
	mdi.On("GetTransactionByID", mock.Anything, "ns1", ref1).Return(&core.Transaction{
		ID: ref1,
	}, nil)
	mdi.On("GetTransactionByID", mock.Anything, "ns1", ref2).Return(&core.Transaction{
		ID: ref2,
	}, nil)
	mdi.On("GetTransactionByID", mock.Anything, "ns1", ref3).Return(&core.Transaction{
		ID: ref3,
	}, nil)
	mdi.On("GetTransactionByID", mock.Anything, "ns1", ref4).Return(&core.Transaction{
		ID: ref4,
	}, nil)

	// Deliver a batch of messages
	batch1Done := make(chan struct{})
	go func() {
		repoll, err := ed.bufferedDelivery([]core.LocallySequenced{
			&core.Event{ID: ev1, Sequence: 10000001, Reference: ref1, Type: core.EventTypeTransactionSubmitted}, // match
			&core.Event{ID: ev2, Sequence: 10000002, Reference: ref2, Type: core.EventTypeTransactionSubmitted}, // match
			&core.Event{ID: ev3, Sequence: 10000003, Reference: ref3, Type: core.EventTypeTransactionSubmitted}, // match
			&core.Event{ID: ev4, Sequence: 10000004, Reference: ref4, Type: core.EventTypeTransactionSubmitted}, // match
		})
		assert.NoError(t, err)
		assert.True(t, repoll)
		close(batch1Done)
	}()

	// Wait for the two calls to deliver the matching messages to the client (read ahead allows this)
	event1 := <-eventDeliveries
	assert.Equal(t, *ev1, *event1.ID)
	assert.Equal(t, *ref1, *event1.Transaction.ID)
	select {
	case <-eventDeliveries:
		assert.Fail(t, "should not have read ahead")
	default:
	}
	ed.deliveryResponse(&core.EventDeliveryResponse{ID: event1.ID})

	event2 := <-eventDeliveries
	ed.deliveryResponse(&core.EventDeliveryResponse{ID: event2.ID})

	event3 := <-eventDeliveries
	ed.deliveryResponse(&core.EventDeliveryResponse{ID: event3.ID})

	event4 := <-eventDeliveries
	ed.deliveryResponse(&core.EventDeliveryResponse{ID: event4.ID})

	// This should complete the batch
	<-batch1Done

	mdi.AssertExpectations(t)
	mei.AssertExpectations(t)
}

func TestEnrichBlockchainEventEvents(t *testing.T) {
	log.SetLevel("debug")
	sub := &subscription{
		dispatcherElection: make(chan bool, 1),
		definition: &core.Subscription{
			SubscriptionRef: core.SubscriptionRef{ID: fftypes.NewUUID(), Namespace: "ns1", Name: "sub1"},
			Ephemeral:       true,
			Options:         core.SubscriptionOptions{},
		},
	}

	ed, cancel := newTestEventDispatcher(sub)
	defer cancel()
	go ed.deliverEvents()

	mdi := ed.database.(*databasemocks.Plugin)
	mei := ed.transport.(*eventsmocks.Plugin)

	eventDeliveries := make(chan *core.EventDelivery)
	deliveryRequestMock := mei.On("DeliveryRequest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	deliveryRequestMock.RunFn = func(a mock.Arguments) {
		eventDeliveries <- a.Get(3).(*core.EventDelivery)
	}

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()
	ref2 := fftypes.NewUUID()
	ev2 := fftypes.NewUUID()
	ref3 := fftypes.NewUUID()
	ev3 := fftypes.NewUUID()
	ref4 := fftypes.NewUUID()
	ev4 := fftypes.NewUUID()

	// Setup enrichment
	mdi.On("GetBlockchainEventByID", mock.Anything, "ns1", ref1).Return(&core.BlockchainEvent{
		ID: ref1,
	}, nil)
	mdi.On("GetBlockchainEventByID", mock.Anything, "ns1", ref2).Return(&core.BlockchainEvent{
		ID: ref2,
	}, nil)
	mdi.On("GetBlockchainEventByID", mock.Anything, "ns1", ref3).Return(&core.BlockchainEvent{
		ID: ref3,
	}, nil)
	mdi.On("GetBlockchainEventByID", mock.Anything, "ns1", ref4).Return(&core.BlockchainEvent{
		ID: ref4,
	}, nil)

	// Deliver a batch of messages
	batch1Done := make(chan struct{})
	go func() {
		repoll, err := ed.bufferedDelivery([]core.LocallySequenced{
			&core.Event{ID: ev1, Sequence: 10000001, Reference: ref1, Type: core.EventTypeBlockchainEventReceived}, // match
			&core.Event{ID: ev2, Sequence: 10000002, Reference: ref2, Type: core.EventTypeBlockchainEventReceived}, // match
			&core.Event{ID: ev3, Sequence: 10000003, Reference: ref3, Type: core.EventTypeBlockchainEventReceived}, // match
			&core.Event{ID: ev4, Sequence: 10000004, Reference: ref4, Type: core.EventTypeBlockchainEventReceived}, // match
		})
		assert.NoError(t, err)
		assert.True(t, repoll)
		close(batch1Done)
	}()

	// Wait for the two calls to deliver the matching messages to the client (read ahead allows this)
	event1 := <-eventDeliveries
	assert.Equal(t, *ev1, *event1.ID)
	assert.Equal(t, *ref1, *event1.BlockchainEvent.ID)
	select {
	case <-eventDeliveries:
		assert.Fail(t, "should not have read ahead")
	default:
	}
	ed.deliveryResponse(&core.EventDeliveryResponse{ID: event1.ID})

	event2 := <-eventDeliveries
	ed.deliveryResponse(&core.EventDeliveryResponse{ID: event2.ID})

	event3 := <-eventDeliveries
	ed.deliveryResponse(&core.EventDeliveryResponse{ID: event3.ID})

	event4 := <-eventDeliveries
	ed.deliveryResponse(&core.EventDeliveryResponse{ID: event4.ID})

	// This should complete the batch
	<-batch1Done

	mdi.AssertExpectations(t)
	mei.AssertExpectations(t)
}

func TestBufferedDeliveryNoEvents(t *testing.T) {

	sub := &subscription{
		definition:        &core.Subscription{},
		messageFilter:     &messageFilter{},
		transactionFilter: &transactionFilter{},
		blockchainFilter:  &blockchainFilter{},
	}
	ed, cancel := newTestEventDispatcher(sub)
	defer cancel()

	repoll, err := ed.bufferedDelivery([]core.LocallySequenced{})
	assert.False(t, repoll)
	assert.Nil(t, err)

}

func TestBufferedDeliveryEnrichFail(t *testing.T) {

	sub := &subscription{
		definition:        &core.Subscription{},
		messageFilter:     &messageFilter{},
		transactionFilter: &transactionFilter{},
		blockchainFilter:  &blockchainFilter{},
	}
	ed, cancel := newTestEventDispatcher(sub)
	defer cancel()

	mdm := ed.data.(*datamocks.Manager)
	mdm.On("GetMessageWithDataCached", mock.Anything, mock.Anything).Return(nil, nil, false, fmt.Errorf("pop"))

	repoll, err := ed.bufferedDelivery([]core.LocallySequenced{&core.Event{ID: fftypes.NewUUID(), Type: core.EventTypeMessageConfirmed}})
	assert.False(t, repoll)
	assert.EqualError(t, err, "pop")

}

func TestBufferedDeliveryClosedContext(t *testing.T) {

	sub := &subscription{
		messageFilter:     &messageFilter{},
		transactionFilter: &transactionFilter{},
		blockchainFilter:  &blockchainFilter{},
		definition:        &core.Subscription{},
	}
	ed, cancel := newTestEventDispatcher(sub)
	go ed.deliverEvents()
	cancel()

	mdi := ed.database.(*databasemocks.Plugin)
	mei := ed.transport.(*eventsmocks.Plugin)
	mdi.On("GetDataRefs", mock.Anything, mock.Anything).Return(nil, nil, nil)
	mei.On("DeliveryRequest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	repoll, err := ed.bufferedDelivery([]core.LocallySequenced{&core.Event{ID: fftypes.NewUUID()}})
	assert.False(t, repoll)
	assert.Regexp(t, "FF10182", err)

}

func TestBufferedDeliveryNackRewind(t *testing.T) {

	sub := &subscription{
		definition: &core.Subscription{},
	}
	ed, cancel := newTestEventDispatcher(sub)
	defer cancel()
	go ed.deliverEvents()

	mdi := ed.database.(*databasemocks.Plugin)
	mei := ed.transport.(*eventsmocks.Plugin)
	mdi.On("GetDataRefs", mock.Anything, mock.Anything).Return(nil, nil, nil)
	mdi.On("UpdateOffset", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)

	delivered := make(chan struct{})
	deliver := mei.On("DeliveryRequest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	deliver.RunFn = func(a mock.Arguments) {
		close(delivered)
	}

	bdDone := make(chan struct{})
	ev1 := fftypes.NewUUID()
	ed.eventPoller.pollingOffset = 100050 // ahead of nack
	go func() {
		repoll, err := ed.bufferedDelivery([]core.LocallySequenced{&core.Event{ID: ev1, Sequence: 100001}})
		assert.NoError(t, err)
		assert.True(t, repoll)
		close(bdDone)
	}()

	<-delivered
	ed.deliveryResponse(&core.EventDeliveryResponse{
		ID:       ev1,
		Rejected: true,
	})

	<-bdDone
	assert.Equal(t, int64(100000), ed.eventPoller.pollingOffset)
}

func TestBufferedDeliveryFailNack(t *testing.T) {
	log.SetLevel("trace")

	sub := &subscription{
		definition:        &core.Subscription{},
		messageFilter:     &messageFilter{},
		transactionFilter: &transactionFilter{},
		blockchainFilter:  &blockchainFilter{},
	}
	ed, cancel := newTestEventDispatcher(sub)
	defer cancel()
	go ed.deliverEvents()
	ed.readAhead = 50

	mdi := ed.database.(*databasemocks.Plugin)
	mei := ed.transport.(*eventsmocks.Plugin)
	mdi.On("GetDataRefs", mock.Anything, mock.Anything).Return(nil, nil, nil)
	mdi.On("UpdateOffset", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))

	failNacked := make(chan bool)
	deliver := mei.On("DeliveryRequest", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
	deliver.RunFn = func(a mock.Arguments) {
		failNacked <- true
	}

	bdDone := make(chan struct{})
	ev1 := fftypes.NewUUID()
	ev2 := fftypes.NewUUID()
	ed.eventPoller.pollingOffset = 100000
	go func() {
		repoll, err := ed.bufferedDelivery([]core.LocallySequenced{
			&core.Event{ID: ev1, Sequence: 100001},
			&core.Event{ID: ev2, Sequence: 100002},
		})
		assert.NoError(t, err)
		assert.True(t, repoll)
		close(bdDone)
	}()

	<-failNacked
	<-failNacked

	<-bdDone
	assert.Equal(t, int64(100000), ed.eventPoller.pollingOffset)

}

func TestAckNotInFlightNoop(t *testing.T) {

	sub := &subscription{
		definition: &core.Subscription{},
	}
	ed, cancel := newTestEventDispatcher(sub)
	defer cancel()

	ed.deliveryResponse(&core.EventDeliveryResponse{ID: fftypes.NewUUID()})
}

func TestEventDeliveryClosed(t *testing.T) {

	sub := &subscription{
		definition: &core.Subscription{},
	}
	ed, cancel := newTestEventDispatcher(sub)
	close(ed.eventDelivery)

	ed.deliverEvents()
	cancel()
}

func TestAckClosed(t *testing.T) {

	sub := &subscription{
		definition: &core.Subscription{},
	}
	ed, cancel := newTestEventDispatcher(sub)
	cancel()

	id1 := fftypes.NewUUID()
	ed.inflight[*id1] = &core.Event{ID: id1}
	ed.deliveryResponse(&core.EventDeliveryResponse{ID: id1})
}

func TestGetEvents(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	sub := &subscription{
		definition: &core.Subscription{
			SubscriptionRef: core.SubscriptionRef{
				Namespace: "ns1",
			},
		},
	}

	ed, cancel := newTestEventDispatcher(sub)
	cancel()

	mdi := ed.database.(*databasemocks.Plugin)

	mdi.On("GetEvents", ag.ctx, "ns1", mock.Anything).Return([]*core.Event{
		{Sequence: 12345},
	}, nil, nil)

	lc, err := ed.getEvents(ag.ctx, database.EventQueryFactory.NewFilter(ag.ctx).Gte("sequence", 12345), 12345)
	assert.NoError(t, err)
	assert.Equal(t, int64(12345), lc[0].LocalSequence())
}

func TestDeliverEventsWithDataFail(t *testing.T) {
	yes := true
	sub := &subscription{
		definition: &core.Subscription{
			Options: core.SubscriptionOptions{
				SubscriptionCoreOptions: core.SubscriptionCoreOptions{
					WithData: &yes,
				},
			},
		},
	}

	ed, cancel := newTestEventDispatcher(sub)
	defer cancel()

	mdm := ed.data.(*datamocks.Manager)
	mdm.On("GetMessageDataCached", ed.ctx, mock.Anything).Return(nil, false, fmt.Errorf("pop"))

	id1 := fftypes.NewUUID()
	ed.eventDelivery <- []*core.EventDelivery{
		{
			EnrichedEvent: core.EnrichedEvent{
				Event: core.Event{
					ID: id1,
				},
				Message: &core.Message{
					Header: core.MessageHeader{
						ID: fftypes.NewUUID(),
					},
					Data: core.DataRefs{
						{ID: fftypes.NewUUID()},
					},
				},
			},
		},
	}

	ed.inflight[*id1] = &core.Event{ID: id1}
	go ed.deliverEvents()

	an := <-ed.acksNacks
	assert.True(t, an.isNack)

}

func TestEventDispatcherWithReply(t *testing.T) {
	log.SetLevel("debug")
	var two = uint(5)
	sub := &subscription{
		dispatcherElection: make(chan bool, 1),
		definition: &core.Subscription{
			SubscriptionRef: core.SubscriptionRef{ID: fftypes.NewUUID(), Namespace: "ns1", Name: "sub1"},
			Options: core.SubscriptionOptions{
				SubscriptionCoreOptions: core.SubscriptionCoreOptions{
					ReadAhead: &two,
				},
			},
		},
		eventMatcher: regexp.MustCompile(fmt.Sprintf("^%s|%s$", core.EventTypeMessageConfirmed, core.EventTypeMessageConfirmed)),
	}

	ed, cancel := newTestEventDispatcher(sub)
	cancel()
	ed.acksNacks = make(chan ackNack, 2)

	event1 := fftypes.NewUUID()
	ed.inflight[*event1] = &core.Event{
		ID:        event1,
		Namespace: "ns1",
	}

	mms := &syncasyncmocks.Sender{}
	mbm := ed.broadcast.(*broadcastmocks.Manager)
	mbm.On("NewBroadcast", mock.Anything).Return(mms)
	mms.On("Send", mock.Anything).Return(nil)

	ed.deliveryResponse(&core.EventDeliveryResponse{
		ID: event1,
		Reply: &core.MessageInOut{
			Message: core.Message{
				Header: core.MessageHeader{
					Tag:  "myreplytag1",
					CID:  fftypes.NewUUID(),
					Type: core.MessageTypeBroadcast,
				},
			},
			InlineData: core.InlineData{
				{Value: fftypes.JSONAnyPtr(`"my reply"`)},
			},
		},
	})

	mbm.AssertExpectations(t)
	mms.AssertExpectations(t)
}

func TestEventDeliveryBatch(t *testing.T) {
	log.SetLevel("debug")
	var five = uint(5)
	truthy := true
	sub := &subscription{
		dispatcherElection: make(chan bool, 1),
		definition: &core.Subscription{
			SubscriptionRef: core.SubscriptionRef{ID: fftypes.NewUUID(), Namespace: "ns1", Name: "sub1"},
			Options: core.SubscriptionOptions{
				SubscriptionCoreOptions: core.SubscriptionCoreOptions{
					ReadAhead: &five,
					Batch:     &truthy,
				},
			},
		},
		eventMatcher: regexp.MustCompile(fmt.Sprintf("^%s|%s$", core.EventTypeMessageConfirmed, core.EventTypeMessageConfirmed)),
	}

	ed, cancel := newTestEventDispatcher(sub)
	cancel()
	ed.acksNacks = make(chan ackNack, 5)

	event1 := fftypes.NewUUID()
	ed.inflight[*event1] = &core.Event{
		ID:        event1,
		Namespace: "ns1",
	}

	mms := &syncasyncmocks.Sender{}
	mbm := ed.broadcast.(*broadcastmocks.Manager)
	mbm.On("NewBroadcast", mock.Anything).Return(mms)
	mms.On("Send", mock.Anything).Return(nil)

	ed.deliveryResponse(&core.EventDeliveryResponse{
		ID: event1,
		Reply: &core.MessageInOut{
			Message: core.Message{
				Header: core.MessageHeader{
					Tag:  "myreplytag1",
					CID:  fftypes.NewUUID(),
					Type: core.MessageTypeBroadcast,
				},
			},
			InlineData: core.InlineData{
				{Value: fftypes.JSONAnyPtr(`"my reply"`)},
			},
		},
	})

	mbm.AssertExpectations(t)
	mms.AssertExpectations(t)
}
