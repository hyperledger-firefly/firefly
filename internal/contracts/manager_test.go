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

package contracts

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/firefly/internal/cache"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/internal/database/sqlcommon"
	"github.com/hyperledger-firefly/firefly/internal/identity"
	"github.com/hyperledger-firefly/firefly/internal/syncasync"
	"github.com/hyperledger-firefly/firefly/internal/txcommon"
	"github.com/hyperledger-firefly/firefly/mocks/batchmocks"
	"github.com/hyperledger-firefly/firefly/mocks/blockchainmocks"
	"github.com/hyperledger-firefly/firefly/mocks/broadcastmocks"
	"github.com/hyperledger-firefly/firefly/mocks/cachemocks"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/mocks/datamocks"
	"github.com/hyperledger-firefly/firefly/mocks/identitymanagermocks"
	"github.com/hyperledger-firefly/firefly/mocks/operationmocks"
	"github.com/hyperledger-firefly/firefly/mocks/privatemessagingmocks"
	"github.com/hyperledger-firefly/firefly/mocks/syncasyncmocks"
	"github.com/hyperledger-firefly/firefly/mocks/txcommonmocks"
	"github.com/hyperledger-firefly/firefly/mocks/txwritermocks"
	"github.com/hyperledger-firefly/firefly/pkg/blockchain"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/hyperledger-firefly/signer/pkg/ffi2abi"
	"github.com/santhosh-tekuri/jsonschema/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func newTestContractManager() *contractManager {
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	mbm := &broadcastmocks.Manager{}
	mpm := &privatemessagingmocks.Manager{}
	mbp := &batchmocks.Manager{}
	mim := &identitymanagermocks.Manager{}
	mbi := &blockchainmocks.Plugin{}
	mom := &operationmocks.Manager{}
	txw := &txwritermocks.Writer{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := txcommon.NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)
	msa := &syncasyncmocks.Bridge{}
	mbi.On("GetFFIParamValidator", mock.Anything).Return(nil, nil)
	mom.On("RegisterHandler", mock.Anything, mock.Anything, mock.Anything)

	mbi.On("Name").Return("mockblockchain").Maybe()

	mdi.On("GetContractListeners", mock.Anything, "ns1", mock.Anything).Return(nil, nil, nil).Once()
	rag := mdi.On("RunAsGroup", mock.Anything, mock.Anything).Maybe()
	rag.RunFn = func(a mock.Arguments) {
		rag.ReturnArguments = mock.Arguments{
			a[1].(func(context.Context) error)(a[0].(context.Context)),
		}
	}
	cm, _ := NewContractManager(context.Background(), "ns1", mdi, mbi, mdm, mbm, mpm, mbp, mim, mom, txHelper, txw, msa, cmi)
	cm.(*contractManager).txHelper = &txcommonmocks.Helper{}
	return cm.(*contractManager)
}

func TestNewContractManagerFail(t *testing.T) {
	_, err := NewContractManager(context.Background(), "", nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	assert.Regexp(t, "FF10128", err)
}

func TestName(t *testing.T) {
	cm := newTestContractManager()
	assert.Equal(t, "ContractManager", cm.Name())
}

func TestNewContractManagerVerifyListenersFails(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	mbm := &broadcastmocks.Manager{}
	mpm := &privatemessagingmocks.Manager{}
	mbp := &batchmocks.Manager{}
	mim := &identitymanagermocks.Manager{}
	mbi := &blockchainmocks.Plugin{}
	mom := &operationmocks.Manager{}
	txw := &txwritermocks.Writer{}
	cmi := &cachemocks.Manager{}
	msa := &syncasyncmocks.Bridge{}

	ctx := context.Background()

	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := txcommon.NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)
	mbi.On("GetFFIParamValidator", mock.Anything).Return(nil, nil)
	mom.On("RegisterHandler", mock.Anything, mock.Anything, mock.Anything)
	mbi.On("Name").Return("mockblockchain").Maybe()
	mdi.On("GetContractListeners", mock.Anything, "ns1", mock.Anything).Return(nil, nil, fmt.Errorf("KABOOM!")).Once()

	cm, err := NewContractManager(context.Background(), "ns1", mdi, mbi, mdm, mbm, mpm, mbp, mim, mom, txHelper, txw, msa, cmi)
	assert.Nil(t, cm)
	assert.NotNil(t, err)
}

func TestNewContractManagerFFISchemaLoaderFail(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	mbm := &broadcastmocks.Manager{}
	mpm := &privatemessagingmocks.Manager{}
	mbp := &batchmocks.Manager{}
	mim := &identitymanagermocks.Manager{}
	mbi := &blockchainmocks.Plugin{}
	mom := &operationmocks.Manager{}
	txw := &txwritermocks.Writer{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := txcommon.NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)
	msa := &syncasyncmocks.Bridge{}
	mbi.On("GetFFIParamValidator", mock.Anything).Return(nil, fmt.Errorf("pop"))
	_, err := NewContractManager(context.Background(), "ns1", mdi, mbi, mdm, mbm, mpm, mbp, mim, mom, txHelper, txw, msa, cmi)
	assert.Regexp(t, "pop", err)
}

func TestNewContractManagerCacheConfigFail(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	mbm := &broadcastmocks.Manager{}
	mpm := &privatemessagingmocks.Manager{}
	mbp := &batchmocks.Manager{}
	mim := &identitymanagermocks.Manager{}
	mbi := &blockchainmocks.Plugin{}
	mom := &operationmocks.Manager{}
	txw := &txwritermocks.Writer{}
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(nil, fmt.Errorf("pop"))
	txHelper := &txcommonmocks.Helper{}
	msa := &syncasyncmocks.Bridge{}
	mbi.On("GetFFIParamValidator", mock.Anything).Return(nil, nil)
	_, err := NewContractManager(context.Background(), "ns1", mdi, mbi, mdm, mbm, mpm, mbp, mim, mom, txHelper, txw, msa, cmi)
	assert.Regexp(t, "pop", err)
}

func TestNewContractManagerFFISchemaLoader(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	mbm := &broadcastmocks.Manager{}
	mpm := &privatemessagingmocks.Manager{}
	mbp := &batchmocks.Manager{}
	mim := &identitymanagermocks.Manager{}
	mbi := &blockchainmocks.Plugin{}
	mom := &operationmocks.Manager{}
	txw := &txwritermocks.Writer{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := txcommon.NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)
	msa := &syncasyncmocks.Bridge{}
	mdi.On("GetContractListeners", mock.Anything, "ns1", mock.Anything).Return(nil, nil, nil)
	mbi.On("GetFFIParamValidator", mock.Anything).Return(&ffi2abi.ParamValidator{}, nil)
	mom.On("RegisterHandler", mock.Anything, mock.Anything, mock.Anything)
	_, err := NewContractManager(context.Background(), "ns1", mdi, mbi, mdm, mbm, mpm, mbp, mim, mom, txHelper, txw, msa, cmi)
	assert.NoError(t, err)
}

func TestResolveFFI(t *testing.T) {
	cm := newTestContractManager()
	mdb := cm.database.(*databasemocks.Plugin)

	mdb.On("GetFFI", mock.Anything, "ns1", "test", "1.0.0").Return(nil, nil)

	ffi := &fftypes.FFI{
		Namespace: "ns1",
		Name:      "test",
		Version:   "1.0.0",
		ID:        fftypes.NewUUID(),
		Methods: []*fftypes.FFIMethod{
			{
				Name: "sum",
			},
		},
		Events: []*fftypes.FFIEvent{
			{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "changed",
				},
			},
		},
		Errors: []*fftypes.FFIError{
			{
				FFIErrorDefinition: fftypes.FFIErrorDefinition{
					Name: "changed",
				},
			},
		},
	}

	err := cm.ResolveFFI(context.Background(), ffi)
	assert.NoError(t, err)
}

func TestBroadcastFFIInvalid(t *testing.T) {
	cm := newTestContractManager()

	ffi := &fftypes.FFI{
		Namespace: "ns1",
		Name:      "test",
		Version:   "1.0.0",
		ID:        fftypes.NewUUID(),
		Methods: []*fftypes.FFIMethod{
			{
				Name: "sum",
				Params: []*fftypes.FFIParam{
					{
						Name:   "x",
						Schema: fftypes.JSONAnyPtr(`{"type": "null"}`),
					},
				},
			},
		},
	}

	err := cm.ResolveFFI(context.Background(), ffi)
	assert.Regexp(t, "does not validate", err)
}

func TestValidateInvokeContractRequest(t *testing.T) {
	cm := newTestContractManager()
	req := &core.ContractCallRequest{
		Type: core.CallTypeInvoke,
		Method: &fftypes.FFIMethod{
			Name: "sum",
			Params: []*fftypes.FFIParam{
				{
					Name:   "x",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
				},
				{
					Name:   "y",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
				},
			},
			Returns: []*fftypes.FFIParam{
				{
					Name:   "z",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
				},
			},
		},
		Input: map[string]interface{}{
			"x": float64(1),
			"y": float64(2),
		},
	}

	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	opaqueData := "anything"
	mbi.On("ParseInterface", context.Background(), req.Method, req.Errors).Return(opaqueData, nil)
	mbi.On("ValidateInvokeRequest", context.Background(), opaqueData, req.Input, false).Return(nil)

	_, err := cm.validateInvokeContractRequest(context.Background(), req, true)
	assert.NoError(t, err)

	mbi.AssertExpectations(t)
}

func TestValidateInvokeContractRequestMissingInput(t *testing.T) {
	cm := newTestContractManager()
	req := &core.ContractCallRequest{
		Type: core.CallTypeInvoke,
		Method: &fftypes.FFIMethod{
			Name: "sum",
			Params: []*fftypes.FFIParam{
				{
					Name:   "x",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
				},
				{
					Name:   "y",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
				},
			},
			Returns: []*fftypes.FFIParam{
				{
					Name:   "z",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
				},
			},
		},
		Errors: []*fftypes.FFIError{
			{
				FFIErrorDefinition: fftypes.FFIErrorDefinition{
					Name: "u",
					Params: fftypes.FFIParams{
						{
							Name:   "t",
							Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
						},
					},
				},
			},
		},
		Input: map[string]interface{}{
			"x": float64(1),
		},
	}
	_, err := cm.validateInvokeContractRequest(context.Background(), req, true)
	assert.Regexp(t, "Missing required input argument 'y'", err)
}

func TestValidateInvokeContractRequestInputWrongType(t *testing.T) {
	cm := newTestContractManager()
	req := &core.ContractCallRequest{
		Type: core.CallTypeInvoke,
		Method: &fftypes.FFIMethod{
			Name: "sum",
			Params: []*fftypes.FFIParam{
				{
					Name:   "x",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
				},
				{
					Name:   "y",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
				},
			},
			Returns: []*fftypes.FFIParam{
				{
					Name:   "z",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
				},
			},
		},
		Input: map[string]interface{}{
			"x": float64(1),
			"y": "two",
		},
	}
	_, err := cm.validateInvokeContractRequest(context.Background(), req, true)
	assert.Regexp(t, "expected integer, but got string", err)
}

func TestValidateInvokeContractRequestErrorInvalid(t *testing.T) {
	cm := newTestContractManager()
	req := &core.ContractCallRequest{
		Type: core.CallTypeInvoke,
		Method: &fftypes.FFIMethod{
			Name: "sum",
			Params: []*fftypes.FFIParam{
				{
					Name:   "x",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
				},
				{
					Name:   "y",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
				},
			},
			Returns: []*fftypes.FFIParam{
				{
					Name:   "z",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
				},
			},
		},
		Errors: []*fftypes.FFIError{
			{
				FFIErrorDefinition: fftypes.FFIErrorDefinition{
					Name: "u",
					Params: fftypes.FFIParams{
						{
							Name:   "t",
							Schema: fftypes.JSONAnyPtr(`{"type": "wrong"}`),
						},
					},
				},
			},
		},
		Input: map[string]interface{}{
			"x": float64(1),
			"y": "two",
		},
	}
	_, err := cm.validateInvokeContractRequest(context.Background(), req, true)
	assert.Regexp(t, "FF10333", err)
}

func TestValidateInvokeContractRequestInvalidParam(t *testing.T) {
	cm := newTestContractManager()
	req := &core.ContractCallRequest{
		Type: core.CallTypeInvoke,
		Method: &fftypes.FFIMethod{
			Name: "sum",
			Params: []*fftypes.FFIParam{
				{
					Name:   "x",
					Schema: fftypes.JSONAnyPtr(`{"type": "null", "details": {"type": "uint256"}}`),
				},
				{
					Name:   "y",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
				},
			},
			Returns: []*fftypes.FFIParam{
				{
					Name:   "z",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
				},
			},
		},
		Input: map[string]interface{}{
			"x": float64(1),
			"y": float64(2),
		},
	}

	_, err := cm.validateInvokeContractRequest(context.Background(), req, true)
	assert.Regexp(t, "does not validate", err)
}

func TestValidateInvokeContractRequestInvalidReturn(t *testing.T) {
	cm := newTestContractManager()
	req := &core.ContractCallRequest{
		Type: core.CallTypeInvoke,
		Method: &fftypes.FFIMethod{
			Name: "sum",
			Params: []*fftypes.FFIParam{
				{
					Name:   "x",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
				},
				{
					Name:   "y",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
				},
			},
			Returns: []*fftypes.FFIParam{
				{
					Name:   "z",
					Schema: fftypes.JSONAnyPtr(`{"type": "null"}`),
				},
			},
		},
		Input: map[string]interface{}{
			"x": float64(1),
			"y": float64(2),
		},
	}

	_, err := cm.validateInvokeContractRequest(context.Background(), req, true)
	assert.Regexp(t, "does not validate", err)
}

func TestValidateFFI(t *testing.T) {
	cm := newTestContractManager()
	mdi := cm.database.(*databasemocks.Plugin)

	ffi := &fftypes.FFI{
		Name:      "math",
		Version:   "1.0.0",
		Namespace: "default",
		Methods: []*fftypes.FFIMethod{
			{
				Name: "sum",
				Params: []*fftypes.FFIParam{
					{
						Name:   "x",
						Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
					},
					{
						Name:   "y",
						Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
					},
				},
				Returns: []*fftypes.FFIParam{
					{
						Name:   "z",
						Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
					},
				},
			},
			{
				Name:        "sum",
				Description: "Override of sum method with different args",
				Params:      []*fftypes.FFIParam{},
				Returns:     []*fftypes.FFIParam{},
			},
		},
		Events: []*fftypes.FFIEvent{
			{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "sum",
					Params: []*fftypes.FFIParam{
						{
							Name:   "z",
							Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
						},
					},
				},
			},
			{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name:        "sum",
					Description: "Override of event with different params",
					Params:      []*fftypes.FFIParam{},
				},
			},
		},
		Errors: []*fftypes.FFIError{
			{
				FFIErrorDefinition: fftypes.FFIErrorDefinition{
					Name: "sum",
					Params: []*fftypes.FFIParam{
						{
							Name:   "z",
							Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
						},
					},
				},
			},
		},
	}

	mdi.On("GetFFI", context.Background(), "ns1", "math", "1.0.0").Return(nil, nil)

	err := cm.ResolveFFI(context.Background(), ffi)
	assert.NoError(t, err)

	assert.Equal(t, "sum", ffi.Methods[0].Pathname)
	assert.Equal(t, "sum_1", ffi.Methods[1].Pathname)
	assert.Equal(t, "sum", ffi.Events[0].Pathname)
	assert.Equal(t, "sum_1", ffi.Events[1].Pathname)
}

func TestValidateFFIFail(t *testing.T) {
	cm := newTestContractManager()
	mdi := cm.database.(*databasemocks.Plugin)

	ffi := &fftypes.FFI{
		Namespace: "default",
		Methods: []*fftypes.FFIMethod{
			{
				Name: "sum",
				Params: []*fftypes.FFIParam{
					{
						Name:   "x",
						Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
					},
					{
						Name:   "y",
						Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
					},
				},
				Returns: []*fftypes.FFIParam{
					{
						Name:   "z",
						Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
					},
				},
			},
			{
				Name:        "sum",
				Description: "Override of sum method with different args",
				Params:      []*fftypes.FFIParam{},
				Returns:     []*fftypes.FFIParam{},
			},
		},
		Events: []*fftypes.FFIEvent{
			{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "sum",
					Params: []*fftypes.FFIParam{
						{
							Name:   "z",
							Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
						},
					},
				},
			},
			{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name:        "sum",
					Description: "Override of event with different params",
					Params:      []*fftypes.FFIParam{},
				},
			},
		},
	}

	mdi.On("GetFFI", context.Background(), "ns1", "math", "1.0.0").Return(nil, nil)

	err := cm.ResolveFFI(context.Background(), ffi)
	assert.Regexp(t, "FF00140", err)
}

func TestValidateFFIBadMethod(t *testing.T) {
	cm := newTestContractManager()
	mdi := cm.database.(*databasemocks.Plugin)

	ffi := &fftypes.FFI{
		Name:      "math",
		Version:   "1.0.0",
		Namespace: "default",
		Methods: []*fftypes.FFIMethod{
			{
				Name: "",
				Params: []*fftypes.FFIParam{
					{
						Name:   "x",
						Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
					},
					{
						Name:   "y",
						Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
					},
				},
				Returns: []*fftypes.FFIParam{
					{
						Name:   "z",
						Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
					},
				},
			},
		},
		Events: []*fftypes.FFIEvent{
			{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "sum",
					Params: []*fftypes.FFIParam{
						{
							Name:   "z",
							Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
						},
					},
				},
			},
		},
	}

	mdi.On("GetFFI", context.Background(), "ns1", "math", "1.0.0").Return(nil, nil)

	err := cm.ResolveFFI(context.Background(), ffi)
	assert.Regexp(t, "FF10320", err)
}

func TestValidateFFIBadEventParam(t *testing.T) {
	cm := newTestContractManager()
	mdi := cm.database.(*databasemocks.Plugin)

	ffi := &fftypes.FFI{
		Name:      "math",
		Version:   "1.0.0",
		Namespace: "default",
		Methods: []*fftypes.FFIMethod{
			{
				Name: "sum",
				Params: []*fftypes.FFIParam{
					{
						Name:   "x",
						Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
					},
					{
						Name:   "y",
						Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
					},
				},
				Returns: []*fftypes.FFIParam{
					{
						Name:   "z",
						Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
					},
				},
			},
		},
		Events: []*fftypes.FFIEvent{
			{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "",
					Params: []*fftypes.FFIParam{
						{
							Name:   "z",
							Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
						},
					},
				},
			},
		},
	}

	mdi.On("GetFFI", context.Background(), "ns1", "math", "1.0.0").Return(nil, nil)

	err := cm.ResolveFFI(context.Background(), ffi)
	assert.Regexp(t, "FF10319", err)
}

func TestValidateFFIBadError(t *testing.T) {
	cm := newTestContractManager()
	mdi := cm.database.(*databasemocks.Plugin)

	ffi := &fftypes.FFI{
		Name:      "math",
		Version:   "1.0.0",
		Namespace: "default",
		Errors: []*fftypes.FFIError{
			{
				FFIErrorDefinition: fftypes.FFIErrorDefinition{
					Name: "",
					Params: []*fftypes.FFIParam{
						{
							Name:   "z",
							Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
						},
					},
				},
			},
		},
	}

	mdi.On("GetFFI", context.Background(), "ns1", "math", "1.0.0").Return(nil, nil)

	err := cm.ResolveFFI(context.Background(), ffi)
	assert.Regexp(t, "FF10433", err)
}

func TestValidateFFIBadErrorParam(t *testing.T) {
	cm := newTestContractManager()
	mdi := cm.database.(*databasemocks.Plugin)

	ffi := &fftypes.FFI{
		Name:      "math",
		Version:   "1.0.0",
		Namespace: "default",
		Errors: []*fftypes.FFIError{
			{
				FFIErrorDefinition: fftypes.FFIErrorDefinition{
					Name: "pop",
					Params: []*fftypes.FFIParam{
						{
							Name:   "z",
							Schema: fftypes.JSONAnyPtr(`{"type": "wrongness"`),
						},
					},
				},
			},
		},
	}

	mdi.On("GetFFI", context.Background(), "ns1", "math", "1.0.0").Return(nil, nil)

	err := cm.ResolveFFI(context.Background(), ffi)
	assert.Regexp(t, "FF10332", err)
}

