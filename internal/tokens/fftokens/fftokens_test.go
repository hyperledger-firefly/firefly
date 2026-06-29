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

package fftokens

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/ffresty"
	"github.com/hyperledger-firefly/common/pkg/fftls"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/retry"
	"github.com/hyperledger-firefly/common/pkg/wsclient"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/operations"
	"github.com/hyperledger-firefly/firefly/mocks/coremocks"
	"github.com/hyperledger-firefly/firefly/mocks/tokenmocks"
	"github.com/hyperledger-firefly/firefly/mocks/wsmocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/tokens"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var ffTokensConfig = config.RootSection("fftokens")

func newTestFFTokens(t *testing.T) (h *FFTokens, toServer, fromServer chan string, httpURL string, done func()) {
	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)

	toServer, fromServer, wsURL, cancel := wsclient.NewTestWSServer(nil)

	u, _ := url.Parse(wsURL)
	u.Scheme = "http"
	httpURL = u.String()

	coreconfig.Reset()
	h = &FFTokens{}
	h.InitConfig(ffTokensConfig)

	ffTokensConfig.AddKnownKey(ffresty.HTTPConfigURL, httpURL)
	ffTokensConfig.AddKnownKey(ffresty.HTTPCustomClient, mockedClient)
	config.Set("tokens", []fftypes.JSONObject{{}})

	ctx, cancelCtx := context.WithCancel(context.Background())
	err := h.Init(ctx, cancelCtx, "testtokens", ffTokensConfig)
	assert.NoError(t, err)
	assert.Equal(t, "fftokens", h.Name())
	assert.Equal(t, "testtokens", h.configuredName)
	assert.NotNil(t, h.Capabilities())
	return h, toServer, fromServer, httpURL, func() {
		cancel()
		httpmock.DeactivateAndReset()
	}
}

func TestUnsetHandler(t *testing.T) {
	h, _, _, _, done := newTestFFTokens(t)
	defer done()
	mcb1 := &tokenmocks.Callbacks{}
	h.SetHandler("ns1", mcb1)
	mocb1 := &coremocks.OperationCallbacks{}
	h.SetOperationHandler("ns1", mocb1)
	assert.Equal(t, 1, len(h.callbacks.handlers))
	assert.Equal(t, 1, len(h.callbacks.opHandlers))
	h.SetHandler("ns1", nil)
	assert.Empty(t, h.callbacks.handlers)
	h.SetOperationHandler("ns1", nil)
	assert.Empty(t, h.callbacks.opHandlers)
	assert.Equal(t, 0, len(h.callbacks.handlers))
	assert.Equal(t, 0, len(h.callbacks.opHandlers))
}

func TestInitBadURL(t *testing.T) {
	coreconfig.Reset()
	h := &FFTokens{}
	h.InitConfig(ffTokensConfig)

	ffTokensConfig.AddKnownKey(ffresty.HTTPConfigURL, "::::////")

	ctx, cancelCtx := context.WithCancel(context.Background())
	err := h.Init(ctx, cancelCtx, "testtokens", ffTokensConfig)
	assert.NoError(t, err)

	err = h.StartNamespace(ctx, "ns1", []*core.TokenPool{})
	assert.Regexp(t, "FF00149", err)
}

func TestStartNamespaceConnectFail(t *testing.T) {
	coreconfig.Reset()
	h := &FFTokens{}
	h.InitConfig(ffTokensConfig)

	ffTokensConfig.AddKnownKey(ffresty.HTTPConfigURL, "http://localhost:8080")
	ffTokensConfig.Set(FFTBackgroundStart, true)

	ctx, cancelCtx := context.WithCancel(context.Background())
	err := h.Init(ctx, cancelCtx, "testtokens", ffTokensConfig)
	assert.NoError(t, err)

	err = h.StartNamespace(ctx, "ns1", []*core.TokenPool{})
	assert.Error(t, err)
}

func TestStopNamespace(t *testing.T) {
	wsm := &wsmocks.WSClient{}
	ctx, cancel := context.WithCancel(context.Background())
	wsm.On("Close").Return(nil)
	cancel()
	h := &FFTokens{
		ctx:    ctx,
		wsconn: map[string]wsclient.WSClient{"ns1": wsm},
	}
	h.StopNamespace(ctx, "ns1")
	assert.Nil(t, h.wsconn["ns1"])
}

func TestInitBadTLS(t *testing.T) {
	coreconfig.Reset()
	h := &FFTokens{}
	h.InitConfig(ffTokensConfig)

	tlsConf := ffTokensConfig.SubSection("tls")
	tlsConf.Set(fftls.HTTPConfTLSEnabled, true)
	tlsConf.Set(fftls.HTTPConfTLSCAFile, "!!!!!badness")
	ffTokensConfig.AddKnownKey(ffresty.HTTPConfigURL, "http://localhost:8080")

	ctx, cancelCtx := context.WithCancel(context.Background())
	err := h.Init(ctx, cancelCtx, "testtokens", ffTokensConfig)
	assert.Regexp(t, "FF00153", err)
}

func TestInitMissingURL(t *testing.T) {
	coreconfig.Reset()
	h := &FFTokens{}
	h.InitConfig(ffTokensConfig)

	ctx, cancelCtx := context.WithCancel(context.Background())
	err := h.Init(ctx, cancelCtx, "testtokens", ffTokensConfig)
	assert.Regexp(t, "FF10138", err)
}

func TestCreateTokenPool(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()

	opID := fftypes.NewUUID()
	nsOpID := "ns1:" + opID.String()
	pool := &core.TokenPool{
		ID: fftypes.NewUUID(),
		TX: core.TransactionRef{
			ID:   fftypes.NewUUID(),
			Type: core.TransactionTypeTokenPool,
		},
		Namespace: "ns1",
		Name:      "new-pool",
		Type:      "fungible",
		Key:       "0x123",
		Config: fftypes.JSONObject{
			"foo": "bar",
		},
		Symbol: "symbol",
	}

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/createpool", httpURL),
		func(req *http.Request) (*http.Response, error) {
			body := make(fftypes.JSONObject)
			err := json.NewDecoder(req.Body).Decode(&body)
			assert.NoError(t, err)
			assert.Equal(t, fftypes.JSONObject{
				"namespace": "ns1",
				"requestId": "ns1:" + opID.String(),
				"signer":    "0x123",
				"type":      "fungible",
				"config": map[string]interface{}{
					"foo": "bar",
				},
				"data": fftypes.JSONObject{
					"tx":     pool.TX.ID.String(),
					"txtype": core.TransactionTypeTokenPool.String(),
				}.String(),
				"name":   "new-pool",
				"symbol": "symbol",
			}, body)

			res := &http.Response{
				Body: io.NopCloser(bytes.NewReader([]byte(`{"id":"1"}`))),
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				StatusCode: 202,
			}
			return res, nil
		})

	phase, err := h.CreateTokenPool(context.Background(), nsOpID, pool)
	assert.Equal(t, core.OpPhasePending, phase)
	assert.NoError(t, err)
}

func TestCreateTokenPoolError(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()

	pool := &core.TokenPool{
		ID: fftypes.NewUUID(),
		TX: core.TransactionRef{
			ID:   fftypes.NewUUID(),
			Type: core.TransactionTypeTokenPool,
		},
	}

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/createpool", httpURL),
		httpmock.NewJsonResponderOrPanic(400, fftypes.JSONObject{
			"error":   "Bad Request",
			"message": "Missing required field",
		}))

	nsOpID := "ns1:" + fftypes.NewUUID().String()
	phase, err := h.CreateTokenPool(context.Background(), nsOpID, pool)
	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.Regexp(t, "FF10274.*Bad Request: Missing required field", err)
}

