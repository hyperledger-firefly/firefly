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

package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func compactJSON(s string) string {
	var buf bytes.Buffer
	if err := json.Compact(&buf, []byte(s)); err != nil {
		panic(err)
	}
	return buf.String()
}

func TestGetTransactionStatusBatchPinSuccess(t *testing.T) {
	or := newTestOrchestrator()

	txID := fftypes.NewUUID()
	tx := &core.Transaction{
		Namespace: "ns1",
		Type:      core.TransactionTypeBatchPin,
	}
	ops := []*core.Operation{
		{
			Namespace: "ns1",
			Status:    core.OpStatusSucceeded,
			ID:        fftypes.NewUUID(),
			Type:      core.OpTypeBlockchainPinBatch,
			Updated:   fftypes.UnixTime(0),
			Output:    fftypes.JSONObject{"transactionHash": "0x100"},
		},
	}
	events := []*core.BlockchainEvent{
		{
			Namespace: "ns1",
			Name:      "BatchPin",
			ID:        fftypes.NewUUID(),
			Timestamp: fftypes.UnixTime(1),
			Info:      fftypes.JSONObject{"transactionHash": "0x100"},
		},
	}
	batches := []*core.BatchPersisted{
		{
			BatchHeader: core.BatchHeader{
				Namespace: "ns1",
				ID:        fftypes.NewUUID(),
				Type:      core.BatchTypeBroadcast,
			},
			Confirmed: fftypes.UnixTime(2),
		},
	}

	or.mth.On("GetTransactionByIDCached", mock.Anything, txID).Return(tx, nil)
	or.mdi.On("GetOperations", mock.Anything, "ns", mock.Anything).Return(ops, nil, nil)
	or.mdi.On("GetBlockchainEvents", mock.Anything, "ns", mock.Anything).Return(events, nil, nil)
	or.mdi.On("GetBatches", mock.Anything, "ns", mock.Anything).Return(batches, nil, nil)

	status, err := or.GetTransactionStatus(context.Background(), txID.String())
	assert.NoError(t, err)

	expectedStatus := compactJSON(`{
		"status": "Succeeded",
		"details": [
			{
				"type": "Batch",
				"subtype": "broadcast",
				"status": "Succeeded",
				"timestamp": "1970-01-01T00:00:02Z",
				"id": "` + batches[0].ID.String() + `"
			},
			{
				"type": "BlockchainEvent",
				"subtype": "BatchPin",
				"status": "Succeeded",
				"timestamp": "1970-01-01T00:00:01Z",
				"id": "` + events[0].ID.String() + `",
				"info": {"transactionHash": "0x100"}
			},
			{
				"type": "Operation",
				"subtype": "blockchain_pin_batch",
				"status": "Succeeded",
				"timestamp": "1970-01-01T00:00:00Z",
				"id": "` + ops[0].ID.String() + `",
				"info": {"transactionHash": "0x100"}
			}
		]
	}`)
	statusJSON, _ := json.Marshal(status)
	assert.Equal(t, expectedStatus, string(statusJSON))

	or.mdi.AssertExpectations(t)
}

func TestGetTransactionStatusBatchPinFail(t *testing.T) {
	or := newTestOrchestrator()

	txID := fftypes.NewUUID()
	tx := &core.Transaction{
		Namespace: "ns1",
		Type:      core.TransactionTypeBatchPin,
	}
	ops := []*core.Operation{
		{
			Namespace: "ns1",
			Status:    core.OpStatusFailed,
			ID:        fftypes.NewUUID(),
			Type:      core.OpTypeBlockchainPinBatch,
			Error:     "complete failure",
		},
	}
	events := []*core.BlockchainEvent{}
	batches := []*core.BatchPersisted{}

	or.mth.On("GetTransactionByIDCached", mock.Anything, txID).Return(tx, nil)
	or.mdi.On("GetOperations", mock.Anything, "ns", mock.Anything).Return(ops, nil, nil)
	or.mdi.On("GetBlockchainEvents", mock.Anything, "ns", mock.Anything).Return(events, nil, nil)
	or.mdi.On("GetBatches", mock.Anything, "ns", mock.Anything).Return(batches, nil, nil)

	status, err := or.GetTransactionStatus(context.Background(), txID.String())
	assert.NoError(t, err)

	expectedStatus := compactJSON(`{
		"status": "Failed",
		"details": [
			{
				"type": "Operation",
				"subtype": "blockchain_pin_batch",
				"status": "Failed",
				"id": "` + ops[0].ID.String() + `",
				"error": "complete failure"
			},
			{
				"type": "BlockchainEvent",
				"status": "Pending"
			},
			{
				"type": "Batch",
				"status": "Pending"
			}
		]
	}`)
	statusJSON, _ := json.Marshal(status)
	assert.Equal(t, expectedStatus, string(statusJSON))

	or.mdi.AssertExpectations(t)
}