func TestAddContractListenerInline(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdi := cm.database.(*databasemocks.Plugin)

	sub := &core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
				"address": "0x123",
			}.String()),
			Event: &core.FFISerializedEvent{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "changed",
					Params: fftypes.FFIParams{
						{
							Name:   "value",
							Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
						},
					},
				},
			},
			Options: &core.ContractListenerOptions{},
			Topic:   "test-topic",
		},
	}

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, sub.Location).Return(sub.Location, nil)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("changed", nil)
	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, sub.Location).Return("0x123:changed", nil)
	mdi.On("GetContractListeners", context.Background(), "ns1", mock.Anything).Return(nil, nil, nil)
	mbi.On("AddContractListener", context.Background(), &sub.ContractListener, "").Return(nil)
	mdi.On("InsertContractListener", context.Background(), &sub.ContractListener).Return(nil)

	result, err := cm.AddContractListener(context.Background(), sub)
	assert.NoError(t, err)
	assert.NotNil(t, result.ID)
	assert.NotNil(t, result.Event)

	mbi.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestAddContractListenerInlineNilLocation(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdi := cm.database.(*databasemocks.Plugin)

	sub := &core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Event: &core.FFISerializedEvent{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "changed",
					Params: fftypes.FFIParams{
						{
							Name:   "value",
							Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
						},
					},
				},
			},
			Options: &core.ContractListenerOptions{},
			Topic:   "test-topic",
		},
	}

	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("changed", nil)
	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, mock.Anything).Return("*:changed", nil)
	mdi.On("GetContractListeners", context.Background(), "ns1", mock.Anything).Return(nil, nil, nil)
	mbi.On("AddContractListener", context.Background(), mock.MatchedBy(func(cl *core.ContractListener) bool {
		// Normalize is not called for this case
		return cl.Location == nil
	}), "").Return(nil)
	mdi.On("InsertContractListener", context.Background(), &sub.ContractListener).Return(nil)

	result, err := cm.AddContractListener(context.Background(), sub)
	assert.NoError(t, err)
	assert.NotNil(t, result.ID)
	assert.NotNil(t, result.Event)

	mbi.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestAddContractListenerNoLocationOK(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdi := cm.database.(*databasemocks.Plugin)

	sub := &core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Event: &core.FFISerializedEvent{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "changed",
					Params: fftypes.FFIParams{
						{
							Name:   "value",
							Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
						},
					},
				},
			},
			Options: &core.ContractListenerOptions{},
			Topic:   "test-topic",
		},
	}

	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("changed", nil)
	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, mock.Anything).Return("*:changed", nil)
	mdi.On("GetContractListeners", context.Background(), "ns1", mock.Anything).Return(nil, nil, nil)
	mbi.On("AddContractListener", context.Background(), &sub.ContractListener, "").Return(nil)
	mdi.On("InsertContractListener", context.Background(), &sub.ContractListener).Return(nil)

	result, err := cm.AddContractListener(context.Background(), sub)
	assert.NoError(t, err)
	assert.NotNil(t, result.ID)
	assert.NotNil(t, result.Event)

	mbi.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestAddContractListenerByEventPath(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdi := cm.database.(*databasemocks.Plugin)

	interfaceID := fftypes.NewUUID()

	event := &fftypes.FFIEvent{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		FFIEventDefinition: fftypes.FFIEventDefinition{
			Name: "changed",
			Params: fftypes.FFIParams{
				{
					Name:   "value",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
				},
			},
		},
	}

	sub := &core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Interface: &fftypes.FFIReference{
				ID: interfaceID,
			},
			Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
				"address": "0x123",
			}.String()),
			Topic: "test-topic",
		},
		EventPath: "changed",
	}

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, sub.Location).Return(sub.Location, nil)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("changed", nil)
	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, mock.Anything).Return("0x123:changed", nil)
	mdi.On("GetContractListeners", context.Background(), "ns1", mock.Anything).Return(nil, nil, nil)
	mbi.On("AddContractListener", context.Background(), &sub.ContractListener, "").Return(nil)
	mdi.On("GetFFIByID", context.Background(), "ns1", interfaceID).Return(&fftypes.FFI{}, nil)
	mdi.On("GetFFIEvent", context.Background(), "ns1", interfaceID, sub.EventPath).Return(event, nil)
	mdi.On("InsertContractListener", context.Background(), &sub.ContractListener).Return(nil)

	result, err := cm.AddContractListener(context.Background(), sub)
	assert.NoError(t, err)
	assert.NotNil(t, result.ID)
	assert.NotNil(t, result.Filters[0].Event)

	mbi.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestAddContractListenerBadLocation(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)

	sub := &core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Interface: &fftypes.FFIReference{
				ID: fftypes.NewUUID(),
			},
			Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
				"address": "0x123",
			}.String()),
			Topic: "test-topic",
		},
		EventPath: "changed",
	}

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, sub.Location).Return(nil, fmt.Errorf("pop"))

	_, err := cm.AddContractListener(context.Background(), sub)
	assert.EqualError(t, err, "pop")

	mbi.AssertExpectations(t)
}

func TestAddContractListenerFFILookupFail(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdi := cm.database.(*databasemocks.Plugin)

	interfaceID := fftypes.NewUUID()

	sub := &core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Interface: &fftypes.FFIReference{
				ID: interfaceID,
			},
			Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
				"address": "0x123",
			}.String()),
			Topic: "test-topic",
		},
		EventPath: "changed",
	}

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, sub.Location).Return(sub.Location, nil)
	mdi.On("GetFFIByID", context.Background(), "ns1", interfaceID).Return(nil, fmt.Errorf("pop"))

	_, err := cm.AddContractListener(context.Background(), sub)
	assert.EqualError(t, err, "pop")

	mbi.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestAddContractListenerEventLookupFail(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdi := cm.database.(*databasemocks.Plugin)

	interfaceID := fftypes.NewUUID()

	sub := &core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Interface: &fftypes.FFIReference{
				ID: interfaceID,
			},
			Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
				"address": "0x123",
			}.String()),
			Topic: "test-topic",
		},
		EventPath: "changed",
	}

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, sub.Location).Return(sub.Location, nil)
	mdi.On("GetFFIByID", context.Background(), "ns1", interfaceID).Return(&fftypes.FFI{}, nil)
	mdi.On("GetFFIEvent", context.Background(), "ns1", interfaceID, sub.EventPath).Return(nil, fmt.Errorf("pop"))

	_, err := cm.AddContractListener(context.Background(), sub)
	assert.EqualError(t, err, "pop")

	mbi.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestAddContractListenerEventLookupNotFound(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdi := cm.database.(*databasemocks.Plugin)

	interfaceID := fftypes.NewUUID()

	sub := &core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Interface: &fftypes.FFIReference{
				ID: interfaceID,
			},
			Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
				"address": "0x123",
			}.String()),
			Topic: "test-topic",
		},
		EventPath: "changed",
	}

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, sub.Location).Return(sub.Location, nil)
	mdi.On("GetFFIByID", context.Background(), "ns1", interfaceID).Return(&fftypes.FFI{}, nil)
	mdi.On("GetFFIEvent", context.Background(), "ns1", interfaceID, sub.EventPath).Return(nil, nil)

	_, err := cm.AddContractListener(context.Background(), sub)
	assert.Regexp(t, "FF10370", err)

	mbi.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestAddContractListenerMissingEventOrID(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)

	sub := &core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
				"address": "0x123",
			}.String()),
			Topic: "test-topic",
		},
	}

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, sub.Location).Return(sub.Location, nil)

	_, err := cm.AddContractListener(context.Background(), sub)
	assert.Regexp(t, "FF10317", err)

	mbi.AssertExpectations(t)
}

func TestAddContractListenerVerifyOk(t *testing.T) {
	cm := newTestContractManager()

	ctx := context.Background()

	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetContractListeners", mock.Anything, "ns1", mock.MatchedBy(func(f ffapi.Filter) bool {
		fi, _ := f.Finalize()
		return fi.Skip == 0 && fi.Limit == 50
	})).Return([]*core.ContractListener{
		{Namespace: "ns1", ID: fftypes.NewUUID(), BackendID: "12345"},
		{Namespace: "ns1", ID: fftypes.NewUUID(), BackendID: "23456"},
	}, nil, nil).Once()
	mdi.On("GetContractListeners", mock.Anything, "ns1", mock.MatchedBy(func(f ffapi.Filter) bool {
		fi, _ := f.Finalize()
		return fi.Skip == 50 && fi.Limit == 50
	})).Return([]*core.ContractListener{}, nil, nil).Once()
	mdi.On("GetBlockchainEvents", mock.Anything, "ns1", mock.MatchedBy(func(f ffapi.Filter) bool {
		fi, err := f.Finalize()
		assert.NoError(t, err)
		return fi.Limit == 1 && strings.Contains(fi.String(), "listener")
	})).Return([]*core.BlockchainEvent{
		{Namespace: "ns1", ID: fftypes.NewUUID(), ProtocolID: "001/002/003"},
	}, nil, nil).Once()

	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mbi.On("GetContractListenerStatus", ctx, "ns1", "12345", true).Return(true, struct{}{}, core.ContractListenerStatusSynced, nil)
	mbi.On("GetContractListenerStatus", ctx, "ns1", "23456", true).Return(false, nil, core.ContractListenerStatusUnknown, nil)
	mbi.On("AddContractListener", ctx, mock.MatchedBy(func(l *core.ContractListener) bool {
		prevBackendID := l.BackendID
		l.BackendID = "34567"
		return prevBackendID == "23456"
	}), "001/002/003").Return(nil)

	mdi.On("UpdateContractListener", ctx, "ns1", mock.Anything, mock.MatchedBy(func(u ffapi.Update) bool {
		uu, _ := u.Finalize()
		return strings.Contains(uu.String(), "34567")
	})).Return(nil).Once()

	err := cm.verifyListeners(ctx)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
	mbi.AssertExpectations(t)
}

func TestAddContractListenerVerifyUpdateFail(t *testing.T) {
	cm := newTestContractManager()

	ctx := context.Background()

	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetContractListeners", mock.Anything, "ns1", mock.MatchedBy(func(f ffapi.Filter) bool {
		fi, _ := f.Finalize()
		return fi.Skip == 0 && fi.Limit == 50
	})).Return([]*core.ContractListener{
		{Namespace: "ns1", ID: fftypes.NewUUID(), BackendID: "12345"},
		{Namespace: "ns1", ID: fftypes.NewUUID(), BackendID: "23456"},
	}, nil, nil).Once()
	mdi.On("GetBlockchainEvents", mock.Anything, "ns1", mock.Anything).Return([]*core.BlockchainEvent{}, nil, nil).Once()

	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mbi.On("GetContractListenerStatus", ctx, "ns1", "12345", true).Return(true, struct{}{}, core.ContractListenerStatusSynced, nil)
	mbi.On("GetContractListenerStatus", ctx, "ns1", "23456", true).Return(false, nil, core.ContractListenerStatusUnknown, nil)
	mbi.On("AddContractListener", ctx, mock.MatchedBy(func(l *core.ContractListener) bool {
		prevBackendID := l.BackendID
		l.BackendID = "34567"
		return prevBackendID == "23456"
	}), "").Return(nil)

	mdi.On("UpdateContractListener", ctx, "ns1", mock.Anything, mock.MatchedBy(func(u ffapi.Update) bool {
		uu, _ := u.Finalize()
		return strings.Contains(uu.String(), "34567")
	})).Return(fmt.Errorf("pop")).Once()

	err := cm.verifyListeners(ctx)
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
	mbi.AssertExpectations(t)
}

func TestAddContractListenerVerifyAddFail(t *testing.T) {
	cm := newTestContractManager()

	ctx := context.Background()

	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetContractListeners", mock.Anything, "ns1", mock.MatchedBy(func(f ffapi.Filter) bool {
		fi, _ := f.Finalize()
		return fi.Skip == 0 && fi.Limit == 50
	})).Return([]*core.ContractListener{
		{Namespace: "ns1", ID: fftypes.NewUUID(), BackendID: "12345"},
		{Namespace: "ns1", ID: fftypes.NewUUID(), BackendID: "23456"},
	}, nil, nil).Once()
	mdi.On("GetBlockchainEvents", mock.Anything, "ns1", mock.Anything).Return([]*core.BlockchainEvent{}, nil, nil).Once()

	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mbi.On("GetContractListenerStatus", ctx, "ns1", "12345", true).Return(true, struct{}{}, core.ContractListenerStatusSynced, nil)
	mbi.On("GetContractListenerStatus", ctx, "ns1", "23456", true).Return(false, nil, core.ContractListenerStatusUnknown, nil)
	mbi.On("AddContractListener", ctx, mock.MatchedBy(func(l *core.ContractListener) bool {
		prevBackendID := l.BackendID
		l.BackendID = "34567"
		return prevBackendID == "23456"
	}), "").Return(fmt.Errorf("pop"))

	err := cm.verifyListeners(ctx)
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
	mbi.AssertExpectations(t)
}

func TestAddContractListenerGetEventsFail(t *testing.T) {
	cm := newTestContractManager()

	ctx := context.Background()

	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetContractListeners", mock.Anything, "ns1", mock.MatchedBy(func(f ffapi.Filter) bool {
		fi, _ := f.Finalize()
		return fi.Skip == 0 && fi.Limit == 50
	})).Return([]*core.ContractListener{
		{Namespace: "ns1", ID: fftypes.NewUUID(), BackendID: "12345"},
		{Namespace: "ns1", ID: fftypes.NewUUID(), BackendID: "23456"},
	}, nil, nil).Once()
	mdi.On("GetBlockchainEvents", mock.Anything, "ns1", mock.Anything).
		Return(nil, nil, fmt.Errorf("pop")).Once()

	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mbi.On("GetContractListenerStatus", ctx, "ns1", "12345", true).Return(true, struct{}{}, core.ContractListenerStatusSynced, nil)
	mbi.On("GetContractListenerStatus", ctx, "ns1", "23456", true).Return(false, nil, core.ContractListenerStatusUnknown, nil)

	err := cm.verifyListeners(ctx)
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
	mbi.AssertExpectations(t)
}

func TestAddContractListenerVerifyGetFail(t *testing.T) {
	cm := newTestContractManager()

	ctx := context.Background()

	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetContractListeners", mock.Anything, "ns1", mock.MatchedBy(func(f ffapi.Filter) bool {
		fi, _ := f.Finalize()
		return fi.Skip == 0 && fi.Limit == 50
	})).Return([]*core.ContractListener{
		{Namespace: "ns1", ID: fftypes.NewUUID(), BackendID: "12345"},
		{Namespace: "ns1", ID: fftypes.NewUUID(), BackendID: "23456"},
	}, nil, nil).Once()

	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mbi.On("GetContractListenerStatus", ctx, "ns1", "12345", true).Return(false, nil, core.ContractListenerStatusUnknown, fmt.Errorf("pop"))

	err := cm.verifyListeners(ctx)
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
	mbi.AssertExpectations(t)
}

func TestAddContractListenerVerifyGetListFail(t *testing.T) {
	cm := newTestContractManager()

	ctx := context.Background()

	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetContractListeners", mock.Anything, "ns1", mock.Anything).Return(nil, nil, fmt.Errorf("pop")).Once()

	err := cm.verifyListeners(ctx)
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
}

func TestAddContractListenerVerifyMigration(t *testing.T) {
	cm := newTestContractManager()

	ctx := context.Background()

	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetContractListeners", mock.Anything, "ns1", mock.MatchedBy(func(f ffapi.Filter) bool {
		fi, _ := f.Finalize()
		return fi.Skip == 0 && fi.Limit == 50
	})).Return([]*core.ContractListener{
		{Namespace: "ns1", ID: fftypes.NewUUID(), BackendID: "12345"},
		{
			Namespace: "ns1",
			ID:        fftypes.NewUUID(),
			BackendID: "23456",
			Event: &core.FFISerializedEvent{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "changed",
					Params: fftypes.FFIParams{
						{
							Name:   "value",
							Schema: fftypes.JSONAnyPtr(`{"type": "null"}`),
						},
					},
				},
			},
		},
	}, nil, nil).Once()
	mdi.On("GetContractListeners", mock.Anything, "ns1", mock.MatchedBy(func(f ffapi.Filter) bool {
		fi, _ := f.Finalize()
		return fi.Skip == 50 && fi.Limit == 50
	})).Return([]*core.ContractListener{}, nil, nil).Once()
	mdi.On("UpsertContractListener", context.Background(), mock.Anything, true).Return(nil)

	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything, mock.Anything).Return("changed", nil)
	mdi.On("GetBlockchainEvents", mock.Anything, "ns1", mock.Anything).Return([]*core.BlockchainEvent{}, nil, nil).Once()
	mbi.On("GetContractListenerStatus", ctx, "ns1", "12345", true).Return(true, struct{}{}, core.ContractListenerStatusSynced, nil)
	mbi.On("GetContractListenerStatus", ctx, "ns1", "23456", true).Return(false, nil, core.ContractListenerStatusUnknown, nil)
	mbi.On("AddContractListener", ctx, mock.MatchedBy(func(l *core.ContractListener) bool {
		prevBackendID := l.BackendID
		l.BackendID = "34567"
		return prevBackendID == "23456" && len(l.Filters) != 0
	}), "").Return(nil)

	mdi.On("UpdateContractListener", ctx, "ns1", mock.Anything, mock.MatchedBy(func(u ffapi.Update) bool {
		uu, _ := u.Finalize()
		return strings.Contains(uu.String(), "34567")
	})).Return(nil).Once()

	err := cm.verifyListeners(ctx)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
	mbi.AssertExpectations(t)
}

func TestVerifyListenersFailMigration(t *testing.T) {
	cm := newTestContractManager()

	ctx := context.Background()

	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetContractListeners", mock.Anything, "ns1", mock.MatchedBy(func(f ffapi.Filter) bool {
		fi, _ := f.Finalize()
		return fi.Skip == 0 && fi.Limit == 50
	})).Return([]*core.ContractListener{
		{Namespace: "ns1", ID: fftypes.NewUUID(), BackendID: "12345"},
		{
			Namespace: "ns1",
			ID:        fftypes.NewUUID(),
			BackendID: "23456",
			Event: &core.FFISerializedEvent{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "changed",
					Params: fftypes.FFIParams{
						{
							Name:   "value",
							Schema: fftypes.JSONAnyPtr(`{"type": "null"}`),
						},
					},
				},
			},
		},
	}, nil, nil).Once()
	mdi.On("UpsertContractListener", context.Background(), mock.Anything, true).Return(fmt.Errorf("pop"))

	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything, mock.Anything).Return("changed", nil)
	mdi.On("GetBlockchainEvents", mock.Anything, "ns1", mock.Anything).Return([]*core.BlockchainEvent{}, nil, nil).Once()
	mbi.On("GetContractListenerStatus", ctx, "ns1", "12345", true).Return(true, struct{}{}, core.ContractListenerStatusSynced, nil)
	mbi.On("GetContractListenerStatus", ctx, "ns1", "23456", true).Return(false, nil, core.ContractListenerStatusUnknown, nil)
	mbi.On("AddContractListener", ctx, mock.MatchedBy(func(l *core.ContractListener) bool {
		prevBackendID := l.BackendID
		l.BackendID = "34567"
		return prevBackendID == "23456" && len(l.Filters) != 0
	}), "").Return(nil)
	mdi.On("UpdateContractListener", ctx, "ns1", mock.Anything, mock.MatchedBy(func(u ffapi.Update) bool {
		uu, _ := u.Finalize()
		return strings.Contains(uu.String(), "34567")
	})).Return(nil).Once()

	err := cm.verifyListeners(ctx)
	assert.Error(t, err)
	assert.Regexp(t, "pop", err.Error())

	mdi.AssertExpectations(t)
	mbi.AssertExpectations(t)
}

func TestVerifyListenersFailUpgradeFilters(t *testing.T) {
	cm := newTestContractManager()

	ctx := context.Background()

	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetContractListeners", mock.Anything, "ns1", mock.MatchedBy(func(f ffapi.Filter) bool {
		fi, _ := f.Finalize()
		return fi.Skip == 0 && fi.Limit == 50
	})).Return([]*core.ContractListener{
		{Namespace: "ns1", ID: fftypes.NewUUID(), BackendID: "12345"},
		{
			Namespace: "ns1",
			ID:        fftypes.NewUUID(),
			BackendID: "23456",
			Event: &core.FFISerializedEvent{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "changed",
					Params: fftypes.FFIParams{
						{
							Name:   "value",
							Schema: fftypes.JSONAnyPtr(`{"type": "null"}`),
						},
					},
				},
			},
		},
	}, nil, nil).Once()

	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything, mock.Anything).Return("changed", fmt.Errorf("pop"))
	mbi.On("GetContractListenerStatus", ctx, "ns1", "12345", true).Return(true, struct{}{}, core.ContractListenerStatusSynced, nil)

	err := cm.verifyListeners(ctx)
	assert.Error(t, err)
	assert.Regexp(t, "pop", err.Error())

	mdi.AssertExpectations(t)
	mbi.AssertExpectations(t)
}

func TestAddContractListenerBadName(t *testing.T) {
	cm := newTestContractManager()
	sub := &core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Name: "!bad",
		},
	}

	_, err := cm.AddContractListener(context.Background(), sub)
	assert.Regexp(t, "FF00140.*'name'", err)
}

func TestAddContractListenerMissingTopic(t *testing.T) {
	cm := newTestContractManager()
	sub := &core.ContractListenerInput{
		ContractListener: core.ContractListener{},
	}

	_, err := cm.AddContractListener(context.Background(), sub)
	assert.Regexp(t, "FF00140.*'topic'", err)
}

