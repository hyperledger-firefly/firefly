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
)

const tokenbalanceTable = "tokenbalance"

var (
	tokenBalanceColumns = []string{
		"pool_id",
		"token_index",
		"uri",
		"connector",
		"namespace",
		"key",
		"balance",
		"updated",
	}
	tokenBalanceFilterFieldMap = map[string]string{
		"pool":       "pool_id",
		"tokenindex": "token_index",
	}
)

func (s *SQLCommon) addTokenBalance(ctx context.Context, tx *dbsql.TXWrapper, transfer *core.TokenTransfer, key string, negate bool) error {
	balance, err := s.GetTokenBalance(ctx, transfer.Namespace, transfer.Pool, transfer.TokenIndex, key)
	if err != nil {
		return err
	}

	var total *fftypes.FFBigInt
	if balance != nil {
		total = &balance.Balance
	} else {
		total = &fftypes.FFBigInt{}
	}
	if negate {
		total.Int().Sub(total.Int(), transfer.Amount.Int())
	} else {
		total.Int().Add(total.Int(), transfer.Amount.Int())
	}

	if balance != nil {
		if _, err = s.UpdateTx(ctx, tokenbalanceTable, tx,
			sq.Update(tokenbalanceTable).
				Set("uri", transfer.URI).
				Set("balance", total).
				Set("updated", fftypes.Now()).
				Where(sq.Eq{
					"namespace":   balance.Namespace,
					"pool_id":     balance.Pool,
					"token_index": balance.TokenIndex,
					"key":         balance.Key,
				}),
			nil,
		); err != nil {
			return err
		}
	} else {
		if _, err = s.InsertTx(ctx, tokenbalanceTable, tx,
			sq.Insert(tokenbalanceTable).
				Columns(tokenBalanceColumns...).
				Values(
					transfer.Pool,
					transfer.TokenIndex,
					transfer.URI,
					transfer.Connector,
					transfer.Namespace,
					key,
					total,
					fftypes.Now(),
				),
			nil,
		); err != nil {
			return err
		}
	}

	return nil
}

func (s *SQLCommon) UpdateTokenBalances(ctx context.Context, transfer *core.TokenTransfer) (err error) {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	if transfer.From != "" {
		if err := s.addTokenBalance(ctx, tx, transfer, transfer.From, true); err != nil {
			return err
		}
	}
	if transfer.To != "" {
		if err := s.addTokenBalance(ctx, tx, transfer, transfer.To, false); err != nil {
			return err
		}
	}

	return s.CommitTx(ctx, tx, autoCommit)
}

func (s *SQLCommon) tokenBalanceResult(ctx context.Context, row *sql.Rows) (*core.TokenBalance, error) {
	account := core.TokenBalance{}
	err := row.Scan(
		&account.Pool,
		&account.TokenIndex,
		&account.URI,
		&account.Connector,
		&account.Namespace,
		&account.Key,
		&account.Balance,
		&account.Updated,
	)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, tokenbalanceTable)
	}
	return &account, nil
}

func (s *SQLCommon) getTokenBalancePred(ctx context.Context, desc string, pred interface{}) (*core.TokenBalance, error) {
	rows, _, err := s.Query(ctx, tokenbalanceTable,
		sq.Select(tokenBalanceColumns...).
			From(tokenbalanceTable).
			Where(pred),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		log.L(ctx).Debugf("Token account '%s' not found", desc)
		return nil, nil
	}

	account, err := s.tokenBalanceResult(ctx, rows)
	if err != nil {
		return nil, err
	}

	return account, nil
}

func (s *SQLCommon) GetTokenBalance(ctx context.Context, namespace string, poolID *fftypes.UUID, tokenIndex, key string) (message *core.TokenBalance, err error) {
	desc := core.TokenBalanceIdentifier(poolID, tokenIndex, key)
	return s.getTokenBalancePred(ctx, desc, sq.And{
		sq.Eq{"namespace": namespace},
		sq.Eq{"pool_id": poolID},
		sq.Eq{"token_index": tokenIndex},
		sq.Eq{"key": key},
	})
}

