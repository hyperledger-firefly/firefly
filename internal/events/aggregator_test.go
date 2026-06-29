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

package events

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/cache"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/data"
	"github.com/hyperledger-firefly/firefly/internal/definitions"
	"github.com/hyperledger-firefly/firefly/mocks/blockchainmocks"
	"github.com/hyperledger-firefly/firefly/mocks/cachemocks"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/mocks/datamocks"
	"github.com/hyperledger-firefly/firefly/mocks/definitionsmocks"
	"github.com/hyperledger-firefly/firefly/mocks/identitymanagermocks"
	"github.com/hyperledger-firefly/firefly/mocks/metricsmocks"
	"github.com/hyperledger-firefly/firefly/mocks/privatemessagingmocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type testAggregator struct {
	aggregator

	cancel func()
	mdi    *databasemocks.Plugin
	mdm    *datamocks.Manager
	mpm    *privatemessagingmocks.Manager
	mbi    *blockchainmocks.Plugin
	mim    *identitymanagermocks.Manager
	mmi    *metricsmocks.Manager
	mdh    *definitionsmocks.Handler
}

func (tag *testAggregator) cleanup(t *testing.T) {
	tag.cancel()
	tag.mdi.AssertExpectations(t)
	tag.mdm.AssertExpectations(t)
	tag.mpm.AssertExpectations(t)
	tag.mbi.AssertExpectations(t)
	tag.mim.AssertExpectations(t)
	tag.mmi.AssertExpectations(t)
	tag.mdh.AssertExpectations(t)
}

func newTestAggregatorCommon(metrics bool) *testAggregator {
	coreconfig.Reset()
	ctx, ctxCancel := context.WithCancel(context.Background())
	logrus.SetLevel(logrus.DebugLevel)
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	mpm := &privatemessagingmocks.Manager{}
	mdh := &definitionsmocks.Handler{}
	mim := &identitymanagermocks.Manager{}
	mmi := &metricsmocks.Manager{}
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	mbi := &blockchainmocks.Plugin{}
	if metrics {
		mmi.On("MessageConfirmed", mock.Anything, core.EventTypeMessageConfirmed).Return()
	}
	mmi.On("IsMetricsEnabled").Return(metrics).Maybe()
	mbi.On("VerifierType").Return(core.VerifierTypeEthAddress)
	ag, _ := newAggregator(ctx, "ns1", mdi, mbi, mpm, mdh, mim, mdm, newEventNotifier(ctx, "ut"), mmi, cmi)
	cancel := func() {
		ctxCancel()
		if ag.batchCache != nil {
			ag.batchCache = nil
		}
	}
	return &testAggregator{
		aggregator: *ag,
		cancel:     cancel,
		mdi:        mdi,
		mdm:        mdm,
		mpm:        mpm,
		mdh:        mdh,
		mim:        mim,
		mmi:        mmi,
		mbi:        mbi,
	}
}

func newTestAggregatorWithMetrics() *testAggregator {
	return newTestAggregatorCommon(true)
}

func newTestAggregator() *testAggregator {
	return newTestAggregatorCommon(false)
}

func newTestManifest(mType core.MessageType, groupID *fftypes.Bytes32) (*core.Message, *core.Message, *core.Identity, *core.BatchManifest) {
	org1 := newTestOrg("org1")

	msg1 := &core.Message{
		Header: core.MessageHeader{
			Type:      mType,
			ID:        fftypes.NewUUID(),
			Namespace: "any",
			Group:     groupID,
			Topics:    fftypes.FFStringArray{"topic1"},
			SignerRef: core.SignerRef{Key: "0x12345", Author: org1.DID},
		},
		Data: core.DataRefs{
			{ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32()},
		},
	}
	msg2 := &core.Message{
		Header: core.MessageHeader{
			Type:      mType,
			ID:        fftypes.NewUUID(),
			Group:     groupID,
			Namespace: "any",
			Topics:    fftypes.FFStringArray{"topic1"},
			SignerRef: core.SignerRef{Key: "0x12345", Author: org1.DID},
		},
		Data: core.DataRefs{
			{ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32()},
		},
	}

	return msg1, msg2, org1, &core.BatchManifest{
		Version: 1,
		ID:      fftypes.NewUUID(),
		TX: core.TransactionRef{
			Type: core.TransactionTypeBatchPin,
			ID:   fftypes.NewUUID(),
		},
		Messages: []*core.MessageManifestEntry{
			{
				MessageRef: core.MessageRef{
					ID:   msg1.Header.ID,
					Hash: msg1.Hash,
				},
				Topics: len(msg1.Header.Topics),
			},
			{
				MessageRef: core.MessageRef{
					ID:   msg2.Header.ID,
					Hash: msg2.Hash,
				},
				Topics: len(msg2.Header.Topics),
			},
		},
	}
}

func TestNewAggregator(t *testing.T) {
	coreconfig.Reset()
	ctx := context.Background()
	logrus.SetLevel(logrus.DebugLevel)
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	mpm := &privatemessagingmocks.Manager{}
	mdh := &definitionsmocks.Handler{}
	mim := &identitymanagermocks.Manager{}
	mmi := &metricsmocks.Manager{}
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	mbi := &blockchainmocks.Plugin{}
	mbi.On("VerifierType").Return(core.VerifierTypeEthAddress)
	ns := "ns1"
	_, err := newAggregator(ctx, ns, mdi, mbi, mpm, mdh, mim, mdm, newEventNotifier(ctx, "ut"), mmi, cmi)
	assert.NoError(t, err)
	cmi.AssertCalled(t, "GetCache", cache.NewCacheConfig(
		ctx,
		coreconfig.CacheBatchLimit,
		coreconfig.CacheBatchTTL,
		ns,
	))
}
func TestCacheInitFail(t *testing.T) {
	coreconfig.Reset()
	ctx := context.Background()
	cacheInitError := errors.New("Initialization error.")
	logrus.SetLevel(logrus.DebugLevel)
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	mpm := &privatemessagingmocks.Manager{}
	mdh := &definitionsmocks.Handler{}
	mim := &identitymanagermocks.Manager{}
	mmi := &metricsmocks.Manager{}
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(nil, cacheInitError)
	mbi := &blockchainmocks.Plugin{}
	mbi.On("VerifierType").Return(core.VerifierTypeEthAddress)
	ns := "ns1"
	_, err := newAggregator(ctx, ns, mdi, mbi, mpm, mdh, mim, mdm, newEventNotifier(ctx, "ut"), mmi, cmi)
	assert.Equal(t, cacheInitError, err)
}

func TestAggregationMaskedZeroNonceMatch(t *testing.T) {

	ag := newTestAggregatorWithMetrics()
	defer ag.cleanup(t)
	bs := newBatchState(&ag.aggregator)

	// Generate some pin data
	member1org := newTestOrg("org1")
	member2org := newTestOrg("org2")
	member2node := newTestNode("node2", member2org)
	member2key := "0x23456"
	topic := "some-topic"
	batchID := fftypes.NewUUID()
	groupID := fftypes.NewRandB32()
	msgID := fftypes.NewUUID()
	contextUnmasked := privateContext(topic, groupID)
	initNPG := &nextPinGroupState{topic: topic, groupID: groupID}
	member1NonceZero := initNPG.calcPinHash(member1org.DID, 0)
	member2NonceZero := initNPG.calcPinHash(member2org.DID, 0)
	member2NonceOne := initNPG.calcPinHash(member2org.DID, 1)

	ag.mim.On("FindIdentityForVerifier", ag.ctx, []core.IdentityType{core.IdentityTypeOrg, core.IdentityTypeCustom}, &core.VerifierRef{
		Type:  core.VerifierTypeEthAddress,
		Value: member2key,
	}).Return(member2org, nil)

	batch := &core.Batch{
		BatchHeader: core.BatchHeader{
			ID:   batchID,
			Node: member2node.ID,
			SignerRef: core.SignerRef{
				Author: member2org.DID,
			},
		},
		Payload: core.BatchPayload{
			Messages: []*core.Message{
				{
					Header: core.MessageHeader{
						ID:        msgID,
						Group:     groupID,
						Namespace: "ns1",
						Topics:    []string{topic},
						SignerRef: core.SignerRef{
							Author: member2org.DID,
							Key:    member2key,
						},
					},
					Pins: []string{fmt.Sprintf("%s:%.9d", member2NonceZero, 0)},
					Data: core.DataRefs{
						{ID: fftypes.NewUUID()},
					},
				},
			},
		},
	}
	bp, _ := batch.Confirmed()

	// Get the batch
	ag.mdi.On("GetBatchByID", ag.ctx, "ns1", batchID).Return(bp, nil)
	// Look for existing nextpins - none found, first on context
	ag.mdi.On("GetNextPinsForContext", ag.ctx, "ns1", contextUnmasked).Return([]*core.NextPin{}, nil).Once()
	// Get the group members
	ag.mpm.On("ResolveInitGroup", ag.ctx, mock.Anything, &core.Member{
		Identity: member2org.DID,
		Node:     member2node.ID,
	}).Return(&core.Group{
		GroupIdentity: core.GroupIdentity{
			Members: core.Members{
				{Identity: member1org.DID},
				{Identity: member2org.DID},
			},
		},
	}, nil)
	// Look for any earlier pins - none found
	ag.mdi.On("GetPins", ag.ctx, "ns1", mock.Anything).Return([]*core.Pin{}, nil, nil).Once()
	// Insert all the zero pins
	ag.mdi.On("InsertNextPin", ag.ctx, mock.MatchedBy(func(np *core.NextPin) bool {
		assert.Equal(t, *np.Context, *contextUnmasked)
		np.Sequence = 10011
		return *np.Hash == *member1NonceZero && np.Nonce == 0
	})).Return(nil).Once()
	ag.mdi.On("InsertNextPin", ag.ctx, mock.MatchedBy(func(np *core.NextPin) bool {
		assert.Equal(t, *np.Context, *contextUnmasked)
		np.Sequence = 10012
		return *np.Hash == *member2NonceOne && np.Nonce == 1
	})).Return(nil).Once()
	// Validate the message is ok
	ag.mdm.On("GetMessageWithDataCached", ag.ctx, batch.Payload.Messages[0].Header.ID, data.CRORequirePins).Return(batch.Payload.Messages[0], core.DataArray{}, true, nil)
	ag.mdm.On("ValidateAll", ag.ctx, mock.Anything).Return(true, nil)
	ag.mdm.On("UpdateMessageStateIfCached", ag.ctx, mock.Anything, core.MessageStateConfirmed, mock.Anything, "").Return()
	// Insert the confirmed event
	ag.mdi.On("InsertEvent", ag.ctx, mock.MatchedBy(func(e *core.Event) bool {
		return *e.Reference == *msgID && e.Type == core.EventTypeMessageConfirmed
	})).Return(nil)
	// Set the pin to dispatched
	ag.mdi.On("UpdatePins", ag.ctx, "ns1", mock.Anything, mock.Anything).Return(nil)
	// Update the message
	ag.mdi.On("UpdateMessages", ag.ctx, "ns1", mock.Anything, mock.MatchedBy(func(u ffapi.Update) bool {
		update, err := u.Finalize()
		assert.NoError(t, err)
		assert.Len(t, update.SetOperations, 3)

		assert.Equal(t, "confirmed", update.SetOperations[0].Field)
		v, err := update.SetOperations[0].Value.Value()
		assert.NoError(t, err)
		assert.Greater(t, v, int64(0))

		assert.Equal(t, "state", update.SetOperations[1].Field)
		v, err = update.SetOperations[1].Value.Value()
		assert.NoError(t, err)
		assert.Equal(t, "confirmed", v)

		return true
	})).Return(nil)

	err := ag.processPins(ag.ctx, []*core.Pin{
		{
			Sequence:   10001,
			Masked:     true,
			Hash:       member2NonceZero,
			Batch:      batchID,
			Index:      0,
			Signer:     member2key,
			Dispatched: false,
		},
	}, bs)
	assert.NoError(t, err)

	err = bs.RunFinalize(ag.ctx)
	assert.NoError(t, err)

	assert.NotNil(t, bs.PendingConfirms[*msgID])

	// Confirm the offset
	assert.Equal(t, int64(10001), <-ag.eventPoller.offsetCommitted)

}