func TestGetTransactionStatusBatchPinPending(t *testing.T) {
	or := newTestOrchestrator()

	txID := fftypes.NewUUID()
	tx := &core.Transaction{
		Namespace: "ns1",
		Type:      core.TransactionTypeBatchPin,
	}
	ops := []*core.Operation{
		{
			Namespace: "ns1",
			Status:    core.OpStatusSucceeded,
			ID:        fftypes.NewUUID(),
			Type:      core.OpTypeBlockchainPinBatch,
			Updated:   fftypes.UnixTime(0),
		},
	}
	events := []*core.BlockchainEvent{}
	batches := []*core.BatchPersisted{}

	or.mth.On("GetTransactionByIDCached", mock.Anything, txID).Return(tx, nil)
	or.mdi.On("GetOperations", mock.Anything, "ns", mock.Anything).Return(ops, nil, nil)
	or.mdi.On("GetBlockchainEvents", mock.Anything, "ns", mock.Anything).Return(events, nil, nil)
	or.mdi.On("GetBatches", mock.Anything, "ns", mock.Anything).Return(batches, nil, nil)

	status, err := or.GetTransactionStatus(context.Background(), txID.String())
	assert.NoError(t, err)

	expectedStatus := compactJSON(`{
		"status": "Pending",
		"details": [
			{
				"type": "BlockchainEvent",
				"status": "Pending"
			},
			{
				"type": "Batch",
				"status": "Pending"
			},
			{
				"type": "Operation",
				"subtype": "blockchain_pin_batch",
				"status": "Succeeded",
				"timestamp": "1970-01-01T00:00:00Z",
				"id": "` + ops[0].ID.String() + `"
			}
		]
	}`)
	statusJSON, _ := json.Marshal(status)
	assert.Equal(t, expectedStatus, string(statusJSON))

	or.mdi.AssertExpectations(t)
}

func TestGetTransactionStatusTokenPoolSuccess(t *testing.T) {
	or := newTestOrchestrator()

	txID := fftypes.NewUUID()
	tx := &core.Transaction{
		Namespace: "ns1",
		Type:      core.TransactionTypeTokenPool,
	}
	ops := []*core.Operation{
		{
			Namespace: "ns1",
			Status:    core.OpStatusSucceeded,
			ID:        fftypes.NewUUID(),
			Type:      core.OpTypeTokenCreatePool,
			Updated:   fftypes.UnixTime(0),
			Output:    fftypes.JSONObject{"transactionHash": "0x100"},
		},
	}
	events := []*core.BlockchainEvent{
		{
			Namespace: "ns1",
			Name:      "TokenPool",
			ID:        fftypes.NewUUID(),
			Timestamp: fftypes.UnixTime(0),
			Info:      fftypes.JSONObject{"transactionHash": "0x100"},
		},
	}
	pools := []*core.TokenPool{
		{
			Namespace: "ns1",
			ID:        fftypes.NewUUID(),
			Type:      core.TokenTypeFungible,
			Created:   fftypes.UnixTime(0),
			Active:    true,
		},
	}

	or.mth.On("GetTransactionByIDCached", mock.Anything, txID).Return(tx, nil)
	or.mdi.On("GetOperations", mock.Anything, "ns", mock.Anything).Return(ops, nil, nil)
	or.mdi.On("GetBlockchainEvents", mock.Anything, "ns", mock.Anything).Return(events, nil, nil)
	or.mdi.On("GetTokenPools", mock.Anything, "ns", mock.Anything).Return(pools, nil, nil)

	status, err := or.GetTransactionStatus(context.Background(), txID.String())
	assert.NoError(t, err)

	expectedStatus := compactJSON(`{
		"status": "Succeeded",
		"details": [
			{
				"type": "Operation",
				"subtype": "token_create_pool",
				"status": "Succeeded",
				"timestamp": "1970-01-01T00:00:00Z",
				"id": "` + ops[0].ID.String() + `",
				"info": {"transactionHash": "0x100"}
			},
			{
				"type": "BlockchainEvent",
				"subtype": "TokenPool",
				"status": "Succeeded",
				"timestamp": "1970-01-01T00:00:00Z",
				"id": "` + events[0].ID.String() + `",
				"info": {"transactionHash": "0x100"}
			},
			{
				"type": "TokenPool",
				"subtype": "fungible",
				"status": "Succeeded",
				"timestamp": "1970-01-01T00:00:00Z",
				"id": "` + pools[0].ID.String() + `"
			}
		]
	}`)
	statusJSON, _ := json.Marshal(status)
	assert.Equal(t, expectedStatus, string(statusJSON))

	or.mdi.AssertExpectations(t)
}

