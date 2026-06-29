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

package identity

import (
	"context"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

// Plugin is the interface implemented by each identity plugin
type Plugin interface {
	core.Named

	// InitConfig initializes the set of configuration options that are valid, with defaults. Called on all plugins.
	InitConfig(config config.Section)

	// Init initializes the plugin, with configuration
	Init(ctx context.Context, config config.Section) error

	// SetHandler registers a handler to receive callbacks
	// Plugin will attempt (but is not guaranteed) to deliver events only for the given namespace
	SetHandler(namespace string, handler Callbacks)

	// Blockchain interface must not deliver any events until start is called
	Start() error

	// Capabilities returns capabilities - not called until after Init
	Capabilities() *Capabilities

	// INTERFACE IS TBD SINCE INTRODUCTION OF THE IDENTITY MANAGER [Im] COMPONENT
	//
	// There is a strong thought that a pluggable infrastructure for mapping external DID based identity
	// solutions into FireFly is required. However, the immediate shift in Sep 2021 moved to defining
	// a strong enough identity construct within FireFly to map from/to.
	//
	// See issue https://github.com/hyperledger-firefly/firefly/issues/187 to contribute to the discussion

}

// Callbacks is the interface provided to the identity plugin, to allow it to request information from firefly, or pass events.
type Callbacks interface {
}

// Capabilities the supported featureset of the identity
// interface implemented by the plugin, with the specified config
type Capabilities struct {
}
