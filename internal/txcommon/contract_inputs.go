// Copyright © 2024 Kaleido, Inc.
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
	"bytes"
	"context"
	"encoding/json"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

type BatchPinData struct {
	Batch      *core.BatchPersisted `json:"batch"`
	Contexts   []*fftypes.Bytes32   `json:"contexts"`
	PayloadRef string               `json:"payloadRef"`
}

type BlockchainInvokeData struct {
	Request  *core.ContractCallRequest `json:"request"`
	BatchPin *BatchPinData             `json:"batchPin"`
}

func RetrieveBlockchainInvokeInputs(ctx context.Context, op *core.Operation) (*core.ContractCallRequest, error) {
	var req core.ContractCallRequest
	s := op.Input.String()
	d := json.NewDecoder(bytes.NewReader([]byte(s)))
	d.UseNumber()
	if err := d.Decode(&req); err != nil {
		return nil, i18n.WrapError(ctx, err, i18n.MsgJSONObjectParseFailed, s)
	}
	return &req, nil
}

func OpBlockchainInvoke(op *core.Operation, req *core.ContractCallRequest, batch *BatchPinData) *core.PreparedOperation {
	return &core.PreparedOperation{
		ID:        op.ID,
		Namespace: op.Namespace,
		Plugin:    op.Plugin,
		Type:      op.Type,
		Data:      BlockchainInvokeData{Request: req, BatchPin: batch},
	}
}
