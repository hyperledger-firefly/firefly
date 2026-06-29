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

package apiserver

import (
	"context"
	"fmt"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/ghodss/yaml"
	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
)

func testFFI() *fftypes.FFI {
	return &fftypes.FFI{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Name:      "math",
		Version:   "v1.0.0",
		Methods: []*fftypes.FFIMethod{
			{
				Name:     "method1",
				Pathname: "method1",
				Params: fftypes.FFIParams{
					{
						Name:   "x",
						Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
					},
					{
						Name:   "y",
						Schema: fftypes.JSONAnyPtr(`{"type": "string", "contentEncoding": "base64"}`),
					},
					{
						Name: "z",
						Schema: fftypes.JSONAnyPtr(`
{
	"type": "object",
	"properties": {
		"name": {"type": "string"},
		"price": {"type": "integer"}
	}
}`),
					},
				},
				Returns: fftypes.FFIParams{
					{
						Name:   "success",
						Schema: fftypes.JSONAnyPtr(`{"type": "boolean"}`),
					},
				},
				Details: fftypes.JSONObject{
					"payable":         true,
					"stateMutability": "payable",
				},
			},
			{
				Name:     "method2",
				Pathname: "method2",
				/* no params */
			},
		},
		Events: []*fftypes.FFIEvent{
			{
				ID:       fftypes.NewUUID(),
				Pathname: "event1",
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "event1",
					Params: fftypes.FFIParams{
						{
							Name:   "result",
							Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
						},
					},
					Details: fftypes.JSONObject{
						"anonymous": true,
					},
				},
			},
		},
	}
}

func pathNames(p *openapi3.Paths) []string {
	var keys []string
	for k := range p.Map() {
		keys = append(keys, k)
	}
	return keys
}

func paramNames(p openapi3.Schemas) []string {
	var keys []string
	for k := range p {
		keys = append(keys, k)
	}
	return keys
}

func TestGenerate(t *testing.T) {
	api := &core.ContractAPI{}
	options, routes := (&ffiSwaggerGen{}).Build(context.Background(), api, testFFI())
	options.BaseURL = "http://localhost:12345"
	doc := ffapi.NewSwaggerGen(options).Generate(context.Background(), routes)

	b, err := yaml.Marshal(doc)
	assert.NoError(t, err)
	fmt.Print(string(b))

	assert.ElementsMatch(t, []string{"/interface", "/invoke/method1", "/invoke/method2", "/query/method1", "/query/method2", "/listeners/event1"}, pathNames(doc.Paths))

	invokeMethod1 := doc.Paths.Value("/invoke/method1").Post.RequestBody.Value.Content.Get("application/json").Schema.Value
	assert.True(t, invokeMethod1.Type.Is("object"))
	assert.ElementsMatch(t, []string{"input", "location", "options", "key", "idempotencyKey"}, paramNames(invokeMethod1.Properties))
	assert.True(t, invokeMethod1.Properties["input"].Value.Type.Is("object"))
	assert.ElementsMatch(t, []string{"x", "y", "z"}, paramNames(invokeMethod1.Properties["input"].Value.Properties))

	invokeMethod2 := doc.Paths.Value("/invoke/method2").Post.RequestBody.Value.Content.Get("application/json").Schema.Value
	assert.True(t, invokeMethod2.Type.Is("object"))
	assert.ElementsMatch(t, []string{"input", "location", "options", "key", "idempotencyKey"}, paramNames(invokeMethod2.Properties))
	assert.True(t, invokeMethod2.Properties["input"].Value.Type.Is("object"))
	assert.ElementsMatch(t, []string{}, paramNames(invokeMethod2.Properties["input"].Value.Properties))

	queryMethod1 := doc.Paths.Value("/query/method1").Post.RequestBody.Value.Content.Get("application/json").Schema.Value
	assert.True(t, queryMethod1.Type.Is("object"))
	assert.ElementsMatch(t, []string{"input", "location", "options", "key", "idempotencyKey"}, paramNames(queryMethod1.Properties))
	assert.True(t, queryMethod1.Properties["input"].Value.Type.Is("object"))
	assert.ElementsMatch(t, []string{"x", "y", "z"}, paramNames(queryMethod1.Properties["input"].Value.Properties))

	queryMethod2 := doc.Paths.Value("/query/method2").Post.RequestBody.Value.Content.Get("application/json").Schema.Value
	assert.True(t, queryMethod2.Type.Is("object"))
	assert.ElementsMatch(t, []string{"input", "location", "options", "key", "idempotencyKey"}, paramNames(queryMethod2.Properties))
	assert.True(t, queryMethod2.Properties["input"].Value.Type.Is("object"))
	assert.ElementsMatch(t, []string{}, paramNames(queryMethod2.Properties["input"].Value.Properties))
}

