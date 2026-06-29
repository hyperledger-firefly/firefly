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

package namespace

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger-firefly/common/mocks/authmocks"
	"github.com/hyperledger-firefly/common/pkg/auth"
	"github.com/hyperledger-firefly/common/pkg/auth/authfactory"
	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/retry"
	"github.com/hyperledger-firefly/firefly/internal/blockchain/bifactory"
	"github.com/hyperledger-firefly/firefly/internal/cache"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/database/difactory"
	"github.com/hyperledger-firefly/firefly/internal/dataexchange/dxfactory"
	"github.com/hyperledger-firefly/firefly/internal/events/eifactory"
	"github.com/hyperledger-firefly/firefly/internal/identity/iifactory"
	"github.com/hyperledger-firefly/firefly/internal/metrics"
	"github.com/hyperledger-firefly/firefly/internal/orchestrator"
	"github.com/hyperledger-firefly/firefly/internal/sharedstorage/ssfactory"
	"github.com/hyperledger-firefly/firefly/internal/tokens/tifactory"
	"github.com/hyperledger-firefly/firefly/mocks/blockchainmocks"
	"github.com/hyperledger-firefly/firefly/mocks/cachemocks"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/mocks/dataexchangemocks"
	"github.com/hyperledger-firefly/firefly/mocks/eventsmocks"
	"github.com/hyperledger-firefly/firefly/mocks/identitymocks"
	"github.com/hyperledger-firefly/firefly/mocks/metricsmocks"
	"github.com/hyperledger-firefly/firefly/mocks/operationmocks"
	"github.com/hyperledger-firefly/firefly/mocks/orchestratormocks"
	"github.com/hyperledger-firefly/firefly/mocks/sharedstoragemocks"
	"github.com/hyperledger-firefly/firefly/mocks/spieventsmocks"
	"github.com/hyperledger-firefly/firefly/mocks/tokenmocks"
	"github.com/hyperledger-firefly/firefly/pkg/blockchain"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/hyperledger-firefly/firefly/pkg/dataexchange"
	"github.com/hyperledger-firefly/firefly/pkg/events"
	"github.com/hyperledger-firefly/firefly/pkg/identity"
	"github.com/hyperledger-firefly/firefly/pkg/sharedstorage"
	"github.com/hyperledger-firefly/firefly/pkg/tokens"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var testBaseConfig = `
---
namespaces:
  predefined:
    - defaultKey: 0xbEa50Ec98776beF144Fc63078e7b15291Ac64cfA
      name: default
      tlsConfigs: []
      plugins:
        - ethereum
        - postgres
        - ffdx
        - ipfs
        - erc721
        - erc1155
        - basicauth
      multiparty:
        enabled: true
        node:
          name: node1
        org:
          name: org1
        contract:
          - firstevent: "0"
            location:
              address: 0x7359d2ecc199C48369b390522c29b77A5Af30882
plugins:
  blockchain:
    - name: ethereum
      type: ethereum
  database:
    - name: postgres
      type: postgres
  dataexchange:
    - name: ffdx
      type: ffdx
  sharedstorage:
    - ipfs:
      name: ipfs
      type: ipfs
  tokens:
    - name: erc721
      type: type1
    - name: erc1155
      type: type2
  auth:
    - name: basicauth
      type: basicauth
  identity:
    - name: tbd
      type: tbd
`

type nmMocks struct {
	nm  *namespaceManager
	mmi *metricsmocks.Manager
	mae *spieventsmocks.Manager
	mbi *blockchainmocks.Plugin
	cmi *cachemocks.Manager
	mdi *databasemocks.Plugin
	mdx *dataexchangemocks.Plugin
	mps *sharedstoragemocks.Plugin
	mti []*tokenmocks.Plugin
	mei []*eventsmocks.Plugin
	mai *authmocks.Plugin
	mii *identitymocks.Plugin
	mo  *orchestratormocks.Orchestrator
}

func (nmm *nmMocks) cleanup(t *testing.T) {
	nmm.mmi.AssertExpectations(t)
	nmm.mae.AssertExpectations(t)
	nmm.mbi.AssertExpectations(t)
	nmm.cmi.AssertExpectations(t)
	nmm.mdi.AssertExpectations(t)
	nmm.mdx.AssertExpectations(t)
	nmm.mps.AssertExpectations(t)
	nmm.mti[0].AssertExpectations(t)
	nmm.mti[1].AssertExpectations(t)
	nmm.mai.AssertExpectations(t)
	nmm.mii.AssertExpectations(t)
	nmm.mei[0].AssertExpectations(t)
	nmm.mei[1].AssertExpectations(t)
	nmm.mei[2].AssertExpectations(t)
	nmm.mo.AssertExpectations(t)
}

func factoryMocks(m *mock.Mock, name string) {
	m.On("Name").Return(name).Maybe()
	m.On("InitConfig", mock.Anything).Maybe()
}

func mockPluginFactories(inm Manager) (nmm *nmMocks) {
	nm := inm.(*namespaceManager)
	nmm = &nmMocks{
		mmi: &metricsmocks.Manager{},
		mae: &spieventsmocks.Manager{},
		mbi: &blockchainmocks.Plugin{},
		cmi: &cachemocks.Manager{},
		mdi: &databasemocks.Plugin{},
		mdx: &dataexchangemocks.Plugin{},
		mps: &sharedstoragemocks.Plugin{},
		mti: []*tokenmocks.Plugin{{}, {}},
		mei: []*eventsmocks.Plugin{{}, {}, {}},
		mai: &authmocks.Plugin{},
		mii: &identitymocks.Plugin{},
		mo:  &orchestratormocks.Orchestrator{},
	}
	factoryMocks(&nmm.mbi.Mock, "ethereum")
	factoryMocks(&nmm.mdi.Mock, "postgres")
	factoryMocks(&nmm.mdx.Mock, "ffdx")
	factoryMocks(&nmm.mps.Mock, "ipfs")
	factoryMocks(&nmm.mti[0].Mock, "erc721")
	factoryMocks(&nmm.mti[1].Mock, "erc1155")
	factoryMocks(&nmm.mei[0].Mock, "system")
	factoryMocks(&nmm.mei[1].Mock, "websockets")
	factoryMocks(&nmm.mei[2].Mock, "webhooks")
	factoryMocks(&nmm.mai.Mock, "basicauth")

	nm.orchestratorFactory = func(ns *core.Namespace, config orchestrator.Config, plugins *orchestrator.Plugins, metrics metrics.Manager, cacheManager cache.Manager) orchestrator.Orchestrator {
		return nmm.mo
	}
	nm.blockchainFactory = func(ctx context.Context, pluginType string) (blockchain.Plugin, error) {
		return nmm.mbi, nil
	}
	nm.databaseFactory = func(ctx context.Context, pluginType string) (database.Plugin, error) {
		return nmm.mdi, nil
	}
	nm.dataexchangeFactory = func(ctx context.Context, pluginType string) (dataexchange.Plugin, error) {
		return nmm.mdx, nil
	}
	nm.sharedstorageFactory = func(ctx context.Context, pluginType string) (sharedstorage.Plugin, error) {
		return nmm.mps, nil
	}
	nm.tokensFactory = func(ctx context.Context, pluginType string) (tokens.Plugin, error) {
		if pluginType == "type1" {
			return nmm.mti[0], nil
		}
		return nmm.mti[1], nil
	}
	nm.identityFactory = func(ctx context.Context, pluginType string) (identity.Plugin, error) {
		return nmm.mii, nil
	}
	nm.eventsFactory = func(ctx context.Context, pluginType string) (events.Plugin, error) {
		switch pluginType {
		case "system":
			return nmm.mei[0], nil
		case "websockets":
			return nmm.mei[1], nil
		case "webhooks":
			return nmm.mei[2], nil
		default:
			panic(fmt.Errorf("Add plugin type %s to test", pluginType))
		}
	}
	nm.authFactory = func(ctx context.Context, pluginType string) (auth.Plugin, error) {
		return nmm.mai, nil
	}

	nmm.nm = nm
	return nmm
}

