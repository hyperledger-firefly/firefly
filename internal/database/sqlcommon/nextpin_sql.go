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
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

var (
	nextpinColumns = []string{
		"namespace",
		"context",
		"identity",
		"hash",
		"nonce",
	}
)

const nextpinsTable = "nextpins"

func (s *SQLCommon) InsertNextPin(ctx context.Context, nextpin *core.NextPin) (err error) {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	sequence, err := s.InsertTx(ctx, nextpinsTable, tx,
		sq.Insert(nextpinsTable).
			Columns(nextpinColumns...).
			Values(
				nextpin.Namespace,
				nextpin.Context,
				nextpin.Identity,
				nextpin.Hash,
				nextpin.Nonce,
			),
		nil, // no change events for next pins
	)
	if err != nil {
		return err
	}
	nextpin.Sequence = sequence

	return s.CommitTx(ctx, tx, autoCommit)
}

func (s *SQLCommon) nextpinResult(ctx context.Context, row *sql.Rows) (*core.NextPin, error) {
	nextpin := core.NextPin{}
	err := row.Scan(
		&nextpin.Namespace,
		&nextpin.Context,
		&nextpin.Identity,
		&nextpin.Hash,
		&nextpin.Nonce,
		&nextpin.Sequence,
	)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, nextpinsTable)
	}
	return &nextpin, nil
}

func (s *SQLCommon) GetNextPinsForContext(ctx context.Context, namespace string, context *fftypes.Bytes32) (message []*core.NextPin, err error) {

	cols := append([]string{}, nextpinColumns...)
	cols = append(cols, s.SequenceColumn())

	rows, _, err := s.Query(ctx, nextpinsTable, sq.Select(cols...).From(nextpinsTable).
		Where(sq.And{
			sq.Eq{"context": context},
			sq.Or{sq.Eq{"namespace": namespace}, sq.Eq{"namespace": nil}}, // old entries will have NULL for namespace
		}))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	nextpin := []*core.NextPin{}
	for rows.Next() {
		d, err := s.nextpinResult(ctx, rows)
		if err != nil {
			return nil, err
		}
		nextpin = append(nextpin, d)
	}

	return nextpin, err

}

func (s *SQLCommon) GetNextPins(ctx context.Context, namespace string, filter ffapi.Filter) ([]*core.NextPin, *ffapi.FilterResult, error) {

	cols := append([]string{}, nextpinColumns...)
	cols = append(cols, s.SequenceColumn())

	query, fop, fi, err := s.FilterSelect(
		ctx, "", sq.Select(cols...).From(nextpinsTable),
		filter, pinFilterFieldMap, []interface{}{"sequence"}, sq.Eq{"namespace": namespace})
	if err != nil {
		return nil, nil, err
	}

	rows, tx, err := s.Query(ctx, pinsTable, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	nextpins := []*core.NextPin{}
	for rows.Next() {
		d, err := s.nextpinResult(ctx, rows)
		if err != nil {
			return nil, nil, err
		}
		nextpins = append(nextpins, d)
	}

	return nextpins, s.QueryRes(ctx, pinsTable, tx, fop, nil, fi), err

}

func (s *SQLCommon) UpdateNextPin(ctx context.Context, namespace string, sequence int64, update ffapi.Update) (err error) {

	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	query, err := s.BuildUpdate(sq.Update(nextpinsTable), update, pinFilterFieldMap)
	if err != nil {
		return err
	}
	query = query.Set("namespace", namespace) // always populate namespace (to migrate the table over time)
	query = query.Where(sq.Eq{s.SequenceColumn(): sequence})

	_, err = s.UpdateTx(ctx, nextpinsTable, tx, query, nil /* no change events for next pins */)
	if err != nil {
		return err
	}

	return s.CommitTx(ctx, tx, autoCommit)
}
