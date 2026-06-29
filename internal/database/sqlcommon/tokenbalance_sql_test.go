// Copyright © 2021 Kaleido, Inc.
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
	"encoding/json"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/stretchr/testify/assert"
)

func TestTokenBalanceE2EWithDB(t *testing.T) {

	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new token account
	uri := "firefly://token/1"
	transfer := &core.TokenTransfer{
		Pool:       fftypes.NewUUID(),
		TokenIndex: "1",
		URI:        uri,
		Connector:  "erc1155",
		Namespace:  "ns1",
		To:         "0x0",
		Amount:     *fftypes.NewFFBigInt(10),
	}
	balance := &core.TokenBalance{
		Pool:       transfer.Pool,
		TokenIndex: "1",
		URI:        uri,
		Connector:  "erc1155",
		Namespace:  "ns1",
		Key:        "0x0",
		Balance:    *fftypes.NewFFBigInt(10),
	}
	balanceJson, _ := json.Marshal(&balance)

	err := s.UpdateTokenBalances(ctx, transfer)
	assert.NoError(t, err)

	// Query back the token balance (by pool ID and identity)
	balanceRead, err := s.GetTokenBalance(ctx, "ns1", transfer.Pool, "1", "0x0")
	assert.NoError(t, err)
	assert.NotNil(t, balanceRead)
	assert.Greater(t, balanceRead.Updated.UnixNano(), int64(0))
	balanceRead.Updated = nil
	balanceReadJson, _ := json.Marshal(&balanceRead)
	assert.Equal(t, string(balanceJson), string(balanceReadJson))

	// Query back the token balance (by query filter)
	fb := database.TokenBalanceQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("pool", balance.Pool),
		fb.Eq("tokenindex", balance.TokenIndex),
		fb.Eq("key", balance.Key),
	)
	balances, res, err := s.GetTokenBalances(ctx, "ns1", filter.Count(true))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(balances))
	assert.Equal(t, int64(1), *res.TotalCount)
	assert.Greater(t, balances[0].Updated.UnixNano(), int64(0))
	balances[0].Updated = nil
	balanceReadJson, _ = json.Marshal(balances[0])
	assert.Equal(t, string(balanceJson), string(balanceReadJson))

	// Transfer half to a different address
	transfer.From = "0x0"
	transfer.To = "0x1"
	transfer.Amount = *fftypes.NewFFBigInt(5)
	err = s.UpdateTokenBalances(ctx, transfer)
	assert.NoError(t, err)

	// Query back the token balance (by pool ID and identity)
	balanceRead, err = s.GetTokenBalance(ctx, "ns1", transfer.Pool, "1", "0x0")
	assert.NoError(t, err)
	assert.NotNil(t, balanceRead)
	assert.Greater(t, balanceRead.Updated.UnixNano(), int64(0))
	balanceRead.Updated = nil
	balanceReadJson, _ = json.Marshal(&balanceRead)
	balance.Balance = *fftypes.NewFFBigInt(5)
	balanceJson, _ = json.Marshal(&balance)
	assert.Equal(t, string(balanceJson), string(balanceReadJson))

	// Query back the other token balance (by pool ID and identity)
	balanceRead, err = s.GetTokenBalance(ctx, "ns1", transfer.Pool, "1", "0x1")
	assert.NoError(t, err)
	assert.NotNil(t, balanceRead)
	assert.Greater(t, balanceRead.Updated.UnixNano(), int64(0))
	balanceRead.Updated = nil
	balanceReadJson, _ = json.Marshal(&balanceRead)
	balance.Key = "0x1"
	balance.Balance = *fftypes.NewFFBigInt(5)
	balanceJson, _ = json.Marshal(&balance)
	assert.Equal(t, string(balanceJson), string(balanceReadJson))

	// Query the list of unique accounts
	accounts, _, err := s.GetTokenAccounts(ctx, "ns1", fb.And())
	assert.NoError(t, err)
	assert.Equal(t, 2, len(accounts))
	assert.Equal(t, "0x1", accounts[0].Key)
	assert.Equal(t, "0x0", accounts[1].Key)

	// Query the pools for each account
	pools, _, err := s.GetTokenAccountPools(ctx, "ns1", "0x0", fb.And())
	assert.NoError(t, err)
	assert.Equal(t, 1, len(pools))
	assert.Equal(t, *transfer.Pool, *pools[0].Pool)
	pools, _, err = s.GetTokenAccountPools(ctx, "ns1", "0x1", fb.And())
	assert.NoError(t, err)
	assert.Equal(t, 1, len(pools))
	assert.Equal(t, *transfer.Pool, *pools[0].Pool)

	// Delete the token balances
	err = s.DeleteTokenBalances(ctx, "ns1", transfer.Pool)
	assert.NoError(t, err)
}

func TestUpdateTokenBalancesFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.UpdateTokenBalances(context.Background(), &core.TokenTransfer{})
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateTokenBalancesFailSelect(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	err := s.UpdateTokenBalances(context.Background(), &core.TokenTransfer{To: "0x0"})
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateTokenBalancesFailInsert(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{}))
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpdateTokenBalances(context.Background(), &core.TokenTransfer{From: "0x0"})
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateTokenBalancesFailInsert2(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{}))
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpdateTokenBalances(context.Background(), &core.TokenTransfer{To: "0x0"})
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateTokenBalancesFailUpdate(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows(tokenBalanceColumns).AddRow(fftypes.NewUUID().String(), "1", "", "", "", "0x0", "0", 0))
	mock.ExpectExec("UPDATE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpdateTokenBalances(context.Background(), &core.TokenTransfer{To: "0x0"})
	assert.Regexp(t, "FF00178", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateTokenBalancesFailCommit(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{}))
	mock.ExpectExec("INSERT .*").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(fmt.Errorf("pop"))
	err := s.UpdateTokenBalances(context.Background(), &core.TokenTransfer{To: "0x0"})
	assert.Regexp(t, "FF00180", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTokenBalanceNotFound(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	msg, err := s.GetTokenBalance(context.Background(), "ns1", fftypes.NewUUID(), "1", "0x0")
	assert.NoError(t, err)
	assert.Nil(t, msg)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTokenBalanceScanFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("only one"))
	_, err := s.GetTokenBalance(context.Background(), "ns1", fftypes.NewUUID(), "1", "0x0")
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTokenBalancesQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	f := database.TokenBalanceQueryFactory.NewFilter(context.Background()).Eq("pool", "")
	_, _, err := s.GetTokenBalances(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTokenBalancesBuildQueryFail(t *testing.T) {
	s, _ := newMockProvider().init()
	f := database.TokenBalanceQueryFactory.NewFilter(context.Background()).Eq("pool", map[bool]bool{true: false})
	_, _, err := s.GetTokenBalances(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00143.*pool", err)
}

func TestGetTokenBalancesScanFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"pool"}).AddRow("only one"))
	f := database.TokenBalanceQueryFactory.NewFilter(context.Background()).Eq("pool", "")
	_, _, err := s.GetTokenBalances(context.Background(), "ns1", f)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTokenAccountsQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	f := database.TokenBalanceQueryFactory.NewFilter(context.Background()).And()
	_, _, err := s.GetTokenAccounts(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTokenAccountsBuildQueryFail(t *testing.T) {
	s, _ := newMockProvider().init()
	f := database.TokenBalanceQueryFactory.NewFilter(context.Background()).Eq("pool", map[bool]bool{true: false})
	_, _, err := s.GetTokenAccounts(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00143.*pool", err)
}

func TestGetTokenAccountsScanFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"key", "bad"}).AddRow("too many", "columns"))
	f := database.TokenBalanceQueryFactory.NewFilter(context.Background()).And()
	_, _, err := s.GetTokenAccounts(context.Background(), "ns1", f)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTokenAccountPoolsQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	f := database.TokenBalanceQueryFactory.NewFilter(context.Background()).And()
	_, _, err := s.GetTokenAccountPools(context.Background(), "ns1", "0x1", f)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetTokenAccountPoolsBuildQueryFail(t *testing.T) {
	s, _ := newMockProvider().init()
	f := database.TokenBalanceQueryFactory.NewFilter(context.Background()).Eq("pool", map[bool]bool{true: false})
	_, _, err := s.GetTokenAccountPools(context.Background(), "ns1", "0x1", f)
	assert.Regexp(t, "FF00143.*pool", err)
}

func TestGetTokenAccountPoolsScanFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"key", "bad"}).AddRow("too many", "columns"))
	f := database.TokenBalanceQueryFactory.NewFilter(context.Background()).And()
	_, _, err := s.GetTokenAccountPools(context.Background(), "ns1", "0x1", f)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteTokenBalancesFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.DeleteTokenBalances(context.Background(), "ns1", fftypes.NewUUID())
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDeleteTokenBalancesFailDelete(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("DELETE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.DeleteTokenBalances(context.Background(), "ns1", fftypes.NewUUID())
	assert.Regexp(t, "FF00179", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
