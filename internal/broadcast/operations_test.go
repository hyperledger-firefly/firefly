// Copyright © 2023 Kaleido, Inc.
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
package broadcast

import (
	"context"
	"fmt"
	"io/ioutil"
	"strings"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/mocks/dataexchangemocks"
	"github.com/hyperledger-firefly/firefly/mocks/datamocks"
	"github.com/hyperledger-firefly/firefly/mocks/sharedstoragemocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestPrepareAndRunBatchBroadcast(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	op := &core.Operation{
		Type: core.OpTypeSharedStorageUploadBatch,
	}
	bp := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			ID: fftypes.NewUUID(),
		},
	}
	batch := &core.Batch{
		BatchHeader: bp.BatchHeader,
	}
	addUploadBatchInputs(op, bp.ID)

	mps := bm.sharedstorage.(*sharedstoragemocks.Plugin)
	mdi := bm.database.(*databasemocks.Plugin)
	mdm := bm.data.(*datamocks.Manager)
	mdm.On("HydrateBatch", context.Background(), bp).Return(batch, nil)
	mdi.On("GetBatchByID", context.Background(), "ns1", bp.ID).Return(bp, nil)
	mps.On("UploadData", context.Background(), mock.Anything).Return("123", nil)

	po, err := bm.PrepareOperation(context.Background(), op)
	assert.NoError(t, err)
	assert.Equal(t, bp.ID, po.Data.(uploadBatchData).Batch.ID)

	_, phase, err := bm.RunOperation(context.Background(), opUploadBatch(op, batch))

	assert.Equal(t, core.OpPhaseComplete, phase)
	assert.NoError(t, err)

	mps.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mdm.AssertExpectations(t)
}

func TestPrepareAndRunBatchBroadcastHydrateFail(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	op := &core.Operation{
		Type: core.OpTypeSharedStorageUploadBatch,
	}
	bp := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			ID: fftypes.NewUUID(),
		},
	}
	addUploadBatchInputs(op, bp.ID)

	mps := bm.sharedstorage.(*sharedstoragemocks.Plugin)
	mdi := bm.database.(*databasemocks.Plugin)
	mdm := bm.data.(*datamocks.Manager)
	mdm.On("HydrateBatch", context.Background(), bp).Return(nil, fmt.Errorf("pop"))
	mdi.On("GetBatchByID", context.Background(), "ns1", bp.ID).Return(bp, nil)

	_, err := bm.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "pop", err)

	mps.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mdm.AssertExpectations(t)
}

func TestPrepareOperationNotSupported(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	_, err := bm.PrepareOperation(context.Background(), &core.Operation{})

	assert.Regexp(t, "FF10371", err)
}

func TestPrepareOperationBatchBroadcastBadInput(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	op := &core.Operation{
		Type:  core.OpTypeSharedStorageUploadBatch,
		Input: fftypes.JSONObject{"id": "bad"},
	}

	_, err := bm.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF00138", err)
}

func TestPrepareOperationBatchBroadcastError(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	batchID := fftypes.NewUUID()
	op := &core.Operation{
		Type:  core.OpTypeSharedStorageUploadBatch,
		Input: fftypes.JSONObject{"id": batchID.String()},
	}

	mdi := bm.database.(*databasemocks.Plugin)
	mdi.On("GetBatchByID", context.Background(), "ns1", batchID).Return(nil, fmt.Errorf("pop"))

	_, err := bm.PrepareOperation(context.Background(), op)
	assert.EqualError(t, err, "pop")
}

func TestPrepareOperationBatchBroadcastNotFound(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	batchID := fftypes.NewUUID()
	op := &core.Operation{
		Type:  core.OpTypeSharedStorageUploadBatch,
		Input: fftypes.JSONObject{"id": batchID.String()},
	}

	mdi := bm.database.(*databasemocks.Plugin)
	mdi.On("GetBatchByID", context.Background(), "ns1", batchID).Return(nil, nil)

	_, err := bm.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF10109", err)
}

func TestRunOperationNotSupported(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	_, phase, err := bm.RunOperation(context.Background(), &core.PreparedOperation{})

	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.Regexp(t, "FF10378", err)
}

func TestRunOperationBatchBroadcastInvalidData(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	op := &core.Operation{}
	batch := &core.Batch{
		Payload: core.BatchPayload{
			Data: core.DataArray{
				{Value: fftypes.JSONAnyPtr(`!json`)},
			},
		},
	}

	_, phase, err := bm.RunOperation(context.Background(), opUploadBatch(op, batch))

	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.Regexp(t, "FF10137", err)
}

