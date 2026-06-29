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
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/stretchr/testify/assert"
)

func TestNodeigration(t *testing.T) {

	node := DeprecatedNode{
		ID:          fftypes.NewUUID(),
		Name:        "node1",
		Description: "Node 1",
		DX: DeprecatedDXInfo{
			Peer: "ignored",
			Endpoint: fftypes.JSONObject{
				"id": "peer1",
			},
		},
	}
	parentID := fftypes.NewUUID()
	assert.Equal(t, &IdentityClaim{
		Identity: &Identity{
			IdentityBase: IdentityBase{
				ID:        node.ID,
				Type:      IdentityTypeNode,
				DID:       "did:firefly:node/node1",
				Namespace: LegacySystemNamespace,
				Name:      "node1",
				Parent:    parentID,
			},
			IdentityProfile: IdentityProfile{
				Description: "Node 1",
				Profile: fftypes.JSONObject{
					"id": "peer1",
				},
			},
		},
	}, node.AddMigratedParent(parentID))

	assert.Equal(t, "14c4157d50d35470b15a6576affa62adea1b191e8238f2273a099d1ef73fb335", node.Topic())

	msg := &Message{
		Header: MessageHeader{
			ID: fftypes.NewUUID(),
		},
	}
	node.SetBroadcastMessage(msg.Header.ID)
	assert.Equal(t, msg.Header.ID, node.Migrated().Identity.Messages.Claim)

}
