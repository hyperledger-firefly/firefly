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

package contracts

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/batch"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/internal/data"
	"github.com/hyperledger-firefly/firefly/internal/txcommon"
	"github.com/hyperledger-firefly/firefly/pkg/blockchain"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

type blockchainContractDeployData struct {
	Request *core.ContractDeployRequest `json:"request"`
}

func addBlockchainReqInputs(op *core.Operation, req interface{}) (err error) {
	var reqJSON []byte
	if reqJSON, err = json.Marshal(req); err == nil {
		d := json.NewDecoder(bytes.NewReader(reqJSON))
		d.UseNumber()
		err = d.Decode(&op.Input)
	}
	return err
}

func retrieveBlockchainDeployInputs(ctx context.Context, op *core.Operation) (*core.ContractDeployRequest, error) {
	var req core.ContractDeployRequest
	s := op.Input.String()
	if err := json.Unmarshal([]byte(s), &req); err != nil {
		return nil, i18n.WrapError(ctx, err, i18n.MsgJSONObjectParseFailed, s)
	}
	return &req, nil
}

func (cm *contractManager) PrepareOperation(ctx context.Context, op *core.Operation) (*core.PreparedOperation, error) {
	switch op.Type {
	case core.OpTypeBlockchainInvoke:
		req, err := txcommon.RetrieveBlockchainInvokeInputs(ctx, op)
		if err != nil {
			return nil, err
		}

		var batchPin *txcommon.BatchPinData
		if req.Message != nil && cm.batch != nil {
			msg, _, _, err := cm.data.GetMessageWithDataCached(ctx, req.Message.Header.ID, data.CRORequireBatchID, data.CRORequirePins)
			if err != nil {
				return nil, err
			} else if msg == nil {
				return nil, i18n.NewError(ctx, coremsgs.MsgMessageNotFound, req.Message.Header.ID)
			}
			msgBatch, err := cm.database.GetBatchByID(ctx, cm.namespace, msg.BatchID)
			if err != nil {
				return nil, err
			} else if msgBatch == nil {
				return nil, i18n.NewError(ctx, coremsgs.MsgBatchNotFound, msg.BatchID)
			}
			var payloadRef string
			if msgBatch.Type == core.BatchTypeBroadcast {
				uploadOp, err := cm.txHelper.FindOperationInTransaction(ctx, msgBatch.TX.ID, core.OpTypeSharedStorageUploadBatch)
				if err != nil {
					return nil, err
				} else if uploadOp == nil {
					return nil, i18n.NewError(ctx, coremsgs.MsgOperationNotFoundInTransaction, core.OpTypeSharedStorageUploadBatch, msgBatch.TX.ID)
				}
				payloadRef = uploadOp.Output.GetString("payloadRef")
			}
			payload := &batch.DispatchPayload{
				Batch:    *msgBatch,
				Messages: []*core.Message{msg},
			}
			err = cm.batch.LoadContexts(ctx, payload)
			if err != nil {
				return nil, err
			}
			batchPin = &txcommon.BatchPinData{
				Batch:      msgBatch,
				Contexts:   payload.Pins,
				PayloadRef: payloadRef,
			}
		}

		return txcommon.OpBlockchainInvoke(op, req, batchPin), nil

	case core.OpTypeBlockchainContractDeploy:
		req, err := retrieveBlockchainDeployInputs(ctx, op)
		if err != nil {
			return nil, err
		}
		return opBlockchainContractDeploy(op, req), nil

	default:
		return nil, i18n.NewError(ctx, coremsgs.MsgOperationNotSupported, op.Type)
	}
}

func (cm *contractManager) RunOperation(ctx context.Context, op *core.PreparedOperation) (outputs fftypes.JSONObject, phase core.OpPhase, err error) {
	switch data := op.Data.(type) {
	case txcommon.BlockchainInvokeData:
		req := data.Request
		var batchPin *blockchain.BatchPin
		if data.BatchPin != nil && data.BatchPin.Batch != nil {
			batchPin = &blockchain.BatchPin{
				TransactionID:   data.BatchPin.Batch.TX.ID,
				BatchID:         data.BatchPin.Batch.ID,
				BatchHash:       data.BatchPin.Batch.Hash,
				BatchPayloadRef: data.BatchPin.PayloadRef,
				Contexts:        data.BatchPin.Contexts,
			}
		}
		bcParsedMethod, err := cm.validateInvokeContractRequest(ctx, req, false /* do-not revalidate with the blockchain connector - just send it */)
		if err != nil {
			return nil, core.OpPhaseInitializing, err
		}
		submissionRejected, err := cm.blockchain.InvokeContract(ctx, op.NamespacedIDString(), req.Key, req.Location, bcParsedMethod, req.Input, req.Options, batchPin)
		return nil, submissionPhase(ctx, submissionRejected, err), err
	case blockchainContractDeployData:
		req := data.Request
		submissionRejected, err := cm.blockchain.DeployContract(ctx, op.NamespacedIDString(), req.Key, req.Definition, req.Contract, req.Input, req.Options)
		return nil, submissionPhase(ctx, submissionRejected, err), err
	default:
		return nil, core.OpPhaseInitializing, i18n.NewError(ctx, coremsgs.MsgOperationDataIncorrect, op.Data)
	}
}

func submissionPhase(ctx context.Context, submissionRejected bool, err error) core.OpPhase {
	if err == nil {
		return core.OpPhasePending
	}
	log.L(ctx).Errorf("Transaction submission failed [submissionRejected=%t]: %s", submissionRejected, err)
	if submissionRejected {
		return core.OpPhaseComplete
	}
	return core.OpPhaseInitializing
}

func (cm *contractManager) OnOperationUpdate(ctx context.Context, op *core.Operation, update *core.OperationUpdate) error {
	// Special handling for blockchain operations, which writes an event when it succeeds or fails
	switch op.Type {
	case core.OpTypeBlockchainInvoke:
		if update.Status == core.OpStatusSucceeded {
			event := core.NewEvent(core.EventTypeBlockchainInvokeOpSucceeded, op.Namespace, op.ID, op.Transaction, "")
			if err := cm.database.InsertEvent(ctx, event); err != nil {
				return err
			}
		}
		if update.Status == core.OpStatusFailed {
			event := core.NewEvent(core.EventTypeBlockchainInvokeOpFailed, op.Namespace, op.ID, op.Transaction, "")
			if err := cm.database.InsertEvent(ctx, event); err != nil {
				return err
			}
		}
	case core.OpTypeBlockchainContractDeploy:
		if update.Status == core.OpStatusSucceeded {
			event := core.NewEvent(core.EventTypeBlockchainContractDeployOpSucceeded, op.Namespace, op.ID, op.Transaction, "")
			if err := cm.database.InsertEvent(ctx, event); err != nil {
				return err
			}
		}
		if update.Status == core.OpStatusFailed {
			event := core.NewEvent(core.EventTypeBlockchainContractDeployOpFailed, op.Namespace, op.ID, op.Transaction, "")
			if err := cm.database.InsertEvent(ctx, event); err != nil {
				return err
			}
		}
	}
	return nil
}

func opBlockchainContractDeploy(op *core.Operation, req *core.ContractDeployRequest) *core.PreparedOperation {
	return &core.PreparedOperation{
		ID:        op.ID,
		Namespace: op.Namespace,
		Plugin:    op.Plugin,
		Type:      op.Type,
		Data:      blockchainContractDeployData{Request: req},
	}
}