func TestAddContractListenerNoInterface(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdi := cm.database.(*databasemocks.Plugin)

	sub := &core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Name: "sub1",
			Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
				"address": "0x123",
			}.String()),
			Topic: "test-topic",
		},
		EventPath: "changed",
	}

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, sub.Location).Return(sub.Location, nil)

	_, err := cm.AddContractListener(context.Background(), sub)
	assert.Regexp(t, "FF10317", err)

	mbi.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestAddContractListenerNameConflict(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdi := cm.database.(*databasemocks.Plugin)
	interfaceID := fftypes.NewUUID()

	sub := &core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Name: "sub1",
			Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
				"address": "0x123",
			}.String()),
			Topic: "test-topic",
			Interface: &fftypes.FFIReference{
				ID: interfaceID,
			},
		},
		EventPath: "changed",
	}

	mdi.On("GetFFIByID", context.Background(), "ns1", interfaceID).Return(&fftypes.FFI{}, nil)
	mdi.On("GetFFIEvent", context.Background(), "ns1", interfaceID, "changed").Return(&fftypes.FFIEvent{
		FFIEventDefinition: fftypes.FFIEventDefinition{
			Name: "changed",
		},
	}, nil)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("changed", nil)
	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, mock.Anything).Return("0x123:changed", nil)
	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, sub.Location).Return(sub.Location, nil)
	mdi.On("GetContractListener", context.Background(), "ns1", "sub1").Return(&core.ContractListener{}, nil)

	_, err := cm.AddContractListener(context.Background(), sub)
	assert.Regexp(t, "FF10312", err)

	mbi.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestAddContractListenerNameError(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdi := cm.database.(*databasemocks.Plugin)

	ffiID := fftypes.NewUUID()

	sub := &core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Name: "sub1",
			Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
				"address": "0x123",
			}.String()),
			Topic: "test-topic",
		},
		Filters: core.ListenerFiltersInput{
			&core.ListenerFilterInput{
				ListenerFilter: core.ListenerFilter{
					Interface: &fftypes.FFIReference{
						ID: ffiID,
					},
				},
				EventPath: "set",
			},
		},
	}

	mdi.On("GetFFIByID", context.Background(), "ns1", ffiID).Return(&fftypes.FFI{}, nil)
	mdi.On("GetFFIEvent", context.Background(), "ns1", ffiID, "set").Return(&fftypes.FFIEvent{
		FFIEventDefinition: fftypes.FFIEventDefinition{
			Name: "eventName",
		},
	}, nil)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("eventSignature", nil)
	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, mock.Anything).Return("0x123:eventSignature", nil)
	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, sub.Location).Return(sub.Location, nil)
	mdi.On("GetContractListener", context.Background(), "ns1", "sub1").Return(nil, fmt.Errorf("pop"))

	_, err := cm.AddContractListener(context.Background(), sub)
	assert.EqualError(t, err, "pop")

	mbi.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestAddContractListenerFiltersAndDeprecatedFail(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdi := cm.database.(*databasemocks.Plugin)

	sub := &core.ContractListenerInput{
		Filters: core.ListenerFiltersInput{
			{
				ListenerFilter: core.ListenerFilter{
					Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
						"address": "0x123",
					}.String()),
				},
			},
		},
		ContractListener: core.ContractListener{
			Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
				"address": "0x123",
			}.String()),
			Event: &core.FFISerializedEvent{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "changed",
					Params: fftypes.FFIParams{
						{
							Name:   "value",
							Schema: fftypes.JSONAnyPtr(`{"type": "null"}`),
						},
					},
				},
			},
			Topic: "test-topic",
		},
	}

	_, err := cm.AddContractListener(context.Background(), sub)
	assert.Regexp(t, "FF10474", err)

	mbi.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestAddContractListenerValidateFail(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdi := cm.database.(*databasemocks.Plugin)

	sub := &core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
				"address": "0x123",
			}.String()),
			Event: &core.FFISerializedEvent{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "changed",
					Params: fftypes.FFIParams{
						{
							Name:   "value",
							Schema: fftypes.JSONAnyPtr(`{"type": "null"}`),
						},
					},
				},
			},
			Topic: "test-topic",
		},
	}

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, sub.Location).Return(sub.Location, nil)

	_, err := cm.AddContractListener(context.Background(), sub)
	assert.Regexp(t, "does not validate", err)

	mbi.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestAddContractListenerBlockchainFail(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdi := cm.database.(*databasemocks.Plugin)

	sub := &core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
				"address": "0x123",
			}.String()),
			Event: &core.FFISerializedEvent{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "changed",
					Params: fftypes.FFIParams{
						{
							Name:   "value",
							Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
						},
					},
				},
			},
			Topic: "test-topic",
		},
	}

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, sub.Location).Return(sub.Location, nil)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("changed", nil)
	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, mock.Anything).Return("0x123:changed", nil)
	mdi.On("GetContractListeners", context.Background(), "ns1", mock.Anything).Return(nil, nil, nil)
	mbi.On("AddContractListener", context.Background(), &sub.ContractListener, "").Return(fmt.Errorf("pop"))

	_, err := cm.AddContractListener(context.Background(), sub)
	assert.EqualError(t, err, "pop")

	mbi.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestAddContractListenerUpsertSubFail(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdi := cm.database.(*databasemocks.Plugin)

	sub := &core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
				"address": "0x123",
			}.String()),
			Event: &core.FFISerializedEvent{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "changed",
					Params: fftypes.FFIParams{
						{
							Name:   "value",
							Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
						},
					},
				},
			},
			Topic: "test-topic",
		},
	}

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, sub.Location).Return(sub.Location, nil)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("changed", nil)
	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, mock.Anything).Return("0x123:changed", nil)
	mdi.On("GetContractListeners", context.Background(), "ns1", mock.Anything).Return(nil, nil, nil)
	mbi.On("AddContractListener", context.Background(), &sub.ContractListener, "").Return(nil)
	mdi.On("InsertContractListener", context.Background(), &sub.ContractListener).Return(fmt.Errorf("pop"))

	_, err := cm.AddContractListener(context.Background(), sub)
	assert.EqualError(t, err, "pop")

	mbi.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestAddContractAPIListener(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdi := cm.database.(*databasemocks.Plugin)

	interfaceID := fftypes.NewUUID()
	api := &core.ContractAPI{
		Interface: &fftypes.FFIReference{
			ID: interfaceID,
		},
		Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
			"address": "0x123",
		}.String()),
	}
	listener := &core.ContractListener{
		Topic: "test-topic",
	}
	event := &fftypes.FFIEvent{
		FFIEventDefinition: fftypes.FFIEventDefinition{
			Name: "changed",
		},
	}

	mdi.On("GetContractAPIByName", context.Background(), "ns1", "simple").Return(api, nil)
	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, api.Location).Return(listener.Location, nil)
	mdi.On("GetFFIByID", context.Background(), "ns1", interfaceID).Return(&fftypes.FFI{}, nil)
	mdi.On("GetFFIEvent", context.Background(), "ns1", interfaceID, "changed").Return(event, nil)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("changed", nil)
	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, mock.Anything).Return("0x123:changed", nil)
	mdi.On("GetContractListeners", context.Background(), "ns1", mock.Anything).Return(nil, nil, nil)
	mbi.On("AddContractListener", context.Background(), mock.MatchedBy(func(l *core.ContractListener) bool {
		return *l.Interface.ID == *interfaceID && l.Topic == "test-topic"
	}), "").Return(nil)
	mdi.On("InsertContractListener", context.Background(), mock.MatchedBy(func(l *core.ContractListener) bool {
		return *l.Filters[0].Interface.ID == *interfaceID && l.Filters[0].Event.Name == "changed" && l.Topic == "test-topic"
	})).Return(nil)

	_, err := cm.AddContractAPIListener(context.Background(), "simple", "changed", listener)
	assert.NoError(t, err)

	mbi.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestAddContractAPIListenerNotFound(t *testing.T) {
	cm := newTestContractManager()
	mdi := cm.database.(*databasemocks.Plugin)

	listener := &core.ContractListener{
		Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
			"address": "0x123",
		}.String()),
		Topic: "test-topic",
	}

	mdi.On("GetContractAPIByName", context.Background(), "ns1", "simple").Return(nil, nil)

	_, err := cm.AddContractAPIListener(context.Background(), "simple", "changed", listener)
	assert.Regexp(t, "FF10109", err)

	mdi.AssertExpectations(t)
}

func TestAddContractAPIListenerFail(t *testing.T) {
	cm := newTestContractManager()
	mdi := cm.database.(*databasemocks.Plugin)

	listener := &core.ContractListener{
		Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
			"address": "0x123",
		}.String()),
		Topic: "test-topic",
	}

	mdi.On("GetContractAPIByName", context.Background(), "ns1", "simple").Return(nil, fmt.Errorf("pop"))

	_, err := cm.AddContractAPIListener(context.Background(), "simple", "changed", listener)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestGetFFI(t *testing.T) {
	cm := newTestContractManager()
	mdb := cm.database.(*databasemocks.Plugin)
	mdb.On("GetFFI", mock.Anything, "ns1", "ffi", "v1.0.0").Return(&fftypes.FFI{}, nil)
	_, err := cm.GetFFI(context.Background(), "ffi", "v1.0.0")
	assert.NoError(t, err)
}

func TestGetFFINotFound(t *testing.T) {
	cm := newTestContractManager()
	mdb := cm.database.(*databasemocks.Plugin)
	mdb.On("GetFFI", mock.Anything, "ns1", "ffi", "v1.0.0").Return(nil, nil)
	_, err := cm.GetFFI(context.Background(), "ffi", "v1.0.0")
	assert.Regexp(t, "FF10109", err)
}

func TestGetFFIFail(t *testing.T) {
	cm := newTestContractManager()
	mdb := cm.database.(*databasemocks.Plugin)
	mdb.On("GetFFI", mock.Anything, "ns1", "ffi", "v1.0.0").Return(nil, fmt.Errorf("pop"))
	_, err := cm.GetFFI(context.Background(), "ffi", "v1.0.0")
	assert.EqualError(t, err, "pop")
}

func TestGetFFIWithChildren(t *testing.T) {
	cm := newTestContractManager()
	mdb := cm.database.(*databasemocks.Plugin)
	mbi := cm.blockchain.(*blockchainmocks.Plugin)

	cid := fftypes.NewUUID()
	mdb.On("GetFFI", mock.Anything, "ns1", "ffi", "v1.0.0").Return(&fftypes.FFI{ID: cid}, nil)
	mdb.On("GetFFIMethods", mock.Anything, "ns1", mock.Anything).Return([]*fftypes.FFIMethod{
		{ID: fftypes.NewUUID(), Name: "method1"},
	}, nil, nil)
	mdb.On("GetFFIEvents", mock.Anything, "ns1", mock.Anything).Return([]*fftypes.FFIEvent{
		{ID: fftypes.NewUUID(), FFIEventDefinition: fftypes.FFIEventDefinition{Name: "event1"}},
	}, nil, nil)
	mbi.On("GenerateEventSignature", mock.Anything, mock.MatchedBy(func(ev *fftypes.FFIEventDefinition) bool {
		return ev.Name == "event1"
	})).Return("event1Sig", nil)
	mdb.On("GetFFIErrors", mock.Anything, "ns1", mock.Anything).Return([]*fftypes.FFIError{
		{ID: fftypes.NewUUID(), FFIErrorDefinition: fftypes.FFIErrorDefinition{Name: "customError1"}},
	}, nil, nil)
	mbi.On("GenerateErrorSignature", mock.Anything, mock.MatchedBy(func(ev *fftypes.FFIErrorDefinition) bool {
		return ev.Name == "customError1"
	})).Return("error1Sig")

	_, err := cm.GetFFIWithChildren(context.Background(), "ffi", "v1.0.0")
	assert.NoError(t, err)

	mdb.AssertExpectations(t)
	mbi.AssertExpectations(t)
}

func TestGetFFIByID(t *testing.T) {
	cm := newTestContractManager()
	mdb := cm.database.(*databasemocks.Plugin)
	cid := fftypes.NewUUID()
	mdb.On("GetFFIByID", mock.Anything, "ns1", cid).Return(&fftypes.FFI{}, nil)
	_, err := cm.GetFFIByID(context.Background(), cid)
	assert.NoError(t, err)
}

func TestGetFFIByIDWithChildren(t *testing.T) {
	cm := newTestContractManager()
	mdb := cm.database.(*databasemocks.Plugin)
	mbi := cm.blockchain.(*blockchainmocks.Plugin)

	cid := fftypes.NewUUID()
	mdb.On("GetFFIByID", mock.Anything, "ns1", cid).Return(&fftypes.FFI{
		ID: cid,
	}, nil)
	mdb.On("GetFFIMethods", mock.Anything, "ns1", mock.Anything).Return([]*fftypes.FFIMethod{
		{ID: fftypes.NewUUID(), Name: "method1"},
	}, nil, nil)
	mdb.On("GetFFIEvents", mock.Anything, "ns1", mock.Anything).Return([]*fftypes.FFIEvent{
		{ID: fftypes.NewUUID(), FFIEventDefinition: fftypes.FFIEventDefinition{Name: "event1"}},
	}, nil, nil)
	mbi.On("GenerateEventSignature", mock.Anything, mock.MatchedBy(func(ev *fftypes.FFIEventDefinition) bool {
		return ev.Name == "event1"
	})).Return("event1Sig", nil)
	mdb.On("GetFFIErrors", mock.Anything, "ns1", mock.Anything).Return([]*fftypes.FFIError{
		{ID: fftypes.NewUUID(), FFIErrorDefinition: fftypes.FFIErrorDefinition{Name: "customError1"}},
	}, nil, nil)
	mbi.On("GenerateErrorSignature", mock.Anything, mock.MatchedBy(func(ev *fftypes.FFIErrorDefinition) bool {
		return ev.Name == "customError1"
	})).Return("error1Sig")

	ffi, err := cm.GetFFIByIDWithChildren(context.Background(), cid)

	assert.NoError(t, err)
	mdb.AssertExpectations(t)
	mbi.AssertExpectations(t)

	assert.Equal(t, "method1", ffi.Methods[0].Name)
	assert.Equal(t, "event1", ffi.Events[0].Name)
}

