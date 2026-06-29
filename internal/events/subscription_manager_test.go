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
	"crypto/tls"
	"fmt"
	"testing"
	"time"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/ffresty"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
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
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func newTestSubManager(t *testing.T, mei *eventsmocks.Plugin) (*subscriptionManager, func()) {
	coreconfig.Reset()
	config.Set(coreconfig.EventTransportsEnabled, []string{})

	mdi := &databasemocks.Plugin{}
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
	mei.On("Name").Return("ut")
	mei.On("Capabilities").Return(&events.Capabilities{}).Maybe()
	mei.On("InitConfig", mock.Anything).Return()
	mei.On("Init", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mdi.On("GetEvents", mock.Anything, mock.Anything, mock.Anything).Return([]*core.Event{}, nil, nil).Maybe()
	mdi.On("GetOffset", mock.Anything, mock.Anything, mock.Anything).Return(&core.Offset{RowID: 3333333, Current: 0}, nil).Maybe()
	sm, err := newSubscriptionManager(ctx, &core.Namespace{Name: "ns1"}, enricher, mdi, mdm, newEventNotifier(ctx, "ut"), mbm, mpm, txHelper, nil)
	assert.NoError(t, err)
	sm.transports = map[string]events.Plugin{
		"ut": mei,
	}
	return sm, cancel
}

func TestRegisterDurableSubscriptions(t *testing.T) {

	sub1 := fftypes.NewUUID()
	sub2 := fftypes.NewUUID()

	// Set some existing ones to be cleaned out
	testED1, cancel1 := newTestEventDispatcher(&subscription{definition: &core.Subscription{SubscriptionRef: core.SubscriptionRef{ID: sub1}}})
	testED1.start()
	defer cancel1()

	mei := testED1.transport.(*eventsmocks.Plugin)
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()

	mdi := sm.database.(*databasemocks.Plugin)
	mdi.On("GetSubscriptions", mock.Anything, "ns1", mock.Anything).Return([]*core.Subscription{
		{SubscriptionRef: core.SubscriptionRef{
			ID: sub1,
		}, Transport: "ut"},
		{SubscriptionRef: core.SubscriptionRef{
			ID: sub2,
		}, Transport: "ut"},
	}, nil, nil)
	mei.On("ValidateOptions", mock.Anything, mock.Anything).Return(nil)
	err := sm.start()
	assert.NoError(t, err)

	sm.connections["conn1"] = &connection{
		ei:        mei,
		id:        "conn1",
		transport: "ut",
		dispatchers: map[fftypes.UUID]*eventDispatcher{
			*sub1: testED1,
		},
	}
	be := &boundCallbacks{sm: sm, ei: mei}

	be.RegisterConnection("conn1", func(sr core.SubscriptionRef) bool {
		return *sr.ID == *sub2
	})
	be.RegisterConnection("conn2", func(sr core.SubscriptionRef) bool {
		return *sr.ID == *sub1
	})

	assert.Equal(t, 1, len(sm.connections["conn1"].dispatchers))
	assert.Equal(t, *sub2, *sm.connections["conn1"].dispatchers[*sub2].subscription.definition.ID)
	assert.Equal(t, 1, len(sm.connections["conn2"].dispatchers))
	assert.Equal(t, *sub1, *sm.connections["conn2"].dispatchers[*sub1].subscription.definition.ID)

	// Close with active conns
	sm.close()
	assert.Nil(t, sm.connections["conn1"])
	assert.Nil(t, sm.connections["conn2"])
}

func TestReloadDurableSubscription(t *testing.T) {

	sub1 := fftypes.NewUUID()

	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()

	sm.connections["conn1"] = &connection{
		ei:          mei,
		id:          "conn1",
		transport:   "ut",
		dispatchers: make(map[fftypes.UUID]*eventDispatcher),
		matcher: func(sr core.SubscriptionRef) bool {
			return sr.Namespace == "ns1" && sr.Name == "sub1"
		},
	}

	mdi := sm.database.(*databasemocks.Plugin)
	mdi.On("GetSubscriptions", mock.Anything, "ns1", mock.Anything).Return([]*core.Subscription{
		{SubscriptionRef: core.SubscriptionRef{
			ID:        sub1,
			Namespace: "ns1",
			Name:      "sub1",
		}, Transport: "ut"},
	}, nil, nil)
	mei.On("ValidateOptions", mock.Anything, mock.Anything).Return(nil)
	err := sm.start()
	assert.NoError(t, err)

	// Close with active conns
	sm.close()
	assert.Nil(t, sm.connections["conn1"])
	assert.Nil(t, sm.connections["conn2"])
}

func TestRegisterEphemeralSubscriptions(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mdi := sm.database.(*databasemocks.Plugin)

	mdi.On("GetSubscriptions", mock.Anything, "ns1", mock.Anything).Return([]*core.Subscription{}, nil, nil)
	mei.On("ValidateOptions", mock.Anything, mock.Anything).Return(nil)

	err := sm.start()
	assert.NoError(t, err)
	be := &boundCallbacks{sm: sm, ei: mei}

	// check with filter
	err = be.EphemeralSubscription("conn1", "ns1", &core.SubscriptionFilter{Message: core.MessageFilter{Author: "flapflip"}}, &core.SubscriptionOptions{})
	assert.NoError(t, err)

	assert.Equal(t, 1, len(sm.connections["conn1"].dispatchers))
	for _, d := range sm.connections["conn1"].dispatchers {
		assert.True(t, d.subscription.definition.Ephemeral)
	}

	be.ConnectionClosed("conn1")
	assert.Nil(t, sm.connections["conn1"])
	// Check we swallow dup closes without errors
	be.ConnectionClosed("conn1")
	assert.Nil(t, sm.connections["conn1"])
}

func TestRegisterEphemeralSubscriptionsFail(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mdi := sm.database.(*databasemocks.Plugin)

	mdi.On("GetSubscriptions", mock.Anything, "ns1", mock.Anything).Return([]*core.Subscription{}, nil, nil)
	mei.On("ValidateOptions", mock.Anything, mock.Anything).Return(nil)
	err := sm.start()
	assert.NoError(t, err)
	be := &boundCallbacks{sm: sm, ei: mei}

	err = be.EphemeralSubscription("conn1", "ns1", &core.SubscriptionFilter{
		Message: core.MessageFilter{
			Author: "[[[[[ !wrong",
		},
	}, &core.SubscriptionOptions{})
	assert.Regexp(t, "FF10171", err)
	assert.Empty(t, sm.connections["conn1"].dispatchers)

}

func TestStartSubRestoreFail(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mdi := sm.database.(*databasemocks.Plugin)

	mdi.On("GetSubscriptions", mock.Anything, "ns1", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))
	err := sm.start()
	assert.EqualError(t, err, "pop")
}

