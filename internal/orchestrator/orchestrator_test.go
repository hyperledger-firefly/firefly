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

package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hyperledger-firefly/common/mocks/authmocks"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/internal/cache"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/identity"
	"github.com/hyperledger-firefly/firefly/mocks/assetmocks"
	"github.com/hyperledger-firefly/firefly/mocks/batchmocks"
	"github.com/hyperledger-firefly/firefly/mocks/blockchainmocks"
	"github.com/hyperledger-firefly/firefly/mocks/broadcastmocks"
	"github.com/hyperledger-firefly/firefly/mocks/cachemocks"
	"github.com/hyperledger-firefly/firefly/mocks/contractmocks"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/mocks/dataexchangemocks"
	"github.com/hyperledger-firefly/firefly/mocks/datamocks"
	"github.com/hyperledger-firefly/firefly/mocks/definitionsmocks"
	"github.com/hyperledger-firefly/firefly/mocks/eventmocks"
	"github.com/hyperledger-firefly/firefly/mocks/identitymanagermocks"
	"github.com/hyperledger-firefly/firefly/mocks/identitymocks"
	"github.com/hyperledger-firefly/firefly/mocks/metricsmocks"
	"github.com/hyperledger-firefly/firefly/mocks/multipartymocks"
	"github.com/hyperledger-firefly/firefly/mocks/networkmapmocks"
	"github.com/hyperledger-firefly/firefly/mocks/operationmocks"
	"github.com/hyperledger-firefly/firefly/mocks/privatemessagingmocks"
	"github.com/hyperledger-firefly/firefly/mocks/shareddownloadmocks"
	"github.com/hyperledger-firefly/firefly/mocks/sharedstoragemocks"
	"github.com/hyperledger-firefly/firefly/mocks/spieventsmocks"
	"github.com/hyperledger-firefly/firefly/mocks/tokenmocks"
	"github.com/hyperledger-firefly/firefly/mocks/txcommonmocks"
	"github.com/hyperledger-firefly/firefly/mocks/txwritermocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/hyperledger-firefly/firefly/pkg/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const configDir = "../../test/data/config"

type testOrchestrator struct {
	orchestrator

	mdi *databasemocks.Plugin
	mdm *datamocks.Manager
	mbm *broadcastmocks.Manager
	mba *batchmocks.Manager
	mem *eventmocks.EventManager
	mnm *networkmapmocks.Manager
	mps *sharedstoragemocks.Plugin
	mpm *privatemessagingmocks.Manager
	mbi *blockchainmocks.Plugin
	mii *identitymocks.Plugin
	mim *identitymanagermocks.Manager
	mdx *dataexchangemocks.Plugin
	mam *assetmocks.Manager
	mti *tokenmocks.Plugin
	mcm *contractmocks.Manager
	mmi *metricsmocks.Manager
	cmi *cachemocks.Manager
	mom *operationmocks.Manager
	mth *txcommonmocks.Helper
	msd *shareddownloadmocks.Manager
	mae *spieventsmocks.Manager
	mdh *definitionsmocks.Handler
	mmp *multipartymocks.Manager
	mds *definitionsmocks.Sender
	mtw *txwritermocks.Writer
}

func (tor *testOrchestrator) cleanup(t *testing.T) {
	tor.mdi.AssertExpectations(t)
	tor.mdm.AssertExpectations(t)
	tor.mbm.AssertExpectations(t)
	tor.mba.AssertExpectations(t)
	tor.mem.AssertExpectations(t)
	tor.mnm.AssertExpectations(t)
	tor.mps.AssertExpectations(t)
	tor.mpm.AssertExpectations(t)
	tor.mbi.AssertExpectations(t)
	tor.mii.AssertExpectations(t)
	tor.mim.AssertExpectations(t)
	tor.mdx.AssertExpectations(t)
	tor.mam.AssertExpectations(t)
	tor.mti.AssertExpectations(t)
	tor.mcm.AssertExpectations(t)
	tor.mmi.AssertExpectations(t)
	tor.cmi.AssertExpectations(t)
	tor.mom.AssertExpectations(t)
	tor.mth.AssertExpectations(t)
	tor.msd.AssertExpectations(t)
	tor.mae.AssertExpectations(t)
	tor.mdh.AssertExpectations(t)
	tor.mmp.AssertExpectations(t)
}