func newTestNamespaceManager(t *testing.T, initConfig bool) (*namespaceManager, *nmMocks, func()) {
	coreconfig.Reset()
	InitConfig()
	ctx, cancelCtx := context.WithCancel(context.Background())
	nm := &namespaceManager{
		ctx:       ctx,
		cancelCtx: cancelCtx,
		reset:     make(chan bool, 1),
		reloadConfig: func() error {
			coreconfig.Reset()
			InitConfig()
			viper.SetConfigType("yaml")
			return viper.ReadConfig(strings.NewReader(testBaseConfig))
		},
		namespaces:          make(map[string]*namespace),
		plugins:             make(map[string]*plugin),
		tokenBroadcastNames: make(map[string]string),
		nsStartupRetry: &retry.Retry{
			InitialDelay: 1 * time.Second,
		},
	}
	nmm := mockPluginFactories(nm)
	nmm.nm.watchConfig = func() {
		<-ctx.Done()
	}
	nmm.nm.metrics = nmm.mmi
	nmm.nm.adminEvents = nmm.mae

	if initConfig {
		viper.SetConfigType("yaml")
		err := viper.ReadConfig(strings.NewReader(testBaseConfig))
		assert.NoError(t, err)

		nmm.mdi.On("Init", mock.Anything, mock.Anything).Return(nil).Once()
		nmm.mdi.On("SetHandler", database.GlobalHandler, mock.Anything).Return().Once()
		nmm.mbi.On("Init", mock.Anything, mock.Anything, mock.Anything, nmm.mmi, mock.Anything).Return(nil).Once()
		nmm.mdx.On("Init", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()
		nmm.mps.On("Init", mock.Anything, mock.Anything).Return(nil).Once()
		nmm.mti[0].On("Init", mock.Anything, mock.Anything, "erc721", mock.Anything).Return(nil).Once()
		nmm.mti[1].On("Init", mock.Anything, mock.Anything, "erc1155", mock.Anything).Return(nil).Once()
		nmm.mei[0].On("Init", mock.Anything, mock.Anything).Return(nil)
		nmm.mei[1].On("Init", mock.Anything, mock.Anything).Return(nil)
		nmm.mei[2].On("Init", mock.Anything, mock.Anything).Return(nil)
		nmm.mai.On("Init", mock.Anything, mock.Anything, mock.Anything).Return(nil).Once()

		err = nmm.nm.Init(nmm.nm.ctx, nmm.nm.cancelCtx, nmm.nm.reset, nmm.nm.reloadConfig)
		assert.NoError(t, err)
	}

	return nm, nmm, func() {
		if a := recover(); a != nil {
			panic(a)
		}
		nmm.nm.cancelCtx()
		nmm.cleanup(t)
	}
}

func TestNewNamespaceManager(t *testing.T) {
	nm := NewNamespaceManager()
	assert.NotNil(t, nm)
}

func TestInitEmpty(t *testing.T) {
	nm, nmm, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	nm.metricsEnabled = true

	nmm.mei[0].On("Init", mock.Anything, mock.Anything).Return(nil)
	nmm.mei[1].On("Init", mock.Anything, mock.Anything).Return(nil)
	nmm.mei[2].On("Init", mock.Anything, mock.Anything).Return(nil)

	err := nm.Init(nm.ctx, nm.cancelCtx, nm.reset, nm.reloadConfig)
	assert.NoError(t, err)

	assert.Len(t, nm.plugins, 3) // events
	assert.Empty(t, nm.namespaces)
}

func TestInitAllPlugins(t *testing.T) {
	_, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()
}

func TestInitComponentsPluginsFail(t *testing.T) {
	nm, nmm, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	nmm.mai.On("Init", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))

	nm.namespaces = map[string]*namespace{}
	nm.plugins = map[string]*plugin{
		"basicauth": nm.plugins["basicauth"],
	}
	err := nm.initComponents()
	assert.Regexp(t, "pop", err)
}

func TestInitDatabaseFail(t *testing.T) {
	nm, nmm, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	nmm.mdi.On("Init", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))

	err := nm.initPlugins(map[string]*plugin{
		"postgres": nm.plugins["postgres"],
	})
	assert.EqualError(t, err, "pop")
}

func TestInitBlockchainFail(t *testing.T) {
	nm, nmm, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	nmm.mbi.On("Init", mock.Anything, mock.Anything, mock.Anything, nmm.mmi, mock.Anything).Return(fmt.Errorf("pop"))

	err := nm.initPlugins(map[string]*plugin{
		"ethereum": nm.plugins["ethereum"],
	})
	assert.EqualError(t, err, "pop")
}

func TestInitDataExchangeFail(t *testing.T) {
	nm, nmm, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	nmm.mdx.On("Init", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))

	err := nm.initPlugins(map[string]*plugin{
		"ffdx": nm.plugins["ffdx"],
	})
	assert.EqualError(t, err, "pop")
}

func TestInitSharedStorageFail(t *testing.T) {
	nm, nmm, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	nmm.mps.On("Init", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))

	err := nm.initPlugins(map[string]*plugin{
		"ipfs": nm.plugins["ipfs"],
	})
	assert.EqualError(t, err, "pop")
}

func TestInitTokensFail(t *testing.T) {
	nm, nmm, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	nmm.mti[0].On("Init", mock.Anything, mock.Anything, "erc721", mock.Anything).Return(fmt.Errorf("pop"))

	err := nm.initPlugins(map[string]*plugin{
		"erc721": nm.plugins["erc721"],
	})
	assert.EqualError(t, err, "pop")
}

func TestInitEventsFail(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	mei := &eventsmocks.Plugin{}
	mei.On("Init", mock.Anything, mock.Anything).Return(fmt.Errorf("pop")).Maybe()
	nm.plugins["websockets"].events = mei
	err := nm.initPlugins(map[string]*plugin{
		"websockets": nm.plugins["websockets"],
	})
	assert.EqualError(t, err, "pop")
}

func TestInitAuthFail(t *testing.T) {
	nm, nmm, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	nmm.mai.On("Init", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))

	err := nm.initPlugins(map[string]*plugin{
		"basicauth": nm.plugins["basicauth"],
	})
	assert.EqualError(t, err, "pop")
}

func TestInitOrchestratorFail(t *testing.T) {
	nm, nmm, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	nm.plugins = make(map[string]*plugin)
	nmm.mo.On("PreInit", mock.Anything, mock.Anything).Return()
	nmm.mo.On("Init").Return(fmt.Errorf("pop"))

	nmm.mdi.On("GetNamespace", mock.Anything, "default").Return(nil, nil)
	nmm.mdi.On("UpsertNamespace", mock.Anything, mock.AnythingOfType("*core.Namespace"), true).Return(nil)

	err := nm.initComponents()
	assert.NoError(t, err)

	nm.preInitNamespace(nm.namespaces["default"])
	err = nm.initNamespace(nm.namespaces["default"])
	assert.Regexp(t, "pop", err)
}

func TestInitConfigListenerFail(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	nm.metricsEnabled = false
	nm.plugins = make(map[string]*plugin)
	nm.namespaces = make(map[string]*namespace)

	badDir := t.TempDir()
	os.Remove(badDir)
	viper.SetConfigFile(fmt.Sprintf("%s/problem", badDir))
	config.Set(coreconfig.ConfigAutoReload, true)

	err := nm.initComponents()
	assert.Regexp(t, "FF00194", err)
}

func TestInitVersion1(t *testing.T) {
	nm, nmm, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	nmm.mo.On("Start", mock.Anything).Return(nil)
	nmm.mo.On("PreInit", mock.Anything, mock.Anything).Return()
	nmm.mo.On("Init").
		Run(func(args mock.Arguments) {
			nm.namespaces["default"].config.Multiparty.Enabled = true
			nm.namespaces["default"].Contracts.Active = &core.MultipartyContract{
				Location: fftypes.JSONAnyPtr("{}"),
				Info: core.MultipartyContractInfo{
					Version: 1,
				},
			}
		}).
		Return(nil).Twice() // legacy system namespace
	nmm.mdi.On("GetNamespace", mock.Anything, "default").Return(nil, nil)
	nmm.mdi.On("GetNamespace", mock.Anything, core.LegacySystemNamespace).Return(nil, nil)
	nmm.mdi.On("UpsertNamespace", mock.Anything, mock.AnythingOfType("*core.Namespace"), true).Return(nil)

	nm.preInitNamespace(nm.namespaces["default"])
	err := nm.initAndStartNamespace(nm.namespaces["default"])
	assert.NoError(t, err)

	assert.Equal(t, nmm.mo, nm.MustOrchestrator("default"))
	assert.Panics(t, func() { nm.MustOrchestrator("unknown") })
	assert.NotNil(t, nm.MustOrchestrator(core.LegacySystemNamespace))

}

