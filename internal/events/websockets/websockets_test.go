// Copyright © 2021 Kaleido, Inc.
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
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-common/pkg/ffresty"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly-common/pkg/wsclient"
	"github.com/hyperledger/firefly/internal/coreconfig"
	"github.com/hyperledger/firefly/mocks/eventsmocks"
	"github.com/hyperledger/firefly/pkg/core"
	"github.com/hyperledger/firefly/pkg/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type testAuthorizer struct{}

func (t *testAuthorizer) Authorize(ctx context.Context, authReq *fftypes.AuthReq) error {
	if authReq.Namespace == "ns1" {
		return nil
	}
	return i18n.NewError(ctx, i18n.MsgUnauthorized)
}

func newTestWebsockets(t *testing.T, cbs *eventsmocks.Callbacks, authorizer core.Authorizer, queryParams ...string) (ws *WebSockets, wsc wsclient.WSClient, cancel func()) {
	return newTestWebsocketsCommon(t, cbs, authorizer, "", queryParams...)
}

type testNamespacedHandler struct {
	ws        *WebSockets
	namespace string
}

func (h *testNamespacedHandler) ServeHTTP(res http.ResponseWriter, req *http.Request) {
	h.ws.ServeHTTPNamespaced(h.namespace, res, req)
}
func newTestWebsocketsCommon(t *testing.T, cbs *eventsmocks.Callbacks, authorizer core.Authorizer, namespace string, queryParams ...string) (ws *WebSockets, wsc wsclient.WSClient, cancel func()) {
	coreconfig.Reset()

	ws = &WebSockets{}
	ctx, cancelCtx := context.WithCancel(context.Background())
	svrConfig := config.RootSection("ut.websockets")
	ws.InitConfig(svrConfig)
	ws.Init(ctx, svrConfig)
	ws.SetHandler("ns1", cbs)
	ws.SetAuthorizer(authorizer)
	assert.Equal(t, "websockets", ws.Name())
	assert.NotNil(t, ws.Capabilities())
	cbs.On("ConnectionClosed", mock.Anything).Return(nil).Maybe()
	var svr *httptest.Server
	if namespace == "" {
		svr = httptest.NewServer(ws)
	} else {
		namespacedHandler := &testNamespacedHandler{
			ws:        ws,
			namespace: namespace,
		}
		svr = httptest.NewServer(namespacedHandler)
	}

	clientConfig := config.RootSection("ut.wsclient")
	wsclient.InitConfig(clientConfig)
	qs := ""
	if len(queryParams) > 0 {
		qs = fmt.Sprintf("?%s", strings.Join(queryParams, "&"))
	}
	clientConfig.Set(ffresty.HTTPConfigURL, fmt.Sprintf("http://%s%s", svr.Listener.Addr(), qs))
	wsConfig, err := wsclient.GenerateConfig(ctx, clientConfig)
	assert.NoError(t, err)

	wsc, err = wsclient.New(ctx, wsConfig, nil, nil)
	assert.NoError(t, err)
	err = wsc.Connect()
	assert.NoError(t, err)

	return ws, wsc, func() {
		// Do not call wsc.Close() after cancel: wsclient.New already closes the client
		// from ctx.Done(), and a second Close() races and can panic (close of closed channel).
		cancelCtx()
		ws.WaitClosed()
		svr.Close()
	}
}
func TestValidateOptionsFail(t *testing.T) {
	cbs := &eventsmocks.Callbacks{}
	ws, _, cancel := newTestWebsockets(t, cbs, nil)
	defer cancel()

	yes := true
	err := ws.ValidateOptions(ws.ctx, &core.SubscriptionOptions{
		SubscriptionCoreOptions: core.SubscriptionCoreOptions{
			WithData: &yes,
		},
	})
	assert.Regexp(t, "FF10244", err)
}

func TestValidateOptionsOk(t *testing.T) {
	cbs := &eventsmocks.Callbacks{}
	ws, _, cancel := newTestWebsockets(t, cbs, nil)
	defer cancel()

	opts := &core.SubscriptionOptions{}
	err := ws.ValidateOptions(ws.ctx, opts)
	assert.NoError(t, err)
	assert.False(t, *opts.WithData)

	ws.SetHandler("ns1", nil)
	assert.Empty(t, ws.callbacks.handlers)

}

func TestSendBadData(t *testing.T) {
	cbs := &eventsmocks.Callbacks{}
	_, wsc, cancel := newTestWebsockets(t, cbs, nil)
	defer cancel()

	cbs.On("ConnectionClosed", mock.Anything).Return(nil)

	err := wsc.Send(context.Background(), []byte(`!json`))
	assert.NoError(t, err)
	b := <-wsc.Receive()
	var res core.WSError
	err = json.Unmarshal(b, &res)
	assert.NoError(t, err)
	assert.Equal(t, core.WSProtocolErrorEventType, res.Type)
	assert.Regexp(t, "FF10176", res.Error)
}

func TestSendBadAction(t *testing.T) {
	cbs := &eventsmocks.Callbacks{}
	_, wsc, cancel := newTestWebsockets(t, cbs, nil)
	defer cancel()
	cbs.On("ConnectionClosed", mock.Anything).Return(nil)

	err := wsc.Send(context.Background(), []byte(`{"type":"lobster"}`))
	assert.NoError(t, err)
	b := <-wsc.Receive()
	var res core.WSError
	err = json.Unmarshal(b, &res)
	assert.NoError(t, err)
	assert.Equal(t, core.WSProtocolErrorEventType, res.Type)
	assert.Regexp(t, "FF10176", res.Error)
}

