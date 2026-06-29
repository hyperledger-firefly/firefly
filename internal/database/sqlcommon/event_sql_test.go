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
	"database/sql/driver"
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

func TestEventE2EWithDB(t *testing.T) {

	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new event entry
	eventID := fftypes.NewUUID()
	event := &core.Event{
		ID:         eventID,
		Namespace:  "ns1",
		Type:       core.EventTypeMessageConfirmed,
		Reference:  fftypes.NewUUID(),
		Correlator: fftypes.NewUUID(),
		Topic:      "topic1",
		Created:    fftypes.Now(),
	}

	s.callbacks.On("OrderedUUIDCollectionNSEvent", database.CollectionEvents, core.ChangeEventTypeCreated, "ns1", eventID, mock.Anything).Return()

	err := s.InsertEvent(ctx, event)
	assert.NoError(t, err)

	// Check we get the exact same event back
	eventRead, err := s.GetEventByID(ctx, "ns1", eventID)
	// The generated sequence will have been added
	event.Sequence = eventRead.Sequence
	assert.NoError(t, err)
	assert.NotNil(t, eventRead)
	eventJson, _ := json.Marshal(&event)
	eventReadJson, _ := json.Marshal(&eventRead)
	assert.Equal(t, string(eventJson), string(eventReadJson))

	// Query back the event
	fb := database.EventQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("id", eventRead.ID.String()),
		fb.Eq("reference", eventRead.Reference.String()),
	)
	events, res, err := s.GetEvents(ctx, "ns1", filter.Count(true))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(events))
	assert.Equal(t, int64(1), *res.TotalCount)
	eventReadJson, _ = json.Marshal(events[0])
	assert.Equal(t, string(eventJson), string(eventReadJson))

	// Negative test on filter
	filter = fb.And(
		fb.Eq("id", eventRead.ID.String()),
		fb.Eq("reference", fftypes.NewUUID().String()),
	)
	events, _, err = s.GetEvents(ctx, "ns1", filter)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(events))

	s.callbacks.AssertExpectations(t)
}

func TestGetEventsInSequenceRangeE2EWithDB(t *testing.T) {

	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	s.callbacks.On("OrderedUUIDCollectionNSEvent", database.CollectionEvents, core.ChangeEventTypeCreated, "ns1", mock.Anything, mock.Anything).Return()

	numberOfEvents := 1000
	var eventID *fftypes.UUID
	for i := 0; i < numberOfEvents; i++ {
		eventID = fftypes.NewUUID()
		event := &core.Event{
			ID:         eventID,
			Namespace:  "ns1",
			Type:       core.EventTypeMessageConfirmed,
			Reference:  fftypes.NewUUID(),
			Correlator: fftypes.NewUUID(),
			Topic:      fmt.Sprintf("topic%d", i%2),
			Created:    fftypes.Now(),
		}
		err := s.InsertEvent(ctx, event)
		assert.NoError(t, err)
	}

	fb := database.EventQueryFactory.NewFilter(ctx)
	fb.Limit(1000)

	// Check we can get back some events given a start and end
	events, _, err := s.GetEventsInSequenceRange(ctx, "ns1", fb.And(), 1, 1001)
	assert.Nil(t, err)
	assert.Equal(t, 1000, len(events))

	// Do some basic filtering to show that works
	fb2 := database.EventQueryFactory.NewFilter(ctx)
	fb2.Limit(1000)

	events, _, err = s.GetEventsInSequenceRange(ctx, "ns1", fb2.And(fb2.Eq("topic", "topic1")), 1, 1001)
	assert.Nil(t, err)
	assert.Equal(t, 500, len(events))

	// And get a single record from a range (EventID right now contains the last event ID created)
	fb3 := database.EventQueryFactory.NewFilter(ctx)
	fb3.Limit(1000)

	events, _, err = s.GetEventsInSequenceRange(ctx, "ns1", fb3.And(fb3.Eq("id", eventID.String())), 1, 1001)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(events))
}

func TestInsertEventFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.InsertEvent(context.Background(), &core.Event{})
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertEventFailLock(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("<acquire lock ns1>").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	eventID := fftypes.NewUUID()
	err := s.InsertEvent(context.Background(), &core.Event{ID: eventID, Namespace: "ns1"})
	assert.Regexp(t, "FF00187", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertEventFailInsert(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("<acquire lock ns1>").WillReturnResult(driver.ResultNoRows)
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	eventID := fftypes.NewUUID()
	err := s.InsertEvent(context.Background(), &core.Event{ID: eventID, Namespace: "ns1"})
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertEventFailCommit(t *testing.T) {
	s, mock := newMockProvider().init()
	eventID := fftypes.NewUUID()
	mock.ExpectBegin()
	mock.ExpectExec("<acquire lock ns1>").WillReturnResult(driver.ResultNoRows)
	mock.ExpectExec("INSERT .*").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(fmt.Errorf("pop"))
	err := s.InsertEvent(context.Background(), &core.Event{ID: eventID, Namespace: "ns1"})
	assert.Regexp(t, "FF00180", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertEventsPreCommitMultiRowOK(t *testing.T) {
	s := newMockProvider()
	s.multiRowInsert = true
	s.fakePSQLInsert = true
	s, mock := s.init()

	ev1 := &core.Event{ID: fftypes.NewUUID(), Namespace: "ns1"}
	ev2 := &core.Event{ID: fftypes.NewUUID(), Namespace: "ns1"}
	s.callbacks.On("OrderedUUIDCollectionNSEvent", database.CollectionEvents, core.ChangeEventTypeCreated, "ns1", ev1.ID, int64(1001))
	s.callbacks.On("OrderedUUIDCollectionNSEvent", database.CollectionEvents, core.ChangeEventTypeCreated, "ns1", ev2.ID, int64(1002))

	mock.ExpectBegin()
	mock.ExpectExec("<acquire lock ns1>").WillReturnResult(driver.ResultNoRows)
	mock.ExpectQuery("INSERT.*").WillReturnRows(sqlmock.NewRows([]string{s.SequenceColumn()}).
		AddRow(int64(1001)).
		AddRow(int64(1002)),
	)
	mock.ExpectCommit()
	ctx, tx, autoCommit, err := s.BeginOrUseTx(context.Background())
	tx.SetPreCommitAccumulator(&eventsPCA{
		s:      &s.SQLCommon,
		events: []*core.Event{ev1, ev2},
	})
	assert.NoError(t, err)
	err = s.CommitTx(ctx, tx, autoCommit)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

func TestInsertEventsPreCommitMultiRowFail(t *testing.T) {
	s := newMockProvider()
	s.multiRowInsert = true
	s.fakePSQLInsert = true
	s, mock := s.init()
	ev1 := &core.Event{ID: fftypes.NewUUID(), Namespace: "ns1"}
	mock.ExpectBegin()
	mock.ExpectExec("<acquire lock ns1>").WillReturnResult(driver.ResultNoRows)
	mock.ExpectQuery("INSERT.*").WillReturnError(fmt.Errorf("pop"))
	ctx, tx, autoCommit, err := s.BeginOrUseTx(context.Background())
	tx.SetPreCommitAccumulator(&eventsPCA{
		s:      &s.SQLCommon,
		events: []*core.Event{ev1},
	})
	assert.NoError(t, err)
	err = s.CommitTx(ctx, tx, autoCommit)
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

func TestInsertEventsPreCommitSingleRowFail(t *testing.T) {
	s, mock := newMockProvider().init()
	ev1 := &core.Event{ID: fftypes.NewUUID(), Namespace: "ns1"}
	mock.ExpectBegin()
	mock.ExpectExec("<acquire lock ns1>").WillReturnResult(driver.ResultNoRows)
	mock.ExpectExec("INSERT.*").WillReturnError(fmt.Errorf("pop"))
	ctx, tx, autoCommit, err := s.BeginOrUseTx(context.Background())
	tx.SetPreCommitAccumulator(&eventsPCA{
		s:      &s.SQLCommon,
		events: []*core.Event{ev1},
	})
	assert.NoError(t, err)
	err = s.CommitTx(ctx, tx, autoCommit)
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

func TestGetEventByIDSelectFail(t *testing.T) {
	s, mock := newMockProvider().init()
	eventID := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetEventByID(context.Background(), "ns1", eventID)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetEventByIDNotFound(t *testing.T) {
	s, mock := newMockProvider().init()
	eventID := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	msg, err := s.GetEventByID(context.Background(), "ns1", eventID)
	assert.NoError(t, err)
	assert.Nil(t, msg)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetEventByIDScanFail(t *testing.T) {
	s, mock := newMockProvider().init()
	eventID := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("only one"))
	_, err := s.GetEventByID(context.Background(), "ns1", eventID)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetEventsQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	f := database.EventQueryFactory.NewFilter(context.Background()).Eq("id", "")
	_, _, err := s.GetEvents(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetEventsBuildQueryFail(t *testing.T) {
	s, _ := newMockProvider().init()
	f := database.EventQueryFactory.NewFilter(context.Background()).Eq("id", map[bool]bool{true: false})
	_, _, err := s.GetEvents(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00143.*id", err)
}

func TestGettEventsReadMessageFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("only one"))
	f := database.EventQueryFactory.NewFilter(context.Background()).Eq("id", "")
	_, _, err := s.GetEvents(context.Background(), "ns1", f)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetEventsInSequenceRangeQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	f := database.EventQueryFactory.NewFilter(context.Background()).Eq("id", "")
	_, _, err := s.GetEventsInSequenceRange(context.Background(), "ns1", f, 0, 100)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetEventsInSequenceRangeBuildQueryFail(t *testing.T) {
	s, _ := newMockProvider().init()
	f := database.EventQueryFactory.NewFilter(context.Background()).Eq("id", map[bool]bool{true: false})
	_, _, err := s.GetEventsInSequenceRange(context.Background(), "ns1", f, 0, 100)
	assert.Regexp(t, "FF00143.*id", err)
}

func TestGetEventsInSequenceRangeShouldCallGetEventsWhenNoSequencedProvidedAndThrowAnError(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("only one"))
	f := database.EventQueryFactory.NewFilter(context.Background()).And()
	_, _, err := s.GetEventsInSequenceRange(context.Background(), "ns1", f, -1, -1)
	assert.NotNil(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
