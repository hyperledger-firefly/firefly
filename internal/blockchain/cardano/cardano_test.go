// Copyright © 2025 IOG Singapore and SundaeSwap, Inc.
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

package cardano

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-common/pkg/ffresty"
	"github.com/hyperledger/firefly-common/pkg/fftls"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-common/pkg/wsclient"
	"github.com/hyperledger/firefly/internal/blockchain/common"
	"github.com/hyperledger/firefly/internal/coreconfig"
	"github.com/hyperledger/firefly/mocks/blockchainmocks"
	"github.com/hyperledger/firefly/mocks/cachemocks"
	"github.com/hyperledger/firefly/mocks/coremocks"
	"github.com/hyperledger/firefly/mocks/wsmocks"
	"github.com/hyperledger/firefly/pkg/blockchain"
	"github.com/hyperledger/firefly/pkg/core"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var utConfig = config.RootSection("cardano_unit_tests")
var utCardanoconnectConf = utConfig.SubSection(CardanoconnectConfigKey)

func testFFIMethod() *fftypes.FFIMethod {
	return &fftypes.FFIMethod{
		Name: "testFunc",
		Params: []*fftypes.FFIParam{
			{
				Name:   "varString",
				Schema: fftypes.JSONAnyPtr(`{"type": "string"}`),
			},
		},
	}
}

func resetConf(c *Cardano) {
	coreconfig.Reset()
	c.InitConfig(utConfig)
}

func newTestCardano() (*Cardano, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	r := resty.New().SetBaseURL("http://localhost:12345")
	c := &Cardano{
		ctx:         ctx,
		cancelCtx:   cancel,
		callbacks:   common.NewBlockchainCallbacks(),
		client:      r,
		pluginTopic: "topic1",
		streamIDs:   make(map[string]string),
		closed:      make(map[string]chan struct{}),
		wsconns:     make(map[string]wsclient.WSClient),
		streams: &streamManager{
			client: r,
		},
	}
	return c, func() {
		cancel()
		if c.closed != nil {
			// We've init'd, wait to close
			for _, cls := range c.closed {
				<-cls
			}
		}
	}
}

func TestInitMissingURL(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	cmi := &cachemocks.Manager{}

	err := c.Init(c.ctx, c.cancelCtx, utConfig, c.metrics, cmi)
	assert.Regexp(t, "FF10138.*url", err)
}

func TestBadTLSConfig(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	resetConf(c)
	utCardanoconnectConf.Set(ffresty.HTTPConfigURL, "http://localhost:12345")

	tlsConf := utCardanoconnectConf.SubSection("tls")
	tlsConf.Set(fftls.HTTPConfTLSEnabled, true)
	tlsConf.Set(fftls.HTTPConfTLSCAFile, "!!!!!badness")

	err := c.Init(c.ctx, c.cancelCtx, utConfig, c.metrics, &cachemocks.Manager{})
	assert.Regexp(t, "FF00153", err)
}

func TestInitMissingTopic(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	resetConf(c)
	utCardanoconnectConf.Set(ffresty.HTTPConfigURL, "http://localhost:12345")

	err := c.Init(c.ctx, c.cancelCtx, utConfig, c.metrics, &cachemocks.Manager{})
	assert.Regexp(t, "FF10138.*topic", err)
}

func TestInitAndStartWithCardanoConnect(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	toServer, fromServer, wsURL, done := wsclient.NewTestWSServer(nil)
	defer done()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	u, _ := url.Parse(wsURL)
	u.Scheme = "http"
	httpURL := u.String()

	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/v1/eventstreams", httpURL),
		httpmock.NewJsonResponderOrPanic(200, []eventStream{}))
	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/eventstreams", httpURL),
		httpmock.NewJsonResponderOrPanic(200, eventStream{ID: "es12345"}))

	resetConf(c)
	utCardanoconnectConf.Set(ffresty.HTTPConfigURL, httpURL)
	utCardanoconnectConf.Set(ffresty.HTTPCustomClient, mockedClient)
	utCardanoconnectConf.Set(CardanoconnectConfigTopic, "topic1")

	err := c.Init(c.ctx, c.cancelCtx, utConfig, c.metrics, &cachemocks.Manager{})
	assert.NoError(t, err)

	assert.Equal(t, "cardano", c.Name())
	assert.Equal(t, core.VerifierTypeCardanoAddress, c.VerifierType())

	err = c.StartNamespace(c.ctx, "ns1")
	assert.NoError(t, err)

	assert.Equal(t, 2, httpmock.GetTotalCallCount())
	assert.Equal(t, "es12345", c.streamIDs["ns1"])
	assert.NotNil(t, c.Capabilities())

	startupMessage := <-toServer
	assert.Equal(t, `{"type":"listen","topic":"topic1/ns1"}`, startupMessage)
	startupMessage = <-toServer
	assert.Equal(t, `{"type":"listenreplies"}`, startupMessage)
	fromServer <- `{"bad":"receipt"}` // will be ignored - no ack
	fromServer <- `[]`                // empty batch, will be ignored, but acked
	reply := <-toServer
	assert.Equal(t, `{"type":"ack","topic":"topic1/ns1"}`, reply)
	fromServer <- `["different kind of bad batch"]` // bad batch, will be ignored but acked
	reply = <-toServer
	assert.Equal(t, `{"type":"ack","topic":"topic1/ns1"}`, reply)
	fromServer <- `[{}]` // bad batch, will be ignored but acked
	reply = <-toServer
	assert.Equal(t, `{"type":"ack","topic":"topic1/ns1"}`, reply)

	// Bad data will be ignored
	fromServer <- `!json`
	fromServer <- `{"not": "a reply"}`
	fromServer <- `42`
}

func TestStartNamespaceWSFail(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	resetConf(c)
	utCardanoconnectConf.Set(ffresty.HTTPConfigURL, "!!!://")
	utCardanoconnectConf.Set(CardanoconnectConfigTopic, "topic1")

	err := c.Init(c.ctx, c.cancelCtx, utConfig, c.metrics, &cachemocks.Manager{})
	assert.NoError(t, err)

	err = c.StartNamespace(c.ctx, "ns1")
	assert.Regexp(t, "FF00149", err)
}

func TestStartNamespaceStreamQueryError(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", "http://localhost:12345/api/v1/eventstreams",
		httpmock.NewStringResponder(500, `pop`))

	resetConf(c)
	utCardanoconnectConf.Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	utCardanoconnectConf.Set(ffresty.HTTPConfigRetryEnabled, false)
	utCardanoconnectConf.Set(ffresty.HTTPCustomClient, mockedClient)
	utCardanoconnectConf.Set(CardanoconnectConfigTopic, "topic1")

	err := c.Init(c.ctx, c.cancelCtx, utConfig, c.metrics, &cachemocks.Manager{})
	assert.NoError(t, err)

	err = c.StartNamespace(c.ctx, "ns1")
	assert.Regexp(t, "FF10484.*pop", err)
}

func TestStartNamespaceStreamCreateError(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", "http://localhost:12345/api/v1/eventstreams",
		httpmock.NewJsonResponderOrPanic(200, []eventStream{}))
	httpmock.RegisterResponder("POST", "http://localhost:12345/api/v1/eventstreams",
		httpmock.NewStringResponder(500, "pop"))

	resetConf(c)
	utCardanoconnectConf.Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	utCardanoconnectConf.Set(ffresty.HTTPConfigRetryEnabled, false)
	utCardanoconnectConf.Set(ffresty.HTTPCustomClient, mockedClient)
	utCardanoconnectConf.Set(CardanoconnectConfigTopic, "topic1")

	err := c.Init(c.ctx, c.cancelCtx, utConfig, c.metrics, &cachemocks.Manager{})
	assert.NoError(t, err)

	err = c.StartNamespace(c.ctx, "ns1")
	assert.Regexp(t, "FF10484.*pop", err)
}

