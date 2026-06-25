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
	"fmt"
	"testing"

	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly/mocks/syncasyncmocks"
	"github.com/hyperledger/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestDefineFFIResolveFail(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	ffi := &fftypes.FFI{
		Name:      "ffi1",
		Version:   "1.0",
		Methods:   []*fftypes.FFIMethod{{}},
		Events:    []*fftypes.FFIEvent{{}},
		Errors:    []*fftypes.FFIError{{}},
		Published: true,
	}

	ds.mcm.On("ResolveFFI", context.Background(), ffi).Return(fmt.Errorf("pop"))

	err := ds.DefineFFI(context.Background(), ffi, false, nil)
	assert.EqualError(t, err, "pop")
}

func TestDefineFFIFail(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	ffi := &fftypes.FFI{
		Name:      "ffi1",
		Version:   "1.0",
		Published: true,
	}

	ds.mdi.On("GetFFIByNetworkName", context.Background(), "ns1", "ffi1", "1.0").Return(nil, nil)
	ds.mcm.On("ResolveFFI", context.Background(), ffi).Return(nil)
	ds.mim.On("GetRootOrg", context.Background()).Return(nil, fmt.Errorf("pop"))

	err := ds.DefineFFI(context.Background(), ffi, false, nil)
	assert.EqualError(t, err, "pop")
}

func TestDefineFFIFailInnerError(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	ffi := &fftypes.FFI{
		Name:      "ffi1",
		Version:   "1.0",
		Published: false,
	}

	ds.mcm.On("ResolveFFI", context.Background(), ffi).Return(nil)
	ds.mdi.On("InsertOrGetFFI", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("error2: [%w]", fmt.Errorf("pop")))
	err := ds.DefineFFI(context.Background(), ffi, false, nil)
	assert.Regexp(t, "pop", err)
}

func TestDefineFFIExists(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	ffi := &fftypes.FFI{
		Name:      "ffi1",
		Version:   "1.0",
		Published: true,
	}

	ds.mdi.On("GetFFIByNetworkName", context.Background(), "ns1", "ffi1", "1.0").Return(&fftypes.FFI{}, nil)
	ds.mcm.On("ResolveFFI", context.Background(), ffi).Return(nil)

	err := ds.DefineFFI(context.Background(), ffi, false, nil)
	assert.Regexp(t, "FF10448", err)
}

func TestDefineFFIQueryFail(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	ffi := &fftypes.FFI{
		Name:      "ffi1",
		Version:   "1.0",
		Published: true,
	}

	ds.mdi.On("GetFFIByNetworkName", context.Background(), "ns1", "ffi1", "1.0").Return(nil, fmt.Errorf("pop"))
	ds.mcm.On("ResolveFFI", context.Background(), ffi).Return(nil)

	err := ds.DefineFFI(context.Background(), ffi, false, nil)
	assert.EqualError(t, err, "pop")
}

func TestDefineFFIOk(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	ffi := &fftypes.FFI{
		Name:      "ffi1",
		Version:   "1.0",
		Published: true,
	}

	ds.mdi.On("GetFFIByNetworkName", context.Background(), "ns1", "ffi1", "1.0").Return(nil, nil)
	ds.mcm.On("ResolveFFI", context.Background(), ffi).Return(nil)
	ds.mim.On("GetRootOrg", context.Background()).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			DID: "firefly:org1",
		},
	}, nil)
	ds.mim.On("ResolveInputSigningIdentity", context.Background(), mock.Anything).Return(nil)

	mms := &syncasyncmocks.Sender{}
	ds.mbm.On("NewBroadcast", mock.Anything).Return(mms)
	mms.On("Send", context.Background()).Return(nil)

	err := ds.DefineFFI(context.Background(), ffi, false, nil)
	assert.NoError(t, err)

	mms.AssertExpectations(t)
}

