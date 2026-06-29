// Copyright © 2023 Kaleido, Inc.
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

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/mocks/syncasyncmocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestBroadcastTokenPoolInvalid(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "",
		Name:      "",
		Type:      core.TokenTypeNonFungible,
		Locator:   "N1",
		Symbol:    "COIN",
		Published: true,
	}

	err := ds.DefineTokenPool(context.Background(), pool, false)
	assert.Regexp(t, "FF10420", err)
}

func TestBroadcastTokenPoolInvalidNonMultiparty(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = false

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "",
		Name:      "",
		Type:      core.TokenTypeNonFungible,
		Locator:   "N1",
		Symbol:    "COIN",
		Published: false,
	}

	err := ds.DefineTokenPool(context.Background(), pool, false)
	assert.Regexp(t, "FF00140", err)
}

func TestBroadcastTokenPoolPublishNonMultiparty(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = false

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "",
		Name:      "",
		Type:      core.TokenTypeNonFungible,
		Locator:   "N1",
		Symbol:    "COIN",
		Published: true,
	}

	err := ds.DefineTokenPool(context.Background(), pool, false)
	assert.Regexp(t, "FF10414", err)
}

func TestBroadcastTokenPoolInvalidNameMultiparty(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "",
		Name:      "",
		Type:      core.TokenTypeNonFungible,
		Locator:   "N1",
		Symbol:    "COIN",
		Connector: "connector1",
		Published: true,
	}

	err := ds.DefineTokenPool(context.Background(), pool, false)
	assert.Regexp(t, "FF00140", err)
}

func TestDefineTokenPoolOk(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	mms := &syncasyncmocks.Sender{}

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Name:      "mypool",
		Type:      core.TokenTypeNonFungible,
		Locator:   "N1",
		Symbol:    "COIN",
		Connector: "connector1",
		Published: true,
	}

	ds.mim.On("GetRootOrg", ds.ctx).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			DID: "firefly:org1",
		},
	}, nil)
	ds.mim.On("ResolveInputSigningIdentity", mock.Anything, mock.Anything).Return(nil)
	ds.mbm.On("NewBroadcast", mock.Anything).Return(mms)
	mms.On("Send", ds.ctx).Return(nil)
	ds.mdi.On("GetTokenPoolByNetworkName", ds.ctx, "ns1", "mypool").Return(nil, nil)

	err := ds.DefineTokenPool(ds.ctx, pool, false)
	assert.NoError(t, err)

	mms.AssertExpectations(t)
}

func TestDefineTokenPoolkONonMultiparty(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = false

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Name:      "mypool",
		Type:      core.TokenTypeNonFungible,
		Locator:   "N1",
		Symbol:    "COIN",
		Connector: "connector1",
		Active:    true,
		Published: false,
	}

	ds.mdi.On("InsertOrGetTokenPool", mock.Anything, pool).Return(nil, nil)
	ds.mam.On("ActivateTokenPool", mock.Anything, pool).Return(nil)

	err := ds.DefineTokenPool(context.Background(), pool, false)
	assert.NoError(t, err)
}

func TestDefineTokenPoolNonMultipartyTokenPoolFail(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Name:      "mypool",
		Type:      core.TokenTypeNonFungible,
		Locator:   "N1",
		Symbol:    "COIN",
		Connector: "connector1",
		Published: false,
	}

	ds.mdi.On("InsertOrGetTokenPool", mock.Anything, pool).Return(nil, fmt.Errorf("pop"))

	err := ds.DefineTokenPool(context.Background(), pool, false)
	assert.Regexp(t, "pop", err)
}

func TestDefineTokenPoolNonMultipartyTokenPoolFailInner(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Name:      "mypool",
		Type:      core.TokenTypeNonFungible,
		Locator:   "N1",
		Symbol:    "COIN",
		Connector: "connector1",
		Published: false,
	}

	ds.mdi.On("InsertOrGetTokenPool", mock.Anything, pool).Return(nil, fmt.Errorf("error2: [%w]", fmt.Errorf("pop")))

	err := ds.DefineTokenPool(context.Background(), pool, false)
	assert.Regexp(t, "pop", err)
}

func TestDefineTokenPoolBadName(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Name:      "///bad/////",
		Type:      core.TokenTypeNonFungible,
		Locator:   "N1",
		Symbol:    "COIN",
		Connector: "connector1",
		Published: false,
	}

	err := ds.DefineTokenPool(context.Background(), pool, false)
	assert.Regexp(t, "FF00140", err)
}

func TestPublishTokenPool(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	mms := &syncasyncmocks.Sender{}

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Name:      "pool1",
		Type:      core.TokenTypeNonFungible,
		Locator:   "N1",
		Symbol:    "COIN",
		Connector: "connector1",
		Published: false,
	}

	ds.mam.On("GetTokenPoolByNameOrID", mock.Anything, "pool1").Return(pool, nil)
	ds.mim.On("GetRootOrg", context.Background()).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			DID: "firefly:org1",
		},
	}, nil)
	ds.mim.On("ResolveInputSigningIdentity", mock.Anything, mock.Anything).Return(nil)
	ds.mbm.On("NewBroadcast", mock.Anything).Return(mms)
	mms.On("Prepare", context.Background()).Return(nil)
	mms.On("Send", context.Background()).Return(nil)
	ds.mdi.On("GetTokenPoolByNetworkName", mock.Anything, "ns1", "pool-shared").Return(nil, nil)
	mockRunAsGroupPassthrough(ds.mdi)

	result, err := ds.PublishTokenPool(context.Background(), "pool1", "pool-shared", false)
	assert.NoError(t, err)
	assert.Equal(t, pool, result)
	assert.True(t, pool.Published)

	mms.AssertExpectations(t)
}

