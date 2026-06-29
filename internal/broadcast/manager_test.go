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

package broadcast

import (
	"context"
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/firefly/internal/batch"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/internal/data"
	"github.com/hyperledger-firefly/firefly/internal/database/sqlcommon"
	"github.com/hyperledger-firefly/firefly/mocks/batchmocks"
	"github.com/hyperledger-firefly/firefly/mocks/blockchainmocks"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/mocks/dataexchangemocks"
	"github.com/hyperledger-firefly/firefly/mocks/datamocks"
	"github.com/hyperledger-firefly/firefly/mocks/identitymanagermocks"
	"github.com/hyperledger-firefly/firefly/mocks/metricsmocks"
	"github.com/hyperledger-firefly/firefly/mocks/multipartymocks"
	"github.com/hyperledger-firefly/firefly/mocks/operationmocks"
	"github.com/hyperledger-firefly/firefly/mocks/sharedstoragemocks"
	"github.com/hyperledger-firefly/firefly/mocks/syncasyncmocks"
	"github.com/hyperledger-firefly/firefly/mocks/txcommonmocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func newTestBroadcastCommon(t *testing.T, metricsEnabled bool) (*broadcastManager, func()) {
	coreconfig.Reset()
	mdi := &databasemocks.Plugin{}
	mim := &identitymanagermocks.Manager{}
	mdm := &datamocks.Manager{}
	mbi := &blockchainmocks.Plugin{}
	mpi := &sharedstoragemocks.Plugin{}
	mba := &batchmocks.Manager{}
	mdx := &dataexchangemocks.Plugin{}
	msa := &syncasyncmocks.Bridge{}
	mmp := &multipartymocks.Manager{}
	mmi := &metricsmocks.Manager{}
	mom := &operationmocks.Manager{}
	mtx := &txcommonmocks.Helper{}
	mmi.On("IsMetricsEnabled").Return(metricsEnabled)
	mbi.On("Name").Return("ut_blockchain").Maybe()
	mpi.On("Name").Return("ut_sharedstorage").Maybe()

	mba.On("RegisterDispatcher",
		broadcastDispatcherName,
		true,
		[]core.MessageType{
			core.MessageTypeBroadcast,
			core.MessageTypeDefinition,
			core.MessageTypeDeprecatedTransferBroadcast,
			core.MessageTypeDeprecatedApprovalBroadcast,
		}, mock.Anything, mock.Anything).Return()

	mom.On("RegisterHandler", mock.Anything, mock.Anything, mock.Anything)

	rag := mdi.On("RunAsGroup", mock.Anything, mock.Anything).Maybe()
	rag.RunFn = func(a mock.Arguments) {
		rag.ReturnArguments = mock.Arguments{
			a[1].(func(context.Context) error)(a[0].(context.Context)),
		}
	}

	ctx, cancel := context.WithCancel(context.Background())
	ns := &core.Namespace{Name: "ns1", NetworkName: "ns1"}
	b, err := NewBroadcastManager(ctx, ns, mdi, mbi, mdx, mpi, mim, mdm, mba, msa, mmp, mmi, mom, mtx)
	assert.NoError(t, err)
	return b.(*broadcastManager), cancel
}

func newTestBroadcast(t *testing.T) (*broadcastManager, func()) {
	return newTestBroadcastCommon(t, false)
}

func newTestBroadcastWithMetrics(t *testing.T) (*broadcastManager, func()) {
	bm, cancel := newTestBroadcastCommon(t, true)
	mmi := bm.metrics.(*metricsmocks.Manager)
	mmi.On("MessageSubmitted", mock.Anything).Return()
	return bm, cancel
}

