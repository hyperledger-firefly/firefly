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

package orchestrator

import (
	"context"
	"sort"

	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

func updateStatus(result *core.TransactionStatus, newStatus core.OpStatus) {
	if result.Status != core.OpStatusFailed && newStatus != core.OpStatusSucceeded {
		result.Status = newStatus
	}
}

func pendingPlaceholder(t core.TransactionStatusType) *core.TransactionStatusDetails {
	return &core.TransactionStatusDetails{
		Type:   t,
		Status: core.OpStatusPending,
	}
}

func txOperationStatus(op *core.Operation) *core.TransactionStatusDetails {
	return &core.TransactionStatusDetails{
		Status:    op.Status,
		Type:      core.TransactionStatusTypeOperation,
		SubType:   op.Type.String(),
		Timestamp: op.Updated,
		ID:        op.ID,
		Error:     op.Error,
		Info:      op.Output,
	}
}

func txBlockchainEventStatus(event *core.BlockchainEvent) *core.TransactionStatusDetails {
	return &core.TransactionStatusDetails{
		Status:    core.OpStatusSucceeded,
		Type:      core.TransactionStatusTypeBlockchainEvent,
		SubType:   event.Name,
		Timestamp: event.Timestamp,
		ID:        event.ID,
		Info:      event.Info,
	}
}

func (or *orchestrator) GetTransactionStatus(ctx context.Context, id string) (*core.TransactionStatus, error) {
	result := &core.TransactionStatus{
		Status:  core.OpStatusSucceeded,
		Details: make([]*core.TransactionStatusDetails, 0),
	}

	tx, err := or.GetTransactionByID(ctx, id)
	if err != nil {
		return nil, err
	} else if tx == nil {
		return nil, i18n.NewError(ctx, coremsgs.Msg404NotFound)
	}

	ops, _, err := or.GetTransactionOperations(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, op := range ops {
		result.Details = append(result.Details, txOperationStatus(op))
		if op.Status == core.OpStatusPending {
			// Check to see if there's an update
			// Operations can stay in "Pending" if FireFly was down when a TX receipt became available
			opWithDetail, err := or.GetOperationByIDWithStatus(ctx, op.ID.String())
			if err != nil {
				return nil, err
			}
			op = &opWithDetail.Operation
		}
		if op.Retry == nil {
			updateStatus(result, op.Status)
		}
	}

	events, _, err := or.GetTransactionBlockchainEvents(ctx, id)
	if err != nil {
		return nil, err
	}
	for _, event := range events {
		result.Details = append(result.Details, txBlockchainEventStatus(event))
	}

	switch tx.Type {
	case core.TransactionTypeBatchPin, core.TransactionTypeContractInvokePin:
		if len(events) == 0 {
			result.Details = append(result.Details, pendingPlaceholder(core.TransactionStatusTypeBlockchainEvent))
			updateStatus(result, core.OpStatusPending)
		}
		f := database.BatchQueryFactory.NewFilter(ctx)
		switch batches, _, err := or.database().GetBatches(ctx, or.namespace.Name, f.Eq("tx.id", id)); {
		case err != nil:
			return nil, err
		case len(batches) == 0:
			result.Details = append(result.Details, pendingPlaceholder(core.TransactionStatusTypeBatch))
			updateStatus(result, core.OpStatusPending)
		default:
			result.Details = append(result.Details, &core.TransactionStatusDetails{
				Status:    core.OpStatusSucceeded,
				Type:      core.TransactionStatusTypeBatch,
				SubType:   batches[0].Type.String(),
				Timestamp: batches[0].Confirmed,
				ID:        batches[0].ID,
			})
		}

	case core.TransactionTypeTokenPool:
		// Note: no assumptions about blockchain events here (may or may not contain one)
		f := database.TokenPoolQueryFactory.NewFilter(ctx)
		switch pools, _, err := or.database().GetTokenPools(ctx, or.namespace.Name, f.Eq("tx.id", id)); {
		case err != nil:
			return nil, err
		case len(pools) == 0:
			result.Details = append(result.Details, pendingPlaceholder(core.TransactionStatusTypeTokenPool))
			updateStatus(result, core.OpStatusPending)
		case !pools[0].Active:
			result.Details = append(result.Details, &core.TransactionStatusDetails{
				Status:  core.OpStatusPending,
				Type:    core.TransactionStatusTypeTokenPool,
				SubType: pools[0].Type.String(),
				ID:      pools[0].ID,
			})
			updateStatus(result, core.OpStatusPending)
		default:
			result.Details = append(result.Details, &core.TransactionStatusDetails{
				Status:    core.OpStatusSucceeded,
				Type:      core.TransactionStatusTypeTokenPool,
				SubType:   pools[0].Type.String(),
				Timestamp: pools[0].Created,
				ID:        pools[0].ID,
			})
		}

	case core.TransactionTypeTokenTransfer:
		if len(events) == 0 {
			result.Details = append(result.Details, pendingPlaceholder(core.TransactionStatusTypeBlockchainEvent))
			updateStatus(result, core.OpStatusPending)
		}
		f := database.TokenTransferQueryFactory.NewFilter(ctx)
		switch transfers, _, err := or.database().GetTokenTransfers(ctx, or.namespace.Name, f.Eq("tx.id", id)); {
		case err != nil:
			return nil, err
		case len(transfers) == 0:
			result.Details = append(result.Details, pendingPlaceholder(core.TransactionStatusTypeTokenTransfer))
			updateStatus(result, core.OpStatusPending)
		default:
			result.Details = append(result.Details, &core.TransactionStatusDetails{
				Status:    core.OpStatusSucceeded,
				Type:      core.TransactionStatusTypeTokenTransfer,
				SubType:   transfers[0].Type.String(),
				Timestamp: transfers[0].Created,
				ID:        transfers[0].LocalID,
			})
		}

	case core.TransactionTypeTokenApproval:
		if len(events) == 0 {
			result.Details = append(result.Details, pendingPlaceholder(core.TransactionStatusTypeBlockchainEvent))
			updateStatus(result, core.OpStatusPending)
		}
		f := database.TokenApprovalQueryFactory.NewFilter(ctx)
		switch approvals, _, err := or.database().GetTokenApprovals(ctx, or.namespace.Name, f.Eq("tx.id", id)); {
		case err != nil:
			return nil, err
		case len(approvals) == 0:
			result.Details = append(result.Details, pendingPlaceholder(core.TransactionStatusTypeTokenApproval))
			updateStatus(result, core.OpStatusPending)
		default:
			result.Details = append(result.Details, &core.TransactionStatusDetails{
				Status:    core.OpStatusSucceeded,
				Type:      core.TransactionStatusTypeTokenApproval,
				Timestamp: approvals[0].Created,
				ID:        approvals[0].LocalID,
			})
		}

	case core.TransactionTypeContractInvoke, core.TransactionTypeContractDeploy, core.TransactionTypeDataPublish:
		// no blockchain events or other objects

	default:
		return nil, i18n.NewError(ctx, coremsgs.MsgUnknownTransactionType, tx.Type)
	}

	// Sort with nil timestamps first (ie Pending), then descending by timestamp
	sort.SliceStable(result.Details, func(i, j int) bool {
		x := result.Details[i].Timestamp
		y := result.Details[j].Timestamp
		switch {
		case y == nil:
			return false
		case x == nil:
			return true
		default:
			return x.Time().After(*y.Time())
		}
	})

	return result, nil
}
