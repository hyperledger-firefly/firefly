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

package core

import (
	"context"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/stretchr/testify/assert"
)

func TestValidateContractAPI(t *testing.T) {
	api := &ContractAPI{
		Namespace: "ns1",
		Name:      "banana",
	}
	err := api.Validate(context.Background())
	assert.NoError(t, err)
}

func TestValidateInvalidContractAPI(t *testing.T) {
	api := &ContractAPI{
		Namespace: "&%&^#()#",
		Name:      "banana",
	}
	err := api.Validate(context.Background())
	assert.Regexp(t, "FF00140", err)

	api = &ContractAPI{
		Namespace: "ns1",
		Name:      "(%&@!^%^)",
	}
	err = api.Validate(context.Background())
	assert.Regexp(t, "FF00140", err)

	api = &ContractAPI{
		Namespace:   "ns1",
		Name:        "banana",
		NetworkName: "(%&@!^%^)",
	}
	err = api.Validate(context.Background())
	assert.Regexp(t, "FF00140", err)
}

func TestContractAPITopic(t *testing.T) {
	api := &ContractAPI{
		Namespace:   "ns1",
		NetworkName: "banana",
	}
	assert.Equal(t, "a2f42ab7c9ef44ec08a103c565041b48b4cff4e4176e5faacfe8512c361e4f2c", api.Topic())
}

func TestContractAPISetBroadCastMessage(t *testing.T) {
	msgID := fftypes.NewUUID()
	api := &ContractAPI{}
	api.SetBroadcastMessage(msgID)
	assert.Equal(t, api.Message, msgID)
}

func TestLocationAndLedgerEquals(t *testing.T) {
	var c1 *ContractAPI = nil
	var c2 *ContractAPI = nil
	assert.False(t, c1.LocationAndLedgerEquals(c2))

	c1 = &ContractAPI{
		ID:       fftypes.NewUUID(),
		Location: fftypes.JSONAnyPtr("abc"),
	}
	c2 = &ContractAPI{
		ID:       fftypes.NewUUID(),
		Location: fftypes.JSONAnyPtr("abc"),
	}
	assert.True(t, c1.LocationAndLedgerEquals(c2))

	c1 = &ContractAPI{
		ID:       fftypes.NewUUID(),
		Location: fftypes.JSONAnyPtr("fff"),
	}
	c2 = &ContractAPI{
		ID:       fftypes.NewUUID(),
		Location: fftypes.JSONAnyPtr("abc"),
	}
	assert.False(t, c1.LocationAndLedgerEquals(c2))

	c1 = &ContractAPI{
		ID:       fftypes.NewUUID(),
		Location: nil,
	}
	c2 = &ContractAPI{
		ID:       fftypes.NewUUID(),
		Location: nil,
	}
	assert.True(t, c1.LocationAndLedgerEquals(c2))
}
