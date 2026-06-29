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

func TestFFIEventsE2EWithDB(t *testing.T) {

	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new event entry
	interfaceID := fftypes.NewUUID()
	eventID := fftypes.NewUUID()
	event := &fftypes.FFIEvent{
		ID:        eventID,
		Interface: interfaceID,
		Namespace: "ns",
		Pathname:  "Changed_1",
		FFIEventDefinition: fftypes.FFIEventDefinition{
			Name:        "Changed",
			Description: "Things changed",
			Params: fftypes.FFIParams{
				{
					Name:   "value",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer"}`),
				},
			},
		},
	}

	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionFFIEvents, core.ChangeEventTypeCreated, "ns", eventID).Return()
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionFFIEvents, core.ChangeEventTypeUpdated, "ns", eventID).Return()

	err := s.UpsertFFIEvent(ctx, event)
	assert.NoError(t, err)

	// Query back the event (by name)
	eventRead, err := s.GetFFIEvent(ctx, "ns", interfaceID, "Changed_1")
	assert.NoError(t, err)
	assert.NotNil(t, eventRead)
	eventJson, _ := json.Marshal(&event)
	eventReadJson, _ := json.Marshal(&eventRead)
	assert.Equal(t, string(eventJson), string(eventReadJson))

	// Query back the event (by query filter)
	fb := database.FFIEventQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("id", eventRead.ID.String()),
		fb.Eq("name", eventRead.Name),
	)
	events, res, err := s.GetFFIEvents(ctx, "ns", filter.Count(true))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(events))
	assert.Equal(t, int64(1), *res.TotalCount)
	eventReadJson, _ = json.Marshal(events[0])
	assert.Equal(t, string(eventJson), string(eventReadJson))

	// Update event
	event.Params = fftypes.FFIParams{}
	err = s.UpsertFFIEvent(ctx, event)
	assert.NoError(t, err)

	s.callbacks.AssertExpectations(t)
}

func TestFFIEventDBFailBeginTransaction(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertFFIEvent(context.Background(), &fftypes.FFIEvent{})
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFFIEventDBFailSelect(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertFFIEvent(context.Background(), &fftypes.FFIEvent{})
	assert.Regexp(t, "pop", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFFIEventDBFailInsert(t *testing.T) {
	rows := sqlmock.NewRows([]string{"id", "namespace", "name", "version"})
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(rows)
	event := &fftypes.FFIEvent{
		ID: fftypes.NewUUID(),
	}
	err := s.UpsertFFIEvent(context.Background(), event)
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFFIEventDBFailUpdate(t *testing.T) {
	rows := sqlmock.NewRows([]string{"id", "namespace", "name", "version"}).
		AddRow("7e2c001c-e270-4fd7-9e82-9dacee843dc2", "ns1", "math", "v1.0.0")
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(rows)
	mock.ExpectQuery("UPDATE .*").WillReturnError(fmt.Errorf("pop"))
	event := &fftypes.FFIEvent{
		ID: fftypes.NewUUID(),
	}
	err := s.UpsertFFIEvent(context.Background(), event)
	assert.Regexp(t, "pop", err)
}

func TestFFIEventDBFailScan(t *testing.T) {
	s, mock := newMockProvider().init()
	id := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("only one"))
	_, err := s.GetFFIEvent(context.Background(), id.String(), fftypes.NewUUID(), "sum")
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFFIEventDBSelectFail(t *testing.T) {
	s, mock := newMockProvider().init()
	id := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetFFIEvent(context.Background(), id.String(), fftypes.NewUUID(), "sum")
	assert.Regexp(t, "pop", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestFFIEventDBNoRows(t *testing.T) {
	s, mock := newMockProvider().init()
	id := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows(ffiEventsColumns))
	_, err := s.GetFFIEvent(context.Background(), id.String(), fftypes.NewUUID(), "sum")
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetFFIEvents(t *testing.T) {
	// filter := database.FFIEventQueryFactory.NewFilter(context.Background()).In("", []driver.Value{})

	fb := database.FFIEventQueryFactory.NewFilter(context.Background())
	filter := fb.And(
		fb.Eq("name", "sum"),
	)
	s, mock := newMockProvider().init()
	rows := sqlmock.NewRows(ffiEventsColumns).
		AddRow(fftypes.NewUUID().String(), fftypes.NewUUID().String(), "ns1", "sum", "sum", "", []byte(`[]`), []byte(`{}`))
	mock.ExpectQuery("SELECT .*").WillReturnRows(rows)
	_, _, err := s.GetFFIEvents(context.Background(), "ns1", filter)
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetFFIEventsFilterSelectFail(t *testing.T) {
	fb := database.FFIEventQueryFactory.NewFilter(context.Background())
	s, _ := newMockProvider().init()
	_, _, err := s.GetFFIEvents(context.Background(), "ns1", fb.And(fb.Eq("id", map[bool]bool{true: false})))
	assert.Error(t, err)
}

func TestGetFFIEventsQueryFail(t *testing.T) {
	fb := database.FFIEventQueryFactory.NewFilter(context.Background())
	filter := fb.And(
		fb.Eq("id", fftypes.NewUUID()),
	)
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, _, err := s.GetFFIEvents(context.Background(), "ns1", filter)
	assert.Regexp(t, "pop", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetFFIEventsQueryResultFail(t *testing.T) {
	fb := database.FFIEventQueryFactory.NewFilter(context.Background())
	filter := fb.And(
		fb.Eq("id", fftypes.NewUUID()),
	)
	s, mock := newMockProvider().init()
	rows := sqlmock.NewRows([]string{"id", "namespace", "name", "version"}).
		AddRow("7e2c001c-e270-4fd7-9e82-9dacee843dc2", "ns1", "math", "v1.0.0").
		AddRow("7e2c001c-e270-4fd7-9e82-9dacee843dc2", nil, "math", "v1.0.0")
	mock.ExpectQuery("SELECT .*").WillReturnRows(rows)
	_, _, err := s.GetFFIEvents(context.Background(), "ns1", filter)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetFFIEvent(t *testing.T) {
	s, mock := newMockProvider().init()
	rows := sqlmock.NewRows(ffiEventsColumns).
		AddRow(fftypes.NewUUID().String(), fftypes.NewUUID().String(), "ns1", "sum", "sum", "", []byte(`[]`), []byte(`{}`))
	mock.ExpectQuery("SELECT .*").WillReturnRows(rows)
	FFIEvent, err := s.GetFFIEvent(context.Background(), "ns1", fftypes.NewUUID(), "math")
	assert.NoError(t, err)
	assert.Equal(t, "sum", FFIEvent.Name)
	assert.NoError(t, mock.ExpectationsWereMet())
}