func TestCreateTokenPoolErrorMessageOnly(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()

	pool := &core.TokenPool{
		ID: fftypes.NewUUID(),
		TX: core.TransactionRef{
			ID:   fftypes.NewUUID(),
			Type: core.TransactionTypeTokenPool,
		},
	}

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/createpool", httpURL),
		httpmock.NewJsonResponderOrPanic(400, fftypes.JSONObject{
			"message": "Missing required field",
		}))

	nsOpID := "ns1:" + fftypes.NewUUID().String()
	phase, err := h.CreateTokenPool(context.Background(), nsOpID, pool)
	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.Regexp(t, "FF10274.*Missing required field", err)
}

func TestCreateTokenPoolUnexpectedError(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()

	pool := &core.TokenPool{
		ID: fftypes.NewUUID(),
		TX: core.TransactionRef{
			ID:   fftypes.NewUUID(),
			Type: core.TransactionTypeTokenPool,
		},
	}

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/createpool", httpURL),
		httpmock.NewStringResponder(400, "Failed miserably"))

	nsOpID := "ns1:" + fftypes.NewUUID().String()
	phase, err := h.CreateTokenPool(context.Background(), nsOpID, pool)
	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.Regexp(t, "FF10274.*Failed miserably", err)
}

func TestCreateTokenPoolSynchronous(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()

	opID := fftypes.NewUUID()
	nsOpID := "ns1:" + opID.String()
	pool := &core.TokenPool{
		ID: fftypes.NewUUID(),
		TX: core.TransactionRef{
			ID:   fftypes.NewUUID(),
			Type: core.TransactionTypeTokenPool,
		},
		Namespace: "ns1",
		Name:      "new-pool",
		Type:      "fungible",
		Key:       "0x123",
		Config: fftypes.JSONObject{
			"foo": "bar",
		},
		Symbol: "symbol",
	}

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/createpool", httpURL),
		func(req *http.Request) (*http.Response, error) {
			body := make(fftypes.JSONObject)
			err := json.NewDecoder(req.Body).Decode(&body)
			assert.NoError(t, err)

			res := &http.Response{
				Body: io.NopCloser(bytes.NewReader([]byte(fftypes.JSONObject{
					"type":        "fungible",
					"poolLocator": "F1",
					"signer":      "0x0",
					"decimals":    18,
					"data":        `{"tx":"` + pool.TX.ID.String() + `"}`,
				}.String()))),
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				StatusCode: 200,
			}
			return res, nil
		})

	mcb := &tokenmocks.Callbacks{}
	h.SetHandler("ns1", mcb)

	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()
	mcb.On("TokenPoolCreated", ctx /* important this gets through */, h, mock.MatchedBy(func(p *tokens.TokenPool) bool {
		return p.PoolLocator == "F1" && p.Type == core.TokenTypeFungible && *p.TX.ID == *pool.TX.ID
	})).Return(nil)

	phase, err := h.CreateTokenPool(ctx, nsOpID, pool)
	assert.Equal(t, core.OpPhaseComplete, phase)
	assert.NoError(t, err)
}

func TestCreateTokenPoolSynchronousBadResponse(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()

	opID := fftypes.NewUUID()
	nsOpID := "ns1:" + opID.String()
	pool := &core.TokenPool{
		ID: fftypes.NewUUID(),
		TX: core.TransactionRef{
			ID:   fftypes.NewUUID(),
			Type: core.TransactionTypeTokenPool,
		},
		Namespace: "ns1",
		Name:      "new-pool",
		Type:      "fungible",
		Key:       "0x123",
		Config: fftypes.JSONObject{
			"foo": "bar",
		},
		Symbol: "symbol",
	}

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/createpool", httpURL),
		func(req *http.Request) (*http.Response, error) {
			body := make(fftypes.JSONObject)
			err := json.NewDecoder(req.Body).Decode(&body)
			assert.NoError(t, err)

			res := &http.Response{
				Body: io.NopCloser(bytes.NewReader([]byte("bad"))),
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				StatusCode: 200,
			}
			return res, nil
		})

	phase, err := h.CreateTokenPool(context.Background(), nsOpID, pool)
	assert.Equal(t, core.OpPhaseComplete, phase)
	assert.Regexp(t, "FF00127", err)
}

func TestActivateTokenPool(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()

	poolConfig := map[string]interface{}{
		"address": "0x12345",
	}
	pool := &core.TokenPool{
		Namespace: "ns1",
		Locator:   "N1",
		Config:    poolConfig,
	}

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/activatepool", httpURL),
		func(req *http.Request) (*http.Response, error) {
			body := make(fftypes.JSONObject)
			err := json.NewDecoder(req.Body).Decode(&body)
			assert.NoError(t, err)
			assert.Equal(t, fftypes.JSONObject{
				"namespace":   "ns1",
				"poolData":    "ns1",
				"poolLocator": "N1",
				"config":      poolConfig,
			}, body)

			res := &http.Response{
				Body: io.NopCloser(bytes.NewReader([]byte(`{"id":"1"}`))),
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				StatusCode: 202,
			}
			return res, nil
		})

	phase, err := h.ActivateTokenPool(context.Background(), pool)
	assert.Equal(t, core.OpPhasePending, phase)
	assert.NoError(t, err)
}

func TestActivateTokenPoolError(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()

	pool := &core.TokenPool{
		ID: fftypes.NewUUID(),
		TX: core.TransactionRef{
			ID:   fftypes.NewUUID(),
			Type: core.TransactionTypeTokenPool,
		},
	}

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/activatepool", httpURL),
		httpmock.NewJsonResponderOrPanic(500, fftypes.JSONObject{}))

	phase, err := h.ActivateTokenPool(context.Background(), pool)
	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.Regexp(t, "FF10274", err)
}

func TestActivateTokenPoolSynchronous(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()

	poolConfig := map[string]interface{}{
		"foo": "bar",
	}
	pool := &core.TokenPool{
		Namespace: "ns1",
		Locator:   "N1",
		Config:    poolConfig,
	}

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/activatepool", httpURL),
		func(req *http.Request) (*http.Response, error) {
			body := make(fftypes.JSONObject)
			err := json.NewDecoder(req.Body).Decode(&body)
			assert.NoError(t, err)
			assert.Equal(t, fftypes.JSONObject{
				"namespace":   "ns1",
				"poolData":    "ns1",
				"poolLocator": "N1",
				"config":      poolConfig,
			}, body)

			res := &http.Response{
				Body: io.NopCloser(bytes.NewReader([]byte(fftypes.JSONObject{
					"type":        "fungible",
					"poolLocator": "F1",
					"signer":      "0x0",
				}.String()))),
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				StatusCode: 200,
			}
			return res, nil
		})

	mcb := &tokenmocks.Callbacks{}
	h.SetHandler("ns1", mcb)
	mcb.On("TokenPoolCreated", mock.Anything, h, mock.MatchedBy(func(p *tokens.TokenPool) bool {
		return p.PoolLocator == "F1" && p.Type == core.TokenTypeFungible && p.TX.ID == nil && p.Event == nil
	})).Return(nil)

	phase, err := h.ActivateTokenPool(context.Background(), pool)
	assert.Equal(t, core.OpPhaseComplete, phase)
	assert.NoError(t, err)
}

func TestActivateTokenPoolSynchronousBadResponse(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()

	poolConfig := map[string]interface{}{
		"foo": "bar",
	}
	pool := &core.TokenPool{
		Namespace: "ns1",
		Locator:   "N1",
		Config:    poolConfig,
	}

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/activatepool", httpURL),
		func(req *http.Request) (*http.Response, error) {
			body := make(fftypes.JSONObject)
			err := json.NewDecoder(req.Body).Decode(&body)
			assert.NoError(t, err)
			assert.Equal(t, fftypes.JSONObject{
				"namespace":   "ns1",
				"poolData":    "ns1",
				"poolLocator": "N1",
				"config":      poolConfig,
			}, body)

			res := &http.Response{
				Body: io.NopCloser(bytes.NewReader([]byte("bad"))),
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				StatusCode: 200,
			}
			return res, nil
		})

	mcb := &tokenmocks.Callbacks{}
	h.SetHandler("ns1", mcb)
	mcb.On("TokenPoolCreated", mock.Anything, h, mock.MatchedBy(func(p *tokens.TokenPool) bool {
		return p.PoolLocator == "F1" && p.Type == core.TokenTypeFungible && p.TX.ID == nil
	})).Return(nil)

	phase, err := h.ActivateTokenPool(context.Background(), pool)
	assert.Equal(t, core.OpPhaseComplete, phase)
	assert.Regexp(t, "FF00127", err)
}

