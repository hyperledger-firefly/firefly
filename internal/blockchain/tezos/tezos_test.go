// Copyright © 2025 Kaleido, Inc.
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

package tezos

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/ffresty"
	"github.com/hyperledger-firefly/common/pkg/fftls"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/retry"
	"github.com/hyperledger-firefly/common/pkg/wsclient"
	"github.com/hyperledger-firefly/firefly/internal/blockchain/common"
	"github.com/hyperledger-firefly/firefly/internal/cache"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/mocks/blockchainmocks"
	"github.com/hyperledger-firefly/firefly/mocks/cachemocks"
	"github.com/hyperledger-firefly/firefly/mocks/coremocks"
	"github.com/hyperledger-firefly/firefly/mocks/metricsmocks"
	"github.com/hyperledger-firefly/firefly/mocks/wsmocks"
	"github.com/hyperledger-firefly/firefly/pkg/blockchain"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var utConfig = config.RootSection("tezos_unit_tests")
var utTezosconnectConf = utConfig.SubSection(TezosconnectConfigKey)
var utAddressResolverConf = utConfig.SubSection(AddressResolverConfigKey)

func testFFIMethod() *fftypes.FFIMethod {
	return &fftypes.FFIMethod{
		Name: "testFunc",
		Params: []*fftypes.FFIParam{
			{
				Name:   "varNat",
				Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details":{"type":"integer","internalType":"nat"}}`),
			},
			{
				Name:   "varInt",
				Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details":{"type":"integer","internalType":"integer"}}`),
			},
			{
				Name:   "varString",
				Schema: fftypes.JSONAnyPtr(`{"type": "string", "details":{"type":"string","internalType":"string"}}`),
			},
			{
				Name:   "varStringOpt",
				Schema: fftypes.JSONAnyPtr(`{"type": "string", "details":{"type":"string","internalType":"string","kind": "option"}}`),
			},
			{
				Name:   "varBytes",
				Schema: fftypes.JSONAnyPtr(`{"type": "string", "details":{"type":"bytes","internalType":"bytes"}}`),
			},
			{
				Name:   "varBool",
				Schema: fftypes.JSONAnyPtr(`{"type": "boolean", "details":{"type":"boolean","internalType":"boolean"}}`),
			},
			{
				Name:   "varAddress",
				Schema: fftypes.JSONAnyPtr(`{"type": "string", "details":{"type":"address","internalType":"address"}}`),
			},
		},
	}
}

func resetConf(t *Tezos) {
	coreconfig.Reset()
	t.InitConfig(utConfig)
}

func newTestTezos() (*Tezos, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	wsm := &wsmocks.WSClient{}
	mm := &metricsmocks.Manager{}
	mm.On("IsMetricsEnabled").Return(true)
	mm.On("BlockchainTransaction", mock.Anything, mock.Anything).Return(nil)
	mm.On("BlockchainQuery", mock.Anything, mock.Anything).Return(nil)
	mm.On("BlockchainContractDeployment", mock.Anything, mock.Anything).Return(nil)
	t := &Tezos{
		ctx:         ctx,
		cancelCtx:   cancel,
		client:      resty.New().SetBaseURL("http://localhost:12345"),
		pluginTopic: "topic1",
		prefixShort: defaultPrefixShort,
		prefixLong:  defaultPrefixLong,
		wsconn:      wsm,
		metrics:     mm,
		cache:       cache.NewUmanagedCache(ctx, 100, 5*time.Minute),
		callbacks:   common.NewBlockchainCallbacks(),
		subs:        common.NewFireflySubscriptions(),
	}
	return t, func() {
		cancel()
		if t.closed != nil {
			// We've init'd, wait to close
			<-t.closed
		}
	}
}

func newTestStreamManager(client *resty.Client) *streamManager {
	return newStreamManager(client, cache.NewUmanagedCache(context.Background(), 100, 5*time.Minute), defaultBatchSize, defaultBatchTimeout)
}

func TestInitMissingURL(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(tz.ctx, 100, 5*time.Minute), nil)

	err := tz.Init(tz.ctx, tz.cancelCtx, utConfig, tz.metrics, cmi)
	assert.Regexp(t, "FF10138.*url", err)
}

func TestBadTLSConfig(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	resetConf(tz)
	utTezosconnectConf.Set(ffresty.HTTPConfigURL, "http://localhost:12345")

	tlsConf := utTezosconnectConf.SubSection("tls")
	tlsConf.Set(fftls.HTTPConfTLSEnabled, true)
	tlsConf.Set(fftls.HTTPConfTLSCAFile, "!!!!!badness")

	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(tz.ctx, 100, 5*time.Minute), nil)

	err := tz.Init(tz.ctx, tz.cancelCtx, utConfig, tz.metrics, cmi)
	assert.Regexp(t, "FF00153", err)
}

func TestInitBadAddressResolver(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	resetConf(tz)
	utAddressResolverConf.Set(AddressResolverURLTemplate, "{{unclosed}")

	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(tz.ctx, 100, 5*time.Minute), nil)

	err := tz.Init(tz.ctx, tz.cancelCtx, utConfig, tz.metrics, cmi)
	assert.Regexp(t, "FF10337.*urlTemplate", err)
}

func TestInitMissingTopic(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	resetConf(tz)
	utTezosconnectConf.Set(ffresty.HTTPConfigURL, "http://localhost:12345")

	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(tz.ctx, 100, 5*time.Minute), nil)

	err := tz.Init(tz.ctx, tz.cancelCtx, utConfig, tz.metrics, cmi)
	assert.Regexp(t, "FF10138.*topic", err)
}

func TestInitAndStartWithTezosConnect(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	toServer, fromServer, wsURL, done := wsclient.NewTestWSServer(nil)
	defer done()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	u, _ := url.Parse(wsURL)
	u.Scheme = "http"
	httpURL := u.String()

	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/eventstreams", httpURL),
		httpmock.NewJsonResponderOrPanic(200, []eventStream{}))
	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/eventstreams", httpURL),
		httpmock.NewJsonResponderOrPanic(200, eventStream{ID: "es12345"}))

	resetConf(tz)
	utTezosconnectConf.Set(ffresty.HTTPConfigURL, httpURL)
	utTezosconnectConf.Set(ffresty.HTTPCustomClient, mockedClient)
	utTezosconnectConf.Set(TezosconnectConfigTopic, "topic1")

	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(tz.ctx, 100, 5*time.Minute), nil)
	err := tz.Init(tz.ctx, tz.cancelCtx, utConfig, tz.metrics, cmi)
	assert.NoError(t, err)

	assert.Equal(t, "tezos", tz.Name())
	assert.Equal(t, core.VerifierTypeTezosAddress, tz.VerifierType())

	assert.NoError(t, err)

	assert.Equal(t, 2, httpmock.GetTotalCallCount())
	assert.Equal(t, "es12345", tz.streamID)
	assert.NotNil(t, tz.Capabilities())

	err = tz.Start()
	assert.NoError(t, err)

	startupMessage := <-toServer
	assert.Equal(t, `{"type":"listen","topic":"topic1"}`, startupMessage)
	startupMessage = <-toServer
	assert.Equal(t, `{"type":"listenreplies"}`, startupMessage)
	fromServer <- `{"bad":"receipt"}` // will be ignored - no ack
	fromServer <- `[]`                // empty batch, will be ignored, but acked
	reply := <-toServer
	assert.Equal(t, `{"type":"ack","topic":"topic1"}`, reply)
	fromServer <- `[{}]` // bad batch

	// Bad data will be ignored
	fromServer <- `!json`
	fromServer <- `{"not": "a reply"}`
	fromServer <- `42`
}

func TestBackgroundStart(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	toServer, fromServer, wsURL, done := wsclient.NewTestWSServer(nil)
	defer done()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	u, _ := url.Parse(wsURL)
	u.Scheme = "http"
	httpURL := u.String()

	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/eventstreams", httpURL),
		httpmock.NewJsonResponderOrPanic(200, []eventStream{}))
	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/eventstreams", httpURL),
		httpmock.NewJsonResponderOrPanic(200, eventStream{ID: "es12345"}))

	resetConf(tz)
	utTezosconnectConf.Set(ffresty.HTTPConfigURL, httpURL)
	utTezosconnectConf.Set(ffresty.HTTPCustomClient, mockedClient)
	utTezosconnectConf.Set(TezosconnectConfigTopic, "topic1")
	utTezosconnectConf.Set(TezosconnectBackgroundStart, true)

	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(tz.ctx, 100, 5*time.Minute), nil)
	err := tz.Init(tz.ctx, tz.cancelCtx, utConfig, tz.metrics, cmi)
	assert.NoError(t, err)

	assert.Equal(t, "tezos", tz.Name())
	assert.Equal(t, core.VerifierTypeTezosAddress, tz.VerifierType())

	assert.NoError(t, err)

	assert.NotNil(t, tz.Capabilities())

	err = tz.Start()
	assert.NoError(t, err)

	assert.Eventually(t, func() bool { return httpmock.GetTotalCallCount() == 2 }, time.Second*5, time.Microsecond)
	assert.Eventually(t, func() bool { return tz.streamID == "es12345" }, time.Second*5, time.Microsecond)

	startupMessage := <-toServer
	assert.Equal(t, `{"type":"listen","topic":"topic1"}`, startupMessage)
	startupMessage = <-toServer
	assert.Equal(t, `{"type":"listenreplies"}`, startupMessage)
	fromServer <- `[]` // empty batch, will be ignored, but acked
	reply := <-toServer
	assert.Equal(t, `{"type":"ack","topic":"topic1"}`, reply)

	// Bad data will be ignored
	fromServer <- `!json`
	fromServer <- `{"not": "a reply"}`
	fromServer <- `42`
}

