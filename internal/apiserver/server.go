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
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftls"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/httpserver"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/internal/events/eifactory"
	"github.com/hyperledger-firefly/firefly/internal/events/websockets"
	"github.com/hyperledger-firefly/firefly/internal/metrics"
	"github.com/hyperledger-firefly/firefly/internal/namespace"
	"github.com/hyperledger-firefly/firefly/internal/orchestrator"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	spiConfig               = config.RootSection("spi")
	apiConfig               = config.RootSection("http")
	deprecatedMetricsConfig = config.RootSection("metrics")
	monitoringConfig        = config.RootSection("monitoring")
	corsConfig              = config.RootSection("cors")
)

// Server is the external interface for the API Server
type Server interface {
	Serve(ctx context.Context, mgr namespace.Manager) error
}

type apiServer struct {
	// Defaults set with config
	apiTimeout               time.Duration
	apiMaxTimeout            time.Duration
	deprecatedMetricsEnabled bool
	monitoringEnabled        bool
	ffiSwaggerGen            FFISwaggerGen
	apiPublicURL             string
	dynamicPublicURLHeader   string
	defaultNamespace         string
}

func InitConfig() {
	httpserver.InitHTTPConfig(apiConfig, 5000)
	httpserver.InitHTTPConfig(spiConfig, 5001)
	httpserver.InitHTTPConfig(deprecatedMetricsConfig, 6000)
	httpserver.InitHTTPConfig(monitoringConfig, 6000)
	httpserver.InitCORSConfig(corsConfig)
	initDeprecatedMetricsConfig(deprecatedMetricsConfig)
	initMonitoringConfig(monitoringConfig)
}

func NewAPIServer() Server {
	as := &apiServer{
		apiTimeout:               config.GetDuration(coreconfig.APIRequestTimeout),
		apiMaxTimeout:            config.GetDuration(coreconfig.APIRequestMaxTimeout),
		dynamicPublicURLHeader:   config.GetString(coreconfig.APIDynamicPublicURLHeader),
		defaultNamespace:         config.GetString(coreconfig.NamespacesDefault),
		deprecatedMetricsEnabled: config.GetBool(coreconfig.DeprecatedMetricsEnabled),
		monitoringEnabled:        config.GetBool(coreconfig.MonitoringEnabled),
		ffiSwaggerGen:            &ffiSwaggerGen{},
	}
	as.apiPublicURL = as.getPublicURL(apiConfig, "")
	return as
}

// Serve is the main entry point for the API Server
func (as *apiServer) Serve(ctx context.Context, mgr namespace.Manager) (err error) {
	httpErrChan := make(chan error)
	spiErrChan := make(chan error)
	metricsErrChan := make(chan error)

	apiHTTPServer, err := httpserver.NewHTTPServer(ctx, "api", as.createMuxRouter(ctx, mgr), httpErrChan, apiConfig, corsConfig, &httpserver.ServerOptions{
		MaximumRequestTimeout: as.apiMaxTimeout,
	})
	if err != nil {
		return err
	}
	go apiHTTPServer.ServeHTTP(ctx)

	if config.GetBool(coreconfig.SPIEnabled) {
		spiHTTPServer, err := httpserver.NewHTTPServer(ctx, "spi", as.createAdminMuxRouter(mgr), spiErrChan, spiConfig, corsConfig, &httpserver.ServerOptions{
			MaximumRequestTimeout: as.apiMaxTimeout,
		})
		if err != nil {
			return err
		}
		go spiHTTPServer.ServeHTTP(ctx)
	} else if config.GetBool(coreconfig.LegacyAdminEnabled) {
		log.L(ctx).Warnf("Your config includes an 'admin' section, which should be renamed to 'spi' - SPI server will not be enabled until this is corrected")
	}
	if as.deprecatedMetricsEnabled || as.monitoringEnabled {
		var monitoringServer httpserver.HTTPServer
		var err error
		if as.monitoringEnabled {
			monitoringServer, err = httpserver.NewHTTPServer(ctx, "monitoring", as.createMonitoringMuxRouter(), metricsErrChan, monitoringConfig, corsConfig, &httpserver.ServerOptions{
				MaximumRequestTimeout: as.apiMaxTimeout,
			})
		} else {
			monitoringServer, err = httpserver.NewHTTPServer(ctx, "metrics", as.createMonitoringMuxRouter(), metricsErrChan, deprecatedMetricsConfig, corsConfig, &httpserver.ServerOptions{
				MaximumRequestTimeout: as.apiMaxTimeout,
			})
		}
		if err != nil {
			return err
		}
		go monitoringServer.ServeHTTP(ctx)
	}

	return as.waitForServerStop(httpErrChan, spiErrChan, metricsErrChan)
}