func TestInitFFSystemWithTerminatedV1Contract(t *testing.T) {
	nm, nmm, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()
	nm.metricsEnabled = true

	nmm.mo.On("Start", mock.Anything).Return(nil)
	nmm.mo.On("PreInit", mock.Anything, mock.Anything).Return()
	nmm.mo.On("Init", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			nm.namespaces["default"].config.Multiparty.Enabled = true
			nm.namespaces["default"].Contracts = &core.MultipartyContracts{
				Active: &core.MultipartyContract{
					Info: core.MultipartyContractInfo{
						Version: 2,
					},
				},
				Terminated: []*core.MultipartyContract{{
					Location: fftypes.JSONAnyPtr("{}"),
					Info: core.MultipartyContractInfo{
						Version: 1,
					},
				}},
			}
		}).
		Return(nil).Twice() // legacy system namespace

	nmm.mdi.On("GetNamespace", mock.Anything, "default").Return(nil, nil)
	nmm.mdi.On("GetNamespace", mock.Anything, core.LegacySystemNamespace).Return(nil, nil)
	nmm.mdi.On("UpsertNamespace", mock.Anything, mock.AnythingOfType("*core.Namespace"), true).Return(nil)

	nm.preInitNamespace(nm.namespaces["default"])
	err := nm.initAndStartNamespace(nm.namespaces["default"])
	assert.NoError(t, err)

	assert.Equal(t, nmm.mo, nm.MustOrchestrator("default"))
	assert.Panics(t, func() { nm.MustOrchestrator("unknown") })

}

func TestLegacyNamespaceConflictingPlugins(t *testing.T) {
	nm, nmm, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()
	nm.metricsEnabled = true

	nmm.mo.On("PreInit", mock.Anything, mock.Anything).Return()
	nmm.mo.On("Init", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			nm.namespaces["ff_system"] = &namespace{
				Namespace: core.Namespace{
					Contracts: &core.MultipartyContracts{
						Active: &core.MultipartyContract{
							Location: fftypes.JSONAnyPtr("{}"),
							Info: core.MultipartyContractInfo{
								Version: 1,
							},
						},
					},
				},
			}
			for range nm.namespaces["default"].pluginNames {
				nm.namespaces["ff_system"].pluginNames = append(nm.namespaces["ff_system"].pluginNames, "something_else")
			}
			nm.namespaces["default"].Contracts = &core.MultipartyContracts{
				Active: &core.MultipartyContract{
					Location: fftypes.JSONAnyPtr("{}"),
					Info: core.MultipartyContractInfo{
						Version: 1,
					},
				},
			}
		}).
		Return(nil)

	nmm.mdi.On("GetNamespace", mock.Anything, "default").Return(nil, nil)
	nmm.mdi.On("UpsertNamespace", mock.Anything, mock.AnythingOfType("*core.Namespace"), true).Return(nil)

	nm.preInitNamespace(nm.namespaces["default"])
	err := nm.initAndStartNamespace(nm.namespaces["default"])
	assert.Regexp(t, "FF10421", err)
}

func TestLegacyNamespaceConflictingPluginsTooManyPlugins(t *testing.T) {
	nm, nmm, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()
	nm.metricsEnabled = true

	nmm.mo.On("PreInit", mock.Anything, mock.Anything).Return()
	nmm.mo.On("Init", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			nm.namespaces["ff_system"] = &namespace{
				pluginNames: []string{},
				Namespace: core.Namespace{
					Contracts: &core.MultipartyContracts{
						Active: &core.MultipartyContract{
							Location: fftypes.JSONAnyPtr("{}"),
							Info: core.MultipartyContractInfo{
								Version: 1,
							},
						},
					},
				},
			}
			nm.namespaces["default"].Contracts = &core.MultipartyContracts{
				Active: &core.MultipartyContract{
					Location: fftypes.JSONAnyPtr("{}"),
					Info: core.MultipartyContractInfo{
						Version: 1,
					},
				},
			}
		}).
		Return(nil)

	nmm.mdi.On("GetNamespace", mock.Anything, "default").Return(nil, nil)
	nmm.mdi.On("UpsertNamespace", mock.Anything, mock.AnythingOfType("*core.Namespace"), true).Return(nil)

	nm.preInitNamespace(nm.namespaces["default"])
	err := nm.initAndStartNamespace(nm.namespaces["default"])
	assert.Regexp(t, "FF10421", err)

	assert.Equal(t, nmm.mo, nm.MustOrchestrator("default"))
	assert.Panics(t, func() { nm.MustOrchestrator("unknown") })

}

func TestLegacyNamespaceMatchingPlugins(t *testing.T) {
	nm, nmm, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()
	nm.metricsEnabled = true

	nmm.mo.On("Start", mock.Anything).Return(nil)
	nmm.mo.On("PreInit", mock.Anything, mock.Anything).Return()
	nmm.mo.On("Init", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			nm.namespaces["ff_system"] = &namespace{
				pluginNames: []string{"ethereum", "postgres", "ffdx", "ipfs", "erc721", "erc1155", "basicauth"},
				Namespace: core.Namespace{
					Contracts: &core.MultipartyContracts{
						Active: &core.MultipartyContract{
							Location: fftypes.JSONAnyPtr("{}"),
							Info: core.MultipartyContractInfo{
								Version: 1,
							},
						},
					},
				},
			}
			nm.namespaces["default"].Contracts = &core.MultipartyContracts{
				Active: &core.MultipartyContract{
					Location: fftypes.JSONAnyPtr("{}"),
					Info: core.MultipartyContractInfo{
						Version: 1,
					},
				},
			}
		}).
		Return(nil)

	nmm.mdi.On("GetNamespace", mock.Anything, "default").Return(nil, nil)
	nmm.mdi.On("GetNamespace", mock.Anything, core.LegacySystemNamespace).Return(nil, nil)
	nmm.mdi.On("UpsertNamespace", mock.Anything, mock.AnythingOfType("*core.Namespace"), true).Return(nil)

	nm.preInitNamespace(nm.namespaces["default"])
	err := nm.initAndStartNamespace(nm.namespaces["default"])
	assert.NoError(t, err)

	assert.Equal(t, nmm.mo, nm.MustOrchestrator("default"))
	assert.Panics(t, func() { nm.MustOrchestrator("unknown") })

}

func TestInitVersion1Fail(t *testing.T) {
	nm, nmm, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	nmm.mo.On("PreInit", mock.Anything, mock.Anything).Return()
	nmm.mo.On("Init", mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			nm.namespaces["default"].config.Multiparty.Enabled = true
			nm.namespaces["default"].Contracts.Active = &core.MultipartyContract{
				Location: fftypes.JSONAnyPtr("{}"),
				Info: core.MultipartyContractInfo{
					Version: 1,
				},
			}
		}).
		Return(nil).Once()
	nmm.mo.On("Init", mock.Anything, mock.Anything).Return(fmt.Errorf("pop")).Once()

	nmm.mdi.On("GetNamespace", mock.Anything, "default").Return(nil, nil)
	nmm.mdi.On("GetNamespace", mock.Anything, core.LegacySystemNamespace).Return(nil, nil)
	nmm.mdi.On("UpsertNamespace", mock.Anything, mock.AnythingOfType("*core.Namespace"), true).Return(nil)

	nm.preInitNamespace(nm.namespaces["default"])
	err := nm.initAndStartNamespace(nm.namespaces["default"])
	assert.EqualError(t, err, "pop")

}

func TestInitFail(t *testing.T) {
	nm, nmm, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	nmm.mo.On("PreInit", mock.Anything, mock.Anything).Return()
	nmm.mo.On("Init", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))

	nmm.mdi.On("GetNamespace", mock.Anything, "default").Return(nil, nil)
	nmm.mdi.On("UpsertNamespace", mock.Anything, mock.AnythingOfType("*core.Namespace"), true).Return(nil)

	nm.preInitNamespace(nm.namespaces["default"])
	err := nm.initAndStartNamespace(nm.namespaces["default"])
	assert.EqualError(t, err, "pop")

}