func TestStartSubRestoreOkSubsFail(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mdi := sm.database.(*databasemocks.Plugin)

	mdi.On("GetSubscriptions", mock.Anything, "ns1", mock.Anything).Return([]*core.Subscription{
		{SubscriptionRef: core.SubscriptionRef{
			ID: fftypes.NewUUID(),
		},
			Filter: core.SubscriptionFilter{
				Events: "[[[[[[not a regex",
			}},
	}, nil, nil)
	err := sm.start()
	assert.NoError(t, err) // swallowed and startup continues
}

func TestStartSubRestoreOkSubsOK(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mdi := sm.database.(*databasemocks.Plugin)

	mdi.On("GetSubscriptions", mock.Anything, "ns1", mock.Anything).Return([]*core.Subscription{
		{SubscriptionRef: core.SubscriptionRef{
			ID: fftypes.NewUUID(),
		},
			Filter: core.SubscriptionFilter{
				Topic:  ".*",
				Events: ".*",
				Message: core.MessageFilter{
					Tag:    ".*",
					Group:  ".*",
					Author: ".*",
				},
				Transaction: core.TransactionFilter{
					Type: ".*",
				},
				BlockchainEvent: core.BlockchainEventFilter{
					Name: ".*",
				},
			}},
	}, nil, nil)
	err := sm.start()
	assert.NoError(t, err) // swallowed and startup continues
}

