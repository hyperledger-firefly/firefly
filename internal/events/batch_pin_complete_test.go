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
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/blockchain"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func sampleBatch(t *testing.T, batchType core.BatchType, txType core.TransactionType, data core.DataArray, blobs ...*core.Blob) *core.Batch {
	identity := core.SignerRef{Author: "signingOrg", Key: "0x12345"}
	msgType := core.MessageTypeBroadcast
	if batchType == core.BatchTypePrivate {
		msgType = core.MessageTypePrivate
	}
	for i, d := range data {
		var blob *core.Blob
		d.Namespace = "ns1"
		if len(blobs) > i {
			blob = blobs[i]
		}
		err := d.Seal(context.Background(), blob)
		assert.NoError(t, err)
	}
	msg := &core.Message{
		Header: core.MessageHeader{
			Namespace: "ns1",
			SignerRef: identity,
			ID:        fftypes.NewUUID(),
			Type:      msgType,
			TxType:    txType,
			Topics:    fftypes.FFStringArray{"topic1"},
		},
		Data: data.Refs(),
	}
	batch := &core.Batch{
		BatchHeader: core.BatchHeader{
			Namespace: "ns1",
			SignerRef: identity,
			Type:      batchType,
			ID:        fftypes.NewUUID(),
			Node:      fftypes.NewUUID(),
		},
		Payload: core.BatchPayload{
			TX: core.TransactionRef{
				ID:   fftypes.NewUUID(),
				Type: txType,
			},
			Messages: []*core.Message{msg},
			Data:     data,
		},
	}
	if batchType == core.BatchTypePrivate {
		batch.Group = fftypes.NewRandB32()
	}
	err := msg.Seal(context.Background())
	assert.NoError(t, err)
	bp, _ := batch.Confirmed()
	batch.Hash = fftypes.HashString(bp.Manifest.String())
	return batch
}

