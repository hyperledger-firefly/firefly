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

package tbd

import (
	"context"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/firefly/pkg/identity"
)

// TBD is a null implementation of the Identity Interface to avoid breaking configuration created with the previous "onchain" plugin
type TBD struct {
	capabilities *identity.Capabilities
}

func (tbd *TBD) Name() string {
	return "onchain" // For backwards compatibility with previous config that might have specified "onchain"
}

func (tbd *TBD) Init(ctx context.Context, config config.Section) (err error) {
	tbd.capabilities = &identity.Capabilities{}
	return nil
}

func (tbd *TBD) SetHandler(namespace string, handler identity.Callbacks) {
}

func (tbd *TBD) Start() error {
	return nil
}

func (tbd *TBD) Capabilities() *identity.Capabilities {
	return tbd.capabilities
}
