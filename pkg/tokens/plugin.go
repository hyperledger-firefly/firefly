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

package tokens

import (
	"context"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/blockchain"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

// Plugin is the interface implemented by each tokens plugin
type Plugin interface {
	core.Named

	// InitConfig initializes the set of configuration options that are valid, with defaults. Called on all plugins.
	InitConfig(config config.Section)

	// Init initializes the plugin, with configuration
	Init(ctx context.Context, cancelCtx context.CancelFunc, name string, config config.Section) error

	// SetHandler registers a handler to receive callbacks
	// Plugin will attempt (but is not guaranteed) to deliver events only for the given namespace
	SetHandler(namespace string, handler Callbacks)

	// SetOperationHandler registers a handler to receive async operation status
	// If namespace is set, plugin will attempt to deliver only events for that namespace
	SetOperationHandler(namespace string, handler core.OperationCallbacks)

	// StartNamespace starts a specific namespace within the plugin
	StartNamespace(ctx context.Context, namespace string, tokenPools []*core.TokenPool) error

	// StopNamespace removes a namespace from use within the plugin
	StopNamespace(ctx context.Context, namespace string) error

	// Capabilities returns capabilities - not called until after Init
	Capabilities() *Capabilities

	// ConnectorName returns the configured connector name (plugin instance)
	ConnectorName() string

	// CreateTokenPool creates a new (fungible or non-fungible) pool of tokens
	CreateTokenPool(ctx context.Context, nsOpID string, pool *core.TokenPool) (phase core.OpPhase, err error)

	// ActivateTokenPool activates a pool in order to begin receiving events
	ActivateTokenPool(ctx context.Context, pool *core.TokenPool) (phase core.OpPhase, err error)

	// DectivateTokenPool deactivates a pool in order to stop receiving events and remove underlying listeners
	DeactivateTokenPool(ctx context.Context, pool *core.TokenPool) error

	// CheckInterface checks which methods of a contract interface are supported by this connector
	CheckInterface(ctx context.Context, pool *core.TokenPool, methods []*fftypes.FFIMethod) (*fftypes.JSONAny, error)

	// MintTokens mints new tokens in a pool and adds them to the recipient's account
	MintTokens(ctx context.Context, nsOpID string, poolLocator string, mint *core.TokenTransfer, methods *fftypes.JSONAny) error

	// BurnTokens burns tokens from an account
	BurnTokens(ctx context.Context, nsOpID string, poolLocator string, burn *core.TokenTransfer, methods *fftypes.JSONAny) error

	// TransferTokens transfers tokens within a pool from one account to another
	TransferTokens(ctx context.Context, nsOpID string, poolLocator string, transfer *core.TokenTransfer, methods *fftypes.JSONAny) error

	// TokenApproval approves an operator to transfer tokens on the owner's behalf
	TokensApproval(ctx context.Context, nsOpID string, poolLocator string, approval *core.TokenApproval, methods *fftypes.JSONAny) error
}

// Callbacks is the interface provided to the tokens plugin, to allow it to pass events back to firefly.
//
// Events must be delivered sequentially, such that event 2 is not delivered until the callback invoked for event 1
// has completed. However, it does not matter if these events are workload balance between the firefly core
// cluster instances of the node.
type Callbacks interface {
	// TokenPoolCreated notifies on the creation of a new token pool, which might have been
	// submitted by us, or by any other authorized party in the network.
	//
	// Error should only be returned in shutdown scenarios
	//
	// Note: The context is passed on this callback (unlike most callbacks), as it might be
	//       involved in-line with the original REST API call in the special case of the
	//       submitter.
	TokenPoolCreated(ctx context.Context, plugin Plugin, pool *TokenPool) error

	// TokensTransferred notifies on a transfer between token accounts.
	//
	// Error should only be returned in shutdown scenarios
	TokensTransferred(plugin Plugin, transfer *TokenTransfer) error

	// TokensApproved notifies on a token approval
	//
	// Error should will only be returned in shutdown scenarios
	TokensApproved(plugin Plugin, approval *TokenApproval) error
}

// Capabilities is the supported featureset of the tokens interface implemented by the plugin, with the specified config
type Capabilities struct {
}

// TokenPool is the set of data returned from the connector when a token pool is created.
type TokenPool struct {
	// Type is the type of tokens (fungible, non-fungible, etc) in this pool
	Type core.TokenType

	// ID is the ID assigned to this pool by FireFly (if known)
	ID *fftypes.UUID

	// PoolLocator is the identifier assigned to this pool by the token connector (includes the contract address or other location info)
	PoolLocator string

	// AlternateLocators is a list of PoolLocators by which a previous version of the connector may have referred to this pool
	// It will only be set on a TokenPoolCreated event and FireFly can use it to match and update an existing pool that is now
	// referred to by a new locator
	AlternateLocators []string

	// TX is the FireFly-assigned information to correlate this to a transaction (optional)
	TX core.TransactionRef

	// Connector is the configured name of this connector
	Connector string

	// Standard is the well-defined token standard that this pool conforms to (optional)
	Standard string

	// InterfaceFormat is the interface encoding format understood by this connector (optional)
	InterfaceFormat string

	// Decimals is the number of decimal places that this token has (optional)
	Decimals int

	// Symbol is the short token symbol, if the connector uses one (optional)
	Symbol string

	// Info is any other connector-specific info on the pool that may be worth saving (optional)
	Info fftypes.JSONObject

	// PluginData is any other data that the plugin would like to save with the pool (optional)
	PluginData string

	// Event contains info on the underlying blockchain event for this pool creation
	Event *blockchain.Event
}

// TokenPoolMethods is a mapped set of interface methods to be used for various token operations
type TokenPoolMethods struct {
	Approval *fftypes.JSONAny `json:"approval"`
	Burn     *fftypes.JSONAny `json:"burn"`
	Mint     *fftypes.JSONAny `json:"mint"`
	Transfer *fftypes.JSONAny `json:"transfer"`
}

type TokenTransfer struct {
	// Although not every field will be filled in, embed core.TokenTransfer to avoid duplicating lots of fields
	core.TokenTransfer

	// PoolLocator is the ID assigned to the token pool by the connector
	PoolLocator string

	// Event contains info on the underlying blockchain event for this transfer
	Event *blockchain.Event
}

type TokenApproval struct {
	// Although not every field will be filled in, embed core.TokenApproval to avoid duplicating lots of fields
	core.TokenApproval

	// PoolLocator is the ID assigned to the token pool by the connector
	PoolLocator string

	// Event contains info on the underlying blockchain event for this transfer
	Event *blockchain.Event
}