func TestBackgroundStartFail(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	_, _, wsURL, done := wsclient.NewTestWSServer(nil)
	defer done()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	u, _ := url.Parse(wsURL)
	u.Scheme = "http"
	httpURL := u.String()

	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/eventstreams", httpURL),
		httpmock.NewJsonResponderOrPanic(500, "Failed to get eventstreams"))

	resetConf(tz)
	utTezosconnectConf.Set(ffresty.HTTPConfigURL, httpURL)
	utTezosconnectConf.Set(ffresty.HTTPCustomClient, mockedClient)
	utTezosconnectConf.Set(TezosconnectConfigTopic, "topic1")
	utTezosconnectConf.Set(TezosconnectBackgroundStart, true)

	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(tz.ctx, 100, 5*time.Minute), nil)
	err := tz.Init(tz.ctx, tz.cancelCtx, utConfig, tz.metrics, cmi)
	assert.NoError(t, err)

	assert.Equal(t, "tezos", tz.Name())
	assert.Equal(t, core.VerifierTypeTezosAddress, tz.VerifierType())

	assert.NoError(t, err)

	err = tz.Start()
	assert.NoError(t, err)

	capturedErr := make(chan error)
	tz.backgroundRetry = &retry.Retry{
		ErrCallback: func(err error) {
			capturedErr <- err
		},
	}

	err = tz.Start()
	assert.NoError(t, err)

	err = <-capturedErr
	assert.Regexp(t, "FF10283", err)
}

func TestBackgroundStartWSFail(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	u, _ := url.Parse("http://localhost:12345")
	u.Scheme = "http"
	httpURL := u.String()

	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/eventstreams", httpURL),
		httpmock.NewJsonResponderOrPanic(200, []eventStream{}))
	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/eventstreams", httpURL),
		httpmock.NewJsonResponderOrPanic(200, eventStream{ID: "es12345"}))

	resetConf(tz)
	utTezosconnectConf.Set(ffresty.HTTPConfigURL, httpURL)
	utTezosconnectConf.Set(ffresty.HTTPCustomClient, mockedClient)
	utTezosconnectConf.Set(TezosconnectConfigTopic, "topic1")
	utTezosconnectConf.Set(TezosconnectBackgroundStart, true)
	utTezosconnectConf.Set(wsclient.WSConfigKeyInitialConnectAttempts, 1)

	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(tz.ctx, 100, 5*time.Minute), nil)
	originalContext := tz.ctx
	err := tz.Init(tz.ctx, tz.cancelCtx, utConfig, &metricsmocks.Manager{}, cmi)
	cmi.AssertCalled(t, "GetCache", cache.NewCacheConfig(
		originalContext,
		coreconfig.CacheBlockchainLimit,
		coreconfig.CacheBlockchainTTL,
		"",
	))
	assert.NoError(t, err)

	capturedErr := make(chan error)
	tz.backgroundRetry = &retry.Retry{
		ErrCallback: func(err error) {
			capturedErr <- err
		},
	}

	err = tz.Start()
	assert.NoError(t, err)

	err = <-capturedErr
	assert.Regexp(t, "FF00148", err)
}

func TestWSInitFail(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	resetConf(tz)
	utTezosconnectConf.Set(ffresty.HTTPConfigURL, "!!!://")
	utTezosconnectConf.Set(TezosconnectConfigTopic, "topic1")

	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(tz.ctx, 100, 5*time.Minute), nil)
	err := tz.Init(tz.ctx, tz.cancelCtx, utConfig, tz.metrics, cmi)
	assert.Regexp(t, "FF00149", err)
}

func TestTezosCacheInitFail(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	resetConf(tz)
	utTezosconnectConf.Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	utTezosconnectConf.Set(ffresty.HTTPCustomClient, mockedClient)
	utTezosconnectConf.Set(TezosconnectConfigTopic, "topic1")

	cmi := &cachemocks.Manager{}
	cacheInitError := errors.New("Initialization error.")
	cmi.On("GetCache", mock.Anything).Return(nil, cacheInitError)

	err := tz.Init(tz.ctx, tz.cancelCtx, utConfig, tz.metrics, cmi)
	assert.Equal(t, cacheInitError, err)
}

func TestStreamQueryError(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", "http://localhost:12345/eventstreams",
		httpmock.NewStringResponder(500, `pop`))

	resetConf(tz)
	utTezosconnectConf.Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	utTezosconnectConf.Set(ffresty.HTTPConfigRetryEnabled, false)
	utTezosconnectConf.Set(ffresty.HTTPCustomClient, mockedClient)
	utTezosconnectConf.Set(TezosconnectConfigTopic, "topic1")

	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(tz.ctx, 100, 5*time.Minute), nil)
	err := tz.Init(tz.ctx, tz.cancelCtx, utConfig, tz.metrics, cmi)
	assert.Regexp(t, "FF10283.*pop", err)
}

func TestStreamCreateError(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", "http://localhost:12345/eventstreams",
		httpmock.NewJsonResponderOrPanic(200, []eventStream{}))
	httpmock.RegisterResponder("POST", "http://localhost:12345/eventstreams",
		httpmock.NewStringResponder(500, `pop`))

	resetConf(tz)
	utTezosconnectConf.Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	utTezosconnectConf.Set(ffresty.HTTPConfigRetryEnabled, false)
	utTezosconnectConf.Set(ffresty.HTTPCustomClient, mockedClient)
	utTezosconnectConf.Set(TezosconnectConfigTopic, "topic1")

	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(tz.ctx, 100, 5*time.Minute), nil)
	err := tz.Init(tz.ctx, tz.cancelCtx, utConfig, tz.metrics, cmi)
	assert.Regexp(t, "FF10283.*pop", err)
}

func TestStreamUpdateError(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", "http://localhost:12345/eventstreams",
		httpmock.NewJsonResponderOrPanic(200, []eventStream{{ID: "es12345", Name: "topic1"}}))
	httpmock.RegisterResponder("PATCH", "http://localhost:12345/eventstreams/es12345",
		httpmock.NewStringResponder(500, `pop`))

	resetConf(tz)
	utTezosconnectConf.Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	utTezosconnectConf.Set(ffresty.HTTPConfigRetryEnabled, false)
	utTezosconnectConf.Set(ffresty.HTTPCustomClient, mockedClient)
	utTezosconnectConf.Set(TezosconnectConfigTopic, "topic1")

	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(tz.ctx, 100, 5*time.Minute), nil)
	err := tz.Init(tz.ctx, tz.cancelCtx, utConfig, tz.metrics, cmi)
	assert.Regexp(t, "FF10283.*pop", err)
}

func TestInitAllExistingStreams(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", "http://localhost:12345/eventstreams",
		httpmock.NewJsonResponderOrPanic(200, []eventStream{{ID: "es12345", Name: "topic1"}}))
	httpmock.RegisterResponder("GET", "http://localhost:12345/subscriptions",
		httpmock.NewJsonResponderOrPanic(200, []subscription{
			{ID: "sub12345", Stream: "es12345", Name: "ns1_BatchPin_4b5431436f737675"},
		}))
	httpmock.RegisterResponder("PATCH", "http://localhost:12345/eventstreams/es12345",
		httpmock.NewJsonResponderOrPanic(200, &eventStream{ID: "es12345", Name: "topic1"}))
	httpmock.RegisterResponder("POST", "http://localhost:12345/subscriptions",
		httpmock.NewJsonResponderOrPanic(200, subscription{}))

	resetConf(tz)
	utTezosconnectConf.Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	utTezosconnectConf.Set(ffresty.HTTPCustomClient, mockedClient)
	utTezosconnectConf.Set(TezosconnectConfigTopic, "topic1")

	location := fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "KT1CosvuPHD6YnY4uYNguJj6m58UuHJWyS1u",
	}.String())
	contract := &blockchain.MultipartyContract{
		Location:   location,
		FirstEvent: "oldest",
	}

	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(tz.ctx, 100, 5*time.Minute), nil)

	err := tz.Init(tz.ctx, tz.cancelCtx, utConfig, tz.metrics, cmi)
	assert.NoError(t, err)

	ns := &core.Namespace{Name: "ns1", NetworkName: "ns1"}
	_, err = tz.AddFireflySubscription(tz.ctx, ns, contract, "")
	assert.NoError(t, err)

	assert.Equal(t, 3, httpmock.GetTotalCallCount())
	assert.Equal(t, "es12345", tz.streamID)
}

func TestVerifyTezosAddress(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	_, err := tz.ResolveSigningKey(context.Background(), "", blockchain.ResolveKeyIntentSign)
	assert.Regexp(t, "FF10354", err)

	_, err = tz.ResolveSigningKey(context.Background(), "tz1err", blockchain.ResolveKeyIntentSign)
	assert.Regexp(t, "FF10142", err)

	key, err := tz.ResolveSigningKey(context.Background(), "tz1Y6GnVhC4EpcDDSmD3ibcC4WX6DJ4Q1QLN", blockchain.ResolveKeyIntentSign)
	assert.NoError(t, err)
	assert.Equal(t, "tz1Y6GnVhC4EpcDDSmD3ibcC4WX6DJ4Q1QLN", key)
}

func TestEventLoopContextCancelled(t *testing.T) {
	tz, cancel := newTestTezos()
	cancel()
	r := make(<-chan []byte)
	wsm := tz.wsconn.(*wsmocks.WSClient)
	wsm.On("Receive").Return(r)
	wsm.On("Close").Return()
	tz.closed = make(chan struct{})
	tz.eventLoop()
	wsm.AssertExpectations(t)
}

