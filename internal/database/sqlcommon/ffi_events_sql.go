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
	ffiEventsColumns = []string{
		"id",
		"interface_id",
		"namespace",
		"name",
		"pathname",
		"description",
		"params",
		"details",
	}
	ffiEventFilterFieldMap = map[string]string{
		"interface": "interface_id",
	}
)

const ffieventsTable = "ffievents"

func (s *SQLCommon) UpsertFFIEvent(ctx context.Context, event *fftypes.FFIEvent) (err error) {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	rows, _, err := s.QueryTx(ctx, ffieventsTable, tx,
		sq.Select("id").
			From(ffieventsTable).
			Where(sq.And{sq.Eq{"interface_id": event.Interface}, sq.Eq{"namespace": event.Namespace}, sq.Eq{"pathname": event.Pathname}}),
	)
	if err != nil {
		return err
	}
	existing := rows.Next()
	rows.Close()

	if existing {
		if _, err = s.UpdateTx(ctx, ffieventsTable, tx,
			sq.Update(ffieventsTable).
				Set("params", event.Params).
				Where(sq.And{sq.Eq{"interface_id": event.Interface}, sq.Eq{"namespace": event.Namespace}, sq.Eq{"pathname": event.Pathname}}),
			func() {
				s.callbacks.UUIDCollectionNSEvent(database.CollectionFFIEvents, core.ChangeEventTypeUpdated, event.Namespace, event.ID)
			},
		); err != nil {
			return err
		}
	} else {
		if _, err = s.InsertTx(ctx, ffieventsTable, tx,
			sq.Insert(ffieventsTable).
				Columns(ffiEventsColumns...).
				Values(
					event.ID,
					event.Interface,
					event.Namespace,
					event.Name,
					event.Pathname,
					event.Description,
					event.Params,
					event.Details,
				),
			func() {
				s.callbacks.UUIDCollectionNSEvent(database.CollectionFFIEvents, core.ChangeEventTypeCreated, event.Namespace, event.ID)
			},
		); err != nil {
			return err
		}
	}

	return s.CommitTx(ctx, tx, autoCommit)
}

func (s *SQLCommon) ffiEventResult(ctx context.Context, row *sql.Rows) (*fftypes.FFIEvent, error) {
	event := fftypes.FFIEvent{}
	err := row.Scan(
		&event.ID,
		&event.Interface,
		&event.Namespace,
		&event.Name,
		&event.Pathname,
		&event.Description,
		&event.Params,
		&event.Details,
	)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, ffieventsTable)
	}
	return &event, nil
}

func (s *SQLCommon) getFFIEventPred(ctx context.Context, desc string, pred interface{}) (*fftypes.FFIEvent, error) {
	rows, _, err := s.Query(ctx, ffieventsTable,
		sq.Select(ffiEventsColumns...).
			From(ffieventsTable).
			Where(pred),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		log.L(ctx).Debugf("FFI event '%s' not found", desc)
		return nil, nil
	}

	ci, err := s.ffiEventResult(ctx, rows)
	if err != nil {
		return nil, err
	}

	return ci, nil
}

func (s *SQLCommon) GetFFIEvents(ctx context.Context, namespace string, filter ffapi.Filter) (events []*fftypes.FFIEvent, res *ffapi.FilterResult, err error) {
	query, fop, fi, err := s.FilterSelect(ctx, "", sq.Select(ffiEventsColumns...).From(ffieventsTable),
		filter, ffiEventFilterFieldMap, []interface{}{"sequence"}, sq.Eq{"namespace": namespace})
	if err != nil {
		return nil, nil, err
	}

	rows, tx, err := s.Query(ctx, ffieventsTable, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	for rows.Next() {
		ci, err := s.ffiEventResult(ctx, rows)
		if err != nil {
			return nil, nil, err
		}
		events = append(events, ci)
	}

	return events, s.QueryRes(ctx, ffieventsTable, tx, fop, nil, fi), err

}

func (s *SQLCommon) GetFFIEvent(ctx context.Context, namespace string, interfaceID *fftypes.UUID, pathName string) (*fftypes.FFIEvent, error) {
	return s.getFFIEventPred(ctx, namespace+":"+pathName, sq.Eq{"namespace": namespace, "interface_id": interfaceID, "pathname": pathName})
}
