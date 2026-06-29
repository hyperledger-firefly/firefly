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

package core

import "github.com/hyperledger-firefly/common/pkg/fftypes"

// EventType indicates what the event means, as well as what the Reference in the event refers to
type EventType = fftypes.FFEnum

var (
	// EventTypeTransactionSubmitted occurs only on the node that initiates a tranaction, when the transaction is submitted
	EventTypeTransactionSubmitted = fftypes.FFEnumValue("eventtype", "transaction_submitted")
	// EventTypeMessageConfirmed is the most important event type in the system. This means a message and all of its data
	// is available for processing by an application. Most applications only need to listen to this event type
	EventTypeMessageConfirmed = fftypes.FFEnumValue("eventtype", "message_confirmed")
	// EventTypeMessageRejected occurs if a message is received and confirmed from a sequencing perspective, but is rejected as invalid (mismatch to schema, or duplicate system broadcast)
	EventTypeMessageRejected = fftypes.FFEnumValue("eventtype", "message_rejected")
	// EventTypeDatatypeConfirmed occurs when a new datatype is ready for use (on the namespace of the datatype)
	EventTypeDatatypeConfirmed = fftypes.FFEnumValue("eventtype", "datatype_confirmed")
	// EventTypeIdentityConfirmed occurs when a new identity has been confirmed, as as result of a signed claim broadcast, and any associated claim verification
	EventTypeIdentityConfirmed = fftypes.FFEnumValue("eventtype", "identity_confirmed")
	// EventTypeIdentityUpdated occurs when an existing identity is update by the owner of that identity
	EventTypeIdentityUpdated = fftypes.FFEnumValue("eventtype", "identity_updated")
	// EventTypePoolConfirmed occurs when a new token pool is ready for use
	EventTypePoolConfirmed = fftypes.FFEnumValue("eventtype", "token_pool_confirmed")
	// EventTypePoolOpFailed occurs when a token pool creation initiated by this node has failed (based on feedback from connector)
	EventTypePoolOpFailed = fftypes.FFEnumValue("eventtype", "token_pool_op_failed")
	// EventTypeTransferConfirmed occurs when a token transfer has been confirmed
	EventTypeTransferConfirmed = fftypes.FFEnumValue("eventtype", "token_transfer_confirmed")
	// EventTypeTransferOpFailed occurs when a token transfer submitted by this node has failed (based on feedback from connector)
	EventTypeTransferOpFailed = fftypes.FFEnumValue("eventtype", "token_transfer_op_failed")
	// EventTypeApprovalConfirmed occurs when a token approval has been confirmed
	EventTypeApprovalConfirmed = fftypes.FFEnumValue("eventtype", "token_approval_confirmed")
	// EventTypeApprovalOpFailed occurs when a token approval submitted by this node has failed (based on feedback from connector)
	EventTypeApprovalOpFailed = fftypes.FFEnumValue("eventtype", "token_approval_op_failed")
	// EventTypeContractInterfaceConfirmed occurs when a new contract interface has been confirmed
	EventTypeContractInterfaceConfirmed = fftypes.FFEnumValue("eventtype", "contract_interface_confirmed")
	// EventTypeContractAPIConfirmed occurs when a new contract API has been confirmed
	EventTypeContractAPIConfirmed = fftypes.FFEnumValue("eventtype", "contract_api_confirmed")
	// EventTypeBlockchainEventReceived occurs when a new event has been received from the blockchain
	EventTypeBlockchainEventReceived = fftypes.FFEnumValue("eventtype", "blockchain_event_received")
	// EventTypeBlockchainInvokeOpSucceeded occurs when a blockchain "invoke" request has succeeded
	EventTypeBlockchainInvokeOpSucceeded = fftypes.FFEnumValue("eventtype", "blockchain_invoke_op_succeeded")
	// EventTypeBlockchainInvokeOpFailed occurs when a blockchain "invoke" request has failed
	EventTypeBlockchainInvokeOpFailed = fftypes.FFEnumValue("eventtype", "blockchain_invoke_op_failed")
	// EventTypeBlockchainContractDeployOpSucceeded occurs when a contract deployment request has succeeded
	EventTypeBlockchainContractDeployOpSucceeded = fftypes.FFEnumValue("eventtype", "blockchain_contract_deploy_op_succeeded")
	// EventTypeBlockchainContractDeployOpFailed occurs when a contract deployment request has failed
	EventTypeBlockchainContractDeployOpFailed = fftypes.FFEnumValue("eventtype", "blockchain_contract_deploy_op_failed")
)

