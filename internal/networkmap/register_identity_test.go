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
	"context"
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/internal/syncasync"
	"github.com/hyperledger-firefly/firefly/mocks/definitionsmocks"
	"github.com/hyperledger-firefly/firefly/mocks/identitymanagermocks"
	"github.com/hyperledger-firefly/firefly/mocks/syncasyncmocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRegisterIdentityOrgWithParentOk(t *testing.T) {

	nm, cancel := newTestNetworkmap(t)
	defer cancel()

	parentIdentity := testOrg("parent1")

	mim := nm.identity.(*identitymanagermocks.Manager)
	mim.On("VerifyIdentityChain", nm.ctx, mock.AnythingOfType("*core.Identity")).Return(parentIdentity, false, nil)
	mim.On("ResolveIdentitySigner", nm.ctx, parentIdentity).Return(&core.SignerRef{
		Key: "0x23456",
	}, nil)

	mds := nm.defsender.(*definitionsmocks.Sender)

	mds.On("ClaimIdentity", nm.ctx,
		mock.AnythingOfType("*core.IdentityClaim"),
		mock.MatchedBy(func(sr *core.SignerRef) bool {
			return sr.Key == "0x12345"
		}),
		mock.MatchedBy(func(sr *core.SignerRef) bool {
			return sr.Key == "0x23456"
		}),
	).Return(nil)

	org, err := nm.RegisterIdentity(nm.ctx, &core.IdentityCreateDTO{
		Name:   "child1",
		Key:    "0x12345",
		Parent: fftypes.NewUUID().String(),
	}, false)
	assert.NoError(t, err)
	assert.NotNil(t, org)

	mim.AssertExpectations(t)
	mds.AssertExpectations(t)
}

func TestRegisterIdentityOrgWithParentWaitConfirmOk(t *testing.T) {

	nm, cancel := newTestNetworkmap(t)
	defer cancel()

	parentIdentity := testOrg("parent1")

	mim := nm.identity.(*identitymanagermocks.Manager)
	mim.On("VerifyIdentityChain", nm.ctx, mock.AnythingOfType("*core.Identity")).Return(parentIdentity, false, nil)
	mim.On("ResolveIdentitySigner", nm.ctx, parentIdentity).Return(&core.SignerRef{
		Key: "0x23456",
	}, nil)

	msa := nm.syncasync.(*syncasyncmocks.Bridge)
	msa.On("WaitForIdentity", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		ctx := args[0].(context.Context)
		id := args[1].(*fftypes.UUID)
		assert.NotNil(t, id)
		cb := args[2].(syncasync.SendFunction)
		err := cb(ctx)
		assert.NoError(t, err)
	}).Return(nil, nil)

	mds := nm.defsender.(*definitionsmocks.Sender)

	mds.On("ClaimIdentity", nm.ctx,
		mock.AnythingOfType("*core.IdentityClaim"),
		mock.MatchedBy(func(sr *core.SignerRef) bool {
			return sr.Key == "0x12345"
		}),
		mock.MatchedBy(func(sr *core.SignerRef) bool {
			return sr.Key == "0x23456"
		}),
	).Return(nil)

	_, err := nm.RegisterIdentity(nm.ctx, &core.IdentityCreateDTO{
		Name:   "child1",
		Key:    "0x12345",
		Parent: fftypes.NewUUID().String(),
	}, true)
	assert.NoError(t, err)

	mim.AssertExpectations(t)
	mds.AssertExpectations(t)
	msa.AssertExpectations(t)
}

func TestRegisterIdentityOrgNonMultiparty(t *testing.T) {

	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	nm.multiparty = nil

	parentIdentity := testOrg("parent1")

	mim := nm.identity.(*identitymanagermocks.Manager)
	mim.On("VerifyIdentityChain", nm.ctx, mock.AnythingOfType("*core.Identity")).Return(parentIdentity, false, nil)
	mim.On("CachedIdentityLookupMustExist", nm.ctx, "did:firefly:org/parent1").Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			ID:  fftypes.NewUUID(),
			DID: "did:firefly:org/parent1",
		},
	}, false, nil)

	mds := nm.defsender.(*definitionsmocks.Sender)
	mds.On("ClaimIdentity", nm.ctx,
		mock.AnythingOfType("*core.IdentityClaim"),
		mock.MatchedBy(func(sr *core.SignerRef) bool {
			return sr.Key == "0x12345"
		}),
		(*core.SignerRef)(nil),
	).Return(fmt.Errorf("pop"))

	_, err := nm.RegisterIdentity(nm.ctx, &core.IdentityCreateDTO{
		Name:   "custom1",
		Key:    "0x12345",
		Parent: "did:firefly:org/parent1",
	}, false)
	assert.Regexp(t, "pop", err)

	mim.AssertExpectations(t)
	mds.AssertExpectations(t)
}

