// Copyright © 2022 Kaleido, Inc.
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
package batch

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"testing"
	"time"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/cache"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/txcommon"
	"github.com/hyperledger-firefly/firefly/mocks/cachemocks"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/mocks/datamocks"
	"github.com/hyperledger-firefly/firefly/mocks/identitymanagermocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func testConfigReset() {
	coreconfig.Reset()
	config.Set(coreconfig.BatchManagerMinimumPollDelay, "0")
	log.SetLevel("debug")
}

func newTestBatchManager(t *testing.T) (*batchManager, func()) {
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	mim := &identitymanagermocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := txcommon.NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)
	config.Set(coreconfig.BatchManagerReadPageSize, 0) // will get min value of 1
	bm, err := NewBatchManager(context.Background(), "ns1", mdi, mdm, mim, txHelper)
	assert.NoError(t, err)
	return bm.(*batchManager), bm.(*batchManager).cancelCtx
}

func TestE2EDispatchBroadcast(t *testing.T) {
	testConfigReset()

	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	mim := &identitymanagermocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := txcommon.NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)
	mim.On("GetLocalNode", mock.Anything).Return(&core.Identity{}, nil)

	readyForDispatch := make(chan bool)
	waitForDispatch := make(chan *DispatchPayload)
	handler := func(ctx context.Context, state *DispatchPayload) error {
		_, ok := <-readyForDispatch
		if !ok {
			return nil
		}
		assert.Len(t, state.Pins, 2)
		h := sha256.New()
		nonceBytes, _ := hex.DecodeString(
			"746f70696331",
		/*|  topic1   | */
		) // little endian 12345 in 8 byte hex
		h.Write(nonceBytes)
		assert.Equal(t, hex.EncodeToString(h.Sum([]byte{})), state.Pins[0].String())

		h = sha256.New()
		nonceBytes, _ = hex.DecodeString(
			"746f70696332",
		/*|   topic2  | */
		) // little endian 12345 in 8 byte hex
		h.Write(nonceBytes)
		assert.Equal(t, hex.EncodeToString(h.Sum([]byte{})), state.Pins[1].String())

		waitForDispatch <- state
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	bmi, _ := NewBatchManager(ctx, "ns1", mdi, mdm, mim, txHelper)
	bm := bmi.(*batchManager)
	bm.readOffset = 1000

	bm.RegisterDispatcher("utdispatcher", true, []core.MessageType{core.MessageTypeBroadcast}, handler, DispatcherOptions{
		BatchMaxSize:   2,
		BatchTimeout:   0,
		DisposeTimeout: 10 * time.Millisecond,
	})

	dataID1 := fftypes.NewUUID()
	dataHash := fftypes.NewRandB32()
	msg := &core.Message{
		Header: core.MessageHeader{
			TxType:    core.TransactionTypeBatchPin,
			Type:      core.MessageTypeBroadcast,
			ID:        fftypes.NewUUID(),
			Topics:    []string{"topic1", "topic2"},
			Namespace: "ns1",
			SignerRef: core.SignerRef{Author: "did:firefly:org/abcd", Key: "0x12345"},
		},
		Data: core.DataRefs{
			{ID: dataID1, Hash: dataHash},
		},
		Sequence: 500,
	}
	data := &core.Data{
		ID:   dataID1,
		Hash: dataHash,
	}
	mdm.On("GetMessageWithDataCached", mock.Anything, mock.Anything).Return(msg, core.DataArray{data}, true, nil)
	mdm.On("UpdateMessageIfCached", mock.Anything, mock.Anything).Return()
	mdi.On("GetMessageIDs", mock.Anything, "ns1", mock.Anything).Return([]*core.IDAndSequence{{ID: *msg.Header.ID}}, nil).Once()
	mdi.On("GetMessageIDs", mock.Anything, "ns1", mock.Anything).Return([]*core.IDAndSequence{}, nil)
	mdi.On("InsertOrGetBatch", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)
	mdi.On("UpdateBatch", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mdi.On("UpdateMessage", mock.Anything, "ns1", mock.Anything, mock.Anything).Return(nil) // pins
	rag := mdi.On("RunAsGroup", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	rag.RunFn = func(a mock.Arguments) {
		ctx := a.Get(0).(context.Context)
		fn := a.Get(1).(func(context.Context) error)
		fn(ctx)
	}
	mdi.On("UpdateMessages", mock.Anything, "ns1", mock.MatchedBy(func(f ffapi.Filter) bool {
		fi, err := f.Finalize()
		assert.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("( id IN ['%s'] ) && ( state == 'ready' )", msg.Header.ID.String()), fi.String())
		return true
	}), mock.Anything).Return(nil)
	mdi.On("InsertTransaction", mock.Anything, mock.Anything).Return(nil)
	mdi.On("InsertEvent", mock.Anything, mock.Anything).Return(nil) // transaction submit

	err := bm.Start()
	assert.NoError(t, err)

	bm.NewMessages() <- msg.Sequence

	readyForDispatch <- true

	// Check the status while we know there's a flush going on
	status := bm.Status()
	assert.NotNil(t, status.Processors[0].Status.Flushing)

	b := <-waitForDispatch
	assert.Equal(t, *msg.Header.ID, *b.Messages[0].Header.ID)
	assert.Equal(t, *data.ID, *b.Data[0].ID)

	close(readyForDispatch)

	// Wait for the reaping
	for len(bm.getProcessors()) > 0 {
		time.Sleep(1 * time.Millisecond)
		bm.shoulderTap <- true
	}

	cancel()
	bm.WaitStop()

}

func TestE2EDispatchPrivateUnpinned(t *testing.T) {
	testConfigReset()

	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	mim := &identitymanagermocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := txcommon.NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)
	mim.On("GetLocalNode", mock.Anything).Return(&core.Identity{}, nil)

	readyForDispatch := make(chan bool)
	waitForDispatch := make(chan *DispatchPayload)
	var groupID fftypes.Bytes32
	_ = groupID.UnmarshalText([]byte("44dc0861e69d9bab17dd5e90a8898c2ea156ad04e5fabf83119cc010486e6c1b"))
	handler := func(ctx context.Context, state *DispatchPayload) error {
		_, ok := <-readyForDispatch
		if !ok {
			return nil
		}
		assert.Len(t, state.Pins, 2)
		h := sha256.New()
		nonceBytes, _ := hex.DecodeString(
			"746f70696331" + "44dc0861e69d9bab17dd5e90a8898c2ea156ad04e5fabf83119cc010486e6c1b" + "6469643a66697265666c793a6f72672f61626364" + "0000000000003039",
		/*|  topic1   |    | ---- group id -------------------------------------------------|   |author'"did:firefly:org/abcd'            |  |i64 nonce (12345) */
		/*|               context                                                           |   |          sender + nonce             */
		) // little endian 12345 in 8 byte hex
		h.Write(nonceBytes)
		assert.Equal(t, hex.EncodeToString(h.Sum([]byte{})), state.Pins[0].String())

		h = sha256.New()
		nonceBytes, _ = hex.DecodeString(
			"746f70696332" + "44dc0861e69d9bab17dd5e90a8898c2ea156ad04e5fabf83119cc010486e6c1b" + "6469643a66697265666c793a6f72672f61626364" + "0000000000003039",
		/*|   topic2  |    | ---- group id -------------------------------------------------|   |author'"did:firefly:org/abcd'            |  |i64 nonce (12345) */
		/*|               context                                                           |   |          sender + nonce             */
		) // little endian 12345 in 8 byte hex
		h.Write(nonceBytes)
		assert.Equal(t, hex.EncodeToString(h.Sum([]byte{})), state.Pins[1].String())
		waitForDispatch <- state
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	bmi, _ := NewBatchManager(ctx, "ns1", mdi, mdm, mim, txHelper)
	bm := bmi.(*batchManager)

	bm.RegisterDispatcher("utdispatcher", true, []core.MessageType{core.MessageTypePrivate}, handler, DispatcherOptions{
		BatchMaxSize:   2,
		BatchTimeout:   0,
		DisposeTimeout: 120 * time.Second,
	})

	dataID1 := fftypes.NewUUID()
	dataHash := fftypes.NewRandB32()
	msg := &core.Message{
		Header: core.MessageHeader{
			TxType:    core.TransactionTypeBatchPin,
			Type:      core.MessageTypePrivate,
			ID:        fftypes.NewUUID(),
			Topics:    []string{"topic1", "topic2"},
			Namespace: "ns1",
			SignerRef: core.SignerRef{Author: "did:firefly:org/abcd", Key: "0x12345"},
			Group:     &groupID,
		},
		Data: core.DataRefs{
			{ID: dataID1, Hash: dataHash},
		},
	}
	data := &core.Data{
		ID:   dataID1,
		Hash: dataHash,
	}
	mdm.On("GetMessageWithDataCached", mock.Anything, mock.Anything).Return(msg, core.DataArray{data}, true, nil)
	mdm.On("UpdateMessageIfCached", mock.Anything, mock.Anything).Return()
	mdi.On("GetMessageIDs", mock.Anything, "ns1", mock.Anything).Return([]*core.IDAndSequence{{ID: *msg.Header.ID}}, nil).Once()
	mdi.On("GetMessageIDs", mock.Anything, "ns1", mock.Anything).Return([]*core.IDAndSequence{}, nil)
	mdi.On("UpdateMessage", mock.Anything, "ns1", mock.Anything, mock.Anything).Return(nil) // pins
	mdi.On("InsertOrGetBatch", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)
	mdi.On("UpdateBatch", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	rag := mdi.On("RunAsGroup", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	rag.RunFn = func(a mock.Arguments) {
		ctx := a.Get(0).(context.Context)
		fn := a.Get(1).(func(context.Context) error)
		fn(ctx)
	}
	mdi.On("UpdateMessages", mock.Anything, "ns1", mock.MatchedBy(func(f ffapi.Filter) bool {
		fi, err := f.Finalize()
		assert.NoError(t, err)
		assert.Equal(t, fmt.Sprintf("( id IN ['%s'] ) && ( state == 'ready' )", msg.Header.ID.String()), fi.String())
		return true
	}), mock.Anything).Return(nil)
	mdi.On("GetNonce", mock.Anything, mock.Anything).Return(&core.Nonce{
		Nonce: int64(12344),
	}, nil).Twice()
	mdi.On("UpdateNonce", mock.Anything, mock.Anything).Return(nil)
	mdi.On("InsertTransaction", mock.Anything, mock.Anything).Return(nil)
	mdi.On("InsertEvent", mock.Anything, mock.Anything).Return(nil) // transaction submit

	err := bm.Start()
	assert.NoError(t, err)

	bm.NewMessages() <- msg.Sequence

	readyForDispatch <- true
	b := <-waitForDispatch
	assert.Equal(t, *msg.Header.ID, *b.Messages[0].Header.ID)
	assert.Equal(t, *data.ID, *b.Data[0].ID)

	// Wait until everything closes
	close(readyForDispatch)
	cancel()
	bm.WaitStop()

}

func TestDispatchUnknownType(t *testing.T) {
	testConfigReset()

	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	mim := &identitymanagermocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := txcommon.NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)
	ctx, cancel := context.WithCancel(context.Background())
	bmi, _ := NewBatchManager(ctx, "ns1", mdi, mdm, mim, txHelper)
	bm := bmi.(*batchManager)

	msg := &core.Message{
		Header: core.MessageHeader{
			ID: fftypes.NewUUID(),
		},
	}
	mdi.On("GetMessageIDs", mock.Anything, "ns1", mock.Anything).Return([]*core.IDAndSequence{{ID: *msg.Header.ID}}, nil).Once()
	mdm.On("GetMessageWithDataCached", mock.Anything, mock.Anything).Return(msg, core.DataArray{}, true, nil)

	err := bm.Start()
	assert.NoError(t, err)

	cancel()
	bm.WaitStop()

}

func TestInitFailNoPersistence(t *testing.T) {
	_, err := NewBatchManager(context.Background(), "", nil, nil, nil, nil)
	assert.Error(t, err)
}

func TestGetInvalidBatchTypeMsg(t *testing.T) {

	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	mim := &identitymanagermocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := txcommon.NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)
	bm, _ := NewBatchManager(context.Background(), "ns1", mdi, mdm, mim, txHelper)
	defer bm.Close()
	_, err := bm.(*batchManager).getProcessor(core.BatchTypeBroadcast, "wrong", nil, "", true)
	assert.Regexp(t, "FF10126", err)
}

func TestMessageSequencerCancelledContext(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	mim := &identitymanagermocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := txcommon.NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)
	mdi.On("GetMessageIDs", mock.Anything, "ns1", mock.Anything).Return(nil, fmt.Errorf("pop")).Once()
	bm, _ := NewBatchManager(context.Background(), "ns1", mdi, mdm, mim, txHelper)
	defer bm.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	bm.(*batchManager).ctx = ctx
	bm.(*batchManager).messageSequencer()
	assert.Equal(t, 1, len(mdi.Calls))
}

