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

package namespace

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/sirupsen/logrus"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var exampleConfig1base = `
---
http:
  address: 0.0.0.0
  port: 5000
  publicURL: https://myfirefly.example.com
log:
  level: debug
metrics:
  enabled: true
  address: 127.0.0.1
  port: 6000
  path: /metrics
namespaces:
  default: ns1
  predefined:
    - defaultKey: 0xbEa50Ec98776beF144Fc63078e7b15291Ac64cfA
      name: ns1
      plugins:
        - sharedstorage-ns1
        - database0
        - blockchain-ns1
        - ff-dx
        - erc1155-ns1
        - erc20_erc721-ns1
        - test_user_auth-ns1
      multiparty:
        enabled: true
        node:
          name: node1
        org:
          name: org1
          key: 0xbEa50Ec98776beF144Fc63078e7b15291Ac64cfA
        contract:
          - firstevent: "0"
            location:
              address: 0x7359d2ecc199C48369b390522c29b77A5Af30882
    - defaultKey: 0x630659A26fa005d50Fa9706D8a4e242fd4169A61
      name: ns2
      plugins:
        - database0
        - blockchain-ns2
        - test_user_auth-ns2
      multiparty: {}
node: {}
plugins:
  blockchain:
    - ethereum:
        ethconnect:
          topic: "0"
          url: https://ethconnect1.example.com:5000
      name: blockchain-ns1
      type: ethereum
    - ethereum:
        ethconnect:
          topic: "0"
          url: https://ethconnect2.example.com:5000
      name: blockchain-ns2
      type: ethereum
  database:
    - name: database0
      postgres:
        url: postgres://postgrs.example.com:5432/firefly?sslmode=require
      type: postgres
  dataexchange:
    - ffdx:
        url: https://ffdx:3000
        initEnabled: true
      name: ff-dx
      type: ffdx
  sharedstorage:
    - ipfs:
        api:
          url: https://ipfs1.example.com
          auth:
            username: someuser
            password: somepass
        gateway:
          url: https://ipfs1.example.com
          auth:
          username: someuser
          password: somepass
      name: sharedstorage-ns1
      type: ipfs
    - ipfs:
        api:
          url: https://ipfs2.example.com
        gateway:
          url: https://ipfs2.example.com
      name: sharedstorage-ns2
      type: ipfs
  tokens:
    - fftokens:
        url: https://ff-tokens-ns1-erc1155:3000
      name: erc1155-ns1
      type: fftokens
    - fftokens:
        url: https://ff-tokens-ns1-erc20-erc721:3000
      name: erc20_erc721-ns1
      type: fftokens
    - fftokens:
        url: https://ff-tokens-ns2-erc20-erc721:3000
      name: erc20_erc721-ns2
      type: fftokens
  auth:
    - name: test_user_auth-ns1
      type: basic
      basic:
        passwordfile: /etc/firefly/test_users
    - name: test_user_auth-ns2
      type: basic
      basic:
        passwordfile: /etc/firefly/test_users              
ui:
  path: ./frontend
`

// here we deliberately make a bunch of ordering changes in fields,
// but the only actual difference is the creation of a third namespace,
// and some NEW plugins
var exampleConfig2extraNS = `
---
http:
  address: 0.0.0.0
  port: 5000
  publicURL: https://myfirefly.example.com
ui:
  path: ./frontend  
