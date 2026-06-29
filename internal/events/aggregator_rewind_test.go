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

package events

import (
	"fmt"
	"testing"
	"time"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/internal/data"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestRewinderE2E(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)
	ag.rewinder.minRewindTimeout = 1 * time.Millisecond
	dataID := fftypes.NewUUID()
	batchID1 := fftypes.NewUUID()
	batchID2 := fftypes.NewUUID()
	batchID3 := fftypes.NewUUID()
	batchID4 := fftypes.NewUUID()
	batchID5 := fftypes.NewUUID()

	mockRunAsGroupPassthrough(ag.mdi)
	ag.mdi.On("GetDataRefs", mock.Anything, "ns1", mock.Anything).
		Return(core.DataRefs{{ID: dataID}}, nil, nil)
	ag.mdi.On("GetBatchIDsForDataAttachments", mock.Anything, "ns1", []*fftypes.UUID{dataID}).
		Return([]*fftypes.UUID{batchID2}, nil)
	ag.mdm.On("PeekMessageCache", mock.Anything, mock.Anything, data.CRORequireBatchID).Return(nil, nil)
	ag.mdi.On("GetBatchIDsForMessages", mock.Anything, "ns1", mock.Anything).
		Return([]*fftypes.UUID{batchID3}, nil).Once()
	ag.mdi.On("GetMessageIDs", mock.Anything, "ns1", mock.Anything).
		Return([]*core.IDAndSequence{{ID: *fftypes.NewUUID()}}, nil).Once()
	ag.mdi.On("GetBatchIDsForMessages", mock.Anything, "ns1", mock.Anything).
		Return([]*fftypes.UUID{batchID4}, nil).Once()

	ag.rewinder.start()

	ag.queueBatchRewind(batchID1)
	ag.queueBlobRewind(fftypes.NewRandB32())
	ag.queueMessageRewind(fftypes.NewUUID())
	ag.queueDIDRewind("did:firefly:org/bob")
	ag.queueBatchRewind(batchID5)

	allReceived := make(map[fftypes.UUID]bool)
	for len(allReceived) < 4 {
		time.Sleep(1 * time.Millisecond)
		batchIDs := ag.rewinder.popRewinds()
		for _, bid := range batchIDs {
			allReceived[bid.(fftypes.UUID)] = true
		}
	}
	assert.True(t, allReceived[*batchID1])
	assert.True(t, allReceived[*batchID2])
	assert.True(t, allReceived[*batchID3])
	assert.True(t, allReceived[*batchID4])
	assert.True(t, allReceived[*batchID5])

	ag.cancel()
	<-ag.rewinder.loop1Done
	<-ag.rewinder.loop2Done

}

func TestProcessStagedRewindsErrorMessages(t *testing.T) {

	ag := newTestAggregator()
	defer ag.cleanup(t)
	ag.cancel()

	mockRunAsGroupPassthrough(ag.mdi)
	ag.mdm.On("PeekMessageCache", mock.Anything, mock.Anything, data.CRORequireBatchID).Return(nil, nil)
	ag.mdi.On("GetBatchIDsForMessages", mock.Anything, "ns1", mock.Anything).Return(nil, fmt.Errorf("pop"))

	ag.rewinder.stagedRewinds = []*rewind{
		{rewindType: rewindMessage},
	}
	ag.rewinder.processStagedRewinds()

}

func TestProcessStagedRewindsMessagesCached(t *testing.T) {

	ag := newTestAggregator()
	defer ag.cleanup(t)
	ag.cancel()

	mockRunAsGroupPassthrough(ag.mdi)
	ag.mdm.On("PeekMessageCache", mock.Anything, mock.Anything, data.CRORequireBatchID).Return(&core.Message{
		BatchID: fftypes.NewUUID(),
	}, nil)

	ag.rewinder.stagedRewinds = []*rewind{
		{rewindType: rewindMessage},
	}
	ag.rewinder.processStagedRewinds()

}

func TestProcessStagedRewindsErrorBlobBatchIDs(t *testing.T) {

	ag := newTestAggregator()
	defer ag.cleanup(t)
	ag.cancel()

	dataID := fftypes.NewUUID()

	mockRunAsGroupPassthrough(ag.mdi)
	ag.mdi.On("GetDataRefs", mock.Anything, "ns1", mock.Anything).
		Return(core.DataRefs{{ID: dataID}}, nil, nil)
	ag.mdi.On("GetBatchIDsForDataAttachments", mock.Anything, "ns1", []*fftypes.UUID{dataID}).
		Return(nil, fmt.Errorf("pop"))

	ag.rewinder.stagedRewinds = []*rewind{
		{rewindType: rewindBlob},
	}
	ag.rewinder.processStagedRewinds()

}

func TestProcessStagedRewindsErrorBlobDataRefs(t *testing.T) {

	ag := newTestAggregator()
	defer ag.cleanup(t)
	ag.cancel()

	mockRunAsGroupPassthrough(ag.mdi)
	ag.mdi.On("GetDataRefs", mock.Anything, "ns1", mock.Anything).
		Return(nil, nil, fmt.Errorf("pop"))

	ag.rewinder.stagedRewinds = []*rewind{
		{rewindType: rewindBlob},
	}
	ag.rewinder.processStagedRewinds()

}

func TestProcessStagedRewindsErrorDIDs(t *testing.T) {

	ag := newTestAggregator()
	defer ag.cleanup(t)
	ag.cancel()

	mockRunAsGroupPassthrough(ag.mdi)
	ag.mdi.On("GetMessageIDs", mock.Anything, "ns1", mock.Anything).
		Return(nil, fmt.Errorf("pop"))

	ag.rewinder.stagedRewinds = []*rewind{
		{rewindType: rewindDIDConfirmed},
	}
	ag.rewinder.processStagedRewinds()

}

func TestProcessStagedRewindsNoDIDs(t *testing.T) {

	ag := newTestAggregator()
	defer ag.cleanup(t)
	ag.cancel()

	mockRunAsGroupPassthrough(ag.mdi)
	ag.mdi.On("GetMessageIDs", mock.Anything, "ns1", mock.Anything).
		Return([]*core.IDAndSequence{}, nil)

	ag.rewinder.stagedRewinds = []*rewind{
		{rewindType: rewindDIDConfirmed},
	}
	ag.rewinder.processStagedRewinds()

}

func TestPopRewindsDoublePopNoBlock(t *testing.T) {

	em := newTestEventManager(t)
	defer em.cleanup(t)

	em.QueueBatchRewind(fftypes.NewUUID())
	batchIDs := em.aggregator.rewinder.popRewinds()
	assert.Empty(t, batchIDs)

	em.QueueBatchRewind(fftypes.NewUUID())
	batchIDs = em.aggregator.rewinder.popRewinds()
	assert.Empty(t, batchIDs)

}

func TestPopRewindsNoTapBlocked(t *testing.T) {

	em := newTestEventManager(t)
	defer em.cleanup(t)

	em.aggregator.rewinder.loop2ShoulderTap <- true
	em.aggregator.rewinder.stagedRewinds = []*rewind{{}}

	_ = em.aggregator.rewinder.popRewinds()

}
