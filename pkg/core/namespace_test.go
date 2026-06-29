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

func TestMultipartyContractsDatabaseSerialization(t *testing.T) {
	contracts1 := &MultipartyContracts{
		Active: &MultipartyContract{
			Index:      1,
			FirstEvent: "oldest",
			Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
				"address": "0x123",
			}.String()),
			Info: MultipartyContractInfo{
				Subscription: "1234",
			},
		},
		Terminated: []*MultipartyContract{
			{
				Index:      0,
				FirstEvent: "oldest",
				Location: fftypes.JSONAnyPtr(fftypes.JSONObject{
					"address": "0x1234",
				}.String()),
				Info: MultipartyContractInfo{
					Subscription: "12345",
					FinalEvent:   "50",
				},
			},
		},
	}

	// Verify it serializes as bytes to the database
	val1, err := contracts1.Value()
	assert.NoError(t, err)
	assert.Equal(t, `{"active":{"index":1,"location":{"address":"0x123"},"firstEvent":"oldest","info":{"subscription":"1234"}},"terminated":[{"index":0,"location":{"address":"0x1234"},"firstEvent":"oldest","info":{"subscription":"12345","finalEvent":"50"}}]}`, string(val1.([]byte)))

	// Verify it restores ok
	contracts2 := &MultipartyContracts{}
	err = contracts2.Scan(val1)
	assert.NoError(t, err)
	assert.Equal(t, 1, contracts2.Active.Index)
	assert.Equal(t, *fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "0x123",
	}.String()), *contracts2.Active.Location)
	assert.Len(t, contracts2.Terminated, 1)

	// Verify it ignores a blank string
	err = contracts2.Scan("")
	assert.NoError(t, err)
	assert.Equal(t, 1, contracts2.Active.Index)

	// Out of luck with anything else
	err = contracts2.Scan(false)
	assert.Regexp(t, "FF00105", err)
}
