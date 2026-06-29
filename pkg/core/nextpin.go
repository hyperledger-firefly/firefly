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

type NextPin struct {
	Namespace string           `ffstruct:"NextPin" json:"namespace"`
	Context   *fftypes.Bytes32 `ffstruct:"NextPin" json:"context"`
	Identity  string           `ffstruct:"NextPin" json:"identity"`
	Hash      *fftypes.Bytes32 `ffstruct:"NextPin" json:"hash"`
	Nonce     int64            `ffstruct:"NextPin" json:"nonce"`
	Sequence  int64            `ffstruct:"NextPin" json:"-"` // Local database sequence used internally for update efficiency
}