func newTestOrchestrator() *testOrchestrator {
	coreconfig.Reset()
	ctx, cancel := context.WithCancel(context.Background())
	tor := &testOrchestrator{
		orchestrator: orchestrator{
			ctx:       ctx,
			cancelCtx: cancel,
			namespace: &core.Namespace{Name: "ns", NetworkName: "ns"},
		},
		mdi: &databasemocks.Plugin{},
		mdm: &datamocks.Manager{},
		mbm: &broadcastmocks.Manager{},
		mba: &batchmocks.Manager{},
		mem: &eventmocks.EventManager{},
		mnm: &networkmapmocks.Manager{},
		mps: &sharedstoragemocks.Plugin{},
		mpm: &privatemessagingmocks.Manager{},
		mbi: &blockchainmocks.Plugin{},
		mii: &identitymocks.Plugin{},
		mim: &identitymanagermocks.Manager{},
		mdx: &dataexchangemocks.Plugin{},
		mam: &assetmocks.Manager{},
		mti: &tokenmocks.Plugin{},
		mcm: &contractmocks.Manager{},
		mmi: &metricsmocks.Manager{},
		cmi: &cachemocks.Manager{},
		mom: &operationmocks.Manager{},
		mth: &txcommonmocks.Helper{},
		msd: &shareddownloadmocks.Manager{},
		mae: &spieventsmocks.Manager{},
		mdh: &definitionsmocks.Handler{},
		mmp: &multipartymocks.Manager{},
		mds: &definitionsmocks.Sender{},
		mtw: &txwritermocks.Writer{},
	}
	tor.orchestrator.multiparty = tor.mmp
	tor.orchestrator.data = tor.mdm
	tor.orchestrator.batch = tor.mba
	tor.orchestrator.broadcast = tor.mbm
	tor.orchestrator.events = tor.mem
	tor.orchestrator.networkmap = tor.mnm
	tor.orchestrator.messaging = tor.mpm
	tor.orchestrator.identity = tor.mim
	tor.orchestrator.assets = tor.mam
	tor.orchestrator.contracts = tor.mcm
	tor.orchestrator.metrics = tor.mmi
	tor.orchestrator.cacheManager = tor.cmi
	tor.orchestrator.operations = tor.mom
	tor.orchestrator.sharedDownload = tor.msd
	tor.orchestrator.txHelper = tor.mth
	tor.orchestrator.txWriter = tor.mtw
	tor.orchestrator.defhandler = tor.mdh
	tor.orchestrator.defsender = tor.mds
	tor.orchestrator.config.Multiparty.Enabled = true
	tor.orchestrator.config.MaxHistoricalEventScanLimit = 1000
	tor.orchestrator.plugins = &Plugins{
		Blockchain: BlockchainPlugin{
			Plugin: tor.mbi,
		},
		SharedStorage: SharedStoragePlugin{
			Plugin: tor.mps,
		},
		DataExchange: DataExchangePlugin{
			Plugin: tor.mdx,
		},
		Database: DatabasePlugin{
			Plugin: tor.mdi,
		},
		Tokens: []TokensPlugin{{
			Name:   "token",
			Plugin: tor.mti,
		}},
	}
	tor.mdi.On("Name").Return("mock-di").Maybe()
	tor.mem.On("Name").Return("mock-ei").Maybe()
	tor.mps.On("Name").Return("mock-ps").Maybe()
	tor.mbi.On("Name").Return("mock-bi").Maybe()
	tor.mii.On("Name").Return("mock-ii").Maybe()
	tor.mdx.On("Name").Return("mock-dx").Maybe()
	tor.mam.On("Name").Return("mock-am").Maybe()
	tor.mti.On("Name").Return("mock-tk").Maybe()
	tor.mcm.On("Name").Return("mock-cm").Maybe()
	tor.mmi.On("Name").Return("mock-mm").Maybe()
	tor.mmp.On("Name").Return("mock-mp").Maybe()
	tor.mem.On("ResolveTransportAndCapabilities", mock.Anything, mock.Anything).Return("websockets", &events.Capabilities{}, nil).Maybe()
	tor.mds.On("Init", mock.Anything).Maybe()
	tor.cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(tor.ctx, 100, 5*time.Minute), nil).Maybe()
	return tor
}