func TestMessageSequencerMissingMessageData(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	mim := &identitymanagermocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := txcommon.NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)
	bm, _ := NewBatchManager(context.Background(), "ns1", mdi, mdm, mim, txHelper)
	bm.RegisterDispatcher("utdispatcher", false, []core.MessageType{core.MessageTypeBroadcast},
		func(c context.Context, state *DispatchPayload) error {
			return nil
		},
		DispatcherOptions{BatchType: core.BatchTypeBroadcast},
	)

	dataID := fftypes.NewUUID()
	msg := &core.Message{
		Header: core.MessageHeader{
			ID:        fftypes.NewUUID(),
			Type:      core.MessageTypeBroadcast,
			Namespace: "ns1",
			TxType:    core.TransactionTypeNone,
		},
		Data: []*core.DataRef{
			{ID: dataID},
		}}

	mdi.On("GetMessageIDs", mock.Anything, "ns1", mock.Anything).
		Return([]*core.IDAndSequence{{ID: *msg.Header.ID}}, nil, nil).
		Run(func(args mock.Arguments) {
			bm.Close()
		}).
		Once()
	mdi.On("GetMessageIDs", mock.Anything, "ns1", mock.Anything).Return([]*core.IDAndSequence{}, nil, nil)
	mdm.On("GetMessageWithDataCached", mock.Anything, mock.Anything).Return(msg, core.DataArray{}, false, nil)

	bm.(*batchManager).messageSequencer()

	bm.WaitStop()

	mdi.AssertExpectations(t)
	mdm.AssertExpectations(t)
}

