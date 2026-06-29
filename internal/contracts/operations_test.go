// Copyright © 2024 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package contracts

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/internal/batch"
	"github.com/hyperledger-firefly/firefly/internal/data"
	"github.com/hyperledger-firefly/firefly/internal/txcommon"
	"github.com/hyperledger-firefly/firefly/mocks/batchmocks"
	"github.com/hyperledger-firefly/firefly/mocks/blockchainmocks"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/mocks/datamocks"
	"github.com/hyperledger-firefly/firefly/mocks/txcommonmocks"
	"github.com/hyperledger-firefly/firefly/pkg/blockchain"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const sampleRequestLargeNumberInput = `{
	"location": {
	  "address": "0x1111"
    },
	"key": "0x123",
	"method": {
      "name": "set",
      "params": null,
      "returns": null
    },
	"input": {
	  "value": 10000000000000000000000000001
	}
}`

func reqWithMessage(msgType core.MessageType) *core.ContractCallRequest {
	return &core.ContractCallRequest{
		Key:      "0x123",
		Location: fftypes.JSONAnyPtr(`{"address":"0x1111"}`),
		Method: &fftypes.FFIMethod{
			Name: "set",
			Params: fftypes.FFIParams{
				{Name: "data", Schema: fftypes.JSONAnyPtr(`{"type":"string"}`)},
			},
		},
		Input: map[string]interface{}{
			"value": "1",
		},
		Message: &core.MessageInOut{
			Message: core.Message{
				Header: core.MessageHeader{
					ID:   fftypes.NewUUID(),
					Type: msgType,
				},
			},
		},
	}
}

