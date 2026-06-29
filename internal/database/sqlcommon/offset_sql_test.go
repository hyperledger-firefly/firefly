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
	"crypto/rand"
	"encoding/json"
	"fmt"
	"math/big"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/stretchr/testify/assert"
)

func TestOffsetsE2EWithDB(t *testing.T) {
	log.SetLevel("debug")

	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new offset entry
	rand1, _ := rand.Int(rand.Reader, big.NewInt(10000000000000))
	offset := &core.Offset{
		Type:    core.OffsetTypeBatch,
		Name:    "offset1",
		Current: rand1.Int64(),
	}

	err := s.UpsertOffset(ctx, offset, true)
	assert.NoError(t, err)

	// Check we get the exact same offset back
	offsetRead, err := s.GetOffset(ctx, offset.Type, offset.Name)
	assert.NoError(t, err)
	assert.NotNil(t, offsetRead)
	offsetJson, _ := json.Marshal(&offset)
	offsetReadJson, _ := json.Marshal(&offsetRead)
	assert.Equal(t, string(offsetJson), string(offsetReadJson))

	// Update the offset (this is testing what's possible at the database layer,
	// and does not account for the verification that happens at the higher level)
	rand2, _ := rand.Int(rand.Reader, big.NewInt(10000000000000))
	offsetUpdated := &core.Offset{
		Type:    core.OffsetTypeBatch,
		Name:    "offset1",
		Current: rand2.Int64(),
	}

	// Remove ID for upsert and retry
	err = s.UpsertOffset(context.Background(), offsetUpdated, true)
	assert.NoError(t, err)

	// Check we get the exact same data back - note the removal of one of the offset elements
	offsetRead, err = s.GetOffset(ctx, offset.Type, offset.Name)
	assert.NoError(t, err)
	offsetJson, _ = json.Marshal(&offsetUpdated)
	offsetReadJson, _ = json.Marshal(&offsetRead)
	assert.Equal(t, string(offsetJson), string(offsetReadJson))

	// Query back the offset
	fb := database.OffsetQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("type", string(offsetUpdated.Type)),
		fb.Eq("name", offsetUpdated.Name),
		fb.Gt("current", 0),
	)
	offsetRes, res, err := s.GetOffsets(ctx, filter.Count(true))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(offsetRes))
	assert.Equal(t, int64(1), *res.TotalCount)
	offsetReadJson, _ = json.Marshal(offsetRes[0])
	assert.Equal(t, string(offsetJson), string(offsetReadJson))

	// Update
	rand3, _ := rand.Int(rand.Reader, big.NewInt(10000000000000))
	up := database.OffsetQueryFactory.NewUpdate(ctx).Set("current", rand3.Int64())
	err = s.UpdateOffset(ctx, offsetUpdated.RowID, up)
	assert.NoError(t, err)

	// Test find updated value
	filter = fb.And(
		fb.Eq("name", offsetUpdated.Name),
		fb.Eq("current", rand3.Int64()),
	)
	offsets, _, err := s.GetOffsets(ctx, filter)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(offsets))

	// Test delete
	err = s.DeleteOffset(ctx, core.OffsetTypeBatch, offsetUpdated.Name)
	assert.NoError(t, err)
	offsets, _, err = s.GetOffsets(ctx, filter)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(offsets))

	s.callbacks.AssertExpectations(t)
}

func TestUpsertOffsetFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertOffset(context.Background(), &core.Offset{}, true)
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertOffsetFailSelect(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpsertOffset(context.Background(), &core.Offset{Name: "name1"}, true)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertOffsetScanFailt(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{}).AddRow())
	mock.ExpectRollback()
	err := s.UpsertOffset(context.Background(), &core.Offset{Name: "name1"}, true)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertOffsetFailInsert(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{}))
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpsertOffset(context.Background(), &core.Offset{Name: "name1"}, true)
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertOffsetFailUpdate(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{s.SequenceColumn()}).
		AddRow(int64(12345)))
	mock.ExpectExec("UPDATE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpsertOffset(context.Background(), &core.Offset{Name: "name1"}, true)
	assert.Regexp(t, "FF00178", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertOffsetFailCommit(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{s.SequenceColumn()}))
	mock.ExpectExec("INSERT .*").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertOffset(context.Background(), &core.Offset{Name: "name1"}, true)
	assert.Regexp(t, "FF00180", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetOffsetByIDSelectFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetOffset(context.Background(), core.OffsetTypeBatch, "name1")
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetOffsetByIDNotFound(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"otype", "name"}))
	msg, err := s.GetOffset(context.Background(), core.OffsetTypeBatch, "name1")
	assert.NoError(t, err)
	assert.Nil(t, msg)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetOffsetByIDScanFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"otype"}).AddRow("only one"))
	_, err := s.GetOffset(context.Background(), core.OffsetTypeBatch, "name1")
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetOffsetQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	f := database.OffsetQueryFactory.NewFilter(context.Background()).Eq("type", "")
	_, _, err := s.GetOffsets(context.Background(), f)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetOffsetBuildQueryFail(t *testing.T) {
	s, _ := newMockProvider().init()
	f := database.OffsetQueryFactory.NewFilter(context.Background()).Eq("type", map[bool]bool{true: false})
	_, _, err := s.GetOffsets(context.Background(), f)
	assert.Regexp(t, "FF00143.*type", err)
}

func TestGetOffsetReadMessageFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"otype"}).AddRow("only one"))
	f := database.OffsetQueryFactory.NewFilter(context.Background()).Eq("type", "")
	_, _, err := s.GetOffsets(context.Background(), f)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestOffsetUpdateBeginFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	u := database.OffsetQueryFactory.NewUpdate(context.Background()).Set("name", "anything")
	err := s.UpdateOffset(context.Background(), 12345, u)
	assert.Regexp(t, "FF00175", err)
}

func TestOffsetUpdateBuildQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	u := database.OffsetQueryFactory.NewUpdate(context.Background()).Set("name", map[bool]bool{true: false})
	err := s.UpdateOffset(context.Background(), 12345, u)
	assert.Regexp(t, "FF00143.*name", err)
}

func TestOffsetUpdateFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	u := database.OffsetQueryFactory.NewUpdate(context.Background()).Set("name", fftypes.NewUUID())
	err := s.UpdateOffset(context.Background(), 12345, u)
	assert.Regexp(t, "FF00178", err)
}

func TestOffsetDeleteBeginFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.DeleteOffset(context.Background(), core.OffsetTypeSubscription, "sub1")
	assert.Regexp(t, "FF00175", err)
}

func TestOffsetDeleteSelectFailFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.DeleteOffset(context.Background(), core.OffsetTypeSubscription, "sub1")
	assert.Regexp(t, "FF00176", err)
}

func TestOffsetDeleteFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	cols := append(append([]string{}, offsetColumns...), s.SequenceColumn())
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows(cols).AddRow(
		core.OffsetTypeSubscription, "sub1", int64(12345), int64(12345),
	))
	mock.ExpectExec("DELETE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.DeleteOffset(context.Background(), core.OffsetTypeSubscription, "sub1")
	assert.Regexp(t, "FF00179", err)
}