func TestMessageSequencerUpdateMessagesFail(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	mim := &identitymanagermocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := txcommon.NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)
	mim.On("GetLocalNode", mock.Anything).Return(&core.Identity{}, nil)
	ctx, cancelCtx := context.WithCancel(context.Background())
	bm, _ := NewBatchManager(ctx, "ns1", mdi, mdm, mim, txHelper)
	bm.RegisterDispatcher("utdispatcher", true, []core.MessageType{core.MessageTypeBroadcast},
		func(c context.Context, state *DispatchPayload) error {
			return nil
		},
		DispatcherOptions{BatchMaxSize: 1, DisposeTimeout: 0},
	)

	dataID := fftypes.NewUUID()
	msg := &core.Message{
		Header: core.MessageHeader{
			ID:        fftypes.NewUUID(),
			TxType:    core.TransactionTypeBatchPin,
			Type:      core.MessageTypeBroadcast,
			Namespace: "ns1",
		},
		Data: []*core.DataRef{
			{ID: dataID},
		},
	}
	mdi.On("GetMessageIDs", mock.Anything, "ns1", mock.Anything).Return([]*core.IDAndSequence{{ID: *msg.Header.ID}}, nil, nil)
	mdm.On("GetMessageWithDataCached", mock.Anything, mock.Anything).Return(msg, core.DataArray{{ID: dataID}}, true, nil)
	mdm.On("UpdateMessageIfCached", mock.Anything, mock.Anything).Return()
	mdi.On("InsertTransaction", mock.Anything, mock.Anything).Return(nil)
	mdi.On("InsertEvent", mock.Anything, mock.Anything).Return(nil) // transaction submit
	mdi.On("InsertOrGetBatch", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)
	mdi.On("UpdateMessages", mock.Anything, "ns1", mock.Anything, mock.Anything).Return(fmt.Errorf("fizzle"))
	rag := mdi.On("RunAsGroup", mock.Anything, mock.Anything, mock.Anything)
	rag.RunFn = func(a mock.Arguments) {
		ctx := a.Get(0).(context.Context)
		fn := a.Get(1).(func(context.Context) error)
		err, ok := fn(ctx).(error)
		if ok && err.Error() == "fizzle" {
			cancelCtx() // so we only go round once
			bm.Close()
		}
		rag.ReturnArguments = mock.Arguments{err}
	}

	bm.(*batchManager).messageSequencer()

	bm.Close()
	bm.WaitStop()

	mdi.AssertExpectations(t)
	mdm.AssertExpectations(t)
}

