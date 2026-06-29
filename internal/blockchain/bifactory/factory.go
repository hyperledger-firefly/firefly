// Copyright © 2025 Kaleido, Inc.
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

package bifactory

import (
	"context"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/firefly/internal/blockchain/cardano"
	"github.com/hyperledger-firefly/firefly/internal/blockchain/ethereum"
	"github.com/hyperledger-firefly/firefly/internal/blockchain/fabric"
	"github.com/hyperledger-firefly/firefly/internal/blockchain/tezos"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/blockchain"
)

var pluginsByType = map[string]func() blockchain.Plugin{
	(*cardano.Cardano)(nil).Name():   func() blockchain.Plugin { return &cardano.Cardano{} },
	(*ethereum.Ethereum)(nil).Name(): func() blockchain.Plugin { return &ethereum.Ethereum{} },
	(*fabric.Fabric)(nil).Name():     func() blockchain.Plugin { return &fabric.Fabric{} },
	(*tezos.Tezos)(nil).Name():       func() blockchain.Plugin { return &tezos.Tezos{} },
}

func InitConfig(config config.ArraySection) {
	config.AddKnownKey(coreconfig.PluginConfigName)
	config.AddKnownKey(coreconfig.PluginConfigType)
	for name, plugin := range pluginsByType {
		plugin().InitConfig(config.SubSection(name))
	}
}

func GetPlugin(ctx context.Context, pluginType string) (blockchain.Plugin, error) {
	plugin, ok := pluginsByType[pluginType]
	if !ok {
		return nil, i18n.NewError(ctx, coremsgs.MsgUnknownBlockchainPlugin, pluginType)
	}
	return plugin(), nil
}
