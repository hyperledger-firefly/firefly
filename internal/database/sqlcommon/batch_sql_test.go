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
	"github.com/stretchr/testify/mock"
)

func TestBatch2EWithDB(t *testing.T) {

	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new batch entry
	batchID := fftypes.NewUUID()
	msgID1 := fftypes.NewUUID()
	batch := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			ID:   batchID,
			Type: core.BatchTypeBroadcast,
			SignerRef: core.SignerRef{
				Key:    "0x12345",
				Author: "did:firefly:org/abcd",
			},
			Namespace: "ns1",
			Node:      fftypes.NewUUID(),
			Created:   fftypes.Now(),
		},
		Hash: fftypes.NewRandB32(),
		TX: core.TransactionRef{
			Type: core.TransactionTypeUnpinned,
		},
		Manifest: fftypes.JSONAnyPtr((&core.BatchManifest{
			Messages: []*core.MessageManifestEntry{
				{MessageRef: core.MessageRef{ID: msgID1}},
			},
		}).String()),
	}

	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionBatches, core.ChangeEventTypeCreated, "ns1", batchID, mock.Anything).Return()

	existing, err := s.InsertOrGetBatch(ctx, batch)
	assert.NoError(t, err)
	assert.Nil(t, existing)

	// Check we get the exact same batch back
	batchRead, err := s.GetBatchByID(ctx, "ns1", batchID)
	assert.NoError(t, err)
	assert.NotNil(t, batchRead)
	batchJson, _ := json.Marshal(&batch)
	batchReadJson, _ := json.Marshal(&batchRead)
	assert.Equal(t, string(batchJson), string(batchReadJson))

	// Try to insert again - should get back the existing row
	existing, err = s.InsertOrGetBatch(ctx, batch)
	assert.NoError(t, err)
	assert.NotNil(t, existing)

	// Query back the batch
	fb := database.BatchQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("id", batch.ID.String()),
		fb.Eq("author", batch.Author),
		fb.Gt("created", "0"),
	)
	batches, _, err := s.GetBatches(ctx, "ns1", filter)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(batches))
	batchReadJson, _ = json.Marshal(batches[0])
	assert.Equal(t, string(batchJson), string(batchReadJson))

	// Negative test on filter
	filter = fb.And(
		fb.Eq("id", batch.ID.String()),
		fb.Eq("created", "0"),
	)
	batches, _, err = s.GetBatches(ctx, "ns1", filter)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(batches))

	// Update
	author2 := "0x222222"
	up := database.BatchQueryFactory.NewUpdate(ctx).Set("author", author2)
	err = s.UpdateBatch(ctx, "ns1", batchID, up)
	assert.NoError(t, err)

	// Test find updated value
	filter = fb.And(
		fb.Eq("id", batch.ID.String()),
		fb.Eq("author", author2),
	)
	batches, res, err := s.GetBatches(ctx, "ns1", filter.Count(true))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(batches))
	assert.Equal(t, int64(1), *res.TotalCount)

	s.callbacks.AssertExpectations(t)
}

func TestUpsertBatchFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	_, err := s.InsertOrGetBatch(context.Background(), &core.BatchPersisted{})
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertBatchFailInsert(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{}))
	mock.ExpectRollback()
	batchID := fftypes.NewUUID()
	_, err := s.InsertOrGetBatch(context.Background(), &core.BatchPersisted{BatchHeader: core.BatchHeader{ID: batchID}})
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertBatchFailCommit(t *testing.T) {
	s, mock := newMockProvider().init()
	batchID := fftypes.NewUUID()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT .*").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(fmt.Errorf("pop"))
	_, err := s.InsertOrGetBatch(context.Background(), &core.BatchPersisted{BatchHeader: core.BatchHeader{ID: batchID}})
	assert.Regexp(t, "FF00180", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetBatchByIDSelectFail(t *testing.T) {
	s, mock := newMockProvider().init()
	batchID := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetBatchByID(context.Background(), "ns1", batchID)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetBatchByIDNotFound(t *testing.T) {
	s, mock := newMockProvider().init()
	batchID := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	msg, err := s.GetBatchByID(context.Background(), "ns1", batchID)
	assert.NoError(t, err)
	assert.Nil(t, msg)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetBatchByIDScanFail(t *testing.T) {
	s, mock := newMockProvider().init()
	batchID := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("only one"))
	_, err := s.GetBatchByID(context.Background(), "ns1", batchID)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetBatchesQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	f := database.BatchQueryFactory.NewFilter(context.Background()).Eq("id", "")
	_, _, err := s.GetBatches(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetBatchesBuildQueryFail(t *testing.T) {
	s, _ := newMockProvider().init()
	f := database.BatchQueryFactory.NewFilter(context.Background()).Eq("id", map[bool]bool{true: false})
	_, _, err := s.GetBatches(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00143.*id", err)
}

func TestGetBatchesReadMessageFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("only one"))
	f := database.BatchQueryFactory.NewFilter(context.Background()).Eq("id", "")
	_, _, err := s.GetBatches(context.Background(), "ns1", f)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBatchUpdateBeginFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	u := database.BatchQueryFactory.NewUpdate(context.Background()).Set("id", "anything")
	err := s.UpdateBatch(context.Background(), "ns1", fftypes.NewUUID(), u)
	assert.Regexp(t, "FF00175", err)
}

func TestBatchUpdateBuildQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	u := database.BatchQueryFactory.NewUpdate(context.Background()).Set("id", map[bool]bool{true: false})
	err := s.UpdateBatch(context.Background(), "ns1", fftypes.NewUUID(), u)
	assert.Regexp(t, "FF00143.*id", err)
}

func TestBatchUpdateFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	u := database.BatchQueryFactory.NewUpdate(context.Background()).Set("id", fftypes.NewUUID())
	err := s.UpdateBatch(context.Background(), "ns1", fftypes.NewUUID(), u)
	assert.Regexp(t, "FF00178", err)
}