func TestInitFail(t *testing.T) {
	_, err := NewBroadcastManager(context.Background(), &core.Namespace{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	assert.Regexp(t, "FF10128", err)
}

func TestName(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()
	assert.Equal(t, "BroadcastManager", bm.Name())
}

func TestBroadcastMessageGood(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	dataID := fftypes.NewUUID()
	dataHash := fftypes.NewRandB32()
	newMsg := &data.NewMessage{
		Message: &core.MessageInOut{
			Message: core.Message{
				Header: core.MessageHeader{
					ID: fftypes.NewUUID(),
				},
				Data: core.DataRefs{
					{ID: dataID, Hash: dataHash},
				},
			},
		},
		AllData: core.DataArray{
			{ID: dataID, Hash: dataHash},
		},
	}

	mdm := bm.data.(*datamocks.Manager)
	mdm.On("WriteNewMessage", mock.Anything, newMsg).Return(nil)

	broadcast := broadcastSender{
		mgr: bm,
		msg: newMsg,
	}
	err := broadcast.sendInternal(context.Background(), methodSend)
	assert.NoError(t, err)

	bm.Start()
	bm.WaitStop()

	mdm.AssertExpectations(t)
}

func TestBroadcastMessageBad(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	newMsg := &data.NewMessage{
		Message: &core.MessageInOut{
			Message: core.Message{
				Header: core.MessageHeader{
					ID: fftypes.NewUUID(),
				},
				Data: core.DataRefs{
					{ID: fftypes.NewUUID(), Hash: nil},
				},
			},
		},
	}

	broadcast := broadcastSender{
		mgr: bm,
		msg: newMsg,
	}
	err := broadcast.sendInternal(context.Background(), methodSend)
	assert.Regexp(t, "FF00128", err)

}

func TestDispatchBatchBlobsFail(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	blobHash := fftypes.NewRandB32()
	state := &batch.DispatchPayload{
		Data: []*core.Data{
			{ID: fftypes.NewUUID(), Blob: &core.BlobRef{
				Hash: blobHash,
			}},
		},
		Pins: []*fftypes.Bytes32{fftypes.NewRandB32()},
	}

	mom := bm.operations.(*operationmocks.Manager)
	mom.On("AddOrReuseOperation", mock.Anything, mock.Anything).Return(nil)

	mdi := bm.database.(*databasemocks.Plugin)
	mdi.On("GetBlobs", mock.Anything, bm.namespace.Name, mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	err := bm.dispatchBatch(bm.ctx, state)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestDispatchBatchInsertOpFail(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	state := &batch.DispatchPayload{
		Pins: []*fftypes.Bytes32{fftypes.NewRandB32()},
	}

	mom := bm.operations.(*operationmocks.Manager)
	mom.On("AddOrReuseOperation", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))

	err := bm.dispatchBatch(context.Background(), state)
	assert.EqualError(t, err, "pop")

	mom.AssertExpectations(t)
}

func TestDispatchBatchUploadFail(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	state := &batch.DispatchPayload{
		Batch: core.BatchPersisted{
			BatchHeader: core.BatchHeader{
				ID: fftypes.NewUUID(),
			},
		},
		Pins: []*fftypes.Bytes32{fftypes.NewRandB32()},
	}

	mom := bm.operations.(*operationmocks.Manager)
	mom.On("AddOrReuseOperation", mock.Anything, mock.Anything).Return(nil)
	mom.On("RunOperation", mock.Anything, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(uploadBatchData)
		return op.Type == core.OpTypeSharedStorageUploadBatch && data.Batch.ID.Equals(state.Batch.ID)
	}), false).Return(nil, fmt.Errorf("pop"))

	err := bm.dispatchBatch(context.Background(), state)
	assert.EqualError(t, err, "pop")

	mom.AssertExpectations(t)
}

func TestDispatchBatchSubmitBatchPinSucceed(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	state := &batch.DispatchPayload{
		Batch: core.BatchPersisted{
			BatchHeader: core.BatchHeader{
				ID: fftypes.NewUUID(),
			},
		},
		Pins: []*fftypes.Bytes32{fftypes.NewRandB32()},
	}

	mdi := bm.database.(*databasemocks.Plugin)
	mmp := bm.multiparty.(*multipartymocks.Manager)
	mom := bm.operations.(*operationmocks.Manager)
	mom.On("AddOrReuseOperation", mock.Anything, mock.Anything).Return(nil)
	mmp.On("SubmitBatchPin", mock.Anything, mock.Anything, mock.Anything, "payload1", false).Return(nil)
	mom.On("RunOperation", mock.Anything, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(uploadBatchData)
		return op.Type == core.OpTypeSharedStorageUploadBatch && data.Batch.ID.Equals(state.Batch.ID)
	}), false).Return(getUploadBatchOutputs("payload1"), nil)

	err := bm.dispatchBatch(context.Background(), state)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
	mmp.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestDispatchBatchSubmitBroadcastFail(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	state := &batch.DispatchPayload{
		Batch: core.BatchPersisted{
			BatchHeader: core.BatchHeader{
				ID:        fftypes.NewUUID(),
				SignerRef: core.SignerRef{Author: "wrong", Key: "wrong"},
			},
		},
		Pins: []*fftypes.Bytes32{fftypes.NewRandB32()},
	}

	mdi := bm.database.(*databasemocks.Plugin)
	mmp := bm.multiparty.(*multipartymocks.Manager)
	mom := bm.operations.(*operationmocks.Manager)
	mom.On("AddOrReuseOperation", mock.Anything, mock.Anything).Return(nil)
	mmp.On("SubmitBatchPin", mock.Anything, mock.Anything, mock.Anything, "payload1", false).Return(fmt.Errorf("pop"))
	mom.On("RunOperation", mock.Anything, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(uploadBatchData)
		return op.Type == core.OpTypeSharedStorageUploadBatch && data.Batch.ID.Equals(state.Batch.ID)
	}), false).Return(getUploadBatchOutputs("payload1"), nil)

	err := bm.dispatchBatch(context.Background(), state)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
	mmp.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestUploadBlobPublishFail(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()
	mdx := bm.exchange.(*dataexchangemocks.Plugin)
	mps := bm.sharedstorage.(*sharedstoragemocks.Plugin)
	mdi := bm.database.(*databasemocks.Plugin)
	mom := bm.operations.(*operationmocks.Manager)
	mtx := bm.txHelper.(*txcommonmocks.Helper)

	blob := &core.Blob{
		Hash:       fftypes.NewRandB32(),
		PayloadRef: "blob/1",
	}
	d := &core.Data{
		ID: fftypes.NewUUID(),
		Blob: &core.BlobRef{
			Hash: blob.Hash,
		},
	}

	ctx := context.Background()
	mtx.On("SubmitNewTransaction", mock.Anything, core.TransactionTypeDataPublish, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mdi.On("GetDataByID", ctx, "ns1", d.ID, true).Return(d, nil)
	mdi.On("GetBlobs", ctx, bm.namespace.Name, mock.Anything).Return([]*core.Blob{blob}, nil, nil)
	mom.On("AddOrReuseOperation", mock.Anything, mock.Anything).Return(nil)
	mom.On("RunOperation", mock.Anything, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(uploadBlobData)
		return op.Type == core.OpTypeSharedStorageUploadBlob && data.Blob == blob
	}), true).Return(nil, fmt.Errorf("pop"))

	_, err := bm.PublishDataBlob(ctx, d.ID.String(), "idem1")
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
	mdx.AssertExpectations(t)
	mps.AssertExpectations(t)

}