func TestMessageSequencerDispatchFail(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	mim := &identitymanagermocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := txcommon.NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)
	mim.On("GetLocalNode", mock.Anything).Return(&core.Identity{}, nil)
	ctx, cancelCtx := context.WithCancel(context.Background())
	bm, _ := NewBatchManager(ctx, "ns1", mdi, mdm, mim, txHelper)
	bm.RegisterDispatcher("utdispatcher", true, []core.MessageType{core.MessageTypeBroadcast},
		func(c context.Context, state *DispatchPayload) error {
			cancelCtx()
			return fmt.Errorf("fizzle")
		}, DispatcherOptions{BatchMaxSize: 1, DisposeTimeout: 0},
	)

	dataID := fftypes.NewUUID()
	msg := &core.Message{
		Header: core.MessageHeader{
			ID:        fftypes.NewUUID(),
			TxType:    core.TransactionTypeBatchPin,
			Type:      core.MessageTypeBroadcast,
			Namespace: "ns1",
		},
		Data: []*core.DataRef{
			{ID: dataID},
		},
	}
	mdi.On("GetMessageIDs", mock.Anything, "ns1", mock.Anything).Return([]*core.IDAndSequence{{ID: *msg.Header.ID}}, nil)
	mdm.On("GetMessageWithDataCached", mock.Anything, mock.Anything).Return(msg, core.DataArray{{ID: dataID}}, true, nil)
	mdi.On("RunAsGroup", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	bm.(*batchManager).messageSequencer()

	bm.Close()
	bm.WaitStop()

	mdi.AssertExpectations(t)
	mdm.AssertExpectations(t)
}

