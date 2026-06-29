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

package eifactory

import (
	"context"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/internal/events/system"
	"github.com/hyperledger-firefly/firefly/internal/events/webhooks"
	"github.com/hyperledger-firefly/firefly/internal/events/websockets"
	"github.com/hyperledger-firefly/firefly/pkg/events"
)

var plugins = []events.Plugin{
	&websockets.WebSockets{},
	&webhooks.WebHooks{},
	&system.Events{},
}

var pluginsByName = make(map[string]events.Plugin)

func init() {
	for _, p := range plugins {
		pluginsByName[p.Name()] = p
	}
}

func InitConfig(config config.Section) {
	for name, plugin := range pluginsByName {
		plugin.InitConfig(config.SubSection(name))
	}
}

func GetPlugin(ctx context.Context, pluginType string) (events.Plugin, error) {
	plugin, ok := pluginsByName[pluginType]
	if !ok {
		return nil, i18n.NewError(ctx, coremsgs.MsgUnknownEventTransportPlugin, pluginType)
	}
	return plugin, nil
}