func TestAggregationMaskedNextSequenceMatch(t *testing.T) {
	log.SetLevel("debug")

	ag := newTestAggregator()
	defer ag.cleanup(t)

	// Generate some pin data
	member1org := newTestOrg("org1")
	member2org := newTestOrg("org2")
	member2node := newTestNode("node2", member2org)
	member2key := "0x12345"
	topic := "some-topic"
	batchID := fftypes.NewUUID()
	groupID := fftypes.NewRandB32()
	msgID := fftypes.NewUUID()
	contextUnmasked := privateContext(topic, groupID)
	initNPG := &nextPinGroupState{topic: topic, groupID: groupID}
	member1Nonce100 := initNPG.calcPinHash(member1org.DID, 100)
	member2Nonce500 := initNPG.calcPinHash(member2org.DID, 500)
	member2Nonce501 := initNPG.calcPinHash(member2org.DID, 501)

	ag.mim.On("FindIdentityForVerifier", ag.ctx, []core.IdentityType{core.IdentityTypeOrg, core.IdentityTypeCustom}, &core.VerifierRef{
		Type:  core.VerifierTypeEthAddress,
		Value: member2key,
	}).Return(member2org, nil)

	rag := ag.mdi.On("RunAsGroup", mock.Anything, mock.Anything).Maybe()
	rag.RunFn = func(a mock.Arguments) {
		rag.ReturnArguments = mock.Arguments{a[1].(func(context.Context) error)(a[0].(context.Context))}
	}

	batch := &core.Batch{
		BatchHeader: core.BatchHeader{
			ID:   batchID,
			Node: member2node.ID,
		},
		Payload: core.BatchPayload{
			Messages: []*core.Message{
				{
					Header: core.MessageHeader{
						ID:        msgID,
						Group:     groupID,
						Namespace: "ns1",
						Topics:    []string{topic},
						SignerRef: core.SignerRef{
							Author: member2org.DID,
							Key:    member2key,
						},
					},
					Pins: []string{member2Nonce500.String()},
					Data: core.DataRefs{
						{ID: fftypes.NewUUID()},
					},
				},
			},
		},
	}
	bp, _ := batch.Confirmed()

	// Get the batch
	ag.mdi.On("GetBatchByID", ag.ctx, "ns1", batchID).Return(bp, nil)
	// Look for existing nextpins
	ag.mdi.On("GetNextPinsForContext", ag.ctx, "ns1", contextUnmasked).Return([]*core.NextPin{
		{Context: contextUnmasked, Identity: member1org.DID, Hash: member1Nonce100, Nonce: 100, Sequence: 929},
		{Context: contextUnmasked, Identity: member2org.DID, Hash: member2Nonce500, Nonce: 500, Sequence: 424},
	}, nil).Once()
	// Validate the message is ok
	ag.mdm.On("GetMessageWithDataCached", ag.ctx, batch.Payload.Messages[0].Header.ID, data.CRORequirePins).Return(batch.Payload.Messages[0], core.DataArray{}, true, nil)
	ag.mdm.On("ValidateAll", ag.ctx, mock.Anything).Return(true, nil)
	ag.mdm.On("UpdateMessageStateIfCached", ag.ctx, mock.Anything, core.MessageStateConfirmed, mock.Anything, "").Return()
	// Insert the confirmed event
	ag.mdi.On("InsertEvent", ag.ctx, mock.MatchedBy(func(e *core.Event) bool {
		return *e.Reference == *msgID && e.Type == core.EventTypeMessageConfirmed
	})).Return(nil)
	// Update member2 to nonce 1
	ag.mdi.On("UpdateNextPin", ag.ctx, "ns1", mock.MatchedBy(func(seq int64) bool {
		return seq == 424
	}), mock.MatchedBy(func(update ffapi.Update) bool {
		ui, _ := update.Finalize()
		assert.Equal(t, "nonce", ui.SetOperations[0].Field)
		v, _ := ui.SetOperations[0].Value.Value()
		assert.Equal(t, int64(501), v.(int64))
		assert.Equal(t, "hash", ui.SetOperations[1].Field)
		v, _ = ui.SetOperations[1].Value.Value()
		assert.Equal(t, member2Nonce501.String(), v)
		return true
	})).Return(nil)
	// Set the pin to dispatched
	ag.mdi.On("UpdatePins", ag.ctx, "ns1", mock.Anything, mock.Anything).Return(nil)
	// Update the message
	ag.mdi.On("UpdateMessages", ag.ctx, "ns1", mock.Anything, mock.Anything).Return(nil)

	_, err := ag.processPinsEventsHandler([]core.LocallySequenced{
		&core.Pin{
			Sequence:   10001,
			Masked:     true,
			Hash:       member2Nonce500,
			Batch:      batchID,
			Index:      0,
			Signer:     member2key,
			Dispatched: false,
		},
	})
	assert.NoError(t, err)

	// Confirm the offset
	assert.Equal(t, int64(10001), <-ag.eventPoller.offsetCommitted)

}

func TestAggregationBroadcast(t *testing.T) {

	ag := newTestAggregator()
	defer ag.cleanup(t)
	bs := newBatchState(&ag.aggregator)

	// Generate some pin data
	member1org := newTestOrg("org1")
	member1key := "0x12345"
	topic := "some-topic"
	batchID := fftypes.NewUUID()
	msgID := fftypes.NewUUID()
	contextUnmasked := broadcastContext(topic)

	ag.mim.On("FindIdentityForVerifier", ag.ctx, []core.IdentityType{core.IdentityTypeOrg, core.IdentityTypeCustom}, &core.VerifierRef{
		Type:  core.VerifierTypeEthAddress,
		Value: member1key,
	}).Return(member1org, nil)

	batch := &core.Batch{
		BatchHeader: core.BatchHeader{
			ID: batchID,
		},
		Payload: core.BatchPayload{
			Messages: []*core.Message{
				{
					Header: core.MessageHeader{
						ID:        msgID,
						Topics:    []string{topic},
						Namespace: "ns1",
						SignerRef: core.SignerRef{
							Author: member1org.DID,
							Key:    member1key,
						},
					},
					Data: core.DataRefs{
						{ID: fftypes.NewUUID()},
					},
				},
			},
		},
	}
	bp, _ := batch.Confirmed()

	// Get the batch
	ag.mdi.On("GetBatchByID", ag.ctx, "ns1", batchID).Return(bp, nil)
	// Do not resolve any pins earlier
	ag.mdi.On("GetPins", mock.Anything, "ns1", mock.Anything).Return([]*core.Pin{}, nil, nil)
	// Validate the message is ok
	ag.mdm.On("GetMessageWithDataCached", ag.ctx, batch.Payload.Messages[0].Header.ID, data.CRORequirePublicBlobRefs).Return(batch.Payload.Messages[0], core.DataArray{}, true, nil)
	ag.mdm.On("ValidateAll", ag.ctx, mock.Anything).Return(true, nil)
	ag.mdm.On("UpdateMessageStateIfCached", ag.ctx, mock.Anything, core.MessageStateConfirmed, mock.Anything, "").Return()
	// Insert the confirmed event
	ag.mdi.On("InsertEvent", ag.ctx, mock.MatchedBy(func(e *core.Event) bool {
		return *e.Reference == *msgID && e.Type == core.EventTypeMessageConfirmed
	})).Return(nil)
	// Set the pin to dispatched
	ag.mdi.On("UpdatePins", ag.ctx, "ns1", mock.Anything, mock.Anything).Return(nil)
	// Update the message
	ag.mdi.On("UpdateMessages", ag.ctx, "ns1", mock.Anything, mock.Anything).Return(nil)

	err := ag.processPins(ag.ctx, []*core.Pin{
		{
			Sequence:   10001,
			Hash:       contextUnmasked,
			Batch:      batchID,
			Index:      0,
			Signer:     member1key,
			Dispatched: false,
		},
	}, bs)
	assert.NoError(t, err)

	err = bs.RunFinalize(ag.ctx)
	assert.NoError(t, err)

	// Confirm the offset
	assert.Equal(t, int64(10001), <-ag.eventPoller.offsetCommitted)

}