func TestEventLoopReceiveClosed(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	r := make(chan []byte)
	wsm := tz.wsconn.(*wsmocks.WSClient)
	close(r)
	wsm.On("Receive").Return((<-chan []byte)(r))
	wsm.On("Close").Return()
	tz.closed = make(chan struct{})
	tz.eventLoop()
	wsm.AssertExpectations(t)
}

func TestEventLoopSendClosed(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	s := make(chan []byte, 1)
	s <- []byte(`[]`)
	r := make(chan []byte)
	wsm := tz.wsconn.(*wsmocks.WSClient)
	wsm.On("Receive").Return((<-chan []byte)(s))
	wsm.On("Send", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		go cancel()
		close(r)
	}).Return(fmt.Errorf("pop"))
	wsm.On("Close").Return()
	tz.closed = make(chan struct{})
	tz.eventLoop()
	wsm.AssertExpectations(t)
}

func TestEventLoopUnexpectedMessage(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	r := make(chan []byte)
	wsm := tz.wsconn.(*wsmocks.WSClient)
	wsm.On("Receive").Return((<-chan []byte)(r))
	wsm.On("Close").Return()
	tz.closed = make(chan struct{})
	operationID := fftypes.NewUUID()
	data := []byte(`{
		"_id": "6fb94fff-81d3-4094-567d-e031b1871694",
		"errorMessage": "Packing arguments for method 'broadcastBatch': cannot use [3]uint8 as type [32]uint8 as argument",
		"headers": {
			"id": "3a37b17b-13b6-4dc5-647a-07c11eae0be3",
			"requestId": "ns1:` + operationID.String() + `",
			"requestOffset": "zzn4y4v4si-zzjjepe9x4-requests:0:0",
			"timeElapsed": 0.020969053,
			"timeReceived": "2021-05-31T02:35:11.458880504Z",
			"type": "Error"
		},
		"receivedAt": 1622428511616
	}`)
	em := &blockchainmocks.Callbacks{}
	tz.SetHandler("ns1", em)
	txsu := em.On("BlockchainOpUpdate",
		tz,
		"ns1:"+operationID.String(),
		core.OpStatusFailed,
		"",
		"Packing arguments for method 'broadcastBatch': cannot use [3]uint8 as type [32]uint8 as argument",
		mock.Anything).Return(fmt.Errorf("Shutdown"))
	done := make(chan struct{})
	txsu.RunFn = func(a mock.Arguments) {
		close(done)
	}

	go tz.eventLoop()
	r <- []byte(`!badjson`)        // ignored bad json
	r <- []byte(`"not an object"`) // ignored wrong type
	r <- data
	tz.ctx.Done()
}

func TestHandleReceiptTXSuccess(t *testing.T) {
	tm := &coremocks.OperationCallbacks{}
	wsm := &wsmocks.WSClient{}
	tz := &Tezos{
		ctx:         context.Background(),
		pluginTopic: "topic1",
		callbacks:   common.NewBlockchainCallbacks(),
		wsconn:      wsm,
	}
	tz.SetOperationHandler("ns1", tm)

	var reply common.BlockchainReceiptNotification
	operationID := fftypes.NewUUID()
	data := fftypes.JSONAnyPtr(`{
		"headers": {
			"requestId": "ns1:` + operationID.String() + `",
			"type": "TransactionSuccess"
		},
		"status": "Succeeded",
		"protocolId": "PtNairobiyssHuh87hEhfVBGCVrK3WnS8Z2FT4ymB5tAa4r1nQf",
		"transactionHash": "ooGcrcazgcGBrY1iym329ovV13MnWrTmV1fttCwWKH5DiYUQsiq",
		"contractLocation": {
			"address": "KT1CosvuPHD6YnY4uYNguJj6m58UuHJWyS1u"
		}
	}`)

	tm.On("OperationUpdate", mock.MatchedBy(func(update *core.OperationUpdateAsync) bool {
		return update.NamespacedOpID == "ns1:"+operationID.String() &&
			update.Status == core.OpStatusSucceeded &&
			update.BlockchainTXID == "ooGcrcazgcGBrY1iym329ovV13MnWrTmV1fttCwWKH5DiYUQsiq" &&
			update.Plugin == "tezos"
	})).Return(nil)

	err := json.Unmarshal(data.Bytes(), &reply)
	assert.NoError(t, err)

	common.HandleReceipt(context.Background(), "", tz, &reply, tz.callbacks)

	tm.AssertExpectations(t)
}

func TestHandleReceiptTXUpdateTezosConnect(t *testing.T) {
	tm := &coremocks.OperationCallbacks{}
	wsm := &wsmocks.WSClient{}
	tz := &Tezos{
		ctx:         context.Background(),
		pluginTopic: "topic1",
		callbacks:   common.NewBlockchainCallbacks(),
		wsconn:      wsm,
	}
	tz.SetOperationHandler("ns1", tm)

	var reply common.BlockchainReceiptNotification
	operationID := fftypes.NewUUID()
	data := fftypes.JSONAnyPtr(`{
		"created": "2023-09-10T14:49:31.147376Z",
		"firstSubmit": "2023-09-10T14:49:31.79751Z",
		"from": "tz1eXM1uGi5THR7Aj8VnkteA5nrBmPyKAufM",
		"gasPrice": 0,
		"headers": {
            "requestId": "ns1:` + operationID.String() + `",
            "type": "TransactionUpdate"
        },
		"id": "ns1:` + operationID.String() + `",
		"lastSubmit": "2023-09-10T14:49:31.79751Z",
		"nonce": "1",
		"policyInfo": {},
		"receipt": {
			"blockHash": "BKp3gNDyJygbAKNmdJnmJjLX6y6BrA2dcJVXJ3zMfkX8gA63rH3",
			"blockNumber": "3835591",
			"contractLocation": {
				"address": "KT1CosvuPHD6YnY4uYNguJj6m58UuHJWyS1u"
			},
			"extraInfo": {
				"consumedGas": "387",
				"contractAddress": "KT1CosvuPHD6YnY4uYNguJj6m58UuHJWyS1u",
				"counter": "18602182",
				"errorMessage": null,
				"fee": "313",
				"from": "tz1eXM1uGi5THR7Aj8VnkteA5nrBmPyKAufM",
				"gasLimit": "487",
				"paidStorageSizeDiff": "0",
				"status": "applied",
				"storage": {
					"admin": "tz1eXM1uGi5THR7Aj8VnkteA5nrBmPyKAufM",
					"destroyed": false,
					"last_token_id": "1",
					"paused": false
				},
				"storageLimit": "0",
				"storageSize": "10380",
				"to": "KT1CosvuPHD6YnY4uYNguJj6m58UuHJWyS1u"
			},
			"protocolId": "PtNairobiyssHuh87hEhfVBGCVrK3WnS8Z2FT4ymB5tAa4r1nQf",
			"success": true,
			"transactionIndex": "0"
		},
		"sequenceId": "018a7f91-b90b-fb45-9f7d-0956b3280c1d",
		"status": "Succeeded",
		"to": "KT1CosvuPHD6YnY4uYNguJj6m58UuHJWyS1u",
		"transactionData": "ff0003d4072e3ece4cbbda5b3f8b0c5b6567520ad5eab21d71ff27595f735cba6c00cf26c62d8a29ab128972a52c46bb6099a2d3675900c6b1ef08000000012e5b393218d67f74660c2cedec8e8bcfa9607d8100ffff057061757365000000020303",
		"transactionHash": "onhZJDmz5JihnW1RaZ96f17FgUBv3GoERkRECK3XVFt1kL5E6Yy",
		"transactionHeaders": {
			"from": "tz1eXM1uGi5THR7Aj8VnkteA5nrBmPyKAufM",
			"nonce": "1",
			"to": "KT1CosvuPHD6YnY4uYNguJj6m58UuHJWyS1u"
		},
		"updated": "2023-09-10T14:49:36.030604Z"
	}`)

	tm.On("OperationUpdate", mock.MatchedBy(func(update *core.OperationUpdateAsync) bool {
		return update.NamespacedOpID == "ns1:"+operationID.String() &&
			update.Status == core.OpStatusPending &&
			update.BlockchainTXID == "onhZJDmz5JihnW1RaZ96f17FgUBv3GoERkRECK3XVFt1kL5E6Yy" &&
			update.Plugin == "tezos"
	})).Return(nil)

	err := json.Unmarshal(data.Bytes(), &reply)
	assert.NoError(t, err)
	expectedReceiptId := "ns1:" + operationID.String()
	assert.Equal(t, reply.Headers.ReceiptID, expectedReceiptId)
	common.HandleReceipt(context.Background(), "", tz, &reply, tz.callbacks)

	tm.AssertExpectations(t)
}

func TestHandleMsgBatchBadData(t *testing.T) {
	wsm := &wsmocks.WSClient{}
	tz := &Tezos{
		ctx:         context.Background(),
		pluginTopic: "topic1",
		wsconn:      wsm,
	}

	var reply common.BlockchainReceiptNotification
	data := fftypes.JSONAnyPtr(`{}`)
	err := json.Unmarshal(data.Bytes(), &reply)
	assert.NoError(t, err)
	common.HandleReceipt(context.Background(), "", tz, &reply, tz.callbacks)
}

func TestAddSubscription(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	httpmock.ActivateNonDefault(tz.client.GetClient())
	defer httpmock.DeactivateAndReset()
	tz.streamID = "es-1"
	tz.streams = &streamManager{
		client: tz.client,
	}

	sub := &core.ContractListener{
		Filters: []*core.ListenerFilter{
			{
				Event: &core.FFISerializedEvent{
					FFIEventDefinition: fftypes.FFIEventDefinition{
						Name: "Changed",
					},
				},
				Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
					"address": "KT123",
				}.String()),
			},
		},
		Options: &core.ContractListenerOptions{
			FirstEvent: string(core.SubOptsFirstEventOldest),
		},
	}

	httpmock.RegisterResponder("POST", `http://localhost:12345/subscriptions`,
		httpmock.NewJsonResponderOrPanic(200, &subscription{}))

	err := tz.AddContractListener(context.Background(), sub, "")

	assert.NoError(t, err)
}