func TestDefineFFIConfirm(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	ffi := &fftypes.FFI{
		Name:      "ffi1",
		Version:   "1.0",
		Published: true,
	}

	ds.mdi.On("GetFFIByNetworkName", context.Background(), "ns1", "ffi1", "1.0").Return(nil, nil)
	ds.mcm.On("ResolveFFI", context.Background(), ffi).Return(nil)
	ds.mim.On("GetRootOrg", context.Background()).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			DID: "firefly:org1",
		},
	}, nil)
	ds.mim.On("ResolveInputSigningIdentity", context.Background(), mock.Anything).Return(nil)

	mms := &syncasyncmocks.Sender{}
	ds.mbm.On("NewBroadcast", mock.Anything).Return(mms)
	mms.On("SendAndWait", context.Background()).Return(nil)

	err := ds.DefineFFI(context.Background(), ffi, true, nil)
	assert.NoError(t, err)

	mms.AssertExpectations(t)
}

func TestDefineFFIPublishNonMultiparty(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = false

	ffi := &fftypes.FFI{
		Name:      "ffi1",
		Version:   "1.0",
		Published: true,
	}

	err := ds.DefineFFI(context.Background(), ffi, false, nil)
	assert.Regexp(t, "FF10414", err)
}

func TestDefineFFINonMultiparty(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)

	ffi := &fftypes.FFI{
		Name:    "ffi1",
		Version: "1.0",
	}

	ds.mcm.On("ResolveFFI", context.Background(), ffi).Return(nil)
	ds.mdi.On("InsertOrGetFFI", context.Background(), ffi).Return(nil, nil)
	ds.mdi.On("InsertEvent", context.Background(), mock.Anything).Return(nil)

	err := ds.DefineFFI(context.Background(), ffi, false, nil)
	assert.NoError(t, err)
}

func TestDefineFFINonMultipartyFail(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)

	ffi := &fftypes.FFI{
		Name:    "ffi1",
		Version: "1.0",
	}

	ds.mcm.On("ResolveFFI", context.Background(), ffi).Return(fmt.Errorf("pop"))

	err := ds.DefineFFI(context.Background(), ffi, false, nil)
	assert.Regexp(t, "FF10403", err)
}

// test that we can call defineFFI with custom topics
func TestDefineFFICustomTopics(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	ffi := &fftypes.FFI{
		Name:      "ffi1",
		Version:   "1.0",
		Published: true,
	}

	ds.mdi.On("GetFFIByNetworkName", context.Background(), "ns1", "ffi1", "1.0").Return(nil, nil)
	ds.mcm.On("ResolveFFI", context.Background(), ffi).Return(nil)
	ds.mim.On("GetRootOrg", context.Background()).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			DID: "firefly:org1",
		},
	}, nil)
	ds.mim.On("ResolveInputSigningIdentity", context.Background(), mock.Anything).Return(nil)

	mms := &syncasyncmocks.Sender{}
	ds.mbm.On("NewBroadcast", mock.Anything).Return(mms)
	mms.On("Send", context.Background()).Return(nil)

	err := ds.DefineFFI(context.Background(), ffi, false, []string{"my-custom-topic"})
	assert.NoError(t, err)

	mms.AssertExpectations(t)
}

func TestDefineContractAPIResolveFail(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	url := "http://firefly"
	api := &core.ContractAPI{
		Name:      "banana",
		Published: true,
	}

	ds.mcm.On("ResolveContractAPI", context.Background(), url, api).Return(fmt.Errorf("pop"))

	err := ds.DefineContractAPI(context.Background(), url, api, false, nil)
	assert.EqualError(t, err, "pop")
}

func TestDefineContractAPIFail(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	url := "http://firefly"
	api := &core.ContractAPI{
		Name:      "banana",
		Published: true,
	}

	ds.mcm.On("ResolveContractAPI", context.Background(), url, api).Return(nil)
	ds.mim.On("GetRootOrg", context.Background()).Return(nil, fmt.Errorf("pop"))
	ds.mdi.On("GetContractAPIByNetworkName", context.Background(), "ns1", "banana").Return(nil, nil)

	err := ds.DefineContractAPI(context.Background(), url, api, false, nil)
	assert.EqualError(t, err, "pop")
}