func TestCreateSubscriptionBadTransport(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	_, err := sm.parseSubscriptionDef(sm.ctx, &core.Subscription{})
	assert.Regexp(t, "FF1017", err)
}

func TestCreateSubscriptionBadTransportOptions(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	sub := &core.Subscription{
		Transport: "ut",
		Options:   core.SubscriptionOptions{},
	}
	sub.Options.TransportOptions()["myoption"] = "badvalue"
	mei.On("ValidateOptions", mock.Anything, mock.MatchedBy(func(opts *core.SubscriptionOptions) bool {
		return opts.TransportOptions()["myoption"] == "badvalue"
	})).Return(fmt.Errorf("pop"))
	_, err := sm.parseSubscriptionDef(sm.ctx, sub)
	assert.Regexp(t, "pop", err)
}

func TestCreateSubscriptionBadEventilter(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mei.On("ValidateOptions", mock.Anything, mock.Anything).Return(nil)
	_, err := sm.parseSubscriptionDef(sm.ctx, &core.Subscription{
		Filter: core.SubscriptionFilter{
			Events: "[[[[! badness",
		},
		Transport: "ut",
	})
	assert.Regexp(t, "FF10171.*events", err)
}

func TestCreateSubscriptionBadTopicFilter(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mei.On("ValidateOptions", mock.Anything, mock.Anything).Return(nil)
	_, err := sm.parseSubscriptionDef(sm.ctx, &core.Subscription{
		Filter: core.SubscriptionFilter{
			Topic: "[[[[! badness",
		},
		Transport: "ut",
	})
	assert.Regexp(t, "FF10171.*topic", err)
}

func TestCreateSubscriptionBadGroupFilter(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mei.On("ValidateOptions", mock.Anything, mock.Anything).Return(nil)
	_, err := sm.parseSubscriptionDef(sm.ctx, &core.Subscription{
		Filter: core.SubscriptionFilter{
			Message: core.MessageFilter{
				Group: "[[[[! badness",
			},
		},
		Transport: "ut",
	})
	assert.Regexp(t, "FF10171.*group", err)
}

func TestCreateSubscriptionBadAuthorFilter(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mei.On("ValidateOptions", mock.Anything, mock.Anything).Return(nil)
	_, err := sm.parseSubscriptionDef(sm.ctx, &core.Subscription{
		Filter: core.SubscriptionFilter{
			Message: core.MessageFilter{
				Author: "[[[[! badness",
			},
		},
		Transport: "ut",
	})
	assert.Regexp(t, "FF10171.*author", err)
}

func TestCreateSubscriptionBadTxTypeFilter(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mei.On("ValidateOptions", mock.Anything, mock.Anything).Return(nil)
	_, err := sm.parseSubscriptionDef(sm.ctx, &core.Subscription{
		Filter: core.SubscriptionFilter{
			Transaction: core.TransactionFilter{
				Type: "[[[[! badness",
			},
		},
		Transport: "ut",
	})
	assert.Regexp(t, "FF10171.*type", err)
}

func TestCreateSubscriptionBadBlockchainEventNameFilter(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mei.On("ValidateOptions", mock.Anything, mock.Anything).Return(nil)
	_, err := sm.parseSubscriptionDef(sm.ctx, &core.Subscription{
		Filter: core.SubscriptionFilter{
			BlockchainEvent: core.BlockchainEventFilter{
				Name: "[[[[! badness",
			},
		},
		Transport: "ut",
	})
	assert.Regexp(t, "FF10171.*name", err)
}

func TestCreateSubscriptionBadDeprecatedGroupFilter(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mei.On("ValidateOptions", mock.Anything, mock.Anything).Return(nil)
	_, err := sm.parseSubscriptionDef(sm.ctx, &core.Subscription{
		Filter: core.SubscriptionFilter{
			DeprecatedGroup: "[[[[! badness",
		},
		Transport: "ut",
	})
	assert.Regexp(t, "FF10171.*group", err)
}

