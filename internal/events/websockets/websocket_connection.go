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

package websockets

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

type websocketStartedSub struct {
	core.WSStart
	startTime *fftypes.FFTime
}

type websocketConnection struct {
	ctx             context.Context
	ws              *WebSockets
	wsConn          *websocket.Conn
	cancelCtx       func()
	connID          string
	sendMessages    chan interface{}
	senderDone      chan struct{}
	receiverDone    chan struct{}
	autoAck         bool
	started         []*websocketStartedSub
	inflight        []*core.EventDeliveryResponse
	inflightBatches []*core.WSEventBatch
	mux             sync.Mutex
	closed          bool
	remoteAddr      string
	userAgent       string
	header          http.Header
	auth            core.Authorizer
	namespaceScoped bool // if true then any request to listen is asserted to be in the context of namespace
	namespace       string
}

func newConnection(pCtx context.Context, ws *WebSockets, wsConn *websocket.Conn, req *http.Request, auth core.Authorizer) *websocketConnection {
	connID := fftypes.NewUUID().String()
	ctx := log.WithLogFields(pCtx, "websocket", connID)
	ctx, cancelCtx := context.WithCancel(ctx)
	wc := &websocketConnection{
		ctx:          ctx,
		ws:           ws,
		wsConn:       wsConn,
		cancelCtx:    cancelCtx,
		connID:       connID,
		sendMessages: make(chan interface{}),
		senderDone:   make(chan struct{}),
		receiverDone: make(chan struct{}),
		remoteAddr:   req.RemoteAddr,
		userAgent:    req.UserAgent(),
		header:       req.Header,
		auth:         auth,
	}
	go wc.sendLoop()
	go wc.receiveLoop()
	return wc
}

func (wc *websocketConnection) assertNamespace(namespace string) (string, error) {

	if wc.namespaceScoped {
		if namespace == "" {
			namespace = wc.namespace
		} else if namespace != wc.namespace {
			return "", i18n.NewError(wc.ctx, coremsgs.MsgWSWrongNamespace)
		}
	}
	return namespace, nil
}

func isBoolQuerySet(query url.Values, boolOption string) bool {
	optionValues, hasOptionValues := query[boolOption]
	return hasOptionValues && (len(optionValues) == 0 || optionValues[0] != "false")
}

func (wc *websocketConnection) getReadAhead(query url.Values, isBatch bool) *uint {
	readaheadStr := query.Get("readahead")
	if readaheadStr != "" {
		readAheadInt, err := strconv.ParseUint(readaheadStr, 10, 16)
		if err == nil {
			readahead := uint(readAheadInt)
			return &readahead
		}
	}
	return nil
}

func (wc *websocketConnection) getBatchTimeout(query url.Values) *string {
	batchTimeout := query.Get("batchtimeout")
	if batchTimeout != "" {
		return &batchTimeout
	}
	return nil
}

// processAutoStart gives a helper to specify query parameters to auto-start your subscription
func (wc *websocketConnection) processAutoStart(req *http.Request) {
	query := req.URL.Query()
	isEphemeral := isBoolQuerySet(query, "ephemeral")
	_, hasName := query["name"]
	isAutoack := isBoolQuerySet(query, "autoack")
	namespace, err := wc.assertNamespace(query.Get("namespace"))
	if err != nil {
		wc.protocolError(err)
		return
	}

	if isEphemeral || hasName {
		isBatch := isBoolQuerySet(query, "batch")
		filter := core.NewSubscriptionFilterFromQuery(query)
		err := wc.handleStart(&core.WSStart{
			AutoAck:   &isAutoack,
			Ephemeral: isEphemeral,
			Namespace: namespace,
			Name:      query.Get("name"),
			Filter:    filter,
			Options: core.SubscriptionOptions{
				SubscriptionCoreOptions: core.SubscriptionCoreOptions{
					Batch:        &isBatch,
					BatchTimeout: wc.getBatchTimeout(query),
					ReadAhead:    wc.getReadAhead(query, isBatch),
				},
			},
		})
		if err != nil {
			wc.protocolError(err)
		}
	}
}

func (wc *websocketConnection) sendLoop() {
	l := log.L(wc.ctx)
	defer close(wc.senderDone)
	defer wc.close()
	for {
		select {
		case msg := <-wc.sendMessages:
			l.Tracef("Sending: %+v", msg)
			writer, err := wc.wsConn.NextWriter(websocket.TextMessage)
			if err == nil {
				err = json.NewEncoder(writer).Encode(msg)
				_ = writer.Close()
			}
			if err != nil {
				l.Errorf("Write failed on socket: %s", err)
				return
			}
		case <-wc.receiverDone:
			l.Debugf("Sender closing - receiver completed")
			return
		case <-wc.ctx.Done():
			l.Debugf("Sender closing - context cancelled")
			return
		}
	}
}