func TestDefineContractAPIOk(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	url := "http://firefly"
	api := &core.ContractAPI{
		Name:      "banana",
		Published: true,
	}

	ds.mcm.On("ResolveContractAPI", context.Background(), url, api).Return(nil)
	ds.mim.On("GetRootOrg", context.Background()).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			DID: "firefly:org1",
		},
	}, nil)
	ds.mim.On("ResolveInputSigningIdentity", context.Background(), mock.Anything).Return(nil)
	ds.mdi.On("GetContractAPIByNetworkName", context.Background(), "ns1", "banana").Return(nil, nil)

	mms := &syncasyncmocks.Sender{}
	ds.mbm.On("NewBroadcast", mock.Anything).Return(mms)
	mms.On("Send", context.Background()).Return(nil)

	err := ds.DefineContractAPI(context.Background(), url, api, false, nil)
	assert.NoError(t, err)

	mms.AssertExpectations(t)
}

func TestDefineContractAPINonMultiparty(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)

	url := "http://firefly"
	api := &core.ContractAPI{}

	ds.mcm.On("ResolveContractAPI", context.Background(), url, api).Return(nil)
	ds.mdi.On("InsertOrGetContractAPI", mock.Anything, mock.Anything).Return(nil, nil)
	ds.mdi.On("InsertEvent", mock.Anything, mock.Anything).Return(nil)

	err := ds.DefineContractAPI(context.Background(), url, api, false, nil)
	assert.NoError(t, err)
}

func TestDefineContractAPIPublishNonMultiparty(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = false

	url := "http://firefly"
	api := &core.ContractAPI{
		Name:      "banana",
		Published: true,
	}

	err := ds.DefineContractAPI(context.Background(), url, api, false, nil)
	assert.Regexp(t, "FF10414", err)
}

func TestDefineContractAPINonMultipartyUpdate(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = false
	testUUID := fftypes.NewUUID()

	url := "http://firefly"
	api := &core.ContractAPI{
		ID:        testUUID,
		Name:      "banana",
		Published: false,
	}
	ds.mcm.On("ResolveContractAPI", context.Background(), url, api).Return(nil)
	ds.mdi.On("InsertOrGetContractAPI", mock.Anything, mock.Anything).Return(api, nil)
	ds.mdi.On("UpsertContractAPI", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	ds.mdi.On("InsertEvent", mock.Anything, mock.Anything).Return(nil)

	err := ds.DefineContractAPI(context.Background(), url, api, false, nil)
	assert.NoError(t, err)
}

// test that we can call defineContractAPI with custom topics
func TestDefineContractAPICustomTopics(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	url := "http://firefly"
	api := &core.ContractAPI{
		Name:      "banana",
		Published: true,
	}

	ds.mcm.On("ResolveContractAPI", context.Background(), url, api).Return(nil)
	ds.mim.On("GetRootOrg", context.Background()).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			DID: "firefly:org1",
		},
	}, nil)
	ds.mim.On("ResolveInputSigningIdentity", context.Background(), mock.Anything).Return(nil)
	ds.mdi.On("GetContractAPIByNetworkName", context.Background(), "ns1", "banana").Return(nil, nil)

	mms := &syncasyncmocks.Sender{}
	ds.mbm.On("NewBroadcast", mock.Anything).Return(mms)
	mms.On("Send", context.Background()).Return(nil)

	err := ds.DefineContractAPI(context.Background(), url, api, false, []string{"my-custom-topic"})
	assert.NoError(t, err)

	mms.AssertExpectations(t)
}