func TestStartNamespaceStreamUpdateError(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", "http://localhost:12345/api/v1/eventstreams",
		httpmock.NewJsonResponderOrPanic(200, []eventStream{{ID: "es12345", Name: "topic1/ns1"}}))
	httpmock.RegisterResponder("PATCH", "http://localhost:12345/api/v1/eventstreams/es12345",
		httpmock.NewStringResponder(500, "pop"))

	resetConf(c)
	utCardanoconnectConf.Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	utCardanoconnectConf.Set(ffresty.HTTPConfigRetryEnabled, false)
	utCardanoconnectConf.Set(ffresty.HTTPCustomClient, mockedClient)
	utCardanoconnectConf.Set(CardanoconnectConfigTopic, "topic1")

	err := c.Init(c.ctx, c.cancelCtx, utConfig, c.metrics, &cachemocks.Manager{})
	assert.NoError(t, err)

	err = c.StartNamespace(c.ctx, "ns1")
	assert.Regexp(t, "FF10484.*pop", err)
}

func TestStartNamespaceWSConnectFail(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	httpURL := "http://localhost:12345"

	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/v1/eventstreams", httpURL),
		httpmock.NewJsonResponderOrPanic(200, []eventStream{}))
	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/eventstreams", httpURL),
		httpmock.NewJsonResponderOrPanic(200, eventStream{ID: "es12345"}))
	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/v1/ws", httpURL),
		httpmock.NewJsonResponderOrPanic(500, "{}"))

	resetConf(c)
	utCardanoconnectConf.Set(ffresty.HTTPConfigURL, httpURL)
	utCardanoconnectConf.Set(ffresty.HTTPCustomClient, mockedClient)
	utCardanoconnectConf.Set(CardanoconnectConfigTopic, "topic1")
	utCardanoconnectConf.Set(wsclient.WSConfigKeyInitialConnectAttempts, 1)

	err := c.Init(c.ctx, c.cancelCtx, utConfig, c.metrics, &cachemocks.Manager{})
	assert.NoError(t, err)

	err = c.StartNamespace(c.ctx, "ns1")
	assert.Regexp(t, "FF00148", err)
}

func TestStartStopNamespace(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	toServer, _, wsURL, done := wsclient.NewTestWSServer(nil)
	defer done()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	u, _ := url.Parse(wsURL)
	u.Scheme = "http"
	httpURL := u.String()

	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/v1/eventstreams", httpURL),
		httpmock.NewJsonResponderOrPanic(200, []eventStream{}))
	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/eventstreams", httpURL),
		httpmock.NewJsonResponderOrPanic(200, eventStream{ID: "es12345"}))

	resetConf(c)
	utCardanoconnectConf.Set(ffresty.HTTPConfigURL, httpURL)
	utCardanoconnectConf.Set(ffresty.HTTPCustomClient, mockedClient)
	utCardanoconnectConf.Set(CardanoconnectConfigTopic, "topic1")

	err := c.Init(c.ctx, c.cancelCtx, utConfig, c.metrics, &cachemocks.Manager{})
	assert.NoError(t, err)

	err = c.StartNamespace(c.ctx, "ns1")
	assert.NoError(t, err)

	<-toServer

	err = c.StopNamespace(c.ctx, "ns1")
	assert.NoError(t, err)
}

func TestStopNamespace(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	err := c.StopNamespace(context.Background(), "ns1")
	assert.NoError(t, err)
}

func TestVerifyCardanoAddress(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	_, err := c.ResolveSigningKey(context.Background(), "", blockchain.ResolveKeyIntentSign)
	assert.Regexp(t, "FF10354", err)

	_, err = c.ResolveSigningKey(context.Background(), "baddr1cafed00d", blockchain.ResolveKeyIntentSign)
	assert.Regexp(t, "FF10483", err)

	key, err := c.ResolveSigningKey(context.Background(), "addr1qx2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzer3n0d3vllmyqwsx5wktcd8cc3sq835lu7drv2xwl2wywfgse35a3x", blockchain.ResolveKeyIntentSign)
	assert.NoError(t, err)
	assert.Equal(t, "addr1qx2fxv2umyhttkxyxp8x0dlpdt3k6cwng5pxj3jhsydzer3n0d3vllmyqwsx5wktcd8cc3sq835lu7drv2xwl2wywfgse35a3x", key)

	key, err = c.ResolveSigningKey(context.Background(), "addr_test1vqeux7xwusdju9dvsj8h7mca9aup2k439kfmwy773xxc2hcu7zy99", blockchain.ResolveKeyIntentSign)
	assert.NoError(t, err)
	assert.Equal(t, "addr_test1vqeux7xwusdju9dvsj8h7mca9aup2k439kfmwy773xxc2hcu7zy99", key)
}

func TestEventLoopContextCancelled(t *testing.T) {
	c, cancel := newTestCardano()
	cancel()
	r := make(<-chan []byte)
	wsm := &wsmocks.WSClient{}
	c.wsconns["ns1"] = wsm
	wsm.On("Receive").Return(r)
	wsm.On("Close").Return()
	c.closed["ns1"] = make(chan struct{})
	c.eventLoop("ns1")
	wsm.AssertExpectations(t)
}

func TestEventLoopReceiveClosed(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	r := make(chan []byte)
	wsm := &wsmocks.WSClient{}
	c.wsconns["ns1"] = wsm
	close(r)
	wsm.On("Receive").Return((<-chan []byte)(r))
	wsm.On("Close").Return()
	c.closed["ns1"] = make(chan struct{})
	c.eventLoop("ns1")
	wsm.AssertExpectations(t)
}

func TestEventLoopSendFailed(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	r := make(chan []byte)
	wsm := &wsmocks.WSClient{}
	c.wsconns["ns1"] = wsm
	wsm.On("Receive").Return((<-chan []byte)(r))
	wsm.On("Close").Return()
	wsm.On("Send", mock.Anything, mock.Anything).Return(errors.New("Send error"))
	c.closed["ns1"] = make(chan struct{})

	go c.eventLoop("ns1")
	r <- []byte(`{"batchNumber":9001,"events":["none"]}`)
	<-c.closed["ns1"]
}

func TestEventLoopBadMessage(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	r := make(chan []byte)
	wsm := &wsmocks.WSClient{}
	c.wsconns["ns1"] = wsm
	wsm.On("Receive").Return((<-chan []byte)(r))
	wsm.On("Close").Return()
	c.closed["ns1"] = make(chan struct{})

	go c.eventLoop("ns1")
	r <- []byte(`!badjson`)        // ignored bad json
	r <- []byte(`"not an object"`) // ignored wrong type
}

func TestEventLoopReceiveBatch(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", "http://localhost:12345/api/v1/eventstreams/es12345/listeners/lst12345",
		httpmock.NewJsonResponderOrPanic(200, listener{ID: "lst12345", Name: "ff-sub-default-12345"}))

	r := make(chan []byte)
	s := make(chan []byte)
	wsm := &wsmocks.WSClient{}
	c.wsconns["ns1"] = wsm
	wsm.On("Receive").Return((<-chan []byte)(r))
	wsm.On("Send", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		bytes, _ := args.Get(1).([]byte)
		s <- bytes
	}).Return(nil)
	wsm.On("Close").Return()
	c.streamIDs["ns1"] = "es12345"
	c.closed["ns1"] = make(chan struct{})

	client := resty.NewWithClient(mockedClient)
	client.SetBaseURL("http://localhost:12345")
	c.streams = &streamManager{
		client:       client,
		batchSize:    500,
		batchTimeout: 10000,
	}

	data := []byte(`{
		"batchNumber": 1337,
		"events": [
			{
				"type": "ContractEvent",
				"listenerId": "lst12345",
				"blockHash": "fcb0504f47abf2cc52cd6d509036d512fd6cbec19d0e1bbaaf21f0699882de7b",
				"blockNumber": 11466734,
				"signature": "TransactionFinalized(string)",
				"timestamp": "2025-02-10T12:00:00.000000000+00:00",
				"transactionIndex": 0,
				"transactionHash": "cc76904959438e05aaa83078bbdc81af5685a8e28ea4fcfcfd741df7df1e596d",
				"logIndex": 0,
				"data": {
					"transactionId": "bdae5f48cd7eec76938f62c648a1972907e24b4abb374b64609710792959e4fa"
				}
			}
		]
	}`)

	em := &blockchainmocks.Callbacks{}
	c.SetHandler("ns1", em)
	em.On("BlockchainEventBatch", mock.MatchedBy(func(events []*blockchain.EventToDispatch) bool {
		return len(events) == 1 &&
			events[0].Type == blockchain.EventTypeForListener &&
			events[0].ForListener.ListenerID == "lst12345"
	})).Return(nil)

	go c.eventLoop("ns1")

	r <- data
	response := <-s
	var parsed cardanoWSCommandPayload
	err := json.Unmarshal(response, &parsed)
	assert.NoError(t, err)
	assert.Equal(t, "topic1/ns1", parsed.Topic)
	assert.Equal(t, int64(1337), parsed.BatchNumber)
	assert.Equal(t, "ack", parsed.Type)
}

