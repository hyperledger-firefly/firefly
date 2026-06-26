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

package ethereum

import (
	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-common/pkg/ffresty"
	"github.com/hyperledger/firefly-common/pkg/wsclient"
)

const (
	defaultBatchSize    = 50
	defaultBatchTimeout = 500
	defaultPrefixShort  = "fly"
	defaultPrefixLong   = "firefly"
	defaultFromBlock    = "0"

	defaultAddressResolverMethod        = "GET"
	defaultAddressResolverResponseField = "address"

	defaultBackgroundInitialDelay = "5s"
	defaultBackgroundRetryFactor  = 2.0
	defaultBackgroundMaxDelay     = "1m"
)

const (
	// EthconnectConfigKey is a sub-key in the config to contain all the ethconnect specific config
	EthconnectConfigKey = "ethconnect"
	// EthconnectConfigTopic is the websocket listen topic that the node should register on, which is important if there are multiple
	// nodes using a single ethconnect
	EthconnectConfigTopic = "topic"
	// EthconnectConfigBatchSize is the batch size to configure on event streams, when auto-defining them
	EthconnectConfigBatchSize = "batchSize"
	// EthconnectConfigBatchTimeout is the batch timeout to configure on event streams, when auto-defining them
	EthconnectConfigBatchTimeout = "batchTimeout"
	// EthconnectPrefixShort is used in the query string in requests to ethconnect
	EthconnectPrefixShort = "prefixShort"
	// EthconnectPrefixLong is used in HTTP headers in requests to ethconnect
	EthconnectPrefixLong = "prefixLong"
	// EthconnectConfigInstanceDeprecated is the ethereum address of the FireFly contract
	EthconnectConfigInstanceDeprecated = "instance"
	// EthconnectConfigFromBlockDeprecated is the configuration of the first block to listen to when creating the listener for the FireFly contract
	EthconnectConfigFromBlockDeprecated = "fromBlock"
	// EthconnectBackgroundStart is used to not fail the ethereum plugin on init and retry to start it in the background
	EthconnectBackgroundStart = "backgroundStart.enabled"
	// EthconnectBackgroundStartInitialDelay is delay between restarts in the case where we retry to restart in the ethereum plugin
	EthconnectBackgroundStartInitialDelay = "backgroundStart.initialDelay"
	// EthconnectBackgroundStartMaxDelay is the max delay between restarts in the case where we retry to restart in the ethereum plugin
	EthconnectBackgroundStartMaxDelay = "backgroundStart.maxDelay"
	// EthconnectBackgroundStartFactor is to set the factor by which the delay increases when retrying
	EthconnectBackgroundStartFactor = "backgroundStart.factor"

	// AddressResolverConfigKey is a sub-key in the config to contain an address resolver config.
	AddressResolverConfigKey = "addressResolver"
	// AddressResolverAlwaysResolve causes the address resolve to be invoked on every API call that resolves an address, regardless of whether the input conforms to an 0x address, and disables any caching
	AddressResolverAlwaysResolve = "alwaysResolve"
	// AddressResolverRetainOriginal when true the original pre-resolved string is retained after the lookup, and passed down to Ethconnect as the from address
	AddressResolverRetainOriginal = "retainOriginal"
	// AddressResolverMethod the HTTP method to use to call the address resolver (default GET)
	AddressResolverMethod = "method"
	// AddressResolverURLTemplate the URL go template string to use when calling the address resolver - a ".intent" string can be used in the go template
	AddressResolverURLTemplate = "urlTemplate"
	// AddressResolverBodyTemplate the body go template string to use when calling the address resolver - a ".intent" string can be used in the go template
	AddressResolverBodyTemplate = "bodyTemplate"
	// AddressResolverResponseField the name of a JSON field that is provided in the response, that contains the ethereum address (default "address")
	AddressResolverResponseField = "responseField"

	// FFTMConfigKey is a sub-key in the config that optionally contains FireFly transaction connection information
	FFTMConfigKey = "fftm"
)

func (e *Ethereum) InitConfig(config config.Section) {
	e.ethconnectConf = config.SubSection(EthconnectConfigKey)
	wsclient.InitConfig(e.ethconnectConf)

	e.ethconnectConf.SetDefault(ffresty.HTTPConfigRetryErrorStatusCodeRegex, "^5|^429$") // 5xx and 429s, but not 409s / 404s
	e.ethconnectConf.AddKnownKey(EthconnectConfigTopic)
	e.ethconnectConf.AddKnownKey(EthconnectBackgroundStart)
	e.ethconnectConf.AddKnownKey(EthconnectBackgroundStartInitialDelay, defaultBackgroundInitialDelay)
	e.ethconnectConf.AddKnownKey(EthconnectBackgroundStartFactor, defaultBackgroundRetryFactor)
	e.ethconnectConf.AddKnownKey(EthconnectBackgroundStartMaxDelay, defaultBackgroundMaxDelay)
	e.ethconnectConf.AddKnownKey(EthconnectConfigBatchSize, defaultBatchSize)
	e.ethconnectConf.AddKnownKey(EthconnectConfigBatchTimeout, defaultBatchTimeout)
	e.ethconnectConf.AddKnownKey(EthconnectPrefixShort, defaultPrefixShort)
	e.ethconnectConf.AddKnownKey(EthconnectPrefixLong, defaultPrefixLong)
	e.ethconnectConf.AddKnownKey(EthconnectConfigInstanceDeprecated)
	e.ethconnectConf.AddKnownKey(EthconnectConfigFromBlockDeprecated, defaultFromBlock)

	// this is unused in reality
	fftmConf := config.SubSection(FFTMConfigKey)
	ffresty.InitConfig(fftmConf)

	addressResolverConf := config.SubSection(AddressResolverConfigKey)
	ffresty.InitConfig(addressResolverConf)
	addressResolverConf.AddKnownKey(AddressResolverAlwaysResolve)
	addressResolverConf.AddKnownKey(AddressResolverRetainOriginal)
	addressResolverConf.AddKnownKey(AddressResolverMethod, defaultAddressResolverMethod)
	addressResolverConf.AddKnownKey(AddressResolverURLTemplate)
	addressResolverConf.AddKnownKey(AddressResolverBodyTemplate)
	addressResolverConf.AddKnownKey(AddressResolverResponseField, defaultAddressResolverResponseField)
}