func TestGetTransactionStatusTokenPoolPending(t *testing.T) {
	or := newTestOrchestrator()

	txID := fftypes.NewUUID()
	tx := &core.Transaction{
		Namespace: "ns1",
		Type:      core.TransactionTypeTokenPool,
	}
	ops := []*core.Operation{
		{
			Namespace: "ns1",
			Status:    core.OpStatusSucceeded,
			ID:        fftypes.NewUUID(),
			Type:      core.OpTypeTokenCreatePool,
			Output:    fftypes.JSONObject{"transactionHash": "0x100"},
		},
	}
	events := []*core.BlockchainEvent{}
	pools := []*core.TokenPool{}

	or.mth.On("GetTransactionByIDCached", mock.Anything, txID).Return(tx, nil)
	or.mdi.On("GetOperations", mock.Anything, "ns", mock.Anything).Return(ops, nil, nil)
	or.mdi.On("GetBlockchainEvents", mock.Anything, "ns", mock.Anything).Return(events, nil, nil)
	or.mdi.On("GetTokenPools", mock.Anything, "ns", mock.Anything).Return(pools, nil, nil)

	status, err := or.GetTransactionStatus(context.Background(), txID.String())
	assert.NoError(t, err)

	expectedStatus := compactJSON(`{
		"status": "Pending",
		"details": [
			{
				"type": "Operation",
				"subtype": "token_create_pool",
				"status": "Succeeded",
				"id": "` + ops[0].ID.String() + `",
				"info": {"transactionHash": "0x100"}
			},
			{
				"type": "TokenPool",
				"status": "Pending"
			}
		]
	}`)
	statusJSON, _ := json.Marshal(status)
	assert.Equal(t, expectedStatus, string(statusJSON))

	or.mdi.AssertExpectations(t)
}

func TestGetTransactionStatusTokenPoolUnconfirmed(t *testing.T) {
	or := newTestOrchestrator()

	txID := fftypes.NewUUID()
	tx := &core.Transaction{
		Namespace: "ns1",
		Type:      core.TransactionTypeTokenPool,
	}
	ops := []*core.Operation{
		{
			Namespace: "ns1",
			Status:    core.OpStatusSucceeded,
			ID:        fftypes.NewUUID(),
			Type:      core.OpTypeTokenCreatePool,
			Output:    fftypes.JSONObject{"transactionHash": "0x100"},
		},
	}
	events := []*core.BlockchainEvent{}
	pools := []*core.TokenPool{
		{
			Namespace: "ns1",
			ID:        fftypes.NewUUID(),
			Type:      core.TokenTypeFungible,
			Created:   fftypes.UnixTime(0),
			Active:    false,
		},
	}

	or.mth.On("GetTransactionByIDCached", mock.Anything, txID).Return(tx, nil)
	or.mdi.On("GetOperations", mock.Anything, "ns", mock.Anything).Return(ops, nil, nil)
	or.mdi.On("GetBlockchainEvents", mock.Anything, "ns", mock.Anything).Return(events, nil, nil)
	or.mdi.On("GetTokenPools", mock.Anything, "ns", mock.Anything).Return(pools, nil, nil)

	status, err := or.GetTransactionStatus(context.Background(), txID.String())
	assert.NoError(t, err)

	expectedStatus := compactJSON(`{
		"status": "Pending",
		"details": [
			{
				"type": "Operation",
				"subtype": "token_create_pool",
				"status": "Succeeded",
				"id": "` + ops[0].ID.String() + `",
				"info": {"transactionHash": "0x100"}
			},
			{
				"type": "TokenPool",
				"subtype": "fungible",
				"status": "Pending",
				"id": "` + pools[0].ID.String() + `"
			}
		]
	}`)
	statusJSON, _ := json.Marshal(status)
	assert.Equal(t, expectedStatus, string(statusJSON))

	or.mdi.AssertExpectations(t)
}

