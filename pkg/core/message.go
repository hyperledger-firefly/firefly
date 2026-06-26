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

import (
	"context"
	"crypto/sha256"
	"encoding/json"

	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-common/pkg/i18n"
)

const (
	// DefaultTopic will be set as the topic of any messages set without a topic
	DefaultTopic = "default"
)

// MessageType is the fundamental type of a message
type MessageType = fftypes.FFEnum

var (
	// MessageTypeDefinition is a message broadcasting a definition of a system type, pre-defined by firefly (namespaces, identities, data definitions, etc.)
	MessageTypeDefinition = fftypes.FFEnumValue("messagetype", "definition")
	// MessageTypeBroadcast is a broadcast message, meaning it is intended to be visible by all parties in the network
	MessageTypeBroadcast = fftypes.FFEnumValue("messagetype", "broadcast")
	// MessageTypePrivate is a private message, meaning it is only sent explicitly to individual parties in the network
	MessageTypePrivate = fftypes.FFEnumValue("messagetype", "private")
	// MessageTypeGroupInit is a special private message that contains the definition of the group
	MessageTypeGroupInit = fftypes.FFEnumValue("messagetype", "groupinit")
	// MessageTypeDeprecatedTransferBroadcast is deprecated - use MessageTypeBroadcast (and refer to TxParent.Type)
	MessageTypeDeprecatedTransferBroadcast = fftypes.FFEnumValue("messagetype", "transfer_broadcast")
	// MessageTypeDeprecatedTransferPrivate is deprecated - use MessageTypePrivate (and refer to TxParent.Type)
	MessageTypeDeprecatedTransferPrivate = fftypes.FFEnumValue("messagetype", "transfer_private")
	// MessageTypeDeprecatedApprovalBroadcast is deprecated - use MessageTypeBroadcast (and refer to TxParent.Type)
	MessageTypeDeprecatedApprovalBroadcast = fftypes.FFEnumValue("messagetype", "approval_broadcast")
	// MessageTypeDeprecatedApprovalPrivate is a deprecated - use MessageTypePrivate (and refer to TxParent.Type)
	MessageTypeDeprecatedApprovalPrivate = fftypes.FFEnumValue("messagetype", "approval_private")
)

// MessageState is the current transmission/confirmation state of a message
type MessageState = fftypes.FFEnum

var (
	// MessageStateStaged is a message created locally which is not ready to send
	MessageStateStaged = fftypes.FFEnumValue("messagestate", "staged")
	// MessageStateReady is a message created locally which is ready to send
	MessageStateReady = fftypes.FFEnumValue("messagestate", "ready")
	// MessageStateSent is a message created locally which has been sent in a batch
	MessageStateSent = fftypes.FFEnumValue("messagestate", "sent")
	// MessageStatePending is a message that has been received but is awaiting aggregation/confirmation
	MessageStatePending = fftypes.FFEnumValue("messagestate", "pending")
	// MessageStateConfirmed is a message that has completed all required confirmations (blockchain if pinned, token transfer if transfer coupled, etc)
	MessageStateConfirmed = fftypes.FFEnumValue("messagestate", "confirmed")
	// MessageStateRejected is a message that has completed confirmation, but has been rejected by FireFly
	MessageStateRejected = fftypes.FFEnumValue("messagestate", "rejected")
	// MessageStateCancelled is a message that was cancelled without being sent
	MessageStateCancelled = fftypes.FFEnumValue("messagestate", "cancelled")
)

// MessageHeader contains all fields that contribute to the hash
// The order of the serialization mut not change, once released
type MessageHeader struct {
	ID     *fftypes.UUID   `ffstruct:"MessageHeader" json:"id,omitempty" ffexcludeinput:"true"`
	CID    *fftypes.UUID   `ffstruct:"MessageHeader" json:"cid,omitempty"`
	Type   MessageType     `ffstruct:"MessageHeader" json:"type" ffenum:"messagetype"`
	TxType TransactionType `ffstruct:"MessageHeader" json:"txtype,omitempty" ffenum:"txtype"`
	SignerRef
	Created   *fftypes.FFTime       `ffstruct:"MessageHeader" json:"created,omitempty" ffexcludeinput:"true"`
	Namespace string                `ffstruct:"MessageHeader" json:"namespace,omitempty" ffexcludeinput:"true"`
	Group     *fftypes.Bytes32      `ffstruct:"MessageHeader" json:"group,omitempty" ffexclude:"postNewMessageBroadcast"`
	Topics    fftypes.FFStringArray `ffstruct:"MessageHeader" json:"topics,omitempty"`
	Tag       string                `ffstruct:"MessageHeader" json:"tag,omitempty"`
	DataHash  *fftypes.Bytes32      `ffstruct:"MessageHeader" json:"datahash,omitempty" ffexcludeinput:"true"`
	TxParent  *TransactionRef       `ffstruct:"MessageHeader" json:"txparent,omitempty" ffexcludeinput:"true"`
}

