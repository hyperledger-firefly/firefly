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

package dataexchange

import (
	"context"
	"io"

	"github.com/hyperledger-firefly/firefly/internal/metrics"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

// Plugin is the interface implemented by each data exchange plugin
//
// Data exchange plugins are responsible for qualities of service for:
// - Storage of large data payloads
// - Security of transfer of data between participants (transport and payload authorization & encryption)
// - Reliability of transfer of data between participants (many transports can be supported - HTTPs/AMQP/MQTT etc.)
//
// Each plugin must handle network addressing, as well as transfer of messages and Blobs.
//
// Network addressing:
// - Each node must have a "peerID" (<=256b) that uniquely identifies a node within the data exchange network
// - A node must provide a JSON package of data for connectivity to a node, which FireFly can distribute to all other nodes to address this node
//
// Messages:
// - Are provided in-line in requests to transfer to a peer
// - Must drive events on the target node that contain the input data
// - No requirement to retain the data beyond the confirmation of receipt of the event at the target
//
// Blobs
// - Can be stored and retrieved separately from their transfer
// - Transfers are initiated via reference (not in-line data)
// - Are hashed by the DX plugin using the same hashing algorithm as FireFly (SHA256)
// - DX plugins can maintain their own internal IDs for Blobs within the following requirements:
//   - Given a namespace and ID, map to a "payloadRef" string (<1024chars) that allows that same payload to be retrieved using only that payloadRef
//   - Example would be a logical filesystem path like "local/namespace/ID"
//   - When data is received from other members in the network, be able to return the hash when provided with the remote peerID string, namespace and ID
//   - Could be done by having a data store to resolve the transfers, or simply a deterministic path to metadata like "receive/peerID/namespace/ID"
//   - Events triggered for arrival of blobs must contain the payloadRef, and the hash
type Plugin interface {
	core.Named

	// InitConfig initializes the set of configuration options that are valid, with defaults. Called on all plugins.
	InitConfig(config config.Section)

	// Init initializes the plugin, with configuration
	Init(ctx context.Context, cancelCtx context.CancelFunc, config config.Section, metrics metrics.Manager) error

	// SetHandler registers a handler to receive callbacks
	// Plugin will attempt (but is not guaranteed) to deliver events only for the given namespace and node
	SetHandler(networkNamespace, nodeName string, handler Callbacks)

	// SetOperationHandler registers a handler to receive async operation status
	// If namespace is set, plugin will attempt to deliver only events for that namespace
	SetOperationHandler(namespace string, handler core.OperationCallbacks)

	// Data exchange interface must not deliver any events until start is called
	Start() error

	// Capabilities returns capabilities - not called until after Init
	Capabilities() *Capabilities

	// GetEndpointInfo returns the information about the local endpoint
	GetEndpointInfo(ctx context.Context, nodeName string) (peer fftypes.JSONObject, err error)

	// AddNode registers details on a node in the multiparty network
	// This may be information loaded from the database at init, or received in flight while running
	AddNode(ctx context.Context, networkNamespace, nodeName string, peer fftypes.JSONObject) (err error)

	// UploadBlob streams a blob to storage, and returns the hash to confirm the hash calculated in Core matches the hash calculated in the plugin
	UploadBlob(ctx context.Context, ns string, id fftypes.UUID, content io.Reader) (payloadRef string, hash *fftypes.Bytes32, size int64, err error)

	// DownloadBlob streams a received blob out of storage
	DownloadBlob(ctx context.Context, payloadRef string) (content io.ReadCloser, err error)

	// DeleteBlob streams a deletes a blob from the local DB and DX
	DeleteBlob(ctx context.Context, payloadRef string) (err error)

	// SendMessage sends an in-line package of data to another network node.
	// Should return as quickly as possible for parallelism, then report completion asynchronously via the operation ID
	SendMessage(ctx context.Context, nsOpID string, peer, sender fftypes.JSONObject, data []byte) (err error)

	// TransferBlob initiates a transfer of a previously stored blob to another node
	TransferBlob(ctx context.Context, nsOpID string, peer, sender fftypes.JSONObject, payloadRef string) (err error)

	// GetPeerID extracts the peer ID from the peer JSON
	GetPeerID(peer fftypes.JSONObject) string

	// CheckNodeIdentityStatus checks the status of the local node's network identity relative to the DX plugin's config
	CheckNodeIdentityStatus(ctx context.Context, node *core.Identity) error
}

// Callbacks is the interface provided to the data exchange plugin, to allow it to pass events back to firefly.
type Callbacks interface {
	DXConnect(plugin Plugin)
	// Event has sub-types as defined below, and can be processed and ack'd asynchronously
	DXEvent(plugin Plugin, event DXEvent) error
}

type DXEventType int

// DXEvent is a single interface that can be passed to all events
type DXEvent interface {
	EventID() string
	Ack()
	AckWithManifest(manifest string)
	Type() DXEventType
	MessageReceived() *MessageReceived
	PrivateBlobReceived() *PrivateBlobReceived
}

const (
	DXEventTypeMessageReceived DXEventType = iota
	DXEventTypePrivateBlobReceived
)

type MessageReceived struct {
	PeerID    string
	Transport *core.TransportWrapper
}

type PrivateBlobReceived struct {
	Namespace  string
	PeerID     string
	Hash       fftypes.Bytes32
	Size       int64
	PayloadRef string
	DataID     string
}

// Capabilities the supported featureset of the data exchange
// interface implemented by the plugin, with the specified config
type Capabilities struct {
	// Manifest - whether TransferResult events contain the manifest generated by the receiving FireFly
	Manifest bool
}