func (as *apiServer) waitForServerStop(httpErrChan, spiErrChan, metricsErrChan chan error) error {
	select {
	case err := <-httpErrChan:
		return err
	case err := <-spiErrChan:
		return err
	case err := <-metricsErrChan:
		return err
	}
}

func (as *apiServer) getPublicURL(conf config.Section, pathPrefix string) string {
	publicURL := strings.TrimSuffix(conf.GetString(httpserver.HTTPConfPublicURL), "/")
	if publicURL == "" {
		proto := "https"
		tlsSection := conf.SubSection("tls")
		if !tlsSection.GetBool(fftls.HTTPConfTLSEnabled) {
			proto = "http"
		}
		publicURL = fmt.Sprintf("%s://%s:%s", proto, conf.GetString(httpserver.HTTPConfAddress), conf.GetString(httpserver.HTTPConfPort))
	}
	if pathPrefix != "" {
		publicURL += "/" + pathPrefix
	}
	return publicURL
}

func getOrchestrator(ctx context.Context, mgr namespace.Manager, tag string, r *ffapi.APIRequest) (or orchestrator.Orchestrator, err error) {
	switch tag {
	case routeTagDefaultNamespace:
		return mgr.Orchestrator(ctx, config.GetString(coreconfig.NamespacesDefault), false)
	case routeTagNonDefaultNamespace:
		vars := mux.Vars(r.Req)
		if ns, ok := vars["ns"]; ok {
			return mgr.Orchestrator(ctx, ns, false)
		}
	case routeTagGlobal:
		return nil, nil
	}
	return nil, i18n.NewError(ctx, coremsgs.MsgMissingNamespace)
}

func (as *apiServer) baseSwaggerGenOptions() ffapi.SwaggerGenOptions {
	return ffapi.SwaggerGenOptions{
		Title:                     "Hyperledger FireFly",
		Version:                   "1.0",
		PanicOnMissingDescription: config.GetBool(coreconfig.APIOASPanicOnMissingDescription),
		DefaultRequestTimeout:     config.GetDuration(coreconfig.APIRequestTimeout),
		APIDefaultFilterLimit:     config.GetString(coreconfig.APIDefaultFilterLimit),
		APIMaxFilterLimit:         config.GetUint(coreconfig.APIMaxFilterLimit),
		APIMaxFilterSkip:          config.GetUint(coreconfig.APIMaxFilterSkip),
	}
}

func (as *apiServer) getBaseURL(req *http.Request) string {
	var baseURL string
	if as.dynamicPublicURLHeader != "" {
		baseURL = req.Header.Get(as.dynamicPublicURLHeader)
		if baseURL != "" {
			return baseURL
		}
	}
	baseURL = strings.TrimSuffix(as.apiPublicURL, "/") + "/api/v1"
	vars := mux.Vars(req)
	if ns, ok := vars["ns"]; ok && ns != "" {
		baseURL += `/namespaces/` + ns
	} else {
		// Use the default namespace
		baseURL += `/namespaces/` + as.defaultNamespace
	}

	return baseURL
}

