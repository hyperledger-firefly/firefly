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

// DeprecatedOrganization is the data structure we used to use prior to FIR-9.
// Now we use the common Identity structure throughout
type DeprecatedOrganization struct {
	ID          *fftypes.UUID      `json:"id"`
	Message     *fftypes.UUID      `json:"message,omitempty"`
	Parent      string             `json:"parent,omitempty"`
	Identity    string             `json:"identity,omitempty"`
	Name        string             `json:"name,omitempty"`
	Description string             `json:"description,omitempty"`
	Profile     fftypes.JSONObject `json:"profile,omitempty"`
	Created     *fftypes.FFTime    `json:"created,omitempty"`

	identityClaim *IdentityClaim
}

// Migrate creates and maintains a migrated IdentityClaim object, which
// is used when processing an old-style organization broadcast received when
// joining an existing network
func (org *DeprecatedOrganization) Migrated() *IdentityClaim {
	if org.identityClaim != nil {
		return org.identityClaim
	}
	org.identityClaim = &IdentityClaim{
		Identity: &Identity{
			IdentityBase: IdentityBase{
				ID:        org.ID,
				Type:      IdentityTypeOrg,
				Namespace: LegacySystemNamespace,
				Name:      org.Name,
				Parent:    nil, // No support for child identity migration (see FIR-9 for details)
			},
			IdentityProfile: IdentityProfile{
				Description: org.Description,
				Profile:     org.Profile,
			},
		},
	}
	org.identityClaim.Identity.DID, _ = org.identityClaim.Identity.GenerateDID(context.Background())
	return org.identityClaim
}

func (org *DeprecatedOrganization) Topic() string {
	return org.Migrated().Topic()
}

func (org *DeprecatedOrganization) SetBroadcastMessage(msgID *fftypes.UUID) {
	org.Migrated().SetBroadcastMessage(msgID)
}
