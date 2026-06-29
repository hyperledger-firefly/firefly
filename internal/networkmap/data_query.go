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

package networkmap

import (
	"context"
	"database/sql/driver"

	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

func (nm *networkMap) GetOrganizationByNameOrID(ctx context.Context, nameOrID string) (org *core.Identity, err error) {
	u, err := fftypes.ParseUUID(ctx, nameOrID)
	if err != nil {
		if err := fftypes.ValidateFFNameField(ctx, nameOrID, "name"); err != nil {
			return nil, err
		}
		if org, err = nm.database.GetIdentityByName(ctx, core.IdentityTypeOrg, nm.namespace, nameOrID); err != nil {
			return nil, err
		}
	} else if org, err = nm.database.GetIdentityByID(ctx, nm.namespace, u); err != nil {
		return nil, err
	}
	if org == nil {
		return nil, i18n.NewError(ctx, coremsgs.Msg404NotFound)
	}
	if org.Type != core.IdentityTypeOrg {
		log.L(ctx).Warnf("Identity '%s' (%s) is not an org identity", org.DID, org.ID)
		return nil, nil
	}
	return org, nil
}

func (nm *networkMap) GetOrganizations(ctx context.Context, filter ffapi.AndFilter) ([]*core.Identity, *ffapi.FilterResult, error) {
	filter.Condition(filter.Builder().Eq("type", core.IdentityTypeOrg))
	return nm.GetIdentities(ctx, filter)
}

func (nm *networkMap) GetOrganizationsWithVerifiers(ctx context.Context, filter ffapi.AndFilter) ([]*core.IdentityWithVerifiers, *ffapi.FilterResult, error) {
	filter.Condition(filter.Builder().Eq("type", core.IdentityTypeOrg))
	return nm.GetIdentitiesWithVerifiers(ctx, filter)
}

func (nm *networkMap) GetNodeByNameOrID(ctx context.Context, nameOrID string) (node *core.Identity, err error) {
	u, err := fftypes.ParseUUID(ctx, nameOrID)
	if err != nil {
		if err := fftypes.ValidateFFNameField(ctx, nameOrID, "name"); err != nil {
			return nil, err
		}
		if node, err = nm.database.GetIdentityByName(ctx, core.IdentityTypeNode, nm.namespace, nameOrID); err != nil {
			return nil, err
		}
	} else if node, err = nm.database.GetIdentityByID(ctx, nm.namespace, u); err != nil {
		return nil, err
	}
	if node == nil {
		return nil, i18n.NewError(ctx, coremsgs.Msg404NotFound)
	}
	if node.Type != core.IdentityTypeNode {
		log.L(ctx).Warnf("Identity '%s' (%s) is not a node identity", node.DID, node.ID)
		return nil, nil
	}
	return node, nil
}

func (nm *networkMap) GetNodes(ctx context.Context, filter ffapi.AndFilter) ([]*core.Identity, *ffapi.FilterResult, error) {
	filter.Condition(filter.Builder().Eq("type", core.IdentityTypeNode))
	return nm.database.GetIdentities(ctx, nm.namespace, filter)
}

func (nm *networkMap) GetIdentityByID(ctx context.Context, id string) (*core.Identity, error) {
	u, err := fftypes.ParseUUID(ctx, id)
	if err != nil {
		return nil, err
	}
	identity, err := nm.database.GetIdentityByID(ctx, nm.namespace, u)
	if err != nil {
		return nil, err
	}
	if identity == nil {
		return nil, i18n.NewError(ctx, coremsgs.Msg404NotFound)
	}
	return identity, nil
}

func (nm *networkMap) withVerifiers(ctx context.Context, identity *core.Identity) (*core.IdentityWithVerifiers, error) {
	fb := database.VerifierQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("identity", identity.ID),
	)
	verifiers, _, err := nm.database.GetVerifiers(ctx, nm.namespace, filter)
	if err != nil {
		return nil, err
	}
	refs := make([]*core.VerifierRef, len(verifiers))
	for i, v := range verifiers {
		refs[i] = &v.VerifierRef
	}
	return &core.IdentityWithVerifiers{
		Identity:  *identity,
		Verifiers: refs,
	}, nil
}