namespaces:
  default: ns1
  predefined:
    - defaultKey: 0xbEa50Ec98776beF144Fc63078e7b15291Ac64cfA
      name: ns1
      plugins:
        - sharedstorage-ns1
        - database0
        - blockchain-ns1
        - ff-dx
        - erc1155-ns1
        - erc20_erc721-ns1
        - test_user_auth-ns1
      multiparty:
        enabled: true
        node:
          name: node1
        org:
          name: org1
          key: 0xbEa50Ec98776beF144Fc63078e7b15291Ac64cfA
        contract:
          - firstevent: "0"
            location:
              address: 0x7359d2ecc199C48369b390522c29b77A5Af30882
    - defaultKey: 0x630659A26fa005d50Fa9706D8a4e242fd4169A61
      plugins:
        - database0
        - blockchain-ns2
        - test_user_auth-ns2
      name: ns2
      multiparty: {}
    - defaultKey: 0xF49C223038FA129c2Ba23D5c5f3Cdb50120F3EDe
      name: ns3
      plugins:
        - database0
        - blockchain-ns3
        - test_user_auth-ns3
      multiparty: {}      
node: {}
log:
  level: debug
metrics:
  enabled: true
  address: 127.0.0.1
  port: 6000
  path: /metrics
plugins:
  blockchain:
    - ethereum:
        ethconnect:
          topic: "0"
          url: https://ethconnect1.example.com:5000
      name: blockchain-ns1
      type: ethereum
    - ethereum:
        ethconnect:
          topic: "0"
          url: https://ethconnect2.example.com:5000
      name: blockchain-ns2
      type: ethereum
    - ethereum:
      ethconnect:
        topic: "0"
        url: https://ethconnect3.example.com:5000
      name: blockchain-ns3
      type: ethereum
  database:
    - name: database0
      type: postgres
      postgres:
        url: postgres://postgrs.example.com:5432/firefly?sslmode=require
  dataexchange:
    - ffdx:
        url: https://ffdx:3000
        initEnabled: true
      name: ff-dx
      type: ffdx
  sharedstorage:
    - ipfs:
        gateway:
          url: https://ipfs1.example.com
          auth:
          username: someuser
          password: somepass
        api:
          url: https://ipfs1.example.com
          auth:
            username: someuser
            password: somepass
      type: ipfs
      name: sharedstorage-ns1
    - ipfs:
        api:
          url: https://ipfs2.example.com
        gateway:
          url: https://ipfs2.example.com
      name: sharedstorage-ns2
      type: ipfs
  tokens:
    - fftokens:
        url: https://ff-tokens-ns1-erc1155:3000
      type: fftokens
      name: erc1155-ns1
    - fftokens:
        url: https://ff-tokens-ns1-erc20-erc721:3000
      type: fftokens
      name: erc20_erc721-ns1
    - fftokens:
        url: https://ff-tokens-ns2-erc20-erc721:3000
      type: fftokens
      name: erc20_erc721-ns2
  auth:
    - name: test_user_auth-ns1
      basic:
        passwordfile: /etc/firefly/test_users
      type: basic
    - name: test_user_auth-ns2
      basic:
        passwordfile: /etc/firefly/test_users
      type: basic
    - name: test_user_auth-ns3
      type: basic
      basic:
        passwordfile: /etc/firefly/test_users
`

var exampleConfig3NSchanges = `
---
http:
  address: 0.0.0.0
  port: 5000
  publicURL: https://myfirefly.example.com
log:
  level: debug
metrics:
  enabled: true
  address: 127.0.0.1
  port: 6000
  path: /metrics
namespaces:
  default: ns1
  predefined:
    - defaultKey: 0x763617D0e180F4909D796F8c46b6b2d17c61b5f2 # new default key 
      name: ns1
      plugins:
        - sharedstorage-ns1
        - database0
        - blockchain-ns1
        - ff-dx
        - erc1155-ns1
        - erc20_erc721-ns1
        - test_user_auth-ns1
      multiparty:
        enabled: true
        node:
          name: node1
        org:
          name: org1
          key: 0xbEa50Ec98776beF144Fc63078e7b15291Ac64cfA
        contract:
          - firstevent: "0"
            location:
              address: 0x7359d2ecc199C48369b390522c29b77A5Af30882
    - defaultKey: 0x630659A26fa005d50Fa9706D8a4e242fd4169A61
      name: ns2
      plugins:
        - database0
        - blockchain-ns2
        - test_user_auth-ns2 # config changed
      multiparty: {}
