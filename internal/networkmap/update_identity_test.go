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
	"github.com/hyperledger-firefly/firefly/mocks/definitionsmocks"
	"github.com/hyperledger-firefly/firefly/mocks/identitymanagermocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestUpdateIdentityProfileOk(t *testing.T) {

	nm, cancel := newTestNetworkmap(t)
	defer cancel()

	identity := testOrg("org1")

	mim := nm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupByID", nm.ctx, identity.ID).Return(identity, nil)
	signerRef := &core.SignerRef{Key: "0x12345"}
	mim.On("ResolveIdentitySigner", nm.ctx, identity).Return(signerRef, nil)

	mds := nm.defsender.(*definitionsmocks.Sender)

	mds.On("UpdateIdentity", nm.ctx,
		mock.AnythingOfType("*core.Identity"),
		mock.AnythingOfType("*core.IdentityUpdate"),
		mock.MatchedBy(func(sr *core.SignerRef) bool {
			return sr.Key == "0x12345"
		}),
		true).Return(nil)

	org, err := nm.UpdateIdentity(nm.ctx, identity.ID.String(), &core.IdentityUpdateDTO{
		IdentityProfile: core.IdentityProfile{
			Description: "new desc",
			Profile:     fftypes.JSONObject{"new": "profile"},
		},
	}, true)
	assert.NoError(t, err)
	assert.NotNil(t, org)

	mim.AssertExpectations(t)
	mds.AssertExpectations(t)
}

func TestUpdateIdentityProfileBroadcastFail(t *testing.T) {

	nm, cancel := newTestNetworkmap(t)
	defer cancel()

	identity := testOrg("org1")

	mim := nm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupByID", nm.ctx, identity.ID).Return(identity, nil)
	signerRef := &core.SignerRef{Key: "0x12345"}
	mim.On("ResolveIdentitySigner", nm.ctx, identity).Return(signerRef, nil)

	mds := nm.defsender.(*definitionsmocks.Sender)
	mds.On("UpdateIdentity", nm.ctx,
		mock.AnythingOfType("*core.Identity"),
		mock.AnythingOfType("*core.IdentityUpdate"),
		mock.MatchedBy(func(sr *core.SignerRef) bool {
			return sr.Key == "0x12345"
		}),
		true).Return(fmt.Errorf("pop"))

	_, err := nm.UpdateIdentity(nm.ctx, identity.ID.String(), &core.IdentityUpdateDTO{
		IdentityProfile: core.IdentityProfile{
			Description: "new desc",
			Profile:     fftypes.JSONObject{"new": "profile"},
		},
	}, true)
	assert.Regexp(t, "pop", err)

	mim.AssertExpectations(t)
	mds.AssertExpectations(t)
}

func TestUpdateIdentityProfileBadProfile(t *testing.T) {

	nm, cancel := newTestNetworkmap(t)
	defer cancel()

	identity := testOrg("org1")

	mim := nm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupByID", nm.ctx, identity.ID).Return(identity, nil)
	signerRef := &core.SignerRef{Key: "0x12345"}
	mim.On("ResolveIdentitySigner", nm.ctx, identity).Return(signerRef, nil)

	_, err := nm.UpdateIdentity(nm.ctx, identity.ID.String(), &core.IdentityUpdateDTO{
		IdentityProfile: core.IdentityProfile{
			Description: string(make([]byte, 4097)),
			Profile:     fftypes.JSONObject{"new": "profile"},
		},
	}, true)
	assert.Regexp(t, "FF00135", err)

	mim.AssertExpectations(t)
}

func TestUpdateIdentityProfileNotFound(t *testing.T) {

	nm, cancel := newTestNetworkmap(t)
	defer cancel()

	identity := testOrg("org1")

	mim := nm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupByID", nm.ctx, identity.ID).Return(nil, nil)

	_, err := nm.UpdateIdentity(nm.ctx, identity.ID.String(), &core.IdentityUpdateDTO{
		IdentityProfile: core.IdentityProfile{
			Description: string(make([]byte, 4097)),
			Profile:     fftypes.JSONObject{"new": "profile"},
		},
	}, true)
	assert.Regexp(t, "FF10143", err)

	mim.AssertExpectations(t)
}

func TestUpdateIdentityProfileLookupFail(t *testing.T) {

	nm, cancel := newTestNetworkmap(t)
	defer cancel()

	identity := testOrg("org1")

	mim := nm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupByID", nm.ctx, identity.ID).Return(nil, fmt.Errorf("pop"))

	_, err := nm.UpdateIdentity(nm.ctx, identity.ID.String(), &core.IdentityUpdateDTO{
		IdentityProfile: core.IdentityProfile{
			Description: string(make([]byte, 4097)),
			Profile:     fftypes.JSONObject{"new": "profile"},
		},
	}, true)
	assert.Regexp(t, "pop", err)

	mim.AssertExpectations(t)
}

