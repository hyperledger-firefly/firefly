// Copyright © 2021 Kaleido, Inc.
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

package shareddownload

import (
	"context"
	"fmt"
	"io/ioutil"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-common/pkg/ffapi"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly/internal/coreconfig"
	"github.com/hyperledger/firefly/mocks/databasemocks"
	"github.com/hyperledger/firefly/mocks/dataexchangemocks"
	"github.com/hyperledger/firefly/mocks/operationmocks"
	"github.com/hyperledger/firefly/mocks/shareddownloadmocks"
	"github.com/hyperledger/firefly/mocks/sharedstoragemocks"
	"github.com/hyperledger/firefly/pkg/core"
	"github.com/hyperledger/firefly/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func newTestDownloadManager(t *testing.T) (*downloadManager, func()) {
	coreconfig.Reset()
	config.Set(coreconfig.DownloadWorkerCount, 1)
	config.Set(coreconfig.DownloadRetryMaxAttempts, 0 /* bumps to 1 */)

	mdi := &databasemocks.Plugin{}
	mss := &sharedstoragemocks.Plugin{}
	mdx := &dataexchangemocks.Plugin{}
	mci := &shareddownloadmocks.Callbacks{}
	mom := &operationmocks.Manager{}
	mom.On("RegisterHandler", mock.Anything, mock.Anything, []core.OpType{
		core.OpTypeSharedStorageDownloadBatch,
		core.OpTypeSharedStorageDownloadBlob,
	}).Return()

	ctx, cancel := context.WithCancel(context.Background())
	ns := &core.Namespace{Name: "ns1", NetworkName: "ns1"}
	pm, err := NewDownloadManager(ctx, ns, mdi, mss, mdx, mom, mci)
	assert.NoError(t, err)

	return pm.(*downloadManager), cancel
}

func TestNewDownloadManagerMissingDeps(t *testing.T) {
	_, err := NewDownloadManager(context.Background(), &core.Namespace{}, nil, nil, nil, nil, nil)
	assert.Regexp(t, "FF10128", err)
}

func TestName(t *testing.T) {
	dm, _ := newTestDownloadManager(t)
	assert.Equal(t, "SharedStorageDownloadManager", dm.Name())
}

func TestDownloadBatchE2EOk(t *testing.T) {

	dm, cancel := newTestDownloadManager(t)
	defer cancel()
	dm.workerCount = 1
	dm.workers = []*downloadWorker{newDownloadWorker(dm, 0)}

	reader := ioutil.NopCloser(strings.NewReader("some batch data"))
	txID := fftypes.NewUUID()
	batchID := fftypes.NewUUID()

	mss := dm.sharedstorage.(*sharedstoragemocks.Plugin)
	mss.On("Name").Return("utss")
	mss.On("DownloadData", mock.Anything, "ref1").Return(reader, nil)

	called := make(chan struct{})

	opID := fftypes.NewUUID()

	mom := dm.operations.(*operationmocks.Manager)
	mom.On("AddOrReuseOperation", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		*opID = *args[1].(*core.Operation).ID
		args[2].(database.PostCompletionHook)()
	}).Return(nil)
	mom.On("RunOperation", mock.Anything, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		assert.Equal(t, core.OpTypeSharedStorageDownloadBatch, op.Type)
		assert.Equal(t, "ns1", op.Namespace)
		assert.Equal(t, "ref1", op.Data.(downloadBatchData).PayloadRef)
		return true
	}), mock.Anything).Return(nil, nil).Run(func(args mock.Arguments) {
		output, phase, err := dm.RunOperation(args[0].(context.Context), args[1].(*core.PreparedOperation))
		assert.NoError(t, err)
		assert.Equal(t, fftypes.JSONObject{"batch": batchID}, output)
		assert.Equal(t, core.OpPhaseComplete, phase)
		close(called)
	})

	mci := dm.callbacks.(*shareddownloadmocks.Callbacks)
	mci.On("SharedStorageBatchDownloaded", "ref1", []byte("some batch data")).Return(batchID, nil)

	err := dm.InitiateDownloadBatch(dm.ctx, txID, "ref1", false)
	assert.NoError(t, err)

	<-called

	mss.AssertExpectations(t)
	mci.AssertExpectations(t)
	mom.AssertExpectations(t)

}