node: {}
plugins:
  blockchain:
    - ethereum:
        ethconnect:
          topic: "0"
          url: https://ethconnect1.example.com:5000
      name: blockchain-ns1
      type: ethereum
    - ethereum:
        ethconnect:
          topic: "0"
          url: https://ethconnect2.example.com:5000
      name: blockchain-ns2
      type: ethereum
  database:
    - name: database0
      postgres:
        url: postgres://postgrs.example.com:5432/firefly?sslmode=require
      type: postgres
  dataexchange:
    - ffdx:
        url: https://ffdx:3000
        initEnabled: true
      name: ff-dx
      type: ffdx
  sharedstorage:
    - ipfs:
        api:
          url: https://ipfs1.example.com
          auth:
            username: someuser
            password: somepass
        gateway:
          url: https://ipfs1.example.com
          auth:
          username: someuser
          password: somepass
      name: sharedstorage-ns1
      type: ipfs
    - ipfs:
        api:
          url: https://ipfs2.example.com
        gateway:
          url: https://ipfs2.example.com
      name: sharedstorage-ns2
      type: ipfs
  tokens:
    - fftokens:
        url: https://ff-tokens-ns1-erc1155:3000
      name: erc1155-ns1
      type: fftokens
    - fftokens:
        url: https://ff-tokens-ns1-erc20-erc721:3000
      name: erc20_erc721-ns1
      type: fftokens
    - fftokens:
        url: https://ff-tokens-ns2-erc20-erc721:3000
      name: erc20_erc721-ns2
      type: fftokens
  auth:
    - name: test_user_auth-ns1
      type: basic
      basic:
        passwordfile: /etc/firefly/test_users
    - name: test_user_auth-ns2
      type: basic
      basic:
        passwordfile: /etc/firefly/test_users_new 
ui:
  path: ./frontend
