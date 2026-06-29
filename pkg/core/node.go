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

package core

import (
	"context"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
)

// DeprecatedNode is the data structure we used to use prior to FIR-9.
// Now we use the common Identity structure throughout
type DeprecatedNode struct {
	ID          *fftypes.UUID    `json:"id"`
	Message     *fftypes.UUID    `json:"message,omitempty"`
	Owner       string           `json:"owner,omitempty"`
	Name        string           `json:"name,omitempty"`
	Description string           `json:"description,omitempty"`
	DX          DeprecatedDXInfo `json:"dx"`
	Created     *fftypes.FFTime  `json:"created,omitempty"`

	identityClaim *IdentityClaim
}

type DeprecatedDXInfo struct {
	Peer     string             `json:"peer,omitempty"`
	Endpoint fftypes.JSONObject `json:"endpoint,omitempty"`
}

// Migrate creates and maintains a migrated IdentityClaim object, which
// is used when processing an old-style node broadcast received when
// joining an existing network
func (node *DeprecatedNode) Migrated() *IdentityClaim {
	if node.identityClaim != nil {
		return node.identityClaim
	}
	node.identityClaim = &IdentityClaim{
		Identity: &Identity{
			IdentityBase: IdentityBase{
				ID:        node.ID,
				Type:      IdentityTypeNode,
				Namespace: LegacySystemNamespace,
				Name:      node.Name,
				Parent:    nil, // Must be set post migrate
			},
			IdentityProfile: IdentityProfile{
				Description: node.Description,
				Profile:     node.DX.Endpoint,
			},
		},
	}
	return node.identityClaim
}

func (node *DeprecatedNode) AddMigratedParent(parentID *fftypes.UUID) *IdentityClaim {
	ic := node.Migrated()
	ic.Identity.Parent = parentID
	node.identityClaim.Identity.DID, _ = node.identityClaim.Identity.GenerateDID(context.Background())
	return ic
}

func (node *DeprecatedNode) Topic() string {
	return node.Migrated().Topic()
}

func (node *DeprecatedNode) SetBroadcastMessage(msgID *fftypes.UUID) {
	node.Migrated().SetBroadcastMessage(msgID)
}
