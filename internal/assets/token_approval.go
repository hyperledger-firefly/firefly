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

func (am *assetManager) GetTokenApprovals(ctx context.Context, filter ffapi.AndFilter) ([]*core.TokenApproval, *ffapi.FilterResult, error) {
	return am.database.GetTokenApprovals(ctx, am.namespace, filter)
}

type approveSender struct {
	mgr              *assetManager
	approval         *core.TokenApprovalInput
	resolved         bool
	msgSender        syncasync.Sender
	idempotentSubmit bool
}

func (s *approveSender) Prepare(ctx context.Context) error {
	return s.resolveAndSend(ctx, methodPrepare)
}

func (s *approveSender) Send(ctx context.Context) error {
	return s.resolveAndSend(ctx, methodSend)
}

func (s *approveSender) SendAndWait(ctx context.Context) error {
	return s.resolveAndSend(ctx, methodSendAndWait)
}

func (s *approveSender) setDefaults() {
	s.approval.LocalID = fftypes.NewUUID()
}

func (am *assetManager) NewApproval(approval *core.TokenApprovalInput) syncasync.Sender {
	sender := &approveSender{
		mgr:              am,
		approval:         approval,
		idempotentSubmit: approval.IdempotencyKey != "",
	}
	if approval.Namespace == "" {
		approval.Namespace = am.namespace
	}
	sender.setDefaults()
	return sender
}

func (am *assetManager) TokenApproval(ctx context.Context, approval *core.TokenApprovalInput, waitConfirm bool) (out *core.TokenApproval, err error) {
	sender := am.NewApproval(approval)
	if waitConfirm {
		err = sender.SendAndWait(ctx)
	} else {
		err = sender.Send(ctx)
	}
	return &approval.TokenApproval, err
}

func (s *approveSender) resolveAndSend(ctx context.Context, method sendMethod) (err error) {
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

	if method == methodSendAndWait && s.approval.Message != nil {
		// Begin waiting for the message, and trigger the approval.
		// A successful approval will trigger the message via the event handler, so we can wait for it all to complete.
		_, err := s.mgr.syncasync.WaitForMessage(ctx, s.approval.Message.Header.ID, func(ctx context.Context) error {
			return s.sendInternal(ctx, methodSendAndWait)
		})
		return err
	}

	return s.sendInternal(ctx, method)
}

func (s *approveSender) resolve(ctx context.Context) (opResubmitted bool, err error) {
	// Create a transaction and attach to the approval
	txid, err := s.mgr.txHelper.SubmitNewTransaction(ctx, core.TransactionTypeTokenApproval, s.approval.IdempotencyKey)
	if err != nil {
		// Check if we've clashed on idempotency key. There might be operations still in "Initialized" state that need
		// submitting to their handlers
		resubmitWholeTX := false
		if idemErr, ok := err.(*sqlcommon.IdempotencyError); ok {
			total, resubmitted, resubmitErr := s.mgr.operations.ResubmitOperations(ctx, idemErr.ExistingTXID)
			if resubmitErr != nil {
				// Error doing resubmit, return the new error
				return false, resubmitErr
			}
			if total == 0 {
				// We didn't do anything last time - just start again
				txid = idemErr.ExistingTXID
				resubmitWholeTX = true
				err = nil
			} else if len(resubmitted) > 0 {
				// We resubmitted something - translate the status code to 200 (true return)
				s.approval.TX.ID = idemErr.ExistingTXID
				s.approval.TX.Type = core.TransactionTypeTokenApproval
				return true, nil
			}
		}
		if !resubmitWholeTX {
			return false, err
		}
	}
	s.approval.TX.ID = txid
	s.approval.TX.Type = core.TransactionTypeTokenApproval

	// Resolve the attached message
	if s.approval.Message != nil {
		s.approval.Message.Header.TxParent = &core.TransactionRef{
			ID:   txid,
			Type: core.TransactionTypeTokenApproval,
		}
		s.msgSender, err = s.buildApprovalMessage(ctx, s.approval.Message)
		if err != nil {
			return false, err
		}
		if err = s.msgSender.Prepare(ctx); err != nil {
			return false, err
		}
		s.approval.TokenApproval.Message = s.approval.Message.Header.ID
		s.approval.TokenApproval.MessageHash = s.approval.Message.Hash
	}
	return false, err
}

