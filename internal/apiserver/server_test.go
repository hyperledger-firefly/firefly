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

package apiserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/go-resty/resty/v2"
	"github.com/gorilla/mux"
	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/httpserver"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/internal/metrics"
	"github.com/hyperledger-firefly/firefly/mocks/apiservermocks"
	"github.com/hyperledger-firefly/firefly/mocks/contractmocks"
	"github.com/hyperledger-firefly/firefly/mocks/namespacemocks"
	"github.com/hyperledger-firefly/firefly/mocks/orchestratormocks"
	"github.com/hyperledger-firefly/firefly/mocks/spieventsmocks"
	"github.com/hyperledger-firefly/firefly/mocks/websocketsmocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const configDir = "../../test/data/config"

func newTestServer() (*namespacemocks.Manager, *orchestratormocks.Orchestrator, *apiServer) {
	coreconfig.Reset()
	InitConfig()
	mgr := &namespacemocks.Manager{}
	o := &orchestratormocks.Orchestrator{}
	mgr.On("Orchestrator", mock.Anything, "default", false).Return(o, nil).Maybe()
	mgr.On("Orchestrator", mock.Anything, "mynamespace", false).Return(o, nil).Maybe()
	mgr.On("Orchestrator", mock.Anything, "ns1", false).Return(o, nil).Maybe()
	config.Set(coreconfig.APIMaxFilterLimit, 100)
	as := NewAPIServer().(*apiServer)
	return mgr, o, as
}

func newTestAPIServer() (*orchestratormocks.Orchestrator, *mux.Router) {
	mgr, o, as := newTestServer()
	r := as.createMuxRouter(context.Background(), mgr)
	return o, r
}

func newTestSPIServer() (*orchestratormocks.Orchestrator, *mux.Router) {
	config.Set(coreconfig.NamespacesDefault, "default")
	mgr, o, as := newTestServer()
	mae := &spieventsmocks.Manager{}
	mgr.On("SPIEvents").Return(mae)
	r := as.createAdminMuxRouter(mgr)
	return o, r
}