func TestGetTransactionStatusTokenTransferSuccess(t *testing.T) {
	or := newTestOrchestrator()

	txID := fftypes.NewUUID()
	tx := &core.Transaction{
		Namespace: "ns1",
		Type:      core.TransactionTypeTokenTransfer,
	}
	ops := []*core.Operation{
		{
			Namespace: "ns1",
			Status:    core.OpStatusSucceeded,
			ID:        fftypes.NewUUID(),
			Type:      core.OpTypeTokenTransfer,
			Updated:   fftypes.UnixTime(0),
			Output:    fftypes.JSONObject{"transactionHash": "0x100"},
		},
	}
	events := []*core.BlockchainEvent{
		{
			Namespace: "ns1",
			Name:      "Mint",
			ID:        fftypes.NewUUID(),
			Timestamp: fftypes.UnixTime(0),
			Info:      fftypes.JSONObject{"transactionHash": "0x100"},
		},
	}
	transfers := []*core.TokenTransfer{
		{
			Namespace: "ns1",
			LocalID:   fftypes.NewUUID(),
			Type:      core.TokenTransferTypeMint,
			Created:   fftypes.UnixTime(0),
		},
	}

	or.mth.On("GetTransactionByIDCached", mock.Anything, txID).Return(tx, nil)
	or.mdi.On("GetOperations", mock.Anything, "ns", mock.Anything).Return(ops, nil, nil)
	or.mdi.On("GetBlockchainEvents", mock.Anything, "ns", mock.Anything).Return(events, nil, nil)
	or.mdi.On("GetTokenTransfers", mock.Anything, "ns", mock.Anything).Return(transfers, nil, nil)

	status, err := or.GetTransactionStatus(context.Background(), txID.String())
	assert.NoError(t, err)

	expectedStatus := compactJSON(`{
		"status": "Succeeded",
		"details": [
			{
				"type": "Operation",
				"subtype": "token_transfer",
				"status": "Succeeded",
				"timestamp": "1970-01-01T00:00:00Z",
				"id": "` + ops[0].ID.String() + `",
				"info": {"transactionHash": "0x100"}
			},
			{
				"type": "BlockchainEvent",
				"subtype": "Mint",
				"status": "Succeeded",
				"timestamp": "1970-01-01T00:00:00Z",
				"id": "` + events[0].ID.String() + `",
				"info": {"transactionHash": "0x100"}
			},
			{
				"type": "TokenTransfer",
				"subtype": "mint",
				"status": "Succeeded",
				"timestamp": "1970-01-01T00:00:00Z",
				"id": "` + transfers[0].LocalID.String() + `"
			}
		]
	}`)
	statusJSON, _ := json.Marshal(status)
	assert.Equal(t, expectedStatus, string(statusJSON))

	or.mdi.AssertExpectations(t)
}

