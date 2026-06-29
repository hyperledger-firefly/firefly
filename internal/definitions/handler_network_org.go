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

func (dh *definitionHandler) handleDeprecatedOrganizationBroadcast(ctx context.Context, state *core.BatchState, msg *core.Message, data core.DataArray) (HandlerResult, error) {

	var orgOld core.DeprecatedOrganization
	valid := dh.getSystemBroadcastPayload(ctx, msg, data, &orgOld)
	if !valid {
		return HandlerResult{Action: core.ActionReject}, i18n.NewError(ctx, coremsgs.MsgDefRejectedBadPayload, "org", msg.Header.ID)
	}

	return dh.handleIdentityClaim(ctx, state, buildIdentityMsgInfo(msg, nil), orgOld.Migrated())

}
