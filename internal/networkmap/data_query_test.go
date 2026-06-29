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
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/mocks/identitymanagermocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestGetOrganizationByNameOrIDOk(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	id := fftypes.NewUUID()
	nm.database.(*databasemocks.Plugin).On("GetIdentityByID", nm.ctx, "ns1", id).
		Return(&core.Identity{IdentityBase: core.IdentityBase{ID: id, Type: core.IdentityTypeOrg}}, nil)
	res, err := nm.GetOrganizationByNameOrID(nm.ctx, id.String())
	assert.NoError(t, err)
	assert.Equal(t, *id, *res.ID)
}

func TestGetOrganizationByNameOrIDNotOrg(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	id := fftypes.NewUUID()
	nm.database.(*databasemocks.Plugin).On("GetIdentityByID", nm.ctx, "ns1", id).
		Return(&core.Identity{IdentityBase: core.IdentityBase{ID: id, Type: core.IdentityTypeNode}}, nil)
	res, err := nm.GetOrganizationByNameOrID(nm.ctx, id.String())
	assert.NoError(t, err)
	assert.Nil(t, res)
}

func TestGetOrganizationByNameOrIDNotFound(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	id := fftypes.NewUUID()
	nm.database.(*databasemocks.Plugin).On("GetIdentityByID", nm.ctx, "ns1", id).Return(nil, nil)
	_, err := nm.GetOrganizationByNameOrID(nm.ctx, id.String())
	assert.Regexp(t, "FF10109", err)
}

func TestGetOrganizationByNameOrIDError(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	id := fftypes.NewUUID()
	nm.database.(*databasemocks.Plugin).On("GetIdentityByID", nm.ctx, "ns1", id).Return(nil, fmt.Errorf("pop"))
	_, err := nm.GetOrganizationByNameOrID(nm.ctx, id.String())
	assert.Regexp(t, "pop", err)
}

func TestGetOrganizationByNameBadName(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	_, err := nm.GetOrganizationByNameOrID(nm.ctx, "!bad")
	assert.Regexp(t, "FF00140", err)
}

func TestGetOrganizationByNameError(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	nm.database.(*databasemocks.Plugin).On("GetIdentityByName", nm.ctx, core.IdentityTypeOrg, "ns1", "bad").Return(nil, fmt.Errorf("pop"))
	_, err := nm.GetOrganizationByNameOrID(nm.ctx, "bad")
	assert.Regexp(t, "pop", err)
}

func TestGetNodeByNameOrIDOk(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	id := fftypes.NewUUID()
	nm.database.(*databasemocks.Plugin).On("GetIdentityByID", nm.ctx, "ns1", id).
		Return(&core.Identity{IdentityBase: core.IdentityBase{ID: id, Type: core.IdentityTypeNode}}, nil)
	res, err := nm.GetNodeByNameOrID(nm.ctx, id.String())
	assert.NoError(t, err)
	assert.Equal(t, *id, *res.ID)
}

func TestGetNodeByNameOrIDWrongType(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	id := fftypes.NewUUID()
	nm.database.(*databasemocks.Plugin).On("GetIdentityByID", nm.ctx, "ns1", id).
		Return(&core.Identity{IdentityBase: core.IdentityBase{ID: id, Type: core.IdentityTypeOrg}}, nil)
	res, err := nm.GetNodeByNameOrID(nm.ctx, id.String())
	assert.NoError(t, err)
	assert.Nil(t, res)
}

func TestGetNodeByNameOrIDNotFound(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	id := fftypes.NewUUID()
	nm.database.(*databasemocks.Plugin).On("GetIdentityByID", nm.ctx, "ns1", id).Return(nil, nil)
	_, err := nm.GetNodeByNameOrID(nm.ctx, id.String())
	assert.Regexp(t, "FF10109", err)
}

func TestGetNodeByNameOrIDError(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	id := fftypes.NewUUID()
	nm.database.(*databasemocks.Plugin).On("GetIdentityByID", nm.ctx, "ns1", id).Return(nil, fmt.Errorf("pop"))
	_, err := nm.GetNodeByNameOrID(nm.ctx, id.String())
	assert.Regexp(t, "pop", err)
}

