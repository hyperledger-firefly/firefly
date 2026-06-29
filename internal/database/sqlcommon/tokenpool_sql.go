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
	tokenPoolColumns = []string{
		"id",
		"namespace",
		"name",
		"network_name",
		"standard",
		"locator",
		"type",
		"connector",
		"symbol",
		"decimals",
		"message_id",
		"active",
		"created",
		"tx_type",
		"tx_id",
		"info",
		"interface",
		"interface_format",
		"methods",
		"published",
		"plugin_data",
	}
	tokenPoolFilterFieldMap = map[string]string{
		"message":         "message_id",
		"tx.type":         "tx_type",
		"tx.id":           "tx_id",
		"interfaceformat": "interface_format",
		"networkname":     "network_name",
	}
)

const tokenpoolTable = "tokenpool"

func (s *SQLCommon) attemptTokenPoolUpdate(ctx context.Context, tx *dbsql.TXWrapper, pool *core.TokenPool) (int64, error) {
	var interfaceID *fftypes.UUID
	if pool.Interface != nil {
		interfaceID = pool.Interface.ID
	}
	var networkName *string
	if pool.NetworkName != "" {
		networkName = &pool.NetworkName
	}
	return s.UpdateTx(ctx, tokenpoolTable, tx,
		sq.Update(tokenpoolTable).
			Set("name", pool.Name).
			Set("network_name", networkName).
			Set("standard", pool.Standard).
			Set("locator", pool.Locator).
			Set("type", pool.Type).
			Set("connector", pool.Connector).
			Set("symbol", pool.Symbol).
			Set("decimals", pool.Decimals).
			Set("message_id", pool.Message).
			Set("active", pool.Active).
			Set("tx_type", pool.TX.Type).
			Set("tx_id", pool.TX.ID).
			Set("info", pool.Info).
			Set("interface", interfaceID).
			Set("interface_format", pool.InterfaceFormat).
			Set("methods", pool.Methods).
			Set("published", pool.Published).
			Set("plugin_data", pool.PluginData).
			Where(sq.Eq{"id": pool.ID}),
		func() {
			s.callbacks.UUIDCollectionNSEvent(database.CollectionTokenPools, core.ChangeEventTypeUpdated, pool.Namespace, pool.ID)
		},
	)
}

func (s *SQLCommon) setTokenPoolInsertValues(query sq.InsertBuilder, pool *core.TokenPool, created *fftypes.FFTime) sq.InsertBuilder {
	var interfaceID *fftypes.UUID
	if pool.Interface != nil {
		interfaceID = pool.Interface.ID
	}
	var networkName *string
	if pool.NetworkName != "" {
		networkName = &pool.NetworkName
	}
	return query.Values(
		pool.ID,
		pool.Namespace,
		pool.Name,
		networkName,
		pool.Standard,
		pool.Locator,
		pool.Type,
		pool.Connector,
		pool.Symbol,
		pool.Decimals,
		pool.Message,
		pool.Active,
		created,
		pool.TX.Type,
		pool.TX.ID,
		pool.Info,
		interfaceID,
		pool.InterfaceFormat,
		pool.Methods,
		pool.Published,
		pool.PluginData,
	)
}

func (s *SQLCommon) attemptTokenPoolInsert(ctx context.Context, tx *dbsql.TXWrapper, pool *core.TokenPool, requestConflictEmptyResult bool) error {
	created := fftypes.Now()
	_, err := s.InsertTxExt(ctx, tokenpoolTable, tx,
		s.setTokenPoolInsertValues(sq.Insert(tokenpoolTable).Columns(tokenPoolColumns...), pool, created),
		func() {
			s.callbacks.UUIDCollectionNSEvent(database.CollectionTokenPools, core.ChangeEventTypeCreated, pool.Namespace, pool.ID)
		}, requestConflictEmptyResult)
	if err == nil {
		pool.Created = created
	}
	return err
}

func (s *SQLCommon) tokenPoolExists(ctx context.Context, tx *dbsql.TXWrapper, pool *core.TokenPool) (bool, error) {
	rows, _, err := s.QueryTx(ctx, tokenpoolTable, tx,
		sq.Select("id").From(tokenpoolTable).Where(sq.And{
			sq.Eq{"namespace": pool.Namespace},
			sq.Or{
				sq.Eq{"name": pool.Name},
				sq.Eq{"network_name": pool.NetworkName},
			},
		}),
	)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	return rows.Next(), nil
}

func (s *SQLCommon) InsertOrGetTokenPool(ctx context.Context, pool *core.TokenPool) (existing *core.TokenPool, err error) {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return nil, err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	insertErr := s.attemptTokenPoolInsert(ctx, tx, pool, true /* we want a failure here we can progress past */)
	if insertErr == nil {
		return nil, s.CommitTx(ctx, tx, autoCommit)
	}

	// Do a select within the transaction to determine if the pool already exists
	existing, queryErr := s.getTokenPoolPred(ctx, pool.Namespace+":"+pool.Name, sq.And{
		sq.Eq{"namespace": pool.Namespace},
		sq.Or{
			sq.Eq{"id": pool.ID},
			sq.Eq{"name": pool.Name},
			sq.Eq{"network_name": pool.NetworkName},
		},
	})
	if queryErr != nil || existing != nil {
		return existing, queryErr
	}

	// Error was apparently not an index conflict - must have been something else
	return nil, insertErr
}