func TestUpdateIdentityProfileClaimLookupFail(t *testing.T) {

	nm, cancel := newTestNetworkmap(t)
	defer cancel()

	identity := testOrg("org1")

	mim := nm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupByID", nm.ctx, identity.ID).Return(identity, nil)
	signerRef := &core.SignerRef{Key: "0x12345"}
	mim.On("ResolveIdentitySigner", nm.ctx, identity).Return(signerRef, fmt.Errorf("pop"))

	_, err := nm.UpdateIdentity(nm.ctx, identity.ID.String(), &core.IdentityUpdateDTO{
		IdentityProfile: core.IdentityProfile{
			Description: "Desc1",
			Profile:     fftypes.JSONObject{"new": "profile"},
		},
	}, true)
	assert.Regexp(t, "pop", err)

	mim.AssertExpectations(t)
}

func TestUpdateIdentityProfileBadID(t *testing.T) {

	nm, cancel := newTestNetworkmap(t)
	defer cancel()

	_, err := nm.UpdateIdentity(nm.ctx, "badness", &core.IdentityUpdateDTO{}, true)
	assert.Regexp(t, "FF00138", err)
}

func TestUpdateIdentityProfileExistingIdNotSpecified(t *testing.T) {

	nm, cancel := newTestNetworkmap(t)
	defer cancel()

	identity := testOrg("org1")
	identity.Profile["id"] = "member_0/node_aaaaaa"

	mim := nm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupByID", nm.ctx, identity.ID).Return(identity, nil)
	signerRef := &core.SignerRef{Key: "0x12345"}
	mim.On("ResolveIdentitySigner", nm.ctx, identity).Return(signerRef, nil)

	mds := nm.defsender.(*definitionsmocks.Sender)

	mds.On("UpdateIdentity", nm.ctx,
		mock.AnythingOfType("*core.Identity"),
		mock.MatchedBy(func(iden *core.IdentityUpdate) bool {
			return iden.Updates.Profile.GetString("id") == "member_0/node_aaaaaa"
		}),
		mock.MatchedBy(func(sr *core.SignerRef) bool {
			return sr.Key == "0x12345"
		}),
		true).Return(nil)

	org, err := nm.UpdateIdentity(nm.ctx, identity.ID.String(), &core.IdentityUpdateDTO{
		IdentityProfile: core.IdentityProfile{
			Description: "new desc",
			Profile:     fftypes.JSONObject{"new": "profile"},
		},
	}, true)
	assert.NoError(t, err)
	assert.NotNil(t, org)

	mim.AssertExpectations(t)
	mds.AssertExpectations(t)
}

func TestUpdateIdentityProfileOverwrite(t *testing.T) {

	nm, cancel := newTestNetworkmap(t)
	defer cancel()

	identity := testOrg("org1")
	identity.Profile["id"] = "member_0/node_aaaaaa"

	mim := nm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupByID", nm.ctx, identity.ID).Return(identity, nil)
	signerRef := &core.SignerRef{Key: "0x12345"}
	mim.On("ResolveIdentitySigner", nm.ctx, identity).Return(signerRef, nil)

	mds := nm.defsender.(*definitionsmocks.Sender)

	mds.On("UpdateIdentity", nm.ctx,
		mock.AnythingOfType("*core.Identity"),
		mock.MatchedBy(func(iden *core.IdentityUpdate) bool {
			return iden.Updates.Profile.GetString("id") == "member_0/node_bbbbbb"
		}),
		mock.MatchedBy(func(sr *core.SignerRef) bool {
			return sr.Key == "0x12345"
		}),
		true).Return(nil)

	org, err := nm.UpdateIdentity(nm.ctx, identity.ID.String(), &core.IdentityUpdateDTO{
		IdentityProfile: core.IdentityProfile{
			Description: "new desc",
			Profile:     fftypes.JSONObject{"new": "profile", "id": "member_0/node_bbbbbb"},
		},
	}, true)
	assert.NoError(t, err)
	assert.NotNil(t, org)

	mim.AssertExpectations(t)
	mds.AssertExpectations(t)
}

func TestUpdateIdentityProfileNotProvided(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()

	identity := testOrg("org1")

	mim := nm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupByID", nm.ctx, identity.ID).Return(identity, nil)

	_, err := nm.UpdateIdentity(nm.ctx, identity.ID.String(), &core.IdentityUpdateDTO{}, true)
	assert.Regexp(t, "FF10480", err)
}
