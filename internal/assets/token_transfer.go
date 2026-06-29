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

	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/internal/database/sqlcommon"
	"github.com/hyperledger-firefly/firefly/internal/syncasync"
	"github.com/hyperledger-firefly/firefly/internal/txcommon"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

func (am *assetManager) GetTokenTransfers(ctx context.Context, filter ffapi.AndFilter) ([]*core.TokenTransfer, *ffapi.FilterResult, error) {
	return am.database.GetTokenTransfers(ctx, am.namespace, filter)
}

func (am *assetManager) GetTokenTransferByID(ctx context.Context, id string) (*core.TokenTransfer, error) {
	transferID, err := fftypes.ParseUUID(ctx, id)
	if err != nil {
		return nil, err
	}
	return am.database.GetTokenTransferByID(ctx, am.namespace, transferID)
}

func (am *assetManager) NewTransfer(transfer *core.TokenTransferInput) syncasync.Sender {
	sender := &transferSender{
		mgr:              am,
		transfer:         transfer,
		idempotentSubmit: transfer.IdempotencyKey != "",
	}
	sender.setDefaults()
	return sender
}

type transferSender struct {
	mgr              *assetManager
	transfer         *core.TokenTransferInput
	resolved         bool
	msgSender        syncasync.Sender
	idempotentSubmit bool
}

// sendMethod is the specific operation requested of the transferSender.
// To minimize duplication and group database operations, there is a single internal flow with subtle differences for each method.
type sendMethod int

const (
	// methodPrepare requests that the transfer be validated and prepared, but not sent (i.e. no database writes are performed)
	methodPrepare sendMethod = iota
	// methodSend requests that the transfer be sent to the blockchain, but does not wait for confirmation
	methodSend
	// methodSendAndWait requests that the transfer be sent and waits until it is confirmed by the blockchain
	methodSendAndWait
)

func (s *transferSender) Prepare(ctx context.Context) error {
	return s.resolveAndSend(ctx, methodPrepare)
}

func (s *transferSender) Send(ctx context.Context) error {
	return s.resolveAndSend(ctx, methodSend)
}

func (s *transferSender) SendAndWait(ctx context.Context) error {
	return s.resolveAndSend(ctx, methodSendAndWait)
}

func (s *transferSender) setDefaults() {
	s.transfer.LocalID = fftypes.NewUUID()
}

func (am *assetManager) validateTransfer(ctx context.Context, transfer *core.TokenTransferInput) (pool *core.TokenPool, err error) {
	if transfer.Pool == "" {
		pool, err = am.getDefaultTokenPool(ctx)
		if err != nil {
			return nil, err
		}
	} else {
		pool, err = am.GetTokenPoolByNameOrID(ctx, transfer.Pool)
		if err != nil {
			return nil, err
		}
	}
	transfer.TokenTransfer.Pool = pool.ID
	transfer.TokenTransfer.Connector = pool.Connector

	if !pool.Active {
		return nil, i18n.NewError(ctx, coremsgs.MsgTokenPoolNotActive)
	}
	if transfer.Key, err = am.identity.ResolveInputSigningKey(ctx, transfer.Key, am.keyNormalization); err != nil {
		return nil, err
	}
	if transfer.From == "" {
		transfer.From = transfer.Key
	}
	if transfer.To == "" {
		transfer.To = transfer.Key
	}
	return pool, nil
}

func (am *assetManager) MintTokens(ctx context.Context, transfer *core.TokenTransferInput, waitConfirm bool) (out *core.TokenTransfer, err error) {
	transfer.Type = core.TokenTransferTypeMint
	if transfer.Namespace == "" {
		transfer.Namespace = am.namespace
	}

	sender := am.NewTransfer(transfer)
	if am.metrics.IsMetricsEnabled() {
		am.metrics.TransferSubmitted(&transfer.TokenTransfer)
	}
	if waitConfirm {
		err = sender.SendAndWait(ctx)
	} else {
		err = sender.Send(ctx)
	}
	return &transfer.TokenTransfer, err
}

func (am *assetManager) BurnTokens(ctx context.Context, transfer *core.TokenTransferInput, waitConfirm bool) (out *core.TokenTransfer, err error) {
	transfer.Type = core.TokenTransferTypeBurn
	if transfer.Namespace == "" {
		transfer.Namespace = am.namespace
	}

	sender := am.NewTransfer(transfer)
	if am.metrics.IsMetricsEnabled() {
		am.metrics.TransferSubmitted(&transfer.TokenTransfer)
	}
	if waitConfirm {
		err = sender.SendAndWait(ctx)
	} else {
		err = sender.Send(ctx)
	}
	return &transfer.TokenTransfer, err
}

func (am *assetManager) TransferTokens(ctx context.Context, transfer *core.TokenTransferInput, waitConfirm bool) (out *core.TokenTransfer, err error) {
	transfer.Type = core.TokenTransferTypeTransfer
	if transfer.Namespace == "" {
		transfer.Namespace = am.namespace
	}

	sender := am.NewTransfer(transfer)
	if am.metrics.IsMetricsEnabled() {
		am.metrics.TransferSubmitted(&transfer.TokenTransfer)
	}
	if waitConfirm {
		err = sender.SendAndWait(ctx)
	} else {
		err = sender.Send(ctx)
	}
	return &transfer.TokenTransfer, err
}

