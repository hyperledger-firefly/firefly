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

func TestTokenPoolValidation(t *testing.T) {
	pool := &TokenPool{
		Namespace: "ok",
		Name:      "!wrong",
	}
	err := pool.Validate(context.Background())
	assert.Regexp(t, "FF00140.*'name'", err)

	pool = &TokenPool{
		Namespace:   "ok",
		Name:        "ok",
		NetworkName: "!wrong",
	}
	err = pool.Validate(context.Background())
	assert.Regexp(t, "FF00140.*'networkName'", err)

	pool = &TokenPool{
		Namespace: "ok",
		Name:      "ok",
	}
	err = pool.Validate(context.Background())
	assert.NoError(t, err)
}

func TestTokenPoolDefinition(t *testing.T) {
	pool := &TokenPool{
		Namespace:   "ok",
		NetworkName: "ok",
	}
	var def Definition = &TokenPoolDefinition{Pool: pool}
	assert.Equal(t, "73008386c5579b7015385528eb892f7773e13a20015c692f6b90b26e413fe8a4", def.Topic())

	id := fftypes.NewUUID()
	def.SetBroadcastMessage(id)
	assert.Equal(t, id, pool.Message)
}