func TestStartStopServer(t *testing.T) {
	coreconfig.Reset()
	metrics.Clear()
	InitConfig()
	apiConfig.Set(httpserver.HTTPConfPort, 0)
	spiConfig.Set(httpserver.HTTPConfPort, 0)
	monitoringConfig.Set(httpserver.HTTPConfPort, 0)
	config.Set(coreconfig.UIPath, "test")
	config.Set(coreconfig.SPIEnabled, true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // server will immediately shut down
	as := NewAPIServer()
	mgr := &namespacemocks.Manager{}
	mae := &spieventsmocks.Manager{}
	mgr.On("SPIEvents").Return(mae)
	err := as.Serve(ctx, mgr)
	assert.NoError(t, err)
}

func TestListeningClosedOnceBound(t *testing.T) {
	coreconfig.Reset()
	metrics.Clear()
	InitConfig()
	apiConfig.Set(httpserver.HTTPConfPort, 0)
	spiConfig.Set(httpserver.HTTPConfPort, 0)
	monitoringConfig.Set(httpserver.HTTPConfPort, 0)
	config.Set(coreconfig.UIPath, "test")
	ctx, cancel := context.WithCancel(context.Background())
	as := NewAPIServer()
	mgr := &namespacemocks.Manager{}

	select {
	case <-as.Listening():
		assert.Fail(t, "listening before Serve was called")
	default:
	}

	serveDone := make(chan error, 1)
	go func() {
		serveDone <- as.Serve(ctx, mgr)
	}()

	select {
	case <-as.Listening():
	case err := <-serveDone:
		assert.Fail(t, "Serve returned before listening", "%v", err)
	case <-time.After(5 * time.Second):
		assert.Fail(t, "never listening")
	}

	cancel()
	assert.NoError(t, <-serveDone)
}

func TestListeningNotClosedOnBindFailure(t *testing.T) {
	coreconfig.Reset()
	metrics.Clear()
	InitConfig()
	apiConfig.Set(httpserver.HTTPConfAddress, "...://")
	as := NewAPIServer()
	err := as.Serve(context.Background(), &namespacemocks.Manager{})
	assert.Regexp(t, "FF00151", err)

	select {
	case <-as.Listening():
		assert.Fail(t, "listening after a failure to bind")
	default:
	}
}

func TestStartLegacyAdminConfig(t *testing.T) {
	coreconfig.Reset()
	metrics.Clear()
	InitConfig()
	apiConfig.Set(httpserver.HTTPConfPort, 0)
	spiConfig.Set(httpserver.HTTPConfPort, 0)
	monitoringConfig.Set(httpserver.HTTPConfPort, 0)
	config.Set(coreconfig.UIPath, "test")
	config.Set(coreconfig.LegacyAdminEnabled, true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // server will immediately shut down
	as := NewAPIServer()
	mgr := &namespacemocks.Manager{}
	mae := &spieventsmocks.Manager{}
	mgr.On("SPIEvents").Return(mae)
	err := as.Serve(ctx, mgr)
	assert.NoError(t, err)
}

func TestStartAPIFail(t *testing.T) {
	coreconfig.Reset()
	metrics.Clear()
	InitConfig()
	apiConfig.Set(httpserver.HTTPConfAddress, "...://")
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // server will immediately shut down
	as := NewAPIServer()
	mgr := &namespacemocks.Manager{}
	err := as.Serve(ctx, mgr)
	assert.Regexp(t, "FF00151", err)
}

func TestStartAdminFail(t *testing.T) {
	coreconfig.Reset()
	metrics.Clear()
	InitConfig()
	spiConfig.Set(httpserver.HTTPConfAddress, "...://")
	config.Set(coreconfig.SPIEnabled, true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // server will immediately shut down
	as := NewAPIServer()
	mgr := &namespacemocks.Manager{}
	mae := &spieventsmocks.Manager{}
	mgr.On("SPIEvents").Return(mae)
	err := as.Serve(ctx, mgr)
	assert.Regexp(t, "FF00151", err)
}

func TestStartAdminWSHandler(t *testing.T) {
	coreconfig.Reset()
	metrics.Clear()
	InitConfig()
	spiConfig.Set(httpserver.HTTPConfAddress, "...://")
	config.Set(coreconfig.SPIEnabled, true)
	as := NewAPIServer().(*apiServer)
	mgr := &namespacemocks.Manager{}
	mae := &spieventsmocks.Manager{}
	mgr.On("SPIEvents").Return(mae)
	mae.On("ServeHTTPWebSocketListener", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		res := args[0].(http.ResponseWriter)
		res.WriteHeader(200)
	}).Return()
	res := httptest.NewRecorder()
	as.spiWSHandler(mgr).ServeHTTP(res, httptest.NewRequest("GET", "/", nil))
	assert.Equal(t, 200, res.Result().StatusCode)
}

func TestStartMetricsFail(t *testing.T) {
	coreconfig.Reset()
	metrics.Clear()
	InitConfig()
	monitoringConfig.Set(httpserver.HTTPConfAddress, "...://")
	config.Set(coreconfig.MonitoringEnabled, true)
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // server will immediately shut down
	as := NewAPIServer()
	mgr := &namespacemocks.Manager{}
	mae := &spieventsmocks.Manager{}
	mgr.On("SPIEvents").Return(mae)
	err := as.Serve(ctx, mgr)
	assert.Regexp(t, "FF00151", err)
}

func TestNotFound(t *testing.T) {
	_, _, as := newTestServer()
	handler := as.handlerFactory().APIWrapper(as.notFoundHandler)
	s := httptest.NewServer(http.HandlerFunc(handler))
	defer s.Close()

	res, err := http.Get(fmt.Sprintf("http://%s/test", s.Listener.Addr()))
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
	var resJSON map[string]interface{}
	json.NewDecoder(res.Body).Decode(&resJSON)
	assert.Regexp(t, "FF10109", resJSON["error"])
}

func TestFilterTooMany(t *testing.T) {
	mgr, o, as := newTestServer()
	o.On("Authorize", mock.Anything, mock.Anything).Return(nil)
	handler := as.routeHandler(as.handlerFactory(), mgr, "", getBatches)

	req := httptest.NewRequest("GET", "http://localhost:12345/test?limit=99999999999", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	assert.Equal(t, 400, res.Result().StatusCode)
	var resJSON map[string]interface{}
	json.NewDecoder(res.Body).Decode(&resJSON)
	assert.Regexp(t, "FF00192", resJSON["error"])
}

func TestUnauthorized(t *testing.T) {
	mgr, o, as := newTestServer()
	o.On("Authorize", mock.Anything, mock.Anything).Return(i18n.NewError(context.Background(), i18n.MsgUnauthorized))
	handler := as.routeHandler(as.handlerFactory(), mgr, "", getBatches)

	req := httptest.NewRequest("GET", "http://localhost:12345/test", nil)
	res := httptest.NewRecorder()
	handler.ServeHTTP(res, req)
	assert.Equal(t, 401, res.Result().StatusCode)
	var resJSON map[string]interface{}
	json.NewDecoder(res.Body).Decode(&resJSON)
	assert.Regexp(t, "FF00169", resJSON["error"])
}

func TestSwaggerJSON(t *testing.T) {
	o, r := newTestAPIServer()
	o.On("Authorize", mock.Anything, mock.Anything).Return(nil)
	s := httptest.NewServer(r)
	defer s.Close()

	res, err := http.Get(fmt.Sprintf("http://%s/api/swagger.json", s.Listener.Addr()))
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	b, _ := io.ReadAll(res.Body)
	err = json.Unmarshal(b, &openapi3.T{})
	assert.NoError(t, err)
}

func TestNamespacedSwaggerJSON(t *testing.T) {
	o, r := newTestAPIServer()
	o.On("Authorize", mock.Anything, mock.Anything).Return(nil)
	s := httptest.NewServer(r)
	defer s.Close()

	res, err := http.Get(fmt.Sprintf("http://%s/api/v1/namespaces/test/api/swagger.json", s.Listener.Addr()))
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	b, _ := io.ReadAll(res.Body)
	err = json.Unmarshal(b, &openapi3.T{})
	assert.NoError(t, err)
}

func TestNamespacedSwaggerUI(t *testing.T) {
	mgr, o, as := newTestServer()
	r := as.createMuxRouter(context.Background(), mgr)
	o.On("Authorize", mock.Anything, mock.Anything).Return(nil)
	s := httptest.NewServer(r)
	defer s.Close()

	res, err := resty.New().R().
		SetDoNotParseResponse(true).
		Get(fmt.Sprintf("http://%s/api/v1/namespaces/test/api", s.Listener.Addr()))
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode())
	b, _ := io.ReadAll(res.RawBody())
	assert.Contains(t, string(b), "http://127.0.0.1:5000/api/v1/namespaces/test/api/openapi.yaml")
}

func TestNamespacedSwaggerUIRewrite(t *testing.T) {
	mgr, o, as := newTestServer()
	r := as.createMuxRouter(context.Background(), mgr)
	o.On("Authorize", mock.Anything, mock.Anything).Return(nil)
	s := httptest.NewServer(r)
	defer s.Close()
	as.dynamicPublicURLHeader = "X-API-Rewrite"

	res, err := resty.New().R().
		SetDoNotParseResponse(true).
		SetHeader("X-API-Rewrite", "https://myhost.example.com/mypath/to/namespace/").
		Get(fmt.Sprintf("http://%s/api/v1/namespaces/test/api", s.Listener.Addr()))
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode())
	b, _ := io.ReadAll(res.RawBody())
	assert.Contains(t, string(b), "https://myhost.example.com/mypath/to/namespace/api/openapi.yaml")
}

func TestSwaggerAdminJSON(t *testing.T) {
	_, r := newTestSPIServer()
	s := httptest.NewServer(r)
	defer s.Close()

	res, err := http.Get(fmt.Sprintf("http://%s/spi/swagger.json", s.Listener.Addr()))
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	b, _ := io.ReadAll(res.Body)
	err = json.Unmarshal(b, &openapi3.T{})
	assert.NoError(t, err)
}

func TestWaitForServerStop(t *testing.T) {

	chl1 := make(chan error, 1)
	chl2 := make(chan error, 1)
	chl3 := make(chan error, 1)
	chl1 <- fmt.Errorf("pop1")

	as := &apiServer{}
	err := as.waitForServerStop(chl1, chl2, chl3)
	assert.EqualError(t, err, "pop1")

	chl2 <- fmt.Errorf("pop2")
	err = as.waitForServerStop(chl1, chl2, chl3)
	assert.EqualError(t, err, "pop2")

	chl3 <- fmt.Errorf("pop3")
	err = as.waitForServerStop(chl1, chl2, chl3)
	assert.EqualError(t, err, "pop3")
}

func TestContractAPISwaggerJSON(t *testing.T) {
	mgr, o, as := newTestServer()
	r := as.createMuxRouter(context.Background(), mgr)
	mcm := &contractmocks.Manager{}
	o.On("Contracts").Return(mcm)
	mffi := apiservermocks.NewFFISwaggerGen(t)
	as.ffiSwaggerGen = mffi
	s := httptest.NewServer(r)
	defer s.Close()

	ffi := &fftypes.FFI{}
	api := &core.ContractAPI{
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
	}
	as.dynamicPublicURLHeader = "X-API-BaseURL"

	mcm.On("GetContractAPI", mock.Anything, "http://mydomain.com/path/to/default", "my-api").Return(api, nil)
	mcm.On("GetFFIByIDWithChildren", mock.Anything, api.Interface.ID).Return(ffi, nil)
	mffi.On("Build", mock.Anything, api, ffi).Return(&ffapi.SwaggerGenOptions{}, []*ffapi.Route{})

	res, err := resty.New().R().
		SetHeader("X-API-BaseURL", "http://mydomain.com/path/to/default").
		Get(fmt.Sprintf("http://%s/api/v1/namespaces/default/apis/my-api/api/swagger.json", s.Listener.Addr()))
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode())
}

