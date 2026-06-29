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

package data

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func newTestMessageWriter(t *testing.T) *messageWriter {
	return newTestMessageWriterConf(t, &messageWriterConf{
		workerCount:  1,
		batchTimeout: 100 * time.Millisecond,
		maxInserts:   200,
	}, &database.Capabilities{Concurrency: true})
}

func newTestMessageWriterNoConcurrency(t *testing.T) *messageWriter {
	return newTestMessageWriterConf(t, &messageWriterConf{workerCount: 1}, &database.Capabilities{Concurrency: false})
}

func newTestMessageWriterConf(t *testing.T, conf *messageWriterConf, dbCapabilities *database.Capabilities) *messageWriter {
	mdi := &databasemocks.Plugin{}
	mdi.On("Capabilities").Return(dbCapabilities)
	return newMessageWriter(context.Background(), mdi, conf)
}

func TestNewMessageWriterNoConcurrency(t *testing.T) {
	mw := newTestMessageWriterNoConcurrency(t)
	assert.Zero(t, mw.conf.workerCount)
}

func TestWriteNewMessageClosed(t *testing.T) {
	mw := newTestMessageWriter(t)
	mw.close()
	err := mw.WriteNewMessage(mw.ctx, &NewMessage{
		Message: &core.MessageInOut{},
	})
	assert.Regexp(t, "FF00154", err)
}

func TestWriteDataClosed(t *testing.T) {
	mw := newTestMessageWriter(t)
	mw.close()
	err := mw.WriteData(mw.ctx, &core.Data{})
	assert.Regexp(t, "FF00154", err)
}

func TestWriteNewMessageSyncFallback(t *testing.T) {
	mw := newTestMessageWriterNoConcurrency(t)
	customCtx := context.WithValue(context.Background(), "dbtx", "on this context")

	msg1 := &core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				ID: fftypes.NewUUID(),
			},
		},
	}
	data1 := &core.Data{ID: fftypes.NewUUID()}

	mdi := mw.database.(*databasemocks.Plugin)
	mdi.On("RunAsGroup", customCtx, mock.Anything).Run(func(args mock.Arguments) {
		err := args[1].(func(context.Context) error)(customCtx)
		assert.NoError(t, err)
	}).Return(nil)
	mdi.On("InsertMessages", customCtx, []*core.Message{&msg1.Message}).Return(nil)
	mdi.On("InsertDataArray", customCtx, core.DataArray{data1}).Return(nil)

	err := mw.WriteNewMessage(customCtx, &NewMessage{
		Message: msg1,
		NewData: core.DataArray{data1},
	})

	assert.NoError(t, err)
}

func TestWriteDataSyncFallback(t *testing.T) {
	mw := newTestMessageWriterNoConcurrency(t)
	customCtx := context.WithValue(context.Background(), "dbtx", "on this context")

	data1 := &core.Data{ID: fftypes.NewUUID()}

	mdi := mw.database.(*databasemocks.Plugin)
	mdi.On("UpsertData", customCtx, data1, database.UpsertOptimizationNew).Return(nil)

	err := mw.WriteData(customCtx, data1)

	assert.NoError(t, err)
}

func TestWriteMessagesInsertMessagesFail(t *testing.T) {
	mw := newTestMessageWriterNoConcurrency(t)

	msg1 := &core.Message{
		Header: core.MessageHeader{
			ID: fftypes.NewUUID(),
		},
	}

	mdi := mw.database.(*databasemocks.Plugin)
	mdi.On("InsertMessages", mw.ctx, []*core.Message{msg1}).Return(fmt.Errorf("pop"))

	err := mw.writeMessages(mw.ctx, []*core.Message{msg1}, core.DataArray{})

	assert.Regexp(t, "pop", err)
}

func TestWriteMessagesInsertDataArrayFail(t *testing.T) {
	mw := newTestMessageWriterNoConcurrency(t)

	data1 := &core.Data{ID: fftypes.NewUUID()}

	mdi := mw.database.(*databasemocks.Plugin)
	mdi.On("InsertDataArray", mw.ctx, core.DataArray{data1}).Return(fmt.Errorf("pop"))

	err := mw.writeMessages(mw.ctx, []*core.Message{}, core.DataArray{data1})

	assert.Regexp(t, "pop", err)
}