func TestBatchPinCompleteOkBroadcast(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	data := &core.Data{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"test"`)}
	batch := sampleBatch(t, core.BatchTypeBroadcast, core.TransactionTypeBatchPin, core.DataArray{data})
	batchPin := &blockchain.BatchPin{
		TransactionID:   batch.Payload.TX.ID,
		BatchID:         batch.ID,
		BatchPayloadRef: "Qmf412jQZiuVUtdgnB36FXFX7xg5V6KEbSJ4dpQuhkLyfD",
		Contexts:        []*fftypes.Bytes32{fftypes.NewRandB32()},
		Event: blockchain.Event{
			Name:           "BatchPin",
			BlockchainTXID: "0x12345",
			ProtocolID:     "10/20/30",
		},
	}

	batch.Hash = batch.Payload.Hash()
	batchPin.BatchHash = batch.Hash

	em.mth.On("PersistTransaction", mock.Anything, batchPin.TransactionID, core.TransactionTypeBatchPin, "0x12345").
		Return(false, fmt.Errorf("pop")).Once()
	em.mth.On("PersistTransaction", mock.Anything, batchPin.TransactionID, core.TransactionTypeBatchPin, "0x12345").
		Return(true, nil)

	rag := em.mdi.On("RunAsGroup", mock.Anything, mock.Anything).Return(nil)
	rag.RunFn = func(a mock.Arguments) {
		// Call through to persistBatch - the hash of our batch will be invalid,
		// which is swallowed without error as we cannot retry (it is logged of course)
		rag.ReturnArguments = mock.Arguments{
			a[1].(func(ctx context.Context) error)(a[0].(context.Context)),
		}
	}

	em.mth.On("InsertNewBlockchainEvents", mock.Anything, mock.MatchedBy(func(e []*core.BlockchainEvent) bool {
		return e[0].Name == batchPin.Event.Name
	})).Return([]*core.BlockchainEvent{{ID: fftypes.NewUUID()}}, nil).Once()
	em.mdi.On("InsertEvent", mock.Anything, mock.MatchedBy(func(e *core.Event) bool {
		return e.Type == core.EventTypeBlockchainEventReceived
	})).Return(nil).Once()
	em.mdi.On("InsertPins", mock.Anything, mock.Anything).Return(nil).Once()
	em.mdi.On("GetBatchByID", mock.Anything, "ns1", mock.Anything).Return(nil, nil)
	em.msd.On("InitiateDownloadBatch", mock.Anything, batchPin.TransactionID, batchPin.BatchPayloadRef, false).Return(nil)

	err := em.BlockchainEventBatch([]*blockchain.EventToDispatch{
		{
			Type: blockchain.EventTypeBatchPinComplete,
			BatchPinComplete: &blockchain.BatchPinCompleteEvent{
				Namespace: "ns1",
				Batch:     batchPin,
				SigningKey: &core.VerifierRef{
					Type:  core.VerifierTypeEthAddress,
					Value: "0x12345",
				},
			},
		},
	})
	assert.NoError(t, err)

}

func TestBatchPinCompleteOkBroadcastExistingBatch(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	data := &core.Data{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"test"`)}
	batch := sampleBatch(t, core.BatchTypeBroadcast, core.TransactionTypeBatchPin, core.DataArray{data})
	batchPin := &blockchain.BatchPin{
		TransactionID:   batch.Payload.TX.ID,
		BatchID:         batch.ID,
		BatchPayloadRef: "Qmf412jQZiuVUtdgnB36FXFX7xg5V6KEbSJ4dpQuhkLyfD",
		Contexts:        []*fftypes.Bytes32{fftypes.NewRandB32()},
		Event: blockchain.Event{
			Name:           "BatchPin",
			BlockchainTXID: "0x12345",
			ProtocolID:     "10/20/30",
		},
	}
	payloadBinary, jsonErr := json.Marshal(&batch.Payload)
	assert.NoError(t, jsonErr)
	batchPersisted := &core.BatchPersisted{
		TX:          batch.Payload.TX,
		BatchHeader: batch.BatchHeader,
		Manifest:    fftypes.JSONAnyPtr(string(payloadBinary)),
		Hash:        batch.Payload.Hash(),
	}

	batch.Hash = batch.Payload.Hash()
	batchPin.BatchHash = batch.Hash

	em.mth.On("PersistTransaction", mock.Anything, batchPin.TransactionID, core.TransactionTypeBatchPin, "0x12345").
		Return(false, fmt.Errorf("pop")).Once()
	em.mth.On("PersistTransaction", mock.Anything, batchPin.TransactionID, core.TransactionTypeBatchPin, "0x12345").
		Return(true, nil)

	rag := em.mdi.On("RunAsGroup", mock.Anything, mock.Anything).Return(nil)
	rag.RunFn = func(a mock.Arguments) {
		// Call through to persistBatch - the hash of our batch will be invalid,
		// which is swallowed without error as we cannot retry (it is logged of course)
		rag.ReturnArguments = mock.Arguments{
			a[1].(func(ctx context.Context) error)(a[0].(context.Context)),
		}
	}

	em.mth.On("InsertNewBlockchainEvents", mock.Anything, mock.MatchedBy(func(e []*core.BlockchainEvent) bool {
		return e[0].Name == batchPin.Event.Name
	})).Return([]*core.BlockchainEvent{{ID: fftypes.NewUUID()}}, nil).Once()
	em.mdi.On("InsertEvent", mock.Anything, mock.MatchedBy(func(e *core.Event) bool {
		return e.Type == core.EventTypeBlockchainEventReceived
	})).Return(nil).Once()
	em.mdi.On("InsertPins", mock.Anything, mock.Anything).Return(nil).Once()
	em.mdi.On("GetBatchByID", mock.Anything, "ns1", mock.Anything).Return(batchPersisted, nil)

	err := em.BlockchainEventBatch([]*blockchain.EventToDispatch{
		{
			Type: blockchain.EventTypeBatchPinComplete,
			BatchPinComplete: &blockchain.BatchPinCompleteEvent{
				Namespace: "ns1",
				Batch:     batchPin,
				SigningKey: &core.VerifierRef{
					Type:  core.VerifierTypeEthAddress,
					Value: "0x12345",
				},
			},
		},
	})
	assert.NoError(t, err)

}

