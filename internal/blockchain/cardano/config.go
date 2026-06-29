// Copyright © 2025 IOG Singapore and SundaeSwap, Inc.
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

package cardano

import (
	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/wsclient"
)

const (
	defaultBatchSize    = 50
	defaultBatchTimeout = 500
)

const (
	// CardanoconnectConfigKey is a sub-key in the config to contain all the cardanoconnect specific config
	CardanoconnectConfigKey = "cardanoconnect"
	// CardanoconnectConfigTopic is the websocket listen topic that the node should register on, which is important if there are multiple
	// nodes using a single cardanoconnect
	CardanoconnectConfigTopic = "topic"
	// CardanoconnectConfigBatchSize is the batch size to configure on event streams, when auto-defining them
	CardanoconnectConfigBatchSize = "batchSize"
	// CardanoconnectConfigBatchTimeout is the batch timeout to configure on event streams, when auto-defining them
	CardanoconnectConfigBatchTimeout = "batchTimeout"
)

func (c *Cardano) InitConfig(config config.Section) {
	c.cardanoconnectConf = config.SubSection(CardanoconnectConfigKey)
	wsclient.InitConfig(c.cardanoconnectConf)
	c.cardanoconnectConf.AddKnownKey(CardanoconnectConfigTopic)
	c.cardanoconnectConf.AddKnownKey(CardanoconnectConfigBatchSize, defaultBatchSize)
	c.cardanoconnectConf.AddKnownKey(CardanoconnectConfigBatchTimeout, defaultBatchTimeout)
}
