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

package definitions

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func testFFI() *fftypes.FFI {
	return &fftypes.FFI{
		ID:          fftypes.NewUUID(),
		Namespace:   "ns1",
		Name:        "math",
		NetworkName: "math",
		Version:     "v1.0.0",
		Published:   true,
		Methods: []*fftypes.FFIMethod{
			{
				Name: "sum",
				Params: fftypes.FFIParams{
					{
						Name:   "x",
						Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
					},
					{
						Name:   "y",
						Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
					},
				},
				Returns: fftypes.FFIParams{
					{
						Name:   "result",
						Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
					},
				},
			},
		},
		Events: []*fftypes.FFIEvent{
			{
				ID: fftypes.NewUUID(),
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "event1",
					Params: fftypes.FFIParams{
						{
							Name:   "result",
							Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
						},
					},
				},
			},
		},
		Errors: []*fftypes.FFIError{
			{
				ID: fftypes.NewUUID(),
				FFIErrorDefinition: fftypes.FFIErrorDefinition{
					Name: "event1",
					Params: fftypes.FFIParams{
						{
							Name:   "result",
							Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
						},
					},
				},
			},
		},
	}
}

func testContractAPI() *core.ContractAPI {
	return &core.ContractAPI{
		ID:          fftypes.NewUUID(),
		Namespace:   "ns1",
		Name:        "math",
		NetworkName: "math",
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
		Location: fftypes.JSONAnyPtr(""),
	}
}

func TestHandleFFIBroadcastOk(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	b, err := json.Marshal(testFFI())
	assert.NoError(t, err)
	data := &core.Data{
		Value: fftypes.JSONAnyPtrBytes(b),
	}

	dh.mdi.On("InsertOrGetFFI", mock.Anything, mock.Anything).Return(nil, nil)
	dh.mdi.On("UpsertFFIMethod", mock.Anything, mock.Anything).Return(nil)
	dh.mdi.On("UpsertFFIEvent", mock.Anything, mock.Anything).Return(nil)
	dh.mdi.On("UpsertFFIError", mock.Anything, mock.Anything).Return(nil)
	dh.mdi.On("InsertEvent", mock.Anything, mock.Anything).Return(nil)
	dh.mcm.On("ResolveFFI", mock.Anything, mock.Anything).Return(nil)
	dh.mim.On("GetRootOrgDID", context.Background()).Return("firefly:org1", nil)

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, &core.Message{
		Header: core.MessageHeader{
			Tag: core.SystemTagDefineFFI,
		},
	}, core.DataArray{data}, fftypes.NewUUID())
	assert.NoError(t, err)
	assert.Equal(t, HandlerResult{Action: core.ActionConfirm}, action)
	err = bs.RunFinalize(context.Background())
	assert.NoError(t, err)
}

func TestHandleFFIBroadcastUpdate(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	ffi := testFFI()
	b, err := json.Marshal(ffi)
	assert.NoError(t, err)
	data := &core.Data{
		Value: fftypes.JSONAnyPtrBytes(b),
	}

	existing := &fftypes.FFI{
		ID:      ffi.ID,
		Message: ffi.Message,
	}

	dh.mdi.On("InsertOrGetFFI", mock.Anything, mock.Anything).Return(existing, nil)
	dh.mdi.On("InsertEvent", mock.Anything, mock.Anything).Return(nil)
	dh.mcm.On("ResolveFFI", mock.Anything, mock.Anything).Return(nil)
	dh.mim.On("GetRootOrgDID", context.Background()).Return("firefly:org1", nil)

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, &core.Message{
		Header: core.MessageHeader{
			Tag: core.SystemTagDefineFFI,
		},
	}, core.DataArray{data}, fftypes.NewUUID())
	assert.NoError(t, err)
	assert.Equal(t, HandlerResult{Action: core.ActionConfirm}, action)
	err = bs.RunFinalize(context.Background())
	assert.NoError(t, err)
}