func TestGetTransactionStatusTokenApprovalSuccess(t *testing.T) {
	or := newTestOrchestrator()

	txID := fftypes.NewUUID()
	tx := &core.Transaction{
		Namespace: "ns1",
		Type:      core.TransactionTypeTokenApproval,
	}
	ops := []*core.Operation{
		{
			Namespace: "ns1",
			Status:    core.OpStatusSucceeded,
			ID:        fftypes.NewUUID(),
			Type:      core.OpTypeTokenApproval,
			Updated:   fftypes.UnixTime(0),
			Output:    fftypes.JSONObject{"transactionHash": "0x100"},
		},
	}
	events := []*core.BlockchainEvent{
		{
			Namespace: "ns1",
			ID:        fftypes.NewUUID(),
			Timestamp: fftypes.UnixTime(0),
			Info:      fftypes.JSONObject{"transactionHash": "0x100"},
		},
	}
	approvals := []*core.TokenApproval{
		{
			Namespace: "ns1",
			LocalID:   fftypes.NewUUID(),
			Created:   fftypes.UnixTime(0),
		},
	}

	or.mth.On("GetTransactionByIDCached", mock.Anything, txID).Return(tx, nil)
	or.mdi.On("GetOperations", mock.Anything, "ns", mock.Anything).Return(ops, nil, nil)
	or.mdi.On("GetBlockchainEvents", mock.Anything, "ns", mock.Anything).Return(events, nil, nil)
	or.mdi.On("GetTokenApprovals", mock.Anything, "ns", mock.Anything).Return(approvals, nil, nil)

	status, err := or.GetTransactionStatus(context.Background(), txID.String())
	assert.NoError(t, err)

	expectedStatus := compactJSON(`{
		"status": "Succeeded",
		"details": [
			{
				"type": "Operation",
				"subtype": "token_approval",
				"status": "Succeeded",
				"timestamp": "1970-01-01T00:00:00Z",
				"id": "` + ops[0].ID.String() + `",
				"info": {"transactionHash": "0x100"}
			},
			{
				"type": "BlockchainEvent",
				"status": "Succeeded",
				"timestamp": "1970-01-01T00:00:00Z",
				"id": "` + events[0].ID.String() + `",
				"info": {"transactionHash": "0x100"}
			},
			{
				"type": "TokenApproval",
				"status": "Succeeded",
				"timestamp": "1970-01-01T00:00:00Z",
				"id": "` + approvals[0].LocalID.String() + `"
			}
		]
	}`)
	statusJSON, _ := json.Marshal(status)
	assert.Equal(t, expectedStatus, string(statusJSON))

	or.mdi.AssertExpectations(t)
}

func TestGetTransactionStatusTokenTransferPending(t *testing.T) {
	or := newTestOrchestrator()

	txID := fftypes.NewUUID()
	tx := &core.Transaction{
		Namespace: "ns1",
		Type:      core.TransactionTypeTokenTransfer,
	}
	ops := []*core.Operation{
		{
			Namespace: "ns1",
			Status:    core.OpStatusSucceeded,
			ID:        fftypes.NewUUID(),
			Type:      core.OpTypeTokenTransfer,
			Output:    fftypes.JSONObject{"transactionHash": "0x100"},
		},
	}
	events := []*core.BlockchainEvent{}
	transfers := []*core.TokenTransfer{}

	or.mth.On("GetTransactionByIDCached", mock.Anything, txID).Return(tx, nil)
	or.mdi.On("GetOperations", mock.Anything, "ns", mock.Anything).Return(ops, nil, nil)
	or.mdi.On("GetBlockchainEvents", mock.Anything, "ns", mock.Anything).Return(events, nil, nil)
	or.mdi.On("GetTokenTransfers", mock.Anything, "ns", mock.Anything).Return(transfers, nil, nil)

	status, err := or.GetTransactionStatus(context.Background(), txID.String())
	assert.NoError(t, err)

	expectedStatus := compactJSON(`{
		"status": "Pending",
		"details": [
			{
				"type": "Operation",
				"subtype": "token_transfer",
				"status": "Succeeded",
				"id": "` + ops[0].ID.String() + `",
				"info": {"transactionHash": "0x100"}
			},
			{
				"type": "BlockchainEvent",
				"status": "Pending"
			},
			{
				"type": "TokenTransfer",
				"status": "Pending"
			}
		]
	}`)
	statusJSON, _ := json.Marshal(status)
	assert.Equal(t, expectedStatus, string(statusJSON))

	or.mdi.AssertExpectations(t)
}

