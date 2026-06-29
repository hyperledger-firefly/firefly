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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/ffresty"
	"github.com/hyperledger-firefly/common/pkg/wsclient"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
)

func newTestSPIEventsManager(t *testing.T) (ae *adminEventManager, ws *webSocket, wsc wsclient.WSClient, cancel func()) {
	coreconfig.Reset()

	ae = NewAdminEventManager(context.Background()).(*adminEventManager)
	svr := httptest.NewServer(http.HandlerFunc(ae.ServeHTTPWebSocketListener))

	clientConfig := config.RootSection("ut.wsclient")
	wsclient.InitConfig(clientConfig)
	clientConfig.Set(ffresty.HTTPConfigURL, fmt.Sprintf("http://%s", svr.Listener.Addr()))
	wsConfig, err := wsclient.GenerateConfig(context.Background(), clientConfig)
	assert.NoError(t, err)

	wsc, err = wsclient.New(ae.ctx, wsConfig, nil, nil)
	assert.NoError(t, err)
	err = wsc.Connect()
	assert.NoError(t, err)

	for ws == nil {
		time.Sleep(1 * time.Microsecond)
		if len(ae.dirtyReadList) > 0 {
			ws = ae.dirtyReadList[0]
		}
	}

	return ae, ws, wsc, func() {
		ae.cancelCtx()
		wsc.Close()
		ae.WaitStop()
		svr.Close()
	}
}

func toJSON(t *testing.T, obj interface{}) []byte {
	b, err := json.Marshal(obj)
	assert.NoError(t, err)
	return b
}

func unmarshalChangeEvent(t *testing.T, msgBytes []byte) *core.ChangeEvent {
	var event core.ChangeEvent
	err := json.Unmarshal(msgBytes, &event)
	assert.NoError(t, err)
	return &event
}

func TestSPIEventsE2E(t *testing.T) {
	ae, _, wsc, cancel := newTestSPIEventsManager(t)

	events := make(chan *core.ChangeEvent)
	go func() {
		for msgBytes := range wsc.Receive() {
			events <- unmarshalChangeEvent(t, msgBytes)
		}
	}()

	// Send some garbage first, to be discarded
	wsc.Send(ae.ctx, toJSON(t, map[string]string{"wrong": "data"}))
	// Then send the actual command to start with a filter
	wsc.Send(ae.ctx, toJSON(t, &core.WSChangeEventCommand{
		Type:        core.WSChangeEventCommandTypeStart,
		Collections: []string{"collection1"},
		Filter: core.ChangeEventFilter{
			Types:      []core.ChangeEventType{core.ChangeEventTypeCreated},
			Namespaces: []string{"ns1"},
		},
	}))
	for len(ae.dirtyReadList) == 0 || ae.dirtyReadList[0].collections == nil {
		time.Sleep(1 * time.Microsecond)
	}

	ignoreDueToWrongCollection := &core.ChangeEvent{
		Collection: "collection2",
		Type:       core.ChangeEventTypeCreated,
		Namespace:  "ns1",
	}
	ae.Dispatch(ignoreDueToWrongCollection)
	ignoreDueToWrongType := &core.ChangeEvent{
		Collection: "collection1",
		Type:       core.ChangeEventTypeDeleted,
		Namespace:  "ns1",
	}
	ae.Dispatch(ignoreDueToWrongType)
	ignoreDueToWrongNamespace := &core.ChangeEvent{
		Collection: "collection1",
		Type:       core.ChangeEventTypeCreated,
		Namespace:  "ns2",
	}
	ae.Dispatch(ignoreDueToWrongNamespace)
	match := &core.ChangeEvent{
		Collection: "collection1",
		Type:       core.ChangeEventTypeCreated,
		Namespace:  "ns1",
	}
	ae.Dispatch(match)

	assert.Equal(t, match, <-events)

	defer cancel()

}

func TestBadUpgrade(t *testing.T) {
	coreconfig.Reset()

	ae := NewAdminEventManager(context.Background()).(*adminEventManager)
	svr := httptest.NewServer(http.HandlerFunc(ae.ServeHTTPWebSocketListener))
	defer svr.Close()

	res, err := http.Post(fmt.Sprintf("http://%s", svr.Listener.Addr()), "application/json", bytes.NewReader([]byte("{}")))
	assert.NoError(t, err)
	assert.True(t, res.StatusCode >= 300)

}