func TestSendEmptyStartAction(t *testing.T) {
	cbs := &eventsmocks.Callbacks{}
	_, wsc, cancel := newTestWebsockets(t, cbs, nil)
	defer cancel()
	cbs.On("ConnectionClosed", mock.Anything).Return(nil)

	err := wsc.Send(context.Background(), []byte(`{"type":"start"}`))
	assert.NoError(t, err)
	b := <-wsc.Receive()
	var res core.WSError
	err = json.Unmarshal(b, &res)
	assert.NoError(t, err)
	assert.Equal(t, core.WSProtocolErrorEventType, res.Type)
	assert.Regexp(t, "FF10176", res.Error)
}

func TestStartReceiveAckEphemeral(t *testing.T) {
	log.SetLevel("trace")

	cbs := &eventsmocks.Callbacks{}
	ws, wsc, cancel := newTestWebsockets(t, cbs, nil)
	defer cancel()
	var connID string
	sub := cbs.On("EphemeralSubscription",
		mock.MatchedBy(func(s string) bool { connID = s; return true }),
		"ns1", mock.Anything, mock.Anything).Return(nil)
	ack := cbs.On("DeliveryResponse",
		mock.MatchedBy(func(s string) bool { return s == connID }),
		mock.Anything).Return(nil)

	waitSubscribed := make(chan struct{})
	sub.RunFn = func(a mock.Arguments) {
		close(waitSubscribed)
	}

	waitAcked := make(chan struct{})
	ack.RunFn = func(a mock.Arguments) {
		close(waitAcked)
	}

	err := wsc.Send(context.Background(), []byte(`{"type":"start","namespace":"ns1","ephemeral":true}`))
	assert.NoError(t, err)

	<-waitSubscribed
	ws.DeliveryRequest(ws.ctx, connID, nil, &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{ID: fftypes.NewUUID()},
		},
		Subscription: core.SubscriptionRef{
			ID:        fftypes.NewUUID(),
			Namespace: "ns1",
		},
	}, nil)

	b := <-wsc.Receive()
	var res core.EventDelivery
	err = json.Unmarshal(b, &res)
	assert.NoError(t, err)

	err = wsc.Send(context.Background(), []byte(`{"type":"ack"}`))
	assert.NoError(t, err)

	<-waitAcked
	cbs.AssertExpectations(t)
}

func TestAutoAckBatch(t *testing.T) {
	log.SetLevel("trace")

	cbs := &eventsmocks.Callbacks{}
	ws, wsc, cancel := newTestWebsockets(t, cbs, nil, "autoack=true")
	defer cancel()
	var connID string
	mes := cbs.On("EphemeralSubscription",
		mock.MatchedBy(func(s string) bool { connID = s; return true }),
		"ns1", mock.Anything, mock.MatchedBy(func(o *core.SubscriptionOptions) bool {
			return *o.Batch
		})).Return(nil)
	ack := cbs.On("DeliveryResponse",
		mock.MatchedBy(func(s string) bool { return s == connID }),
		mock.Anything).Return(nil)

	waitSubscribed := make(chan struct{})
	mes.RunFn = func(a mock.Arguments) {
		close(waitSubscribed)
	}

	waitAcked := make(chan struct{})
	ack.RunFn = func(a mock.Arguments) {
		close(waitAcked)
	}

	err := wsc.Send(context.Background(), []byte(`{
		"type":"start",
		"namespace":"ns1",
		"ephemeral":true,
		"autoack": true,
		"options": {
			"batch": true
		}
	}`))
	assert.NoError(t, err)

	<-waitSubscribed
	sub := &core.Subscription{
		SubscriptionRef: core.SubscriptionRef{ID: fftypes.NewUUID(), Namespace: "ns1", Name: "sub1"},
	}
	ws.BatchDeliveryRequest(ws.ctx, connID, sub, []*core.CombinedEventDataDelivery{
		{Event: &core.EventDelivery{
			EnrichedEvent: core.EnrichedEvent{
				Event: core.Event{ID: fftypes.NewUUID()},
			},
			Subscription: core.SubscriptionRef{
				ID:        fftypes.NewUUID(),
				Namespace: "ns1",
			},
		}},
	})

	b := <-wsc.Receive()
	var res core.EventDelivery
	err = json.Unmarshal(b, &res)
	assert.NoError(t, err)

	<-waitAcked
	cbs.AssertExpectations(t)
}