func TestInitNamespaceQueryFail(t *testing.T) {
	nm, nmm, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	nmm.mdi.On("GetNamespace", mock.Anything, "default").Return(nil, fmt.Errorf("pop"))

	err := nm.preInitNamespace(nm.namespaces["default"])
	assert.EqualError(t, err, "pop")
}

func TestInitNamespaceExistingUpsertFail(t *testing.T) {
	nm, nmm, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	existing := &core.Namespace{
		NetworkName: "ns1",
	}

	nmm.mdi.On("GetNamespace", mock.Anything, "default").Return(existing, nil)
	nmm.mdi.On("UpsertNamespace", mock.Anything, mock.AnythingOfType("*core.Namespace"), true).Return(fmt.Errorf("pop"))

	err := nm.preInitNamespace(nm.namespaces["default"])
	assert.EqualError(t, err, "pop")
}

func TestDatabasePlugin(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	difactory.InitConfig(databaseConfig)
	config.Set("plugins.database", []fftypes.JSONObject{{}})
	databaseConfig.AddKnownKey(coreconfig.PluginConfigName, "flapflip")
	databaseConfig.AddKnownKey(coreconfig.PluginConfigType, "postgres")
	plugins := make(map[string]*plugin)
	err := nm.getDatabasePlugins(context.Background(), plugins, nm.dumpRootConfig())
	assert.Equal(t, 1, len(plugins))
	assert.NoError(t, err)
}

func TestDatabasePluginBadType(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	difactory.InitConfig(databaseConfig)
	config.Set("plugins.database", []fftypes.JSONObject{{}})
	databaseConfig.AddKnownKey(coreconfig.PluginConfigName, "flapflip")
	databaseConfig.AddKnownKey(coreconfig.PluginConfigType, "unknown")
	nm.databaseFactory = func(ctx context.Context, pluginType string) (database.Plugin, error) {
		return nil, fmt.Errorf("pop")
	}
	err := nm.getDatabasePlugins(context.Background(), make(map[string]*plugin), nm.dumpRootConfig())
	assert.Regexp(t, "pop", err)
}

func TestDatabasePluginBadName(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	difactory.InitConfig(databaseConfig)
	config.Set("plugins.database", []fftypes.JSONObject{{}})
	databaseConfig.AddKnownKey(coreconfig.PluginConfigName, "wrong////")
	databaseConfig.AddKnownKey(coreconfig.PluginConfigType, "postgres")
	err := nm.getDatabasePlugins(context.Background(), make(map[string]*plugin), nm.dumpRootConfig())
	assert.Regexp(t, "FF00140", err)
}

func TestIdentityPluginBadName(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	iifactory.InitConfig(identityConfig)
	identityConfig.AddKnownKey(coreconfig.PluginConfigName, "wrong//")
	identityConfig.AddKnownKey(coreconfig.PluginConfigType, "tbd")
	config.Set("plugins.identity", []fftypes.JSONObject{{}})
	err := nm.getIdentityPlugins(context.Background(), make(map[string]*plugin), nm.dumpRootConfig())
	assert.Regexp(t, "FF00140.*name", err)
}

func TestIdentityPluginBadType(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	iifactory.InitConfig(identityConfig)
	identityConfig.AddKnownKey(coreconfig.PluginConfigName, "flapflip")
	identityConfig.AddKnownKey(coreconfig.PluginConfigType, "wrong")
	config.Set("plugins.identity", []fftypes.JSONObject{{}})
	nm.identityFactory = func(ctx context.Context, pluginType string) (identity.Plugin, error) {
		return nil, fmt.Errorf("pop")
	}
	err := nm.getIdentityPlugins(context.Background(), make(map[string]*plugin), nm.dumpRootConfig())
	assert.Regexp(t, "pop", err)
}

func TestIdentityPluginNoType(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	iifactory.InitConfig(identityConfig)
	identityConfig.AddKnownKey(coreconfig.PluginConfigName, "flapflip")
	config.Set("plugins.identity", []fftypes.JSONObject{{}})

	ctx, cancelCtx := context.WithCancel(context.Background())
	err := nm.Init(ctx, cancelCtx, nm.reset, nm.reloadConfig)
	assert.Regexp(t, "FF10386.*type", err)
}

func TestIdentityPlugin(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	iifactory.InitConfig(identityConfig)
	identityConfig.AddKnownKey(coreconfig.PluginConfigName, "flapflip")
	identityConfig.AddKnownKey(coreconfig.PluginConfigType, "onchain")
	config.Set("plugins.identity", []fftypes.JSONObject{{}})
	plugins := make(map[string]*plugin)
	err := nm.getIdentityPlugins(context.Background(), plugins, nm.dumpRootConfig())
	assert.NoError(t, err)
	assert.Equal(t, 1, len(plugins))
}

func TestBlockchainPlugin(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	bifactory.InitConfig(blockchainConfig)
	config.Set("plugins.blockchain", []fftypes.JSONObject{{}})
	blockchainConfig.AddKnownKey(coreconfig.PluginConfigName, "flapflip")
	blockchainConfig.AddKnownKey(coreconfig.PluginConfigType, "ethereum")
	plugins := make(map[string]*plugin)
	err := nm.getBlockchainPlugins(context.Background(), plugins, nm.dumpRootConfig())
	assert.Equal(t, 1, len(plugins))
	assert.NoError(t, err)
}

func TestBlockchainPluginNoType(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	bifactory.InitConfig(blockchainConfig)
	config.Set("plugins.blockchain", []fftypes.JSONObject{{}})
	blockchainConfig.AddKnownKey(coreconfig.PluginConfigName, "flapflip")
	plugins := make(map[string]*plugin)
	err := nm.getBlockchainPlugins(context.Background(), plugins, nm.dumpRootConfig())
	assert.Regexp(t, "FF10386", err)
}

func TestBlockchainPluginBadType(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	bifactory.InitConfig(blockchainConfig)
	config.Set("plugins.blockchain", []fftypes.JSONObject{{}})
	blockchainConfig.AddKnownKey(coreconfig.PluginConfigName, "flapflip")
	blockchainConfig.AddKnownKey(coreconfig.PluginConfigType, "wrong//")

	nm.blockchainFactory = func(ctx context.Context, pluginType string) (blockchain.Plugin, error) {
		return nil, fmt.Errorf("pop")
	}
	_, err := nm.loadPlugins(context.Background(), nm.dumpRootConfig())
	assert.Regexp(t, "pop", err)
}

func TestSharedStoragePlugin(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	ssfactory.InitConfig(sharedstorageConfig)
	config.Set("plugins.sharedstorage", []fftypes.JSONObject{{}})
	sharedstorageConfig.AddKnownKey(coreconfig.PluginConfigName, "flapflip")
	sharedstorageConfig.AddKnownKey(coreconfig.PluginConfigType, "ipfs")
	plugins := make(map[string]*plugin)
	err := nm.getSharedStoragePlugins(context.Background(), plugins, nm.dumpRootConfig())
	assert.Equal(t, 1, len(plugins))
	assert.NoError(t, err)
}

func TestSharedStoragePluginNoType(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	ssfactory.InitConfig(sharedstorageConfig)
	config.Set("plugins.sharedstorage", []fftypes.JSONObject{{}})
	sharedstorageConfig.AddKnownKey(coreconfig.PluginConfigName, "flapflip")
	plugins := make(map[string]*plugin)
	err := nm.getSharedStoragePlugins(context.Background(), plugins, nm.dumpRootConfig())
	assert.Regexp(t, "FF10386", err)
}

func TestSharedStoragePluginBadType(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	ssfactory.InitConfig(sharedstorageConfig)
	config.Set("plugins.sharedstorage", []fftypes.JSONObject{{}})
	sharedstorageConfig.AddKnownKey(coreconfig.PluginConfigName, "flapflip")
	sharedstorageConfig.AddKnownKey(coreconfig.PluginConfigType, "wrong//")

	nm.sharedstorageFactory = func(ctx context.Context, pluginType string) (sharedstorage.Plugin, error) {
		return nil, fmt.Errorf("pop")
	}
	_, err := nm.loadPlugins(context.Background(), nm.dumpRootConfig())
	assert.Regexp(t, "pop", err)
}