func TestActivateTokenPoolNoContent(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()

	poolConfig := map[string]interface{}{
		"foo": "bar",
	}
	pool := &core.TokenPool{
		Namespace: "ns1",
		Locator:   "N1",
		ID:        fftypes.NewUUID(),
		Config:    poolConfig,
	}

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/activatepool", httpURL),
		func(req *http.Request) (*http.Response, error) {
			body := make(fftypes.JSONObject)
			err := json.NewDecoder(req.Body).Decode(&body)
			assert.NoError(t, err)
			assert.Equal(t, fftypes.JSONObject{
				"namespace":   "ns1",
				"poolData":    "ns1|" + pool.ID.String(),
				"poolLocator": "N1",
				"config":      poolConfig,
			}, body)

			res := &http.Response{
				StatusCode: 204,
			}
			return res, nil
		})

	phase, err := h.ActivateTokenPool(context.Background(), pool)
	assert.Equal(t, core.OpPhaseComplete, phase)
	assert.NoError(t, err)
}

func TestDeactivateTokenPool(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()

	pool := &core.TokenPool{
		Namespace:  "ns1",
		Locator:    "N1",
		PluginData: "ns1|pool1",
	}

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/deactivatepool", httpURL),
		func(req *http.Request) (*http.Response, error) {
			body := make(fftypes.JSONObject)
			err := json.NewDecoder(req.Body).Decode(&body)
			assert.NoError(t, err)
			assert.Equal(t, fftypes.JSONObject{
				"namespace":   "ns1",
				"poolData":    "ns1|pool1",
				"poolLocator": "N1",
				"config":      nil,
			}, body)

			res := &http.Response{
				StatusCode: 204,
			}
			return res, nil
		})

	err := h.DeactivateTokenPool(context.Background(), pool)
	assert.NoError(t, err)
}

func TestDeactivateTokenPoolFail(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()

	pool := &core.TokenPool{
		Namespace:  "ns1",
		Locator:    "N1",
		PluginData: "ns1|pool1",
	}

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/deactivatepool", httpURL),
		httpmock.NewJsonResponderOrPanic(500, fftypes.JSONObject{}))

	err := h.DeactivateTokenPool(context.Background(), pool)
	assert.Regexp(t, "FF10274", err)
}

func TestMintTokens(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()

	mint := &core.TokenTransfer{
		Namespace: "ns1",
		LocalID:   fftypes.NewUUID(),
		To:        "user1",
		Key:       "0x123",
		Amount:    *fftypes.NewFFBigInt(10),
		TX: core.TransactionRef{
			ID:   fftypes.NewUUID(),
			Type: core.TransactionTypeTokenTransfer,
		},
		URI: "FLAPFLIP",
		Config: fftypes.JSONObject{
			"foo": "bar",
		},
	}
	opID := fftypes.NewUUID()
	nsOpID := "ns1:" + opID.String()

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/mint", httpURL),
		func(req *http.Request) (*http.Response, error) {
			body := make(fftypes.JSONObject)
			err := json.NewDecoder(req.Body).Decode(&body)
			assert.NoError(t, err)
			assert.Equal(t, fftypes.JSONObject{
				"namespace":   "ns1",
				"poolLocator": "123",
				"to":          "user1",
				"amount":      "10",
				"signer":      "0x123",
				"config": map[string]interface{}{
					"foo": "bar",
				},
				"requestId": "ns1:" + opID.String(),
				"data": fftypes.JSONObject{
					"tx":     mint.TX.ID.String(),
					"txtype": core.TransactionTypeTokenTransfer.String(),
				}.String(),
				"uri": "FLAPFLIP",
			}, body)

			res := &http.Response{
				Body: io.NopCloser(bytes.NewReader([]byte(`{"id":"1"}`))),
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				StatusCode: 202,
			}
			return res, nil
		})

	err := h.MintTokens(context.Background(), nsOpID, "123", mint, nil)
	assert.NoError(t, err)
}

func TestMintTokensWithInterface(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()

	mint := &core.TokenTransfer{
		Namespace: "ns1",
		LocalID:   fftypes.NewUUID(),
		To:        "user1",
		Key:       "0x123",
		Amount:    *fftypes.NewFFBigInt(10),
		TX: core.TransactionRef{
			ID:   fftypes.NewUUID(),
			Type: core.TransactionTypeTokenTransfer,
		},
		URI: "FLAPFLIP",
		Config: fftypes.JSONObject{
			"foo": "bar",
		},
	}
	methods := fftypes.JSONAnyPtr(`{"mint": "test_interface"}`)
	opID := fftypes.NewUUID()
	nsOpID := "ns1:" + opID.String()

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/mint", httpURL),
		func(req *http.Request) (*http.Response, error) {
			body := make(fftypes.JSONObject)
			err := json.NewDecoder(req.Body).Decode(&body)
			assert.NoError(t, err)
			assert.Equal(t, fftypes.JSONObject{
				"namespace":   "ns1",
				"poolLocator": "123",
				"to":          "user1",
				"amount":      "10",
				"signer":      "0x123",
				"config": map[string]interface{}{
					"foo": "bar",
				},
				"requestId": "ns1:" + opID.String(),
				"data": fftypes.JSONObject{
					"tx":     mint.TX.ID.String(),
					"txtype": core.TransactionTypeTokenTransfer.String(),
				}.String(),
				"uri":       "FLAPFLIP",
				"interface": "test_interface",
			}, body)

			res := &http.Response{
				Body: io.NopCloser(bytes.NewReader([]byte(`{"id":"1"}`))),
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				StatusCode: 202,
			}
			return res, nil
		})

	err := h.MintTokens(context.Background(), nsOpID, "123", mint, methods)
	assert.NoError(t, err)
}

func TestTokenApproval(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()

	approval := &core.TokenApproval{
		Namespace: "ns1",
		LocalID:   fftypes.NewUUID(),
		Operator:  "0x02",
		Key:       "0x123",
		Approved:  true,
		Config: fftypes.JSONObject{
			"foo": "bar",
		},
		TX: core.TransactionRef{
			ID:   fftypes.NewUUID(),
			Type: core.TransactionTypeTokenApproval,
		},
	}
	opID := fftypes.NewUUID()
	nsOpID := "ns1:" + opID.String()

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/approval", httpURL),
		func(req *http.Request) (*http.Response, error) {
			body := make(fftypes.JSONObject)
			err := json.NewDecoder(req.Body).Decode(&body)
			assert.NoError(t, err)
			assert.Equal(t, fftypes.JSONObject{
				"namespace":   "ns1",
				"poolLocator": "123",
				"operator":    "0x02",
				"approved":    true,
				"signer":      "0x123",
				"config": map[string]interface{}{
					"foo": "bar",
				},
				"requestId": "ns1:" + opID.String(),
				"data": fftypes.JSONObject{
					"tx":     approval.TX.ID.String(),
					"txtype": core.TransactionTypeTokenApproval.String(),
				}.String(),
			}, body)

			res := &http.Response{
				Body: io.NopCloser(bytes.NewReader([]byte(`{"id":"1"}`))),
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				StatusCode: 202,
			}
			return res, nil
		})

	err := h.TokensApproval(context.Background(), nsOpID, "123", approval, nil)
	assert.NoError(t, err)
}

