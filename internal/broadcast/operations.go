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

package broadcast

import (
	"bytes"
	"context"
	"encoding/json"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

type uploadBatchData struct {
	Batch *core.Batch `json:"batch"`
}

type uploadBlobData struct {
	Data *core.Data `json:"data"`
	Blob *core.Blob `json:"blob"`
}

type uploadValue struct {
	Data *core.Data `json:"data"`
}

func addUploadBatchInputs(op *core.Operation, batchID *fftypes.UUID) {
	op.Input = fftypes.JSONObject{
		"id": batchID.String(),
	}
}

func getUploadBatchOutputs(payloadRef string) fftypes.JSONObject {
	return fftypes.JSONObject{
		"payloadRef": payloadRef,
	}
}

func addUploadBlobInputs(op *core.Operation, dataID *fftypes.UUID) {
	op.Input = fftypes.JSONObject{
		"dataId": dataID.String(),
	}
}

func addUploadValueInputs(op *core.Operation, dataID *fftypes.UUID) {
	op.Input = fftypes.JSONObject{
		"dataId": dataID.String(),
	}
}

func getUploadBlobOutputs(payloadRef string) fftypes.JSONObject {
	return fftypes.JSONObject{
		"payloadRef": payloadRef,
	}
}

func retrieveUploadBatchInputs(ctx context.Context, op *core.Operation) (*fftypes.UUID, error) {
	return fftypes.ParseUUID(ctx, op.Input.GetString("id"))
}

func retrieveUploadBlobInputs(ctx context.Context, op *core.Operation) (*fftypes.UUID, error) {
	return fftypes.ParseUUID(ctx, op.Input.GetString("dataId"))
}

func retrieveUploadValueInputs(ctx context.Context, op *core.Operation) (*fftypes.UUID, error) {
	return fftypes.ParseUUID(ctx, op.Input.GetString("dataId"))
}

func (bm *broadcastManager) PrepareOperation(ctx context.Context, op *core.Operation) (*core.PreparedOperation, error) {
	switch op.Type {
	case core.OpTypeSharedStorageUploadBatch:
		id, err := retrieveUploadBatchInputs(ctx, op)
		if err != nil {
			return nil, err
		}
		bp, err := bm.database.GetBatchByID(ctx, bm.namespace.Name, id)
		if err != nil {
			return nil, err
		} else if bp == nil {
			return nil, i18n.NewError(ctx, coremsgs.Msg404NotFound)
		}
		batch, err := bm.data.HydrateBatch(ctx, bp)
		if err != nil {
			return nil, err
		}
		return opUploadBatch(op, batch), nil

	case core.OpTypeSharedStorageUploadBlob:
		dataID, err := retrieveUploadBlobInputs(ctx, op)
		if err != nil {
			return nil, err
		}
		d, err := bm.database.GetDataByID(ctx, bm.namespace.Name, dataID, false)
		if err != nil {
			return nil, err
		} else if d == nil || d.Blob == nil {
			return nil, i18n.NewError(ctx, coremsgs.Msg404NotFound)
		}
		fb := database.BlobQueryFactory.NewFilter(ctx)
		blobs, _, err := bm.database.GetBlobs(ctx, bm.namespace.Name, fb.And(fb.Eq("data_id", dataID), fb.Eq("hash", d.Blob.Hash)))
		if err != nil {
			return nil, err
		} else if len(blobs) == 0 || blobs[0] == nil {
			return nil, i18n.NewError(ctx, coremsgs.Msg404NotFound)
		}
		return opUploadBlob(op, d, blobs[0]), nil

	case core.OpTypeSharedStorageUploadValue:
		dataID, err := retrieveUploadValueInputs(ctx, op)
		if err != nil {
			return nil, err
		}
		d, err := bm.database.GetDataByID(ctx, bm.namespace.Name, dataID, false)
		if err != nil {
			return nil, err
		} else if d == nil {
			return nil, i18n.NewError(ctx, coremsgs.Msg404NotFound)
		}
		return opUploadValue(op, d), nil
	default:
		return nil, i18n.NewError(ctx, coremsgs.MsgOperationNotSupported, op.Type)
	}
}

func (bm *broadcastManager) RunOperation(ctx context.Context, op *core.PreparedOperation) (outputs fftypes.JSONObject, phase core.OpPhase, err error) {
	switch data := op.Data.(type) {
	case uploadBatchData:
		return bm.uploadBatch(ctx, data)
	case uploadBlobData:
		return bm.uploadBlob(ctx, data)
	case uploadValue:
		return bm.uploadValue(ctx, data)
	default:
		return nil, core.OpPhaseInitializing, i18n.NewError(ctx, coremsgs.MsgOperationDataIncorrect, op.Data)
	}
}

// uploadBatch uploads the serialized batch to public storage
func (bm *broadcastManager) uploadBatch(ctx context.Context, data uploadBatchData) (outputs fftypes.JSONObject, phase core.OpPhase, err error) {
	// Serialize the full payload, which has already been sealed for us by the BatchManager
	data.Batch.Namespace = bm.namespace.NetworkName
	payload, err := json.Marshal(data.Batch)
	if err != nil {
		return nil, core.OpPhaseInitializing, i18n.WrapError(ctx, err, coremsgs.MsgSerializationFailed)
	}

	// Write it to IPFS to get a payload reference
	payloadRef, err := bm.sharedstorage.UploadData(ctx, bytes.NewReader(payload))
	if err != nil {
		return nil, core.OpPhaseInitializing, err
	}
	log.L(ctx).Infof("Published batch '%s' to shared storage: '%s'", data.Batch.ID, payloadRef)
	return getUploadBatchOutputs(payloadRef), core.OpPhaseComplete, nil
}

// uploadBlob streams a blob from the local data exchange, to public storage
func (bm *broadcastManager) uploadBlob(ctx context.Context, data uploadBlobData) (outputs fftypes.JSONObject, phase core.OpPhase, err error) {

	// Stream from the local data exchange ...
	reader, err := bm.exchange.DownloadBlob(ctx, data.Blob.PayloadRef)
	if err != nil {
		return nil, core.OpPhaseInitializing, i18n.WrapError(ctx, err, coremsgs.MsgDownloadBlobFailed, data.Blob.PayloadRef)
	}
	defer reader.Close()

	// ... to the shared storage
	data.Data.Blob.Public, err = bm.sharedstorage.UploadData(ctx, reader)
	if err != nil {
		return nil, core.OpPhaseInitializing, err
	}

	// Update the data in the DB
	err = bm.database.UpdateData(ctx, bm.namespace.Name, data.Data.ID, database.DataQueryFactory.NewUpdate(ctx).Set("blob.public", data.Data.Blob.Public))
	if err != nil {
		return nil, core.OpPhaseInitializing, err
	}

	log.L(ctx).Infof("Published blob with hash '%s' for data '%s' to shared storage: '%s'", data.Data.Blob.Hash, data.Data.ID, data.Data.Blob.Public)
	return getUploadBlobOutputs(data.Data.Blob.Public), core.OpPhaseComplete, nil
}

// uploadValue streams the value JSON from a data record to public storage
func (bm *broadcastManager) uploadValue(ctx context.Context, data uploadValue) (outputs fftypes.JSONObject, phase core.OpPhase, err error) {

	// Upload to shared storage
	data.Data.Public, err = bm.sharedstorage.UploadData(ctx, bytes.NewReader(data.Data.Value.Bytes()))
	if err != nil {
		return nil, core.OpPhaseInitializing, err
	}

	// Update the public reference for the data in the DB
	err = bm.database.UpdateData(ctx, bm.namespace.Name, data.Data.ID, database.DataQueryFactory.NewUpdate(ctx).Set("public", data.Data.Public))
	if err != nil {
		return nil, core.OpPhaseInitializing, err
	}

	log.L(ctx).Infof("Published value for data '%s' to shared storage: '%s'", data.Data.ID, data.Data.Public)
	return getUploadBlobOutputs(data.Data.Public), core.OpPhaseComplete, nil
}

func (bm *broadcastManager) OnOperationUpdate(ctx context.Context, op *core.Operation, update *core.OperationUpdate) error {
	return nil
}

func opUploadBatch(op *core.Operation, batch *core.Batch) *core.PreparedOperation {
	return &core.PreparedOperation{
		ID:        op.ID,
		Namespace: op.Namespace,
		Plugin:    op.Plugin,
		Type:      op.Type,
		Data:      uploadBatchData{Batch: batch},
	}
}

func opUploadBlob(op *core.Operation, data *core.Data, blob *core.Blob) *core.PreparedOperation {
	return &core.PreparedOperation{
		ID:        op.ID,
		Namespace: op.Namespace,
		Plugin:    op.Plugin,
		Type:      op.Type,
		Data:      uploadBlobData{Data: data, Blob: blob},
	}
}

func opUploadValue(op *core.Operation, data *core.Data) *core.PreparedOperation {
	return &core.PreparedOperation{
		ID:        op.ID,
		Namespace: op.Namespace,
		Plugin:    op.Plugin,
		Type:      op.Type,
		Data:      uploadValue{Data: data},
	}
}