func TestEventLoopReceiveBadBatch(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", "http://localhost:12345/api/v1/eventstreams/es12345/listeners/lst12345",
		httpmock.NewJsonResponderOrPanic(200, listener{ID: "lst12345", Name: "ff-sub-default-12345"}))
	client := resty.NewWithClient(mockedClient)
	client.SetBaseURL("http://localhost:12345")
	c.streams = &streamManager{
		client:       client,
		batchSize:    500,
		batchTimeout: 10000,
	}

	r := make(chan []byte)
	s := make(chan []byte)
	wsm := &wsmocks.WSClient{}
	c.wsconns["ns1"] = wsm
	wsm.On("Receive").Return((<-chan []byte)(r))
	wsm.On("Send", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		bytes, _ := args.Get(1).([]byte)
		s <- bytes
	}).Return(nil)
	wsm.On("Close").Return()
	c.streamIDs["ns1"] = "es12345"
	c.closed["ns1"] = make(chan struct{})

	data := []byte(`{
		"batchNumber": 1337,
		"events": [
		    {
		        "type": "ContractEvent",
				"listenerId": "lst12345",
				"blockHash": "fcb0504f47abf2cc52cd6d509036d512fd6cbec19d0e1bbaaf21f0699882de7b",
				"blockNumber": 11466734,
				"signature": "TransactionFinalized(string)",
				"timestamp": "2025-02-10T12:00:00.000000000+00:00",
				"transactionIndex": 0,
				"transactionHash": "cc76904959438e05aaa83078bbdc81af5685a8e28ea4fcfcfd741df7df1e596d",
				"logIndex": 0,
				"data": {
					"transactionId": "bdae5f48cd7eec76938f62c648a1972907e24b4abb374b64609710792959e4fa"
				}
			}
		]
	}`)

	em := &blockchainmocks.Callbacks{}
	c.SetHandler("ns1", em)
	em.On("BlockchainEventBatch", mock.MatchedBy(func(events []*blockchain.EventToDispatch) bool {
		return len(events) == 1 &&
			events[0].Type == blockchain.EventTypeForListener &&
			events[0].ForListener.ListenerID == "lst12345"
	})).Return(errors.New("My Error Message"))

	go c.eventLoop("ns1")

	r <- data
	response := <-s
	var parsed cardanoWSCommandPayload
	err := json.Unmarshal(response, &parsed)
	assert.NoError(t, err)
	assert.Equal(t, "topic1/ns1", parsed.Topic)
	assert.Equal(t, int64(1337), parsed.BatchNumber)
	assert.Equal(t, "error", parsed.Type)
	assert.Equal(t, "My Error Message", parsed.Message)
}

func TestEventLoopReceiveMalformedBatch(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	r := make(chan []byte)
	s := make(chan []byte)
	wsm := &wsmocks.WSClient{}
	c.wsconns["ns1"] = wsm
	wsm.On("Receive").Return((<-chan []byte)(r))
	wsm.On("Send", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		bytes, _ := args.Get(1).([]byte)
		s <- bytes
	}).Return(nil)
	wsm.On("Close").Return()
	c.streamIDs["ns1"] = "es12345"
	c.closed["ns1"] = make(chan struct{})

	go c.eventLoop("ns1")

	r <- []byte(`{
		"batchNumber": 1338,
		"events": [
			{
				"type": "ContractEvent",
				"listenerId": "lst12345",
				"blockNumber": 1337,
				"transactionHash": "cafed00d"
			},
			{
				"type": "ContractEvent",
				"listenerId": "lst12345",
				"blockNumber": 1337,
				"timestamp": "2025-02-10T12:00:00.000000000+00:00"
			},
			{
				"type": "ContractEvent",
				"listenerId": "lst12345",
				"timestamp": "2025-02-10T12:00:00.000000000+00:00",
				"transactionHash": "cafed00d"
			}
		]
	}`)
	response := <-s
	var parsed cardanoWSCommandPayload
	err := json.Unmarshal(response, &parsed)
	assert.NoError(t, err)
	assert.Equal(t, "topic1/ns1", parsed.Topic)
	assert.Equal(t, int64(1338), parsed.BatchNumber)
	assert.Equal(t, "ack", parsed.Type)
}

func TestEventLoopReceiveReceipt(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	r := make(chan []byte)
	s := make(chan []byte)
	wsm := &wsmocks.WSClient{}
	c.wsconns["ns1"] = wsm
	wsm.On("Receive").Return((<-chan []byte)(r))
	wsm.On("Send", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		bytes, _ := args.Get(1).([]byte)
		s <- bytes
	}).Return(nil)
	wsm.On("Close").Return()
	c.streamIDs["ns1"] = "es12345"
	c.closed["ns1"] = make(chan struct{})

	tm := &coremocks.OperationCallbacks{}
	c.SetOperationHandler("ns1", tm)
	tm.On("BulkOperationUpdates", mock.Anything, mock.MatchedBy(func(updates []*core.OperationUpdate) bool {
		return updates[0].NamespacedOpID == "ns1:5678" &&
			updates[0].Status == core.OpStatusSucceeded &&
			updates[0].BlockchainTXID == "txHash" &&
			updates[0].Plugin == "cardano"
	})).Return(nil)

	go c.eventLoop("ns1")

	r <- []byte(`{
		"batchNumber": 1339,
		"events": [
			{
				"type": "Nonsense"
			},
			{
				"type": "Receipt",
				"headers": {
					"requestId": "ns1:1234"
				}
			},
			{
				"type": "Receipt",
				"headers": {
					"requestId": "ns1:5678",
					"type": "TransactionSuccess"
				},
				"transactionHash": "txHash"
			}
		]
	}`)
	response := <-s
	var parsed cardanoWSCommandPayload
	err := json.Unmarshal(response, &parsed)
	assert.NoError(t, err)
	assert.Equal(t, "topic1/ns1", parsed.Topic)
	assert.Equal(t, int64(1339), parsed.BatchNumber)
	assert.Equal(t, "ack", parsed.Type)
}