func TestGenerateWithLocation(t *testing.T) {
	api := &core.ContractAPI{Location: fftypes.JSONAnyPtr(`{}`)}
	options, routes := (&ffiSwaggerGen{}).Build(context.Background(), api, testFFI())
	options.BaseURL = "http://localhost:12345"
	doc := ffapi.NewSwaggerGen(options).Generate(context.Background(), routes)

	b, err := yaml.Marshal(doc)
	assert.NoError(t, err)
	fmt.Print(string(b))

	assert.ElementsMatch(t, []string{"/interface", "/invoke/method1", "/invoke/method2", "/query/method1", "/query/method2", "/listeners/event1"}, pathNames(doc.Paths))

	invokeMethod1 := doc.Paths.Value("/invoke/method1").Post.RequestBody.Value.Content.Get("application/json").Schema.Value
	assert.True(t, invokeMethod1.Type.Is("object"))
	assert.ElementsMatch(t, []string{"input", "options", "key", "idempotencyKey"}, paramNames(invokeMethod1.Properties))
	assert.True(t, invokeMethod1.Properties["input"].Value.Type.Is("object"))
	assert.ElementsMatch(t, []string{"x", "y", "z"}, paramNames(invokeMethod1.Properties["input"].Value.Properties))

	invokeMethod2 := doc.Paths.Value("/invoke/method2").Post.RequestBody.Value.Content.Get("application/json").Schema.Value
	assert.True(t, invokeMethod2.Type.Is("object"))
	assert.ElementsMatch(t, []string{"input", "options", "key", "idempotencyKey"}, paramNames(invokeMethod2.Properties))
	assert.True(t, invokeMethod2.Properties["input"].Value.Type.Is("object"))
	assert.ElementsMatch(t, []string{}, paramNames(invokeMethod2.Properties["input"].Value.Properties))

	queryMethod1 := doc.Paths.Value("/query/method1").Post.RequestBody.Value.Content.Get("application/json").Schema.Value
	assert.True(t, queryMethod1.Type.Is("object"))
	assert.ElementsMatch(t, []string{"input", "options", "key", "idempotencyKey"}, paramNames(queryMethod1.Properties))
	assert.True(t, queryMethod1.Properties["input"].Value.Type.Is("object"))
	assert.ElementsMatch(t, []string{"x", "y", "z"}, paramNames(queryMethod1.Properties["input"].Value.Properties))

	queryMethod2 := doc.Paths.Value("/query/method2").Post.RequestBody.Value.Content.Get("application/json").Schema.Value
	assert.True(t, queryMethod2.Type.Is("object"))
	assert.ElementsMatch(t, []string{"input", "options", "key", "idempotencyKey"}, paramNames(queryMethod2.Properties))
	assert.True(t, queryMethod2.Properties["input"].Value.Type.Is("object"))
	assert.ElementsMatch(t, []string{}, paramNames(queryMethod2.Properties["input"].Value.Properties))
}

func TestFFIParamBadSchema(t *testing.T) {
	ctx := context.Background()
	params := &fftypes.FFIParams{
		&fftypes.FFIParam{
			Name:   "test",
			Schema: fftypes.JSONAnyPtr(`{`),
		},
	}
	_, err := contractRequestJSONSchema(ctx, params, true)
	assert.Error(t, err)

	params = &fftypes.FFIParams{
		&fftypes.FFIParam{
			Name:   "test",
			Schema: fftypes.JSONAnyPtr(`{"type": false}`),
		},
	}
	_, err = contractRequestJSONSchema(ctx, params, true)
	assert.Error(t, err)
}

func TestUnnamedOutputs(t *testing.T) {
	ctx := context.Background()
	params := &fftypes.FFIParams{
		{
			Name:   "",
			Schema: fftypes.JSONAnyPtr(`{}`),
		},
		{
			Name:   "",
			Schema: fftypes.JSONAnyPtr(`{}`),
		},
	}

	expectedJSON := `{
		"description": "A map of named outputs",
		"properties": {
			"output": {},
			"output1": {}
		},
		"type": "object"
	}`

	ref, err := contractQueryResponseJSONSchema(ctx, params)
	assert.NoError(t, err)
	b, err := ref.MarshalJSON()
	assert.JSONEq(t, expectedJSON, string(b))
}

func TestBadSchema(t *testing.T) {
	ctx := context.Background()
	params := &fftypes.FFIParams{
		{
			Name:   "",
			Schema: fftypes.JSONAnyPtr(`{`),
		},
	}
	_, err := contractQueryResponseJSONSchema(ctx, params)
	assert.Error(t, err)

	ctx = context.Background()
	params = &fftypes.FFIParams{
		{
			Name:   "",
			Schema: fftypes.JSONAnyPtr(`{"type": false}`),
		},
	}
	_, err = contractQueryResponseJSONSchema(ctx, params)
	assert.Error(t, err)
}
