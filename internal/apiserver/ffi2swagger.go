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

package apiserver

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

type FFISwaggerGen interface {
	Build(ctx context.Context, api *core.ContractAPI, ffi *fftypes.FFI) (*ffapi.SwaggerGenOptions, []*ffapi.Route)
}

type ContractListenerInput struct {
	Name    string                        `ffstruct:"ContractListener" json:"name,omitempty"`
	Topic   string                        `ffstruct:"ContractListener" json:"topic,omitempty"`
	Options *core.ContractListenerOptions `ffstruct:"ContractListener" json:"options,omitempty"`
}

type ContractListenerInputWithLocation struct {
	ContractListenerInput
	Location *fftypes.JSONAny `ffstruct:"ContractListener" json:"location,omitempty"`
}

type ffiSwaggerGen struct{}

func (swg *ffiSwaggerGen) Build(ctx context.Context, api *core.ContractAPI, ffi *fftypes.FFI) (*ffapi.SwaggerGenOptions, []*ffapi.Route) {
	hasLocation := !api.Location.IsNil()

	routes := []*ffapi.Route{
		{
			Name:            "interface",
			Path:            "interface", // must match a route defined in apiserver routes!
			Method:          http.MethodGet,
			JSONInputValue:  nil,
			JSONOutputValue: func() interface{} { return &fftypes.FFI{} },
			JSONOutputCodes: []int{http.StatusOK},
		},
	}
	for _, method := range ffi.Methods {
		routes = addFFIMethod(ctx, routes, method, hasLocation)
	}
	for _, event := range ffi.Events {
		routes = addFFIEvent(ctx, routes, event, hasLocation)
	}

	return &ffapi.SwaggerGenOptions{
		Title:                 ffi.Name,
		Version:               ffi.Version,
		Description:           ffi.Description,
		DefaultRequestTimeout: config.GetDuration(coreconfig.APIRequestTimeout),
	}, routes
}

func addFFIMethod(ctx context.Context, routes []*ffapi.Route, method *fftypes.FFIMethod, hasLocation bool) []*ffapi.Route {
	description := method.Description
	if len(method.Details) > 0 {
		additionalDetailsHeader := i18n.Expand(ctx, coremsgs.APISmartContractDetails)
		description = fmt.Sprintf("%s\n\n%s:\n\n%s", description, additionalDetailsHeader, buildDetailsTable(ctx, method.Details))
	}
	routes = append(routes, &ffapi.Route{
		Name:   fmt.Sprintf("invoke_%s", method.Pathname),
		Path:   fmt.Sprintf("invoke/%s", method.Pathname), // must match a route defined in apiserver routes!
		Method: http.MethodPost,
		QueryParams: []*ffapi.QueryParam{
			{Name: "confirm", Description: coremsgs.APIConfirmInvokeQueryParam, IsBool: true, Example: "true"},
		},
		JSONInputSchema: func(ctx context.Context, schemaGen ffapi.SchemaGenerator) (*openapi3.SchemaRef, error) {
			return contractRequestJSONSchema(ctx, &method.Params, hasLocation)
		},
		JSONOutputValue:          func() interface{} { return &core.OperationWithDetail{} },
		JSONOutputCodes:          []int{http.StatusOK},
		PreTranslatedDescription: description,
	})
	routes = append(routes, &ffapi.Route{
		Name:   fmt.Sprintf("query_%s", method.Pathname),
		Path:   fmt.Sprintf("query/%s", method.Pathname), // must match a route defined in apiserver routes!
		Method: http.MethodPost,
		JSONInputSchema: func(ctx context.Context, schemaGen ffapi.SchemaGenerator) (*openapi3.SchemaRef, error) {
			return contractRequestJSONSchema(ctx, &method.Params, hasLocation)
		},
		JSONOutputSchema: func(ctx context.Context, schemaGen ffapi.SchemaGenerator) (*openapi3.SchemaRef, error) {
			return contractQueryResponseJSONSchema(ctx, &method.Returns)
		},
		JSONOutputCodes:          []int{http.StatusOK},
		PreTranslatedDescription: description,
	})
	return routes
}