func TestStartReceiveDurable(t *testing.T) {
	cbs := &eventsmocks.Callbacks{}
	ws, wsc, cancel := newTestWebsockets(t, cbs, nil)
	defer cancel()
	var connID string
	sub := cbs.On("RegisterConnection",
		mock.MatchedBy(func(s string) bool { connID = s; return true }),
		mock.Anything,
	).Return(nil).Run(func(args mock.Arguments) {
		subMatch := args[1].(events.SubscriptionMatcher)
		assert.True(t, subMatch(core.SubscriptionRef{Namespace: "ns1", Name: "sub1"}))
		assert.False(t, subMatch(core.SubscriptionRef{Namespace: "ns2", Name: "sub1"}))
		assert.False(t, subMatch(core.SubscriptionRef{Namespace: "ns1", Name: "sub2"}))
	})
	ack := cbs.On("DeliveryResponse",
		mock.MatchedBy(func(s string) bool { return s == connID }),
		mock.Anything).Return(nil)

	waitSubscribed := make(chan struct{})
	sub.RunFn = func(a mock.Arguments) {
		close(waitSubscribed)
	}

	waitAcked := make(chan struct{})
	ack.RunFn = func(a mock.Arguments) {
		close(waitAcked)
	}

	err := wsc.Send(context.Background(), []byte(`{"type":"start","namespace":"ns1","name":"sub1"}`))
	assert.NoError(t, err)

	<-waitSubscribed
	ws.DeliveryRequest(ws.ctx, connID, nil, &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{ID: fftypes.NewUUID()},
		},
		Subscription: core.SubscriptionRef{
			ID:        fftypes.NewUUID(),
			Namespace: "ns1",
			Name:      "sub1",
		},
	}, nil)
	// Put a second in flight
	ws.DeliveryRequest(ws.ctx, connID, nil, &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{ID: fftypes.NewUUID()},
		},
		Subscription: core.SubscriptionRef{
			ID:        fftypes.NewUUID(),
			Namespace: "ns1",
			Name:      "sub2",
		},
	}, nil)

	b := <-wsc.Receive()
	var res core.EventDelivery
	err = json.Unmarshal(b, &res)
	assert.NoError(t, err)

	assert.Equal(t, "ns1", res.Subscription.Namespace)
	assert.Equal(t, "sub1", res.Subscription.Name)
	err = wsc.Send(context.Background(), []byte(fmt.Sprintf(`{
		"type":"ack",
		"id": "%s",
		"subscription": {
			"namespace": "ns1",
			"name": "sub1"
		}
	}`, res.ID)))
	assert.NoError(t, err)

	<-waitAcked

	// Check we left the right one behind
	conn := ws.connections[connID]
	assert.Equal(t, 1, len(conn.inflight))
	assert.Equal(t, "sub2", conn.inflight[0].Subscription.Name)

	cbs.AssertExpectations(t)
}

func TestStartReceiveDurableBatch(t *testing.T) {
	cbs := &eventsmocks.Callbacks{}
	ws, wsc, cancel := newTestWebsockets(t, cbs, nil)
	defer cancel()
	var connID string
	mrg := cbs.On("RegisterConnection",
		mock.MatchedBy(func(s string) bool { connID = s; return true }),
		mock.Anything,
	).Return(nil).Run(func(args mock.Arguments) {
		subMatch := args[1].(events.SubscriptionMatcher)
		assert.True(t, subMatch(core.SubscriptionRef{Namespace: "ns1", Name: "sub1"}))
		assert.False(t, subMatch(core.SubscriptionRef{Namespace: "ns2", Name: "sub1"}))
		assert.False(t, subMatch(core.SubscriptionRef{Namespace: "ns1", Name: "sub2"}))
	})
	ack := cbs.On("DeliveryResponse",
		mock.MatchedBy(func(s string) bool { return s == connID }),
		mock.Anything).Return(nil)

	waitSubscribed := make(chan struct{})
	mrg.RunFn = func(a mock.Arguments) {
		close(waitSubscribed)
	}

	acks := make(chan *core.EventDeliveryResponse)
	ack.RunFn = func(a mock.Arguments) {
		acks <- a[1].(*core.EventDeliveryResponse)
	}

	err := wsc.Send(context.Background(), []byte(`{"type":"start","namespace":"ns1","name":"sub1"}`))
	assert.NoError(t, err)

	<-waitSubscribed
	sub := &core.Subscription{
		SubscriptionRef: core.SubscriptionRef{ID: fftypes.NewUUID(), Namespace: "ns1", Name: "sub1"},
	}
	event1ID := fftypes.NewUUID()
	event2ID := fftypes.NewUUID()
	ws.BatchDeliveryRequest(ws.ctx, connID, sub, []*core.CombinedEventDataDelivery{
		{
			Event: &core.EventDelivery{
				EnrichedEvent: core.EnrichedEvent{
					Event: core.Event{ID: event1ID},
				},
				Subscription: sub.SubscriptionRef,
			},
		},
		{
			Event: &core.EventDelivery{
				EnrichedEvent: core.EnrichedEvent{
					Event: core.Event{ID: event2ID},
				},
				Subscription: sub.SubscriptionRef,
			},
		},
	})

	b := <-wsc.Receive()
	var deliveredBatch core.WSEventBatch
	err = json.Unmarshal(b, &deliveredBatch)
	assert.NoError(t, err)
	assert.Len(t, deliveredBatch.Events, 2)
	assert.Equal(t, "ns1", deliveredBatch.Subscription.Namespace)
	assert.Equal(t, "sub1", deliveredBatch.Subscription.Name)
	err = wsc.Send(context.Background(), []byte(fmt.Sprintf(`{
		"type":"ack",
		"id": "%s",
		"subscription": {
			"namespace": "ns1",
			"name": "sub1"
		}
	}`, deliveredBatch.ID)))
	assert.NoError(t, err)

	ack1 := <-acks
	assert.Equal(t, *event1ID, *ack1.ID)
	ack2 := <-acks
	assert.Equal(t, *event2ID, *ack2.ID)

	cbs.AssertExpectations(t)
}