func TestHandleFFIBroadcastNameExists(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	ffi := testFFI()
	existing := &fftypes.FFI{
		Name:    ffi.Name,
		Version: ffi.Version,
	}

	b, err := json.Marshal(ffi)
	assert.NoError(t, err)
	data := &core.Data{
		Value: fftypes.JSONAnyPtrBytes(b),
	}

	dh.mdi.On("InsertOrGetFFI", mock.Anything, mock.MatchedBy(func(f *fftypes.FFI) bool {
		return f.Name == "math"
	})).Return(existing, nil)
	dh.mdi.On("InsertOrGetFFI", mock.Anything, mock.MatchedBy(func(f *fftypes.FFI) bool {
		return f.Name == "math-1"
	})).Return(nil, nil)
	dh.mdi.On("UpsertFFIMethod", mock.Anything, mock.Anything).Return(nil)
	dh.mdi.On("UpsertFFIEvent", mock.Anything, mock.Anything).Return(nil)
	dh.mdi.On("UpsertFFIError", mock.Anything, mock.Anything).Return(nil)
	dh.mdi.On("InsertEvent", mock.Anything, mock.Anything).Return(nil)
	dh.mcm.On("ResolveFFI", mock.Anything, mock.Anything).Return(nil)
	dh.mim.On("GetRootOrgDID", context.Background()).Return("firefly:org1", nil)

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, &core.Message{
		Header: core.MessageHeader{
			Tag: core.SystemTagDefineFFI,
		},
	}, core.DataArray{data}, fftypes.NewUUID())
	assert.NoError(t, err)
	assert.Equal(t, HandlerResult{Action: core.ActionConfirm}, action)
	err = bs.RunFinalize(context.Background())
	assert.NoError(t, err)
}

func TestHandleFFILocalNameExists(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	ffi := testFFI()
	ffi.Published = false
	existing := &fftypes.FFI{
		Name:    ffi.Name,
		Version: ffi.Version,
	}

	dh.mdi.On("InsertOrGetFFI", mock.Anything, mock.MatchedBy(func(f *fftypes.FFI) bool {
		return f.Name == "math"
	})).Return(existing, nil)
	dh.mcm.On("ResolveFFI", mock.Anything, mock.Anything).Return(nil)

	action, err := dh.handleFFIDefinition(context.Background(), &bs.BatchState, ffi, fftypes.NewUUID(), true)
	assert.Regexp(t, "FF10407", err)
	assert.Equal(t, HandlerResult{Action: core.ActionReject}, action)
	err = bs.RunFinalize(context.Background())
	assert.NoError(t, err)
}

func TestPersistFFIValidateFFIFail(t *testing.T) {
	dh, _ := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	dh.mcm.On("ResolveFFI", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
	retry, err := dh.persistFFI(context.Background(), testFFI(), true)
	assert.Regexp(t, "FF10403", err)
	assert.False(t, retry)
}

func TestHandleFFIBroadcastReject(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	action, err := dh.handleFFIBroadcast(context.Background(), &bs.BatchState, &core.Message{
		Header: core.MessageHeader{
			Tag: core.SystemTagDefineFFI,
		},
	}, core.DataArray{}, fftypes.NewUUID())

	assert.Equal(t, HandlerResult{Action: core.ActionReject}, action)
	assert.Error(t, err)
	bs.assertNoFinalizers()
}

func TestPersistFFIUpsertFFIFail(t *testing.T) {
	dh, _ := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	dh.mdi.On("InsertOrGetFFI", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))
	dh.mcm.On("ResolveFFI", mock.Anything, mock.Anything).Return(nil)
	retry, err := dh.persistFFI(context.Background(), testFFI(), true)
	assert.Regexp(t, "pop", err)
	assert.True(t, retry)
}