func TestDownloadBlobWithRetryOk(t *testing.T) {

	dm, _ := newTestDownloadManager(t)
	defer dm.WaitStop()
	dm.workerCount = 1
	dm.retryMaxAttempts = 3
	dm.retryInitDelay = 10 * time.Microsecond
	dm.retryMaxDelay = 15 * time.Microsecond
	dm.workers = []*downloadWorker{newDownloadWorker(dm, 0)}

	reader := ioutil.NopCloser(strings.NewReader("some blob data"))
	txID := fftypes.NewUUID()
	dataID := fftypes.NewUUID()
	blobHash := fftypes.NewRandB32()

	mss := dm.sharedstorage.(*sharedstoragemocks.Plugin)
	mss.On("Name").Return("utss")
	mss.On("DownloadData", mock.Anything, "ref1").Return(reader, nil)

	mdx := dm.dataexchange.(*dataexchangemocks.Plugin)
	mdx.On("UploadBlob", mock.Anything, "ns1", *dataID, mock.Anything).Return("", nil, int64(-1), fmt.Errorf("pop")).Twice()
	mdx.On("UploadBlob", mock.Anything, "ns1", *dataID, mock.Anything).Return("privateRef1", blobHash, int64(12345), nil)

	called := make(chan struct{})

	mom := dm.operations.(*operationmocks.Manager)
	mom.On("AddOrReuseOperation", mock.Anything, mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		args[2].(database.PostCompletionHook)()
	}).Return(nil)
	mom.On("RunOperation", mock.Anything, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		assert.Equal(t, core.OpTypeSharedStorageDownloadBlob, op.Type)
		assert.Equal(t, "ns1", op.Namespace)
		assert.Equal(t, dataID, op.Data.(downloadBlobData).DataID)
		assert.Equal(t, "ref1", op.Data.(downloadBlobData).PayloadRef)
		return true
	}), mock.Anything).Return(nil, fmt.Errorf("pop")).Run(func(args mock.Arguments) {
		_, _, err := dm.RunOperation(args[0].(context.Context), args[1].(*core.PreparedOperation))
		assert.Regexp(t, "FF10376", err)
	}).Twice()
	mom.On("RunOperation", mock.Anything, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		assert.Equal(t, core.OpTypeSharedStorageDownloadBlob, op.Type)
		assert.Equal(t, "ns1", op.Namespace)
		assert.Equal(t, dataID, op.Data.(downloadBlobData).DataID)
		assert.Equal(t, "ref1", op.Data.(downloadBlobData).PayloadRef)
		return true
	}), mock.Anything).Return(nil, nil).Run(func(args mock.Arguments) {
		output, phase, err := dm.RunOperation(args[0].(context.Context), args[1].(*core.PreparedOperation))
		assert.NoError(t, err)
		assert.Equal(t, fftypes.JSONObject{
			"dxPayloadRef": "privateRef1",
			"hash":         blobHash,
			"size":         12345,
		}.String(), output.String())
		assert.Equal(t, core.OpPhaseComplete, phase)
		close(called)
	}).Once()

	mci := dm.callbacks.(*shareddownloadmocks.Callbacks)
	mci.On("SharedStorageBlobDownloaded", *blobHash, int64(12345), "privateRef1", dataID).Return(nil)

	err := dm.InitiateDownloadBlob(dm.ctx, txID, dataID, "ref1", false)
	assert.NoError(t, err)

	<-called

	mss.AssertExpectations(t)
	mdx.AssertExpectations(t)
	mci.AssertExpectations(t)
	mom.AssertExpectations(t)

}

func TestDownloadBlobInsertOpFail(t *testing.T) {

	dm, cancel := newTestDownloadManager(t)
	defer cancel()

	txID := fftypes.NewUUID()
	dataID := fftypes.NewUUID()

	mss := dm.sharedstorage.(*sharedstoragemocks.Plugin)
	mss.On("Name").Return("utss")

	mom := dm.operations.(*operationmocks.Manager)
	mom.On("AddOrReuseOperation", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))

	err := dm.InitiateDownloadBlob(dm.ctx, txID, dataID, "ref1", false)
	assert.Regexp(t, "pop", err)

	mom.AssertExpectations(t)

}