`

func mockInitConfig(nmm *nmMocks) {
	nmm.mdi.On("Init", mock.Anything, mock.Anything).Return(nil)
	nmm.mdi.On("SetHandler", database.GlobalHandler, mock.Anything).Return()
	nmm.mbi.On("Init", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	nmm.mdx.On("Init", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	nmm.mps.On("Init", mock.Anything, mock.Anything).Return(nil)
	nmm.mti[1].On("Init", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil)
	nmm.mei[0].On("Init", mock.Anything, mock.Anything).Return(nil)
	nmm.mei[1].On("Init", mock.Anything, mock.Anything).Return(nil)
	nmm.mei[2].On("Init", mock.Anything, mock.Anything).Return(nil)
	nmm.mdi.On("GetNamespace", mock.Anything, "ns1").Return(nil, nil)
	nmm.mdi.On("GetNamespace", mock.Anything, "ns2").Return(nil, nil)
	nmm.mdi.On("GetNamespace", mock.Anything, "ns3").Return(nil, nil).Maybe()
	nmm.mdi.On("UpsertNamespace", mock.Anything, mock.AnythingOfType("*core.Namespace"), true).Return(nil)
	nmm.mai.On("Init", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	nmm.mdx.On("Start").Return(nil)

	nmm.mo.On("PreInit", mock.Anything, mock.Anything, mock.Anything).Return()
	nmm.mo.On("Init").
		Run(func(args mock.Arguments) {
			if nmm.nm != nil && nmm.nm.namespaces["ns1"] != nil && nmm.nm.namespaces["ns1"].Contracts != nil {
				nmm.nm.namespaces["ns1"].Contracts.Active = &core.MultipartyContract{
					Info: core.MultipartyContractInfo{
						Version: 2,
					},
				}
			}
		}).
		Return(nil)
	nmm.mo.On("Start").Return(nil)
	nmm.mo.On("WaitStop").Return(nil).Maybe()
}

func namespaceInitWaiter(t *testing.T, nmm *nmMocks, namespaces []string) *sync.WaitGroup {
	wg := &sync.WaitGroup{}
	for _, ns := range namespaces {
		_ns := ns
		count := len(nmm.mei)
		for i, mei := range nmm.mei {
			idx := i + 1
			wg.Add(1)
			mei.On("NamespaceRestarted", _ns, mock.Anything).Return().Run(func(args mock.Arguments) {
				log.L(context.Background()).Infof("WAITER: Namespace started (cb=%d/%d) '%s'", idx, count, _ns)
				wg.Done()
			}).Once()
		}
	}
	return wg
}

func renameWriteFile(t *testing.T, filename string, data []byte) {
	tempFile := fmt.Sprintf("%s-%s", t.TempDir(), fftypes.NewUUID())
	err := ioutil.WriteFile(tempFile, data, 0664)
	assert.NoError(t, err)
	err = os.Rename(tempFile, filename)
	assert.NoError(t, err)
}

func TestConfigListenerE2E(t *testing.T) {

	testDir := t.TempDir()
	configFilename := fmt.Sprintf("%s/firefly.core", testDir)
	renameWriteFile(t, configFilename, []byte(exampleConfig1base))

	coreconfig.Reset()
	InitConfig()
	err := config.ReadConfig("core", configFilename)
	assert.NoError(t, err)
	config.Set(coreconfig.ConfigAutoReload, true)

	nm := NewNamespaceManager().(*namespaceManager)
	nmm := mockPluginFactories(nm)
	mockInitConfig(nmm)
	waitInit := namespaceInitWaiter(t, nmm, []string{"ns1", "ns2"})

	ctx, cancelCtx := context.WithCancel(context.Background())
	err = nm.Init(ctx, cancelCtx, make(chan bool), func() error {
		coreconfig.Reset()
		return config.ReadConfig("core", configFilename)
	})
	assert.NoError(t, err)
	defer func() {
		cancelCtx()
		nm.WaitStop()
	}()

	err = nm.Start()
	assert.NoError(t, err)

	waitInit.Wait()

	waitInit = namespaceInitWaiter(t, nmm, []string{"ns3"})

	renameWriteFile(t, configFilename, []byte(exampleConfig2extraNS))

	waitInit.Wait()

}

func TestConfigListenerUnreadableYAML(t *testing.T) {

	testDir := t.TempDir()
	configFilename := fmt.Sprintf("%s/firefly.core", testDir)
	viper.SetConfigFile(configFilename)

	coreconfig.Reset()
	InitConfig()
	config.Set(coreconfig.ConfigAutoReload, true)

	nm := NewNamespaceManager().(*namespaceManager)
	nmm := mockPluginFactories(nm)
	mockInitConfig(nmm)

	ctx, cancelCtx := context.WithCancel(context.Background())
	err := nm.Init(ctx, cancelCtx, make(chan bool), func() error {
		coreconfig.Reset()
		return config.ReadConfig("core", configFilename)
	})
	assert.NoError(t, err)

	err = nm.Start()
	assert.NoError(t, err)

	renameWriteFile(t, configFilename, []byte(`--\n: ! YAML !!!: !`))

	// Should stop itself
	<-nm.ctx.Done()
	nm.WaitStop()

}

func TestConfigReload1to2(t *testing.T) {
	logrus.SetLevel(logrus.TraceLevel)

	nm, nmm, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()

	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(exampleConfig1base))
	assert.NoError(t, err)

	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	mockInitConfig(nmm)
	waitInit := namespaceInitWaiter(t, nmm, []string{"ns1", "ns2"})

	err = nm.Init(ctx, cancelCtx, make(chan bool), func() error { return nil })
	assert.NoError(t, err)

	err = nm.Start()
	assert.NoError(t, err)

	waitInit.Wait()
	waitInit = namespaceInitWaiter(t, nmm, []string{"ns3"})

	originalPlugins := nm.plugins
	originalPluginHashes := make(map[string]*fftypes.Bytes32)
	for k, v := range originalPlugins {
		originalPluginHashes[k] = v.configHash
	}
	originaNS := nm.namespaces
	originalNSHashes := make(map[string]*fftypes.Bytes32)
	for k, v := range originaNS {
		originalNSHashes[k] = v.configHash
	}

	coreconfig.Reset()
	InitConfig()
	viper.SetConfigType("yaml")
	err = viper.ReadConfig(strings.NewReader(exampleConfig2extraNS))
	assert.NoError(t, err)

	// Drive the config reload
	nm.configReloaded(nm.ctx)

	// Check that we didn't cancel the context
	select {
	case <-nm.ctx.Done():
		assert.Fail(t, "Error occurred in config reload")
	default:
	}

	// Check none of the plugins reloaded
	for name := range originalPlugins {
		assert.True(t, originalPlugins[name] == nm.plugins[name], name)
		assert.Equal(t, originalPluginHashes[name], nm.plugins[name].configHash, name)
	}

	// Check we have two more than before
	assert.Len(t, nm.plugins, len(originalPlugins)+2)
	assert.NotNil(t, nm.plugins["blockchain-ns3"])
	assert.NotNil(t, nm.plugins["test_user_auth-ns3"])

	// Check none of the namespaces reloaded
	for name := range originaNS {
		assert.True(t, originaNS[name] == nm.namespaces[name], name)
		assert.Equal(t, originalNSHashes[name], nm.namespaces[name].configHash, name)
	}

	// Check we have one more than before
	assert.Len(t, nm.namespaces, len(originaNS)+1)
	assert.NotNil(t, nm.namespaces["ns3"])

	waitInit.Wait()

}

func TestConfigReload1to3(t *testing.T) {
	logrus.SetLevel(logrus.TraceLevel)

	nm, nmm, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()

	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(exampleConfig1base))
	assert.NoError(t, err)

	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	mockInitConfig(nmm)
	waitInit := namespaceInitWaiter(t, nmm, []string{"ns1", "ns2"})

	err = nm.Init(ctx, cancelCtx, make(chan bool), func() error { return nil })
	assert.NoError(t, err)

	err = nm.Start()
	assert.NoError(t, err)
	waitInit.Wait()

	originalPlugins := nm.plugins
	originalPluginHashes := make(map[string]*fftypes.Bytes32)
	for k, v := range originalPlugins {
		originalPluginHashes[k] = v.configHash
	}
	originaNS := nm.namespaces
	originalNSHashes := make(map[string]*fftypes.Bytes32)
	for k, v := range originaNS {
		originalNSHashes[k] = v.configHash
	}

	waitInit = namespaceInitWaiter(t, nmm, []string{"ns1", "ns2"})
	coreconfig.Reset()
	InitConfig()
	viper.SetConfigType("yaml")
	err = viper.ReadConfig(strings.NewReader(exampleConfig3NSchanges))
	assert.NoError(t, err)

	// Drive the config reload
	nm.configReloaded(nm.ctx)

	// Check that we didn't cancel the context
	select {
	case <-nm.ctx.Done():
		assert.Fail(t, "Error occurred in config reload")
	default:
	}

	// Check none of the plugins reloaded
	for name := range originalPlugins {
		if name != "test_user_auth-ns2" {
			assert.True(t, originalPlugins[name] == nm.plugins[name], name)
			assert.Equal(t, originalPluginHashes[name], nm.plugins[name].configHash, name)
		} else {
			assert.False(t, originalPlugins[name] == nm.plugins[name], name)
			assert.NotEqual(t, originalPluginHashes[name], nm.plugins[name].configHash, name)
		}
	}

	// Check both namespaces reloaded
	assert.Len(t, nm.namespaces, len(originaNS))
	assert.False(t, originaNS["ns1"] == nm.namespaces["ns1"])
	assert.NotEqual(t, originalNSHashes["ns1"], nm.namespaces["ns1"].configHash)

	waitInit.Wait()
}

func TestConfigReloadBadNewConfigPlugins(t *testing.T) {
	logrus.SetLevel(logrus.TraceLevel)

	nm, nmm, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()

	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(exampleConfig1base))
	assert.NoError(t, err)

	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	mockInitConfig(nmm)
	waitInit := namespaceInitWaiter(t, nmm, []string{"ns1", "ns2"})

	err = nm.Init(ctx, cancelCtx, make(chan bool), func() error { return nil })
	assert.NoError(t, err)

	originalPlugins := nm.plugins
	originaNS := nm.namespaces

	err = nm.Start()
	assert.NoError(t, err)

	waitInit.Wait()

	coreconfig.Reset()
	InitConfig()
	viper.SetConfigType("yaml")
	err = viper.ReadConfig(strings.NewReader(`
