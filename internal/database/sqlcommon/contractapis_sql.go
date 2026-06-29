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
	contractAPIsColumns = []string{
		"id",
		"interface_id",
		"location",
		"name",
		"network_name",
		"namespace",
		"message_id",
		"published",
	}
	contractAPIsFilterFieldMap = map[string]string{
		"interface":   "interface_id",
		"message":     "message_id",
		"networkname": "network_name",
	}
)

const contractapisTable = "contractapis"

func (s *SQLCommon) attemptContractAPIUpdate(ctx context.Context, tx *dbsql.TXWrapper, api *core.ContractAPI) (int64, error) {
	var networkName *string
	if api.NetworkName != "" {
		networkName = &api.NetworkName
	}
	var ifaceID *fftypes.UUID
	if api.Interface != nil {
		ifaceID = api.Interface.ID
	}
	return s.UpdateTx(ctx, contractapisTable, tx,
		sq.Update(contractapisTable).
			Set("interface_id", ifaceID).
			Set("location", api.Location).
			Set("name", api.Name).
			Set("network_name", networkName).
			Set("message_id", api.Message).
			Set("published", api.Published).
			Where(sq.Eq{"id": api.ID}),
		func() {
			s.callbacks.UUIDCollectionNSEvent(database.CollectionContractAPIs, core.ChangeEventTypeUpdated, api.Namespace, api.ID)
		},
	)
}

func (s *SQLCommon) setContractAPIInsertValues(query sq.InsertBuilder, api *core.ContractAPI) sq.InsertBuilder {
	var networkName *string
	if api.NetworkName != "" {
		networkName = &api.NetworkName
	}
	var ifaceID *fftypes.UUID
	if api.Interface != nil {
		ifaceID = api.Interface.ID
	}
	return query.Values(
		api.ID,
		ifaceID,
		api.Location,
		api.Name,
		networkName,
		api.Namespace,
		api.Message,
		api.Published,
	)
}

func (s *SQLCommon) attemptContractAPIInsert(ctx context.Context, tx *dbsql.TXWrapper, api *core.ContractAPI, requestConflictEmptyResult bool) error {
	_, err := s.InsertTxExt(ctx, contractapisTable, tx,
		s.setContractAPIInsertValues(sq.Insert(contractapisTable).Columns(contractAPIsColumns...), api),
		func() {
			s.callbacks.UUIDCollectionNSEvent(database.CollectionContractAPIs, core.ChangeEventTypeCreated, api.Namespace, api.ID)
		}, requestConflictEmptyResult)
	return err
}

func (s *SQLCommon) contractAPIExists(ctx context.Context, tx *dbsql.TXWrapper, api *core.ContractAPI) (bool, error) {
	rows, _, err := s.QueryTx(ctx, contractapisTable, tx,
		sq.Select("id").From(contractapisTable).Where(sq.And{
			sq.Eq{
				"namespace": api.Namespace,
			},
			sq.Or{
				sq.Eq{"name": api.Name},
				sq.Eq{"network_name": api.NetworkName},
			},
		}),
	)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	return rows.Next(), nil
}

func (s *SQLCommon) InsertOrGetContractAPI(ctx context.Context, api *core.ContractAPI) (*core.ContractAPI, error) {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return nil, err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	insertErr := s.attemptContractAPIInsert(ctx, tx, api, true /* we want a failure here we can progress past */)
	if insertErr == nil {
		return nil, s.CommitTx(ctx, tx, autoCommit)
	}
	log.L(ctx).Debugf("Contract API insert failed due to err: %+v, retrieving the existing contract API", insertErr)

	// Do a select within the transaction to determine if the API already exists
	existing, queryErr := s.getContractAPIPred(ctx, api.Namespace+":"+api.Name, sq.And{
		sq.Eq{"namespace": api.Namespace},
		sq.Or{
			sq.Eq{"id": api.ID},
			sq.Eq{"name": api.Name},
			sq.Eq{"network_name": api.NetworkName},
		},
	})
	if queryErr != nil || existing != nil {
		return existing, queryErr
	}

	// Error was apparently not an index conflict - must have been something else
	return nil, insertErr
}