func TestRunOperationBatchBroadcastPublishFail(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	op := &core.Operation{}
	batch := &core.Batch{
		BatchHeader: core.BatchHeader{
			ID: fftypes.NewUUID(),
		},
	}

	mps := bm.sharedstorage.(*sharedstoragemocks.Plugin)
	mps.On("UploadData", context.Background(), mock.Anything).Return("", fmt.Errorf("pop"))

	_, phase, err := bm.RunOperation(context.Background(), opUploadBatch(op, batch))

	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.EqualError(t, err, "pop")

	mps.AssertExpectations(t)
}

func TestRunOperationBatchBroadcast(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	op := &core.Operation{}
	batch := &core.Batch{
		BatchHeader: core.BatchHeader{
			ID: fftypes.NewUUID(),
		},
	}

	mps := bm.sharedstorage.(*sharedstoragemocks.Plugin)
	mdi := bm.database.(*databasemocks.Plugin)
	mps.On("UploadData", context.Background(), mock.Anything).Return("123", nil)

	outputs, phase, err := bm.RunOperation(context.Background(), opUploadBatch(op, batch))
	assert.Equal(t, "123", outputs["payloadRef"])

	assert.Equal(t, core.OpPhaseComplete, phase)
	assert.NoError(t, err)

	mps.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestPrepareAndRunUploadBlob(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	op := &core.Operation{
		Type: core.OpTypeSharedStorageUploadBlob,
	}
	blob := &core.Blob{
		Hash: fftypes.NewRandB32(),
	}
	data := &core.Data{
		Namespace: "ns1",
		ID:        fftypes.NewUUID(),
		Blob: &core.BlobRef{
			Hash: blob.Hash,
		},
	}
	addUploadBlobInputs(op, data.ID)

	mps := bm.sharedstorage.(*sharedstoragemocks.Plugin)
	mdx := bm.exchange.(*dataexchangemocks.Plugin)
	mdi := bm.database.(*databasemocks.Plugin)

	reader := ioutil.NopCloser(strings.NewReader("some data"))
	mdi.On("GetDataByID", mock.Anything, "ns1", data.ID, false).Return(data, nil)
	mdi.On("GetBlobs", mock.Anything, bm.namespace.Name, mock.Anything).Return([]*core.Blob{blob}, nil, nil)
	mps.On("UploadData", context.Background(), mock.Anything).Return("123", nil)
	mdx.On("DownloadBlob", context.Background(), mock.Anything).Return(reader, nil)
	mdi.On("UpdateData", context.Background(), "ns1", data.ID, mock.MatchedBy(func(update ffapi.Update) bool {
		info, _ := update.Finalize()
		assert.Equal(t, 1, len(info.SetOperations))
		assert.Equal(t, "blob.public", info.SetOperations[0].Field)
		val, _ := info.SetOperations[0].Value.Value()
		assert.Equal(t, "123", val)
		return true
	})).Return(nil)

	po, err := bm.PrepareOperation(context.Background(), op)
	assert.NoError(t, err)
	assert.Equal(t, data, po.Data.(uploadBlobData).Data)
	assert.Equal(t, blob, po.Data.(uploadBlobData).Blob)

	outputs, phase, err := bm.RunOperation(context.Background(), opUploadBlob(op, data, blob))
	assert.Equal(t, "123", outputs["payloadRef"])

	assert.Equal(t, core.OpPhaseComplete, phase)
	assert.NoError(t, err)

	mps.AssertExpectations(t)
	mdx.AssertExpectations(t)
	mdi.AssertExpectations(t)

}

func TestPrepareAndRunValue(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	op := &core.Operation{
		Type: core.OpTypeSharedStorageUploadValue,
	}
	data := &core.Data{
		ID:    fftypes.NewUUID(),
		Value: fftypes.JSONAnyPtr(`{"some":"data"}`),
	}
	addUploadValueInputs(op, data.ID)

	mps := bm.sharedstorage.(*sharedstoragemocks.Plugin)
	mdi := bm.database.(*databasemocks.Plugin)

	mdi.On("GetDataByID", mock.Anything, "ns1", data.ID, false).Return(data, nil)
	mps.On("UploadData", context.Background(), mock.Anything).Return("123", nil)
	mdi.On("UpdateData", context.Background(), "ns1", data.ID, mock.MatchedBy(func(update ffapi.Update) bool {
		info, _ := update.Finalize()
		assert.Equal(t, 1, len(info.SetOperations))
		assert.Equal(t, "public", info.SetOperations[0].Field)
		val, _ := info.SetOperations[0].Value.Value()
		assert.Equal(t, "123", val)
		return true
	})).Return(nil)

	po, err := bm.PrepareOperation(context.Background(), op)
	assert.NoError(t, err)
	assert.Equal(t, data, po.Data.(uploadValue).Data)

	outputs, phase, err := bm.RunOperation(context.Background(), opUploadValue(op, data))
	assert.Equal(t, "123", outputs["payloadRef"])

	assert.Equal(t, core.OpPhaseComplete, phase)
	assert.NoError(t, err)

	mps.AssertExpectations(t)
	mdi.AssertExpectations(t)

}

func TestPrepareUploadBlobGetBlobMissing(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	op := &core.Operation{
		Type: core.OpTypeSharedStorageUploadBlob,
	}
	blob := &core.Blob{
		Hash: fftypes.NewRandB32(),
	}
	data := &core.Data{
		Namespace: "ns1",
		ID:        fftypes.NewUUID(),
		Blob: &core.BlobRef{
			Hash: blob.Hash,
		},
	}
	addUploadBlobInputs(op, data.ID)

	mps := bm.sharedstorage.(*sharedstoragemocks.Plugin)
	mdx := bm.exchange.(*dataexchangemocks.Plugin)
	mdi := bm.database.(*databasemocks.Plugin)

	mdi.On("GetDataByID", mock.Anything, "ns1", data.ID, false).Return(data, nil)
	mdi.On("GetBlobs", mock.Anything, bm.namespace.Name, mock.Anything).Return([]*core.Blob{}, nil, nil)

	_, err := bm.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF10109", err)

	mps.AssertExpectations(t)
	mdx.AssertExpectations(t)
	mdi.AssertExpectations(t)

}

func TestPrepareUploadBlobGetBlobFailing(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	op := &core.Operation{
		Type: core.OpTypeSharedStorageUploadBlob,
	}
	blob := &core.Blob{
		Hash: fftypes.NewRandB32(),
	}
	data := &core.Data{
		Namespace: "ns1",
		ID:        fftypes.NewUUID(),
		Blob: &core.BlobRef{
			Hash: blob.Hash,
		},
	}
	addUploadBlobInputs(op, data.ID)

	mdi := bm.database.(*databasemocks.Plugin)

	mdi.On("GetDataByID", mock.Anything, "ns1", data.ID, false).Return(data, nil)
	mdi.On("GetBlobs", mock.Anything, bm.namespace.Name, mock.Anything).Return([]*core.Blob{}, nil, fmt.Errorf("pop"))

	_, err := bm.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)

}