func (wc *websocketConnection) receiveLoop() {
	l := log.L(wc.ctx)
	defer close(wc.receiverDone)
	for {
		var msgData []byte
		var msgHeader core.WSActionBase
		_, reader, err := wc.wsConn.NextReader()
		if err == nil {
			msgData, err = io.ReadAll(reader)
			if err == nil {
				err = json.Unmarshal(msgData, &msgHeader)
				if err != nil {
					// We can notify the client on this one, before we bail
					wc.protocolError(i18n.WrapError(wc.ctx, err, coremsgs.MsgWSClientSentInvalidData))
				}
			}
		}
		if err != nil {
			l.Errorf("Read failed: %s", err)
			return
		}
		l.Tracef("Received: %s", string(msgData))
		switch msgHeader.Type {
		case core.WSClientActionStart:
			var msg core.WSStart
			err = json.Unmarshal(msgData, &msg)
			if err == nil {
				msg.Namespace, err = wc.assertNamespace(msg.Namespace)
				if err == nil {
					err = wc.authorizeMessage(msg.Namespace)
				}
				if err == nil {
					err = wc.handleStart(&msg)
				}
			}
		case core.WSClientActionAck:
			var msg core.WSAck
			err = json.Unmarshal(msgData, &msg)
			if err == nil {
				// acks are not authenticated because they will only be accepted for
				// messages that were sent by FireFly on this connection, which would
				// have previously checked authorization in the start message
				err = wc.handleAck(&msg)
			}
		default:
			err = i18n.NewError(wc.ctx, coremsgs.MsgWSClientUnknownAction, msgHeader.Type)
		}
		if err != nil {
			wc.protocolError(i18n.WrapError(wc.ctx, err, coremsgs.MsgWSClientSentInvalidData))
			l.Errorf("Invalid request sent on socket: %s", err)
			return
		}
	}
}

func (wc *websocketConnection) dispatch(event *core.EventDelivery) error {
	inflight := &core.EventDeliveryResponse{
		ID:           event.ID,
		Subscription: event.Subscription,
	}

	var autoAck bool
	wc.mux.Lock()
	autoAck = wc.autoAck
	if !autoAck {
		wc.inflight = append(wc.inflight, inflight)
	}
	wc.mux.Unlock()

	err := wc.send(event)
	if err != nil {
		return err
	}

	if autoAck {
		wc.ws.ack(wc.connID, inflight)
	}

	return nil
}

func (wc *websocketConnection) dispatchBatch(sub *core.Subscription, events []*core.CombinedEventDataDelivery) error {
	inflightBatch := &core.WSEventBatch{
		Type:   core.WSEventBatchType,
		ID:     fftypes.NewUUID(),
		Events: make([]*core.EventDelivery, len(events)),
	}
	if sub != nil {
		inflightBatch.Subscription = sub.SubscriptionRef
	}
	for i, e := range events {
		// For ephemeral there's no sub, so we pick up from first event
		if inflightBatch.Subscription.Namespace == "" {
			inflightBatch.Subscription = e.Event.Subscription
		}
		inflightBatch.Events[i] = e.Event
	}

	var autoAck bool
	wc.mux.Lock()
	autoAck = wc.autoAck
	if !autoAck {
		wc.inflightBatches = append(wc.inflightBatches, inflightBatch)
	}
	wc.mux.Unlock()

	err := wc.send(inflightBatch)
	if err != nil {
		return err
	}

	if autoAck {
		wc.ackBatch(inflightBatch)
	}

	return nil
}

func (wc *websocketConnection) ackBatch(batch *core.WSEventBatch) {
	for _, e := range batch.Events {
		// We individually drive an ack back on each event, but do so in one pass
		// (this matches the webhook implementation of batching).
		wc.ws.ack(wc.connID, &core.EventDeliveryResponse{
			ID:           e.ID,
			Subscription: batch.Subscription,
		})
	}
}

func (wc *websocketConnection) protocolError(err error) {
	log.L(wc.ctx).Errorf("Sending protocol error to client: %s", err)
	sendErr := wc.send(&core.WSError{
		Type:  core.WSProtocolErrorEventType,
		Error: err.Error(),
	})
	if sendErr != nil {
		log.L(wc.ctx).Errorf("Failed to send protocol error: %s", sendErr)
	}
}

func (wc *websocketConnection) send(msg interface{}) error {
	if wc.closed {
		return i18n.NewError(wc.ctx, coremsgs.MsgWSClosed)
	}
	select {
	case wc.sendMessages <- msg:
		return nil
	case <-wc.ctx.Done():
		return i18n.NewError(wc.ctx, i18n.MsgWSClosing)
	}
}

func (wc *websocketConnection) restartForNamespace(ns string, startTime time.Time) {
	wc.mux.Lock()
	toStart := []*core.WSStart{}
	for _, s := range wc.started {
		if s.Namespace == ns && s.startTime.Time().Before(startTime) {
			log.L(wc.ctx).Infof("Restarting subscription '%s:%s' (ephemeral=%t)", s.Namespace, s.Name, s.Ephemeral)
			toStart = append(toStart, &s.WSStart)
			s.startTime = fftypes.Now()
		}
	}
	wc.mux.Unlock()
	for _, s := range toStart {
		if err := wc.ws.start(wc, s); err != nil {
			log.L(wc.ctx).Errorf("Failed restart subscription '%s:%s' (closing): %s", s.Namespace, s.Name, err)
			wc.close()
		}
	}
}