func (s *SQLCommon) UpsertContractAPI(ctx context.Context, api *core.ContractAPI, optimization database.UpsertOptimization) (err error) {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	optimized := false
	if optimization == database.UpsertOptimizationNew {
		opErr := s.attemptContractAPIInsert(ctx, tx, api, true /* we want a failure here we can progress past */)
		optimized = opErr == nil
	} else if optimization == database.UpsertOptimizationExisting {
		rowsAffected, opErr := s.attemptContractAPIUpdate(ctx, tx, api)
		optimized = opErr == nil && rowsAffected == 1
	}

	if !optimized {
		// Do a select within the transaction to determine if the API already exists
		exists, err := s.contractAPIExists(ctx, tx, api)
		if err != nil {
			return err
		} else if exists {
			if _, err := s.attemptContractAPIUpdate(ctx, tx, api); err != nil {
				return err
			}
		}
		if err := s.attemptContractAPIInsert(ctx, tx, api, false); err != nil {
			return err
		}
	}

	return s.CommitTx(ctx, tx, autoCommit)
}

func (s *SQLCommon) contractAPIResult(ctx context.Context, row *sql.Rows) (*core.ContractAPI, error) {
	api := core.ContractAPI{
		Interface: &fftypes.FFIReference{},
	}
	var networkName *string
	err := row.Scan(
		&api.ID,
		&api.Interface.ID,
		&api.Location,
		&api.Name,
		&networkName,
		&api.Namespace,
		&api.Message,
		&api.Published,
	)
	if networkName != nil {
		api.NetworkName = *networkName
	}
	if err != nil {
		return nil, i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, "contract")
	}
	return &api, nil
}

func (s *SQLCommon) getContractAPIPred(ctx context.Context, desc string, pred interface{}) (*core.ContractAPI, error) {
	rows, _, err := s.Query(ctx, contractapisTable,
		sq.Select(contractAPIsColumns...).
			From(contractapisTable).
			Where(pred),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		log.L(ctx).Debugf("Contract API '%s' not found", desc)
		return nil, nil
	}

	api, err := s.contractAPIResult(ctx, rows)
	if err != nil {
		return nil, err
	}

	return api, nil
}

func (s *SQLCommon) GetContractAPIs(ctx context.Context, namespace string, filter ffapi.AndFilter) (contractAPIs []*core.ContractAPI, res *ffapi.FilterResult, err error) {

	query, fop, fi, err := s.FilterSelect(ctx, "", sq.Select(contractAPIsColumns...).From(contractapisTable),
		filter, contractAPIsFilterFieldMap, []interface{}{"sequence"}, sq.Eq{"namespace": namespace})
	if err != nil {
		return nil, nil, err
	}

	rows, tx, err := s.Query(ctx, contractapisTable, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	apis := []*core.ContractAPI{}
	for rows.Next() {
		api, err := s.contractAPIResult(ctx, rows)
		if err != nil {
			return nil, nil, err
		}
		apis = append(apis, api)
	}

	return apis, s.QueryRes(ctx, contractapisTable, tx, fop, nil, fi), err

}

func (s *SQLCommon) GetContractAPIByID(ctx context.Context, namespace string, id *fftypes.UUID) (*core.ContractAPI, error) {
	return s.getContractAPIPred(ctx, id.String(), sq.Eq{"id": id, "namespace": namespace})
}

func (s *SQLCommon) GetContractAPIByName(ctx context.Context, namespace, name string) (*core.ContractAPI, error) {
	return s.getContractAPIPred(ctx, namespace+":"+name, sq.Eq{"namespace": namespace, "name": name})
}

func (s *SQLCommon) GetContractAPIByNetworkName(ctx context.Context, namespace, networkName string) (*core.ContractAPI, error) {
	return s.getContractAPIPred(ctx, namespace+":"+networkName, sq.Eq{"namespace": namespace, "network_name": networkName})
}

func (s *SQLCommon) DeleteContractAPI(ctx context.Context, namespace string, id *fftypes.UUID) error {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	err = s.DeleteTx(ctx, contractapisTable, tx, sq.Delete(contractapisTable).Where(sq.Eq{
		"id": id, "namespace": namespace,
	}), func() {
		s.callbacks.UUIDCollectionNSEvent(database.CollectionContractAPIs, core.ChangeEventTypeDeleted, namespace, id)
	})
	if err != nil {
		return err
	}

	return s.CommitTx(ctx, tx, autoCommit)
}