func (as *apiServer) routeHandler(hf *ffapi.HandlerFactory, mgr namespace.Manager, fixedBaseURL string, route *ffapi.Route) http.HandlerFunc {
	// We extend the base ffapi functionality, with standardized DB filter support for all core resources.
	// We also pass the Orchestrator context through
	ce := route.Extensions.(*coreExtensions)
	route.JSONHandler = func(r *ffapi.APIRequest) (output interface{}, err error) {
		or, err := getOrchestrator(r.Req.Context(), mgr, route.Tag, r)
		if err != nil {
			return nil, err
		}

		// Authorize the request
		authReq := &fftypes.AuthReq{
			Method: r.Req.Method,
			URL:    r.Req.URL,
			Header: r.Req.Header,
		}
		if or != nil {
			if err := or.Authorize(r.Req.Context(), authReq); err != nil {
				return nil, err
			}
		}

		if ce.EnabledIf != nil && !ce.EnabledIf(or) {
			return nil, i18n.NewError(r.Req.Context(), coremsgs.MsgActionNotSupported)
		}

		apiBaseURL := fixedBaseURL // for SPI
		if apiBaseURL == "" {
			apiBaseURL = as.getBaseURL(r.Req)
		}
		cr := &coreRequest{
			mgr:        mgr,
			or:         or,
			ctx:        r.Req.Context(),
			apiBaseURL: apiBaseURL,
		}
		return ce.CoreJSONHandler(r, cr)
	}
	if ce.CoreFormUploadHandler != nil {
		route.FormUploadHandler = func(r *ffapi.APIRequest) (output interface{}, err error) {
			or, err := getOrchestrator(r.Req.Context(), mgr, route.Tag, r)
			if err != nil {
				return nil, err
			}
			if ce.EnabledIf != nil && !ce.EnabledIf(or) {
				return nil, i18n.NewError(r.Req.Context(), coremsgs.MsgActionNotSupported)
			}

			apiBaseURL := fixedBaseURL // for SPI
			if apiBaseURL == "" {
				apiBaseURL = as.getBaseURL(r.Req)
			}
			cr := &coreRequest{
				mgr:        mgr,
				or:         or,
				ctx:        r.Req.Context(),
				apiBaseURL: apiBaseURL,
			}
			return ce.CoreFormUploadHandler(r, cr)
		}
	}
	return hf.RouteHandler(route)
}

func (as *apiServer) handlerFactory() *ffapi.HandlerFactory {
	return &ffapi.HandlerFactory{
		DefaultFilterLimit:    uint64(config.GetUint(coreconfig.APIDefaultFilterLimit)),
		MaxFilterLimit:        uint64(config.GetUint(coreconfig.APIMaxFilterLimit)),
		MaxFilterSkip:         uint64(config.GetUint(coreconfig.APIMaxFilterSkip)),
		DefaultRequestTimeout: config.GetDuration(coreconfig.APIRequestTimeout),
		MaxTimeout:            config.GetDuration(coreconfig.APIRequestMaxTimeout),
		PassthroughHeaders:    config.GetStringSlice(coreconfig.APIPassthroughHeaders),
	}
}

// For namespace relative APIs, we add the resolved namespace to the public URL of the swagger
// generator (so it can be fully replaced by the API Gateway routing HTTP Header),
// and generate the API itself as if it's root is at the namespace level.
//
// This gives a clean namespace scoped swagger for apps interested in just working with
// a single namespace.
func (as *apiServer) nsOpenAPIHandlerFactory(req *http.Request, publicURL string) *ffapi.OpenAPIHandlerFactory {
	vars := mux.Vars(req)
	return &ffapi.OpenAPIHandlerFactory{
		BaseSwaggerGenOptions:  as.baseSwaggerGenOptions(),
		StaticPublicURL:        publicURL + "/api/v1/namespaces/" + vars["ns"],
		DynamicPublicURLHeader: as.dynamicPublicURLHeader,
	}
}