func TestMessageSequencerUpdateBatchFail(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	mim := &identitymanagermocks.Manager{}
	ctx, cancelCtx := context.WithCancel(context.Background())
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := txcommon.NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)
	mim.On("GetLocalNode", mock.Anything).Return(&core.Identity{}, nil)
	bm, _ := NewBatchManager(ctx, "ns1", mdi, mdm, mim, txHelper)
	bm.RegisterDispatcher("utdispatcher", true, []core.MessageType{core.MessageTypeBroadcast},
		func(c context.Context, state *DispatchPayload) error {
			return nil
		},
		DispatcherOptions{BatchMaxSize: 1, DisposeTimeout: 0},
	)

	dataID := fftypes.NewUUID()
	msg := &core.Message{
		Header: core.MessageHeader{
			ID:        fftypes.NewUUID(),
			TxType:    core.TransactionTypeBatchPin,
			Type:      core.MessageTypeBroadcast,
			Namespace: "ns1",
		},
		Data: []*core.DataRef{
			{ID: dataID},
		},
	}
	mdi.On("GetMessageIDs", mock.Anything, "ns1", mock.Anything).Return([]*core.IDAndSequence{{ID: *msg.Header.ID}}, nil)
	mdm.On("GetMessageWithDataCached", mock.Anything, mock.Anything).Return(msg, core.DataArray{{ID: dataID}}, true, nil)
	mdi.On("InsertOrGetBatch", mock.Anything, mock.Anything, mock.Anything).Return(nil, fmt.Errorf("fizzle"))
	rag := mdi.On("RunAsGroup", mock.Anything, mock.Anything, mock.Anything)
	rag.RunFn = func(a mock.Arguments) {
		ctx := a.Get(0).(context.Context)
		fn := a.Get(1).(func(context.Context) error)
		err, ok := fn(ctx).(error)
		if ok && err.Error() == "fizzle" {
			cancelCtx() // so we only go round once
			bm.Close()
		}
		rag.ReturnArguments = mock.Arguments{err}
	}
	mdi.On("InsertTransaction", mock.Anything, mock.Anything).Return(nil).Maybe()
	mdi.On("InsertEvent", mock.Anything, mock.Anything).Return(nil).Maybe()

	bm.(*batchManager).messageSequencer()

	bm.Close()
	bm.WaitStop()

	mdi.AssertExpectations(t)
	mdm.AssertExpectations(t)
}

func TestWaitForPollTimeout(t *testing.T) {
	bm, _ := newTestBatchManager(t)
	bm.messagePollTimeout = 1 * time.Microsecond
	bm.waitForNewMessages()
}

func TestRewindForNewMessage(t *testing.T) {
	bm, cancel := newTestBatchManager(t)
	defer cancel()
	go bm.newMessageNotifier()
	bm.messagePollTimeout = 1 * time.Second
	bm.waitForNewMessages()
	bm.readOffset = 22222
	bm.NewMessages() <- 12346
	bm.NewMessages() <- 12347
	bm.NewMessages() <- 12345
	bm.waitForNewMessages()
	assert.Equal(t, int64(12344), bm.rewindOffset)

	mdi := bm.database.(*databasemocks.Plugin)
	mdi.On("GetMessageIDs", mock.Anything, "ns1", mock.MatchedBy(func(f ffapi.Filter) bool {
		fi, err := f.Finalize()
		assert.NoError(t, err)
		v, err := fi.Children[0].Value.Value()
		assert.NoError(t, err)
		assert.Equal(t, int64(12344), v)
		return true
	})).Return(nil, nil)
	_, _, err := bm.readPage(false)
	assert.NoError(t, err)
}

