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

package orchestrator

import (
	"context"
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestGetNamespace(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	ns := or.GetNamespace(context.Background())
	assert.Equal(t, "ns", ns.Name)
}

func TestGetTransactionByID(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	u := fftypes.NewUUID()
	or.mth.On("GetTransactionByIDCached", mock.Anything, u).Return(nil, nil)
	_, err := or.GetTransactionByID(context.Background(), u.String())
	assert.NoError(t, err)
}

func TestGetTransactionByIDBadID(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	_, err := or.GetTransactionByID(context.Background(), "")
	assert.Regexp(t, "FF00138", err)
}

func TestGetTransactionOperationsOk(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	or.mdi.On("GetOperations", mock.Anything, "ns", mock.Anything).Return([]*core.Operation{}, nil, nil)
	_, _, err := or.GetTransactionOperations(context.Background(), fftypes.NewUUID().String())
	assert.NoError(t, err)
}

func TestGetTransactionOperationBadID(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	_, _, err := or.GetTransactionOperations(context.Background(), "")
	assert.Regexp(t, "FF00138", err)
}

func TestGetTransactions(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	u := fftypes.NewUUID()
	or.mdi.On("GetTransactions", mock.Anything, "ns", mock.Anything).Return([]*core.Transaction{}, nil, nil)
	fb := database.TransactionQueryFactory.NewFilter(context.Background())
	f := fb.And(fb.Eq("id", u))
	_, _, err := or.GetTransactions(context.Background(), f)
	assert.NoError(t, err)
}

func TestGetMessageByIDBadID(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	_, err := or.GetMessageByID(context.Background(), "")
	assert.Regexp(t, "FF00138", err)
}

func TestGetMessageByIDNoValuesOk(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	msgID := fftypes.NewUUID()
	msg := &core.Message{
		Header: core.MessageHeader{
			Namespace: "ns",
			ID:        msgID,
		},
		Data: core.DataRefs{
			{ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32()},
			{ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32()},
		},
	}
	or.mdi.On("GetMessageByID", mock.Anything, "ns", msgID).Return(msg, nil)

	msgI, err := or.GetMessageByID(context.Background(), msgID.String())
	assert.NoError(t, err)
	assert.NotNil(t, msgI.Data[0].ID)
	assert.NotNil(t, msgI.Data[0].Hash)
	assert.NotNil(t, msgI.Data[1].ID)
	assert.NotNil(t, msgI.Data[1].Hash)
}

func TestGetMessageByIDWithDataOk(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	msgID := fftypes.NewUUID()
	msg := &core.Message{
		Header: core.MessageHeader{
			Namespace: "ns",
			ID:        msgID,
		},
		Data: core.DataRefs{
			{ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32()},
			{ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32()},
		},
	}
	or.mdi.On("GetMessageByID", mock.Anything, "ns", msgID).Return(msg, nil)
	or.mdm.On("GetMessageDataCached", mock.Anything, mock.Anything).Return(core.DataArray{
		{ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32(), Value: fftypes.JSONAnyPtr("{}")},
		{ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32(), Value: fftypes.JSONAnyPtr("{}")},
	}, true, nil)

	msgI, err := or.GetMessageByIDWithData(context.Background(), msgID.String())
	assert.NoError(t, err)
	assert.NotNil(t, msgI.InlineData[0].ID)
	assert.NotNil(t, msgI.InlineData[0].Hash)
	assert.NotNil(t, msgI.InlineData[0].Value)
	assert.NotNil(t, msgI.InlineData[1].ID)
	assert.NotNil(t, msgI.InlineData[1].Hash)
	assert.NotNil(t, msgI.InlineData[1].Value)
}

func TestGetMessageByIDWithDataMsgFail(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	msgID := fftypes.NewUUID()
	or.mdi.On("GetMessageByID", mock.Anything, "ns", mock.Anything).Return(nil, fmt.Errorf("pop"))

	_, err := or.GetMessageByIDWithData(context.Background(), msgID.String())
	assert.EqualError(t, err, "pop")
}

func TestGetMessageByIDWithDataFail(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	msgID := fftypes.NewUUID()
	msg := &core.Message{
		Header: core.MessageHeader{
			Namespace: "ns",
			ID:        msgID,
		},
		Data: core.DataRefs{
			{ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32()},
			{ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32()},
		},
	}
	or.mdi.On("GetMessageByID", mock.Anything, "ns", mock.Anything).Return(msg, nil)
	or.mdm.On("GetMessageDataCached", mock.Anything, mock.Anything).Return(nil, false, fmt.Errorf("pop"))

	_, err := or.GetMessageByIDWithData(context.Background(), msgID.String())
	assert.EqualError(t, err, "pop")
}

func TestGetMessages(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	u := fftypes.NewUUID()
	or.mdi.On("GetMessages", mock.Anything, "ns", mock.Anything).Return([]*core.Message{}, nil, nil)
	fb := database.MessageQueryFactory.NewFilter(context.Background())
	f := fb.And(fb.Eq("id", u))
	_, _, err := or.GetMessages(context.Background(), f)
	assert.NoError(t, err)
}

func TestGetMessagesWithDataFailMsg(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	or.mdi.On("GetMessages", mock.Anything, "ns", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))
	fb := database.MessageQueryFactory.NewFilter(context.Background())
	f := fb.And(fb.Eq("id", fftypes.NewUUID()))
	_, _, err := or.GetMessagesWithData(context.Background(), f)
	assert.EqualError(t, err, "pop")
}

