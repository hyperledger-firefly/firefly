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

package syncasync

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/internal/data"
	"github.com/hyperledger-firefly/firefly/internal/events/system"
	"github.com/hyperledger-firefly/firefly/internal/operations"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

// Bridge translates synchronous (HTTP API) calls, into asynchronously sending a
// message and blocking until a correlating response is received, or we hit a timeout.
type Bridge interface {
	// Init is required as there's a bi-directional relationship between event manager and syncasync bridge
	Init(sysevents system.EventInterface)

	// The following "WaitFor*" methods all wait for a particular type of event callback, and block until it is received.
	// To use them, invoke the appropriate method, and pass a "send" callback that is expected to trigger the relevant event.

	// WaitForReply waits for a reply to the message with the supplied ID
	WaitForReply(ctx context.Context, id *fftypes.UUID, send SendFunction) (*core.MessageInOut, error)
	// WaitForMessage waits for a message with the supplied ID
	WaitForMessage(ctx context.Context, id *fftypes.UUID, send SendFunction) (*core.Message, error)
	// WaitForIdentity waits for an identity with the supplied ID
	WaitForIdentity(ctx context.Context, id *fftypes.UUID, send SendFunction) (*core.Identity, error)
	// WaitForTokenPool waits for a token pool with the supplied ID
	WaitForTokenPool(ctx context.Context, id *fftypes.UUID, send SendFunction) (*core.TokenPool, error)
	// WaitForTokenTransfer waits for a token transfer with the supplied ID
	WaitForTokenTransfer(ctx context.Context, id *fftypes.UUID, send SendFunction) (*core.TokenTransfer, error)
	// WaitForTokenTransfer waits for a token approval with the supplied ID
	WaitForTokenApproval(ctx context.Context, id *fftypes.UUID, send SendFunction) (*core.TokenApproval, error)
	// WaitForInvokeOperation waits for an operation with the supplied ID
	WaitForInvokeOperation(ctx context.Context, id *fftypes.UUID, send SendFunction) (*core.Operation, error)
	// WaitForDeployOperation waits for an operation with the supplied ID
	WaitForDeployOperation(ctx context.Context, id *fftypes.UUID, send SendFunction) (*core.Operation, error)
}

// Sender interface may be implemented by other types that wish to provide generic sync/async capabilities
type Sender interface {
	Prepare(ctx context.Context) error
	Send(ctx context.Context) error
	SendAndWait(ctx context.Context) error
}

type SendFunction func(ctx context.Context) error

type requestType int

const (
	messageConfirm requestType = iota
	messageReply
	identityConfirm
	tokenPoolConfirm
	tokenTransferConfirm
	tokenApproveConfirm
	invokeOperationConfirm
	deployOperationConfirm
)

type inflightRequest struct {
	id        *fftypes.UUID
	startTime time.Time
	response  chan inflightResponse
	reqType   requestType
}

type inflightResponse struct {
	id   *fftypes.UUID
	data interface{}
	err  error
}

type inflightRequestMap map[string]map[fftypes.UUID]*inflightRequest

type syncAsyncBridge struct {
	ctx         context.Context
	namespace   string
	database    database.Plugin
	data        data.Manager
	operations  operations.Manager
	sysevents   system.EventInterface
	inflightMux sync.Mutex
	inflight    inflightRequestMap
}

func NewSyncAsyncBridge(ctx context.Context, ns string, di database.Plugin, dm data.Manager, om operations.Manager) Bridge {
	sa := &syncAsyncBridge{
		ctx:        log.WithLogFields(ctx, "role", "sync-async-bridge"),
		namespace:  ns,
		database:   di,
		data:       dm,
		operations: om,
		inflight:   make(inflightRequestMap),
	}
	return sa
}

func (sa *syncAsyncBridge) Init(sysevents system.EventInterface) {
	sa.sysevents = sysevents
}

func (sa *syncAsyncBridge) addInFlight(ns string, id *fftypes.UUID, reqType requestType) (*inflightRequest, error) {
	inflight := &inflightRequest{
		id:        id,
		startTime: time.Now(),
		response:  make(chan inflightResponse),
		reqType:   reqType,
	}
	sa.inflightMux.Lock()
	defer func() {
		sa.inflightMux.Unlock()
	}()

	inflightNS := sa.inflight[ns]
	if inflightNS == nil {
		err := sa.sysevents.AddSystemEventListener(ns, sa.eventCallback)
		if err != nil {
			return nil, err
		}
		inflightNS = make(map[fftypes.UUID]*inflightRequest)
		sa.inflight[ns] = inflightNS
	}
	inflightNS[*inflight.id] = inflight
	return inflight, nil
}