func TestRegisterIdentityCustomWithParentFail(t *testing.T) {

	nm, cancel := newTestNetworkmap(t)
	defer cancel()

	parentIdentity := testOrg("parent1")

	mim := nm.identity.(*identitymanagermocks.Manager)
	mim.On("VerifyIdentityChain", nm.ctx, mock.AnythingOfType("*core.Identity")).Return(parentIdentity, false, nil)
	mim.On("ResolveIdentitySigner", nm.ctx, parentIdentity).Return(&core.SignerRef{
		Key: "0x23456",
	}, nil)

	mds := nm.defsender.(*definitionsmocks.Sender)

	mds.On("ClaimIdentity", nm.ctx,
		mock.AnythingOfType("*core.IdentityClaim"),
		mock.MatchedBy(func(sr *core.SignerRef) bool {
			return sr.Key == "0x12345"
		}),
		mock.MatchedBy(func(sr *core.SignerRef) bool {
			return sr.Key == "0x23456"
		}),
	).Return(nil)

	org, err := nm.RegisterIdentity(nm.ctx, &core.IdentityCreateDTO{
		Name:   "child1",
		Key:    "0x12345",
		Parent: fftypes.NewUUID().String(),
	}, false)
	assert.NoError(t, err)
	assert.NotNil(t, org)

	mim.AssertExpectations(t)
	mds.AssertExpectations(t)
}

func TestRegisterIdentityGetParentMsgFail(t *testing.T) {

	nm, cancel := newTestNetworkmap(t)
	defer cancel()

	parentIdentity := testOrg("parent1")

	mim := nm.identity.(*identitymanagermocks.Manager)
	mim.On("VerifyIdentityChain", nm.ctx, mock.AnythingOfType("*core.Identity")).Return(parentIdentity, false, nil)
	mim.On("ResolveIdentitySigner", nm.ctx, parentIdentity).Return(nil, fmt.Errorf("pop"))

	_, err := nm.RegisterIdentity(nm.ctx, &core.IdentityCreateDTO{
		Name:   "custom1",
		Key:    "0x12345",
		Parent: fftypes.NewUUID().String(),
	}, false)
	assert.Regexp(t, "pop", err)

	mim.AssertExpectations(t)
}

func TestRegisterIdentityMissingKey(t *testing.T) {

	nm, cancel := newTestNetworkmap(t)
	defer cancel()

	mim := nm.identity.(*identitymanagermocks.Manager)
	mim.On("VerifyIdentityChain", nm.ctx, mock.AnythingOfType("*core.Identity")).Return(nil, false, nil)

	_, err := nm.RegisterIdentity(nm.ctx, &core.IdentityCreateDTO{
		Name:   "custom1",
		Parent: fftypes.NewUUID().String(),
	}, false)
	assert.Regexp(t, "FF10352", err)

	mim.AssertExpectations(t)
}

func TestRegisterIdentityVerifyFail(t *testing.T) {

	nm, cancel := newTestNetworkmap(t)
	defer cancel()

	mim := nm.identity.(*identitymanagermocks.Manager)
	mim.On("VerifyIdentityChain", nm.ctx, mock.AnythingOfType("*core.Identity")).Return(nil, false, fmt.Errorf("pop"))

	_, err := nm.RegisterIdentity(nm.ctx, &core.IdentityCreateDTO{
		Name:   "custom1",
		Parent: fftypes.NewUUID().String(),
	}, false)
	assert.Regexp(t, "pop", err)

	mim.AssertExpectations(t)
}

func TestRegisterIdentityBadParent(t *testing.T) {

	nm, cancel := newTestNetworkmap(t)
	defer cancel()

	mim := nm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupMustExist", nm.ctx, "did:firefly:org/1").Return(nil, false, fmt.Errorf("pop"))

	_, err := nm.RegisterIdentity(nm.ctx, &core.IdentityCreateDTO{
		Name:   "custom1",
		Parent: "did:firefly:org/1",
	}, false)
	assert.Regexp(t, "pop", err)

	mim.AssertExpectations(t)
}
