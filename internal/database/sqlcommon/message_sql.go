// Copyright © 2024 Kaleido, Inc.
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
	"database/sql"
	"fmt"

	sq "github.com/Masterminds/squirrel"
	"github.com/hyperledger-firefly/common/pkg/dbsql"
	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

var (
	msgColumns = []string{
		"id",
		"cid",
		"mtype",
		"author",
		"key",
		"created",
		"namespace",
		"namespace_local",
		"topics",
		"tag",
		"group_hash",
		"datahash",
		"hash",
		"pins",
		"state",
		"confirmed",
		"reject_reason",
		"tx_type",
		"tx_id",
		"tx_parent_type",
		"tx_parent_id",
		"batch_id",
		"idempotency_key",
	}
	msgFilterFieldMap = map[string]string{
		"type":           "mtype",
		"txtype":         "tx_type",
		"txid":           "tx_id",
		"txparent.type":  "tx_parent_type",
		"txparent.id":    "tx_parent_id",
		"batch":          "batch_id",
		"group":          "group_hash",
		"idempotencykey": "idempotency_key",
		"rejectreason":   "reject_reason",
	}
)

const messagesTable = "messages"
const messagesDataJoinTable = "messages_data"

func (s *SQLCommon) attemptMessageUpdate(ctx context.Context, tx *dbsql.TXWrapper, message *core.Message) (int64, error) {
	var txParentID *fftypes.UUID
	var txParentType core.TransactionType
	if message.Header.TxParent != nil {
		txParentID = message.Header.TxParent.ID
		txParentType = message.Header.TxParent.Type
	}

	return s.UpdateTx(ctx, messagesTable, tx,
		sq.Update(messagesTable).
			Set("cid", message.Header.CID).
			Set("mtype", string(message.Header.Type)).
			Set("author", message.Header.Author).
			Set("key", message.Header.Key).
			Set("created", message.Header.Created).
			Set("topics", message.Header.Topics).
			Set("tag", message.Header.Tag).
			Set("group_hash", message.Header.Group).
			Set("datahash", message.Header.DataHash).
			Set("hash", message.Hash).
			Set("pins", message.Pins).
			Set("state", message.State).
			Set("confirmed", message.Confirmed).
			Set("reject_reason", message.RejectReason).
			Set("tx_type", message.Header.TxType).
			Set("tx_id", message.TransactionID).
			Set("tx_parent_type", txParentType).
			Set("tx_parent_id", txParentID).
			Set("batch_id", message.BatchID).
			Set("idempotency_key", message.IdempotencyKey).
			Where(sq.Eq{
				"id":              message.Header.ID,
				"hash":            message.Hash,
				"namespace_local": message.LocalNamespace,
				"namespace":       message.Header.Namespace,
			}),
		func() {
			s.callbacks.OrderedUUIDCollectionNSEvent(database.CollectionMessages, core.ChangeEventTypeUpdated, message.LocalNamespace, message.Header.ID, -1 /* not applicable on update */)
		})
}

func (s *SQLCommon) setMessageInsertValues(query sq.InsertBuilder, message *core.Message) sq.InsertBuilder {
	var txParentID *fftypes.UUID
	var txParentType core.TransactionType
	if message.Header.TxParent != nil {
		txParentID = message.Header.TxParent.ID
		txParentType = message.Header.TxParent.Type
	}

	return query.Values(
		message.Header.ID,
		message.Header.CID,
		string(message.Header.Type),
		message.Header.Author,
		message.Header.Key,
		message.Header.Created,
		message.Header.Namespace,
		message.LocalNamespace,
		message.Header.Topics,
		message.Header.Tag,
		message.Header.Group,
		message.Header.DataHash,
		message.Hash,
		message.Pins,
		message.State,
		message.Confirmed,
		message.RejectReason,
		message.Header.TxType,
		message.TransactionID,
		txParentType,
		txParentID,
		message.BatchID,
		message.IdempotencyKey,
	)
}

