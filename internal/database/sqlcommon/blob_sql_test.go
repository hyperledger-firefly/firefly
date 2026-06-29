// Copyright © 2023 Kaleido, Inc.
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

func TestBlobsE2EWithDB(t *testing.T) {
	dataID := fftypes.NewUUID()
	namespace := "e2e"
	log.SetLevel("debug")

	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new blob entry
	blob := &core.Blob{
		Namespace:  namespace,
		Hash:       fftypes.NewRandB32(),
		Size:       12345,
		PayloadRef: fftypes.NewRandB32().String(),
		Peer:       "peer1",
		Created:    fftypes.Now(),
		DataID:     dataID,
	}
	err := s.InsertBlob(ctx, blob)
	assert.NoError(t, err)

	// Check we get the exact same blob back
	fb := database.BlobQueryFactory.NewFilter(ctx)
	blobs, _, err := s.GetBlobs(ctx, namespace, fb.Eq("payloadref", blob.PayloadRef))
	blobRead := blobs[0]
	assert.NoError(t, err)
	assert.NotNil(t, blobRead)
	blobJson, _ := json.Marshal(&blob)
	blobReadJson, _ := json.Marshal(&blobRead)
	assert.Equal(t, string(blobJson), string(blobReadJson))

	// Query back the blob
	fb = database.BlobQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("hash", blob.Hash),
		fb.Eq("payloadref", blob.PayloadRef),
		fb.Eq("created", blob.Created),
	)
	blobRes, res, err := s.GetBlobs(ctx, namespace, filter.Count(true))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(blobRes))
	assert.Equal(t, int64(1), *res.TotalCount)
	blobReadJson, _ = json.Marshal(blobRes[0])
	assert.Equal(t, string(blobJson), string(blobReadJson))
	assert.Equal(t, blob.Sequence, blobRes[0].Sequence)

	// Test delete
	err = s.DeleteBlob(ctx, blob.Sequence)
	assert.NoError(t, err)
	blobs, _, err = s.GetBlobs(ctx, namespace, filter)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(blobs))

}

func TestInsertBlobFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.InsertBlob(context.Background(), &core.Blob{})
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertBlobFailInsert(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.InsertBlob(context.Background(), &core.Blob{Hash: fftypes.NewRandB32()})
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertBlobFailCommit(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT .*").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(fmt.Errorf("pop"))
	err := s.InsertBlob(context.Background(), &core.Blob{Hash: fftypes.NewRandB32()})
	assert.Regexp(t, "FF00180", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertBlobsBeginFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.InsertBlobs(context.Background(), []*core.Blob{})
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

func TestInsertBlobsMultiRowOK(t *testing.T) {
	s := newMockProvider()
	s.multiRowInsert = true
	s.fakePSQLInsert = true
	s, mock := s.init()

	blob1 := &core.Blob{Hash: fftypes.NewRandB32(), PayloadRef: "pay1"}
	blob2 := &core.Blob{Hash: fftypes.NewRandB32(), PayloadRef: "pay2"}

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT.*").WillReturnRows(sqlmock.NewRows([]string{s.SequenceColumn()}).
		AddRow(int64(1001)).
		AddRow(int64(1002)),
	)
	mock.ExpectCommit()
	err := s.InsertBlobs(context.Background(), []*core.Blob{blob1, blob2})
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

func TestInsertBlobsMultiRowFail(t *testing.T) {
	s := newMockProvider()
	s.multiRowInsert = true
	s.fakePSQLInsert = true
	s, mock := s.init()
	blob1 := &core.Blob{Hash: fftypes.NewRandB32(), PayloadRef: "pay1"}
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT.*").WillReturnError(fmt.Errorf("pop"))
	err := s.InsertBlobs(context.Background(), []*core.Blob{blob1})
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

func TestInsertBlobsSingleRowFail(t *testing.T) {
	s, mock := newMockProvider().init()
	blob1 := &core.Blob{Hash: fftypes.NewRandB32(), PayloadRef: "pay1"}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT.*").WillReturnError(fmt.Errorf("pop"))
	err := s.InsertBlobs(context.Background(), []*core.Blob{blob1})
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

// func TestGetBlobByIDSelectFail(t *testing.T) {
// 	s, mock := newMockProvider().init()
// 	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
// 	_, err := s.GetBlob(context.Background(), "ns1", fftypes.NewUUID(), fftypes.NewRandB32())
// 	assert.Regexp(t, "FF00176", err)
// 	assert.NoError(t, mock.ExpectationsWereMet())
// }

// func TestGetBlobByIDNotFound(t *testing.T) {
// 	s, mock := newMockProvider().init()
// 	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{}))
// 	msg, err := s.GetBlob(context.Background(), "ns1", fftypes.NewUUID(), fftypes.NewRandB32())
// 	assert.NoError(t, err)
// 	assert.Nil(t, msg)
// 	assert.NoError(t, mock.ExpectationsWereMet())
// }

// func TestGetBlobByIDScanFail(t *testing.T) {
// 	s, mock := newMockProvider().init()
// 	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"hash"}).AddRow("only one"))
// 	_, err := s.GetBlob(context.Background(), "ns1", fftypes.NewUUID(), fftypes.NewRandB32())
// 	assert.Regexp(t, "FF10121", err)
// 	assert.NoError(t, mock.ExpectationsWereMet())
// }

func TestGetBlobQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	f := database.BlobQueryFactory.NewFilter(context.Background()).Eq("hash", "")
	_, _, err := s.GetBlobs(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetBlobBuildQueryFail(t *testing.T) {
	s, _ := newMockProvider().init()
	f := database.BlobQueryFactory.NewFilter(context.Background()).Eq("hash", map[bool]bool{true: false})
	_, _, err := s.GetBlobs(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00143.*type", err)
}

func TestGetBlobReadMessageFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"hash"}).AddRow("only one"))
	f := database.BlobQueryFactory.NewFilter(context.Background()).Eq("hash", "")
	_, _, err := s.GetBlobs(context.Background(), "ns1", f)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestBlobDeleteBeginFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.DeleteBlob(context.Background(), 12345)
	assert.Regexp(t, "FF00175", err)
}

func TestBlobDeleteFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("DELETE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.DeleteBlob(context.Background(), 12345)
	assert.Regexp(t, "FF00179", err)
}