func TestGetMessagesWithDataOk(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	u := fftypes.NewUUID()
	msgID := fftypes.NewUUID()
	msg := &core.Message{
		Header: core.MessageHeader{
			ID: msgID,
		},
		Data: core.DataRefs{},
	}
	or.mdi.On("GetMessages", mock.Anything, "ns", mock.Anything).Return([]*core.Message{msg}, nil, nil)
	fb := database.MessageQueryFactory.NewFilter(context.Background())
	or.mdm.On("GetMessageDataCached", mock.Anything, mock.Anything).Return(core.DataArray{}, true, nil)
	f := fb.And(fb.Eq("id", u))
	_, _, err := or.GetMessagesWithData(context.Background(), f)
	assert.NoError(t, err)
}

func TestGetMessagesWithDataFail(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	u := fftypes.NewUUID()
	msgID := fftypes.NewUUID()
	msg := &core.Message{
		Header: core.MessageHeader{
			ID: msgID,
		},
		Data: core.DataRefs{},
	}
	or.mdi.On("GetMessages", mock.Anything, "ns", mock.Anything).Return([]*core.Message{msg}, nil, nil)
	fb := database.MessageQueryFactory.NewFilter(context.Background())
	or.mdm.On("GetMessageDataCached", mock.Anything, mock.Anything).Return(nil, true, fmt.Errorf("pop"))
	f := fb.And(fb.Eq("id", u))
	_, _, err := or.GetMessagesWithData(context.Background(), f)
	assert.EqualError(t, err, "pop")
}

func TestGetMessagesForData(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	u := fftypes.NewUUID()
	or.mdi.On("GetMessagesForData", mock.Anything, "ns", u, mock.Anything).Return([]*core.Message{}, nil, nil)
	fb := database.MessageQueryFactory.NewFilter(context.Background())
	f := fb.And(fb.Eq("id", u))
	_, _, err := or.GetMessagesForData(context.Background(), u.String(), f)
	assert.NoError(t, err)
}

func TestGetMessagesForDataBadID(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	f := database.MessageQueryFactory.NewFilter(context.Background()).And()
	_, _, err := or.GetMessagesForData(context.Background(), "!bad", f)
	assert.Regexp(t, "FF00138", err)
}