func TestDataExchangePlugin(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	dxfactory.InitConfig(dataexchangeConfig)
	config.Set("plugins.dataexchange", []fftypes.JSONObject{{}})
	dataexchangeConfig.AddKnownKey(coreconfig.PluginConfigName, "flapflip")
	dataexchangeConfig.AddKnownKey(coreconfig.PluginConfigType, "ffdx")
	plugins := make(map[string]*plugin)
	err := nm.getDataExchangePlugins(context.Background(), plugins, nm.dumpRootConfig())
	assert.Equal(t, 1, len(plugins))
	assert.NoError(t, err)
}

func TestDataExchangePluginNoType(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	dxfactory.InitConfig(dataexchangeConfig)
	config.Set("plugins.dataexchange", []fftypes.JSONObject{{}})
	dataexchangeConfig.AddKnownKey(coreconfig.PluginConfigName, "flapflip")
	plugins := make(map[string]*plugin)
	err := nm.getDataExchangePlugins(context.Background(), plugins, nm.dumpRootConfig())
	assert.Regexp(t, "FF10386", err)
}

func TestDataExchangePluginBadType(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	dxfactory.InitConfig(dataexchangeConfig)
	config.Set("plugins.dataexchange", []fftypes.JSONObject{{}})
	dataexchangeConfig.AddKnownKey(coreconfig.PluginConfigName, "flapflip")
	dataexchangeConfig.AddKnownKey(coreconfig.PluginConfigType, "wrong//")

	nm.dataexchangeFactory = func(ctx context.Context, pluginType string) (dataexchange.Plugin, error) {
		return nil, fmt.Errorf("pop")
	}
	_, err := nm.loadPlugins(context.Background(), nm.dumpRootConfig())
	assert.Regexp(t, "pop", err)
}

func TestTokensPlugin(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	tifactory.InitConfig(tokensConfig)
	config.Set("plugins.tokens", []fftypes.JSONObject{{}})
	tokensConfig.AddKnownKey(coreconfig.PluginConfigName, "erc20_erc721")
	tokensConfig.AddKnownKey(coreconfig.PluginConfigType, "fftokens")
	plugins := make(map[string]*plugin)
	err := nm.getTokensPlugins(context.Background(), plugins, nm.dumpRootConfig())
	assert.Equal(t, 1, len(plugins))
	assert.NoError(t, err)
}

func TestTokensPluginDuplicateBroadcastName(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	tifactory.InitConfig(tokensConfig)
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
  plugins:
    tokens:
    - name: test1
      broadcastName: remote1
      type: fftokens
      fftokens:
        url: http://tokens:3000
    - name: test2
      broadcastName: remote1
      type: fftokens
      fftokens:
        url: http://tokens:3000
  `))
	assert.NoError(t, err)

	plugins := make(map[string]*plugin)
	err = nm.getTokensPlugins(context.Background(), plugins, nm.dumpRootConfig())
	assert.Regexp(t, "FF10419", err)
}

func TestMultipleTokensPluginsWithBroadcastName(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	tifactory.InitConfig(tokensConfig)
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
  plugins:
    tokens:
    - name: test1
      broadcastName: remote1
      type: fftokens
      fftokens:
        url: http://tokens:3000
    - name: test2
      broadcastName: remote2
      type: fftokens
      fftokens:
        url: http://tokens:3000
  `))
	assert.NoError(t, err)

	plugins := make(map[string]*plugin)
	err = nm.getTokensPlugins(context.Background(), plugins, nm.dumpRootConfig())
	assert.Equal(t, 2, len(plugins))
	assert.NoError(t, err)
}

func TestTokensPluginNoType(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	tifactory.InitConfig(tokensConfig)
	config.Set("plugins.tokens", []fftypes.JSONObject{{}})
	tokensConfig.AddKnownKey(coreconfig.PluginConfigName, "erc20_erc721")
	plugins := make(map[string]*plugin)
	err := nm.getTokensPlugins(context.Background(), plugins, nm.dumpRootConfig())
	assert.Regexp(t, "FF10386", err)
}

func TestTokensPluginBadType(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	tifactory.InitConfig(tokensConfig)
	config.Set("plugins.tokens", []fftypes.JSONObject{{}})
	tokensConfig.AddKnownKey(coreconfig.PluginConfigName, "erc20_erc721")
	tokensConfig.AddKnownKey(coreconfig.PluginConfigType, "wrong")

	nm.tokensFactory = func(ctx context.Context, pluginType string) (tokens.Plugin, error) {
		return nil, fmt.Errorf("pop")
	}
	_, err := nm.loadPlugins(context.Background(), nm.dumpRootConfig())
	assert.Regexp(t, "pop", err)
}

func TestTokensPluginDuplicate(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	tifactory.InitConfig(tokensConfig)
	config.Set("plugins.tokens", []fftypes.JSONObject{{}})
	tokensConfig.AddKnownKey(coreconfig.PluginConfigName, "erc20_erc721")
	tokensConfig.AddKnownKey(coreconfig.PluginConfigType, "fftokens")
	plugins := make(map[string]*plugin)
	plugins["erc20_erc721"] = &plugin{}
	err := nm.getTokensPlugins(context.Background(), plugins, nm.dumpRootConfig())
	assert.Regexp(t, "FF10395", err)
}

func TestEventsPluginDefaults(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	plugins := make(map[string]*plugin)
	err := nm.getEventPlugins(context.Background(), plugins, nm.dumpRootConfig())
	assert.Equal(t, 3, len(plugins))
	assert.NoError(t, err)
}

func TestEventsPluginDuplicate(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	eifactory.InitConfig(eventsConfig)
	eventsConfig.AddKnownKey(coreconfig.PluginConfigName, "websockets")
	eventsConfig.AddKnownKey(coreconfig.PluginConfigType, "websockets")
	plugins := make(map[string]*plugin)
	plugins["websockets"] = &plugin{}
	err := nm.getEventPlugins(context.Background(), plugins, nm.dumpRootConfig())
	assert.Regexp(t, "FF10395", err)
}

func TestAuthPlugin(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	authfactory.InitConfigArray(authConfig)
	config.Set("plugins.auth", []fftypes.JSONObject{{}})
	authConfig.AddKnownKey(coreconfig.PluginConfigName, "basicauth")
	authConfig.AddKnownKey(coreconfig.PluginConfigType, "basic")
	plugins := make(map[string]*plugin)
	err := nm.getAuthPlugin(context.Background(), plugins, nm.dumpRootConfig())
	assert.Equal(t, 1, len(plugins))
	assert.NoError(t, err)
}

func TestAuthPluginBadType(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	authfactory.InitConfigArray(authConfig)
	config.Set("plugins.auth", []fftypes.JSONObject{{}})
	authConfig.AddKnownKey(coreconfig.PluginConfigName, "basicauth")
	authConfig.AddKnownKey(coreconfig.PluginConfigType, "wrong")

	nm.authFactory = func(ctx context.Context, pluginType string) (auth.Plugin, error) {
		return nil, fmt.Errorf("pop")
	}
	_, err := nm.loadPlugins(context.Background(), nm.dumpRootConfig())
	assert.Regexp(t, "pop", err)
}

func TestAuthPluginInvalid(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	authfactory.InitConfigArray(authConfig)
	config.Set("plugins.auth", []fftypes.JSONObject{{}})
	authConfig.AddKnownKey(coreconfig.PluginConfigName, "bad name not allowed")
	authConfig.AddKnownKey(coreconfig.PluginConfigType, "basic")
	plugins := make(map[string]*plugin)
	err := nm.getAuthPlugin(context.Background(), plugins, nm.dumpRootConfig())
	assert.Regexp(t, "FF00140", err)
}

func TestAuthPluginDuplicate(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	authfactory.InitConfigArray(authConfig)
	config.Set("plugins.auth", []fftypes.JSONObject{{}})
	authConfig.AddKnownKey(coreconfig.PluginConfigName, "basicauth")
	authConfig.AddKnownKey(coreconfig.PluginConfigType, "basicauth")
	plugins := make(map[string]*plugin)
	plugins["basicauth"] = &plugin{}
	err := nm.getAuthPlugin(context.Background(), plugins, nm.dumpRootConfig())
	assert.Regexp(t, "FF10395", err)
}