func TestPersistFFIUpsertFFIMethodFail(t *testing.T) {
	dh, _ := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	dh.mdi.On("InsertOrGetFFI", mock.Anything, mock.Anything).Return(nil, nil)
	dh.mdi.On("UpsertFFIMethod", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
	dh.mcm.On("ResolveFFI", mock.Anything, mock.Anything).Return(nil)
	retry, err := dh.persistFFI(context.Background(), testFFI(), true)
	assert.Regexp(t, "pop", err)
	assert.True(t, retry)
}

func TestPersistFFIUpsertFFIEventFail(t *testing.T) {
	dh, _ := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	dh.mdi.On("InsertOrGetFFI", mock.Anything, mock.Anything).Return(nil, nil)
	dh.mdi.On("UpsertFFIMethod", mock.Anything, mock.Anything).Return(nil)
	dh.mdi.On("UpsertFFIEvent", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
	dh.mcm.On("ResolveFFI", mock.Anything, mock.Anything).Return(nil)
	retry, err := dh.persistFFI(context.Background(), testFFI(), true)
	assert.Regexp(t, "pop", err)
	assert.True(t, retry)
}

func TestPersistFFIUpsertFFIErrorFail(t *testing.T) {
	dh, _ := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	dh.mdi.On("InsertOrGetFFI", mock.Anything, mock.Anything).Return(nil, nil)
	dh.mdi.On("UpsertFFIMethod", mock.Anything, mock.Anything).Return(nil)
	dh.mdi.On("UpsertFFIEvent", mock.Anything, mock.Anything).Return(nil)
	dh.mdi.On("UpsertFFIError", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
	dh.mcm.On("ResolveFFI", mock.Anything, mock.Anything).Return(nil)
	retry, err := dh.persistFFI(context.Background(), testFFI(), true)
	assert.Regexp(t, "pop", err)
	assert.True(t, retry)
}

func TestPersistFFILocalPublish(t *testing.T) {
	dh, _ := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	published := testFFI()
	published.Message = fftypes.NewUUID()
	existing := &fftypes.FFI{
		ID: published.ID,
	}

	dh.mdi.On("InsertOrGetFFI", mock.Anything, mock.Anything).Return(existing, nil)
	dh.mcm.On("ResolveFFI", mock.Anything, mock.Anything).Return(nil)
	dh.mdi.On("UpsertFFI", mock.Anything, published, database.UpsertOptimizationExisting).Return(nil)

	retry, err := dh.persistFFI(context.Background(), published, true)
	assert.NoError(t, err)
	assert.False(t, retry)
}

func TestPersistFFILocalPublishUpsertFail(t *testing.T) {
	dh, _ := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	published := testFFI()
	published.Message = fftypes.NewUUID()
	existing := &fftypes.FFI{
		ID: published.ID,
	}

	dh.mdi.On("InsertOrGetFFI", mock.Anything, mock.Anything).Return(existing, nil)
	dh.mcm.On("ResolveFFI", mock.Anything, mock.Anything).Return(nil)
	dh.mdi.On("UpsertFFI", mock.Anything, published, database.UpsertOptimizationExisting).Return(fmt.Errorf("pop"))

	retry, err := dh.persistFFI(context.Background(), published, true)
	assert.EqualError(t, err, "pop")
	assert.True(t, retry)
}

func TestPersistFFIWrongMessage(t *testing.T) {
	dh, _ := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	published := testFFI()
	published.Message = fftypes.NewUUID()
	existing := &fftypes.FFI{
		ID:      published.ID,
		Message: fftypes.NewUUID(),
	}

	dh.mdi.On("InsertOrGetFFI", mock.Anything, mock.Anything).Return(existing, nil)
	dh.mcm.On("ResolveFFI", mock.Anything, mock.Anything).Return(nil)

	retry, err := dh.persistFFI(context.Background(), published, true)
	assert.Regexp(t, "FF10407", err)
	assert.False(t, retry)
}

func TestHandleFFIBroadcastOrgFail(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	ffi := testFFI()
	b, err := json.Marshal(ffi)
	assert.NoError(t, err)
	data := &core.Data{
		Value: fftypes.JSONAnyPtrBytes(b),
	}

	dh.mim.On("GetRootOrgDID", context.Background()).Return("", fmt.Errorf("pop"))

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, &core.Message{
		Header: core.MessageHeader{
			Tag: core.SystemTagDefineFFI,
		},
	}, core.DataArray{data}, fftypes.NewUUID())

	assert.Equal(t, HandlerResult{Action: core.ActionRetry}, action)
	assert.Regexp(t, "pop", err)
	bs.assertNoFinalizers()
}

func TestHandleFFIBroadcastPersistFail(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	ffi := testFFI()
	b, err := json.Marshal(ffi)
	assert.NoError(t, err)
	data := &core.Data{
		Value: fftypes.JSONAnyPtrBytes(b),
	}
	dh.mdi.On("InsertOrGetFFI", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))
	dh.mcm.On("ResolveFFI", mock.Anything, mock.Anything).Return(nil)
	dh.mim.On("GetRootOrgDID", context.Background()).Return("firefly:org1", nil)

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, &core.Message{
		Header: core.MessageHeader{
			Tag: core.SystemTagDefineFFI,
		},
	}, core.DataArray{data}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionRetry}, action)
	assert.Regexp(t, "pop", err)
	bs.assertNoFinalizers()
}

func TestHandleFFIBroadcastResolveFail(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	ffi := testFFI()
	b, err := json.Marshal(ffi)
	assert.NoError(t, err)
	data := &core.Data{
		Value: fftypes.JSONAnyPtrBytes(b),
	}

	dh.mcm.On("ResolveFFI", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
	dh.mim.On("GetRootOrgDID", context.Background()).Return("firefly:org1", nil)

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, &core.Message{
		Header: core.MessageHeader{
			Tag: core.SystemTagDefineFFI,
		},
	}, core.DataArray{data}, fftypes.NewUUID())

	assert.Equal(t, HandlerResult{Action: core.ActionReject}, action)
	assert.Regexp(t, "pop", err)
	bs.assertNoFinalizers()
}

