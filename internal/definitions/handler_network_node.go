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

package definitions

import (
	"context"

	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

func (dh *definitionHandler) handleDeprecatedNodeBroadcast(ctx context.Context, state *core.BatchState, msg *core.Message, data core.DataArray) (HandlerResult, error) {
	var nodeOld core.DeprecatedNode
	if valid := dh.getSystemBroadcastPayload(ctx, msg, data, &nodeOld); !valid {
		return HandlerResult{Action: core.ActionReject}, i18n.NewError(ctx, coremsgs.MsgDefRejectedBadPayload, "node", msg.Header.ID)
	}

	owner, err := dh.identity.FindIdentityForVerifier(ctx, []core.IdentityType{core.IdentityTypeOrg}, &core.VerifierRef{
		Type:  dh.blockchain.VerifierType(),
		Value: nodeOld.Owner,
	})
	if err != nil {
		return HandlerResult{Action: core.ActionRetry}, err // We only return database errors
	}
	if owner == nil {
		return HandlerResult{Action: core.ActionReject}, i18n.NewError(ctx, coremsgs.MsgDefRejectedIdentityNotFound, "node", nodeOld.ID, nodeOld.Owner)
	}

	return dh.handleIdentityClaim(ctx, state, buildIdentityMsgInfo(msg, nil), nodeOld.AddMigratedParent(owner.ID))

}
