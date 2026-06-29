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

package assets

import (
	"context"
	"fmt"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/internal/operations"
	"github.com/hyperledger-firefly/firefly/internal/txcommon"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

type createPoolData struct {
	Pool *core.TokenPool `json:"pool"`
}

type activatePoolData struct {
	Pool *core.TokenPool `json:"pool"`
}

type transferData struct {
	Pool     *core.TokenPool     `json:"pool"`
	Transfer *core.TokenTransfer `json:"transfer"`
}

type approvalData struct {
	Pool     *core.TokenPool     `json:"pool"`
	Approval *core.TokenApproval `json:"approval"`
}

func (am *assetManager) PrepareOperation(ctx context.Context, op *core.Operation) (*core.PreparedOperation, error) {
	switch op.Type {
	case core.OpTypeTokenCreatePool:
		pool, err := txcommon.RetrieveTokenPoolCreateInputs(ctx, op)
		if err != nil {
			return nil, err
		}
		return opCreatePool(op, pool), nil

	case core.OpTypeTokenActivatePool:
		poolID, err := txcommon.RetrieveTokenPoolActivateInputs(ctx, op)
		if err != nil {
			return nil, err
		}
		pool, err := am.GetTokenPoolByID(ctx, poolID)
		if err != nil {
			return nil, err
		} else if pool == nil {
			return nil, i18n.NewError(ctx, coremsgs.Msg404NotFound)
		}
		return opActivatePool(op, pool), nil

	case core.OpTypeTokenTransfer:
		transfer, err := txcommon.RetrieveTokenTransferInputs(ctx, op)
		if err != nil {
			return nil, err
		}
		pool, err := am.GetTokenPoolByID(ctx, transfer.Pool)
		if err != nil {
			return nil, err
		} else if pool == nil {
			return nil, i18n.NewError(ctx, coremsgs.Msg404NotFound)
		}
		return opTransfer(op, pool, transfer), nil

	case core.OpTypeTokenApproval:
		approval, err := txcommon.RetrieveTokenApprovalInputs(ctx, op)
		if err != nil {
			return nil, err
		}
		pool, err := am.GetTokenPoolByID(ctx, approval.Pool)
		if err != nil {
			return nil, err
		} else if pool == nil {
			return nil, i18n.NewError(ctx, coremsgs.Msg404NotFound)
		}
		return opApproval(op, pool, approval), nil

	default:
		return nil, i18n.NewError(ctx, coremsgs.MsgOperationNotSupported, op.Type)
	}
}

func (am *assetManager) RunOperation(ctx context.Context, op *core.PreparedOperation) (outputs fftypes.JSONObject, phase core.OpPhase, err error) {
	switch data := op.Data.(type) {
	case createPoolData:
		plugin, err := am.selectTokenPlugin(ctx, data.Pool.Connector)
		if err != nil {
			return nil, core.OpPhaseInitializing, err
		}
		phase, err = plugin.CreateTokenPool(ctx, op.NamespacedIDString(), data.Pool)
		return nil, phase, err

	case activatePoolData:
		plugin, err := am.selectTokenPlugin(ctx, data.Pool.Connector)
		if err != nil {
			return nil, core.OpPhaseInitializing, err
		}
		phase, err = plugin.ActivateTokenPool(ctx, data.Pool)
		return nil, phase, err

	case transferData:
		plugin, err := am.selectTokenPlugin(ctx, data.Pool.Connector)
		if err != nil {
			return nil, core.OpPhaseInitializing, err
		}
		switch data.Transfer.Type {
		case core.TokenTransferTypeMint:
			err = plugin.MintTokens(ctx, op.NamespacedIDString(), data.Pool.Locator, data.Transfer, data.Pool.Methods)
		case core.TokenTransferTypeTransfer:
			err = plugin.TransferTokens(ctx, op.NamespacedIDString(), data.Pool.Locator, data.Transfer, data.Pool.Methods)
		case core.TokenTransferTypeBurn:
			err = plugin.BurnTokens(ctx, op.NamespacedIDString(), data.Pool.Locator, data.Transfer, data.Pool.Methods)
		default:
			panic(fmt.Sprintf("unknown transfer type: %v", data.Transfer.Type))
		}
		return nil, operations.ErrTernary(err, core.OpPhaseInitializing, core.OpPhasePending), err

	case approvalData:
		plugin, err := am.selectTokenPlugin(ctx, data.Pool.Connector)
		if err != nil {
			return nil, core.OpPhaseInitializing, err
		}
		return nil, core.OpPhaseInitializing, plugin.TokensApproval(ctx, op.NamespacedIDString(), data.Pool.Locator, data.Approval, data.Pool.Methods)

	default:
		return nil, core.OpPhaseInitializing, i18n.NewError(ctx, coremsgs.MsgOperationDataIncorrect, op.Data)
	}
}

func (am *assetManager) OnOperationUpdate(ctx context.Context, op *core.Operation, update *core.OperationUpdate) error {
	// Write an event for failed pool operations
	if op.Type == core.OpTypeTokenCreatePool && update.Status == core.OpStatusFailed {
		tokenPool, err := txcommon.RetrieveTokenPoolCreateInputs(ctx, op)
		topic := ""
		if tokenPool != nil {
			topic = tokenPool.ID.String()
		}
		event := core.NewEvent(core.EventTypePoolOpFailed, op.Namespace, op.ID, op.Transaction, topic)
		if err != nil || tokenPool.ID == nil {
			log.L(ctx).Warnf("Could not parse token pool: %s (%+v)", err, op.Input)
		} else {
			event.Correlator = tokenPool.ID
		}
		if err := am.database.InsertEvent(ctx, event); err != nil {
			return err
		}
	}

	// Write an event for failed transfer operations
	if op.Type == core.OpTypeTokenTransfer && update.Status == core.OpStatusFailed {
		tokenTransfer, err := txcommon.RetrieveTokenTransferInputs(ctx, op)
		topic := ""
		if tokenTransfer != nil {
			topic = tokenTransfer.Pool.String()
		}
		event := core.NewEvent(core.EventTypeTransferOpFailed, op.Namespace, op.ID, op.Transaction, topic)
		if err != nil || tokenTransfer.LocalID == nil || tokenTransfer.Type == "" {
			log.L(ctx).Warnf("Could not parse token transfer: %s (%+v)", err, op.Input)
		} else {
			event.Correlator = tokenTransfer.LocalID
		}
		if err := am.database.InsertEvent(ctx, event); err != nil {
			return err
		}
	}

	// Write an event for failed approval operations
	if op.Type == core.OpTypeTokenApproval && update.Status == core.OpStatusFailed {
		tokenApproval, err := txcommon.RetrieveTokenApprovalInputs(ctx, op)
		topic := ""
		if tokenApproval != nil {
			topic = tokenApproval.Pool.String()
		}
		event := core.NewEvent(core.EventTypeApprovalOpFailed, op.Namespace, op.ID, op.Transaction, topic)
		if err != nil || tokenApproval.LocalID == nil {
			log.L(ctx).Warnf("Could not parse token approval: %s (%+v)", err, op.Input)
		} else {
			event.Correlator = tokenApproval.LocalID
		}
		if err := am.database.InsertEvent(ctx, event); err != nil {
			return err
		}
	}

	return nil
}

func opCreatePool(op *core.Operation, pool *core.TokenPool) *core.PreparedOperation {
	return &core.PreparedOperation{
		ID:        op.ID,
		Namespace: op.Namespace,
		Plugin:    op.Plugin,
		Type:      op.Type,
		Data:      createPoolData{Pool: pool},
	}
}

func opActivatePool(op *core.Operation, pool *core.TokenPool) *core.PreparedOperation {
	return &core.PreparedOperation{
		ID:        op.ID,
		Namespace: op.Namespace,
		Plugin:    op.Plugin,
		Type:      op.Type,
		Data:      activatePoolData{Pool: pool},
	}
}

func opTransfer(op *core.Operation, pool *core.TokenPool, transfer *core.TokenTransfer) *core.PreparedOperation {
	return &core.PreparedOperation{
		ID:        op.ID,
		Namespace: op.Namespace,
		Plugin:    op.Plugin,
		Type:      op.Type,
		Data:      transferData{Pool: pool, Transfer: transfer},
	}
}

func opApproval(op *core.Operation, pool *core.TokenPool, approval *core.TokenApproval) *core.PreparedOperation {
	return &core.PreparedOperation{
		ID:        op.ID,
		Namespace: op.Namespace,
		Plugin:    op.Plugin,
		Type:      op.Type,
		Data:      approvalData{Pool: pool, Approval: approval},
	}
}
