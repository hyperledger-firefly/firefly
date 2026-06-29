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

package events

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/mocks/sharedstoragemocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestSharedStorageBatchDownloadedOk(t *testing.T) {

	em := newTestEventManager(t)
	defer em.cleanup(t)

	data := &core.Data{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"test"`)}
	batch := sampleBatch(t, core.BatchTypeBroadcast, core.TransactionTypeBatchPin, core.DataArray{data})
	b, _ := json.Marshal(&batch)

	mss := &sharedstoragemocks.Plugin{}
	em.mdi.On("InsertOrGetBatch", em.ctx, mock.Anything).Return(nil, nil)
	em.mdi.On("InsertDataArray", em.ctx, mock.Anything).Return(nil, nil)
	em.mdi.On("InsertMessages", em.ctx, mock.Anything, mock.AnythingOfType("database.PostCompletionHook")).Return(nil, nil).Run(func(args mock.Arguments) {
		args[2].(database.PostCompletionHook)()
	})
	mss.On("Name").Return("utdx").Maybe()
	em.mdm.On("UpdateMessageCache", mock.Anything, mock.Anything).Return()

	em.mim.On("GetLocalNode", mock.Anything).Return(testNode, nil)

	bid, err := em.SharedStorageBatchDownloaded(mss, "payload1", b)
	assert.NoError(t, err)
	assert.Equal(t, batch.ID, bid)

	brw := <-em.aggregator.rewinder.rewindRequests
	assert.Equal(t, *batch.ID, brw.uuid)

	mss.AssertExpectations(t)

}

func TestSharedStorageBatchDownloadedPersistFail(t *testing.T) {

	em := newTestEventManager(t)
	defer em.cleanup(t)
	em.cancel()

	data := &core.Data{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"test"`)}
	batch := sampleBatch(t, core.BatchTypeBroadcast, core.TransactionTypeBatchPin, core.DataArray{data})
	b, _ := json.Marshal(&batch)

	mss := &sharedstoragemocks.Plugin{}
	em.mdi.On("InsertOrGetBatch", em.ctx, mock.Anything).Return(nil, fmt.Errorf("pop"))
	mss.On("Name").Return("utdx").Maybe()

	_, err := em.SharedStorageBatchDownloaded(mss, "payload1", b)
	assert.Regexp(t, "FF00154", err)

	mss.AssertExpectations(t)

}

func TestSharedStorageBatchDownloadedNSMismatch(t *testing.T) {

	em := newTestEventManager(t)
	defer em.cleanup(t)

	data := &core.Data{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"test"`)}
	batch := sampleBatch(t, core.BatchTypeBroadcast, core.TransactionTypeBatchPin, core.DataArray{data})
	b, _ := json.Marshal(&batch)

	mss := &sharedstoragemocks.Plugin{}
	mss.On("Name").Return("utdx").Maybe()

	em.namespace.NetworkName = "ns2"
	_, err := em.SharedStorageBatchDownloaded(mss, "payload1", b)
	assert.NoError(t, err)

	mss.AssertExpectations(t)

}

func TestSharedStorageBatchDownloadedNonMultiparty(t *testing.T) {

	em := newTestEventManager(t)
	defer em.cleanup(t)
	em.multiparty = nil

	data := &core.Data{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"test"`)}
	batch := sampleBatch(t, core.BatchTypeBroadcast, core.TransactionTypeBatchPin, core.DataArray{data})
	b, _ := json.Marshal(&batch)

	mss := &sharedstoragemocks.Plugin{}
	_, err := em.SharedStorageBatchDownloaded(mss, "payload1", b)
	assert.NoError(t, err)

	mss.AssertExpectations(t)

}

func TestSharedStorageBatchDownloadedBadData(t *testing.T) {

	em := newTestEventManager(t)
	defer em.cleanup(t)

	mss := &sharedstoragemocks.Plugin{}
	mss.On("Name").Return("utdx").Maybe()

	_, err := em.SharedStorageBatchDownloaded(mss, "payload1", []byte("!json"))
	assert.NoError(t, err)

	mss.AssertExpectations(t)

}

func TestSharedStorageBlobDownloadedOk(t *testing.T) {

	em := newTestEventManager(t)
	defer em.cleanup(t)

	mss := &sharedstoragemocks.Plugin{}
	mss.On("Name").Return("utsd")
	em.mdi.On("GetBlobs", em.ctx, mock.Anything, mock.Anything).Return(nil, nil, nil)
	em.mdi.On("InsertBlobs", em.ctx, mock.Anything).Return(nil, nil)

	hash := fftypes.NewRandB32()
	dataID := fftypes.NewUUID()
	em.SharedStorageBlobDownloaded(mss, *hash, 12345, "payload1", dataID)

	brw := <-em.aggregator.rewinder.rewindRequests
	assert.Equal(t, rewind{hash: *hash, rewindType: rewindBlob}, brw)

	mss.AssertExpectations(t)

}