func TestAssembleMessageDataNilData(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	mim := &identitymanagermocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := txcommon.NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)
	bm, _ := NewBatchManager(context.Background(), "ns1", mdi, mdm, mim, txHelper)
	bm.Close()
	mdm.On("GetMessageWithDataCached", mock.Anything, mock.Anything).Return(nil, nil, false, nil)
	_, _, err := bm.(*batchManager).assembleMessageData(fftypes.NewUUID())
	assert.Regexp(t, "FF10133", err)
}

func TestGetMessageDataFail(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	mim := &identitymanagermocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := txcommon.NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)
	bm, _ := NewBatchManager(context.Background(), "ns1", mdi, mdm, mim, txHelper)
	mdm.On("GetMessageWithDataCached", mock.Anything, mock.Anything).Return(nil, nil, false, fmt.Errorf("pop"))
	bm.Close()
	_, _, err := bm.(*batchManager).assembleMessageData(fftypes.NewUUID())
	assert.Regexp(t, "FF00154", err)
	mdm.AssertExpectations(t)
}

func TestGetMessageNotFound(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	mim := &identitymanagermocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := txcommon.NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)
	bm, _ := NewBatchManager(context.Background(), "ns1", mdi, mdm, mim, txHelper)
	mdm.On("GetMessageWithDataCached", mock.Anything, mock.Anything).Return(nil, nil, false, nil)
	bm.Close()
	_, _, err := bm.(*batchManager).assembleMessageData(fftypes.NewUUID())
	assert.Regexp(t, "FF10133", err)
}

func TestDoubleTap(t *testing.T) {
	bm, cancel := newTestBatchManager(t)
	defer cancel()
	bm.readOffset = 3000
	go bm.newMessageNotifier()

	bm.NewMessages() <- 2000
	bm.NewMessages() <- 1000

	for bm.rewindOffset != int64(999) {
		time.Sleep(1 * time.Microsecond)
	}
}

func TestLoadContextsBroadcast(t *testing.T) {
	bm, cancel := newTestBatchManager(t)
	defer cancel()

	payload := &DispatchPayload{
		Batch: core.BatchPersisted{},
		Messages: []*core.Message{{
			Header: core.MessageHeader{
				Topics: fftypes.FFStringArray{"topic1"},
			},
		}},
	}

	err := bm.LoadContexts(context.Background(), payload)

	expected := []*fftypes.Bytes32{
		fftypes.MustParseBytes32("9e065a7cbddfc57be742bc32956674c3c389521ac2bbb1dce0500d5131fede75"),
	}
	assert.NoError(t, err)
	assert.Equal(t, expected, payload.Pins)
}

func TestLoadContextsPrivate(t *testing.T) {
	bm, cancel := newTestBatchManager(t)
	defer cancel()

	pin := fftypes.NewRandB32()
	payload := &DispatchPayload{
		Batch: core.BatchPersisted{},
		Messages: []*core.Message{{
			Header: core.MessageHeader{
				Group: fftypes.NewRandB32(),
			},
			Pins: fftypes.FFStringArray{pin.String()},
		}},
	}

	err := bm.LoadContexts(context.Background(), payload)

	expected := []*fftypes.Bytes32{pin}
	assert.NoError(t, err)
	assert.Equal(t, expected, payload.Pins)
}

func TestLoadContextsPrivateNoPins(t *testing.T) {
	bm, cancel := newTestBatchManager(t)
	defer cancel()

	payload := &DispatchPayload{
		Batch: core.BatchPersisted{},
		Messages: []*core.Message{{
			Header: core.MessageHeader{
				Group: fftypes.NewRandB32(),
			},
		}},
	}

	err := bm.LoadContexts(context.Background(), payload)

	assert.Regexp(t, "FF10442", err)
}

func TestLoadContextsPrivateBadPin(t *testing.T) {
	bm, cancel := newTestBatchManager(t)
	defer cancel()

	payload := &DispatchPayload{
		Batch: core.BatchPersisted{},
		Messages: []*core.Message{{
			Header: core.MessageHeader{
				Group: fftypes.NewRandB32(),
			},
			Pins: fftypes.FFStringArray{"bad"},
		}},
	}

	err := bm.LoadContexts(context.Background(), payload)

	assert.Regexp(t, "FF00107", err)
}