func TestGetNodeByNameBadName(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	_, err := nm.GetNodeByNameOrID(nm.ctx, "!bad")
	assert.Regexp(t, "FF00140", err)
}

func TestGetNodeByNameError(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	nm.database.(*databasemocks.Plugin).On("GetIdentityByName", nm.ctx, core.IdentityTypeNode, "ns1", "bad").Return(nil, fmt.Errorf("pop"))
	_, err := nm.GetNodeByNameOrID(nm.ctx, "bad")
	assert.Regexp(t, "pop", err)
}

func TestGetOrganizations(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	nm.database.(*databasemocks.Plugin).On("GetIdentities", nm.ctx, "ns1", mock.Anything).Return([]*core.Identity{}, nil, nil)
	res, _, err := nm.GetOrganizations(nm.ctx, database.IdentityQueryFactory.NewFilter(nm.ctx).And())
	assert.NoError(t, err)
	assert.Empty(t, res)
}

func TestGetNodes(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	nm.database.(*databasemocks.Plugin).On("GetIdentities", nm.ctx, "ns1", mock.Anything).Return([]*core.Identity{}, nil, nil)
	res, _, err := nm.GetNodes(nm.ctx, database.IdentityQueryFactory.NewFilter(nm.ctx).And())
	assert.NoError(t, err)
	assert.Empty(t, res)
}

func TestGetIdentityByIDOk(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	id := fftypes.NewUUID()
	nm.database.(*databasemocks.Plugin).On("GetIdentityByID", nm.ctx, "ns1", id).
		Return(&core.Identity{IdentityBase: core.IdentityBase{ID: id, Type: core.IdentityTypeOrg, Namespace: "ns1"}}, nil)
	res, err := nm.GetIdentityByID(nm.ctx, id.String())
	assert.NoError(t, err)
	assert.Equal(t, *id, *res.ID)
}

func TestGetIdentityByIDNotFound(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	id := fftypes.NewUUID()
	nm.database.(*databasemocks.Plugin).On("GetIdentityByID", nm.ctx, "ns1", id).Return(nil, nil)
	_, err := nm.GetIdentityByID(nm.ctx, id.String())
	assert.Regexp(t, "FF10109", err)
}

func TestGetIdentityByIDError(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	id := fftypes.NewUUID()
	nm.database.(*databasemocks.Plugin).On("GetIdentityByID", nm.ctx, "ns1", id).Return(nil, fmt.Errorf("pop"))
	_, err := nm.GetIdentityByID(nm.ctx, id.String())
	assert.Regexp(t, "pop", err)
}

func TestGetIdentityByIDWithVerifiersError(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	id := fftypes.NewUUID()
	nm.database.(*databasemocks.Plugin).On("GetIdentityByID", nm.ctx, "ns1", id).Return(nil, fmt.Errorf("pop"))
	_, err := nm.GetIdentityByIDWithVerifiers(nm.ctx, id.String())
	assert.Regexp(t, "pop", err)
}

func TestGetIdentityByIDBadUUID(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	_, err := nm.GetIdentityByID(nm.ctx, "bad")
	assert.Regexp(t, "FF00138", err)
}

func TestGetIdentityByIDWithVerifiers(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	id := fftypes.NewUUID()
	nm.database.(*databasemocks.Plugin).On("GetIdentityByID", nm.ctx, "ns1", id).
		Return(&core.Identity{IdentityBase: core.IdentityBase{ID: id, Type: core.IdentityTypeOrg, Namespace: "ns1"}}, nil)
	nm.database.(*databasemocks.Plugin).On("GetVerifiers", nm.ctx, "ns1", mock.Anything).Return([]*core.Verifier{
		{Hash: fftypes.NewRandB32(), VerifierRef: core.VerifierRef{
			Type:  core.VerifierTypeEthAddress,
			Value: "0x12345",
		}, Identity: id},
	}, nil, nil)
	identity, err := nm.GetIdentityByIDWithVerifiers(nm.ctx, id.String())
	assert.NoError(t, err)
	assert.Equal(t, "0x12345", identity.Verifiers[0].Value)
}

