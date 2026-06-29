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

package ffdx

import (
	"time"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/wsclient"
)

const (
	// DataExchangeManifestEnabled determines whether to require+validate a manifest from other DX instances in the network. Must be supported by the connector
	DataExchangeManifestEnabled = "manifestEnabled"
	// DataExchangeInitEnabled instructs FireFly to always post all current nodes to the /init API before connecting or reconnecting to the connector
	DataExchangeInitEnabled = "initEnabled"

	DataExchangeEventRetryInitialDelay = "eventRetry.initialDelay"
	DataExchangeEventRetryMaxDelay     = "eventRetry.maxDelay"
	DataExchangeEventRetryFactor       = "eventRetry.factor"

	DataExchangeBackgroundStart             = "backgroundStart.enabled"
	DataExchangeBackgroundStartInitialDelay = "backgroundStart.initialDelay"
	DataExchangeBackgroundStartMaxDelay     = "backgroundStart.maxDelay"
	DataExchangeBackgroundStartFactor       = "backgroundStart.factor"
	defaultBackgroundInitialDelay           = "5s"
	defaultBackgroundRetryFactor            = 2.0
	defaultBackgroundMaxDelay               = "1m"
)

func (h *FFDX) InitConfig(config config.Section) {
	wsclient.InitConfig(config)
	config.AddKnownKey(DataExchangeManifestEnabled, false)
	config.AddKnownKey(DataExchangeInitEnabled, false)
	config.AddKnownKey(DataExchangeEventRetryInitialDelay, 50*time.Millisecond)
	config.AddKnownKey(DataExchangeEventRetryMaxDelay, 30*time.Second)
	config.AddKnownKey(DataExchangeEventRetryFactor, 2.0)
	config.AddKnownKey(DataExchangeBackgroundStart, false)
	config.AddKnownKey(DataExchangeBackgroundStartInitialDelay, defaultBackgroundInitialDelay)
	config.AddKnownKey(DataExchangeBackgroundStartMaxDelay, defaultBackgroundMaxDelay)
	config.AddKnownKey(DataExchangeBackgroundStartFactor, defaultBackgroundRetryFactor)
}