func TestAddSubscriptionWithoutLocation(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	httpmock.ActivateNonDefault(tz.client.GetClient())
	defer httpmock.DeactivateAndReset()
	tz.streamID = "es-1"
	tz.streams = &streamManager{
		client: tz.client,
	}

	sub := &core.ContractListener{
		Filters: []*core.ListenerFilter{
			{
				Event: &core.FFISerializedEvent{
					FFIEventDefinition: fftypes.FFIEventDefinition{
						Name: "Changed",
					},
				},
			},
		},
		Options: &core.ContractListenerOptions{
			FirstEvent: string(core.SubOptsFirstEventNewest),
		},
	}

	httpmock.RegisterResponder("POST", `http://localhost:12345/subscriptions`,
		httpmock.NewJsonResponderOrPanic(200, &subscription{}))

	err := tz.AddContractListener(context.Background(), sub, "")

	assert.NoError(t, err)
}

func TestAddSubscriptionBadLocation(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	httpmock.ActivateNonDefault(tz.client.GetClient())
	defer httpmock.DeactivateAndReset()

	tz.streamID = "es-1"
	tz.streams = &streamManager{
		client: tz.client,
	}

	sub := &core.ContractListener{
		Filters: core.ListenerFilters{
			{
				Location: fftypes.JSONAnyPtr(""),
				Event:    &core.FFISerializedEvent{},
			},
		},
	}

	err := tz.AddContractListener(context.Background(), sub, "")
	assert.Regexp(t, "FF10310", err)
}

func TestAddSubscriptionFail(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	httpmock.ActivateNonDefault(tz.client.GetClient())
	defer httpmock.DeactivateAndReset()

	tz.streamID = "es-1"
	tz.streams = &streamManager{
		client: tz.client,
	}

	sub := &core.ContractListener{
		Filters: core.ListenerFilters{
			{
				Event: &core.FFISerializedEvent{},
				Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
					"address": "KT123",
				}.String()),
			},
		},
		Options: &core.ContractListenerOptions{
			FirstEvent: string(core.SubOptsFirstEventNewest),
		},
	}

	httpmock.RegisterResponder("POST", `http://localhost:12345/subscriptions`,
		httpmock.NewStringResponder(500, "pop"))

	err := tz.AddContractListener(context.Background(), sub, "")

	assert.Regexp(t, "FF10283.*pop", err)
}

func TestAddSubscriptionNoFiltersFail(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	tz.streamID = "es-1"
	tz.streams = &streamManager{
		client: tz.client,
	}

	sub := &core.ContractListener{
		Options: &core.ContractListenerOptions{
			FirstEvent: string(core.SubOptsFirstEventNewest),
		},
	}

	err := tz.AddContractListener(context.Background(), sub, "")

	assert.Regexp(t, "FF10475", err)
}

func TestAddSubscriptionTwoManyFiltersFail(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	tz.streamID = "es-1"
	tz.streams = &streamManager{
		client: tz.client,
	}

	sub := &core.ContractListener{
		Filters: core.ListenerFilters{
			{
				Event: &core.FFISerializedEvent{},
				Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
					"address": "KT123",
				}.String()),
			},
			{
				Event: &core.FFISerializedEvent{},
				Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
					"address": "KT123",
				}.String()),
			},
		},
		Options: &core.ContractListenerOptions{
			FirstEvent: string(core.SubOptsFirstEventNewest),
		},
	}

	err := tz.AddContractListener(context.Background(), sub, "")

	assert.Regexp(t, "FF10476", err)
}

func TestDeleteSubscription(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	httpmock.ActivateNonDefault(tz.client.GetClient())
	defer httpmock.DeactivateAndReset()

	tz.streamID = "es-1"
	tz.streams = &streamManager{
		client: tz.client,
	}

	sub := &core.ContractListener{
		BackendID: "sb-1",
	}

	httpmock.RegisterResponder("DELETE", `http://localhost:12345/subscriptions/sb-1`,
		httpmock.NewStringResponder(204, ""))

	err := tz.DeleteContractListener(context.Background(), sub, true)
	assert.NoError(t, err)
}

func TestDeleteSubscriptionFail(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	httpmock.ActivateNonDefault(tz.client.GetClient())
	defer httpmock.DeactivateAndReset()

	tz.streamID = "es-1"
	tz.streams = &streamManager{
		client: tz.client,
	}

	sub := &core.ContractListener{
		BackendID: "sb-1",
	}

	httpmock.RegisterResponder("DELETE", `http://localhost:12345/subscriptions/sb-1`,
		httpmock.NewStringResponder(500, ""))

	err := tz.DeleteContractListener(context.Background(), sub, true)
	assert.Regexp(t, "FF10283", err)
}

func TestDeleteSubscriptionNotFound(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	httpmock.ActivateNonDefault(tz.client.GetClient())
	defer httpmock.DeactivateAndReset()

	tz.streamID = "es-1"
	tz.streams = &streamManager{
		client: tz.client,
	}

	sub := &core.ContractListener{
		BackendID: "sb-1",
	}

	httpmock.RegisterResponder("DELETE", `http://localhost:12345/subscriptions/sb-1`,
		httpmock.NewStringResponder(404, ""))

	err := tz.DeleteContractListener(context.Background(), sub, true)
	assert.NoError(t, err)
}

func TestDeployContractOK(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	httpmock.ActivateNonDefault(tz.client.GetClient())
	defer httpmock.DeactivateAndReset()
	signingKey := "tz1Y6GnVhC4EpcDDSmD3ibcC4WX6DJ4Q1QLN"
	input := []interface{}{}
	options := map[string]interface{}{}
	definitionBytes, err := json.Marshal([]interface{}{})
	contract := "{\"code\":[{\"args\":[{\"prim\":\"string\"}],\"prim\":\"parameter\"},{\"args\":[{\"prim\":\"string\"}],\"prim\":\"storage\"},{\"args\":[[{\"prim\":\"CAR\"},{\"args\":[{\"prim\":\"operation\"}],\"prim\":\"NIL\"},{\"prim\":\"PAIR\"}]],\"prim\":\"code\"}],\"storage\":{\"string\":\"hello\"}}"
	contractBytes, err := json.Marshal(contract)
	assert.NoError(t, err)
	httpmock.RegisterResponder("POST", `http://localhost:12345/`,
		func(req *http.Request) (*http.Response, error) {
			var body map[string]interface{}
			json.NewDecoder(req.Body).Decode(&body)
			headers := body["headers"].(map[string]interface{})
			assert.Equal(t, core.DeployContract, headers["type"])
			assert.Equal(t, "123", headers["id"])
			assert.Equal(t, contract, body["contract"])
			return httpmock.NewJsonResponderOrPanic(200, "")(req)
		})

	_, err = tz.DeployContract(context.Background(), "123", signingKey, fftypes.JSONAnyPtrBytes(definitionBytes), fftypes.JSONAnyPtrBytes(contractBytes), input, options)

	assert.NoError(t, err)
}

func TestDeployContractError(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	httpmock.ActivateNonDefault(tz.client.GetClient())
	defer httpmock.DeactivateAndReset()
	signingKey := "tz1Y6GnVhC4EpcDDSmD3ibcC4WX6DJ4Q1QLN"
	input := []interface{}{}
	options := map[string]interface{}{}
	definitionBytes, err := json.Marshal([]interface{}{})
	contract := "{\"code\":[{\"args\":[{\"prim\":\"string\"}],\"prim\":\"parameter\"},{\"args\":[{\"prim\":\"string\"}],\"prim\":\"storage\"},{\"args\":[[{\"prim\":\"CAR\"},{\"args\":[{\"prim\":\"operation\"}],\"prim\":\"NIL\"},{\"prim\":\"PAIR\"}]],\"prim\":\"code\"}],\"storage\":{\"string\":\"hello\"}}"
	contractBytes, err := json.Marshal(contract)
	assert.NoError(t, err)
	httpmock.RegisterResponder("POST", `http://localhost:12345/`,
		func(req *http.Request) (*http.Response, error) {
			var body map[string]interface{}
			json.NewDecoder(req.Body).Decode(&body)
			headers := body["headers"].(map[string]interface{})
			assert.Equal(t, "DeployContract", headers["type"])
			assert.Equal(t, "123", headers["id"])
			assert.Equal(t, contract, body["contract"])
			return httpmock.NewJsonResponderOrPanic(400, "error")(req)
		})

	_, err = tz.DeployContract(context.Background(), "123", signingKey, fftypes.JSONAnyPtrBytes(definitionBytes), fftypes.JSONAnyPtrBytes(contractBytes), input, options)

	assert.Regexp(t, "FF10283", err)
}