func (sa *syncAsyncBridge) getInFlight(ns string, reqType requestType, id *fftypes.UUID) *inflightRequest {
	if id == nil {
		return nil
	}
	inflightNS := sa.inflight[ns]
	if inflightNS != nil && id != nil {
		inflight := inflightNS[*id]
		if inflight != nil && inflight.reqType == reqType {
			return inflight
		}
	}
	return nil
}

func (sa *syncAsyncBridge) removeInFlight(ns string, id *fftypes.UUID) {
	sa.inflightMux.Lock()
	defer func() {
		sa.inflightMux.Unlock()
	}()
	inflightNS := sa.inflight[ns]
	if inflightNS != nil {
		delete(inflightNS, *id)
	}
}

func (inflight *inflightRequest) msInflight() float64 {
	dur := time.Since(inflight.startTime)
	return float64(dur) / float64(time.Millisecond)
}

func (sa *syncAsyncBridge) getMessageFromEvent(event *core.EventDelivery) (msg *core.Message, err error) {
	if msg, err = sa.database.GetMessageByID(sa.ctx, sa.namespace, event.Reference); err != nil {
		return nil, err
	}
	if msg == nil {
		// This should not happen (but we need to move on)
		log.L(sa.ctx).Errorf("Unable to resolve message '%s' for %s event '%s'", event.Reference, event.Type, event.ID)
	}
	return msg, nil
}

func (sa *syncAsyncBridge) getIdentityFromEvent(event *core.EventDelivery) (identity *core.Identity, err error) {
	if identity, err = sa.database.GetIdentityByID(sa.ctx, sa.namespace, event.Reference); err != nil {
		return nil, err
	}
	if identity == nil {
		// This should not happen (but we need to move on)
		log.L(sa.ctx).Errorf("Unable to resolve identity '%s' for %s event '%s'", event.Reference, event.Type, event.ID)
	}
	return identity, nil
}

func (sa *syncAsyncBridge) getPoolFromEvent(event *core.EventDelivery) (pool *core.TokenPool, err error) {
	if pool, err = sa.database.GetTokenPoolByID(sa.ctx, sa.namespace, event.Reference); err != nil {
		return nil, err
	}
	if pool == nil {
		// This should not happen (but we need to move on)
		log.L(sa.ctx).Errorf("Unable to resolve token pool '%s' for %s event '%s'", event.Reference, event.Type, event.ID)
	}
	return pool, nil
}

func (sa *syncAsyncBridge) getPoolFromMessage(msg *core.Message) (*core.TokenPool, error) {
	if len(msg.Data) > 0 {
		data, err := sa.database.GetDataByID(sa.ctx, sa.namespace, msg.Data[0].ID, true)
		if err != nil || data == nil {
			return nil, err
		}
		var pool core.TokenPoolDefinition
		if err := json.Unmarshal(data.Value.Bytes(), &pool); err == nil {
			return pool.Pool, nil
		}
	}
	return nil, nil
}

func (sa *syncAsyncBridge) getTransferFromEvent(event *core.EventDelivery) (transfer *core.TokenTransfer, err error) {
	if transfer, err = sa.database.GetTokenTransferByID(sa.ctx, sa.namespace, event.Reference); err != nil {
		return nil, err
	}
	if transfer == nil {
		// This should not happen (but we need to move on)
		log.L(sa.ctx).Errorf("Unable to resolve token transfer '%s' for %s event '%s'", event.Reference, event.Type, event.ID)
	}
	return transfer, nil
}

func (sa *syncAsyncBridge) getApprovalFromEvent(event *core.EventDelivery) (approval *core.TokenApproval, err error) {
	if approval, err = sa.database.GetTokenApprovalByID(sa.ctx, sa.namespace, event.Reference); err != nil {
		return nil, err
	}

	if approval == nil {
		// This should not happen (but we need to move on)
		log.L(sa.ctx).Errorf("Unable to resolve token approval '%s' for %s event '%s'", event.Reference, event.Type, event.ID)
	}
	return approval, nil
}

