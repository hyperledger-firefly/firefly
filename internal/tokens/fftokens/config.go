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

package fftokens

import (
	"time"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/wsclient"
)

const (
	FFTEventRetryInitialDelay      = "eventRetry.initialDelay"
	FFTEventRetryMaxDelay          = "eventRetry.maxDelay"
	FFTEventRetryFactor            = "eventRetry.factor"
	FFTBackgroundStart             = "backgroundStart.enabled"
	FFTBackgroundStartInitialDelay = "backgroundStart.initialDelay"
	FFTBackgroundStartMaxDelay     = "backgroundStart.maxDelay"
	FFTBackgroundStartFactor       = "backgroundStart.factor"

	defaultBackgroundInitialDelay = "5s"
	defaultBackgroundRetryFactor  = 2.0
	defaultBackgroundMaxDelay     = "1m"
)

func (ft *FFTokens) InitConfig(config config.Section) {
	wsclient.InitConfig(config)

	config.AddKnownKey(FFTEventRetryInitialDelay, 50*time.Millisecond)
	config.AddKnownKey(FFTEventRetryMaxDelay, 30*time.Second)
	config.AddKnownKey(FFTEventRetryFactor, 2.0)
	config.AddKnownKey(FFTBackgroundStart, false)
	config.AddKnownKey(FFTBackgroundStartInitialDelay, defaultBackgroundInitialDelay)
	config.AddKnownKey(FFTBackgroundStartMaxDelay, defaultBackgroundMaxDelay)
	config.AddKnownKey(FFTBackgroundStartFactor, defaultBackgroundRetryFactor)
}
