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

package txcommon

import (
	"context"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
)

func TestRetrieveBlockchainInvokeInputs(t *testing.T) {
	msgID := fftypes.NewUUID()
	op := &core.Operation{
		Input: fftypes.JSONObject{
			"methodPath": "hello",
			"message": fftypes.JSONObject{
				"header": fftypes.JSONObject{
					"id": msgID,
				},
			},
		},
	}

	req, err := RetrieveBlockchainInvokeInputs(context.Background(), op)
	assert.NoError(t, err)
	assert.Equal(t, msgID, req.Message.Header.ID)
	assert.Equal(t, "hello", req.MethodPath)
}

func TestRetrieveBlockchainInvokeInputsBadID(t *testing.T) {
	op := &core.Operation{
		Input: fftypes.JSONObject{
			"methodPath": "hello",
			"message": fftypes.JSONObject{
				"header": fftypes.JSONObject{
					"id": "BAD",
				},
			},
		},
	}

	_, err := RetrieveBlockchainInvokeInputs(context.Background(), op)
	assert.Regexp(t, "FF00127", err)
}

func TestPrepareBlockchainInvoke(t *testing.T) {
	op := &core.Operation{
		Type: core.OpTypeBlockchainInvoke,
	}
	req := &core.ContractCallRequest{}
	batchPin := &BatchPinData{}

	po := OpBlockchainInvoke(op, req, batchPin)

	assert.Equal(t, core.OpTypeBlockchainInvoke, po.Type)
	assert.Equal(t, req, po.Data.(BlockchainInvokeData).Request)
	assert.Equal(t, batchPin, po.Data.(BlockchainInvokeData).BatchPin)
}
