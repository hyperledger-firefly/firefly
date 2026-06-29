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
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/events"
)

type WebSocketsNamespaced interface {
	ServeHTTPNamespaced(namespace string, res http.ResponseWriter, req *http.Request)
}

type WebSockets struct {
	ctx          context.Context
	capabilities *events.Capabilities
	callbacks    callbacks
	connections  map[string]*websocketConnection
	connMux      sync.Mutex
	upgrader     websocket.Upgrader
	auth         core.Authorizer
}

type callbacks struct {
	writeLock sync.Mutex
	handlers  map[string]events.Callbacks
}

func (ws *WebSockets) Name() string { return "websockets" }

func (ws *WebSockets) Init(ctx context.Context, config config.Section) error {
	*ws = WebSockets{
		ctx:         ctx,
		connections: make(map[string]*websocketConnection),
		capabilities: &events.Capabilities{
			BatchDelivery: true,
		},
		callbacks: callbacks{
			handlers: make(map[string]events.Callbacks),
		},
		upgrader: websocket.Upgrader{
			ReadBufferSize:  int(config.GetByteSize(ReadBufferSize)),
			WriteBufferSize: int(config.GetByteSize(WriteBufferSize)),
			CheckOrigin: func(r *http.Request) bool {
				// Cors is handled by the API server that wraps this handler
				return true
			},
		},
	}
	return nil
}

func (ws *WebSockets) SetAuthorizer(auth core.Authorizer) {
	ws.auth = auth
}

func (ws *WebSockets) SetHandler(namespace string, handler events.Callbacks) error {
	ws.callbacks.writeLock.Lock()
	defer ws.callbacks.writeLock.Unlock()
	if handler == nil {
		delete(ws.callbacks.handlers, namespace)
		return nil
	}
	ws.callbacks.handlers[namespace] = handler
	return nil
}

func (ws *WebSockets) Capabilities() *events.Capabilities {
	return ws.capabilities
}

func (ws *WebSockets) ValidateOptions(ctx context.Context, options *core.SubscriptionOptions) error {
	// We don't support streaming the full data over websockets
	if options.WithData != nil && *options.WithData {
		return i18n.NewError(ctx, coremsgs.MsgWebsocketsNoData)
	}
	forceFalse := false
	options.WithData = &forceFalse
	return nil
}

func (ws *WebSockets) DeliveryRequest(ctx context.Context, connID string, sub *core.Subscription, event *core.EventDelivery, data core.DataArray) error {
	ws.connMux.Lock()
	conn, ok := ws.connections[connID]
	ws.connMux.Unlock()
	if !ok {
		return i18n.NewError(ctx, coremsgs.MsgWSConnectionNotActive, connID)
	}
	return conn.dispatch(event)
}

func (ws *WebSockets) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	wsConn, err := ws.upgrader.Upgrade(res, req, nil)
	if err != nil {
		log.L(ws.ctx).Errorf("WebSocket upgrade failed: %s", err)
		return
	}

	ws.connMux.Lock()
	wc := newConnection(ws.ctx, ws, wsConn, req, ws.auth)
	ws.connections[wc.connID] = wc
	ws.connMux.Unlock()

	wc.processAutoStart(req)
}

func (ws *WebSockets) ServeHTTPNamespaced(namespace string, res http.ResponseWriter, req *http.Request) {

	wsConn, err := ws.upgrader.Upgrade(res, req, nil)
	if err != nil {
		log.L(ws.ctx).Errorf("WebSocket upgrade failed: %s", err)
		return
	}

	ws.connMux.Lock()
	wc := newConnection(ws.ctx, ws, wsConn, req, ws.auth)
	wc.namespaceScoped = true
	wc.namespace = namespace
	ws.connections[wc.connID] = wc
	ws.connMux.Unlock()

	wc.processAutoStart(req)

}

func (ws *WebSockets) ack(connID string, inflight *core.EventDeliveryResponse) {
	if cb, ok := ws.callbacks.handlers[inflight.Subscription.Namespace]; ok {
		cb.DeliveryResponse(connID, inflight)
	}
}

func (ws *WebSockets) start(wc *websocketConnection, start *core.WSStart) error {
	if start.Namespace == "" || (!start.Ephemeral && start.Name == "") {
		return i18n.NewError(ws.ctx, coremsgs.MsgWSInvalidStartAction)
	}
	if cb, ok := ws.callbacks.handlers[start.Namespace]; ok {
		if start.Ephemeral {
			return cb.EphemeralSubscription(wc.connID, start.Namespace, &start.Filter, &start.Options)
		}
		// We can have multiple subscriptions on a single connection
		return cb.RegisterConnection(wc.connID, func(sr core.SubscriptionRef) bool {
			return wc.durableSubMatcher(sr)
		})
	}
	return i18n.NewError(ws.ctx, coremsgs.MsgNamespaceDoesNotExist)
}

func (ws *WebSockets) connClosed(connID string) {
	ws.connMux.Lock()
	delete(ws.connections, connID)
	ws.connMux.Unlock()
	// Drop lock before calling back
	for _, cb := range ws.callbacks.handlers {
		cb.ConnectionClosed(connID)
	}
}

func (ws *WebSockets) WaitClosed() {
	closedConnections := []*websocketConnection{}
	ws.connMux.Lock()
	for _, ws := range ws.connections {
		closedConnections = append(closedConnections, ws)
	}
	ws.connMux.Unlock()
	for _, ws := range closedConnections {
		ws.waitClose()
	}
}

func (ws *WebSockets) NamespaceRestarted(ns string, startTime time.Time) {

	ws.connMux.Lock()
	connections := make([]*websocketConnection, 0, len(ws.connections))
	for _, c := range ws.connections {
		connections = append(connections, c)
	}
	ws.connMux.Unlock()

	for _, wc := range connections {
		wc.restartForNamespace(ns, startTime)
	}

}

func (ws *WebSockets) GetStatus() *core.WebSocketStatus {
	status := &core.WebSocketStatus{
		Enabled:     true,
		Connections: make([]*core.WSConnectionStatus, 0),
	}

	ws.connMux.Lock()
	connections := make([]*websocketConnection, 0, len(ws.connections))
	for _, c := range ws.connections {
		connections = append(connections, c)
	}
	ws.connMux.Unlock()

	for _, wc := range connections {
		wc.mux.Lock()
		conn := &core.WSConnectionStatus{
			ID:            wc.connID,
			RemoteAddress: wc.remoteAddr,
			UserAgent:     wc.userAgent,
			Subscriptions: make([]*core.WSSubscriptionStatus, 0),
		}
		status.Connections = append(status.Connections, conn)
		for _, s := range wc.started {
			sub := &core.WSSubscriptionStatus{
				StartTime: s.startTime,
				Name:      s.Name,
				Namespace: s.Namespace,
				Ephemeral: s.Ephemeral,
				Filter:    s.Filter,
			}
			conn.Subscriptions = append(conn.Subscriptions, sub)
		}
		wc.mux.Unlock()
	}
	return status
}

func (ws *WebSockets) BatchDeliveryRequest(ctx context.Context, connID string, sub *core.Subscription, events []*core.CombinedEventDataDelivery) error {
	ws.connMux.Lock()
	conn, ok := ws.connections[connID]
	ws.connMux.Unlock()
	if !ok {
		return i18n.NewError(ctx, coremsgs.MsgWSConnectionNotActive, connID)
	}
	return conn.dispatchBatch(sub, events)
}