func TestGetIdentityByIDWithVerifiersFail(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	id := fftypes.NewUUID()
	nm.database.(*databasemocks.Plugin).On("GetIdentityByID", nm.ctx, "ns1", id).
		Return(&core.Identity{IdentityBase: core.IdentityBase{ID: id, Type: core.IdentityTypeOrg, Namespace: "ns1"}}, nil)
	nm.database.(*databasemocks.Plugin).On("GetVerifiers", nm.ctx, "ns1", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))
	_, err := nm.GetIdentityByIDWithVerifiers(nm.ctx, id.String())
	assert.Regexp(t, "pop", err)
}

func TestGetIdentities(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	nm.database.(*databasemocks.Plugin).On("GetIdentities", nm.ctx, "ns1", mock.Anything).Return([]*core.Identity{}, nil, nil)
	res, _, err := nm.GetIdentities(nm.ctx, database.IdentityQueryFactory.NewFilter(nm.ctx).And())
	assert.NoError(t, err)
	assert.Empty(t, res)
}

func TestGetIdentityVerifiers(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	id := fftypes.NewUUID()
	nm.database.(*databasemocks.Plugin).On("GetIdentityByID", nm.ctx, "ns1", id).
		Return(&core.Identity{IdentityBase: core.IdentityBase{ID: id, Type: core.IdentityTypeOrg, Namespace: "ns1"}}, nil)
	nm.database.(*databasemocks.Plugin).On("GetVerifiers", nm.ctx, "ns1", mock.Anything).Return([]*core.Verifier{}, nil, nil)
	res, _, err := nm.GetIdentityVerifiers(nm.ctx, id.String(), database.IdentityQueryFactory.NewFilter(nm.ctx).And())
	assert.NoError(t, err)
	assert.Empty(t, res)
}

func TestGetIdentityVerifiersIdentityFail(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	id := fftypes.NewUUID()
	nm.database.(*databasemocks.Plugin).On("GetIdentityByID", nm.ctx, "ns1", id).Return(nil, fmt.Errorf("pop"))
	res, _, err := nm.GetIdentityVerifiers(nm.ctx, id.String(), database.IdentityQueryFactory.NewFilter(nm.ctx).And())
	assert.Regexp(t, "pop", err)
	assert.Empty(t, res)
}

func TestGetVerifiers(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	nm.database.(*databasemocks.Plugin).On("GetVerifiers", nm.ctx, "ns1", mock.Anything).Return([]*core.Verifier{}, nil, nil)
	res, _, err := nm.GetVerifiers(nm.ctx, database.VerifierQueryFactory.NewFilter(nm.ctx).And())
	assert.NoError(t, err)
	assert.Empty(t, res)
}

func TestGetVerifierByHashOk(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	hash := fftypes.NewRandB32()
	nm.database.(*databasemocks.Plugin).On("GetVerifierByHash", nm.ctx, "ns1", hash).
		Return(&core.Verifier{Hash: hash, Namespace: "ns1"}, nil)
	res, err := nm.GetVerifierByHash(nm.ctx, hash.String())
	assert.NoError(t, err)
	assert.Equal(t, *hash, *res.Hash)
}

func TestGetVerifierByHashNotFound(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	hash := fftypes.NewRandB32()
	nm.database.(*databasemocks.Plugin).On("GetVerifierByHash", nm.ctx, "ns1", hash).Return(nil, nil)
	_, err := nm.GetVerifierByHash(nm.ctx, hash.String())
	assert.Regexp(t, "FF10109", err)
}

func TestGetVerifierByHashError(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	hash := fftypes.NewRandB32()
	nm.database.(*databasemocks.Plugin).On("GetVerifierByHash", nm.ctx, "ns1", hash).Return(nil, fmt.Errorf("pop"))
	_, err := nm.GetVerifierByHash(nm.ctx, hash.String())
	assert.Regexp(t, "pop", err)
}

func TestGetVerifierByHashBadUUID(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	_, err := nm.GetVerifierByHash(nm.ctx, "bad")
	assert.Regexp(t, "FF00107", err)
}