func TestEventLoopReceiveReceiptBulkOperationUpdateFail(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	r := make(chan []byte)
	s := make(chan []byte)
	wsm := &wsmocks.WSClient{}
	c.wsconns["ns1"] = wsm
	wsm.On("Receive").Return((<-chan []byte)(r))
	wsm.On("Send", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		bytes, _ := args.Get(1).([]byte)
		s <- bytes
	}).Return(nil)
	wsm.On("Close").Return()
	c.streamIDs["ns1"] = "es12345"
	c.closed["ns1"] = make(chan struct{})

	tm := &coremocks.OperationCallbacks{}
	c.SetOperationHandler("ns1", tm)
	tm.On("BulkOperationUpdates", mock.Anything, mock.MatchedBy(func(updates []*core.OperationUpdate) bool {
		return updates[0].NamespacedOpID == "ns1:5678" &&
			updates[0].Status == core.OpStatusSucceeded &&
			updates[0].BlockchainTXID == "txHash" &&
			updates[0].Plugin == "cardano"
	})).Return(errors.New("whoops"))

	go c.eventLoop("ns1")

	r <- []byte(`{
		"batchNumber": 1339,
		"events": [
			{
				"type": "Receipt",
				"headers": {
					"requestId": "ns1:5678",
					"type": "TransactionSuccess"
				},
				"transactionHash": "txHash"
			}
		]
	}`)
	response := <-s
	var parsed cardanoWSCommandPayload
	err := json.Unmarshal(response, &parsed)
	assert.NoError(t, err)
	assert.Equal(t, "topic1/ns1", parsed.Topic)
	assert.Equal(t, int64(1339), parsed.BatchNumber)
	assert.Equal(t, "error", parsed.Type)
	assert.Equal(t, "whoops", parsed.Message)
}

func TestSubmitBatchPinNotSupported(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	err := c.SubmitBatchPin(c.ctx, "", "", "", nil, nil)
	assert.Regexp(t, "FF10429", err)
}

func TestAddContractListener(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	httpmock.ActivateNonDefault(c.client.GetClient())
	defer httpmock.DeactivateAndReset()
	c.streamIDs["ns1"] = "es-1"

	sub := &core.ContractListener{
		Name:      "sample",
		Namespace: "ns1",
		Filters: []*core.ListenerFilter{
			{
				Event: &core.FFISerializedEvent{
					FFIEventDefinition: fftypes.FFIEventDefinition{
						Name: "Changed",
					},
				},
				Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
					"address": "submit-tx",
				}.String()),
			},
		},
		Options: &core.ContractListenerOptions{
			FirstEvent: string(core.SubOptsFirstEventOldest),
		},
	}

	httpmock.RegisterResponder("POST", "http://localhost:12345/api/v1/eventstreams/es-1/listeners",
		httpmock.NewJsonResponderOrPanic(200, &listener{ID: "new-id"}))

	err := c.AddContractListener(context.Background(), sub, "")
	assert.NoError(t, err)
	assert.Equal(t, "new-id", sub.BackendID)
}

func TestAddContractListenerNoFilter(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	httpmock.ActivateNonDefault(c.client.GetClient())
	defer httpmock.DeactivateAndReset()
	c.streamIDs["ns1"] = "es-1"

	sub := &core.ContractListener{
		Name:      "sample",
		Namespace: "ns1",
		Filters:   []*core.ListenerFilter{},
		Options: &core.ContractListenerOptions{
			FirstEvent: string(core.SubOptsFirstEventOldest),
		},
	}

	err := c.AddContractListener(context.Background(), sub, "")
	assert.Regexp(t, "FF10475", err)
}

func TestAddContractListenerBadLocation(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	httpmock.ActivateNonDefault(c.client.GetClient())
	defer httpmock.DeactivateAndReset()
	c.streamIDs["ns1"] = "es-1"

	sub := &core.ContractListener{
		Name:      "Sample",
		Namespace: "ns1",
		Filters: []*core.ListenerFilter{
			{
				Event: &core.FFISerializedEvent{
					FFIEventDefinition: fftypes.FFIEventDefinition{
						Name: "Changed",
					},
				},
				Location: fftypes.JSONAnyPtr("42"),
			},
		},
		Options: &core.ContractListenerOptions{
			FirstEvent: string(core.SubOptsFirstEventOldest),
		},
	}

	httpmock.RegisterResponder("POST", "http://localhost:12345/api/v1/eventstreams/es-1/listeners",
		httpmock.NewJsonResponderOrPanic(200, &listener{ID: "new-id"}))

	err := c.AddContractListener(context.Background(), sub, "")
	assert.Regexp(t, "10310", err)
}

func TestDeleteContractListener(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	httpmock.ActivateNonDefault(c.client.GetClient())
	defer httpmock.DeactivateAndReset()

	c.streamIDs["ns1"] = "es-1"

	sub := &core.ContractListener{
		Namespace: "ns1",
		BackendID: "sb-1",
	}

	httpmock.RegisterResponder("DELETE", `http://localhost:12345/api/v1/eventstreams/es-1/listeners/sb-1`,
		httpmock.NewStringResponder(204, ""))

	err := c.DeleteContractListener(context.Background(), sub, true)
	assert.NoError(t, err)
}

func TestDeleteContractListenerFail(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	httpmock.ActivateNonDefault(c.client.GetClient())
	defer httpmock.DeactivateAndReset()

	c.streamIDs["ns1"] = "es-1"

	sub := &core.ContractListener{
		Namespace: "ns1",
		BackendID: "sb-1",
	}

	httpmock.RegisterResponder("DELETE", `http://localhost:12345/api/v1/eventstreams/es-1/listeners/sb-1`,
		httpmock.NewStringResponder(500, "oops"))

	err := c.DeleteContractListener(context.Background(), sub, true)
	assert.Regexp(t, "FF10484", err)
}

func TestGetContractListenerStatus(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	httpmock.ActivateNonDefault(c.client.GetClient())
	defer httpmock.DeactivateAndReset()

	c.streamIDs["ns1"] = "es-1"

	httpmock.RegisterResponder("GET", `http://localhost:12345/api/v1/eventstreams/es-1/listeners/sb-1`,
		httpmock.NewJsonResponderOrPanic(200, &listener{ID: "sb-1", Name: "something"}))

	found, _, status, err := c.GetContractListenerStatus(context.Background(), "ns1", "sb-1", true)
	assert.NoError(t, err)
	assert.Equal(t, core.ContractListenerStatusUnknown, status)
	assert.True(t, found)
}

func TestGetContractListenerStatusNotFound(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	httpmock.ActivateNonDefault(c.client.GetClient())
	defer httpmock.DeactivateAndReset()

	c.streamIDs["ns1"] = "es-1"

	httpmock.RegisterResponder("GET", `http://localhost:12345/api/v1/eventstreams/es-1/listeners/sb-1`,
		httpmock.NewStringResponder(404, "no"))

	found, _, status, err := c.GetContractListenerStatus(context.Background(), "ns1", "sb-1", true)
	assert.NoError(t, err)
	assert.Equal(t, core.ContractListenerStatusUnknown, status)
	assert.False(t, found)
}

func TestGetContractListenerErrorNotFound(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	httpmock.ActivateNonDefault(c.client.GetClient())
	defer httpmock.DeactivateAndReset()

	c.streamIDs["ns1"] = "es-1"

	httpmock.RegisterResponder("GET", `http://localhost:12345/api/v1/eventstreams/es-1/listeners/sb-1`,
		httpmock.NewStringResponder(404, "no"))

	_, _, _, err := c.GetContractListenerStatus(context.Background(), "ns1", "sb-1", false)
	assert.Regexp(t, "FF10484", err)
}

func TestGetTransactionStatusSuccess(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	httpmock.ActivateNonDefault(c.client.GetClient())
	defer httpmock.DeactivateAndReset()

	op := &core.Operation{
		Namespace: "ns1",
		ID:        fftypes.MustParseUUID("9ffc50ff-6bfe-4502-adc7-93aea54cc059"),
		Status:    "Pending",
	}

	httpmock.RegisterResponder("GET", "http://localhost:12345/api/v1/transactions/ns1:9ffc50ff-6bfe-4502-adc7-93aea54cc059",
		func(req *http.Request) (*http.Response, error) {
			transactionStatus := make(map[string]interface{})
			transactionStatus["id"] = "ns1:9ffc50ff-6bfe-4502-adc7-93aea54cc059"
			transactionStatus["status"] = "Succeeded"
			transactionStatus["transactionHash"] = "txHash"
			return httpmock.NewJsonResponderOrPanic(200, transactionStatus)(req)
		})

	status, err := c.GetTransactionStatus(context.Background(), op)
	assert.NoError(t, err)
	assert.NotNil(t, status)
}

