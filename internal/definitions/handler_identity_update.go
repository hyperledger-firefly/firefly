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
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

type identityUpdateMsgInfo struct {
	ID     *fftypes.UUID
	Author string
}

func (dh *definitionHandler) handleIdentityUpdateBroadcast(ctx context.Context, state *core.BatchState, msg *core.Message, data core.DataArray) (HandlerResult, error) {
	var update core.IdentityUpdate
	if valid := dh.getSystemBroadcastPayload(ctx, msg, data, &update); !valid {
		return HandlerResult{Action: core.ActionReject}, i18n.NewError(ctx, coremsgs.MsgDefRejectedBadPayload, "identity update", msg.Header.ID)
	}
	return dh.handleIdentityUpdate(ctx, state, &identityUpdateMsgInfo{
		ID:     msg.Header.ID,
		Author: msg.Header.Author,
	}, &update)
}

func (dh *definitionHandler) handleIdentityUpdate(ctx context.Context, state *core.BatchState, msg *identityUpdateMsgInfo, update *core.IdentityUpdate) (HandlerResult, error) {
	if err := update.Identity.Validate(ctx); err != nil {
		return HandlerResult{Action: core.ActionReject}, i18n.WrapError(ctx, err, coremsgs.MsgDefRejectedValidateFail, "identity update", update.Identity.ID)
	}

	// Get the existing identity (must be a confirmed identity at the point an update is issued)
	identity, err := dh.identity.CachedIdentityLookupByID(ctx, update.Identity.ID)
	if err != nil {
		return HandlerResult{Action: core.ActionRetry}, err
	}
	if identity == nil {
		return HandlerResult{Action: core.ActionReject}, i18n.NewError(ctx, coremsgs.MsgDefRejectedIdentityNotFound, "identity update", update.Identity.ID, update.Identity.ID)
	}

	if dh.multiparty {

		parent, retryable, err := dh.identity.VerifyIdentityChain(ctx, identity)
		if err != nil && retryable {
			return HandlerResult{Action: core.ActionRetry}, err
		} else if err != nil {
			log.L(ctx).Infof("Unable to process identity update (parked) %s: %s", msg.ID, err)
			return HandlerResult{Action: core.ActionWait}, nil
		}

		// Check the author matches
		expectedSigner := dh.getExpectedSigner(identity, parent)
		if expectedSigner.DID != msg.Author {
			return HandlerResult{Action: core.ActionReject}, i18n.NewError(ctx, coremsgs.MsgDefRejectedWrongAuthor, "identity update", update.Identity.ID, msg.Author)
		}

	}

	// Update the profile
	identity.IdentityProfile = update.Updates
	identity.Messages.Update = msg.ID
	err = dh.database.UpsertIdentity(ctx, identity, database.UpsertOptimizationExisting)
	if err != nil {
		return HandlerResult{Action: core.ActionRetry}, err
	}

	state.AddFinalize(func(ctx context.Context) error {
		event := core.NewEvent(core.EventTypeIdentityUpdated, identity.Namespace, identity.ID, nil, core.SystemTopicDefinitions)
		return dh.database.InsertEvent(ctx, event)
	})

	if dh.multiparty && identity.Type == core.IdentityTypeNode {
		nodeDID, err := dh.identity.GetLocalNodeDID(ctx)
		if err != nil {
			return HandlerResult{Action: core.ActionRetry}, err
		}

		if nodeDID == identity.DID {
			state.AddFinalize(func(ctx context.Context) error {
				err := dh.exchange.CheckNodeIdentityStatus(ctx, identity)
				if err != nil {
					log.L(ctx).Warnf("Failed to check node identity status relative to dataexchange: %s", err)
				}
				return nil
			})
		}
	}

	return HandlerResult{Action: core.ActionConfirm}, err

}