func TestBatchPinCompleteOkPrivate(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	batchPin := &blockchain.BatchPin{
		TransactionID: fftypes.NewUUID(),
		BatchID:       fftypes.NewUUID(),
		Contexts:      []*fftypes.Bytes32{fftypes.NewRandB32()},
		Event: blockchain.Event{
			BlockchainTXID: "0x12345",
			ProtocolID:     "10/20/30",
		},
	}

	em.mth.On("PersistTransaction", mock.Anything, batchPin.TransactionID, core.TransactionTypeBatchPin, "0x12345").Return(true, nil)

	em.mdi.On("RunAsGroup", mock.Anything, mock.Anything).Return(nil)
	em.mdi.On("InsertPins", mock.Anything, mock.Anything).Return(fmt.Errorf("These pins have been seen before")) // simulate replay fallback
	em.mdi.On("UpsertPin", mock.Anything, mock.Anything).Return(nil)
	em.mth.On("InsertNewBlockchainEvents", mock.Anything, mock.Anything).Return([]*core.BlockchainEvent{{ID: fftypes.NewUUID()}}, nil)
	em.mdi.On("InsertEvent", mock.Anything, mock.Anything).Return(nil)
	em.mdi.On("GetBatchByID", mock.Anything, "ns1", mock.Anything).Return(nil, nil)

	err := em.BlockchainEventBatch([]*blockchain.EventToDispatch{
		{
			Type: blockchain.EventTypeBatchPinComplete,
			BatchPinComplete: &blockchain.BatchPinCompleteEvent{
				Namespace: "ns1",
				Batch:     batchPin,
				SigningKey: &core.VerifierRef{
					Type:  core.VerifierTypeEthAddress,
					Value: "0xffffeeee",
				},
			},
		},
	})
	assert.NoError(t, err)

	// Call through to persistBatch - the hash of our batch will be invalid,
	// which is swallowed without error as we cannot retry (it is logged of course)
	fn := em.mdi.Calls[1].Arguments[1].(func(ctx context.Context) error)
	err = fn(context.Background())
	assert.NoError(t, err)

}