func TestCancelBatchBadID(t *testing.T) {
	bm, cancel := newTestBatchManager(t)
	defer cancel()

	err := bm.CancelBatch(context.Background(), "bad-id")
	assert.Regexp(t, "FF00138", err)
}

func TestCancelBatchFailLoad(t *testing.T) {
	bm, cancel := newTestBatchManager(t)
	defer cancel()

	batchID := fftypes.NewUUID()

	mdi := bm.database.(*databasemocks.Plugin)
	mdi.On("GetBatchByID", context.Background(), "ns1", batchID).Return(nil, fmt.Errorf("pop"))

	err := bm.CancelBatch(context.Background(), batchID.String())
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestCancelBatchFailHydrate(t *testing.T) {
	bm, cancel := newTestBatchManager(t)
	defer cancel()

	batchID := fftypes.NewUUID()
	bp := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			ID: batchID,
		},
		TX: core.TransactionRef{
			Type: core.TransactionTypeContractInvokePin,
		},
	}

	mdi := bm.database.(*databasemocks.Plugin)
	mdm := bm.data.(*datamocks.Manager)
	mdi.On("GetBatchByID", context.Background(), "ns1", batchID).Return(bp, nil)
	mdm.On("HydrateBatch", context.Background(), bp).Return(nil, fmt.Errorf("pop"))

	err := bm.CancelBatch(context.Background(), batchID.String())
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
	mdm.AssertExpectations(t)
}

func TestCancelBatchNoPayload(t *testing.T) {
	bm, cancel := newTestBatchManager(t)
	defer cancel()

	batchID := fftypes.NewUUID()
	bp := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			ID: batchID,
		},
		TX: core.TransactionRef{
			Type: core.TransactionTypeContractInvokePin,
		},
	}
	batch := &core.Batch{
		BatchHeader: bp.BatchHeader,
		Payload:     core.BatchPayload{},
	}

	mdi := bm.database.(*databasemocks.Plugin)
	mdm := bm.data.(*datamocks.Manager)
	mdi.On("GetBatchByID", context.Background(), "ns1", batchID).Return(bp, nil)
	mdm.On("HydrateBatch", context.Background(), bp).Return(batch, nil)

	err := bm.CancelBatch(context.Background(), batchID.String())
	assert.Regexp(t, "FF10467", err)

	mdi.AssertExpectations(t)
	mdm.AssertExpectations(t)
}

func TestCancelBatchUnregisteredProcessor(t *testing.T) {
	bm, cancel := newTestBatchManager(t)
	defer cancel()

	group := fftypes.NewRandB32()

	batchID := fftypes.NewUUID()
	bp := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			ID: batchID,
		},
		TX: core.TransactionRef{
			Type: core.TransactionTypeContractInvokePin,
		},
	}
	msgid := fftypes.NewUUID()
	msg := &core.Message{
		Header: core.MessageHeader{
			ID:     msgid,
			Type:   core.MessageTypePrivate,
			TxType: core.TransactionTypeContractInvokePin,
			Group:  group,
			SignerRef: core.SignerRef{
				Author: "did:firefly:org/abcd",
			},
		},
	}
	batch := &core.Batch{
		BatchHeader: bp.BatchHeader,
		Payload: core.BatchPayload{
			Messages: []*core.Message{msg},
		},
	}

	mdi := bm.database.(*databasemocks.Plugin)
	mdm := bm.data.(*datamocks.Manager)
	mdi.On("GetBatchByID", context.Background(), "ns1", batchID).Return(bp, nil)
	mdm.On("HydrateBatch", context.Background(), bp).Return(batch, nil)

	err := bm.CancelBatch(context.Background(), batchID.String())
	assert.Regexp(t, "FF10126", err)

	mdi.AssertExpectations(t)
	mdm.AssertExpectations(t)
}

