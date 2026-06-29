// Copyright © 2021 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.verifier/licenses/LICENSE-2.0
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

func TestVerifiersE2EWithDB(t *testing.T) {
	log.SetLevel("debug")

	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new verifier entry
	verifier := &core.Verifier{
		Identity:  fftypes.NewUUID(),
		Namespace: "ns1",
		VerifierRef: core.VerifierRef{
			Type:  core.VerifierTypeEthAddress,
			Value: "0x12345",
		},
	}
	verifier.Seal()

	s.callbacks.On("HashCollectionNSEvent", database.CollectionVerifiers, core.ChangeEventTypeCreated, "ns1", verifier.Hash).Return()
	s.callbacks.On("HashCollectionNSEvent", database.CollectionVerifiers, core.ChangeEventTypeUpdated, "ns1", verifier.Hash).Return()

	err := s.UpsertVerifier(ctx, verifier, database.UpsertOptimizationNew)
	assert.NoError(t, err)

	// Check we get the exact same verifier back
	verifierRead, err := s.GetVerifierByHash(ctx, "ns1", verifier.Hash)
	assert.NoError(t, err)
	assert.NotNil(t, verifierRead)
	verifierJson, _ := json.Marshal(&verifier)
	verifierReadJson, _ := json.Marshal(&verifierRead)
	assert.Equal(t, string(verifierJson), string(verifierReadJson))

	// Update the verifier (this is testing what's possible at the database layer,
	// and does not account for the verification that happens at the higher level)
	verifierUpdated := &core.Verifier{
		Identity:  fftypes.NewUUID(),
		Created:   verifier.Created,
		Namespace: "ns1",
		VerifierRef: core.VerifierRef{
			Type:  core.VerifierTypeEthAddress,
			Value: "0x12345",
		},
	}
	verifierUpdated.Seal()
	err = s.UpsertVerifier(context.Background(), verifierUpdated, database.UpsertOptimizationExisting)
	assert.NoError(t, err)

	// Check we get the exact same data back - note the removal of one of the verifier elements
	verifierRead, err = s.GetVerifierByValue(ctx, verifierUpdated.Type, verifierUpdated.Namespace, verifierUpdated.Value)
	assert.NoError(t, err)
	verifierJson, _ = json.Marshal(&verifierUpdated)
	verifierReadJson, _ = json.Marshal(&verifierRead)
	assert.Equal(t, string(verifierJson), string(verifierReadJson))

	// Query back the verifier
	fb := database.VerifierQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("value", string(verifierUpdated.Value)),
	)
	verifierRes, res, err := s.GetVerifiers(ctx, "ns1", filter.Count(true))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(verifierRes))
	assert.Equal(t, int64(1), *res.TotalCount)
	verifierReadJson, _ = json.Marshal(verifierRes[0])
	assert.Equal(t, string(verifierJson), string(verifierReadJson))

	s.callbacks.AssertExpectations(t)
}

func TestUpsertVerifierFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertVerifier(context.Background(), &core.Verifier{}, database.UpsertOptimizationSkip)
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertVerifierFailSelect(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpsertVerifier(context.Background(), &core.Verifier{Hash: fftypes.NewRandB32()}, database.UpsertOptimizationSkip)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertVerifierFailInsert(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{}))
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpsertVerifier(context.Background(), &core.Verifier{Hash: fftypes.NewRandB32()}, database.UpsertOptimizationSkip)
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertVerifierFailUpdate(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"verifier"}).
		AddRow("id1"))
	mock.ExpectExec("UPDATE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpsertVerifier(context.Background(), &core.Verifier{Hash: fftypes.NewRandB32()}, database.UpsertOptimizationSkip)
	assert.Regexp(t, "FF00178", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertVerifierFailCommit(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"verifier"}))
	mock.ExpectExec("INSERT .*").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertVerifier(context.Background(), &core.Verifier{Hash: fftypes.NewRandB32()}, database.UpsertOptimizationSkip)
	assert.Regexp(t, "FF00180", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetVerifierByHashSelectFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetVerifierByHash(context.Background(), "ns1", fftypes.NewRandB32())
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetVerifierByNameSelectFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetVerifierByValue(context.Background(), core.VerifierTypeEthAddress, "ff_system", "0x12345")
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetVerifierByVerifierSelectFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetVerifierByValue(context.Background(), core.VerifierTypeEthAddress, "ff_system", "0x12345")
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetVerifierByHashNotFound(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"verifier", "verifier", "verifier"}))
	msg, err := s.GetVerifierByHash(context.Background(), "ns1", fftypes.NewRandB32())
	assert.NoError(t, err)
	assert.Nil(t, msg)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetVerifierByHashScanFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"verifier"}).AddRow("only one"))
	_, err := s.GetVerifierByHash(context.Background(), "ns1", fftypes.NewRandB32())
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetVerifierQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	f := database.VerifierQueryFactory.NewFilter(context.Background()).Eq("value", "")
	_, _, err := s.GetVerifiers(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetVerifierBuildQueryFail(t *testing.T) {
	s, _ := newMockProvider().init()
	f := database.VerifierQueryFactory.NewFilter(context.Background()).Eq("value", map[bool]bool{true: false})
	_, _, err := s.GetVerifiers(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00143.*type", err)
}

func TestGetVerifierReadMessageFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"value"}).AddRow("only one"))
	f := database.VerifierQueryFactory.NewFilter(context.Background()).Eq("value", "")
	_, _, err := s.GetVerifiers(context.Background(), "ns1", f)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