func TestCreateSubscriptionBadDeprecatedTagFilter(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mei.On("ValidateOptions", mock.Anything, mock.Anything).Return(nil)
	_, err := sm.parseSubscriptionDef(sm.ctx, &core.Subscription{
		Filter: core.SubscriptionFilter{
			DeprecatedTag: "[[[[! badness",
		},
		Transport: "ut",
	})
	assert.Regexp(t, "FF10171.*tag", err)
}

func TestCreateSubscriptionBadMessageTagFilter(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mei.On("ValidateOptions", mock.Anything, mock.Anything).Return(nil)
	_, err := sm.parseSubscriptionDef(sm.ctx, &core.Subscription{
		Filter: core.SubscriptionFilter{
			Message: core.MessageFilter{
				Tag: "[[[[! badness",
			},
		},
		Transport: "ut",
	})
	assert.Regexp(t, "FF10171.*message.tag", err)
}

func TestCreateSubscriptionBadDeprecatedAuthorFilter(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mei.On("ValidateOptions", mock.Anything, mock.Anything).Return(nil)
	_, err := sm.parseSubscriptionDef(sm.ctx, &core.Subscription{
		Filter: core.SubscriptionFilter{
			DeprecatedAuthor: "[[[[! badness",
		},
		Transport: "ut",
	})
	assert.Regexp(t, "FF10171.*author", err)
}

func TestCreateSubscriptionBadDeprecatedTopicsFilter(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mei.On("ValidateOptions", mock.Anything, mock.Anything).Return(nil)
	_, err := sm.parseSubscriptionDef(sm.ctx, &core.Subscription{
		Filter: core.SubscriptionFilter{
			DeprecatedTopics: "[[[[! badness",
		},
		Transport: "ut",
	})
	assert.Regexp(t, "FF10171.*topics", err)
}

func TestCreateSubscriptionBadBlockchainEventListenerFilter(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mei.On("ValidateOptions", mock.Anything, mock.Anything).Return(nil)
	_, err := sm.parseSubscriptionDef(sm.ctx, &core.Subscription{
		Filter: core.SubscriptionFilter{
			BlockchainEvent: core.BlockchainEventFilter{
				Listener: "[[[[! badness",
			},
		},
		Transport: "ut",
	})
	assert.Regexp(t, "FF10171.*listener", err)
}

func TestCreateSubscriptionSuccessMessageFilter(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mei.On("ValidateOptions", mock.Anything, mock.Anything).Return(nil)
	_, err := sm.parseSubscriptionDef(sm.ctx, &core.Subscription{
		Filter: core.SubscriptionFilter{
			Message: core.MessageFilter{
				Author: "flapflip",
			},
		},
		Transport: "ut",
	})
	assert.NoError(t, err)
}

func TestCreateSubscriptionSuccessTxFilter(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mei.On("ValidateOptions", mock.Anything, mock.Anything).Return(nil)
	_, err := sm.parseSubscriptionDef(sm.ctx, &core.Subscription{
		Filter: core.SubscriptionFilter{
			Transaction: core.TransactionFilter{
				Type: "flapflip",
			},
		},
		Transport: "ut",
	})
	assert.NoError(t, err)
}

func TestCreateSubscriptionSuccessBlockchainEvent(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mei.On("ValidateOptions", mock.Anything, mock.Anything).Return(nil)
	_, err := sm.parseSubscriptionDef(sm.ctx, &core.Subscription{
		Filter: core.SubscriptionFilter{
			BlockchainEvent: core.BlockchainEventFilter{
				Name: "flapflip",
			},
		},
		Transport: "ut",
	})
	assert.NoError(t, err)
}

func TestCreateSubscriptionSuccessTLSConfig(t *testing.T) {
	coreconfig.Reset()

	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()

	sm.namespace.TLSConfigs = map[string]*tls.Config{
		"myconfig": {},
	}

	mei.On("GetFFRestyConfig", mock.Anything).Return(&ffresty.Config{})
	mei.On("ValidateOptions", mock.Anything, mock.Anything).Return(nil)
	sub, err := sm.parseSubscriptionDef(sm.ctx, &core.Subscription{
		Options: core.SubscriptionOptions{
			WebhookSubOptions: core.WebhookSubOptions{
				TLSConfigName: "myconfig",
			},
		},
		Transport: "ut",
	})
	assert.NoError(t, err)

	assert.NotNil(t, sub.definition.Options.TLSConfig)
}