func TestGetFFIByIDWithChildrenErrorsFail(t *testing.T) {
	cm := newTestContractManager()
	mdb := cm.database.(*databasemocks.Plugin)

	cid := fftypes.NewUUID()
	mdb.On("GetFFIByID", mock.Anything, "ns1", cid).Return(&fftypes.FFI{
		ID: cid,
	}, nil)
	mdb.On("GetFFIMethods", mock.Anything, "ns1", mock.Anything).Return([]*fftypes.FFIMethod{
		{ID: fftypes.NewUUID(), Name: "method1"},
	}, nil, nil)
	mdb.On("GetFFIEvents", mock.Anything, "ns1", mock.Anything).Return([]*fftypes.FFIEvent{}, nil, nil)
	mdb.On("GetFFIErrors", mock.Anything, "ns1", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	_, err := cm.GetFFIByIDWithChildren(context.Background(), cid)

	assert.EqualError(t, err, "pop")
	mdb.AssertExpectations(t)
}

func TestGetFFIByIDWithChildrenEventsFail(t *testing.T) {
	cm := newTestContractManager()
	mdb := cm.database.(*databasemocks.Plugin)

	cid := fftypes.NewUUID()
	mdb.On("GetFFIByID", mock.Anything, "ns1", cid).Return(&fftypes.FFI{
		ID: cid,
	}, nil)
	mdb.On("GetFFIMethods", mock.Anything, "ns1", mock.Anything).Return([]*fftypes.FFIMethod{
		{ID: fftypes.NewUUID(), Name: "method1"},
	}, nil, nil)
	mdb.On("GetFFIEvents", mock.Anything, "ns1", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	_, err := cm.GetFFIByIDWithChildren(context.Background(), cid)

	assert.EqualError(t, err, "pop")
	mdb.AssertExpectations(t)
}

func TestGetFFIByIDWithChildrenMethodsFail(t *testing.T) {
	cm := newTestContractManager()
	mdb := cm.database.(*databasemocks.Plugin)

	cid := fftypes.NewUUID()
	mdb.On("GetFFIByID", mock.Anything, "ns1", cid).Return(&fftypes.FFI{
		ID: cid,
	}, nil)
	mdb.On("GetFFIMethods", mock.Anything, "ns1", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	_, err := cm.GetFFIByIDWithChildren(context.Background(), cid)

	assert.EqualError(t, err, "pop")
	mdb.AssertExpectations(t)
}

func TestGetFFIByIDWithChildrenFFILookupFail(t *testing.T) {
	cm := newTestContractManager()
	mdb := cm.database.(*databasemocks.Plugin)

	cid := fftypes.NewUUID()
	mdb.On("GetFFIByID", mock.Anything, "ns1", cid).Return(nil, fmt.Errorf("pop"))

	_, err := cm.GetFFIByIDWithChildren(context.Background(), cid)

	assert.EqualError(t, err, "pop")
	mdb.AssertExpectations(t)
}

func TestGetFFIByIDWithChildrenFFINotFound(t *testing.T) {
	cm := newTestContractManager()
	mdb := cm.database.(*databasemocks.Plugin)

	cid := fftypes.NewUUID()
	mdb.On("GetFFIByID", mock.Anything, "ns1", cid).Return(nil, nil)

	ffi, err := cm.GetFFIByIDWithChildren(context.Background(), cid)

	assert.NoError(t, err)
	assert.Nil(t, ffi)
	mdb.AssertExpectations(t)
}

func TestGetFFIs(t *testing.T) {
	cm := newTestContractManager()
	mdb := cm.database.(*databasemocks.Plugin)
	filter := database.FFIQueryFactory.NewFilter(context.Background()).And()
	mdb.On("GetFFIs", mock.Anything, "ns1", filter).Return([]*fftypes.FFI{}, &ffapi.FilterResult{}, nil)
	_, _, err := cm.GetFFIs(context.Background(), filter)
	assert.NoError(t, err)
}

func TestDeployContract(t *testing.T) {
	cm := newTestContractManager()
	mim := cm.identity.(*identitymanagermocks.Manager)
	mdi := cm.database.(*databasemocks.Plugin)
	mth := cm.txHelper.(*txcommonmocks.Helper)
	mom := cm.operations.(*operationmocks.Manager)
	txw := cm.txWriter.(*txwritermocks.Writer)
	signingKey := "0x2468"
	req := &core.ContractDeployRequest{
		Key:            signingKey,
		Definition:     fftypes.JSONAnyPtr("[]"),
		Contract:       fftypes.JSONAnyPtr("\"0x123456\""),
		Input:          []interface{}{"one", "two", "three"},
		IdempotencyKey: "idem1",
	}

	txw.On("WriteTransactionAndOps", mock.Anything, core.TransactionTypeContractDeploy, core.IdempotencyKey("idem1"), mock.MatchedBy(func(op *core.Operation) bool {
		return op.Namespace == "ns1" && op.Type == core.OpTypeBlockchainContractDeploy && op.Plugin == "mockblockchain"
	})).Return(&core.Transaction{ID: fftypes.NewUUID()}, nil)
	mim.On("ResolveInputSigningKey", mock.Anything, signingKey, identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)
	mom.On("RunOperation", mock.Anything, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(blockchainContractDeployData)
		return op.Type == core.OpTypeBlockchainContractDeploy && data.Request == req
	}), true).Return(nil, nil)

	_, err := cm.DeployContract(context.Background(), req, false)

	assert.NoError(t, err)

	mth.AssertExpectations(t)
	mim.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestDeployContractIdempotentResubmitOperation(t *testing.T) {
	cm := newTestContractManager()
	var id = fftypes.NewUUID()
	mim := cm.identity.(*identitymanagermocks.Manager)
	mth := cm.txHelper.(*txcommonmocks.Helper)
	mom := cm.operations.(*operationmocks.Manager)
	txw := cm.txWriter.(*txwritermocks.Writer)
	signingKey := "0x2468"
	req := &core.ContractDeployRequest{
		Key:            signingKey,
		Definition:     fftypes.JSONAnyPtr("[]"),
		Contract:       fftypes.JSONAnyPtr("\"0x123456\""),
		Input:          []interface{}{"one", "two", "three"},
		IdempotencyKey: "idem1",
	}

	txw.On("WriteTransactionAndOps", mock.Anything, core.TransactionTypeContractDeploy, core.IdempotencyKey("idem1"), mock.MatchedBy(func(op *core.Operation) bool {
		return op.Namespace == "ns1" && op.Type == core.OpTypeBlockchainContractDeploy && op.Plugin == "mockblockchain"
	})).Return(nil, &sqlcommon.IdempotencyError{
		ExistingTXID:  id,
		OriginalError: i18n.NewError(context.Background(), coremsgs.MsgIdempotencyKeyDuplicateTransaction, "idem1", id)})
	mom.On("ResubmitOperations", context.Background(), id).Return(1, []*core.Operation{{}}, nil)
	mim.On("ResolveInputSigningKey", mock.Anything, signingKey, identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)

	// If ResubmitOperations returns an operation it's because it found one to resubmit, so we return 2xx not 409, and don't expect an error
	_, err := cm.DeployContract(context.Background(), req, false)
	assert.NoError(t, err)

	mth.AssertExpectations(t)
	mim.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestDeployContractIdempotentNoOperationToResubmit(t *testing.T) {
	cm := newTestContractManager()
	var id = fftypes.NewUUID()
	mim := cm.identity.(*identitymanagermocks.Manager)
	mth := cm.txHelper.(*txcommonmocks.Helper)
	mom := cm.operations.(*operationmocks.Manager)
	txw := cm.txWriter.(*txwritermocks.Writer)
	signingKey := "0x2468"
	req := &core.ContractDeployRequest{
		Key:            signingKey,
		Definition:     fftypes.JSONAnyPtr("[]"),
		Contract:       fftypes.JSONAnyPtr("\"0x123456\""),
		Input:          []interface{}{"one", "two", "three"},
		IdempotencyKey: "idem1",
	}

	txw.On("WriteTransactionAndOps", mock.Anything, core.TransactionTypeContractDeploy, core.IdempotencyKey("idem1"), mock.MatchedBy(func(op *core.Operation) bool {
		return op.Namespace == "ns1" && op.Type == core.OpTypeBlockchainContractDeploy && op.Plugin == "mockblockchain"
	})).Return(nil, &sqlcommon.IdempotencyError{
		ExistingTXID:  id,
		OriginalError: i18n.NewError(context.Background(), coremsgs.MsgIdempotencyKeyDuplicateTransaction, "idem1", id)})
	mom.On("ResubmitOperations", context.Background(), id).Return(1 /* total */, nil /* to resubmit */, nil)
	mim.On("ResolveInputSigningKey", mock.Anything, signingKey, identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)

	// If ResubmitOperations returns nil it's because there was no operation in initialized state, so we expect the regular 409 error back
	_, err := cm.DeployContract(context.Background(), req, false)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "FF10431")

	mth.AssertExpectations(t)
	mim.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestDeployContractIdempotentErrorOnOperationResubmit(t *testing.T) {
	cm := newTestContractManager()
	var id = fftypes.NewUUID()
	mim := cm.identity.(*identitymanagermocks.Manager)
	mth := cm.txHelper.(*txcommonmocks.Helper)
	mom := cm.operations.(*operationmocks.Manager)
	txw := cm.txWriter.(*txwritermocks.Writer)
	signingKey := "0x2468"
	req := &core.ContractDeployRequest{
		Key:            signingKey,
		Definition:     fftypes.JSONAnyPtr("[]"),
		Contract:       fftypes.JSONAnyPtr("\"0x123456\""),
		Input:          []interface{}{"one", "two", "three"},
		IdempotencyKey: "idem1",
	}

	txw.On("WriteTransactionAndOps", mock.Anything, core.TransactionTypeContractDeploy, core.IdempotencyKey("idem1"), mock.MatchedBy(func(op *core.Operation) bool {
		return op.Namespace == "ns1" && op.Type == core.OpTypeBlockchainContractDeploy && op.Plugin == "mockblockchain"
	})).Return(nil, &sqlcommon.IdempotencyError{
		ExistingTXID:  id,
		OriginalError: i18n.NewError(context.Background(), coremsgs.MsgIdempotencyKeyDuplicateTransaction, "idem1", id)})
	mom.On("ResubmitOperations", context.Background(), id).Return(-1, nil, fmt.Errorf("pop"))
	mim.On("ResolveInputSigningKey", mock.Anything, signingKey, identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)

	_, err := cm.DeployContract(context.Background(), req, false)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "pop")

	mth.AssertExpectations(t)
	mim.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestDeployContractSync(t *testing.T) {
	cm := newTestContractManager()
	mim := cm.identity.(*identitymanagermocks.Manager)
	mdi := cm.database.(*databasemocks.Plugin)
	mth := cm.txHelper.(*txcommonmocks.Helper)
	mom := cm.operations.(*operationmocks.Manager)
	sam := cm.syncasync.(*syncasyncmocks.Bridge)
	txw := cm.txWriter.(*txwritermocks.Writer)
	signingKey := "0x2468"
	req := &core.ContractDeployRequest{
		Key:            signingKey,
		Definition:     fftypes.JSONAnyPtr("[]"),
		Contract:       fftypes.JSONAnyPtr("\"0x123456\""),
		Input:          []interface{}{"one", "two", "three"},
		IdempotencyKey: "idem1",
	}

	txw.On("WriteTransactionAndOps", mock.Anything, core.TransactionTypeContractDeploy, core.IdempotencyKey("idem1"), mock.MatchedBy(func(op *core.Operation) bool {
		return op.Namespace == "ns1" && op.Type == core.OpTypeBlockchainContractDeploy && op.Plugin == "mockblockchain"
	})).Return(&core.Transaction{ID: fftypes.NewUUID()}, nil)
	mim.On("ResolveInputSigningKey", mock.Anything, signingKey, identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)

	sam.On("WaitForDeployOperation", mock.Anything, mock.Anything, mock.Anything).Return(&core.Operation{Status: core.OpStatusSucceeded}, nil)

	_, err := cm.DeployContract(context.Background(), req, true)

	assert.NoError(t, err)

	mth.AssertExpectations(t)
	mim.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestDeployContractResolveInputSigningKeyFail(t *testing.T) {
	cm := newTestContractManager()
	mim := cm.identity.(*identitymanagermocks.Manager)
	mdi := cm.database.(*databasemocks.Plugin)
	mth := cm.txHelper.(*txcommonmocks.Helper)
	mom := cm.operations.(*operationmocks.Manager)
	signingKey := "0x2468"
	req := &core.ContractDeployRequest{
		Key:        signingKey,
		Definition: fftypes.JSONAnyPtr("[]"),
		Contract:   fftypes.JSONAnyPtr("\"0x123456\""),
		Input:      []interface{}{"one", "two", "three"},
	}

	mim.On("ResolveInputSigningKey", mock.Anything, signingKey, identity.KeyNormalizationBlockchainPlugin).Return("", errors.New("pop"))
	_, err := cm.DeployContract(context.Background(), req, false)

	assert.Regexp(t, "pop", err)

	mth.AssertExpectations(t)
	mim.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestDeployContractSubmitNewTransactionFail(t *testing.T) {
	cm := newTestContractManager()
	mim := cm.identity.(*identitymanagermocks.Manager)
	mdi := cm.database.(*databasemocks.Plugin)
	mth := cm.txHelper.(*txcommonmocks.Helper)
	mom := cm.operations.(*operationmocks.Manager)
	txw := cm.txWriter.(*txwritermocks.Writer)
	signingKey := "0x2468"
	req := &core.ContractDeployRequest{
		Key:            signingKey,
		Definition:     fftypes.JSONAnyPtr("[]"),
		Contract:       fftypes.JSONAnyPtr("\"0x123456\""),
		Input:          []interface{}{"one", "two", "three"},
		IdempotencyKey: "idem1",
	}

	txw.On("WriteTransactionAndOps", mock.Anything, core.TransactionTypeContractDeploy, core.IdempotencyKey("idem1"), mock.MatchedBy(func(op *core.Operation) bool {
		return op.Namespace == "ns1" && op.Type == core.OpTypeBlockchainContractDeploy && op.Plugin == "mockblockchain"
	})).Return(nil, errors.New("pop"))
	mim.On("ResolveInputSigningKey", mock.Anything, signingKey, identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)

	_, err := cm.DeployContract(context.Background(), req, false)

	assert.Regexp(t, "pop", err)

	mth.AssertExpectations(t)
	mim.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestInvokeContract(t *testing.T) {
	cm := newTestContractManager()
	mim := cm.identity.(*identitymanagermocks.Manager)
	mdi := cm.database.(*databasemocks.Plugin)
	mth := cm.txHelper.(*txcommonmocks.Helper)
	mom := cm.operations.(*operationmocks.Manager)
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	txw := cm.txWriter.(*txwritermocks.Writer)

	req := &core.ContractCallRequest{
		Type:      core.CallTypeInvoke,
		Interface: fftypes.NewUUID(),
		Location:  fftypes.JSONAnyPtr(""),
		Method: &fftypes.FFIMethod{
			Name:    "doStuff",
			ID:      fftypes.NewUUID(),
			Params:  fftypes.FFIParams{},
			Returns: fftypes.FFIParams{},
		},
		IdempotencyKey: "idem1",
	}

	txw.On("WriteTransactionAndOps", mock.Anything, core.TransactionTypeContractInvoke, core.IdempotencyKey("idem1"), mock.MatchedBy(func(op *core.Operation) bool {
		return op.Namespace == "ns1" && op.Type == core.OpTypeBlockchainInvoke && op.Plugin == "mockblockchain"
	})).Return(&core.Transaction{ID: fftypes.NewUUID()}, nil)
	mim.On("ResolveInputSigningKey", mock.Anything, "", identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)
	mom.On("RunOperation", mock.Anything, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(txcommon.BlockchainInvokeData)
		return op.Type == core.OpTypeBlockchainInvoke && data.Request == req
	}), true).Return(nil, nil)
	opaqueData := "anything"
	mbi.On("ParseInterface", context.Background(), req.Method, req.Errors).Return(opaqueData, nil)
	mbi.On("ValidateInvokeRequest", mock.Anything, opaqueData, req.Input, false).Return(nil)

	_, err := cm.InvokeContract(context.Background(), req, false)

	assert.NoError(t, err)

	mth.AssertExpectations(t)
	mim.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mom.AssertExpectations(t)
	mbi.AssertExpectations(t)
}

func TestInvokeContractViaFFI(t *testing.T) {
	cm := newTestContractManager()
	mim := cm.identity.(*identitymanagermocks.Manager)
	mdi := cm.database.(*databasemocks.Plugin)
	mth := cm.txHelper.(*txcommonmocks.Helper)
	mom := cm.operations.(*operationmocks.Manager)
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	txw := cm.txWriter.(*txwritermocks.Writer)

	req := &core.ContractCallRequest{
		Type:           core.CallTypeInvoke,
		Interface:      fftypes.NewUUID(),
		Location:       fftypes.JSONAnyPtr(""),
		MethodPath:     "doStuff",
		IdempotencyKey: "idem1",
	}

	method := &fftypes.FFIMethod{
		Name:    "doStuff",
		ID:      fftypes.NewUUID(),
		Params:  fftypes.FFIParams{},
		Returns: fftypes.FFIParams{},
	}
	errors := []*fftypes.FFIError{}
	txw.On("WriteTransactionAndOps", mock.Anything, core.TransactionTypeContractInvoke, core.IdempotencyKey("idem1"), mock.MatchedBy(func(op *core.Operation) bool {
		return op.Namespace == "ns1" && op.Type == core.OpTypeBlockchainInvoke && op.Plugin == "mockblockchain"
	})).Return(&core.Transaction{ID: fftypes.NewUUID()}, nil)
	mim.On("ResolveInputSigningKey", mock.Anything, "", identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)
	mom.On("RunOperation", mock.Anything, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(txcommon.BlockchainInvokeData)
		return op.Type == core.OpTypeBlockchainInvoke && data.Request == req
	}), true).Return(nil, nil)
	opaqueData := "anything"
	mbi.On("ParseInterface", context.Background(), method, errors).Return(opaqueData, nil)
	mbi.On("ValidateInvokeRequest", mock.Anything, opaqueData, req.Input, false).Return(nil)

	mdb := cm.database.(*databasemocks.Plugin)
	mdb.On("GetFFIMethod", mock.Anything, "ns1", req.Interface, req.MethodPath).Return(method, nil)
	mdb.On("GetFFIErrors", mock.Anything, "ns1", mock.Anything).Return(errors, nil, nil)

	_, err := cm.InvokeContract(context.Background(), req, false)

	assert.NoError(t, err)

	mth.AssertExpectations(t)
	mim.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mom.AssertExpectations(t)
	mbi.AssertExpectations(t)
}

func TestInvokeContractWithBroadcast(t *testing.T) {
	cm := newTestContractManager()
	mim := cm.identity.(*identitymanagermocks.Manager)
	mdi := cm.database.(*databasemocks.Plugin)
	mth := cm.txHelper.(*txcommonmocks.Helper)
	mom := cm.operations.(*operationmocks.Manager)
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mbm := cm.broadcast.(*broadcastmocks.Manager)
	txw := cm.txWriter.(*txwritermocks.Writer)
	sender := &syncasyncmocks.Sender{}

	req := &core.ContractCallRequest{
		Type:      core.CallTypeInvoke,
		Interface: fftypes.NewUUID(),
		Location:  fftypes.JSONAnyPtr(""),
		Method: &fftypes.FFIMethod{
			Name: "doStuff",
			ID:   fftypes.NewUUID(),
			Params: fftypes.FFIParams{
				{
					Name:   "data",
					Schema: fftypes.JSONAnyPtr(`{"type":"string"}`),
				},
			},
			Returns: fftypes.FFIParams{},
		},
		IdempotencyKey: "idem1",
		Message: &core.MessageInOut{
			InlineData: core.InlineData{
				&core.DataRefOrValue{Value: fftypes.JSONAnyPtr("\"test-message\"")},
			},
		},
	}

	txw.On("WriteTransactionAndOps", mock.Anything, core.TransactionTypeContractInvokePin, core.IdempotencyKey("idem1"), mock.MatchedBy(func(op *core.Operation) bool {
		return op.Namespace == "ns1" && op.Type == core.OpTypeBlockchainInvoke && op.Plugin == "mockblockchain"
	})).Return(&core.Transaction{ID: fftypes.NewUUID()}, nil)
	mim.On("ResolveInputSigningKey", mock.Anything, "", identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)
	opaqueData := "anything"
	mbi.On("ParseInterface", context.Background(), req.Method, req.Errors).Return(opaqueData, nil)
	mbi.On("ValidateInvokeRequest", mock.Anything, opaqueData, req.Input, true).Return(nil)
	mbm.On("NewBroadcast", req.Message).Return(sender, nil)
	sender.On("Prepare", mock.Anything).Return(nil)
	sender.On("Send", mock.Anything).Return(nil)

	_, err := cm.InvokeContract(context.Background(), req, false)

	assert.NoError(t, err)

	mth.AssertExpectations(t)
	mim.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mom.AssertExpectations(t)
	mbi.AssertExpectations(t)
	mbm.AssertExpectations(t)
	sender.AssertExpectations(t)
}

func TestInvokeContractWithBroadcastConfirm(t *testing.T) {
	cm := newTestContractManager()
	mim := cm.identity.(*identitymanagermocks.Manager)
	mdi := cm.database.(*databasemocks.Plugin)
	mth := cm.txHelper.(*txcommonmocks.Helper)
	mom := cm.operations.(*operationmocks.Manager)
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mbm := cm.broadcast.(*broadcastmocks.Manager)
	txw := cm.txWriter.(*txwritermocks.Writer)
	sender := &syncasyncmocks.Sender{}

	req := &core.ContractCallRequest{
		Type:      core.CallTypeInvoke,
		Interface: fftypes.NewUUID(),
		Location:  fftypes.JSONAnyPtr(""),
		Method: &fftypes.FFIMethod{
			Name: "doStuff",
			ID:   fftypes.NewUUID(),
			Params: fftypes.FFIParams{
				{
					Name:   "data",
					Schema: fftypes.JSONAnyPtr(`{"type":"string"}`),
				},
			},
			Returns: fftypes.FFIParams{},
		},
		IdempotencyKey: "idem1",
		Message: &core.MessageInOut{
			InlineData: core.InlineData{
				&core.DataRefOrValue{Value: fftypes.JSONAnyPtr("\"test-message\"")},
			},
		},
	}

	txw.On("WriteTransactionAndOps", mock.Anything, core.TransactionTypeContractInvokePin, core.IdempotencyKey("idem1"), mock.MatchedBy(func(op *core.Operation) bool {
		return op.Namespace == "ns1" && op.Type == core.OpTypeBlockchainInvoke && op.Plugin == "mockblockchain"
	})).Return(&core.Transaction{ID: fftypes.NewUUID()}, nil)
	mim.On("ResolveInputSigningKey", mock.Anything, "", identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)
	opaqueData := "anything"
	mbi.On("ParseInterface", context.Background(), req.Method, req.Errors).Return(opaqueData, nil)
	mbi.On("ValidateInvokeRequest", mock.Anything, opaqueData, req.Input, true).Return(nil)
	mbm.On("NewBroadcast", req.Message).Return(sender, nil)
	sender.On("Prepare", mock.Anything).Return(nil)
	sender.On("SendAndWait", mock.Anything).Return(nil)

	_, err := cm.InvokeContract(context.Background(), req, true)

	assert.NoError(t, err)

	mth.AssertExpectations(t)
	mim.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mom.AssertExpectations(t)
	mbi.AssertExpectations(t)
	mbm.AssertExpectations(t)
	sender.AssertExpectations(t)
}

func TestInvokeContractWithBroadcastPrepareFail(t *testing.T) {
	cm := newTestContractManager()
	mim := cm.identity.(*identitymanagermocks.Manager)
	mdi := cm.database.(*databasemocks.Plugin)
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mbm := cm.broadcast.(*broadcastmocks.Manager)
	sender := &syncasyncmocks.Sender{}

	req := &core.ContractCallRequest{
		Type:      core.CallTypeInvoke,
		Interface: fftypes.NewUUID(),
		Location:  fftypes.JSONAnyPtr(""),
		Method: &fftypes.FFIMethod{
			Name: "doStuff",
			ID:   fftypes.NewUUID(),
			Params: fftypes.FFIParams{
				{
					Name:   "data",
					Schema: fftypes.JSONAnyPtr(`{"type":"string"}`),
				},
			},
			Returns: fftypes.FFIParams{},
		},
		IdempotencyKey: "idem1",
		Message: &core.MessageInOut{
			InlineData: core.InlineData{
				&core.DataRefOrValue{Value: fftypes.JSONAnyPtr("\"test-message\"")},
			},
		},
	}

	mim.On("ResolveInputSigningKey", mock.Anything, "", identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)
	opaqueData := "anything"
	mbi.On("ParseInterface", context.Background(), req.Method, req.Errors).Return(opaqueData, nil)
	mbi.On("ValidateInvokeRequest", mock.Anything, opaqueData, req.Input, true).Return(nil)
	mbm.On("NewBroadcast", req.Message).Return(sender, nil)
	sender.On("Prepare", mock.Anything).Return(fmt.Errorf("pop"))

	_, err := cm.InvokeContract(context.Background(), req, true)

	assert.EqualError(t, err, "pop")

	mim.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mbi.AssertExpectations(t)
	mbm.AssertExpectations(t)
	sender.AssertExpectations(t)
}

func TestInvokeContractWithBroadcastBadRequest(t *testing.T) {
	cm := newTestContractManager()
	mim := cm.identity.(*identitymanagermocks.Manager)
	mbm := cm.broadcast.(*broadcastmocks.Manager)
	sender := &syncasyncmocks.Sender{}

	req := &core.ContractCallRequest{
		Type:      core.CallTypeInvoke,
		Interface: fftypes.NewUUID(),
		Location:  fftypes.JSONAnyPtr(""),
		Method: &fftypes.FFIMethod{
			Name: "doStuff",
			ID:   fftypes.NewUUID(),
			Params: fftypes.FFIParams{
				{
					Name:   "data",
					Schema: fftypes.JSONAnyPtr(`{"type":"string"}`),
				}},
			Returns: fftypes.FFIParams{},
		},
		IdempotencyKey: "idem1",
		Message: &core.MessageInOut{
			InlineData: core.InlineData{
				&core.DataRefOrValue{Value: fftypes.JSONAnyPtr("\"test-message\"")},
			},
		},
		Input: map[string]interface{}{
			"data": "should not be here",
		},
	}

	mim.On("ResolveInputSigningKey", mock.Anything, "", identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)
	mbm.On("NewBroadcast", req.Message).Return(sender, nil)

	_, err := cm.InvokeContract(context.Background(), req, false)

	assert.Regexp(t, "FF10445", err)

	mim.AssertExpectations(t)
	mbm.AssertExpectations(t)
}

func TestInvokeContractWithBroadcastBadMethod(t *testing.T) {
	cm := newTestContractManager()
	mim := cm.identity.(*identitymanagermocks.Manager)
	mbm := cm.broadcast.(*broadcastmocks.Manager)
	sender := &syncasyncmocks.Sender{}

	req := &core.ContractCallRequest{
		Type:      core.CallTypeInvoke,
		Interface: fftypes.NewUUID(),
		Location:  fftypes.JSONAnyPtr(""),
		Method: &fftypes.FFIMethod{
			Name:    "doStuff",
			ID:      fftypes.NewUUID(),
			Params:  fftypes.FFIParams{},
			Returns: fftypes.FFIParams{},
		},
		IdempotencyKey: "idem1",
		Message: &core.MessageInOut{
			InlineData: core.InlineData{
				&core.DataRefOrValue{Value: fftypes.JSONAnyPtr("\"test-message\"")},
			},
		},
	}

	mim.On("ResolveInputSigningKey", mock.Anything, "", identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)
	mbm.On("NewBroadcast", req.Message).Return(sender, nil)

	_, err := cm.InvokeContract(context.Background(), req, false)

	assert.Regexp(t, "FF10443", err)

	mim.AssertExpectations(t)
	mbm.AssertExpectations(t)
}

func TestInvokeContractWithPrivateMessageBadMethod(t *testing.T) {
	cm := newTestContractManager()
	mim := cm.identity.(*identitymanagermocks.Manager)
	mpm := cm.messaging.(*privatemessagingmocks.Manager)
	sender := &syncasyncmocks.Sender{}

	req := &core.ContractCallRequest{
		Type:      core.CallTypeInvoke,
		Interface: fftypes.NewUUID(),
		Location:  fftypes.JSONAnyPtr(""),
		Method: &fftypes.FFIMethod{
			Name:    "doStuff",
			ID:      fftypes.NewUUID(),
			Params:  fftypes.FFIParams{},
			Returns: fftypes.FFIParams{},
		},
		IdempotencyKey: "idem1",
		Message: &core.MessageInOut{
			Message: core.Message{
				Header: core.MessageHeader{
					Type: core.MessageTypePrivate,
				},
			},
			InlineData: core.InlineData{
				&core.DataRefOrValue{Value: fftypes.JSONAnyPtr("\"test-message\"")},
			},
		},
	}

	mim.On("ResolveInputSigningKey", mock.Anything, "", identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)
	mpm.On("NewMessage", req.Message).Return(sender, nil)

	_, err := cm.InvokeContract(context.Background(), req, false)

	assert.Regexp(t, "FF10443", err)

	mim.AssertExpectations(t)
	mpm.AssertExpectations(t)
}

func TestInvokeContractWithBroadcastUnsupported(t *testing.T) {
	cm := newTestContractManager()
	mim := cm.identity.(*identitymanagermocks.Manager)

	req := &core.ContractCallRequest{
		Type:      core.CallTypeInvoke,
		Interface: fftypes.NewUUID(),
		Location:  fftypes.JSONAnyPtr(""),
		Method: &fftypes.FFIMethod{
			Name:    "doStuff",
			ID:      fftypes.NewUUID(),
			Params:  fftypes.FFIParams{},
			Returns: fftypes.FFIParams{},
		},
		IdempotencyKey: "idem1",
		Message: &core.MessageInOut{
			InlineData: core.InlineData{
				&core.DataRefOrValue{Value: fftypes.JSONAnyPtr("\"test-message\"")},
			},
		},
	}

	mim.On("ResolveInputSigningKey", mock.Anything, "", identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)

	cm.broadcast = nil
	_, err := cm.InvokeContract(context.Background(), req, false)

	assert.Regexp(t, "FF10415", err)

	mim.AssertExpectations(t)
}

func TestInvokeContractWithPrivateMessageUnsupported(t *testing.T) {
	cm := newTestContractManager()
	mim := cm.identity.(*identitymanagermocks.Manager)

	req := &core.ContractCallRequest{
		Type:      core.CallTypeInvoke,
		Interface: fftypes.NewUUID(),
		Location:  fftypes.JSONAnyPtr(""),
		Method: &fftypes.FFIMethod{
			Name:    "doStuff",
			ID:      fftypes.NewUUID(),
			Params:  fftypes.FFIParams{},
			Returns: fftypes.FFIParams{},
		},
		IdempotencyKey: "idem1",
		Message: &core.MessageInOut{
			Message: core.Message{
				Header: core.MessageHeader{
					Type: core.MessageTypePrivate,
				},
			},
			InlineData: core.InlineData{
				&core.DataRefOrValue{Value: fftypes.JSONAnyPtr("\"test-message\"")},
			},
		},
	}

	mim.On("ResolveInputSigningKey", mock.Anything, "", identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)

	cm.messaging = nil
	_, err := cm.InvokeContract(context.Background(), req, false)

	assert.Regexp(t, "FF10415", err)

	mim.AssertExpectations(t)
}

func TestInvokeContractIdempotentResubmitOperation(t *testing.T) {
	cm := newTestContractManager()
	var id = fftypes.NewUUID()
	mim := cm.identity.(*identitymanagermocks.Manager)
	mth := cm.txHelper.(*txcommonmocks.Helper)
	mom := cm.operations.(*operationmocks.Manager)
	mbm := cm.blockchain.(*blockchainmocks.Plugin)
	mbrm := cm.broadcast.(*broadcastmocks.Manager)
	txw := cm.txWriter.(*txwritermocks.Writer)
	sender := &syncasyncmocks.Sender{}

	req := &core.ContractCallRequest{
		Type:      core.CallTypeInvoke,
		Interface: fftypes.NewUUID(),
		Location:  fftypes.JSONAnyPtr(""),
		Method: &fftypes.FFIMethod{
			Name: "doStuff",
			ID:   fftypes.NewUUID(),
			Params: fftypes.FFIParams{
				{
					Name:   "data",
					Schema: fftypes.JSONAnyPtr(`{"type":"string"}`),
				}},
			Returns: fftypes.FFIParams{},
		},
		Message:        &core.MessageInOut{},
		IdempotencyKey: "idem1",
	}

	mbrm.On("NewBroadcast", req.Message).Return(sender, nil)
	txw.On("WriteTransactionAndOps", mock.Anything, core.TransactionTypeContractInvokePin, core.IdempotencyKey("idem1"), mock.MatchedBy(func(op *core.Operation) bool {
		return op.Namespace == "ns1" && op.Type == core.OpTypeBlockchainInvoke && op.Plugin == "mockblockchain"
	})).Return(nil, &sqlcommon.IdempotencyError{
		ExistingTXID:  id,
		OriginalError: i18n.NewError(context.Background(), coremsgs.MsgIdempotencyKeyDuplicateTransaction, "idem1", id)})
	mom.On("ResubmitOperations", context.Background(), id).Return(1, []*core.Operation{{}}, nil)
	mim.On("ResolveInputSigningKey", mock.Anything, "", identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)
	opaqueData := "anything"
	mbm.On("ParseInterface", context.Background(), req.Method, req.Errors).Return(opaqueData, nil)
	sender.On("Prepare", mock.Anything).Return(nil) // we won't do send though
	mbm.On("ValidateInvokeRequest", context.Background(), opaqueData, req.Input, true).Return(nil)

	// If ResubmitOperations returns an operation it's because it found one to resubmit, so we return 2xx not 409, and don't expect an error
	_, err := cm.InvokeContract(context.Background(), req, false)
	assert.NoError(t, err)

	mth.AssertExpectations(t)
	mim.AssertExpectations(t)
	mom.AssertExpectations(t)
	mbm.AssertExpectations(t)
}

func TestInvokeContractIdempotentNoOperationToResubmit(t *testing.T) {
	cm := newTestContractManager()
	var id = fftypes.NewUUID()
	mim := cm.identity.(*identitymanagermocks.Manager)
	mth := cm.txHelper.(*txcommonmocks.Helper)
	mom := cm.operations.(*operationmocks.Manager)
	mbm := cm.blockchain.(*blockchainmocks.Plugin)
	txw := cm.txWriter.(*txwritermocks.Writer)

	req := &core.ContractCallRequest{
		Type:      core.CallTypeInvoke,
		Interface: fftypes.NewUUID(),
		Location:  fftypes.JSONAnyPtr(""),
		Method: &fftypes.FFIMethod{
			Name:    "doStuff",
			ID:      fftypes.NewUUID(),
			Params:  fftypes.FFIParams{},
			Returns: fftypes.FFIParams{},
		},
		IdempotencyKey: "idem1",
	}

	txw.On("WriteTransactionAndOps", mock.Anything, core.TransactionTypeContractInvoke, core.IdempotencyKey("idem1"), mock.MatchedBy(func(op *core.Operation) bool {
		return op.Namespace == "ns1" && op.Type == core.OpTypeBlockchainInvoke && op.Plugin == "mockblockchain"
	})).Return(nil, &sqlcommon.IdempotencyError{
		ExistingTXID:  id,
		OriginalError: i18n.NewError(context.Background(), coremsgs.MsgIdempotencyKeyDuplicateTransaction, "idem1", id)})
	mom.On("ResubmitOperations", context.Background(), id).Return(1 /* total */, nil /* to resubmit */, nil)
	mim.On("ResolveInputSigningKey", mock.Anything, "", identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)
	opaqueData := "anything"
	mbm.On("ParseInterface", context.Background(), req.Method, req.Errors).Return(opaqueData, nil)
	mbm.On("ValidateInvokeRequest", context.Background(), opaqueData, req.Input, false).Return(nil)

	// If ResubmitOperations returns nil it's because there was no operation in initialized state, so we expect the regular 409 error back
	_, err := cm.InvokeContract(context.Background(), req, false)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "FF10431")

	mth.AssertExpectations(t)
	mim.AssertExpectations(t)
	mom.AssertExpectations(t)
	mbm.AssertExpectations(t)
}

func TestInvokeContractIdempotentErrorOnOperationResubmit(t *testing.T) {
	cm := newTestContractManager()
	var id = fftypes.NewUUID()
	mim := cm.identity.(*identitymanagermocks.Manager)
	mth := cm.txHelper.(*txcommonmocks.Helper)
	mom := cm.operations.(*operationmocks.Manager)
	mbm := cm.blockchain.(*blockchainmocks.Plugin)
	txw := cm.txWriter.(*txwritermocks.Writer)

	req := &core.ContractCallRequest{
		Type:      core.CallTypeInvoke,
		Interface: fftypes.NewUUID(),
		Location:  fftypes.JSONAnyPtr(""),
		Method: &fftypes.FFIMethod{
			Name:    "doStuff",
			ID:      fftypes.NewUUID(),
			Params:  fftypes.FFIParams{},
			Returns: fftypes.FFIParams{},
		},
		IdempotencyKey: "idem1",
	}

	txw.On("WriteTransactionAndOps", mock.Anything, core.TransactionTypeContractInvoke, core.IdempotencyKey("idem1"), mock.MatchedBy(func(op *core.Operation) bool {
		return op.Namespace == "ns1" && op.Type == core.OpTypeBlockchainInvoke && op.Plugin == "mockblockchain"
	})).Return(nil, &sqlcommon.IdempotencyError{
		ExistingTXID:  id,
		OriginalError: i18n.NewError(context.Background(), coremsgs.MsgIdempotencyKeyDuplicateTransaction, "idem1", id)})
	mom.On("ResubmitOperations", context.Background(), id).Return(-1, nil, fmt.Errorf("pop"))
	mim.On("ResolveInputSigningKey", mock.Anything, "", identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)
	opaqueData := "anything"
	mbm.On("ParseInterface", context.Background(), req.Method, req.Errors).Return(opaqueData, nil)
	mbm.On("ValidateInvokeRequest", context.Background(), opaqueData, req.Input, false).Return(nil)

	// If ResubmitOperations returns an error trying to resubmit an operation we expect that back, not a 409 conflict
	_, err := cm.InvokeContract(context.Background(), req, false)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "pop")

	mth.AssertExpectations(t)
	mim.AssertExpectations(t)
	mom.AssertExpectations(t)
	mbm.AssertExpectations(t)
}

