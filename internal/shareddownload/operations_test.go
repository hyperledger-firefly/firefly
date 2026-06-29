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
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/mocks/dataexchangemocks"
	"github.com/hyperledger-firefly/firefly/mocks/shareddownloadmocks"
	"github.com/hyperledger-firefly/firefly/mocks/sharedstoragemocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestDownloadBatchDownloadDataFail(t *testing.T) {

	dm, cancel := newTestDownloadManager(t)
	defer cancel()

	mss := dm.sharedstorage.(*sharedstoragemocks.Plugin)
	mss.On("DownloadData", mock.Anything, "ref1").Return(nil, fmt.Errorf("pop"))

	_, _, err := dm.downloadBatch(dm.ctx, downloadBatchData{
		PayloadRef: "ref1",
	})
	assert.Regexp(t, "FF10376", err)

	mss.AssertExpectations(t)
}

func TestDownloadBatchDownloadDataReadFail(t *testing.T) {

	dm, cancel := newTestDownloadManager(t)
	defer cancel()

	reader := ioutil.NopCloser(iotest.ErrReader(fmt.Errorf("read failed")))

	mss := dm.sharedstorage.(*sharedstoragemocks.Plugin)
	mss.On("DownloadData", mock.Anything, "ref1").Return(reader, nil)

	_, _, err := dm.downloadBatch(dm.ctx, downloadBatchData{
		PayloadRef: "ref1",
	})
	assert.Regexp(t, "FF10376", err)

	mss.AssertExpectations(t)
}

func TestDownloadBatchDownloadDataReadMaxedOut(t *testing.T) {

	dm, cancel := newTestDownloadManager(t)
	defer cancel()

	dm.broadcastBatchPayloadLimit = 1
	reader := ioutil.NopCloser(bytes.NewBuffer(make([]byte, 2048)))

	mss := dm.sharedstorage.(*sharedstoragemocks.Plugin)
	mss.On("DownloadData", mock.Anything, "ref1").Return(reader, nil)

	_, _, err := dm.downloadBatch(dm.ctx, downloadBatchData{
		PayloadRef: "ref1",
	})
	assert.Regexp(t, "FF10377", err)

	mss.AssertExpectations(t)
}

func TestDownloadBatchDownloadCallbackFailed(t *testing.T) {

	dm, cancel := newTestDownloadManager(t)
	defer cancel()

	reader := ioutil.NopCloser(strings.NewReader("some batch data"))

	mss := dm.sharedstorage.(*sharedstoragemocks.Plugin)
	mss.On("DownloadData", mock.Anything, "ref1").Return(reader, nil)

	mci := dm.callbacks.(*shareddownloadmocks.Callbacks)
	mci.On("SharedStorageBatchDownloaded", "ref1", []byte("some batch data")).Return(nil, fmt.Errorf("pop"))

	_, _, err := dm.downloadBatch(dm.ctx, downloadBatchData{
		PayloadRef: "ref1",
	})
	assert.Regexp(t, "pop", err)

	mss.AssertExpectations(t)
	mci.AssertExpectations(t)
}

func TestDownloadBlobDownloadDataReadFail(t *testing.T) {

	dm, cancel := newTestDownloadManager(t)
	defer cancel()

	reader := ioutil.NopCloser(iotest.ErrReader(fmt.Errorf("read failed")))

	mss := dm.sharedstorage.(*sharedstoragemocks.Plugin)
	mss.On("DownloadData", mock.Anything, "ref1").Return(reader, nil)

	mdx := dm.dataexchange.(*dataexchangemocks.Plugin)
	mdx.On("UploadBlob", mock.Anything, "ns1", mock.Anything, reader).Return("", nil, int64(-1), fmt.Errorf("pop"))

	_, _, err := dm.downloadBlob(dm.ctx, downloadBlobData{
		PayloadRef: "ref1",
		DataID:     fftypes.NewUUID(),
	})
	assert.Regexp(t, "FF10376", err)

	mss.AssertExpectations(t)
	mdx.AssertExpectations(t)
}

func TestDownloadBlobDownloadDataCBCalled(t *testing.T) {

	dm, cancel := newTestDownloadManager(t)
	defer cancel()

	reader := ioutil.NopCloser(iotest.ErrReader(fmt.Errorf("read failed")))

	mss := dm.sharedstorage.(*sharedstoragemocks.Plugin)
	mss.On("DownloadData", mock.Anything, "ref1").Return(reader, nil)

	mdx := dm.dataexchange.(*dataexchangemocks.Plugin)
	mdx.On("UploadBlob", mock.Anything, "ns1", mock.Anything, reader).Return("", fftypes.NewRandB32(), int64(-1), nil)

	mdc := &shareddownloadmocks.Callbacks{}
	mdc.On("SharedStorageBlobDownloaded", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
	dm.callbacks = mdc

	_, _, err := dm.downloadBlob(dm.ctx, downloadBlobData{
		PayloadRef: "ref1",
		DataID:     fftypes.NewUUID(),
	})
	assert.Regexp(t, "pop", err)

	mss.AssertExpectations(t)
	mdx.AssertExpectations(t)
}

func TestOperationUpdate(t *testing.T) {
	dm, cancel := newTestDownloadManager(t)
	defer cancel()
	assert.NoError(t, dm.OnOperationUpdate(context.Background(), nil, nil))
}
