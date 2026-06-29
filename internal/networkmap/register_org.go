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

package networkmap

import (
	"context"

	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

// RegisterNodeOrganization is a convenience helper to register the org configured on the node, without any extra info
func (nm *networkMap) RegisterNodeOrganization(ctx context.Context, waitConfirm bool) (*core.Identity, error) {

	key, err := nm.identity.ResolveMultipartyRootVerifier(ctx)
	if err != nil {
		return nil, err
	}

	orgName := nm.multiparty.RootOrg().Name
	if orgName == "" {
		return nil, i18n.NewError(ctx, coremsgs.MsgNodeAndOrgIDMustBeSet)
	}
	orgRequest := &core.IdentityCreateDTO{
		Name: orgName,
		IdentityProfile: core.IdentityProfile{
			Description: nm.multiparty.RootOrg().Description,
		},
		Key: key.Value,
	}
	return nm.RegisterOrganization(ctx, orgRequest, waitConfirm)
}

func (nm *networkMap) RegisterOrganization(ctx context.Context, orgRequest *core.IdentityCreateDTO, waitConfirm bool) (*core.Identity, error) {
	orgRequest.Type = core.IdentityTypeOrg
	return nm.RegisterIdentity(ctx, orgRequest, waitConfirm)
}