func TestGetTransactionStatusFailure(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	httpmock.ActivateNonDefault(c.client.GetClient())
	defer httpmock.DeactivateAndReset()

	op := &core.Operation{
		Namespace: "ns1",
		ID:        fftypes.MustParseUUID("9ffc50ff-6bfe-4502-adc7-93aea54cc059"),
		Status:    "Pending",
	}

	httpmock.RegisterResponder("GET", "http://localhost:12345/api/v1/transactions/ns1:9ffc50ff-6bfe-4502-adc7-93aea54cc059",
		func(req *http.Request) (*http.Response, error) {
			transactionStatus := make(map[string]interface{})
			transactionStatus["id"] = "ns1:9ffc50ff-6bfe-4502-adc7-93aea54cc059"
			transactionStatus["status"] = "Failed"
			transactionStatus["errorMessage"] = "Something went wrong"
			return httpmock.NewJsonResponderOrPanic(200, transactionStatus)(req)
		})

	status, err := c.GetTransactionStatus(context.Background(), op)
	assert.NoError(t, err)
	assert.NotNil(t, status)
}

func TestGetTransactionStatusEmptyObject(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	httpmock.ActivateNonDefault(c.client.GetClient())
	defer httpmock.DeactivateAndReset()

	op := &core.Operation{
		Namespace: "ns1",
		ID:        fftypes.MustParseUUID("9ffc50ff-6bfe-4502-adc7-93aea54cc059"),
		Status:    "Pending",
	}

	httpmock.RegisterResponder("GET", "http://localhost:12345/api/v1/transactions/ns1:9ffc50ff-6bfe-4502-adc7-93aea54cc059",
		func(req *http.Request) (*http.Response, error) {
			transactionStatus := make(map[string]interface{})
			return httpmock.NewJsonResponderOrPanic(200, transactionStatus)(req)
		})

	status, err := c.GetTransactionStatus(context.Background(), op)
	assert.NoError(t, err)
	assert.NotNil(t, status)
}

func TestGetTransactionStatusInvalidTx(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	httpmock.ActivateNonDefault(c.client.GetClient())
	defer httpmock.DeactivateAndReset()

	op := &core.Operation{
		Namespace: "",
		ID:        fftypes.MustParseUUID("9ffc50ff-6bfe-4502-adc7-93aea54cc059"),
		Status:    "Pending",
	}

	httpmock.RegisterResponder("GET", "http://localhost:12345/api/v1/transactions/:9ffc50ff-6bfe-4502-adc7-93aea54cc059",
		func(req *http.Request) (*http.Response, error) {
			transactionStatus := make(map[string]interface{})
			transactionStatus["status"] = "Failed"
			transactionStatus["errorMessage"] = "Something went wrong"
			return httpmock.NewJsonResponderOrPanic(200, transactionStatus)(req)
		})

	status, err := c.GetTransactionStatus(context.Background(), op)
	assert.NoError(t, err)
	assert.NotNil(t, status)
}

func TestGetTransactionStatusNotFound(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	httpmock.ActivateNonDefault(c.client.GetClient())
	defer httpmock.DeactivateAndReset()

	op := &core.Operation{
		Namespace: "ns1",
		ID:        fftypes.MustParseUUID("9ffc50ff-6bfe-4502-adc7-93aea54cc059"),
		Status:    "Pending",
	}

	httpmock.RegisterResponder("GET", "http://localhost:12345/api/v1/transactions/ns1:9ffc50ff-6bfe-4502-adc7-93aea54cc059",
		httpmock.NewStringResponder(404, "nah"))

	status, err := c.GetTransactionStatus(context.Background(), op)
	assert.NoError(t, err)
	assert.Nil(t, status)
}

func TestGetTransactionStatusError(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	httpmock.ActivateNonDefault(c.client.GetClient())
	defer httpmock.DeactivateAndReset()

	op := &core.Operation{
		Namespace: "ns1",
		ID:        fftypes.MustParseUUID("9ffc50ff-6bfe-4502-adc7-93aea54cc059"),
		Status:    "Pending",
	}

	httpmock.RegisterResponder("GET", "http://localhost:12345/api/v1/transactions/ns1:9ffc50ff-6bfe-4502-adc7-93aea54cc059",
		httpmock.NewStringResponder(500, "uh oh"))

	_, err := c.GetTransactionStatus(context.Background(), op)
	assert.Regexp(t, "FF10484", err)
}

func TestGetTransactionStatusHandleReceipt(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	httpmock.ActivateNonDefault(c.client.GetClient())
	defer httpmock.DeactivateAndReset()

	op := &core.Operation{
		Namespace: "ns1",
		ID:        fftypes.MustParseUUID("9ffc50ff-6bfe-4502-adc7-93aea54cc059"),
		Status:    "Pending",
	}

	httpmock.RegisterResponder("GET", "http://localhost:12345/api/v1/transactions/ns1:9ffc50ff-6bfe-4502-adc7-93aea54cc059",
		func(req *http.Request) (*http.Response, error) {
			transactionStatus := make(map[string]interface{})
			transactionStatus["status"] = "Succeeded"
			return httpmock.NewJsonResponderOrPanic(200, transactionStatus)(req)
		})

	status, err := c.GetTransactionStatus(context.Background(), op)
	assert.NoError(t, err)
	assert.NotNil(t, status)
}

func TestSubmitNetworkActionNotSupported(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	err := c.SubmitNetworkAction(c.ctx, "", "", core.NetworkActionTerminate, nil)
	assert.Regexp(t, "FF10429", err)
}

func TestAddFireflySubscriptionBadLocation(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", "http://localhost:12345/api/v1/eventstreams/es12345/listeners",
		httpmock.NewJsonResponderOrPanic(200, &[]listener{}))
	client := resty.NewWithClient(mockedClient)
	client.SetBaseURL("http://localhost:12345")
	c.streamIDs["ns1"] = "es12345"

	location := fftypes.JSONAnyPtr(fftypes.JSONObject{
		"bad": "bad",
	}.String())
	contract := &blockchain.MultipartyContract{
		Location:   location,
		FirstEvent: "oldest",
	}

	ns := &core.Namespace{Name: "ns1", NetworkName: "ns1"}
	_, err := c.AddFireflySubscription(c.ctx, ns, contract, "")
	assert.Regexp(t, "FF10310", err)
}

func TestAddAndRemoveFireflySubscription(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	toServer, _, wsURL, done := wsclient.NewTestWSServer(nil)
	defer done()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	u, _ := url.Parse(wsURL)
	u.Scheme = "http"
	httpURL := u.String()

	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/v1/eventstreams", httpURL),
		httpmock.NewJsonResponderOrPanic(200, []eventStream{{ID: "es12345", Name: "topic1/ns1"}}))
	httpmock.RegisterResponder("PATCH", fmt.Sprintf("%s/api/v1/eventstreams/es12345", httpURL),
		httpmock.NewJsonResponderOrPanic(200, eventStream{ID: "es12345", Name: "topic1/ns1"}))
	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/v1/eventstreams/es12345/listeners", httpURL),
		httpmock.NewJsonResponderOrPanic(200, []listener{}))
	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/eventstreams/es12345/listeners", httpURL),
		httpmock.NewJsonResponderOrPanic(200, listener{ID: "lst12345", Name: "ns1_2_BatchPin"}))

	resetConf(c)
	utCardanoconnectConf.Set(ffresty.HTTPConfigURL, httpURL)
	utCardanoconnectConf.Set(ffresty.HTTPCustomClient, mockedClient)
	utCardanoconnectConf.Set(CardanoconnectConfigTopic, "topic1")

	err := c.Init(c.ctx, c.cancelCtx, utConfig, c.metrics, &cachemocks.Manager{})
	assert.NoError(t, err)
	err = c.StartNamespace(c.ctx, "ns1")
	assert.NoError(t, err)
	startupMessage := <-toServer
	assert.Equal(t, `{"type":"listen","topic":"topic1/ns1"}`, startupMessage)
	startupMessage = <-toServer
	assert.Equal(t, `{"type":"listenreplies"}`, startupMessage)

	location := fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "submit-tx",
	}.String())
	contract := &blockchain.MultipartyContract{
		Location:   location,
		FirstEvent: "oldest",
	}

	ns := &core.Namespace{Name: "ns1", NetworkName: "ns1"}
	subID, err := c.AddFireflySubscription(c.ctx, ns, contract, "")
	assert.NoError(t, err)
	assert.NotNil(t, c.subs.GetSubscription(subID))

	c.RemoveFireflySubscription(c.ctx, subID)
	assert.Nil(t, c.subs.GetSubscription(subID))
}

