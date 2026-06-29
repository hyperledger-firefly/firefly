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
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestPinsE2EWithDB(t *testing.T) {
	log.SetLevel("trace")

	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new pin entry
	pin := &core.Pin{
		Namespace:  "ns",
		Masked:     true,
		Hash:       fftypes.NewRandB32(),
		Batch:      fftypes.NewUUID(),
		BatchHash:  fftypes.NewRandB32(),
		Index:      10,
		Created:    fftypes.Now(),
		Signer:     "0x12345",
		Dispatched: false,
	}

	s.callbacks.On("OrderedCollectionNSEvent", database.CollectionPins, core.ChangeEventTypeCreated, "ns", mock.Anything).Return()

	err := s.UpsertPin(ctx, pin)
	assert.NoError(t, err)

	// Query back the pin
	fb := database.PinQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("masked", pin.Masked),
		fb.Eq("hash", pin.Hash),
		fb.Eq("batch", pin.Batch),
		fb.Gt("created", 0),
	)
	pinRes, res, err := s.GetPins(ctx, "ns", filter.Count(true))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(pinRes))
	assert.Equal(t, int64(1), *res.TotalCount)

	// Set it dispatched
	err = s.UpdatePins(ctx, "ns", database.PinQueryFactory.NewFilter(ctx).Eq("sequence", pin.Sequence), database.PinQueryFactory.NewUpdate(ctx).Set("dispatched", true))
	assert.NoError(t, err)

	// Double insert, checking no error and we keep the dispatched flag
	existingSequence := pin.Sequence
	pin.Sequence = 99999
	err = s.UpsertPin(ctx, pin)
	assert.NoError(t, err)
	pinRes, _, err = s.GetPins(ctx, "ns", filter)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(pinRes)) // we didn't add twice
	assert.Equal(t, existingSequence, pin.Sequence)
	assert.True(t, pin.Dispatched)

	s.callbacks.AssertExpectations(t)
}

func TestUpsertPinFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertPin(context.Background(), &core.Pin{})
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertPinFailInsert(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"sequence", "masked", "dispatched"}))
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpsertPin(context.Background(), &core.Pin{Hash: fftypes.NewRandB32()})
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertPinFailSelect(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpsertPin(context.Background(), &core.Pin{Hash: fftypes.NewRandB32()})
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertPinFailExistingSequenceScan(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(mock.NewRows([]string{"only one"}).AddRow(true))
	mock.ExpectRollback()
	err := s.UpsertPin(context.Background(), &core.Pin{Hash: fftypes.NewRandB32()})
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertPinFailCommit(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"sequence", "masked", "dispatched"}))
	mock.ExpectExec("INSERT .*").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertPin(context.Background(), &core.Pin{Hash: fftypes.NewRandB32()})
	assert.Regexp(t, "FF00180", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertPinsBeginFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.InsertPins(context.Background(), []*core.Pin{})
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

func TestInsertPinsMultiRowOK(t *testing.T) {
	s := newMockProvider()
	s.multiRowInsert = true
	s.fakePSQLInsert = true
	s, mock := s.init()

	pin1 := &core.Pin{Namespace: "ns1", Hash: fftypes.NewRandB32()}
	pin2 := &core.Pin{Namespace: "ns1", Hash: fftypes.NewRandB32()}
	s.callbacks.On("OrderedCollectionNSEvent", database.CollectionPins, core.ChangeEventTypeCreated, "ns1", int64(1001))
	s.callbacks.On("OrderedCollectionNSEvent", database.CollectionPins, core.ChangeEventTypeCreated, "ns1", int64(1002))

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT.*").WillReturnRows(sqlmock.NewRows([]string{s.SequenceColumn()}).
		AddRow(int64(1001)).
		AddRow(int64(1002)),
	)
	mock.ExpectCommit()
	err := s.InsertPins(context.Background(), []*core.Pin{pin1, pin2})
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

func TestInsertPinsMultiRowFail(t *testing.T) {
	s := newMockProvider()
	s.multiRowInsert = true
	s.fakePSQLInsert = true
	s, mock := s.init()
	pin1 := &core.Pin{Hash: fftypes.NewRandB32()}
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT.*").WillReturnError(fmt.Errorf("pop"))
	err := s.InsertPins(context.Background(), []*core.Pin{pin1})
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

func TestInsertPinsSingleRowFail(t *testing.T) {
	s, mock := newMockProvider().init()
	pin1 := &core.Pin{Hash: fftypes.NewRandB32()}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT.*").WillReturnError(fmt.Errorf("pop"))
	err := s.InsertPins(context.Background(), []*core.Pin{pin1})
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

func TestGetPinQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	f := database.PinQueryFactory.NewFilter(context.Background()).Eq("hash", "")
	_, _, err := s.GetPins(context.Background(), "ns", f)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetPinBuildQueryFail(t *testing.T) {
	s, _ := newMockProvider().init()
	f := database.PinQueryFactory.NewFilter(context.Background()).Eq("hash", map[bool]bool{true: false})
	_, _, err := s.GetPins(context.Background(), "ns", f)
	assert.Regexp(t, "FF00143.*type", err)
}

func TestGetPinReadMessageFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"pin"}).AddRow("only one"))
	f := database.PinQueryFactory.NewFilter(context.Background()).Eq("hash", "")
	_, _, err := s.GetPins(context.Background(), "ns", f)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdatePinsBeginFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	ctx := context.Background()
	err := s.UpdatePins(ctx, "ns1", database.PinQueryFactory.NewFilter(ctx).Eq("sequence", 1), database.PinQueryFactory.NewUpdate(ctx).Set("dispatched", true))
	assert.Regexp(t, "FF00175", err)
}

func TestUpdatePinsUpdateFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	ctx := context.Background()
	err := s.UpdatePins(ctx, "ns1", database.PinQueryFactory.NewFilter(ctx).Eq("sequence", 1), database.PinQueryFactory.NewUpdate(ctx).Set("dispatched", true))
	assert.Regexp(t, "FF00178", err)
}

func TestUpdatePinsBadFilter(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectRollback()
	ctx := context.Background()
	err := s.UpdatePins(ctx, "ns1", database.PinQueryFactory.NewFilter(ctx).Eq("sequence", 1), database.PinQueryFactory.NewUpdate(ctx).Set("bad", true))
	assert.Regexp(t, "FF00142", err)
}

func TestUpdatePinsBadUpdate(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectRollback()
	ctx := context.Background()
	err := s.UpdatePins(ctx, "ns1", database.PinQueryFactory.NewFilter(ctx).Eq("bad", 1), database.PinQueryFactory.NewUpdate(ctx).Set("dispatched", true))
	assert.Regexp(t, "FF00142", err)
}