func TestContractAPISwaggerJSONGetAPIFail(t *testing.T) {
	mgr, o, as := newTestServer()
	r := as.createMuxRouter(context.Background(), mgr)
	mcm := &contractmocks.Manager{}
	o.On("Contracts").Return(mcm)
	s := httptest.NewServer(r)
	defer s.Close()

	mcm.On("GetContractAPI", mock.Anything, "http://127.0.0.1:5000/api/v1/namespaces/default", "my-api").Return(nil, fmt.Errorf("pop"))

	res, err := http.Get(fmt.Sprintf("http://%s/api/v1/namespaces/default/apis/my-api/api/swagger.json", s.Listener.Addr()))
	assert.NoError(t, err)
	assert.Equal(t, 500, res.StatusCode)
}

func TestContractAPISwaggerJSONGetAPINotFound(t *testing.T) {
	mgr, o, as := newTestServer()
	r := as.createMuxRouter(context.Background(), mgr)
	mcm := &contractmocks.Manager{}
	o.On("Contracts").Return(mcm)
	s := httptest.NewServer(r)
	defer s.Close()

	mcm.On("GetContractAPI", mock.Anything, "http://127.0.0.1:5000/api/v1/namespaces/default", "my-api").Return(nil, nil)

	res, err := http.Get(fmt.Sprintf("http://%s/api/v1/namespaces/default/apis/my-api/api/swagger.json", s.Listener.Addr()))
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func TestContractAPISwaggerJSONGetFFIFail(t *testing.T) {
	mgr, o, as := newTestServer()
	r := as.createMuxRouter(context.Background(), mgr)
	mcm := &contractmocks.Manager{}
	o.On("Contracts").Return(mcm)
	s := httptest.NewServer(r)
	defer s.Close()

	api := &core.ContractAPI{
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
	}

	mcm.On("GetContractAPI", mock.Anything, "http://127.0.0.1:5000/api/v1/namespaces/default", "my-api").Return(api, nil)
	mcm.On("GetFFIByIDWithChildren", mock.Anything, api.Interface.ID).Return(nil, fmt.Errorf("pop"))

	res, err := http.Get(fmt.Sprintf("http://%s/api/v1/namespaces/default/apis/my-api/api/swagger.json", s.Listener.Addr()))
	assert.NoError(t, err)
	assert.Equal(t, 500, res.StatusCode)
}

func TestContractAPISwaggerJSONBadNamespace(t *testing.T) {
	mgr, o, as := newTestServer()
	r := as.createMuxRouter(context.Background(), mgr)
	mcm := &contractmocks.Manager{}
	o.On("Contracts").Return(mcm)
	s := httptest.NewServer(r)
	defer s.Close()

	mgr.On("Orchestrator", mock.Anything, "BAD", false).Return(nil, i18n.NewError(context.Background(), coremsgs.MsgUnknownNamespace))

	res, err := http.Get(fmt.Sprintf("http://%s/api/v1/namespaces/BAD/apis/my-api/api/swagger.json", s.Listener.Addr()))
	assert.NoError(t, err)
	assert.Equal(t, 404, res.StatusCode)
}

func TestContractAPISwaggerUI(t *testing.T) {
	o, r := newTestAPIServer()
	o.On("Authorize", mock.Anything, mock.Anything).Return(nil)
	s := httptest.NewServer(r)
	defer s.Close()

	res, err := http.Get(fmt.Sprintf("http://%s/api/v1/namespaces/default/apis/my-api/api", s.Listener.Addr()))
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
	b, _ := io.ReadAll(res.Body)
	assert.Regexp(t, "html", string(b))
}

func TestJSONBadNamespace(t *testing.T) {
	mgr, _, as := newTestServer()
	r := as.createMuxRouter(context.Background(), mgr)
	s := httptest.NewServer(r)
	defer s.Close()

	mgr.On("Orchestrator", mock.Anything, "BAD", false).Return(nil, i18n.NewError(context.Background(), coremsgs.MsgUnknownNamespace))

	var b bytes.Buffer
	req := httptest.NewRequest("GET", "/api/v1/namespaces/BAD/apis", &b)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	res := httptest.NewRecorder()

	r.ServeHTTP(res, req)

	assert.Equal(t, 404, res.Result().StatusCode)
}

func TestFormDataBadNamespace(t *testing.T) {
	mgr, _, as := newTestServer()
	r := as.createMuxRouter(context.Background(), mgr)
	s := httptest.NewServer(r)
	defer s.Close()

	mgr.On("Orchestrator", mock.Anything, "BAD", false).Return(nil, i18n.NewError(context.Background(), coremsgs.MsgUnknownNamespace))

	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	writer, err := w.CreateFormFile("file", "filename.ext")
	assert.NoError(t, err)
	writer.Write([]byte(`some data`))
	w.Close()
	req := httptest.NewRequest("POST", "/api/v1/namespaces/BAD/data", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	res := httptest.NewRecorder()

	r.ServeHTTP(res, req)

	assert.Equal(t, 404, res.Result().StatusCode)
}

func TestJSONDisabledRoute(t *testing.T) {
	mgr, o, as := newTestServer()
	o.On("Authorize", mock.Anything, mock.Anything).Return(nil)
	r := as.createMuxRouter(context.Background(), mgr)
	s := httptest.NewServer(r)
	defer s.Close()

	o.On("PrivateMessaging").Return(nil)

	var b bytes.Buffer
	req := httptest.NewRequest("GET", "/api/v1/namespaces/ns1/groups", &b)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	res := httptest.NewRecorder()

	r.ServeHTTP(res, req)

	assert.Equal(t, 400, res.Result().StatusCode)
}

func TestFormDataDisabledRoute(t *testing.T) {
	mgr, o, as := newTestServer()
	o.On("Authorize", mock.Anything, mock.Anything).Return(nil)
	o.On("Data").Return(nil)
	r := as.createMuxRouter(context.Background(), mgr)
	s := httptest.NewServer(r)
	defer s.Close()

	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	writer, err := w.CreateFormFile("file", "filename.ext")
	assert.NoError(t, err)
	writer.Write([]byte(`some data`))
	w.Close()
	req := httptest.NewRequest("POST", "/api/v1/namespaces/ns1/data", &b)
	req.Header.Set("Content-Type", w.FormDataContentType())
	res := httptest.NewRecorder()

	r.ServeHTTP(res, req)

	assert.Equal(t, 400, res.Result().StatusCode)
}

func TestGetOrchestratorMissingTag(t *testing.T) {
	_, err := getOrchestrator(context.Background(), &namespacemocks.Manager{}, "", nil)
	assert.Regexp(t, "FF10437", err)
}

func TestGetNamespacedWebSocketHandler(t *testing.T) {
	mgr, _, _ := newTestServer()
	mwsns := &websocketsmocks.WebSocketsNamespaced{}
	mwsns.On("ServeHTTPNamespaced", "ns1", mock.Anything, mock.Anything).Return()

	var b bytes.Buffer
	req := httptest.NewRequest("GET", "/api/v1/namespaces/ns1/ws", &b)
	req = mux.SetURLVars(req, map[string]string{"ns": "ns1"})
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	res := httptest.NewRecorder()

	handler := getNamespacedWebSocketHandler(mwsns, mgr)
	status, err := handler(res, req)
	assert.NoError(t, err)
	assert.Equal(t, 200, status)
}

func TestGetNamespacedWebSocketHandlerUnknownNamespace(t *testing.T) {
	mgr, _, _ := newTestServer()
	mwsns := &websocketsmocks.WebSocketsNamespaced{}

	mgr.On("Orchestrator", mock.Anything, "unknown", false).Return(nil, errors.New("unknown namespace")).Maybe()
	var b bytes.Buffer
	req := httptest.NewRequest("GET", "/api/v1/namespaces/unknown/ws", &b)
	req = mux.SetURLVars(req, map[string]string{"ns": "unknown"})
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	res := httptest.NewRecorder()

	handler := getNamespacedWebSocketHandler(mwsns, mgr)
	status, err := handler(res, req)
	assert.Error(t, err)
	assert.Equal(t, 404, status)
}

func TestContractAPIDefaultNS(t *testing.T) {
	mgr, o, as := newTestServer()
	r := as.createMuxRouter(context.Background(), mgr)
	mcm := &contractmocks.Manager{}
	o.On("Contracts").Return(mcm)
	mffi := apiservermocks.NewFFISwaggerGen(t)
	as.ffiSwaggerGen = mffi
	s := httptest.NewServer(r)
	defer s.Close()

	o.On("Authorize", mock.Anything, mock.Anything).Return(nil)

	api := &core.ContractAPI{
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
	}

	mcm.On("GetContractAPIs", mock.Anything, "http://127.0.0.1:5000/api/v1/namespaces/default", mock.Anything).Return([]*core.ContractAPI{api}, nil, nil)

	res, err := resty.New().R().
		Get(fmt.Sprintf("http://%s/api/v1/apis", s.Listener.Addr()))
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode())
}

func TestMonitoringServerRoutes(t *testing.T) {
	_, _, as := newTestServer()
	s := httptest.NewServer(as.createMonitoringMuxRouter(&namespacemocks.Manager{}))
	defer s.Close()

	res, err := http.Get(fmt.Sprintf("http://%s/livez", s.Listener.Addr()))
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)

	res, err = http.Get(fmt.Sprintf("http://%s/metrics", s.Listener.Addr()))
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
}

