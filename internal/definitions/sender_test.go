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

package definitions

import (
	"context"
	"fmt"
	"testing"

	"github.com/hyperledger/firefly/mocks/assetmocks"
	"github.com/hyperledger/firefly/mocks/blockchainmocks"
	"github.com/hyperledger/firefly/mocks/broadcastmocks"
	"github.com/hyperledger/firefly/mocks/contractmocks"
	"github.com/hyperledger/firefly/mocks/databasemocks"
	"github.com/hyperledger/firefly/mocks/dataexchangemocks"
	"github.com/hyperledger/firefly/mocks/datamocks"
	"github.com/hyperledger/firefly/mocks/identitymanagermocks"
	"github.com/hyperledger/firefly/mocks/syncasyncmocks"
	"github.com/hyperledger/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type testDefinitionSender struct {
	definitionSender

	cancel func()
	mdi    *databasemocks.Plugin
	mbi    *blockchainmocks.Plugin
	mdx    *dataexchangemocks.Plugin
	mbm    *broadcastmocks.Manager
	mim    *identitymanagermocks.Manager
	mdm    *datamocks.Manager
	mam    *assetmocks.Manager
	mcm    *contractmocks.Manager
}

func (tds *testDefinitionSender) cleanup(t *testing.T) {
	tds.cancel()
	tds.mdi.AssertExpectations(t)
	tds.mbi.AssertExpectations(t)
	tds.mdx.AssertExpectations(t)
	tds.mbm.AssertExpectations(t)
	tds.mim.AssertExpectations(t)
	tds.mdm.AssertExpectations(t)
	tds.mam.AssertExpectations(t)
	tds.mcm.AssertExpectations(t)
}

func newTestDefinitionSender(t *testing.T) *testDefinitionSender {
	mdi := &databasemocks.Plugin{}
	mbi := &blockchainmocks.Plugin{}
	mdx := &dataexchangemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	mim := &identitymanagermocks.Manager{}
	mdm := &datamocks.Manager{}
	mam := &assetmocks.Manager{}
	mcm := &contractmocks.Manager{}
	tokenBroadcastNames := make(map[string]string)
	tokenBroadcastNames["connector1"] = "remote1"

	ctx, cancel := context.WithCancel(context.Background())
	ns := &core.Namespace{Name: "ns1", NetworkName: "ns1"}
	ds, _, err := NewDefinitionSender(ctx, ns, false, mdi, mbi, mdx, mbm, mim, mdm, mam, mcm, tokenBroadcastNames)
	assert.NoError(t, err)

	return &testDefinitionSender{
		definitionSender: *ds.(*definitionSender),
		cancel:           cancel,
		mdi:              mdi,
		mbi:              mbi,
		mdx:              mdx,
		mbm:              mbm,
		mim:              mim,
		mdm:              mdm,
		mam:              mam,
		mcm:              mcm,
	}
}

func mockRunAsGroupPassthrough(mdi *databasemocks.Plugin) {
	rag := mdi.On("RunAsGroup", mock.Anything, mock.Anything)
	rag.RunFn = func(a mock.Arguments) {
		fn := a[1].(func(context.Context) error)
		rag.ReturnArguments = mock.Arguments{fn(a[0].(context.Context))}
	}
}

func TestInitSenderFail(t *testing.T) {
	_, _, err := NewDefinitionSender(context.Background(), &core.Namespace{}, false, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	assert.Regexp(t, "FF10128", err)
}

func TestNewDefinitionSenderHandlerThrows(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mbi := &blockchainmocks.Plugin{}
	mdx := &dataexchangemocks.Plugin{}
	mbm := &broadcastmocks.Manager{}
	mim := &identitymanagermocks.Manager{}
	mdm := &datamocks.Manager{}
	mcm := &contractmocks.Manager{}

	tokenBroadcastNames := make(map[string]string)
	tokenBroadcastNames["connector1"] = "remote1"

	ctx := context.Background()
	ns := &core.Namespace{Name: "ns1", NetworkName: "ns1"}
	ds, dh, err := NewDefinitionSender(ctx, ns, false, mdi, mbi, mdx, mbm, mim, mdm, nil, mcm, tokenBroadcastNames)
	assert.Nil(t, ds)
	assert.Nil(t, dh)
	assert.NotNil(t, err)
}

func TestName(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	assert.Equal(t, "DefinitionSender", ds.Name())
}

func TestCreateDefinitionConfirm(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)

	mim := ds.identity.(*identitymanagermocks.Manager)
	mbm := ds.broadcast.(*broadcastmocks.Manager)
	mms := &syncasyncmocks.Sender{}

	mim.On("GetRootOrg", ds.ctx).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			DID: "firefly:org1",
		},
	}, nil)
	mim.On("ResolveInputSigningIdentity", mock.Anything, mock.Anything).Return(nil)
	mbm.On("NewBroadcast", mock.Anything).Return(mms)
	mms.On("SendAndWait", mock.Anything).Return(nil)

	ds.multiparty = true
	_, err := ds.getSenderDefault(ds.ctx, &core.Datatype{}, core.SystemTagDefineDatatype, nil).send(ds.ctx, true)
	assert.NoError(t, err)

	mim.AssertExpectations(t)
	mbm.AssertExpectations(t)
	mms.AssertExpectations(t)
}

func TestCreateDatatypeDefinitionAsNodeConfirm(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)

	mim := ds.identity.(*identitymanagermocks.Manager)
	mbm := ds.broadcast.(*broadcastmocks.Manager)
	mms := &syncasyncmocks.Sender{}

	mim.On("GetRootOrg", ds.ctx).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			DID: "firefly:org1",
		},
	}, nil)
	mim.On("ResolveInputSigningIdentity", mock.Anything, mock.Anything).Return(nil)
	mbm.On("NewBroadcast", mock.Anything).Return(mms)
	mms.On("SendAndWait", mock.Anything).Return(nil)

	ds.multiparty = true

	_, err := ds.getSenderDefault(ds.ctx, &core.Datatype{}, core.SystemTagDefineDatatype, nil).send(ds.ctx, true)
	assert.NoError(t, err)

	mim.AssertExpectations(t)
	mbm.AssertExpectations(t)
	mms.AssertExpectations(t)
}

func TestCreateDefinitionBadIdentity(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)

	ds.multiparty = true

	mim := ds.identity.(*identitymanagermocks.Manager)
	mim.On("ResolveInputSigningIdentity", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
	_, err := ds.getSender(ds.ctx, &core.Datatype{}, &core.SignerRef{
		Author: "wrong",
		Key:    "wrong",
	}, core.SystemTagDefineDatatype, nil).send(ds.ctx, false)
	assert.Regexp(t, "pop", err)
}