func (as *apiServer) namespacedSwaggerHandler(hf *ffapi.HandlerFactory, r *mux.Router, publicURL, relativePath string, format ffapi.OpenAPIFormat) {
	r.HandleFunc(`/api/v1/namespaces/{ns}`+relativePath, hf.APIWrapper(func(res http.ResponseWriter, req *http.Request) (status int, err error) {
		return as.nsOpenAPIHandlerFactory(req, publicURL).OpenAPIHandler("", ffapi.OpenAPIFormatJSON, nsRoutes)(res, req)
	}))
}

func (as *apiServer) namespacedSwaggerUI(hf *ffapi.HandlerFactory, r *mux.Router, publicURL, relativePath string) {
	r.HandleFunc(`/api/v1/namespaces/{ns}`+relativePath, hf.APIWrapper(func(res http.ResponseWriter, req *http.Request) (status int, err error) {
		return as.nsOpenAPIHandlerFactory(req, publicURL).SwaggerUIHandler(`/api/openapi.yaml`)(res, req)
	}))
}

func (as *apiServer) namespacedContractSwaggerGenerator(hf *ffapi.HandlerFactory, r *mux.Router, mgr namespace.Manager, publicURL, relativePath string, format ffapi.OpenAPIFormat) {
	r.HandleFunc(`/api/v1/namespaces/{ns}/apis/{apiName}`+relativePath, hf.APIWrapper(func(res http.ResponseWriter, req *http.Request) (status int, err error) {
		vars := mux.Vars(req)
		or, err := mgr.Orchestrator(req.Context(), vars["ns"], false)
		if err != nil {
			return -1, err
		}
		apiBaseURL := as.getBaseURL(req)
		cm := or.Contracts()
		api, err := cm.GetContractAPI(req.Context(), apiBaseURL, vars["apiName"])
		if err != nil {
			return -1, err
		} else if api == nil || api.Interface == nil {
			return -1, i18n.NewError(req.Context(), coremsgs.Msg404NoResult)
		}

		ffi, err := cm.GetFFIByIDWithChildren(req.Context(), api.Interface.ID)
		if err != nil {
			return -1, err
		}

		options, routes := as.ffiSwaggerGen.Build(req.Context(), api, ffi)
		return (&ffapi.OpenAPIHandlerFactory{
			BaseSwaggerGenOptions:  *options,
			StaticPublicURL:        apiBaseURL,
			DynamicPublicURLHeader: as.dynamicPublicURLHeader,
		}).OpenAPIHandler(fmt.Sprintf("/apis/%s", vars["apiName"]), format, routes)(res, req)
	}))
}

func (as *apiServer) namespacedContractSwaggerUI(hf *ffapi.HandlerFactory, r *mux.Router, publicURL, relativePath string) {
	r.HandleFunc(`/api/v1/namespaces/{ns}/apis/{apiName}`+relativePath, hf.APIWrapper(func(res http.ResponseWriter, req *http.Request) (status int, err error) {
		vars := mux.Vars(req)
		oaf := &ffapi.OpenAPIHandlerFactory{
			StaticPublicURL:        publicURL + "/api/v1/namespaces/" + vars["ns"],
			DynamicPublicURLHeader: as.dynamicPublicURLHeader,
		}
		return oaf.SwaggerUIHandler(`/apis/`+vars["apiName"]+`/api/openapi.yaml`)(res, req)
	}))
}