func TestInvokeContractConfirm(t *testing.T) {
	cm := newTestContractManager()
	mim := cm.identity.(*identitymanagermocks.Manager)
	mdi := cm.database.(*databasemocks.Plugin)
	mth := cm.txHelper.(*txcommonmocks.Helper)
	mom := cm.operations.(*operationmocks.Manager)
	msa := cm.syncasync.(*syncasyncmocks.Bridge)
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	txw := cm.txWriter.(*txwritermocks.Writer)

	req := &core.ContractCallRequest{
		Type:      core.CallTypeInvoke,
		Interface: fftypes.NewUUID(),
		Location:  fftypes.JSONAnyPtr(""),
		Method: &fftypes.FFIMethod{
			Name:    "doStuff",
			ID:      fftypes.NewUUID(),
			Params:  fftypes.FFIParams{},
			Returns: fftypes.FFIParams{},
		},
		IdempotencyKey: "idem1",
	}

	txw.On("WriteTransactionAndOps", mock.Anything, core.TransactionTypeContractInvoke, core.IdempotencyKey("idem1"), mock.MatchedBy(func(op *core.Operation) bool {
		return op.Namespace == "ns1" && op.Type == core.OpTypeBlockchainInvoke && op.Plugin == "mockblockchain"
	})).Return(&core.Transaction{ID: fftypes.NewUUID()}, nil)
	mim.On("ResolveInputSigningKey", mock.Anything, "", identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)
	mom.On("RunOperation", mock.Anything, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(txcommon.BlockchainInvokeData)
		return op.Type == core.OpTypeBlockchainInvoke && data.Request == req
	}), true).Return(nil, nil)
	msa.On("WaitForInvokeOperation", mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			send := args[2].(syncasync.SendFunction)
			send(context.Background())
		}).
		Return(&core.Operation{}, nil)
	opaqueData := "anything"
	mbi.On("ParseInterface", context.Background(), req.Method, req.Errors).Return(opaqueData, nil)
	mbi.On("ValidateInvokeRequest", mock.Anything, opaqueData, req.Input, false).Return(nil)

	_, err := cm.InvokeContract(context.Background(), req, true)

	assert.NoError(t, err)

	mth.AssertExpectations(t)
	mim.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mom.AssertExpectations(t)
	msa.AssertExpectations(t)
	mbi.AssertExpectations(t)
}

func TestInvokeContractFail(t *testing.T) {
	cm := newTestContractManager()
	mim := cm.identity.(*identitymanagermocks.Manager)
	mdi := cm.database.(*databasemocks.Plugin)
	mth := cm.txHelper.(*txcommonmocks.Helper)
	mom := cm.operations.(*operationmocks.Manager)
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	txw := cm.txWriter.(*txwritermocks.Writer)

	req := &core.ContractCallRequest{
		Type:      core.CallTypeInvoke,
		Interface: fftypes.NewUUID(),
		Location:  fftypes.JSONAnyPtr(""),
		Method: &fftypes.FFIMethod{
			Name:    "doStuff",
			ID:      fftypes.NewUUID(),
			Params:  fftypes.FFIParams{},
			Returns: fftypes.FFIParams{},
		},
		IdempotencyKey: "idem1",
	}

	txw.On("WriteTransactionAndOps", mock.Anything, core.TransactionTypeContractInvoke, core.IdempotencyKey("idem1"), mock.MatchedBy(func(op *core.Operation) bool {
		return op.Namespace == "ns1" && op.Type == core.OpTypeBlockchainInvoke && op.Plugin == "mockblockchain"
	})).Return(&core.Transaction{ID: fftypes.NewUUID()}, nil)
	mim.On("ResolveInputSigningKey", mock.Anything, "", identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)
	mom.On("RunOperation", mock.Anything, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(txcommon.BlockchainInvokeData)
		return op.Type == core.OpTypeBlockchainInvoke && data.Request == req
	}), true).Return(nil, fmt.Errorf("pop"))
	opaqueData := "anything"
	mbi.On("ParseInterface", context.Background(), req.Method, req.Errors).Return(opaqueData, nil)
	mbi.On("ValidateInvokeRequest", mock.Anything, opaqueData, req.Input, false).Return(nil)

	_, err := cm.InvokeContract(context.Background(), req, false)

	assert.EqualError(t, err, "pop")

	mim.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
	mbi.AssertExpectations(t)
}

func TestInvokeContractBadInput(t *testing.T) {
	cm := newTestContractManager()

	req := &core.ContractCallRequest{
		Type:      core.CallTypeInvoke,
		Interface: fftypes.NewUUID(),
		Location:  fftypes.JSONAnyPtr(""),
		Input: map[string]interface{}{
			"badness": map[bool]bool{false: true}, // cannot be serialized to JSON
		},
	}

	_, _, err := cm.writeInvokeTransaction(context.Background(), req)

	assert.Regexp(t, "json", err)
}

func TestDeployContractBadInput(t *testing.T) {
	cm := newTestContractManager()

	req := &core.ContractDeployRequest{
		Options: map[string]interface{}{
			"badness": map[bool]bool{false: true}, // cannot be serialized to JSON
		},
	}

	_, _, err := cm.writeDeployTransaction(context.Background(), req)

	assert.Regexp(t, "json", err)
}

func TestInvokeContractFailResolveInputSigningKey(t *testing.T) {
	cm := newTestContractManager()
	mim := cm.identity.(*identitymanagermocks.Manager)

	req := &core.ContractCallRequest{
		Type:      core.CallTypeInvoke,
		Interface: fftypes.NewUUID(),
		Location:  fftypes.JSONAnyPtr(""),
	}

	mim.On("ResolveInputSigningKey", mock.Anything, "", identity.KeyNormalizationBlockchainPlugin).Return("", fmt.Errorf("pop"))

	_, err := cm.InvokeContract(context.Background(), req, false)

	assert.Regexp(t, "pop", err)
}

func TestInvokeContractFailResolve(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mim := cm.identity.(*identitymanagermocks.Manager)

	req := &core.ContractCallRequest{
		Type:      core.CallTypeInvoke,
		Interface: fftypes.NewUUID(),
		Location:  fftypes.JSONAnyPtr(""),
	}

	mim.On("ResolveInputSigningKey", mock.Anything, "", identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)
	mbi.On("InvokeContract", mock.Anything, mock.AnythingOfType("*fftypes.UUID"), "key-resolved", req.Location, req.Method, req.Input, req.Errors).Return(nil)

	_, err := cm.InvokeContract(context.Background(), req, false)

	assert.Regexp(t, "FF10313", err)
}

func TestInvokeContractTXFail(t *testing.T) {
	cm := newTestContractManager()
	mim := cm.identity.(*identitymanagermocks.Manager)
	mth := cm.txHelper.(*txcommonmocks.Helper)
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	txw := cm.txWriter.(*txwritermocks.Writer)

	req := &core.ContractCallRequest{
		Type:      core.CallTypeInvoke,
		Interface: fftypes.NewUUID(),
		Location:  fftypes.JSONAnyPtr(""),
		Method: &fftypes.FFIMethod{
			Name:    "doStuff",
			ID:      fftypes.NewUUID(),
			Params:  fftypes.FFIParams{},
			Returns: fftypes.FFIParams{},
		},
		IdempotencyKey: "idem1",
	}

	mim.On("ResolveInputSigningKey", mock.Anything, "", identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)
	txw.On("WriteTransactionAndOps", mock.Anything, core.TransactionTypeContractInvoke, core.IdempotencyKey("idem1"), mock.MatchedBy(func(op *core.Operation) bool {
		return op.Namespace == "ns1" && op.Type == core.OpTypeBlockchainInvoke && op.Plugin == "mockblockchain"
	})).Return(nil, fmt.Errorf("pop"))
	opaqueData := "anything"
	mbi.On("ParseInterface", context.Background(), req.Method, req.Errors).Return(opaqueData, nil)
	mbi.On("ValidateInvokeRequest", mock.Anything, opaqueData, req.Input, false).Return(nil)

	_, err := cm.InvokeContract(context.Background(), req, false)

	assert.EqualError(t, err, "pop")

	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
	mbi.AssertExpectations(t)
}

func TestInvokeContractMethodNotFound(t *testing.T) {
	cm := newTestContractManager()
	mdb := cm.database.(*databasemocks.Plugin)
	mim := cm.identity.(*identitymanagermocks.Manager)

	req := &core.ContractCallRequest{
		Type:       core.CallTypeInvoke,
		Interface:  fftypes.NewUUID(),
		Location:   fftypes.JSONAnyPtr(""),
		MethodPath: "set",
	}

	mim.On("ResolveInputSigningKey", mock.Anything, "", identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)
	mdb.On("GetFFIMethod", mock.Anything, "ns1", req.Interface, req.MethodPath).Return(nil, fmt.Errorf("pop"))

	_, err := cm.InvokeContract(context.Background(), req, false)

	assert.Regexp(t, "FF10315", err)
}