func TestTokenApprovalError(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()

	approval := &core.TokenApproval{}
	methods := fftypes.JSONAnyPtr(`{}`)

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/approval", httpURL),
		httpmock.NewJsonResponderOrPanic(500, fftypes.JSONObject{}))

	nsOpID := "ns1:" + fftypes.NewUUID().String()
	err := h.TokensApproval(context.Background(), nsOpID, "F1", approval, methods)
	assert.Regexp(t, "FF10274", err)
}

func TestMintTokensError(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()

	mint := &core.TokenTransfer{}

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/mint", httpURL),
		httpmock.NewJsonResponderOrPanic(500, fftypes.JSONObject{}))

	nsOpID := "ns1:" + fftypes.NewUUID().String()
	err := h.MintTokens(context.Background(), nsOpID, "F1", mint, nil)
	assert.Regexp(t, "FF10274", err)
}

func TestBurnTokens(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()

	burn := &core.TokenTransfer{
		Namespace:  "ns1",
		LocalID:    fftypes.NewUUID(),
		TokenIndex: "1",
		From:       "user1",
		Key:        "0x123",
		Amount:     *fftypes.NewFFBigInt(10),
		TX: core.TransactionRef{
			ID:   fftypes.NewUUID(),
			Type: core.TransactionTypeTokenTransfer,
		},
		Config: fftypes.JSONObject{
			"foo": "bar",
		},
	}
	opID := fftypes.NewUUID()
	nsOpID := "ns1:" + opID.String()

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/burn", httpURL),
		func(req *http.Request) (*http.Response, error) {
			body := make(fftypes.JSONObject)
			err := json.NewDecoder(req.Body).Decode(&body)
			assert.NoError(t, err)
			assert.Equal(t, fftypes.JSONObject{
				"namespace":   "ns1",
				"poolLocator": "123",
				"tokenIndex":  "1",
				"from":        "user1",
				"amount":      "10",
				"signer":      "0x123",
				"config": map[string]interface{}{
					"foo": "bar",
				},
				"requestId": "ns1:" + opID.String(),
				"data": fftypes.JSONObject{
					"tx":     burn.TX.ID.String(),
					"txtype": core.TransactionTypeTokenTransfer.String(),
				}.String(),
			}, body)

			res := &http.Response{
				Body: io.NopCloser(bytes.NewReader([]byte(`{"id":"1"}`))),
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				StatusCode: 202,
			}
			return res, nil
		})

	err := h.BurnTokens(context.Background(), nsOpID, "123", burn, nil)
	assert.NoError(t, err)
}

func TestBurnTokensError(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()

	burn := &core.TokenTransfer{}
	methods := fftypes.JSONAnyPtr(`{}`)

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/burn", httpURL),
		httpmock.NewJsonResponderOrPanic(500, fftypes.JSONObject{}))

	nsOpID := "ns1:" + fftypes.NewUUID().String()
	err := h.BurnTokens(context.Background(), nsOpID, "F1", burn, methods)
	assert.Regexp(t, "FF10274", err)
}

func TestTransferTokens(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()

	transfer := &core.TokenTransfer{
		Namespace:  "ns1",
		LocalID:    fftypes.NewUUID(),
		TokenIndex: "1",
		From:       "user1",
		To:         "user2",
		Key:        "0x123",
		Amount:     *fftypes.NewFFBigInt(10),
		TX: core.TransactionRef{
			ID:   fftypes.NewUUID(),
			Type: core.TransactionTypeTokenTransfer,
		},
		Config: fftypes.JSONObject{
			"foo": "bar",
		},
	}
	opID := fftypes.NewUUID()
	nsOpID := "ns1:" + opID.String()

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/transfer", httpURL),
		func(req *http.Request) (*http.Response, error) {
			body := make(fftypes.JSONObject)
			err := json.NewDecoder(req.Body).Decode(&body)
			assert.NoError(t, err)
			assert.Equal(t, fftypes.JSONObject{
				"namespace":   "ns1",
				"poolLocator": "123",
				"tokenIndex":  "1",
				"from":        "user1",
				"to":          "user2",
				"amount":      "10",
				"signer":      "0x123",
				"config": map[string]interface{}{
					"foo": "bar",
				},
				"requestId": "ns1:" + opID.String(),
				"data": fftypes.JSONObject{
					"tx":     transfer.TX.ID.String(),
					"txtype": core.TransactionTypeTokenTransfer.String(),
				}.String(),
			}, body)

			res := &http.Response{
				Body: io.NopCloser(bytes.NewReader([]byte(`{"id":"1"}`))),
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				StatusCode: 202,
			}
			return res, nil
		})

	err := h.TransferTokens(context.Background(), nsOpID, "123", transfer, nil)
	assert.NoError(t, err)
}

func TestTransferTokensError(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()

	transfer := &core.TokenTransfer{}
	methods := fftypes.JSONAnyPtr(`{}`)

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/transfer", httpURL),
		httpmock.NewJsonResponderOrPanic(500, fftypes.JSONObject{}))

	nsOpID := "ns1:" + fftypes.NewUUID().String()
	err := h.TransferTokens(context.Background(), nsOpID, "F1", transfer, methods)
	assert.Regexp(t, "FF10274", err)
}

func TestIgnoredEvents(t *testing.T) {
	h, toServer, fromServer, _, done := newTestFFTokens(t)
	defer done()

	err := h.StartNamespace(context.Background(), "ns1", []*core.TokenPool{})
	assert.NoError(t, err)

	fromServer <- `!}`         // ignored
	fromServer <- `{}`         // ignored
	fromServer <- `{"id":"1"}` // ignored but acked

	msg := <-toServer
	assert.Equal(t, "{\"type\":\"start\",\"autoack\":null,\"namespace\":\"ns1\",\"name\":\"\",\"ephemeral\":false,\"filter\":{\"message\":{},\"transaction\":{},\"blockchainevent\":{}},\"options\":{}}", string(msg))

	msg = <-toServer
	assert.JSONEq(t, `{"id":"1","type":"ack"}`, string(msg))

	fromServer <- fftypes.JSONObject{
		"namespace": "ns1",
		"id":        "2",
		"event":     "receipt",
		"data":      fftypes.JSONObject{},
	}.String()
}

func TestWSStartEvent(t *testing.T) {
	h, toServer, _, _, done := newTestFFTokens(t)
	defer done()

	err := h.StartNamespace(context.Background(), "ns1", []*core.TokenPool{})
	assert.NoError(t, err)

	msg := <-toServer
	assert.Equal(t, "{\"type\":\"start\",\"autoack\":null,\"namespace\":\"ns1\",\"name\":\"\",\"ephemeral\":false,\"filter\":{\"message\":{},\"transaction\":{},\"blockchainevent\":{}},\"options\":{}}", string(msg))
}