func (s *SQLCommon) attemptMessageInsert(ctx context.Context, tx *dbsql.TXWrapper, message *core.Message, requestConflictEmptyResult bool) (err error) {
	message.Sequence, err = s.InsertTxExt(ctx, messagesTable, tx,
		s.setMessageInsertValues(sq.Insert(messagesTable).Columns(msgColumns...), message),
		func() {
			s.callbacks.OrderedUUIDCollectionNSEvent(database.CollectionMessages, core.ChangeEventTypeCreated, message.LocalNamespace, message.Header.ID, message.Sequence)
		}, requestConflictEmptyResult)
	return err
}

func (s *SQLCommon) UpsertMessage(ctx context.Context, message *core.Message, optimization database.UpsertOptimization, hooks ...database.PostCompletionHook) (err error) {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	// This is a performance critical function, as we stream data into the database for every message, in every batch.
	//
	// First attempt the operation based on the optimization passed in.
	// The expectation is that the optimization will hit almost all of the time,
	// as only recovery paths require us to go down the un-optimized route.
	optimized := false
	recreateDatarefs := false
	if optimization == database.UpsertOptimizationNew {
		opErr := s.attemptMessageInsert(ctx, tx, message, true /* we want a failure here we can progress past */)
		optimized = opErr == nil
	} else if optimization == database.UpsertOptimizationExisting {
		rowsAffected, opErr := s.attemptMessageUpdate(ctx, tx, message)
		optimized = opErr == nil && rowsAffected == 1
	}

	if !optimized {
		// Do a select within the transaction to detemine if the UUID already exists
		msgRows, _, err := s.QueryTx(ctx, messagesTable, tx,
			sq.Select("hash", s.SequenceColumn()).
				From(messagesTable).
				Where(sq.Eq{"id": message.Header.ID, "namespace_local": message.LocalNamespace}),
		)
		if err != nil {
			return err
		}

		existing := msgRows.Next()
		if existing {
			var hash *fftypes.Bytes32
			_ = msgRows.Scan(&hash, &message.Sequence)
			if !fftypes.SafeHashCompare(hash, message.Hash) {
				msgRows.Close()
				log.L(ctx).Errorf("Existing=%s New=%s", hash, message.Hash)
				return database.HashMismatch
			}
			recreateDatarefs = true // non-optimized update path
		}
		msgRows.Close()

		if existing {
			// Update the message
			if _, err = s.attemptMessageUpdate(ctx, tx, message); err != nil {
				return err
			}
		} else {
			if err = s.attemptMessageInsert(ctx, tx, message, false); err != nil {
				return err
			}
		}
	}

	// Note the message data refs are not allowed to change, as they are part of the hash.
	// So the optimization above relies on the fact these are in a transaction, so the
	// whole message (with datarefs) will have been inserted
	if !optimized || optimization == database.UpsertOptimizationNew {
		if err = s.updateMessageDataRefs(ctx, tx, message, recreateDatarefs); err != nil {
			return err
		}
	}

	for _, hook := range hooks {
		tx.AddPostCommitHook(hook)
	}

	return s.CommitTx(ctx, tx, autoCommit)
}

func (s *SQLCommon) InsertMessages(ctx context.Context, messages []*core.Message, hooks ...database.PostCompletionHook) (err error) {

	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)
	if s.Features().MultiRowInsert {
		msgQuery := sq.Insert(messagesTable).Columns(msgColumns...)
		dataRefQuery := sq.Insert(messagesDataJoinTable).Columns(
			"namespace",
			"message_id",
			"data_id",
			"data_hash",
			"data_idx",
		)
		dataRefCount := 0
		for _, message := range messages {
			msgQuery = s.setMessageInsertValues(msgQuery, message)
			for idx, dataRef := range message.Data {
				dataRefQuery = dataRefQuery.Values(message.LocalNamespace, message.Header.ID, dataRef.ID, dataRef.Hash, idx)
				dataRefCount++
			}
		}
		sequences := make([]int64, len(messages))

		// Use a single multi-row insert for the messages
		err := s.InsertTxRows(ctx, messagesTable, tx, msgQuery, func() {
			for i, message := range messages {
				message.Sequence = sequences[i]
				s.callbacks.OrderedUUIDCollectionNSEvent(database.CollectionMessages, core.ChangeEventTypeCreated, message.LocalNamespace, message.Header.ID, message.Sequence)
			}
		}, sequences, true /* we want the caller to be able to retry with individual upserts */)
		if err != nil {
			return err
		}

		// Use a single multi-row insert for the data refs
		if dataRefCount > 0 {
			dataRefSeqs := make([]int64, dataRefCount)
			err = s.InsertTxRows(ctx, messagesDataJoinTable, tx, dataRefQuery, nil, dataRefSeqs, false)
			if err != nil {
				return err
			}
		}
	} else {
		// Fall back to individual inserts grouped in a TX
		for _, message := range messages {
			err := s.attemptMessageInsert(ctx, tx, message, false)
			if err != nil {
				return err
			}
			err = s.updateMessageDataRefs(ctx, tx, message, false)
			if err != nil {
				return err
			}
		}
	}

	for _, hook := range hooks {
		tx.AddPostCommitHook(hook)
	}

	return s.CommitTx(ctx, tx, autoCommit)

}

