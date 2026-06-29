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
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/stretchr/testify/assert"
)

func TestContractListenerLegacyE2EWithDB(t *testing.T) {
	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new contract listener entry
	location := fftypes.JSONObject{"path": "my-api"}
	locationJson, _ := json.Marshal(location)
	sub := &core.ContractListener{
		ID: fftypes.NewUUID(),
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
		Event: &core.FFISerializedEvent{
			FFIEventDefinition: fftypes.FFIEventDefinition{
				Name: "event1",
			},
		},
		Namespace: "ns",
		Name:      "sub1",
		BackendID: "sb-123",
		Location:  fftypes.JSONAnyPtrBytes(locationJson),
		Topic:     "topic1",
		Options: &core.ContractListenerOptions{
			FirstEvent: "0",
		},
	}

	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionContractListeners, core.ChangeEventTypeCreated, "ns", sub.ID).Return()
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionContractListeners, core.ChangeEventTypeUpdated, "ns", sub.ID).Return()
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionContractListeners, core.ChangeEventTypeDeleted, "ns", sub.ID).Return()

	err := s.InsertContractListener(ctx, sub)
	assert.NotNil(t, sub.Created)
	assert.NoError(t, err)
	subJson, _ := json.Marshal(&sub)

	// Query back the listener (by query filter)
	fb := database.ContractListenerQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("backendid", sub.BackendID),
	)
	subs, res, err := s.GetContractListeners(ctx, "ns", filter.Count(true))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(subs))
	assert.Equal(t, int64(1), *res.TotalCount)
	subReadJson, _ := json.Marshal(subs[0])
	assert.Equal(t, string(subJson), string(subReadJson))

	// Update by backend ID
	err = s.UpdateContractListener(ctx, "ns", sub.ID, database.ContractListenerQueryFactory.NewUpdate(ctx).Set("backendid", "sb-234"))
	assert.NoError(t, err)

	// Query back the listener (by name)
	subRead, err := s.GetContractListener(ctx, "ns", "sub1")
	assert.NoError(t, err)
	sub.BackendID = "sb-234"
	subJson, _ = json.Marshal(&sub)
	subReadJson, _ = json.Marshal(subRead)
	assert.Equal(t, string(subJson), string(subReadJson))

	// Query back the listener (by ID)
	subRead, err = s.GetContractListenerByID(ctx, "ns", sub.ID)
	assert.NoError(t, err)
	subReadJson, _ = json.Marshal(subRead)
	assert.Equal(t, string(subJson), string(subReadJson))

	// Query back the listener (by protocol ID)
	subRead, err = s.GetContractListenerByBackendID(ctx, "ns", sub.BackendID)
	assert.NoError(t, err)
	subReadJson, _ = json.Marshal(subRead)
	assert.Equal(t, string(subJson), string(subReadJson))

	// Query back the listener (by query filter)
	filter = fb.And(
		fb.Eq("backendid", sub.BackendID),
	)
	subs, res, err = s.GetContractListeners(ctx, "ns", filter.Count(true))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(subs))
	assert.Equal(t, int64(1), *res.TotalCount)
	subReadJson, _ = json.Marshal(subs[0])
	assert.Equal(t, string(subJson), string(subReadJson))

	// Test delete, and refind no return
	err = s.DeleteContractListenerByID(ctx, "ns", sub.ID)
	assert.NoError(t, err)
	subs, _, err = s.GetContractListeners(ctx, "ns", filter)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(subs))
}

func TestInsertContractListenerFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.InsertContractListener(context.Background(), &core.ContractListener{})
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertContractListenerFailInsert(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.InsertContractListener(context.Background(), &core.ContractListener{})
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertContractListenerFailCommit(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT .*").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(fmt.Errorf("pop"))
	err := s.InsertContractListener(context.Background(), &core.ContractListener{})
	assert.Regexp(t, "FF00180", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetContractListenerByIDSelectFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetContractListenerByID(context.Background(), "ns", fftypes.NewUUID())
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetContractListenerByIDNotFound(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"backendid"}))
	msg, err := s.GetContractListenerByID(context.Background(), "ns", fftypes.NewUUID())
	assert.NoError(t, err)
	assert.Nil(t, msg)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetContractListenerByIDScanFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"backendid"}).AddRow("only one"))
	_, err := s.GetContractListenerByID(context.Background(), "ns", fftypes.NewUUID())
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetContractListenersQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	f := database.ContractListenerQueryFactory.NewFilter(context.Background()).Eq("backendid", "")
	_, _, err := s.GetContractListeners(context.Background(), "ns", f)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetContractListenersBuildQueryFail(t *testing.T) {
	s, _ := newMockProvider().init()
	f := database.ContractListenerQueryFactory.NewFilter(context.Background()).Eq("backendid", map[bool]bool{true: false})
	_, _, err := s.GetContractListeners(context.Background(), "ns", f)
	assert.Regexp(t, "FF00143.*id", err)
}

func TestGetContractListenersScanFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"backendid"}).AddRow("only one"))
	f := database.ContractListenerQueryFactory.NewFilter(context.Background()).Eq("backendid", "")
	_, _, err := s.GetContractListeners(context.Background(), "ns", f)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContractListenerDeleteBeginFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.DeleteContractListenerByID(context.Background(), "ns", fftypes.NewUUID())
	assert.Regexp(t, "FF00175", err)
}

func TestContractListenerDeleteFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows(contractListenerColumns).AddRow(
		fftypes.NewUUID(), nil, []byte("{}"), "ns1", "sub1", "123", "{}", "sig", "topic1", nil, fftypes.Now(), "[]"),
	)
	mock.ExpectExec("DELETE .*").WillReturnError(fmt.Errorf("pop"))
	err := s.DeleteContractListenerByID(context.Background(), "ns", fftypes.NewUUID())
	assert.Regexp(t, "FF00179", err)
}

func TestContractListenerOptions(t *testing.T) {
	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	l := &core.ContractListener{
		ID:        fftypes.NewUUID(),
		Namespace: "ns",
		Event:     &core.FFISerializedEvent{},
		Location:  fftypes.JSONAnyPtr("{}"),
		Options: &core.ContractListenerOptions{
			FirstEvent: string(core.SubOptsFirstEventOldest),
		},
	}

	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionContractListeners, core.ChangeEventTypeCreated, "ns", l.ID).Return()

	err := s.InsertContractListener(ctx, l)
	assert.NoError(t, err)

	li, err := s.GetContractListenerByID(ctx, "ns", l.ID)
	assert.NoError(t, err)

	assert.Equal(t, l.Options, li.Options)
}

