// Copyright © 2022 Kaleido, Inc.
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

package spievents

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

type Manager interface {
	Dispatch(changeEvent *core.ChangeEvent)
	ServeHTTPWebSocketListener(res http.ResponseWriter, req *http.Request)
	WaitStop()
}

type adminEventManager struct {
	ctx              context.Context
	cancelCtx        func()
	activeWebsockets map[string]*webSocket
	dirtyReadList    []*webSocket
	mux              sync.Mutex
	upgrader         websocket.Upgrader

	queueLength         int
	blockedWarnInterval time.Duration
}

func NewAdminEventManager(ctx context.Context) Manager {
	ae := &adminEventManager{
		upgrader: websocket.Upgrader{
			ReadBufferSize:  int(config.GetByteSize(coreconfig.SPIWebSocketReadBufferSize)),
			WriteBufferSize: int(config.GetByteSize(coreconfig.SPIWebSocketWriteBufferSize)),
			CheckOrigin: func(r *http.Request) bool {
				// Cors is handled by the API server that wraps this handler
				return true
			},
		},
		activeWebsockets:    make(map[string]*webSocket),
		queueLength:         config.GetInt(coreconfig.SPIWebSocketEventQueueLength),
		blockedWarnInterval: config.GetDuration(coreconfig.SPIWebSocketBlockedWarnInterval),
	}
	ae.ctx, ae.cancelCtx = context.WithCancel(
		log.WithLogField(ctx, "role", "change-event-manager"),
	)
	return ae
}

func (ae *adminEventManager) ServeHTTPWebSocketListener(res http.ResponseWriter, req *http.Request) {
	wsConn, err := ae.upgrader.Upgrade(res, req, nil)
	if err != nil {
		log.L(ae.ctx).Errorf("WebSocket upgrade failed: %s", err)
		return
	}

	ae.mux.Lock()
	wc := newWebSocket(ae, wsConn)
	ae.activeWebsockets[wc.connID] = wc
	ae.makeDirtyReadList()
	ae.mux.Unlock()
}

func (ae *adminEventManager) wsClosed(connID string) {
	ae.mux.Lock()
	delete(ae.activeWebsockets, connID)
	ae.makeDirtyReadList()
	ae.mux.Unlock()
}

func (ae *adminEventManager) WaitStop() {
	ae.cancelCtx()

	ae.mux.Lock()
	activeWebsockets := make([]*webSocket, 0, len(ae.activeWebsockets))
	for _, ws := range ae.activeWebsockets {
		activeWebsockets = append(activeWebsockets, ws)
	}
	ae.mux.Unlock()

	for _, ws := range activeWebsockets {
		ws.waitClose()
	}
}

func (ae *adminEventManager) makeDirtyReadList() {
	ae.dirtyReadList = make([]*webSocket, 0, len(ae.activeWebsockets))
	for _, ws := range ae.activeWebsockets {
		ae.dirtyReadList = append(ae.dirtyReadList, ws)
	}
}

func (ae *adminEventManager) Dispatch(changeEvent *core.ChangeEvent) {
	// We don't lock here, as this is a critical path function.
	// We use a dirty copy of the connection list, updated on add/remove
	for _, ws := range ae.dirtyReadList {
		ws.dispatch(changeEvent)
	}
}