func (as *apiServer) createMuxRouter(ctx context.Context, mgr namespace.Manager) *mux.Router {
	r := mux.NewRouter()
	hf := as.handlerFactory()

	if as.deprecatedMetricsEnabled || as.monitoringEnabled {
		r.Use(metrics.GetRestServerInstrumentation().Middleware)
	}

	for _, route := range routes {
		if ce, ok := route.Extensions.(*coreExtensions); ok {
			if ce.CoreJSONHandler != nil {
				r.HandleFunc(fmt.Sprintf("/api/v1/%s", route.Path), as.routeHandler(hf, mgr, "", route)).
					Methods(route.Method)
			}
		}
	}

	// Swagger builder for the root
	oaf := &ffapi.OpenAPIHandlerFactory{
		BaseSwaggerGenOptions:  as.baseSwaggerGenOptions(),
		StaticPublicURL:        as.apiPublicURL,
		DynamicPublicURLHeader: as.dynamicPublicURLHeader,
	}

	// Root APIs
	r.HandleFunc(`/api/swagger.json`, hf.APIWrapper(oaf.OpenAPIHandler(`/api/v1`, ffapi.OpenAPIFormatJSON, routes)))
	r.HandleFunc(`/api/openapi.json`, hf.APIWrapper(oaf.OpenAPIHandler(`/api/v1`, ffapi.OpenAPIFormatJSON, routes)))
	r.HandleFunc(`/api/swagger.yaml`, hf.APIWrapper(oaf.OpenAPIHandler(`/api/v1`, ffapi.OpenAPIFormatYAML, routes)))
	r.HandleFunc(`/api/openapi.yaml`, hf.APIWrapper(oaf.OpenAPIHandler(`/api/v1`, ffapi.OpenAPIFormatYAML, routes)))
	r.HandleFunc(`/api`, hf.APIWrapper(oaf.SwaggerUIHandler(`/api/openapi.yaml`)))
	// Namespace relative APIs
	as.namespacedSwaggerHandler(hf, r, as.apiPublicURL, `/api/swagger.json`, ffapi.OpenAPIFormatJSON)
	as.namespacedSwaggerHandler(hf, r, as.apiPublicURL, `/api/openapi.json`, ffapi.OpenAPIFormatJSON)
	as.namespacedSwaggerHandler(hf, r, as.apiPublicURL, `/api/swagger.yaml`, ffapi.OpenAPIFormatYAML)
	as.namespacedSwaggerHandler(hf, r, as.apiPublicURL, `/api/openapi.yaml`, ffapi.OpenAPIFormatYAML)
	as.namespacedSwaggerUI(hf, r, as.apiPublicURL, `/api`)
	// Dynamic swagger for namespaced contract APIs
	as.namespacedContractSwaggerGenerator(hf, r, mgr, as.apiPublicURL, `/api/swagger.json`, ffapi.OpenAPIFormatJSON)
	as.namespacedContractSwaggerGenerator(hf, r, mgr, as.apiPublicURL, `/api/openapi.json`, ffapi.OpenAPIFormatJSON)
	as.namespacedContractSwaggerGenerator(hf, r, mgr, as.apiPublicURL, `/api/swagger.yaml`, ffapi.OpenAPIFormatYAML)
	as.namespacedContractSwaggerGenerator(hf, r, mgr, as.apiPublicURL, `/api/openapi.yaml`, ffapi.OpenAPIFormatYAML)
	as.namespacedContractSwaggerUI(hf, r, as.apiPublicURL, `/api`)

	r.HandleFunc(`/favicon{any:.*}.png`, favIcons)

	ws, _ := eifactory.GetPlugin(ctx, "websockets")
	ws.(*websockets.WebSockets).SetAuthorizer(mgr)
	r.HandleFunc(`/ws`, ws.(*websockets.WebSockets).ServeHTTP)

	// namespace scoped web sockets
	r.HandleFunc("/api/v1/namespaces/{ns}/ws", hf.APIWrapper(getNamespacedWebSocketHandler(ws.(*websockets.WebSockets), mgr)))

	uiPath := config.GetString(coreconfig.UIPath)
	if uiPath != "" && config.GetBool(coreconfig.UIEnabled) {
		r.PathPrefix(`/ui`).Handler(newStaticHandler(uiPath, "index.html", `/ui`))
	}

	r.NotFoundHandler = hf.APIWrapper(as.notFoundHandler)
	return r
}

func getNamespacedWebSocketHandler(ws websockets.WebSocketsNamespaced, mgr namespace.Manager) ffapi.HandlerFunction {
	return func(res http.ResponseWriter, req *http.Request) (status int, err error) {

		vars := mux.Vars(req)
		namespace := vars["ns"]
		or, err := mgr.Orchestrator(req.Context(), namespace, false)
		if err != nil || or == nil {
			return 404, i18n.NewError(req.Context(), coremsgs.Msg404NotFound)
		}

		ws.ServeHTTPNamespaced(namespace, res, req)

		return 200, nil
	}

}