func TestAddFireflySubscriptionListError(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	toServer, _, wsURL, done := wsclient.NewTestWSServer(nil)
	defer done()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	u, _ := url.Parse(wsURL)
	u.Scheme = "http"
	httpURL := u.String()

	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/v1/eventstreams", httpURL),
		httpmock.NewJsonResponderOrPanic(200, []eventStream{{ID: "es12345", Name: "topic1/ns1"}}))
	httpmock.RegisterResponder("PATCH", fmt.Sprintf("%s/api/v1/eventstreams/es12345", httpURL),
		httpmock.NewJsonResponderOrPanic(200, eventStream{ID: "es12345", Name: "topic1/ns1"}))
	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/v1/eventstreams/es12345/listeners", httpURL),
		httpmock.NewStringResponder(500, "whoopsies"))

	resetConf(c)
	utCardanoconnectConf.Set(ffresty.HTTPConfigURL, httpURL)
	utCardanoconnectConf.Set(ffresty.HTTPCustomClient, mockedClient)
	utCardanoconnectConf.Set(CardanoconnectConfigTopic, "topic1")

	err := c.Init(c.ctx, c.cancelCtx, utConfig, c.metrics, &cachemocks.Manager{})
	assert.NoError(t, err)
	err = c.StartNamespace(c.ctx, "ns1")
	assert.NoError(t, err)
	startupMessage := <-toServer
	assert.Equal(t, `{"type":"listen","topic":"topic1/ns1"}`, startupMessage)
	startupMessage = <-toServer
	assert.Equal(t, `{"type":"listenreplies"}`, startupMessage)

	location := fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "submit-tx",
	}.String())
	contract := &blockchain.MultipartyContract{
		Location:   location,
		FirstEvent: "oldest",
	}

	ns := &core.Namespace{Name: "ns1", NetworkName: "ns1"}
	_, err = c.AddFireflySubscription(c.ctx, ns, contract, "")
	assert.Regexp(t, "FF10484", err)
}

func TestAddFireflySubscriptionAlreadyExists(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	toServer, _, wsURL, done := wsclient.NewTestWSServer(nil)
	defer done()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	u, _ := url.Parse(wsURL)
	u.Scheme = "http"
	httpURL := u.String()

	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/v1/eventstreams", httpURL),
		httpmock.NewJsonResponderOrPanic(200, []eventStream{{ID: "es12345", Name: "topic1/ns1"}}))
	httpmock.RegisterResponder("PATCH", fmt.Sprintf("%s/api/v1/eventstreams/es12345", httpURL),
		httpmock.NewJsonResponderOrPanic(200, eventStream{ID: "es12345", Name: "topic1/ns1"}))
	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/v1/eventstreams/es12345/listeners", httpURL),
		httpmock.NewJsonResponderOrPanic(200, []listener{{ID: "lst12345", Name: "ns1_2_BatchPin"}}))

	resetConf(c)
	utCardanoconnectConf.Set(ffresty.HTTPConfigURL, httpURL)
	utCardanoconnectConf.Set(ffresty.HTTPCustomClient, mockedClient)
	utCardanoconnectConf.Set(CardanoconnectConfigTopic, "topic1")

	err := c.Init(c.ctx, c.cancelCtx, utConfig, c.metrics, &cachemocks.Manager{})
	assert.NoError(t, err)
	err = c.StartNamespace(c.ctx, "ns1")
	startupMessage := <-toServer
	assert.Equal(t, `{"type":"listen","topic":"topic1/ns1"}`, startupMessage)
	startupMessage = <-toServer
	assert.Equal(t, `{"type":"listenreplies"}`, startupMessage)

	location := fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "submit-tx",
	}.String())
	contract := &blockchain.MultipartyContract{
		Location:   location,
		FirstEvent: "oldest",
	}

	ns := &core.Namespace{Name: "ns1", NetworkName: "ns1"}
	subID, err := c.AddFireflySubscription(c.ctx, ns, contract, "")
	assert.NoError(t, err)
	assert.Equal(t, "lst12345", subID)
}

func TestAddFireflySubscriptionCreateError(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	toServer, _, wsURL, done := wsclient.NewTestWSServer(nil)
	defer done()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	u, _ := url.Parse(wsURL)
	u.Scheme = "http"
	httpURL := u.String()

	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/v1/eventstreams", httpURL),
		httpmock.NewJsonResponderOrPanic(200, []eventStream{{ID: "es12345", Name: "topic1/ns1"}}))
	httpmock.RegisterResponder("PATCH", fmt.Sprintf("%s/api/v1/eventstreams/es12345", httpURL),
		httpmock.NewJsonResponderOrPanic(200, eventStream{ID: "es12345", Name: "topic1/ns1"}))
	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/v1/eventstreams/es12345/listeners", httpURL),
		httpmock.NewJsonResponderOrPanic(200, []listener{}))
	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/eventstreams/es12345/listeners", httpURL),
		httpmock.NewStringResponder(500, "whoopsies"))

	resetConf(c)
	utCardanoconnectConf.Set(ffresty.HTTPConfigURL, httpURL)
	utCardanoconnectConf.Set(ffresty.HTTPCustomClient, mockedClient)
	utCardanoconnectConf.Set(CardanoconnectConfigTopic, "topic1")

	err := c.Init(c.ctx, c.cancelCtx, utConfig, c.metrics, &cachemocks.Manager{})
	assert.NoError(t, err)
	err = c.StartNamespace(c.ctx, "ns1")
	assert.NoError(t, err)
	startupMessage := <-toServer
	assert.Equal(t, `{"type":"listen","topic":"topic1/ns1"}`, startupMessage)
	startupMessage = <-toServer
	assert.Equal(t, `{"type":"listenreplies"}`, startupMessage)

	location := fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "submit-tx",
	}.String())
	contract := &blockchain.MultipartyContract{
		Location:   location,
		FirstEvent: "oldest",
	}

	ns := &core.Namespace{Name: "ns1", NetworkName: "ns1"}
	_, err = c.AddFireflySubscription(c.ctx, ns, contract, "")
	assert.Regexp(t, "FF10484", err)
}

func TestInvokeContractOK(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	httpmock.ActivateNonDefault(c.client.GetClient())
	defer httpmock.DeactivateAndReset()
	location := &Location{
		Address: "simple-tx",
	}
	options := map[string]interface{}{
		"customOption": "customValue",
	}
	signingKey := "signingKey"
	method := testFFIMethod()
	params := map[string]interface{}{
		"varString": "str",
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)

	httpmock.RegisterResponder("POST", "http://localhost:12345/api/v1/contracts/invoke", func(req *http.Request) (*http.Response, error) {
		var body map[string]interface{}
		json.NewDecoder(req.Body).Decode(&body)
		params := body["params"].([]interface{})
		assert.Equal(t, "opId", body["id"])
		assert.Equal(t, "simple-tx", body["address"])
		assert.Equal(t, "testFunc", body["method"].(map[string]interface{})["name"])
		assert.Equal(t, 1, len(params))
		assert.Equal(t, signingKey, body["from"])
		return httpmock.NewJsonResponderOrPanic(200, "")(req)
	})

	parsedMethod, err := c.ParseInterface(context.Background(), method, nil)
	assert.NoError(t, err)

	_, err = c.InvokeContract(context.Background(), "opId", signingKey, fftypes.JSONAnyPtrBytes(locationBytes), parsedMethod, params, options, nil)
	assert.NoError(t, err)
}