func TestInvokeContractErrorsFail(t *testing.T) {
	cm := newTestContractManager()
	mdb := cm.database.(*databasemocks.Plugin)
	mim := cm.identity.(*identitymanagermocks.Manager)

	req := &core.ContractCallRequest{
		Type:       core.CallTypeInvoke,
		Interface:  fftypes.NewUUID(),
		Location:   fftypes.JSONAnyPtr(""),
		MethodPath: "set",
	}

	mim.On("ResolveInputSigningKey", mock.Anything, "", identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)
	mdb.On("GetFFIMethod", mock.Anything, "ns1", req.Interface, req.MethodPath).Return(&fftypes.FFIMethod{Name: "set"}, nil)
	mdb.On("GetFFIErrors", mock.Anything, "ns1", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	_, err := cm.InvokeContract(context.Background(), req, false)

	assert.Regexp(t, "FF10434", err)
}

func TestInvokeContractMethodBadInput(t *testing.T) {
	cm := newTestContractManager()
	mim := cm.identity.(*identitymanagermocks.Manager)

	req := &core.ContractCallRequest{
		Type:      core.CallTypeInvoke,
		Interface: fftypes.NewUUID(),
		Location:  fftypes.JSONAnyPtr(""),
		Method: &fftypes.FFIMethod{
			Name: "sum",
			Params: fftypes.FFIParams{
				{
					Name:   "x",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
				},
				{
					Name:   "y",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
				},
			},
			Returns: fftypes.FFIParams{

				{
					Name:   "sum",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
				},
			},
		},
	}
	mim.On("ResolveInputSigningKey", mock.Anything, "", identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)

	_, err := cm.InvokeContract(context.Background(), req, false)
	assert.Regexp(t, "FF10304", err)
}

func TestQueryContract(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mim := cm.identity.(*identitymanagermocks.Manager)

	req := &core.ContractCallRequest{
		Type:      core.CallTypeQuery,
		Interface: fftypes.NewUUID(),
		Location:  fftypes.JSONAnyPtr(""),
		Method: &fftypes.FFIMethod{
			Name:    "doStuff",
			ID:      fftypes.NewUUID(),
			Params:  fftypes.FFIParams{},
			Returns: fftypes.FFIParams{},
		},
		IdempotencyKey: "idem1",
		Key:            "key-unresolved",
	}

	mim.On("ResolveQuerySigningKey", mock.Anything, "key-unresolved", identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)
	opaqueData := "anything"
	mbi.On("ParseInterface", context.Background(), req.Method, req.Errors).Return(opaqueData, nil)
	mbi.On("ValidateInvokeRequest", mock.Anything, opaqueData, req.Input, false).Return(nil)
	mbi.On("QueryContract", mock.Anything, "key-resolved", req.Location, opaqueData, req.Input, req.Options).Return(struct{}{}, nil)

	_, err := cm.InvokeContract(context.Background(), req, false)

	assert.NoError(t, err)

	mbi.AssertExpectations(t)
	mim.AssertExpectations(t)
}

func TestCallContractInvalidType(t *testing.T) {
	cm := newTestContractManager()
	mim := cm.identity.(*identitymanagermocks.Manager)
	mom := cm.operations.(*operationmocks.Manager)
	mbi := cm.blockchain.(*blockchainmocks.Plugin)

	req := &core.ContractCallRequest{
		Interface: fftypes.NewUUID(),
		Location:  fftypes.JSONAnyPtr(""),
		Method: &fftypes.FFIMethod{
			Name:    "doStuff",
			ID:      fftypes.NewUUID(),
			Params:  fftypes.FFIParams{},
			Returns: fftypes.FFIParams{},
		},
		IdempotencyKey: "idem1",
	}

	mim.On("ResolveInputSigningKey", mock.Anything, "", identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)
	mom.On("AddOrReuseOperation", mock.Anything, mock.MatchedBy(func(op *core.Operation) bool {
		return op.Namespace == "ns1" && op.Type == core.OpTypeBlockchainInvoke && op.Plugin == "mockblockchain"
	})).Return(nil)
	opaqueData := "anything"
	mbi.On("ParseInterface", context.Background(), req.Method, req.Errors).Return(opaqueData, nil)
	mbi.On("ValidateInvokeRequest", mock.Anything, opaqueData, req.Input, false).Return(nil)

	assert.PanicsWithValue(t, "unknown call type: ", func() {
		cm.InvokeContract(context.Background(), req, false)
	})

	mim.AssertExpectations(t)
	mbi.AssertExpectations(t)
}

func TestGetContractListenerByNameOrID(t *testing.T) {
	cm := newTestContractManager()
	mdi := cm.database.(*databasemocks.Plugin)

	id := fftypes.NewUUID()
	mdi.On("GetContractListenerByID", context.Background(), "ns1", id).Return(&core.ContractListener{}, nil)

	_, err := cm.GetContractListenerByNameOrID(context.Background(), id.String())
	assert.NoError(t, err)
}

func TestGetContractListenerByNameOrIDWithStatus(t *testing.T) {
	cm := newTestContractManager()
	mdi := cm.database.(*databasemocks.Plugin)
	mbi := cm.blockchain.(*blockchainmocks.Plugin)

	id := fftypes.NewUUID()
	backendID := "testID"
	mdi.On("GetContractListenerByID", context.Background(), "ns1", id).Return(&core.ContractListener{Namespace: "ns1", BackendID: backendID}, nil)
	mbi.On("GetContractListenerStatus", context.Background(), "ns1", backendID, false).Return(true, fftypes.JSONAnyPtr(fftypes.JSONObject{}.String()), core.ContractListenerStatusSynced, nil)

	_, err := cm.GetContractListenerByNameOrIDWithStatus(context.Background(), id.String())
	assert.NoError(t, err)
}

func TestGetContractListenerByNameOrIDWithStatusListenerFail(t *testing.T) {
	cm := newTestContractManager()
	mdi := cm.database.(*databasemocks.Plugin)

	id := fftypes.NewUUID()
	mdi.On("GetContractListenerByID", context.Background(), "ns1", id).Return(nil, fmt.Errorf("pop"))

	_, err := cm.GetContractListenerByNameOrIDWithStatus(context.Background(), id.String())
	assert.EqualError(t, err, "pop")
}

func TestGetContractListenerByNameOrIDWithStatusPluginFail(t *testing.T) {
	cm := newTestContractManager()
	mdi := cm.database.(*databasemocks.Plugin)
	mbi := cm.blockchain.(*blockchainmocks.Plugin)

	id := fftypes.NewUUID()
	backendID := "testID"
	mdi.On("GetContractListenerByID", context.Background(), "ns1", id).Return(&core.ContractListener{Namespace: "ns1", BackendID: backendID}, nil)
	mbi.On("GetContractListenerStatus", context.Background(), "ns1", backendID, false).Return(false, nil, core.ContractListenerStatusUnknown, fmt.Errorf("pop"))

	listener, err := cm.GetContractListenerByNameOrIDWithStatus(context.Background(), id.String())

	testError := core.ListenerStatusError{
		StatusError: "pop",
	}

	assert.Equal(t, listener.Status, testError)
	assert.NoError(t, err)
}

func TestGetContractListenerByNameOrIDFail(t *testing.T) {
	cm := newTestContractManager()
	mdi := cm.database.(*databasemocks.Plugin)

	id := fftypes.NewUUID()
	mdi.On("GetContractListenerByID", context.Background(), "ns1", id).Return(nil, fmt.Errorf("pop"))

	_, err := cm.GetContractListenerByNameOrID(context.Background(), id.String())
	assert.EqualError(t, err, "pop")
}

func TestGetContractListenerByName(t *testing.T) {
	cm := newTestContractManager()
	mdi := cm.database.(*databasemocks.Plugin)

	mdi.On("GetContractListener", context.Background(), "ns1", "sub1").Return(&core.ContractListener{}, nil)

	_, err := cm.GetContractListenerByNameOrID(context.Background(), "sub1")
	assert.NoError(t, err)
}

func TestGetContractListenerBadName(t *testing.T) {
	cm := newTestContractManager()

	_, err := cm.GetContractListenerByNameOrID(context.Background(), "!bad")
	assert.Regexp(t, "FF00140", err)
}

func TestGetContractListenerByNameFail(t *testing.T) {
	cm := newTestContractManager()
	mdi := cm.database.(*databasemocks.Plugin)

	mdi.On("GetContractListener", context.Background(), "ns1", "sub1").Return(nil, fmt.Errorf("pop"))

	_, err := cm.GetContractListenerByNameOrID(context.Background(), "sub1")
	assert.EqualError(t, err, "pop")
}

func TestGetContractListenerNotFound(t *testing.T) {
	cm := newTestContractManager()
	mdi := cm.database.(*databasemocks.Plugin)

	mdi.On("GetContractListener", context.Background(), "ns1", "sub1").Return(nil, nil)

	_, err := cm.GetContractListenerByNameOrID(context.Background(), "sub1")
	assert.Regexp(t, "FF10109", err)
}

func TestGetContractListeners(t *testing.T) {
	cm := newTestContractManager()
	mdi := cm.database.(*databasemocks.Plugin)

	mdi.On("GetContractListeners", context.Background(), "ns1", mock.Anything).Return(nil, nil, nil)

	f := database.ContractListenerQueryFactory.NewFilter(context.Background())
	_, _, err := cm.GetContractListeners(context.Background(), f.And())
	assert.NoError(t, err)
}

func TestGetContractListenersFail(t *testing.T) {
	cm := newTestContractManager()
	mdi := cm.database.(*databasemocks.Plugin)

	mdi.On("GetContractListeners", context.Background(), "ns1", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	f := database.ContractListenerQueryFactory.NewFilter(context.Background())
	_, _, err := cm.GetContractListeners(context.Background(), f.And())
	assert.Error(t, err)
	assert.Regexp(t, "pop", err.Error())
}

func TestGetContractAPIListeners(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdi := cm.database.(*databasemocks.Plugin)

	interfaceID := fftypes.NewUUID()
	api := &core.ContractAPI{
		Interface: &fftypes.FFIReference{
			ID: interfaceID,
		},
		Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
			"address": "0x123",
		}.String()),
	}
	event := &fftypes.FFIEvent{
		FFIEventDefinition: fftypes.FFIEventDefinition{
			Name: "changed",
		},
	}

	mdi.On("GetContractAPIByName", context.Background(), "ns1", "simple").Return(api, nil)
	mdi.On("GetFFIByID", context.Background(), "ns1", interfaceID).Return(&fftypes.FFI{}, nil)
	mdi.On("GetFFIEvent", context.Background(), "ns1", interfaceID, "changed").Return(event, nil)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("changed", nil)
	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, mock.Anything).Return("0x123:changed", nil)
	mdi.On("GetContractListeners", context.Background(), "ns1", mock.Anything).Return(nil, nil, nil)

	f := database.ContractListenerQueryFactory.NewFilter(context.Background())
	_, _, err := cm.GetContractAPIListeners(context.Background(), "simple", "changed", f.And())
	assert.NoError(t, err)

	mbi.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestGetContractAPIListenersSignatureFail(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdi := cm.database.(*databasemocks.Plugin)

	interfaceID := fftypes.NewUUID()
	api := &core.ContractAPI{
		Interface: &fftypes.FFIReference{
			ID: interfaceID,
		},
		Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
			"address": "0x123",
		}.String()),
	}
	event := &fftypes.FFIEvent{
		FFIEventDefinition: fftypes.FFIEventDefinition{
			Name: "changed",
		},
	}

	mdi.On("GetContractAPIByName", context.Background(), "ns1", "simple").Return(api, nil)
	mdi.On("GetFFIByID", context.Background(), "ns1", interfaceID).Return(&fftypes.FFI{}, nil)
	mdi.On("GetFFIEvent", context.Background(), "ns1", interfaceID, "changed").Return(event, nil)
	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, mock.Anything).Return("0x123:changed", fmt.Errorf("pop"))
	mdi.On("GetContractListeners", context.Background(), "ns1", mock.Anything).Return(nil, nil, nil)

	f := database.ContractListenerQueryFactory.NewFilter(context.Background())
	_, _, err := cm.GetContractAPIListeners(context.Background(), "simple", "changed", f.And())
	assert.Error(t, err)
	assert.Regexp(t, "pop", err.Error())

	mbi.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestGetContractAPIListenersOldSignatureFail(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdi := cm.database.(*databasemocks.Plugin)

	interfaceID := fftypes.NewUUID()
	api := &core.ContractAPI{
		Interface: &fftypes.FFIReference{
			ID: interfaceID,
		},
		Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
			"address": "0x123",
		}.String()),
	}
	event := &fftypes.FFIEvent{
		FFIEventDefinition: fftypes.FFIEventDefinition{
			Name: "changed",
		},
	}

	mdi.On("GetContractAPIByName", context.Background(), "ns1", "simple").Return(api, nil)
	mdi.On("GetFFIByID", context.Background(), "ns1", interfaceID).Return(&fftypes.FFI{}, nil)
	mdi.On("GetFFIEvent", context.Background(), "ns1", interfaceID, "changed").Return(event, nil)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("changed", fmt.Errorf("pop"))
	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, mock.Anything).Return("0x123:changed", nil)
	mdi.On("GetContractListeners", context.Background(), "ns1", mock.Anything).Return(nil, nil, nil)

	f := database.ContractListenerQueryFactory.NewFilter(context.Background())
	_, _, err := cm.GetContractAPIListeners(context.Background(), "simple", "changed", f.And())
	assert.Error(t, err)
	assert.Regexp(t, "pop", err.Error())

	mbi.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestGetContractAPIListenersNotFound(t *testing.T) {
	cm := newTestContractManager()
	mdi := cm.database.(*databasemocks.Plugin)

	mdi.On("GetContractAPIByName", context.Background(), "ns1", "simple").Return(nil, nil)

	f := database.ContractListenerQueryFactory.NewFilter(context.Background())
	_, _, err := cm.GetContractAPIListeners(context.Background(), "simple", "changed", f.And())
	assert.Regexp(t, "FF10109", err)

	mdi.AssertExpectations(t)
}

func TestGetContractAPIListenersFail(t *testing.T) {
	cm := newTestContractManager()
	mdi := cm.database.(*databasemocks.Plugin)

	mdi.On("GetContractAPIByName", context.Background(), "ns1", "simple").Return(nil, fmt.Errorf("pop"))

	f := database.ContractListenerQueryFactory.NewFilter(context.Background())
	_, _, err := cm.GetContractAPIListeners(context.Background(), "simple", "changed", f.And())
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestGetContractAPIListenersEventNotFound(t *testing.T) {
	cm := newTestContractManager()
	mdi := cm.database.(*databasemocks.Plugin)

	interfaceID := fftypes.NewUUID()
	api := &core.ContractAPI{
		Interface: &fftypes.FFIReference{
			ID: interfaceID,
		},
		Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
			"address": "0x123",
		}.String()),
	}

	mdi.On("GetContractAPIByName", context.Background(), "ns1", "simple").Return(api, nil)
	mdi.On("GetFFIByID", context.Background(), "ns1", interfaceID).Return(&fftypes.FFI{}, nil)
	mdi.On("GetFFIEvent", context.Background(), "ns1", interfaceID, "changed").Return(nil, nil)

	f := database.ContractListenerQueryFactory.NewFilter(context.Background())
	_, _, err := cm.GetContractAPIListeners(context.Background(), "simple", "changed", f.And())
	assert.Regexp(t, "FF10370", err)

	mdi.AssertExpectations(t)
}

func TestDeleteContractListener(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdi := cm.database.(*databasemocks.Plugin)

	sub := &core.ContractListener{
		ID: fftypes.NewUUID(),
	}

	mdi.On("GetContractListener", context.Background(), "ns1", "sub1").Return(sub, nil)
	mbi.On("DeleteContractListener", context.Background(), sub, true).Return(nil)
	mdi.On("DeleteContractListenerByID", context.Background(), "ns1", sub.ID).Return(nil)

	err := cm.DeleteContractListenerByNameOrID(context.Background(), "sub1")
	assert.NoError(t, err)
}

func TestDeleteContractListenerBlockchainFail(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdi := cm.database.(*databasemocks.Plugin)

	sub := &core.ContractListener{
		ID: fftypes.NewUUID(),
	}

	mdi.On("GetContractListener", context.Background(), "ns1", "sub1").Return(sub, nil)
	mbi.On("DeleteContractListener", context.Background(), sub, true).Return(fmt.Errorf("pop"))
	mdi.On("DeleteContractListenerByID", context.Background(), "ns1", sub.ID).Return(nil)

	err := cm.DeleteContractListenerByNameOrID(context.Background(), "sub1")
	assert.EqualError(t, err, "pop")
}

func TestDeleteContractListenerNotFound(t *testing.T) {
	cm := newTestContractManager()
	mdi := cm.database.(*databasemocks.Plugin)

	mdi.On("GetContractListener", context.Background(), "ns1", "sub1").Return(nil, nil)

	err := cm.DeleteContractListenerByNameOrID(context.Background(), "sub1")
	assert.Regexp(t, "FF10109", err)
}

func TestInvokeContractAPI(t *testing.T) {
	cm := newTestContractManager()
	mdb := cm.database.(*databasemocks.Plugin)
	mim := cm.identity.(*identitymanagermocks.Manager)
	mdi := cm.database.(*databasemocks.Plugin)
	mth := cm.txHelper.(*txcommonmocks.Helper)
	mom := cm.operations.(*operationmocks.Manager)
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	txw := cm.txWriter.(*txwritermocks.Writer)

	req := &core.ContractCallRequest{
		Type:      core.CallTypeInvoke,
		Interface: fftypes.NewUUID(),
		Location:  fftypes.JSONAnyPtr(""),
		Method: &fftypes.FFIMethod{
			ID:   fftypes.NewUUID(),
			Name: "peel",
		},
		IdempotencyKey: "idem1",
	}

	api := &core.ContractAPI{
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
		Location: fftypes.JSONAnyPtr(""),
	}

	mim.On("ResolveInputSigningKey", mock.Anything, "", identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)
	mdb.On("GetContractAPIByName", mock.Anything, "ns1", "banana").Return(api, nil)
	txw.On("WriteTransactionAndOps", mock.Anything, core.TransactionTypeContractInvoke, core.IdempotencyKey("idem1"), mock.MatchedBy(func(op *core.Operation) bool {
		return op.Namespace == "ns1" && op.Type == core.OpTypeBlockchainInvoke && op.Plugin == "mockblockchain"
	})).Return(&core.Transaction{ID: fftypes.NewUUID()}, nil)
	mom.On("RunOperation", mock.Anything, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(txcommon.BlockchainInvokeData)
		return op.Type == core.OpTypeBlockchainInvoke && data.Request == req
	}), true).Return(nil, nil)
	opaqueData := "anything"
	mbi.On("ParseInterface", context.Background(), req.Method, req.Errors).Return(opaqueData, nil)
	mbi.On("ValidateInvokeRequest", mock.Anything, opaqueData, req.Input, false).Return(nil)

	_, err := cm.InvokeContractAPI(context.Background(), "banana", "peel", req, false)

	assert.NoError(t, err)

	mdb.AssertExpectations(t)
	mim.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
	mbi.AssertExpectations(t)
}

func TestInvokeContractAPIFailContractLookup(t *testing.T) {
	cm := newTestContractManager()
	mdb := cm.database.(*databasemocks.Plugin)
	mim := cm.identity.(*identitymanagermocks.Manager)
	req := &core.ContractCallRequest{
		Type:      core.CallTypeInvoke,
		Interface: fftypes.NewUUID(),
		Location:  fftypes.JSONAnyPtr(""),
		Method: &fftypes.FFIMethod{
			ID: fftypes.NewUUID(),
		},
	}

	mim.On("ResolveInputSigningKey", mock.Anything, "", identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)
	mdb.On("GetContractAPIByName", mock.Anything, "ns1", "banana").Return(nil, fmt.Errorf("pop"))

	_, err := cm.InvokeContractAPI(context.Background(), "banana", "peel", req, false)

	assert.Regexp(t, "pop", err)
}

func TestInvokeContractAPIContractNotFound(t *testing.T) {
	cm := newTestContractManager()
	mdb := cm.database.(*databasemocks.Plugin)
	mim := cm.identity.(*identitymanagermocks.Manager)
	req := &core.ContractCallRequest{
		Type:      core.CallTypeInvoke,
		Interface: fftypes.NewUUID(),
		Location:  fftypes.JSONAnyPtr(""),
		Method: &fftypes.FFIMethod{
			ID: fftypes.NewUUID(),
		},
	}

	mim.On("ResolveInputSigningKey", mock.Anything, "", identity.KeyNormalizationBlockchainPlugin).Return("key-resolved", nil)
	mdb.On("GetContractAPIByName", mock.Anything, "ns1", "banana").Return(nil, nil)

	_, err := cm.InvokeContractAPI(context.Background(), "banana", "peel", req, false)

	assert.Regexp(t, "FF10109", err)
}

func TestGetContractAPI(t *testing.T) {
	cm := newTestContractManager()
	mdb := cm.database.(*databasemocks.Plugin)

	api := &core.ContractAPI{
		Namespace: "ns1",
		Name:      "banana",
	}
	mdb.On("GetContractAPIByName", mock.Anything, "ns1", "banana").Return(api, nil)

	result, err := cm.GetContractAPI(context.Background(), "http://localhost/api/v1/namespaces/ns1", "banana")

	assert.NoError(t, err)
	assert.Equal(t, "http://localhost/api/v1/namespaces/ns1/apis/banana/api/swagger.json", result.URLs.OpenAPI)
	assert.Equal(t, "http://localhost/api/v1/namespaces/ns1/apis/banana/api", result.URLs.UI)
	assert.Equal(t, "http://localhost/api/v1/namespaces/ns1/apis/banana", result.URLs.API)
}

func TestGetContractAPIs(t *testing.T) {
	cm := newTestContractManager()
	mdb := cm.database.(*databasemocks.Plugin)

	apis := []*core.ContractAPI{
		{
			Namespace: "ns1",
			Name:      "banana",
		},
	}
	filter := database.ContractAPIQueryFactory.NewFilter(context.Background()).And()
	mdb.On("GetContractAPIs", mock.Anything, "ns1", filter).Return(apis, &ffapi.FilterResult{}, nil)

	results, _, err := cm.GetContractAPIs(context.Background(), "http://localhost/api/v1/namespaces/ns1", filter)

	assert.NoError(t, err)
	assert.Equal(t, 1, len(results))
	assert.Equal(t, "http://localhost/api/v1/namespaces/ns1/apis/banana/api/swagger.json", results[0].URLs.OpenAPI)
	assert.Equal(t, "http://localhost/api/v1/namespaces/ns1/apis/banana/api", results[0].URLs.UI)
	assert.Equal(t, "http://localhost/api/v1/namespaces/ns1/apis/banana", results[0].URLs.API)
}

func TestGetContractAPIInterface(t *testing.T) {
	cm := newTestContractManager()
	mdb := cm.database.(*databasemocks.Plugin)
	mbi := cm.blockchain.(*blockchainmocks.Plugin)

	interfaceID := fftypes.NewUUID()
	api := &core.ContractAPI{
		Namespace: "ns1",
		Name:      "banana",
		Interface: &fftypes.FFIReference{ID: interfaceID},
	}

	mdb.On("GetContractAPIByName", mock.Anything, "ns1", "banana").Return(api, nil)
	mdb.On("GetFFIByID", mock.Anything, "ns1", interfaceID).Return(&fftypes.FFI{}, nil)
	mdb.On("GetFFIMethods", mock.Anything, "ns1", mock.Anything).Return([]*fftypes.FFIMethod{
		{ID: fftypes.NewUUID(), Name: "method1"},
	}, nil, nil)
	mdb.On("GetFFIEvents", mock.Anything, "ns1", mock.Anything).Return([]*fftypes.FFIEvent{
		{ID: fftypes.NewUUID(), FFIEventDefinition: fftypes.FFIEventDefinition{Name: "event1"}},
	}, nil, nil)
	mbi.On("GenerateEventSignature", mock.Anything, mock.MatchedBy(func(ev *fftypes.FFIEventDefinition) bool {
		return ev.Name == "event1"
	})).Return("event1Sig", nil)
	mdb.On("GetFFIErrors", mock.Anything, "ns1", mock.Anything).Return([]*fftypes.FFIError{
		{ID: fftypes.NewUUID(), FFIErrorDefinition: fftypes.FFIErrorDefinition{Name: "customError1"}},
	}, nil, nil)
	mbi.On("GenerateErrorSignature", mock.Anything, mock.MatchedBy(func(ev *fftypes.FFIErrorDefinition) bool {
		return ev.Name == "customError1"
	})).Return("error1Sig")

	result, err := cm.GetContractAPIInterface(context.Background(), "banana")

	assert.NoError(t, err)
	assert.NotNil(t, result)

	mdb.AssertExpectations(t)
	mbi.AssertExpectations(t)
}

