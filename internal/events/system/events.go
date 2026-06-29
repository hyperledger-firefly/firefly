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

package system

import (
	"context"
	"sync"
	"time"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/events"
)

const (
	SystemEventsTransport = "system"
)

// Events the system events listener is used for system components that need access to confirmed events,
// such as the sync<->async bridge.
type Events struct {
	ctx          context.Context
	capabilities *events.Capabilities
	callbacks
	mux       sync.Mutex
	listeners map[string][]EventListener
	connID    string
	readAhead uint
}

type callbacks struct {
	writeLock sync.Mutex
	handlers  map[string]events.Callbacks
}

type EventInterface interface {
	AddSystemEventListener(ns string, el EventListener) error
}

type EventListener func(event *core.EventDelivery) error

func (se *Events) Name() string { return SystemEventsTransport }

func (se *Events) Init(ctx context.Context, config config.Section) (err error) {
	*se = Events{
		ctx:          ctx,
		capabilities: &events.Capabilities{},
		callbacks: callbacks{
			handlers: make(map[string]events.Callbacks),
		},
		listeners: make(map[string][]EventListener),
		readAhead: config.GetUint(SystemEventsConfReadAhead),
		connID:    fftypes.ShortID(),
	}
	return nil
}

func (se *Events) SetHandler(namespace string, handler events.Callbacks) error {
	se.callbacks.writeLock.Lock()
	defer se.callbacks.writeLock.Unlock()
	if handler == nil {
		delete(se.callbacks.handlers, namespace)
		return nil
	}
	se.callbacks.handlers[namespace] = handler
	// We have a single logical connection, that matches all subscriptions
	return handler.RegisterConnection(se.connID, func(sr core.SubscriptionRef) bool { return true })
}

func (se *Events) Capabilities() *events.Capabilities {
	return se.capabilities
}

func (se *Events) ValidateOptions(ctx context.Context, options *core.SubscriptionOptions) error {
	return nil
}

func (se *Events) AddListener(ns string, el EventListener) error {
	no := false
	newest := core.SubOptsFirstEventNewest
	if cb, ok := se.callbacks.handlers[ns]; ok {
		err := cb.EphemeralSubscription(se.connID, ns, &core.SubscriptionFilter{ /* all events */ }, &core.SubscriptionOptions{
			SubscriptionCoreOptions: core.SubscriptionCoreOptions{
				WithData:   &no,
				ReadAhead:  &se.readAhead,
				FirstEvent: &newest,
			},
		})
		if err != nil {
			return err
		}
		se.mux.Lock()
		se.listeners[ns] = append(se.listeners[ns], el)
		se.mux.Unlock()
	}
	return nil
}

func (se *Events) DeliveryRequest(ctx context.Context, connID string, sub *core.Subscription, event *core.EventDelivery, data core.DataArray) error {
	se.mux.Lock()
	defer se.mux.Unlock()
	for ns, listeners := range se.listeners {
		if event.Event.Namespace == ns {
			for _, el := range listeners {
				if err := el(event); err != nil {
					return err
				}
			}
		}
	}
	if cb, ok := se.callbacks.handlers[sub.Namespace]; ok {
		cb.DeliveryResponse(connID, &core.EventDeliveryResponse{
			ID:           event.ID,
			Rejected:     false,
			Subscription: event.Subscription,
			Reply:        nil,
		})
	}
	return nil
}

func (se *Events) BatchDeliveryRequest(ctx context.Context, connID string, sub *core.Subscription, events []*core.CombinedEventDataDelivery) error {
	return i18n.NewError(ctx, coremsgs.MsgBatchDeliveryNotSupported, se.Name()) // should never happen
}

func (se *Events) NamespaceRestarted(ns string, startTime time.Time) {
	// no-op
}