func (sa *syncAsyncBridge) getOperationFromEvent(event *core.EventDelivery) (op *core.Operation, err error) {
	if op, err = sa.operations.GetOperationByIDCached(sa.ctx, event.Reference); err != nil {
		return nil, err
	}
	if op == nil {
		// This should not happen (but we need to move on)
		log.L(sa.ctx).Errorf("Unable to resolve operation '%s' for %s event '%s'", event.Reference, event.Type, event.ID)
	}
	return op, nil
}

func (sa *syncAsyncBridge) handleMessageConfirmedEvent(event *core.EventDelivery) error {

	// See if the CID marks this as a reply to an inflight message
	inflight := sa.getInFlight(event.Namespace, messageConfirm, event.Reference)
	inflightReply := sa.getInFlight(event.Namespace, messageReply, event.Correlator)

	if inflightReply == nil && inflight == nil {
		return nil
	}

	msg, err := sa.getMessageFromEvent(event)
	if err != nil || msg == nil {
		return err
	}

	if inflightReply != nil {
		go sa.resolveReply(inflightReply, msg)
	}
	if inflight != nil {
		go sa.resolveConfirmed(inflight, msg)
	}

	return nil
}

func (sa *syncAsyncBridge) handleMessageRejectedEvent(event *core.EventDelivery) error {

	// See if this is a rejection of an inflight message
	inflight := sa.getInFlight(event.Namespace, messageConfirm, event.Reference)
	inflightPool := sa.getInFlight(event.Namespace, tokenPoolConfirm, event.Correlator)

	if inflight == nil && inflightPool == nil {
		return nil
	}

	msg, err := sa.getMessageFromEvent(event)
	if err != nil || msg == nil {
		return err
	}
	if inflight != nil {
		go sa.resolveRejected(inflight, msg.Header.ID)
	}

	// See if this is a rejection of an inflight token pool
	if inflightPool != nil {
		if pool, err := sa.getPoolFromMessage(msg); err != nil {
			return err
		} else if pool != nil {
			go sa.resolveRejectedTokenPool(inflightPool, pool.ID)
		}
	}

	return nil
}

func (sa *syncAsyncBridge) handleIdentityConfirmedEvent(event *core.EventDelivery) error {
	// See if the CID marks this as a reply to an inflight identity
	inflightReply := sa.getInFlight(event.Namespace, identityConfirm, event.Reference)
	if inflightReply == nil {
		return nil
	}

	identity, err := sa.getIdentityFromEvent(event)
	if err != nil || identity == nil {
		return err
	}

	go sa.resolveIdentity(inflightReply, identity)

	return nil
}

func (sa *syncAsyncBridge) handlePoolConfirmedEvent(event *core.EventDelivery) error {
	// See if this is a confirmation of an inflight token pool
	inflight := sa.getInFlight(event.Namespace, tokenPoolConfirm, event.Reference)
	if inflight == nil {
		return nil
	}

	pool, err := sa.getPoolFromEvent(event)
	if err != nil || pool == nil {
		return err
	}

	go sa.resolveConfirmedTokenPool(inflight, pool)

	return nil
}

func (sa *syncAsyncBridge) handlePoolOpFailedEvent(event *core.EventDelivery) error {
	// See if this is a failure of an inflight token pool operation
	inflight := sa.getInFlight(event.Namespace, tokenPoolConfirm, event.Correlator)
	if inflight == nil {
		return nil
	}

	op, err := sa.getOperationFromEvent(event)
	if err != nil || op == nil {
		return err
	}

	go sa.resolveFailedOperation(inflight, "token pool", op)

	return nil
}

func (sa *syncAsyncBridge) handleTransferConfirmedEvent(event *core.EventDelivery) error {
	// See if this is a confirmation of an inflight token transfer
	inflight := sa.getInFlight(event.Namespace, tokenTransferConfirm, event.Reference)
	if inflight == nil {
		return nil
	}

	transfer, err := sa.getTransferFromEvent(event)
	if err != nil || transfer == nil {
		return err
	}

	go sa.resolveConfirmedTokenTransfer(inflight, transfer)

	return nil
}

