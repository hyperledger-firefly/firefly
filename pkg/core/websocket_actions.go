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

package core

import "github.com/hyperledger-firefly/common/pkg/fftypes"

// WSClientPayloadType actions go from client->server
type WSClientPayloadType = fftypes.FFEnum

var (
	// WSClientActionStart is a request to the server to start delivering messages to the client
	WSClientActionStart = fftypes.FFEnumValue("wstype", "start")
	// WSClientActionAck acknowledges an event that was delivered, allowing further messages to be sent
	WSClientActionAck = fftypes.FFEnumValue("wstype", "ack")

	// WSProtocolErrorEventType is a special event "type" field for server to send the client, if it performs a ProtocolError
	WSProtocolErrorEventType = fftypes.FFEnumValue("wstype", "protocol_error")

	// WSEventBatchType is the type set when the message contains an array of events
	WSEventBatchType = fftypes.FFEnumValue("wstype", "event_batch")
)

// WSActionBase is the base fields of all client actions sent on the websocket
type WSActionBase struct {
	Type WSClientPayloadType `ffstruct:"WSActionBase" json:"type,omitempty" ffenum:"wstype"`
}

// WSStart starts a subscription on this socket - either an existing one, or creating an ephemeral one
type WSStart struct {
	WSActionBase

	AutoAck   *bool               `ffstruct:"WSStart" json:"autoack"`
	Namespace string              `ffstruct:"WSStart" json:"namespace"`
	Name      string              `ffstruct:"WSStart" json:"name"`
	Ephemeral bool                `ffstruct:"WSStart" json:"ephemeral"`
	Filter    SubscriptionFilter  `ffstruct:"WSStart" json:"filter"`
	Options   SubscriptionOptions `ffstruct:"WSStart" json:"options"`
}

// WSAck acknowledges a received event (not applicable in AutoAck mode)
type WSAck struct {
	WSActionBase

	ID           *fftypes.UUID    `ffstruct:"WSAck" json:"id,omitempty"`
	Subscription *SubscriptionRef `ffstruct:"WSAck" json:"subscription,omitempty"`
}

// WSError is sent to the client by the server in the case of a protocol error
type WSError struct {
	Type  WSClientPayloadType `ffstruct:"WSAck" json:"type" ffenum:"wstype"`
	Error string              `ffstruct:"WSAck" json:"error"`
}

// WSEventBatch is used when batched delivery is enabled over the websocket, allowing
// an array of events to be ack'd as a whole (rather than ack'ing individually)
type WSEventBatch struct {
	Type         WSClientPayloadType `ffstruct:"WSEventBatch" json:"type" ffenum:"wstype"`
	ID           *fftypes.UUID       `ffstruct:"WSEventBatch" json:"id"`
	Subscription SubscriptionRef     `ffstruct:"WSEventBatch" json:"subscription"`
	Events       []*EventDelivery    `ffstruct:"WSEventBatch" json:"events"`
}