func (s *approveSender) sendInternal(ctx context.Context, method sendMethod) (err error) {
	if method == methodSendAndWait {
		out, err := s.mgr.syncasync.WaitForTokenApproval(ctx, s.approval.LocalID, s.Send)
		if out != nil {
			s.approval.TokenApproval = *out
		}
		return err
	}

	var op *core.Operation
	var pool *core.TokenPool
	err = s.mgr.database.RunAsGroup(ctx, func(ctx context.Context) (err error) {
		pool, err = s.mgr.validateApproval(ctx, s.approval)
		if err != nil {
			return err
		}

		plugin, err := s.mgr.selectTokenPlugin(ctx, s.approval.Connector)
		if err != nil {
			return err
		}

		if method == methodPrepare {
			return nil
		}

		op = core.NewOperation(
			plugin,
			s.mgr.namespace,
			s.approval.TX.ID,
			core.TransactionTypeTokenApproval)
		if err = txcommon.AddTokenApprovalInputs(op, &s.approval.TokenApproval); err == nil {
			err = s.mgr.operations.AddOrReuseOperation(ctx, op)
		}
		return err
	})
	if err != nil {
		return err
	} else if method == methodPrepare {
		return nil
	}

	// Write the approval message outside of any DB transaction, as it will use the background message writer.
	if s.approval.Message != nil {
		s.approval.Message.State = core.MessageStateStaged
		if err = s.msgSender.Send(ctx); err != nil {
			return err
		}
	}

	_, err = s.mgr.operations.RunOperation(ctx, opApproval(op, pool, &s.approval.TokenApproval), s.idempotentSubmit)
	return err
}

func (am *assetManager) validateApproval(ctx context.Context, approval *core.TokenApprovalInput) (pool *core.TokenPool, err error) {
	if approval.Pool == "" {
		pool, err = am.getDefaultTokenPool(ctx)
		if err != nil {
			return nil, err
		}
	} else {
		pool, err = am.GetTokenPoolByNameOrID(ctx, approval.Pool)
		if err != nil {
			return nil, err
		}
	}
	approval.TokenApproval.Pool = pool.ID
	approval.TokenApproval.Connector = pool.Connector

	if !pool.Active {
		return nil, i18n.NewError(ctx, coremsgs.MsgTokenPoolNotActive)
	}
	approval.Key, err = am.identity.ResolveInputSigningKey(ctx, approval.Key, am.keyNormalization)
	return pool, err
}

func (s *approveSender) buildApprovalMessage(ctx context.Context, in *core.MessageInOut) (syncasync.Sender, error) {
	allowedTypes := []fftypes.FFEnum{
		core.MessageTypeBroadcast,
		core.MessageTypePrivate,
		core.MessageTypeDeprecatedApprovalBroadcast,
		core.MessageTypeDeprecatedApprovalPrivate,
	}
	if in.Header.Type == "" {
		in.Header.Type = core.MessageTypeBroadcast
	}
	switch in.Header.Type {
	case core.MessageTypeBroadcast, core.MessageTypeDeprecatedApprovalBroadcast:
		if s.mgr.broadcast == nil {
			return nil, i18n.NewError(ctx, coremsgs.MsgMessagesNotSupported)
		}
		return s.mgr.broadcast.NewBroadcast(in), nil
	case core.MessageTypePrivate, core.MessageTypeDeprecatedApprovalPrivate:
		if s.mgr.messaging == nil {
			return nil, i18n.NewError(ctx, coremsgs.MsgMessagesNotSupported)
		}
		return s.mgr.messaging.NewMessage(in), nil
	default:
		return nil, i18n.NewError(ctx, coremsgs.MsgInvalidMessageType, allowedTypes)
	}
}