func TestBatchPinCompleteInsertPinsFail(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	em.cancel()

	batchPin := &blockchain.BatchPin{
		TransactionID: fftypes.NewUUID(),
		BatchID:       fftypes.NewUUID(),
		Contexts:      []*fftypes.Bytes32{fftypes.NewRandB32()},
		Event: blockchain.Event{
			BlockchainTXID: "0x12345",
			ProtocolID:     "10/20/30",
		},
	}

	em.mth.On("PersistTransaction", mock.Anything, batchPin.TransactionID, core.TransactionTypeBatchPin, "0x12345").Return(true, nil)

	em.mdi.On("RunAsGroup", mock.Anything, mock.Anything).Return(nil)
	em.mdi.On("InsertPins", mock.Anything, mock.Anything).Return(fmt.Errorf("optimization miss"))
	em.mdi.On("UpsertPin", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
	em.mth.On("InsertNewBlockchainEvents", mock.Anything, mock.MatchedBy(func(e []*core.BlockchainEvent) bool {
		return e[0].Name == batchPin.Event.Name
	})).Return([]*core.BlockchainEvent{{ID: fftypes.NewUUID()}}, nil).Once()
	em.mdi.On("InsertEvent", mock.Anything, mock.Anything).Return(nil)

	err := em.BlockchainEventBatch([]*blockchain.EventToDispatch{
		{
			Type: blockchain.EventTypeBatchPinComplete,
			BatchPinComplete: &blockchain.BatchPinCompleteEvent{
				Namespace: "ns1",
				Batch:     batchPin,
				SigningKey: &core.VerifierRef{
					Type:  core.VerifierTypeEthAddress,
					Value: "0xffffeeee",
				},
			},
		},
	})
	assert.Regexp(t, "FF00154", err)

}

func TestBatchPinCompleteGetBatchByIDFails(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	em.cancel()

	batchPin := &blockchain.BatchPin{
		TransactionID: fftypes.NewUUID(),
		BatchID:       fftypes.NewUUID(),
		Contexts:      []*fftypes.Bytes32{fftypes.NewRandB32()},
		Event: blockchain.Event{
			BlockchainTXID: "0x12345",
			ProtocolID:     "10/20/30",
		},
	}

	em.mth.On("PersistTransaction", mock.Anything, batchPin.TransactionID, core.TransactionTypeBatchPin, "0x12345").Return(true, nil)

	em.mdi.On("RunAsGroup", mock.Anything, mock.Anything).Return(nil)
	em.mdi.On("InsertPins", mock.Anything, mock.Anything).Return(nil)
	em.mth.On("InsertNewBlockchainEvents", mock.Anything, mock.MatchedBy(func(e []*core.BlockchainEvent) bool {
		return e[0].Name == batchPin.Event.Name
	})).Return([]*core.BlockchainEvent{{ID: fftypes.NewUUID()}}, nil).Once()
	em.mdi.On("InsertEvent", mock.Anything, mock.Anything).Return(nil)
	em.mdi.On("GetBatchByID", mock.Anything, "ns1", mock.Anything).Return(nil, fmt.Errorf("batch lookup failed"))

	err := em.BlockchainEventBatch([]*blockchain.EventToDispatch{
		{
			Type: blockchain.EventTypeBatchPinComplete,
			BatchPinComplete: &blockchain.BatchPinCompleteEvent{
				Namespace: "ns1",
				Batch:     batchPin,
				SigningKey: &core.VerifierRef{
					Type:  core.VerifierTypeEthAddress,
					Value: "0xffffeeee",
				},
			},
		},
	})
	assert.Regexp(t, "FF00154", err)

}

func TestSequencedBroadcastInitiateDownloadFail(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	batchPin := &blockchain.BatchPin{
		TransactionID:   fftypes.NewUUID(),
		BatchID:         fftypes.NewUUID(),
		BatchPayloadRef: "Qmf412jQZiuVUtdgnB36FXFX7xg5V6KEbSJ4dpQuhkLyfD",
		Contexts:        []*fftypes.Bytes32{fftypes.NewRandB32()},
		Event: blockchain.Event{
			BlockchainTXID: "0x12345",
			ProtocolID:     "10/20/30",
		},
	}

	em.cancel() // to avoid retry

	em.mth.On("PersistTransaction", mock.Anything, batchPin.TransactionID, core.TransactionTypeBatchPin, "0x12345").Return(true, nil)

	em.mth.On("InsertNewBlockchainEvents", mock.Anything, mock.MatchedBy(func(e []*core.BlockchainEvent) bool {
		return e[0].Name == batchPin.Event.Name
	})).Return([]*core.BlockchainEvent{{ID: fftypes.NewUUID()}}, nil).Once()
	em.mdi.On("InsertEvent", mock.Anything, mock.Anything).Return(nil)
	em.mdi.On("InsertPins", mock.Anything, mock.Anything).Return(nil)
	em.mdi.On("GetBatchByID", mock.Anything, "ns1", mock.Anything).Return(nil, nil)
	em.msd.On("InitiateDownloadBatch", mock.Anything, batchPin.TransactionID, batchPin.BatchPayloadRef, false).Return(fmt.Errorf("pop"))

	err := em.BlockchainEventBatch([]*blockchain.EventToDispatch{
		{
			Type: blockchain.EventTypeBatchPinComplete,
			BatchPinComplete: &blockchain.BatchPinCompleteEvent{
				Namespace: "ns1",
				Batch:     batchPin,
				SigningKey: &core.VerifierRef{
					Type:  core.VerifierTypeEthAddress,
					Value: "0xffffeeee",
				},
			},
		},
	})
	assert.Regexp(t, "FF00154", err)
}

func TestBatchPinCompleteNoTX(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	batch := &blockchain.BatchPin{}

	err := em.BlockchainEventBatch([]*blockchain.EventToDispatch{
		{
			Type: blockchain.EventTypeBatchPinComplete,
			BatchPinComplete: &blockchain.BatchPinCompleteEvent{
				Namespace: "ns1",
				Batch:     batch,
				SigningKey: &core.VerifierRef{
					Type:  core.VerifierTypeEthAddress,
					Value: "0x12345",
				},
			},
		},
	})
	assert.NoError(t, err)
}

func TestBatchPinCompleteWrongNamespace(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	batch := &blockchain.BatchPin{
		TransactionID: fftypes.NewUUID(),
		Event: blockchain.Event{
			BlockchainTXID: "0x12345",
		},
	}

	err := em.BlockchainEventBatch([]*blockchain.EventToDispatch{
		{
			Type: blockchain.EventTypeBatchPinComplete,
			BatchPinComplete: &blockchain.BatchPinCompleteEvent{
				Namespace: "ns2",
				Batch:     batch,
				SigningKey: &core.VerifierRef{
					Type:  core.VerifierTypeEthAddress,
					Value: "0xffffeeee",
				},
			},
		},
	})
	assert.NoError(t, err)
}

func TestBatchPinCompleteNonMultiparty(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	em.multiparty = nil

	batch := &blockchain.BatchPin{
		TransactionID: fftypes.NewUUID(),
		Event: blockchain.Event{
			BlockchainTXID: "0x12345",
		},
	}

	err := em.BlockchainEventBatch([]*blockchain.EventToDispatch{
		{
			Type: blockchain.EventTypeBatchPinComplete,
			BatchPinComplete: &blockchain.BatchPinCompleteEvent{
				Namespace: "ns1",
				Batch:     batch,
				SigningKey: &core.VerifierRef{
					Type:  core.VerifierTypeEthAddress,
					Value: "0x12345",
				},
			},
		},
	})
	assert.NoError(t, err)
}

func TestPersistBatchMissingID(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	batch, valid, err := em.persistBatch(context.Background(), &core.Batch{})
	assert.False(t, valid)
	assert.Nil(t, batch)
	assert.NoError(t, err)
}

func TestPersistBatchAuthorResolveFail(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	batchHash := fftypes.NewRandB32()
	batch := &core.Batch{
		BatchHeader: core.BatchHeader{
			ID: fftypes.NewUUID(),
			SignerRef: core.SignerRef{
				Author: "author1",
				Key:    "0x12345",
			},
		},
		Hash: batchHash,
		Payload: core.BatchPayload{
			TX: core.TransactionRef{
				Type: core.TransactionTypeBatchPin,
				ID:   fftypes.NewUUID(),
			},
		},
	}
	batch.Hash = batch.Payload.Hash()
	_, valid, err := em.persistBatch(context.Background(), batch)
	assert.NoError(t, err) // retryable
	assert.False(t, valid)
}

func TestPersistBatchBadAuthor(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	batchHash := fftypes.NewRandB32()
	batch := &core.Batch{
		BatchHeader: core.BatchHeader{
			ID: fftypes.NewUUID(),
			SignerRef: core.SignerRef{
				Author: "author1",
				Key:    "0x12345",
			},
		},
		Hash: batchHash,
		Payload: core.BatchPayload{
			TX: core.TransactionRef{
				Type: core.TransactionTypeBatchPin,
				ID:   fftypes.NewUUID(),
			},
		},
	}
	batch.Hash = batch.Payload.Hash()
	_, valid, err := em.persistBatch(context.Background(), batch)
	assert.NoError(t, err)
	assert.False(t, valid)
}

func TestPersistBatchMismatchChainHash(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	batch := &core.Batch{
		BatchHeader: core.BatchHeader{
			ID: fftypes.NewUUID(),
			SignerRef: core.SignerRef{
				Author: "author1",
				Key:    "0x12345",
			},
		},
		Hash: fftypes.NewRandB32(),
		Payload: core.BatchPayload{
			TX: core.TransactionRef{
				Type: core.TransactionTypeBatchPin,
				ID:   fftypes.NewUUID(),
			},
		},
	}
	batch.Hash = batch.Payload.Hash()
	_, valid, err := em.persistBatch(context.Background(), batch)
	assert.NoError(t, err)
	assert.False(t, valid)
}

func TestPersistBatchBadHash(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	data := &core.Data{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"test"`)}
	batch := sampleBatch(t, core.BatchTypeBroadcast, core.TransactionTypeBatchPin, core.DataArray{data})
	batch.Hash = fftypes.NewRandB32()

	bp, valid, err := em.persistBatch(context.Background(), batch)
	assert.False(t, valid)
	assert.Nil(t, bp)
	assert.NoError(t, err)
}

func TestPersistBatchNoData(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	batch := &core.Batch{
		BatchHeader: core.BatchHeader{
			ID: fftypes.NewUUID(),
			SignerRef: core.SignerRef{
				Author: "author1",
				Key:    "0x12345",
			},
		},
		Payload: core.BatchPayload{
			TX: core.TransactionRef{
				Type: core.TransactionTypeBatchPin,
				ID:   fftypes.NewUUID(),
			},
		},
	}
	batch.Hash = fftypes.NewRandB32()

	bp, valid, err := em.persistBatch(context.Background(), batch)
	assert.False(t, valid)
	assert.Nil(t, bp)
	assert.NoError(t, err)
}

func TestPersistBatchUpsertBatchFail(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	data := &core.Data{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"test"`)}
	batch := sampleBatch(t, core.BatchTypeBroadcast, core.TransactionTypeBatchPin, core.DataArray{data})

	em.mdi.On("InsertOrGetBatch", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))

	bp, valid, err := em.persistBatch(context.Background(), batch)
	assert.Nil(t, bp)
	assert.False(t, valid)
	assert.EqualError(t, err, "pop")
}