func (s *transferSender) resolveAndSend(ctx context.Context, method sendMethod) (err error) {
	if !s.resolved {
		var opResubmit bool
		if opResubmit, err = s.resolve(ctx); err != nil {
			return err
		}
		s.resolved = true
		if opResubmit {
			// Operation had already been created on a previous call but never got submitted. We've resubmitted
			// it now so no need to carry on
			return nil
		}
	}

	if method == methodSendAndWait && s.transfer.Message != nil {
		// Begin waiting for the message, and trigger the transfer.
		// A successful transfer will trigger the message via the event handler, so we can wait for it all to complete.
		_, err := s.mgr.syncasync.WaitForMessage(ctx, s.transfer.Message.Header.ID, func(ctx context.Context) error {
			return s.sendInternal(ctx, methodSendAndWait)
		})
		return err
	}

	return s.sendInternal(ctx, method)
}

func (s *transferSender) resolve(ctx context.Context) (opResubmitted bool, err error) {
	// Create a transaction and attach to the transfer
	txid, err := s.mgr.txHelper.SubmitNewTransaction(ctx, core.TransactionTypeTokenTransfer, s.transfer.IdempotencyKey)
	if err != nil {
		// Check if we've clashed on idempotency key. There might be operations still in "Initialized" state that need
		// submitting to their handlers. Note that we'll return the result of resubmitting the operation, not a 409 Conflict error
		resubmitWholeTX := false
		if idemErr, ok := err.(*sqlcommon.IdempotencyError); ok {
			total, resubmitted, resubmitErr := s.mgr.operations.ResubmitOperations(ctx, idemErr.ExistingTXID)
			if resubmitErr != nil {
				// Error doing resubmit, return the new error
				err = resubmitErr
			}
			if total == 0 {
				// We didn't do anything last time - just start again
				txid = idemErr.ExistingTXID
				resubmitWholeTX = true
				err = nil
			} else if len(resubmitted) > 0 {
				// We resubmitted something - translate the status code to 200 (true return)
				s.transfer.TX.ID = idemErr.ExistingTXID
				s.transfer.TX.Type = core.TransactionTypeTokenTransfer
				return true, nil
			}

		}
		if !resubmitWholeTX {
			return false, err
		}
	}
	s.transfer.TX.ID = txid
	s.transfer.TX.Type = core.TransactionTypeTokenTransfer

	// Resolve the attached message
	if s.transfer.Message != nil {
		s.transfer.Message.Header.TxParent = &core.TransactionRef{
			ID:   txid,
			Type: core.TransactionTypeTokenTransfer,
		}
		s.msgSender, err = s.buildTransferMessage(ctx, s.transfer.Message)
		if err != nil {
			return false, err
		}
		if err = s.msgSender.Prepare(ctx); err != nil {
			return false, err
		}
		s.transfer.TokenTransfer.Message = s.transfer.Message.Header.ID
		s.transfer.TokenTransfer.MessageHash = s.transfer.Message.Hash
	}
	return false, nil
}

func (s *transferSender) sendInternal(ctx context.Context, method sendMethod) (err error) {
	if method == methodSendAndWait {
		out, err := s.mgr.syncasync.WaitForTokenTransfer(ctx, s.transfer.LocalID, s.Send)
		if out != nil {
			s.transfer.TokenTransfer = *out
		}
		return err
	}

	var op *core.Operation
	var pool *core.TokenPool
	err = s.mgr.database.RunAsGroup(ctx, func(ctx context.Context) (err error) {
		pool, err = s.mgr.validateTransfer(ctx, s.transfer)
		if err != nil {
			return err
		}
		if s.transfer.Type == core.TokenTransferTypeTransfer && s.transfer.From == s.transfer.To {
			return i18n.NewError(ctx, coremsgs.MsgCannotTransferToSelf)
		}

		plugin, err := s.mgr.selectTokenPlugin(ctx, s.transfer.Connector)
		if err != nil {
			return err
		}

		if method == methodPrepare {
			return nil
		}

		op = core.NewOperation(
			plugin,
			s.mgr.namespace,
			s.transfer.TX.ID,
			core.OpTypeTokenTransfer)
		if err = txcommon.AddTokenTransferInputs(op, &s.transfer.TokenTransfer); err == nil {
			err = s.mgr.operations.AddOrReuseOperation(ctx, op)
		}
		return err
	})
	if err != nil {
		return err
	} else if method == methodPrepare {
		return nil
	}

	// Write the transfer message outside of any DB transaction, as it will use the background message writer.
	if s.transfer.Message != nil {
		s.transfer.Message.State = core.MessageStateStaged
		if err = s.msgSender.Send(ctx); err != nil {
			return err
		}
	}

	_, err = s.mgr.operations.RunOperation(ctx, opTransfer(op, pool, &s.transfer.TokenTransfer), s.idempotentSubmit)
	return err
}

func (s *transferSender) buildTransferMessage(ctx context.Context, in *core.MessageInOut) (syncasync.Sender, error) {
	allowedTypes := []fftypes.FFEnum{
		core.MessageTypeBroadcast,
		core.MessageTypePrivate,
		core.MessageTypeDeprecatedTransferBroadcast,
		core.MessageTypeDeprecatedTransferPrivate,
	}
	if in.Header.Type == "" {
		in.Header.Type = core.MessageTypeBroadcast
	}
	switch in.Header.Type {
	case core.MessageTypeBroadcast, core.MessageTypeDeprecatedTransferBroadcast:
		if s.mgr.broadcast == nil {
			return nil, i18n.NewError(ctx, coremsgs.MsgMessagesNotSupported)
		}
		return s.mgr.broadcast.NewBroadcast(in), nil
	case core.MessageTypePrivate, core.MessageTypeDeprecatedTransferPrivate:
		if s.mgr.messaging == nil {
			return nil, i18n.NewError(ctx, coremsgs.MsgMessagesNotSupported)
		}
		return s.mgr.messaging.NewMessage(in), nil
	default:
		return nil, i18n.NewError(ctx, coremsgs.MsgInvalidMessageType, allowedTypes)
	}
}