func TestCancelBatchInactiveProcessor(t *testing.T) {
	bm, cancel := newTestBatchManager(t)
	defer cancel()

	bm.RegisterDispatcher("utdispatcher", true, []core.MessageType{core.MessageTypePrivate},
		func(c context.Context, state *DispatchPayload) error {
			return nil
		},
		DispatcherOptions{BatchType: core.BatchTypePrivate},
	)
	group := fftypes.NewRandB32()

	batchID := fftypes.NewUUID()
	bp := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			ID: batchID,
		},
		TX: core.TransactionRef{
			Type: core.TransactionTypeContractInvokePin,
		},
	}
	msgid := fftypes.NewUUID()
	msg := &core.Message{
		Header: core.MessageHeader{
			ID:     msgid,
			Type:   core.MessageTypePrivate,
			TxType: core.TransactionTypeContractInvokePin,
			Group:  group,
			SignerRef: core.SignerRef{
				Author: "did:firefly:org/abcd",
			},
		},
	}
	batch := &core.Batch{
		BatchHeader: bp.BatchHeader,
		Payload: core.BatchPayload{
			Messages: []*core.Message{msg},
		},
	}

	mdi := bm.database.(*databasemocks.Plugin)
	mdm := bm.data.(*datamocks.Manager)
	mdi.On("GetBatchByID", context.Background(), "ns1", batchID).Return(bp, nil)
	mdm.On("HydrateBatch", context.Background(), bp).Return(batch, nil)

	err := bm.CancelBatch(context.Background(), batchID.String())
	assert.Regexp(t, "FF10468", err)

	mdi.AssertExpectations(t)
	mdm.AssertExpectations(t)
}

func TestCancelBatchInvalidType(t *testing.T) {
	bm, cancel := newTestBatchManager(t)
	defer cancel()

	bm.RegisterDispatcher("utdispatcher", true, []core.MessageType{core.MessageTypePrivate},
		func(c context.Context, state *DispatchPayload) error {
			return nil
		},
		DispatcherOptions{BatchType: core.BatchTypePrivate},
	)

	batchID := fftypes.NewUUID()
	bp := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			ID: batchID,
		},
		TX: core.TransactionRef{
			Type: core.TransactionTypeBatchPin,
		},
	}

	mdi := bm.database.(*databasemocks.Plugin)
	mdi.On("GetBatchByID", context.Background(), "ns1", batchID).Return(bp, nil)

	err := bm.CancelBatch(context.Background(), batchID.String())
	assert.Regexp(t, "FF10466", err)

	mdi.AssertExpectations(t)
}

func TestCancelBatchNotFound(t *testing.T) {
	bm, cancel := newTestBatchManager(t)
	defer cancel()

	bm.RegisterDispatcher("utdispatcher", true, []core.MessageType{core.MessageTypePrivate},
		func(c context.Context, state *DispatchPayload) error {
			return nil
		},
		DispatcherOptions{BatchType: core.BatchTypePrivate},
	)

	batchID := fftypes.NewUUID()

	mdi := bm.database.(*databasemocks.Plugin)
	mdi.On("GetBatchByID", context.Background(), "ns1", batchID).Return(nil, nil)

	err := bm.CancelBatch(context.Background(), batchID.String())
	assert.Regexp(t, "FF10109", err)

	mdi.AssertExpectations(t)
}

func TestCancelBatch(t *testing.T) {
	bm, cancel := newTestBatchManager(t)
	defer cancel()

	bm.RegisterDispatcher("utdispatcher", true, []core.MessageType{core.MessageTypePrivate},
		func(c context.Context, state *DispatchPayload) error {
			return nil
		},
		DispatcherOptions{BatchType: core.BatchTypePrivate},
	)
	group := fftypes.NewRandB32()
	_, err := bm.getProcessor(core.TransactionTypeContractInvokePin, core.MessageTypePrivate, group, "did:firefly:org/abcd", true)
	assert.NoError(t, err)

	batchID := fftypes.NewUUID()
	bp := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			ID: batchID,
		},
		TX: core.TransactionRef{
			Type: core.TransactionTypeContractInvokePin,
		},
	}
	msgid := fftypes.NewUUID()
	msg := &core.Message{
		Header: core.MessageHeader{
			ID:     msgid,
			Type:   core.MessageTypePrivate,
			TxType: core.TransactionTypeContractInvokePin,
			Group:  group,
			SignerRef: core.SignerRef{
				Author: "did:firefly:org/abcd",
			},
		},
	}
	batch := &core.Batch{
		BatchHeader: bp.BatchHeader,
		Payload: core.BatchPayload{
			Messages: []*core.Message{msg},
		},
	}

	mdi := bm.database.(*databasemocks.Plugin)
	mdm := bm.data.(*datamocks.Manager)
	mdi.On("GetBatchByID", context.Background(), "ns1", batchID).Return(bp, nil)
	mdm.On("HydrateBatch", context.Background(), bp).Return(batch, nil)

	err = bm.CancelBatch(context.Background(), batchID.String())
	assert.Regexp(t, "FF10468", err)

	mdi.AssertExpectations(t)
	mdm.AssertExpectations(t)
}