func TestDeployContractInvalidOption(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	httpmock.ActivateNonDefault(tz.client.GetClient())
	defer httpmock.DeactivateAndReset()
	signingKey := "tz1Y6GnVhC4EpcDDSmD3ibcC4WX6DJ4Q1QLN"
	input := []interface{}{}
	options := map[string]interface{}{
		"contract": "shouldn't be allowed",
	}
	definitionBytes, err := json.Marshal([]interface{}{})
	contract := "{\"code\":[{\"args\":[{\"prim\":\"string\"}],\"prim\":\"parameter\"},{\"args\":[{\"prim\":\"string\"}],\"prim\":\"storage\"},{\"args\":[[{\"prim\":\"CAR\"},{\"args\":[{\"prim\":\"operation\"}],\"prim\":\"NIL\"},{\"prim\":\"PAIR\"}]],\"prim\":\"code\"}],\"storage\":{\"string\":\"hello\"}}"
	contractBytes, err := json.Marshal(contract)
	assert.NoError(t, err)
	httpmock.RegisterResponder("POST", `http://localhost:12345/`,
		func(req *http.Request) (*http.Response, error) {
			var body map[string]interface{}
			json.NewDecoder(req.Body).Decode(&body)
			headers := body["headers"].(map[string]interface{})
			assert.Equal(t, core.DeployContract, headers["type"])
			assert.Equal(t, "123", headers["id"])
			assert.Equal(t, contract, body["contract"])
			return httpmock.NewJsonResponderOrPanic(200, "")(req)
		})

	_, err = tz.DeployContract(context.Background(), "123", signingKey, fftypes.JSONAnyPtrBytes(definitionBytes), fftypes.JSONAnyPtrBytes(contractBytes), input, options)

	assert.Regexp(t, "FF10398", err)
}

func TestInvokeContractOK(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	httpmock.ActivateNonDefault(tz.client.GetClient())
	defer httpmock.DeactivateAndReset()
	signingKey := "tz1Y6GnVhC4EpcDDSmD3ibcC4WX6DJ4Q1QLN"
	location := &Location{
		Address: "KT12345",
	}
	options := map[string]interface{}{
		"customOption": "customValue",
	}
	method := testFFIMethod()
	params := map[string]interface{}{
		"varNat":       float64(1),
		"varInt":       float64(2),
		"varString":    "str",
		"varStringOpt": "optional str",
		"varBytes":     "0xAA",
		"varBool":      true,
		"varAddress":   "tz1Y6GnVhC4EpcDDSmD3ibcC4WX6DJ4Q1QLN",
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)

	httpmock.RegisterResponder("POST", `http://localhost:12345/`,
		func(req *http.Request) (*http.Response, error) {
			var body map[string]interface{}
			json.NewDecoder(req.Body).Decode(&body)
			params := body["params"].([]interface{})
			michelineParams := params[0].(map[string]interface{})
			headers := body["headers"].(map[string]interface{})
			assert.Equal(t, "SendTransaction", headers["type"])
			assert.Equal(t, "testFunc", michelineParams["entrypoint"].(string))
			assert.Equal(t, 7, len(michelineParams["value"].([]interface{})))
			assert.Equal(t, body["customOption"].(string), "customValue")
			return httpmock.NewJsonResponderOrPanic(200, "")(req)
		})

	parsedMethod, err := tz.ParseInterface(context.Background(), method, nil)
	assert.NoError(t, err)

	_, err = tz.InvokeContract(context.Background(), "opID", signingKey, fftypes.JSONAnyPtrBytes(locationBytes), parsedMethod, params, options, nil)
	assert.NoError(t, err)
}

func TestInvokeContractInvalidOption(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	httpmock.ActivateNonDefault(tz.client.GetClient())
	defer httpmock.DeactivateAndReset()
	signingKey := "tz1Y6GnVhC4EpcDDSmD3ibcC4WX6DJ4Q1QLN"
	location := &Location{
		Address: "KT12345",
	}
	options := map[string]interface{}{
		"params": "shouldn't be allowed",
	}
	method := testFFIMethod()
	params := map[string]interface{}{
		"varNat":       float64(1),
		"varInt":       float64(2),
		"varString":    "str",
		"varStringOpt": "optional str",
		"varBytes":     "0xAA",
		"varBool":      true,
		"varAddress":   "tz1Y6GnVhC4EpcDDSmD3ibcC4WX6DJ4Q1QLN",
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)

	httpmock.RegisterResponder("POST", `http://localhost:12345/`,
		func(req *http.Request) (*http.Response, error) {
			var body map[string]interface{}
			json.NewDecoder(req.Body).Decode(&body)
			params := body["params"].([]interface{})
			michelineParams := params[0].(map[string]interface{})
			headers := body["headers"].(map[string]interface{})
			assert.Equal(t, "SendTransaction", headers["type"])
			assert.Equal(t, "testFunc", michelineParams["entrypoint"].(string))
			assert.Equal(t, 7, len(michelineParams["value"].([]interface{})))
			assert.Equal(t, body["customOption"].(string), "customValue")
			return httpmock.NewJsonResponderOrPanic(200, "")(req)
		})

	parsedMethod, err := tz.ParseInterface(context.Background(), method, nil)
	assert.NoError(t, err)

	_, err = tz.InvokeContract(context.Background(), "", signingKey, fftypes.JSONAnyPtrBytes(locationBytes), parsedMethod, params, options, nil)
	assert.Regexp(t, "FF10398", err)
}

func TestInvokeContractBadSchema(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	httpmock.ActivateNonDefault(tz.client.GetClient())
	defer httpmock.DeactivateAndReset()
	signingKey := "tz1Y6GnVhC4EpcDDSmD3ibcC4WX6DJ4Q1QLN"
	location := &Location{
		Address: "KT12345",
	}
	options := map[string]interface{}{}
	method := &fftypes.FFIMethod{
		Name: "sum",
		Params: []*fftypes.FFIParam{
			{
				Name:   "varInt",
				Schema: fftypes.JSONAnyPtr(`{not json]`),
			},
		},
		Returns: []*fftypes.FFIParam{},
	}
	params := map[string]interface{}{
		"varInt": float64(2),
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)

	parsedMethod, err := tz.ParseInterface(context.Background(), method, nil)
	assert.NoError(t, err)

	_, err = tz.InvokeContract(context.Background(), "", signingKey, fftypes.JSONAnyPtrBytes(locationBytes), parsedMethod, params, options, nil)
	assert.Regexp(t, "FF00127", err)
}

func TestInvokeContractAddressNotSet(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	httpmock.ActivateNonDefault(tz.client.GetClient())
	defer httpmock.DeactivateAndReset()
	signingKey := "tz1Y6GnVhC4EpcDDSmD3ibcC4WX6DJ4Q1QLN"
	location := &Location{}
	options := map[string]interface{}{}
	method := testFFIMethod()
	params := map[string]interface{}{
		"varNat":       float64(1),
		"varInt":       float64(2),
		"varString":    "str",
		"varStringOpt": "optional str",
		"varBytes":     "0xAA",
		"varBool":      true,
		"varAddress":   "tz1Y6GnVhC4EpcDDSmD3ibcC4WX6DJ4Q1QLN",
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)

	parsedMethod, err := tz.ParseInterface(context.Background(), method, nil)
	assert.NoError(t, err)

	_, err = tz.InvokeContract(context.Background(), "", signingKey, fftypes.JSONAnyPtrBytes(locationBytes), parsedMethod, params, options, nil)
	assert.Regexp(t, "'address' not set", err)
}

func TestInvokeContractTezosconnectError(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	httpmock.ActivateNonDefault(tz.client.GetClient())
	defer httpmock.DeactivateAndReset()
	signingKey := "tz1Y6GnVhC4EpcDDSmD3ibcC4WX6DJ4Q1QLN"
	location := &Location{
		Address: "KT12345",
	}
	options := map[string]interface{}{}
	method := testFFIMethod()
	params := map[string]interface{}{
		"varNat":       float64(1),
		"varInt":       float64(2),
		"varString":    "str",
		"varStringOpt": "optional str",
		"varBytes":     "0xAA",
		"varBool":      true,
		"varAddress":   "tz1Y6GnVhC4EpcDDSmD3ibcC4WX6DJ4Q1QLN",
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)

	httpmock.RegisterResponder("POST", `http://localhost:12345/`,
		func(req *http.Request) (*http.Response, error) {
			return httpmock.NewJsonResponderOrPanic(400, "")(req)
		})

	parsedMethod, err := tz.ParseInterface(context.Background(), method, nil)
	assert.NoError(t, err)

	_, err = tz.InvokeContract(context.Background(), "", signingKey, fftypes.JSONAnyPtrBytes(locationBytes), parsedMethod, params, options, nil)
	assert.Regexp(t, "FF10283", err)
}

func TestInvokeContractPrepareFail(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	httpmock.ActivateNonDefault(tz.client.GetClient())
	defer httpmock.DeactivateAndReset()
	signingKey := "tz1Y6GnVhC4EpcDDSmD3ibcC4WX6DJ4Q1QLN"
	location := &Location{
		Address: "KT12345",
	}
	options := map[string]interface{}{}
	params := map[string]interface{}{
		"varNat": float64(1),
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)

	_, err = tz.InvokeContract(context.Background(), "", signingKey, fftypes.JSONAnyPtrBytes(locationBytes), "wrong", params, options, nil)
	assert.Regexp(t, "FF10457", err)
}

func TestQueryContractOK(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	httpmock.ActivateNonDefault(tz.client.GetClient())
	defer httpmock.DeactivateAndReset()
	location := &Location{
		Address: "KT12345",
	}
	options := map[string]interface{}{
		"customOption": "customValue",
	}
	method := testFFIMethod()
	params := map[string]interface{}{
		"varNat":       float64(1),
		"varInt":       float64(2),
		"varString":    "str",
		"varStringOpt": "optional str",
		"varBytes":     "0xAA",
		"varBool":      true,
		"varAddress":   "tz1Y6GnVhC4EpcDDSmD3ibcC4WX6DJ4Q1QLN",
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)

	httpmock.RegisterResponder("POST", `http://localhost:12345/`,
		func(req *http.Request) (*http.Response, error) {
			var body map[string]interface{}
			json.NewDecoder(req.Body).Decode(&body)
			headers := body["headers"].(map[string]interface{})
			assert.Equal(t, "Query", headers["type"])
			assert.Equal(t, "KT12345", body["to"].(string))
			assert.Equal(t, "tz12345", body["from"].(string))
			assert.Equal(t, body["customOption"].(string), "customValue")
			return httpmock.NewJsonResponderOrPanic(200, "result")(req)
		})

	parsedMethod, err := tz.ParseInterface(context.Background(), method, nil)
	assert.NoError(t, err)

	result, err := tz.QueryContract(context.Background(), "tz12345", fftypes.JSONAnyPtrBytes(locationBytes), parsedMethod, params, options)
	assert.NoError(t, err)

	j, err := json.Marshal(result)
	assert.NoError(t, err)
	assert.Equal(t, `"result"`, string(j))
}