func TestGetTransactionStatusTokenTransferRetry(t *testing.T) {
	or := newTestOrchestrator()

	txID := fftypes.NewUUID()
	tx := &core.Transaction{
		Namespace: "ns1",
		Type:      core.TransactionTypeTokenTransfer,
	}
	op1ID := fftypes.NewUUID()
	op2ID := fftypes.NewUUID()
	ops := []*core.Operation{
		{
			Namespace: "ns1",
			Status:    core.OpStatusFailed,
			ID:        op1ID,
			Type:      core.OpTypeTokenTransfer,
			Retry:     op2ID,
		},
		{
			Namespace: "ns1",
			Status:    core.OpStatusPending,
			ID:        op2ID,
			Type:      core.OpTypeTokenTransfer,
		},
	}
	events := []*core.BlockchainEvent{}
	transfers := []*core.TokenTransfer{}

	or.mth.On("GetTransactionByIDCached", mock.Anything, txID).Return(tx, nil)
	or.mdi.On("GetOperations", mock.Anything, "ns", mock.Anything).Return(ops, nil, nil)
	or.mom.On("GetOperationByIDCached", mock.Anything, op1ID).Return(ops[0], nil)
	or.mom.On("GetOperationByIDCached", mock.Anything, op2ID).Return(ops[1], nil)
	or.mbi.On("GetTransactionStatus", mock.Anything, mock.Anything).Return(nil, nil)
	or.mdi.On("GetBlockchainEvents", mock.Anything, "ns", mock.Anything).Return(events, nil, nil)
	or.mdi.On("GetTokenTransfers", mock.Anything, "ns", mock.Anything).Return(transfers, nil, nil)

	status, err := or.GetTransactionStatus(context.Background(), txID.String())
	assert.NoError(t, err)

	expectedStatus := compactJSON(`{
		"status": "Pending",
		"details": [
			{
				"type": "Operation",
				"subtype": "token_transfer",
				"status": "Failed",
				"id": "` + op1ID.String() + `"
			},
			{
				"type": "Operation",
				"subtype": "token_transfer",
				"status": "Pending",
				"id": "` + op2ID.String() + `"
			},
			{
				"type": "BlockchainEvent",
				"status": "Pending"
			},
			{
				"type": "TokenTransfer",
				"status": "Pending"
			}
		]
	}`)
	statusJSON, _ := json.Marshal(status)
	assert.Equal(t, expectedStatus, string(statusJSON))

	or.mdi.AssertExpectations(t)
}

func TestGetTransactionStatusTokenApprovalPending(t *testing.T) {
	or := newTestOrchestrator()

	txID := fftypes.NewUUID()
	tx := &core.Transaction{
		Namespace: "ns1",
		Type:      core.TransactionTypeTokenApproval,
	}
	ops := []*core.Operation{
		{
			Namespace: "ns1",
			Status:    core.OpStatusSucceeded,
			ID:        fftypes.NewUUID(),
			Type:      core.OpTypeTokenApproval,
			Output:    fftypes.JSONObject{"transactionHash": "0x100"},
		},
	}
	events := []*core.BlockchainEvent{}
	approvals := []*core.TokenApproval{}

	or.mth.On("GetTransactionByIDCached", mock.Anything, txID).Return(tx, nil)
	or.mdi.On("GetOperations", mock.Anything, "ns", mock.Anything).Return(ops, nil, nil)
	or.mdi.On("GetBlockchainEvents", mock.Anything, "ns", mock.Anything).Return(events, nil, nil)
	or.mdi.On("GetTokenApprovals", mock.Anything, "ns", mock.Anything).Return(approvals, nil, nil)

	status, err := or.GetTransactionStatus(context.Background(), txID.String())
	assert.NoError(t, err)

	expectedStatus := compactJSON(`{
		"status": "Pending",
		"details": [
			{
				"type": "Operation",
				"subtype": "token_approval",
				"status": "Succeeded",
				"id": "` + ops[0].ID.String() + `",
				"info": {"transactionHash": "0x100"}
			},
			{
				"type": "BlockchainEvent",
				"status": "Pending"
			},
			{
				"type": "TokenApproval",
				"status": "Pending"
			}
		]
	}`)
	statusJSON, _ := json.Marshal(status)
	assert.Equal(t, expectedStatus, string(statusJSON))

	or.mdi.AssertExpectations(t)
}

