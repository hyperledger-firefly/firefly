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

package blockchain

import (
	"context"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/internal/cache"
	"github.com/hyperledger-firefly/firefly/internal/metrics"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

// ResolveKeyIntent allows us to distinguish between resolving a key just for a lookup, vs. accepting in an action to sign
type ResolveKeyIntent string

const (
	ResolveKeyIntentSign   ResolveKeyIntent = "sign"   // used everywhere we accept an signing action (messages, tokens, custom invoke)
	ResolveKeyIntentQuery  ResolveKeyIntent = "query"  // used to perform a query to the blockchain, to run logic in order to query blockchain state
	ResolveKeyIntentLookup ResolveKeyIntent = "lookup" // used only on the /api/v1/resolve API
)

// Plugin is the interface implemented by each blockchain plugin
type Plugin interface {
	core.Named

	// InitConfig initializes the set of configuration options that are valid, with defaults. Called on all plugins.
	InitConfig(config config.Section)

	// Init initializes the plugin, with configuration
	Init(ctx context.Context, cancelCtx context.CancelFunc, config config.Section, metrics metrics.Manager, cacheManager cache.Manager) error

	// StartNamespace starts a specific namespace within the plugin
	StartNamespace(ctx context.Context, namespace string) error

	// StopNamespace removes a namespace from use within the plugin
	StopNamespace(ctx context.Context, namespace string) error

	// SetHandler registers a handler to receive callbacks
	// Plugin will attempt (but is not guaranteed) to deliver events only for the given namespace
	SetHandler(namespace string, handler Callbacks)

	// SetOperationHandler registers a handler to receive async operation status
	// If namespace is set, plugin will attempt to deliver only events for that namespace
	SetOperationHandler(namespace string, handler core.OperationCallbacks)

	// Capabilities returns capabilities - not called until after Init
	Capabilities() *Capabilities

	// VerifierType returns the verifier (key) type that is used by this blockchain
	VerifierType() core.VerifierType

	// ResolveSigningKey allows blockchain specific processing of keys supplied by users
	// of this FireFly core API before a transaction is accepted using that signing key.
	// May perform sophisticated checks and resolution as determined by the blockchain connector,
	// and associated resolution plugins:
	// - Such as resolving a Fabric shortname to a MSP ID
	// - Such using an external REST API plugin to resolve a HD wallet address, or other key alias
	// - Results in a string that can be stored/compared consistently with the key emitted on events signed by this key
	ResolveSigningKey(ctx context.Context, keyRef string, intent ResolveKeyIntent) (string, error)

	// SubmitBatchPin sequences a batch of message globally to all viewers of a given ledger
	SubmitBatchPin(ctx context.Context, nsOpID, networkNamespace, signingKey string, batch *BatchPin, location *fftypes.JSONAny) error

	// SubmitNetworkAction writes a special "BatchPin" event which signals the plugin to take an action
	SubmitNetworkAction(ctx context.Context, nsOpID, signingKey string, action core.NetworkActionType, location *fftypes.JSONAny) error

	// DeployContract submits a new transaction to deploy a new instance of a smart contract
	DeployContract(ctx context.Context, nsOpID, signingKey string, definition, contract *fftypes.JSONAny, input []interface{}, options map[string]interface{}) (submissionRejected bool, err error)

	// ParseInterface processes an FFIMethod and FFIError array into a blockchain specific object, that will be
	// cached for this given interface, and passed back on all future invocations.
	ParseInterface(ctx context.Context, method *fftypes.FFIMethod, errors []*fftypes.FFIError) (interface{}, error)

	// ValidateInvokeRequest performs pre-flight validation of a method call
	ValidateInvokeRequest(ctx context.Context, parsedMethod interface{}, input map[string]interface{}, hasMessage bool) error

	// InvokeContract submits a new transaction to be executed by custom on-chain logic
	InvokeContract(ctx context.Context, nsOpID, signingKey string, location *fftypes.JSONAny, parsedMethod interface{}, input map[string]interface{}, options map[string]interface{}, batch *BatchPin) (submissionRejected bool, err error)

	// QueryContract executes a method via custom on-chain logic and returns the result
	QueryContract(ctx context.Context, signingKey string, location *fftypes.JSONAny, parsedMethod interface{}, input map[string]interface{}, options map[string]interface{}) (interface{}, error)

	// AddContractListener adds a new subscription to a user-specified contract and event
	AddContractListener(ctx context.Context, subscription *core.ContractListener, lastProtocolID string) error

	// DeleteContractListener deletes a previously-created subscription
	DeleteContractListener(ctx context.Context, subscription *core.ContractListener, okNotFound bool) error

	// GetContractListenerStatus gets the status of a contract listener from the backend connector. Returns false if not found
	GetContractListenerStatus(ctx context.Context, namespace, subID string, okNotFound bool) (bool, interface{}, core.ContractListenerStatus, error)

	// GetFFIParamValidator returns a blockchain-plugin-specific validator for FFIParams and their JSON Schema
	GetFFIParamValidator(ctx context.Context) (fftypes.FFIParamValidator, error)

	// GenerateFFI returns an FFI from a blockchain specific interface format e.g. an Ethereum ABI
	GenerateFFI(ctx context.Context, generationRequest *fftypes.FFIGenerationRequest) (*fftypes.FFI, error)

	// NormalizeContractLocation validates and normalizes the formatting of the location JSON
	NormalizeContractLocation(ctx context.Context, ntype NormalizeType, location *fftypes.JSONAny) (*fftypes.JSONAny, error)

	// GenerateEventSignature generates a strigified signature for the event, incorporating any fields significant to identifying the event as unique
	GenerateEventSignature(ctx context.Context, event *fftypes.FFIEventDefinition) (string, error)

	// GenerateEventSignatureWithLocation generates a strigified signature for the event , incorporating any fields significant to identifying the event as unique and the location
	GenerateEventSignatureWithLocation(ctx context.Context, event *fftypes.FFIEventDefinition, location *fftypes.JSONAny) (string, error)

	// CompareEventSignatures will compare both signatures return true if they overlap
	CheckOverlappingLocations(ctx context.Context, left *fftypes.JSONAny, right *fftypes.JSONAny) (bool, error)

	// GenerateErrorSignature generates a strigified signature for the custom error, incorporating any fields significant to identifying the error as unique
	GenerateErrorSignature(ctx context.Context, errorDef *fftypes.FFIErrorDefinition) string

	// GetNetworkVersion queries the provided contract to get the network version
	GetNetworkVersion(ctx context.Context, location *fftypes.JSONAny) (int, error)

	// GetAndConvertDeprecatedContractConfig converts the deprecated ethconnect config to a location object
	GetAndConvertDeprecatedContractConfig(ctx context.Context) (location *fftypes.JSONAny, fromBlock string, err error)

	// AddFireflySubscription creates a FireFly BatchPin subscription for the provided location
	AddFireflySubscription(ctx context.Context, namespace *core.Namespace, contract *MultipartyContract, lastProtocolID string) (subID string, err error)

	// RemoveFireFlySubscription removes the provided FireFly subscription
	RemoveFireflySubscription(ctx context.Context, subID string)

	// Get the latest status of the given transaction
	GetTransactionStatus(ctx context.Context, operation *core.Operation) (interface{}, error)
}

type NormalizeType int

const (
	NormalizeCall NormalizeType = iota
	NormalizeListener
)

const FireFlyActionPrefix = "firefly:"

type EventType int

const (
	EventTypeBatchPinComplete EventType = iota
	EventTypeNetworkAction
	EventTypeForListener
)

// BatchPinComplete notifies on the arrival of a sequenced batch of messages, which might have been
// submitted by us, or by any other authorized party in the network.
type BatchPinCompleteEvent struct {
	Namespace  string
	Batch      *BatchPin
	SigningKey *core.VerifierRef
}

// BlockchainNetworkAction notifies on the arrival of a network operator action
type NetworkActionEvent struct {
	Action     string
	Location   *fftypes.JSONAny
	Event      *Event
	SigningKey *core.VerifierRef
}

// EventForListener notifies on the arrival of any event from a user-created listener.
type EventForListener struct {
	*Event
	// ListenerID is the ID assigned to a custom contract listener by the connector
	ListenerID string
}

// EventToDispatch is a wrapper around the other event types, to allow them to be dispatched as a group
type EventToDispatch struct {
	Type             EventType
	BatchPinComplete *BatchPinCompleteEvent
	NetworkAction    *NetworkActionEvent
	ForListener      *EventForListener
}

// Callbacks is the interface provided to the blockchain plugin, to allow it to pass events back to firefly.
//
// Events must be delivered sequentially, such that event 2 is not delivered until the callback invoked for event 1
// has completed. However, it does not matter if these events are workload balance between the firefly core
// cluster instances of the node.
type Callbacks interface {
	// BlockchainEventBatch notifies of a sequential batch of blockchain events received. Batching allows efficiency
	// by grouping commits a the database level when processing these events.
	//
	// Errors are only returned in cases where the event appears valid, but a transient error has occurred that
	// means FireFly core is unable to process the event batch right now, and the events should be pushed
	// back to the connector for re-delivery. For example because the server is shutting down, or the namespace
	// is currently reloading.
	BlockchainEventBatch(batch []*EventToDispatch) error
}

// Capabilities the supported featureset of the blockchain
// interface implemented by the plugin, with the specified config
type Capabilities struct {
}

// MultipartyContract represents the location and configuration of a FireFly multiparty contract for batch pinning of messages
type MultipartyContract struct {
	Location   *fftypes.JSONAny
	FirstEvent string
	Options    *fftypes.JSONAny
}

// BatchPin is the set of data pinned to the blockchain for a batch - whether it's private or broadcast.
type BatchPin struct {

	// TransactionID is the FireFly transaction ID allocated before transaction submission for correlation with events (it's a UUID so no leakage)
	TransactionID *fftypes.UUID

	// TransactionType is the type of the FireFly transaction that initiated this BatchPin
	TransactionType core.TransactionType

	// BatchID is the id of the batch - not strictly required, but writing this in plain text to the blockchain makes for easy human correlation on-chain/off-chain (it's a UUID so no leakage)
	BatchID *fftypes.UUID

	// BatchHash is the SHA256 hash of the batch
	BatchHash *fftypes.Bytes32

	// BatchPayloadRef is a string that can be passed to to the storage interface to retrieve the payload. Nil for private messages
	BatchPayloadRef string

	// Contexts is an array of hashes that allow the FireFly runtimes to identify whether one of the messgages in
	// that batch is the next message for a sequence that involves that node. If so that means the FireFly runtime must
	//
	// - The primary subject of each hash is a "context"
	// - The context is a function of:
	//   - A single topic declared in a message - topics are just a string representing a sequence of events that must be processed in order
	//   - A ledger - everone with access to this ledger will see these hashes (Fabric channel, Ethereum chain, EEA privacy group, Corda linear ID)
	//   - A restricted group - if the mesage is private, these are the nodes that are eligible to receive a copy of the private message+data
	// - Each message might choose to include multiple topics (and hence attach to multiple contexts)
	//   - This allows multiple contexts to merge - very important in multi-party data matching scenarios
	// - A batch contains many messages, each with one or more topics
	//   - The array of sequence hashes will cover every unique context within the batch
	// - For private group communications, the hash is augmented as follow:
	//   - The hashes are salted with a UUID that is only passed off chain (the UUID of the Group).
	//   - The hashes are made unique to the sender
	//   - The hashes contain a sender specific nonce that is a monotonically increasing number
	//     for batches sent by that sender, within the context (maintined by the sender FireFly node)
	Contexts []*fftypes.Bytes32

	// Event contains info on the underlying blockchain event for this batch pin
	Event Event
}

type Event struct {
	// Source indicates where the event originated (ie plugin name)
	Source string

	// Name is a short name for the event
	Name string

	// ProtocolID is an alphanumerically sortable string that represents this event uniquely on the blockchain
	ProtocolID string

	// Output is the raw output data from the event
	Output fftypes.JSONObject

	// Info is any additional blockchain info for the event (transaction hash, block number, etc)
	Info fftypes.JSONObject

	// Timestamp is the time the event was emitted from the blockchain
	Timestamp *fftypes.FFTime

	// We capture the blockchain TXID as in the case
	// of a FireFly transaction we want to reflect that blockchain TX back onto the FireFly TX object
	BlockchainTXID string

	// Location is the blockchain location of the contract that emitted the event
	Location string

	// Signature is the event signature, including the event name and output types
	Signature string
}
