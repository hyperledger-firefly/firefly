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
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/stretchr/testify/assert"
)

func TestNoncesE2EWithDB(t *testing.T) {
	log.SetLevel("trace")

	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new nonce entry
	nonceZero := &core.Nonce{
		Hash: fftypes.NewRandB32(),
	}
	err := s.InsertNonce(ctx, nonceZero)
	assert.NoError(t, err)

	// Check we get the exact same nonce back
	nonceRead, err := s.GetNonce(ctx, nonceZero.Hash)
	assert.NoError(t, err)
	assert.NotNil(t, nonceRead)
	nonceJson, _ := json.Marshal(&nonceZero)
	nonceReadJson, _ := json.Marshal(&nonceRead)
	assert.Equal(t, string(nonceJson), string(nonceReadJson))

	// Update the nonce
	nonceUpdated := core.Nonce{
		Hash:  nonceZero.Hash,
		Nonce: 12345,
	}

	// Update nonce
	err = s.UpdateNonce(context.Background(), &nonceUpdated)
	assert.NoError(t, err)
	assert.Equal(t, int64(12345), nonceUpdated.Nonce)

	// Check we get the exact same data back
	nonceRead, err = s.GetNonce(ctx, nonceUpdated.Hash)
	assert.NoError(t, err)
	nonceJson, _ = json.Marshal(&nonceUpdated)
	nonceReadJson, _ = json.Marshal(&nonceRead)
	assert.Equal(t, string(nonceJson), string(nonceReadJson))

	// Query back the nonce
	fb := database.NonceQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("hash", nonceUpdated.Hash),
		fb.Eq("nonce", nonceUpdated.Nonce),
	)
	nonceRes, res, err := s.GetNonces(ctx, filter.Count(true))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(nonceRes))
	assert.Equal(t, int64(1), *res.TotalCount)
	nonceReadJson, _ = json.Marshal(nonceRes[0])
	assert.Equal(t, string(nonceJson), string(nonceReadJson))

	// Test delete
	err = s.DeleteNonce(ctx, nonceUpdated.Hash)
	assert.NoError(t, err)
	nonces, _, err := s.GetNonces(ctx, filter)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(nonces))

}

func TestInsertNonceFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.InsertNonce(context.Background(), &core.Nonce{})
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertNonceFailInsert(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.InsertNonce(context.Background(), &core.Nonce{Hash: fftypes.NewRandB32()})
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateNonceFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.UpdateNonce(context.Background(), &core.Nonce{})
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateNonceFailUpdate(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpdateNonce(context.Background(), &core.Nonce{Hash: fftypes.NewRandB32()})
	assert.Regexp(t, "FF00178", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetNonceSelectFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetNonce(context.Background(), fftypes.NewRandB32())
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetNonceNotFound(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"hash", "nonce", "group_hash", "topic"}))
	msg, err := s.GetNonce(context.Background(), fftypes.NewRandB32())
	assert.NoError(t, err)
	assert.Nil(t, msg)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetNonceScanFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"hash"}).AddRow("only one"))
	_, err := s.GetNonce(context.Background(), fftypes.NewRandB32())
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetNonceQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	f := database.NonceQueryFactory.NewFilter(context.Background()).Eq("hash", "")
	_, _, err := s.GetNonces(context.Background(), f)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetNonceBuildQueryFail(t *testing.T) {
	s, _ := newMockProvider().init()
	f := database.NonceQueryFactory.NewFilter(context.Background()).Eq("hash", map[bool]bool{true: false})
	_, _, err := s.GetNonces(context.Background(), f)
	assert.Regexp(t, "FF00143.*hash", err)
}

func TestGetNonceReadMessageFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"hash"}).AddRow("only one"))
	f := database.NonceQueryFactory.NewFilter(context.Background()).Eq("hash", "")
	_, _, err := s.GetNonces(context.Background(), f)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNonceDeleteBeginFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.DeleteNonce(context.Background(), fftypes.NewRandB32())
	assert.Regexp(t, "FF00175", err)
}

func TestNonceDeleteFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("DELETE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.DeleteNonce(context.Background(), fftypes.NewRandB32())
	assert.Regexp(t, "FF00179", err)
}
