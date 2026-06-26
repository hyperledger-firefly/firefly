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

//go:build reference

package config

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly/internal/apiserver"
	"github.com/hyperledger/firefly/internal/coreconfig"
	"github.com/hyperledger/firefly/internal/namespace"
	"github.com/stretchr/testify/assert"
)

func TestGenerateConfigDocs(t *testing.T) {
	// Initialize config of all plugins
	coreconfig.Reset()
	namespace.InitConfig()
	apiserver.InitConfig()
	f, err := os.Create(filepath.Join("docs", "reference", "config.md"))
	assert.NoError(t, err)
	generatedConfig, err := config.GenerateConfigMarkdown(context.Background(), configDocHeader, config.GetKnownKeys())
	assert.NoError(t, err)
	_, err = f.Write(generatedConfig)
	assert.NoError(t, err)
	err = f.Close()
	assert.NoError(t, err)
}

const configDocHeader = `---
title: Configuration Reference
---
`