func TestGetTransactionStatusContractInvokeSuccess(t *testing.T) {
	or := newTestOrchestrator()

	txID := fftypes.NewUUID()
	tx := &core.Transaction{
		Namespace: "ns1",
		Type:      core.TransactionTypeContractInvoke,
	}
	ops := []*core.Operation{
		{
			Namespace: "ns1",
			Status:    core.OpStatusSucceeded,
			ID:        fftypes.NewUUID(),
			Type:      core.OpTypeBlockchainInvoke,
			Updated:   fftypes.UnixTime(0),
			Output:    fftypes.JSONObject{"transactionHash": "0x100"},
		},
	}
	events := []*core.BlockchainEvent{}

	or.mth.On("GetTransactionByIDCached", mock.Anything, txID).Return(tx, nil)
	or.mdi.On("GetOperations", mock.Anything, "ns", mock.Anything).Return(ops, nil, nil)
	or.mdi.On("GetBlockchainEvents", mock.Anything, "ns", mock.Anything).Return(events, nil, nil)

	status, err := or.GetTransactionStatus(context.Background(), txID.String())
	assert.NoError(t, err)

	expectedStatus := compactJSON(`{
		"status": "Succeeded",
		"details": [
			{
				"type": "Operation",
				"subtype": "blockchain_invoke",
				"status": "Succeeded",
				"timestamp": "1970-01-01T00:00:00Z",
				"id": "` + ops[0].ID.String() + `",
				"info": {"transactionHash": "0x100"}
			}
		]
	}`)
	statusJSON, _ := json.Marshal(status)
	assert.Equal(t, expectedStatus, string(statusJSON))

	or.mdi.AssertExpectations(t)
}

func TestGetTransactionStatusTXError(t *testing.T) {
	or := newTestOrchestrator()

	txID := fftypes.NewUUID()
	or.mth.On("GetTransactionByIDCached", mock.Anything, txID).Return(nil, fmt.Errorf("pop"))

	_, err := or.GetTransactionStatus(context.Background(), txID.String())
	assert.EqualError(t, err, "pop")

	or.mdi.AssertExpectations(t)
}

func TestGetTransactionStatusNotFound(t *testing.T) {
	or := newTestOrchestrator()

	txID := fftypes.NewUUID()
	or.mth.On("GetTransactionByIDCached", mock.Anything, txID).Return(nil, nil)

	_, err := or.GetTransactionStatus(context.Background(), txID.String())
	assert.Regexp(t, "FF10109", err)

	or.mdi.AssertExpectations(t)
}