func TestStartReceiveDurableWithAuth(t *testing.T) {
	cbs := &eventsmocks.Callbacks{}
	ws, wsc, cancel := newTestWebsockets(t, cbs, &testAuthorizer{})
	defer cancel()
	var connID string
	waitSubscribed := make(chan struct{})
	cbs.On("RegisterConnection",
		mock.MatchedBy(func(s string) bool { connID = s; return true }),
		mock.Anything,
	).Run(func(args mock.Arguments) {
		subMatch := args[1].(events.SubscriptionMatcher)
		assert.True(t, subMatch(core.SubscriptionRef{Namespace: "ns1", Name: "sub1"}))
		assert.False(t, subMatch(core.SubscriptionRef{Namespace: "ns2", Name: "sub1"}))
		assert.False(t, subMatch(core.SubscriptionRef{Namespace: "ns1", Name: "sub2"}))
		close(waitSubscribed)
	}).Return(nil)
	ack := cbs.On("DeliveryResponse",
		mock.MatchedBy(func(s string) bool { return s == connID }),
		mock.Anything).Return(nil)

	waitAcked := make(chan struct{})
	ack.RunFn = func(a mock.Arguments) {
		close(waitAcked)
	}

	err := wsc.Send(context.Background(), []byte(`{"type":"start","namespace":"ns1","name":"sub1"}`))
	assert.NoError(t, err)

	<-waitSubscribed
	ws.DeliveryRequest(ws.ctx, connID, nil, &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{ID: fftypes.NewUUID()},
		},
		Subscription: core.SubscriptionRef{
			ID:        fftypes.NewUUID(),
			Namespace: "ns1",
			Name:      "sub1",
		},
	}, nil)
	// Put a second in flight
	ws.DeliveryRequest(ws.ctx, connID, nil, &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{ID: fftypes.NewUUID()},
		},
		Subscription: core.SubscriptionRef{
			ID:        fftypes.NewUUID(),
			Namespace: "ns1",
			Name:      "sub2",
		},
	}, nil)

	b := <-wsc.Receive()
	var res core.EventDelivery
	err = json.Unmarshal(b, &res)
	assert.NoError(t, err)

	assert.Equal(t, "ns1", res.Subscription.Namespace)
	assert.Equal(t, "sub1", res.Subscription.Name)
	err = wsc.Send(context.Background(), []byte(fmt.Sprintf(`{
		"type":"ack",
		"id": "%s",
		"subscription": {
			"namespace": "ns1",
			"name": "sub1"
		}
	}`, res.ID)))
	assert.NoError(t, err)

	<-waitAcked

	// Check we left the right one behind
	conn := ws.connections[connID]
	assert.Equal(t, 1, len(conn.inflight))
	assert.Equal(t, "sub2", conn.inflight[0].Subscription.Name)

	cbs.AssertExpectations(t)
}

func TestStartReceiveDurableUnauthorized(t *testing.T) {
	cbs := &eventsmocks.Callbacks{}
	_, wsc, cancel := newTestWebsockets(t, cbs, &testAuthorizer{})
	defer cancel()
	var connID string
	waitSubscribed := make(chan struct{})
	cbs.On("RegisterConnection",
		mock.MatchedBy(func(s string) bool { connID = s; return true }),
		mock.Anything,
	).Return(nil).Run(func(args mock.Arguments) {
		subMatch := args[1].(events.SubscriptionMatcher)
		assert.True(t, subMatch(core.SubscriptionRef{Namespace: "ns2", Name: "sub1"}))
		assert.False(t, subMatch(core.SubscriptionRef{Namespace: "ns1", Name: "sub2"}))
		close(waitSubscribed)
	})
	ack := cbs.On("DeliveryResponse",
		mock.MatchedBy(func(s string) bool { return s == connID }),
		mock.Anything).Return(nil)

	waitAcked := make(chan struct{})
	ack.RunFn = func(a mock.Arguments) {
		close(waitAcked)
	}

	err := wsc.Send(context.Background(), []byte(`{"type":"start","namespace":"ns2","name":"sub1"}`))
	assert.NoError(t, err)

	b := <-wsc.Receive()
	var res fftypes.JSONObject
	err = json.Unmarshal(b, &res)
	assert.NoError(t, err)
	assert.Regexp(t, "FF00169", res.GetString("error"))
}

func TestAutoStartReceiveAckEphemeral(t *testing.T) {
	var connID string
	cbs := &eventsmocks.Callbacks{}
	waitSubscribed := make(chan struct{})
	cbs.On("EphemeralSubscription",
		mock.MatchedBy(func(s string) bool { connID = s; return true }),
		"ns1", mock.Anything, mock.Anything).
		Return(nil).
		Run(func(args mock.Arguments) {
			close(waitSubscribed)
		})
	ack := cbs.On("DeliveryResponse",
		mock.MatchedBy(func(s string) bool { return s == connID }),
		mock.Anything).Return(nil)

	waitAcked := make(chan struct{})
	ack.RunFn = func(a mock.Arguments) {
		close(waitAcked)
	}

	ws, wsc, cancel := newTestWebsockets(t, cbs, nil, "ephemeral", "namespace=ns1")
	defer cancel()

	<-waitSubscribed
	ws.DeliveryRequest(ws.ctx, connID, nil, &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{ID: fftypes.NewUUID()},
		},
		Subscription: core.SubscriptionRef{
			ID:        fftypes.NewUUID(),
			Namespace: "ns1",
		},
	}, nil)

	b := <-wsc.Receive()
	var res core.EventDelivery
	err := json.Unmarshal(b, &res)
	assert.NoError(t, err)

	err = wsc.Send(context.Background(), []byte(`{"type":"ack"}`))
	assert.NoError(t, err)

	<-waitAcked
	cbs.AssertExpectations(t)
}