func TestNewOrchestrator(t *testing.T) {
	or := NewOrchestrator(
		&core.Namespace{Name: "ns1", NetworkName: "ns1"},
		Config{},
		&Plugins{},
		&metricsmocks.Manager{},
		&cachemocks.Manager{},
	)
	assert.NotNil(t, or)
}

func TestInitOK(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	or.namespace.NetworkName = "ns2"
	or.config.Multiparty.Node.Name = "node1"
	node := &core.Identity{
		IdentityBase: core.IdentityBase{
			Name: "node1",
		},
	}
	or.mdi.On("SetHandler", "ns", mock.Anything).Return()
	or.mbi.On("SetHandler", "ns", mock.Anything).Return()
	or.mbi.On("SetOperationHandler", "ns", mock.Anything).Return()
	or.mbi.On("StartNamespace", mock.Anything, "ns").Return(nil)
	or.mdi.On("GetIdentities", mock.Anything, "ns", mock.Anything).Return([]*core.Identity{node}, nil, nil)
	or.mdx.On("SetHandler", "ns2", "node1", mock.Anything).Return()
	or.mdx.On("SetOperationHandler", "ns", mock.Anything).Return()
	or.mdx.On("AddNode", mock.Anything, "ns2", "node1", mock.Anything).Return(nil)
	or.mps.On("SetHandler", "ns", mock.Anything).Return()
	or.mti.On("SetHandler", "ns", mock.Anything).Return(nil)
	or.mti.On("SetOperationHandler", "ns", mock.Anything).Return()
	or.mmp.On("ConfigureContract", mock.Anything, mock.Anything).Return(nil)
	or.PreInit(or.ctx, or.cancelCtx)
	err := or.Init()
	assert.NoError(t, err)

	assert.Equal(t, or.mba, or.BatchManager())
	assert.Equal(t, or.mbm, or.Broadcast())
	assert.Equal(t, or.mpm, or.PrivateMessaging())
	assert.Equal(t, or.mds, or.DefinitionSender())
	assert.Equal(t, or.mem, or.Events())
	assert.Equal(t, or.mam, or.Assets())
	assert.Equal(t, or.mdm, or.Data())
	assert.Equal(t, or.mom, or.Operations())
	assert.Equal(t, or.mcm, or.Contracts())
	assert.Equal(t, or.mnm, or.NetworkMap())
	assert.Equal(t, or.mmp, or.MultiParty())
	assert.Equal(t, or.identity, or.Identity())
}

func TestCacheInitFail(t *testing.T) {
	or := newTestOrchestrator()
	cacheInitError := errors.New("Initialization error.")
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(nil, cacheInitError)
	or.txHelper = nil
	or.cacheManager = cmi
	or.mmp.On("ConfigureContract", mock.Anything, mock.Anything).Return(nil)
	err := or.initManagers(context.Background())
	assert.Equal(t, cacheInitError, err)
}

func TestInitDataexchangeLookupNodesFail(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	or.mdi.On("GetIdentities", mock.Anything, "ns", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))
	ctx := context.Background()
	err := or.initMultiParty(ctx)
	assert.EqualError(t, err, "pop")
}

