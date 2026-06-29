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

	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/internal/definitions"
	"github.com/hyperledger-firefly/firefly/internal/identity"
	"github.com/hyperledger-firefly/firefly/internal/multiparty"
	"github.com/hyperledger-firefly/firefly/internal/syncasync"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/hyperledger-firefly/firefly/pkg/dataexchange"
)

type Manager interface {
	RegisterOrganization(ctx context.Context, org *core.IdentityCreateDTO, waitConfirm bool) (identity *core.Identity, err error)
	RegisterNode(ctx context.Context, waitConfirm bool) (node *core.Identity, err error)
	RegisterNodeOrganization(ctx context.Context, waitConfirm bool) (org *core.Identity, err error)
	RegisterIdentity(ctx context.Context, dto *core.IdentityCreateDTO, waitConfirm bool) (identity *core.Identity, err error)
	UpdateIdentity(ctx context.Context, id string, dto *core.IdentityUpdateDTO, waitConfirm bool) (identity *core.Identity, err error)
	CheckNodeIdentityStatus(ctx context.Context) error

	GetOrganizationByNameOrID(ctx context.Context, nameOrID string) (*core.Identity, error)
	GetOrganizations(ctx context.Context, filter ffapi.AndFilter) ([]*core.Identity, *ffapi.FilterResult, error)
	GetOrganizationsWithVerifiers(ctx context.Context, filter ffapi.AndFilter) ([]*core.IdentityWithVerifiers, *ffapi.FilterResult, error)
	GetNodeByNameOrID(ctx context.Context, nameOrID string) (*core.Identity, error)
	GetNodes(ctx context.Context, filter ffapi.AndFilter) ([]*core.Identity, *ffapi.FilterResult, error)
	GetIdentityByID(ctx context.Context, id string) (*core.Identity, error)
	GetIdentityByIDWithVerifiers(ctx context.Context, id string) (*core.IdentityWithVerifiers, error)
	GetIdentityByDID(ctx context.Context, did string) (*core.Identity, error)
	GetIdentityByDIDWithVerifiers(ctx context.Context, did string) (*core.IdentityWithVerifiers, error)
	GetIdentities(ctx context.Context, filter ffapi.AndFilter) ([]*core.Identity, *ffapi.FilterResult, error)
	GetIdentitiesWithVerifiers(ctx context.Context, filter ffapi.AndFilter) ([]*core.IdentityWithVerifiers, *ffapi.FilterResult, error)
	GetIdentityVerifiers(ctx context.Context, id string, filter ffapi.AndFilter) ([]*core.Verifier, *ffapi.FilterResult, error)
	GetVerifiers(ctx context.Context, filter ffapi.AndFilter) ([]*core.Verifier, *ffapi.FilterResult, error)
	GetVerifierByHash(ctx context.Context, hash string) (*core.Verifier, error)
	GetDIDDocForIndentityByID(ctx context.Context, id string) (*DIDDocument, error)
	GetDIDDocForIndentityByDID(ctx context.Context, did string) (*DIDDocument, error)
}

type networkMap struct {
	ctx        context.Context
	namespace  string
	database   database.Plugin
	defsender  definitions.Sender
	exchange   dataexchange.Plugin // optional
	identity   identity.Manager
	syncasync  syncasync.Bridge
	multiparty multiparty.Manager // optional
}

func NewNetworkMap(ctx context.Context, ns string, di database.Plugin, dx dataexchange.Plugin, ds definitions.Sender, im identity.Manager, sa syncasync.Bridge, mm multiparty.Manager) (Manager, error) {
	if di == nil || ds == nil || im == nil {
		return nil, i18n.NewError(ctx, coremsgs.MsgInitializationNilDepError, "NetworkMap")
	}

	nm := &networkMap{
		ctx:        ctx,
		namespace:  ns,
		database:   di,
		defsender:  ds,
		exchange:   dx,
		identity:   im,
		syncasync:  sa,
		multiparty: mm,
	}
	return nm, nil
}