func TestAutoStartReceiveAckBatchEphemeral(t *testing.T) {
	var connID string
	cbs := &eventsmocks.Callbacks{}
	waitSubscribed := make(chan struct{})
	cbs.On("EphemeralSubscription",
		mock.MatchedBy(func(s string) bool { connID = s; return true }),
		"ns1", mock.Anything, mock.Anything).
		Return(nil).
		Run(func(args mock.Arguments) {
			close(waitSubscribed)
		})
	ack := cbs.On("DeliveryResponse",
		mock.MatchedBy(func(s string) bool { return s == connID }),
		mock.Anything).Return(nil)

	waitAcked := make(chan struct{})
	ack.RunFn = func(a mock.Arguments) {
		close(waitAcked)
	}

	ws, wsc, cancel := newTestWebsockets(t, cbs, nil, "ephemeral", "namespace=ns1", "batch")
	defer cancel()

	<-waitSubscribed
	ws.BatchDeliveryRequest(ws.ctx, connID, nil, []*core.CombinedEventDataDelivery{
		{Event: &core.EventDelivery{
			EnrichedEvent: core.EnrichedEvent{
				Event: core.Event{ID: fftypes.NewUUID()},
			},
			Subscription: core.SubscriptionRef{
				ID:        fftypes.NewUUID(),
				Namespace: "ns1",
			},
		}},
	})

	b := <-wsc.Receive()
	var deliveredBatch core.WSEventBatch
	err := json.Unmarshal(b, &deliveredBatch)
	assert.NoError(t, err)
	assert.Len(t, deliveredBatch.Events, 1)

	err = wsc.Send(context.Background(), []byte(`{"type":"ack", "id": "`+deliveredBatch.ID.String()+`"}`))
	assert.NoError(t, err)

	<-waitAcked
	cbs.AssertExpectations(t)
}

func TestAutoStartBadOptions(t *testing.T) {
	cbs := &eventsmocks.Callbacks{}
	_, wsc, cancel := newTestWebsockets(t, cbs, nil, "name=missingnamespace")
	defer cancel()

	b := <-wsc.Receive()
	var res core.WSError
	err := json.Unmarshal(b, &res)
	assert.NoError(t, err)
	assert.Regexp(t, "FF10178", res.Error)
	cbs.AssertExpectations(t)
}

func TestAutoStartCustomReadAheadBatch(t *testing.T) {
	cbs := &eventsmocks.Callbacks{}

	subscribedConn := make(chan string, 1)
	cbs.On("EphemeralSubscription",
		mock.MatchedBy(func(s string) bool {
			subscribedConn <- s
			return true
		}),
		"ns1",
		mock.Anything,
		mock.MatchedBy(func(o *core.SubscriptionOptions) bool {
			return *o.ReadAhead == 42 && *o.BatchTimeout == "1s"
		}),
	).Return(nil)

	_, _, cancel := newTestWebsockets(t, cbs, nil, "namespace=ns1", "ephemeral", "batch", "batchtimeout=1s", "readahead=42")
	defer cancel()

	<-subscribedConn

}

func TestAutoStartBadNamespace(t *testing.T) {
	cbs := &eventsmocks.Callbacks{}
	_, wsc, cancel := newTestWebsockets(t, cbs, nil, "ephemeral", "namespace=ns2")
	defer cancel()

	b := <-wsc.Receive()
	var res core.WSError
	err := json.Unmarshal(b, &res)
	assert.NoError(t, err)
	assert.Regexp(t, "FF10187", res.Error)
	cbs.AssertExpectations(t)
}

func TestHandleAckWithAutoAck(t *testing.T) {
	eventUUID := fftypes.NewUUID()
	wsc := &websocketConnection{
		ctx: context.Background(),
		started: []*websocketStartedSub{{WSStart: core.WSStart{
			Ephemeral: false, Name: "name1", Namespace: "ns1",
		}}},
		sendMessages: make(chan interface{}, 1),
		inflight: []*core.EventDeliveryResponse{
			{ID: eventUUID},
		},
		autoAck: true,
	}
	err := wsc.handleAck(&core.WSAck{
		ID: eventUUID,
	})
	assert.Regexp(t, "FF10180", err)
}

func TestHandleBatchNotMatch(t *testing.T) {
	eventUUID := fftypes.NewUUID()
	wsc := &websocketConnection{
		ctx: context.Background(),
		started: []*websocketStartedSub{{WSStart: core.WSStart{
			Ephemeral: false, Name: "name1", Namespace: "ns1",
		}}},
		sendMessages: make(chan interface{}, 1),
		inflight: []*core.EventDeliveryResponse{
			{ID: eventUUID},
		},
		inflightBatches: []*core.WSEventBatch{
			{ID: fftypes.NewUUID()},
		},
		autoAck: true,
	}
	err := wsc.handleAck(&core.WSAck{
		ID: eventUUID,
	})
	assert.Regexp(t, "FF10180", err)
	assert.Len(t, wsc.inflight, 1)
	assert.Len(t, wsc.inflightBatches, 1)
}