// Event is an activity in the system, delivered reliably to applications, that indicates something has happened in the network
type Event struct {
	ID          *fftypes.UUID   `ffstruct:"Event" json:"id"`
	Sequence    int64           `ffstruct:"Event" json:"sequence"`
	Type        EventType       `ffstruct:"Event" json:"type" ffenum:"eventtype"`
	Namespace   string          `ffstruct:"Event" json:"namespace"`
	Reference   *fftypes.UUID   `ffstruct:"Event" json:"reference"`
	Correlator  *fftypes.UUID   `ffstruct:"Event" json:"correlator,omitempty"`
	Transaction *fftypes.UUID   `ffstruct:"Event" json:"tx,omitempty"`
	Topic       string          `ffstruct:"Event" json:"topic,omitempty"`
	Created     *fftypes.FFTime `ffstruct:"Event" json:"created"`
}

// EnrichedEvent adds the referred object to an event
type EnrichedEvent struct {
	Event
	BlockchainEvent   *BlockchainEvent `ffstruct:"EnrichedEvent" json:"blockchainEvent,omitempty"`
	ContractAPI       *ContractAPI     `ffstruct:"EnrichedEvent" json:"contractAPI,omitempty"`
	ContractInterface *fftypes.FFI     `ffstruct:"EnrichedEvent" json:"contractInterface,omitempty"`
	Datatype          *Datatype        `ffstruct:"EnrichedEvent" json:"datatype,omitempty"`
	Identity          *Identity        `ffstruct:"EnrichedEvent" json:"identity,omitempty"`
	Message           *Message         `ffstruct:"EnrichedEvent" json:"message,omitempty"`
	TokenApproval     *TokenApproval   `ffstruct:"EnrichedEvent" json:"tokenApproval,omitempty"`
	TokenPool         *TokenPool       `ffstruct:"EnrichedEvent" json:"tokenPool,omitempty"`
	TokenTransfer     *TokenTransfer   `ffstruct:"EnrichedEvent" json:"tokenTransfer,omitempty"`
	Transaction       *Transaction     `ffstruct:"EnrichedEvent" json:"transaction,omitempty"`
	Operation         *Operation       `ffstruct:"EnrichedEvent" json:"operation,omitempty"`
}

// EventDelivery adds the referred object to an event, as well as details of the subscription that caused the event to
// be dispatched to an application.
type EventDelivery struct {
	EnrichedEvent
	Subscription SubscriptionRef `json:"subscription"`
}

type CombinedEventDataDelivery struct {
	Event *EventDelivery
	Data  DataArray
}

// EventDeliveryResponse is the payload an application sends back, to confirm it has accepted (or rejected) the event and as such
// does not need to receive it again.
type EventDeliveryResponse struct {
	ID           *fftypes.UUID   `json:"id"`
	Rejected     bool            `json:"rejected,omitempty"`
	Info         string          `json:"info,omitempty"`
	Subscription SubscriptionRef `json:"subscription"`
	Reply        *MessageInOut   `json:"reply,omitempty"`
}

func NewEvent(t EventType, ns string, ref *fftypes.UUID, tx *fftypes.UUID, topic string) *Event {
	return &Event{
		ID:          fftypes.NewUUID(),
		Type:        t,
		Namespace:   ns,
		Reference:   ref,
		Transaction: tx,
		Topic:       topic,
		Created:     fftypes.Now(),
	}
}

func (e *Event) LocalSequence() int64 {
	return e.Sequence
}