// In SQL update+bump is a delete+insert within a TX
func (s *SQLCommon) ReplaceMessage(ctx context.Context, message *core.Message) (err error) {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	if err := s.DeleteTx(ctx, messagesTable, tx,
		sq.Delete(messagesTable).
			Where(sq.And{
				sq.Eq{"id": message.Header.ID, "namespace_local": message.LocalNamespace},
			}),
		nil, // no change event
	); err != nil {
		return err
	}

	if err = s.attemptMessageInsert(ctx, tx, message, false); err != nil {
		return err
	}

	// Note there is no call to updateMessageDataRefs as the data refs are not allowed to change,
	// and are correlated by UUID (not sequence)

	return s.CommitTx(ctx, tx, autoCommit)
}

func (s *SQLCommon) updateMessageDataRefs(ctx context.Context, tx *dbsql.TXWrapper, message *core.Message, recreateDatarefs bool) error {

	if recreateDatarefs {
		// Delete all the existing references, to replace them with new ones below
		if err := s.DeleteTx(ctx, messagesDataJoinTable, tx,
			sq.Delete(messagesDataJoinTable).
				Where(sq.And{
					sq.Eq{"message_id": message.Header.ID, "namespace": message.LocalNamespace},
				}),
			nil, // no change event
		); err != nil && err != database.DeleteRecordNotFound {
			return err
		}
	}

	for msgDataRefIDx, msgDataRef := range message.Data {
		if msgDataRef.ID == nil {
			return i18n.NewError(ctx, coremsgs.MsgNullDataReferenceID, msgDataRefIDx)
		}
		if msgDataRef.Hash == nil {
			return i18n.NewError(ctx, coremsgs.MsgMissingDataHashIndex, msgDataRefIDx)
		}
		// Add the linkage
		if _, err := s.InsertTx(ctx, messagesDataJoinTable, tx,
			sq.Insert(messagesDataJoinTable).
				Columns(
					"namespace",
					"message_id",
					"data_id",
					"data_hash",
					"data_idx",
				).
				Values(
					message.LocalNamespace,
					message.Header.ID,
					msgDataRef.ID,
					msgDataRef.Hash,
					msgDataRefIDx,
				),
			nil, // no change event
		); err != nil {
			return err
		}
	}

	return nil

}

// Why not a LEFT JOIN you ask? ... well we need to be able to reliably perform a LIMIT on
// the number of messages, and it seems there isn't a clean and cross-database
// way for a single-query option. So a two-query option ended up being simplest.
// See commit e304161a30b8044a42b5bac3fcfca7e7bd8f8ab7 for the abandoned changeset
// that implemented LEFT JOIN
func (s *SQLCommon) loadDataRefs(ctx context.Context, namespace string, msgs []*core.Message) error {

	msgIDs := make([]string, len(msgs))
	for i, m := range msgs {
		if m != nil {
			msgIDs[i] = m.Header.ID.String()
		}
	}

	existingRefs, _, err := s.Query(ctx, messagesDataJoinTable,
		sq.Select(
			"message_id",
			"data_id",
			"data_hash",
			"data_idx",
		).
			From(messagesDataJoinTable).
			Where(sq.Eq{"message_id": msgIDs, "namespace": namespace}).
			OrderBy("data_idx"),
	)
	if err != nil {
		return err
	}
	defer existingRefs.Close()

	for existingRefs.Next() {
		var msgID fftypes.UUID
		var dataID fftypes.UUID
		var dataHash fftypes.Bytes32
		var dataIDx int
		if err = existingRefs.Scan(&msgID, &dataID, &dataHash, &dataIDx); err != nil {
			return i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, messagesDataJoinTable)
		}
		for _, m := range msgs {
			if *m.Header.ID == msgID {
				m.Data = append(m.Data, &core.DataRef{
					ID:   &dataID,
					Hash: &dataHash,
				})
			}
		}
	}
	// Ensure we return an empty array if no entries, and a consistent order for the data
	for _, m := range msgs {
		if m.Data == nil {
			m.Data = core.DataRefs{}
		}
	}

	return nil
}