func TestHandleStartFlippingAutoAck(t *testing.T) {
	eventUUID := fftypes.NewUUID()
	wsc := &websocketConnection{
		ctx: context.Background(),
		started: []*websocketStartedSub{{WSStart: core.WSStart{
			Ephemeral: false, Name: "name1", Namespace: "ns1",
		}}},
		sendMessages: make(chan interface{}, 1),
		inflight: []*core.EventDeliveryResponse{
			{ID: eventUUID},
		},
		autoAck: true,
	}
	no := false
	err := wsc.handleStart(&core.WSStart{
		AutoAck: &no,
	})
	assert.Regexp(t, "FF10179", err)
}

func TestHandleAckMultipleStartedMissingSub(t *testing.T) {
	eventUUID := fftypes.NewUUID()
	wsc := &websocketConnection{
		ctx: context.Background(),
		started: []*websocketStartedSub{
			{WSStart: core.WSStart{
				Ephemeral: false, Name: "name1", Namespace: "ns1",
			}},
			{WSStart: core.WSStart{
				Ephemeral: false, Name: "name2", Namespace: "ns1",
			}},
			{WSStart: core.WSStart{
				Ephemeral: false, Name: "name3", Namespace: "ns1",
			}},
		},
		sendMessages: make(chan interface{}, 1),
		inflight: []*core.EventDeliveryResponse{
			{ID: eventUUID},
		},
	}
	err := wsc.handleAck(&core.WSAck{
		ID: eventUUID,
	})
	assert.Regexp(t, "FF10175", err)

}

func TestHandleAckMultipleStartedNoSubSingleMatch(t *testing.T) {
	cbs := &eventsmocks.Callbacks{}
	cbs.On("DeliveryResponse", mock.Anything, mock.Anything).Return(nil)
	eventUUID := fftypes.NewUUID()
	wsc := &websocketConnection{
		ctx: context.Background(),
		ws: &WebSockets{
			ctx: context.Background(),
			callbacks: callbacks{
				handlers: map[string]events.Callbacks{"ns1": cbs},
			},
		},
		started: []*websocketStartedSub{{WSStart: core.WSStart{
			Ephemeral: false, Name: "name1", Namespace: "ns1",
		}}},
		sendMessages: make(chan interface{}, 1),
		inflight: []*core.EventDeliveryResponse{
			{ID: eventUUID},
		},
	}
	err := wsc.handleAck(&core.WSAck{
		ID: eventUUID,
	})
	assert.NoError(t, err)

}

func TestHandleAckNoneInflight(t *testing.T) {
	wsc := &websocketConnection{
		ctx:          context.Background(),
		sendMessages: make(chan interface{}, 1),
		inflight:     []*core.EventDeliveryResponse{},
	}
	err := wsc.handleAck(&core.WSAck{})
	assert.Regexp(t, "FF10175", err)
}

func TestProtocolErrorSwallowsSendError(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	wsc := &websocketConnection{
		ctx:          ctx,
		sendMessages: make(chan interface{}),
	}
	wsc.protocolError(fmt.Errorf("pop"))

}

func TestSendLoopBadData(t *testing.T) {
	cbs := &eventsmocks.Callbacks{}
	ws, wsc, cancel := newTestWebsockets(t, cbs, nil)
	defer cancel()

	subscribedConn := make(chan string, 1)
	cbs.On("EphemeralSubscription",
		mock.MatchedBy(func(s string) bool {
			subscribedConn <- s
			return true
		}),
		"ns1", mock.Anything, mock.Anything).Return(nil)

	err := wsc.Send(context.Background(), []byte(`{"type":"start","namespace":"ns1","ephemeral":true}`))
	assert.NoError(t, err)

	connID := <-subscribedConn
	connection := ws.connections[connID]
	connection.sendMessages <- map[bool]bool{false: true} // no JSON representation

	// Connection should close on its own with that bad data
	<-connection.senderDone

}

func TestUpgradeFail(t *testing.T) {
	cbs := &eventsmocks.Callbacks{}
	_, wsc, cancel := newTestWebsockets(t, cbs, nil)
	defer cancel()

	u, _ := url.Parse(wsc.URL())
	u.Scheme = "http"
	res, err := http.Get(u.String())
	assert.NoError(t, err)
	assert.Equal(t, 400, res.StatusCode)

}
func TestConnectionDispatchAfterClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	wsc := &websocketConnection{
		ctx: ctx,
	}
	err := wsc.dispatch(&core.EventDelivery{})
	assert.Regexp(t, "FF00147", err)
}

func TestConnectionDispatchBatchAfterClose(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	wsc := &websocketConnection{
		ctx: ctx,
	}
	err := wsc.dispatchBatch(&core.Subscription{}, []*core.CombinedEventDataDelivery{})
	assert.Regexp(t, "FF00147", err)
}

func TestWebsocketDispatchAfterClose(t *testing.T) {
	ws := &WebSockets{
		ctx:         context.Background(),
		connections: make(map[string]*websocketConnection),
	}
	err := ws.DeliveryRequest(ws.ctx, "gone", nil, &core.EventDelivery{}, nil)
	assert.Regexp(t, "FF10173", err)
}

func TestWebsocketBatchDispatchAfterClose(t *testing.T) {
	ws := &WebSockets{
		ctx:         context.Background(),
		connections: make(map[string]*websocketConnection),
	}
	err := ws.BatchDeliveryRequest(ws.ctx, "gone", nil, []*core.CombinedEventDataDelivery{})
	assert.Regexp(t, "FF10173", err)
}