func TestPublishTokenPoolNonMultiparty(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = false

	_, err := ds.PublishTokenPool(context.Background(), "pool1", "pool-shared", false)
	assert.Regexp(t, "FF10414", err)
}

func TestPublishTokenPoolAlreadyPublished(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Name:      "pool1",
		Type:      core.TokenTypeNonFungible,
		Locator:   "N1",
		Symbol:    "COIN",
		Connector: "connector1",
		Published: true,
	}

	ds.mam.On("GetTokenPoolByNameOrID", mock.Anything, "pool1").Return(pool, nil)
	mockRunAsGroupPassthrough(ds.mdi)

	_, err := ds.PublishTokenPool(context.Background(), "pool1", "pool-shared", false)
	assert.Regexp(t, "FF10450", err)
}

func TestPublishTokenPoolQueryFail(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	ds.mam.On("GetTokenPoolByNameOrID", mock.Anything, "pool1").Return(nil, fmt.Errorf("pop"))
	mockRunAsGroupPassthrough(ds.mdi)

	_, err := ds.PublishTokenPool(context.Background(), "pool1", "pool-shared", false)
	assert.EqualError(t, err, "pop")
}

func TestPublishTokenPoolNetworkNameError(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Name:      "pool1",
		Type:      core.TokenTypeNonFungible,
		Locator:   "N1",
		Symbol:    "COIN",
		Connector: "connector1",
		Published: false,
	}

	ds.mam.On("GetTokenPoolByNameOrID", mock.Anything, "pool1").Return(pool, nil)
	ds.mdi.On("GetTokenPoolByNetworkName", mock.Anything, "ns1", "pool-shared").Return(nil, fmt.Errorf("pop"))
	mockRunAsGroupPassthrough(ds.mdi)

	_, err := ds.PublishTokenPool(context.Background(), "pool1", "pool-shared", false)
	assert.EqualError(t, err, "pop")
}

func TestPublishTokenPoolNetworkNameConflict(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Name:      "pool1",
		Type:      core.TokenTypeNonFungible,
		Locator:   "N1",
		Symbol:    "COIN",
		Connector: "connector1",
		Published: false,
	}

	ds.mam.On("GetTokenPoolByNameOrID", mock.Anything, "pool1").Return(pool, nil)
	ds.mdi.On("GetTokenPoolByNetworkName", mock.Anything, "ns1", "pool-shared").Return(&core.TokenPool{}, nil)
	mockRunAsGroupPassthrough(ds.mdi)

	_, err := ds.PublishTokenPool(context.Background(), "pool1", "pool-shared", false)
	assert.Regexp(t, "FF10448", err)
}

func TestPublishTokenPoolResolveFail(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Name:      "pool1",
		Type:      core.TokenTypeNonFungible,
		Locator:   "N1",
		Symbol:    "COIN",
		Connector: "connector1",
		Published: false,
	}

	ds.mam.On("GetTokenPoolByNameOrID", mock.Anything, "pool1").Return(pool, nil)
	ds.mim.On("GetRootOrg", context.Background()).Return(nil, fmt.Errorf("pop"))
	ds.mdi.On("GetTokenPoolByNetworkName", mock.Anything, "ns1", "pool-shared").Return(nil, nil)
	mockRunAsGroupPassthrough(ds.mdi)

	_, err := ds.PublishTokenPool(context.Background(), "pool1", "pool-shared", false)
	assert.EqualError(t, err, "pop")
}

func TestPublishTokenPoolPrepareFail(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	mms := &syncasyncmocks.Sender{}

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Name:      "pool1",
		Type:      core.TokenTypeNonFungible,
		Locator:   "N1",
		Symbol:    "COIN",
		Connector: "connector1",
		Published: false,
	}

	ds.mam.On("GetTokenPoolByNameOrID", mock.Anything, "pool1").Return(pool, nil)
	ds.mim.On("GetRootOrg", context.Background()).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			DID: "firefly:org1",
		},
	}, nil)
	ds.mim.On("ResolveInputSigningIdentity", mock.Anything, mock.Anything).Return(nil)
	ds.mbm.On("NewBroadcast", mock.Anything).Return(mms)
	mms.On("Prepare", context.Background()).Return(fmt.Errorf("pop"))
	ds.mdi.On("GetTokenPoolByNetworkName", mock.Anything, "ns1", "pool-shared").Return(nil, nil)
	mockRunAsGroupPassthrough(ds.mdi)

	_, err := ds.PublishTokenPool(context.Background(), "pool1", "pool-shared", false)
	assert.EqualError(t, err, "pop")

	mms.AssertExpectations(t)
}

