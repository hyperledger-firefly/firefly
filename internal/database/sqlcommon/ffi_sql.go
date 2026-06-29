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
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

var (
	ffiColumns = []string{
		"id",
		"namespace",
		"name",
		"network_name",
		"version",
		"description",
		"message_id",
		"published",
	}
	ffiFilterFieldMap = map[string]string{
		"message":     "message_id",
		"networkname": "network_name",
	}
)

const ffiTable = "ffi"

func (s *SQLCommon) attemptFFIUpdate(ctx context.Context, tx *dbsql.TXWrapper, ffi *fftypes.FFI) (int64, error) {
	var networkName *string
	if ffi.NetworkName != "" {
		networkName = &ffi.NetworkName
	}
	return s.UpdateTx(ctx, ffiTable, tx,
		sq.Update(ffiTable).
			Set("name", ffi.Name).
			Set("network_name", networkName).
			Set("version", ffi.Version).
			Set("description", ffi.Description).
			Set("message_id", ffi.Message).
			Set("published", ffi.Published).
			Where(sq.Eq{"id": ffi.ID}),
		func() {
			s.callbacks.UUIDCollectionNSEvent(database.CollectionFFIs, core.ChangeEventTypeUpdated, ffi.Namespace, ffi.ID)
		},
	)
}

func (s *SQLCommon) setFFIInsertValues(query sq.InsertBuilder, ffi *fftypes.FFI) sq.InsertBuilder {
	var networkName *string
	if ffi.NetworkName != "" {
		networkName = &ffi.NetworkName
	}
	return query.Values(
		ffi.ID,
		ffi.Namespace,
		ffi.Name,
		networkName,
		ffi.Version,
		ffi.Description,
		ffi.Message,
		ffi.Published,
	)
}

func (s *SQLCommon) attemptFFIInsert(ctx context.Context, tx *dbsql.TXWrapper, ffi *fftypes.FFI, requestConflictEmptyResult bool) error {
	_, err := s.InsertTxExt(ctx, ffiTable, tx,
		s.setFFIInsertValues(sq.Insert(ffiTable).Columns(ffiColumns...), ffi),
		func() {
			s.callbacks.UUIDCollectionNSEvent(database.CollectionFFIs, core.ChangeEventTypeCreated, ffi.Namespace, ffi.ID)
		}, requestConflictEmptyResult)
	return err
}

func (s *SQLCommon) ffiExists(ctx context.Context, tx *dbsql.TXWrapper, ffi *fftypes.FFI) (bool, error) {
	rows, _, err := s.QueryTx(ctx, ffiTable, tx,
		sq.Select("id").From(ffiTable).Where(sq.And{
			sq.Eq{
				"namespace": ffi.Namespace,
				"version":   ffi.Version,
			},
			sq.Or{
				sq.Eq{"name": ffi.Name},
				sq.Eq{"network_name": ffi.NetworkName},
			},
		}),
	)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	return rows.Next(), nil
}

func (s *SQLCommon) InsertOrGetFFI(ctx context.Context, ffi *fftypes.FFI) (existing *fftypes.FFI, err error) {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return nil, err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	insertErr := s.attemptFFIInsert(ctx, tx, ffi, true /* we want a failure here we can progress past */)
	if insertErr == nil {
		return nil, s.CommitTx(ctx, tx, autoCommit)
	}

	// Do a select within the transaction to determine if the FFI already exists
	existing, queryErr := s.getFFIPred(ctx, ffi.Namespace+":"+ffi.Name, sq.And{
		sq.Eq{"namespace": ffi.Namespace},
		sq.Or{
			sq.Eq{"id": ffi.ID},
			sq.Eq{"name": ffi.Name, "version": ffi.Version},
			sq.Eq{"network_name": ffi.NetworkName, "version": ffi.Version},
		},
	})
	if queryErr != nil || existing != nil {
		return existing, queryErr
	}

	// Error was apparently not an index conflict - must have been something else
	return nil, insertErr
}