func TestGetMessageTransactionOk(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	msgID := fftypes.NewUUID()
	batchID := fftypes.NewUUID()
	txID := fftypes.NewUUID()
	or.mdi.On("GetMessageByID", mock.Anything, "ns", msgID).Return(&core.Message{
		BatchID: batchID,
		Header: core.MessageHeader{
			Namespace: "ns",
			TxType:    core.TransactionTypeBatchPin,
		},
		TransactionID: txID,
	}, nil)
	or.mth.On("GetTransactionByIDCached", mock.Anything, txID).Return(&core.Transaction{
		ID: txID,
	}, nil)
	tx, err := or.GetMessageTransaction(context.Background(), msgID.String())
	assert.NoError(t, err)
	assert.Equal(t, *txID, *tx.ID)
	or.mdi.AssertExpectations(t)
}

func TestGetMessageTransactionLookupErr(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	msgID := fftypes.NewUUID()
	batchID := fftypes.NewUUID()
	txID := fftypes.NewUUID()
	or.mdi.On("GetMessageByID", mock.Anything, "ns", msgID).Return(&core.Message{
		BatchID: batchID,
		Header: core.MessageHeader{
			Namespace: "ns",
			TxType:    core.TransactionTypeBatchPin,
		},
		TransactionID: txID,
	}, nil)
	or.mth.On("GetTransactionByIDCached", mock.Anything, txID).Return(nil, fmt.Errorf("pop"))
	_, err := or.GetMessageTransaction(context.Background(), msgID.String())
	assert.Regexp(t, "pop", err)
}

func TestGetMessageTransactionNoTx(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	msgID := fftypes.NewUUID()
	or.mdi.On("GetMessageByID", mock.Anything, "ns", msgID).Return(&core.Message{
		Header: core.MessageHeader{
			Namespace: "ns",
		},
	}, nil)
	_, err := or.GetMessageTransaction(context.Background(), msgID.String())
	assert.Regexp(t, "FF10210", err)
}

func TestGetMessageTransactionMessageNotFound(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	msgID := fftypes.NewUUID()
	or.mdi.On("GetMessageByID", mock.Anything, "ns", msgID).Return(nil, nil)
	_, err := or.GetMessageTransaction(context.Background(), msgID.String())
	assert.Regexp(t, "FF10109", err)
}

func TestGetMessageData(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	msg := &core.Message{
		Header: core.MessageHeader{
			Namespace: "ns",
			ID:        fftypes.NewUUID(),
		},
		Data: core.DataRefs{
			{ID: fftypes.NewUUID()},
			{ID: fftypes.NewUUID()},
		},
	}
	or.mdi.On("GetMessageByID", mock.Anything, "ns", mock.Anything).Return(msg, nil)
	or.mdm.On("GetMessageDataCached", mock.Anything, mock.Anything).Return(core.DataArray{}, true, nil)
	_, err := or.GetMessageData(context.Background(), fftypes.NewUUID().String())
	assert.NoError(t, err)
}

func TestGetMessageDataBadMsg(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	or.mdi.On("GetMessageByID", mock.Anything, "ns", mock.Anything).Return(nil, nil)
	_, err := or.GetMessageData(context.Background(), fftypes.NewUUID().String())
	assert.Regexp(t, "FF10109", err)
}

func TestGetMessageEventsOk(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	msg := &core.Message{
		Header: core.MessageHeader{
			Namespace: "ns",
			ID:        fftypes.NewUUID(),
		},
		Data: core.DataRefs{
			{ID: fftypes.NewUUID()},
			{ID: fftypes.NewUUID()},
		},
	}
	or.mdi.On("GetMessageByID", mock.Anything, "ns", mock.Anything).Return(msg, nil)
	or.mdi.On("GetEvents", mock.Anything, "ns", mock.Anything).Return([]*core.Event{}, nil, nil)
	fb := database.EventQueryFactory.NewFilter(context.Background())
	f := fb.And(fb.Eq("type", core.EventTypeMessageConfirmed))
	_, _, err := or.GetMessageEvents(context.Background(), fftypes.NewUUID().String(), f)
	assert.NoError(t, err)
	calculatedFilter, err := or.mdi.Calls[1].Arguments[2].(ffapi.Filter).Finalize()
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf(
		`( type == 'message_confirmed' ) && ( reference IN ['%s','%s','%s'] )`,
		msg.Header.ID, msg.Data[0].ID, msg.Data[1].ID,
	), calculatedFilter.String())
	assert.NoError(t, err)
}