func TestReceiptEvents(t *testing.T) {
	h, _, fromServer, _, done := newTestFFTokens(t)
	defer done()

	err := h.StartNamespace(context.Background(), "ns1", []*core.TokenPool{})
	assert.NoError(t, err)

	mcb := &coremocks.OperationCallbacks{}
	h.SetOperationHandler("ns1", mcb)
	opID := fftypes.NewUUID()
	mockCalled := make(chan bool)

	// receipt: bad ID - passed through
	mcb.On("OperationUpdate", mock.MatchedBy(func(update *core.OperationUpdateAsync) bool {
		return update.NamespacedOpID == "ns1:wrong" &&
			update.Status == core.OpStatusPending &&
			update.Plugin == "fftokens"
	})).Return(nil).Once().Run(func(args mock.Arguments) { mockCalled <- true })
	fromServer <- fftypes.JSONObject{
		"id":    "3",
		"event": "receipt",
		"data": fftypes.JSONObject{
			"headers": fftypes.JSONObject{
				"requestId": "ns1:wrong", // passed through to OperationUpdate to ignore
				"type":      "TransactionUpdate",
			},
		},
	}.String()
	<-mockCalled

	// receipt: success
	mcb.On("OperationUpdate", mock.MatchedBy(func(update *core.OperationUpdateAsync) bool {
		return update.NamespacedOpID == "ns1:"+opID.String() &&
			update.Status == core.OpStatusSucceeded &&
			update.BlockchainTXID == "0xffffeeee" &&
			update.Plugin == "fftokens"
	})).Return(nil).Once().Run(func(args mock.Arguments) { mockCalled <- true })
	fromServer <- fftypes.JSONObject{
		"id":    "4",
		"event": "receipt",
		"data": fftypes.JSONObject{
			"headers": fftypes.JSONObject{
				"requestId": "ns1:" + opID.String(),
				"type":      "TransactionSuccess",
			},
			"transactionHash": "0xffffeeee",
		},
	}.String()
	<-mockCalled

	// receipt: update
	mcb.On("OperationUpdate", mock.MatchedBy(func(update *core.OperationUpdateAsync) bool {
		return update.NamespacedOpID == "ns1:"+opID.String() &&
			update.Status == core.OpStatusPending &&
			update.BlockchainTXID == "0xffffeeee"
	})).Return(nil).Once().Run(func(args mock.Arguments) { mockCalled <- true })
	fromServer <- fftypes.JSONObject{
		"id":    "5",
		"event": "receipt",
		"data": fftypes.JSONObject{
			"headers": fftypes.JSONObject{
				"requestId": "ns1:" + opID.String(),
				"type":      "TransactionUpdate",
			},
			"transactionHash": "0xffffeeee",
		},
	}.String()
	<-mockCalled

	// receipt: failure
	mcb.On("OperationUpdate", mock.MatchedBy(func(update *core.OperationUpdateAsync) bool {
		return update.NamespacedOpID == "ns1:"+opID.String() &&
			update.Status == core.OpStatusFailed &&
			update.BlockchainTXID == "0xffffeeee" &&
			update.Plugin == "fftokens"
	})).Return(nil).Once().Run(func(args mock.Arguments) { mockCalled <- true })
	fromServer <- fftypes.JSONObject{
		"id":    "5",
		"event": "receipt",
		"data": fftypes.JSONObject{
			"headers": fftypes.JSONObject{
				"requestId": "ns1:" + opID.String(),
				"type":      "TransactionFailed",
			},
			"transactionHash": "0xffffeeee",
		},
	}.String()
	<-mockCalled

	mcb.AssertExpectations(t)
}

func TestPoolEvents(t *testing.T) {
	h, toServer, fromServer, _, done := newTestFFTokens(t)
	defer done()

	err := h.StartNamespace(context.Background(), "ns1", []*core.TokenPool{})
	assert.NoError(t, err)

	mcb := &tokenmocks.Callbacks{}
	h.SetHandler("ns1", mcb)
	txID := fftypes.NewUUID()

	// token-pool: missing data
	fromServer <- fftypes.JSONObject{
		"id":    "6",
		"event": "token-pool",
	}.String()

	msg := <-toServer
	assert.Equal(t, "{\"type\":\"start\",\"autoack\":null,\"namespace\":\"ns1\",\"name\":\"\",\"ephemeral\":false,\"filter\":{\"message\":{},\"transaction\":{},\"blockchainevent\":{}},\"options\":{}}", string(msg))

	msg = <-toServer
	assert.JSONEq(t, `{"id":"6","type":"ack"}`, string(msg))

	// token-pool: invalid uuid (success)
	mcb.On("TokenPoolCreated", mock.Anything, h, mock.MatchedBy(func(p *tokens.TokenPool) bool {
		return p.PoolLocator == "F1" && p.Type == core.TokenTypeFungible && p.TX.ID == nil && p.Event.ProtocolID == "000000000010/000020/000030"
	})).Return(nil).Once()
	fromServer <- fftypes.JSONObject{
		"id":    "7",
		"event": "token-pool",
		"data": fftypes.JSONObject{
			"namespace":   "ns1",
			"id":          "000000000010/000020/000030/000040",
			"type":        "fungible",
			"poolLocator": "F1",
			"signer":      "0x0",
			"data":        fftypes.JSONObject{"tx": "bad"}.String(),
			"blockchain": fftypes.JSONObject{
				"id": "000000000010/000020/000030",
				"info": fftypes.JSONObject{
					"transactionHash": "0xffffeeee",
				},
			},
		},
	}.String()
	msg = <-toServer
	assert.JSONEq(t, `{"id":"7","type":"ack"}`, string(msg))

	// token-pool: success
	mcb.On("TokenPoolCreated", mock.Anything, h, mock.MatchedBy(func(p *tokens.TokenPool) bool {
		return p.PoolLocator == "F1" && p.Type == core.TokenTypeFungible && txID.Equals(p.TX.ID) && p.Event.ProtocolID == "000000000010/000020/000030"
	})).Return(nil).Once()
	fromServer <- fftypes.JSONObject{
		"id":    "8",
		"event": "token-pool",
		"data": fftypes.JSONObject{
			"id":          "000000000010/000020/000030/000041",
			"poolData":    "ns1",
			"type":        "fungible",
			"poolLocator": "F1",
			"signer":      "0x0",
			"data":        fftypes.JSONObject{"tx": txID.String()}.String(),
			"blockchain": fftypes.JSONObject{
				"id": "000000000010/000020/000030",
				"info": fftypes.JSONObject{
					"transactionHash": "0xffffeeee",
				},
			},
		},
	}.String()
	msg = <-toServer
	assert.JSONEq(t, `{"id":"8","type":"ack"}`, string(msg))

	// token-pool: no handler
	fromServer <- fftypes.JSONObject{
		"id":    "10",
		"event": "token-pool",
		"data": fftypes.JSONObject{
			"id":          "000000000010/000020/000030/000042",
			"poolData":    "BAD-NAMESPACE",
			"type":        "fungible",
			"poolLocator": "F1",
			"signer":      "0x0",
			"data":        fftypes.JSONObject{"tx": txID.String()}.String(),
			"blockchain": fftypes.JSONObject{
				"id": "000000000010/000020/000030",
				"info": fftypes.JSONObject{
					"transactionHash": "0xffffeeee",
				},
			},
		},
	}.String()

	// token-pool: batch + callback fail (terminates loop)
	mcb.On("TokenPoolCreated", mock.Anything, h, mock.MatchedBy(func(p *tokens.TokenPool) bool {
		return p.PoolLocator == "F1" && p.Type == core.TokenTypeFungible && txID.Equals(p.TX.ID) && p.Event.ProtocolID == "000000000010/000020/000030"
	})).Return(fmt.Errorf("pop")).Once()
	fromServer <- fftypes.JSONObject{
		"id":    "9",
		"event": "batch",
		"data": fftypes.JSONObject{
			"events": fftypes.JSONObjectArray{{
				"event": "token-pool",
				"data": fftypes.JSONObject{
					"id":          "000000000010/000020/000030/000043",
					"type":        "fungible",
					"poolLocator": "F1",
					"signer":      "0x0",
					"data":        fftypes.JSONObject{"tx": txID.String()}.String(),
					"blockchain": fftypes.JSONObject{
						"id": "000000000010/000020/000030",
						"info": fftypes.JSONObject{
							"transactionHash": "0xffffeeee",
						},
					},
				},
			}},
		},
	}.String()
}

