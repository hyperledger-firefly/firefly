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

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

func (dh *definitionHandler) handleDatatypeBroadcast(ctx context.Context, state *core.BatchState, msg *core.Message, data core.DataArray, tx *fftypes.UUID) (HandlerResult, error) {
	var dt core.Datatype
	valid := dh.getSystemBroadcastPayload(ctx, msg, data, &dt)
	if !valid {
		return HandlerResult{Action: core.ActionReject}, i18n.NewError(ctx, coremsgs.MsgDefRejectedBadPayload, "datatype", msg.Header.ID)
	}
	dt.Namespace = dh.namespace.Name
	if err := dt.Validate(ctx, true); err != nil {
		return HandlerResult{Action: core.ActionReject}, i18n.NewError(ctx, coremsgs.MsgDefRejectedValidateFail, "datatype", dt.ID, err)
	}
	if err := dh.data.CheckDatatype(ctx, &dt); err != nil {
		return HandlerResult{Action: core.ActionReject}, i18n.NewError(ctx, coremsgs.MsgDefRejectedSchemaFail, "datatype", dt.ID, err)
	}

	existing, err := dh.database.GetDatatypeByName(ctx, dt.Namespace, dt.Name, dt.Version)
	if err != nil {
		return HandlerResult{Action: core.ActionRetry}, err
	} else if existing != nil {
		return HandlerResult{Action: core.ActionReject}, i18n.NewError(ctx, coremsgs.MsgDefRejectedConflict, "datatype", dt.ID, existing.ID)
	}

	if err = dh.database.UpsertDatatype(ctx, &dt, false); err != nil {
		return HandlerResult{Action: core.ActionRetry}, err
	}

	state.AddFinalize(func(ctx context.Context) error {
		event := core.NewEvent(core.EventTypeDatatypeConfirmed, dt.Namespace, dt.ID, tx, core.SystemTopicDefinitions)
		return dh.database.InsertEvent(ctx, event)
	})
	return HandlerResult{Action: core.ActionConfirm}, nil
}