func TestGetTransactionStatusOpError(t *testing.T) {
	or := newTestOrchestrator()

	txID := fftypes.NewUUID()
	or.mth.On("GetTransactionByIDCached", mock.Anything, txID).Return(&core.Transaction{
		Namespace: "ns1",
	}, nil)
	or.mdi.On("GetOperations", mock.Anything, "ns", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	_, err := or.GetTransactionStatus(context.Background(), txID.String())
	assert.EqualError(t, err, "pop")

	or.mdi.AssertExpectations(t)
}

func TestGetTransactionStatusBlockchainEventError(t *testing.T) {
	or := newTestOrchestrator()

	txID := fftypes.NewUUID()
	or.mth.On("GetTransactionByIDCached", mock.Anything, txID).Return(&core.Transaction{
		Namespace: "ns1",
	}, nil)
	or.mdi.On("GetOperations", mock.Anything, "ns", mock.Anything).Return(nil, nil, nil)
	or.mdi.On("GetBlockchainEvents", mock.Anything, "ns", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	_, err := or.GetTransactionStatus(context.Background(), txID.String())
	assert.EqualError(t, err, "pop")

	or.mdi.AssertExpectations(t)
}

func TestGetTransactionStatusBatchError(t *testing.T) {
	or := newTestOrchestrator()

	txID := fftypes.NewUUID()
	tx := &core.Transaction{
		Namespace: "ns1",
		Type:      core.TransactionTypeBatchPin,
	}

	or.mth.On("GetTransactionByIDCached", mock.Anything, txID).Return(tx, nil)
	or.mdi.On("GetOperations", mock.Anything, "ns", mock.Anything).Return(nil, nil, nil)
	or.mdi.On("GetBlockchainEvents", mock.Anything, "ns", mock.Anything).Return(nil, nil, nil)
	or.mdi.On("GetBatches", mock.Anything, "ns", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	_, err := or.GetTransactionStatus(context.Background(), txID.String())
	assert.EqualError(t, err, "pop")

	or.mdi.AssertExpectations(t)
}

func TestGetTransactionStatusPoolError(t *testing.T) {
	or := newTestOrchestrator()

	txID := fftypes.NewUUID()
	tx := &core.Transaction{
		Namespace: "ns1",
		Type:      core.TransactionTypeTokenPool,
	}

	or.mth.On("GetTransactionByIDCached", mock.Anything, txID).Return(tx, nil)
	or.mdi.On("GetOperations", mock.Anything, "ns", mock.Anything).Return(nil, nil, nil)
	or.mdi.On("GetBlockchainEvents", mock.Anything, "ns", mock.Anything).Return(nil, nil, nil)
	or.mdi.On("GetTokenPools", mock.Anything, "ns", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	_, err := or.GetTransactionStatus(context.Background(), txID.String())
	assert.EqualError(t, err, "pop")

	or.mdi.AssertExpectations(t)
}

func TestGetTransactionStatusTransferError(t *testing.T) {
	or := newTestOrchestrator()

	txID := fftypes.NewUUID()
	tx := &core.Transaction{
		Namespace: "ns1",
		Type:      core.TransactionTypeTokenTransfer,
	}

	or.mth.On("GetTransactionByIDCached", mock.Anything, txID).Return(tx, nil)
	or.mdi.On("GetOperations", mock.Anything, "ns", mock.Anything).Return(nil, nil, nil)
	or.mdi.On("GetBlockchainEvents", mock.Anything, "ns", mock.Anything).Return(nil, nil, nil)
	or.mdi.On("GetTokenTransfers", mock.Anything, "ns", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	_, err := or.GetTransactionStatus(context.Background(), txID.String())
	assert.EqualError(t, err, "pop")

	or.mdi.AssertExpectations(t)
}

func TestGetTransactionStatusApprovalError(t *testing.T) {
	or := newTestOrchestrator()

	txID := fftypes.NewUUID()
	tx := &core.Transaction{
		Namespace: "ns1",
		Type:      core.TransactionTypeTokenApproval,
	}

	or.mth.On("GetTransactionByIDCached", mock.Anything, txID).Return(tx, nil)
	or.mdi.On("GetOperations", mock.Anything, "ns", mock.Anything).Return(nil, nil, nil)
	or.mdi.On("GetBlockchainEvents", mock.Anything, "ns", mock.Anything).Return(nil, nil, nil)
	or.mdi.On("GetTokenApprovals", mock.Anything, "ns", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	_, err := or.GetTransactionStatus(context.Background(), txID.String())
	assert.EqualError(t, err, "pop")

	or.mdi.AssertExpectations(t)
}

func TestGetTransactionStatusUnknownType(t *testing.T) {
	or := newTestOrchestrator()

	txID := fftypes.NewUUID()
	tx := &core.Transaction{
		Namespace: "ns1",
		Type:      "bad",
	}

	or.mth.On("GetTransactionByIDCached", mock.Anything, txID).Return(tx, nil)
	or.mdi.On("GetOperations", mock.Anything, "ns", mock.Anything).Return(nil, nil, nil)
	or.mdi.On("GetBlockchainEvents", mock.Anything, "ns", mock.Anything).Return(nil, nil, nil)

	_, err := or.GetTransactionStatus(context.Background(), txID.String())
	assert.Regexp(t, "FF10336", err)

	or.mdi.AssertExpectations(t)
}

func TestGetTransactionStatusOpStatusError(t *testing.T) {
	or := newTestOrchestrator()

	txID := fftypes.NewUUID()
	tx := &core.Transaction{
		Namespace: "ns1",
		Type:      core.TransactionTypeTokenTransfer,
	}
	op1ID := fftypes.NewUUID()
	ops := []*core.Operation{
		{
			Namespace: "ns1",
			Status:    core.OpStatusPending,
			ID:        op1ID,
			Type:      core.OpTypeTokenTransfer,
		},
	}

	or.mth.On("GetTransactionByIDCached", mock.Anything, txID).Return(tx, nil)
	or.mdi.On("GetOperations", mock.Anything, "ns", mock.Anything).Return(ops, nil, nil)
	or.mom.On("GetOperationByIDCached", mock.Anything, op1ID).Return(nil, fmt.Errorf("pop"))

	_, err := or.GetTransactionStatus(context.Background(), txID.String())
	assert.EqualError(t, err, "pop")
}