func TestUploadBlobPublishIdempotentResubmitOperation(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	var id = fftypes.NewUUID()
	defer cancel()
	mdi := bm.database.(*databasemocks.Plugin)
	mom := bm.operations.(*operationmocks.Manager)
	mtx := bm.txHelper.(*txcommonmocks.Helper)

	blob := &core.Blob{
		Hash:       fftypes.NewRandB32(),
		PayloadRef: "blob/1",
	}
	d := &core.Data{
		ID: fftypes.NewUUID(),
		Blob: &core.BlobRef{
			Hash: blob.Hash,
		},
	}
	op := &core.Operation{}

	ctx := context.Background()
	mtx.On("SubmitNewTransaction", mock.Anything, core.TransactionTypeDataPublish, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), &sqlcommon.IdempotencyError{
		ExistingTXID:  id,
		OriginalError: i18n.NewError(context.Background(), coremsgs.MsgIdempotencyKeyDuplicateTransaction, "idem1", id)})
	mom.On("ResubmitOperations", context.Background(), id).Return(1, []*core.Operation{op}, nil)
	mdi.On("GetDataByID", ctx, "ns1", d.ID, true).Return(d, nil)

	// If ResubmitOperations returns an operation it's because it found one to resubmit, we return 2xx not 409 and hence don't expect any errors here
	d, err := bm.PublishDataBlob(ctx, d.ID.String(), "idem1")
	assert.NotNil(t, d)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestUploadBlobPublishIdempotentResubmitAll(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	var id = fftypes.NewUUID()
	defer cancel()
	mdi := bm.database.(*databasemocks.Plugin)
	mom := bm.operations.(*operationmocks.Manager)
	mtx := bm.txHelper.(*txcommonmocks.Helper)

	blob := &core.Blob{
		Hash:       fftypes.NewRandB32(),
		PayloadRef: "blob/1",
	}
	d := &core.Data{
		ID: fftypes.NewUUID(),
		Blob: &core.BlobRef{
			Hash: blob.Hash,
		},
	}

	ctx := context.Background()
	mtx.On("SubmitNewTransaction", mock.Anything, core.TransactionTypeDataPublish, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), &sqlcommon.IdempotencyError{
		ExistingTXID:  id,
		OriginalError: i18n.NewError(context.Background(), coremsgs.MsgIdempotencyKeyDuplicateTransaction, "idem1", id)})
	mom.On("ResubmitOperations", context.Background(), id).Return(0, nil, nil)
	mdi.On("GetDataByID", ctx, "ns1", d.ID, true).Return(d, nil)
	mom.On("AddOrReuseOperation", mock.Anything, mock.Anything).Return(nil)
	mdi.On("GetBlobs", ctx, bm.namespace.Name, mock.Anything).Return([]*core.Blob{blob}, nil, nil)
	mom.On("RunOperation", mock.Anything, mock.Anything, true).Return(nil, nil)

	// If ResubmitOperations returns an operation it's because it found one to resubmit, we return 2xx not 409 and hence don't expect any errors here
	d, err := bm.PublishDataBlob(ctx, d.ID.String(), "idem1")
	assert.NotNil(t, d)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestUploadBlobPublishIdempotentNoOperationToResubmit(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	var id = fftypes.NewUUID()
	defer cancel()
	mdi := bm.database.(*databasemocks.Plugin)
	mom := bm.operations.(*operationmocks.Manager)
	mtx := bm.txHelper.(*txcommonmocks.Helper)

	blob := &core.Blob{
		Hash:       fftypes.NewRandB32(),
		PayloadRef: "blob/1",
	}
	d := &core.Data{
		ID: fftypes.NewUUID(),
		Blob: &core.BlobRef{
			Hash: blob.Hash,
		},
	}

	ctx := context.Background()
	mtx.On("SubmitNewTransaction", mock.Anything, core.TransactionTypeDataPublish, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), &sqlcommon.IdempotencyError{
		ExistingTXID:  id,
		OriginalError: i18n.NewError(context.Background(), coremsgs.MsgIdempotencyKeyDuplicateTransaction, "idem1", id)})
	mom.On("ResubmitOperations", context.Background(), id).Return(1 /* total */, nil /* to resubmit */, nil)
	mdi.On("GetDataByID", ctx, "ns1", d.ID, true).Return(d, nil)

	// If ResubmitOperations returns nil it's because there was no operation in initialized state, so we expect the regular 409 error back
	d, err := bm.PublishDataBlob(ctx, d.ID.String(), "idem1")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "FF10431")

	mdi.AssertExpectations(t)
}