func TestPersistBatchSwallowBadData(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	batch := &core.Batch{
		BatchHeader: core.BatchHeader{
			ID: fftypes.NewUUID(),
			SignerRef: core.SignerRef{
				Author: "author1",
				Key:    "0x12345",
			},
			Namespace: "ns1",
		},
		Payload: core.BatchPayload{
			TX: core.TransactionRef{
				Type: core.TransactionTypeBatchPin,
				ID:   fftypes.NewUUID(),
			},
			Messages: []*core.Message{nil},
			Data:     core.DataArray{nil},
		},
	}
	batch.Hash = batch.Payload.Hash()

	em.mdi.On("InsertOrGetBatch", mock.Anything, mock.Anything).Return(nil, nil)

	bp, valid, err := em.persistBatch(context.Background(), batch)
	assert.False(t, valid)
	assert.NoError(t, err)
	assert.Nil(t, bp)
}

func TestPersistBatchGoodDataUpsertOptimizeFail(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	data := &core.Data{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"test"`)}
	batch := sampleBatch(t, core.BatchTypeBroadcast, core.TransactionTypeBatchPin, core.DataArray{data})

	em.mdi.On("InsertOrGetBatch", mock.Anything, mock.Anything).Return(nil, nil)
	em.mdi.On("InsertDataArray", mock.Anything, mock.Anything).Return(fmt.Errorf("optimzation miss"))
	em.mdi.On("UpsertData", mock.Anything, mock.Anything, database.UpsertOptimizationExisting).Return(fmt.Errorf("pop"))

	em.mim.On("GetLocalNode", mock.Anything).Return(testNode, nil)

	bp, valid, err := em.persistBatch(context.Background(), batch)
	assert.Nil(t, bp)
	assert.False(t, valid)
	assert.EqualError(t, err, "pop")
}

func TestPersistBatchGoodDataMessageFail(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	data := &core.Data{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"test"`)}
	batch := sampleBatch(t, core.BatchTypeBroadcast, core.TransactionTypeBatchPin, core.DataArray{data})

	em.mdi.On("InsertOrGetBatch", mock.Anything, mock.Anything).Return(nil, nil)
	em.mdi.On("InsertDataArray", mock.Anything, mock.Anything).Return(nil)
	em.mdi.On("InsertMessages", mock.Anything, mock.Anything, mock.AnythingOfType("database.PostCompletionHook")).Return(fmt.Errorf("optimzation miss")).Once()
	em.mdi.On("GetMessageIDs", mock.Anything, "ns1", mock.Anything).Return([]*core.IDAndSequence{}, nil)
	em.mdi.On("InsertMessages", mock.Anything, mock.Anything, mock.AnythingOfType("database.PostCompletionHook")).Return(fmt.Errorf("pop")).Once()

	em.mim.On("GetLocalNode", mock.Anything).Return(testNode, nil)

	bp, valid, err := em.persistBatch(context.Background(), batch)
	assert.False(t, valid)
	assert.Nil(t, bp)
	assert.EqualError(t, err, "pop")
}