// Message is the envelope by which coordinated data exchange can happen between parties in the network
// Data is passed by reference in these messages, and a chain of hashes covering the data and the
// details of the message, provides a verification against tampering.
type Message struct {
	Header         MessageHeader         `ffstruct:"Message" json:"header"`
	LocalNamespace string                `ffstruct:"Message" json:"localNamespace,omitempty" ffexcludeinput:"true"`
	Hash           *fftypes.Bytes32      `ffstruct:"Message" json:"hash,omitempty" ffexcludeinput:"true"`
	BatchID        *fftypes.UUID         `ffstruct:"Message" json:"batch,omitempty" ffexcludeinput:"true"`
	TransactionID  *fftypes.UUID         `ffstruct:"Message" json:"txid,omitempty" ffexcludeinput:"true"`
	State          MessageState          `ffstruct:"Message" json:"state,omitempty" ffenum:"messagestate" ffexcludeinput:"true"`
	Confirmed      *fftypes.FFTime       `ffstruct:"Message" json:"confirmed,omitempty" ffexcludeinput:"true"`
	RejectReason   string                `ffstruct:"Message" json:"rejectReason,omitempty" ffexcludeinput:"true"`
	Data           DataRefs              `ffstruct:"Message" json:"data" ffexcludeinput:"true"`
	Pins           fftypes.FFStringArray `ffstruct:"Message" json:"pins,omitempty" ffexcludeinput:"true"`
	IdempotencyKey IdempotencyKey        `ffstruct:"Message" json:"idempotencyKey,omitempty"`
	Sequence       int64                 `ffstruct:"Message" json:"-"` // Local database sequence used internally for batch assembly
}

// BatchMessage is the fields in a message record that are assured to be consistent on all parties.
// This is what is transferred and hashed in a batch payload between nodes.
//
// Fields such as the idempotencyKey do NOT transfer, as these are meant for local processing of messages before being sent.
//
// Fields such as the state/confirmed do NOT transfer, as these are calculated individually by each member.
func (m *Message) BatchMessage() *Message {
	return &Message{
		Header:        m.Header,
		Hash:          m.Hash,
		Data:          m.Data,
		TransactionID: m.TransactionID,
		// The pins are immutable once assigned by the sender, which happens before the batch is sealed
		Pins: m.Pins,
	}
}

// MessageInOut allows API users to submit values in-line in the payload submitted, which
// will be broken out and stored separately during the call.
type MessageInOut struct {
	Message
	InlineData InlineData  `ffstruct:"MessageInOut" json:"data,omitempty"`
	Group      *InputGroup `ffstruct:"MessageInOut" json:"group,omitempty" ffexclude:"postNewMessageBroadcast"`
}

// InputGroup declares a group in-line for automatic resolution, without having to define a group up-front
type InputGroup struct {
	Name    string        `ffstruct:"InputGroup" json:"name,omitempty"`
	Members []MemberInput `ffstruct:"InputGroup" json:"members"`
}

// InlineData is an array of data references or values
type InlineData []*DataRefOrValue

// DataRefOrValue allows a value to be specified in-line in the data array of an input
// message, avoiding the need for a multiple API calls.
type DataRefOrValue struct {
	DataRef

	Validator ValidatorType    `ffstruct:"DataRefOrValue" json:"validator,omitempty"`
	Datatype  *DatatypeRef     `ffstruct:"DataRefOrValue" json:"datatype,omitempty"`
	Value     *fftypes.JSONAny `ffstruct:"DataRefOrValue" json:"value,omitempty"`
	Blob      *BlobRef         `ffstruct:"DataRefOrValue" json:"blob,omitempty" ffexcludeinput:"true"`
}

// MessageRef is a lightweight data structure that can be used to refer to a message
type MessageRef struct {
	ID   *fftypes.UUID    `ffstruct:"MessageRef" json:"id,omitempty"`
	Hash *fftypes.Bytes32 `ffstruct:"MessageRef" json:"hash,omitempty"`
}

func (h *MessageHeader) Hash() *fftypes.Bytes32 {
	b, _ := json.Marshal(&h)
	var b32 fftypes.Bytes32 = sha256.Sum256(b)
	return &b32
}

func (m *MessageInOut) SetInlineData(data []*Data) {
	m.InlineData = make(InlineData, len(data))
	for i, d := range data {
		m.InlineData[i] = &DataRefOrValue{
			DataRef: DataRef{
				ID:   d.ID,
				Hash: d.Hash,
			},
			Validator: d.Validator,
			Datatype:  d.Datatype,
			Value:     d.Value,
		}
	}
}

const messageSizeEstimateBase = int64(1024)

func (m *Message) EstimateSize(includeDataRefs bool) int64 {
	// For now we have a static estimate for the size of the serialized header structure.
	//
	// includeDataRefs should only be set when the data has been resolved from the database.
	size := messageSizeEstimateBase
	if includeDataRefs {
		for _, dr := range m.Data {
			size += dr.ValueSize
		}
	}
	return size
}