func TestAggregationMigratedBroadcast(t *testing.T) {

	ag := newTestAggregator()
	defer ag.cleanup(t)
	bs := newBatchState(&ag.aggregator)

	// Generate some pin data
	member1org := newTestOrg("org1")
	member1key := "0x12345"
	topic := "some-topic"
	batchID := fftypes.NewUUID()
	msgID := fftypes.NewUUID()
	contextUnmasked := broadcastContext(topic)

	ag.mim.On("FindIdentityForVerifier", ag.ctx, []core.IdentityType{core.IdentityTypeOrg, core.IdentityTypeCustom}, &core.VerifierRef{
		Type:  core.VerifierTypeEthAddress,
		Value: member1key,
	}).Return(member1org, nil)

	batch := &core.Batch{
		BatchHeader: core.BatchHeader{
			ID: batchID,
		},
		Payload: core.BatchPayload{
			Messages: []*core.Message{
				{
					Header: core.MessageHeader{
						ID:        msgID,
						Topics:    []string{topic},
						Namespace: "ns1",
						SignerRef: core.SignerRef{
							Author: member1org.DID,
							Key:    member1key,
						},
					},
					Data: core.DataRefs{
						{ID: fftypes.NewUUID()},
					},
				},
			},
		},
	}
	payloadBinary, err := json.Marshal(&batch.Payload)
	assert.NoError(t, err)
	bp := &core.BatchPersisted{
		TX:          batch.Payload.TX,
		BatchHeader: batch.BatchHeader,
		Manifest:    fftypes.JSONAnyPtr(string(payloadBinary)),
	}

	// Get the batch
	ag.mdi.On("GetBatchByID", ag.ctx, "ns1", batchID).Return(bp, nil)
	// Do not resolve any pins earlier
	ag.mdi.On("GetPins", mock.Anything, "ns1", mock.Anything).Return([]*core.Pin{}, nil, nil)
	// Validate the message is ok
	ag.mdm.On("GetMessageWithDataCached", ag.ctx, batch.Payload.Messages[0].Header.ID, data.CRORequirePublicBlobRefs).Return(batch.Payload.Messages[0], core.DataArray{}, true, nil)
	ag.mdm.On("ValidateAll", ag.ctx, mock.Anything).Return(true, nil)
	ag.mdm.On("UpdateMessageStateIfCached", ag.ctx, mock.Anything, core.MessageStateConfirmed, mock.Anything, "").Return()
	// Insert the confirmed event
	ag.mdi.On("InsertEvent", ag.ctx, mock.MatchedBy(func(e *core.Event) bool {
		return *e.Reference == *msgID && e.Type == core.EventTypeMessageConfirmed
	})).Return(nil)
	// Set the pin to dispatched
	ag.mdi.On("UpdatePins", ag.ctx, "ns1", mock.Anything, mock.Anything).Return(nil)
	// Update the message
	ag.mdi.On("UpdateMessages", ag.ctx, "ns1", mock.Anything, mock.Anything).Return(nil)

	err = ag.processPins(ag.ctx, []*core.Pin{
		{
			Sequence:   10001,
			Hash:       contextUnmasked,
			Batch:      batchID,
			Index:      0,
			Signer:     member1key,
			Dispatched: false,
		},
	}, bs)
	assert.NoError(t, err)

	err = bs.RunFinalize(ag.ctx)
	assert.NoError(t, err)

	// Confirm the offset
	assert.Equal(t, int64(10001), <-ag.eventPoller.offsetCommitted)

}

func TestAggregationMigratedBroadcastNilMessageID(t *testing.T) {

	ag := newTestAggregator()
	defer ag.cleanup(t)
	bs := newBatchState(&ag.aggregator)

	// Generate some pin data
	member1key := "0x12345"
	topic := "some-topic"
	batchID := fftypes.NewUUID()
	contextUnmasked := broadcastContext(topic)

	batch := &core.Batch{
		BatchHeader: core.BatchHeader{
			ID: batchID,
		},
		Payload: core.BatchPayload{
			Messages: []*core.Message{{
				Header: core.MessageHeader{
					Topics: fftypes.FFStringArray{"topic1"},
				},
			}},
		},
	}
	payloadBinary, err := json.Marshal(&batch.Payload)
	assert.NoError(t, err)
	bp := &core.BatchPersisted{
		TX:          batch.Payload.TX,
		BatchHeader: batch.BatchHeader,
		Manifest:    fftypes.JSONAnyPtr(string(payloadBinary)),
	}

	ag.mdi.On("GetBatchByID", ag.ctx, "ns1", batchID).Return(bp, nil)

	err = ag.processPins(ag.ctx, []*core.Pin{
		{
			Sequence:   10001,
			Hash:       contextUnmasked,
			Batch:      batchID,
			Index:      0,
			Signer:     member1key,
			Dispatched: false,
		},
	}, bs)
	assert.NoError(t, err)

	err = bs.RunFinalize(ag.ctx)
	assert.NoError(t, err)

	// Confirm the offset
	assert.Equal(t, int64(10001), <-ag.eventPoller.offsetCommitted)

}

func TestAggregationMigratedBroadcastInvalid(t *testing.T) {

	ag := newTestAggregator()
	defer ag.cleanup(t)
	bs := newBatchState(&ag.aggregator)

	// Generate some pin data
	member1key := "0x12345"
	topic := "some-topic"
	batchID := fftypes.NewUUID()
	contextUnmasked := broadcastContext(topic)

	batch := &core.Batch{
		BatchHeader: core.BatchHeader{
			ID: batchID,
		},
		Payload: core.BatchPayload{
			Messages: []*core.Message{{
				Header: core.MessageHeader{
					Topics: fftypes.FFStringArray{"topic1"},
				},
			}},
		},
	}
	bp := &core.BatchPersisted{
		TX:          batch.Payload.TX,
		BatchHeader: batch.BatchHeader,
		Manifest:    fftypes.JSONAnyPtr("{}"),
	}

	ag.mdi.On("GetBatchByID", ag.ctx, "ns1", batchID).Return(bp, nil)

	err := ag.processPins(ag.ctx, []*core.Pin{
		{
			Sequence:   10001,
			Hash:       contextUnmasked,
			Batch:      batchID,
			Index:      0,
			Signer:     member1key,
			Dispatched: false,
		},
	}, bs)
	assert.NoError(t, err)

	err = bs.RunFinalize(ag.ctx)
	assert.NoError(t, err)

	// Confirm the offset
	assert.Equal(t, int64(10001), <-ag.eventPoller.offsetCommitted)

}

func TestShutdownOnCancel(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)
	ag.mdi.On("GetOffset", mock.Anything, core.OffsetTypeAggregator, aggregatorOffsetName).Return(&core.Offset{
		Type:    core.OffsetTypeAggregator,
		Name:    aggregatorOffsetName,
		Current: 12345,
		RowID:   333333,
	}, nil)
	ag.mdi.On("GetPins", mock.Anything, "ns1", mock.Anything).Return([]*core.Pin{}, nil, nil)
	ag.start()
	assert.Equal(t, int64(12345), ag.eventPoller.pollingOffset)
	ag.eventPoller.eventNotifier.newEvents <- 12345
	ag.cancel()
	<-ag.eventPoller.closed
	<-ag.rewinder.loop1Done
	<-ag.rewinder.loop2Done
}

func TestProcessPinsDBGroupFail(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	rag := ag.mdi.On("RunAsGroup", ag.ctx, mock.Anything)
	rag.RunFn = func(a mock.Arguments) {
		rag.ReturnArguments = mock.Arguments{
			a[1].(func(context.Context) error)(a[0].(context.Context)),
		}
	}
	ag.mdi.On("GetBatchByID", ag.ctx, "ns1", mock.Anything).Return(nil, fmt.Errorf("pop"))

	_, err := ag.processPinsEventsHandler([]core.LocallySequenced{
		&core.Pin{
			Batch: fftypes.NewUUID(),
		},
	})
	assert.Regexp(t, "pop", err)
}

func TestGetPins(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	ag.mdi.On("GetPins", ag.ctx, "ns1", mock.Anything).Return([]*core.Pin{
		{Sequence: 12345},
	}, nil, nil)

	lc, err := ag.getPins(ag.ctx, database.EventQueryFactory.NewFilter(ag.ctx).Gte("sequence", 12345), 12345)
	assert.NoError(t, err)
	assert.Equal(t, int64(12345), lc[0].LocalSequence())
}

func TestProcessPinsMissingBatch(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)
	bs := newBatchState(&ag.aggregator)

	ag.mdi.On("GetBatchByID", ag.ctx, "ns1", mock.Anything).Return(nil, nil)

	err := ag.processPins(ag.ctx, []*core.Pin{
		{Sequence: 12345, Batch: fftypes.NewUUID()},
	}, bs)
	assert.NoError(t, err)

	// Confirm the offset
	assert.Equal(t, int64(12345), <-ag.eventPoller.offsetCommitted)

}

func TestProcessPinsMissingNoMsg(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)
	bs := newBatchState(&ag.aggregator)

	batch := &core.Batch{
		BatchHeader: core.BatchHeader{
			ID: fftypes.NewUUID(),
		},
		Payload: core.BatchPayload{
			Messages: []*core.Message{
				{Header: core.MessageHeader{ID: fftypes.NewUUID()}},
			},
		},
	}
	bp, _ := batch.Confirmed()

	ag.mdi.On("GetBatchByID", ag.ctx, "ns1", mock.Anything).Return(bp, nil)

	err := ag.processPins(ag.ctx, []*core.Pin{
		{Sequence: 12345, Batch: fftypes.NewUUID(), Index: 25},
	}, bs)
	assert.NoError(t, err)

	// Confirm the offset
	assert.Equal(t, int64(12345), <-ag.eventPoller.offsetCommitted)

}

func TestProcessPinsBadMsgHeader(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)
	bs := newBatchState(&ag.aggregator)

	batch := &core.Batch{
		BatchHeader: core.BatchHeader{
			ID: fftypes.NewUUID(),
		},
		Payload: core.BatchPayload{
			Messages: []*core.Message{
				{Header: core.MessageHeader{
					ID:     nil, /* missing */
					Topics: fftypes.FFStringArray{"topic1"},
				}},
			},
		},
	}
	bp, _ := batch.Confirmed()

	ag.mdi.On("GetBatchByID", ag.ctx, "ns1", mock.Anything).Return(bp, nil)

	err := ag.processPins(ag.ctx, []*core.Pin{
		{Sequence: 12345, Batch: fftypes.NewUUID(), Index: 0},
	}, bs)
	assert.NoError(t, err)

	// Confirm the offset
	assert.Equal(t, int64(12345), <-ag.eventPoller.offsetCommitted)

}

func TestProcessSkipDupMsg(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)
	bs := newBatchState(&ag.aggregator)
	org1 := newTestOrg("org1")

	batchID := fftypes.NewUUID()
	batch := &core.Batch{
		BatchHeader: core.BatchHeader{
			ID: batchID,
		},
		Payload: core.BatchPayload{
			Messages: []*core.Message{
				{Header: core.MessageHeader{
					ID:     fftypes.NewUUID(),
					Topics: fftypes.FFStringArray{"topic1", "topic2"},
					SignerRef: core.SignerRef{
						Author: org1.DID,
						Key:    "key1",
					},
				}},
			},
		},
	}
	bp, _ := batch.Confirmed()

	ag.mdi.On("GetBatchByID", ag.ctx, "ns1", mock.Anything).Return(bp, nil).Once()
	ag.mdi.On("GetPins", mock.Anything, "ns1", mock.Anything).Return([]*core.Pin{
		{Sequence: 1111}, // blocks the context
	}, nil, nil)

	ag.mdm.On("GetMessageWithDataCached", ag.ctx, mock.Anything, data.CRORequirePublicBlobRefs).Return(batch.Payload.Messages[0], nil, true, nil)

	ag.mim.On("FindIdentityForVerifier", ag.ctx, mock.Anything, mock.Anything).Return(org1, nil)

	err := ag.processPins(ag.ctx, []*core.Pin{
		{Sequence: 12345, Batch: batchID, Index: 0, Hash: fftypes.NewRandB32(), Signer: "key1"},
		{Sequence: 12345, Batch: batchID, Index: 1, Hash: fftypes.NewRandB32(), Signer: "key1"},
	}, bs)
	assert.NoError(t, err)

	// Confirm the offset
	assert.Equal(t, int64(12345), <-ag.eventPoller.offsetCommitted)

}