func (s *SQLCommon) UpsertTokenPool(ctx context.Context, pool *core.TokenPool, optimization database.UpsertOptimization) (err error) {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	optimized := false
	if optimization == database.UpsertOptimizationNew {
		opErr := s.attemptTokenPoolInsert(ctx, tx, pool, true /* we want a failure here we can progress past */)
		optimized = opErr == nil
	} else if optimization == database.UpsertOptimizationExisting {
		rowsAffected, opErr := s.attemptTokenPoolUpdate(ctx, tx, pool)
		optimized = opErr == nil && rowsAffected == 1
	}

	if !optimized {
		// Do a select within the transaction to determine if the pool already exists
		exists, err := s.tokenPoolExists(ctx, tx, pool)
		if err != nil {
			return err
		} else if exists {
			if _, err := s.attemptTokenPoolUpdate(ctx, tx, pool); err != nil {
				return err
			}
		} else if err := s.attemptTokenPoolInsert(ctx, tx, pool, false); err != nil {
			return err
		}
	}

	return s.CommitTx(ctx, tx, autoCommit)
}

func (s *SQLCommon) tokenPoolResult(ctx context.Context, row *sql.Rows) (*core.TokenPool, error) {
	pool := core.TokenPool{}
	iface := fftypes.FFIReference{}
	var networkName *string
	err := row.Scan(
		&pool.ID,
		&pool.Namespace,
		&pool.Name,
		&networkName,
		&pool.Standard,
		&pool.Locator,
		&pool.Type,
		&pool.Connector,
		&pool.Symbol,
		&pool.Decimals,
		&pool.Message,
		&pool.Active,
		&pool.Created,
		&pool.TX.Type,
		&pool.TX.ID,
		&pool.Info,
		&iface.ID,
		&pool.InterfaceFormat,
		&pool.Methods,
		&pool.Published,
		&pool.PluginData,
	)
	if iface.ID != nil {
		pool.Interface = &iface
	}
	if networkName != nil {
		pool.NetworkName = *networkName
	}
	if err != nil {
		return nil, i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, tokenpoolTable)
	}
	return &pool, nil
}

func (s *SQLCommon) getTokenPoolPred(ctx context.Context, desc string, pred interface{}) (*core.TokenPool, error) {
	rows, _, err := s.Query(ctx, tokenpoolTable,
		sq.Select(tokenPoolColumns...).
			From(tokenpoolTable).
			Where(pred),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		log.L(ctx).Debugf("Token pool '%s' not found", desc)
		return nil, nil
	}

	pool, err := s.tokenPoolResult(ctx, rows)
	if err != nil {
		return nil, err
	}

	return pool, nil
}

func (s *SQLCommon) GetTokenPool(ctx context.Context, namespace string, name string) (message *core.TokenPool, err error) {
	return s.getTokenPoolPred(ctx, namespace+":"+name, sq.Eq{"namespace": namespace, "name": name})
}

func (s *SQLCommon) GetTokenPoolByID(ctx context.Context, namespace string, id *fftypes.UUID) (message *core.TokenPool, err error) {
	return s.getTokenPoolPred(ctx, id.String(), sq.Eq{"id": id, "namespace": namespace})
}

func (s *SQLCommon) GetTokenPoolByNetworkName(ctx context.Context, namespace, networkName string) (*core.TokenPool, error) {
	return s.getTokenPoolPred(ctx, networkName, sq.Eq{"namespace": namespace, "network_name": networkName})
}

func (s *SQLCommon) GetTokenPools(ctx context.Context, namespace string, filter ffapi.Filter) (message []*core.TokenPool, fr *ffapi.FilterResult, err error) {
	query, fop, fi, err := s.FilterSelect(ctx, "", sq.Select(tokenPoolColumns...).From("tokenpool"),
		filter, tokenPoolFilterFieldMap, []interface{}{"seq"}, sq.Eq{"namespace": namespace})
	if err != nil {
		return nil, nil, err
	}

	rows, tx, err := s.Query(ctx, tokenpoolTable, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	pools := []*core.TokenPool{}
	for rows.Next() {
		d, err := s.tokenPoolResult(ctx, rows)
		if err != nil {
			return nil, nil, err
		}
		pools = append(pools, d)
	}

	return pools, s.QueryRes(ctx, tokenpoolTable, tx, fop, nil, fi), err
}

func (s *SQLCommon) DeleteTokenPool(ctx context.Context, namespace string, id *fftypes.UUID) error {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	err = s.DeleteTx(ctx, "tokenpool", tx, sq.Delete("tokenpool").Where(sq.Eq{
		"id": id, "namespace": namespace,
	}), func() {
		s.callbacks.UUIDCollectionNSEvent(database.CollectionTokenPools, core.ChangeEventTypeDeleted, namespace, id)
	})
	if err != nil {
		return err
	}

	return s.CommitTx(ctx, tx, autoCommit)
}