func TestPrepareUploadBlobGetDataMissing(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	op := &core.Operation{
		Type: core.OpTypeSharedStorageUploadBlob,
	}
	dataID := fftypes.NewUUID()
	addUploadBlobInputs(op, dataID)

	mdi := bm.database.(*databasemocks.Plugin)

	mdi.On("GetDataByID", mock.Anything, "ns1", dataID, false).Return(nil, nil)

	_, err := bm.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF10109", err)

	mdi.AssertExpectations(t)

}

func TestPrepareUploadValueGetDataMissing(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	op := &core.Operation{
		Type: core.OpTypeSharedStorageUploadValue,
	}
	dataID := fftypes.NewUUID()
	addUploadBlobInputs(op, dataID)

	mdi := bm.database.(*databasemocks.Plugin)

	mdi.On("GetDataByID", mock.Anything, "ns1", dataID, false).Return(nil, nil)

	_, err := bm.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF10109", err)

	mdi.AssertExpectations(t)

}

func TestPrepareUploadBlobGetDataFail(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	op := &core.Operation{
		Type: core.OpTypeSharedStorageUploadBlob,
	}
	dataID := fftypes.NewUUID()
	addUploadBlobInputs(op, dataID)

	mdi := bm.database.(*databasemocks.Plugin)

	mdi.On("GetDataByID", mock.Anything, "ns1", dataID, false).Return(nil, fmt.Errorf("pop"))

	_, err := bm.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)

}

func TestPrepareUploadValueGetDataFail(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	op := &core.Operation{
		Type: core.OpTypeSharedStorageUploadValue,
	}
	dataID := fftypes.NewUUID()
	addUploadBlobInputs(op, dataID)

	mdi := bm.database.(*databasemocks.Plugin)

	mdi.On("GetDataByID", mock.Anything, "ns1", dataID, false).Return(nil, fmt.Errorf("pop"))

	_, err := bm.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)

}

func TestPrepareUploadBlobGetDataBadID(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	op := &core.Operation{
		Type: core.OpTypeSharedStorageUploadBlob,
	}

	_, err := bm.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF00138", err)

}