func TestProcessMsgFailGetPins(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)
	bs := newBatchState(&ag.aggregator)
	org1 := newTestOrg("org1")

	batchID := fftypes.NewUUID()
	batch := &core.Batch{
		BatchHeader: core.BatchHeader{
			ID: batchID,
		},
		Payload: core.BatchPayload{
			Messages: []*core.Message{
				{Header: core.MessageHeader{
					ID:     fftypes.NewUUID(),
					Topics: fftypes.FFStringArray{"topic1"},
					SignerRef: core.SignerRef{
						Author: org1.DID,
						Key:    "key1",
					},
				}},
			},
		},
	}
	bp, _ := batch.Confirmed()

	ag.mdi.On("GetBatchByID", ag.ctx, "ns1", mock.Anything).Return(bp, nil).Once()
	ag.mdi.On("GetPins", mock.Anything, "ns1", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	ag.mdm.On("GetMessageWithDataCached", ag.ctx, mock.Anything, data.CRORequirePublicBlobRefs).Return(batch.Payload.Messages[0], nil, true, nil)

	ag.mim.On("FindIdentityForVerifier", ag.ctx, mock.Anything, mock.Anything).Return(org1, nil)

	err := ag.processPins(ag.ctx, []*core.Pin{
		{Sequence: 12345, Batch: batchID, Index: 0, Hash: fftypes.NewRandB32(), Signer: "key1"},
	}, bs)
	assert.EqualError(t, err, "pop")
}

func TestProcessMsgFailData(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	ag.mdm.On("GetMessageWithDataCached", ag.ctx, mock.Anything, data.CRORequirePins).Return(nil, nil, false, fmt.Errorf("pop"))

	err := ag.processMessage(ag.ctx, &core.BatchManifest{}, &core.Pin{Masked: true, Sequence: 12345}, 10, &core.MessageManifestEntry{}, &core.BatchPersisted{}, nil)
	assert.Regexp(t, "pop", err)

}

func TestProcessMsgNoData(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	ag.mdm.On("GetMessageWithDataCached", ag.ctx, mock.Anything, data.CRORequirePins).Return(nil, nil, false, nil)

	err := ag.processMessage(ag.ctx, &core.BatchManifest{}, &core.Pin{Masked: true, Sequence: 12345}, 10, &core.MessageManifestEntry{}, &core.BatchPersisted{}, nil)
	assert.NoError(t, err)

}

func TestProcessMsgFailMissingData(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	ag.mdm.On("GetMessageWithDataCached", ag.ctx, mock.Anything, data.CRORequirePins).Return(&core.Message{
		Header: core.MessageHeader{ID: fftypes.NewUUID()},
	}, nil, false, nil)

	err := ag.processMessage(ag.ctx, &core.BatchManifest{},
		&core.Pin{Masked: true, Sequence: 12345},
		10,
		&core.MessageManifestEntry{},
		&core.BatchPersisted{},
		nil)
	assert.NoError(t, err)

}

func TestProcessMsgFailMissingGroup(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)
	org1 := newTestOrg("org1")

	ag.mdm.On("GetMessageWithDataCached", ag.ctx, mock.Anything, data.CRORequirePins).Return(&core.Message{
		Header: core.MessageHeader{
			ID: fftypes.NewUUID(),
			SignerRef: core.SignerRef{
				Author: org1.DID,
				Key:    "key1",
			},
		},
	}, nil, true, nil)

	ag.mim.On("FindIdentityForVerifier", ag.ctx, mock.Anything, mock.Anything).Return(org1, nil)

	err := ag.processMessage(ag.ctx, &core.BatchManifest{},
		&core.Pin{Masked: true, Sequence: 12345, Signer: "key1"},
		10, &core.MessageManifestEntry{},
		&core.BatchPersisted{},
		&batchState{})
	assert.NoError(t, err)

}

func TestProcessMsgWrongSigner(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	ag.mdm.On("GetMessageWithDataCached", ag.ctx, mock.Anything, data.CRORequirePins).Return(&core.Message{
		Header: core.MessageHeader{
			ID: fftypes.NewUUID(),
			SignerRef: core.SignerRef{
				Key: "key1",
			},
		},
		BatchID: fftypes.NewUUID(),
	}, nil, true, nil)

	err := ag.processMessage(ag.ctx, &core.BatchManifest{},
		&core.Pin{Masked: true, Sequence: 12345, Signer: "key2"},
		10, &core.MessageManifestEntry{},
		&core.BatchPersisted{},
		&batchState{})
	assert.NoError(t, err)

}

func TestProcessMsgFailFindIdentity(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)
	org1 := newTestOrg("org1")

	ag.mdm.On("GetMessageWithDataCached", ag.ctx, mock.Anything, data.CRORequirePins).Return(&core.Message{
		Header: core.MessageHeader{
			ID: fftypes.NewUUID(),
			SignerRef: core.SignerRef{
				Author: org1.DID,
				Key:    "key1",
			},
		},
	}, nil, true, nil)

	ag.mim.On("FindIdentityForVerifier", ag.ctx, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))

	err := ag.processMessage(ag.ctx, &core.BatchManifest{},
		&core.Pin{Masked: true, Sequence: 12345, Signer: "key1"},
		10, &core.MessageManifestEntry{},
		&core.BatchPersisted{},
		&batchState{})
	assert.EqualError(t, err, "pop")

}

func TestProcessMsgFailBadPin(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)
	org1 := newTestOrg("org1")

	msg := &core.Message{
		Header: core.MessageHeader{
			ID:     fftypes.NewUUID(),
			Group:  fftypes.NewRandB32(),
			Topics: fftypes.FFStringArray{"topic1"},
			SignerRef: core.SignerRef{
				Author: org1.DID,
				Key:    "key1",
			},
		},
		Hash: fftypes.NewRandB32(),
		Pins: fftypes.FFStringArray{"!Wrong"},
	}

	ag.mdm.On("GetMessageWithDataCached", ag.ctx, mock.Anything, data.CRORequirePins).Return(msg, nil, true, nil)

	ag.mim.On("FindIdentityForVerifier", ag.ctx, mock.Anything, mock.Anything).Return(org1, nil)

	err := ag.processMessage(ag.ctx, &core.BatchManifest{},
		&core.Pin{Masked: true, Sequence: 12345, Signer: "key1"},
		10,
		&core.MessageManifestEntry{
			MessageRef: core.MessageRef{
				ID:   msg.Header.ID,
				Hash: msg.Hash,
			},
			Topics: len(msg.Header.Topics),
		}, &core.BatchPersisted{}, newBatchState(&ag.aggregator))
	assert.NoError(t, err)

}

func TestProcessMsgFailGetNextPins(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)
	org1 := newTestOrg("org1")

	ag.mdi.On("GetNextPinsForContext", ag.ctx, "ns1", mock.Anything).Return(nil, fmt.Errorf("pop"))

	msg := &core.Message{
		Header: core.MessageHeader{
			ID:     fftypes.NewUUID(),
			Group:  fftypes.NewRandB32(),
			Topics: fftypes.FFStringArray{"topic1"},
			SignerRef: core.SignerRef{
				Author: org1.DID,
				Key:    "key1",
			},
		},
		Pins: fftypes.FFStringArray{fftypes.NewRandB32().String()},
	}

	ag.mdm.On("GetMessageWithDataCached", ag.ctx, mock.Anything, data.CRORequirePins).Return(msg, nil, true, nil)

	ag.mim.On("FindIdentityForVerifier", ag.ctx, mock.Anything, mock.Anything).Return(org1, nil)

	err := ag.processMessage(ag.ctx, &core.BatchManifest{},
		&core.Pin{Masked: true, Sequence: 12345, Signer: "key1"},
		10,
		&core.MessageManifestEntry{
			MessageRef: core.MessageRef{
				ID:   msg.Header.ID,
				Hash: msg.Hash,
			},
			Topics: len(msg.Header.Topics),
		}, &core.BatchPersisted{}, newBatchState(&ag.aggregator))
	assert.EqualError(t, err, "pop")

}

func TestProcessMsgFailPinUpdate(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)
	bs := newBatchState(&ag.aggregator)
	pin := fftypes.NewRandB32()
	org1 := newTestOrg("org1")

	groupID := fftypes.NewRandB32()
	msg := &core.Message{
		Header: core.MessageHeader{
			ID:        fftypes.NewUUID(),
			Group:     groupID,
			Topics:    fftypes.FFStringArray{"topic1"},
			Namespace: "ns1",
			SignerRef: core.SignerRef{
				Author: org1.DID,
				Key:    "0x12345",
			},
		},
		Pins: fftypes.FFStringArray{pin.String()},
	}

	ag.mim.On("FindIdentityForVerifier", ag.ctx, []core.IdentityType{core.IdentityTypeOrg, core.IdentityTypeCustom}, &core.VerifierRef{
		Type:  core.VerifierTypeEthAddress,
		Value: "0x12345",
	}).Return(org1, nil)
	ag.mdi.On("GetNextPinsForContext", ag.ctx, "ns1", mock.Anything).Return([]*core.NextPin{
		{Context: fftypes.NewRandB32(), Hash: pin, Identity: org1.DID},
	}, nil)
	ag.mdm.On("GetMessageWithDataCached", ag.ctx, mock.Anything, data.CRORequirePins).Return(msg, nil, true, nil)
	ag.mdi.On("InsertEvent", ag.ctx, mock.Anything).Return(nil)
	ag.mdi.On("UpdateNextPin", ag.ctx, "ns1", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))

	err := ag.processMessage(ag.ctx, &core.BatchManifest{
		ID: fftypes.NewUUID(),
	}, &core.Pin{Masked: true, Sequence: 12345, Signer: "0x12345"}, 10, &core.MessageManifestEntry{
		MessageRef: core.MessageRef{
			ID:   msg.Header.ID,
			Hash: msg.Hash,
		},
		Topics: len(msg.Header.Topics),
	}, &core.BatchPersisted{}, bs)
	assert.NoError(t, err)

	err = bs.RunFinalize(ag.ctx)
	assert.EqualError(t, err, "pop")

}