func (s *SQLCommon) msgResult(ctx context.Context, row *sql.Rows) (*core.Message, error) {
	var msg core.Message
	var txParent core.TransactionRef
	err := row.Scan(
		&msg.Header.ID,
		&msg.Header.CID,
		&msg.Header.Type,
		&msg.Header.Author,
		&msg.Header.Key,
		&msg.Header.Created,
		&msg.Header.Namespace,
		&msg.LocalNamespace,
		&msg.Header.Topics,
		&msg.Header.Tag,
		&msg.Header.Group,
		&msg.Header.DataHash,
		&msg.Hash,
		&msg.Pins,
		&msg.State,
		&msg.Confirmed,
		&msg.RejectReason,
		&msg.Header.TxType,
		&msg.TransactionID,
		&txParent.Type,
		&txParent.ID,
		&msg.BatchID,
		&msg.IdempotencyKey,
		// Must be added to the list of columns in all selects
		&msg.Sequence,
	)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, messagesTable)
	}
	if txParent.ID != nil {
		msg.Header.TxParent = &txParent
	}
	return &msg, nil
}

func (s *SQLCommon) GetMessageByID(ctx context.Context, namespace string, id *fftypes.UUID) (message *core.Message, err error) {

	cols := append([]string{}, msgColumns...)
	cols = append(cols, s.SequenceColumn())
	rows, _, err := s.Query(ctx, messagesTable,
		sq.Select(cols...).
			From(messagesTable).
			Where(sq.Eq{"id": id, "namespace_local": namespace}),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		log.L(ctx).Debugf("Message '%s' not found", id)
		return nil, nil
	}

	msg, err := s.msgResult(ctx, rows)
	if err != nil {
		return nil, err
	}

	rows.Close()
	if err = s.loadDataRefs(ctx, namespace, []*core.Message{msg}); err != nil {
		return nil, err
	}

	return msg, nil
}