func (sa *syncAsyncBridge) handleTransferOpFailedEvent(event *core.EventDelivery) error {
	// See if this is a failure of an inflight token transfer operation
	inflight := sa.getInFlight(event.Namespace, tokenTransferConfirm, event.Correlator)
	if inflight == nil {
		return nil
	}

	op, err := sa.getOperationFromEvent(event)
	if err != nil || op == nil {
		return err
	}

	go sa.resolveFailedOperation(inflight, "token transfer", op)

	return nil
}

func (sa *syncAsyncBridge) handleApprovalConfirmedEvent(event *core.EventDelivery) error {

	// See if this is a confirmation of an inflight token approval
	inflight := sa.getInFlight(event.Namespace, tokenApproveConfirm, event.Reference)
	if inflight == nil {
		return nil
	}

	approval, err := sa.getApprovalFromEvent(event)
	if err != nil || approval == nil {
		return err
	}

	go sa.resolveConfirmedTokenApproval(inflight, approval)

	return nil
}

func (sa *syncAsyncBridge) handleApprovalOpFailedEvent(event *core.EventDelivery) error {
	// See if this is a failure of an inflight token approval operation
	inflight := sa.getInFlight(event.Namespace, tokenApproveConfirm, event.Correlator)
	if inflight == nil {
		return nil
	}

	op, err := sa.getOperationFromEvent(event)
	if err != nil || op == nil {
		return err
	}

	go sa.resolveFailedOperation(inflight, "token approval", op)

	return nil
}

func (sa *syncAsyncBridge) handleOperationSucceededEvent(event *core.EventDelivery) error {
	// See if this is a failure of an inflight invoke operation
	inflight := sa.getInFlight(event.Namespace, invokeOperationConfirm, event.Reference)
	if inflight == nil {
		return nil
	}

	op, err := sa.getOperationFromEvent(event)
	if err != nil || op == nil {
		return err
	}

	go sa.resolveSuccessfulOperation(inflight, "invoke", op)

	return nil
}

func (sa *syncAsyncBridge) handleContractDeployOperationSucceededEvent(event *core.EventDelivery) error {
	// See if this is a failure of an inflight invoke operation
	inflight := sa.getInFlight(event.Namespace, deployOperationConfirm, event.Reference)
	if inflight == nil {
		return nil
	}

	op, err := sa.getOperationFromEvent(event)
	if err != nil || op == nil {
		return err
	}

	go sa.resolveSuccessfulOperation(inflight, "deploy", op)

	return nil
}

func (sa *syncAsyncBridge) handleContractDeployOperationFailedEvent(event *core.EventDelivery) error {
	// See if this is a failure of an inflight deploy operation
	inflight := sa.getInFlight(event.Namespace, deployOperationConfirm, event.Reference)
	if inflight == nil {
		return nil
	}

	op, err := sa.getOperationFromEvent(event)
	if err != nil || op == nil {
		return err
	}

	go sa.resolveFailedOperation(inflight, "deploy", op)

	return nil
}

func (sa *syncAsyncBridge) handleOperationFailedEvent(event *core.EventDelivery) error {
	// See if this is a failure of an inflight invoke operation
	inflight := sa.getInFlight(event.Namespace, invokeOperationConfirm, event.Reference)
	if inflight == nil {
		return nil
	}

	op, err := sa.getOperationFromEvent(event)
	if err != nil || op == nil {
		return err
	}

	go sa.resolveFailedOperation(inflight, "invoke", op)

	return nil
}

func (sa *syncAsyncBridge) eventCallback(event *core.EventDelivery) error {
	sa.inflightMux.Lock()
	defer sa.inflightMux.Unlock()

	inflightNS := sa.inflight[event.Namespace]
	if len(inflightNS) == 0 {
		// No need to do any expensive lookups/matching - this could not be a match
		return nil
	}

	switch event.Type {
	case core.EventTypeMessageConfirmed:
		return sa.handleMessageConfirmedEvent(event)

	case core.EventTypeMessageRejected:
		return sa.handleMessageRejectedEvent(event)

	case core.EventTypeIdentityConfirmed:
		return sa.handleIdentityConfirmedEvent(event)

	case core.EventTypePoolConfirmed:
		return sa.handlePoolConfirmedEvent(event)

	case core.EventTypePoolOpFailed:
		return sa.handlePoolOpFailedEvent(event)

	case core.EventTypeTransferConfirmed:
		return sa.handleTransferConfirmedEvent(event)

	case core.EventTypeTransferOpFailed:
		return sa.handleTransferOpFailedEvent(event)

	case core.EventTypeApprovalConfirmed:
		return sa.handleApprovalConfirmedEvent(event)

	case core.EventTypeApprovalOpFailed:
		return sa.handleApprovalOpFailedEvent(event)

	case core.EventTypeBlockchainInvokeOpSucceeded:
		return sa.handleOperationSucceededEvent(event)

	case core.EventTypeBlockchainInvokeOpFailed:
		return sa.handleOperationFailedEvent(event)

	case core.EventTypeBlockchainContractDeployOpSucceeded:
		return sa.handleContractDeployOperationSucceededEvent(event)

	case core.EventTypeBlockchainContractDeployOpFailed:
		return sa.handleContractDeployOperationFailedEvent(event)
	}

	return nil
}