func TestUploadBlobPublishIdempotentErrorOnOperationResubmit(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	var id = fftypes.NewUUID()
	defer cancel()
	mdi := bm.database.(*databasemocks.Plugin)
	mom := bm.operations.(*operationmocks.Manager)
	mtx := bm.txHelper.(*txcommonmocks.Helper)

	blob := &core.Blob{
		Hash:       fftypes.NewRandB32(),
		PayloadRef: "blob/1",
	}
	d := &core.Data{
		ID: fftypes.NewUUID(),
		Blob: &core.BlobRef{
			Hash: blob.Hash,
		},
	}

	ctx := context.Background()
	mtx.On("SubmitNewTransaction", mock.Anything, core.TransactionTypeDataPublish, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), &sqlcommon.IdempotencyError{
		ExistingTXID:  id,
		OriginalError: i18n.NewError(context.Background(), coremsgs.MsgIdempotencyKeyDuplicateTransaction, "idem1", id)})
	mom.On("ResubmitOperations", context.Background(), id).Return(-1, nil, fmt.Errorf("pop"))
	mdi.On("GetDataByID", ctx, "ns1", d.ID, true).Return(d, nil)

	// If ResubmitOperations returned an error trying to resubmit an operation we expect that error back, not the 409 conflict error
	d, err := bm.PublishDataBlob(ctx, d.ID.String(), "idem1")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestUploadBlobsGetBlobFail(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()
	mdi := bm.database.(*databasemocks.Plugin)

	blob := &core.Blob{
		Hash:       fftypes.NewRandB32(),
		PayloadRef: "blob/1",
	}
	dataID := fftypes.NewUUID()

	ctx := context.Background()
	mdi.On("GetBlobs", ctx, bm.namespace.Name, mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	mom := bm.operations.(*operationmocks.Manager)
	mom.On("AddOrReuseOperation", mock.Anything, mock.Anything).Return(nil)

	err := bm.uploadBlobs(ctx, fftypes.NewUUID(), core.DataArray{
		{
			ID: dataID,
			Blob: &core.BlobRef{
				Hash: blob.Hash,
			},
		},
	}, false)
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
	mom.AssertExpectations(t)

}

func TestUploadBlobsGetBlobNotFound(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()
	mdi := bm.database.(*databasemocks.Plugin)

	blob := &core.Blob{
		Hash:       fftypes.NewRandB32(),
		PayloadRef: "blob/1",
	}
	dataID := fftypes.NewUUID()

	ctx := context.Background()
	mdi.On("GetBlobs", ctx, bm.namespace.Name, mock.Anything).Return([]*core.Blob{}, nil, nil)

	mom := bm.operations.(*operationmocks.Manager)
	mom.On("AddOrReuseOperation", mock.Anything, mock.Anything).Return(nil)

	err := bm.uploadBlobs(ctx, fftypes.NewUUID(), core.DataArray{
		{
			ID: dataID,
			Blob: &core.BlobRef{
				Hash: blob.Hash,
			},
		},
	}, false)
	assert.Regexp(t, "FF10239", err)

	mdi.AssertExpectations(t)
	mom.AssertExpectations(t)

}

func TestUploadBlobsGetBlobInsertOpFail(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	blob := &core.Blob{
		Hash:       fftypes.NewRandB32(),
		PayloadRef: "blob/1",
	}
	dataID := fftypes.NewUUID()
	ctx := context.Background()

	mom := bm.operations.(*operationmocks.Manager)
	mom.On("AddOrReuseOperation", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))

	err := bm.uploadBlobs(ctx, fftypes.NewUUID(), core.DataArray{
		{
			ID: dataID,
			Blob: &core.BlobRef{
				Hash: blob.Hash,
			},
		},
	}, true)
	assert.EqualError(t, err, "pop")

	mom.AssertExpectations(t)

}

