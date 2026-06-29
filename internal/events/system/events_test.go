// Copyright © 2021 Kaleido, Inc.
//
// SPDX-License-Identifser: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implsed.
// See the License for the specific language governing permissions and
// limitations under the License.

package system

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/mocks/eventsmocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func newTestEvents(t *testing.T) (se *Events, cancel func()) {
	coreconfig.Reset()

	cbs := &eventsmocks.Callbacks{}
	rc := cbs.On("RegisterConnection", mock.Anything, mock.Anything).Return(nil)
	rc.RunFn = func(a mock.Arguments) {
		assert.Equal(t, true, a[1].(events.SubscriptionMatcher)(core.SubscriptionRef{}))
	}
	se = &Events{}
	ctx, cancelCtx := context.WithCancel(context.Background())
	config := config.RootSection("ut.events")
	se.InitConfig(config)
	se.Init(ctx, config)
	se.SetHandler("ns1", cbs)
	assert.Equal(t, "system", se.Name())
	assert.NotNil(t, se.Capabilities())
	assert.Nil(t, se.ValidateOptions(ctx, &core.SubscriptionOptions{}))
	return se, cancelCtx
}

func TestDeliveryRequestOk(t *testing.T) {

	se, cancel := newTestEvents(t)
	defer cancel()

	sub := &core.Subscription{
		SubscriptionRef: core.SubscriptionRef{
			Namespace: "ns1",
		},
	}

	cbs := se.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)
	cbs.On("EphemeralSubscription", mock.Anything, "ns1", mock.Anything, mock.Anything).Return(nil)
	cbs.On("DeliveryResponse", se.connID, mock.Anything).Return(nil)

	called := 0
	err := se.AddListener("ns1", func(event *core.EventDelivery) error {
		called++
		return nil
	})
	assert.NoError(t, err)

	err = se.DeliveryRequest(se.ctx, se.connID, sub, &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				Namespace: "ns1",
			},
		},
	}, nil)
	assert.NoError(t, err)

	err = se.DeliveryRequest(se.ctx, se.connID, &core.Subscription{}, &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				Namespace: "ns2",
			},
		},
	}, nil)
	assert.NoError(t, err)

	assert.Equal(t, 1, called)
	cbs.AssertExpectations(t)

	se.SetHandler("ns1", nil)
	assert.Empty(t, se.callbacks.handlers)

}

func TestDeliveryRequestFail(t *testing.T) {

	se, cancel := newTestEvents(t)
	defer cancel()

	cbs := se.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)
	cbs.On("EphemeralSubscription", mock.Anything, "ns1", mock.Anything, mock.Anything).Return(nil)

	err := se.AddListener("ns1", func(event *core.EventDelivery) error {
		return fmt.Errorf("pop")
	})
	assert.NoError(t, err)

	err = se.DeliveryRequest(se.ctx, mock.Anything, &core.Subscription{}, &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				Namespace: "ns1",
			},
		},
	}, nil)
	assert.EqualError(t, err, "pop")

}

func TestAddListenerFail(t *testing.T) {

	se, cancel := newTestEvents(t)
	defer cancel()

	cbs := se.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)
	cbs.On("EphemeralSubscription", mock.Anything, "ns1", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))

	err := se.AddListener("ns1", func(event *core.EventDelivery) error { return nil })
	assert.EqualError(t, err, "pop")

}

func TestNamespaceRestarted(t *testing.T) {
	se, cancel := newTestEvents(t)
	defer cancel()

	se.NamespaceRestarted("ns1", time.Now())
}

func TestEventDeliveryBatch(t *testing.T) {
	se, cancel := newTestEvents(t)
	defer cancel()

	sub := &core.Subscription{
		SubscriptionRef: core.SubscriptionRef{
			Namespace: "ns1",
		},
	}

	err := se.BatchDeliveryRequest(se.ctx, "id", sub, []*core.CombinedEventDataDelivery{})
	assert.Regexp(t, "FF10461", err)
}
