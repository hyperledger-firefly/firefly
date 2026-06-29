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
	"github.com/hyperledger-firefly/common/pkg/dbsql"
	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

var (
	blobColumns = []string{
		"namespace",
		"hash",
		"payload_ref",
		"created",
		"peer",
		"size",
		"data_id",
	}
	blobFilterFieldMap = map[string]string{
		"payloadref": "payload_ref",
	}
)

const blobsTable = "blobs"

func (s *SQLCommon) InsertBlob(ctx context.Context, blob *core.Blob) (err error) {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	err = s.attemptBlobInsert(ctx, tx, blob)
	if err != nil {
		return err
	}

	return s.CommitTx(ctx, tx, autoCommit)
}

func (s *SQLCommon) setBlobInsertValues(query sq.InsertBuilder, blob *core.Blob) sq.InsertBuilder {
	return query.Values(
		blob.Namespace,
		blob.Hash,
		blob.PayloadRef,
		blob.Created,
		blob.Peer,
		blob.Size,
		blob.DataID,
	)
}

func (s *SQLCommon) attemptBlobInsert(ctx context.Context, tx *dbsql.TXWrapper, blob *core.Blob) (err error) {
	blob.Sequence, err = s.InsertTx(ctx, blobsTable, tx,
		s.setBlobInsertValues(sq.Insert(blobsTable).Columns(blobColumns...), blob),
		nil, // no change events for blobs
	)
	return err
}

func (s *SQLCommon) InsertBlobs(ctx context.Context, blobs []*core.Blob) (err error) {

	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	if s.Features().MultiRowInsert {
		query := sq.Insert(blobsTable).Columns(blobColumns...)
		for _, blob := range blobs {
			query = s.setBlobInsertValues(query, blob)
		}
		sequences := make([]int64, len(blobs))
		err := s.InsertTxRows(ctx, blobsTable, tx, query,
			nil, /* no change events for blobs */
			sequences,
			true /* we want the caller to be able to retry with individual upserts */)
		if err != nil {
			return err
		}
	} else {
		// Fall back to individual inserts grouped in a TX
		for _, blob := range blobs {
			err := s.attemptBlobInsert(ctx, tx, blob)
			if err != nil {
				return err
			}
		}
	}

	return s.CommitTx(ctx, tx, autoCommit)

}

func (s *SQLCommon) blobResult(ctx context.Context, row *sql.Rows) (*core.Blob, error) {
	blob := core.Blob{}
	err := row.Scan(
		&blob.Namespace,
		&blob.Hash,
		&blob.PayloadRef,
		&blob.Created,
		&blob.Peer,
		&blob.Size,
		&blob.DataID,
		&blob.Sequence,
	)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, blobsTable)
	}
	return &blob, nil
}

func (s *SQLCommon) GetBlobs(ctx context.Context, namespace string, filter ffapi.Filter) (message []*core.Blob, res *ffapi.FilterResult, err error) {

	cols := append([]string{}, blobColumns...)
	cols = append(cols, s.SequenceColumn())
	query, fop, fi, err := s.FilterSelect(
		ctx,
		"",
		sq.Select(cols...).From(blobsTable), filter, blobFilterFieldMap,
		[]interface{}{"sequence"},
		sq.Eq{"namespace": namespace})
	if err != nil {
		return nil, nil, err
	}

	rows, tx, err := s.Query(ctx, blobsTable, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	blob := []*core.Blob{}
	for rows.Next() {
		d, err := s.blobResult(ctx, rows)
		if err != nil {
			return nil, nil, err
		}
		blob = append(blob, d)
	}

	return blob, s.QueryRes(ctx, blobsTable, tx, fop, nil, fi), err

}

func (s *SQLCommon) DeleteBlob(ctx context.Context, sequence int64) (err error) {

	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	err = s.DeleteTx(ctx, blobsTable, tx, sq.Delete(blobsTable).Where(sq.Eq{
		s.SequenceColumn(): sequence,
	}), nil /* no change events for blobs */)
	if err != nil {
		return err
	}

	return s.CommitTx(ctx, tx, autoCommit)
}
