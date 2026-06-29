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
	"testing"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/stretchr/testify/assert"
)

func TestGetPluginUnknown(t *testing.T) {
	ctx := context.Background()
	_, err := GetPlugin(ctx, "foo")
	assert.Error(t, err)
	assert.Regexp(t, "FF10272", err)
}

func TestGetPlugin(t *testing.T) {
	ctx := context.Background()
	plugin, err := GetPlugin(ctx, "fftokens")
	assert.NoError(t, err)
	assert.NotNil(t, plugin)
}

var root = config.RootSection("tokens")

func TestInitConfig(t *testing.T) {
	conf := root.SubArray("plugins")
	InitConfig(conf)
}