plugins:
  database: [{"type": "invalid"}]
`))
	assert.NoError(t, err)

	// Drive the config reload
	nm.configReloaded(nm.ctx)

	// Check that we didn't cancel the context
	select {
	case <-nm.ctx.Done():
		assert.Fail(t, "Config parse failure should not have crashed the system")
	default:
	}

	// Check we didn't lose our plugins
	assert.Len(t, nm.plugins, len(originalPlugins))
	assert.Len(t, nm.namespaces, len(originaNS))

}

func TestConfigReloadBadNSMissingRequiredPlugins(t *testing.T) {
	logrus.SetLevel(logrus.TraceLevel)

	nm, nmm, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()

	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(exampleConfig1base))
	assert.NoError(t, err)

	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	mockInitConfig(nmm)
	waitInit := namespaceInitWaiter(t, nmm, []string{"ns1", "ns2"})

	err = nm.Init(ctx, cancelCtx, make(chan bool), func() error { return nil })
	assert.NoError(t, err)

	originalPlugins := nm.plugins
	originaNS := nm.namespaces

	err = nm.Start()
	assert.NoError(t, err)

	waitInit.Wait()

	coreconfig.Reset()
	InitConfig()
	viper.SetConfigType("yaml")
	err = viper.ReadConfig(strings.NewReader(`