func TestGetMessageEventsBadMsgID(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	fb := database.EventQueryFactory.NewFilter(context.Background())
	f := fb.And(fb.Eq("type", core.EventTypeMessageConfirmed))
	or.mdi.On("GetMessageByID", mock.Anything, "ns", mock.Anything).Return(nil, nil)
	ev, _, err := or.GetMessageEvents(context.Background(), fftypes.NewUUID().String(), f)
	assert.Regexp(t, "FF10109", err)
	assert.Nil(t, ev)
}

func TestGetBatchByID(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	u := fftypes.NewUUID()
	or.mdi.On("GetBatchByID", mock.Anything, "ns", u).Return(&core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			Namespace: "ns",
		},
	}, nil)
	_, err := or.GetBatchByID(context.Background(), u.String())
	assert.NoError(t, err)
}

func TestGetBatchByIDBadID(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	_, err := or.GetBatchByID(context.Background(), "")
	assert.Regexp(t, "FF00138", err)
}

func TestGetBatches(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	u := fftypes.NewUUID()
	or.mdi.On("GetBatches", mock.Anything, "ns", mock.Anything).Return([]*core.BatchPersisted{}, nil, nil)
	fb := database.BatchQueryFactory.NewFilter(context.Background())
	f := fb.And(fb.Eq("id", u))
	_, _, err := or.GetBatches(context.Background(), f)
	assert.NoError(t, err)
}

func TestGetDataByID(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	u := fftypes.NewUUID()
	or.mdi.On("GetDataByID", mock.Anything, "ns", u, true).Return(&core.Data{
		Namespace: "ns",
	}, nil)
	_, err := or.GetDataByID(context.Background(), u.String())
	assert.NoError(t, err)
}

func TestGetDataByIDBadID(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	_, err := or.GetDataByID(context.Background(), "")
	assert.Regexp(t, "FF00138", err)
}

func TestGetData(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	u := fftypes.NewUUID()
	or.mdi.On("GetData", mock.Anything, "ns", mock.Anything).Return(core.DataArray{}, nil, nil)
	fb := database.DataQueryFactory.NewFilter(context.Background())
	f := fb.And(fb.Eq("id", u))
	_, _, err := or.GetData(context.Background(), f)
	assert.NoError(t, err)
}

func TestGetDataSubPaths(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	or.mdi.On("GetDataSubPaths", mock.Anything, "ns", "/parent").Return([]string{}, nil)
	_, err := or.GetDataSubPaths(context.Background(), "/parent")
	assert.NoError(t, err)
}

func TestGetDatatypeByID(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	u := fftypes.NewUUID()
	or.mdi.On("GetDatatypeByID", mock.Anything, "ns", u).Return(&core.Datatype{
		Namespace: "ns1",
	}, nil)
	_, err := or.GetDatatypeByID(context.Background(), u.String())
	assert.NoError(t, err)
}

func TestGetDatatypeByIDBadID(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	_, err := or.GetDatatypeByID(context.Background(), "")
	assert.Regexp(t, "FF00138", err)
}

func TestGetDatatypeByName(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	or.mdi.On("GetDatatypeByName", context.Background(), "ns", "dt", "1").Return(&core.Datatype{
		Namespace: "ns1",
	}, nil)
	_, err := or.GetDatatypeByName(context.Background(), "dt", "1")
	assert.NoError(t, err)
}

func TestGetDatatypeByNameBadNamespace(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	_, err := or.GetDatatypeByName(context.Background(), "", "")
	assert.Regexp(t, "FF00140", err)
}

func TestGetDatatypeByNameBadName(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	_, err := or.GetDatatypeByName(context.Background(), "", "")
	assert.Regexp(t, "FF00140", err)
}