func (s *SQLCommon) GetTokenBalances(ctx context.Context, namespace string, filter ffapi.Filter) ([]*core.TokenBalance, *ffapi.FilterResult, error) {
	query, fop, fi, err := s.FilterSelect(ctx, "", sq.Select(tokenBalanceColumns...).From(tokenbalanceTable),
		filter, tokenBalanceFilterFieldMap, []interface{}{"seq"}, sq.Eq{"namespace": namespace})
	if err != nil {
		return nil, nil, err
	}

	rows, tx, err := s.Query(ctx, tokenbalanceTable, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	accounts := []*core.TokenBalance{}
	for rows.Next() {
		d, err := s.tokenBalanceResult(ctx, rows)
		if err != nil {
			return nil, nil, err
		}
		accounts = append(accounts, d)
	}

	return accounts, s.QueryRes(ctx, tokenbalanceTable, tx, fop, nil, fi), err
}

func (s *SQLCommon) GetTokenAccounts(ctx context.Context, namespace string, filter ffapi.Filter) ([]*core.TokenAccount, *ffapi.FilterResult, error) {
	query, fop, fi, err := s.FilterSelect(ctx, "",
		sq.Select("key", "MAX(updated) AS updated", "MAX(seq) AS seq").From(tokenbalanceTable).GroupBy("key"),
		filter, tokenBalanceFilterFieldMap, []interface{}{"seq"}, sq.Eq{"namespace": namespace})
	if err != nil {
		return nil, nil, err
	}

	rows, tx, err := s.Query(ctx, tokenbalanceTable, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	accounts := make([]*core.TokenAccount, 0)
	for rows.Next() {
		var account core.TokenAccount
		var updated fftypes.FFTime
		var seq int64
		if err := rows.Scan(&account.Key, &updated, &seq); err != nil {
			return nil, nil, i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, tokenbalanceTable)
		}
		accounts = append(accounts, &account)
	}

	return accounts, s.QueryRes(ctx, tokenbalanceTable, tx, fop, nil, fi), err
}

func (s *SQLCommon) GetTokenAccountPools(ctx context.Context, namespace, key string, filter ffapi.Filter) ([]*core.TokenAccountPool, *ffapi.FilterResult, error) {
	query, fop, fi, err := s.FilterSelect(ctx, "",
		sq.Select("pool_id", "MAX(updated) AS updated", "MAX(seq) AS seq").From(tokenbalanceTable).GroupBy("pool_id"),
		filter, tokenBalanceFilterFieldMap, []interface{}{"seq"}, sq.Eq{"key": key, "namespace": namespace})
	if err != nil {
		return nil, nil, err
	}

	rows, tx, err := s.Query(ctx, tokenbalanceTable, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	pools := make([]*core.TokenAccountPool, 0)
	for rows.Next() {
		var pool core.TokenAccountPool
		var updated fftypes.FFTime
		var seq int64
		if err := rows.Scan(&pool.Pool, &updated, &seq); err != nil {
			return nil, nil, i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, tokenbalanceTable)
		}
		pools = append(pools, &pool)
	}

	return pools, s.QueryRes(ctx, tokenbalanceTable, tx, fop, nil, fi), err
}

func (s *SQLCommon) DeleteTokenBalances(ctx context.Context, namespace string, poolID *fftypes.UUID) error {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	err = s.DeleteTx(ctx, tokenbalanceTable, tx, sq.Delete(tokenbalanceTable).Where(sq.Eq{
		"namespace": namespace,
		"pool_id":   poolID,
	}), nil)
	if err != nil && err != fftypes.DeleteRecordNotFound {
		return err
	}

	return s.CommitTx(ctx, tx, autoCommit)
}