func TestProcessMsgGapFill(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)
	bs := newBatchState(&ag.aggregator)
	pin := fftypes.NewRandB32()
	org1 := newTestOrg("org1")

	groupID := fftypes.NewRandB32()
	msg := &core.Message{
		Header: core.MessageHeader{
			ID:        fftypes.NewUUID(),
			Group:     groupID,
			Topics:    fftypes.FFStringArray{"topic1"},
			Namespace: "ns1",
			SignerRef: core.SignerRef{
				Author: org1.DID,
				Key:    "0x12345",
			},
			CID: fftypes.NewUUID(),
			Tag: core.SystemTagGapFill,
		},
		Pins: fftypes.FFStringArray{pin.String()},
	}

	ag.mim.On("FindIdentityForVerifier", ag.ctx, []core.IdentityType{core.IdentityTypeOrg, core.IdentityTypeCustom}, &core.VerifierRef{
		Type:  core.VerifierTypeEthAddress,
		Value: "0x12345",
	}).Return(org1, nil)
	ag.mdi.On("GetNextPinsForContext", ag.ctx, "ns1", mock.Anything).Return([]*core.NextPin{
		{Context: fftypes.NewRandB32(), Hash: pin, Identity: org1.DID},
	}, nil)
	ag.mdm.On("GetMessageWithDataCached", ag.ctx, mock.Anything, data.CRORequirePins).Return(msg, nil, true, nil)
	ag.mdm.On("UpdateMessageStateIfCached", ag.ctx, msg.Header.ID, core.MessageStateConfirmed, mock.Anything, "").Once().Return()
	ag.mdm.On("UpdateMessageStateIfCached", ag.ctx, msg.Header.CID, core.MessageStateCancelled, mock.Anything, "").Once().Return()

	ag.mdi.On("InsertEvent", ag.ctx, mock.Anything).Return(nil)
	ag.mdi.On("UpdateNextPin", ag.ctx, "ns1", mock.Anything, mock.Anything).Return(nil)
	ag.mdi.On("UpdatePins", ag.ctx, "ns1", mock.Anything, mock.Anything).Return(nil)

	ag.mdi.On("UpdateMessages", ag.ctx, "ns1", mock.Anything, mock.Anything).Twice().Return(nil)

	err := ag.processMessage(ag.ctx, &core.BatchManifest{
		ID: fftypes.NewUUID(),
	}, &core.Pin{Masked: true, Sequence: 12345, Signer: "0x12345"}, 10, &core.MessageManifestEntry{
		MessageRef: core.MessageRef{
			ID:   msg.Header.ID,
			Hash: msg.Hash,
		},
		Topics: len(msg.Header.Topics),
	}, &core.BatchPersisted{}, bs)
	assert.NoError(t, err)

	err = bs.RunFinalize(ag.ctx)
	assert.NoError(t, err)

}

func TestCheckMaskedContextReadyMismatchedAuthor(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)
	pin := fftypes.NewRandB32()

	ag.mdi.On("GetNextPinsForContext", ag.ctx, "ns1", mock.Anything).Return([]*core.NextPin{
		{Context: fftypes.NewRandB32(), Hash: pin},
	}, nil)

	bs := newBatchState(&ag.aggregator)
	_, err := bs.checkMaskedContextReady(ag.ctx, &core.Message{
		Header: core.MessageHeader{
			ID:     fftypes.NewUUID(),
			Group:  fftypes.NewRandB32(),
			Tag:    core.SystemTagDefineDatatype,
			Topics: fftypes.FFStringArray{"topic1"},
			SignerRef: core.SignerRef{
				Author: "author1",
				Key:    "0x12345",
			},
		},
	}, nil, "topic1", 12345, fftypes.NewRandB32(), "12345")
	assert.NoError(t, err)

}

func TestAttemptContextInitGetGroupByIDFail(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	signer := core.SignerRef{
		Author: "author1",
		Key:    "0x12345",
	}
	batch := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			SignerRef: signer,
			Node:      fftypes.NewUUID(),
		},
	}
	creator := &core.Member{
		Identity: batch.Author,
		Node:     batch.Node,
	}

	ag.mpm.On("ResolveInitGroup", ag.ctx, mock.Anything, creator).Return(nil, fmt.Errorf("pop"))

	bs := newBatchState(&ag.aggregator)
	_, err := bs.attemptContextInit(ag.ctx, &core.Message{
		Header: core.MessageHeader{
			ID:        fftypes.NewUUID(),
			Group:     fftypes.NewRandB32(),
			SignerRef: signer,
		},
	}, batch, "topic1", 12345, fftypes.NewRandB32(), fftypes.NewRandB32())
	assert.EqualError(t, err, "pop")

}

func TestAttemptContextInitGroupNotFound(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	signer := core.SignerRef{
		Author: "author1",
		Key:    "0x12345",
	}
	batch := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			SignerRef: signer,
			Node:      fftypes.NewUUID(),
		},
	}
	creator := &core.Member{
		Identity: batch.Author,
		Node:     batch.Node,
	}

	ag.mpm.On("ResolveInitGroup", ag.ctx, mock.Anything, creator).Return(nil, nil)

	bs := newBatchState(&ag.aggregator)
	_, err := bs.attemptContextInit(ag.ctx, &core.Message{
		Header: core.MessageHeader{
			ID:        fftypes.NewUUID(),
			Group:     fftypes.NewRandB32(),
			SignerRef: signer,
		},
	}, batch, "topic1", 12345, fftypes.NewRandB32(), fftypes.NewRandB32())
	assert.NoError(t, err)

}

func TestAttemptContextInitAuthorMismatch(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	signer := core.SignerRef{
		Author: "author1",
		Key:    "0x12345",
	}
	batch := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			SignerRef: signer,
			Node:      fftypes.NewUUID(),
		},
	}
	creator := &core.Member{
		Identity: batch.Author,
		Node:     batch.Node,
	}

	groupID := fftypes.NewRandB32()
	initNPG := &nextPinGroupState{topic: "topic1", groupID: groupID}
	zeroHash := initNPG.calcPinHash("author2", 0)
	ag.mpm.On("ResolveInitGroup", ag.ctx, mock.Anything, creator).Return(&core.Group{
		GroupIdentity: core.GroupIdentity{
			Members: core.Members{
				{Identity: "author2"},
			},
		},
	}, nil)

	bs := newBatchState(&ag.aggregator)
	_, err := bs.attemptContextInit(ag.ctx, &core.Message{
		Header: core.MessageHeader{
			ID:        fftypes.NewUUID(),
			Group:     groupID,
			SignerRef: signer,
		},
	}, batch, "topic1", 12345, fftypes.NewRandB32(), zeroHash)
	assert.NoError(t, err)

}

func TestAttemptContextInitNoMatch(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	signer := core.SignerRef{
		Author: "author1",
		Key:    "0x12345",
	}
	batch := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			SignerRef: signer,
			Node:      fftypes.NewUUID(),
		},
	}
	creator := &core.Member{
		Identity: batch.Author,
		Node:     batch.Node,
	}

	groupID := fftypes.NewRandB32()
	ag.mpm.On("ResolveInitGroup", ag.ctx, mock.Anything, creator).Return(&core.Group{
		GroupIdentity: core.GroupIdentity{
			Members: core.Members{
				{Identity: "author2"},
			},
		},
	}, nil)

	bs := newBatchState(&ag.aggregator)
	_, err := bs.attemptContextInit(ag.ctx, &core.Message{
		Header: core.MessageHeader{
			ID:        fftypes.NewUUID(),
			Group:     groupID,
			SignerRef: signer,
		},
	}, batch, "topic1", 12345, fftypes.NewRandB32(), fftypes.NewRandB32())
	assert.NoError(t, err)

}

func TestAttemptContextInitGetPinsFail(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	signer := core.SignerRef{
		Author: "author1",
		Key:    "0x12345",
	}
	batch := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			SignerRef: signer,
			Node:      fftypes.NewUUID(),
		},
	}
	creator := &core.Member{
		Identity: batch.Author,
		Node:     batch.Node,
	}

	groupID := fftypes.NewRandB32()
	initNPG := &nextPinGroupState{topic: "topic1", groupID: groupID}
	zeroHash := initNPG.calcPinHash("author1", 0)
	ag.mpm.On("ResolveInitGroup", ag.ctx, mock.Anything, creator).Return(&core.Group{
		GroupIdentity: core.GroupIdentity{
			Members: core.Members{
				{Identity: "author1"},
			},
		},
	}, nil)
	ag.mdi.On("GetPins", ag.ctx, "ns1", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	bs := newBatchState(&ag.aggregator)
	_, err := bs.attemptContextInit(ag.ctx, &core.Message{
		Header: core.MessageHeader{
			ID:        fftypes.NewUUID(),
			Group:     groupID,
			SignerRef: signer,
		},
	}, batch, "topic1", 12345, fftypes.NewRandB32(), zeroHash)
	assert.EqualError(t, err, "pop")

}

func TestAttemptContextInitGetPinsBlocked(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	signer := core.SignerRef{
		Author: "author1",
		Key:    "0x12345",
	}
	batch := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			SignerRef: signer,
			Node:      fftypes.NewUUID(),
		},
	}
	creator := &core.Member{
		Identity: batch.Author,
		Node:     batch.Node,
	}

	groupID := fftypes.NewRandB32()
	initNPG := &nextPinGroupState{topic: "topic1", groupID: groupID}
	zeroHash := initNPG.calcPinHash("author1", 0)
	ag.mpm.On("ResolveInitGroup", ag.ctx, mock.Anything, creator).Return(&core.Group{
		GroupIdentity: core.GroupIdentity{
			Members: core.Members{
				{Identity: "author1"},
			},
		},
	}, nil)
	ag.mdi.On("GetPins", ag.ctx, "ns1", mock.Anything).Return([]*core.Pin{
		{Sequence: 12345},
	}, nil, nil)

	bs := newBatchState(&ag.aggregator)
	np, err := bs.attemptContextInit(ag.ctx, &core.Message{
		Header: core.MessageHeader{
			ID:        fftypes.NewUUID(),
			Group:     groupID,
			SignerRef: signer,
		},
	}, batch, "topic1", 12345, fftypes.NewRandB32(), zeroHash)
	assert.NoError(t, err)
	assert.Nil(t, np)

}

func TestAttemptContextInitInsertPinsFail(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	signer := core.SignerRef{
		Author: "author1",
		Key:    "0x12345",
	}
	batch := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			SignerRef: signer,
			Node:      fftypes.NewUUID(),
		},
	}
	creator := &core.Member{
		Identity: batch.Author,
		Node:     batch.Node,
	}

	groupID := fftypes.NewRandB32()
	initNPG := &nextPinGroupState{topic: "topic1", groupID: groupID}
	zeroHash := initNPG.calcPinHash("author1", 0)
	ag.mpm.On("ResolveInitGroup", ag.ctx, mock.Anything, creator).Return(&core.Group{
		GroupIdentity: core.GroupIdentity{
			Members: core.Members{
				{Identity: "author1"},
			},
		},
	}, nil)
	ag.mdi.On("GetPins", ag.ctx, "ns1", mock.Anything).Return([]*core.Pin{}, nil, nil)
	ag.mdi.On("InsertNextPin", ag.ctx, mock.Anything).Return(fmt.Errorf("pop"))

	bs := newBatchState(&ag.aggregator)
	np, err := bs.attemptContextInit(ag.ctx, &core.Message{
		Header: core.MessageHeader{
			ID:        fftypes.NewUUID(),
			Group:     groupID,
			SignerRef: signer,
		},
	}, batch, "topic1", 12345, fftypes.NewRandB32(), zeroHash)
	assert.NoError(t, err)
	assert.NotNil(t, np)
	err = bs.RunFinalize(ag.ctx)
	assert.EqualError(t, err, "pop")

}

