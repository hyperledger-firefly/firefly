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

package database

import (
	"context"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

var (
	// HashMismatch sentinel error
	HashMismatch = i18n.NewError(context.Background(), coremsgs.MsgHashMismatch)
	// IDMismatch sentinel error
	IDMismatch = i18n.NewError(context.Background(), coremsgs.MsgIDMismatch)
	// DeleteRecordNotFound sentinel error
	DeleteRecordNotFound = i18n.NewError(context.Background(), coremsgs.Msg404NotFound)
)

type UpsertOptimization int

const (
	UpsertOptimizationSkip UpsertOptimization = iota
	UpsertOptimizationNew
	UpsertOptimizationExisting
)

const (
	// Pseudo-namespace to register a global callback handler, which will receive all namespaced and non-namespaced events
	GlobalHandler = "ff:global"
)

// Plugin is the interface implemented by each plugin
type Plugin interface {
	PersistenceInterface // Split out to aid pluggability the next level down (SQL provider etc.)

	// InitConfig initializes the set of configuration options that are valid, with defaults. Called on all plugins.
	InitConfig(config config.Section)

	// Init initializes the plugin, with configuration
	Init(ctx context.Context, config config.Section) error

	// SetHandler registers a handler to receive callbacks
	// Plugin will attempt (but is not guaranteed) to deliver events only for the given namespace
	SetHandler(namespace string, handler Callbacks)

	// Capabilities returns capabilities - not called until after Init
	Capabilities() *Capabilities
}

type iNamespaceCollection interface {
	// UpsertNamespace - Upsert a namespace
	UpsertNamespace(ctx context.Context, data *core.Namespace, allowExisting bool) (err error)

	// GetNamespace - Get an namespace by name
	GetNamespace(ctx context.Context, name string) (namespace *core.Namespace, err error)
}

type iMessageCollection interface {
	// UpsertMessage - Upsert a message, with all the embedded data references.
	//                 The database layer must ensure that if a record already exists, the hash of that existing record
	//                 must match the hash of the record that is being inserted.
	UpsertMessage(ctx context.Context, message *core.Message, optimization UpsertOptimization, hooks ...PostCompletionHook) (err error)

	// InsertMessages performs a batch insert of messages assured to be new records - fails if they already exist, so caller can fall back to upsert individually
	InsertMessages(ctx context.Context, messages []*core.Message, hooks ...PostCompletionHook) (err error)

	// UpdateMessage - Update message
	UpdateMessage(ctx context.Context, namespace string, id *fftypes.UUID, update ffapi.Update) (err error)

	// ReplaceMessage updates the message, and assigns it a new sequence number at the front of the list.
	// A new event is raised for the message, with the new sequence number - as if it was brand new.
	ReplaceMessage(ctx context.Context, message *core.Message) (err error)

	// UpdateMessages - Update messages
	UpdateMessages(ctx context.Context, namespace string, filter ffapi.Filter, update ffapi.Update) (err error)

	// GetMessageByID - Get a message by ID
	GetMessageByID(ctx context.Context, namespace string, id *fftypes.UUID) (message *core.Message, err error)

	// GetMessages - List messages, reverse sorted (newest first) by Confirmed then Created, with pagination, and simple must filters
	GetMessages(ctx context.Context, namespace string, filter ffapi.Filter) (message []*core.Message, res *ffapi.FilterResult, err error)

	// GetMessageIDs - Retrieves messages, but only querying the messages ID (no other fields)
	GetMessageIDs(ctx context.Context, namespace string, filter ffapi.Filter) (ids []*core.IDAndSequence, err error)

	// GetMessagesForData - List messages where there is a data reference to the specified ID
	GetMessagesForData(ctx context.Context, namespace string, dataID *fftypes.UUID, filter ffapi.Filter) (message []*core.Message, res *ffapi.FilterResult, err error)

	// GetBatchIDsForMessages - an optimized query to retrieve any non-null batch IDs for a list of message IDs
	GetBatchIDsForMessages(ctx context.Context, namespace string, msgIDs []*fftypes.UUID) (batchIDs []*fftypes.UUID, err error)

	// GetBatchIDsForDataAttachments - an optimized query to retrieve any non-null batch IDs for a list of data IDs that might be attached to messages in batches
	GetBatchIDsForDataAttachments(ctx context.Context, namespace string, dataIDs []*fftypes.UUID) (batchIDs []*fftypes.UUID, err error)
}

type iDataCollection interface {
	// UpsertData - Upsert a data record. A hint can be supplied to whether the data already exists.
	//              The database layer must ensure that if a record already exists, the hash of that existing record
	//              must match the hash of the record that is being inserted.
	UpsertData(ctx context.Context, data *core.Data, optimization UpsertOptimization) (err error)

	// InsertDataArray performs a batch insert of data assured to be new records - fails if they already exist, so caller can fall back to upsert individually
	InsertDataArray(ctx context.Context, data core.DataArray) (err error)

	// UpdateData - Update data
	UpdateData(ctx context.Context, namespace string, id *fftypes.UUID, update ffapi.Update) (err error)

	// GetDataByID - Get a data record by ID
	GetDataByID(ctx context.Context, namespace string, id *fftypes.UUID, withValue bool) (message *core.Data, err error)

	// GetData - Get data
	GetData(ctx context.Context, namespace string, filter ffapi.Filter) (message core.DataArray, res *ffapi.FilterResult, err error)

	// GetDataSubPaths - returns unique paths that have files in them, under the specified path.
	// Requires DB specific processing of the blob.path field.
	GetDataSubPaths(ctx context.Context, namespace, path string) (subPaths []string, err error)

	// GetDataRefs - Get data references only (no data)
	GetDataRefs(ctx context.Context, namespace string, filter ffapi.Filter) (message core.DataRefs, res *ffapi.FilterResult, err error)

	// DeleteData - Deletes a data record by ID
	DeleteData(ctx context.Context, namespace string, id *fftypes.UUID) (err error)
}

type iBatchCollection interface {
	// InsertBatch - Insert a new batch, or retrieve the existing one if it has already been recorded
	InsertOrGetBatch(ctx context.Context, data *core.BatchPersisted) (existing *core.BatchPersisted, err error)

	// UpdateBatch - Update a batch
	UpdateBatch(ctx context.Context, namespace string, id *fftypes.UUID, update ffapi.Update) (err error)

	// GetBatchByID - Get a batch by ID
	GetBatchByID(ctx context.Context, namespace string, id *fftypes.UUID) (message *core.BatchPersisted, err error)

	// GetBatches - Get batches
	GetBatches(ctx context.Context, namespace string, filter ffapi.Filter) (message []*core.BatchPersisted, res *ffapi.FilterResult, err error)
}

type iTransactionCollection interface {
	// InsertTransaction - Insert a new transaction
	InsertTransaction(ctx context.Context, txn *core.Transaction) (err error)

	// InsertTransactions performs a batch insert of transactions - returns error if idempotency keys clash while inserting the non-clashing ones, so caller can query to find the existing ones
	InsertTransactions(ctx context.Context, txns []*core.Transaction) (err error)

	// UpdateTransaction - Update transaction
	UpdateTransaction(ctx context.Context, namespace string, id *fftypes.UUID, update ffapi.Update) (err error)

	// GetTransactionByID - Get a transaction by ID
	GetTransactionByID(ctx context.Context, namespace string, id *fftypes.UUID) (txn *core.Transaction, err error)

	// GetTransactions - Get transactions
	GetTransactions(ctx context.Context, namespace string, filter ffapi.Filter) (txn []*core.Transaction, res *ffapi.FilterResult, err error)
}

type iDatatypeCollection interface {
	// UpsertDatatype - Upsert a data definition
	UpsertDatatype(ctx context.Context, datadef *core.Datatype, allowExisting bool) (err error)

	// GetDatatypeByID - Get a data definition by ID
	GetDatatypeByID(ctx context.Context, namespace string, id *fftypes.UUID) (datadef *core.Datatype, err error)

	// GetDatatypeByName - Get a data definition by name
	GetDatatypeByName(ctx context.Context, namespace, name, version string) (datadef *core.Datatype, err error)

	// GetDatatypes - Get data definitions
	GetDatatypes(ctx context.Context, namespace string, filter ffapi.Filter) (datadef []*core.Datatype, res *ffapi.FilterResult, err error)
}

type iOffsetCollection interface {
	// UpsertOffset - Upsert an offset
	UpsertOffset(ctx context.Context, data *core.Offset, allowExisting bool) (err error)

	// UpdateOffset - Update offset
	UpdateOffset(ctx context.Context, rowID int64, update ffapi.Update) (err error)

	// GetOffset - Get an offset by name
	GetOffset(ctx context.Context, t core.OffsetType, name string) (offset *core.Offset, err error)

	// GetOffsets - Get offsets
	GetOffsets(ctx context.Context, filter ffapi.Filter) (offset []*core.Offset, res *ffapi.FilterResult, err error)

	// DeleteOffset - Delete an offset by name
	DeleteOffset(ctx context.Context, t core.OffsetType, name string) (err error)
}

type iPinCollection interface {
	// InsertPins - Inserts a list of pins - fails if they already exist, so caller can fall back to upsert individually
	InsertPins(ctx context.Context, pins []*core.Pin) (err error)

	// UpsertPin - Will insert a pin at the end of the sequence, unless the batch+hash+index sequence already exists
	UpsertPin(ctx context.Context, parked *core.Pin) (err error)

	// GetPins - Get pins
	GetPins(ctx context.Context, namespace string, filter ffapi.Filter) (offset []*core.Pin, res *ffapi.FilterResult, err error)

	// UpdatePins - Updates pins
	UpdatePins(ctx context.Context, namespace string, filter ffapi.Filter, update ffapi.Update) (err error)
}

type iOperationCollection interface {
	// InsertOperation - Insert an operation
	InsertOperation(ctx context.Context, operation *core.Operation, hooks ...PostCompletionHook) (err error)

	// InsertOperations bulk insert operations - all must succeed/fail together (idempotency clashes are handled by containing transaction)
	InsertOperations(ctx context.Context, ops []*core.Operation, hooks ...PostCompletionHook) (err error)

	// UpdateOperation - Update an operation
	UpdateOperation(ctx context.Context, namespace string, id *fftypes.UUID, filter ffapi.Filter, update ffapi.Update) (updated bool, err error)

	// GetOperationByID - Get an operation by ID
	GetOperationByID(ctx context.Context, namespace string, id *fftypes.UUID) (operation *core.Operation, err error)

	// GetOperations - Get operation
	GetOperations(ctx context.Context, namespace string, filter ffapi.Filter) (operation []*core.Operation, res *ffapi.FilterResult, err error)
}

type iSubscriptionCollection interface {
	// UpsertSubscription - Upsert a subscription
	UpsertSubscription(ctx context.Context, data *core.Subscription, allowExisting bool) (err error)

	// UpdateSubscription - Update subscription
	// Throws IDMismatch error if updating and ids don't match
	UpdateSubscription(ctx context.Context, namespace, name string, update ffapi.Update) (err error)

	// GetSubscriptionByName - Get an subscription by name
	GetSubscriptionByName(ctx context.Context, namespace, name string) (offset *core.Subscription, err error)

	// GetSubscriptionByID - Get an subscription by id
	GetSubscriptionByID(ctx context.Context, namespace string, id *fftypes.UUID) (offset *core.Subscription, err error)

	// GetSubscriptions - Get subscriptions
	GetSubscriptions(ctx context.Context, namespace string, filter ffapi.Filter) (offset []*core.Subscription, res *ffapi.FilterResult, err error)

	// DeleteSubscriptionByID - Delete a subscription
	DeleteSubscriptionByID(ctx context.Context, namespace string, id *fftypes.UUID) (err error)
}

type iEventCollection interface {
	// InsertEvent - Insert an event. The order of the sequences added to the database, must match the order that
	//               the rows/objects appear available to the event dispatcher. For a concurrency enabled database
	//               with multi-operation transactions (like PSQL or other enterprise SQL based DB) we need
	//               to hold an exclusive table lock.
	InsertEvent(ctx context.Context, data *core.Event) (err error)

	// GetEventByID - Get a event by ID
	GetEventByID(ctx context.Context, namespace string, id *fftypes.UUID) (message *core.Event, err error)

	// GetEvents - Get events
	GetEvents(ctx context.Context, namespace string, filter ffapi.Filter) (message []*core.Event, res *ffapi.FilterResult, err error)

	// GetEventsInSequenceRange - Get a range of events between 2 sequence values
	GetEventsInSequenceRange(ctx context.Context, namespace string, filter ffapi.Filter, startSequence int, endSequence int) (message []*core.Event, res *ffapi.FilterResult, err error)
}

type iIdentitiesCollection interface {
	// UpsertIdentity - Upsert an identity
	UpsertIdentity(ctx context.Context, data *core.Identity, optimization UpsertOptimization) (err error)

	// GetIdentityByDID - Get a identity by DID
	GetIdentityByDID(ctx context.Context, namespace, did string) (org *core.Identity, err error)

	// GetIdentityByName - Get a identity by name
	GetIdentityByName(ctx context.Context, iType core.IdentityType, namespace, name string) (org *core.Identity, err error)

	// GetIdentityByID - Get a identity by ID
	GetIdentityByID(ctx context.Context, namespace string, id *fftypes.UUID) (org *core.Identity, err error)

	// GetIdentities - Get identities
	GetIdentities(ctx context.Context, namespace string, filter ffapi.Filter) (org []*core.Identity, res *ffapi.FilterResult, err error)
}

type iVerifiersCollection interface {
	// UpsertVerifier - Upsert an verifier
	UpsertVerifier(ctx context.Context, data *core.Verifier, optimization UpsertOptimization) (err error)

	// GetVerifierByValue - Get a verifier by name
	GetVerifierByValue(ctx context.Context, vType core.VerifierType, namespace, value string) (org *core.Verifier, err error)

	// GetVerifierByHash - Get a verifier by its hash
	GetVerifierByHash(ctx context.Context, namespace string, hash *fftypes.Bytes32) (org *core.Verifier, err error)

	// GetVerifiers - Get verifiers
	GetVerifiers(ctx context.Context, namespace string, filter ffapi.Filter) (org []*core.Verifier, res *ffapi.FilterResult, err error)
}

type iGroupCollection interface {
	// UpsertGroup - Upsert a group, with a hint to whether to optmize for existing or new
	UpsertGroup(ctx context.Context, data *core.Group, optimization UpsertOptimization) (err error)

	// GetGroupByHash - Get a group by ID
	GetGroupByHash(ctx context.Context, namespace string, hash *fftypes.Bytes32) (node *core.Group, err error)

	// GetGroups - Get groups
	GetGroups(ctx context.Context, namespace string, filter ffapi.Filter) (node []*core.Group, res *ffapi.FilterResult, err error)
}

type iNonceCollection interface {
	// InsertNonce - Inserts a new nonce. Caller (batch processor) is responsible for ensuring it is the only active thread charge of assigning nonces to this context
	InsertNonce(ctx context.Context, nonce *core.Nonce) (err error)

	// UpdateNonce - Updates an existing nonce. Caller (batch processor) is responsible for ensuring it is the only active thread charge of assigning nonces to this context
	UpdateNonce(ctx context.Context, nonce *core.Nonce) (err error)

	// GetNonce - Get a context by hash
	GetNonce(ctx context.Context, hash *fftypes.Bytes32) (message *core.Nonce, err error)

	// GetNonces - Get contexts
	GetNonces(ctx context.Context, filter ffapi.Filter) (node []*core.Nonce, res *ffapi.FilterResult, err error)

	// DeleteNonce - Delete context by hash
	DeleteNonce(ctx context.Context, hash *fftypes.Bytes32) (err error)
}

type iNextPinCollection interface {
	// InsertNextPin - insert a nextpin
	InsertNextPin(ctx context.Context, nextpin *core.NextPin) (err error)

	// GetNextPins - get nextpins with generic filters
	GetNextPins(ctx context.Context, namespace string, filter ffapi.Filter) ([]*core.NextPin, *ffapi.FilterResult, error)

	// GetNextPins - get nextpins optimized for minimal processing getting by context
	GetNextPinsForContext(ctx context.Context, namespace string, context *fftypes.Bytes32) (message []*core.NextPin, err error)

	// UpdateNextPin - update a next hash using its local database ID
	UpdateNextPin(ctx context.Context, namespace string, sequence int64, update ffapi.Update) (err error)
}

type iBlobCollection interface {
	// InsertBlob - insert a blob
	InsertBlob(ctx context.Context, blob *core.Blob) (err error)

	// InsertBlobs performs a batch insert of blobs assured to be new records - fails if they already exist, so caller can fall back to upsert individually
	InsertBlobs(ctx context.Context, blobs []*core.Blob) (err error)

	// GetBlobs - get blobs
	GetBlobs(ctx context.Context, namespace string, filter ffapi.Filter) (message []*core.Blob, res *ffapi.FilterResult, err error)

	// DeleteBlob - delete a blob, using its local database ID
	DeleteBlob(ctx context.Context, sequence int64) (err error)
}

type iTokenPoolCollection interface {
	// InsertTokenPool - Insert a new token pool
	// If a pool with the same name has already been recorded, does not insert but returns the existing row
	InsertOrGetTokenPool(ctx context.Context, pool *core.TokenPool) (existing *core.TokenPool, err error)

	// UpsertTokenPool - Upsert a token pool
	UpsertTokenPool(ctx context.Context, pool *core.TokenPool, optimization UpsertOptimization) error

	// GetTokenPool - Get a token pool by name
	GetTokenPool(ctx context.Context, namespace, name string) (*core.TokenPool, error)

	// GetTokenPoolByID - Get a token pool by pool ID
	GetTokenPoolByID(ctx context.Context, namespace string, id *fftypes.UUID) (*core.TokenPool, error)

	// GetTokenPoolByNetworkName - Get a token pool by network name
	GetTokenPoolByNetworkName(ctx context.Context, namespace, networkName string) (*core.TokenPool, error)

	// GetTokenPools - Get token pools
	GetTokenPools(ctx context.Context, namespace string, filter ffapi.Filter) ([]*core.TokenPool, *ffapi.FilterResult, error)

	// DeleteTokenPool - delete a token pool
	DeleteTokenPool(ctx context.Context, namespace string, id *fftypes.UUID) error
}

type iTokenBalanceCollection interface {
	// UpdateTokenBalances - Move some token balance from one account to another
	UpdateTokenBalances(ctx context.Context, transfer *core.TokenTransfer) error

	// GetTokenBalance - Get a token balance by pool and account identity
	GetTokenBalance(ctx context.Context, namespace string, poolID *fftypes.UUID, tokenIndex, identity string) (*core.TokenBalance, error)

	// GetTokenBalances - Get token balances
	GetTokenBalances(ctx context.Context, namespace string, filter ffapi.Filter) ([]*core.TokenBalance, *ffapi.FilterResult, error)

	// GetTokenAccounts - Get token accounts (all distinct addresses that have a balance)
	GetTokenAccounts(ctx context.Context, namespace string, filter ffapi.Filter) ([]*core.TokenAccount, *ffapi.FilterResult, error)

	// GetTokenAccountPools - Get the list of pools referenced by a given account
	GetTokenAccountPools(ctx context.Context, namespace, key string, filter ffapi.Filter) ([]*core.TokenAccountPool, *ffapi.FilterResult, error)

	// DeleteTokenBalances - Delete token balances from a particular pool
	DeleteTokenBalances(ctx context.Context, namespace string, poolID *fftypes.UUID) error
}

type iTokenTransferCollection interface {
	// InsertOrGetTokenTransfer - insert a token transfer event from the blockchain
	// If the ProtocolID has already been recorded, it does not insert but returns the existing row
	InsertOrGetTokenTransfer(ctx context.Context, approval *core.TokenTransfer) (existing *core.TokenTransfer, err error)

	// GetTokenTransferByID - Get a token transfer by ID
	GetTokenTransferByID(ctx context.Context, namespace string, localID *fftypes.UUID) (*core.TokenTransfer, error)

	// GetTokenTransferByProtocolID - Get a token transfer by protocol ID
	GetTokenTransferByProtocolID(ctx context.Context, namespace string, poolID *fftypes.UUID, protocolID string) (*core.TokenTransfer, error)

	// GetTokenTransfers - Get token transfers
	GetTokenTransfers(ctx context.Context, namespace string, filter ffapi.Filter) ([]*core.TokenTransfer, *ffapi.FilterResult, error)

	// DeleteTokenTransfers - Delete token transfers from a particular pool
	DeleteTokenTransfers(ctx context.Context, namespace string, poolID *fftypes.UUID) error
}

type iTokenApprovalCollection interface {
	// UpsertTokenApproval - Upsert a token approval
	UpsertTokenApproval(ctx context.Context, approval *core.TokenApproval) error

	// UpdateTokenApprovals - Update multiple token approvals
	UpdateTokenApprovals(ctx context.Context, filter ffapi.Filter, update ffapi.Update) (err error)

	// GetTokenApprovalByID - Get a token approval by ID
	GetTokenApprovalByID(ctx context.Context, namespace string, localID *fftypes.UUID) (*core.TokenApproval, error)

	// GetTokenTransferByProtocolID - Get a token approval by protocol ID
	GetTokenApprovalByProtocolID(ctx context.Context, namespace string, poolID *fftypes.UUID, protocolID string) (*core.TokenApproval, error)

	// GetTokenApprovals - Get token approvals
	GetTokenApprovals(ctx context.Context, namespace string, filter ffapi.Filter) ([]*core.TokenApproval, *ffapi.FilterResult, error)

	// DeleteTokenApprovals - Delete token approvals from a particular pool
	DeleteTokenApprovals(ctx context.Context, namespace string, poolID *fftypes.UUID) error
}

type iFFICollection interface {
	// InsertOrGetFFI - Insert an FFI
	// If an FFI with the same name has already been recorded, does not insert but returns the existing row
	InsertOrGetFFI(ctx context.Context, ffi *fftypes.FFI) (*fftypes.FFI, error)

	// UpsertFFI - Upsert an FFI
	UpsertFFI(ctx context.Context, ffi *fftypes.FFI, optimization UpsertOptimization) error

	// GetFFIs - Get FFIs
	GetFFIs(ctx context.Context, namespace string, filter ffapi.Filter) ([]*fftypes.FFI, *ffapi.FilterResult, error)

	// GetFFIByID - Get an FFI by ID
	GetFFIByID(ctx context.Context, namespace string, id *fftypes.UUID) (*fftypes.FFI, error)

	// GetFFI - Get an FFI by name and version
	GetFFI(ctx context.Context, namespace, name, version string) (*fftypes.FFI, error)

	// GetFFIByNetworkName - Get an FFI by network name and version
	GetFFIByNetworkName(ctx context.Context, namespace, networkName, version string) (*fftypes.FFI, error)

	// DeleteFFI - Delete an FFI
	DeleteFFI(ctx context.Context, namespace string, id *fftypes.UUID) error
}

type iFFIMethodCollection interface {
	// UpsertFFIMethod - Upsert an FFI method
	UpsertFFIMethod(ctx context.Context, method *fftypes.FFIMethod) error

	// GetFFIMethod - Get an FFI method by path
	GetFFIMethod(ctx context.Context, namespace string, interfaceID *fftypes.UUID, pathName string) (*fftypes.FFIMethod, error)

	// GetFFIMethods - Get FFI methods
	GetFFIMethods(ctx context.Context, namespace string, filter ffapi.Filter) (methods []*fftypes.FFIMethod, res *ffapi.FilterResult, err error)
}

type iFFIEventCollection interface {
	// UpsertFFIEvent - Upsert an FFI event
	UpsertFFIEvent(ctx context.Context, method *fftypes.FFIEvent) error

	// GetFFIEvent - Get an FFI event by path
	GetFFIEvent(ctx context.Context, namespace string, interfaceID *fftypes.UUID, pathName string) (*fftypes.FFIEvent, error)

	// GetFFIEvents - Get FFI events
	GetFFIEvents(ctx context.Context, namespace string, filter ffapi.Filter) (events []*fftypes.FFIEvent, res *ffapi.FilterResult, err error)
}

type iFFIErrorCollection interface {
	// UpsertFFIError - Upsert an FFI error
	UpsertFFIError(ctx context.Context, method *fftypes.FFIError) error

	// GetFFIErrors - Get FFI error
	GetFFIErrors(ctx context.Context, namespace string, filter ffapi.Filter) (errors []*fftypes.FFIError, res *ffapi.FilterResult, err error)
}

type iContractAPICollection interface {
	// InsertOrGetContractAPI - Insert a contract API
	// If an API with the same name has already been recorded, does not insert but returns the existing row
	InsertOrGetContractAPI(ctx context.Context, api *core.ContractAPI) (*core.ContractAPI, error)

	// UpsertFFIEvent - Upsert a contract API
	UpsertContractAPI(ctx context.Context, api *core.ContractAPI, optimization UpsertOptimization) error

	// GetContractAPIs - Get contract APIs
	GetContractAPIs(ctx context.Context, namespace string, filter ffapi.AndFilter) ([]*core.ContractAPI, *ffapi.FilterResult, error)

	// GetContractAPIByID - Get a contract API by ID
	GetContractAPIByID(ctx context.Context, namespace string, id *fftypes.UUID) (*core.ContractAPI, error)

	// GetContractAPIByName - Get a contract API by name
	GetContractAPIByName(ctx context.Context, namespace, name string) (*core.ContractAPI, error)

	// GetContractAPIByNetworkName - Get a contract API by network name
	GetContractAPIByNetworkName(ctx context.Context, namespace, networkName string) (*core.ContractAPI, error)

	// DeleteContractAPI - Delete a contract API
	DeleteContractAPI(ctx context.Context, namespace string, id *fftypes.UUID) error
}

type iContractListenerCollection interface {
	// InsertContractListener - insert a listener to an external smart contract
	InsertContractListener(ctx context.Context, sub *core.ContractListener) (err error)

	// UpsertContractListener - upsert a listener to an external smart contract
	UpsertContractListener(ctx context.Context, sub *core.ContractListener, allowExisting bool) (err error)

	// UpdateContractListener - update contract listener by id
	UpdateContractListener(ctx context.Context, namespace string, id *fftypes.UUID, update ffapi.Update) (err error)

	// GetContractListener - get contract listener by name
	GetContractListener(ctx context.Context, namespace, name string) (sub *core.ContractListener, err error)

	// GetContractListenerByID - get contract listener by ID
	GetContractListenerByID(ctx context.Context, namespace string, id *fftypes.UUID) (sub *core.ContractListener, err error)

	// GetContractListenerByBackendID - get contract listener by backend ID
	GetContractListenerByBackendID(ctx context.Context, namespace, id string) (sub *core.ContractListener, err error)

	// GetContractListeners - get contract listeners
	GetContractListeners(ctx context.Context, namespace string, filter ffapi.Filter) ([]*core.ContractListener, *ffapi.FilterResult, error)

	// DeleteContractListener - delete a contract listener
	DeleteContractListenerByID(ctx context.Context, namespace string, id *fftypes.UUID) (err error)
}

type iBlockchainEventCollection interface {

	// InsertBlockchainEvents performs a batch insert of blockchain events - fails if they already exist, so caller can fall back to InsertOrGetBlockchainEvent individually
	InsertBlockchainEvents(ctx context.Context, messages []*core.BlockchainEvent, hooks ...PostCompletionHook) (err error)

	// InsertOrGetBlockchainEvent - insert an event from the blockchain
	// If the ProtocolID has already been recorded, it does not insert but returns the existing row
	InsertOrGetBlockchainEvent(ctx context.Context, event *core.BlockchainEvent) (existing *core.BlockchainEvent, err error)

	// GetBlockchainEventByID - get blockchain event by ID
	GetBlockchainEventByID(ctx context.Context, namespace string, id *fftypes.UUID) (*core.BlockchainEvent, error)

	// GetBlockchainEventByID - get blockchain event by protocol ID
	GetBlockchainEventByProtocolID(ctx context.Context, namespace string, listener *fftypes.UUID, protocolID string) (*core.BlockchainEvent, error)

	// GetBlockchainEvents - get blockchain events
	GetBlockchainEvents(ctx context.Context, namespace string, filter ffapi.Filter) ([]*core.BlockchainEvent, *ffapi.FilterResult, error)
}

// PersistenceInterface are the operations that must be implemented by a database interface plugin.
type iChartCollection interface {
	// GetChartHistogram - Get charting data for a histogram
	GetChartHistogram(ctx context.Context, namespace string, intervals []core.ChartHistogramInterval, collection CollectionName) ([]*core.ChartHistogram, error)
}

// PeristenceInterface are the operations that must be implemented by a database interface plugin.
// The database mechanism of Firefly is designed to provide the balance between being able
// to query the data a member of the network has transferred/received via Firefly efficiently,
// while not trying to become the core database of the application (where full deeply nested
// rich query is needed).
//
// This means that we treat business data as opaque within the storage, only verifying it against
// a data definition within the Firefly core runtime itself.
// The data types, indexes and relationships are designed to be simple, and map closely to the
// REST semantics of the Firefly API itself.
//
// As a result, the database interface could be implemented efficiently by most database technologies.
// Including both Relational/SQL and Document/NoSQL database technologies.
//
// As such we suggest the factors in choosing your database should be non-functional, such as:
// - Which provides you with the HA/DR capabilities you require
// - Which is most familiar within your existing devops pipeline for the application
// - Whether you can consolidate the HA/DR and server infrastructure for your app DB with the Firefly DB
//
// Each database does need an update to the core codebase, to provide a plugin that implements this
// interface.
// For SQL databases the process of adding a new database is simplified via the common SQL layer.
// For NoSQL databases, the code should be straight forward to map the collections, indexes, and operations.
type PersistenceInterface interface {
	core.Named

	// RunAsGroup instructs the database plugin that all database operations performed within the context
	// function can be grouped into a single transaction (if supported).
	// Requirements:
	// - Firefly must not depend on this to guarantee ACID properties (it is only a suggestion/optimization)
	// - The database implementation must support nested RunAsGroup calls (ie by reusing a transaction if one exists)
	// - The caller is responsible for passing the supplied context to all database operations within the callback function
	RunAsGroup(ctx context.Context, fn func(ctx context.Context) error) error

	iNamespaceCollection
	iMessageCollection
	iDataCollection
	iBatchCollection
	iTransactionCollection
	iDatatypeCollection
	iOffsetCollection
	iPinCollection
	iOperationCollection
	iSubscriptionCollection
	iEventCollection
	iIdentitiesCollection
	iVerifiersCollection
	iGroupCollection
	iNonceCollection
	iNextPinCollection
	iBlobCollection
	iTokenPoolCollection
	iTokenBalanceCollection
	iTokenTransferCollection
	iTokenApprovalCollection
	iFFICollection
	iFFIMethodCollection
	iFFIEventCollection
	iFFIErrorCollection
	iContractAPICollection
	iContractListenerCollection
	iBlockchainEventCollection
	iChartCollection
}

// CollectionName represents all collections
type CollectionName string

// OrderedUUIDCollectionNS collections have a strong order that includes a sequence integer
// that uniquely identifies the entry in a sequence. The sequence is LOCAL to this
// FireFly node. We try to minimize adding new collections of this type, as they have
// implementation complexity in some databases (such as NoSQL databases)
type OrderedUUIDCollectionNS CollectionName

const (
	CollectionMessages OrderedUUIDCollectionNS = "messages"
	CollectionEvents   OrderedUUIDCollectionNS = "events"
)

// OrderedCollectionNS is a collection that is ordered, and that sequence is the only key
type OrderedCollectionNS CollectionName

const (
	CollectionPins OrderedCollectionNS = "pins"
)

// UUIDCollectionNS is the most common type of collection - each entry has a UUID that
// is globally unique, and used externally by apps to address entries in the collection.
// Objects in these collections are all namespaced,.
type UUIDCollectionNS CollectionName

const (
	CollectionBatches           UUIDCollectionNS = "batches"
	CollectionBlockchainEvents  UUIDCollectionNS = "blockchainevents"
	CollectionData              UUIDCollectionNS = "data"
	CollectionDataTypes         UUIDCollectionNS = "datatypes"
	CollectionOperations        UUIDCollectionNS = "operations"
	CollectionSubscriptions     UUIDCollectionNS = "subscriptions"
	CollectionTransactions      UUIDCollectionNS = "transactions"
	CollectionTokenPools        UUIDCollectionNS = "tokenpools"
	CollectionTokenTransfers    UUIDCollectionNS = "tokentransfers"
	CollectionTokenApprovals    UUIDCollectionNS = "tokenapprovals"
	CollectionFFIs              UUIDCollectionNS = "ffi"
	CollectionFFIMethods        UUIDCollectionNS = "ffimethods"
	CollectionFFIEvents         UUIDCollectionNS = "ffievents"
	CollectionFFIErrors         UUIDCollectionNS = "ffierrors"
	CollectionContractAPIs      UUIDCollectionNS = "contractapis"
	CollectionContractListeners UUIDCollectionNS = "contractlisteners"
	CollectionIdentities        UUIDCollectionNS = "identities"
)

// HashCollectionNS is a collection where the primary key is a hash, such that it can
// by identified by any member of the network at any time, without it first having
// been broadcast.
type HashCollectionNS CollectionName

const (
	CollectionGroups    HashCollectionNS = "groups"
	CollectionVerifiers HashCollectionNS = "verifiers"
)

// OtherCollection are odd balls, that don't fit any of the categories above.
// These collections do not support change events, and generally their
// creation is coordinated with creation of another object that does support change events.
// Mainly they are entries that require lookup by compound IDs.
type OtherCollection CollectionName

const (
	CollectionBlobs         OtherCollection = "blobs"
	CollectionNextpins      OtherCollection = "nextpins"
	CollectionNonces        OtherCollection = "nonces"
	CollectionOffsets       OtherCollection = "offsets"
	CollectionTokenBalances OtherCollection = "tokenbalances"
)

// PostCompletionHook is a closure/function that will be called after a successful insertion.
// This includes where the insert is nested in a RunAsGroup, and the database is transactional.
// These hooks are useful when triggering code that relies on the inserted database object being available.
type PostCompletionHook func()

// Callbacks are the methods for passing data from plugin to core
//
// If Capabilities returns ClusterEvents=true then these should be broadcast to every instance within
// a cluster that is connected to the database.
//
// If Capabilities returns ClusterEvents=false then these events can be simply coupled in-process to
// update activities.
//
// The system does not rely on these events exclusively for data/transaction integrity, but if an event is
// missed/delayed it might result in slower processing.
// For example, the batch interface will initiate a batch as soon as an event is triggered, but it will use
// a subsequent database query as the source of truth of the latest set/order of data, and it will periodically
// check for new messages even if it does not receive any events.
//
// Events are emitted locally to the individual FireFly core process. However, a WebSocket interface is
// available for remote listening to these events. That allows the UI to listen to the events, as well as
// providing a building block for a cluster of FireFly servers to directly propgate events to each other.
type Callbacks interface {
	// OrderedUUIDCollectionNSEvent emits the sequence on insert, but it will be -1 on update
	OrderedUUIDCollectionNSEvent(resType OrderedUUIDCollectionNS, eventType core.ChangeEventType, namespace string, id *fftypes.UUID, sequence int64)
	OrderedCollectionNSEvent(resType OrderedCollectionNS, eventType core.ChangeEventType, namespace string, sequence int64)
	UUIDCollectionNSEvent(resType UUIDCollectionNS, eventType core.ChangeEventType, namespace string, id *fftypes.UUID)
	HashCollectionNSEvent(resType HashCollectionNS, eventType core.ChangeEventType, namespace string, hash *fftypes.Bytes32)
}

// Capabilities defines the capabilities a plugin can report as implementing or not
type Capabilities struct {
	Concurrency bool
}

// MessageQueryFactory filter fields for messages
var MessageQueryFactory = &ffapi.QueryFields{
	"id":             &ffapi.UUIDField{},
	"cid":            &ffapi.UUIDField{},
	"type":           &ffapi.StringField{},
	"author":         &ffapi.StringField{},
	"key":            &ffapi.StringField{},
	"topics":         &ffapi.FFStringArrayField{},
	"tag":            &ffapi.StringField{},
	"group":          &ffapi.Bytes32Field{},
	"created":        &ffapi.TimeField{},
	"datahash":       &ffapi.Bytes32Field{},
	"idempotencykey": &ffapi.StringField{},
	"hash":           &ffapi.Bytes32Field{},
	"pins":           &ffapi.FFStringArrayField{},
	"state":          &ffapi.StringField{},
	"confirmed":      &ffapi.TimeField{},
	"rejectreason":   &ffapi.StringField{},
	"sequence":       &ffapi.Int64Field{},
	"txtype":         &ffapi.StringField{},
	"batch":          &ffapi.UUIDField{},
	"txid":           &ffapi.UUIDField{},
	"txparent.type":  &ffapi.StringField{},
	"txparent.id":    &ffapi.UUIDField{},
}

// BatchQueryFactory filter fields for batches
var BatchQueryFactory = &ffapi.QueryFields{
	"id":         &ffapi.UUIDField{},
	"type":       &ffapi.StringField{},
	"author":     &ffapi.StringField{},
	"key":        &ffapi.StringField{},
	"group":      &ffapi.Bytes32Field{},
	"hash":       &ffapi.Bytes32Field{},
	"payloadref": &ffapi.StringField{},
	"created":    &ffapi.TimeField{},
	"confirmed":  &ffapi.TimeField{},
	"tx.type":    &ffapi.StringField{},
	"tx.id":      &ffapi.UUIDField{},
	"node":       &ffapi.UUIDField{},
}

// TransactionQueryFactory filter fields for transactions
var TransactionQueryFactory = &ffapi.QueryFields{
	"id":             &ffapi.UUIDField{},
	"type":           &ffapi.StringField{},
	"created":        &ffapi.TimeField{},
	"idempotencykey": &ffapi.StringField{},
	"blockchainids":  &ffapi.FFStringArrayField{},
}

// DataQueryFactory filter fields for data
var DataQueryFactory = &ffapi.QueryFields{
	"id":               &ffapi.UUIDField{},
	"validator":        &ffapi.StringField{},
	"datatype.name":    &ffapi.StringField{},
	"datatype.version": &ffapi.StringField{},
	"hash":             &ffapi.Bytes32Field{},
	"blob.hash":        &ffapi.Bytes32Field{},
	"blob.public":      &ffapi.StringField{},
	"blob.name":        &ffapi.StringField{},
	"blob.path":        &ffapi.StringField{},
	"blob.size":        &ffapi.Int64Field{},
	"created":          &ffapi.TimeField{},
	"value":            &ffapi.JSONField{},
	"public":           &ffapi.StringField{},
}

// DatatypeQueryFactory filter fields for data definitions
var DatatypeQueryFactory = &ffapi.QueryFields{
	"id":        &ffapi.UUIDField{},
	"message":   &ffapi.UUIDField{},
	"validator": &ffapi.StringField{},
	"name":      &ffapi.StringField{},
	"version":   &ffapi.StringField{},
	"created":   &ffapi.TimeField{},
}

// OffsetQueryFactory filter fields for data offsets
var OffsetQueryFactory = &ffapi.QueryFields{
	"name":    &ffapi.StringField{},
	"type":    &ffapi.StringField{},
	"current": &ffapi.Int64Field{},
}

// OperationQueryFactory filter fields for data operations
var OperationQueryFactory = &ffapi.QueryFields{
	"id":      &ffapi.UUIDField{},
	"tx":      &ffapi.UUIDField{},
	"type":    &ffapi.StringField{},
	"status":  &ffapi.StringField{},
	"error":   &ffapi.StringField{},
	"plugin":  &ffapi.StringField{},
	"input":   &ffapi.JSONField{},
	"output":  &ffapi.JSONField{},
	"created": &ffapi.TimeField{},
	"updated": &ffapi.TimeField{},
	"retry":   &ffapi.UUIDField{},
}

// SubscriptionQueryFactory filter fields for data subscriptions
var SubscriptionQueryFactory = &ffapi.QueryFields{
	"id":        &ffapi.UUIDField{},
	"name":      &ffapi.StringField{},
	"transport": &ffapi.StringField{},
	"events":    &ffapi.StringField{},
	"filters":   &ffapi.JSONField{},
	"options":   &ffapi.StringField{},
	"created":   &ffapi.TimeField{},
}

// EventQueryFactory filter fields for data events
var EventQueryFactory = &ffapi.QueryFields{
	"id":         &ffapi.UUIDField{},
	"type":       &ffapi.StringField{},
	"reference":  &ffapi.UUIDField{},
	"correlator": &ffapi.UUIDField{},
	"tx":         &ffapi.UUIDField{},
	"topic":      &ffapi.StringField{},
	"sequence":   &ffapi.Int64Field{},
	"created":    &ffapi.TimeField{},
}

// PinQueryFactory filter fields for parked contexts
var PinQueryFactory = &ffapi.QueryFields{
	"sequence":   &ffapi.Int64Field{},
	"masked":     &ffapi.BoolField{},
	"hash":       &ffapi.Bytes32Field{},
	"batch":      &ffapi.UUIDField{},
	"index":      &ffapi.Int64Field{},
	"dispatched": &ffapi.BoolField{},
	"created":    &ffapi.TimeField{},
}

// IdentityQueryFactory filter fields for identities
var IdentityQueryFactory = &ffapi.QueryFields{
	"id":                    &ffapi.UUIDField{},
	"did":                   &ffapi.StringField{},
	"parent":                &ffapi.UUIDField{},
	"messages.claim":        &ffapi.UUIDField{},
	"messages.verification": &ffapi.UUIDField{},
	"messages.update":       &ffapi.UUIDField{},
	"type":                  &ffapi.StringField{},
	"name":                  &ffapi.StringField{},
	"description":           &ffapi.StringField{},
	"profile":               &ffapi.JSONField{},
	"created":               &ffapi.TimeField{},
	"updated":               &ffapi.TimeField{},
}

// VerifierQueryFactory filter fields for identities
var VerifierQueryFactory = &ffapi.QueryFields{
	"hash":     &ffapi.Bytes32Field{},
	"identity": &ffapi.UUIDField{},
	"type":     &ffapi.StringField{},
	"value":    &ffapi.StringField{},
	"created":  &ffapi.TimeField{},
}

// GroupQueryFactory filter fields for groups
var GroupQueryFactory = &ffapi.QueryFields{
	"hash":        &ffapi.Bytes32Field{},
	"message":     &ffapi.UUIDField{},
	"description": &ffapi.StringField{},
	"ledger":      &ffapi.UUIDField{},
	"created":     &ffapi.TimeField{},
}

// NonceQueryFactory filter fields for nonces
var NonceQueryFactory = &ffapi.QueryFields{
	"hash":  &ffapi.StringField{},
	"nonce": &ffapi.Int64Field{},
}

// NextPinQueryFactory filter fields for next pins
var NextPinQueryFactory = &ffapi.QueryFields{
	"context":  &ffapi.Bytes32Field{},
	"identity": &ffapi.StringField{},
	"hash":     &ffapi.Bytes32Field{},
	"nonce":    &ffapi.Int64Field{},
}

// BlobQueryFactory filter fields for config records
var BlobQueryFactory = &ffapi.QueryFields{
	"hash":       &ffapi.Bytes32Field{},
	"size":       &ffapi.Int64Field{},
	"payloadref": &ffapi.StringField{},
	"created":    &ffapi.TimeField{},
	"data_id":    &ffapi.UUIDField{},
}

// TokenPoolQueryFactory filter fields for token pools
var TokenPoolQueryFactory = &ffapi.QueryFields{
	"id":              &ffapi.UUIDField{},
	"type":            &ffapi.StringField{},
	"name":            &ffapi.StringField{},
	"networkname":     &ffapi.StringField{},
	"standard":        &ffapi.StringField{},
	"locator":         &ffapi.StringField{},
	"symbol":          &ffapi.StringField{},
	"decimals":        &ffapi.Int64Field{},
	"message":         &ffapi.UUIDField{},
	"active":          &ffapi.BoolField{},
	"created":         &ffapi.TimeField{},
	"connector":       &ffapi.StringField{},
	"tx.type":         &ffapi.StringField{},
	"tx.id":           &ffapi.UUIDField{},
	"interface":       &ffapi.UUIDField{},
	"interfaceformat": &ffapi.StringField{},
	"published":       &ffapi.BoolField{},
}

// TokenBalanceQueryFactory filter fields for token balances
var TokenBalanceQueryFactory = &ffapi.QueryFields{
	"pool":       &ffapi.UUIDField{},
	"tokenindex": &ffapi.StringField{},
	"uri":        &ffapi.StringField{},
	"connector":  &ffapi.StringField{},
	"key":        &ffapi.StringField{},
	"balance":    &ffapi.Int64Field{},
	"updated":    &ffapi.TimeField{},
}

// TokenAccountQueryFactory filter fields for token accounts
var TokenAccountQueryFactory = &ffapi.QueryFields{
	"key":     &ffapi.StringField{},
	"updated": &ffapi.TimeField{},
}

// TokenAccountPoolQueryFactory filter fields for token account pools
var TokenAccountPoolQueryFactory = &ffapi.QueryFields{
	"pool":    &ffapi.UUIDField{},
	"updated": &ffapi.TimeField{},
}

// TokenTransferQueryFactory filter fields for token transfers
var TokenTransferQueryFactory = &ffapi.QueryFields{
	"localid":         &ffapi.StringField{},
	"pool":            &ffapi.UUIDField{},
	"tokenindex":      &ffapi.StringField{},
	"uri":             &ffapi.StringField{},
	"connector":       &ffapi.StringField{},
	"key":             &ffapi.StringField{},
	"from":            &ffapi.StringField{},
	"to":              &ffapi.StringField{},
	"amount":          &ffapi.Int64Field{},
	"protocolid":      &ffapi.StringField{},
	"message":         &ffapi.UUIDField{},
	"messagehash":     &ffapi.Bytes32Field{},
	"created":         &ffapi.TimeField{},
	"tx.type":         &ffapi.StringField{},
	"tx.id":           &ffapi.UUIDField{},
	"blockchainevent": &ffapi.UUIDField{},
	"type":            &ffapi.StringField{},
}

var TokenApprovalQueryFactory = &ffapi.QueryFields{
	"localid":         &ffapi.StringField{},
	"pool":            &ffapi.UUIDField{},
	"connector":       &ffapi.StringField{},
	"key":             &ffapi.StringField{},
	"operator":        &ffapi.StringField{},
	"approved":        &ffapi.BoolField{},
	"protocolid":      &ffapi.StringField{},
	"subject":         &ffapi.StringField{},
	"active":          &ffapi.BoolField{},
	"created":         &ffapi.TimeField{},
	"tx.type":         &ffapi.StringField{},
	"tx.id":           &ffapi.UUIDField{},
	"blockchainevent": &ffapi.UUIDField{},
	"message":         &ffapi.UUIDField{},
	"messagehash":     &ffapi.Bytes32Field{},
}

// FFIQueryFactory filter fields for contract definitions
var FFIQueryFactory = &ffapi.QueryFields{
	"id":          &ffapi.UUIDField{},
	"name":        &ffapi.StringField{},
	"networkname": &ffapi.StringField{},
	"version":     &ffapi.StringField{},
	"published":   &ffapi.BoolField{},
}

// FFIMethodQueryFactory filter fields for contract methods
var FFIMethodQueryFactory = &ffapi.QueryFields{
	"id":          &ffapi.UUIDField{},
	"name":        &ffapi.StringField{},
	"pathname":    &ffapi.StringField{},
	"interface":   &ffapi.UUIDField{},
	"description": &ffapi.StringField{},
}

// FFIEventQueryFactory filter fields for contract events
var FFIEventQueryFactory = &ffapi.QueryFields{
	"id":          &ffapi.UUIDField{},
	"name":        &ffapi.StringField{},
	"pathname":    &ffapi.StringField{},
	"interface":   &ffapi.UUIDField{},
	"description": &ffapi.StringField{},
}

// FFIErrorQueryFactory filter fields for contract errors
var FFIErrorQueryFactory = &ffapi.QueryFields{
	"id":          &ffapi.UUIDField{},
	"name":        &ffapi.StringField{},
	"pathname":    &ffapi.StringField{},
	"interface":   &ffapi.UUIDField{},
	"description": &ffapi.StringField{},
}

// ContractListenerQueryFactory filter fields for contract listeners
var ContractListenerQueryFactory = &ffapi.QueryFields{
	"id":        &ffapi.UUIDField{},
	"name":      &ffapi.StringField{},
	"interface": &ffapi.UUIDField{},
	"location":  &ffapi.JSONField{},
	"topic":     &ffapi.StringField{},
	"signature": &ffapi.StringField{},
	"backendid": &ffapi.StringField{},
	"created":   &ffapi.TimeField{},
	"updated":   &ffapi.TimeField{},
	"state":     &ffapi.JSONField{},
	"filters":   &ffapi.JSONField{},
}

// BlockchainEventQueryFactory filter fields for contract events
var BlockchainEventQueryFactory = &ffapi.QueryFields{
	"id":              &ffapi.UUIDField{},
	"source":          &ffapi.StringField{},
	"name":            &ffapi.StringField{},
	"protocolid":      &ffapi.StringField{},
	"listener":        &ffapi.StringField{},
	"tx.type":         &ffapi.StringField{},
	"tx.id":           &ffapi.UUIDField{},
	"tx.blockchainid": &ffapi.StringField{},
	"timestamp":       &ffapi.TimeField{},
}

// ContractAPIQueryFactory filter fields for Contract APIs
var ContractAPIQueryFactory = &ffapi.QueryFields{
	"id":          &ffapi.UUIDField{},
	"name":        &ffapi.StringField{},
	"networkname": &ffapi.StringField{},
	"interface":   &ffapi.UUIDField{},
	"published":   &ffapi.BoolField{},
}