func TestUpdateContractListenerFailFilter(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	err := s.UpdateContractListener(context.Background(), "ns1", fftypes.NewUUID(),
		database.ContractListenerQueryFactory.NewUpdate(context.Background()).Set("wrong", "sb-234"))
	assert.Regexp(t, "FF00142", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateContractListenerFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.UpdateContractListener(context.Background(), "ns1", fftypes.NewUUID(),
		database.ContractListenerQueryFactory.NewUpdate(context.Background()).Set("backendid", "sb-234"))
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateContractListenerFailUpdate(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpdateContractListener(context.Background(), "ns1", fftypes.NewUUID(),
		database.ContractListenerQueryFactory.NewUpdate(context.Background()).Set("backendid", "sb-234"))
	assert.Regexp(t, "FF00178", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateContractListenerNotFount(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE .*").WillReturnResult(driver.ResultNoRows)
	mock.ExpectRollback()
	err := s.UpdateContractListener(context.Background(), "ns1", fftypes.NewUUID(),
		database.ContractListenerQueryFactory.NewUpdate(context.Background()).Set("backendid", "sb-234"))
	assert.Regexp(t, "FF10143", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertContractListenerFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertContractListener(context.Background(), &core.ContractListener{}, false)
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertContractListenerFailInsert(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpsertContractListener(context.Background(), &core.ContractListener{}, false)
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertContractListenerFailCommit(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT .*").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertContractListener(context.Background(), &core.ContractListener{}, false)
	assert.Regexp(t, "FF00180", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestContractListenerE2eWithDB(t *testing.T) {
	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new contract listener entry
	location := fftypes.JSONObject{"path": "my-api"}
	locationJson, _ := json.Marshal(location)
	sub := &core.ContractListener{
		ID: fftypes.NewUUID(),
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
		Event: &core.FFISerializedEvent{
			FFIEventDefinition: fftypes.FFIEventDefinition{
				Name: "event1",
			},
		},
		Filters: []*core.ListenerFilter{
			{
				Event: &core.FFISerializedEvent{
					FFIEventDefinition: fftypes.FFIEventDefinition{
						Name: "event1",
					},
				},
			},
		},
		Namespace: "ns",
		Name:      "sub1",
		BackendID: "sb-123",
		Location:  fftypes.JSONAnyPtrBytes(locationJson),
		Topic:     "topic1",
		Options: &core.ContractListenerOptions{
			FirstEvent: "0",
		},
	}

	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionContractListeners, core.ChangeEventTypeCreated, "ns", sub.ID).Return()
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionContractListeners, core.ChangeEventTypeUpdated, "ns", sub.ID).Return()
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionContractListeners, core.ChangeEventTypeDeleted, "ns", sub.ID).Return()

	err := s.UpsertContractListener(ctx, sub, false)
	assert.NotNil(t, sub.Created)
	assert.NoError(t, err)
	subJson, _ := json.Marshal(&sub)

	// Query back the listener (by query filter)
	fb := database.ContractListenerQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("backendid", sub.BackendID),
	)
	subs, res, err := s.GetContractListeners(ctx, "ns", filter.Count(true))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(subs))
	assert.Equal(t, int64(1), *res.TotalCount)
	subReadJson, _ := json.Marshal(subs[0])
	assert.Equal(t, string(subJson), string(subReadJson))

	sub2 := &core.ContractListener{
		ID: fftypes.NewUUID(),
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
		Event: &core.FFISerializedEvent{
			FFIEventDefinition: fftypes.FFIEventDefinition{
				Name: "event1",
			},
		},
		Filters: []*core.ListenerFilter{
			{
				Event: &core.FFISerializedEvent{
					FFIEventDefinition: fftypes.FFIEventDefinition{
						Name: "event1",
					},
				},
			},
		},
		Namespace: "ns",
		Name:      "sub1",
		BackendID: "sb-123",
		Location:  fftypes.JSONAnyPtrBytes(locationJson),
		Topic:     "topic1",
		Options: &core.ContractListenerOptions{
			FirstEvent: "0",
		},
	}

	// Rejects attempt to update ID
	err = s.UpsertContractListener(context.Background(), sub2, true)
	assert.Equal(t, database.IDMismatch, err)

	// Update by backend ID
	sub.BackendID = "sb-234"
	err = s.UpsertContractListener(ctx, sub, true)
	assert.NoError(t, err)

	// Query back the listener (by name)
	subRead, err := s.GetContractListener(ctx, "ns", "sub1")
	assert.NoError(t, err)
	sub.BackendID = "sb-234"
	subJson, _ = json.Marshal(&sub)
	subReadJson, _ = json.Marshal(subRead)
	assert.Equal(t, string(subJson), string(subReadJson))

	// Query back the listener (by ID)
	subRead, err = s.GetContractListenerByID(ctx, "ns", sub.ID)
	assert.NoError(t, err)
	subReadJson, _ = json.Marshal(subRead)
	assert.Equal(t, string(subJson), string(subReadJson))

	// Query back the listener (by protocol ID)
	subRead, err = s.GetContractListenerByBackendID(ctx, "ns", sub.BackendID)
	assert.NoError(t, err)
	subReadJson, _ = json.Marshal(subRead)
	assert.Equal(t, string(subJson), string(subReadJson))

	// Query back the listener (by query filter)
	filter = fb.And(
		fb.Eq("backendid", sub.BackendID),
	)
	subs, res, err = s.GetContractListeners(ctx, "ns", filter.Count(true))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(subs))
	assert.Equal(t, int64(1), *res.TotalCount)
	subReadJson, _ = json.Marshal(subs[0])
	assert.Equal(t, string(subJson), string(subReadJson))

	// Test delete, and refind no return
	err = s.DeleteContractListenerByID(ctx, "ns", sub.ID)
	assert.NoError(t, err)
	subs, _, err = s.GetContractListeners(ctx, "ns", filter)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(subs))
}

func TestUpsertContractListenerFailBeginExisting(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertContractListener(context.Background(), &core.ContractListener{}, true)
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertContractListenerFailSelect(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	id := fftypes.NewUUID()
	err := s.UpsertContractListener(context.Background(), &core.ContractListener{ID: id}, true)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertContractListenerFailUpdate(t *testing.T) {
	s, mock := newMockProvider().init()
	id := fftypes.NewUUID()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow(id.String()))
	mock.ExpectExec("UPDATE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpsertContractListener(context.Background(), &core.ContractListener{ID: id}, true)
	assert.Regexp(t, "pop", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