func TestReadyForDispatchFailValidateData(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	org1 := newTestOrg("org1")
	ag.mdm.On("ValidateAll", ag.ctx, mock.Anything).Return(false, fmt.Errorf("pop"))

	_, _, err := ag.readyForDispatch(ag.ctx, &core.Message{
		Header: core.MessageHeader{ID: fftypes.NewUUID(), SignerRef: core.SignerRef{Key: "0x12345", Author: org1.DID}},
		Data: core.DataRefs{
			{ID: fftypes.NewUUID()},
		},
	}, core.DataArray{}, nil, &batchState{})
	assert.EqualError(t, err, "pop")

}

func TestReadyForDispatchMissingBlobs(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	blobHash := fftypes.NewRandB32()

	org1 := newTestOrg("org1")

	ag.mdi.On("GetBlobs", ag.ctx, mock.Anything, mock.Anything).Return([]*core.Blob{}, nil, nil)

	action, _, err := ag.readyForDispatch(ag.ctx, &core.Message{
		Header: core.MessageHeader{ID: fftypes.NewUUID(), SignerRef: core.SignerRef{Key: "0x12345", Author: org1.DID}},
	}, core.DataArray{
		{ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32(), Blob: &core.BlobRef{
			Hash:   blobHash,
			Public: "public-ref",
		}},
	}, nil, &batchState{})
	assert.NoError(t, err)
	assert.Equal(t, core.ActionWait, action)

}

func TestReadyForDispatchBlobsError(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	blobHash := fftypes.NewRandB32()

	org1 := newTestOrg("org1")

	ag.mdi.On("GetBlobs", ag.ctx, mock.Anything, mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	action, _, err := ag.readyForDispatch(ag.ctx, &core.Message{
		Header: core.MessageHeader{ID: fftypes.NewUUID(), SignerRef: core.SignerRef{Key: "0x12345", Author: org1.DID}},
	}, core.DataArray{
		{ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32(), Blob: &core.BlobRef{
			Hash:   blobHash,
			Public: "public-ref",
		}},
	}, nil, &batchState{})
	assert.EqualError(t, err, "pop")
	assert.Equal(t, core.ActionRetry, action)

}

func TestReadyForDispatchMissingTransfers(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	org1 := newTestOrg("org1")
	ag.mdi.On("GetTokenTransfers", ag.ctx, "ns1", mock.Anything).Return([]*core.TokenTransfer{}, nil, nil)

	msg := &core.Message{
		Header: core.MessageHeader{
			ID:   fftypes.NewUUID(),
			Type: core.MessageTypeDeprecatedTransferBroadcast,
			SignerRef: core.SignerRef{
				Author: org1.DID,
				Key:    "0x12345",
			},
		},
	}
	msg.Hash = msg.Header.Hash()
	action, _, err := ag.readyForDispatch(ag.ctx, msg, core.DataArray{}, nil, &batchState{})
	assert.NoError(t, err)
	assert.Equal(t, core.ActionWait, action)

}

func TestReadyForDispatchGetTransfersFail(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	org1 := newTestOrg("org1")

	ag.mdi.On("GetTokenTransfers", ag.ctx, "ns1", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	msg := &core.Message{
		Header: core.MessageHeader{
			ID:        fftypes.NewUUID(),
			Type:      core.MessageTypeDeprecatedTransferBroadcast,
			SignerRef: core.SignerRef{Key: "0x12345", Author: org1.DID},
		},
	}
	msg.Hash = msg.Header.Hash()
	action, _, err := ag.readyForDispatch(ag.ctx, msg, core.DataArray{}, nil, &batchState{})
	assert.EqualError(t, err, "pop")
	assert.Equal(t, core.ActionRetry, action)

}

func TestReadyForDispatchTransferMismatch(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	org1 := newTestOrg("org1")

	msg := &core.Message{
		Header: core.MessageHeader{
			ID:        fftypes.NewUUID(),
			Type:      core.MessageTypeDeprecatedTransferBroadcast,
			SignerRef: core.SignerRef{Key: "0x12345", Author: org1.DID},
		},
	}
	msg.Hash = msg.Header.Hash()

	transfers := []*core.TokenTransfer{{
		Message:     msg.Header.ID,
		MessageHash: fftypes.NewRandB32(),
	}}

	ag.mdi.On("GetTokenTransfers", ag.ctx, "ns1", mock.Anything).Return(transfers, nil, nil)

	action, _, err := ag.readyForDispatch(ag.ctx, msg, core.DataArray{}, nil, &batchState{})
	assert.NoError(t, err)
	assert.Equal(t, core.ActionWait, action)

}

func TestReadyForDispatchGetApprovalsFail(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	org1 := newTestOrg("org1")

	ag.mdi.On("GetTokenApprovals", ag.ctx, "ns1", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	msg := &core.Message{
		Header: core.MessageHeader{
			ID:        fftypes.NewUUID(),
			Type:      core.MessageTypeDeprecatedApprovalBroadcast,
			SignerRef: core.SignerRef{Key: "0x12345", Author: org1.DID},
		},
	}
	msg.Hash = msg.Header.Hash()
	action, _, err := ag.readyForDispatch(ag.ctx, msg, core.DataArray{}, nil, &batchState{})
	assert.EqualError(t, err, "pop")
	assert.Equal(t, core.ActionRetry, action)

}

func TestReadyForDispatchGetApprovalsMissing(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	org1 := newTestOrg("org1")

	ag.mdi.On("GetTokenApprovals", ag.ctx, "ns1", mock.Anything).Return(nil, nil, nil)

	msg := &core.Message{
		Header: core.MessageHeader{
			ID:        fftypes.NewUUID(),
			Type:      core.MessageTypeDeprecatedApprovalBroadcast,
			SignerRef: core.SignerRef{Key: "0x12345", Author: org1.DID},
		},
	}
	msg.Hash = msg.Header.Hash()
	action, _, err := ag.readyForDispatch(ag.ctx, msg, core.DataArray{}, nil, &batchState{})
	assert.NoError(t, err)
	assert.Equal(t, core.ActionWait, action)

}

func TestReadyForDispatchApprovalMismatch(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	org1 := newTestOrg("org1")

	msg := &core.Message{
		Header: core.MessageHeader{
			ID:        fftypes.NewUUID(),
			Type:      core.MessageTypeDeprecatedApprovalBroadcast,
			SignerRef: core.SignerRef{Key: "0x12345", Author: org1.DID},
		},
	}
	msg.Hash = msg.Header.Hash()

	approvals := []*core.TokenApproval{{
		Message:     msg.Header.ID,
		MessageHash: fftypes.NewRandB32(),
	}}

	ag.mdi.On("GetTokenApprovals", ag.ctx, "ns1", mock.Anything).Return(approvals, nil, nil)

	action, _, err := ag.readyForDispatch(ag.ctx, msg, core.DataArray{}, nil, &batchState{})
	assert.NoError(t, err)
	assert.Equal(t, core.ActionWait, action)

}

func TestDefinitionBroadcastActionRejectFailUpdate(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)
	bs := newBatchState(&ag.aggregator)
	org1 := newTestOrg("org1")

	msg := &core.Message{
		Header: core.MessageHeader{
			Type:      core.MessageTypeDefinition,
			ID:        fftypes.NewUUID(),
			Namespace: "any",
			SignerRef: core.SignerRef{Key: "0x12345", Author: org1.DID},
			Tag:       core.SystemTagDefineDatatype,
			Topics:    fftypes.FFStringArray{"topic1"},
		},
		Data: core.DataRefs{
			{ID: fftypes.NewUUID()},
		},
	}
	customCorrelator := fftypes.NewUUID()

	ag.mdi.On("InsertEvent", ag.ctx, mock.MatchedBy(func(event *core.Event) bool {
		return event.Type == core.EventTypeMessageRejected && event.Correlator.Equals(customCorrelator)
	})).Return(nil)
	ag.mdm.On("UpdateMessageStateIfCached", ag.ctx, msg.Header.ID, core.MessageStateRejected, mock.Anything, "reject-reason").Return()
	ag.mdi.On("UpdateMessages", ag.ctx, "ns1", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))

	newState := ag.completeDispatch(core.ActionReject, customCorrelator, msg, nil, bs)
	assert.Equal(t, core.MessageStateRejected, newState)
	msg.RejectReason = "reject-reason"

	bs.markMessageDispatched(fftypes.NewUUID(), msg, 0, newState)
	err := bs.RunFinalize(ag.ctx)
	assert.EqualError(t, err, "pop")
}

func TestDispatchBroadcastQueuesLaterDispatch(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)
	bs := newBatchState(&ag.aggregator)

	msg1, msg2, org1, manifest := newTestManifest(core.MessageTypeDefinition, nil)

	ag.mim.On("FindIdentityForVerifier", ag.ctx, mock.Anything, mock.Anything).Return(org1, nil)

	data1 := core.DataArray{}
	data2 := core.DataArray{}
	ag.mdm.On("GetMessageWithDataCached", ag.ctx, msg1.Header.ID, data.CRORequirePublicBlobRefs).Return(msg1, data1, true, nil).Once()
	ag.mdm.On("GetMessageWithDataCached", ag.ctx, msg2.Header.ID, data.CRORequirePublicBlobRefs).Return(msg2, data2, true, nil).Once()

	ag.mdi.On("GetPins", ag.ctx, "ns1", mock.Anything).Return([]*core.Pin{}, nil, nil)

	result1 := definitions.HandlerResult{Action: core.ActionConfirm}
	ag.mdh.On("HandleDefinitionBroadcast", ag.ctx, mock.Anything, msg1, data1, manifest.TX.ID).Return(result1, nil).Once()
	result2 := definitions.HandlerResult{Action: core.ActionWait}
	ag.mdh.On("HandleDefinitionBroadcast", ag.ctx, mock.Anything, msg2, data2, manifest.TX.ID).Return(result2, nil).Once()

	// First message should dispatch
	pin1 := &core.Pin{Sequence: 12345, Signer: msg1.Header.Key}
	err := ag.processMessage(ag.ctx, manifest, pin1, 0, manifest.Messages[0], &core.BatchPersisted{}, bs)
	assert.NoError(t, err)

	// Second message should block
	pin2 := &core.Pin{Sequence: 12346, Signer: msg2.Header.Key}
	err = ag.processMessage(ag.ctx, manifest, pin2, 0, manifest.Messages[1], &core.BatchPersisted{}, bs)
	assert.NoError(t, err)

	assert.Len(t, bs.dispatchedMessages, 1)
	assert.Equal(t, msg1.Header.ID, bs.dispatchedMessages[0].msgID)

	msgContext := broadcastContext(msg2.Header.Topics[0])
	assert.Equal(t, pin2.Sequence, bs.unmaskedContexts[*msgContext].blockedBy)

}

