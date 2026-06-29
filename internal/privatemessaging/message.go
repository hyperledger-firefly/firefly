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

package privatemessaging

import (
	"context"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/internal/data"
	"github.com/hyperledger-firefly/firefly/internal/syncasync"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

func (pm *privateMessaging) NewMessage(in *core.MessageInOut) syncasync.Sender {
	message := &messageSender{
		mgr: pm,
		msg: &data.NewMessage{
			Message: in,
		},
	}
	if pm.metrics.IsMetricsEnabled() {
		pm.metrics.MessageSubmitted(&in.Message)
	}
	message.setDefaults()
	return message
}

func (pm *privateMessaging) SendMessage(ctx context.Context, in *core.MessageInOut, waitConfirm bool) (out *core.Message, err error) {
	message := pm.NewMessage(in)
	in.Header.Type = core.MessageTypePrivate
	if pm.metrics.IsMetricsEnabled() {
		pm.metrics.MessageSubmitted(&in.Message)
	}
	if waitConfirm {
		err = message.SendAndWait(ctx)
	} else {
		err = message.Send(ctx)
	}
	return &in.Message, err
}

func (pm *privateMessaging) RequestReply(ctx context.Context, in *core.MessageInOut) (*core.MessageInOut, error) {
	if in.Header.Tag == "" {
		return nil, i18n.NewError(ctx, coremsgs.MsgRequestReplyTagRequired)
	}
	if in.Header.CID != nil {
		return nil, i18n.NewError(ctx, coremsgs.MsgRequestCannotHaveCID)
	}
	message := pm.NewMessage(in)
	return pm.syncasync.WaitForReply(ctx, in.Header.ID, message.Send)
}

// sendMethod is the specific operation requested of the messageSender.
// To minimize duplication and group database operations, there is a single internal flow with subtle differences for each method.
type messageSender struct {
	mgr      *privateMessaging
	msg      *data.NewMessage
	resolved bool
}

type sendMethod int

const (
	// methodPrepare requests that the message be validated and sealed, but not sent (i.e. no database writes are performed)
	methodPrepare sendMethod = iota
	// methodSend requests that the message be sent and pinned to the blockchain, but does not wait for confirmation
	methodSend
	// methodSendAndWait requests that the message be sent and waits until it is pinned and confirmed by the blockchain
	methodSendAndWait
)

func (s *messageSender) Prepare(ctx context.Context) error {
	return s.resolveAndSend(ctx, methodPrepare)
}

func (s *messageSender) Send(ctx context.Context) error {
	return s.resolveAndSend(ctx, methodSend)
}

func (s *messageSender) SendAndWait(ctx context.Context) error {
	return s.resolveAndSend(ctx, methodSendAndWait)
}

func (s *messageSender) setDefaults() {
	msg := s.msg.Message
	msg.Header.ID = fftypes.NewUUID()
	msg.Header.Namespace = s.mgr.namespace.NetworkName
	msg.LocalNamespace = s.mgr.namespace.Name
	msg.State = core.MessageStateReady
	if msg.Header.Type == "" {
		msg.Header.Type = core.MessageTypePrivate
	}
	switch msg.Header.TxType {
	case core.TransactionTypeContractInvokePin:
		// valid
	case core.TransactionTypeUnpinned, core.TransactionTypeNone:
		// "unpinned" used to be called "none" (before we introduced batching + a TX on unpinned sends)
		msg.Header.TxType = core.TransactionTypeUnpinned
	default:
		// the only other valid option is "batch_pin"
		msg.Header.TxType = core.TransactionTypeBatchPin
	}
}

func (s *messageSender) resolveAndSend(ctx context.Context, method sendMethod) error {

	if !s.resolved {
		if err := s.resolve(ctx); err != nil {
			return err
		}
		msgSizeEstimate := s.msg.Message.EstimateSize(true)
		if msgSizeEstimate > s.mgr.maxBatchPayloadLength {
			return i18n.NewError(ctx, coremsgs.MsgTooLargePrivate, float64(msgSizeEstimate)/1024, float64(s.mgr.maxBatchPayloadLength)/1024)
		}
		s.resolved = true
	}

	return s.sendInternal(ctx, method)
}

func (s *messageSender) resolve(ctx context.Context) error {
	msg := s.msg.Message

	// Resolve the sending identity
	if err := s.mgr.identity.ResolveInputSigningIdentity(ctx, &msg.Header.SignerRef); err != nil {
		return i18n.WrapError(ctx, err, coremsgs.MsgAuthorInvalid)
	}

	// Resolve the member list into a group
	if err := s.mgr.resolveRecipientList(ctx, s.msg.Message); err != nil {
		return err
	}

	// The data manager is responsible for the heavy lifting of storing/validating all our in-line data elements
	err := s.mgr.data.ResolveInlineData(ctx, s.msg)
	return err
}

func (s *messageSender) sendInternal(ctx context.Context, method sendMethod) error {
	msg := &s.msg.Message.Message

	if method == methodSendAndWait {
		// Pass it to the sync-async handler to wait for the confirmation to come back in.
		// NOTE: Our caller makes sure we are not in a RunAsGroup (which would be bad)
		out, err := s.mgr.syncasync.WaitForMessage(ctx, msg.Header.ID, s.Send)
		if out != nil {
			*msg = *out
		}
		return err
	}

	// Seal the message
	if err := s.msg.Message.Seal(ctx); err != nil {
		return err
	}
	if method == methodPrepare {
		return nil
	}

	// Store the message - this asynchronously triggers the next step in process
	if err := s.mgr.data.WriteNewMessage(ctx, s.msg); err != nil {
		return err
	}
	log.L(ctx).Infof("Sent private message %s sequence=%d", msg.Header.ID, msg.Sequence)

	return nil
}