func TestDownloadManagerStartupRecoveryCombinations(t *testing.T) {

	dm, cancel := newTestDownloadManager(t)
	defer cancel()
	dm.workerCount = 1
	dm.retryInitDelay = 1 * time.Microsecond
	dm.workers = []*downloadWorker{newDownloadWorker(dm, 0)}

	called := make(chan bool)

	reader := ioutil.NopCloser(strings.NewReader("some batch data"))
	batchID := fftypes.NewUUID()

	mss := dm.sharedstorage.(*sharedstoragemocks.Plugin)
	mss.On("DownloadData", mock.Anything, "ref1").Return(nil, fmt.Errorf("pop"))
	mss.On("DownloadData", mock.Anything, "ref2").Return(reader, nil)

	mdi := dm.database.(*databasemocks.Plugin)
	mdi.On("GetOperations", mock.Anything, "ns1", mock.Anything).Return([]*core.Operation{}, nil, fmt.Errorf("initial error")).Once()
	mdi.On("GetOperations", mock.Anything, "ns1", mock.MatchedBy(func(filter ffapi.Filter) bool {
		fi, err := filter.Finalize()
		assert.NoError(t, err)
		return fi.Skip == 0 && fi.Limit == 25
	})).Return([]*core.Operation{
		{
			// This one won't submit
			Type:      core.OpTypeSharedStorageDownloadBlob,
			ID:        fftypes.NewUUID(),
			Namespace: "ns1",
			Input: fftypes.JSONObject{
				"bad": "inputs",
			},
		},
		{
			// This one will be re-submitted and be marked failed
			Type:      core.OpTypeSharedStorageDownloadBlob,
			ID:        fftypes.NewUUID(),
			Namespace: "ns1",
			Input: fftypes.JSONObject{
				"namespace":  "ns1",
				"dataId":     fftypes.NewUUID().String(),
				"payloadRef": "ref1",
			},
		},
		{
			// This one will be re-submitted and succeed
			Type:      core.OpTypeSharedStorageDownloadBatch,
			ID:        fftypes.NewUUID(),
			Namespace: "ns1",
			Input: fftypes.JSONObject{
				"namespace":  "ns1",
				"payloadRef": "ref2",
			},
		},
	}, nil, nil).Once()
	mdi.On("GetOperations", mock.Anything, "ns1", mock.MatchedBy(func(filter ffapi.Filter) bool {
		fi, err := filter.Finalize()
		assert.NoError(t, err)
		return fi.Skip == 25 && fi.Limit == 25
	})).Return([]*core.Operation{}, nil, nil).Once()

	mom := dm.operations.(*operationmocks.Manager)
	mom.On("RunOperation", mock.Anything, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		return op.Type == core.OpTypeSharedStorageDownloadBlob && op.Data.(downloadBlobData).PayloadRef == "ref1"
	}), mock.Anything).Return(nil, fmt.Errorf("pop")).Run(func(args mock.Arguments) {
		_, _, err := dm.RunOperation(args[0].(context.Context), args[1].(*core.PreparedOperation))
		assert.EqualError(t, err, "pop")
		called <- true
	})
	mom.On("RunOperation", mock.Anything, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		return op.Type == core.OpTypeSharedStorageDownloadBatch && op.Data.(downloadBatchData).PayloadRef == "ref2"
	}), mock.Anything).Return(nil, nil).Run(func(args mock.Arguments) {
		output, phase, err := dm.RunOperation(args[0].(context.Context), args[1].(*core.PreparedOperation))
		assert.NoError(t, err)
		assert.Equal(t, fftypes.JSONObject{
			"batch": batchID,
		}.String(), output.String())
		assert.Equal(t, core.OpPhaseComplete, phase)
		called <- true
	})
	//mom.On("SubmitOperationUpdate", mock.Anything).Return(nil)
	updateCalled := make(chan bool)
	mom.On("SubmitOperationUpdate", mock.Anything).Return(nil).Run(func(args mock.Arguments) {
		updateCalled <- true
	})

	mci := dm.callbacks.(*shareddownloadmocks.Callbacks)
	mci.On("SharedStorageBatchDownloaded", "ref2", []byte("some batch data")).Return(batchID, nil)

	err := dm.Start()
	assert.NoError(t, err)

	<-called
	<-called
	<-dm.recoveryComplete
	<-updateCalled

	mss.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mci.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestPrepareOperationUnknown(t *testing.T) {

	dm, cancel := newTestDownloadManager(t)
	defer cancel()

	_, err := dm.PrepareOperation(dm.ctx, &core.Operation{
		Type: core.CallTypeInvoke,
	})
	assert.Regexp(t, "FF10371", err)
}

func TestRunOperationUnknown(t *testing.T) {

	dm, cancel := newTestDownloadManager(t)
	defer cancel()

	_, _, err := dm.RunOperation(dm.ctx, &core.PreparedOperation{
		Type: core.CallTypeInvoke,
	})
	assert.Regexp(t, "FF10378", err)
}
