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

package core

import "github.com/hyperledger-firefly/common/pkg/fftypes"

// Nonce is this local node's state record for the context of a group+topic+author combination.
// The Hash is the state of the hash before the nonce is added on to make it unique to the message.
type Nonce struct {
	Hash  *fftypes.Bytes32 `json:"hash"`
	Nonce int64            `json:"nonce"`
}
