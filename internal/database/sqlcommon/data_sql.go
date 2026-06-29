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
	"strings"

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
	dataColumnsNoValue = []string{
		"id",
		"validator",
		"namespace",
		"datatype_name",
		"datatype_version",
		"hash",
		"created",
		"blob_hash",
		"blob_public",
		"blob_name",
		"blob_path",
		"blob_size",
		"public",
		"value_size",
	}
	dataColumnsWithValue = append(append([]string{}, dataColumnsNoValue...), "value")
	dataFilterFieldMap   = map[string]string{
		"validator":        "validator",
		"datatype.name":    "datatype_name",
		"datatype.version": "datatype_version",
		"blob.hash":        "blob_hash",
		"blob.public":      "blob_public",
		"blob.name":        "blob_name",
		"blob.path":        "blob_path",
		"blob.size":        "blob_size",
	}
)

const dataTable = "data"

func (s *SQLCommon) attemptDataUpdate(ctx context.Context, tx *dbsql.TXWrapper, data *core.Data) (int64, error) {
	datatype := data.Datatype
	if datatype == nil {
		datatype = &core.DatatypeRef{}
	}
	blob := data.Blob
	if blob == nil {
		blob = &core.BlobRef{}
	}
	data.CalcPath()
	return s.UpdateTx(ctx, dataTable, tx,
		sq.Update(dataTable).
			Set("validator", string(data.Validator)).
			Set("datatype_name", datatype.Name).
			Set("datatype_version", datatype.Version).
			Set("hash", data.Hash).
			Set("created", data.Created).
			Set("blob_hash", blob.Hash).
			Set("blob_public", blob.Public).
			Set("blob_name", blob.Name).
			Set("blob_path", blob.Path).
			Set("blob_size", blob.Size).
			Set("public", data.Public).
			Set("value_size", data.ValueSize).
			Set("value", data.Value).
			Where(sq.Eq{
				"id":        data.ID,
				"hash":      data.Hash,
				"namespace": data.Namespace,
			}),
		func() {
			s.callbacks.UUIDCollectionNSEvent(database.CollectionData, core.ChangeEventTypeUpdated, data.Namespace, data.ID)
		})
}

func (s *SQLCommon) setDataInsertValues(query sq.InsertBuilder, data *core.Data) sq.InsertBuilder {
	datatype := data.Datatype
	if datatype == nil {
		datatype = &core.DatatypeRef{}
	}
	blob := data.Blob
	if blob == nil {
		blob = &core.BlobRef{}
	}
	data.CalcPath()
	return query.Values(
		data.ID,
		string(data.Validator),
		data.Namespace,
		datatype.Name,
		datatype.Version,
		data.Hash,
		data.Created,
		blob.Hash,
		blob.Public,
		blob.Name,
		blob.Path,
		blob.Size,
		data.Public,
		data.ValueSize,
		data.Value,
	)
}

func (s *SQLCommon) attemptDataInsert(ctx context.Context, tx *dbsql.TXWrapper, data *core.Data, requestConflictEmptyResult bool) (int64, error) {
	return s.InsertTxExt(ctx, dataTable, tx,
		s.setDataInsertValues(sq.Insert(dataTable).Columns(dataColumnsWithValue...), data),
		func() {
			s.callbacks.UUIDCollectionNSEvent(database.CollectionData, core.ChangeEventTypeCreated, data.Namespace, data.ID)
		}, requestConflictEmptyResult)
}

func (s *SQLCommon) UpsertData(ctx context.Context, data *core.Data, optimization database.UpsertOptimization) (err error) {
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
	if optimization == database.UpsertOptimizationNew {
		_, opErr := s.attemptDataInsert(ctx, tx, data, true /* we want a failure here we can progress past */)
		optimized = opErr == nil
	} else if optimization == database.UpsertOptimizationExisting {
		rowsAffected, opErr := s.attemptDataUpdate(ctx, tx, data)
		optimized = opErr == nil && rowsAffected == 1
	}

	if !optimized {
		// Do a select within the transaction to determine if the UUID already exists
		dataRows, _, err := s.QueryTx(ctx, dataTable, tx,
			sq.Select("hash").
				From(dataTable).
				Where(sq.Eq{"id": data.ID, "namespace": data.Namespace}),
		)
		if err != nil {
			return err
		}

		existing := dataRows.Next()
		if existing {
			var hash *fftypes.Bytes32
			_ = dataRows.Scan(&hash)
			if !fftypes.SafeHashCompare(hash, data.Hash) {
				dataRows.Close()
				log.L(ctx).Errorf("Existing=%s New=%s", hash, data.Hash)
				return database.HashMismatch
			}
		}
		dataRows.Close()

		if existing {
			if _, err = s.attemptDataUpdate(ctx, tx, data); err != nil {
				return err
			}
		} else {
			if _, err = s.attemptDataInsert(ctx, tx, data, false); err != nil {
				return err
			}
		}
	}

	return s.CommitTx(ctx, tx, autoCommit)
}

func (s *SQLCommon) InsertDataArray(ctx context.Context, dataArray core.DataArray) (err error) {

	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	if s.Features().MultiRowInsert {
		query := sq.Insert(dataTable).Columns(dataColumnsWithValue...)
		for _, data := range dataArray {
			query = s.setDataInsertValues(query, data)
		}
		sequences := make([]int64, len(dataArray))
		err := s.InsertTxRows(ctx, dataTable, tx, query, func() {
			for _, data := range dataArray {
				s.callbacks.UUIDCollectionNSEvent(database.CollectionData, core.ChangeEventTypeCreated, data.Namespace, data.ID)
			}
		}, sequences, true /* we want the caller to be able to retry with individual upserts */)
		if err != nil {
			return err
		}
	} else {
		// Fall back to individual inserts grouped in a TX
		for _, data := range dataArray {
			_, err := s.attemptDataInsert(ctx, tx, data, true)
			if err != nil {
				return err
			}
		}
	}

	return s.CommitTx(ctx, tx, autoCommit)

}