func TestInvokeContractAddressNotSet(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	httpmock.ActivateNonDefault(c.client.GetClient())
	defer httpmock.DeactivateAndReset()
	location := &Location{}
	options := map[string]interface{}{
		"customOption": "customValue",
	}
	signingKey := "signingKey"
	method := testFFIMethod()
	params := map[string]interface{}{
		"varString": "str",
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)

	parsedMethod, err := c.ParseInterface(context.Background(), method, nil)
	assert.NoError(t, err)

	_, err = c.InvokeContract(context.Background(), "", signingKey, fftypes.JSONAnyPtrBytes(locationBytes), parsedMethod, params, options, nil)
	assert.Regexp(t, "FF10310", err)
}

func TestInvokeContractBadMethod(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	httpmock.ActivateNonDefault(c.client.GetClient())
	defer httpmock.DeactivateAndReset()
	location := &Location{
		Address: "simple-tx",
	}
	options := map[string]interface{}{
		"customOption": "customValue",
	}
	signingKey := "signingKey"
	method := &fftypes.FFIMethod{}
	params := map[string]interface{}{
		"varString": "str",
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)

	parsedMethod, err := c.ParseInterface(context.Background(), method, nil)
	assert.NoError(t, err)

	_, err = c.InvokeContract(context.Background(), "", signingKey, fftypes.JSONAnyPtrBytes(locationBytes), parsedMethod, params, options, nil)
	assert.Regexp(t, "FF10457", err)
}

func TestInvokeContractConnectorError(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	httpmock.ActivateNonDefault(c.client.GetClient())
	defer httpmock.DeactivateAndReset()
	location := &Location{
		Address: "simple-tx",
	}
	options := map[string]interface{}{
		"customOption": "customValue",
	}
	signingKey := "signingKey"
	method := testFFIMethod()
	params := map[string]interface{}{
		"varString": "str",
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)

	httpmock.RegisterResponder("POST", "http://localhost:12345/api/v1/contracts/invoke", func(req *http.Request) (*http.Response, error) {
		var body map[string]interface{}
		json.NewDecoder(req.Body).Decode(&body)
		params := body["params"].([]interface{})
		assert.Equal(t, "opId", body["id"])
		assert.Equal(t, "simple-tx", body["address"])
		assert.Equal(t, "testFunc", body["method"].(map[string]interface{})["name"])
		assert.Equal(t, 1, len(params))
		assert.Equal(t, signingKey, body["from"])
		return httpmock.NewJsonResponderOrPanic(500, &common.BlockchainRESTError{
			Error:              "something went wrong",
			SubmissionRejected: true,
		})(req)
	})

	parsedMethod, err := c.ParseInterface(context.Background(), method, nil)
	assert.NoError(t, err)

	rejected, err := c.InvokeContract(context.Background(), "opId", signingKey, fftypes.JSONAnyPtrBytes(locationBytes), parsedMethod, params, options, nil)
	assert.True(t, rejected)
	assert.Regexp(t, "FF10484", err)
}

func TestQueryContractOK(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	httpmock.ActivateNonDefault(c.client.GetClient())
	defer httpmock.DeactivateAndReset()
	location := &Location{
		Address: "simple-tx",
	}
	options := map[string]interface{}{
		"customOption": "customValue",
	}
	signingKey := "signingKey"
	method := testFFIMethod()
	params := map[string]interface{}{
		"varString": "str",
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)

	httpmock.RegisterResponder("POST", "http://localhost:12345/api/v1/contracts/query", func(req *http.Request) (*http.Response, error) {
		var body map[string]interface{}
		json.NewDecoder(req.Body).Decode(&body)
		params := body["params"].([]interface{})
		assert.Equal(t, "simple-tx", body["address"])
		assert.Equal(t, "testFunc", body["method"].(map[string]interface{})["name"])
		assert.Equal(t, 1, len(params))
		assert.Equal(t, signingKey, body["from"])
		res := map[string]interface{}{
			"foo": "bar",
		}
		return httpmock.NewJsonResponderOrPanic(200, res)(req)
	})

	parsedMethod, err := c.ParseInterface(context.Background(), method, nil)
	assert.NoError(t, err)

	res, err := c.QueryContract(context.Background(), signingKey, fftypes.JSONAnyPtrBytes(locationBytes), parsedMethod, params, options)
	assert.NoError(t, err)
	assert.Equal(t, map[string]interface{}{"foo": "bar"}, res)
}

func TestQueryContractAddressNotSet(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	httpmock.ActivateNonDefault(c.client.GetClient())
	defer httpmock.DeactivateAndReset()
	location := &Location{}
	options := map[string]interface{}{
		"customOption": "customValue",
	}
	signingKey := "signingKey"
	method := testFFIMethod()
	params := map[string]interface{}{
		"varString": "str",
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)

	parsedMethod, err := c.ParseInterface(context.Background(), method, nil)
	assert.NoError(t, err)

	_, err = c.QueryContract(context.Background(), signingKey, fftypes.JSONAnyPtrBytes(locationBytes), parsedMethod, params, options)
	assert.Regexp(t, "FF10310", err)
}

func TestQueryContractBadMethod(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	httpmock.ActivateNonDefault(c.client.GetClient())
	defer httpmock.DeactivateAndReset()
	location := &Location{
		Address: "simple-tx",
	}
	options := map[string]interface{}{
		"customOption": "customValue",
	}
	signingKey := "signingKey"
	method := &fftypes.FFIMethod{}
	params := map[string]interface{}{
		"varString": "str",
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)

	parsedMethod, err := c.ParseInterface(context.Background(), method, nil)
	assert.NoError(t, err)

	_, err = c.QueryContract(context.Background(), signingKey, fftypes.JSONAnyPtrBytes(locationBytes), parsedMethod, params, options)
	assert.Regexp(t, "FF10457", err)
}

func TestQueryContractConnectorError(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	httpmock.ActivateNonDefault(c.client.GetClient())
	defer httpmock.DeactivateAndReset()
	location := &Location{
		Address: "simple-tx",
	}
	options := map[string]interface{}{
		"customOption": "customValue",
	}
	signingKey := "signingKey"
	method := testFFIMethod()
	params := map[string]interface{}{
		"varString": "str",
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)

	httpmock.RegisterResponder("POST", "http://localhost:12345/api/v1/contracts/invoke", func(req *http.Request) (*http.Response, error) {
		var body map[string]interface{}
		json.NewDecoder(req.Body).Decode(&body)
		params := body["params"].([]interface{})
		assert.Equal(t, "opId", body["id"])
		assert.Equal(t, "simple-tx", body["address"])
		assert.Equal(t, "testFunc", body["method"].(map[string]interface{})["name"])
		assert.Equal(t, 1, len(params))
		assert.Equal(t, signingKey, body["from"])
		return httpmock.NewJsonResponderOrPanic(500, &common.BlockchainRESTError{
			Error: "something went wrong",
		})(req)
	})

	parsedMethod, err := c.ParseInterface(context.Background(), method, nil)
	assert.NoError(t, err)

	_, err = c.QueryContract(context.Background(), signingKey, fftypes.JSONAnyPtrBytes(locationBytes), parsedMethod, params, options)
	assert.Regexp(t, "FF10484", err)
}

