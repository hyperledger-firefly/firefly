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
	"time"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

// Plugin is the interface implemented by event interface
// - Delivery to generic application code - WebSockets, Webhooks, AMQP etc.
// - Integration of frameworks for coordination of multi-party compute - Hyperledger Avalon,  etc.
type Plugin interface {
	core.Named

	// InitConfig initializes the set of configuration options that are valid, with defaults. Called on all plugins.
	InitConfig(config config.Section)

	// Init initializes the plugin, with configuration
	Init(ctx context.Context, config config.Section) error

	// SetHandler registers a handler to receive callbacks
	// If namespace is set, plugin will attempt to deliver only events for that namespace
	SetHandler(namespace string, handler Callbacks) error

	// Capabilities returns capabilities - not called until after Init
	Capabilities() *Capabilities

	// ValidateOptions verifies a set of input options, prior to storage of a new subscription
	// The plugin can modify the core subscription options, such as overriding whether data is delivered.
	ValidateOptions(ctx context.Context, options *core.SubscriptionOptions) error

	// DeliveryRequest requests delivery of work on a connection, which must later be responded to
	// Data will only be supplied as non-nil if the subscription is set to include data
	DeliveryRequest(ctx context.Context, connID string, sub *core.Subscription, event *core.EventDelivery, data core.DataArray) error

	// DeliveryBatchRequest requests delivery of multiple events on a connection, which must later be responded to
	// Data will only be supplied as non-nil if the subscription is set to include data
	BatchDeliveryRequest(ctx context.Context, connID string, sub *core.Subscription, events []*core.CombinedEventDataDelivery) error

	// NamespaceRestarted is called after a namespace restarts. For a connect-in style plugin, like
	// WebSockets, this must re-register any active connections that started before the time passed in.
	NamespaceRestarted(ns string, startTime time.Time)
}

type SubscriptionMatcher func(core.SubscriptionRef) bool

type Callbacks interface {

	// RegisterConnection can be fired as often as required.
	// Dispatchers will be started against this connection for all persisted subscriptions that match via the supplied function.
	// It can be fired multiple times for the same connection ID, to update the subscription list
	// For a "connect-out" style plugin (MQTT/AMQP/JMS broker), you might fire it at startup (from Init) for each target queue, with a subscription match
	// For a "connect-in" style plugin (inbound WebSocket connections), you fire it every time the client application connects attaches to a subscription
	RegisterConnection(connID string, matcher SubscriptionMatcher) error

	// EphemeralSubscription creates an ephemeral (non-durable) subscription, and associates it with a connection
	EphemeralSubscription(connID, namespace string, filter *core.SubscriptionFilter, options *core.SubscriptionOptions) error

	// ConnectionClosed is a notification that a connection has closed, and all dispatchers should be re-allocated.
	// Note the plugin must not crash if it receives PublishEvent calls on the connID after the ConnectionClosed event is fired
	ConnectionClosed(connID string)

	// DeliveryResponse responds to a previous event delivery, to either:
	// - Acknowledge it: the offset for the associated subscription can move forwards
	//   * Note all gaps must fill before the offset can move forwards, so this message might still be redelivered if streaming ahead
	//   * If a message is included in the response, then that will be automatically sent with the correct CID
	// - Reject it: This resets the associated subscription back to the last committed offset
	//   * Note all message since the last committed offet will be redelivered, so additional messages to be redelivered if streaming ahead
	DeliveryResponse(connID string, inflight *core.EventDeliveryResponse)
}

type Capabilities struct {
	BatchDelivery bool
}