func (s *SQLCommon) dataResult(ctx context.Context, row *sql.Rows, withValue bool) (*core.Data, error) {
	data := core.Data{
		Datatype: &core.DatatypeRef{},
		Blob:     &core.BlobRef{},
	}
	results := []interface{}{
		&data.ID,
		&data.Validator,
		&data.Namespace,
		&data.Datatype.Name,
		&data.Datatype.Version,
		&data.Hash,
		&data.Created,
		&data.Blob.Hash,
		&data.Blob.Public,
		&data.Blob.Name,
		&data.Blob.Path,
		&data.Blob.Size,
		&data.Public,
		&data.ValueSize,
	}
	if withValue {
		results = append(results, &data.Value)
	}
	err := row.Scan(results...)
	if data.Blob.Hash == nil && data.Blob.Public == "" {
		data.Blob = nil
	}
	if data.Datatype.Name == "" && data.Datatype.Version == "" {
		data.Datatype = nil
	}
	if err != nil {
		return nil, i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, dataTable)
	}
	return &data, nil
}

func (s *SQLCommon) GetDataByID(ctx context.Context, namespace string, id *fftypes.UUID, withValue bool) (message *core.Data, err error) {

	var cols []string
	if withValue {
		cols = dataColumnsWithValue
	} else {
		cols = dataColumnsNoValue
	}
	rows, _, err := s.Query(ctx, dataTable,
		sq.Select(cols...).
			From(dataTable).
			Where(sq.Eq{"id": id, "namespace": namespace}),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		log.L(ctx).Debugf("Data '%s' not found", id)
		return nil, nil
	}

	data, err := s.dataResult(ctx, rows, withValue)
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (s *SQLCommon) GetData(ctx context.Context, namespace string, filter ffapi.Filter) (message core.DataArray, res *ffapi.FilterResult, err error) {

	query, fop, fi, err := s.FilterSelect(
		ctx, "", sq.Select(dataColumnsWithValue...).From(dataTable),
		filter, dataFilterFieldMap, []interface{}{"sequence"}, sq.Eq{"namespace": namespace})
	if err != nil {
		return nil, nil, err
	}

	rows, tx, err := s.Query(ctx, dataTable, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	data := core.DataArray{}
	for rows.Next() {
		d, err := s.dataResult(ctx, rows, true)
		if err != nil {
			return nil, nil, err
		}
		data = append(data, d)
	}

	return data, s.QueryRes(ctx, dataTable, tx, fop, nil, fi), err

}

func (s *SQLCommon) GetDataRefs(ctx context.Context, namespace string, filter ffapi.Filter) (message core.DataRefs, res *ffapi.FilterResult, err error) {

	query, fop, fi, err := s.FilterSelect(
		ctx, "", sq.Select("id", "hash").From(dataTable),
		filter, dataFilterFieldMap, []interface{}{"sequence"}, sq.Eq{"namespace": namespace})
	if err != nil {
		return nil, nil, err
	}

	rows, tx, err := s.Query(ctx, dataTable, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	refs := core.DataRefs{}
	for rows.Next() {
		ref := core.DataRef{}
		err := rows.Scan(
			&ref.ID,
			&ref.Hash,
		)
		if err != nil {
			return nil, nil, i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, dataTable)
		}
		refs = append(refs, &ref)
	}

	return refs, s.QueryRes(ctx, dataTable, tx, fop, nil, fi), err

}

func (s *SQLCommon) GetDataSubPaths(ctx context.Context, namespace, path string) (subPaths []string, err error) {

	// Find all distinct path entries, which are sub-paths of the specified path
	searchPath := strings.Trim(path, "/")
	if searchPath == "" {
		searchPath = "/"
	} else {
		searchPath = "/" + searchPath + "/"
	}
	sel := sq.Select("blob_path").From(dataTable).Distinct().Where(
		sq.And{
			sq.Eq{"namespace": namespace},
			sq.Like{
				"blob_path": searchPath + "%",
			},
			sq.NotLike{
				"blob_path": searchPath + "%/%",
			},
		},
	).OrderBy("blob_path")
	rows, _, err := s.Query(ctx, dataTable, sel)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	paths := []string{}
	for rows.Next() {
		var path string
		err := rows.Scan(
			&path,
		)
		if err != nil {
			return nil, i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, dataTable)
		}
		paths = append(paths, path)
	}

	return paths, err

}

func (s *SQLCommon) UpdateData(ctx context.Context, namespace string, id *fftypes.UUID, update ffapi.Update) (err error) {

	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	query, err := s.BuildUpdate(sq.Update(dataTable), update, dataFilterFieldMap)
	if err != nil {
		return err
	}
	query = query.Where(sq.Eq{"id": id, "namespace": namespace})

	_, err = s.UpdateTx(ctx, dataTable, tx, query, nil /* no change events for filter based updates */)
	if err != nil {
		return err
	}

	return s.CommitTx(ctx, tx, autoCommit)
}

func (s *SQLCommon) DeleteData(ctx context.Context, namespace string, id *fftypes.UUID) (err error) {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	err = s.DeleteTx(ctx, blobsTable, tx, sq.Delete(dataTable).Where(sq.Eq{"id": id, "namespace": namespace}),
		nil /* no change events for blobs */)
	if err != nil {
		return err
	}

	return s.CommitTx(ctx, tx, autoCommit)
}