func TestUploadValueNotFound(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	mdi := bm.database.(*databasemocks.Plugin)
	mdi.On("GetDataByID", mock.Anything, "ns1", mock.Anything, true).Return(nil, nil)

	_, err := bm.PublishDataValue(bm.ctx, fftypes.NewUUID().String(), "")
	assert.Regexp(t, "FF10109", err)

	mdi.AssertExpectations(t)
}

func TestUploadBlobLookupError(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	mdi := bm.database.(*databasemocks.Plugin)
	mdi.On("GetDataByID", mock.Anything, "ns1", mock.Anything, true).Return(nil, fmt.Errorf("pop"))

	_, err := bm.PublishDataBlob(bm.ctx, fftypes.NewUUID().String(), "")
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
}

func TestUploadValueBadID(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	_, err := bm.PublishDataValue(bm.ctx, "badness", "")
	assert.Regexp(t, "FF00138", err)

}

func TestUploadValueFailPrepare(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	d := &core.Data{
		ID:    fftypes.NewUUID(),
		Value: fftypes.JSONAnyPtr(`{"some": "value"}`),
	}

	mdi := bm.database.(*databasemocks.Plugin)
	mdi.On("GetDataByID", mock.Anything, "ns1", d.ID, true).Return(d, nil)

	mom := bm.operations.(*operationmocks.Manager)
	mom.On("AddOrReuseOperation", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))

	mtx := bm.txHelper.(*txcommonmocks.Helper)
	mtx.On("SubmitNewTransaction", mock.Anything, core.TransactionTypeDataPublish, core.IdempotencyKey("")).Return(fftypes.NewUUID(), nil)

	_, err := bm.PublishDataValue(bm.ctx, d.ID.String(), "")
	assert.EqualError(t, err, "pop")

	mom.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestUploadValueFail(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	d := &core.Data{
		ID:    fftypes.NewUUID(),
		Value: fftypes.JSONAnyPtr(`{"some": "value"}`),
	}

	mdi := bm.database.(*databasemocks.Plugin)
	mdi.On("GetDataByID", mock.Anything, "ns1", d.ID, true).Return(d, nil)

	mom := bm.operations.(*operationmocks.Manager)
	mom.On("AddOrReuseOperation", mock.Anything, mock.Anything).Return(nil)
	mom.On("RunOperation", mock.Anything, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(uploadValue)
		return op.Type == core.OpTypeSharedStorageUploadValue && data.Data.ID.Equals(d.ID)
	}), false).Return(nil, fmt.Errorf("pop"))

	mtx := bm.txHelper.(*txcommonmocks.Helper)
	mtx.On("SubmitNewTransaction", mock.Anything, core.TransactionTypeDataPublish, core.IdempotencyKey("")).Return(fftypes.NewUUID(), nil)

	_, err := bm.PublishDataValue(bm.ctx, d.ID.String(), "")
	assert.EqualError(t, err, "pop")

	mom.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestUploadValueOK(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	d := &core.Data{
		ID:    fftypes.NewUUID(),
		Value: fftypes.JSONAnyPtr(`{"some": "value"}`),
	}

	mdi := bm.database.(*databasemocks.Plugin)
	mdi.On("GetDataByID", mock.Anything, "ns1", d.ID, true).Return(d, nil)

	mom := bm.operations.(*operationmocks.Manager)
	mom.On("AddOrReuseOperation", mock.Anything, mock.Anything).Return(nil)
	mom.On("RunOperation", mock.Anything, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(uploadValue)
		return op.Type == core.OpTypeSharedStorageUploadValue && data.Data.ID.Equals(d.ID)
	}), false).Return(nil, nil)

	mtx := bm.txHelper.(*txcommonmocks.Helper)
	mtx.On("SubmitNewTransaction", mock.Anything, core.TransactionTypeDataPublish, core.IdempotencyKey("")).Return(fftypes.NewUUID(), nil)

	d1, err := bm.PublishDataValue(bm.ctx, d.ID.String(), "")
	assert.NoError(t, err)
	assert.Equal(t, d.ID, d1.ID)

	mom.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestUploadValueIdempotentResubmitOperation(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	var id = fftypes.NewUUID()
	defer cancel()

	d := &core.Data{
		ID:    fftypes.NewUUID(),
		Value: fftypes.JSONAnyPtr(`{"some": "value"}`),
	}

	op := &core.Operation{}

	mdi := bm.database.(*databasemocks.Plugin)
	mdi.On("GetDataByID", mock.Anything, "ns1", d.ID, true).Return(d, nil)

	mom := bm.operations.(*operationmocks.Manager)

	mtx := bm.txHelper.(*txcommonmocks.Helper)
	mtx.On("SubmitNewTransaction", context.Background(), core.TransactionTypeDataPublish, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), &sqlcommon.IdempotencyError{
		ExistingTXID:  id,
		OriginalError: i18n.NewError(context.Background(), coremsgs.MsgIdempotencyKeyDuplicateTransaction, "idem1", id)})
	mom.On("ResubmitOperations", context.Background(), id).Return(1, []*core.Operation{op}, nil)

	// If ResubmitOperations returns an operation it's because it found one to resubmit, we return 2xx not 409 and hence don't expect any errors here
	d1, err := bm.PublishDataValue(context.Background(), d.ID.String(), "idem1")
	assert.NoError(t, err)
	assert.Equal(t, d.ID, d1.ID)

	mom.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestUploadValueIdempotentResubmitAll(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	var id = fftypes.NewUUID()
	defer cancel()

	d := &core.Data{
		ID:    fftypes.NewUUID(),
		Value: fftypes.JSONAnyPtr(`{"some": "value"}`),
	}

	mdi := bm.database.(*databasemocks.Plugin)
	mdi.On("GetDataByID", mock.Anything, "ns1", d.ID, true).Return(d, nil)

	mom := bm.operations.(*operationmocks.Manager)

	mtx := bm.txHelper.(*txcommonmocks.Helper)
	mtx.On("SubmitNewTransaction", context.Background(), core.TransactionTypeDataPublish, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), &sqlcommon.IdempotencyError{
		ExistingTXID:  id,
		OriginalError: i18n.NewError(context.Background(), coremsgs.MsgIdempotencyKeyDuplicateTransaction, "idem1", id)})
	mom.On("ResubmitOperations", context.Background(), id).Return(0, nil, nil)
	mom.On("AddOrReuseOperation", mock.Anything, mock.Anything).Return(nil)
	mom.On("RunOperation", mock.Anything, mock.Anything, true).Return(nil, nil)

	// If ResubmitOperations returns an operation it's because it found one to resubmit, we return 2xx not 409 and hence don't expect any errors here
	d1, err := bm.PublishDataValue(context.Background(), d.ID.String(), "idem1")
	assert.NoError(t, err)
	assert.Equal(t, d.ID, d1.ID)

	mom.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestUploadValueIdempotentNoOperationToResubmit(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	var id = fftypes.NewUUID()
	defer cancel()

	d := &core.Data{
		ID:    fftypes.NewUUID(),
		Value: fftypes.JSONAnyPtr(`{"some": "value"}`),
	}

	mdi := bm.database.(*databasemocks.Plugin)
	mdi.On("GetDataByID", mock.Anything, "ns1", d.ID, true).Return(d, nil)

	mom := bm.operations.(*operationmocks.Manager)

	mtx := bm.txHelper.(*txcommonmocks.Helper)
	mtx.On("SubmitNewTransaction", context.Background(), core.TransactionTypeDataPublish, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), &sqlcommon.IdempotencyError{
		ExistingTXID:  id,
		OriginalError: i18n.NewError(context.Background(), coremsgs.MsgIdempotencyKeyDuplicateTransaction, "idem1", id)})
	mom.On("ResubmitOperations", context.Background(), id).Return(1 /* total */, nil /* to resubmit */, nil)

	// If ResubmitOperations returns nil it's because there was no operation in initialized state, so we expect the regular 409 error back
	_, err := bm.PublishDataValue(context.Background(), d.ID.String(), "idem1")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "FF10431")

	mom.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestUploadValueIdempotentErrorOnOperationResubmit(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	var id = fftypes.NewUUID()
	defer cancel()

	d := &core.Data{
		ID:    fftypes.NewUUID(),
		Value: fftypes.JSONAnyPtr(`{"some": "value"}`),
	}

	mdi := bm.database.(*databasemocks.Plugin)
	mdi.On("GetDataByID", mock.Anything, "ns1", d.ID, true).Return(d, nil)

	mom := bm.operations.(*operationmocks.Manager)

	mtx := bm.txHelper.(*txcommonmocks.Helper)
	mtx.On("SubmitNewTransaction", context.Background(), core.TransactionTypeDataPublish, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), &sqlcommon.IdempotencyError{
		ExistingTXID:  id,
		OriginalError: i18n.NewError(context.Background(), coremsgs.MsgIdempotencyKeyDuplicateTransaction, "idem1", id)})
	mom.On("ResubmitOperations", context.Background(), id).Return(-1, nil, fmt.Errorf("pop"))

	// If ResubmitOperations returns nil it's because there was no operation in initialized state, so we expect the regular 409 error back
	_, err := bm.PublishDataValue(context.Background(), d.ID.String(), "idem1")
	assert.Error(t, err)
	assert.ErrorContains(t, err, "pop")

	mom.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestUploadBlobFailNoBlob(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	d := &core.Data{
		ID:   fftypes.NewUUID(),
		Blob: nil,
	}

	mdi := bm.database.(*databasemocks.Plugin)
	mdi.On("GetDataByID", mock.Anything, "ns1", d.ID, true).Return(d, nil)

	mtx := bm.txHelper.(*txcommonmocks.Helper)
	mtx.On("SubmitNewTransaction", mock.Anything, core.TransactionTypeDataPublish, core.IdempotencyKey("")).Return(fftypes.NewUUID(), nil)

	_, err := bm.PublishDataBlob(bm.ctx, d.ID.String(), "")
	assert.Regexp(t, "FF10241", err)

	mdi.AssertExpectations(t)
}

