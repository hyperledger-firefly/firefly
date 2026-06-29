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

package multiparty

import (
	"context"
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/internal/txcommon"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestPrepareAndRunBatchPin(t *testing.T) {
	mp := newTestMultipartyManager()
	defer mp.cleanup(t)

	op := &core.Operation{
		Type:      core.OpTypeBlockchainPinBatch,
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	batch := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			ID: fftypes.NewUUID(),
			SignerRef: core.SignerRef{
				Key: "0x123",
			},
			Namespace: "ns1",
		},
	}
	contexts := []*fftypes.Bytes32{
		fftypes.NewRandB32(),
		fftypes.NewRandB32(),
	}
	addBatchPinInputs(op, batch.ID, contexts, "payload1")

	mp.mdi.On("GetBatchByID", context.Background(), "ns1", batch.ID).Return(batch, nil)
	mp.mbi.On("SubmitBatchPin", context.Background(), "ns1:"+op.ID.String(), "ns1", "0x123", mock.Anything, mock.Anything).Return(nil)

	po, err := mp.PrepareOperation(context.Background(), op)
	assert.NoError(t, err)
	assert.Equal(t, batch, po.Data.(txcommon.BatchPinData).Batch)

	_, phase, err := mp.RunOperation(context.Background(), opBatchPin(op, batch, contexts, "payload1"))

	assert.Equal(t, core.OpPhasePending, phase)
	assert.NoError(t, err)
}

func TestPrepareAndRunNetworkAction(t *testing.T) {
	mp := newTestMultipartyManager()
	defer mp.cleanup(t)

	op := &core.Operation{
		Type:      core.OpTypeBlockchainNetworkAction,
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	addNetworkActionInputs(op, core.NetworkActionTerminate, "0x123")

	mp.mbi.On("SubmitNetworkAction", context.Background(), "ns1:"+op.ID.String(), "0x123", core.NetworkActionTerminate, mock.Anything).Return(nil)

	po, err := mp.PrepareOperation(context.Background(), op)
	assert.NoError(t, err)
	assert.Equal(t, core.NetworkActionTerminate, po.Data.(networkActionData).Type)

	_, phase, err := mp.RunOperation(context.Background(), opNetworkAction(op, core.NetworkActionTerminate, "0x123"))

	assert.Equal(t, core.OpPhasePending, phase)
	assert.NoError(t, err)

	mp.mbi.AssertExpectations(t)
}

func TestPrepareOperationNotSupported(t *testing.T) {
	mp := newTestMultipartyManager()
	defer mp.cleanup(t)

	po, err := mp.PrepareOperation(context.Background(), &core.Operation{})

	assert.Nil(t, po)
	assert.Regexp(t, "FF10371", err)
}

func TestPrepareOperationBatchPinBadBatch(t *testing.T) {
	mp := newTestMultipartyManager()
	defer mp.cleanup(t)

	op := &core.Operation{
		Type:  core.OpTypeBlockchainPinBatch,
		Input: fftypes.JSONObject{"batch": "bad"},
	}

	_, err := mp.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF00138", err)
}

func TestPrepareOperationBatchPinBadContext(t *testing.T) {
	mp := newTestMultipartyManager()
	defer mp.cleanup(t)

	op := &core.Operation{
		Type: core.OpTypeBlockchainPinBatch,
		Input: fftypes.JSONObject{
			"batch":    fftypes.NewUUID().String(),
			"contexts": []string{"bad"},
		},
	}

	_, err := mp.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF00107", err)
}

func TestRunOperationNotSupported(t *testing.T) {
	mp := newTestMultipartyManager()
	defer mp.cleanup(t)

	_, phase, err := mp.RunOperation(context.Background(), &core.PreparedOperation{})

	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.Regexp(t, "FF10378", err)
}

func TestPrepareOperationBatchPinError(t *testing.T) {
	mp := newTestMultipartyManager()
	defer mp.cleanup(t)

	batchID := fftypes.NewUUID()
	op := &core.Operation{
		Type: core.OpTypeBlockchainPinBatch,
		Input: fftypes.JSONObject{
			"batch":    batchID.String(),
			"contexts": []string{},
		},
	}

	mp.mdi.On("GetBatchByID", context.Background(), "ns1", batchID).Return(nil, fmt.Errorf("pop"))

	_, err := mp.PrepareOperation(context.Background(), op)
	assert.EqualError(t, err, "pop")
}

func TestPrepareOperationBatchPinNotFound(t *testing.T) {
	mp := newTestMultipartyManager()
	defer mp.cleanup(t)

	batchID := fftypes.NewUUID()
	op := &core.Operation{
		Type: core.OpTypeBlockchainPinBatch,
		Input: fftypes.JSONObject{
			"batch":    batchID.String(),
			"contexts": []string{},
		},
	}

	mp.mdi.On("GetBatchByID", context.Background(), "ns1", batchID).Return(nil, nil)

	_, err := mp.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF10109", err)
}

func TestRunBatchPinV1(t *testing.T) {
	mp := newTestMultipartyManager()
	defer mp.cleanup(t)
	mp.namespace.Contracts.Active.Info.Version = 1

	op := &core.Operation{
		Type:      core.OpTypeBlockchainPinBatch,
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	batch := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			ID: fftypes.NewUUID(),
			SignerRef: core.SignerRef{
				Key: "0x123",
			},
			Namespace: "ns1",
		},
	}
	contexts := []*fftypes.Bytes32{
		fftypes.NewRandB32(),
		fftypes.NewRandB32(),
	}
	addBatchPinInputs(op, batch.ID, contexts, "payload1")

	mp.mbi.On("SubmitBatchPin", context.Background(), "ns1:"+op.ID.String(), "ns1", "0x123", mock.Anything, mock.Anything).Return(nil)

	_, phase, err := mp.RunOperation(context.Background(), opBatchPin(op, batch, contexts, "payload1"))

	assert.Equal(t, core.OpPhasePending, phase)
	assert.NoError(t, err)
}

func TestOperationUpdate(t *testing.T) {
	mp := newTestMultipartyManager()
	defer mp.cleanup(t)
	assert.NoError(t, mp.OnOperationUpdate(context.Background(), nil, nil))
}
