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

package txcommon

import (
	"context"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
)

func TestAddTokenPoolCreateInputs(t *testing.T) {
	op := &core.Operation{}
	config := fftypes.JSONObject{
		"foo": "bar",
	}
	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Name:      "testpool",
		Symbol:    "FFT",
		Config:    config,
	}

	AddTokenPoolCreateInputs(op, pool)
	assert.Equal(t, pool.ID.String(), op.Input.GetString("id"))
	assert.Equal(t, "ns1", op.Input.GetString("namespace"))
	assert.Equal(t, "testpool", op.Input.GetString("name"))
	assert.Equal(t, "FFT", op.Input.GetString("symbol"))
	assert.Equal(t, pool.Config, op.Input.GetObject("config"))
}

func TestRetrieveTokenPoolCreateInputs(t *testing.T) {
	id := fftypes.NewUUID()
	config := fftypes.JSONObject{
		"foo": "bar",
	}
	op := &core.Operation{
		Input: fftypes.JSONObject{
			"id":        id.String(),
			"namespace": "ns1",
			"name":      "testpool",
			"symbol":    "FFT",
			"config":    config,
		},
	}

	pool, err := RetrieveTokenPoolCreateInputs(context.Background(), op)
	assert.NoError(t, err)
	assert.Equal(t, *id, *pool.ID)
	assert.Equal(t, "ns1", pool.Namespace)
	assert.Equal(t, "testpool", pool.Name)
	assert.Equal(t, "FFT", pool.Symbol)
	assert.Equal(t, config, pool.Config)
}

func TestRetrieveTokenPoolCreateInputsBadID(t *testing.T) {
	op := &core.Operation{
		Input: fftypes.JSONObject{
			"id": "bad",
		},
	}

	_, err := RetrieveTokenPoolCreateInputs(context.Background(), op)
	assert.Regexp(t, "FF00127", err)
}

func TestAddTokenPoolActivateInputs(t *testing.T) {
	op := &core.Operation{}
	poolID := fftypes.NewUUID()

	AddTokenPoolActivateInputs(op, poolID)
	assert.Equal(t, poolID.String(), op.Input.GetString("id"))
}

func TestRetrieveTokenPoolActivateInputs(t *testing.T) {
	id := fftypes.NewUUID()
	op := &core.Operation{
		Input: fftypes.JSONObject{
			"id": id.String(),
		},
	}

	poolID, err := RetrieveTokenPoolActivateInputs(context.Background(), op)
	assert.NoError(t, err)
	assert.Equal(t, *id, *poolID)
}

func TestAddTokenTransferInputs(t *testing.T) {
	op := &core.Operation{}
	transfer := &core.TokenTransfer{
		LocalID: fftypes.NewUUID(),
		Type:    core.TokenTransferTypeTransfer,
		Amount:  *fftypes.NewFFBigInt(1),
		TX: core.TransactionRef{
			Type: core.TransactionTypeTokenTransfer,
			ID:   fftypes.NewUUID(),
		},
	}

	AddTokenTransferInputs(op, transfer)
	assert.Equal(t, fftypes.JSONObject{
		"amount":  "1",
		"localId": transfer.LocalID.String(),
		"tx": map[string]interface{}{
			"id":   transfer.TX.ID.String(),
			"type": "token_transfer",
		},
		"type": "transfer",
	}, op.Input)
}

func TestRetrieveTokenTransferInputs(t *testing.T) {
	id := fftypes.NewUUID()
	op := &core.Operation{
		Input: fftypes.JSONObject{
			"amount":  "1",
			"localId": id.String(),
		},
	}

	transfer, err := RetrieveTokenTransferInputs(context.Background(), op)
	assert.NoError(t, err)
	assert.Equal(t, *id, *transfer.LocalID)
	assert.Equal(t, int64(1), transfer.Amount.Int().Int64())
}

func TestRetrieveTokenTransferInputsBadID(t *testing.T) {
	op := &core.Operation{
		Input: fftypes.JSONObject{
			"localId": "bad",
		},
	}

	_, err := RetrieveTokenTransferInputs(context.Background(), op)
	assert.Regexp(t, "FF00127", err)
}

func TestAddTokenApprovalInputs(t *testing.T) {
	op := &core.Operation{}
	approval := &core.TokenApproval{
		LocalID:  fftypes.NewUUID(),
		Approved: true,
		Operator: "0x01",
		Key:      "0x02",
		TX: core.TransactionRef{
			Type: core.TransactionTypeTokenApproval,
			ID:   fftypes.NewUUID(),
		},
	}

	AddTokenApprovalInputs(op, approval)
	assert.Equal(t, fftypes.JSONObject{
		"approved": true,
		"operator": "0x01",
		"key":      "0x02",
		"localId":  approval.LocalID.String(),
		"tx": map[string]interface{}{
			"id":   approval.TX.ID.String(),
			"type": "token_approval",
		},
	}, op.Input)
}

func TestRetrieveTokenApprovalInputs(t *testing.T) {
	id := fftypes.NewUUID()
	op := &core.Operation{
		Input: fftypes.JSONObject{
			"amount":   "1",
			"localId":  id.String(),
			"approved": true,
		},
	}

	approval, err := RetrieveTokenApprovalInputs(context.Background(), op)
	assert.NoError(t, err)
	assert.Equal(t, *id, *approval.LocalID)
	assert.Equal(t, true, approval.Approved)
}

func TestRetrieveTokenApprovalInputsBadID(t *testing.T) {
	op := &core.Operation{
		Input: fftypes.JSONObject{
			"localId": "bad",
		},
	}

	_, err := RetrieveTokenApprovalInputs(context.Background(), op)
	assert.Regexp(t, "FF00127", err)
}