func TestTransferEvents(t *testing.T) {
	h, toServer, fromServer, _, done := newTestFFTokens(t)
	defer done()

	err := h.StartNamespace(context.Background(), "ns1", []*core.TokenPool{})
	assert.NoError(t, err)

	mcb := &tokenmocks.Callbacks{}
	h.SetHandler("ns1", mcb)
	txID := fftypes.NewUUID()

	// token-mint: missing data
	fromServer <- fftypes.JSONObject{
		"id":    "9",
		"event": "token-mint",
	}.String()

	msg := <-toServer
	assert.Equal(t, "{\"type\":\"start\",\"autoack\":null,\"namespace\":\"ns1\",\"name\":\"\",\"ephemeral\":false,\"filter\":{\"message\":{},\"transaction\":{},\"blockchainevent\":{}},\"options\":{}}", string(msg))

	msg = <-toServer
	assert.JSONEq(t, `{"id":"9","type":"ack"}`, string(msg))

	// token-mint: invalid amount
	fromServer <- fftypes.JSONObject{
		"id":    "10",
		"event": "token-mint",
		"data": fftypes.JSONObject{
			"id":          "1.0.0",
			"poolLocator": "F1",
			"poolData":    "ns1|" + fftypes.NewUUID().String(),
			"tokenIndex":  "0",
			"signer":      "0x0",
			"to":          "0x0",
			"amount":      "bad",
			"data":        fftypes.JSONObject{"tx": txID.String()}.String(),
			"blockchain": fftypes.JSONObject{
				"info": fftypes.JSONObject{
					"transactionHash": "0xffffeeee",
				},
			},
		},
	}.String()
	msg = <-toServer
	assert.JSONEq(t, `{"id":"10","type":"ack"}`, string(msg))

	// token-mint: success
	mcb.On("TokensTransferred", h, mock.MatchedBy(func(t *tokens.TokenTransfer) bool {
		return t.Amount.Int().Int64() == 2 && t.To == "0x0" && t.TokenIndex == "" && *t.TX.ID == *txID && t.PoolLocator == "F1" && t.Event.ProtocolID == "000000000010/000020/000030"
	})).Return(nil).Once()
	fromServer <- fftypes.JSONObject{
		"id":    "11",
		"event": "token-mint",
		"data": fftypes.JSONObject{
			"id":          "000000000010/000020/000030/000040",
			"poolData":    "ns1|id1",
			"poolLocator": "F1",
			"signer":      "0x0",
			"to":          "0x0",
			"amount":      "2",
			"data":        fftypes.JSONObject{"tx": txID.String()}.String(),
			"blockchain": fftypes.JSONObject{
				"id": "000000000010/000020/000030",
				"info": fftypes.JSONObject{
					"transactionHash": "0xffffeeee",
				},
			},
		},
	}.String()
	msg = <-toServer
	assert.JSONEq(t, `{"id":"11","type":"ack"}`, string(msg))

	// token-mint: invalid uuid (success)
	mcb.On("TokensTransferred", h, mock.MatchedBy(func(t *tokens.TokenTransfer) bool {
		return t.Amount.Int().Int64() == 1 && t.To == "0x0" && t.TokenIndex == "1" && t.PoolLocator == "N1" && t.Event.ProtocolID == "000000000010/000020/000030"
	})).Return(nil).Once()
	fromServer <- fftypes.JSONObject{
		"id":    "12",
		"event": "token-mint",
		"data": fftypes.JSONObject{
			"id":          "000000000010/000020/000030/000040",
			"poolLocator": "N1",
			"tokenIndex":  "1",
			"signer":      "0x0",
			"to":          "0x0",
			"amount":      "1",
			"data":        fftypes.JSONObject{"tx": "bad"}.String(),
			"blockchain": fftypes.JSONObject{
				"id": "000000000010/000020/000030",
				"info": fftypes.JSONObject{
					"transactionHash": "0xffffeeee",
				},
			},
		},
	}.String()
	msg = <-toServer
	assert.JSONEq(t, `{"id":"12","type":"ack"}`, string(msg))

	// token-transfer: missing from
	fromServer <- fftypes.JSONObject{
		"id":    "13",
		"event": "token-transfer",
		"data": fftypes.JSONObject{
			"id":          "000000000010/000020/000030/000040",
			"poolLocator": "F1",
			"tokenIndex":  "0",
			"signer":      "0x0",
			"to":          "0x0",
			"amount":      "2",
			"data":        fftypes.JSONObject{"tx": txID.String()}.String(),
			"blockchain": fftypes.JSONObject{
				"info": fftypes.JSONObject{
					"transactionHash": "0xffffeeee",
				},
			},
		},
	}.String()
	msg = <-toServer
	assert.JSONEq(t, `{"id":"13","type":"ack"}`, string(msg))

	// token-transfer: bad message hash (success)
	mcb.On("TokensTransferred", h, mock.MatchedBy(func(t *tokens.TokenTransfer) bool {
		return t.Amount.Int().Int64() == 2 && t.From == "0x0" && t.To == "0x1" && t.TokenIndex == "" && t.PoolLocator == "F1" && t.Event.ProtocolID == "000000000010/000020/000030"
	})).Return(nil).Once()
	fromServer <- fftypes.JSONObject{
		"id":    "14",
		"event": "token-transfer",
		"data": fftypes.JSONObject{
			"id":          "000000000010/000020/000030/000040",
			"poolLocator": "F1",
			"signer":      "0x0",
			"from":        "0x0",
			"to":          "0x1",
			"amount":      "2",
			"data":        fftypes.JSONObject{"tx": txID.String(), "messageHash": "bad"}.String(),
			"blockchain": fftypes.JSONObject{
				"id": "000000000010/000020/000030",
				"info": fftypes.JSONObject{
					"transactionHash": "0xffffeeee",
				},
			},
		},
	}.String()
	msg = <-toServer
	assert.JSONEq(t, `{"id":"14","type":"ack"}`, string(msg))

	// token-transfer: success
	messageID := fftypes.NewUUID()
	mcb.On("TokensTransferred", h, mock.MatchedBy(func(t *tokens.TokenTransfer) bool {
		return t.Amount.Int().Int64() == 2 && t.From == "0x0" && t.To == "0x1" && t.TokenIndex == "" && messageID.Equals(t.Message) && t.PoolLocator == "F1" && t.Event.ProtocolID == "000000000010/000020/000030"
	})).Return(nil).Once()
	fromServer <- fftypes.JSONObject{
		"id":    "15",
		"event": "token-transfer",
		"data": fftypes.JSONObject{
			"id":          "000000000010/000020/000030/000040",
			"poolLocator": "F1",
			"signer":      "0x0",
			"from":        "0x0",
			"to":          "0x1",
			"amount":      "2",
			"data":        fftypes.JSONObject{"tx": txID.String(), "message": messageID.String()}.String(),
			"blockchain": fftypes.JSONObject{
				"id": "000000000010/000020/000030",
				"info": fftypes.JSONObject{
					"transactionHash": "0xffffeeee",
				},
			},
		},
	}.String()
	msg = <-toServer
	assert.JSONEq(t, `{"id":"15","type":"ack"}`, string(msg))

	// token-burn: success
	mcb.On("TokensTransferred", h, mock.MatchedBy(func(t *tokens.TokenTransfer) bool {
		return t.Amount.Int().Int64() == 2 && t.From == "0x0" && t.TokenIndex == "0" && t.PoolLocator == "F1" && t.Event.ProtocolID == "000000000010/000020/000030"
	})).Return(nil).Once()
	fromServer <- fftypes.JSONObject{
		"id":    "16",
		"event": "token-burn",
		"data": fftypes.JSONObject{
			"id":          "000000000010/000020/000030/000040",
			"poolLocator": "F1",
			"tokenIndex":  "0",
			"signer":      "0x0",
			"from":        "0x0",
			"amount":      "2",
			"data":        fftypes.JSONObject{"tx": txID.String()}.String(),
			"blockchain": fftypes.JSONObject{
				"id": "000000000010/000020/000030",
				"info": fftypes.JSONObject{
					"transactionHash": "0xffffeeee",
				},
			},
		},
	}.String()
	msg = <-toServer
	assert.JSONEq(t, `{"id":"16","type":"ack"}`, string(msg))

	// token-transfer: callback fail
	mcb.On("TokensTransferred", h, mock.MatchedBy(func(t *tokens.TokenTransfer) bool {
		return t.Amount.Int().Int64() == 2 && t.From == "0x0" && t.To == "0x1" && t.TokenIndex == "" && messageID.Equals(t.Message) && t.PoolLocator == "F1" && t.Event.ProtocolID == "000000000010/000020/000030"
	})).Return(fmt.Errorf("pop")).Once()
	fromServer <- fftypes.JSONObject{
		"id":    "17",
		"event": "token-transfer",
		"data": fftypes.JSONObject{
			"id":          "000000000010/000020/000030/000040",
			"poolLocator": "F1",
			"signer":      "0x0",
			"from":        "0x0",
			"to":          "0x1",
			"amount":      "2",
			"data":        fftypes.JSONObject{"tx": txID.String(), "message": messageID.String()}.String(),
			"blockchain": fftypes.JSONObject{
				"id": "000000000010/000020/000030",
				"info": fftypes.JSONObject{
					"transactionHash": "0xffffeeee",
				},
			},
		},
	}.String()
}