func TestUploadBlobOK(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()
	mdx := bm.exchange.(*dataexchangemocks.Plugin)
	mps := bm.sharedstorage.(*sharedstoragemocks.Plugin)
	mdi := bm.database.(*databasemocks.Plugin)
	mom := bm.operations.(*operationmocks.Manager)

	blob := &core.Blob{
		Hash:       fftypes.NewRandB32(),
		PayloadRef: "blob/1",
	}
	d := &core.Data{
		ID: fftypes.NewUUID(),
		Blob: &core.BlobRef{
			Hash: blob.Hash,
		},
	}

	ctx := context.Background()
	mdi.On("GetDataByID", ctx, "ns1", d.ID, true).Return(d, nil)
	mdi.On("GetBlobs", ctx, bm.namespace.Name, mock.Anything).Return([]*core.Blob{blob}, nil, nil)
	mom.On("AddOrReuseOperation", mock.Anything, mock.Anything).Return(nil)
	mom.On("RunOperation", mock.Anything, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(uploadBlobData)
		return op.Type == core.OpTypeSharedStorageUploadBlob && data.Blob == blob
	}), false).Return(nil, nil)

	mtx := bm.txHelper.(*txcommonmocks.Helper)
	mtx.On("SubmitNewTransaction", mock.Anything, core.TransactionTypeDataPublish, core.IdempotencyKey("")).Return(fftypes.NewUUID(), nil)

	d1, err := bm.PublishDataBlob(ctx, d.ID.String(), "")
	assert.NoError(t, err)
	assert.Equal(t, d.ID, d1.ID)

	mdi.AssertExpectations(t)
	mdx.AssertExpectations(t)
	mps.AssertExpectations(t)

}

