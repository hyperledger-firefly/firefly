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

package sqlcommon

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestUpsertE2EWithDB(t *testing.T) {
	log.SetLevel("trace")

	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new message
	msgID := fftypes.NewUUID()
	dataID1 := fftypes.NewUUID()
	dataID2 := fftypes.NewUUID()
	rand1 := fftypes.NewRandB32()
	rand2 := fftypes.NewRandB32()
	msg := &core.Message{
		LocalNamespace: "ns12345",
		Header: core.MessageHeader{
			ID:   msgID,
			CID:  nil,
			Type: core.MessageTypeBroadcast,
			SignerRef: core.SignerRef{
				Key:    "0x12345",
				Author: "did:firefly:org/abcd",
			},
			Created:   fftypes.Now(),
			Namespace: "ns12345",
			Topics:    []string{"test1"},
			Group:     nil,
			DataHash:  fftypes.NewRandB32(),
			TxType:    core.TransactionTypeUnpinned,
			TxParent: &core.TransactionRef{
				Type: core.TransactionTypeTokenTransfer,
				ID:   fftypes.NewUUID(),
			},
		},
		Hash:      fftypes.NewRandB32(),
		State:     core.MessageStateStaged,
		Confirmed: nil,
		Data: []*core.DataRef{
			{ID: dataID1, Hash: rand1},
			{ID: dataID2, Hash: rand2},
		},
	}

	s.callbacks.On("OrderedUUIDCollectionNSEvent", database.CollectionMessages, core.ChangeEventTypeCreated, "ns12345", msgID, mock.Anything).Return().Twice()
	s.callbacks.On("OrderedUUIDCollectionNSEvent", database.CollectionMessages, core.ChangeEventTypeUpdated, "ns12345", msgID, mock.Anything).Return()

	err := s.UpsertMessage(ctx, msg, database.UpsertOptimizationNew)
	assert.NoError(t, err)

	// Check we get the exact same message back
	msgRead, err := s.GetMessageByID(ctx, "ns12345", msgID)
	assert.NoError(t, err)
	// The generated sequence will have been added
	msg.Sequence = msgRead.Sequence
	assert.NoError(t, err)
	msgJson, _ := json.Marshal(&msg)
	msgReadJson, _ := json.Marshal(&msgRead)
	assert.Equal(t, string(msgJson), string(msgReadJson))

	// Update the message (this is testing what's possible at the database layer,
	// and does not account for the verification that happens at the higher level)
	cid := fftypes.NewUUID()
	gid := fftypes.NewRandB32()
	bid := fftypes.NewUUID()
	msgUpdated := &core.Message{
		LocalNamespace: "ns12345",
		Header: core.MessageHeader{
			ID:   msgID,
			CID:  cid,
			Type: core.MessageTypeBroadcast,
			SignerRef: core.SignerRef{
				Key:    "0x12345",
				Author: "did:firefly:org/abcd",
			},
			Created:   fftypes.Now(),
			Namespace: "ns12345",
			Topics:    []string{"topic1", "topic2"},
			Tag:       "tag_1",
			Group:     gid,
			DataHash:  fftypes.NewRandB32(),
			TxType:    core.TransactionTypeBatchPin,
			TxParent: &core.TransactionRef{
				Type: core.TransactionTypeTokenTransfer,
				ID:   fftypes.NewUUID(),
			},
		},
		Hash:           fftypes.NewRandB32(),
		Pins:           []string{fftypes.NewRandB32().String(), fftypes.NewRandB32().String()},
		State:          core.MessageStateRejected,
		Confirmed:      fftypes.Now(),
		BatchID:        bid,
		IdempotencyKey: "myBusinessIdentifier",
		Data: []*core.DataRef{
			{ID: dataID1, Hash: rand1},
			{ID: dataID2, Hash: rand2}, // Note the data refs cannot change, as it would affect the hash, and the hash is immutable
		},
	}

	// Ensure hash change rejected, on any optimization
	err = s.UpsertMessage(context.Background(), msgUpdated, database.UpsertOptimizationSkip)
	assert.Equal(t, database.HashMismatch, err)
	err = s.UpsertMessage(context.Background(), msgUpdated, database.UpsertOptimizationNew)
	assert.Equal(t, database.HashMismatch, err)
	err = s.UpsertMessage(context.Background(), msgUpdated, database.UpsertOptimizationExisting)
	assert.Equal(t, database.HashMismatch, err)

	msgUpdated.Hash = msg.Hash
	hookCalled := make(chan struct{}, 1)
	err = s.UpsertMessage(context.Background(), msgUpdated, database.UpsertOptimizationExisting, func() {
		close(hookCalled)
	})
	<-hookCalled
	assert.NoError(t, err)

	// Check we get the exact same message back - note the removal of one of the data elements
	msgRead, err = s.GetMessageByID(ctx, "ns12345", msgID)
	// The generated sequence will have been added
	msgUpdated.Sequence = msgRead.Sequence
	assert.NoError(t, err)
	msgJson, _ = json.Marshal(&msgUpdated)
	msgReadJson, _ = json.Marshal(&msgRead)
	assert.Equal(t, string(msgJson), string(msgReadJson))

	// Query back the message
	fb := database.MessageQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("id", msgUpdated.Header.ID.String()),
		fb.Eq("type", string(msgUpdated.Header.Type)),
		fb.Eq("author", msgUpdated.Header.Author),
		fb.Eq("topics", msgUpdated.Header.Topics),
		fb.Eq("group", msgUpdated.Header.Group),
		fb.Eq("cid", msgUpdated.Header.CID),
		fb.Eq("idempotencykey", msgUpdated.IdempotencyKey),
		fb.Gt("created", "0"),
		fb.Gt("confirmed", "0"),
	)
	msgs, res, err := s.GetMessages(ctx, "ns12345", filter.Count(true))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(msgs))
	assert.Equal(t, int64(1), *res.TotalCount)
	msgReadJson, _ = json.Marshal(msgs[0])
	assert.Equal(t, string(msgJson), string(msgReadJson))

	msgIDs, err := s.GetMessageIDs(ctx, "ns12345", filter)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(msgIDs))
	assert.Equal(t, msg.Header.ID, &msgIDs[0].ID)
	assert.Equal(t, msg.Sequence, msgIDs[0].Sequence)

	batchIDs, err := s.GetBatchIDsForMessages(ctx, "ns12345", []*fftypes.UUID{msg.Header.ID})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(batchIDs))
	assert.Equal(t, *msgUpdated.BatchID, *batchIDs[0])

	batchIDs, err = s.GetBatchIDsForDataAttachments(ctx, "ns12345", []*fftypes.UUID{dataID2})
	assert.NoError(t, err)
	assert.Equal(t, 1, len(batchIDs))
	assert.Equal(t, *msgUpdated.BatchID, *batchIDs[0])

	// Check we can get it with a filter on only messages with a particular data ref
	msgs, _, err = s.GetMessagesForData(ctx, "ns12345", dataID2, filter.Count(true))
	assert.Regexp(t, "FF10267", err) // The left join means it will take non-trivial extra work to support this. So not supported for now
	msgs, _, err = s.GetMessagesForData(ctx, "ns12345", dataID2, filter.Count(false))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(msgs))
	msgReadJson, _ = json.Marshal(msgs[0])
	assert.Equal(t, string(msgJson), string(msgReadJson))

	// Negative test on filter
	filter = fb.And(
		fb.Eq("id", msgUpdated.Header.ID.String()),
		fb.Eq("created", "0"),
	)
	msgs, _, err = s.GetMessages(ctx, "ns12345", filter)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(msgs))

	// Update
	gid2 := fftypes.NewRandB32()
	bid2 := fftypes.NewUUID()
	up := database.MessageQueryFactory.NewUpdate(ctx).
		Set("group", gid2).
		Set("batch", bid2)
	err = s.UpdateMessage(ctx, "ns12345", msgID, up)
	assert.NoError(t, err)

	// Test find updated value
	filter = fb.And(
		fb.Eq("id", msgUpdated.Header.ID.String()),
		fb.Eq("group", gid2),
	)
	msgs, _, err = s.GetMessages(ctx, "ns12345", filter)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(msgs))
	assert.Equal(t, *bid2, *msgs[0].BatchID)

	// Bump and Update - this is for a ready transition
	msgUpdated.State = core.MessageStateReady
	err = s.ReplaceMessage(context.Background(), msgUpdated)
	assert.NoError(t, err)
	msgRead, err = s.GetMessageByID(ctx, "ns12345", msgUpdated.Header.ID)
	msgJson, _ = json.Marshal(&msgUpdated)
	msgReadJson, _ = json.Marshal(msgRead)
	assert.Equal(t, string(msgJson), string(msgReadJson))

	// Query with a complex "like" filter
	filter = fb.And(
		fb.IStartsWith("tag", "TAG_"),
		fb.EndsWith("tag", "_1"),
		fb.NotContains("tag", "%tag%"),
	)
	msgs, _, err = s.GetMessages(ctx, "ns12345", filter)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(msgs))
	assert.Equal(t, *msgID, *msgs[0].Header.ID)

	s.callbacks.AssertExpectations(t)
}

func TestUpsertMessageFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertMessage(context.Background(), &core.Message{}, database.UpsertOptimizationSkip)
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertMessageFailSelect(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	msgID := fftypes.NewUUID()
	err := s.UpsertMessage(context.Background(), &core.Message{Header: core.MessageHeader{ID: msgID}}, database.UpsertOptimizationSkip)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertMessageFailInsert(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{}))
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	msgID := fftypes.NewUUID()
	err := s.UpsertMessage(context.Background(), &core.Message{Header: core.MessageHeader{ID: msgID}}, database.UpsertOptimizationSkip)
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertMessageFailUpdate(t *testing.T) {
	s, mock := newMockProvider().init()
	msgID := fftypes.NewUUID()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(msgID.String()))
	mock.ExpectExec("UPDATE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpsertMessage(context.Background(), &core.Message{Header: core.MessageHeader{ID: msgID}}, database.UpsertOptimizationSkip)
	assert.Regexp(t, "FF00178", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertMessageFailUpdateRefs(t *testing.T) {
	s, mock := newMockProvider().init()
	msgID := fftypes.NewUUID()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(msgID.String()))
	mock.ExpectExec("UPDATE .*").WillReturnResult(driver.ResultNoRows)
	mock.ExpectExec("DELETE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpsertMessage(context.Background(), &core.Message{Header: core.MessageHeader{ID: msgID}}, database.UpsertOptimizationSkip)
	assert.Regexp(t, "FF00179", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertMessageFailCommit(t *testing.T) {
	s, mock := newMockProvider().init()
	msgID := fftypes.NewUUID()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec("INSERT .*").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertMessage(context.Background(), &core.Message{Header: core.MessageHeader{ID: msgID}}, database.UpsertOptimizationSkip)
	assert.Regexp(t, "FF00180", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertMessagesBeginFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.InsertMessages(context.Background(), []*core.Message{})
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

func TestInsertMessagesMultiRowOK(t *testing.T) {
	s := newMockProvider()
	s.multiRowInsert = true
	s.fakePSQLInsert = true
	s, mock := s.init()

	msg1 := &core.Message{Header: core.MessageHeader{ID: fftypes.NewUUID(), Namespace: "ns1"}, LocalNamespace: "ns1", Data: core.DataRefs{{ID: fftypes.NewUUID()}}}
	msg2 := &core.Message{Header: core.MessageHeader{ID: fftypes.NewUUID(), Namespace: "ns1"}, LocalNamespace: "ns1", Data: core.DataRefs{{ID: fftypes.NewUUID()}}}
	s.callbacks.On("OrderedUUIDCollectionNSEvent", database.CollectionMessages, core.ChangeEventTypeCreated, "ns1", msg1.Header.ID, int64(1001))
	s.callbacks.On("OrderedUUIDCollectionNSEvent", database.CollectionMessages, core.ChangeEventTypeCreated, "ns1", msg2.Header.ID, int64(1002))

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT.*messages").WillReturnRows(sqlmock.NewRows([]string{s.SequenceColumn()}).
		AddRow(int64(1001)).
		AddRow(int64(1002)),
	)
	mock.ExpectQuery("INSERT.*messages_data").WillReturnRows(sqlmock.NewRows([]string{s.SequenceColumn()}).
		AddRow(int64(1003)).
		AddRow(int64(1004)),
	)
	mock.ExpectCommit()
	hookCalled := make(chan struct{}, 1)
	err := s.InsertMessages(context.Background(), []*core.Message{msg1, msg2}, func() {
		close(hookCalled)
	})
	<-hookCalled
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

func TestInsertMessagesMultiRowDataRefsFail(t *testing.T) {
	s := newMockProvider()
	s.multiRowInsert = true
	s.fakePSQLInsert = true
	s, mock := s.init()

	msg1 := &core.Message{Header: core.MessageHeader{ID: fftypes.NewUUID(), Namespace: "ns1"}, Data: core.DataRefs{{ID: fftypes.NewUUID()}}}

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT.*messages").WillReturnRows(sqlmock.NewRows([]string{s.SequenceColumn()}).AddRow(int64(1001)))
	mock.ExpectQuery("INSERT.*messages_data").WillReturnError(fmt.Errorf("pop"))
	err := s.InsertMessages(context.Background(), []*core.Message{msg1})
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

func TestInsertMessagesMultiRowFail(t *testing.T) {
	s := newMockProvider()
	s.multiRowInsert = true
	s.fakePSQLInsert = true
	s, mock := s.init()
	msg1 := &core.Message{Header: core.MessageHeader{ID: fftypes.NewUUID(), Namespace: "ns1"}}
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT.*").WillReturnError(fmt.Errorf("pop"))
	err := s.InsertMessages(context.Background(), []*core.Message{msg1})
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

func TestInsertMessagesSingleRowFail(t *testing.T) {
	s, mock := newMockProvider().init()
	msg1 := &core.Message{Header: core.MessageHeader{ID: fftypes.NewUUID(), Namespace: "ns1"}}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT.*").WillReturnError(fmt.Errorf("pop"))
	err := s.InsertMessages(context.Background(), []*core.Message{msg1})
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

func TestInsertMessagesSingleRowFailDataRefs(t *testing.T) {
	s, mock := newMockProvider().init()
	msg1 := &core.Message{Header: core.MessageHeader{ID: fftypes.NewUUID(), Namespace: "ns1"}, Data: core.DataRefs{{ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32()}}}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT.*messages").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT.*messages_data").WillReturnError(fmt.Errorf("pop"))
	err := s.InsertMessages(context.Background(), []*core.Message{msg1})
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

func TestReplaceMessageFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	msgID := fftypes.NewUUID()
	err := s.ReplaceMessage(context.Background(), &core.Message{Header: core.MessageHeader{ID: msgID}})
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestReplaceMessageFailDelete(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("DELETE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	msgID := fftypes.NewUUID()
	err := s.ReplaceMessage(context.Background(), &core.Message{Header: core.MessageHeader{ID: msgID}})
	assert.Regexp(t, "FF00179", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestReplaceMessageFailInsert(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("DELETE .*").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	msgID := fftypes.NewUUID()
	err := s.ReplaceMessage(context.Background(), &core.Message{Header: core.MessageHeader{ID: msgID}})
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateMessageDataRefsNilID(t *testing.T) {
	s, mock := newMockProvider().init()
	msgID := fftypes.NewUUID()
	mock.ExpectBegin()
	ctx, tx, _, err := s.BeginOrUseTx(context.Background())
	assert.NoError(t, err)
	err = s.updateMessageDataRefs(ctx, tx, &core.Message{
		Header: core.MessageHeader{ID: msgID},
		Data:   []*core.DataRef{{ID: nil}},
	}, false)
	assert.Regexp(t, "FF10123", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateMessageDataRefsNilHash(t *testing.T) {
	s, mock := newMockProvider().init()
	msgID := fftypes.NewUUID()
	mock.ExpectBegin()
	ctx, tx, _, err := s.BeginOrUseTx(context.Background())
	assert.NoError(t, err)
	err = s.updateMessageDataRefs(ctx, tx, &core.Message{
		Header: core.MessageHeader{ID: msgID},
		Data:   []*core.DataRef{{ID: fftypes.NewUUID()}},
	}, false)
	assert.Regexp(t, "FF10139", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateMessageDataDeleteFail(t *testing.T) {
	s, mock := newMockProvider().init()
	msgID := fftypes.NewUUID()
	mock.ExpectBegin()
	ctx, tx, _, err := s.BeginOrUseTx(context.Background())
	assert.NoError(t, err)
	mock.ExpectExec("DELETE .*").WillReturnError(fmt.Errorf("pop"))
	err = s.updateMessageDataRefs(ctx, tx, &core.Message{
		Header: core.MessageHeader{ID: msgID},
	}, true)
	assert.Regexp(t, "FF00179", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateMessageDataAddFail(t *testing.T) {
	s, mock := newMockProvider().init()
	msgID := fftypes.NewUUID()
	dataID := fftypes.NewUUID()
	dataHash := fftypes.NewRandB32()
	mock.ExpectBegin()
	ctx, tx, _, err := s.BeginOrUseTx(context.Background())
	assert.NoError(t, err)
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	err = s.updateMessageDataRefs(ctx, tx, &core.Message{
		Header: core.MessageHeader{ID: msgID},
		Data:   []*core.DataRef{{ID: dataID, Hash: dataHash}},
	}, false)
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadMessageDataRefsQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	msgID := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	err := s.loadDataRefs(context.Background(), "ns1", []*core.Message{
		{
			Header: core.MessageHeader{ID: msgID},
		},
	})
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadMessageDataRefsScanFail(t *testing.T) {
	s, mock := newMockProvider().init()
	msgID := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"data_id"}).AddRow("only one"))
	err := s.loadDataRefs(context.Background(), "ns1", []*core.Message{
		{
			Header: core.MessageHeader{ID: msgID},
		},
	})
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadMessageDataRefsEmpty(t *testing.T) {
	s, mock := newMockProvider().init()
	msgID := fftypes.NewUUID()
	msg := &core.Message{Header: core.MessageHeader{ID: msgID}}
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"data_id", "data_hash"}))
	err := s.loadDataRefs(context.Background(), "ns1", []*core.Message{msg})
	assert.NoError(t, err)
	assert.Equal(t, core.DataRefs{}, msg.Data)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetMessageByIDSelectFail(t *testing.T) {
	s, mock := newMockProvider().init()
	msgID := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetMessageByID(context.Background(), "ns1", msgID)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetMessageByIDNotFound(t *testing.T) {
	s, mock := newMockProvider().init()
	msgID := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	msg, err := s.GetMessageByID(context.Background(), "ns1", msgID)
	assert.NoError(t, err)
	assert.Nil(t, msg)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetMessageByIDScanFail(t *testing.T) {
	s, mock := newMockProvider().init()
	msgID := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("only one"))
	_, err := s.GetMessageByID(context.Background(), "ns1", msgID)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetMessageByIDLoadRefsFail(t *testing.T) {
	s, mock := newMockProvider().init()
	msgID := fftypes.NewUUID()
	b32 := fftypes.NewRandB32()
	cols := append([]string{}, msgColumns...)
	cols = append(cols, "id()")
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows(cols).
		AddRow(msgID.String(), nil, core.MessageTypeBroadcast, "author1", "0x12345", 0, "ns1", "ns1", "t1", "c1", nil, b32.String(), b32.String(), b32.String(), "confirmed", 0, "", "pin", nil, "", nil, nil, "bob", 0))
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetMessageByID(context.Background(), "ns1", msgID)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetMessagesBuildQueryFail(t *testing.T) {
	s, _ := newMockProvider().init()
	f := database.MessageQueryFactory.NewFilter(context.Background()).Eq("id", map[bool]bool{true: false})
	_, _, err := s.GetMessages(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00143.*id", err)
}

func TestGetMessagesQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	f := database.MessageQueryFactory.NewFilter(context.Background()).Eq("id", "")
	_, _, err := s.GetMessages(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetMessagesForDataBadQuery(t *testing.T) {
	s, mock := newMockProvider().init()
	f := database.MessageQueryFactory.NewFilter(context.Background()).Eq("!wrong", "")
	_, _, err := s.GetMessagesForData(context.Background(), "ns1", fftypes.NewUUID(), f)
	assert.Regexp(t, "FF00142", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetMessagesReadMessageFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("only one"))
	f := database.MessageQueryFactory.NewFilter(context.Background()).Eq("id", "")
	_, _, err := s.GetMessages(context.Background(), "ns1", f)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetMessagesLoadRefsFail(t *testing.T) {
	s, mock := newMockProvider().init()
	msgID := fftypes.NewUUID()
	b32 := fftypes.NewRandB32()
	cols := append([]string{}, msgColumns...)
	cols = append(cols, "id()")
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows(cols).
		AddRow(msgID.String(), nil, core.MessageTypeBroadcast, "author1", "0x12345", 0, "ns1", "ns1", "t1", "c1", nil, b32.String(), b32.String(), b32.String(), "confirmed", 0, "", "pin", nil, "", nil, nil, "bob", 0))
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	f := database.MessageQueryFactory.NewFilter(context.Background()).Gt("confirmed", "0")
	_, _, err := s.GetMessages(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMessageUpdateBeginFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	u := database.MessageQueryFactory.NewUpdate(context.Background()).Set("id", "anything")
	err := s.UpdateMessage(context.Background(), "ns1", fftypes.NewUUID(), u)
	assert.Regexp(t, "FF00175", err)
}

func TestGetMessageIDsQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	f := database.MessageQueryFactory.NewFilter(context.Background()).Eq("id", "")
	_, err := s.GetMessageIDs(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetMessageIDsReadMessageFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("bad id"))
	f := database.MessageQueryFactory.NewFilter(context.Background()).Eq("id", "")
	_, err := s.GetMessageIDs(context.Background(), "ns1", f)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetMessageIDsBadQuery(t *testing.T) {
	s, mock := newMockProvider().init()
	f := database.MessageQueryFactory.NewFilter(context.Background()).Eq("!wrong", "")
	_, err := s.GetMessageIDs(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00142", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestMessageUpdateBuildQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	u := database.MessageQueryFactory.NewUpdate(context.Background()).Set("id", map[bool]bool{true: false})
	err := s.UpdateMessage(context.Background(), "ns1", fftypes.NewUUID(), u)
	assert.Regexp(t, "FF00143.*id", err)
}

func TestMessagesUpdateBuildFilterFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	f := database.MessageQueryFactory.NewFilter(context.Background()).Eq("id", map[bool]bool{true: false})
	u := database.MessageQueryFactory.NewUpdate(context.Background()).Set("type", core.MessageTypeBroadcast)
	err := s.UpdateMessages(context.Background(), "ns1", f, u)
	assert.Regexp(t, "FF00143.*id", err)
}

func TestMessageUpdateFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	u := database.MessageQueryFactory.NewUpdate(context.Background()).Set("group", fftypes.NewRandB32())
	err := s.UpdateMessage(context.Background(), "ns1", fftypes.NewUUID(), u)
	assert.Regexp(t, "FF00178", err)
}

func TestGetBatchIDsForMessagesSelectFail(t *testing.T) {
	s, mock := newMockProvider().init()
	msgID := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetBatchIDsForMessages(context.Background(), "ns1", []*fftypes.UUID{msgID})
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetBatchIDsForMessagesScanFail(t *testing.T) {
	s, mock := newMockProvider().init()
	msgID := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"batch_id"}).AddRow("not a UUID"))
	_, err := s.GetBatchIDsForMessages(context.Background(), "ns1", []*fftypes.UUID{msgID})
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