func getReady(t *testing.T, addr string) (int, *readyStatus) {
	res, err := http.Get(fmt.Sprintf("http://%s/readyz", addr))
	assert.NoError(t, err)
	defer res.Body.Close()
	assert.Equal(t, "application/json", res.Header.Get("Content-Type"))
	var status readyStatus
	assert.NoError(t, json.NewDecoder(res.Body).Decode(&status))
	return res.StatusCode, &status
}

func TestReadyzAllNamespacesStarted(t *testing.T) {
	mgr, _, as := newTestServer()
	mgr.On("GetNamespaces", mock.Anything, true).Return([]*core.NamespaceWithInitStatus{
		{Namespace: &core.Namespace{Name: "ns1"}},
		{Namespace: &core.Namespace{Name: "ns2"}},
	}, nil)

	for _, r := range []*mux.Router{as.createMuxRouter(context.Background(), mgr), as.createMonitoringMuxRouter(mgr)} {
		s := httptest.NewServer(r)
		code, status := getReady(t, s.Listener.Addr().String())
		s.Close()
		assert.Equal(t, 200, code)
		assert.True(t, status.Ready)
		assert.Empty(t, status.Initializing)
	}
}

func TestReadyzNoNamespaces(t *testing.T) {
	mgr, _, as := newTestServer()
	mgr.On("GetNamespaces", mock.Anything, true).Return([]*core.NamespaceWithInitStatus{}, nil)

	s := httptest.NewServer(as.createMonitoringMuxRouter(mgr))
	defer s.Close()
	code, status := getReady(t, s.Listener.Addr().String())
	assert.Equal(t, 200, code)
	assert.True(t, status.Ready)
}