func TestDispatchPrivateQueuesLaterDispatch(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)
	bs := newBatchState(&ag.aggregator)

	groupID := fftypes.NewRandB32()
	msg1, msg2, org1, manifest := newTestManifest(core.MessageTypePrivate, groupID)

	ag.mim.On("FindIdentityForVerifier", ag.ctx, mock.Anything, mock.Anything).Return(org1, nil)

	data1 := core.DataArray{}
	data2 := core.DataArray{{Namespace: "ns1", Blob: &core.BlobRef{Hash: fftypes.NewRandB32()}}}
	ag.mdm.On("GetMessageWithDataCached", ag.ctx, msg1.Header.ID, data.CRORequirePins).Return(msg1, data1, true, nil).Once()
	ag.mdm.On("ValidateAll", ag.ctx, data1).Return(true, nil)
	ag.mdm.On("GetMessageWithDataCached", ag.ctx, msg2.Header.ID, data.CRORequirePins).Return(msg2, data2, true, nil).Once()

	initNPG := &nextPinGroupState{topic: "topic1", groupID: groupID}
	member1NonceOne := initNPG.calcPinHash("did:firefly:org/org1", 1)
	member1NonceTwo := initNPG.calcPinHash("did:firefly:org/org1", 2)
	context := broadcastContext("topic1")

	ag.mdi.On("GetNextPinsForContext", ag.ctx, "ns1", mock.Anything).Return([]*core.NextPin{
		{Context: context, Nonce: 1 /* match member1NonceOne */, Identity: org1.DID, Hash: member1NonceOne},
	}, nil).Once()
	ag.mdi.On("GetBlobs", ag.ctx, "ns1", mock.Anything).Return([]*core.Blob{}, nil, nil)

	msg1.Pins = fftypes.FFStringArray{member1NonceOne.String()}
	msg2.Pins = fftypes.FFStringArray{member1NonceTwo.String()}

	// First message should dispatch
	pin1 := &core.Pin{Masked: true, Sequence: 12345, Signer: msg1.Header.Key}
	err := ag.processMessage(ag.ctx, manifest, pin1, 0, manifest.Messages[0], &core.BatchPersisted{}, bs)
	assert.NoError(t, err)

	// Second message should block
	pin2 := &core.Pin{Masked: true, Sequence: 12346, Signer: msg2.Header.Key}
	err = ag.processMessage(ag.ctx, manifest, pin2, 0, manifest.Messages[1], &core.BatchPersisted{}, bs)
	assert.NoError(t, err)

	assert.Len(t, bs.dispatchedMessages, 1)
	assert.Equal(t, msg1.Header.ID, bs.dispatchedMessages[0].msgID)

}

func TestDispatchPrivateNextPinIncremented(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)
	bs := newBatchState(&ag.aggregator)

	groupID := fftypes.NewRandB32()
	msg1, msg2, org1, manifest := newTestManifest(core.MessageTypePrivate, groupID)

	ag.mim.On("FindIdentityForVerifier", ag.ctx, mock.Anything, mock.Anything).Return(org1, nil)

	ag.mdm.On("GetMessageWithDataCached", ag.ctx, msg1.Header.ID, data.CRORequirePins).Return(msg1, core.DataArray{}, true, nil).Once()
	ag.mdm.On("GetMessageWithDataCached", ag.ctx, msg2.Header.ID, data.CRORequirePins).Return(msg2, core.DataArray{}, true, nil).Once()
	ag.mdm.On("ValidateAll", ag.ctx, mock.Anything).Return(true, nil)

	initNPG := &nextPinGroupState{topic: "topic1", groupID: groupID}
	member1NonceOne := initNPG.calcPinHash(org1.DID, 1)
	member1NonceTwo := initNPG.calcPinHash(org1.DID, 2)
	context := broadcastContext("topic1")

	ag.mdi.On("GetNextPinsForContext", ag.ctx, "ns1", mock.Anything).Return([]*core.NextPin{
		{Context: context, Nonce: 1 /* match member1NonceOne */, Identity: org1.DID, Hash: member1NonceOne},
	}, nil)

	msg1.Pins = fftypes.FFStringArray{member1NonceOne.String()}
	msg2.Pins = fftypes.FFStringArray{member1NonceTwo.String()}

	// First message should dispatch
	err := ag.processMessage(ag.ctx, manifest, &core.Pin{Masked: true, Sequence: 12345, Signer: "0x12345"}, 0, manifest.Messages[0], &core.BatchPersisted{}, bs)
	assert.NoError(t, err)

	// Second message should dispatch too (Twice on GetMessageData)
	err = ag.processMessage(ag.ctx, manifest, &core.Pin{Masked: true, Sequence: 12346, Signer: "0x12345"}, 0, manifest.Messages[1], &core.BatchPersisted{}, bs)
	assert.NoError(t, err)

}

func TestDefinitionBroadcastActionRetry(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	msg1, _, _, _ := newTestManifest(core.MessageTypeDefinition, nil)

	ag.mdh.On("HandleDefinitionBroadcast", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(definitions.HandlerResult{Action: core.ActionRetry}, fmt.Errorf("pop"))

	_, _, err := ag.readyForDispatch(ag.ctx, msg1, nil, nil, &batchState{})
	assert.EqualError(t, err, "pop")

}

func TestDefinitionBroadcastActionReject(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)
	bs := newBatchState(&ag.aggregator)

	msg1, _, org1, manifest := newTestManifest(core.MessageTypeDefinition, nil)

	ag.mim.On("FindIdentityForVerifier", ag.ctx, mock.Anything, mock.Anything).Return(org1, nil)
	ag.mdm.On("GetMessageWithDataCached", ag.ctx, msg1.Header.ID, data.CRORequirePublicBlobRefs).Return(msg1, core.DataArray{}, true, nil).Once()
	ag.mdi.On("GetPins", ag.ctx, "ns1", mock.Anything).Return([]*core.Pin{}, nil, nil).Once()
	ag.mdh.On("HandleDefinitionBroadcast", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(definitions.HandlerResult{Action: core.ActionReject}, fmt.Errorf("pop"))

	pin1 := &core.Pin{Masked: false, Sequence: 12345, Signer: msg1.Header.Key}
	err := ag.processMessage(ag.ctx, manifest, pin1, 0, manifest.Messages[0], &core.BatchPersisted{}, bs)
	assert.NoError(t, err)

	ag.mdi.On("InsertEvent", ag.ctx, mock.MatchedBy(func(e *core.Event) bool {
		return *e.Reference == *msg1.Header.ID && e.Type == core.EventTypeMessageRejected
	})).Return(nil)
	ag.mdm.On("UpdateMessageStateIfCached", ag.ctx, mock.Anything, core.MessageStateRejected, mock.Anything, "pop").Return()
	ag.mdi.On("UpdateMessages", ag.ctx, "ns1", mock.Anything, mock.MatchedBy(func(u ffapi.Update) bool {
		update, err := u.Finalize()
		assert.NoError(t, err)
		assert.Len(t, update.SetOperations, 3)

		assert.Equal(t, "confirmed", update.SetOperations[0].Field)
		v, err := update.SetOperations[0].Value.Value()
		assert.NoError(t, err)
		assert.Greater(t, v, int64(0))

		assert.Equal(t, "state", update.SetOperations[1].Field)
		v, err = update.SetOperations[1].Value.Value()
		assert.NoError(t, err)
		assert.Equal(t, "rejected", v)

		assert.Equal(t, "rejectreason", update.SetOperations[2].Field)
		v, err = update.SetOperations[2].Value.Value()
		assert.NoError(t, err)
		assert.Equal(t, "pop", v)

		return true
	})).Return(nil)
	ag.mdi.On("UpdatePins", ag.ctx, "ns1", mock.Anything, mock.Anything).Return(nil)

	err = bs.RunFinalize(ag.ctx)
	assert.NoError(t, err)

}

func TestDefinitionBroadcastRejectSignerLookupWrongOrg(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	msg1, _, _, _ := newTestManifest(core.MessageTypeDefinition, nil)

	ag.mim.On("FindIdentityForVerifier", ag.ctx, mock.Anything, mock.Anything).Return(newTestOrg("org2"), nil)

	action, err := ag.checkOnchainConsistency(ag.ctx, msg1, &core.Pin{Signer: "0x12345"})
	assert.Equal(t, core.ActionReject, action)
	assert.Regexp(t, "FF10453", err)

}

func TestDefinitionBroadcastParkUnregisteredSignerIdentity(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	msg1, _, _, _ := newTestManifest(core.MessageTypeDefinition, nil)

	ag.mim.On("FindIdentityForVerifier", ag.ctx, mock.Anything, mock.Anything).Return(nil, nil)

	action, err := ag.checkOnchainConsistency(ag.ctx, msg1, &core.Pin{Signer: "0x12345"})
	assert.NoError(t, err)
	assert.Equal(t, core.ActionWait, action)

}

func TestDefinitionBroadcastRootUnregistered(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	msg1, _, _, _ := newTestManifest(core.MessageTypeDefinition, nil)
	msg1.Header.Tag = core.SystemTagIdentityClaim

	ag.mim.On("FindIdentityForVerifier", ag.ctx, mock.Anything, mock.Anything).Return(nil, nil)

	action, err := ag.checkOnchainConsistency(ag.ctx, msg1, &core.Pin{Signer: "0x12345"})
	assert.NoError(t, err)
	assert.Equal(t, core.ActionConfirm, action)

}

func TestDefinitionBroadcastActionWait(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	msg1, _, _, _ := newTestManifest(core.MessageTypeDefinition, nil)

	ag.mdh.On("HandleDefinitionBroadcast", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(definitions.HandlerResult{Action: core.ActionWait}, nil)

	_, _, err := ag.readyForDispatch(ag.ctx, msg1, nil, nil, &batchState{})
	assert.NoError(t, err)

}

func TestPrivateMessageUnregisteredSigner(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	msg1, _, _, _ := newTestManifest(core.MessageTypePrivate, nil)
	msg1.Header.Tag = core.SystemTagIdentityClaim

	ag.mim.On("FindIdentityForVerifier", ag.ctx, mock.Anything, mock.Anything).Return(nil, nil)

	action, err := ag.checkOnchainConsistency(ag.ctx, msg1, &core.Pin{Signer: "0x12345"})
	assert.NoError(t, err)
	assert.Equal(t, core.ActionConfirm, action)

}

func TestCompleteDispatchEventFail(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)
	bs := newBatchState(&ag.aggregator)
	msg1, _, _, _ := newTestManifest(core.MessageTypeBroadcast, nil)

	ag.mdi.On("InsertEvent", ag.ctx, mock.Anything).Return(fmt.Errorf("pop"))

	ag.completeDispatch(core.ActionConfirm, nil, msg1, nil, bs)

	err := bs.RunFinalize(ag.ctx)
	assert.EqualError(t, err, "pop")

}

func TestReadyForDispatchGroupInit(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)
	bs := newBatchState(&ag.aggregator)
	org1 := newTestOrg("org1")

	action, _, err := ag.readyForDispatch(ag.ctx, &core.Message{
		Header: core.MessageHeader{
			ID:        fftypes.NewUUID(),
			Type:      core.MessageTypeGroupInit,
			SignerRef: core.SignerRef{Key: "0x12345", Author: org1.DID},
		},
	}, nil, nil, bs)
	assert.NoError(t, err)
	assert.Equal(t, core.ActionConfirm, action)

}

