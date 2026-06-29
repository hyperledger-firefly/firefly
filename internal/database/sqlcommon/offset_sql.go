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
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

var (
	offsetColumns = []string{
		"otype",
		"name",
		"current",
	}
	offsetFilterFieldMap = map[string]string{
		"type": "otype",
	}
)

const offsetsTable = "offsets"

func (s *SQLCommon) UpsertOffset(ctx context.Context, offset *core.Offset, allowExisting bool) (err error) {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	existing := false
	if allowExisting {
		// Do a select within the transaction to detemine if the UUID already exists
		offsetRows, _, err := s.QueryTx(ctx, offsetsTable, tx,
			sq.Select(s.SequenceColumn()).
				From(offsetsTable).
				Where(
					sq.Eq{"otype": offset.Type,
						"name": offset.Name}),
		)
		if err != nil {
			return err
		}
		existing = offsetRows.Next()
		if existing {
			err := offsetRows.Scan(&offset.RowID)
			if err != nil {
				offsetRows.Close()
				return i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, offsetsTable)
			}
		}
		offsetRows.Close()
	}

	if existing {

		// Update the offset
		if _, err = s.UpdateTx(ctx, offsetsTable, tx,
			sq.Update(offsetsTable).
				Set("otype", string(offset.Type)).
				Set("name", offset.Name).
				Set("current", offset.Current).
				Where(sq.Eq{s.SequenceColumn(): offset.RowID}),
			nil, // offsets do not have events
		); err != nil {
			return err
		}
	} else {
		if offset.RowID, err = s.InsertTx(ctx, offsetsTable, tx,
			sq.Insert(offsetsTable).
				Columns(offsetColumns...).
				Values(
					string(offset.Type),
					offset.Name,
					offset.Current,
				),
			nil, // offsets do not have events
		); err != nil {
			return err
		}
	}

	return s.CommitTx(ctx, tx, autoCommit)
}

func (s *SQLCommon) offsetResult(ctx context.Context, row *sql.Rows) (*core.Offset, error) {
	offset := core.Offset{}
	err := row.Scan(
		&offset.Type,
		&offset.Name,
		&offset.Current,
		&offset.RowID, // must include s.SequenceColumn() in colum list
	)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, offsetsTable)
	}
	return &offset, nil
}

func (s *SQLCommon) GetOffset(ctx context.Context, t core.OffsetType, name string) (message *core.Offset, err error) {

	cols := append([]string{}, offsetColumns...)
	cols = append(cols, s.SequenceColumn())
	rows, _, err := s.Query(ctx, offsetsTable,
		sq.Select(cols...).
			From(offsetsTable).
			Where(sq.Eq{
				"otype": t,
				"name":  name,
			}),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		log.L(ctx).Debugf("Offset '%s:%s' not found", t, name)
		return nil, nil
	}

	offset, err := s.offsetResult(ctx, rows)
	if err != nil {
		return nil, err
	}

	return offset, nil
}

func (s *SQLCommon) GetOffsets(ctx context.Context, filter ffapi.Filter) (message []*core.Offset, fr *ffapi.FilterResult, err error) {

	cols := append([]string{}, offsetColumns...)
	cols = append(cols, s.SequenceColumn())
	query, fop, fi, err := s.FilterSelect(ctx, "", sq.Select(cols...).From(offsetsTable), filter, offsetFilterFieldMap, []interface{}{"sequence"})
	if err != nil {
		return nil, nil, err
	}

	rows, tx, err := s.Query(ctx, offsetsTable, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	offset := []*core.Offset{}
	for rows.Next() {
		d, err := s.offsetResult(ctx, rows)
		if err != nil {
			return nil, nil, err
		}
		offset = append(offset, d)
	}

	return offset, s.QueryRes(ctx, offsetsTable, tx, fop, nil, fi), err

}

func (s *SQLCommon) UpdateOffset(ctx context.Context, rowID int64, update ffapi.Update) (err error) {

	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	query, err := s.BuildUpdate(sq.Update(offsetsTable), update, offsetFilterFieldMap)
	if err != nil {
		return err
	}
	query = query.Where(sq.Eq{s.SequenceColumn(): rowID})

	_, err = s.UpdateTx(ctx, offsetsTable, tx, query, nil /* offsets do not have change events */)
	if err != nil {
		return err
	}

	return s.CommitTx(ctx, tx, autoCommit)
}

func (s *SQLCommon) DeleteOffset(ctx context.Context, t core.OffsetType, name string) (err error) {

	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	offset, err := s.GetOffset(ctx, t, name)
	if err != nil {
		return err
	}
	if offset != nil {
		err = s.DeleteTx(ctx, offsetsTable, tx, sq.Delete(offsetsTable).Where(sq.Eq{
			s.SequenceColumn(): offset.RowID,
		}), nil /* offsets do not have change events */)
		if err != nil {
			return err
		}
	}

	return s.CommitTx(ctx, tx, autoCommit)
}