func TestHandleContractAPIBroadcastOk(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	b, err := json.Marshal(testContractAPI())
	assert.NoError(t, err)
	data := &core.Data{
		Value: fftypes.JSONAnyPtrBytes(b),
	}

	dh.mdi.On("InsertOrGetContractAPI", mock.Anything, mock.Anything).Return(nil, nil)
	dh.mdi.On("InsertEvent", mock.Anything, mock.Anything).Return(nil)
	dh.mcm.On("ResolveContractAPI", context.Background(), "", mock.Anything).Return(nil)
	dh.mim.On("GetRootOrgDID", context.Background()).Return("firefly:org1", nil)

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, &core.Message{
		Header: core.MessageHeader{
			Tag: core.SystemTagDefineContractAPI,
		},
	}, core.DataArray{data}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionConfirm}, action)
	assert.NoError(t, err)
	err = bs.RunFinalize(context.Background())
	assert.NoError(t, err)
}

func TestHandleContractAPIBadPayload(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	data := &core.Data{
		Value: fftypes.JSONAnyPtr("bad"),
	}

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, &core.Message{
		Header: core.MessageHeader{
			Tag: core.SystemTagDefineContractAPI,
		},
	}, core.DataArray{data}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionReject}, action)
	assert.Regexp(t, "FF10400", err)
}

func TestHandleContractAPIBroadcastPersistFail(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	b, err := json.Marshal(testContractAPI())
	assert.NoError(t, err)
	data := &core.Data{
		Value: fftypes.JSONAnyPtrBytes(b),
	}

	dh.mdi.On("InsertOrGetContractAPI", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))
	dh.mcm.On("ResolveContractAPI", context.Background(), "", mock.Anything).Return(nil)
	dh.mim.On("GetRootOrgDID", context.Background()).Return("firefly:org1", nil)

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, &core.Message{
		Header: core.MessageHeader{
			Tag: core.SystemTagDefineContractAPI,
		},
	}, core.DataArray{data}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionRetry}, action)
	assert.Regexp(t, "pop", err)

	bs.assertNoFinalizers()
}

func TestHandleContractAPIBroadcastResolveFail(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	b, err := json.Marshal(testContractAPI())
	assert.NoError(t, err)
	data := &core.Data{
		Value: fftypes.JSONAnyPtrBytes(b),
	}

	dh.mcm.On("ResolveContractAPI", context.Background(), "", mock.Anything).Return(fmt.Errorf("pop"))
	dh.mim.On("GetRootOrgDID", context.Background()).Return("firefly:org1", nil)

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, &core.Message{
		Header: core.MessageHeader{
			Tag: core.SystemTagDefineContractAPI,
		},
	}, core.DataArray{data}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionReject}, action)
	assert.Regexp(t, "pop", err)

	bs.assertNoFinalizers()
}

func TestHandleContractAPIBroadcastOrgFail(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	b, err := json.Marshal(testContractAPI())
	assert.NoError(t, err)
	data := &core.Data{
		Value: fftypes.JSONAnyPtrBytes(b),
	}

	dh.mim.On("GetRootOrgDID", context.Background()).Return("", fmt.Errorf("pop"))

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, &core.Message{
		Header: core.MessageHeader{
			Tag: core.SystemTagDefineContractAPI,
		},
	}, core.DataArray{data}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionRetry}, action)
	assert.Regexp(t, "pop", err)

	bs.assertNoFinalizers()
}

func TestPersistContractAPIConfirmMessage(t *testing.T) {
	dh, _ := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	api := testContractAPI()
	api.Published = true
	api.Message = fftypes.NewUUID()
	existing := &core.ContractAPI{
		ID:      api.ID,
		Message: api.Message,
	}

	dh.mdi.On("InsertOrGetContractAPI", mock.Anything, mock.Anything).Return(existing, nil)
	dh.mcm.On("ResolveContractAPI", context.Background(), "", mock.Anything).Return(nil)

	_, err := dh.persistContractAPI(context.Background(), "", api, true)
	assert.NoError(t, err)
}

func TestPersistContractAPIUpsert(t *testing.T) {
	dh, _ := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	api := testContractAPI()
	api.Published = true
	api.Message = fftypes.NewUUID()
	existing := &core.ContractAPI{
		ID: api.ID,
	}

	dh.mdi.On("InsertOrGetContractAPI", mock.Anything, mock.Anything).Return(existing, nil)
	dh.mcm.On("ResolveContractAPI", context.Background(), "", mock.Anything).Return(nil)
	dh.mdi.On("UpsertContractAPI", context.Background(), api, database.UpsertOptimizationExisting).Return(nil)

	_, err := dh.persistContractAPI(context.Background(), "", api, true)
	assert.NoError(t, err)
}

