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

package core

import "github.com/hyperledger-firefly/common/pkg/fftypes"

// Definition is implemented by all objects that can be broadcast as system definitions to the network
type Definition interface {
	// Topic returns the topic on which the object should be broadcast
	Topic() string
	// SetBroadcastMessage sets the message that broadcast the definition
	SetBroadcastMessage(msgID *fftypes.UUID)
}

type DefinitionPublish struct {
	NetworkName string                `ffstruct:"DefinitionPublish" json:"networkName,omitempty"`
	Topics      fftypes.FFStringArray `ffstruct:"DefinitionPublish" json:"topics,omitempty"`
}