func TestPublishFFI(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	mms := &syncasyncmocks.Sender{}

	ffi := &fftypes.FFI{
		Name:      "ffi1",
		Version:   "1.0",
		Namespace: "ns1",
		Published: false,
	}

	ds.mdi.On("GetFFIByNetworkName", context.Background(), "ns1", "ffi1-shared", "1.0").Return(nil, nil)
	ds.mcm.On("GetFFIWithChildren", context.Background(), "ffi1", "1.0").Return(ffi, nil)
	ds.mcm.On("ResolveFFI", context.Background(), ffi).Return(nil)
	ds.mim.On("GetRootOrg", context.Background()).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			DID: "firefly:org1",
		},
	}, nil)
	ds.mim.On("ResolveInputSigningIdentity", mock.Anything, mock.Anything).Return(nil)
	ds.mbm.On("NewBroadcast", mock.Anything).Return(mms)
	mms.On("Prepare", context.Background()).Return(nil)
	mms.On("Send", context.Background()).Return(nil)
	mockRunAsGroupPassthrough(ds.mdi)

	result, err := ds.PublishFFI(context.Background(), "ffi1", "1.0", "ffi1-shared", false, nil)
	assert.NoError(t, err)
	assert.Equal(t, ffi, result)
	assert.True(t, ffi.Published)

	mms.AssertExpectations(t)
}

func TestPublishFFIAlreadyPublished(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	ffi := &fftypes.FFI{
		Name:      "ffi1",
		Version:   "1.0",
		Namespace: "ns1",
		Published: true,
	}

	ds.mcm.On("GetFFIWithChildren", context.Background(), "ffi1", "1.0").Return(ffi, nil)
	mockRunAsGroupPassthrough(ds.mdi)

	_, err := ds.PublishFFI(context.Background(), "ffi1", "1.0", "ffi1-shared", false, nil)
	assert.Regexp(t, "FF10450", err)
}

func TestPublishFFIQueryFail(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	ds.mcm.On("GetFFIWithChildren", context.Background(), "ffi1", "1.0").Return(nil, fmt.Errorf("pop"))
	mockRunAsGroupPassthrough(ds.mdi)

	_, err := ds.PublishFFI(context.Background(), "ffi1", "1.0", "ffi1-shared", false, nil)
	assert.EqualError(t, err, "pop")
}

func TestPublishFFIResolveFail(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	ffi := &fftypes.FFI{
		Name:      "ffi1",
		Version:   "1.0",
		Namespace: "ns1",
		Published: false,
	}

	ds.mcm.On("GetFFIWithChildren", context.Background(), "ffi1", "1.0").Return(ffi, nil)
	ds.mcm.On("ResolveFFI", context.Background(), ffi).Return(fmt.Errorf("pop"))
	mockRunAsGroupPassthrough(ds.mdi)

	_, err := ds.PublishFFI(context.Background(), "ffi1", "1.0", "ffi1-shared", false, nil)
	assert.EqualError(t, err, "pop")
}

func TestPublishFFIPrepareFail(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	mms := &syncasyncmocks.Sender{}

	ffi := &fftypes.FFI{
		Name:      "ffi1",
		Version:   "1.0",
		Namespace: "ns1",
		Published: false,
	}

	ds.mdi.On("GetFFIByNetworkName", context.Background(), "ns1", "ffi1-shared", "1.0").Return(nil, nil)
	ds.mcm.On("GetFFIWithChildren", context.Background(), "ffi1", "1.0").Return(ffi, nil)
	ds.mcm.On("ResolveFFI", context.Background(), ffi).Return(nil)
	ds.mim.On("GetRootOrg", context.Background()).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			DID: "firefly:org1",
		},
	}, nil)
	ds.mim.On("ResolveInputSigningIdentity", mock.Anything, mock.Anything).Return(nil)
	ds.mbm.On("NewBroadcast", mock.Anything).Return(mms)
	mms.On("Prepare", context.Background()).Return(fmt.Errorf("pop"))
	mockRunAsGroupPassthrough(ds.mdi)

	_, err := ds.PublishFFI(context.Background(), "ffi1", "1.0", "ffi1-shared", false, nil)
	assert.EqualError(t, err, "pop")

	mms.AssertExpectations(t)
}