func (as *apiServer) notFoundHandler(res http.ResponseWriter, req *http.Request) (status int, err error) {
	res.Header().Add("Content-Type", "application/json")
	return 404, i18n.NewError(req.Context(), coremsgs.Msg404NotFound)
}

func (as *apiServer) spiWSHandler(mgr namespace.Manager) http.HandlerFunc {
	// The SPI events listener will be initialized when we start, so we access it it from Orchestrator on demand
	return func(w http.ResponseWriter, r *http.Request) {
		mgr.SPIEvents().ServeHTTPWebSocketListener(w, r)
	}
}

func (as *apiServer) createAdminMuxRouter(mgr namespace.Manager) *mux.Router {
	r := mux.NewRouter()
	if as.deprecatedMetricsEnabled || as.monitoringEnabled {
		r.Use(metrics.GetAdminServerInstrumentation().Middleware)
	}
	hf := as.handlerFactory()

	publicURL := as.getPublicURL(spiConfig, "spi")
	spiBaseURL := fmt.Sprintf("%s/v1", publicURL)
	for _, route := range spiRoutes {
		if ce, ok := route.Extensions.(*coreExtensions); ok {
			if ce.CoreJSONHandler != nil {
				r.HandleFunc(fmt.Sprintf("/spi/v1/%s", route.Path), as.routeHandler(hf, mgr, spiBaseURL, route)).
					Methods(route.Method)
			}
		}
	}

	// Swagger for SPI
	oaf := &ffapi.OpenAPIHandlerFactory{
		BaseSwaggerGenOptions:  as.baseSwaggerGenOptions(),
		StaticPublicURL:        publicURL,
		DynamicPublicURLHeader: as.dynamicPublicURLHeader,
	}
	r.HandleFunc(`/spi/swagger.json`, hf.APIWrapper(oaf.OpenAPIHandler(`/spi/v1`, ffapi.OpenAPIFormatJSON, spiRoutes)))
	r.HandleFunc(`/spi/openapi.json`, hf.APIWrapper(oaf.OpenAPIHandler(`/spi/v1`, ffapi.OpenAPIFormatJSON, spiRoutes)))
	r.HandleFunc(`/spi/swagger.yaml`, hf.APIWrapper(oaf.OpenAPIHandler(`/spi/v1`, ffapi.OpenAPIFormatYAML, spiRoutes)))
	r.HandleFunc(`/spi/openapi.yaml`, hf.APIWrapper(oaf.OpenAPIHandler(`/spi/v1`, ffapi.OpenAPIFormatYAML, spiRoutes)))
	r.HandleFunc(`/spi`, hf.APIWrapper(oaf.SwaggerUIHandler(`/spi/openapi.yaml`)))

	r.HandleFunc(`/favicon{any:.*}.png`, favIcons)

	r.HandleFunc(`/spi/ws`, as.spiWSHandler(mgr))

	return r
}

func (as *apiServer) createMonitoringMuxRouter() *mux.Router {
	r := mux.NewRouter()
	metricsPath := config.GetString(coreconfig.DeprecatedMetricsPath)
	if as.monitoringEnabled {
		metricsPath = config.GetString(coreconfig.MonitoringMetricsPath)
	}
	r.Path(metricsPath).Handler(promhttp.InstrumentMetricHandler(metrics.Registry(),
		promhttp.HandlerFor(metrics.Registry(), promhttp.HandlerOpts{})))
	hf := ffapi.HandlerFactory{}
	r.HandleFunc("/livez", hf.APIWrapper(func(res http.ResponseWriter, req *http.Request) (status int, err error) {
		// a simple liveness check
		return http.StatusOK, nil
	}))
	r.NotFoundHandler = hf.APIWrapper(as.notFoundHandler)
	return r
}

func syncRetcode(isSync bool) int {
	if isSync {
		return http.StatusOK
	}
	return http.StatusAccepted
}