func TestGetOperationByID(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	u := fftypes.NewUUID()
	or.mom.On("GetOperationByIDCached", mock.Anything, u).Return(&core.Operation{
		Namespace: "ns1",
	}, nil)
	_, err := or.GetOperationByID(context.Background(), u.String())
	assert.NoError(t, err)
}

func TestGetOperationIDBadID(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	_, err := or.GetOperationByID(context.Background(), "")
	assert.Regexp(t, "FF00138", err)
}

type txnStatus struct {
	TxnId string
}

func TestGetOperationByIDWithStatus(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	u := fftypes.NewUUID()

	or.mom.On("GetOperationByIDCached", mock.Anything, u).Return(&core.Operation{
		Namespace: "ns1",
		Type:      core.OpTypeBlockchainInvoke,
	}, nil)

	or.mbi.On("GetTransactionStatus", mock.Anything, mock.Anything).Return(&txnStatus{TxnId: "abc123"}, nil)
	opStatus, err := or.GetOperationByIDWithStatus(context.Background(), u.String())
	assert.Nil(t, err)
	assert.NotNil(t, opStatus)
	var expectedTxnStatus = &txnStatus{TxnId: "abc123"}
	assert.Equal(t, expectedTxnStatus, opStatus.Detail)
}

func TestGetOperationByIDWithGoodEmptyStatus(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	u := fftypes.NewUUID()

	or.mom.On("GetOperationByIDCached", mock.Anything, u).Return(&core.Operation{
		Namespace: "ns1",
		Type:      core.OpTypeBlockchainInvoke,
	}, nil)

	or.mbi.On("GetTransactionStatus", mock.Anything, mock.Anything).Return(nil, nil)
	opStatus, err := or.GetOperationByIDWithStatus(context.Background(), u.String())
	assert.Nil(t, err)
	assert.NotNil(t, opStatus)
	assert.Equal(t, nil, opStatus.Detail)
}

func TestGetOperationByIDWithStatusWrongOpType(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	u := fftypes.NewUUID()

	or.mom.On("GetOperationByIDCached", mock.Anything, u).Return(&core.Operation{
		Namespace: "ns1",
		Type:      core.OpTypeDataExchangeSendBatch,
	}, nil)

	opStatus, err := or.GetOperationByIDWithStatus(context.Background(), u.String())
	assert.Nil(t, err)
	assert.NotNil(t, opStatus)
	assert.Equal(t, nil, opStatus.Detail)
}

func TestGetOperationIDWithStatusBadID(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	_, err := or.GetOperationByIDWithStatus(context.Background(), "")
	assert.Regexp(t, "FF00138", err)
}

func TestGetOperationByIDWithStatusNoOp(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	u := fftypes.NewUUID()

	or.mom.On("GetOperationByIDCached", mock.Anything, u).Return(nil, fmt.Errorf("pop"))
	_, err := or.GetOperationByIDWithStatus(context.Background(), u.String())
	assert.Error(t, err)
}

func TestGetOperationByIDWithStatusError(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	u := fftypes.NewUUID()

	or.mom.On("GetOperationByIDCached", mock.Anything, u).Return(&core.Operation{
		Namespace: "ns1",
		Type:      core.OpTypeBlockchainInvoke,
	}, nil)

	or.mbi.On("GetTransactionStatus", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))
	opStatus, err := or.GetOperationByIDWithStatus(context.Background(), u.String())
	assert.Nil(t, err)
	assert.NotNil(t, opStatus)
	assert.Regexp(t, "pop", opStatus.Detail)
}

func TestGetEventByID(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	u := fftypes.NewUUID()
	or.mdi.On("GetEventByID", mock.Anything, "ns", u).Return(&core.Event{
		Namespace: "ns",
	}, nil)
	_, err := or.GetEventByID(context.Background(), u.String())
	assert.NoError(t, err)
}

func TestGetEventIDBadID(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	_, err := or.GetEventByID(context.Background(), "")
	assert.Regexp(t, "FF00138", err)
}