func TestDispatchAutoAck(t *testing.T) {
	cbs := &eventsmocks.Callbacks{}
	cbs.On("DeliveryResponse", mock.Anything, mock.Anything).Return(nil)
	wsc := &websocketConnection{
		ctx:    context.Background(),
		connID: fftypes.NewUUID().String(),
		ws: &WebSockets{
			ctx: context.Background(),
			callbacks: callbacks{
				handlers: map[string]events.Callbacks{"ns1": cbs},
			},
			connections: make(map[string]*websocketConnection),
		},
		started: []*websocketStartedSub{{WSStart: core.WSStart{
			Ephemeral: false, Name: "name1", Namespace: "ns1",
		}}},
		sendMessages: make(chan interface{}, 1),
		autoAck:      true,
	}
	wsc.ws.connections[wsc.connID] = wsc
	err := wsc.ws.DeliveryRequest(wsc.ctx, wsc.connID, nil, &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{ID: fftypes.NewUUID()},
		},
		Subscription: core.SubscriptionRef{ID: fftypes.NewUUID(), Namespace: "ns1", Name: "sub1"},
	}, nil)
	assert.NoError(t, err)
	cbs.AssertExpectations(t)
}

func TestWebsocketSendAfterClose(t *testing.T) {
	cbs := &eventsmocks.Callbacks{}
	ws, wsc, cancel := newTestWebsockets(t, cbs, nil)
	defer cancel()

	subscribedConn := make(chan string, 1)
	cbs.On("EphemeralSubscription",
		mock.MatchedBy(func(s string) bool {
			subscribedConn <- s
			return true
		}),
		"ns1", mock.Anything, mock.Anything).Return(nil)

	err := wsc.Send(context.Background(), []byte(`{"type":"start","namespace":"ns1","ephemeral":true}`))
	assert.NoError(t, err)

	connID := <-subscribedConn
	connection := ws.connections[connID]
	connection.wsConn.Close()
	<-connection.senderDone
	err = connection.send(map[string]string{"foo": "bar"})
	assert.Regexp(t, "FF10290", err)
}

func TestWebsocketStatus(t *testing.T) {
	ws := &WebSockets{
		ctx:         context.Background(),
		connections: make(map[string]*websocketConnection),
	}
	ws.connections["id1"] = &websocketConnection{
		connID: "id1",
		started: []*websocketStartedSub{{WSStart: core.WSStart{
			Ephemeral: false, Name: "name1", Namespace: "ns1",
		}}},
		remoteAddr: "otherhost",
		userAgent:  "user",
	}

	status := ws.GetStatus()

	assert.Equal(t, true, status.Enabled)
	assert.Len(t, status.Connections, 1)
	assert.Equal(t, "id1", status.Connections[0].ID)
	assert.Equal(t, "otherhost", status.Connections[0].RemoteAddress)
	assert.Equal(t, "user", status.Connections[0].UserAgent)
	assert.Len(t, status.Connections[0].Subscriptions, 1)
	assert.Equal(t, false, status.Connections[0].Subscriptions[0].Ephemeral)
	assert.Equal(t, "ns1", status.Connections[0].Subscriptions[0].Namespace)
	assert.Equal(t, "name1", status.Connections[0].Subscriptions[0].Name)
}

func TestNamespaceRestarted(t *testing.T) {
	mcb := &eventsmocks.Callbacks{}
	mcb.On("RegisterConnection", mock.Anything, mock.Anything).Return(nil)
	ws := &WebSockets{
		ctx:         context.Background(),
		connections: make(map[string]*websocketConnection),
		callbacks: callbacks{
			handlers: map[string]events.Callbacks{"ns1": mcb},
		},
	}
	origTime := fftypes.Now()
	time.Sleep(1 * time.Microsecond)
	ws.connections["id1"] = &websocketConnection{
		ctx:    context.Background(),
		ws:     ws,
		connID: "id1",
		started: []*websocketStartedSub{
			{
				WSStart: core.WSStart{
					Ephemeral: false, Name: "name1", Namespace: "ns1",
				},
				startTime: origTime,
			},
		},
		remoteAddr: "otherhost",
		userAgent:  "user",
	}

	ws.NamespaceRestarted("ns1", time.Now())

	assert.Greater(t, *ws.connections["id1"].started[0].startTime, *origTime.Time())

	mcb.AssertExpectations(t)
}

