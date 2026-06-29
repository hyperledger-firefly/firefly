// Copyright © 2021 Kaleido, Inc.
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
	"testing"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/firefly/mocks/identitymocks"
	"github.com/hyperledger-firefly/firefly/pkg/identity"
	"github.com/stretchr/testify/assert"
)

var utConfig = config.RootSection("onchain_unit_tests")

func TestInit(t *testing.T) {
	var oc identity.Plugin = &TBD{}
	oc.InitConfig(utConfig)
	err := oc.Init(context.Background(), utConfig)
	assert.NoError(t, err)
	assert.Equal(t, "onchain", oc.Name())
	err = oc.Start()
	assert.NoError(t, err)
	capabilities := oc.Capabilities()
	assert.NotNil(t, capabilities)
	cbs := &identitymocks.Callbacks{}
	oc.SetHandler("ns1", cbs) // no-op
}