func TestInitDataexchangeAddNodesFail(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	node := &core.Identity{
		IdentityBase: core.IdentityBase{
			Name: "node1",
		},
	}
	or.mdi.On("GetIdentities", mock.Anything, "ns", mock.Anything).Return([]*core.Identity{node}, nil, nil)
	or.mdx.On("AddNode", mock.Anything, "ns", "node1", mock.Anything).Return(fmt.Errorf("pop"))
	ctx := context.Background()
	err := or.initMultiParty(ctx)
	assert.EqualError(t, err, "pop")
}

func TestInitMessagingComponentFail(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	or.plugins.Database.Plugin = nil
	or.messaging = nil
	or.mbi.On("StartNamespace", mock.Anything, "ns").Return(nil)
	or.mmp.On("ConfigureContract", mock.Anything, mock.Anything).Return(nil)
	err := or.initComponents(context.Background())
	assert.Regexp(t, "FF10128", err)
}

func TestInitEventsComponentFail(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	or.plugins.Database.Plugin = nil
	or.events = nil
	or.mbi.On("StartNamespace", mock.Anything, "ns").Return(nil)
	or.mmp.On("ConfigureContract", mock.Anything, mock.Anything).Return(nil)
	err := or.initComponents(context.Background())
	assert.Regexp(t, "FF10128", err)
}

func TestInitEventsComponentStartNamespaceFail(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	or.plugins.Database.Plugin = nil
	or.events = nil
	or.mbi.On("StartNamespace", mock.Anything, "ns").Return(fmt.Errorf("pop"))
	err := or.initComponents(context.Background())
	assert.Regexp(t, "pop", err)
}

func TestInitNetworkMapComponentFail(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	or.plugins.Database.Plugin = nil
	or.networkmap = nil
	or.mbi.On("StartNamespace", mock.Anything, "ns").Return(nil)
	or.mmp.On("ConfigureContract", mock.Anything, mock.Anything).Return(nil)
	err := or.initComponents(context.Background())
	assert.Regexp(t, "FF10128", err)
}

func TestInitMultipartyComponentFail(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	or.mbi.On("StartNamespace", mock.Anything, "ns").Return(nil)
	or.plugins.Database.Plugin = nil
	or.multiparty = nil
	err := or.initComponents(context.Background())
	assert.Regexp(t, "FF10128", err)
}