func TestQueryContractInvalidJson(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	httpmock.ActivateNonDefault(c.client.GetClient())
	defer httpmock.DeactivateAndReset()
	location := &Location{
		Address: "simple-tx",
	}
	options := map[string]interface{}{
		"customOption": "customValue",
	}
	signingKey := "signingKey"
	method := testFFIMethod()
	params := map[string]interface{}{
		"varString": "str",
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)

	httpmock.RegisterResponder("POST", "http://localhost:12345/api/v1/contracts/query", httpmock.NewStringResponder(200, "\"whoops forgot a quote"))

	parsedMethod, err := c.ParseInterface(context.Background(), method, nil)
	assert.NoError(t, err)

	_, err = c.QueryContract(context.Background(), signingKey, fftypes.JSONAnyPtrBytes(locationBytes), parsedMethod, params, options)
	assert.Error(t, err)
}

func TestDeployContractOK(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	httpmock.ActivateNonDefault(c.client.GetClient())
	defer httpmock.DeactivateAndReset()
	nsOpId := "ns1:opId"
	signingKey := "signingKey"
	definition := fftypes.JSONAnyPtr("{}")
	contract := fftypes.JSONAnyPtr("\"cafed00d\"")

	httpmock.RegisterResponder("POST", "http://localhost:12345/api/v1/contracts/deploy",
		httpmock.NewStringResponder(202, ""))

	_, err := c.DeployContract(context.Background(), nsOpId, signingKey, definition, contract, nil, nil)
	assert.NoError(t, err)
}

func TestDeployContractConnectorError(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	httpmock.ActivateNonDefault(c.client.GetClient())
	defer httpmock.DeactivateAndReset()
	nsOpId := "ns1:opId"
	signingKey := "signingKey"
	definition := fftypes.JSONAnyPtr("{}")
	contract := fftypes.JSONAnyPtr("\"cafed00d\"")

	httpmock.RegisterResponder("POST", "http://localhost:12345/api/v1/contracts/deploy",
		httpmock.NewJsonResponderOrPanic(500, &common.BlockchainRESTError{
			Error:              "oh no",
			SubmissionRejected: true,
		}))

	rejected, err := c.DeployContract(context.Background(), nsOpId, signingKey, definition, contract, nil, nil)
	assert.True(t, rejected)
	assert.Regexp(t, "FF10484", err)
}

func TestGetFFIParamValidator(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	_, err := c.GetFFIParamValidator(context.Background())
	assert.NoError(t, err)
}

func TestValidateInvokeRequest(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	err := c.ValidateInvokeRequest(context.Background(), &ffiMethodAndErrors{
		method: testFFIMethod(),
	}, nil, false)
	assert.NoError(t, err)
}

func TestValidateInvokeRequestInvalidMethod(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	err := c.ValidateInvokeRequest(context.Background(), &ffiMethodAndErrors{
		method: &fftypes.FFIMethod{},
	}, nil, false)
	assert.Regexp(t, "FF10457", err)
}

func TestGenerateFFINotSupported(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	_, err := c.GenerateFFI(context.Background(), nil)
	assert.Regexp(t, "FF10347", err)
}

func TestConvertDeprecatedContractConfig(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()

	_, _, err := c.GetAndConvertDeprecatedContractConfig(c.ctx)
	assert.NoError(t, err)
}

func TestNormalizeContractLocation(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	location := &Location{
		Address: "submit-tx",
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)
	_, err = c.NormalizeContractLocation(context.Background(), blockchain.NormalizeCall, fftypes.JSONAnyPtrBytes(locationBytes))
	assert.NoError(t, err)
}

func TestNormalizeInvalidContractLocation(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	location := fftypes.JSONAnyPtr("not valid")
	_, err := c.NormalizeContractLocation(context.Background(), blockchain.NormalizeCall, location)
	assert.Regexp(t, "10310", err)
}

func TestGenerateEventSignature(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	event := &fftypes.FFIEventDefinition{
		Name: "TransactionFinalized",
		Params: fftypes.FFIParams{
			&fftypes.FFIParam{
				Name:   "transactionId",
				Schema: fftypes.JSONAnyPtr(`{"type": "string"}`),
			},
		},
	}
	signature, err := c.GenerateEventSignature(context.Background(), event)
	assert.NoError(t, err)
	assert.Equal(t, "TransactionFinalized(string)", signature)
}

func TestGenerateEventSignatureWithLocation(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	event := &fftypes.FFIEventDefinition{
		Name: "TransactionFinalized",
		Params: fftypes.FFIParams{
			&fftypes.FFIParam{
				Name:   "transactionId",
				Schema: fftypes.JSONAnyPtr(`{"type": "string"}`),
			},
		},
	}
	location := fftypes.JSONAnyPtr(`{"address":"submit-tx"}`)
	signature, err := c.GenerateEventSignatureWithLocation(context.Background(), event, location)
	assert.NoError(t, err)
	assert.Equal(t, "submit-tx:TransactionFinalized(string)", signature)
}

func TestGenerateEventSignatureWithNilLocation(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	event := &fftypes.FFIEventDefinition{
		Name: "TransactionFinalized",
		Params: fftypes.FFIParams{
			&fftypes.FFIParam{
				Name:   "transactionId",
				Schema: fftypes.JSONAnyPtr(`{"type": "string"}`),
			},
		},
	}
	signature, err := c.GenerateEventSignatureWithLocation(context.Background(), event, nil)
	assert.NoError(t, err)
	assert.Equal(t, "*:TransactionFinalized(string)", signature)
}

func TestGenerateEventSignatureWithInvalidLocation(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	event := &fftypes.FFIEventDefinition{
		Name: "TransactionFinalized",
		Params: fftypes.FFIParams{
			&fftypes.FFIParam{
				Name:   "transactionId",
				Schema: fftypes.JSONAnyPtr(`{"type": "string"}`),
			},
		},
	}
	location := fftypes.JSONAnyPtr(`{"address":""}`)
	_, err := c.GenerateEventSignatureWithLocation(context.Background(), event, location)
	assert.Regexp(t, "FF10310", err)
}

func TestGenerateErrorSignature(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	event := &fftypes.FFIErrorDefinition{
		Name: "TransactionFailed",
		Params: fftypes.FFIParams{
			&fftypes.FFIParam{
				Name:   "transactionId",
				Schema: fftypes.JSONAnyPtr(`{"type": "string"}`),
			},
		},
	}
	signature := c.GenerateErrorSignature(context.Background(), event)
	assert.Equal(t, "TransactionFailed(string)", signature)
}

func TestCheckOverlappingLocationsEmpty(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	location := &Location{}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)
	overlapping, err := c.CheckOverlappingLocations(context.Background(), nil, fftypes.JSONAnyPtrBytes(locationBytes))
	assert.NoError(t, err)
	assert.True(t, overlapping)
}

func TestCheckOverlappingLocationsBadLocation(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	location := &Location{}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)
	_, err = c.CheckOverlappingLocations(context.Background(), fftypes.JSONAnyPtrBytes(locationBytes), fftypes.JSONAnyPtrBytes(locationBytes))
	assert.Error(t, err)
	assert.Regexp(t, "FF10310", err.Error())
}

func TestCheckOverlappingLocationsOneLocation(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	location := &Location{
		Address: "3081D84FD367044F4ED453F2024709242470388C",
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)

	location2 := &Location{}
	location2Bytes, err := json.Marshal(location2)
	assert.NoError(t, err)
	_, err = c.CheckOverlappingLocations(context.Background(), fftypes.JSONAnyPtrBytes(locationBytes), fftypes.JSONAnyPtrBytes(location2Bytes))
	assert.Error(t, err)
	assert.Regexp(t, "FF10310", err.Error())
}

func TestCheckOverlappingLocationsSameLocation(t *testing.T) {
	c, cancel := newTestCardano()
	defer cancel()
	location := &Location{
		Address: "3081D84FD367044F4ED453F2024709242470388C",
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)
	result, err := c.CheckOverlappingLocations(context.Background(), fftypes.JSONAnyPtrBytes(locationBytes), fftypes.JSONAnyPtrBytes(locationBytes))
	assert.NoError(t, err)
	assert.True(t, result)
}