func TestRawConfigCorrelation(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	_, err := nm.loadNamespaces(nm.ctx, fftypes.JSONObject{}, nm.plugins)
	err = nm.getDatabasePlugins(nm.ctx, nm.plugins, fftypes.JSONObject{})
	assert.Regexp(t, "FF10439", err)
	err = nm.getBlockchainPlugins(nm.ctx, nm.plugins, fftypes.JSONObject{})
	assert.Regexp(t, "FF10439", err)
	err = nm.getDataExchangePlugins(nm.ctx, nm.plugins, fftypes.JSONObject{})
	assert.Regexp(t, "FF10439", err)
	err = nm.getSharedStoragePlugins(nm.ctx, nm.plugins, fftypes.JSONObject{})
	assert.Regexp(t, "FF10439", err)
	err = nm.getTokensPlugins(nm.ctx, nm.plugins, fftypes.JSONObject{})
	assert.Regexp(t, "FF10439", err)
	err = nm.getIdentityPlugins(nm.ctx, nm.plugins, fftypes.JSONObject{})
	assert.Regexp(t, "FF10439", err)
	err = nm.getAuthPlugin(nm.ctx, nm.plugins, fftypes.JSONObject{})
	assert.Regexp(t, "FF10439", err)
}

func TestEventsPluginBadType(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()
	config.Set(coreconfig.EventTransportsEnabled, []string{"!unknown!"})

	nm.eventsFactory = func(ctx context.Context, pluginType string) (events.Plugin, error) {
		return nil, fmt.Errorf("pop")
	}
	_, err := nm.loadPlugins(context.Background(), nm.dumpRootConfig())
	assert.Regexp(t, "pop", err)
}

func TestInitBadNamespace(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
  namespaces:
    default: "!Badness"
    predefined:
    - name: "!Badness"
    `))
	assert.NoError(t, err)

	ctx, cancelCtx := context.WithCancel(context.Background())
	err = nm.Init(ctx, cancelCtx, nm.reset, nm.reloadConfig)
	assert.Regexp(t, "FF00140", err)
}

func TestLoadNamespacesReservedName(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
  namespaces:
    default: ns1
    predefined:
    - name: ff_system
    `))
	assert.NoError(t, err)

	_, err = nm.loadNamespaces(context.Background(), nm.dumpRootConfig(), nm.plugins)
	assert.Regexp(t, "FF10388", err)
}

func TestLoadNamespacesNetworkName(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
  namespaces:
    default: ns1
    predefined:
    - name: ns1
      multiparty:
        enabled: true
        networknamespace: default
    `))
	assert.NoError(t, err)

	newNS, err := nm.loadNamespaces(context.Background(), nm.dumpRootConfig(), nm.plugins)
	assert.NoError(t, err)

	assert.Equal(t, "default", newNS["ns1"].NetworkName)
}

func TestLoadNamespacesReservedNetworkName(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
  namespaces:
    default: ns1
    predefined:
    - name: ns1
      multiparty:
        enabled: true
        networknamespace: ff_system
    `))
	assert.NoError(t, err)

	_, err = nm.loadNamespaces(context.Background(), nm.dumpRootConfig(), nm.plugins)
	assert.Regexp(t, "FF10388", err)
}

func TestLoadNamespacesDuplicate(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
  namespaces:
    default: ns1
    predefined:
    - name: ns1
      plugins: [postgres]
    - name: ns1
      plugins: [postgres]
    `))
	assert.NoError(t, err)

	newNS, err := nm.loadNamespaces(context.Background(), nm.dumpRootConfig(), nm.plugins)
	assert.NoError(t, err)
	assert.Len(t, newNS, 1)
}

func TestLoadNamespacesNoName(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
  namespaces:
    predefined:
    - plugins: [postgres]
    `))
	assert.NoError(t, err)

	_, err = nm.loadNamespaces(context.Background(), nm.dumpRootConfig(), nm.plugins)
	assert.Regexp(t, "FF10166", err)
}

func TestLoadNamespacesNoDefault(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
  namespaces:
    default: ns1
    predefined:
    - name: ns2
      plugins: [postgres]
  `))
	assert.NoError(t, err)

	_, err = nm.loadNamespaces(context.Background(), nm.dumpRootConfig(), nm.plugins)
	assert.Regexp(t, "FF10166", err)
}

func TestLoadNamespacesUseDefaults(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
  namespaces:
    default: ns1
    predefined:
    - name: ns1
      multiparty:
        contract:
        - location:
          address: 0x1234
  org:
    name: org1
  node:
    name: node1
  `))
	assert.NoError(t, err)

	newNS, err := nm.loadNamespaces(context.Background(), nm.dumpRootConfig(), nm.plugins)
	assert.NoError(t, err)
	assert.Len(t, newNS, 1)
	assert.Equal(t, "oldest", newNS["ns1"].config.Multiparty.Contracts[0].FirstEvent)
}

func TestLoadTLSConfigsBadTLS(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
namespaces:
  default: ns1
  predefined:
  - name: ns1
    tlsConfigs:
    - name: myconfig
      tls:
        enabled: true
        caFile: my-ca
        certFile: my-cert
        keyFile: my-key
  `))
	assert.NoError(t, err)

	// RawConfig to Section!
	tlsConfigArray := namespacePredefined.ArrayEntry(0).SubArray(coreconfig.NamespaceTLSConfigs)
	tlsConfigs := make(map[string]*tls.Config)
	err = nm.loadTLSConfig(nm.ctx, tlsConfigs, tlsConfigArray)
	assert.Regexp(t, "FF00153", err)
}

func TestLoadTLSConfigsNotEnabled(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
namespaces:
  default: ns1
  predefined:
  - name: ns1
    tlsConfigs:
    - name: myconfig
      tls:
        enabled: false
        caFile: my-ca
        certFile: my-cert
        keyFile: my-key
  `))
	assert.NoError(t, err)

	// RawConfig to Section!
	tlsConfigArray := namespacePredefined.ArrayEntry(0).SubArray(coreconfig.NamespaceTLSConfigs)
	tlsConfigs := make(map[string]*tls.Config)
	err = nm.loadTLSConfig(nm.ctx, tlsConfigs, tlsConfigArray)
	assert.NoError(t, err)
	assert.Nil(t, tlsConfigs["myconfig"])
}

func generateTestCertificates() (*os.File, *os.File, func()) {
	// Create an X509 certificate pair
	privatekey, _ := rsa.GenerateKey(rand.Reader, 2048)
	publickey := &privatekey.PublicKey
	var privateKeyBytes []byte = x509.MarshalPKCS1PrivateKey(privatekey)
	privateKeyFile, _ := os.CreateTemp("", "key.pem")
	privateKeyBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: privateKeyBytes}
	pem.Encode(privateKeyFile, privateKeyBlock)
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	x509Template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Unit Tests"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(1000 * time.Second),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	derBytes, _ := x509.CreateCertificate(rand.Reader, x509Template, x509Template, publickey, privatekey)
	publicKeyFile, _ := os.CreateTemp("", "cert.pem")
	pem.Encode(publicKeyFile, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	return publicKeyFile, privateKeyFile, func() {
		os.Remove(publicKeyFile.Name())
		os.Remove(privateKeyFile.Name())
	}
}