func TestInitMultipartyComponentConfigureFail(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	or.mbi.On("StartNamespace", mock.Anything, "ns").Return(nil)
	or.mmp.On("ConfigureContract", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
	err := or.initComponents(context.Background())
	assert.EqualError(t, err, "pop")
}

func TestInitSharedStorageDownloadComponentFail(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	or.plugins.Database.Plugin = nil
	or.sharedDownload = nil
	or.mbi.On("StartNamespace", mock.Anything, "ns").Return(nil)
	or.mmp.On("ConfigureContract", mock.Anything, mock.Anything).Return(nil)
	err := or.initComponents(context.Background())
	assert.Regexp(t, "FF10128", err)
}

func TestInitBatchComponentFail(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	or.plugins.Database.Plugin = nil
	or.batch = nil
	or.mbi.On("StartNamespace", mock.Anything, "ns").Return(nil)
	or.mmp.On("ConfigureContract", mock.Anything, mock.Anything).Return(nil)
	err := or.initComponents(context.Background())
	assert.Regexp(t, "FF10128", err)
}

func TestInitBroadcastComponentFail(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	or.plugins.Database.Plugin = nil
	or.broadcast = nil
	or.mbi.On("StartNamespace", mock.Anything, "ns").Return(nil)
	or.mmp.On("ConfigureContract", mock.Anything, mock.Anything).Return(nil)
	err := or.initComponents(context.Background())
	assert.Regexp(t, "FF10128", err)
}

func TestInitDefSenderComponentFail(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	or.data = nil
	or.defsender = nil
	or.mmp.On("ConfigureContract", mock.Anything, mock.Anything).Return(nil)
	err := or.initManagers(context.Background())
	assert.Regexp(t, "FF10128", err)
}

func TestInitDataComponentFail(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	or.plugins.Database.Plugin = nil
	or.data = nil
	or.mbi.On("StartNamespace", mock.Anything, "ns").Return(nil)
	err := or.initComponents(context.Background())
	assert.Regexp(t, "FF10128", err)
}

func TestInitIdentityComponentFail(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	or.plugins.Database.Plugin = nil
	or.identity = nil
	or.mbi.On("StartNamespace", mock.Anything, "ns").Return(nil)
	or.mmp.On("ConfigureContract", mock.Anything, mock.Anything).Return(nil)
	err := or.initComponents(context.Background())
	assert.Regexp(t, "FF10128", err)
}

func TestInitAssetsComponentFail(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	or.plugins.Database.Plugin = nil
	or.assets = nil
	or.mbi.On("StartNamespace", mock.Anything, "ns").Return(nil)
	or.mmp.On("ConfigureContract", mock.Anything, mock.Anything).Return(nil)
	err := or.initComponents(context.Background())
	assert.Regexp(t, "FF10128", err)
}

func TestInitContractsComponentFail(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	or.plugins.Database.Plugin = nil
	or.contracts = nil
	or.mbi.On("StartNamespace", mock.Anything, "ns").Return(nil)
	or.mmp.On("ConfigureContract", mock.Anything, mock.Anything).Return(nil)
	err := or.initComponents(context.Background())
	assert.Regexp(t, "FF10128", err)
}

func TestInitOperationsComponentFail(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	or.plugins.Database.Plugin = nil
	or.operations = nil
	or.txHelper = nil
	or.mbi.On("StartNamespace", mock.Anything, "ns").Return(nil)
	err := or.initComponents(context.Background())
	assert.Regexp(t, "FF10128", err)
}

func TestStartBatchFail(t *testing.T) {
	coreconfig.Reset()
	or := newTestOrchestrator()
	defer or.cleanup(t)
	or.mdm.On("Start").Return(nil)
	or.mba.On("Start").Return(fmt.Errorf("pop"))
	or.mnm.On("CheckNodeIdentityStatus", or.ctx).Return(nil)
	err := or.Start()
	assert.EqualError(t, err, "pop")
}

func TestInitTXWriter(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	or.txWriter = nil
	or.config.Multiparty.Enabled = false
	or.mdi.On("Capabilities").Return(&database.Capabilities{Concurrency: false})
	err := or.initManagers(context.Background())
	assert.NoError(t, err)
}

func TestStartStopOk(t *testing.T) {
	coreconfig.Reset()
	or := newTestOrchestrator()
	defer or.cleanup(t)
	or.mdm.On("Start").Return(nil)
	or.mba.On("Start").Return(nil)
	or.mem.On("Start").Return(nil)
	or.mbm.On("Start").Return(nil)
	or.msd.On("Start").Return(nil)
	or.mnm.On("CheckNodeIdentityStatus", or.ctx).Return(nil)
	or.mom.On("Start").Return(nil)
	or.mtw.On("Start").Return()
	or.mam.On("Start").Return(nil)
	or.mba.On("WaitStop").Return(nil)
	or.mbm.On("WaitStop").Return(nil)
	or.mdm.On("WaitStop").Return(nil)
	or.msd.On("WaitStop").Return(nil)
	or.mom.On("WaitStop").Return(nil)
	or.mem.On("WaitStop").Return(nil)
	or.mtw.On("Close").Return(nil)
	or.mbi.On("StopNamespace", mock.Anything, "ns").Return(nil)
	or.mti.On("StopNamespace", mock.Anything, "ns").Return(nil)
	err := or.Start()
	assert.NoError(t, err)
	or.WaitStop()
	or.WaitStop() // swallows dups

	or = newTestOrchestrator()
	or.mdm.On("Start").Return(nil)
	or.mba.On("Start").Return(nil)
	or.mem.On("Start").Return(nil)
	or.mbm.On("Start").Return(nil)
	or.msd.On("Start").Return(nil)
	or.mnm.On("CheckNodeIdentityStatus", or.ctx).Return(errors.New("benign error"))
	or.mom.On("Start").Return(nil)
	or.mtw.On("Start").Return()
	or.mam.On("Start").Return(nil)
	or.mba.On("WaitStop").Return(nil)
	or.mbm.On("WaitStop").Return(nil)
	or.mdm.On("WaitStop").Return(nil)
	or.msd.On("WaitStop").Return(nil)
	or.mom.On("WaitStop").Return(nil)
	or.mem.On("WaitStop").Return(nil)
	or.mtw.On("Close").Return(nil)
	or.mbi.On("StopNamespace", mock.Anything, "ns").Return(fmt.Errorf("pop"))
	or.mti.On("StopNamespace", mock.Anything, "ns").Return(fmt.Errorf("pop"))
	err = or.Start()
	assert.NoError(t, err)
	or.WaitStop()
	or.WaitStop() // swallows dups
}

func TestPurge(t *testing.T) {
	coreconfig.Reset()
	or := newTestOrchestrator()
	defer or.cleanup(t)
	// Note additional testing of this happens in namespace manager
	or.mdi.On("SetHandler", mock.Anything, mock.Anything).Return(nil)
	or.mbi.On("SetHandler", mock.Anything, mock.Anything).Return(nil)
	or.mbi.On("SetOperationHandler", mock.Anything, mock.Anything).Return(nil)
	// or.mbi.On("StopNamespace", mock.Anything, mock.Anything).Return(nil)
	or.mps.On("SetHandler", mock.Anything, mock.Anything).Return(nil)
	or.mdx.On("SetHandler", mock.Anything, "Test1", mock.Anything).Return(nil)
	or.mdx.On("SetOperationHandler", mock.Anything, mock.Anything).Return(nil)
	or.mti.On("SetHandler", mock.Anything, mock.Anything).Return(nil)
	or.mti.On("SetOperationHandler", mock.Anything, mock.Anything).Return(nil)
	// or.mti.On("StopNamespace", mock.Anything, "ns").Return(nil)
	Purge(context.Background(), or.namespace, or.plugins, "Test1")
}

func TestPurgeBlockchainError(t *testing.T) {
	coreconfig.Reset()
	or := newTestOrchestrator()
	defer or.cleanup(t)
	// Note additional testing of this happens in namespace manager
	or.mdi.On("SetHandler", mock.Anything, mock.Anything).Return(nil)
	or.mbi.On("SetHandler", mock.Anything, mock.Anything).Return(nil)
	or.mbi.On("SetOperationHandler", mock.Anything, mock.Anything).Return(nil)
	or.mps.On("SetHandler", mock.Anything, mock.Anything).Return(nil)
	or.mdx.On("SetHandler", mock.Anything, "Test1", mock.Anything).Return(nil)
	or.mdx.On("SetOperationHandler", mock.Anything, mock.Anything).Return(nil)
	or.mti.On("SetHandler", mock.Anything, mock.Anything).Return(nil)
	or.mti.On("SetOperationHandler", mock.Anything, mock.Anything).Return(nil)
	Purge(context.Background(), or.namespace, or.plugins, "Test1")
}

func TestPurgeTokenError(t *testing.T) {
	coreconfig.Reset()
	or := newTestOrchestrator()
	defer or.cleanup(t)
	// Note additional testing of this happens in namespace manager
	or.mdi.On("SetHandler", mock.Anything, mock.Anything).Return(nil)
	or.mbi.On("SetHandler", mock.Anything, mock.Anything).Return(nil)
	or.mbi.On("SetOperationHandler", mock.Anything, mock.Anything).Return(nil)
	or.mps.On("SetHandler", mock.Anything, mock.Anything).Return(nil)
	or.mdx.On("SetHandler", mock.Anything, "Test1", mock.Anything).Return(nil)
	or.mdx.On("SetOperationHandler", mock.Anything, mock.Anything).Return(nil)
	or.mti.On("SetHandler", mock.Anything, mock.Anything).Return(nil)
	or.mti.On("SetOperationHandler", mock.Anything, mock.Anything).Return(nil)
	Purge(context.Background(), or.namespace, or.plugins, "Test1")
}

func TestNetworkAction(t *testing.T) {
	or := newTestOrchestrator()
	or.namespace.Name = core.LegacySystemNamespace
	action := &core.NetworkAction{Type: core.NetworkActionTerminate}
	or.mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("0x123", nil)
	or.mmp.On("SubmitNetworkAction", context.Background(), "0x123", action, false).Return(nil)
	err := or.SubmitNetworkAction(context.Background(), action)
	assert.NoError(t, err)
}

func TestNetworkActionBadKey(t *testing.T) {
	or := newTestOrchestrator()
	or.namespace.Name = core.LegacySystemNamespace
	action := &core.NetworkAction{Type: core.NetworkActionTerminate}
	or.mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("", fmt.Errorf("pop"))
	err := or.SubmitNetworkAction(context.Background(), action)
	assert.EqualError(t, err, "pop")
}

func TestNetworkActionNonMultiparty(t *testing.T) {
	or := newTestOrchestrator()
	or.multiparty = nil
	err := or.SubmitNetworkAction(context.Background(), &core.NetworkAction{Type: core.NetworkActionTerminate})
	assert.Regexp(t, "FF10414", err)
}

func TestAuthorize(t *testing.T) {
	or := newTestOrchestrator()
	auth := &authmocks.Plugin{}
	auth.On("Authorize", mock.Anything, mock.Anything).Return(nil)
	or.plugins.Auth.Plugin = auth
	err := or.Authorize(context.Background(), &fftypes.AuthReq{})
	assert.NoError(t, err)
}

func TestAuthorizeNoPlugin(t *testing.T) {
	or := newTestOrchestrator()
	err := or.Authorize(context.Background(), &fftypes.AuthReq{})
	assert.NoError(t, err)
}

func TestRewindPinsSeq(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	rewind := &core.PinRewind{Sequence: 100}
	batchID := fftypes.NewUUID()

	or.mdi.On("GetPins", mock.Anything, "ns", mock.Anything).Return([]*core.Pin{{Batch: batchID}}, nil, nil)
	or.mem.On("QueueBatchRewind", batchID).Return()

	result, err := or.RewindPins(context.Background(), rewind)
	assert.NoError(t, err)
	assert.Equal(t, int64(100), result.Sequence)
	assert.Equal(t, batchID, result.Batch)
}

func TestRewindPinsFail(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	rewind := &core.PinRewind{Sequence: 100}

	or.mdi.On("GetPins", mock.Anything, "ns", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	_, err := or.RewindPins(context.Background(), rewind)
	assert.EqualError(t, err, "pop")
}

func TestRewindPinsNotFound(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	rewind := &core.PinRewind{Sequence: 100}

	or.mdi.On("GetPins", mock.Anything, "ns", mock.Anything).Return(nil, nil, nil)

	_, err := or.RewindPins(context.Background(), rewind)
	assert.Regexp(t, "FF10109", err)
}

func TestRewindPinsBatch(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	batchID := fftypes.NewUUID()
	rewind := &core.PinRewind{Batch: batchID}

	or.mem.On("QueueBatchRewind", batchID).Return()

	result, err := or.RewindPins(context.Background(), rewind)
	assert.NoError(t, err)
	assert.Equal(t, int64(0), result.Sequence)
	assert.Equal(t, batchID, result.Batch)
}