func TestPublishFFINonMultiparty(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = false

	_, err := ds.PublishFFI(context.Background(), "ffi1", "1.0", "ffi1-shared", false, nil)
	assert.Regexp(t, "FF10414", err)
}

func TestPublishContractAPI(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	mms := &syncasyncmocks.Sender{}

	url := "http://firefly"
	api := &core.ContractAPI{
		Name:      "ffi1",
		Namespace: "ns1",
		Published: false,
	}

	ds.mdi.On("GetContractAPIByNetworkName", context.Background(), "ns1", "api-shared").Return(nil, nil)
	ds.mcm.On("GetContractAPI", context.Background(), url, "api").Return(api, nil)
	ds.mcm.On("ResolveContractAPI", context.Background(), url, api).Return(nil)
	ds.mim.On("GetRootOrg", context.Background()).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			DID: "firefly:org1",
		},
	}, nil)
	ds.mim.On("ResolveInputSigningIdentity", mock.Anything, mock.Anything).Return(nil)
	ds.mbm.On("NewBroadcast", mock.Anything).Return(mms)
	mms.On("Prepare", context.Background()).Return(nil)
	mms.On("Send", context.Background()).Return(nil)
	mockRunAsGroupPassthrough(ds.mdi)

	result, err := ds.PublishContractAPI(context.Background(), url, "api", "api-shared", false, nil)
	assert.NoError(t, err)
	assert.Equal(t, api, result)
	assert.True(t, api.Published)

	mms.AssertExpectations(t)
}

func TestPublishContractAPIAlreadyPublished(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	url := "http://firefly"
	api := &core.ContractAPI{
		Name:      "ffi1",
		Namespace: "ns1",
		Published: true,
	}

	ds.mcm.On("GetContractAPI", context.Background(), url, "api").Return(api, nil)
	mockRunAsGroupPassthrough(ds.mdi)

	_, err := ds.PublishContractAPI(context.Background(), url, "api", "api-shared", false, nil)
	assert.Regexp(t, "FF10450", err)
}

func TestPublishContractAPIQueryFail(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	url := "http://firefly"

	ds.mcm.On("GetContractAPI", context.Background(), url, "api").Return(nil, fmt.Errorf("pop"))
	mockRunAsGroupPassthrough(ds.mdi)

	_, err := ds.PublishContractAPI(context.Background(), url, "api", "api-shared", false, nil)
	assert.EqualError(t, err, "pop")
}

func TestPublishContractAPIResolveFail(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	url := "http://firefly"
	api := &core.ContractAPI{
		Name:      "ffi1",
		Namespace: "ns1",
		Published: false,
	}

	ds.mcm.On("GetContractAPI", context.Background(), url, "api").Return(api, nil)
	ds.mcm.On("ResolveContractAPI", context.Background(), url, api).Return(fmt.Errorf("pop"))
	mockRunAsGroupPassthrough(ds.mdi)

	_, err := ds.PublishContractAPI(context.Background(), url, "api", "api-shared", false, nil)
	assert.EqualError(t, err, "pop")
}

func TestPublishContractAPINonMultiparty(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = false

	url := "http://firefly"

	_, err := ds.PublishContractAPI(context.Background(), url, "api", "api-shared", false, nil)
	assert.Regexp(t, "FF10414", err)
}