func TestQueryContractInvalidOption(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	httpmock.ActivateNonDefault(tz.client.GetClient())
	defer httpmock.DeactivateAndReset()
	location := &Location{
		Address: "KT12345",
	}
	options := map[string]interface{}{
		"params": "shouldn't be allowed",
	}
	method := testFFIMethod()
	params := map[string]interface{}{
		"varNat":       float64(1),
		"varInt":       float64(2),
		"varString":    "str",
		"varStringOpt": "optional str",
		"varBytes":     "0xAA",
		"varBool":      true,
		"varAddress":   "tz1Y6GnVhC4EpcDDSmD3ibcC4WX6DJ4Q1QLN",
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)

	parsedMethod, err := tz.ParseInterface(context.Background(), method, nil)
	assert.NoError(t, err)

	_, err = tz.QueryContract(context.Background(), "tz12345", fftypes.JSONAnyPtrBytes(locationBytes), parsedMethod, params, options)
	assert.Regexp(t, "FF10398", err)
}

func TestQueryContractErrorPrepare(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	httpmock.ActivateNonDefault(tz.client.GetClient())
	defer httpmock.DeactivateAndReset()

	location := &Location{
		Address: "KT12345",
	}
	options := map[string]interface{}{}
	params := map[string]interface{}{
		"varNat": float64(1),
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)

	_, err = tz.QueryContract(context.Background(), "tz12345", fftypes.JSONAnyPtrBytes(locationBytes), "wrong", params, options)
	assert.Regexp(t, "FF10457", err)
}

func TestQueryContractAddressNotSet(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	httpmock.ActivateNonDefault(tz.client.GetClient())
	defer httpmock.DeactivateAndReset()
	location := &Location{}
	options := map[string]interface{}{}
	method := testFFIMethod()
	params := map[string]interface{}{
		"varNat":       float64(1),
		"varInt":       float64(2),
		"varString":    "str",
		"varStringOpt": "optional str",
		"varBytes":     "0xAA",
		"varBool":      true,
		"varAddress":   "tz1Y6GnVhC4EpcDDSmD3ibcC4WX6DJ4Q1QLN",
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)

	parsedMethod, err := tz.ParseInterface(context.Background(), method, nil)
	assert.NoError(t, err)

	_, err = tz.QueryContract(context.Background(), "tz12345", fftypes.JSONAnyPtrBytes(locationBytes), parsedMethod, params, options)
	assert.Regexp(t, "'address' not set", err)
}

func TestQueryContractTezosconnectError(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	httpmock.ActivateNonDefault(tz.client.GetClient())
	defer httpmock.DeactivateAndReset()
	location := &Location{
		Address: "KT12345",
	}
	options := map[string]interface{}{}
	method := testFFIMethod()
	params := map[string]interface{}{
		"varNat":       float64(1),
		"varInt":       float64(2),
		"varString":    "str",
		"varStringOpt": "optional str",
		"varBytes":     "0xAA",
		"varBool":      true,
		"varAddress":   "tz1Y6GnVhC4EpcDDSmD3ibcC4WX6DJ4Q1QLN",
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)

	httpmock.RegisterResponder("POST", `http://localhost:12345/`,
		func(req *http.Request) (*http.Response, error) {
			return httpmock.NewJsonResponderOrPanic(400, "")(req)
		})

	parsedMethod, err := tz.ParseInterface(context.Background(), method, nil)
	assert.NoError(t, err)

	_, err = tz.QueryContract(context.Background(), "tz12345", fftypes.JSONAnyPtrBytes(locationBytes), parsedMethod, params, options)
	assert.Regexp(t, "FF10283", err)
}

func TestQueryContractUnmarshalResponseError(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	httpmock.ActivateNonDefault(tz.client.GetClient())
	defer httpmock.DeactivateAndReset()
	location := &Location{
		Address: "KT12345",
	}
	options := map[string]interface{}{}
	method := testFFIMethod()
	params := map[string]interface{}{
		"varNat":       float64(1),
		"varInt":       float64(2),
		"varString":    "str",
		"varStringOpt": "optional str",
		"varBytes":     "0xAA",
		"varBool":      true,
		"varAddress":   "tz1Y6GnVhC4EpcDDSmD3ibcC4WX6DJ4Q1QLN",
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)

	httpmock.RegisterResponder("POST", `http://localhost:12345/`,
		func(req *http.Request) (*http.Response, error) {
			var body map[string]interface{}
			json.NewDecoder(req.Body).Decode(&body)
			headers := body["headers"].(map[string]interface{})
			assert.Equal(t, "Query", headers["type"])
			return httpmock.NewStringResponder(200, "[definitely not JSON}")(req)
		})

	parsedMethod, err := tz.ParseInterface(context.Background(), method, nil)
	assert.NoError(t, err)

	_, err = tz.QueryContract(context.Background(), "tz12345", fftypes.JSONAnyPtrBytes(locationBytes), parsedMethod, params, options)
	assert.Regexp(t, "invalid character", err)
}

func TestGetFFIParamValidator(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	_, err := tz.GetFFIParamValidator(context.Background())
	assert.NoError(t, err)
}

func TestGenerateFFI(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	_, err := tz.GenerateFFI(context.Background(), &fftypes.FFIGenerationRequest{
		Name:        "Simple",
		Version:     "v0.0.1",
		Description: "desc",
		Input:       fftypes.JSONAnyPtr(`[]`),
	})
	assert.Regexp(t, "FF10347", err)
}

func TestConvertDeprecatedContractConfigNoChaincode(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	_, _, err := tz.GetAndConvertDeprecatedContractConfig(tz.ctx)
	assert.NoError(t, err)
}

func TestNormalizeContractLocation(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	location := &Location{
		Address: "KT1CosvuPHD6YnY4uYNguJj6m58UuHJWyS1u",
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)
	_, err = tz.NormalizeContractLocation(context.Background(), blockchain.NormalizeCall, fftypes.JSONAnyPtrBytes(locationBytes))
	assert.NoError(t, err)
}

func TestNormalizeContractLocationInvalid(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	location := &Location{
		Address: "wrong",
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)
	_, err = tz.NormalizeContractLocation(context.Background(), blockchain.NormalizeCall, fftypes.JSONAnyPtrBytes(locationBytes))
	assert.Regexp(t, "FF10142", err)
}

func TestNormalizeContractLocationBlank(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	location := &Location{}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)
	_, err = tz.NormalizeContractLocation(context.Background(), blockchain.NormalizeCall, fftypes.JSONAnyPtrBytes(locationBytes))
	assert.Regexp(t, "FF10310", err)
}

func TestGenerateEventSignature(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	signature, err := tz.GenerateEventSignature(context.Background(), &fftypes.FFIEventDefinition{Name: "Changed"})
	assert.NoError(t, err)
	assert.Equal(t, "Changed", signature)
}

func TestGenerateEventSignatureWithLocationEmpty(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	signature, err := tz.GenerateEventSignatureWithLocation(context.Background(), &fftypes.FFIEventDefinition{Name: "Changed"}, nil)
	assert.NoError(t, err)
	assert.Equal(t, "*:Changed", signature)
}

func TestGenerateEventSignatureWithLocationBlank(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	location := &Location{}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)
	_, err = tz.GenerateEventSignatureWithLocation(context.Background(), &fftypes.FFIEventDefinition{Name: "Changed"}, fftypes.JSONAnyPtrBytes(locationBytes))
	assert.Error(t, err)
	assert.Regexp(t, "FF10310", err)
}

func TestGenerateEventSignatureWithLocation(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	location := &Location{
		Address: "KT1CosvuPHD6YnY4uYNguJj6m58UuHJWyS1u",
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)
	signature, err := tz.GenerateEventSignatureWithLocation(context.Background(), &fftypes.FFIEventDefinition{Name: "Changed"}, fftypes.JSONAnyPtrBytes(locationBytes))
	assert.NoError(t, err)
	assert.Equal(t, "KT1CosvuPHD6YnY4uYNguJj6m58UuHJWyS1u:Changed", signature)
}

func TestAddSubBadLocation(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	location := fftypes.JSONAnyPtr(fftypes.JSONObject{
		"bad": "bad",
	}.String())
	contract := &blockchain.MultipartyContract{
		Location:   location,
		FirstEvent: "oldest",
	}

	ns := &core.Namespace{Name: "ns1", NetworkName: "ns1"}
	_, err := tz.AddFireflySubscription(tz.ctx, ns, contract, "")
	assert.Regexp(t, "FF10310", err)
}