func addFFIEvent(ctx context.Context, routes []*ffapi.Route, event *fftypes.FFIEvent, hasLocation bool) []*ffapi.Route {
	description := event.Description
	if len(event.Details) > 0 {
		additionalDetailsHeader := i18n.Expand(ctx, coremsgs.APISmartContractDetails)
		description = fmt.Sprintf("%s\n\n%s:\n\n%s", description, additionalDetailsHeader, buildDetailsTable(ctx, event.Details))
	}
	routes = append(routes, &ffapi.Route{
		Name:   fmt.Sprintf("createlistener_%s", event.Pathname),
		Path:   fmt.Sprintf("listeners/%s", event.Pathname), // must match a route defined in apiserver routes!
		Method: http.MethodPost,
		JSONInputValue: func() interface{} {
			if hasLocation {
				return &ContractListenerInput{}
			}
			return &ContractListenerInputWithLocation{}
		},
		JSONOutputValue:          func() interface{} { return &core.ContractListener{} },
		JSONOutputCodes:          []int{http.StatusOK},
		PreTranslatedDescription: description,
	})
	routes = append(routes, &ffapi.Route{
		Name:            fmt.Sprintf("getlistener_%s", event.Pathname),
		Path:            fmt.Sprintf("listeners/%s", event.Pathname), // must match a route defined in apiserver routes!
		Method:          http.MethodGet,
		JSONInputValue:  nil,
		JSONOutputValue: func() interface{} { return []*core.ContractListener{} },
		JSONOutputCodes: []int{http.StatusOK},
	})
	return routes
}

/**
 * Parse the FFI and build a corresponding JSON Schema to describe the request body for "invoke" or "query" requests
 * Returns the JSON Schema as an `fftypes.JSONObject`
 */
func contractRequestJSONSchema(ctx context.Context, params *fftypes.FFIParams, hasLocation bool) (*openapi3.SchemaRef, error) {
	paramSchema := make(fftypes.JSONObject, len(*params))
	for _, param := range *params {
		paramSchema[param.Name] = param.Schema
	}
	inputSchema := fftypes.JSONObject{
		"type":        "object",
		"description": i18n.Expand(ctx, coremsgs.ContractCallRequestInput),
		"properties":  paramSchema,
	}
	properties := fftypes.JSONObject{
		"input": inputSchema,
		"options": fftypes.JSONObject{
			"type":        "object",
			"description": i18n.Expand(ctx, coremsgs.ContractCallRequestOptions),
		},
		"key": fftypes.JSONObject{
			"type":        "string",
			"description": i18n.Expand(ctx, coremsgs.ContractCallRequestKey),
		},
		"idempotencyKey": fftypes.JSONObject{
			"type":        "string",
			"description": i18n.Expand(ctx, coremsgs.ContractCallIdempotencyKey),
		},
	}
	if !hasLocation {
		properties["location"] = fftypes.JSONAnyPtr(`{}`)
	}
	schema := fftypes.JSONObject{
		"type":       "object",
		"properties": properties,
	}
	b, err := json.Marshal(schema)
	if err != nil {
		return nil, err
	}
	s := openapi3.NewSchema()
	err = s.UnmarshalJSON(b)
	if err != nil {
		return nil, err
	}
	return openapi3.NewSchemaRef("", s), nil
}

/**
 * Parse the FFI and build a corresponding JSON Schema to describe the response body for "query" requests
 * Returns the JSON Schema as an `fftypes.JSONObject`
 */
func contractQueryResponseJSONSchema(ctx context.Context, params *fftypes.FFIParams) (*openapi3.SchemaRef, error) {
	paramSchema := make(fftypes.JSONObject, len(*params))
	for i, param := range *params {
		paramName := param.Name
		if paramName == "" {
			if i > 0 {
				paramName = fmt.Sprintf("output%v", i)
			} else {
				paramName = "output"
			}
		}
		paramSchema[paramName] = param.Schema
	}
	outputSchema := fftypes.JSONObject{
		"type":        "object",
		"description": i18n.Expand(ctx, coremsgs.ContractCallRequestOutput),
		"properties":  paramSchema,
	}
	b, err := json.Marshal(outputSchema)
	if err != nil {
		return nil, err
	}
	s := openapi3.NewSchema()
	err = s.UnmarshalJSON(b)
	if err != nil {
		return nil, err
	}
	return openapi3.NewSchemaRef("", s), nil
}

func buildDetailsTable(ctx context.Context, details map[string]interface{}) string {
	keyHeader := i18n.Expand(ctx, coremsgs.APISmartContractDetailsKey)
	valueHeader := i18n.Expand(ctx, coremsgs.APISmartContractDetailsKey)
	var s strings.Builder
	s.WriteString(fmt.Sprintf("| %s | %s |\n|-----|-------|\n", keyHeader, valueHeader))
	keys := make([]string, len(details))
	i := 0
	for key := range details {
		keys[i] = key
	}
	sort.Strings(keys)
	for _, key := range keys {
		s.WriteString(fmt.Sprintf("|%s|%s|\n", key, details[key]))
	}
	return s.String()
}
