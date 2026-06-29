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
	"github.com/hyperledger-firefly/firefly/pkg/tokens"
)

// Determine if this transfer event should use a LocalID that was pre-assigned to an operation submitted by this node.
// This will ensure that the original LocalID provided to the user can later be used in a lookup, and also causes requests that
// use "confirm=true" to resolve as expected.
// Must follow these rules to reuse the LocalID:
//   - The transaction ID on the transfer must match a transaction+operation initiated by this node.
//   - The connector and pool for this event must match the connector and pool targeted by the initial operation. Connectors are
//     allowed to trigger side-effects in other pools, but only the event from the targeted pool should use the original LocalID.
//   - The LocalID must not have been used yet. Connectors are allowed to emit multiple events in response to a single operation,
//     but only the first of them can use the original LocalID.
func (em *eventManager) loadTransferID(ctx context.Context, tx *fftypes.UUID, transfer *core.TokenTransfer) (*fftypes.UUID, error) {
	op, err := em.txHelper.FindOperationInTransaction(ctx, tx, core.OpTypeTokenTransfer)
	if err != nil {
		return nil, err
	}
	if op != nil {
		// This transfer matches a transfer transaction+operation submitted by this node.
		// Check the operation inputs to see if they match the connector and pool on this event.
		if input, err := txcommon.RetrieveTokenTransferInputs(ctx, op); err != nil {
			log.L(ctx).Warnf("Failed to read operation inputs for token transfer '%s': %s", transfer.ProtocolID, err)
		} else if input != nil && input.Connector == transfer.Connector && input.Pool.Equals(transfer.Pool) {
			// Check if the LocalID has already been used
			if existing, err := em.database.GetTokenTransferByID(ctx, em.namespace.Name, input.LocalID); err != nil {
				return nil, err
			} else if existing == nil {
				// Everything matches - use the LocalID that was assigned up-front when the operation was submitted
				return input.LocalID, nil
			}
		}
	}

	return fftypes.NewUUID(), nil
}

func (em *eventManager) persistTokenTransfer(ctx context.Context, transfer *tokens.TokenTransfer) (valid bool, err error) {
	// Check that this is from a known pool
	pool, err := em.getPoolByIDOrLocator(ctx, transfer.Pool, transfer.Connector, transfer.PoolLocator)
	if err != nil {
		return false, err
	}
	if pool == nil {
		log.L(ctx).Infof("Token transfer received for unknown pool '%s' - ignoring: %s", transfer.PoolLocator, transfer.Event.ProtocolID)
		return false, nil
	}
	transfer.Namespace = pool.Namespace
	transfer.Pool = pool.ID

	if transfer.TX.ID == nil {
		transfer.LocalID = fftypes.NewUUID()
	} else {
		if transfer.LocalID, err = em.loadTransferID(ctx, transfer.TX.ID, &transfer.TokenTransfer); err != nil {
			return false, err
		}
		if valid, err := em.txHelper.PersistTransaction(ctx, transfer.TX.ID, transfer.TX.Type, transfer.Event.BlockchainTXID); err != nil || !valid {
			return valid, err
		}
	}

	// This is a no-op if we've already persisted this token transfer
	chainEvent := buildBlockchainEvent(pool.Namespace, nil, transfer.Event, &core.BlockchainTransactionRef{
		ID:           transfer.TX.ID,
		Type:         transfer.TX.Type,
		BlockchainID: transfer.Event.BlockchainTXID,
	})
	created, err := em.maybePersistBlockchainEvent(ctx, chainEvent, nil)
	if err != nil {
		return false, err
	}
	if created {
		em.emitBlockchainEventMetric(transfer.Event)
	}
	transfer.BlockchainEvent = chainEvent.ID

	// This is a no-op if we've already persisted this token transfer
	existing, err := em.database.InsertOrGetTokenTransfer(ctx, &transfer.TokenTransfer)

	if err != nil {
		log.L(ctx).Errorf("Failed to record token transfer '%s': %s", transfer.ProtocolID, err)
		return false, err
	}
	if existing != nil {
		log.L(ctx).Debugf("Ignoring duplicate token transfer event %s", existing.ProtocolID)
		return false, nil
	}

	if err := em.database.UpdateTokenBalances(ctx, &transfer.TokenTransfer); err != nil {
		log.L(ctx).Errorf("Failed to update accounts %s -> %s for token transfer '%s': %s", transfer.From, transfer.To, transfer.ProtocolID, err)
		return false, err
	}

	log.L(ctx).Infof("Token transfer recorded id=%s author=%s", transfer.ProtocolID, transfer.Key)
	if em.metrics.IsMetricsEnabled() {
		em.metrics.TransferConfirmed(&transfer.TokenTransfer)
	}

	return true, nil
}

func (em *eventManager) TokensTransferred(ti tokens.Plugin, transfer *tokens.TokenTransfer) error {
	var msgIDforRewind *fftypes.UUID

	err := em.retry.Do(em.ctx, "persist token transfer", func(attempt int) (bool, error) {
		err := em.database.RunAsGroup(em.ctx, func(ctx context.Context) error {
			if valid, err := em.persistTokenTransfer(ctx, transfer); !valid || err != nil {
				return err
			}

			if transfer.Message != nil {
				msg, err := em.database.GetMessageByID(ctx, em.namespace.Name, transfer.Message)
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
					msgIDforRewind = transfer.Message
				}
			}
			em.emitBlockchainEventMetric(transfer.Event)

			event := core.NewEvent(core.EventTypeTransferConfirmed, transfer.Namespace, transfer.LocalID, transfer.TX.ID, transfer.Pool.String())
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