func TestApprovalEvents(t *testing.T) {
	h, toServer, fromServer, _, done := newTestFFTokens(t)
	defer done()

	err := h.StartNamespace(context.Background(), "ns1", []*core.TokenPool{})
	assert.NoError(t, err)

	mcb := &tokenmocks.Callbacks{}
	h.SetHandler("ns1", mcb)
	txID := fftypes.NewUUID()

	// token-approval: success
	mcb.On("TokensApproved", h, mock.MatchedBy(func(t *tokens.TokenApproval) bool {
		return t.Approved == true &&
			t.Operator == "0x0" &&
			t.PoolLocator == "F1" &&
			t.Event != nil &&
			t.Event.ProtocolID == "000000000010/000020/000030"
	})).Return(nil).Once()
	fromServer <- fftypes.JSONObject{
		"id":    "17",
		"event": "token-approval",
		"data": fftypes.JSONObject{
			"id":          "000000000010/000020/000030/000040",
			"poolData":    "ns1",
			"subject":     "a:b",
			"poolLocator": "F1",
			"signer":      "0x0",
			"operator":    "0x0",
			"approved":    true,
			"data":        fftypes.JSONObject{"tx": txID.String()}.String(),
			"blockchain": fftypes.JSONObject{
				"id": "000000000010/000020/000030",
				"info": fftypes.JSONObject{
					"transactionHash": "0xffffeeee",
				},
			},
		},
	}.String()

	msg := <-toServer
	assert.Equal(t, "{\"type\":\"start\",\"autoack\":null,\"namespace\":\"ns1\",\"name\":\"\",\"ephemeral\":false,\"filter\":{\"message\":{},\"transaction\":{},\"blockchainevent\":{}},\"options\":{}}", string(msg))

	msg = <-toServer
	assert.JSONEq(t, `{"id":"17","type":"ack"}`, string(msg))

	// token-approval: success (no data)
	mcb.On("TokensApproved", h, mock.MatchedBy(func(t *tokens.TokenApproval) bool {
		return t.Approved == true && t.Operator == "0x0" && t.PoolLocator == "F1"
	})).Return(nil).Once()
	fromServer <- fftypes.JSONObject{
		"id":    "18",
		"event": "token-approval",
		"data": fftypes.JSONObject{
			"id":          "000000000010/000020/000030/000040",
			"poolData":    "ns1",
			"subject":     "a:b",
			"poolLocator": "F1",
			"signer":      "0x0",
			"operator":    "0x0",
			"approved":    true,
			"blockchain": fftypes.JSONObject{
				"id": "000000000010/000020/000030",
				"info": fftypes.JSONObject{
					"transactionHash": "0xffffeeee",
				},
			},
		},
	}.String()
	msg = <-toServer
	assert.JSONEq(t, `{"id":"18","type":"ack"}`, string(msg))

	// token-approval: missing data
	fromServer <- fftypes.JSONObject{
		"id":    "19",
		"event": "token-approval",
	}.String()
	msg = <-toServer
	assert.JSONEq(t, `{"id":"19","type":"ack"}`, string(msg))

	// token-approval: callback fail
	errProcessed := make(chan struct{})
	mcb.On("TokensApproved", h, mock.MatchedBy(func(t *tokens.TokenApproval) bool {
		return t.Approved == true && t.Operator == "0x0" && t.PoolLocator == "F1" && t.Event.ProtocolID == "000000000010/000020/000030"
	})).Return(fmt.Errorf("pop")).Once().Run(func(args mock.Arguments) {
		// We do not ack in the case of an error
		close(errProcessed)
	})
	fromServer <- fftypes.JSONObject{
		"id":    "20",
		"event": "token-approval",
		"data": fftypes.JSONObject{
			"id":          "000000000010/000020/000030/000040",
			"poolData":    "", // deliberately to drive to all namespaces
			"subject":     "a:b",
			"poolLocator": "F1",
			"signer":      "0x0",
			"operator":    "0x0",
			"approved":    true,
			"data":        fftypes.JSONObject{"tx": txID.String()}.String(),
			"blockchain": fftypes.JSONObject{
				"id": "000000000010/000020/000030",
				"info": fftypes.JSONObject{
					"transactionHash": "0xffffeeee",
				},
			},
		},
	}.String()
	<-errProcessed

	mcb.AssertExpectations(t)
}

func TestEventLoopReceiveClosed(t *testing.T) {
	wsm := &wsmocks.WSClient{}
	called := false
	h := &FFTokens{
		ctx:       context.Background(),
		cancelCtx: func() { called = true },
		wsconn:    map[string]wsclient.WSClient{"ns1": wsm},
	}
	r := make(chan []byte)
	close(r)
	wsm.On("Close").Return()
	wsm.On("Receive").Return((<-chan []byte)(r))
	h.eventLoop("ns1")
	assert.True(t, called)
}