func TestPersistMWBatchIdempotencyAllFail(t *testing.T) {
	mw := newTestMessageWriterNoConcurrency(t)
	customCtx := context.WithValue(context.Background(), "dbtx", "on this context")

	m1ID := fftypes.NewUUID()
	m1d := core.DataArray{
		&core.Data{Namespace: "ns1", ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32()},
		&core.Data{Namespace: "ns1", ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32()},
	}
	m1 := &core.Message{
		Header:         core.MessageHeader{Namespace: "ns1", ID: m1ID},
		Data:           m1d.Refs(),
		IdempotencyKey: "idem1",
	}
	c1 := make(chan error, 1)

	m2ID := fftypes.NewUUID()
	m2d := core.DataArray{
		&core.Data{Namespace: "ns1", ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32()},
		&core.Data{Namespace: "ns1", ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32()},
	}
	m2 := &core.Message{
		Header:         core.MessageHeader{Namespace: "ns1", ID: m2ID},
		Data:           m2d.Refs(),
		IdempotencyKey: "idem2",
	}
	c2 := make(chan error, 1)

	mdi := mw.database.(*databasemocks.Plugin)
	rag := mdi.On("RunAsGroup", mock.Anything, mock.Anything)
	rag.Run(func(args mock.Arguments) {
		rag.Return(args[1].(func(context.Context) error)(customCtx))
	}).Return(nil)
	mdi.On("InsertDataArray", customCtx, append(m1d, m2d...)).Return(nil)
	mdi.On("InsertMessages", customCtx, []*core.Message{m1, m2}).Return(fmt.Errorf("various keys were not unique"))
	mdi.On("GetMessages", mock.Anything, "ns1", mock.MatchedBy(func(f ffapi.Filter) bool {
		ff, _ := f.Finalize()
		return strings.Contains(ff.String(), "idem1")
	})).Return([]*core.Message{m1}, nil, nil)
	mdi.On("GetMessages", mock.Anything, "ns1", mock.MatchedBy(func(f ffapi.Filter) bool {
		ff, _ := f.Finalize()
		return strings.Contains(ff.String(), "idem2")
	})).Return([]*core.Message{m2}, nil, nil)

	mw.persistMWBatch(&messageWriterBatch{
		messages: []*core.Message{m1, m2},
		data:     append(m1d, m2d...),
		listeners: map[fftypes.UUID]chan error{
			*m1ID: c1,
			*m2ID: c2,
		},
	})

	mdi.AssertExpectations(t)

	assert.Regexp(t, "FF10430.*idem1", <-c1)
	assert.Regexp(t, "FF10430.*idem2", <-c2)
}

func TestPersistMWBatchHalfFailResubmit(t *testing.T) {
	mw := newTestMessageWriterNoConcurrency(t)
	customCtx := context.WithValue(context.Background(), "dbtx", "on this context")

	m1ID := fftypes.NewUUID()
	m1d := core.DataArray{
		&core.Data{Namespace: "ns1", ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32()},
		&core.Data{Namespace: "ns1", ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32()},
	}
	m1 := &core.Message{
		Header:         core.MessageHeader{Namespace: "ns1", ID: m1ID},
		Data:           m1d.Refs(),
		IdempotencyKey: "idem1",
	}
	c1 := make(chan error, 1)

	m2ID := fftypes.NewUUID()
	m2d := core.DataArray{
		&core.Data{Namespace: "ns1", ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32()},
		&core.Data{Namespace: "ns1", ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32()},
	}
	m2 := &core.Message{
		Header:         core.MessageHeader{Namespace: "ns1", ID: m2ID},
		Data:           m2d.Refs(),
		IdempotencyKey: "idem2",
	}
	c2 := make(chan error, 1)

	mdi := mw.database.(*databasemocks.Plugin)
	rag := mdi.On("RunAsGroup", mock.Anything, mock.Anything)
	rag.Run(func(args mock.Arguments) {
		rag.Return(args[1].(func(context.Context) error)(customCtx))
	}).Return(nil)
	mdi.On("InsertDataArray", customCtx, append(m1d, m2d...)).Return(nil)
	mdi.On("InsertMessages", customCtx, []*core.Message{m1, m2}).Return(fmt.Errorf("various keys were not unique"))
	mdi.On("GetMessages", mock.Anything, "ns1", mock.MatchedBy(func(f ffapi.Filter) bool {
		ff, _ := f.Finalize()
		return strings.Contains(ff.String(), "idem1")
	})).Return([]*core.Message{}, nil, nil) // no result
	mdi.On("GetMessages", mock.Anything, "ns1", mock.MatchedBy(func(f ffapi.Filter) bool {
		ff, _ := f.Finalize()
		return strings.Contains(ff.String(), "idem2")
	})).Return([]*core.Message{m2}, nil, nil) // found result
	mdi.On("InsertDataArray", customCtx, m1d).Return(nil)
	mdi.On("InsertMessages", customCtx, []*core.Message{m1}).Return(nil) // resubmit reduced batch

	mw.persistMWBatch(&messageWriterBatch{
		messages: []*core.Message{m1, m2},
		data:     append(m1d, m2d...),
		listeners: map[fftypes.UUID]chan error{
			*m1ID: c1,
			*m2ID: c2,
		},
	})

	mdi.AssertExpectations(t)

	assert.Nil(t, <-c1)
	assert.Regexp(t, "FF10430.*idem2", <-c2)
}