func (sa *syncAsyncBridge) resolveReply(inflight *inflightRequest, msg *core.Message) {
	log.L(sa.ctx).Debugf("Resolving reply request '%s' with message '%s'", inflight.id, msg.Header.ID)

	response := &core.MessageInOut{Message: *msg}
	data, _, err := sa.data.GetMessageDataCached(sa.ctx, msg)
	if err != nil {
		log.L(sa.ctx).Errorf("Failed to read response data for message '%s' on request '%s': %s", msg.Header.ID, inflight.id, err)
		return
	}
	response.SetInlineData(data)
	inflight.response <- inflightResponse{id: msg.Header.ID, data: response}
}

func (sa *syncAsyncBridge) resolveConfirmed(inflight *inflightRequest, msg *core.Message) {
	log.L(sa.ctx).Debugf("Resolving message confirmation request '%s' with ID '%s'", inflight.id, msg.Header.ID)
	inflight.response <- inflightResponse{id: msg.Header.ID, data: msg}
}

func (sa *syncAsyncBridge) resolveRejected(inflight *inflightRequest, msgID *fftypes.UUID) {
	err := i18n.NewError(sa.ctx, coremsgs.MsgRejected, msgID)
	log.L(sa.ctx).Errorf("Resolving message confirmation request '%s' with error: %s", inflight.id, err)
	inflight.response <- inflightResponse{err: err}
}

func (sa *syncAsyncBridge) resolveIdentity(inflight *inflightRequest, identity *core.Identity) {
	log.L(sa.ctx).Debugf("Resolving identity creation '%s' with ID '%s'", inflight.id, identity.ID)
	inflight.response <- inflightResponse{id: identity.ID, data: identity}
}

func (sa *syncAsyncBridge) resolveConfirmedTokenPool(inflight *inflightRequest, pool *core.TokenPool) {
	log.L(sa.ctx).Debugf("Resolving token pool confirmation request '%s' with ID '%s'", inflight.id, pool.ID)
	inflight.response <- inflightResponse{id: pool.ID, data: pool}
}

func (sa *syncAsyncBridge) resolveRejectedTokenPool(inflight *inflightRequest, poolID *fftypes.UUID) {
	err := i18n.NewError(sa.ctx, coremsgs.MsgTokenPoolRejected, poolID)
	log.L(sa.ctx).Errorf("Resolving token pool confirmation request '%s' with error '%s'", inflight.id, err)
	inflight.response <- inflightResponse{err: err}
}

func (sa *syncAsyncBridge) resolveConfirmedTokenTransfer(inflight *inflightRequest, transfer *core.TokenTransfer) {
	log.L(sa.ctx).Debugf("Resolving token transfer confirmation request '%s' with ID '%s'", inflight.id, transfer.LocalID)
	inflight.response <- inflightResponse{id: transfer.LocalID, data: transfer}
}

func (sa *syncAsyncBridge) resolveConfirmedTokenApproval(inflight *inflightRequest, approval *core.TokenApproval) {
	log.L(sa.ctx).Debugf("Resolving token approval confirmation request '%s' with ID '%s'", inflight.id, approval.LocalID)
	inflight.response <- inflightResponse{id: approval.LocalID, data: approval}
}

func (sa *syncAsyncBridge) resolveSuccessfulOperation(inflight *inflightRequest, typeName string, op *core.Operation) {
	log.L(sa.ctx).Debugf("Resolving %s request '%s' with ID '%s'", typeName, inflight.id, op.ID)
	inflight.response <- inflightResponse{id: op.ID, data: op}
}