func TestRewindOffchainBatchesNoBatches(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	rewind, offset := ag.rewindOffchainBatches()
	assert.False(t, rewind)
	assert.Equal(t, int64(0), offset)
}

func TestRewindOffchainBatchesBatchesNoRewind(t *testing.T) {
	config.Set(coreconfig.EventAggregatorBatchSize, 10)

	ag := newTestAggregator()
	defer ag.cleanup(t)

	rewind, offset := ag.rewindOffchainBatches()
	assert.False(t, rewind)
	assert.Equal(t, int64(0), offset)
}

func TestRewindOffchainBatchesAndTXRewind(t *testing.T) {
	config.Set(coreconfig.EventAggregatorBatchSize, 10)

	ag := newTestAggregator()
	defer ag.cleanup(t)

	batchID := fftypes.NewUUID()
	ag.rewinder.readyRewinds = map[fftypes.UUID]bool{
		*batchID: true,
	}

	mockRunAsGroupPassthrough(ag.mdi)
	ag.mdi.On("GetPins", ag.ctx, "ns1", mock.Anything).Return([]*core.Pin{
		{Sequence: 12345, Batch: fftypes.NewUUID()},
	}, nil, nil)

	rewind, offset := ag.rewindOffchainBatches()
	assert.True(t, rewind)
	assert.Equal(t, int64(12344) /* one before the batch */, offset)

}

func TestRewindOffchainBatchesError(t *testing.T) {
	config.Set(coreconfig.EventAggregatorBatchSize, 10)

	ag := newTestAggregator()
	defer ag.cleanup(t)
	ag.cancel()

	batchID := fftypes.NewUUID()
	ag.rewinder.readyRewinds = map[fftypes.UUID]bool{
		*batchID: true,
	}

	mockRunAsGroupPassthrough(ag.mdi)
	ag.mdi.On("GetPins", ag.ctx, "ns1", mock.Anything).Return([]*core.Pin{
		{Sequence: 12345, Batch: fftypes.NewUUID()},
	}, nil, fmt.Errorf("pop"))

	// We will return 0, as context ended so retry will exit after error
	rewind, offset := ag.rewindOffchainBatches()
	assert.False(t, rewind)
	assert.Equal(t, int64(0), offset)

}

func TestResolveBlobsNoop(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	resolved, err := ag.resolveBlobs(ag.ctx, core.DataArray{
		{ID: fftypes.NewUUID(), Blob: &core.BlobRef{}},
	})

	assert.NoError(t, err)
	assert.True(t, resolved)
}

func TestResolveBlobsErrorGettingHash(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	ag.mdi.On("GetBlobs", ag.ctx, mock.Anything, mock.Anything).Return([]*core.Blob{}, nil, fmt.Errorf("pop"))

	resolved, err := ag.resolveBlobs(ag.ctx, core.DataArray{
		{ID: fftypes.NewUUID(), Blob: &core.BlobRef{
			Hash: fftypes.NewRandB32(),
		}},
	})

	assert.EqualError(t, err, "pop")
	assert.False(t, resolved)
}

func TestResolveBlobsNotFoundPrivate(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	ag.mdi.On("GetBlobs", ag.ctx, mock.Anything, mock.Anything).Return([]*core.Blob{}, nil, nil)

	resolved, err := ag.resolveBlobs(ag.ctx, core.DataArray{
		{ID: fftypes.NewUUID(), Blob: &core.BlobRef{
			Hash: fftypes.NewRandB32(),
		}},
	})

	assert.NoError(t, err)
	assert.False(t, resolved)
}

func TestResolveBlobsFoundPrivate(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	ag.mdi.On("GetBlobs", ag.ctx, mock.Anything, mock.Anything).Return([]*core.Blob{{}}, nil, nil)

	resolved, err := ag.resolveBlobs(ag.ctx, core.DataArray{
		{ID: fftypes.NewUUID(), Blob: &core.BlobRef{
			Hash: fftypes.NewRandB32(),
		}},
	})

	assert.NoError(t, err)
	assert.True(t, resolved)
}

func TestBatchActions(t *testing.T) {
	prefinalizeCalled := false
	finalizeCalled := false

	ag := newTestAggregator()
	defer ag.cleanup(t)
	bs := newBatchState(&ag.aggregator)

	bs.AddPreFinalize(func(ctx context.Context) error {
		prefinalizeCalled = true
		return nil
	})
	bs.AddFinalize(func(ctx context.Context) error {
		finalizeCalled = true
		return nil
	})

	err := bs.RunPreFinalize(context.Background())
	assert.NoError(t, err)
	assert.True(t, prefinalizeCalled)

	err = bs.RunFinalize(context.Background())
	assert.NoError(t, err)
	assert.True(t, finalizeCalled)
}

func TestBatchActionsError(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)
	bs := newBatchState(&ag.aggregator)

	bs.AddPreFinalize(func(ctx context.Context) error {
		return fmt.Errorf("pop")
	})
	bs.AddFinalize(func(ctx context.Context) error {
		return fmt.Errorf("pop")
	})

	err := bs.RunPreFinalize(context.Background())
	assert.EqualError(t, err, "pop")

	err = bs.RunFinalize(context.Background())
	assert.EqualError(t, err, "pop")
}

func TestProcessWithBatchActionsPreFinalizeError(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	rag := ag.mdi.On("RunAsGroup", mock.Anything, mock.Anything).Maybe()
	rag.RunFn = func(a mock.Arguments) {
		rag.ReturnArguments = mock.Arguments{a[1].(func(context.Context) error)(a[0].(context.Context))}
	}

	err := ag.processWithBatchState(func(ctx context.Context, actions *batchState) error {
		actions.AddPreFinalize(func(ctx context.Context) error { return fmt.Errorf("pop") })
		return nil
	})
	assert.EqualError(t, err, "pop")
}

func TestProcessWithBatchActionsSuccess(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	rag := ag.mdi.On("RunAsGroup", mock.Anything, mock.Anything).Maybe()
	rag.RunFn = func(a mock.Arguments) {
		rag.ReturnArguments = mock.Arguments{a[1].(func(context.Context) error)(a[0].(context.Context))}
	}

	err := ag.processWithBatchState(func(ctx context.Context, actions *batchState) error {
		actions.AddPreFinalize(func(ctx context.Context) error { return nil })
		actions.AddFinalize(func(ctx context.Context) error { return nil })
		return nil
	})
	assert.NoError(t, err)
}

func TestProcessWithBatchRewindsSuccess(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	rag := ag.mdi.On("RunAsGroup", mock.Anything, mock.Anything).Maybe()
	rag.RunFn = func(a mock.Arguments) {
		rag.ReturnArguments = mock.Arguments{a[1].(func(context.Context) error)(a[0].(context.Context))}
	}

	err := ag.processWithBatchState(func(ctx context.Context, actions *batchState) error {
		actions.AddConfirmedDIDClaim("did:firefly:org/test")
		return nil
	})
	assert.NoError(t, err)
}

func TestProcessWithBatchActionsFail(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	rag := ag.mdi.On("RunAsGroup", mock.Anything, mock.Anything).Once()
	rag.RunFn = func(a mock.Arguments) {
		rag.ReturnArguments = mock.Arguments{a[1].(func(context.Context) error)(a[0].(context.Context))}
	}
	ag.mdi.On("RunAsGroup", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))

	err := ag.processWithBatchState(func(ctx context.Context, actions *batchState) error {
		actions.AddPreFinalize(func(ctx context.Context) error { return nil })
		return nil
	})
	assert.EqualError(t, err, "pop")
}

func TestExtractManifestFail(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	manifest := ag.extractManifest(ag.ctx, &core.BatchPersisted{
		Manifest: fftypes.JSONAnyPtr("!wrong"),
	})

	assert.Nil(t, manifest)
}

func TestExtractManifestBadVersion(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	manifest := ag.extractManifest(ag.ctx, &core.BatchPersisted{
		Manifest: fftypes.JSONAnyPtr(`{"version":999}`),
	})

	assert.Nil(t, manifest)
}

func TestMigrateManifestFail(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	manifest := ag.migrateManifest(ag.ctx, &core.BatchPersisted{
		Manifest: fftypes.JSONAnyPtr("!wrong"),
	})

	assert.Nil(t, manifest)
}

func TestBatchCaching(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	data := &core.Data{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"test"`)}
	batch := sampleBatch(t, core.BatchTypeBroadcast, core.TransactionTypeBatchPin, core.DataArray{data})
	persisted, expectedManifest := batch.Confirmed()

	pin := &core.Pin{
		Batch:     batch.ID,
		BatchHash: batch.Hash,
	}

	ag.mdi.On("GetBatchByID", ag.ctx, "ns1", batch.ID).Return(persisted, nil).Once() // to prove caching

	batchRetrieved, manifest, err := ag.GetBatchForPin(ag.ctx, pin)
	assert.NoError(t, err)
	assert.Equal(t, persisted, batchRetrieved)
	assert.Equal(t, expectedManifest, manifest)

	batchRetrieved, manifest, err = ag.GetBatchForPin(ag.ctx, pin)
	assert.NoError(t, err)
	assert.Equal(t, persisted, batchRetrieved)
	assert.Equal(t, expectedManifest, manifest)

}

func TestGetBatchForPinHashMismatch(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)

	data := &core.Data{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"test"`)}
	batch := sampleBatch(t, core.BatchTypeBroadcast, core.TransactionTypeBatchPin, core.DataArray{data})
	persisted, _ := batch.Confirmed()
	pin := &core.Pin{
		Batch:     batch.ID,
		BatchHash: fftypes.NewRandB32(),
	}

	ag.mdi.On("GetBatchByID", ag.ctx, "ns1", batch.ID).Return(persisted, nil)

	batchRetrieved, manifest, err := ag.GetBatchForPin(ag.ctx, pin)
	assert.Nil(t, batchRetrieved)
	assert.Nil(t, manifest)
	assert.Nil(t, err)

}
