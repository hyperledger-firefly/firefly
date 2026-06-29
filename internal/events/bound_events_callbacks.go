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
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/events"
)

type boundCallbacks struct {
	sm *subscriptionManager
	ei events.Plugin
}

func (bc *boundCallbacks) RegisterConnection(connID string, matcher events.SubscriptionMatcher) error {
	return bc.sm.registerConnection(bc.ei, connID, matcher)
}

func (bc *boundCallbacks) EphemeralSubscription(connID, namespace string, filter *core.SubscriptionFilter, options *core.SubscriptionOptions) error {
	return bc.sm.ephemeralSubscription(bc.ei, connID, namespace, filter, options)
}

func (bc *boundCallbacks) DeliveryResponse(connID string, inflight *core.EventDeliveryResponse) {
	bc.sm.deliveryResponse(bc.ei, connID, inflight)
}

func (bc *boundCallbacks) ConnectionClosed(connID string) {
	bc.sm.connectionClosed(bc.ei, connID)
}