namespaces:
  predefined:
  - defaultKey: 0xbEa50Ec98776beF144Fc63078e7b15291Ac64cfA
    name: ns1
    plugins:
      - sharedstorage-ns1
      - database0
      - blockchain-ns1
      - ff-dx
      - erc1155-ns1
      - erc20_erc721-ns1
      - test_user_auth-ns1
`))
	assert.NoError(t, err)

	// Drive the config reload
	nm.configReloaded(nm.ctx)

	// Check that we didn't cancel the context
	select {
	case <-nm.ctx.Done():
		assert.Fail(t, "Config parse failure should not have crashed the system")
	default:
	}

	// Check we didn't lose our plugins
	assert.Len(t, nm.plugins, len(originalPlugins))
	assert.Len(t, nm.namespaces, len(originaNS))

}

func mockPurge(nmm *nmMocks, nsName string) {
	matchNil := mock.MatchedBy(func(i interface{}) bool {
		return i == nil || reflect.ValueOf(i).IsNil()
	})
	nmm.mdi.On("SetHandler", nsName, matchNil).Return()
	nmm.mbi.On("SetHandler", nsName, matchNil).Return()
	nmm.mbi.On("SetOperationHandler", nsName, matchNil).Return()
	nmm.mps.On("SetHandler", nsName, matchNil).Return().Maybe()
	nmm.mps.On("SetOperationHandler", nsName, matchNil).Return().Maybe()
	nmm.mdx.On("SetHandler", nsName, mock.Anything, matchNil).Return().Maybe()
	nmm.mdx.On("SetOperationHandler", nsName, matchNil).Return().Maybe()
	for _, mti := range nmm.mti {
		mti.On("SetHandler", nsName, matchNil).Return().Maybe()
		mti.On("SetOperationHandler", nsName, matchNil).Return().Maybe()
		mti.On("StartNamespace", mock.Anything, nsName).Return(nil).Maybe()
	}
}

func TestConfigDownToNothingOk(t *testing.T) {
	logrus.SetLevel(logrus.TraceLevel)

	nm, nmm, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()

	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(exampleConfig1base))
	assert.NoError(t, err)

	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	mockInitConfig(nmm)

	// Because the namespaces are deleted, we should get calls with nil on all the plugins
	mockPurge(nmm, "ns1")
	mockPurge(nmm, "ns2")
	waitInit := namespaceInitWaiter(t, nmm, []string{"ns1", "ns2"})

	err = nm.Init(ctx, cancelCtx, make(chan bool), func() error { return nil })
	assert.NoError(t, err)

	err = nm.Start()
	assert.NoError(t, err)
	waitInit.Wait()

	coreconfig.Reset()
	InitConfig()
	viper.SetConfigType("yaml")
	// Nothing - no plugins, no namespaces
	assert.NoError(t, err)

	nmm.mo.On("WaitStop").Return(nil)

	// Drive the config reload
	nm.configReloaded(nm.ctx)

	// Check that we didn't cancel the context
	select {
	case <-nm.ctx.Done():
		assert.Fail(t, "Should have been happy destroying everything")
	default:
	}

	// Check we didn't lose our plugins
	assert.Len(t, nm.plugins, len(nmm.mei)) // Just the events plugins
	assert.Empty(t, nm.namespaces, 0)

}

func TestConfigStartPluginsFails(t *testing.T) {
	logrus.SetLevel(logrus.TraceLevel)

	nm, nmm, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()

	viper.SetConfigType("yaml")
	err := viper.ReadConfig(strings.NewReader(exampleConfig1base))
	assert.NoError(t, err)

	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	mockInitConfig(nmm)
	mockPurge(nmm, "ns1")
	mockPurge(nmm, "ns2")

	waitInit := namespaceInitWaiter(t, nmm, []string{"ns1", "ns2"})

	err = nm.Init(ctx, cancelCtx, make(chan bool), func() error { return nil })
	assert.NoError(t, err)

	err = nm.Start()
	assert.NoError(t, err)
	waitInit.Wait()

	coreconfig.Reset()
	InitConfig()
	viper.SetConfigType("yaml")
	// Nothing - no plugins, no namespaces
	assert.NoError(t, err)

	nmm.mo.On("WaitStop").Return(nil)

	// Drive the config reload
	nm.configReloaded(nm.ctx)

	// Check that we didn't cancel the context
	select {
	case <-nm.ctx.Done():
		assert.Fail(t, "Should have been happy destroying everything")
	default:
	}

	// Check we didn't lose our plugins
	assert.Len(t, nm.plugins, len(nmm.mei)) // Just the events plugins
	assert.Empty(t, nm.namespaces, 0)

}

func TestConfigReloadInitPluginsFailOnReload(t *testing.T) {
	logrus.SetLevel(logrus.TraceLevel)

	nm, nmm, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()

	// Start with empty config
	for _, mei := range nmm.mei {
		mei.On("Init", mock.Anything, mock.Anything).Return(nil).Maybe()
	}

	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	err := nm.Init(ctx, cancelCtx, make(chan bool), func() error { return nil })
	assert.NoError(t, err)

	err = nm.Start()
	assert.NoError(t, err)

	coreconfig.Reset()
	InitConfig()
	viper.SetConfigType("yaml")
	err = viper.ReadConfig(strings.NewReader(`