func TestAddAndRemoveFireflySubscription(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", "http://localhost:12345/eventstreams",
		httpmock.NewJsonResponderOrPanic(200, []eventStream{}))
	httpmock.RegisterResponder("POST", "http://localhost:12345/eventstreams",
		httpmock.NewJsonResponderOrPanic(200, eventStream{ID: "es12345"}))
	httpmock.RegisterResponder("GET", "http://localhost:12345/subscriptions",
		httpmock.NewJsonResponderOrPanic(200, []subscription{}))
	httpmock.RegisterResponder("POST", "http://localhost:12345/subscriptions",
		httpmock.NewJsonResponderOrPanic(200, subscription{
			ID: "sub1",
		}))

	resetConf(tz)
	utTezosconnectConf.Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	utTezosconnectConf.Set(ffresty.HTTPCustomClient, mockedClient)
	utTezosconnectConf.Set(TezosconnectConfigTopic, "topic1")

	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(tz.ctx, 100, 5*time.Minute), nil)
	originalContext := tz.ctx
	err := tz.Init(tz.ctx, tz.cancelCtx, utConfig, tz.metrics, cmi)
	cmi.AssertCalled(t, "GetCache", cache.NewCacheConfig(
		originalContext,
		coreconfig.CacheBlockchainLimit,
		coreconfig.CacheBlockchainTTL,
		"",
	))
	assert.NoError(t, err)
	location := fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "KT123",
	}.String())
	contract := &blockchain.MultipartyContract{
		Location:   location,
		FirstEvent: "newest",
	}

	ns := &core.Namespace{Name: "ns1", NetworkName: "ns1"}
	subID, err := tz.AddFireflySubscription(tz.ctx, ns, contract, "")
	assert.NoError(t, err)
	assert.NotNil(t, tz.subs.GetSubscription("sub1"))

	tz.RemoveFireflySubscription(tz.ctx, subID)
	assert.Nil(t, tz.subs.GetSubscription("sub1"))
}

func TestAddFireflySubscriptionQuerySubsFail(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", "http://localhost:12345/eventstreams",
		httpmock.NewJsonResponderOrPanic(200, []eventStream{}))
	httpmock.RegisterResponder("POST", "http://localhost:12345/eventstreams",
		httpmock.NewJsonResponderOrPanic(200, eventStream{ID: "es12345"}))
	httpmock.RegisterResponder("GET", "http://localhost:12345/subscriptions",
		httpmock.NewStringResponder(500, `pop`))
	httpmock.RegisterResponder("POST", "http://localhost:12345/subscriptions",
		httpmock.NewJsonResponderOrPanic(200, subscription{}))

	resetConf(tz)
	utTezosconnectConf.Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	utTezosconnectConf.Set(ffresty.HTTPCustomClient, mockedClient)
	utTezosconnectConf.Set(TezosconnectConfigTopic, "topic1")

	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(tz.ctx, 100, 5*time.Minute), nil)
	err := tz.Init(tz.ctx, tz.cancelCtx, utConfig, tz.metrics, cmi)
	assert.NoError(t, err)

	location := fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "KT123",
	}.String())
	contract := &blockchain.MultipartyContract{
		Location:   location,
		FirstEvent: "oldest",
	}

	ns := &core.Namespace{Name: "ns1", NetworkName: "ns1"}
	_, err = tz.AddFireflySubscription(tz.ctx, ns, contract, "")
	assert.Regexp(t, "FF10283", err)
}

func TestAddFireflySubscriptionCreateError(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", "http://localhost:12345/eventstreams",
		httpmock.NewJsonResponderOrPanic(200, []eventStream{}))
	httpmock.RegisterResponder("POST", "http://localhost:12345/eventstreams",
		httpmock.NewJsonResponderOrPanic(200, eventStream{ID: "es12345"}))
	httpmock.RegisterResponder("GET", "http://localhost:12345/subscriptions",
		httpmock.NewJsonResponderOrPanic(200, []subscription{}))
	httpmock.RegisterResponder("POST", "http://localhost:12345/subscriptions",
		httpmock.NewJsonResponderOrPanic(500, `pop`))

	resetConf(tz)
	utTezosconnectConf.Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	utTezosconnectConf.Set(ffresty.HTTPCustomClient, mockedClient)
	utTezosconnectConf.Set(TezosconnectConfigTopic, "topic1")

	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(tz.ctx, 100, 5*time.Minute), nil)
	err := tz.Init(tz.ctx, tz.cancelCtx, utConfig, tz.metrics, cmi)
	assert.NoError(t, err)

	location := fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "KT123",
	}.String())
	contract := &blockchain.MultipartyContract{
		Location:   location,
		FirstEvent: "oldest",
	}

	ns := &core.Namespace{Name: "ns1", NetworkName: "ns1"}
	_, err = tz.AddFireflySubscription(tz.ctx, ns, contract, "")
	assert.Regexp(t, "FF10283", err)
}

func TestGetContractListenerStatus(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	checkpoint := ListenerCheckpoint{
		Block:                   0,
		TransactionBatchIndex:   -1,
		TransactionIndex:        -1,
		MetaInternalResultIndex: -1,
	}

	httpmock.RegisterResponder("GET", "http://localhost:12345/eventstreams",
		httpmock.NewJsonResponderOrPanic(200, []eventStream{}))
	httpmock.RegisterResponder("POST", "http://localhost:12345/eventstreams",
		httpmock.NewJsonResponderOrPanic(200, eventStream{ID: "es12345"}))
	httpmock.RegisterResponder("GET", "http://localhost:12345/subscriptions",
		httpmock.NewJsonResponderOrPanic(200, []subscription{}))
	httpmock.RegisterResponder("GET", "http://localhost:12345/subscriptions/sub1",
		httpmock.NewJsonResponderOrPanic(200, subscription{
			ID: "sub1", Stream: "es12345", Name: "ff-sub-1132312312312", subscriptionCheckpoint: subscriptionCheckpoint{
				Catchup:    false,
				Checkpoint: checkpoint,
			},
		}))

	resetConf(tz)
	utTezosconnectConf.Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	utTezosconnectConf.Set(ffresty.HTTPCustomClient, mockedClient)
	utTezosconnectConf.Set(TezosconnectConfigTopic, "topic1")

	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(tz.ctx, 100, 5*time.Minute), nil)
	err := tz.Init(tz.ctx, tz.cancelCtx, utConfig, tz.metrics, cmi)
	assert.NoError(t, err)

	found, detail, status, err := tz.GetContractListenerStatus(context.Background(), "ns1", "sub1", true)
	assert.NotNil(t, detail)
	assert.Equal(t, core.ContractListenerStatusSynced, status)
	assert.NoError(t, err)
	assert.True(t, found)
}

func TestGetContractListenerStatusSyncing(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	checkpoint := ListenerCheckpoint{
		Block:                   0,
		TransactionBatchIndex:   -1,
		TransactionIndex:        -1,
		MetaInternalResultIndex: -1,
	}

	httpmock.RegisterResponder("GET", "http://localhost:12345/eventstreams",
		httpmock.NewJsonResponderOrPanic(200, []eventStream{}))
	httpmock.RegisterResponder("POST", "http://localhost:12345/eventstreams",
		httpmock.NewJsonResponderOrPanic(200, eventStream{ID: "es12345"}))
	httpmock.RegisterResponder("GET", "http://localhost:12345/subscriptions",
		httpmock.NewJsonResponderOrPanic(200, []subscription{}))
	httpmock.RegisterResponder("GET", "http://localhost:12345/subscriptions/sub1",
		httpmock.NewJsonResponderOrPanic(200, subscription{
			ID: "sub1", Stream: "es12345", Name: "ff-sub-1132312312312", subscriptionCheckpoint: subscriptionCheckpoint{
				Catchup:    true,
				Checkpoint: checkpoint,
			},
		}))

	resetConf(tz)
	utTezosconnectConf.Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	utTezosconnectConf.Set(ffresty.HTTPCustomClient, mockedClient)
	utTezosconnectConf.Set(TezosconnectConfigTopic, "topic1")

	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(tz.ctx, 100, 5*time.Minute), nil)
	err := tz.Init(tz.ctx, tz.cancelCtx, utConfig, tz.metrics, cmi)
	assert.NoError(t, err)

	found, detail, status, err := tz.GetContractListenerStatus(context.Background(), "ns1", "sub1", true)
	assert.NotNil(t, detail)
	assert.Equal(t, core.ContractListenerStatusSyncing, status)
	assert.NoError(t, err)
	assert.True(t, found)
}

func TestGetContractListenerStatusGetSubFail(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", "http://localhost:12345/eventstreams",
		httpmock.NewJsonResponderOrPanic(200, []eventStream{}))
	httpmock.RegisterResponder("POST", "http://localhost:12345/eventstreams",
		httpmock.NewJsonResponderOrPanic(200, eventStream{ID: "es12345"}))
	httpmock.RegisterResponder("GET", "http://localhost:12345/subscriptions",
		httpmock.NewJsonResponderOrPanic(200, []subscription{}))
	httpmock.RegisterResponder("GET", "http://localhost:12345/subscriptions/sub1",
		httpmock.NewJsonResponderOrPanic(500, `pop`))

	resetConf(tz)
	utTezosconnectConf.Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	utTezosconnectConf.Set(ffresty.HTTPCustomClient, mockedClient)
	utTezosconnectConf.Set(TezosconnectConfigTopic, "topic1")

	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(tz.ctx, 100, 5*time.Minute), nil)
	err := tz.Init(tz.ctx, tz.cancelCtx, utConfig, tz.metrics, cmi)
	assert.NoError(t, err)

	found, detail, status, err := tz.GetContractListenerStatus(context.Background(), "ns1", "sub1", true)
	assert.Nil(t, detail)
	assert.Equal(t, core.ContractListenerStatusUnknown, status)
	assert.Regexp(t, "FF10283", err)
	assert.False(t, found)
}