func TestGetContractAPIInterfaceFail(t *testing.T) {
	cm := newTestContractManager()
	mdb := cm.database.(*databasemocks.Plugin)

	mdb.On("GetContractAPIByName", mock.Anything, "ns1", "banana").Return(nil, fmt.Errorf("pop"))

	_, err := cm.GetContractAPIInterface(context.Background(), "banana")

	assert.EqualError(t, err, "pop")

	mdb.AssertExpectations(t)
}

func TestResolveContractAPI(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdb := cm.database.(*databasemocks.Plugin)

	api := &core.ContractAPI{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Location:  fftypes.JSONAnyPtr(""),
		Name:      "banana",
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
	}

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeCall, api.Location).Return(api.Location, nil)
	mdb.On("GetContractAPIByName", mock.Anything, api.Namespace, api.Name).Return(nil, nil)
	mdb.On("GetFFIByID", mock.Anything, "ns1", api.Interface.ID).Return(&fftypes.FFI{}, nil)

	err := cm.ResolveContractAPI(context.Background(), "http://localhost/api", api)
	assert.NoError(t, err)

	mbi.AssertExpectations(t)
	mdb.AssertExpectations(t)
}

func TestResolveContractAPIValidateFail(t *testing.T) {
	cm := newTestContractManager()

	api := &core.ContractAPI{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Name:      "BAD***BAD",
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
	}

	err := cm.ResolveContractAPI(context.Background(), "http://localhost/api", api)
	assert.Regexp(t, "FF00140", err)
}

func TestResolveContractAPIBadLocation(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)

	api := &core.ContractAPI{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Location:  fftypes.JSONAnyPtr(""),
		Name:      "banana",
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
	}

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeCall, api.Location).Return(api.Location, fmt.Errorf("pop"))

	err := cm.ResolveContractAPI(context.Background(), "http://localhost/api", api)
	assert.EqualError(t, err, "pop")

	mbi.AssertExpectations(t)
}

func TestResolveContractAPICannotChangeLocation(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdb := cm.database.(*databasemocks.Plugin)

	apiID := fftypes.NewUUID()
	existing := &core.ContractAPI{
		ID:        apiID,
		Namespace: "ns1",
		Location:  fftypes.JSONAnyPtr(`"old"`),
		Name:      "banana",
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
	}
	api := &core.ContractAPI{
		ID:        apiID,
		Namespace: "ns1",
		Location:  fftypes.JSONAnyPtr(`"new"`),
		Name:      "banana",
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
	}

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeCall, api.Location).Return(api.Location, nil)
	mdb.On("GetContractAPIByName", mock.Anything, api.Namespace, api.Name).Return(existing, nil)

	err := cm.ResolveContractAPI(context.Background(), "http://localhost/api", api)
	assert.Regexp(t, "FF10316", err)

	mbi.AssertExpectations(t)
	mdb.AssertExpectations(t)
}

func TestResolveContractAPIInterfaceName(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdb := cm.database.(*databasemocks.Plugin)

	api := &core.ContractAPI{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Location:  fftypes.JSONAnyPtr(""),
		Name:      "banana",
		Interface: &fftypes.FFIReference{
			Name:    "my-ffi",
			Version: "1",
		},
	}
	interfaceID := fftypes.NewUUID()

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeCall, api.Location).Return(api.Location, nil)
	mdb.On("GetContractAPIByName", mock.Anything, api.Namespace, api.Name).Return(nil, nil)
	mdb.On("GetFFI", mock.Anything, "ns1", "my-ffi", "1").Return(&fftypes.FFI{ID: interfaceID}, nil)

	err := cm.ResolveContractAPI(context.Background(), "http://localhost/api", api)
	assert.NoError(t, err)

	mbi.AssertExpectations(t)
	mdb.AssertExpectations(t)
}

func TestResolveContractAPINoInterface(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdb := cm.database.(*databasemocks.Plugin)

	api := &core.ContractAPI{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Location:  fftypes.JSONAnyPtr(""),
		Name:      "banana",
	}

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeCall, api.Location).Return(api.Location, nil)
	mdb.On("GetContractAPIByName", mock.Anything, api.Namespace, api.Name).Return(nil, nil)

	err := cm.ResolveContractAPI(context.Background(), "http://localhost/api", api)
	assert.Regexp(t, "FF10303", err)

	mbi.AssertExpectations(t)
	mdb.AssertExpectations(t)
}

func TestResolveContractAPIInterfaceIDFail(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdb := cm.database.(*databasemocks.Plugin)

	api := &core.ContractAPI{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Location:  fftypes.JSONAnyPtr(""),
		Name:      "banana",
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
	}

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeCall, api.Location).Return(api.Location, nil)
	mdb.On("GetContractAPIByName", mock.Anything, api.Namespace, api.Name).Return(nil, nil)
	mdb.On("GetFFIByID", mock.Anything, "ns1", api.Interface.ID).Return(nil, fmt.Errorf("pop"))

	err := cm.ResolveContractAPI(context.Background(), "http://localhost/api", api)
	assert.EqualError(t, err, "pop")

	mbi.AssertExpectations(t)
	mdb.AssertExpectations(t)
}

func TestResolveContractAPIInterfaceIDNotFound(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdb := cm.database.(*databasemocks.Plugin)

	api := &core.ContractAPI{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Location:  fftypes.JSONAnyPtr(""),
		Name:      "banana",
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
	}

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeCall, api.Location).Return(api.Location, nil)
	mdb.On("GetContractAPIByName", mock.Anything, api.Namespace, api.Name).Return(nil, nil)
	mdb.On("GetFFIByID", mock.Anything, "ns1", api.Interface.ID).Return(nil, nil)

	err := cm.ResolveContractAPI(context.Background(), "http://localhost/api", api)
	assert.Regexp(t, "FF10303.*"+api.Interface.ID.String(), err)

	mbi.AssertExpectations(t)
	mdb.AssertExpectations(t)
}

func TestResolveContractAPIInterfaceNameFail(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdb := cm.database.(*databasemocks.Plugin)

	api := &core.ContractAPI{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Location:  fftypes.JSONAnyPtr(""),
		Name:      "banana",
		Interface: &fftypes.FFIReference{
			Name:    "my-ffi",
			Version: "1",
		},
	}

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeCall, api.Location).Return(api.Location, nil)
	mdb.On("GetContractAPIByName", mock.Anything, api.Namespace, api.Name).Return(nil, nil)
	mdb.On("GetFFI", mock.Anything, "ns1", "my-ffi", "1").Return(nil, fmt.Errorf("pop"))

	err := cm.ResolveContractAPI(context.Background(), "http://localhost/api", api)
	assert.EqualError(t, err, "pop")

	mbi.AssertExpectations(t)
	mdb.AssertExpectations(t)
}

func TestResolveContractAPIInterfaceNameNotFound(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdb := cm.database.(*databasemocks.Plugin)

	api := &core.ContractAPI{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Location:  fftypes.JSONAnyPtr(""),
		Name:      "banana",
		Interface: &fftypes.FFIReference{
			Name:    "my-ffi",
			Version: "1",
		},
	}

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeCall, api.Location).Return(api.Location, nil)
	mdb.On("GetContractAPIByName", mock.Anything, api.Namespace, api.Name).Return(nil, nil)
	mdb.On("GetFFI", mock.Anything, "ns1", "my-ffi", "1").Return(nil, nil)

	err := cm.ResolveContractAPI(context.Background(), "http://localhost/api", api)
	assert.Regexp(t, "FF10303.*my-ffi", err)

	mbi.AssertExpectations(t)
	mdb.AssertExpectations(t)
}

func TestResolveContractAPIInterfaceNoVersion(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdb := cm.database.(*databasemocks.Plugin)

	api := &core.ContractAPI{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Location:  fftypes.JSONAnyPtr(""),
		Name:      "banana",
		Interface: &fftypes.FFIReference{
			Name: "my-ffi",
		},
	}

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeCall, api.Location).Return(api.Location, nil)
	mdb.On("GetContractAPIByName", mock.Anything, api.Namespace, api.Name).Return(nil, nil)

	err := cm.ResolveContractAPI(context.Background(), "http://localhost/api", api)
	assert.Regexp(t, "FF10303.*my-ffi", err)

	mbi.AssertExpectations(t)
	mdb.AssertExpectations(t)
}

func TestValidateFFIParamBadSchemaJSON(t *testing.T) {
	cm := newTestContractManager()
	param := &fftypes.FFIParam{
		Name:   "x",
		Schema: fftypes.JSONAnyPtr(`{"type": "integer"`),
	}
	_, _, err := cm.validateFFIParam(context.Background(), param)
	assert.Regexp(t, "unexpected EOF", err)
}

func TestAddJSONSchemaExtension(t *testing.T) {
	cm := &contractManager{
		database:          &databasemocks.Plugin{},
		identity:          &identitymanagermocks.Manager{},
		blockchain:        &blockchainmocks.Plugin{},
		ffiParamValidator: &MockFFIParamValidator{},
	}
	c := cm.newFFISchemaCompiler()
	assert.NotNil(t, c)
}

func TestGenerateFFI(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	gfi := mbi.On("GenerateFFI", mock.Anything, mock.Anything)
	gfi.Run(func(args mock.Arguments) {
		gf := args[1].(*fftypes.FFIGenerationRequest)
		gfi.Return(&fftypes.FFI{
			Name:    gf.Name,
			Version: gf.Version,
			Methods: []*fftypes.FFIMethod{
				{
					Name: "method1",
				},
				{
					Name: "method1",
				},
			},
		}, nil)
	})

	ffi, err := cm.GenerateFFI(context.Background(), &fftypes.FFIGenerationRequest{})
	assert.NoError(t, err)
	assert.NotNil(t, ffi)
	assert.Equal(t, "generated", ffi.Name)
	assert.Equal(t, "0.0.1", ffi.Version)
	assert.Equal(t, "method1", ffi.Methods[0].Name)
	assert.Equal(t, "method1", ffi.Methods[0].Pathname)
	assert.Equal(t, "method1", ffi.Methods[1].Name)
	assert.Equal(t, "method1_1", ffi.Methods[1].Pathname)
}

type MockFFIParamValidator struct{}

func (v MockFFIParamValidator) Compile(ctx jsonschema.CompilerContext, m map[string]interface{}) (jsonschema.ExtSchema, error) {
	return nil, nil
}

func (v *MockFFIParamValidator) GetMetaSchema() *jsonschema.Schema {
	return nil
}

func (v *MockFFIParamValidator) GetExtensionName() string {
	return "ffi"
}

func TestBuildInvokeMessageInvalidType(t *testing.T) {
	cm := newTestContractManager()
	_, err := cm.buildInvokeMessage(context.Background(), &core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				Type: core.MessageTypeDefinition,
			},
		},
	})
	assert.Regexp(t, "FF10287", err)
}

func TestDeleteFFI(t *testing.T) {
	cm := newTestContractManager()

	id := fftypes.NewUUID()

	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetFFIByID", context.Background(), "ns1", id).Return(&fftypes.FFI{}, nil)
	mdi.On("DeleteFFI", context.Background(), "ns1", id).Return(nil)

	err := cm.DeleteFFI(context.Background(), id)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestDeleteFFINotFound(t *testing.T) {
	cm := newTestContractManager()

	id := fftypes.NewUUID()

	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetFFIByID", context.Background(), "ns1", id).Return(nil, nil)

	err := cm.DeleteFFI(context.Background(), id)
	assert.Regexp(t, "FF10109", err)

	mdi.AssertExpectations(t)
}

func TestDeleteFFIFailGet(t *testing.T) {
	cm := newTestContractManager()

	id := fftypes.NewUUID()

	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetFFIByID", context.Background(), "ns1", id).Return(nil, fmt.Errorf("pop"))

	err := cm.DeleteFFI(context.Background(), id)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestDeleteFFIPublished(t *testing.T) {
	cm := newTestContractManager()

	id := fftypes.NewUUID()

	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetFFIByID", context.Background(), "ns1", id).Return(&fftypes.FFI{Published: true}, nil)

	err := cm.DeleteFFI(context.Background(), id)
	assert.Regexp(t, "FF10449", err)

	mdi.AssertExpectations(t)
}

func TestDeleteContractAPI(t *testing.T) {
	cm := newTestContractManager()

	id := fftypes.NewUUID()

	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetContractAPIByName", context.Background(), "ns1", "banana").Return(&core.ContractAPI{ID: id}, nil)
	mdi.On("DeleteContractAPI", context.Background(), "ns1", id).Return(nil)

	err := cm.DeleteContractAPI(context.Background(), "banana")
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestDeleteContractAPIFailGet(t *testing.T) {
	cm := newTestContractManager()

	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetContractAPIByName", context.Background(), "ns1", "banana").Return(nil, fmt.Errorf("pop"))

	err := cm.DeleteContractAPI(context.Background(), "banana")
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestDeleteContractAPINotFound(t *testing.T) {
	cm := newTestContractManager()

	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetContractAPIByName", context.Background(), "ns1", "banana").Return(nil, nil)

	err := cm.DeleteContractAPI(context.Background(), "banana")
	assert.Regexp(t, "FF10109", err)

	mdi.AssertExpectations(t)
}

func TestDeleteContractAPIPublished(t *testing.T) {
	cm := newTestContractManager()

	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetContractAPIByName", context.Background(), "ns1", "banana").Return(&core.ContractAPI{Published: true}, nil)

	err := cm.DeleteContractAPI(context.Background(), "banana")
	assert.Regexp(t, "FF10449", err)

	mdi.AssertExpectations(t)
}

func TestResolveInvokeContractRequestCache(t *testing.T) {
	cm := newTestContractManager()
	mim := cm.identity.(*identitymanagermocks.Manager)
	mdi := cm.database.(*databasemocks.Plugin)
	mth := cm.txHelper.(*txcommonmocks.Helper)
	mom := cm.operations.(*operationmocks.Manager)
	mbi := cm.blockchain.(*blockchainmocks.Plugin)

	method := &fftypes.FFIMethod{
		Name:    "doStuff",
		ID:      fftypes.NewUUID(),
		Params:  fftypes.FFIParams{},
		Returns: fftypes.FFIParams{},
	}
	errors := []*fftypes.FFIError{}

	mdb := cm.database.(*databasemocks.Plugin)
	mdb.On("GetFFIMethod", mock.Anything, "ns1", mock.Anything, mock.Anything).Return(method, nil).Once()
	mdb.On("GetFFIErrors", mock.Anything, "ns1", mock.Anything).Return(errors, nil, nil).Once()

	interfaceID := fftypes.NewUUID()
	err := cm.resolveInvokeContractRequest(context.Background(), &core.ContractCallRequest{
		Type:           core.CallTypeInvoke,
		Interface:      interfaceID,
		Location:       fftypes.JSONAnyPtr("location1"),
		MethodPath:     "doStuff",
		IdempotencyKey: "idem1",
	})
	assert.NoError(t, err)

	// Test with Once() that the second is cached
	err = cm.resolveInvokeContractRequest(context.Background(), &core.ContractCallRequest{
		Type:           core.CallTypeInvoke,
		Interface:      interfaceID,
		Location:       fftypes.JSONAnyPtr("location2"),
		MethodPath:     "doStuff",
		IdempotencyKey: "idem2",
	})
	assert.NoError(t, err)

	mth.AssertExpectations(t)
	mim.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mom.AssertExpectations(t)
	mbi.AssertExpectations(t)
}

func TestEnsureParamNamesIncludedInCacheKeys(t *testing.T) {
	method1 := `{
        "name": "myFunction",
        "params": [
            {
                "name": "param_name_1",
                "schema": {
                    "details": {
                        "internalType": "address",
                        "type": "address"
                    },
                    "type": "string"
                }
            }
        ],
        "returns": []
    }`
	var method1FFI *fftypes.FFIMethod
	err := json.Unmarshal([]byte(method1), &method1FFI)
	assert.NoError(t, err)

	method2 := `{
        "name": "myFunction",
        "params": [
            {
                "name": "param_name_2",
                "schema": {
                    "details": {
                        "internalType": "address",
                        "type": "address"
                    },
                    "type": "string"
                }
            }
        ],
        "returns": []
    }`
	var method2FFI *fftypes.FFIMethod
	err = json.Unmarshal([]byte(method2), &method2FFI)
	assert.NoError(t, err)

	cm := newTestContractManager()
	paramUniqueHash1, _, err := cm.validateFFIMethod(context.Background(), method1FFI)
	assert.NoError(t, err)
	paramUniqueHash2, _, err := cm.validateFFIMethod(context.Background(), method2FFI)
	assert.NoError(t, err)

	assert.NotEqual(t, hex.EncodeToString(paramUniqueHash1.Sum(nil)), hex.EncodeToString(paramUniqueHash2.Sum(nil)))

}

func TestGenerateContractDeprecatedEventSignature(t *testing.T) {
	cm := newTestContractManager()

	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("changed", nil)
	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, mock.Anything).Return("0x123:changed", nil)

	sub := &core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
				"address": "0x123",
			}.String()),
			Event: &core.FFISerializedEvent{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "changed",
					Params: fftypes.FFIParams{
						{
							Name:   "value",
							Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
						},
					},
				},
			},
			Options: &core.ContractListenerOptions{},
			Topic:   "test-topic",
		},
	}

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, sub.Location).Return(sub.Location, nil)

	output, err := cm.ConstructContractListenerSignature(context.Background(), sub)
	assert.NoError(t, err)
	assert.Equal(t, "0x123:changed", output.Signature)
}

func TestGenerateContractFiltersCheckDuplicatesError(t *testing.T) {
	cm := newTestContractManager()
	event := &fftypes.FFIEvent{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		FFIEventDefinition: fftypes.FFIEventDefinition{
			Name: "firstEvent",
			Params: fftypes.FFIParams{
				{
					Name:   "value",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
				},
			},
		},
	}

	location := fftypes.JSONAnyPtr(`{"address":"0x1fa04bd8ca1b9ce9f19794faf790961134518434"}`)

	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("firstEvent", nil)
	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, mock.Anything).Return("*:firstEvent", nil)
	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, location).Return(location, nil)
	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetFFIByID", context.Background(), "ns1", mock.Anything).Return(&fftypes.FFI{}, nil)
	mdi.On("GetFFIEvent", context.Background(), "ns1", mock.Anything, "firstEvent").Return(event, nil)

	sub := &core.ContractListenerInput{
		Filters: []*core.ListenerFilterInput{
			{
				ListenerFilter: core.ListenerFilter{
					Location: location,
					Interface: &fftypes.FFIReference{
						ID: fftypes.NewUUID(),
					},
				},
				EventPath: "firstEvent",
			},
			{
				ListenerFilter: core.ListenerFilter{
					Location: location,
					Interface: &fftypes.FFIReference{
						ID: fftypes.NewUUID(),
					},
				},
				EventPath: "firstEvent",
			},
		},
		ContractListener: core.ContractListener{
			Options: &core.ContractListenerOptions{},
			Topic:   "test-topic",
		},
	}

	_, err := cm.ConstructContractListenerSignature(context.Background(), sub)
	assert.Error(t, err)
	assert.Regexp(t, "FF10477", err)
}

func TestGenerateContractFiltersSignature(t *testing.T) {
	cm := newTestContractManager()
	event := &fftypes.FFIEvent{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		FFIEventDefinition: fftypes.FFIEventDefinition{
			Name: "firstEvent",
			Params: fftypes.FFIParams{
				{
					Name:   "value",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
				},
			},
		},
	}

	location1 := fftypes.JSONAnyPtr(`{"address":"0x1fa04bd8ca1b9ce9f19794faf790961134518434"}`)
	location2 := fftypes.JSONAnyPtr(`{"address":"0x1aa04bd8ca1b9ce9f19794faf790961134518445"}`)

	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("changed", nil)
	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, location1).Return(location1, nil).Once()
	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, location2).Return(location2, nil).Once()
	mbi.On("CheckOverlappingLocations", context.Background(), location1, location2).Return(false, nil).Once()
	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetFFIByID", context.Background(), "ns1", mock.Anything).Return(&fftypes.FFI{}, nil)
	mdi.On("GetFFIEvent", context.Background(), "ns1", mock.Anything, "firstEvent").Return(event, nil)

	sub := &core.ContractListenerInput{
		Filters: []*core.ListenerFilterInput{
			{
				ListenerFilter: core.ListenerFilter{
					Location: location1,
					Interface: &fftypes.FFIReference{
						ID: fftypes.NewUUID(),
					},
				},
				EventPath: "firstEvent",
			},
			{
				ListenerFilter: core.ListenerFilter{
					Location: location2,
					Interface: &fftypes.FFIReference{
						ID: fftypes.NewUUID(),
					},
				},
				EventPath: "firstEvent",
			},
		},
		ContractListener: core.ContractListener{
			Options: &core.ContractListenerOptions{},
			Topic:   "test-topic",
		},
	}

	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, location1).Return("0x1fa04bd8ca1b9ce9f19794faf790961134518434:firstEvent", nil).Once()
	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, location2).Return("0x1aa04bd8ca1b9ce9f19794faf790961134518445:firstEvent", nil).Once()
	output, err := cm.ConstructContractListenerSignature(context.Background(), sub)
	assert.NoError(t, err)
	assert.Equal(t, "0x1aa04bd8ca1b9ce9f19794faf790961134518445:firstEvent;0x1fa04bd8ca1b9ce9f19794faf790961134518434:firstEvent", output.Signature)
}

