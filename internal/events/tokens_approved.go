// Copyright © 2024 Kaleido, Inc.
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

package events

import (
	"context"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/txcommon"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/hyperledger-firefly/firefly/pkg/tokens"
)

// Determine if this approval event should use a LocalID that was pre-assigned to an operation submitted by this node.
// This will ensure that the original LocalID provided to the user can later be used in a lookup, and also causes requests that
// use "confirm=true" to resolve as expected.
// Must follow these rules to reuse the LocalID:
//   - The transaction ID on the approval must match a transaction+operation initiated by this node.
//   - The connector and pool for this event must match the connector and pool targeted by the initial operation. Connectors are
//     allowed to trigger side-effects in other pools, but only the event from the targeted pool should use the original LocalID.
//   - The LocalID must not have been used yet. Connectors are allowed to emit multiple events in response to a single operation,
//     but only the first of them can use the original LocalID.
func (em *eventManager) loadApprovalID(ctx context.Context, tx *fftypes.UUID, approval *core.TokenApproval) (*fftypes.UUID, error) {
	op, err := em.txHelper.FindOperationInTransaction(ctx, tx, core.OpTypeTokenApproval)
	if err != nil {
		return nil, err
	}
	if op != nil {
		// This approval matches an approval transaction+operation submitted by this node.
		// Check the operation inputs to see if they match the connector and pool on this event.
		if input, err := txcommon.RetrieveTokenApprovalInputs(ctx, op); err != nil {
			log.L(ctx).Warnf("Failed to read operation inputs for token approval '%s': %s", approval.Subject, err)
		} else if input != nil && input.Connector == approval.Connector && input.Pool.Equals(approval.Pool) {
			// Check if the LocalID has already been used
			if existing, err := em.database.GetTokenApprovalByID(ctx, em.namespace.Name, input.LocalID); err != nil {
				return nil, err
			} else if existing == nil {
				// Everything matches - use the LocalID that was assigned up-front when the operation was submitted
				return input.LocalID, nil
			}
		}
	}

	return fftypes.NewUUID(), nil
}

func (em *eventManager) persistTokenApproval(ctx context.Context, approval *tokens.TokenApproval) (valid bool, err error) {
	// Check that this is from a known pool
	pool, err := em.getPoolByIDOrLocator(ctx, approval.Pool, approval.Connector, approval.PoolLocator)
	if err != nil {
		return false, err
	}
	if pool == nil {
		log.L(ctx).Infof("Token approval received for unknown pool '%s' - ignoring: %s", approval.PoolLocator, approval.Event.ProtocolID)
		return false, nil
	}
	approval.Namespace = pool.Namespace
	approval.Pool = pool.ID

	// Check that approval has not already been recorded
	if existing, err := em.database.GetTokenApprovalByProtocolID(ctx, em.namespace.Name, approval.Pool, approval.ProtocolID); err != nil {
		return false, err
	} else if existing != nil {
		log.L(ctx).Warnf("Token approval '%s' has already been recorded - ignoring", approval.ProtocolID)
		return false, nil
	}

	if approval.TX.ID == nil {
		approval.LocalID = fftypes.NewUUID()
	} else {
		if approval.LocalID, err = em.loadApprovalID(ctx, approval.TX.ID, &approval.TokenApproval); err != nil {
			return false, err
		}
		if valid, err := em.txHelper.PersistTransaction(ctx, approval.TX.ID, approval.TX.Type, approval.Event.BlockchainTXID); err != nil || !valid {
			return valid, err
		}
	}

	chainEvent := buildBlockchainEvent(approval.Namespace, nil, approval.Event, &core.BlockchainTransactionRef{
		ID:           approval.TX.ID,
		Type:         approval.TX.Type,
		BlockchainID: approval.Event.BlockchainTXID,
	})
	created, err := em.maybePersistBlockchainEvent(ctx, chainEvent, nil)
	if err != nil {
		return false, err
	}
	if created {
		em.emitBlockchainEventMetric(approval.Event)
	}
	approval.BlockchainEvent = chainEvent.ID

	fb := database.TokenApprovalQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("pool", approval.Pool),
		fb.Eq("subject", approval.Subject),
	)
	update := database.TokenApprovalQueryFactory.NewUpdate(ctx).Set("active", false)
	if err := em.database.UpdateTokenApprovals(ctx, filter, update); err != nil {
		log.L(ctx).Errorf("Failed to update prior token approvals for '%s': %s", approval.Subject, err)
		return false, err
	}

	approval.Active = true
	if err := em.database.UpsertTokenApproval(ctx, &approval.TokenApproval); err != nil {
		log.L(ctx).Errorf("Failed to record token approval '%s': %s", approval.Subject, err)
		return false, err
	}

	log.L(ctx).Infof("Token approval recorded id=%s author=%s", approval.Subject, approval.Key)
	return true, nil
}

func (em *eventManager) TokensApproved(ti tokens.Plugin, approval *tokens.TokenApproval) error {
	var msgIDforRewind *fftypes.UUID

	err := em.retry.Do(em.ctx, "persist token approval", func(attempt int) (bool, error) {
		err := em.database.RunAsGroup(em.ctx, func(ctx context.Context) error {
			if valid, err := em.persistTokenApproval(ctx, approval); !valid || err != nil {
				return err
			}

			if approval.Message != nil {
				msg, err := em.database.GetMessageByID(ctx, em.namespace.Name, approval.Message)
				switch {
				case err != nil:
					return err
				case msg != nil && msg.State == core.MessageStateStaged:
					// Message can now be sent
					msg.State = core.MessageStateReady
					if err := em.database.ReplaceMessage(ctx, msg); err != nil {
						return err
					}
				default:
					// Message might already have been received, we need to rewind
					msgIDforRewind = approval.Message
				}
			}
			em.emitBlockchainEventMetric(approval.Event)

			event := core.NewEvent(core.EventTypeApprovalConfirmed, approval.Namespace, approval.LocalID, approval.TX.ID, approval.Pool.String())
			return em.database.InsertEvent(ctx, event)
		})
		return err != nil, err // retry indefinitely (until context closes)
	})

	// Initiate a rewind if a batch was potentially completed by the arrival of this transfer
	if err == nil && msgIDforRewind != nil {
		em.aggregator.queueMessageRewind(msgIDforRewind)
	}

	return err
}