func (s *SQLCommon) getMessagesQuery(ctx context.Context, namespace string, query sq.SelectBuilder, fop sq.Sqlizer, fi *ffapi.FilterInfo, allowCount bool) (message []*core.Message, fr *ffapi.FilterResult, err error) {
	if fi.Count && !allowCount {
		return nil, nil, i18n.NewError(ctx, coremsgs.MsgFilterCountNotSupported)
	}

	rows, tx, err := s.Query(ctx, messagesTable, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	msgs := []*core.Message{}
	for rows.Next() {
		msg, err := s.msgResult(ctx, rows)
		if err != nil {
			return nil, nil, err
		}
		msgs = append(msgs, msg)
	}

	rows.Close()
	if len(msgs) > 0 {
		if err = s.loadDataRefs(ctx, namespace, msgs); err != nil {
			return nil, nil, err
		}
	}
	return msgs, s.QueryRes(ctx, messagesTable, tx, fop, nil, fi), err
}

func (s *SQLCommon) GetMessageIDs(ctx context.Context, namespace string, filter ffapi.Filter) (ids []*core.IDAndSequence, err error) {
	query, _, _, err := s.FilterSelect(ctx, "", sq.Select("id", s.SequenceColumn()).From(messagesTable), filter, msgFilterFieldMap,
		[]interface{}{
			&ffapi.SortField{Field: "confirmed", Descending: true, Nulls: ffapi.NullsFirst},
			"created",
		}, sq.Eq{"namespace_local": namespace})
	if err != nil {
		return nil, err
	}

	rows, _, err := s.Query(ctx, messagesTable, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	ids = []*core.IDAndSequence{}
	for rows.Next() {
		var id core.IDAndSequence
		err = rows.Scan(&id.ID, &id.Sequence)
		if err != nil {
			return nil, i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, messagesTable)
		}
		ids = append(ids, &id)
	}
	return ids, nil
}

func (s *SQLCommon) GetBatchIDsForDataAttachments(ctx context.Context, namespace string, dataIDs []*fftypes.UUID) (batchIDs []*fftypes.UUID, err error) {
	query := sq.Select("m.batch_id").From("messages_data AS md").LeftJoin("messages AS m ON m.id = md.message_id").
		Where(sq.Eq{"md.data_id": dataIDs, "md.namespace": namespace})
	return s.queryBatchIDs(ctx, query)
}

func (s *SQLCommon) GetBatchIDsForMessages(ctx context.Context, namespace string, msgIDs []*fftypes.UUID) (batchIDs []*fftypes.UUID, err error) {
	return s.queryBatchIDs(ctx, sq.Select("batch_id").From(messagesTable).
		Where(sq.Eq{"id": msgIDs, "namespace_local": namespace}))
}

func (s *SQLCommon) queryBatchIDs(ctx context.Context, query sq.SelectBuilder) (batchIDs []*fftypes.UUID, err error) {
	rows, _, err := s.Query(ctx, messagesTable, query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	batchIDs = []*fftypes.UUID{}
	for rows.Next() {
		var batchID *fftypes.UUID
		err = rows.Scan(&batchID)
		if err != nil {
			return nil, i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, messagesTable)
		}
		// Only append non-nil batch IDs
		if batchID != nil {
			batchIDs = append(batchIDs, batchID)
		}
	}
	return batchIDs, nil
}

func (s *SQLCommon) GetMessages(ctx context.Context, namespace string, filter ffapi.Filter) (message []*core.Message, fr *ffapi.FilterResult, err error) {
	cols := append([]string{}, msgColumns...)
	cols = append(cols, s.SequenceColumn())
	query, fop, fi, err := s.FilterSelect(ctx, "", sq.Select(cols...).From(messagesTable), filter, msgFilterFieldMap,
		[]interface{}{
			&ffapi.SortField{Field: "confirmed", Descending: true, Nulls: ffapi.NullsFirst},
			&ffapi.SortField{Field: "created", Descending: true},
		}, sq.Eq{"namespace_local": namespace})
	if err != nil {
		return nil, nil, err
	}
	return s.getMessagesQuery(ctx, namespace, query, fop, fi, true)
}

func (s *SQLCommon) GetMessagesForData(ctx context.Context, namespace string, dataID *fftypes.UUID, filter ffapi.Filter) (message []*core.Message, fr *ffapi.FilterResult, err error) {
	cols := make([]string, len(msgColumns)+1)
	for i, col := range msgColumns {
		cols[i] = fmt.Sprintf("m.%s", col)
	}
	cols[len(msgColumns)] = "m.seq"
	query, fop, fi, err := s.FilterSelect(
		ctx, "m", sq.Select(cols...).From("messages_data AS md"),
		filter, msgFilterFieldMap, []interface{}{"sequence"},
		sq.Eq{"md.data_id": dataID, "md.namespace": namespace})
	if err != nil {
		return nil, nil, err
	}

	query = query.LeftJoin("messages AS m ON m.id = md.message_id")
	return s.getMessagesQuery(ctx, namespace, query, fop, fi, false)
}

func (s *SQLCommon) UpdateMessage(ctx context.Context, namespace string, msgid *fftypes.UUID, update ffapi.Update) (err error) {
	return s.UpdateMessages(ctx, namespace, database.MessageQueryFactory.NewFilter(ctx).Eq("id", msgid), update)
}

func (s *SQLCommon) UpdateMessages(ctx context.Context, namespace string, filter ffapi.Filter, update ffapi.Update) (err error) {

	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	query, err := s.BuildUpdate(sq.Update(messagesTable).Where(sq.Eq{"namespace_local": namespace}), update, msgFilterFieldMap)
	if err != nil {
		return err
	}

	query, err = s.FilterUpdate(ctx, query, filter, msgFilterFieldMap)
	if err != nil {
		return err
	}

	_, err = s.UpdateTx(ctx, messagesTable, tx, query, nil /* no change events filter based update */)
	if err != nil {
		return err
	}

	return s.CommitTx(ctx, tx, autoCommit)
}
