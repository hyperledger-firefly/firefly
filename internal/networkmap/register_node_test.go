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

package networkmap

import (
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/internal/multiparty"
	"github.com/hyperledger-firefly/firefly/mocks/dataexchangemocks"
	"github.com/hyperledger-firefly/firefly/mocks/definitionsmocks"
	"github.com/hyperledger-firefly/firefly/mocks/identitymanagermocks"
	"github.com/hyperledger-firefly/firefly/mocks/multipartymocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRegisterNodeOk(t *testing.T) {

	nm, cancel := newTestNetworkmap(t)
	defer cancel()

	parentOrg := testOrg("org1")
	signerRef := &core.SignerRef{Key: "0x23456"}

	mim := nm.identity.(*identitymanagermocks.Manager)
	mim.On("GetRootOrg", nm.ctx).Return(parentOrg, nil)
	mim.On("VerifyIdentityChain", nm.ctx, mock.AnythingOfType("*core.Identity")).Return(parentOrg, false, nil)
	mim.On("ResolveIdentitySigner", nm.ctx, parentOrg).Return(signerRef, nil)

	mdx := nm.exchange.(*dataexchangemocks.Plugin)
	dxPeer := fftypes.JSONObject{
		"id":       "peer1",
		"endpoint": "details",
	}
	mdx.On("GetEndpointInfo", nm.ctx, "node1").Return(dxPeer, nil)

	mds := nm.defsender.(*definitionsmocks.Sender)
	mds.On("ClaimIdentity", nm.ctx,
		mock.AnythingOfType("*core.IdentityClaim"),
		signerRef,
		(*core.SignerRef)(nil),
	).Return(nil)

	mmp := nm.multiparty.(*multipartymocks.Manager)
	mmp.On("LocalNode").Return(multiparty.LocalNode{Name: "node1"})

	node, err := nm.RegisterNode(nm.ctx, false)
	assert.NoError(t, err)
	assert.NotNil(t, node)

	mim.AssertExpectations(t)
	mdx.AssertExpectations(t)
	mds.AssertExpectations(t)
	mmp.AssertExpectations(t)
}

func TestRegisterNodeMissingName(t *testing.T) {

	nm, cancel := newTestNetworkmap(t)
	defer cancel()

	parentOrg := testOrg("org1")

	mim := nm.identity.(*identitymanagermocks.Manager)
	mim.On("GetRootOrg", nm.ctx).Return(parentOrg, nil)

	mmp := nm.multiparty.(*multipartymocks.Manager)
	mmp.On("LocalNode").Return(multiparty.LocalNode{})

	_, err := nm.RegisterNode(nm.ctx, false)
	assert.Regexp(t, "FF10216", err)

	mim.AssertExpectations(t)
	mmp.AssertExpectations(t)
}

func TestRegisterNodePeerInfoFail(t *testing.T) {

	nm, cancel := newTestNetworkmap(t)
	defer cancel()

	parentOrg := testOrg("org1")

	mim := nm.identity.(*identitymanagermocks.Manager)
	mim.On("GetRootOrg", nm.ctx).Return(parentOrg, nil)

	mdx := nm.exchange.(*dataexchangemocks.Plugin)
	mdx.On("GetEndpointInfo", nm.ctx, "node1").Return(fftypes.JSONObject{}, fmt.Errorf("pop"))

	mmp := nm.multiparty.(*multipartymocks.Manager)
	mmp.On("LocalNode").Return(multiparty.LocalNode{Name: "node1"})

	_, err := nm.RegisterNode(nm.ctx, false)
	assert.Regexp(t, "pop", err)

	mim.AssertExpectations(t)
	mdx.AssertExpectations(t)
	mmp.AssertExpectations(t)
}

func TestRegisterNodeGetOwnerFail(t *testing.T) {

	nm, cancel := newTestNetworkmap(t)
	defer cancel()

	mim := nm.identity.(*identitymanagermocks.Manager)
	mim.On("GetRootOrg", nm.ctx).Return(nil, fmt.Errorf("pop"))

	_, err := nm.RegisterNode(nm.ctx, false)
	assert.Regexp(t, "pop", err)
}
