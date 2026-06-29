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

func TestBlockchainEventsE2EWithDB(t *testing.T) {
	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new contract event entry
	event := &core.BlockchainEvent{
		ID:         fftypes.NewUUID(),
		Namespace:  "ns",
		Listener:   fftypes.NewUUID(),
		Name:       "Changed",
		ProtocolID: "tx1",
		Output:     fftypes.JSONObject{"value": 1},
		Info:       fftypes.JSONObject{"blockNumber": 1},
		Timestamp:  fftypes.Now(),
		TX: core.BlockchainTransactionRef{
			ID:           fftypes.NewUUID(),
			Type:         core.TransactionTypeBatchPin,
			BlockchainID: "0x12345",
		},
	}

	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionBlockchainEvents, core.ChangeEventTypeCreated, "ns", event.ID).Return().Once()

	_, err := s.InsertOrGetBlockchainEvent(ctx, event)
	assert.NotNil(t, event.Timestamp)
	assert.NoError(t, err)
	eventJson, _ := json.Marshal(&event)

	// Query back the event (by query filter)
	fb := database.BlockchainEventQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("name", "Changed"),
		fb.Eq("listener", event.Listener),
	)
	events, res, err := s.GetBlockchainEvents(ctx, "ns", filter.Count(true))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(events))
	assert.Equal(t, int64(1), *res.TotalCount)
	eventReadJson, _ := json.Marshal(events[0])
	assert.Equal(t, string(eventJson), string(eventReadJson))

	// Query back the event (by ID)
	eventRead, err := s.GetBlockchainEventByID(ctx, "ns", event.ID)
	assert.NoError(t, err)
	eventReadJson, _ = json.Marshal(eventRead)
	assert.Equal(t, string(eventJson), string(eventReadJson))

	// Query back the event (by protocol ID)
	eventRead, err = s.GetBlockchainEventByProtocolID(ctx, event.Namespace, event.Listener, event.ProtocolID)
	assert.NoError(t, err)
	eventReadJson, _ = json.Marshal(eventRead)
	assert.Equal(t, string(eventJson), string(eventReadJson))

	// Try to insert again with a new ID - should return existing row
	event2 := &core.BlockchainEvent{
		ID:         fftypes.NewUUID(),
		Namespace:  event.Namespace,
		Listener:   event.Listener,
		ProtocolID: event.ProtocolID,
		Timestamp:  fftypes.Now(),
	}
	existing, err := s.InsertOrGetBlockchainEvent(ctx, event2)
	assert.NoError(t, err)
	assert.Equal(t, event.ID, existing.ID)

	// Try to insert an event twice (with nil listener) - should return existing row
	event3 := &core.BlockchainEvent{
		ID:         fftypes.NewUUID(),
		Namespace:  "ns",
		ProtocolID: "tx2",
		Timestamp:  fftypes.Now(),
	}
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionBlockchainEvents, core.ChangeEventTypeCreated, "ns", event3.ID).Return().Once()
	existing, err = s.InsertOrGetBlockchainEvent(ctx, event3)
	assert.NoError(t, err)
	assert.Nil(t, existing)
	event4 := &core.BlockchainEvent{
		ID:         fftypes.NewUUID(),
		Namespace:  "ns",
		ProtocolID: "tx2",
		Timestamp:  fftypes.Now(),
	}
	existing, err = s.InsertOrGetBlockchainEvent(ctx, event4)
	assert.NoError(t, err)
	assert.Equal(t, event3.ID, existing.ID)

}

func TestInsertBlockchainEventFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	_, err := s.InsertOrGetBlockchainEvent(context.Background(), &core.BlockchainEvent{})
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertBlockchainEventsBeginFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.InsertBlockchainEvents(context.Background(), []*core.BlockchainEvent{})
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

func TestInsertBlockchainEventsMultiRowOK(t *testing.T) {
	s := newMockProvider()
	s.multiRowInsert = true
	s.fakePSQLInsert = true
	s, mock := s.init()

	be1 := &core.BlockchainEvent{ID: fftypes.NewUUID(), Namespace: "ns1"}
	be2 := &core.BlockchainEvent{ID: fftypes.NewUUID(), Namespace: "ns1"}
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionBlockchainEvents, core.ChangeEventTypeCreated, "ns1", be1.ID)
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionBlockchainEvents, core.ChangeEventTypeCreated, "ns1", be2.ID)

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT.*blockchainevents").WillReturnRows(sqlmock.NewRows([]string{s.SequenceColumn()}).
		AddRow(int64(1001)).
		AddRow(int64(1002)),
	)
	mock.ExpectCommit()
	hookCalled := make(chan struct{}, 1)
	err := s.InsertBlockchainEvents(context.Background(), []*core.BlockchainEvent{be1, be2}, func() {
		close(hookCalled)
	})
	<-hookCalled
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

func TestInsertBlockchainEventsMultiRowFail(t *testing.T) {
	s := newMockProvider()
	s.multiRowInsert = true
	s.fakePSQLInsert = true
	s, mock := s.init()
	be1 := &core.BlockchainEvent{ID: fftypes.NewUUID(), Namespace: "ns1"}
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT.*").WillReturnError(fmt.Errorf("pop"))
	err := s.InsertBlockchainEvents(context.Background(), []*core.BlockchainEvent{be1})
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

func TestInsertBlockchainEventsSingleRowFail(t *testing.T) {
	s, mock := newMockProvider().init()
	be1 := &core.BlockchainEvent{ID: fftypes.NewUUID(), Namespace: "ns1"}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT.*").WillReturnError(fmt.Errorf("pop"))
	err := s.InsertBlockchainEvents(context.Background(), []*core.BlockchainEvent{be1})
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

func TestInsertBlockchainEventFailInsert(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{}))
	mock.ExpectRollback()
	_, err := s.InsertOrGetBlockchainEvent(context.Background(), &core.BlockchainEvent{})
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertBlockchainEventFailCommit(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT .*").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(fmt.Errorf("pop"))
	_, err := s.InsertOrGetBlockchainEvent(context.Background(), &core.BlockchainEvent{})
	assert.Regexp(t, "FF00180", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetBlockchainEventByIDSelectFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetBlockchainEventByID(context.Background(), "ns", fftypes.NewUUID())
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetBlockchainEventByIDNotFound(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{}))
	be, err := s.GetBlockchainEventByID(context.Background(), "ns", fftypes.NewUUID())
	assert.NoError(t, err)
	assert.Nil(t, be)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetBlockchainEventByIDScanFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("only one"))
	_, err := s.GetBlockchainEventByID(context.Background(), "ns", fftypes.NewUUID())
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetBlockchainEventsQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	f := database.BlockchainEventQueryFactory.NewFilter(context.Background()).Eq("id", "")
	_, _, err := s.GetBlockchainEvents(context.Background(), "ns", f)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetBlockchainEventsBuildQueryFail(t *testing.T) {
	s, _ := newMockProvider().init()
	f := database.BlockchainEventQueryFactory.NewFilter(context.Background()).Eq("id", map[bool]bool{true: false})
	_, _, err := s.GetBlockchainEvents(context.Background(), "ns", f)
	assert.Regexp(t, "FF00143.*id", err)
}

func TestGetBlockchainEventsScanFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("only one"))
	f := database.BlockchainEventQueryFactory.NewFilter(context.Background()).Eq("id", "")
	_, _, err := s.GetBlockchainEvents(context.Background(), "ns", f)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
