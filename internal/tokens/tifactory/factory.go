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

package tifactory

import (
	"context"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/internal/tokens/fftokens"
	"github.com/hyperledger-firefly/firefly/pkg/tokens"
)

var pluginsByName = map[string]func() tokens.Plugin{
	(*fftokens.FFTokens)(nil).Name(): func() tokens.Plugin { return &fftokens.FFTokens{} },
}

func InitConfig(config config.ArraySection) {
	config.AddKnownKey(coreconfig.PluginConfigName)
	config.AddKnownKey(coreconfig.PluginConfigType)
	config.AddKnownKey(coreconfig.PluginBroadcastName)
	for name, plugin := range pluginsByName {
		plugin().InitConfig(config.SubSection(name))
	}
}

func GetPlugin(ctx context.Context, connectorName string) (tokens.Plugin, error) {
	plugin, ok := pluginsByName[connectorName]
	if !ok {
		return nil, i18n.NewError(ctx, coremsgs.MsgUnknownTokensPlugin, connectorName)
	}
	return plugin(), nil
}