func TestGenerateContractFiltersSignatureOverlappingError(t *testing.T) {
	cm := newTestContractManager()
	event := &fftypes.FFIEvent{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		FFIEventDefinition: fftypes.FFIEventDefinition{
			Name: "firstEvent",
			Params: fftypes.FFIParams{
				{
					Name:   "value",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
				},
			},
		},
	}

	location1 := fftypes.JSONAnyPtr(`{"address":"0x1fa04bd8ca1b9ce9f19794faf790961134518434"}`)
	location2 := fftypes.JSONAnyPtr(`{"address":"0x1aa04bd8ca1b9ce9f19794faf790961134518445"}`)

	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("changed", nil)
	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, location1).Return(location1, nil).Once()
	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, location2).Return(location2, nil).Once()
	mbi.On("CheckOverlappingLocations", context.Background(), location1, location2).Return(false, fmt.Errorf("pop")).Once()
	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetFFIByID", context.Background(), "ns1", mock.Anything).Return(&fftypes.FFI{}, nil)
	mdi.On("GetFFIEvent", context.Background(), "ns1", mock.Anything, "firstEvent").Return(event, nil)

	sub := &core.ContractListenerInput{
		Filters: []*core.ListenerFilterInput{
			{
				ListenerFilter: core.ListenerFilter{
					Location: location1,
					Interface: &fftypes.FFIReference{
						ID: fftypes.NewUUID(),
					},
				},
				EventPath: "firstEvent",
			},
			{
				ListenerFilter: core.ListenerFilter{
					Location: location2,
					Interface: &fftypes.FFIReference{
						ID: fftypes.NewUUID(),
					},
				},
				EventPath: "firstEvent",
			},
		},
		ContractListener: core.ContractListener{
			Options: &core.ContractListenerOptions{},
			Topic:   "test-topic",
		},
	}

	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, location1).Return("0x1fa04bd8ca1b9ce9f19794faf790961134518434:firstEvent", nil).Once()
	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, location2).Return("0x1aa04bd8ca1b9ce9f19794faf790961134518445:firstEvent", nil).Once()
	_, err := cm.ConstructContractListenerSignature(context.Background(), sub)
	assert.Error(t, err)
	assert.Regexp(t, "pop", err.Error())
}

func TestGenerateContractFiltersSignatureOverlapping(t *testing.T) {
	cm := newTestContractManager()
	event := &fftypes.FFIEvent{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		FFIEventDefinition: fftypes.FFIEventDefinition{
			Name: "firstEvent",
			Params: fftypes.FFIParams{
				{
					Name:   "value",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
				},
			},
		},
	}

	location1 := fftypes.JSONAnyPtr(`{"address":"0x1fa04bd8ca1b9ce9f19794faf790961134518434"}`)
	location2 := fftypes.JSONAnyPtr(`{"address":"0x1aa04bd8ca1b9ce9f19794faf790961134518445"}`)

	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("changed", nil)
	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, location1).Return(location1, nil).Once()
	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, location2).Return(location2, nil).Once()
	mbi.On("CheckOverlappingLocations", context.Background(), location1, location2).Return(true, nil).Once()
	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetFFIByID", context.Background(), "ns1", mock.Anything).Return(&fftypes.FFI{}, nil)
	mdi.On("GetFFIEvent", context.Background(), "ns1", mock.Anything, "firstEvent").Return(event, nil)

	sub := &core.ContractListenerInput{
		Filters: []*core.ListenerFilterInput{
			{
				ListenerFilter: core.ListenerFilter{
					Location: location1,
					Interface: &fftypes.FFIReference{
						ID: fftypes.NewUUID(),
					},
				},
				EventPath: "firstEvent",
			},
			{
				ListenerFilter: core.ListenerFilter{
					Location: location2,
					Interface: &fftypes.FFIReference{
						ID: fftypes.NewUUID(),
					},
				},
				EventPath: "firstEvent",
			},
		},
		ContractListener: core.ContractListener{
			Options: &core.ContractListenerOptions{},
			Topic:   "test-topic",
		},
	}

	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, location1).Return("0x1fa04bd8ca1b9ce9f19794faf790961134518434:firstEvent", nil).Once()
	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, location2).Return("0x1aa04bd8ca1b9ce9f19794faf790961134518445:firstEvent", nil).Once()
	_, err := cm.ConstructContractListenerSignature(context.Background(), sub)
	assert.Error(t, err)
	assert.Regexp(t, "FF10477", err.Error())
}

func TestGenerateContractFiltersSignatureSorted(t *testing.T) {
	cm := newTestContractManager()
	event := &fftypes.FFIEvent{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		FFIEventDefinition: fftypes.FFIEventDefinition{
			Name: "firstEvent",
			Params: fftypes.FFIParams{
				{
					Name:   "value",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
				},
			},
		},
	}

	event2 := &fftypes.FFIEvent{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		FFIEventDefinition: fftypes.FFIEventDefinition{
			Name: "secondEvent",
			Params: fftypes.FFIParams{
				{
					Name:   "value",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
				},
			},
		},
	}

	location1 := fftypes.JSONAnyPtr(`{"address":"0x1fa04bd8ca1b9ce9f19794faf790961134518434"}`)
	location2 := fftypes.JSONAnyPtr(`{"address":"0x1aa04bd8ca1b9ce9f19794faf790961134518445"}`)

	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("firstEvent", nil).Once()
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("secondEvent", nil).Once()
	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, location1).Return(location1, nil).Once()
	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, location2).Return(location2, nil).Once()
	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetFFIByID", context.Background(), "ns1", mock.Anything).Return(&fftypes.FFI{}, nil)
	mdi.On("GetFFIEvent", context.Background(), "ns1", mock.Anything, "firstEvent").Return(event, nil)
	mdi.On("GetFFIEvent", context.Background(), "ns1", mock.Anything, "secondEvent").Return(event2, nil)

	sub := &core.ContractListenerInput{
		Filters: []*core.ListenerFilterInput{
			{
				ListenerFilter: core.ListenerFilter{
					Location: location1,
					Interface: &fftypes.FFIReference{
						ID: fftypes.NewUUID(),
					},
				},
				EventPath: "firstEvent",
			},
			{
				ListenerFilter: core.ListenerFilter{
					Location: location2,
					Interface: &fftypes.FFIReference{
						ID: fftypes.NewUUID(),
					},
				},
				EventPath: "secondEvent",
			},
		},
		ContractListener: core.ContractListener{
			Options: &core.ContractListenerOptions{},
			Topic:   "test-topic",
		},
	}

	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, location1).Return("0x1fa04bd8ca1b9ce9f19794faf790961134518434:firstEvent", nil).Once()
	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, location2).Return("0x1aa04bd8ca1b9ce9f19794faf790961134518445:secondEvent", nil).Once()
	output, err := cm.ConstructContractListenerSignature(context.Background(), sub)
	assert.NoError(t, err)
	assert.Equal(t, "0x1aa04bd8ca1b9ce9f19794faf790961134518445:secondEvent;0x1fa04bd8ca1b9ce9f19794faf790961134518434:firstEvent", output.Signature)
}

func TestGenerateContractFiltersSignatureDuplicateFail(t *testing.T) {
	cm := newTestContractManager()
	event := &fftypes.FFIEvent{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		FFIEventDefinition: fftypes.FFIEventDefinition{
			Name: "firstEvent",
			Params: fftypes.FFIParams{
				{
					Name:   "value",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
				},
			},
		},
	}

	location1 := fftypes.JSONAnyPtr(`{"address":"0x1fa04bd8ca1b9ce9f19794faf790961134518434"}`)
	location2 := fftypes.JSONAnyPtr(`{"address":"0x1fa04bd8ca1b9ce9f19794faf790961134518434"}`)

	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("changed", nil)
	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, location1).Return(location1, nil).Once()
	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, location2).Return(location2, nil).Once()
	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetFFIByID", context.Background(), "ns1", mock.Anything).Return(&fftypes.FFI{}, nil)
	mdi.On("GetFFIEvent", context.Background(), "ns1", mock.Anything, "firstEvent").Return(event, nil)

	sub := &core.ContractListenerInput{
		Filters: []*core.ListenerFilterInput{
			{
				ListenerFilter: core.ListenerFilter{
					Location: location1,
					Interface: &fftypes.FFIReference{
						ID: fftypes.NewUUID(),
					},
				},
				EventPath: "firstEvent",
			},
			{
				ListenerFilter: core.ListenerFilter{
					Location: location2,
					Interface: &fftypes.FFIReference{
						ID: fftypes.NewUUID(),
					},
				},
				EventPath: "firstEvent",
			},
		},
		ContractListener: core.ContractListener{
			Options: &core.ContractListenerOptions{},
			Topic:   "test-topic",
		},
	}

	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, mock.Anything).Return("0x1fa04bd8ca1b9ce9f19794faf790961134518434:firstEvent", nil)
	_, err := cm.ConstructContractListenerSignature(context.Background(), sub)
	assert.Error(t, err)
	assert.Regexp(t, "FF10477", err.Error())
}

func TestGenerateContractFiltersSignatureDuplicateGenericFail(t *testing.T) {
	cm := newTestContractManager()
	event := &fftypes.FFIEvent{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		FFIEventDefinition: fftypes.FFIEventDefinition{
			Name: "firstEvent",
			Params: fftypes.FFIParams{
				{
					Name:   "value",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
				},
			},
		},
	}

	location2 := fftypes.JSONAnyPtr(`{"address":"0x1fa04bd8ca1b9ce9f19794faf790961134518434"}`)

	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("changed", nil)
	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, location2).Return(location2, nil).Once()
	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetFFIByID", context.Background(), "ns1", mock.Anything).Return(&fftypes.FFI{}, nil)
	mdi.On("GetFFIEvent", context.Background(), "ns1", mock.Anything, "firstEvent").Return(event, nil)

	sub := &core.ContractListenerInput{
		Filters: []*core.ListenerFilterInput{
			{
				ListenerFilter: core.ListenerFilter{
					Interface: &fftypes.FFIReference{
						ID: fftypes.NewUUID(),
					},
				},
				EventPath: "firstEvent",
			},
			{
				ListenerFilter: core.ListenerFilter{
					Location: location2,
					Interface: &fftypes.FFIReference{
						ID: fftypes.NewUUID(),
					},
				},
				EventPath: "firstEvent",
			},
		},
		ContractListener: core.ContractListener{
			Options: &core.ContractListenerOptions{},
			Topic:   "test-topic",
		},
	}

	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, (*fftypes.JSONAny)(nil)).Return("*:firstEvent", nil).Once()
	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, location2).Return("0x1fa04bd8ca1b9ce9f19794faf790961134518434:firstEvent", nil).Once()
	_, err := cm.ConstructContractListenerSignature(context.Background(), sub)
	assert.Error(t, err)
	assert.Regexp(t, "FF10477", err.Error())
}

func TestGenerateContractFiltersSignatureNormalizeError(t *testing.T) {
	cm := newTestContractManager()
	event := &fftypes.FFIEvent{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		FFIEventDefinition: fftypes.FFIEventDefinition{
			Name: "firstEvent",
			Params: fftypes.FFIParams{
				{
					Name:   "value",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
				},
			},
		},
	}
	location1 := fftypes.JSONAnyPtr(`{"channel":"my-channel"}`)

	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("changed", nil)
	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, location1).Return(location1, fmt.Errorf("pop")).Once()
	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetFFIByID", context.Background(), "ns1", mock.Anything).Return(&fftypes.FFI{}, nil)
	mdi.On("GetFFIEvent", context.Background(), "ns1", mock.Anything, "firstEvent").Return(event, nil)

	sub := &core.ContractListenerInput{
		Filters: []*core.ListenerFilterInput{
			{
				ListenerFilter: core.ListenerFilter{
					Location: location1,
					Interface: &fftypes.FFIReference{
						ID: fftypes.NewUUID(),
					},
				},
				EventPath: "firstEvent",
			},
		},
		ContractListener: core.ContractListener{
			Options: &core.ContractListenerOptions{},
			Topic:   "test-topic",
		},
	}

	_, err := cm.ConstructContractListenerSignature(context.Background(), sub)
	assert.Error(t, err)
	assert.Regexp(t, "pop", err.Error())
}

func TestMigrateFilters(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything, mock.Anything).Return("changed", nil)

	listener := &core.ContractListener{
		Options: &core.ContractListenerOptions{},
		Topic:   "test-topic",
		Event: &core.FFISerializedEvent{
			FFIEventDefinition: fftypes.FFIEventDefinition{
				Name: "changed",
				Params: fftypes.FFIParams{
					{
						Name:   "value",
						Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
					},
				},
			},
		},
	}

	migrated, listener, err := cm.MigrateToFiltersIfNeeded(context.Background(), listener)
	assert.NoError(t, err)
	assert.True(t, migrated)
	assert.NotEmpty(t, listener.Filters)
}

func TestMigrateFiltersSignatureError(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything, mock.Anything).Return("changed", fmt.Errorf("pop"))

	listener := &core.ContractListener{
		Options: &core.ContractListenerOptions{},
		Topic:   "test-topic",
		Event: &core.FFISerializedEvent{
			FFIEventDefinition: fftypes.FFIEventDefinition{
				Name: "changed",
				Params: fftypes.FFIParams{
					{
						Name:   "value",
						Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
					},
				},
			},
		},
	}

	_, listener, err := cm.MigrateToFiltersIfNeeded(context.Background(), listener)
	assert.Error(t, err)
	assert.Regexp(t, "pop", err.Error())
}

func TestAddContractListenerFail(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdi := cm.database.(*databasemocks.Plugin)

	sub := &core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
				"address": "0x123",
			}.String()),
			Event: &core.FFISerializedEvent{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "changed",
					Params: fftypes.FFIParams{
						{
							Name:   "value",
							Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
						},
					},
				},
			},
			Options: &core.ContractListenerOptions{},
			Topic:   "test-topic",
		},
	}

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, sub.Location).Return(sub.Location, nil)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("changed", nil)
	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, sub.Location).Return("0x123:changed", nil)
	mdi.On("GetContractListeners", context.Background(), "ns1", mock.Anything).Return(nil, nil, fmt.Errorf("db error"))

	_, err := cm.AddContractListener(context.Background(), sub)
	assert.Error(t, err)
	assert.Regexp(t, "db error", err.Error())

	mbi.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestAddContractListenerFailDuplicate(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdi := cm.database.(*databasemocks.Plugin)

	sub := &core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
				"address": "0x123",
			}.String()),
			Event: &core.FFISerializedEvent{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "changed",
					Params: fftypes.FFIParams{
						{
							Name:   "value",
							Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
						},
					},
				},
			},
			Options: &core.ContractListenerOptions{},
			Topic:   "test-topic",
		},
	}

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, sub.Location).Return(sub.Location, nil)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("changed", nil)
	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, sub.Location).Return("0x123:changed", nil)
	mdi.On("GetContractListeners", context.Background(), "ns1", mock.Anything).Return([]*core.ContractListener{
		&sub.ContractListener,
	}, nil, nil)

	_, err := cm.AddContractListener(context.Background(), sub)
	assert.Error(t, err)
	assert.Regexp(t, "FF10383", err.Error())

	mbi.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestAddContractListenerFailDuplicateError(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdi := cm.database.(*databasemocks.Plugin)

	sub := &core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
				"address": "0x123",
			}.String()),
			Event: &core.FFISerializedEvent{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "changed",
					Params: fftypes.FFIParams{
						{
							Name:   "value",
							Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
						},
					},
				},
			},
			Options: &core.ContractListenerOptions{},
			Topic:   "test-topic",
		},
	}

	old := &core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
				"address": "0x123",
			}.String()),
			Event: &core.FFISerializedEvent{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "changed",
					Params: fftypes.FFIParams{
						{
							Name:   "value",
							Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
						},
					},
				},
			},
			Options:   &core.ContractListenerOptions{},
			Topic:     "test-topic",
			Signature: "changed",
		},
	}

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, sub.Location).Return(sub.Location, nil)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("changed", nil)
	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, sub.Location).Return("0x123:changed", nil)
	mdi.On("GetContractListeners", context.Background(), "ns1", mock.Anything).Return([]*core.ContractListener{}, nil, nil).Once()
	mdi.On("GetContractListeners", context.Background(), "ns1", mock.Anything).Return([]*core.ContractListener{
		&old.ContractListener,
	}, nil, fmt.Errorf("pop"))

	_, err := cm.AddContractListener(context.Background(), sub)
	assert.Error(t, err)
	assert.Regexp(t, "pop", err.Error())

	mbi.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestAddContractListenerFailDuplicatePrevious(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdi := cm.database.(*databasemocks.Plugin)

	sub := &core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
				"address": "0x123",
			}.String()),
			Event: &core.FFISerializedEvent{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "changed",
					Params: fftypes.FFIParams{
						{
							Name:   "value",
							Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
						},
					},
				},
			},
			Options: &core.ContractListenerOptions{},
			Topic:   "test-topic",
		},
	}

	old := &core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
				"address": "0x123",
			}.String()),
			Event: &core.FFISerializedEvent{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "changed",
					Params: fftypes.FFIParams{
						{
							Name:   "value",
							Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
						},
					},
				},
			},
			Options:   &core.ContractListenerOptions{},
			Topic:     "test-topic",
			Signature: "changed",
		},
	}

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, sub.Location).Return(sub.Location, nil)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("changed", nil)
	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, sub.Location).Return("0x123:changed", nil)
	mdi.On("GetContractListeners", context.Background(), "ns1", mock.Anything).Return([]*core.ContractListener{}, nil, nil).Once()
	mdi.On("GetContractListeners", context.Background(), "ns1", mock.Anything).Return([]*core.ContractListener{
		&old.ContractListener,
	}, nil, nil)

	_, err := cm.AddContractListener(context.Background(), sub)
	assert.Error(t, err)
	assert.Regexp(t, "FF10383", err.Error())

	mbi.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestParseContractListenerFiltersFailSignature(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdi := cm.database.(*databasemocks.Plugin)

	sub := &core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
				"address": "0x123",
			}.String()),
			Event: &core.FFISerializedEvent{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "changed",
					Params: fftypes.FFIParams{
						{
							Name:   "value",
							Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
						},
					},
				},
			},
			Options: &core.ContractListenerOptions{},
			Topic:   "test-topic",
		},
	}

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, sub.Location).Return(sub.Location, nil)
	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, sub.Location).Return("0x123:changed", fmt.Errorf("pop"))

	err := cm.parseContractListenerFilters(context.Background(), sub)
	assert.Error(t, err)
	assert.Regexp(t, "pop", err.Error())

	mbi.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestParseContractListenerFiltersFailEventSignature(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdi := cm.database.(*databasemocks.Plugin)

	sub := &core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
				"address": "0x123",
			}.String()),
			Event: &core.FFISerializedEvent{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "changed",
					Params: fftypes.FFIParams{
						{
							Name:   "value",
							Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
						},
					},
				},
			},
			Options: &core.ContractListenerOptions{},
			Topic:   "test-topic",
		},
	}

	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, sub.Location).Return(sub.Location, nil)
	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, sub.Location).Return("0x123:changed", nil)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("changed", fmt.Errorf("pop"))

	err := cm.parseContractListenerFilters(context.Background(), sub)
	assert.Error(t, err)
	assert.Regexp(t, "pop", err.Error())

	mbi.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestGetFFIEventsSignatureFail(t *testing.T) {
	cm := newTestContractManager()
	mdb := cm.database.(*databasemocks.Plugin)
	mbi := cm.blockchain.(*blockchainmocks.Plugin)

	mdb.On("GetFFIEvents", mock.Anything, "ns1", mock.Anything).Return([]*fftypes.FFIEvent{
		{ID: fftypes.NewUUID(), FFIEventDefinition: fftypes.FFIEventDefinition{Name: "event1"}},
	}, nil, nil)
	mbi.On("GenerateEventSignature", mock.Anything, mock.MatchedBy(func(ev *fftypes.FFIEventDefinition) bool {
		return ev.Name == "event1"
	})).Return("event1Sig", fmt.Errorf("pop"))

	_, err := cm.GetFFIEvents(context.Background(), fftypes.NewUUID())

	assert.Error(t, err)
	assert.Regexp(t, "pop", err.Error())

	mdb.AssertExpectations(t)
	mbi.AssertExpectations(t)
}

func TestParseContractListenerFiltersFailEventSignatureDuplicateCheck(t *testing.T) {
	cm := newTestContractManager()
	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mdi := cm.database.(*databasemocks.Plugin)

	sub := &core.ContractListenerInput{
		ContractListener: core.ContractListener{
			Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
				"address": "0x123",
			}.String()),
			Event: &core.FFISerializedEvent{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "changed",
					Params: fftypes.FFIParams{
						{
							Name:   "value",
							Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
						},
					},
				},
			},
			Options: &core.ContractListenerOptions{},
			Topic:   "test-topic",
		},
	}

	mdi.On("GetContractListeners", context.Background(), "ns1", mock.Anything).Return(nil, nil, nil)
	mbi.On("NormalizeContractLocation", context.Background(), blockchain.NormalizeListener, sub.Location).Return(sub.Location, nil)
	mbi.On("GenerateEventSignatureWithLocation", context.Background(), mock.Anything, sub.Location).Return("0x123:changed", nil)
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("changed", nil).Once()
	mbi.On("GenerateEventSignature", context.Background(), mock.Anything).Return("changed", fmt.Errorf("pop"))

	_, err := cm.verifyContractListener(context.Background(), sub)
	assert.Error(t, err)
	assert.Regexp(t, "pop", err.Error())

	mbi.AssertExpectations(t)
	mdi.AssertExpectations(t)
}