func (wc *websocketConnection) handleStart(start *core.WSStart) (err error) {
	// this will very likely already be checked before we get here but
	// it doesn't do any harm to do a final assertion just in case it hasn't been done yet

	start.Namespace, err = wc.assertNamespace(start.Namespace)
	if err != nil {
		return err
	}

	wc.mux.Lock()
	if start.AutoAck != nil {
		if *start.AutoAck != wc.autoAck && len(wc.started) > 0 {
			wc.mux.Unlock()
			return i18n.NewError(wc.ctx, coremsgs.MsgWSAutoAckChanged)
		}
		wc.autoAck = *start.AutoAck
	}
	wc.started = append(wc.started, &websocketStartedSub{
		startTime: fftypes.Now(),
		WSStart:   *start,
	})
	wc.mux.Unlock()
	return wc.ws.start(wc, start)
}

func (wc *websocketConnection) durableSubMatcher(sr core.SubscriptionRef) bool {
	wc.mux.Lock()
	defer wc.mux.Unlock()
	for _, startedSub := range wc.started {
		if !startedSub.Ephemeral && startedSub.Namespace == sr.Namespace && startedSub.Name == sr.Name {
			return true
		}
	}
	return false
}

func (wc *websocketConnection) handleBatchAck(ack *core.WSAck) (handled bool) {
	wc.mux.Lock()
	defer wc.mux.Unlock()
	var newInflightBatches []*core.WSEventBatch
	for _, batch := range wc.inflightBatches {
		if batch.ID.Equals(ack.ID) { // nil safe check
			wc.ackBatch(batch)
			handled = true
		} else {
			newInflightBatches = append(newInflightBatches, batch)
		}
	}
	wc.inflightBatches = newInflightBatches
	return handled
}

func (wc *websocketConnection) checkAck(ack *core.WSAck) (*core.EventDeliveryResponse, error) {
	l := log.L(wc.ctx)
	var inflight *core.EventDeliveryResponse
	wc.mux.Lock()
	defer wc.mux.Unlock()

	if wc.autoAck {
		return nil, i18n.NewError(wc.ctx, coremsgs.MsgWSAutoAckEnabled)
	}

	if ack.ID != nil {
		newInflight := make([]*core.EventDeliveryResponse, 0, len(wc.inflight))
		for _, candidate := range wc.inflight {
			var match bool
			if *candidate.ID == *ack.ID {
				if ack.Subscription != nil {
					// A subscription has been explicitly specified, so it must match
					if (ack.Subscription.ID != nil && *ack.Subscription.ID == *candidate.Subscription.ID) ||
						(ack.Subscription.Name == candidate.Subscription.Name && ack.Subscription.Namespace == candidate.Subscription.Namespace) {
						match = true
					}
				} else {
					// If there's more than one started subscription, that's a problem
					if len(wc.started) != 1 {
						l.Errorf("No subscription specified on ack, and there is not exactly one started subscription")
						return nil, i18n.NewError(wc.ctx, coremsgs.MsgWSMsgSubNotMatched)
					}
					match = true
				}
			}
			// Remove from the inflight list
			if match {
				inflight = candidate
			} else {
				newInflight = append(newInflight, candidate)
			}
		}
		wc.inflight = newInflight
	} else {
		// Just ack the front of the queue
		if len(wc.inflight) == 0 {
			l.Errorf("Ack received, but no messages in flight")
		} else {
			inflight = wc.inflight[0]
			wc.inflight = wc.inflight[1:]
		}
	}
	if inflight == nil {
		return nil, i18n.NewError(wc.ctx, coremsgs.MsgWSMsgSubNotMatched)
	}
	return inflight, nil
}

func (wc *websocketConnection) handleAck(ack *core.WSAck) error {
	if handled := wc.handleBatchAck(ack); handled {
		return nil
	}

	// Perform a locked set of check
	inflight, err := wc.checkAck(ack)
	if err != nil {
		return err
	}

	// Deliver the ack to the core, now we're unlocked
	wc.ws.ack(wc.connID, inflight)
	return nil
}

func (wc *websocketConnection) close() {
	var didClosed bool
	wc.mux.Lock()
	if !wc.closed {
		didClosed = true
		wc.closed = true
		if wc.wsConn != nil {
			_ = wc.wsConn.Close()
		}
		wc.cancelCtx()
	}
	wc.mux.Unlock()
	// Drop lock before callback
	if didClosed {
		wc.ws.connClosed(wc.connID)
	}
}

func (wc *websocketConnection) waitClose() {
	<-wc.senderDone
	<-wc.receiverDone
}

func (wc *websocketConnection) authorizeMessage(ns string) error {
	wc.mux.Lock()
	defer wc.mux.Unlock()
	authReq := &fftypes.AuthReq{
		Namespace: ns,
		Header:    wc.header,
	}
	if wc.auth != nil {
		if err := wc.auth.Authorize(wc.ctx, authReq); err != nil {
			return err
		}
	}
	return nil
}