func TestPrepareUploadValueGetDataBadID(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	op := &core.Operation{
		Type: core.OpTypeSharedStorageUploadValue,
	}

	_, err := bm.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF00138", err)

}

func TestRunOperationUploadBlobUpdateFail(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	op := &core.Operation{}
	blob := &core.Blob{
		Hash: fftypes.NewRandB32(),
	}
	data := &core.Data{
		ID: fftypes.NewUUID(),
		Blob: &core.BlobRef{
			Hash: blob.Hash,
		},
	}

	mps := bm.sharedstorage.(*sharedstoragemocks.Plugin)
	mdx := bm.exchange.(*dataexchangemocks.Plugin)
	mdi := bm.database.(*databasemocks.Plugin)

	reader := ioutil.NopCloser(strings.NewReader("some data"))
	mdx.On("DownloadBlob", context.Background(), mock.Anything).Return(reader, nil)
	mps.On("UploadData", context.Background(), mock.Anything).Return("123", nil)
	mdi.On("UpdateData", context.Background(), "ns1", data.ID, mock.Anything).Return(fmt.Errorf("pop"))

	_, phase, err := bm.RunOperation(context.Background(), opUploadBlob(op, data, blob))

	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.Regexp(t, "pop", err)

	mps.AssertExpectations(t)
	mdx.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestRunOperationUploadValueUpdateFail(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	op := &core.Operation{}
	data := &core.Data{
		ID:    fftypes.NewUUID(),
		Value: fftypes.JSONAnyPtr(`{"some":"data"}`),
	}

	mps := bm.sharedstorage.(*sharedstoragemocks.Plugin)
	mdi := bm.database.(*databasemocks.Plugin)

	mps.On("UploadData", context.Background(), mock.Anything).Return("123", nil)
	mdi.On("UpdateData", context.Background(), "ns1", data.ID, mock.Anything).Return(fmt.Errorf("pop"))

	_, phase, err := bm.RunOperation(context.Background(), opUploadValue(op, data))

	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.Regexp(t, "pop", err)

	mps.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestRunOperationUploadBlobUploadFail(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	op := &core.Operation{}
	blob := &core.Blob{
		Hash: fftypes.NewRandB32(),
	}
	data := &core.Data{
		ID: fftypes.NewUUID(),
		Blob: &core.BlobRef{
			Hash: blob.Hash,
		},
	}

	mps := bm.sharedstorage.(*sharedstoragemocks.Plugin)
	mdx := bm.exchange.(*dataexchangemocks.Plugin)

	reader := ioutil.NopCloser(strings.NewReader("some data"))
	mdx.On("DownloadBlob", context.Background(), mock.Anything).Return(reader, nil)
	mps.On("UploadData", context.Background(), mock.Anything).Return("", fmt.Errorf("pop"))

	_, phase, err := bm.RunOperation(context.Background(), opUploadBlob(op, data, blob))

	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.Regexp(t, "pop", err)

	mps.AssertExpectations(t)
	mdx.AssertExpectations(t)
}

func TestRunOperationUploadValueUploadFail(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	op := &core.Operation{}
	data := &core.Data{
		ID:    fftypes.NewUUID(),
		Value: fftypes.JSONAnyPtr(`{"some":"data"}`),
	}

	mps := bm.sharedstorage.(*sharedstoragemocks.Plugin)
	mps.On("UploadData", context.Background(), mock.Anything).Return("", fmt.Errorf("pop"))

	_, phase, err := bm.RunOperation(context.Background(), opUploadValue(op, data))

	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.Regexp(t, "pop", err)

	mps.AssertExpectations(t)
}

func TestRunOperationUploadBlobDownloadFail(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	op := &core.Operation{}
	blob := &core.Blob{
		Hash: fftypes.NewRandB32(),
	}
	data := &core.Data{
		ID: fftypes.NewUUID(),
		Blob: &core.BlobRef{
			Hash: blob.Hash,
		},
	}

	mps := bm.sharedstorage.(*sharedstoragemocks.Plugin)
	mdx := bm.exchange.(*dataexchangemocks.Plugin)

	reader := ioutil.NopCloser(strings.NewReader("some data"))
	mdx.On("DownloadBlob", context.Background(), mock.Anything).Return(reader, fmt.Errorf("pop"))

	_, phase, err := bm.RunOperation(context.Background(), opUploadBlob(op, data, blob))

	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.Regexp(t, "pop", err)

	mps.AssertExpectations(t)
	mdx.AssertExpectations(t)
}

func TestOperationUpdate(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()
	assert.NoError(t, bm.OnOperationUpdate(context.Background(), nil, nil))
}