func TestPublishContractAPINetworkNameFail(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	url := "http://firefly"
	api := &core.ContractAPI{
		Name:      "ffi1",
		Namespace: "ns1",
		Published: false,
	}

	ds.mdi.On("GetContractAPIByNetworkName", context.Background(), "ns1", "api-shared").Return(nil, fmt.Errorf("pop"))
	ds.mcm.On("GetContractAPI", context.Background(), url, "api").Return(api, nil)
	ds.mcm.On("ResolveContractAPI", context.Background(), url, api).Return(nil)
	mockRunAsGroupPassthrough(ds.mdi)

	_, err := ds.PublishContractAPI(context.Background(), url, "api", "api-shared", false, nil)
	assert.EqualError(t, err, "pop")
}

func TestPublishContractAPINetworkNameConflict(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	url := "http://firefly"
	api := &core.ContractAPI{
		Name:      "ffi1",
		Namespace: "ns1",
		Published: false,
	}

	ds.mdi.On("GetContractAPIByNetworkName", context.Background(), "ns1", "api-shared").Return(&core.ContractAPI{}, nil)
	ds.mcm.On("GetContractAPI", context.Background(), url, "api").Return(api, nil)
	ds.mcm.On("ResolveContractAPI", context.Background(), url, api).Return(nil)
	mockRunAsGroupPassthrough(ds.mdi)

	_, err := ds.PublishContractAPI(context.Background(), url, "api", "api-shared", false, nil)
	assert.Regexp(t, "FF10448", err)
}

func TestPublishContractAPIInterfaceFail(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	url := "http://firefly"
	api := &core.ContractAPI{
		Name:      "ffi1",
		Namespace: "ns1",
		Published: false,
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
	}

	ds.mcm.On("GetContractAPI", context.Background(), url, "api").Return(api, nil)
	ds.mdi.On("GetContractAPIByNetworkName", context.Background(), "ns1", "api-shared").Return(nil, nil)
	ds.mcm.On("ResolveContractAPI", context.Background(), url, api).Return(nil)
	mockRunAsGroupPassthrough(ds.mdi)
	ds.mdi.On("GetFFIByID", context.Background(), "ns1", api.Interface.ID).Return(nil, fmt.Errorf("pop"))

	_, err := ds.PublishContractAPI(context.Background(), url, "api", "api-shared", false, nil)
	assert.EqualError(t, err, "pop")
}

func TestPublishContractAPIInterfaceNotFound(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	url := "http://firefly"
	api := &core.ContractAPI{
		Name:      "ffi1",
		Namespace: "ns1",
		Published: false,
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
	}

	ds.mcm.On("GetContractAPI", context.Background(), url, "api").Return(api, nil)
	ds.mdi.On("GetContractAPIByNetworkName", context.Background(), "ns1", "api-shared").Return(nil, nil)
	ds.mcm.On("ResolveContractAPI", context.Background(), url, api).Return(nil)
	mockRunAsGroupPassthrough(ds.mdi)
	ds.mdi.On("GetFFIByID", context.Background(), "ns1", api.Interface.ID).Return(nil, nil)

	_, err := ds.PublishContractAPI(context.Background(), url, "api", "api-shared", false, nil)
	assert.Regexp(t, "FF10303", err)
}

func TestPublishContractAPIInterfaceNotPublished(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	url := "http://firefly"
	api := &core.ContractAPI{
		Name:      "ffi1",
		Namespace: "ns1",
		Published: false,
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
	}

	ds.mcm.On("GetContractAPI", context.Background(), url, "api").Return(api, nil)
	ds.mdi.On("GetContractAPIByNetworkName", context.Background(), "ns1", "api-shared").Return(nil, nil)
	ds.mcm.On("ResolveContractAPI", context.Background(), url, api).Return(nil)
	mockRunAsGroupPassthrough(ds.mdi)
	ds.mdi.On("GetFFIByID", context.Background(), "ns1", api.Interface.ID).Return(&fftypes.FFI{
		Published: false,
	}, nil)

	_, err := ds.PublishContractAPI(context.Background(), url, "api", "api-shared", false, nil)
	assert.Regexp(t, "FF10451", err)
}