func TestGetDatatypes(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	u := fftypes.NewUUID()
	or.mdi.On("GetDatatypes", mock.Anything, "ns", mock.Anything).Return([]*core.Datatype{}, nil, nil)
	fb := database.DatatypeQueryFactory.NewFilter(context.Background())
	f := fb.And(fb.Eq("id", u))
	_, _, err := or.GetDatatypes(context.Background(), f)
	assert.NoError(t, err)
}

func TestGetOperations(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	u := fftypes.NewUUID()
	or.mdi.On("GetOperations", mock.Anything, "ns", mock.Anything).Return([]*core.Operation{}, nil, nil)
	fb := database.OperationQueryFactory.NewFilter(context.Background())
	f := fb.And(fb.Eq("id", u))
	_, _, err := or.GetOperations(context.Background(), f)
	assert.NoError(t, err)
}

func TestGetEvents(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	u := fftypes.NewUUID()
	or.mdi.On("GetEvents", mock.Anything, "ns", mock.Anything).Return([]*core.Event{}, nil, nil)
	fb := database.EventQueryFactory.NewFilter(context.Background())
	f := fb.And(fb.Eq("id", u))
	_, _, err := or.GetEvents(context.Background(), f)
	assert.NoError(t, err)
}

func TestGetEventsWithReferencesFail(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	u := fftypes.NewUUID()
	or.mdi.On("GetEvents", mock.Anything, "ns", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))
	fb := database.EventQueryFactory.NewFilter(context.Background())
	f := fb.And(fb.Eq("id", u))
	_, _, err := or.GetEventsWithReferences(context.Background(), f)
	assert.EqualError(t, err, "pop")
}

func TestGetEventsWithReferences(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	u := fftypes.NewUUID()

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()
	ref2 := fftypes.NewUUID()
	ev2 := fftypes.NewUUID()
	ref3 := fftypes.NewUUID()
	ev3 := fftypes.NewUUID()

	blockchainEvent := &core.Event{
		ID:        ev1,
		Sequence:  10000001,
		Reference: ref1,
		Type:      core.EventTypeBlockchainEventReceived,
	}

	txEvent := &core.Event{
		ID:        ev2,
		Sequence:  10000002,
		Reference: ref2,
		Type:      core.EventTypeTransactionSubmitted,
	}

	msgEvent := &core.Event{
		ID:        ev3,
		Sequence:  10000003,
		Reference: ref3,
		Type:      core.EventTypeMessageConfirmed,
	}

	or.mem.On("EnrichEvents", mock.Anything, mock.Anything).Return([]*core.EnrichedEvent{
		{
			Event: *blockchainEvent,
			BlockchainEvent: &core.BlockchainEvent{
				ID: ref1,
			},
		},
		{
			Event: *txEvent,
			Transaction: &core.Transaction{
				ID: ref2,
			},
		},
		{
			Event: *msgEvent,
			Message: &core.Message{
				Header: core.MessageHeader{
					ID: ref3,
				},
			},
		},
	}, nil)

	or.mdi.On("GetEvents", mock.Anything, "ns", mock.Anything).Return([]*core.Event{
		blockchainEvent,
		txEvent,
		msgEvent,
	}, nil, nil)
	fb := database.EventQueryFactory.NewFilter(context.Background())
	f := fb.And(fb.Eq("id", u))
	_, _, err := or.GetEventsWithReferences(context.Background(), f)
	assert.NoError(t, err)
}

func TestGetEventsWithReferencesEnrichFail(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	u := fftypes.NewUUID()

	or.mdi.On("GetEvents", mock.Anything, "ns", mock.Anything).Return([]*core.Event{{ID: fftypes.NewUUID()}}, nil, nil)
	or.mem.On("EnrichEvents", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))
	fb := database.EventQueryFactory.NewFilter(context.Background())
	f := fb.And(fb.Eq("id", u))
	_, _, err := or.GetEventsWithReferences(context.Background(), f)
	assert.EqualError(t, err, "pop")
}

func TestGetEventWithReferenceQueryFail(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	id := fftypes.NewUUID()
	or.mdi.On("GetEventByID", mock.Anything, "ns", mock.Anything).Return(nil, fmt.Errorf("pop"))
	_, err := or.GetEventByIDWithReference(context.Background(), id.String())
	assert.EqualError(t, err, "pop")
}

