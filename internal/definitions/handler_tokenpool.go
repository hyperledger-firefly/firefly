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
	"fmt"

	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

func (dh *definitionHandler) handleTokenPoolBroadcast(ctx context.Context, state *core.BatchState, msg *core.Message, data core.DataArray) (HandlerResult, error) {
	var definition core.TokenPoolDefinition
	if valid := dh.getSystemBroadcastPayload(ctx, msg, data, &definition); !valid {
		return HandlerResult{Action: core.ActionReject}, i18n.NewError(ctx, coremsgs.MsgDefRejectedBadPayload, "token pool", msg.Header.ID)
	}

	pool := definition.Pool
	// Map remote connector name -> local name
	if localName, ok := dh.tokenNames[pool.Connector]; ok {
		pool.Connector = localName
	} else {
		log.L(ctx).Infof("Could not find local name for token connector: %s", pool.Connector)
		return HandlerResult{Action: core.ActionReject}, i18n.NewError(ctx, coremsgs.MsgInvalidConnectorName, pool.Connector, "token")
	}

	org, err := dh.identity.GetRootOrgDID(ctx)
	if err != nil {
		return HandlerResult{Action: core.ActionRetry}, err
	}
	isAuthor := org == msg.Header.Author

	pool.Message = msg.Header.ID
	pool.Name = pool.NetworkName
	pool.Published = true
	return dh.handleTokenPoolDefinition(ctx, state, pool, isAuthor)
}

func (dh *definitionHandler) handleTokenPoolDefinition(ctx context.Context, state *core.BatchState, pool *core.TokenPool, isAuthor bool) (HandlerResult, error) {
	// Set an event correlator, so that if we reject then the sync-async bridge action can know
	// from the event (without downloading and parsing the msg)
	correlator := pool.ID

	// Attempt to create the pool in pending state
	pool.Namespace = dh.namespace.Name
	pool.Active = false
	for i := 1; ; i++ {
		if err := pool.Validate(ctx); err != nil {
			return HandlerResult{Action: core.ActionReject, CustomCorrelator: correlator}, i18n.WrapError(ctx, err, coremsgs.MsgDefRejectedValidateFail, "token pool", pool.ID)
		}

		// Check if the pool conflicts with an existing pool
		existing, err := dh.database.InsertOrGetTokenPool(ctx, pool)
		if err != nil {
			return HandlerResult{Action: core.ActionRetry}, err
		}

		if existing == nil {
			// No conflict - new pool was inserted successfully
			break
		}

		if pool.Published {
			if existing.ID.Equals(pool.ID) {
				// ID conflict - check if this matches (or should overwrite) the existing record
				action, err := dh.reconcilePublishedPool(ctx, existing, pool, isAuthor)
				return HandlerResult{Action: action, CustomCorrelator: correlator}, err
			}

			if existing.Name == pool.Name {
				// Local name conflict - generate a unique name and try again
				pool.Name = fmt.Sprintf("%s-%d", pool.NetworkName, i)
				continue
			}
		}

		// Any other conflict - reject
		return HandlerResult{Action: core.ActionReject, CustomCorrelator: correlator}, i18n.NewError(ctx, coremsgs.MsgDefRejectedConflict, "token pool", pool.ID, existing.ID)
	}

	// Message will remain unconfirmed, but plugin will be notified to activate the pool
	// This will ultimately trigger a pool creation event and a rewind
	state.AddPreFinalize(func(ctx context.Context) error {
		if err := dh.assets.ActivateTokenPool(ctx, pool); err != nil {
			log.L(ctx).Errorf("Failed to activate token pool '%s': %s", pool.ID, err)
			return err
		}
		return nil
	})
	return HandlerResult{Action: core.ActionWait, CustomCorrelator: correlator}, nil
}

func (dh *definitionHandler) reconcilePublishedPool(ctx context.Context, existing, pool *core.TokenPool, isAuthor bool) (core.MessageAction, error) {
	if existing.Message.Equals(pool.Message) {
		if existing.Active {
			// Pool was previously activated - this must be a rewind to confirm the message
			return core.ActionConfirm, nil
		} else {
			// Pool is still awaiting activation
			return core.ActionWait, nil
		}
	}

	if existing.Message == nil && isAuthor {
		// Pool was previously unpublished - if it was now published by this node, upsert the new version
		pool.Name = existing.Name
		if err := dh.database.UpsertTokenPool(ctx, pool, database.UpsertOptimizationExisting); err != nil {
			return core.ActionRetry, err
		}
		return core.ActionConfirm, nil
	}

	return core.ActionReject, i18n.NewError(ctx, coremsgs.MsgDefRejectedConflict, "token pool", pool.ID, existing.ID)
}