func TestNamespaceRestartedFailClose(t *testing.T) {
	mcb := &eventsmocks.Callbacks{}
	mcb.On("RegisterConnection", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
	mcb.On("ConnectionClosed", mock.Anything, mock.Anything).Return(nil)
	ws := &WebSockets{
		ctx:         context.Background(),
		connections: make(map[string]*websocketConnection),
		callbacks: callbacks{
			handlers: map[string]events.Callbacks{"ns1": mcb},
		},
	}
	origTime := fftypes.Now()
	time.Sleep(1 * time.Microsecond)
	ctx, cancelCtx := context.WithCancel(context.Background())
	ws.connections["id1"] = &websocketConnection{
		ctx:       ctx,
		cancelCtx: cancelCtx,
		ws:        ws,
		connID:    "id1",
		started: []*websocketStartedSub{
			{
				WSStart: core.WSStart{
					Ephemeral: false, Name: "name1", Namespace: "ns1",
				},
				startTime: origTime,
			},
		},
		remoteAddr: "otherhost",
		userAgent:  "user",
	}

	ws.NamespaceRestarted("ns1", time.Now())

	mcb.AssertExpectations(t)
}

func TestNamespaceScopedSendWrongNamespaceStartAction(t *testing.T) {
	cbs := &eventsmocks.Callbacks{}
	_, wsc, cancel := newTestWebsocketsCommon(t, cbs, nil, "ns1")
	defer cancel()
	cbs.On("ConnectionClosed", mock.Anything).Return(nil)

	err := wsc.Send(context.Background(), []byte(`{"type":"start","namespace":"ns2"}`))
	assert.NoError(t, err)
	b := <-wsc.Receive()
	var res core.WSError
	err = json.Unmarshal(b, &res)
	assert.NoError(t, err)
	assert.Equal(t, core.WSProtocolErrorEventType, res.Type)
	assert.Regexp(t, "FF10462", res.Error)
}

func TestNamespaceScopedSendWrongNamespaceQueryParameter(t *testing.T) {
	cbs := &eventsmocks.Callbacks{}
	_, wsc, cancel := newTestWebsocketsCommon(t, cbs, nil, "ns1", "namespace=ns2")
	defer cancel()
	cbs.On("ConnectionClosed", mock.Anything).Return(nil)

	b := <-wsc.Receive()
	var res core.WSError
	err := json.Unmarshal(b, &res)
	assert.NoError(t, err)
	assert.Equal(t, core.WSProtocolErrorEventType, res.Type)
	assert.Regexp(t, "FF10462", res.Error)
}

func TestNamespaceScopedUpgradeFail(t *testing.T) {
	cbs := &eventsmocks.Callbacks{}
	_, wsc, cancel := newTestWebsocketsCommon(t, cbs, nil, "ns1")
	defer cancel()

	u, _ := url.Parse(wsc.URL())
	u.Scheme = "http"
	res, err := http.Get(u.String())
	assert.NoError(t, err)
	assert.Equal(t, 400, res.StatusCode)

}

func TestNamespaceScopedSuccess(t *testing.T) {
	cbs := &eventsmocks.Callbacks{}
	ws, wsc, cancel := newTestWebsocketsCommon(t, cbs, nil, "ns1")
	defer cancel()
	var connID string
	waitSubscribed := make(chan struct{})

	cbs.On("RegisterConnection",
		mock.MatchedBy(func(s string) bool { connID = s; return true }),
		mock.Anything,
	).Return(nil).Run(func(args mock.Arguments) {
		subMatch := args[1].(events.SubscriptionMatcher)
		assert.True(t, subMatch(core.SubscriptionRef{Namespace: "ns1", Name: "sub1"}))
		assert.False(t, subMatch(core.SubscriptionRef{Namespace: "ns2", Name: "sub1"}))
		assert.False(t, subMatch(core.SubscriptionRef{Namespace: "ns1", Name: "sub2"}))
		close(waitSubscribed)
	})
	ack := cbs.On("DeliveryResponse",
		mock.MatchedBy(func(s string) bool { return s == connID }),
		mock.Anything).Return(nil)

	waitAcked := make(chan struct{})
	ack.RunFn = func(a mock.Arguments) {
		close(waitAcked)
	}

	err := wsc.Send(context.Background(), []byte(`{"type":"start","name":"sub1"}`))
	assert.NoError(t, err)

	<-waitSubscribed
	ws.DeliveryRequest(ws.ctx, connID, nil, &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{ID: fftypes.NewUUID()},
		},
		Subscription: core.SubscriptionRef{
			ID:        fftypes.NewUUID(),
			Namespace: "ns1",
			Name:      "sub1",
		},
	}, nil)
	// Put a second in flight
	ws.DeliveryRequest(ws.ctx, connID, nil, &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{ID: fftypes.NewUUID()},
		},
		Subscription: core.SubscriptionRef{
			ID:        fftypes.NewUUID(),
			Namespace: "ns1",
			Name:      "sub2",
		},
	}, nil)

	b := <-wsc.Receive()
	var res core.EventDelivery
	err = json.Unmarshal(b, &res)
	assert.NoError(t, err)

	assert.Equal(t, "ns1", res.Subscription.Namespace)
	assert.Equal(t, "sub1", res.Subscription.Name)
	err = wsc.Send(context.Background(), []byte(fmt.Sprintf(`{
		"type":"ack",
		"id": "%s",
		"subscription": {
			"namespace": "ns1",
			"name": "sub1"
		}
	}`, res.ID)))
	assert.NoError(t, err)

	<-waitAcked

	// Check we left the right one behind
	conn := ws.connections[connID]
	assert.Equal(t, 1, len(conn.inflight))
	assert.Equal(t, "sub2", conn.inflight[0].Subscription.Name)

	cbs.AssertExpectations(t)
}

func TestHandleStartWrongNamespace(t *testing.T) {

	// it is not currently possible through exported functions to get to handleStart with the wrong namespace
	// but we like to have a final assertion in there as a safety net for accidentaly data leakage across namespaces
	// so to prove that safety net, we need to drive the private function handleStart directly.
	wc := &websocketConnection{
		ctx:             context.Background(),
		namespaceScoped: true,
		namespace:       "ns1",
	}
	startMessage := &core.WSStart{
		Namespace: "ns2",
	}
	err := wc.handleStart(startMessage)
	assert.Error(t, err)
	assert.Regexp(t, "FF10462", err)
}