func TestPersistBatchGoodMessageAuthorMismatch(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	data := &core.Data{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"test"`)}
	batch := sampleBatch(t, core.BatchTypeBroadcast, core.TransactionTypeBatchPin, core.DataArray{data})
	batch.Payload.Messages[0].Header.Key = "0x9999999"
	batch.Payload.Messages[0].Header.DataHash = batch.Payload.Messages[0].Data.Hash()
	batch.Payload.Messages[0].Hash = batch.Payload.Messages[0].Header.Hash()
	batch.Hash = batch.Payload.Hash()

	em.mdi.On("InsertOrGetBatch", mock.Anything, mock.Anything).Return(nil, nil)

	bp, valid, err := em.persistBatch(context.Background(), batch)
	assert.Nil(t, bp)
	assert.False(t, valid)
	assert.NoError(t, err)
}

func TestPersistBatchDataNilData(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	batch := &core.Batch{
		BatchHeader: core.BatchHeader{
			ID: fftypes.NewUUID(),
		},
	}
	data := &core.Data{
		ID: fftypes.NewUUID(),
	}
	valid := em.validateBatchData(context.Background(), batch, 0, data)
	assert.False(t, valid)
}

func TestPersistBatchDataBadHash(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	data := &core.Data{
		ID:    fftypes.NewUUID(),
		Value: fftypes.JSONAnyPtr(`"test"`),
	}
	batch := sampleBatch(t, core.BatchTypeBroadcast, core.TransactionTypeBatchPin, core.DataArray{data})
	batch.Payload.Data[0].Hash = fftypes.NewRandB32()
	valid := em.validateBatchData(context.Background(), batch, 0, data)
	assert.False(t, valid)
}

func TestPersistBatchDataOk(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	data := &core.Data{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"test"`)}
	batch := sampleBatch(t, core.BatchTypeBroadcast, core.TransactionTypeBatchPin, core.DataArray{data})

	valid := em.validateBatchData(context.Background(), batch, 0, data)
	assert.True(t, valid)
}