plugins:
  database:
  - name: "badness"
    type: "postgres"
`))
	assert.NoError(t, err)

	// Drive the config reload
	nmm.mdi.On("Init", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
	nm.configReloaded(nm.ctx)

	// Should terminate
	<-nm.ctx.Done()
	nmm.mae.On("WaitStop").Return(nil).Maybe()
	nmm.mo.On("WaitStop").Return(nil).Maybe()
	nm.WaitStop()

}

func TestConfigReloadInitNamespacesFailOnReload(t *testing.T) {
	logrus.SetLevel(logrus.TraceLevel)

	nm, nmm, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()

	// Start with empty config
	for _, mei := range nmm.mei {
		mei.On("Init", mock.Anything, mock.Anything).Return(nil).Maybe()
	}

	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	err := nm.Init(ctx, cancelCtx, make(chan bool), func() error { return nil })
	assert.NoError(t, err)

	err = nm.Start()
	assert.NoError(t, err)

	coreconfig.Reset()
	InitConfig()
	viper.SetConfigType("yaml")
	err = viper.ReadConfig(strings.NewReader(`
plugins:
  database:
  - name: "postgres"
    type: "postgres"
namespaces:
  predefined:
  - name: default
    plugins:
    - postgres
  `))
	assert.NoError(t, err)

	// Drive the config reload
	nmm.mdi.On("Init", mock.Anything, mock.Anything).Return(nil)
	nmm.mdi.On("SetHandler", database.GlobalHandler, mock.Anything).Return()
	nmm.mdi.On("GetNamespace", mock.Anything, "default").Run(func(args mock.Arguments) {
		nm.cancelCtx()
	}).Return(nil, fmt.Errorf("pop"))
	nm.configReloaded(nm.ctx)

	// Should terminate
	<-nm.ctx.Done()
	nmm.mae.On("WaitStop").Return(nil).Maybe()
	nmm.mo.On("WaitStop").Return(nil).Maybe()
	nm.WaitStop()

}

func TestConfigReloadInitNamespacesFailOnStart(t *testing.T) {
	logrus.SetLevel(logrus.TraceLevel)

	nm, nmm, cleanup := newTestNamespaceManager(t, false)
	defer cleanup()

	// Start with empty config
	for _, mei := range nmm.mei {
		mei.On("Init", mock.Anything, mock.Anything).Return(nil).Maybe()
	}

	ctx, cancelCtx := context.WithCancel(context.Background())
	defer cancelCtx()

	err := nm.Init(ctx, cancelCtx, make(chan bool), func() error { return nil })
	assert.NoError(t, err)

	err = nm.Start()
	assert.NoError(t, err)

	coreconfig.Reset()
	InitConfig()
	viper.SetConfigType("yaml")
	err = viper.ReadConfig(strings.NewReader(`
plugins:
  database:
  - name: "postgres"
    type: "postgres"
namespaces:
  predefined:
  - name: default
    plugins:
    - postgres
  `))
	assert.NoError(t, err)

	// Drive the config reload
	nmm.mdi.On("Init", mock.Anything, mock.Anything).Return(nil)
	nmm.mdi.On("SetHandler", database.GlobalHandler, mock.Anything).Return()
	nmm.mdi.On("GetNamespace", mock.Anything, "default").Return(nil, nil)
	nmm.mdi.On("UpsertNamespace", mock.Anything, mock.AnythingOfType("*core.Namespace"), true).Return(nil)
	nmm.mo.On("PreInit", mock.Anything, mock.Anything).Return()
	nmm.mo.On("Init").Return(nil)
	nmm.mo.On("Start", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		nm.cancelCtx()
	}).Return(fmt.Errorf("pop"))
	nm.configReloaded(nm.ctx)

	// Should terminate
	<-nm.ctx.Done()
	nmm.mae.On("WaitStop").Return(nil).Maybe()
	nmm.mo.On("WaitStop").Return(nil).Maybe()
	nm.WaitStop()

}