func TestCreateSubscriptionSuccessBatch(t *testing.T) {
	coreconfig.Reset()

	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()

	mei.On("GetFFRestyConfig", mock.Anything).Return(&ffresty.Config{})
	mei.On("ValidateOptions", mock.Anything, mock.Anything).Return(nil)
	truthy := true
	sub, err := sm.parseSubscriptionDef(sm.ctx, &core.Subscription{
		Options: core.SubscriptionOptions{
			SubscriptionCoreOptions: core.SubscriptionCoreOptions{
				Batch: &truthy,
			},
		},
		Transport: "ut",
	})
	assert.NoError(t, err)

	assert.Equal(t, uint(50), *sub.definition.Options.ReadAhead)
	assert.Equal(t, "50ms", *sub.definition.Options.BatchTimeout)
}

func TestCreateSubscriptionWithDeprecatedFilters(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mei.On("ValidateOptions", mock.Anything, mock.Anything).Return(nil)
	_, err := sm.parseSubscriptionDef(sm.ctx, &core.Subscription{
		Filter: core.SubscriptionFilter{
			Topic:            "flop",
			DeprecatedTopics: "test",
			DeprecatedTag:    "flap",
			DeprecatedAuthor: "flip",
			DeprecatedGroup:  "flapflip",
		},
		Transport: "ut",
	})
	assert.NoError(t, err)

}

func TestDispatchDeliveryResponseOK(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mdi := sm.database.(*databasemocks.Plugin)
	mdi.On("GetSubscriptions", mock.Anything, "ns1", mock.Anything).Return([]*core.Subscription{}, nil, nil)
	mei.On("ValidateOptions", mock.Anything, mock.Anything).Return(nil)
	err := sm.start()
	assert.NoError(t, err)
	be := &boundCallbacks{sm: sm, ei: mei}

	err = be.EphemeralSubscription("conn1", "ns1", &core.SubscriptionFilter{}, &core.SubscriptionOptions{})
	assert.NoError(t, err)

	assert.Equal(t, 1, len(sm.connections["conn1"].dispatchers))
	var subID *fftypes.UUID
	for _, d := range sm.connections["conn1"].dispatchers {
		assert.True(t, d.subscription.definition.Ephemeral)
		subID = d.subscription.definition.ID
	}

	be.DeliveryResponse("conn1", &core.EventDeliveryResponse{
		ID: fftypes.NewUUID(), // Won't be in-flight, but that's fine
		Subscription: core.SubscriptionRef{
			ID: subID,
		},
	})
	mdi.AssertExpectations(t)
}

func TestDispatchDeliveryResponseInvalidSubscription(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mdi := sm.database.(*databasemocks.Plugin)
	mdi.On("GetSubscriptions", mock.Anything, "ns1", mock.Anything).Return([]*core.Subscription{}, nil, nil)
	err := sm.start()
	assert.NoError(t, err)
	be := &boundCallbacks{sm: sm, ei: mei}

	be.DeliveryResponse("conn1", &core.EventDeliveryResponse{
		ID: fftypes.NewUUID(),
		Subscription: core.SubscriptionRef{
			ID: fftypes.NewUUID(),
		},
	})
	mdi.AssertExpectations(t)
}

func TestConnIDSafetyChecking(t *testing.T) {
	mei1 := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei1)
	defer cancel()
	mei2 := &eventsmocks.Plugin{}
	mei2.On("Name").Return("ut2")
	be2 := &boundCallbacks{sm: sm, ei: mei2}

	sm.connections["conn1"] = &connection{
		ei:          mei1,
		id:          "conn1",
		transport:   "ut",
		dispatchers: map[fftypes.UUID]*eventDispatcher{},
	}

	err := be2.RegisterConnection("conn1", func(sr core.SubscriptionRef) bool { return true })
	assert.Regexp(t, "FF10190", err)

	err = be2.EphemeralSubscription("conn1", "ns1", &core.SubscriptionFilter{}, &core.SubscriptionOptions{})
	assert.Regexp(t, "FF10190", err)

	be2.DeliveryResponse("conn1", &core.EventDeliveryResponse{})

	be2.ConnectionClosed("conn1")

	assert.NotNil(t, sm.connections["conn1"])

}