func (nm *networkMap) GetIdentityByIDWithVerifiers(ctx context.Context, id string) (*core.IdentityWithVerifiers, error) {
	identity, err := nm.GetIdentityByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return nm.withVerifiers(ctx, identity)
}

func (nm *networkMap) GetIdentityByDID(ctx context.Context, did string) (*core.Identity, error) {
	identity, _, err := nm.identity.CachedIdentityLookupMustExist(ctx, did)
	if err != nil {
		return nil, err
	}
	return identity, nil
}

func (nm *networkMap) GetIdentityByDIDWithVerifiers(ctx context.Context, did string) (*core.IdentityWithVerifiers, error) {
	identity, _, err := nm.identity.CachedIdentityLookupMustExist(ctx, did)
	if err != nil {
		return nil, err
	}
	return nm.withVerifiers(ctx, identity)
}

func (nm *networkMap) GetIdentities(ctx context.Context, filter ffapi.AndFilter) ([]*core.Identity, *ffapi.FilterResult, error) {
	return nm.database.GetIdentities(ctx, nm.namespace, filter)
}

func (nm *networkMap) GetIdentitiesWithVerifiers(ctx context.Context, filter ffapi.AndFilter) ([]*core.IdentityWithVerifiers, *ffapi.FilterResult, error) {
	identities, res, err := nm.database.GetIdentities(ctx, nm.namespace, filter)
	if err != nil {
		return nil, nil, err
	}
	iids := make([]driver.Value, len(identities))
	for idx, identity := range identities {
		iids[idx] = identity.ID
	}
	fb := database.VerifierQueryFactory.NewFilter(ctx)
	verifierFilter := fb.And(
		fb.In("identity", iids),
	)
	idsWithVerifiers := make([]*core.IdentityWithVerifiers, len(identities))
	verifiers, _, err := nm.database.GetVerifiers(ctx, nm.namespace, verifierFilter)
	if err != nil {
		return nil, nil, err
	}
	for idx, identity := range identities {
		idsWithVerifiers[idx] = &core.IdentityWithVerifiers{
			Identity:  *identity,
			Verifiers: make([]*core.VerifierRef, 0, 1),
		}
		for _, verifier := range verifiers {
			if verifier.Identity.Equals(identity.ID) {
				idsWithVerifiers[idx].Verifiers = append(idsWithVerifiers[idx].Verifiers, &verifier.VerifierRef)
			}
		}
	}
	return idsWithVerifiers, res, err
}

func (nm *networkMap) GetIdentityVerifiers(ctx context.Context, id string, filter ffapi.AndFilter) ([]*core.Verifier, *ffapi.FilterResult, error) {
	identity, err := nm.GetIdentityByID(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	filter.Condition(filter.Builder().Eq("identity", identity.ID))
	return nm.database.GetVerifiers(ctx, nm.namespace, filter)
}

func (nm *networkMap) GetDIDDocForIndentityByID(ctx context.Context, id string) (*DIDDocument, error) {
	identity, err := nm.GetIdentityByID(ctx, id)
	if err != nil {
		return nil, err
	}
	return nm.generateDIDDocument(ctx, identity)
}

func (nm *networkMap) GetDIDDocForIndentityByDID(ctx context.Context, did string) (*DIDDocument, error) {
	identity, err := nm.GetIdentityByDID(ctx, did)
	if err != nil {
		return nil, err
	}
	return nm.generateDIDDocument(ctx, identity)
}

func (nm *networkMap) GetVerifierByHash(ctx context.Context, hash string) (*core.Verifier, error) {
	b32, err := fftypes.ParseBytes32(ctx, hash)
	if err != nil {
		return nil, err
	}
	verifier, err := nm.database.GetVerifierByHash(ctx, nm.namespace, b32)
	if err != nil {
		return nil, err
	}
	if verifier == nil {
		return nil, i18n.NewError(ctx, coremsgs.Msg404NotFound)
	}
	return verifier, nil
}

func (nm *networkMap) GetVerifiers(ctx context.Context, filter ffapi.AndFilter) ([]*core.Verifier, *ffapi.FilterResult, error) {
	return nm.database.GetVerifiers(ctx, nm.namespace, filter)
}