func TestLoadTLSConfigs(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	publicKeyFile, privateKeyFile, cleanCertificates := generateTestCertificates()
	defer cleanCertificates()

	caCert, err := os.ReadFile(publicKeyFile.Name())
	if err != nil {
		log.Fatal(err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	cert, err := tls.LoadX509KeyPair(publicKeyFile.Name(), privateKeyFile.Name())
	assert.NoError(t, err)
	expectedTLSConfig := &tls.Config{
		RootCAs:      caCertPool,
		Certificates: []tls.Certificate{cert},
	}

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err = viper.ReadConfig(strings.NewReader(fmt.Sprintf(`
namespaces:
  default: ns1
  predefined:
  - name: ns1
    tlsConfigs:
    - name: myconfig
      tls:
        enabled: true
        caFile: %s
        certFile: %s
        keyFile: %s 
  `, publicKeyFile.Name(), publicKeyFile.Name(), privateKeyFile.Name())))
	assert.NoError(t, err)

	// RawConfig to Section!
	tlsConfigArray := namespacePredefined.ArrayEntry(0).SubArray(coreconfig.NamespaceTLSConfigs)
	tlsConfigs := make(map[string]*tls.Config)
	err = nm.loadTLSConfig(nm.ctx, tlsConfigs, tlsConfigArray)
	assert.NoError(t, err)
	assert.NotNil(t, tlsConfigs["myconfig"])
	assert.True(t, tlsConfigs["myconfig"].RootCAs.Equal(expectedTLSConfig.RootCAs))
	certificate, err := tlsConfigs["myconfig"].GetCertificate(nil)
	assert.NoError(t, err)
	assert.Equal(t, *certificate, cert)
}

func TestLoadTLSConfigsDuplicateConfigs(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	publicKeyFile, privateKeyFile, cleanCertificates := generateTestCertificates()
	defer cleanCertificates()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(fmt.Sprintf(`
namespaces:
  default: ns1
  predefined:
  - name: ns1
    tlsConfigs:
    - name: myconfig
      tls:
        enabled: true
        caFile: %s
        certFile: %s
        keyFile: %s 
    - name: myconfig
      tls:
        enabled: true
        caFile: %s
        certFile: %s
        keyFile: %s 
  `, publicKeyFile.Name(), publicKeyFile.Name(), privateKeyFile.Name(),
		publicKeyFile.Name(), publicKeyFile.Name(), privateKeyFile.Name())))
	assert.NoError(t, err)

	// RawConfig to Section!
	tlsConfigArray := namespacePredefined.ArrayEntry(0).SubArray(coreconfig.NamespaceTLSConfigs)
	tlsConfigs := make(map[string]*tls.Config)
	err = nm.loadTLSConfig(nm.ctx, tlsConfigs, tlsConfigArray)
	assert.Regexp(t, "FF10454", err)
}

func TestLoadNamespacesWithErrorTLSConfigs(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
namespaces:
  default: ns1
  predefined:
  - name: ns1
    tlsConfigs:
    - name: myconfig
      tls:
        enabled: true
        caFile: my-ca
        certFile: my-cert
        keyFile: my-key
  `))
	assert.NoError(t, err)

	nm.namespaces, err = nm.loadNamespaces(context.Background(), nm.dumpRootConfig(), nm.plugins)

	assert.Regexp(t, "FF00153", err)
}

func TestLoadNamespacesNonMultipartyNoDatabase(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
  namespaces:
    default: ns1
    predefined:
    - name: ns1
      plugins: []
  `))
	assert.NoError(t, err)

	nm.namespaces, err = nm.loadNamespaces(context.Background(), nm.dumpRootConfig(), nm.plugins)
	assert.Regexp(t, "FF10392", err)

}

func TestLoadNamespacesMultipartyUnknownPlugin(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
  namespaces:
    default: ns1
    predefined:
    - name: ns1
      plugins: [basicauth, bad]
      multiparty:
        enabled: true
  `))
	assert.NoError(t, err)

	nm.namespaces, err = nm.loadNamespaces(context.Background(), nm.dumpRootConfig(), nm.plugins)
	assert.Regexp(t, "FF10390.*unknown", err)
}

func TestLoadNamespacesMultipartyMultipleBlockchains(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
  namespaces:
    default: ns1
    predefined:
    - name: ns1
      plugins: [ethereum, ethereum]
      multiparty:
        enabled: true
  `))
	assert.NoError(t, err)

	nm.namespaces, err = nm.loadNamespaces(context.Background(), nm.dumpRootConfig(), nm.plugins)
	assert.Regexp(t, "FF10394.*blockchain", err)
}

func TestLoadNamespacesMultipartyMultipleDX(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
  namespaces:
    default: ns1
    predefined:
    - name: ns1
      plugins: [ffdx, ffdx]
      multiparty:
        enabled: true
  `))
	assert.NoError(t, err)

	nm.namespaces, err = nm.loadNamespaces(context.Background(), nm.dumpRootConfig(), nm.plugins)
	assert.Regexp(t, "FF10394.*dataexchange", err)
}

func TestLoadNamespacesMultipartyMultipleSS(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
  namespaces:
    default: ns1
    predefined:
    - name: ns1
      plugins: [ipfs, ipfs]
      multiparty:
        enabled: true
  `))
	assert.NoError(t, err)

	nm.namespaces, err = nm.loadNamespaces(context.Background(), nm.dumpRootConfig(), nm.plugins)
	assert.Regexp(t, "FF10394.*sharedstorage", err)
}

func TestLoadNamespacesMultipartyMultipleDB(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
  namespaces:
    default: ns1
    predefined:
    - name: ns1
      plugins: [postgres, postgres]
      multiparty:
        enabled: true
  `))
	assert.NoError(t, err)

	nm.namespaces, err = nm.loadNamespaces(context.Background(), nm.dumpRootConfig(), nm.plugins)
	assert.Regexp(t, "FF10394.*database", err)
}

func TestLoadNamespacesMultipartyMultipleAuths(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
  namespaces:
    default: ns1
    predefined:
    - name: ns1
      plugins: [basicauth, basicauth]
      multiparty:
        enabled: true
  `))
	assert.NoError(t, err)

	nm.namespaces, err = nm.loadNamespaces(context.Background(), nm.dumpRootConfig(), nm.plugins)
	assert.Regexp(t, "FF10394.*auth", err)
}

func TestLoadNamespacesMultipartyMultipleIdentity(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
  namespaces:
    default: ns1
    predefined:
    - name: ns1
      plugins: [tbd, tbd]
      multiparty:
        enabled: true
  `))
	assert.NoError(t, err)

	nm.namespaces, err = nm.loadNamespaces(context.Background(), nm.dumpRootConfig(), nm.plugins)
	assert.Regexp(t, "FF10394.*identity", err)
}

func TestInitNamespacesMultipartyWithAuth(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
  namespaces:
    default: ns1
    predefined:
    - name: ns1
      plugins: [ethereum, postgres, ipfs, ffdx, basicauth]
      multiparty:
        enabled: true
  `))
	assert.NoError(t, err)

	nm.namespaces, err = nm.loadNamespaces(context.Background(), nm.dumpRootConfig(), nm.plugins)
	assert.NoError(t, err)
}

func TestLoadNamespacesNonMultipartyWithAuth(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
  namespaces:
    default: ns1
    predefined:
    - name: ns1
      plugins: [ethereum, postgres, basicauth]
      multiparty:
        enabled: false
  `))
	assert.NoError(t, err)

	nm.namespaces, err = nm.loadNamespaces(context.Background(), nm.dumpRootConfig(), nm.plugins)
	assert.NoError(t, err)
}

func TestLoadNamespacesMultipartyContract(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
  namespaces:
    default: ns1
    predefined:
    - name: ns1
      multiparty:
        enabled: true
        contract:
        - location:
            address: 0x4ae50189462b0e5d52285f59929d037f790771a6
          firstEvent: oldest
  `))
	assert.NoError(t, err)

	nm.namespaces, err = nm.loadNamespaces(context.Background(), nm.dumpRootConfig(), nm.plugins)
	assert.NoError(t, err)
}

func TestLoadNamespacesNonMultipartyMultipleDB(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
  namespaces:
    default: ns1
    predefined:
    - name: ns1
      plugins: [postgres, postgres]
  `))
	assert.NoError(t, err)

	nm.namespaces, err = nm.loadNamespaces(context.Background(), nm.dumpRootConfig(), nm.plugins)
	assert.Regexp(t, "FF10394.*database", err)
}

func TestLoadNamespacesNonMultipartyMultipleBlockchains(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
  namespaces:
    default: ns1
    predefined:
    - name: ns1
      plugins: [ethereum, ethereum]
  `))
	assert.NoError(t, err)

	nm.namespaces, err = nm.loadNamespaces(context.Background(), nm.dumpRootConfig(), nm.plugins)
	assert.Regexp(t, "FF10394.*blockchain", err)
}

func TestLoadNamespacesMultipartyMissingPlugins(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
  namespaces:
    default: ns1
    predefined:
    - name: ns1
      plugins: [postgres]
      multiparty:
        enabled: true
  `))
	assert.NoError(t, err)

	nm.namespaces, err = nm.loadNamespaces(context.Background(), nm.dumpRootConfig(), nm.plugins)
	assert.Regexp(t, "FF10391", err)
}

func TestLoadNamespacesNonMultipartyUnknownPlugin(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
  namespaces:
    default: ns1
    predefined:
    - name: ns1
      plugins: [basicauth, erc721, bad]
  `))
	assert.NoError(t, err)

	nm.namespaces, err = nm.loadNamespaces(context.Background(), nm.dumpRootConfig(), nm.plugins)
	assert.Regexp(t, "FF10390.*unknown", err)
}

