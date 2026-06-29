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

func TestOrgMigration(t *testing.T) {

	org := DeprecatedOrganization{
		ID:          fftypes.NewUUID(),
		Name:        "org1",
		Description: "Org 1",
		Profile: fftypes.JSONObject{
			"test": "profile",
		},
	}
	assert.Equal(t, &IdentityClaim{
		Identity: &Identity{
			IdentityBase: IdentityBase{
				ID:        org.ID,
				Type:      IdentityTypeOrg,
				DID:       "did:firefly:org/org1",
				Namespace: LegacySystemNamespace,
				Name:      "org1",
			},
			IdentityProfile: IdentityProfile{
				Description: "Org 1",
				Profile: fftypes.JSONObject{
					"test": "profile",
				},
			},
		},
	}, org.Migrated())

	assert.Equal(t, "7ea456fa05fc63778e7c4cb22d0498d73f184b2778c11fd2ba31b5980f8490b9", org.Topic())

	msg := &Message{
		Header: MessageHeader{
			ID: fftypes.NewUUID(),
		},
	}
	org.SetBroadcastMessage(msg.Header.ID)
	assert.Equal(t, msg.Header.ID, org.Migrated().Identity.Messages.Claim)

}
