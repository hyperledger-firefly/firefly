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
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

var (
	ffiErrorsColumns = []string{
		"id",
		"interface_id",
		"namespace",
		"name",
		"pathname",
		"description",
		"params",
	}
	ffiErrorFilterFieldMap = map[string]string{
		"interface": "interface_id",
	}
)

const ffierrorsTable = "ffierrors"

func (s *SQLCommon) UpsertFFIError(ctx context.Context, errorDef *fftypes.FFIError) (err error) {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	rows, _, err := s.QueryTx(ctx, ffierrorsTable, tx,
		sq.Select("id").
			From(ffierrorsTable).
			Where(sq.And{sq.Eq{"interface_id": errorDef.Interface}, sq.Eq{"namespace": errorDef.Namespace}, sq.Eq{"pathname": errorDef.Pathname}}),
	)
	if err != nil {
		return err
	}
	existing := rows.Next()
	rows.Close()

	if existing {
		if _, err = s.UpdateTx(ctx, ffierrorsTable, tx,
			sq.Update(ffierrorsTable).
				Set("params", errorDef.Params).
				Where(sq.And{sq.Eq{"interface_id": errorDef.Interface}, sq.Eq{"namespace": errorDef.Namespace}, sq.Eq{"pathname": errorDef.Pathname}}),
			func() {
				s.callbacks.UUIDCollectionNSEvent(database.CollectionFFIErrors, core.ChangeEventTypeUpdated, errorDef.Namespace, errorDef.ID)
			},
		); err != nil {
			return err
		}
	} else {
		if _, err = s.InsertTx(ctx, ffierrorsTable, tx,
			sq.Insert(ffierrorsTable).
				Columns(ffiErrorsColumns...).
				Values(
					errorDef.ID,
					errorDef.Interface,
					errorDef.Namespace,
					errorDef.Name,
					errorDef.Pathname,
					errorDef.Description,
					errorDef.Params,
				),
			func() {
				s.callbacks.UUIDCollectionNSEvent(database.CollectionFFIErrors, core.ChangeEventTypeCreated, errorDef.Namespace, errorDef.ID)
			},
		); err != nil {
			return err
		}
	}

	return s.CommitTx(ctx, tx, autoCommit)
}

func (s *SQLCommon) ffiErrorResult(ctx context.Context, row *sql.Rows) (*fftypes.FFIError, error) {
	errorDef := fftypes.FFIError{}
	err := row.Scan(
		&errorDef.ID,
		&errorDef.Interface,
		&errorDef.Namespace,
		&errorDef.Name,
		&errorDef.Pathname,
		&errorDef.Description,
		&errorDef.Params,
	)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, ffierrorsTable)
	}
	return &errorDef, nil
}

func (s *SQLCommon) GetFFIErrors(ctx context.Context, namespace string, filter ffapi.Filter) (errors []*fftypes.FFIError, res *ffapi.FilterResult, err error) {
	query, fop, fi, err := s.FilterSelect(ctx, "", sq.Select(ffiErrorsColumns...).From(ffierrorsTable),
		filter, ffiErrorFilterFieldMap, []interface{}{"sequence"}, sq.Eq{"namespace": namespace})
	if err != nil {
		return nil, nil, err
	}

	rows, tx, err := s.Query(ctx, ffierrorsTable, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	for rows.Next() {
		ci, err := s.ffiErrorResult(ctx, rows)
		if err != nil {
			return nil, nil, err
		}
		errors = append(errors, ci)
	}

	return errors, s.QueryRes(ctx, ffierrorsTable, tx, fop, nil, fi), err

}