func TestGetVerifierByDIDOk(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	nm.identity.(*identitymanagermocks.Manager).On("CachedIdentityLookupMustExist", nm.ctx, "did:firefly:org/abc").
		Return(testOrg("abc"), true, nil)
	id, err := nm.GetIdentityByDID(nm.ctx, "did:firefly:org/abc")
	assert.NoError(t, err)
	assert.Equal(t, "did:firefly:org/abc", id.DID)
}

func TestGetVerifierByDIDWithVerifiersOk(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	nm.identity.(*identitymanagermocks.Manager).On("CachedIdentityLookupMustExist", nm.ctx, "did:firefly:org/abc").
		Return(testOrg("abc"), true, nil)
	nm.database.(*databasemocks.Plugin).On("GetVerifiers", nm.ctx, "ns1", mock.Anything).Return([]*core.Verifier{
		{Hash: fftypes.NewRandB32(), VerifierRef: core.VerifierRef{
			Type:  core.VerifierTypeEthAddress,
			Value: "0x12345",
		}},
	}, nil, nil)
	id, err := nm.GetIdentityByDIDWithVerifiers(nm.ctx, "did:firefly:org/abc")
	assert.NoError(t, err)
	assert.Equal(t, "did:firefly:org/abc", id.DID)
	assert.Equal(t, "0x12345", id.Verifiers[0].Value)
}

func TestGetVerifierByDIDWithVerifiersError(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	nm.identity.(*identitymanagermocks.Manager).On("CachedIdentityLookupMustExist", nm.ctx, "did:firefly:org/abc").
		Return(nil, true, fmt.Errorf("pop"))
	_, err := nm.GetIdentityByDIDWithVerifiers(nm.ctx, "did:firefly:org/abc")
	assert.Regexp(t, "pop", err)
}

func TestGetVerifierByDIDNotErr(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	nm.identity.(*identitymanagermocks.Manager).On("CachedIdentityLookupMustExist", nm.ctx, "did:firefly:org/abc").
		Return(nil, true, fmt.Errorf("pop"))
	id, err := nm.GetIdentityByDID(nm.ctx, "did:firefly:org/abc")
	assert.Regexp(t, "pop", err)
	assert.Nil(t, id)
}

func TestGetOrganizationsWithVerifiers(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	id1 := fftypes.NewUUID()
	nm.database.(*databasemocks.Plugin).On("GetIdentities", nm.ctx, "ns1", mock.Anything).Return([]*core.Identity{
		{IdentityBase: core.IdentityBase{
			ID: id1,
		}},
	}, nil, nil)
	nm.database.(*databasemocks.Plugin).On("GetVerifiers", nm.ctx, "ns1", mock.Anything).Return([]*core.Verifier{
		{Hash: fftypes.NewRandB32(), Identity: id1, VerifierRef: core.VerifierRef{
			Type:  core.VerifierTypeEthAddress,
			Value: "0x12345",
		}},
	}, nil, nil)
	res, _, err := nm.GetOrganizationsWithVerifiers(nm.ctx, database.IdentityQueryFactory.NewFilter(nm.ctx).And())
	assert.NoError(t, err)
	assert.Len(t, res, 1)
	assert.Equal(t, "0x12345", res[0].Verifiers[0].Value)
}

func TestGetOrganizationsWithVerifiersFailLookup(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	nm.database.(*databasemocks.Plugin).On("GetIdentities", nm.ctx, "ns1", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))
	_, _, err := nm.GetOrganizationsWithVerifiers(nm.ctx, database.IdentityQueryFactory.NewFilter(nm.ctx).And())
	assert.Regexp(t, "pop", err)
}

func TestGetIdentitiesWithVerifiersFailEnrich(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	id1 := fftypes.NewUUID()
	nm.database.(*databasemocks.Plugin).On("GetIdentities", nm.ctx, "ns1", mock.Anything).Return([]*core.Identity{
		{IdentityBase: core.IdentityBase{
			ID: id1,
		}},
	}, nil, nil)
	nm.database.(*databasemocks.Plugin).On("GetVerifiers", nm.ctx, "ns1", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))
	_, _, err := nm.GetOrganizationsWithVerifiers(nm.ctx, database.IdentityQueryFactory.NewFilter(nm.ctx).And())
	assert.Regexp(t, "pop", err)
}