func TestEventLoopSendClosed(t *testing.T) {
	wsm := &wsmocks.WSClient{}
	called := false
	h := &FFTokens{
		ctx:       context.Background(),
		cancelCtx: func() { called = true },
		wsconn:    map[string]wsclient.WSClient{"ns1": wsm},
		retry:     &retry.Retry{},
	}
	r := make(chan []byte, 1)
	r <- []byte(`{"id":"1"}`) // ignored but acked
	wsm.On("Close").Return()
	wsm.On("Receive").Return((<-chan []byte)(r))
	wsm.On("Send", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
	h.eventLoop("ns1")
	assert.True(t, called)
}

func TestStartNamespaceWSConnectFail(t *testing.T) {
	wsm := &wsmocks.WSClient{}
	h := &FFTokens{
		ctx:    context.Background(),
		wsconn: map[string]wsclient.WSClient{"ns1": wsm},
		retry:  &retry.Retry{},
	}
	wsm.On("Connect").Return(fmt.Errorf("pop"))
	err := h.StartNamespace(context.Background(), "ns1", []*core.TokenPool{})
	assert.Regexp(t, "pop", err)
}

func TestEventLoopClosedContext(t *testing.T) {
	wsm := &wsmocks.WSClient{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h := &FFTokens{
		ctx:    ctx,
		wsconn: map[string]wsclient.WSClient{"ns1": wsm},
		retry:  &retry.Retry{},
	}
	r := make(chan []byte, 1)
	wsm.On("Close").Return()
	wsm.On("Receive").Return((<-chan []byte)(r))
	h.eventLoop("ns1") // we're simply looking for it exiting
}

func TestCallbacksWrongNamespace(t *testing.T) {
	h, _, _, _, done := newTestFFTokens(t)
	defer done()
	nsOpID := "ns1:" + fftypes.NewUUID().String()
	h.callbacks.OperationUpdate(context.Background(), nsOpID, core.OpStatusSucceeded, "tx123", "", nil)
	h.callbacks.TokensTransferred(context.Background(), "ns1", nil)
	h.callbacks.TokensApproved(context.Background(), "ns1", nil)
}

func TestSendWSFail(t *testing.T) {
	h, _, _, _, done := newTestFFTokens(t)
	defer done()
	namespace := "ns1"
	wsm := &wsmocks.WSClient{}
	wsm.On("Send", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
	err := h.sendWSStartMsg(context.Background(), wsm, namespace)
	assert.Regexp(t, "pop", err)
}

func TestCheckInterfaceBadFormat(t *testing.T) {
	h, _, _, _, done := newTestFFTokens(t)
	defer done()

	pool := &core.TokenPool{
		InterfaceFormat: "bad",
	}

	_, err := h.CheckInterface(context.Background(), pool, nil)
	assert.Regexp(t, "FF10435.*bad", err)
}

func TestCheckInterfaceABI(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()

	pool := &core.TokenPool{
		InterfaceFormat: "abi",
		Locator:         "N1",
	}
	methods := []*fftypes.FFIMethod{
		{Name: "method1"},
	}

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/checkinterface", httpURL),
		func(req *http.Request) (*http.Response, error) {
			body := make(fftypes.JSONObject)
			err := json.NewDecoder(req.Body).Decode(&body)
			assert.NoError(t, err)
			assert.Equal(t, fftypes.JSONObject{
				"format":      "abi",
				"poolLocator": "N1",
				"methods": []interface{}{
					map[string]interface{}{
						"name":    "method1",
						"type":    "function",
						"inputs":  []interface{}{},
						"outputs": []interface{}{},
					},
				},
			}, body)

			res := &http.Response{
				Body: io.NopCloser(bytes.NewReader([]byte(fftypes.JSONObject{
					"approval": fftypes.JSONAny(`[]`),
					"burn":     fftypes.JSONAny(`[]`),
					"mint":     fftypes.JSONAny(`[]`),
					"transfer": fftypes.JSONAny(`[]`),
				}.String()))),
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				StatusCode: 200,
			}
			return res, nil
		})

	_, err := h.CheckInterface(context.Background(), pool, methods)
	assert.NoError(t, err)
}

func TestCheckInterfaceABIBadInterface(t *testing.T) {
	h, _, _, _, done := newTestFFTokens(t)
	defer done()

	pool := &core.TokenPool{
		InterfaceFormat: "abi",
		Locator:         "N1",
	}
	methods := []*fftypes.FFIMethod{
		{Name: "method1", Params: fftypes.FFIParams{{Schema: fftypes.JSONAnyPtr("bad")}}},
	}

	_, err := h.CheckInterface(context.Background(), pool, methods)
	assert.Regexp(t, "FF22052", err)
}

func TestCheckInterfaceFFI(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()

	pool := &core.TokenPool{
		InterfaceFormat: "ffi",
		Locator:         "N1",
	}
	methods := []*fftypes.FFIMethod{
		{Name: "method1"},
	}

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/checkinterface", httpURL),
		func(req *http.Request) (*http.Response, error) {
			body := make(fftypes.JSONObject)
			err := json.NewDecoder(req.Body).Decode(&body)
			assert.NoError(t, err)
			assert.Equal(t, fftypes.JSONObject{
				"format":      "ffi",
				"poolLocator": "N1",
				"methods": []interface{}{
					map[string]interface{}{
						"name":        "method1",
						"description": "",
						"pathname":    "",
						"params":      nil,
						"returns":     nil,
					},
				},
			}, body)

			res := &http.Response{
				Body: io.NopCloser(bytes.NewReader([]byte(fftypes.JSONObject{
					"approval": fftypes.JSONAny(`[]`),
					"burn":     fftypes.JSONAny(`[]`),
					"mint":     fftypes.JSONAny(`[]`),
					"transfer": fftypes.JSONAny(`[]`),
				}.String()))),
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				StatusCode: 200,
			}
			return res, nil
		})

	_, err := h.CheckInterface(context.Background(), pool, methods)
	assert.NoError(t, err)
}

func TestCheckInterfaceFFIFail(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()

	pool := &core.TokenPool{
		InterfaceFormat: "ffi",
		Locator:         "N1",
	}
	methods := []*fftypes.FFIMethod{
		{Name: "method1"},
	}

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/checkinterface", httpURL),
		httpmock.NewJsonResponderOrPanic(500, fftypes.JSONObject{}))

	_, err := h.CheckInterface(context.Background(), pool, methods)
	assert.Regexp(t, "FF10274", err)
}

func TestCheckInterfaceFFIBadResponse(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()

	pool := &core.TokenPool{
		InterfaceFormat: "ffi",
		Locator:         "N1",
	}
	methods := []*fftypes.FFIMethod{
		{Name: "method1"},
	}

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/checkinterface", httpURL),
		func(req *http.Request) (*http.Response, error) {
			res := &http.Response{
				Body: io.NopCloser(bytes.NewReader([]byte(fftypes.JSONObject{
					"approval": map[bool]bool{true: false},
				}.String()))),
				Header: http.Header{
					"Content-Type": []string{"application/json"},
				},
				StatusCode: 200,
			}
			return res, nil
		})

	_, err := h.CheckInterface(context.Background(), pool, methods)
	assert.Regexp(t, "FF00127", err)
}

func TestHandleEventRetryableFailure(t *testing.T) {
	ft, _, _, _, done := newTestFFTokens(t)
	defer done()

	mcb := &tokenmocks.Callbacks{}
	mcb.On("TokenPoolCreated", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
	ft.callbacks.handlers = map[string]tokens.Callbacks{
		"ns1": mcb,
	}
	retry, err := ft.handleMessage(context.Background(), "ns1", []byte(`{
		"event": "batch",
		"data": {
			"events": [{
				"event": "token-pool",
				"data": {
					"type": "fungible",
					"poolLocator": "over-there"
				}
			}]
		}
	}`))
	assert.Regexp(t, "pop", err)
	assert.True(t, retry)
}

func TestErrorWrappingNoBodyError(t *testing.T) {
	ctx := context.Background()
	res := &resty.Response{
		RawResponse: &http.Response{StatusCode: 409},
	}
	err := wrapError(ctx, nil, res, fmt.Errorf("pop"))
	assert.Regexp(t, "FF10459", err)
	assert.Regexp(t, "pop", err)

	errInterface, ok := err.(operations.ConflictError)
	assert.True(t, ok)
	assert.True(t, errInterface.IsConflictError())
}

func TestErrorWrappingBodyErr(t *testing.T) {
	ctx := context.Background()
	res := &resty.Response{
		RawResponse: &http.Response{StatusCode: 409},
	}
	err := wrapError(ctx, &tokenError{Error: "snap"}, res, fmt.Errorf("pop"))
	assert.Regexp(t, "FF10459", err)
	assert.Regexp(t, "snap", err)

	errInterface, ok := err.(operations.ConflictError)
	assert.True(t, ok)
	assert.True(t, errInterface.IsConflictError())
}

func TestHandleNamespaceStartedEnsureActive(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()
	h.poolsToActivate = map[string][]*core.TokenPool{
		"ns1": {{Active: true}},
	}
	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/activatepool", httpURL),
		httpmock.NewJsonResponderOrPanic(200, fftypes.JSONObject{}))

	_, err := h.handleMessage(context.Background(), "ns1", []byte(`{"event":"started","data":{"namespace": "ns1"}}`))
	assert.NoError(t, err)
}

func TestHandleNamespaceStartedEnsureActiveError(t *testing.T) {
	h, _, _, httpURL, done := newTestFFTokens(t)
	defer done()
	h.poolsToActivate = map[string][]*core.TokenPool{
		"ns1": {{Active: true}},
	}
	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/activatepool", httpURL),
		httpmock.NewJsonResponderOrPanic(500, fftypes.JSONObject{}))

	_, err := h.handleMessage(context.Background(), "ns1", []byte(`{"event":"started","data":{"namespace": "ns1"}}`))
	assert.NoError(t, err)
}

func TestHandlePoolActivated(t *testing.T) {
	h, _, _, _, done := newTestFFTokens(t)
	defer done()
	_, err := h.handleMessage(context.Background(), "ns1", []byte(`{"event":"activated","dat":{"namespace": "ns1"}}`))
	assert.NoError(t, err)
}

func TestConnectorName(t *testing.T) {
	h := &FFTokens{
		configuredName: "bob",
	}
	assert.Equal(t, h.ConnectorName(), "bob")
}