func TestPrepareAndRunBlockchainInvoke(t *testing.T) {

	cm := newTestContractManager()

	op := &core.Operation{
		Type:      core.OpTypeBlockchainInvoke,
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	req := &core.ContractCallRequest{
		Key:      "0x123",
		Location: fftypes.JSONAnyPtr(`{"address":"0x1111"}`),
		Method: &fftypes.FFIMethod{
			Name: "set",
		},
		Input: map[string]interface{}{
			"value": "1",
		},
	}
	err := addBlockchainReqInputs(op, req)
	assert.NoError(t, err)

	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	opaqueData := "anything"
	mbi.On("ParseInterface", context.Background(), mock.MatchedBy(func(method *fftypes.FFIMethod) bool {
		return method.Name == req.Method.Name
	}), req.Errors).Return(opaqueData, nil)
	mbi.On("InvokeContract", context.Background(), "ns1:"+op.ID.String(), "0x123", mock.MatchedBy(func(loc *fftypes.JSONAny) bool {
		return loc.String() == req.Location.String()
	}), opaqueData, req.Input, req.Options, (*blockchain.BatchPin)(nil)).Return(false, nil)

	po, err := cm.PrepareOperation(context.Background(), op)
	assert.NoError(t, err)
	assert.Equal(t, req, po.Data.(txcommon.BlockchainInvokeData).Request)

	_, phase, err := cm.RunOperation(context.Background(), po)

	assert.Equal(t, core.OpPhasePending, phase)
	assert.NoError(t, err)

	mbi.AssertExpectations(t)
}

func TestPrepareAndRunBlockchainInvokeLargeNumberInput(t *testing.T) {

	cm := newTestContractManager()

	var req core.ContractCallRequest
	d := json.NewDecoder(bytes.NewReader([]byte(sampleRequestLargeNumberInput)))
	d.UseNumber()
	err := d.Decode(&req)
	assert.NoError(t, err)

	op := &core.Operation{
		Type:      core.OpTypeBlockchainInvoke,
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}

	err = addBlockchainReqInputs(op, req)
	assert.NoError(t, err)

	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	opaqueData := "anything"
	mbi.On("ParseInterface", context.Background(), mock.MatchedBy(func(method *fftypes.FFIMethod) bool {
		return method.Name == req.Method.Name
	}), req.Errors).Return(opaqueData, nil)
	mbi.On("InvokeContract", context.Background(), "ns1:"+op.ID.String(), "0x123", mock.MatchedBy(func(loc *fftypes.JSONAny) bool {
		return loc.String() == req.Location.String()
	}), opaqueData, req.Input, req.Options, (*blockchain.BatchPin)(nil)).Return(false, nil)

	po, err := cm.PrepareOperation(context.Background(), op)
	assert.NoError(t, err)
	assert.Equal(t, &req, po.Data.(txcommon.BlockchainInvokeData).Request)

	_, phase, err := cm.RunOperation(context.Background(), po)

	assert.Equal(t, core.OpPhasePending, phase)
	assert.NoError(t, err)

	mbi.AssertExpectations(t)
}

func TestPrepareAndRunBlockchainInvokeRejected(t *testing.T) {
	cm := newTestContractManager()

	op := &core.Operation{
		Type:      core.OpTypeBlockchainInvoke,
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	req := &core.ContractCallRequest{
		Key:      "0x123",
		Location: fftypes.JSONAnyPtr(`{"address":"0x1111"}`),
		Method: &fftypes.FFIMethod{
			Name: "set",
		},
		Input: map[string]interface{}{
			"value": "1",
		},
	}
	err := addBlockchainReqInputs(op, req)
	assert.NoError(t, err)

	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	opaqueData := "anything"
	mbi.On("ParseInterface", context.Background(), mock.MatchedBy(func(method *fftypes.FFIMethod) bool {
		return method.Name == req.Method.Name
	}), req.Errors).Return(opaqueData, nil)
	mbi.On("InvokeContract", context.Background(), "ns1:"+op.ID.String(), "0x123", mock.MatchedBy(func(loc *fftypes.JSONAny) bool {
		return loc.String() == req.Location.String()
	}), opaqueData, req.Input, req.Options, (*blockchain.BatchPin)(nil)).
		Return(true, fmt.Errorf("rejected"))

	po, err := cm.PrepareOperation(context.Background(), op)
	assert.NoError(t, err)
	assert.Equal(t, req, po.Data.(txcommon.BlockchainInvokeData).Request)

	_, phase, err := cm.RunOperation(context.Background(), po)

	assert.Equal(t, core.OpPhaseComplete, phase)
	assert.Regexp(t, "rejected", err)

	mbi.AssertExpectations(t)
}

func TestPrepareAndRunBlockchainInvokeRetryable(t *testing.T) {
	cm := newTestContractManager()

	op := &core.Operation{
		Type:      core.OpTypeBlockchainInvoke,
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	req := &core.ContractCallRequest{
		Key:      "0x123",
		Location: fftypes.JSONAnyPtr(`{"address":"0x1111"}`),
		Method: &fftypes.FFIMethod{
			Name: "set",
		},
		Input: map[string]interface{}{
			"value": "1",
		},
	}
	err := addBlockchainReqInputs(op, req)
	assert.NoError(t, err)

	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	opaqueData := "anything"
	mbi.On("ParseInterface", context.Background(), mock.MatchedBy(func(method *fftypes.FFIMethod) bool {
		return method.Name == req.Method.Name
	}), req.Errors).Return(opaqueData, nil)
	mbi.On("InvokeContract", context.Background(), "ns1:"+op.ID.String(), "0x123", mock.MatchedBy(func(loc *fftypes.JSONAny) bool {
		return loc.String() == req.Location.String()
	}), opaqueData, req.Input, req.Options, (*blockchain.BatchPin)(nil)).
		Return(false, fmt.Errorf("rejected"))

	po, err := cm.PrepareOperation(context.Background(), op)
	assert.NoError(t, err)
	assert.Equal(t, req, po.Data.(txcommon.BlockchainInvokeData).Request)

	_, phase, err := cm.RunOperation(context.Background(), po)

	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.Regexp(t, "rejected", err)

	mbi.AssertExpectations(t)
}

func TestPrepareAndRunBlockchainInvokeValidateFail(t *testing.T) {
	cm := newTestContractManager()

	op := &core.Operation{
		Type:      core.OpTypeBlockchainInvoke,
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	req := &core.ContractCallRequest{
		Key:      "0x123",
		Location: fftypes.JSONAnyPtr(`{"address":"0x1111"}`),
		Method: &fftypes.FFIMethod{
			Name: "set",
		},
		Input: map[string]interface{}{
			"value": "1",
		},
	}
	err := addBlockchainReqInputs(op, req)
	assert.NoError(t, err)

	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mbi.On("ParseInterface", context.Background(), mock.MatchedBy(func(method *fftypes.FFIMethod) bool {
		return method.Name == req.Method.Name
	}), req.Errors).Return(nil, fmt.Errorf("pop"))

	po, err := cm.PrepareOperation(context.Background(), op)
	assert.NoError(t, err)
	assert.Equal(t, req, po.Data.(txcommon.BlockchainInvokeData).Request)

	_, phase, err := cm.RunOperation(context.Background(), po)

	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.Regexp(t, "pop", err)

	mbi.AssertExpectations(t)
}

func TestPrepareAndRunBlockchainContractDeploy(t *testing.T) {
	cm := newTestContractManager()

	op := &core.Operation{
		Type:      core.OpTypeBlockchainContractDeploy,
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	signingKey := "0x2468"
	req := &core.ContractDeployRequest{
		Key:        signingKey,
		Definition: fftypes.JSONAnyPtr("[]"),
		Contract:   fftypes.JSONAnyPtr("\"0x123456\""),
		Input:      []interface{}{"one", "two", "three"},
	}
	err := addBlockchainReqInputs(op, req)
	assert.NoError(t, err)

	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mbi.On("DeployContract", context.Background(), "ns1:"+op.ID.String(), signingKey, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(false, nil)

	po, err := cm.PrepareOperation(context.Background(), op)
	assert.NoError(t, err)
	assert.Equal(t, req, po.Data.(blockchainContractDeployData).Request)

	_, phase, err := cm.RunOperation(context.Background(), po)

	assert.Equal(t, core.OpPhasePending, phase)
	assert.NoError(t, err)

	mbi.AssertExpectations(t)
}

func TestPrepareAndRunBlockchainContractDeployRejected(t *testing.T) {
	cm := newTestContractManager()

	op := &core.Operation{
		Type:      core.OpTypeBlockchainContractDeploy,
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	signingKey := "0x2468"
	req := &core.ContractDeployRequest{
		Key:        signingKey,
		Definition: fftypes.JSONAnyPtr("[]"),
		Contract:   fftypes.JSONAnyPtr("\"0x123456\""),
		Input:      []interface{}{"one", "two", "three"},
	}
	err := addBlockchainReqInputs(op, req)
	assert.NoError(t, err)

	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	mbi.On("DeployContract", context.Background(), "ns1:"+op.ID.String(), signingKey, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(true, fmt.Errorf("rejected"))

	po, err := cm.PrepareOperation(context.Background(), op)
	assert.NoError(t, err)
	assert.Equal(t, req, po.Data.(blockchainContractDeployData).Request)

	_, phase, err := cm.RunOperation(context.Background(), po)

	assert.Equal(t, core.OpPhaseComplete, phase)
	assert.Regexp(t, "rejected", err)

	mbi.AssertExpectations(t)
}

func TestPrepareOperationNotSupported(t *testing.T) {
	cm := newTestContractManager()

	po, err := cm.PrepareOperation(context.Background(), &core.Operation{})

	assert.Nil(t, po)
	assert.Regexp(t, "FF10371", err)
}

func TestPrepareOperationBlockchainDeployBadInput(t *testing.T) {
	cm := newTestContractManager()

	op := &core.Operation{
		Type:  core.OpTypeBlockchainContractDeploy,
		Input: fftypes.JSONObject{"input": "bad"},
	}

	_, err := cm.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF00127", err)
}

func TestPrepareOperationBlockchainInvokeBadInput(t *testing.T) {
	cm := newTestContractManager()

	op := &core.Operation{
		Type:  core.OpTypeBlockchainInvoke,
		Input: fftypes.JSONObject{"interface": "bad"},
	}

	_, err := cm.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF00127", err)
}

func TestPrepareOperationBlockchainInvokeWithPrivateBatch(t *testing.T) {
	cm := newTestContractManager()

	op := &core.Operation{
		Type:      core.OpTypeBlockchainInvoke,
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	req := reqWithMessage(core.MessageTypePrivate)
	err := addBlockchainReqInputs(op, req)
	assert.NoError(t, err)
	msg := &core.Message{
		Header: core.MessageHeader{
			ID: fftypes.NewUUID(),
		},
		BatchID: fftypes.NewUUID(),
	}
	storedBatch := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			Type: core.BatchTypePrivate,
		},
		TX: core.TransactionRef{
			ID: fftypes.NewUUID(),
		},
	}
	pin := fftypes.NewRandB32()

	mdm := cm.data.(*datamocks.Manager)
	mdm.On("GetMessageWithDataCached", context.Background(), req.Message.Header.ID, data.CRORequireBatchID, data.CRORequirePins).Return(msg, nil, true, nil)
	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetBatchByID", context.Background(), "ns1", msg.BatchID).Return(storedBatch, nil)
	mbp := cm.batch.(*batchmocks.Manager)
	mbp.On("LoadContexts", context.Background(), mock.MatchedBy(func(b *batch.DispatchPayload) bool {
		assert.Equal(t, storedBatch, &b.Batch)
		assert.Equal(t, []*core.Message{msg}, b.Messages)
		return true
	})).Run(func(args mock.Arguments) {
		payload := args[1].(*batch.DispatchPayload)
		payload.Pins = []*fftypes.Bytes32{pin}
	}).Return(nil)

	po, err := cm.PrepareOperation(context.Background(), op)
	assert.NoError(t, err)

	opData := po.Data.(txcommon.BlockchainInvokeData)
	assert.Equal(t, req, opData.Request)
	assert.Equal(t, storedBatch, opData.BatchPin.Batch)
	assert.Equal(t, []*fftypes.Bytes32{pin}, opData.BatchPin.Contexts)
	assert.Equal(t, "", opData.BatchPin.PayloadRef)

	mdm.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mbp.AssertExpectations(t)
}

func TestPrepareOperationBlockchainInvokeWithBroadcastBatch(t *testing.T) {
	cm := newTestContractManager()

	op := &core.Operation{
		Type:      core.OpTypeBlockchainInvoke,
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	req := reqWithMessage(core.MessageTypeBroadcast)
	err := addBlockchainReqInputs(op, req)
	assert.NoError(t, err)
	msg := &core.Message{
		Header: core.MessageHeader{
			ID: fftypes.NewUUID(),
		},
		BatchID: fftypes.NewUUID(),
	}
	storedBatch := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			Type: core.BatchTypeBroadcast,
		},
		TX: core.TransactionRef{
			ID: fftypes.NewUUID(),
		},
	}
	pin := fftypes.NewRandB32()
	uploadOp := &core.Operation{
		Output: fftypes.JSONObject{
			"payloadRef": "test-payload",
		},
	}

	mdm := cm.data.(*datamocks.Manager)
	mdm.On("GetMessageWithDataCached", context.Background(), req.Message.Header.ID, data.CRORequireBatchID, data.CRORequirePins).Return(msg, nil, true, nil)
	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetBatchByID", context.Background(), "ns1", msg.BatchID).Return(storedBatch, nil)
	mth := cm.txHelper.(*txcommonmocks.Helper)
	mth.On("FindOperationInTransaction", context.Background(), storedBatch.TX.ID, core.OpTypeSharedStorageUploadBatch).Return(uploadOp, nil)
	mbp := cm.batch.(*batchmocks.Manager)
	mbp.On("LoadContexts", context.Background(), mock.MatchedBy(func(b *batch.DispatchPayload) bool {
		assert.Equal(t, storedBatch, &b.Batch)
		assert.Equal(t, []*core.Message{msg}, b.Messages)
		return true
	})).Run(func(args mock.Arguments) {
		payload := args[1].(*batch.DispatchPayload)
		payload.Pins = []*fftypes.Bytes32{pin}
	}).Return(nil)

	po, err := cm.PrepareOperation(context.Background(), op)
	assert.NoError(t, err)

	opData := po.Data.(txcommon.BlockchainInvokeData)
	assert.Equal(t, req, opData.Request)
	assert.Equal(t, storedBatch, opData.BatchPin.Batch)
	assert.Equal(t, []*fftypes.Bytes32{pin}, opData.BatchPin.Contexts)
	assert.Equal(t, "test-payload", opData.BatchPin.PayloadRef)

	mdm.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mbp.AssertExpectations(t)
	mth.AssertExpectations(t)
}

func TestPrepareOperationBlockchainInvokeMessageFailure(t *testing.T) {
	cm := newTestContractManager()

	op := &core.Operation{
		Type:      core.OpTypeBlockchainInvoke,
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	req := reqWithMessage(core.MessageTypeBroadcast)
	err := addBlockchainReqInputs(op, req)
	assert.NoError(t, err)

	mdm := cm.data.(*datamocks.Manager)
	mdm.On("GetMessageWithDataCached", context.Background(), req.Message.Header.ID, data.CRORequireBatchID, data.CRORequirePins).Return(nil, nil, false, fmt.Errorf("pop"))

	_, err = cm.PrepareOperation(context.Background(), op)
	assert.EqualError(t, err, "pop")

	mdm.AssertExpectations(t)
}

func TestPrepareOperationBlockchainInvokeMessageNotFound(t *testing.T) {
	cm := newTestContractManager()

	op := &core.Operation{
		Type:      core.OpTypeBlockchainInvoke,
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	req := reqWithMessage(core.MessageTypeBroadcast)
	err := addBlockchainReqInputs(op, req)
	assert.NoError(t, err)

	mdm := cm.data.(*datamocks.Manager)
	mdm.On("GetMessageWithDataCached", context.Background(), req.Message.Header.ID, data.CRORequireBatchID, data.CRORequirePins).Return(nil, nil, false, nil)

	_, err = cm.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF10207", err)

	mdm.AssertExpectations(t)
}

func TestPrepareOperationBlockchainInvokeBatchFailure(t *testing.T) {
	cm := newTestContractManager()

	op := &core.Operation{
		Type:      core.OpTypeBlockchainInvoke,
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	req := reqWithMessage(core.MessageTypeBroadcast)
	err := addBlockchainReqInputs(op, req)
	assert.NoError(t, err)
	msg := &core.Message{
		Header: core.MessageHeader{
			ID: fftypes.NewUUID(),
		},
		BatchID: fftypes.NewUUID(),
	}

	mdm := cm.data.(*datamocks.Manager)
	mdm.On("GetMessageWithDataCached", context.Background(), req.Message.Header.ID, data.CRORequireBatchID, data.CRORequirePins).Return(msg, nil, true, nil)
	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetBatchByID", context.Background(), "ns1", msg.BatchID).Return(nil, fmt.Errorf("pop"))

	_, err = cm.PrepareOperation(context.Background(), op)
	assert.EqualError(t, err, "pop")

	mdm.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestPrepareOperationBlockchainInvokeBatchNotFound(t *testing.T) {
	cm := newTestContractManager()

	op := &core.Operation{
		Type:      core.OpTypeBlockchainInvoke,
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	req := reqWithMessage(core.MessageTypeBroadcast)
	err := addBlockchainReqInputs(op, req)
	assert.NoError(t, err)
	msg := &core.Message{
		Header: core.MessageHeader{
			ID: fftypes.NewUUID(),
		},
		BatchID: fftypes.NewUUID(),
	}

	mdm := cm.data.(*datamocks.Manager)
	mdm.On("GetMessageWithDataCached", context.Background(), req.Message.Header.ID, data.CRORequireBatchID, data.CRORequirePins).Return(msg, nil, true, nil)
	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetBatchByID", context.Background(), "ns1", msg.BatchID).Return(nil, nil)

	_, err = cm.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF10209", err)

	mdm.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestPrepareOperationBlockchainInvokeGetUploadFailure(t *testing.T) {
	cm := newTestContractManager()

	op := &core.Operation{
		Type:      core.OpTypeBlockchainInvoke,
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	req := reqWithMessage(core.MessageTypeBroadcast)
	err := addBlockchainReqInputs(op, req)
	assert.NoError(t, err)
	msg := &core.Message{
		Header: core.MessageHeader{
			ID: fftypes.NewUUID(),
		},
		BatchID: fftypes.NewUUID(),
	}
	storedBatch := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			Type: core.BatchTypeBroadcast,
		},
		TX: core.TransactionRef{
			ID: fftypes.NewUUID(),
		},
	}

	mdm := cm.data.(*datamocks.Manager)
	mdm.On("GetMessageWithDataCached", context.Background(), req.Message.Header.ID, data.CRORequireBatchID, data.CRORequirePins).Return(msg, nil, true, nil)
	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetBatchByID", context.Background(), "ns1", msg.BatchID).Return(storedBatch, nil)
	mth := cm.txHelper.(*txcommonmocks.Helper)
	mth.On("FindOperationInTransaction", context.Background(), storedBatch.TX.ID, core.OpTypeSharedStorageUploadBatch).Return(nil, fmt.Errorf("pop"))

	_, err = cm.PrepareOperation(context.Background(), op)
	assert.EqualError(t, err, "pop")

	mdm.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mth.AssertExpectations(t)
}

func TestPrepareOperationBlockchainInvokeGetUploadNotFound(t *testing.T) {
	cm := newTestContractManager()

	op := &core.Operation{
		Type:      core.OpTypeBlockchainInvoke,
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	req := reqWithMessage(core.MessageTypeBroadcast)
	err := addBlockchainReqInputs(op, req)
	assert.NoError(t, err)
	msg := &core.Message{
		Header: core.MessageHeader{
			ID: fftypes.NewUUID(),
		},
		BatchID: fftypes.NewUUID(),
	}
	storedBatch := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			Type: core.BatchTypeBroadcast,
		},
		TX: core.TransactionRef{
			ID: fftypes.NewUUID(),
		},
	}

	mdm := cm.data.(*datamocks.Manager)
	mdm.On("GetMessageWithDataCached", context.Background(), req.Message.Header.ID, data.CRORequireBatchID, data.CRORequirePins).Return(msg, nil, true, nil)
	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetBatchByID", context.Background(), "ns1", msg.BatchID).Return(storedBatch, nil)
	mth := cm.txHelper.(*txcommonmocks.Helper)
	mth.On("FindOperationInTransaction", context.Background(), storedBatch.TX.ID, core.OpTypeSharedStorageUploadBatch).Return(nil, nil)

	_, err = cm.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF10444", err)

	mdm.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mth.AssertExpectations(t)
}

func TestPrepareOperationBlockchainInvokeLoadContextsFail(t *testing.T) {
	cm := newTestContractManager()

	op := &core.Operation{
		Type:      core.OpTypeBlockchainInvoke,
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	req := reqWithMessage(core.MessageTypePrivate)
	err := addBlockchainReqInputs(op, req)
	assert.NoError(t, err)
	msg := &core.Message{
		Header: core.MessageHeader{
			ID: fftypes.NewUUID(),
		},
		BatchID: fftypes.NewUUID(),
	}
	storedBatch := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			Type: core.BatchTypePrivate,
		},
		TX: core.TransactionRef{
			ID: fftypes.NewUUID(),
		},
	}

	mdm := cm.data.(*datamocks.Manager)
	mdm.On("GetMessageWithDataCached", context.Background(), req.Message.Header.ID, data.CRORequireBatchID, data.CRORequirePins).Return(msg, nil, true, nil)
	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("GetBatchByID", context.Background(), "ns1", msg.BatchID).Return(storedBatch, nil)
	mbp := cm.batch.(*batchmocks.Manager)
	mbp.On("LoadContexts", context.Background(), mock.MatchedBy(func(b *batch.DispatchPayload) bool {
		assert.Equal(t, storedBatch, &b.Batch)
		assert.Equal(t, []*core.Message{msg}, b.Messages)
		return true
	})).Return(fmt.Errorf("pop"))

	_, err = cm.PrepareOperation(context.Background(), op)
	assert.EqualError(t, err, "pop")

	mdm.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mbp.AssertExpectations(t)
}

func TestRunBlockchainInvokeWithBatch(t *testing.T) {
	cm := newTestContractManager()

	op := &core.Operation{
		Type:      core.OpTypeBlockchainInvoke,
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	req := reqWithMessage(core.MessageTypePrivate)
	err := addBlockchainReqInputs(op, req)
	assert.NoError(t, err)
	storedBatch := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			ID: fftypes.NewUUID(),
		},
		Hash: fftypes.NewRandB32(),
		TX: core.TransactionRef{
			ID: fftypes.NewUUID(),
		},
	}
	pin := fftypes.NewRandB32()

	mbi := cm.blockchain.(*blockchainmocks.Plugin)
	opaqueData := "anything"
	mbi.On("ParseInterface", context.Background(), mock.MatchedBy(func(method *fftypes.FFIMethod) bool {
		return method.Name == req.Method.Name
	}), req.Errors).Return(opaqueData, nil)
	mbi.On("InvokeContract", context.Background(), "ns1:"+op.ID.String(), "0x123", mock.MatchedBy(func(loc *fftypes.JSONAny) bool {
		return loc.String() == req.Location.String()
	}), opaqueData, req.Input, req.Options, mock.MatchedBy(func(batchPin *blockchain.BatchPin) bool {
		assert.Equal(t, storedBatch.ID, batchPin.BatchID)
		assert.Equal(t, storedBatch.Hash, batchPin.BatchHash)
		assert.Equal(t, storedBatch.TX.ID, batchPin.TransactionID)
		assert.Equal(t, []*fftypes.Bytes32{pin}, batchPin.Contexts)
		assert.Equal(t, "test-payload", batchPin.BatchPayloadRef)
		return true
	})).Return(false, nil)

	po := txcommon.OpBlockchainInvoke(op, req, &txcommon.BatchPinData{
		Batch:      storedBatch,
		Contexts:   []*fftypes.Bytes32{pin},
		PayloadRef: "test-payload",
	})
	_, phase, err := cm.RunOperation(context.Background(), po)

	assert.Equal(t, core.OpPhasePending, phase)
	assert.NoError(t, err)

	mbi.AssertExpectations(t)
}

func TestRunOperationNotSupported(t *testing.T) {
	cm := newTestContractManager()

	_, phase, err := cm.RunOperation(context.Background(), &core.PreparedOperation{})

	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.Regexp(t, "FF10378", err)
}

func TestOperationUpdate(t *testing.T) {
	cm := newTestContractManager()

	op := &core.Operation{}

	err := cm.OnOperationUpdate(context.Background(), op, nil)
	assert.NoError(t, err)
}

func TestOperationUpdateDeploySucceed(t *testing.T) {
	cm := newTestContractManager()

	op := &core.Operation{
		ID:   fftypes.NewUUID(),
		Type: core.OpTypeBlockchainContractDeploy,
	}
	update := &core.OperationUpdate{
		Status: core.OpStatusSucceeded,
	}

	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("InsertEvent", context.Background(), mock.MatchedBy(func(event *core.Event) bool {
		return event.Type == core.EventTypeBlockchainContractDeployOpSucceeded && *event.Reference == *op.ID
	})).Return(fmt.Errorf("pop"))

	err := cm.OnOperationUpdate(context.Background(), op, update)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestOperationUpdateDeployFail(t *testing.T) {
	cm := newTestContractManager()

	op := &core.Operation{
		ID:   fftypes.NewUUID(),
		Type: core.OpTypeBlockchainContractDeploy,
	}
	update := &core.OperationUpdate{
		Status: core.OpStatusFailed,
	}

	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("InsertEvent", context.Background(), mock.MatchedBy(func(event *core.Event) bool {
		return event.Type == core.EventTypeBlockchainContractDeployOpFailed && *event.Reference == *op.ID
	})).Return(fmt.Errorf("pop"))

	err := cm.OnOperationUpdate(context.Background(), op, update)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestOperationUpdateInvokeSucceed(t *testing.T) {
	cm := newTestContractManager()

	op := &core.Operation{
		ID:   fftypes.NewUUID(),
		Type: core.OpTypeBlockchainInvoke,
	}
	update := &core.OperationUpdate{
		Status: core.OpStatusSucceeded,
	}

	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("InsertEvent", context.Background(), mock.MatchedBy(func(event *core.Event) bool {
		return event.Type == core.EventTypeBlockchainInvokeOpSucceeded && *event.Reference == *op.ID
	})).Return(fmt.Errorf("pop"))

	err := cm.OnOperationUpdate(context.Background(), op, update)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestOperationUpdateInvokeFail(t *testing.T) {
	cm := newTestContractManager()

	op := &core.Operation{
		ID:   fftypes.NewUUID(),
		Type: core.OpTypeBlockchainInvoke,
	}
	update := &core.OperationUpdate{
		Status: core.OpStatusFailed,
	}

	mdi := cm.database.(*databasemocks.Plugin)
	mdi.On("InsertEvent", context.Background(), mock.MatchedBy(func(event *core.Event) bool {
		return event.Type == core.EventTypeBlockchainInvokeOpFailed && *event.Reference == *op.ID
	})).Return(fmt.Errorf("pop"))

	err := cm.OnOperationUpdate(context.Background(), op, update)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}
