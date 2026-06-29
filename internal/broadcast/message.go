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

package broadcast

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

func (bm *broadcastManager) NewBroadcast(in *core.MessageInOut) syncasync.Sender {
	broadcast := &broadcastSender{
		mgr: bm,
		msg: &data.NewMessage{
			Message: in,
		},
	}
	if bm.metrics.IsMetricsEnabled() {
		bm.metrics.MessageSubmitted(&in.Message)
	}
	broadcast.setDefaults()
	return broadcast
}

func (bm *broadcastManager) BroadcastMessage(ctx context.Context, in *core.MessageInOut, waitConfirm bool) (out *core.Message, err error) {
	broadcast := bm.NewBroadcast(in)
	in.Header.Type = core.MessageTypeBroadcast
	if bm.metrics.IsMetricsEnabled() {
		bm.metrics.MessageSubmitted(&in.Message)
	}
	if waitConfirm {
		err = broadcast.SendAndWait(ctx)
	} else {
		err = broadcast.Send(ctx)
	}
	return &in.Message, err
}

type broadcastSender struct {
	mgr      *broadcastManager
	msg      *data.NewMessage
	resolved bool
}

// sendMethod is the specific operation requested of the broadcastSender.
// To minimize duplication and group database operations, there is a single internal flow with subtle differences for each method.
type sendMethod int

const (
	// methodPrepare requests that the message be validated and sealed, but not sent (i.e. no database writes are performed)
	methodPrepare sendMethod = iota
	// methodSend requests that the message be sent and pinned to the blockchain, but does not wait for confirmation
	methodSend
	// methodSendAndWait requests that the message be sent and waits until it is pinned and confirmed by the blockchain
	methodSendAndWait
)

func (s *broadcastSender) Prepare(ctx context.Context) error {
	return s.resolveAndSend(ctx, methodPrepare)
}

func (s *broadcastSender) Send(ctx context.Context) error {
	return s.resolveAndSend(ctx, methodSend)
}

func (s *broadcastSender) SendAndWait(ctx context.Context) error {
	return s.resolveAndSend(ctx, methodSendAndWait)
}

func (s *broadcastSender) setDefaults() {
	msg := s.msg.Message
	msg.Header.ID = fftypes.NewUUID()
	msg.Header.Group = nil
	msg.Header.Namespace = s.mgr.namespace.NetworkName
	msg.LocalNamespace = s.mgr.namespace.Name
	msg.State = core.MessageStateReady
	if msg.Header.Type == "" {
		msg.Header.Type = core.MessageTypeBroadcast
	}
	switch msg.Header.TxType {
	case core.TransactionTypeContractInvokePin:
		// valid
	default:
		// the only other valid option is "batch_pin"
		msg.Header.TxType = core.TransactionTypeBatchPin
	}
}

func (s *broadcastSender) resolveAndSend(ctx context.Context, method sendMethod) error {

	if !s.resolved {
		if err := s.resolve(ctx); err != nil {
			return err
		}
		msgSizeEstimate := s.msg.Message.EstimateSize(true)
		if msgSizeEstimate > s.mgr.maxBatchPayloadLength {
			return i18n.NewError(ctx, coremsgs.MsgTooLargeBroadcast, float64(msgSizeEstimate)/1024, float64(s.mgr.maxBatchPayloadLength)/1024)
		}
		s.resolved = true
	}
	return s.sendInternal(ctx, method)
}

func (s *broadcastSender) resolve(ctx context.Context) error {
	msg := s.msg.Message

	// Resolve the sending identity
	if msg.Header.Type != core.MessageTypeDefinition || msg.Header.Tag != core.SystemTagIdentityClaim {
		if err := s.mgr.identity.ResolveInputSigningIdentity(ctx, &msg.Header.SignerRef); err != nil {
			return i18n.WrapError(ctx, err, coremsgs.MsgAuthorInvalid)
		}
	}

	// The data manager is responsible for the heavy lifting of storing/validating all our in-line data elements
	err := s.mgr.data.ResolveInlineData(ctx, s.msg)
	return err
}

func (s *broadcastSender) sendInternal(ctx context.Context, method sendMethod) (err error) {
	if method == methodSendAndWait {
		out, err := s.mgr.syncasync.WaitForMessage(ctx, s.msg.Message.Header.ID, s.Send)
		if out != nil {
			s.msg.Message.Message = *out
		}
		return err
	}

	// Seal the message
	msg := s.msg.Message
	if err := msg.Seal(ctx); err != nil {
		return err
	}
	if method == methodPrepare {
		return nil
	}

	// Write the message
	if err := s.mgr.data.WriteNewMessage(ctx, s.msg); err != nil {
		return err
	}
	log.L(ctx).Infof("Sent broadcast message %s sequence=%d datacount=%d", msg.Header.ID, msg.Sequence, len(s.msg.AllData))

	return err
}