func TestUploadBlobTXFail(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()
	mdx := bm.exchange.(*dataexchangemocks.Plugin)
	mps := bm.sharedstorage.(*sharedstoragemocks.Plugin)
	mdi := bm.database.(*databasemocks.Plugin)

	blob := &core.Blob{
		Hash:       fftypes.NewRandB32(),
		PayloadRef: "blob/1",
	}
	d := &core.Data{
		ID: fftypes.NewUUID(),
		Blob: &core.BlobRef{
			Hash: blob.Hash,
		},
	}

	ctx := context.Background()
	mdi.On("GetDataByID", ctx, "ns1", d.ID, true).Return(d, nil)

	mtx := bm.txHelper.(*txcommonmocks.Helper)
	mtx.On("SubmitNewTransaction", mock.Anything, core.TransactionTypeDataPublish, core.IdempotencyKey("")).Return(nil, fmt.Errorf("pop"))

	_, err := bm.PublishDataBlob(ctx, d.ID.String(), "")
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
	mdx.AssertExpectations(t)
	mps.AssertExpectations(t)

}

func TestUploadValueTXFail(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()
	mdx := bm.exchange.(*dataexchangemocks.Plugin)
	mps := bm.sharedstorage.(*sharedstoragemocks.Plugin)
	mdi := bm.database.(*databasemocks.Plugin)

	d := &core.Data{
		ID: fftypes.NewUUID(),
	}

	ctx := context.Background()
	mdi.On("GetDataByID", ctx, "ns1", d.ID, true).Return(d, nil)

	mtx := bm.txHelper.(*txcommonmocks.Helper)
	mtx.On("SubmitNewTransaction", mock.Anything, core.TransactionTypeDataPublish, core.IdempotencyKey("")).Return(nil, fmt.Errorf("pop"))

	_, err := bm.PublishDataValue(ctx, d.ID.String(), "")
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
	mdx.AssertExpectations(t)
	mps.AssertExpectations(t)

}