func TestGetContractListenerStatusGetSubNotFound(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	httpmock.RegisterResponder("GET", "http://localhost:12345/eventstreams",
		httpmock.NewJsonResponderOrPanic(200, []eventStream{}))
	httpmock.RegisterResponder("POST", "http://localhost:12345/eventstreams",
		httpmock.NewJsonResponderOrPanic(200, eventStream{ID: "es12345"}))
	httpmock.RegisterResponder("GET", "http://localhost:12345/subscriptions",
		httpmock.NewJsonResponderOrPanic(200, []subscription{}))
	httpmock.RegisterResponder("GET", "http://localhost:12345/subscriptions/sub1",
		httpmock.NewJsonResponderOrPanic(404, `not found`))

	resetConf(tz)
	utTezosconnectConf.Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	utTezosconnectConf.Set(ffresty.HTTPCustomClient, mockedClient)
	utTezosconnectConf.Set(TezosconnectConfigTopic, "topic1")

	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(tz.ctx, 100, 5*time.Minute), nil)
	err := tz.Init(tz.ctx, tz.cancelCtx, utConfig, tz.metrics, cmi)
	assert.NoError(t, err)

	found, detail, status, err := tz.GetContractListenerStatus(context.Background(), "ns1", "sub1", true)
	assert.Nil(t, detail)
	assert.Equal(t, core.ContractListenerStatusUnknown, status)
	assert.Nil(t, err)
	assert.False(t, found)
}

func TestGetTransactionStatusSuccess(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	httpmock.ActivateNonDefault(tz.client.GetClient())
	defer httpmock.DeactivateAndReset()

	op := &core.Operation{
		Namespace: "ns1",
		ID:        fftypes.MustParseUUID("9ffc50ff-6bfe-4502-adc7-93aea54cc059"),
		Status:    "Pending",
	}

	httpmock.RegisterResponder("GET", `http://localhost:12345/transactions/ns1:9ffc50ff-6bfe-4502-adc7-93aea54cc059`,
		func(req *http.Request) (*http.Response, error) {
			transactionStatus := make(map[string]interface{})
			transactionStatus["status"] = "Succeeded"
			return httpmock.NewJsonResponderOrPanic(200, transactionStatus)(req)
		})

	status, err := tz.GetTransactionStatus(context.Background(), op)
	assert.NotNil(t, status)
	assert.NoError(t, err)
}

func TestGetTransactionStatusFailed(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	httpmock.ActivateNonDefault(tz.client.GetClient())
	defer httpmock.DeactivateAndReset()

	op := &core.Operation{
		Namespace: "ns1",
		ID:        fftypes.MustParseUUID("9ffc50ff-6bfe-4502-adc7-93aea54cc059"),
		Status:    "Pending",
	}

	httpmock.RegisterResponder("GET", `http://localhost:12345/transactions/ns1:9ffc50ff-6bfe-4502-adc7-93aea54cc059`,
		func(req *http.Request) (*http.Response, error) {
			transactionStatus := make(map[string]interface{})
			transactionStatus["status"] = "Failed"
			return httpmock.NewJsonResponderOrPanic(200, transactionStatus)(req)
		})

	status, err := tz.GetTransactionStatus(context.Background(), op)
	assert.NotNil(t, status)
	assert.NoError(t, err)
}

func TestGetTransactionStatusEmptyResult(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	httpmock.ActivateNonDefault(tz.client.GetClient())
	defer httpmock.DeactivateAndReset()

	op := &core.Operation{
		Namespace: "ns1",
		ID:        fftypes.MustParseUUID("9ffc50ff-6bfe-4502-adc7-93aea54cc059"),
		Status:    "Pending",
	}

	httpmock.RegisterResponder("GET", `http://localhost:12345/transactions/ns1:9ffc50ff-6bfe-4502-adc7-93aea54cc059`,
		func(req *http.Request) (*http.Response, error) {
			return httpmock.NewJsonResponderOrPanic(200, make(map[string]interface{}))(req)
		})

	status, err := tz.GetTransactionStatus(context.Background(), op)
	assert.NotNil(t, status)
	assert.NoError(t, err)
}

func TestGetTransactionStatusNoResult(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	httpmock.ActivateNonDefault(tz.client.GetClient())
	defer httpmock.DeactivateAndReset()

	op := &core.Operation{
		Namespace: "ns1",
		ID:        fftypes.MustParseUUID("9ffc50ff-6bfe-4502-adc7-93aea54cc059"),
	}

	httpmock.RegisterResponder("GET", `http://localhost:12345/transactions/ns1:9ffc50ff-6bfe-4502-adc7-93aea54cc059`,
		func(req *http.Request) (*http.Response, error) {
			return httpmock.NewJsonResponderOrPanic(404, make(map[string]interface{}))(req)
		})

	status, err := tz.GetTransactionStatus(context.Background(), op)
	assert.Nil(t, status)
	assert.Nil(t, err)
}

func TestGetTransactionStatusBadResult(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	httpmock.ActivateNonDefault(tz.client.GetClient())
	defer httpmock.DeactivateAndReset()

	op := &core.Operation{
		Namespace: "ns1",
		ID:        fftypes.MustParseUUID("9ffc50ff-6bfe-4502-adc7-93aea54cc059"),
	}

	httpmock.RegisterResponder("GET", `http://localhost:12345/transactions/ns1:9ffc50ff-6bfe-4502-adc7-93aea54cc059`,
		func(req *http.Request) (*http.Response, error) {
			return httpmock.NewJsonResponderOrPanic(500, make(map[string]interface{}))(req)
		})

	status, err := tz.GetTransactionStatus(context.Background(), op)
	assert.Nil(t, status)
	assert.Error(t, err)
}

func TestValidateInvokeRequest(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	err := tz.ValidateInvokeRequest(context.Background(), &ffiMethodAndErrors{
		method: &fftypes.FFIMethod{},
	}, nil, false)
	assert.NoError(t, err)
}

func TestGenerateErrorSignature(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	res := tz.GenerateErrorSignature(context.Background(), nil)
	assert.Equal(t, res, "")
}

func TestSubmitNetworkAction(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	location := fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "KT123",
	}.String())
	singer := "tz1Y6GnVhC4EpcDDSmD3ibcC4WX6DJ4Q1QLN"

	err := tz.SubmitNetworkAction(context.Background(), "", singer, core.NetworkActionTerminate, location)
	assert.NoError(t, err)
}

func TestSubmitBatchPin(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()

	location := fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "KT123",
	}.String())
	singer := "tz1Y6GnVhC4EpcDDSmD3ibcC4WX6DJ4Q1QLN"

	err := tz.SubmitBatchPin(context.Background(), "", "", singer, nil, location)
	assert.NoError(t, err)
}

func TestStartNamespace(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	err := tz.StartNamespace(context.Background(), "ns1")
	assert.NoError(t, err)
}

func TestStopNamespace(t *testing.T) {
	tz, cancel := newTestTezos()
	defer cancel()
	err := tz.StopNamespace(context.Background(), "ns1")
	assert.NoError(t, err)
}

func TestStringifyNormalizeContractLocation(t *testing.T) {
	e, cancel := newTestTezos()
	defer cancel()
	location := &Location{
		Address: "3081D84FD367044F4ED453F2024709242470388C",
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)
	result, err := e.StringifyContractLocation(context.Background(), fftypes.JSONAnyPtrBytes(locationBytes))
	assert.NoError(t, err)
	assert.Equal(t, "3081D84FD367044F4ED453F2024709242470388C", result)
}

func TestStringifyNormalizeContractLocationError(t *testing.T) {
	e, cancel := newTestTezos()
	defer cancel()
	location := &Location{}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)
	_, err = e.StringifyContractLocation(context.Background(), fftypes.JSONAnyPtrBytes(locationBytes))
	assert.Error(t, err)
	assert.Regexp(t, "FF10310", err)
}

func TestCheckOverlappingLocationsEmpty(t *testing.T) {
	e, cancel := newTestTezos()
	defer cancel()
	location := &Location{}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)
	overlapping, err := e.CheckOverlappingLocations(context.Background(), nil, fftypes.JSONAnyPtrBytes(locationBytes))
	assert.NoError(t, err)
	assert.True(t, overlapping)
}

func TestCheckOverlappingLocationsBadLocation(t *testing.T) {
	e, cancel := newTestTezos()
	defer cancel()
	location := &Location{}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)
	_, err = e.CheckOverlappingLocations(context.Background(), fftypes.JSONAnyPtrBytes(locationBytes), fftypes.JSONAnyPtrBytes(locationBytes))
	assert.Error(t, err)
	assert.Regexp(t, "FF10310", err.Error())
}

func TestCheckOverlappingLocationsOneLocation(t *testing.T) {
	e, cancel := newTestTezos()
	defer cancel()
	location := &Location{
		Address: "3081D84FD367044F4ED453F2024709242470388C",
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)

	location2 := &Location{}
	location2Bytes, err := json.Marshal(location2)
	assert.NoError(t, err)
	_, err = e.CheckOverlappingLocations(context.Background(), fftypes.JSONAnyPtrBytes(locationBytes), fftypes.JSONAnyPtrBytes(location2Bytes))
	assert.Error(t, err)
	assert.Regexp(t, "FF10310", err.Error())
}

func TestCheckOverlappingLocationsSameLocation(t *testing.T) {
	e, cancel := newTestTezos()
	defer cancel()
	location := &Location{
		Address: "3081D84FD367044F4ED453F2024709242470388C",
	}
	locationBytes, err := json.Marshal(location)
	assert.NoError(t, err)
	result, err := e.CheckOverlappingLocations(context.Background(), fftypes.JSONAnyPtrBytes(locationBytes), fftypes.JSONAnyPtrBytes(locationBytes))
	assert.NoError(t, err)
	assert.True(t, result)
}