func TestLoadNamespacesNonMultipartyTokens(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
  namespaces:
    default: ns1
    predefined:
    - name: ns1
      plugins: [postgres, erc721]
  `))
	assert.NoError(t, err)

	nm.namespaces, err = nm.loadNamespaces(context.Background(), nm.dumpRootConfig(), nm.plugins)
	assert.NoError(t, err)
}

func TestStart(t *testing.T) {
	nm, nmm, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	waitInit := namespaceInitWaiter(t, nmm, []string{"default"})

	nmm.mdx.On("Start", mock.Anything).Return(nil)
	nmm.mdi.On("GetNamespace", mock.Anything, "default").Return(nil, nil)
	nmm.mdi.On("UpsertNamespace", mock.Anything, mock.AnythingOfType("*core.Namespace"), true).Return(nil)
	nmm.mo.On("PreInit", mock.Anything, mock.Anything).Return(nil)
	nmm.mo.On("Init").Return(nil)
	nmm.mo.On("Start", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		nm.cancelCtx()
	})

	err := nm.Start()
	assert.NoError(t, err)

	waitInit.Wait()
}

func TestStartDataExchangeFail(t *testing.T) {
	nm, nmm, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	nm.namespaces = nil
	nmm.mdx.On("Start").Return(fmt.Errorf("pop"))

	err := nm.startNamespacesAndPlugins(nm.namespaces, map[string]*plugin{
		"ffdx": nm.plugins["ffdx"],
	})
	assert.EqualError(t, err, "pop")

}

func TestStartOrchestratorFail(t *testing.T) {
	nm, nmm, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	nsStarted := make(chan struct{})
	nmm.mo.On("Start", mock.Anything).Return(fmt.Errorf("pop")).Run(func(args mock.Arguments) {
		nm.cancelCtx()
		close(nsStarted)
	})

	nmm.mdi.On("GetNamespace", mock.Anything, "default").Return(nil, nil)
	nmm.mdi.On("UpsertNamespace", mock.Anything, mock.AnythingOfType("*core.Namespace"), true).Return(nil)
	nmm.mo.On("PreInit", mock.Anything, mock.Anything).Return()
	nmm.mo.On("Init").Return(nil)
	err := nm.startNamespacesAndPlugins(nm.namespaces, map[string]*plugin{})
	assert.NoError(t, err)

	<-nsStarted
}

func TestWaitStop(t *testing.T) {
	nm, nmm, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	waitInit := namespaceInitWaiter(t, nmm, []string{"default"})

	nmm.mdi.On("GetNamespace", mock.Anything, "default").Return(nil, nil)
	nmm.mdi.On("UpsertNamespace", mock.Anything, mock.AnythingOfType("*core.Namespace"), true).Return(nil)
	nmm.mo.On("PreInit", mock.Anything, mock.Anything).Return()
	nmm.mo.On("Init").Return(nil)
	nmm.mo.On("Start", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		nm.cancelCtx()
	})

	nmm.mo.On("WaitStop").Return()
	nmm.mae.On("WaitStop").Return()

	err := nm.startNamespacesAndPlugins(nm.namespaces, map[string]*plugin{})
	assert.NoError(t, err)

	waitInit.Wait()

	nm.WaitStop()

	nmm.mo.AssertExpectations(t)
	nmm.mae.AssertExpectations(t)
}

func TestReset(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	childCtx, childCancel := context.WithCancel(context.Background())
	err := nm.Reset(childCtx)
	assert.NoError(t, err)
	childCancel()

	assert.True(t, <-nm.reset)
}

func TestResetRejectIfConfigAutoReload(t *testing.T) {
	coreconfig.Reset()
	config.Set(coreconfig.ConfigAutoReload, true)

	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()
	config.Set(coreconfig.ConfigAutoReload, true)

	err := nm.Reset(context.Background())
	assert.Regexp(t, "FF10438", err)

}

func TestLoadMetrics(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	nm.metrics = nil

	nm.loadManagers(context.Background())
}

func TestLoadAdminEvents(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	nm.adminEvents = nil

	nm.loadManagers(context.Background())
	assert.NotNil(t, nm.SPIEvents())
}

func TestGetNamespaces(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	results, err := nm.GetNamespaces(context.Background(), true)
	assert.Nil(t, err)
	assert.Len(t, results, 1)
}

func TestGetOperationByNamespacedID(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	mo := &orchestratormocks.Orchestrator{}
	nm.namespaces = map[string]*namespace{
		"default": {orchestrator: mo},
	}

	opID := fftypes.NewUUID()
	mo.On("GetOperationByID", context.Background(), opID.String()).Return(nil, nil)

	op, err := nm.GetOperationByNamespacedID(context.Background(), "default:"+opID.String())
	assert.Nil(t, err)
	assert.Nil(t, op)

	mo.AssertExpectations(t)
}

func TestGetOperationByNamespacedIDBadID(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	mo := &orchestratormocks.Orchestrator{}
	nm.namespaces = map[string]*namespace{
		"default": {orchestrator: mo},
	}

	_, err := nm.GetOperationByNamespacedID(context.Background(), "default:bad")
	assert.Regexp(t, "FF00138", err)

	mo.AssertExpectations(t)
}

func TestGetOperationByNamespacedIDNoOrchestrator(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	mo := &orchestratormocks.Orchestrator{}
	nm.namespaces = map[string]*namespace{
		"default": {orchestrator: mo},
	}

	opID := fftypes.NewUUID()

	_, err := nm.GetOperationByNamespacedID(context.Background(), "bad:"+opID.String())
	assert.Regexp(t, "FF10436", err)

	mo.AssertExpectations(t)
}

func TestResolveOperationByNamespacedID(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	mo := &orchestratormocks.Orchestrator{}
	mom := &operationmocks.Manager{}
	nm.namespaces = map[string]*namespace{
		"default": {orchestrator: mo},
	}

	opID := fftypes.NewUUID()
	mo.On("Operations").Return(mom)
	mom.On("ResolveOperationByID", context.Background(), opID, mock.Anything).Return(nil)

	err := nm.ResolveOperationByNamespacedID(context.Background(), "default:"+opID.String(), &core.OperationUpdateDTO{})
	assert.Nil(t, err)

	mo.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestResolveOperationByNamespacedIDBadID(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	mo := &orchestratormocks.Orchestrator{}
	nm.namespaces = map[string]*namespace{
		"default": {orchestrator: mo},
	}

	err := nm.ResolveOperationByNamespacedID(context.Background(), "default:bad", &core.OperationUpdateDTO{})
	assert.Regexp(t, "FF00138", err)

	mo.AssertExpectations(t)
}

func TestResolveOperationByNamespacedIDNoOrchestrator(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	mo := &orchestratormocks.Orchestrator{}
	nm.namespaces = map[string]*namespace{
		"default": {orchestrator: mo},
	}

	opID := fftypes.NewUUID()

	err := nm.ResolveOperationByNamespacedID(context.Background(), "bad:"+opID.String(), &core.OperationUpdateDTO{})
	assert.Regexp(t, "FF10436", err)

	mo.AssertExpectations(t)
}

func TestAuthorize(t *testing.T) {
	nm, nmm, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()
	nmm.mo.On("Authorize", mock.Anything, mock.Anything).Return(nil)
	nm.namespaces["ns1"] = &namespace{
		orchestrator: nmm.mo,
	}
	err := nm.Authorize(context.Background(), &fftypes.AuthReq{
		Namespace: "ns1",
	})
	assert.NoError(t, err)
}

func TestAuthorizeBadNamespace(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()
	err := nm.Authorize(context.Background(), &fftypes.AuthReq{
		Namespace: "ns1",
	})
	assert.Regexp(t, "FF10436", err)
}

func TestValidateNonMultipartyConfig(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	coreconfig.Reset()
	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(`
    namespaces:
      default: ns1
      predefined:
      - name: ns1
        plugins: [postgres, erc721]
    `))
	assert.NoError(t, err)

	_, err = nm.loadNamespaces(context.Background(), nm.dumpRootConfig(), nm.plugins)
	assert.NoError(t, err)
}

func TestOrchestratorWhileInitializing(t *testing.T) {
	nm, _, cleanup := newTestNamespaceManager(t, true)
	defer cleanup()

	_, err := nm.Orchestrator(nm.ctx, "default", false)
	assert.Regexp(t, "FF10441", err)
}