func TestPersistBatchDataWithPublicAlreaydDownloadedOk(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	blob := &core.Blob{
		Hash: fftypes.NewRandB32(),
		Size: 12345,
	}
	data := &core.Data{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"test"`), Blob: &core.BlobRef{
		Hash:   blob.Hash,
		Size:   12345,
		Name:   "myfile.txt",
		Public: "ref1",
	}}
	batch := sampleBatch(t, core.BatchTypeBroadcast, core.TransactionTypeBatchPin, core.DataArray{data}, blob)

	em.mdi.On("GetBlobs", mock.Anything, mock.Anything, mock.Anything).Return([]*core.Blob{blob}, nil, nil)

	valid, err := em.checkAndInitiateBlobDownloads(context.Background(), batch, 0, data)
	assert.Nil(t, err)
	assert.True(t, valid)
}

func TestPersistBatchDataWithPublicInitiateDownload(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	blob := &core.Blob{
		Hash: fftypes.NewRandB32(),
		Size: 12345,
	}
	data := &core.Data{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"test"`), Blob: &core.BlobRef{
		Hash:   blob.Hash,
		Size:   12345,
		Name:   "myfile.txt",
		Public: "ref1",
	}}
	batch := sampleBatch(t, core.BatchTypeBroadcast, core.TransactionTypeBatchPin, core.DataArray{data}, blob)

	em.mdi.On("GetBlobs", mock.Anything, mock.Anything, mock.Anything).Return([]*core.Blob{}, nil, nil)

	em.msd.On("InitiateDownloadBlob", mock.Anything, batch.Payload.TX.ID, data.ID, "ref1", false).Return(nil)

	valid, err := em.checkAndInitiateBlobDownloads(context.Background(), batch, 0, data)
	assert.Nil(t, err)
	assert.True(t, valid)
}

