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
	"github.com/stretchr/testify/mock"
)

func TestUpsertGroupE2EWithDB(t *testing.T) {
	log.SetLevel("debug")

	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new group
	groupHash := fftypes.NewRandB32()
	group := &core.Group{
		GroupIdentity: core.GroupIdentity{
			Name:      "group1",
			Namespace: "ns1",
			Members: core.Members{
				{Identity: "0x12345", Node: fftypes.NewUUID()},
				{Identity: "0x23456", Node: fftypes.NewUUID()},
			},
		},
		LocalNamespace: "ns1",
		Hash:           groupHash,
		Created:        fftypes.Now(),
	}

	s.callbacks.On("HashCollectionNSEvent", database.CollectionGroups, core.ChangeEventTypeCreated, "ns1", groupHash, mock.Anything).Return()
	s.callbacks.On("HashCollectionNSEvent", database.CollectionGroups, core.ChangeEventTypeUpdated, "ns1", groupHash, mock.Anything).Return()

	err := s.UpsertGroup(ctx, group, database.UpsertOptimizationNew)
	assert.NoError(t, err)

	// Check we get the exact same group back
	groupRead, err := s.GetGroupByHash(ctx, "ns1", group.Hash)
	assert.NoError(t, err)
	groupJson, _ := json.Marshal(&group)
	groupReadJson, _ := json.Marshal(&groupRead)
	assert.Equal(t, string(groupJson), string(groupReadJson))

	// Update the group (this is testing what's possible at the database layer,
	// and does not account for the verification that happens at the higher level)
	groupUpdated := &core.Group{
		GroupIdentity: core.GroupIdentity{
			Name:      "group1",
			Namespace: "ns1",
			Members:   group.Members,
		},
		LocalNamespace: "ns1",
		Created:        fftypes.Now(),
		Message:        fftypes.NewUUID(),
		Hash:           groupHash,
	}

	err = s.UpsertGroup(context.Background(), groupUpdated, database.UpsertOptimizationExisting)
	assert.NoError(t, err)

	// Check we get the exact same group back - note the removal of one of the data elements
	groupRead, err = s.GetGroupByHash(ctx, "ns1", group.Hash)
	assert.NoError(t, err)
	groupJson, _ = json.Marshal(&groupUpdated)
	groupReadJson, _ = json.Marshal(&groupRead)
	assert.Equal(t, string(groupJson), string(groupReadJson))

	// Query back the group
	fb := database.GroupQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("hash", groupUpdated.Hash),
		fb.Eq("message", groupUpdated.Message),
		fb.Gt("created", "0"),
	)
	groups, _, err := s.GetGroups(ctx, "ns1", filter)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(groups))
	groupReadJson, _ = json.Marshal(groups[0])
	assert.Equal(t, string(groupJson), string(groupReadJson))

	// Negative test on filter
	filter = fb.And(
		fb.Eq("hash", groupUpdated.Hash.String()),
		fb.Eq("created", "0"),
	)
	groups, _, err = s.GetGroups(ctx, "ns1", filter)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(groups))

	s.callbacks.AssertExpectations(t)
}

func TestUpsertGroupFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertGroup(context.Background(), &core.Group{}, database.UpsertOptimizationSkip)
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertGroupFailSelect(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	groupID := fftypes.NewRandB32()
	err := s.UpsertGroup(context.Background(), &core.Group{Hash: groupID}, database.UpsertOptimizationSkip)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertGroupFailInsert(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{}))
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	groupID := fftypes.NewRandB32()
	err := s.UpsertGroup(context.Background(), &core.Group{Hash: groupID}, database.UpsertOptimizationSkip)
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertGroupFailUpdate(t *testing.T) {
	s, mock := newMockProvider().init()
	groupID := fftypes.NewRandB32()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"hash"}).AddRow(groupID.String()))
	mock.ExpectExec("UPDATE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpsertGroup(context.Background(), &core.Group{Hash: groupID}, database.UpsertOptimizationSkip)
	assert.Regexp(t, "FF00178", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertGroupFailMembers(t *testing.T) {
	s, mock := newMockProvider().init()
	groupID := fftypes.NewRandB32()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"hash"}))
	mock.ExpectExec("INSERT .*").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpsertGroup(context.Background(), &core.Group{
		Hash: groupID,
		GroupIdentity: core.GroupIdentity{
			Members: core.Members{
				{Identity: "org1", Node: fftypes.NewUUID()},
			},
		},
	}, database.UpsertOptimizationSkip)
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertGroupFailCommit(t *testing.T) {
	s, mock := newMockProvider().init()
	groupID := fftypes.NewRandB32()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"hash"}))
	mock.ExpectExec("INSERT .*").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertGroup(context.Background(), &core.Group{Hash: groupID}, database.UpsertOptimizationSkip)
	assert.Regexp(t, "FF00180", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateMembersRecreateFail(t *testing.T) {
	s, mock := newMockProvider().init()
	groupID := fftypes.NewRandB32()
	mock.ExpectBegin()
	mock.ExpectExec("DELETE .*").WillReturnError(fmt.Errorf("pop"))
	ctx, tx, _, err := s.BeginOrUseTx(context.Background())
	assert.NoError(t, err)
	err = s.updateMembers(ctx, tx, &core.Group{
		Hash: groupID,
		GroupIdentity: core.GroupIdentity{
			Members: core.Members{{Node: fftypes.NewUUID()}},
		},
	}, true)
	assert.Regexp(t, "FF00179", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateMembersMissingOrg(t *testing.T) {
	s, mock := newMockProvider().init()
	groupID := fftypes.NewRandB32()
	mock.ExpectBegin()
	ctx, tx, _, err := s.BeginOrUseTx(context.Background())
	assert.NoError(t, err)
	err = s.updateMembers(ctx, tx, &core.Group{
		Hash: groupID,
		GroupIdentity: core.GroupIdentity{
			Members: core.Members{{Node: fftypes.NewUUID()}},
		},
	}, false)
	assert.Regexp(t, "FF00116", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateMembersMissingNode(t *testing.T) {
	s, mock := newMockProvider().init()
	groupID := fftypes.NewRandB32()
	mock.ExpectBegin()
	ctx, tx, _, err := s.BeginOrUseTx(context.Background())
	assert.NoError(t, err)
	err = s.updateMembers(ctx, tx, &core.Group{
		Hash: groupID,
		GroupIdentity: core.GroupIdentity{
			Members: core.Members{{Identity: "0x12345"}},
		},
	}, false)
	assert.Regexp(t, "FF00117", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateGroupDataDeleteFail(t *testing.T) {
	s, mock := newMockProvider().init()
	groupID := fftypes.NewRandB32()
	mock.ExpectBegin()
	ctx, tx, _, err := s.BeginOrUseTx(context.Background())
	assert.NoError(t, err)
	mock.ExpectExec("DELETE .*").WillReturnError(fmt.Errorf("pop"))
	err = s.updateMembers(ctx, tx, &core.Group{
		Hash: groupID,
	}, true)
	assert.Regexp(t, "FF00179", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpdateGroupDataAddFail(t *testing.T) {
	s, mock := newMockProvider().init()
	groupID := fftypes.NewRandB32()
	mock.ExpectBegin()
	ctx, tx, _, err := s.BeginOrUseTx(context.Background())
	assert.NoError(t, err)
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	err = s.updateMembers(ctx, tx, &core.Group{
		Hash: groupID,
		GroupIdentity: core.GroupIdentity{
			Members: core.Members{{Identity: "0x12345", Node: fftypes.NewUUID()}},
		},
	}, false)
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadMembersQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	groupID := fftypes.NewRandB32()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	err := s.loadMembers(context.Background(), []*core.Group{{Hash: groupID}})
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadMembersScanFail(t *testing.T) {
	s, mock := newMockProvider().init()
	groupID := fftypes.NewRandB32()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"identity"}).AddRow("only one"))
	err := s.loadMembers(context.Background(), []*core.Group{{Hash: groupID}})
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestLoadMembersEmpty(t *testing.T) {
	s, mock := newMockProvider().init()
	groupID := fftypes.NewRandB32()
	group := &core.Group{Hash: groupID}
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"identity"}))
	err := s.loadMembers(context.Background(), []*core.Group{group})
	assert.NoError(t, err)
	assert.Equal(t, core.Members{}, group.Members)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetGroupByIDSelectFail(t *testing.T) {
	s, mock := newMockProvider().init()
	groupID := fftypes.NewRandB32()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetGroupByHash(context.Background(), "ns1", groupID)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetGroupByIDNotFound(t *testing.T) {
	s, mock := newMockProvider().init()
	groupID := fftypes.NewRandB32()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"hash"}))
	group, err := s.GetGroupByHash(context.Background(), "ns1", groupID)
	assert.NoError(t, err)
	assert.Nil(t, group)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetGroupByIDScanFail(t *testing.T) {
	s, mock := newMockProvider().init()
	groupID := fftypes.NewRandB32()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"hash"}).AddRow("only one"))
	_, err := s.GetGroupByHash(context.Background(), "ns1", groupID)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetGroupByIDLoadMembersFail(t *testing.T) {
	s, mock := newMockProvider().init()
	groupID := fftypes.NewRandB32()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows(groupColumns).
		AddRow(nil, "ns1", "ns1", "name1", fftypes.NewRandB32(), fftypes.Now()))
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetGroupByHash(context.Background(), "ns1", groupID)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetGroupsBuildQueryFail(t *testing.T) {
	s, _ := newMockProvider().init()
	f := database.GroupQueryFactory.NewFilter(context.Background()).Eq("hash", map[bool]bool{true: false})
	_, _, err := s.GetGroups(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00143.*hash", err)
}

func TestGetGroupsQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	f := database.GroupQueryFactory.NewFilter(context.Background()).Eq("hash", "")
	_, _, err := s.GetGroups(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetGroupsReadGroupFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"hash"}).AddRow("only one"))
	f := database.GroupQueryFactory.NewFilter(context.Background()).Eq("hash", "")
	_, _, err := s.GetGroups(context.Background(), "ns1", f)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetGroupsLoadMembersFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows(groupColumns).
		AddRow(nil, "ns1", "ns1", "group1", fftypes.NewRandB32(), fftypes.Now()))
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	f := database.GroupQueryFactory.NewFilter(context.Background()).Gt("created", "0")
	_, _, err := s.GetGroups(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