func (m *Message) Seal(ctx context.Context) (err error) {
	if len(m.Header.Topics) == 0 {
		m.Header.Topics = []string{DefaultTopic}
	}
	if m.Header.ID == nil {
		m.Header.ID = fftypes.NewUUID()
	}
	if m.Header.Created == nil {
		m.Header.Created = fftypes.Now()
	}
	m.Confirmed = nil
	if m.Data == nil {
		m.Data = DataRefs{}
	}
	if m.Header.TxType == "" {
		m.Header.TxType = TransactionTypeBatchPin
	}
	err = m.VerifyFields(ctx)
	if err == nil {
		m.Header.DataHash = m.Data.Hash()
		m.Hash = m.Header.Hash()
	}
	return err
}

func (m *Message) DupDataCheck(ctx context.Context) (err error) {
	dupCheck := make(map[string]bool)
	for i, d := range m.Data {
		if d.ID == nil || d.Hash == nil {
			return i18n.NewError(ctx, i18n.MsgNilDataReferenceSealFail, i)
		}
		if dupCheck[d.ID.String()] {
			return i18n.NewError(ctx, i18n.MsgDupDataReferenceSealFail, i)
		}
		dupCheck[d.ID.String()] = true
		dupCheck[d.Hash.String()] = true
	}
	return nil
}

func (m *Message) VerifyFields(ctx context.Context) error {
	// Validate topics
	if len(m.Header.Topics) == 0 {
		return i18n.NewError(ctx, i18n.MsgEmptyTopics)
	}
	for i, topic := range m.Header.Topics {
		if topic == "" {
			return i18n.NewError(ctx, i18n.MsgEmptyTopic, i)
		}
		// Add security check for topic length and characters
		if len(topic) > 256 {
			return i18n.NewError(ctx, i18n.MsgTopicTooLong, i)
		}
		if !isValidTopicString(topic) {
			return i18n.NewError(ctx, i18n.MsgInvalidTopicChars, i)
		}
	}

	// Validate tag
	if m.Header.Tag != "" {
		if !isValidTagString(m.Header.Tag) {
			return i18n.NewError(ctx, i18n.MsgInvalidTagChars)
		}
		if len(m.Header.Tag) > 64 {
			return i18n.NewError(ctx, i18n.MsgTagTooLong)
		}
	}

	// Validate author
	if m.Header.Author == "" {
		return i18n.NewError(ctx, i18n.MsgMissingAuthor)
	}
	if len(m.Header.Author) > 256 {
		return i18n.NewError(ctx, i18n.MsgAuthorTooLong)
	}

	// Validate data references
	if err := m.DupDataCheck(ctx); err != nil {
		return err
	}

	return nil
}

// isValidTopicString checks if a topic string contains only valid characters
func isValidTopicString(s string) bool {
	for _, r := range s {
		if !isValidTopicChar(r) {
			return false
		}
	}
	return true
}

// isValidTopicChar checks if a rune is valid for a topic string
func isValidTopicChar(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '-' || r == '_' || r == '.' ||
		r == '/' || r == ':' || r == '@'
}

// isValidTagString checks if a tag string contains only valid characters
func isValidTagString(s string) bool {
	for _, r := range s {
		if !isValidTagChar(r) {
			return false
		}
	}
	return true
}

// isValidTagChar checks if a rune is valid for a tag string
func isValidTagChar(r rune) bool {
	return (r >= 'a' && r <= 'z') ||
		(r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '-' || r == '_'
}

func (m *Message) Verify(ctx context.Context) error {
	err := m.VerifyFields(ctx)
	if err != nil {
		return err
	}
	if m.Hash == nil || m.Header.DataHash == nil {
		return i18n.NewError(ctx, i18n.MsgVerifyFailedNilHashes)
	}
	headerHash := m.Header.Hash()
	dataHash := m.Data.Hash()
	if *m.Hash != *headerHash || *m.Header.DataHash != *dataHash {
		return i18n.NewError(ctx, i18n.MsgVerifyFailedInvalidHashes, m.Hash.String(), headerHash.String(), m.Header.DataHash.String(), dataHash.String())
	}
	return nil
}

func (m *Message) LocalSequence() int64 {
	return m.Sequence
}

// MessageAction is an action to be taken on a message during processing
type MessageAction int

const (
	// ActionReject the message was successfully processed, but was malformed/invalid and should be marked as rejected
	ActionReject MessageAction = iota

	// ActionConfirm the message was valid and should be confirmed
	ActionConfirm

	// ActionRetry a recoverable error was encountered - batch should be halted and then re-processed from the start
	ActionRetry

	// ActionWait the message is still awaiting further pieces for aggregation and should be held in pending state
	ActionWait
)

func (dma MessageAction) String() string {
	switch dma {
	case ActionReject:
		return "reject"
	case ActionConfirm:
		return "confirm"
	case ActionRetry:
		return "retry"
	case ActionWait:
		return "wait"
	default:
		return "unknown"
	}
}