func TestNewDurableSubscriptionBadSub(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mdi := sm.database.(*databasemocks.Plugin)

	subID := fftypes.NewUUID()
	mdi.On("GetSubscriptionByID", mock.Anything, "ns1", subID).Return(&core.Subscription{
		Filter: core.SubscriptionFilter{
			Events: "![[[[badness",
		},
	}, nil)
	sm.newOrUpdatedDurableSubscription(subID)

	assert.Empty(t, sm.durableSubs)
}

func TestNewDurableSubscriptionUnknownTransport(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mdi := sm.database.(*databasemocks.Plugin)

	sm.connections["conn1"] = &connection{
		ei:        mei,
		id:        "conn1",
		transport: "ut",
		matcher: func(sr core.SubscriptionRef) bool {
			return sr.Namespace == "ns1" && sr.Name == "sub1"
		},
		dispatchers: map[fftypes.UUID]*eventDispatcher{},
	}

	subID := fftypes.NewUUID()
	mdi.On("GetSubscriptionByID", mock.Anything, "ns1", subID).Return(&core.Subscription{
		SubscriptionRef: core.SubscriptionRef{
			ID:        subID,
			Namespace: "ns1",
			Name:      "sub1",
		},
		Transport: "unknown",
	}, nil)
	sm.newOrUpdatedDurableSubscription(subID)

	assert.Empty(t, sm.connections["conn1"].dispatchers)
	assert.Empty(t, sm.durableSubs)
}

func TestNewDurableSubscriptionOK(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mdi := sm.database.(*databasemocks.Plugin)
	mei.On("ValidateOptions", mock.Anything, mock.Anything).Return(nil)

	sm.connections["conn1"] = &connection{
		ei:        mei,
		id:        "conn1",
		transport: "ut",
		matcher: func(sr core.SubscriptionRef) bool {
			return sr.Namespace == "ns1" && sr.Name == "sub1"
		},
		dispatchers: map[fftypes.UUID]*eventDispatcher{},
	}

	subID := fftypes.NewUUID()
	mdi.On("GetSubscriptionByID", mock.Anything, "ns1", subID).Return(&core.Subscription{
		SubscriptionRef: core.SubscriptionRef{
			ID:        subID,
			Namespace: "ns1",
			Name:      "sub1",
		},
		Transport: "ut",
	}, nil)
	sm.newOrUpdatedDurableSubscription(subID)

	assert.NotEmpty(t, sm.connections["conn1"].dispatchers)
	assert.NotEmpty(t, sm.durableSubs)
}

func TestUpdatedDurableSubscriptionNoOp(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mdi := sm.database.(*databasemocks.Plugin)
	mei.On("ValidateOptions", mock.Anything, mock.Anything).Return(nil)

	subID := fftypes.NewUUID()
	sub := &core.Subscription{
		SubscriptionRef: core.SubscriptionRef{
			ID:        subID,
			Namespace: "ns1",
			Name:      "sub1",
		},
		Transport: "ut",
	}
	s := &subscription{
		definition: sub,
	}
	sm.durableSubs[*subID] = s

	ed, cancelEd := newTestEventDispatcher(s)
	defer cancelEd()
	sm.connections["conn1"] = &connection{
		ei:        mei,
		id:        "conn1",
		transport: "ut",
		matcher: func(sr core.SubscriptionRef) bool {
			return sr.Namespace == "ns1" && sr.Name == "sub1"
		},
		dispatchers: map[fftypes.UUID]*eventDispatcher{
			*subID: ed,
		},
	}

	mdi.On("GetSubscriptionByID", mock.Anything, "ns1", subID).Return(sub, nil)
	sm.newOrUpdatedDurableSubscription(subID)

	assert.Equal(t, ed, sm.connections["conn1"].dispatchers[*subID])
	assert.Equal(t, s, sm.durableSubs[*subID])
}