func TestPersistContractAPIUpsertFail(t *testing.T) {
	dh, _ := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	api := testContractAPI()
	api.Published = true
	api.Message = fftypes.NewUUID()
	existing := &core.ContractAPI{
		ID: api.ID,
	}

	dh.mdi.On("InsertOrGetContractAPI", mock.Anything, mock.Anything).Return(existing, nil)
	dh.mcm.On("ResolveContractAPI", context.Background(), "", mock.Anything).Return(nil)
	dh.mdi.On("UpsertContractAPI", context.Background(), api, database.UpsertOptimizationExisting).Return(fmt.Errorf("pop"))

	_, err := dh.persistContractAPI(context.Background(), "", api, true)
	assert.EqualError(t, err, "pop")
}

func TestPersistContractAPIUpsertNonPublished(t *testing.T) {
	dh, _ := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	api := testContractAPI()
	api.Published = false
	api.Message = fftypes.NewUUID()
	existing := &core.ContractAPI{
		ID: api.ID,
	}

	dh.mdi.On("InsertOrGetContractAPI", mock.Anything, mock.Anything).Return(existing, nil)
	dh.mcm.On("ResolveContractAPI", context.Background(), "", mock.Anything).Return(nil)
	dh.mdi.On("UpsertContractAPI", context.Background(), api, database.UpsertOptimizationExisting).Return(nil)

	_, err := dh.persistContractAPI(context.Background(), "", api, true)
	assert.NoError(t, err)
}

func TestPersistContractAPIUpsertFailNonPublished(t *testing.T) {
	dh, _ := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	api := testContractAPI()
	api.Published = false
	api.Message = fftypes.NewUUID()
	existing := &core.ContractAPI{
		ID: api.ID,
	}

	dh.mdi.On("InsertOrGetContractAPI", mock.Anything, mock.Anything).Return(existing, nil)
	dh.mcm.On("ResolveContractAPI", context.Background(), "", mock.Anything).Return(nil)
	dh.mdi.On("UpsertContractAPI", context.Background(), api, database.UpsertOptimizationExisting).Return(fmt.Errorf("pop"))

	_, err := dh.persistContractAPI(context.Background(), "", api, true)
	assert.EqualError(t, err, "pop")
}
func TestPersistContractAPIWrongMessage(t *testing.T) {
	dh, _ := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	api := testContractAPI()
	api.Published = true
	api.Message = fftypes.NewUUID()
	existing := &core.ContractAPI{
		ID:      api.ID,
		Message: fftypes.NewUUID(),
	}

	dh.mdi.On("InsertOrGetContractAPI", mock.Anything, mock.Anything).Return(existing, nil)
	dh.mcm.On("ResolveContractAPI", context.Background(), "", mock.Anything).Return(nil)

	_, err := dh.persistContractAPI(context.Background(), "", api, true)
	assert.Regexp(t, "FF10407", err)
}

func TestPersistContractAPINameConflict(t *testing.T) {
	dh, _ := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	api := testContractAPI()
	api.Published = true
	api.Message = fftypes.NewUUID()
	existing := &core.ContractAPI{
		ID:   fftypes.NewUUID(),
		Name: api.Name,
	}

	dh.mdi.On("InsertOrGetContractAPI", mock.Anything, mock.Anything).Return(existing, nil).Once()
	dh.mdi.On("InsertOrGetContractAPI", mock.Anything, mock.Anything).Return(nil, nil).Once()
	dh.mcm.On("ResolveContractAPI", context.Background(), "", mock.Anything).Return(nil)

	_, err := dh.persistContractAPI(context.Background(), "", api, true)
	assert.NoError(t, err)
	assert.Equal(t, "math-1", api.Name)
}

func TestPersistContractAPINetworkNameConflict(t *testing.T) {
	dh, _ := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	api := testContractAPI()
	api.Published = true
	api.Message = fftypes.NewUUID()
	existing := &core.ContractAPI{
		ID:          fftypes.NewUUID(),
		NetworkName: api.NetworkName,
	}

	dh.mdi.On("InsertOrGetContractAPI", mock.Anything, mock.Anything).Return(existing, nil)
	dh.mcm.On("ResolveContractAPI", context.Background(), "", mock.Anything).Return(nil)

	_, err := dh.persistContractAPI(context.Background(), "", api, true)
	assert.Regexp(t, "FF10407", err)
}