func (s *SQLCommon) UpsertFFI(ctx context.Context, ffi *fftypes.FFI, optimization database.UpsertOptimization) error {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	optimized := false
	if optimization == database.UpsertOptimizationNew {
		opErr := s.attemptFFIInsert(ctx, tx, ffi, true /* we want a failure here we can progress past */)
		optimized = opErr == nil
	} else if optimization == database.UpsertOptimizationExisting {
		rowsAffected, opErr := s.attemptFFIUpdate(ctx, tx, ffi)
		optimized = opErr == nil && rowsAffected == 1
	}

	if !optimized {
		// Do a select within the transaction to determine if the FFI already exists
		exists, err := s.ffiExists(ctx, tx, ffi)
		if err != nil {
			return err
		} else if exists {
			if _, err := s.attemptFFIUpdate(ctx, tx, ffi); err != nil {
				return err
			}
		}
		if err := s.attemptFFIInsert(ctx, tx, ffi, false); err != nil {
			return err
		}
	}

	return s.CommitTx(ctx, tx, autoCommit)
}

func (s *SQLCommon) ffiResult(ctx context.Context, row *sql.Rows) (*fftypes.FFI, error) {
	ffi := fftypes.FFI{}
	var networkName *string
	err := row.Scan(
		&ffi.ID,
		&ffi.Namespace,
		&ffi.Name,
		&networkName,
		&ffi.Version,
		&ffi.Description,
		&ffi.Message,
		&ffi.Published,
	)
	if networkName != nil {
		ffi.NetworkName = *networkName
	}
	if err != nil {
		return nil, i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, ffiTable)
	}
	return &ffi, nil
}

func (s *SQLCommon) getFFIPred(ctx context.Context, desc string, pred interface{}) (*fftypes.FFI, error) {
	rows, _, err := s.Query(ctx, ffiTable,
		sq.Select(ffiColumns...).
			From(ffiTable).
			Where(pred),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		log.L(ctx).Debugf("FFI '%s' not found", desc)
		return nil, nil
	}

	ffi, err := s.ffiResult(ctx, rows)
	if err != nil {
		return nil, err
	}

	return ffi, nil
}

func (s *SQLCommon) GetFFIs(ctx context.Context, namespace string, filter ffapi.Filter) (ffis []*fftypes.FFI, res *ffapi.FilterResult, err error) {

	query, fop, fi, err := s.FilterSelect(ctx, "", sq.Select(ffiColumns...).From(ffiTable),
		filter, ffiFilterFieldMap, []interface{}{"sequence"}, sq.Eq{"namespace": namespace})
	if err != nil {
		return nil, nil, err
	}

	rows, tx, err := s.Query(ctx, ffiTable, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	ffis = []*fftypes.FFI{}
	for rows.Next() {
		cd, err := s.ffiResult(ctx, rows)
		if err != nil {
			return nil, nil, err
		}
		ffis = append(ffis, cd)
	}

	return ffis, s.QueryRes(ctx, ffiTable, tx, fop, nil, fi), err

}

func (s *SQLCommon) GetFFIByID(ctx context.Context, namespace string, id *fftypes.UUID) (*fftypes.FFI, error) {
	return s.getFFIPred(ctx, id.String(), sq.Eq{"id": id, "namespace": namespace})
}

func (s *SQLCommon) GetFFI(ctx context.Context, namespace, name, version string) (*fftypes.FFI, error) {
	return s.getFFIPred(ctx, namespace+":"+name+":"+version, sq.Eq{
		"namespace": namespace,
		"name":      name,
		"version":   version,
	})
}

func (s *SQLCommon) GetFFIByNetworkName(ctx context.Context, namespace, networkName, version string) (*fftypes.FFI, error) {
	return s.getFFIPred(ctx, namespace+":"+networkName+":"+version, sq.Eq{
		"namespace":    namespace,
		"network_name": networkName,
		"version":      version,
	})
}

func (s *SQLCommon) DeleteFFI(ctx context.Context, namespace string, id *fftypes.UUID) error {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	err = s.DeleteTx(ctx, ffiTable, tx, sq.Delete(ffiTable).Where(sq.Eq{
		"id": id, "namespace": namespace,
	}), func() {
		s.callbacks.UUIDCollectionNSEvent(database.CollectionFFIs, core.ChangeEventTypeDeleted, namespace, id)
	})
	if err != nil {
		return err
	}

	return s.CommitTx(ctx, tx, autoCommit)
}
