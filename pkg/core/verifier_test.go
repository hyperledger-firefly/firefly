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
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/stretchr/testify/assert"
)

func TestVerifierSeal(t *testing.T) {

	v := &Verifier{
		Identity:  fftypes.NewUUID(), // does not contribute to hash
		Namespace: "ns1",
		VerifierRef: VerifierRef{
			Type:  VerifierTypeEthAddress,
			Value: "0xdfceac9b26ac099d7e4df958c22939878c19c948",
		},
		Created: fftypes.Now(),
	}
	v.Seal()
	assert.Equal(t, "c7742ed06a6c36dece56d9c6d65d4ee6ba0db2a643e7f8efc75ec4e7ca31d45d", v.Hash.String())

}