func TestPersistBatchDataWithPublicInitiateDownloadFail(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	blob := &core.Blob{
		Hash: fftypes.NewRandB32(),
		Size: 12345,
	}
	data := &core.Data{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"test"`), Blob: &core.BlobRef{
		Hash:   blob.Hash,
		Size:   12345,
		Name:   "myfile.txt",
		Public: "ref1",
	}}
	batch := sampleBatch(t, core.BatchTypeBroadcast, core.TransactionTypeBatchPin, core.DataArray{data}, blob)

	em.mdi.On("GetBlobs", mock.Anything, mock.Anything, mock.Anything).Return([]*core.Blob{}, nil, nil)

	em.msd.On("InitiateDownloadBlob", mock.Anything, batch.Payload.TX.ID, data.ID, "ref1", false).Return(fmt.Errorf("pop"))

	valid, err := em.checkAndInitiateBlobDownloads(context.Background(), batch, 0, data)
	assert.Regexp(t, "pop", err)
	assert.False(t, valid)
}

func TestPersistBatchDataWithBlobGetBlobFail(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)

	blob := &core.Blob{
		Hash: fftypes.NewRandB32(),
		Size: 12345,
	}
	data := &core.Data{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"test"`), Blob: &core.BlobRef{
		Hash:   blob.Hash,
		Size:   12345,
		Name:   "myfile.txt",
		Public: "ref1",
	}}
	batch := sampleBatch(t, core.BatchTypeBroadcast, core.TransactionTypeBatchPin, core.DataArray{data}, blob)

	em.mdi.On("GetBlobs", mock.Anything, mock.Anything, mock.Anything).Return([]*core.Blob{}, nil, fmt.Errorf("pop"))

	valid, err := em.checkAndInitiateBlobDownloads(context.Background(), batch, 0, data)
	assert.Regexp(t, "pop", err)
	assert.False(t, valid)
}

func TestPersistBatchMessageNilData(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	batch := &core.Batch{
		BatchHeader: core.BatchHeader{
			ID: fftypes.NewUUID(),
		},
	}
	msg := &core.Message{
		Header: core.MessageHeader{
			ID: fftypes.NewUUID(),
		},
	}
	valid := em.validateBatchMessage(context.Background(), batch, 0, msg)
	assert.False(t, valid)
}

func TestPersistBatchMessageOK(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	batch := sampleBatch(t, core.BatchTypeBroadcast, core.TransactionTypeBatchPin, core.DataArray{})

	valid := em.validateBatchMessage(context.Background(), batch, 0, batch.Payload.Messages[0])
	assert.True(t, valid)
}