func TestPersistMWBatchFailIdemCheck(t *testing.T) {
	mw := newTestMessageWriterNoConcurrency(t)
	customCtx := context.WithValue(context.Background(), "dbtx", "on this context")

	m1ID := fftypes.NewUUID()
	m1d := core.DataArray{
		&core.Data{Namespace: "ns1", ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32()},
		&core.Data{Namespace: "ns1", ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32()},
	}
	m1 := &core.Message{
		Header:         core.MessageHeader{Namespace: "ns1", ID: m1ID},
		Data:           m1d.Refs(),
		IdempotencyKey: "idem1",
	}
	c1 := make(chan error, 1)

	mdi := mw.database.(*databasemocks.Plugin)
	rag := mdi.On("RunAsGroup", mock.Anything, mock.Anything)
	rag.Run(func(args mock.Arguments) {
		rag.Return(args[1].(func(context.Context) error)(customCtx))
	}).Return(nil)
	mdi.On("InsertDataArray", customCtx, m1d).Return(nil)
	mdi.On("InsertMessages", customCtx, []*core.Message{m1}).Return(fmt.Errorf("failure1... which leads to..."))
	mdi.On("GetMessages", mock.Anything, "ns1", mock.MatchedBy(func(f ffapi.Filter) bool {
		ff, _ := f.Finalize()
		return strings.Contains(ff.String(), "idem1")
	})).Return([]*core.Message{}, nil, fmt.Errorf("failure1"))

	mw.persistMWBatch(&messageWriterBatch{
		messages: []*core.Message{m1},
		data:     m1d,
		listeners: map[fftypes.UUID]chan error{
			*m1ID: c1,
		},
	})

	mdi.AssertExpectations(t)

	assert.Regexp(t, "failure1" /* we get the first failure, not the idempotency check failure */, <-c1)
}

func TestPersistMWBatchIdempotencySkipNoIdem(t *testing.T) {
	mw := newTestMessageWriterNoConcurrency(t)
	customCtx := context.WithValue(context.Background(), "dbtx", "on this context")

	m1ID := fftypes.NewUUID()
	m1d := core.DataArray{
		&core.Data{Namespace: "ns1", ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32()},
		&core.Data{Namespace: "ns1", ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32()},
	}
	m1 := &core.Message{
		Header: core.MessageHeader{Namespace: "ns1", ID: m1ID},
		Data:   m1d.Refs(),
	}
	c1 := make(chan error, 1)

	mdi := mw.database.(*databasemocks.Plugin)
	rag := mdi.On("RunAsGroup", mock.Anything, mock.Anything)
	rag.Run(func(args mock.Arguments) {
		rag.Return(args[1].(func(context.Context) error)(customCtx))
	}).Return(nil)
	mdi.On("InsertDataArray", customCtx, m1d).Return(nil)
	mdi.On("InsertMessages", customCtx, []*core.Message{m1}).Return(fmt.Errorf("not idempotency related"))

	mw.persistMWBatch(&messageWriterBatch{
		messages: []*core.Message{m1},
		data:     m1d,
		listeners: map[fftypes.UUID]chan error{
			*m1ID: c1,
		},
	})

	mdi.AssertExpectations(t)

	assert.Regexp(t, "not idempotency related", <-c1)
}

func TestWriteMessageNoWorkersIdempotencyDuplicate(t *testing.T) {
	mw := newTestMessageWriterNoConcurrency(t)
	customCtx := context.WithValue(context.Background(), "dbtx", "on this context")
	mw.conf.workerCount = 0

	m1ID := fftypes.NewUUID()
	m1 := &core.MessageInOut{
		Message: core.Message{
			Header:         core.MessageHeader{Namespace: "ns1", ID: m1ID},
			IdempotencyKey: "idem1",
		},
	}

	mdi := mw.database.(*databasemocks.Plugin)
	rag := mdi.On("RunAsGroup", mock.Anything, mock.Anything)
	rag.Run(func(args mock.Arguments) {
		rag.Return(args[1].(func(context.Context) error)(customCtx))
	}).Return(nil)
	mdi.On("InsertMessages", customCtx, []*core.Message{&m1.Message}).Return(fmt.Errorf("failure1... which leads to..."))
	mdi.On("GetMessages", mock.Anything, "ns1", mock.MatchedBy(func(f ffapi.Filter) bool {
		ff, _ := f.Finalize()
		return strings.Contains(ff.String(), "idem1")
	})).Return([]*core.Message{
		{Header: core.MessageHeader{ID: fftypes.NewUUID()}, IdempotencyKey: "idem1"},
	}, nil, nil)

	err := mw.WriteNewMessage(mw.ctx, &NewMessage{Message: m1})
	assert.Regexp(t, "FF10430", err)

	mdi.AssertExpectations(t)

}