func (sa *syncAsyncBridge) resolveFailedOperation(inflight *inflightRequest, typeName string, op *core.Operation) {
	log.L(sa.ctx).Debugf("Resolving %s request '%s' with error '%s'", typeName, inflight.id, op.Error)
	inflight.response <- inflightResponse{err: errors.New(op.Error)}
}

func (sa *syncAsyncBridge) sendAndWait(ctx context.Context, ns string, id *fftypes.UUID, reqType requestType, send SendFunction) (interface{}, error) {
	inflight, err := sa.addInFlight(ns, id, reqType)
	if err != nil {
		return nil, err
	}
	log.L(sa.ctx).Infof("Inflight request '%s' added", inflight.id)
	var replyID *fftypes.UUID
	defer func() {
		sa.removeInFlight(ns, inflight.id)
		if replyID != nil {
			log.L(sa.ctx).Infof("Inflight request '%s' resolved with reply '%s' after %.2fms", inflight.id, replyID, inflight.msInflight())
		} else {
			log.L(sa.ctx).Infof("Inflight request '%s' resolved with timeout after %.2fms", inflight.id, inflight.msInflight())
		}
	}()

	err = send(ctx)
	if err != nil {
		return nil, err
	}

	select {
	case <-ctx.Done():
		return nil, i18n.NewError(ctx, coremsgs.MsgRequestTimeout, inflight.id, inflight.msInflight())
	case reply := <-inflight.response:
		replyID = reply.id
		return reply.data, reply.err
	}
}

func (sa *syncAsyncBridge) WaitForReply(ctx context.Context, id *fftypes.UUID, send SendFunction) (*core.MessageInOut, error) {
	reply, err := sa.sendAndWait(ctx, sa.namespace, id, messageReply, send)
	if err != nil {
		return nil, err
	}
	return reply.(*core.MessageInOut), err
}

func (sa *syncAsyncBridge) WaitForMessage(ctx context.Context, id *fftypes.UUID, send SendFunction) (*core.Message, error) {
	reply, err := sa.sendAndWait(ctx, sa.namespace, id, messageConfirm, send)
	if err != nil {
		return nil, err
	}
	return reply.(*core.Message), err
}

func (sa *syncAsyncBridge) WaitForIdentity(ctx context.Context, id *fftypes.UUID, send SendFunction) (*core.Identity, error) {
	reply, err := sa.sendAndWait(ctx, sa.namespace, id, identityConfirm, send)
	if err != nil {
		return nil, err
	}
	return reply.(*core.Identity), err
}

func (sa *syncAsyncBridge) WaitForTokenPool(ctx context.Context, id *fftypes.UUID, send SendFunction) (*core.TokenPool, error) {
	reply, err := sa.sendAndWait(ctx, sa.namespace, id, tokenPoolConfirm, send)
	if err != nil {
		return nil, err
	}
	return reply.(*core.TokenPool), err
}

func (sa *syncAsyncBridge) WaitForTokenTransfer(ctx context.Context, id *fftypes.UUID, send SendFunction) (*core.TokenTransfer, error) {
	reply, err := sa.sendAndWait(ctx, sa.namespace, id, tokenTransferConfirm, send)
	if err != nil {
		return nil, err
	}
	return reply.(*core.TokenTransfer), err
}

func (sa *syncAsyncBridge) WaitForTokenApproval(ctx context.Context, id *fftypes.UUID, send SendFunction) (*core.TokenApproval, error) {
	reply, err := sa.sendAndWait(ctx, sa.namespace, id, tokenApproveConfirm, send)
	if err != nil {
		return nil, err
	}
	return reply.(*core.TokenApproval), err
}

func (sa *syncAsyncBridge) WaitForInvokeOperation(ctx context.Context, id *fftypes.UUID, send SendFunction) (*core.Operation, error) {
	reply, err := sa.sendAndWait(ctx, sa.namespace, id, invokeOperationConfirm, send)
	if err != nil {
		return nil, err
	}
	return reply.(*core.Operation), err
}

func (sa *syncAsyncBridge) WaitForDeployOperation(ctx context.Context, id *fftypes.UUID, send SendFunction) (*core.Operation, error) {
	reply, err := sa.sendAndWait(ctx, sa.namespace, id, deployOperationConfirm, send)
	if err != nil {
		return nil, err
	}
	return reply.(*core.Operation), err
}