func TestGetEventWithReferenceParseUUIDFail(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	_, err := or.GetEventByIDWithReference(context.Background(), "1")
	assert.Regexp(t, "FF00138", err)
}

func TestGetEventWithReferenceOK(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()

	blockchainEvent := &core.Event{
		ID:        ev1,
		Sequence:  10000001,
		Reference: ref1,
		Type:      core.EventTypeBlockchainEventReceived,
	}

	or.mem.On("EnrichEvent", mock.Anything, blockchainEvent).Return(&core.EnrichedEvent{
		Event: *blockchainEvent,
		BlockchainEvent: &core.BlockchainEvent{
			ID: ref1,
		},
	}, nil)

	or.mdi.On("GetEventByID", mock.Anything, "ns", mock.Anything).Return(blockchainEvent, nil)
	_, err := or.GetEventByIDWithReference(context.Background(), ev1.String())
	assert.NoError(t, err)
}

func TestGetBlockchainEventByID(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)

	id := fftypes.NewUUID()
	or.mth.On("GetBlockchainEventByIDCached", context.Background(), id).Return(&core.BlockchainEvent{
		Namespace: "ns",
	}, nil)

	_, err := or.GetBlockchainEventByID(context.Background(), id.String())
	assert.NoError(t, err)
}

func TestGetBlockchainEventByIDBadID(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	_, err := or.GetBlockchainEventByID(context.Background(), "")
	assert.Regexp(t, "FF00138", err)
}

func TestGetBlockchainEvents(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)

	or.mdi.On("GetBlockchainEvents", context.Background(), "ns", mock.Anything).Return(nil, nil, nil)

	f := database.ContractListenerQueryFactory.NewFilter(context.Background())
	_, _, err := or.GetBlockchainEvents(context.Background(), f.And())
	assert.NoError(t, err)
}

func TestGetTransactionBlockchainEventsOk(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	or.mdi.On("GetBlockchainEvents", mock.Anything, "ns", mock.Anything).Return([]*core.BlockchainEvent{}, nil, nil)
	_, _, err := or.GetTransactionBlockchainEvents(context.Background(), fftypes.NewUUID().String())
	assert.NoError(t, err)
}

func TestGetTransactionBlockchainEventsBadID(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	_, _, err := or.GetTransactionBlockchainEvents(context.Background(), "")
	assert.Regexp(t, "FF00138", err)
}

func TestGetPins(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	u := fftypes.NewUUID()
	or.mdi.On("GetPins", mock.Anything, "ns", mock.Anything).Return([]*core.Pin{}, nil, nil)
	fb := database.PinQueryFactory.NewFilter(context.Background())
	f := fb.And(fb.Eq("hash", u))
	_, _, err := or.GetPins(context.Background(), f)
	assert.NoError(t, err)
}

func TestGetNextPins(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	u := fftypes.NewUUID()
	or.mdi.On("GetNextPins", mock.Anything, "ns", mock.Anything).Return([]*core.NextPin{}, nil, nil)
	fb := database.NextPinQueryFactory.NewFilter(context.Background())
	f := fb.And(fb.Eq("hash", u))
	_, _, err := or.GetNextPins(context.Background(), f)
	assert.NoError(t, err)
}

func TestGetEventsInSequenceWithReferencesWhenEnrichEventsFails(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)
	u := fftypes.NewUUID()
	or.mdi.On("GetEventsInSequenceRange", mock.Anything, "ns", mock.Anything, mock.Anything, mock.Anything).Return([]*core.Event{}, nil, nil)
	or.mem.On("EnrichEvents", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("Oops..."))
	fb := database.EventQueryFactory.NewFilter(context.Background())
	f := fb.And(fb.Eq("id", u))
	_, _, err := or.GetEventsWithReferencesInSequenceRange(context.Background(), f, 0, 100)
	assert.EqualError(t, err, "Oops...")
}