func TestPublishTokenPoolSendFail(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	mms := &syncasyncmocks.Sender{}

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Name:      "pool1",
		Type:      core.TokenTypeNonFungible,
		Locator:   "N1",
		Symbol:    "COIN",
		Connector: "connector1",
		Published: false,
	}

	ds.mam.On("GetTokenPoolByNameOrID", mock.Anything, "pool1").Return(pool, nil)
	ds.mim.On("GetRootOrg", context.Background()).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			DID: "firefly:org1",
		},
	}, nil)
	ds.mim.On("ResolveInputSigningIdentity", mock.Anything, mock.Anything).Return(nil)
	ds.mbm.On("NewBroadcast", mock.Anything).Return(mms)
	mms.On("Prepare", context.Background()).Return(nil)
	mms.On("Send", context.Background()).Return(fmt.Errorf("pop"))
	ds.mdi.On("GetTokenPoolByNetworkName", mock.Anything, "ns1", "pool-shared").Return(nil, nil)
	mockRunAsGroupPassthrough(ds.mdi)

	_, err := ds.PublishTokenPool(context.Background(), "pool1", "pool-shared", false)
	assert.EqualError(t, err, "pop")

	mms.AssertExpectations(t)
}

func TestPublishTokenPoolConfirm(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	mms := &syncasyncmocks.Sender{}

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Name:      "pool1",
		Type:      core.TokenTypeNonFungible,
		Locator:   "N1",
		Symbol:    "COIN",
		Connector: "connector1",
		Published: false,
	}

	ds.mam.On("GetTokenPoolByNameOrID", mock.Anything, "pool1").Return(pool, nil)
	ds.mim.On("GetRootOrg", context.Background()).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			DID: "firefly:org1",
		},
	}, nil)
	ds.mim.On("ResolveInputSigningIdentity", mock.Anything, mock.Anything).Return(nil)
	ds.mbm.On("NewBroadcast", mock.Anything).Return(mms)
	mms.On("Prepare", context.Background()).Return(nil)
	mms.On("SendAndWait", context.Background()).Return(nil)
	ds.mdi.On("GetTokenPoolByNetworkName", mock.Anything, "ns1", "pool-shared").Return(nil, nil)
	mockRunAsGroupPassthrough(ds.mdi)

	_, err := ds.PublishTokenPool(context.Background(), "pool1", "pool-shared", true)
	assert.NoError(t, err)
	assert.True(t, pool.Published)

	mms.AssertExpectations(t)
}

func TestPublishTokenPoolInterfaceFail(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Name:      "pool1",
		Type:      core.TokenTypeNonFungible,
		Locator:   "N1",
		Symbol:    "COIN",
		Connector: "connector1",
		Published: false,
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
	}

	ds.mam.On("GetTokenPoolByNameOrID", mock.Anything, "pool1").Return(pool, nil)
	ds.mdi.On("GetTokenPoolByNetworkName", mock.Anything, "ns1", "pool-shared").Return(nil, nil)
	mockRunAsGroupPassthrough(ds.mdi)
	ds.mdi.On("GetFFIByID", context.Background(), "ns1", pool.Interface.ID).Return(nil, fmt.Errorf("pop"))

	_, err := ds.PublishTokenPool(context.Background(), "pool1", "pool-shared", false)
	assert.EqualError(t, err, "pop")
}

func TestPublishTokenPoolInterfaceNotFound(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Name:      "pool1",
		Type:      core.TokenTypeNonFungible,
		Locator:   "N1",
		Symbol:    "COIN",
		Connector: "connector1",
		Published: false,
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
	}

	ds.mam.On("GetTokenPoolByNameOrID", mock.Anything, "pool1").Return(pool, nil)
	ds.mdi.On("GetTokenPoolByNetworkName", mock.Anything, "ns1", "pool-shared").Return(nil, nil)
	mockRunAsGroupPassthrough(ds.mdi)
	ds.mdi.On("GetFFIByID", context.Background(), "ns1", pool.Interface.ID).Return(nil, nil)

	_, err := ds.PublishTokenPool(context.Background(), "pool1", "pool-shared", false)
	assert.Regexp(t, "FF10303", err)
}

func TestPublishTokenPoolInterfaceNotPublished(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Name:      "pool1",
		Type:      core.TokenTypeNonFungible,
		Locator:   "N1",
		Symbol:    "COIN",
		Connector: "connector1",
		Published: false,
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
	}

	ds.mam.On("GetTokenPoolByNameOrID", mock.Anything, "pool1").Return(pool, nil)
	ds.mdi.On("GetTokenPoolByNetworkName", mock.Anything, "ns1", "pool-shared").Return(nil, nil)
	mockRunAsGroupPassthrough(ds.mdi)
	ds.mdi.On("GetFFIByID", context.Background(), "ns1", pool.Interface.ID).Return(&fftypes.FFI{
		Published: false,
	}, nil)

	_, err := ds.PublishTokenPool(context.Background(), "pool1", "pool-shared", false)
	assert.Regexp(t, "FF10451", err)
}