func TestUpdatedDurableSubscriptionOK(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mdi := sm.database.(*databasemocks.Plugin)
	mei.On("ValidateOptions", mock.Anything, mock.Anything).Return(nil)

	subID := fftypes.NewUUID()
	sub := &core.Subscription{
		SubscriptionRef: core.SubscriptionRef{
			ID:        subID,
			Namespace: "ns1",
			Name:      "sub1",
		},
		Transport: "ut",
	}
	sub2 := *sub
	sub2.Updated = fftypes.Now()
	s := &subscription{
		definition: sub,
	}
	sm.durableSubs[*subID] = s

	ed, cancelEd := newTestEventDispatcher(s)
	cancelEd()
	close(ed.closed)
	sm.connections["conn1"] = &connection{
		ei:        mei,
		id:        "conn1",
		transport: "ut",
		matcher: func(sr core.SubscriptionRef) bool {
			return sr.Namespace == "ns1" && sr.Name == "sub1"
		},
		dispatchers: map[fftypes.UUID]*eventDispatcher{
			*subID: ed,
		},
	}

	mdi.On("GetSubscriptionByID", mock.Anything, "ns1", subID).Return(&sub2, nil)
	sm.newOrUpdatedDurableSubscription(subID)

	assert.NotEqual(t, ed, sm.connections["conn1"].dispatchers[*subID])
	assert.NotEqual(t, s, sm.durableSubs[*subID])
	assert.NotEmpty(t, sm.connections["conn1"].dispatchers)
	assert.NotEmpty(t, sm.durableSubs)
}

func TestMatchedSubscriptionWithLockUnknownTransport(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()

	conn := &connection{
		matcher: func(sr core.SubscriptionRef) bool { return true },
	}
	sm.matchSubToConnLocked(conn, &subscription{definition: &core.Subscription{Transport: "Wrong!"}})
	assert.Nil(t, conn.dispatchers)
}

func TestMatchedSubscriptionWithBadMatcherRegisteredt(t *testing.T) {
	mei := &eventsmocks.Plugin{}
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()

	conn := &connection{}
	sm.matchSubToConnLocked(conn, &subscription{definition: &core.Subscription{Transport: "Wrong!"}})
	assert.Nil(t, conn.dispatchers)
}

func TestDeleteDurableSubscriptionOk(t *testing.T) {
	subID := fftypes.NewUUID()
	subDef := &core.Subscription{
		SubscriptionRef: core.SubscriptionRef{
			ID:        subID,
			Namespace: "ns1",
			Name:      "sub1",
		},
		Transport: "websockets",
	}
	sub := &subscription{
		definition: subDef,
	}
	testED1, _ := newTestEventDispatcher(sub)

	mei := testED1.transport.(*eventsmocks.Plugin)
	sm, cancel := newTestSubManager(t, mei)
	defer cancel()
	mdi := sm.database.(*databasemocks.Plugin)

	sm.durableSubs[*subID] = sub
	ed, _ := newTestEventDispatcher(sub)
	ed.database = mdi
	ed.start()
	sm.connections["conn1"] = &connection{
		ei:        mei,
		id:        "conn1",
		transport: "ut",
		matcher: func(sr core.SubscriptionRef) bool {
			return sr.Namespace == "ns1" && sr.Name == "sub1"
		},
		dispatchers: map[fftypes.UUID]*eventDispatcher{
			*subID: ed,
		},
	}

	mdi.On("GetSubscriptionByID", mock.Anything, "ns1", subID).Return(subDef, nil)
	mdi.On("DeleteOffset", mock.Anything, fftypes.FFEnum("subscription"), subID.String()).Return(fmt.Errorf("this error is logged and swallowed"))
	sm.deletedDurableSubscription(subID)

	assert.Empty(t, sm.connections["conn1"].dispatchers)
	assert.Empty(t, sm.durableSubs)
	<-ed.closed
}
