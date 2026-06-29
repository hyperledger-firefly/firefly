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
	ffiMethodsColumns = []string{
		"id",
		"interface_id",
		"namespace",
		"name",
		"pathname",
		"description",
		"params",
		"returns",
		"details",
	}
	ffiMethodFilterFieldMap = map[string]string{
		"interface": "interface_id",
	}
)

const ffimethodsTable = "ffimethods"

func (s *SQLCommon) UpsertFFIMethod(ctx context.Context, method *fftypes.FFIMethod) (err error) {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	rows, _, err := s.QueryTx(ctx, ffimethodsTable, tx,
		sq.Select("id").
			From(ffimethodsTable).
			Where(sq.And{sq.Eq{"interface_id": method.Interface}, sq.Eq{"namespace": method.Namespace}, sq.Eq{"pathname": method.Pathname}}),
	)
	if err != nil {
		return err
	}
	existing := rows.Next()
	rows.Close()

	if existing {
		if _, err = s.UpdateTx(ctx, ffimethodsTable, tx,
			sq.Update(ffimethodsTable).
				Set("params", method.Params).
				Set("returns", method.Returns).
				Where(sq.And{sq.Eq{"interface_id": method.Interface}, sq.Eq{"namespace": method.Namespace}, sq.Eq{"pathname": method.Pathname}}),
			func() {
				s.callbacks.UUIDCollectionNSEvent(database.CollectionFFIMethods, core.ChangeEventTypeUpdated, method.Namespace, method.ID)
			},
		); err != nil {
			return err
		}
	} else {
		if _, err = s.InsertTx(ctx, ffimethodsTable, tx,
			sq.Insert(ffimethodsTable).
				Columns(ffiMethodsColumns...).
				Values(
					method.ID,
					method.Interface,
					method.Namespace,
					method.Name,
					method.Pathname,
					method.Description,
					method.Params,
					method.Returns,
					method.Details,
				),
			func() {
				s.callbacks.UUIDCollectionNSEvent(database.CollectionFFIMethods, core.ChangeEventTypeCreated, method.Namespace, method.ID)
			},
		); err != nil {
			return err
		}
	}

	return s.CommitTx(ctx, tx, autoCommit)
}

func (s *SQLCommon) ffiMethodResult(ctx context.Context, row *sql.Rows) (*fftypes.FFIMethod, error) {
	method := fftypes.FFIMethod{}
	err := row.Scan(
		&method.ID,
		&method.Interface,
		&method.Namespace,
		&method.Name,
		&method.Pathname,
		&method.Description,
		&method.Params,
		&method.Returns,
		&method.Details,
	)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, ffimethodsTable)
	}
	return &method, nil
}

func (s *SQLCommon) getFFIMethodPred(ctx context.Context, desc string, pred interface{}) (*fftypes.FFIMethod, error) {
	rows, _, err := s.Query(ctx, ffimethodsTable,
		sq.Select(ffiMethodsColumns...).
			From(ffimethodsTable).
			Where(pred),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		log.L(ctx).Debugf("FFI method '%s' not found", desc)
		return nil, nil
	}

	ci, err := s.ffiMethodResult(ctx, rows)
	if err != nil {
		return nil, err
	}

	return ci, nil
}

func (s *SQLCommon) GetFFIMethods(ctx context.Context, namespace string, filter ffapi.Filter) (methods []*fftypes.FFIMethod, res *ffapi.FilterResult, err error) {
	query, fop, fi, err := s.FilterSelect(ctx, "", sq.Select(ffiMethodsColumns...).From(ffimethodsTable),
		filter, ffiMethodFilterFieldMap, []interface{}{"sequence"}, sq.Eq{"namespace": namespace})
	if err != nil {
		return nil, nil, err
	}

	rows, tx, err := s.Query(ctx, ffimethodsTable, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	for rows.Next() {
		ci, err := s.ffiMethodResult(ctx, rows)
		if err != nil {
			return nil, nil, err
		}
		methods = append(methods, ci)
	}

	return methods, s.QueryRes(ctx, ffimethodsTable, tx, fop, nil, fi), err

}

func (s *SQLCommon) GetFFIMethod(ctx context.Context, ns string, interfaceID *fftypes.UUID, pathName string) (*fftypes.FFIMethod, error) {
	return s.getFFIMethodPred(ctx, ns+":"+pathName, sq.Eq{"namespace": ns, "interface_id": interfaceID, "pathname": pathName})
}
