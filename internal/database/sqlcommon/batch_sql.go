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

	sq "github.com/Masterminds/squirrel"
	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

var (
	batchColumns = []string{
		"id",
		"btype",
		"namespace",
		"author",
		"key",
		"group_hash",
		"created",
		"hash",
		"manifest",
		"confirmed",
		"tx_type",
		"tx_id",
		"node_id",
	}
	batchFilterFieldMap = map[string]string{
		"type":    "btype",
		"tx.type": "tx_type",
		"tx.id":   "tx_id",
		"group":   "group_hash",
		"node":    "node_id",
	}
)

const batchesTable = "batches"

func (s *SQLCommon) InsertOrGetBatch(ctx context.Context, batch *core.BatchPersisted) (existing *core.BatchPersisted, err error) {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return nil, err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	// Try the insert first
	_, insertErr := s.InsertTxExt(ctx, batchesTable, tx,
		sq.Insert(batchesTable).
			Columns(batchColumns...).
			Values(
				batch.ID,
				string(batch.Type),
				batch.Namespace,
				batch.Author,
				batch.Key,
				batch.Group,
				batch.Created,
				batch.Hash,
				batch.Manifest,
				batch.Confirmed,
				batch.TX.Type,
				batch.TX.ID,
				batch.Node,
			),
		func() {
			s.callbacks.UUIDCollectionNSEvent(database.CollectionBatches, core.ChangeEventTypeCreated, batch.Namespace, batch.ID)
		},
		true, /* we want a failure here we can progress past */
	)
	if insertErr == nil {
		return nil, s.CommitTx(ctx, tx, autoCommit)
	}

	// Do a select within the transaction to determine if the batch already exists
	existing, err = s.GetBatchByID(ctx, batch.Namespace, batch.ID)
	if err != nil || existing != nil {
		return existing, err
	}

	// Error was apparently not an ID conflict - must have been something else
	return nil, insertErr
}

func (s *SQLCommon) batchResult(ctx context.Context, row *sql.Rows) (*core.BatchPersisted, error) {
	var batch core.BatchPersisted
	err := row.Scan(
		&batch.ID,
		&batch.Type,
		&batch.Namespace,
		&batch.Author,
		&batch.Key,
		&batch.Group,
		&batch.Created,
		&batch.Hash,
		&batch.Manifest,
		&batch.Confirmed,
		&batch.TX.Type,
		&batch.TX.ID,
		&batch.Node,
	)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, batchesTable)
	}
	return &batch, nil
}

func (s *SQLCommon) GetBatchByID(ctx context.Context, namespace string, id *fftypes.UUID) (message *core.BatchPersisted, err error) {

	rows, _, err := s.Query(ctx, batchesTable,
		sq.Select(batchColumns...).
			From(batchesTable).
			Where(sq.Eq{"id": id, "namespace": namespace}),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		log.L(ctx).Debugf("Batch '%s' not found", id)
		return nil, nil
	}

	batch, err := s.batchResult(ctx, rows)
	if err != nil {
		return nil, err
	}

	return batch, nil
}

func (s *SQLCommon) GetBatches(ctx context.Context, namespace string, filter ffapi.Filter) (message []*core.BatchPersisted, res *ffapi.FilterResult, err error) {

	query, fop, fi, err := s.FilterSelect(ctx, "", sq.Select(batchColumns...).From(batchesTable), filter, batchFilterFieldMap, []interface{}{"sequence"}, sq.Eq{"namespace": namespace})
	if err != nil {
		return nil, nil, err
	}

	rows, tx, err := s.Query(ctx, batchesTable, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	batches := []*core.BatchPersisted{}
	for rows.Next() {
		batch, err := s.batchResult(ctx, rows)
		if err != nil {
			return nil, nil, err
		}
		batches = append(batches, batch)
	}

	return batches, s.QueryRes(ctx, batchesTable, tx, fop, nil, fi), err

}

func (s *SQLCommon) UpdateBatch(ctx context.Context, namespace string, id *fftypes.UUID, update ffapi.Update) (err error) {

	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	query, err := s.BuildUpdate(sq.Update(batchesTable), update, batchFilterFieldMap)
	if err != nil {
		return err
	}
	query = query.Where(sq.Eq{"id": id, "namespace": namespace})

	_, err = s.UpdateTx(ctx, batchesTable, tx, query, nil /* no change events on filter update */)
	if err != nil {
		return err
	}

	return s.CommitTx(ctx, tx, autoCommit)
}