func TestReadyzNamespaceInitializing(t *testing.T) {
	mgr, _, as := newTestServer()
	mgr.On("GetNamespaces", mock.Anything, true).Return([]*core.NamespaceWithInitStatus{
		{Namespace: &core.Namespace{Name: "ns1"}},
		{Namespace: &core.Namespace{Name: "ns2"}, Initializing: true, InitializationError: "pop"},
	}, nil)

	for _, r := range []*mux.Router{as.createMuxRouter(context.Background(), mgr), as.createMonitoringMuxRouter(mgr)} {
		s := httptest.NewServer(r)
		code, status := getReady(t, s.Listener.Addr().String())
		s.Close()
		assert.Equal(t, 503, code)
		assert.False(t, status.Ready)
		assert.Equal(t, []string{"ns2"}, status.Initializing)
	}
}

func TestReadyzManagerError(t *testing.T) {
	mgr, _, as := newTestServer()
	mgr.On("GetNamespaces", mock.Anything, true).Return(nil, fmt.Errorf("pop"))

	s := httptest.NewServer(as.createMonitoringMuxRouter(mgr))
	defer s.Close()
	code, status := getReady(t, s.Listener.Addr().String())
	assert.Equal(t, 503, code)
	assert.False(t, status.Ready)
}

func TestLivezUnaffectedByNamespaceState(t *testing.T) {
	mgr, _, as := newTestServer() // no GetNamespaces expectation: liveness must not depend on it
	s := httptest.NewServer(as.createMonitoringMuxRouter(mgr))
	defer s.Close()
	res, err := http.Get(fmt.Sprintf("http://%s/livez", s.Listener.Addr()))
	assert.NoError(t, err)
	assert.Equal(t, 200, res.StatusCode)
}
