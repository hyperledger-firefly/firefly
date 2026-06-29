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

func (nm *networkMap) RegisterNode(ctx context.Context, waitConfirm bool) (identity *core.Identity, err error) {
	nodeOwningOrg, err := nm.identity.GetRootOrg(ctx)
	if err != nil {
		return nil, err
	}

	localNodeName := nm.multiparty.LocalNode().Name
	if localNodeName == "" {
		return nil, i18n.NewError(ctx, coremsgs.MsgNodeAndOrgIDMustBeSet)
	}
	nodeRequest := &core.IdentityCreateDTO{
		Parent: nodeOwningOrg.ID.String(),
		Name:   localNodeName,
		Type:   core.IdentityTypeNode,
		IdentityProfile: core.IdentityProfile{
			Description: nm.multiparty.LocalNode().Description,
		},
	}

	nodeRequest.Profile, err = nm.exchange.GetEndpointInfo(ctx, localNodeName)
	if err != nil {
		return nil, err
	}

	return nm.RegisterIdentity(ctx, nodeRequest, waitConfirm)
}
