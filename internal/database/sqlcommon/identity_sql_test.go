// Copyright © 2021 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.identity/licenses/LICENSE-2.0
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

func TestIdentitiesE2EWithDB(t *testing.T) {
	log.SetLevel("debug")

	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new identity entry
	identityID := fftypes.NewUUID()
	identity := &core.Identity{
		IdentityBase: core.IdentityBase{
			ID:        identityID,
			DID:       "did:firefly:/ns/ns1/1",
			Parent:    fftypes.NewUUID(),
			Type:      core.IdentityTypeCustom,
			Namespace: "ns1",
			Name:      "identity1",
		},
		IdentityProfile: core.IdentityProfile{
			Description: "Identity One",
		},
		Messages: core.IdentityMessages{
			Claim: fftypes.NewUUID(),
		},
	}

	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionIdentities, core.ChangeEventTypeCreated, "ns1", identityID).Return()
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionIdentities, core.ChangeEventTypeUpdated, "ns1", identityID).Return()

	err := s.UpsertIdentity(ctx, identity, database.UpsertOptimizationNew)
	assert.NoError(t, err)

	// Check we get the exact same identity back
	identityRead, err := s.GetIdentityByID(ctx, "ns1", identity.ID)
	assert.NoError(t, err)
	assert.NotNil(t, identityRead)
	identityJson, _ := json.Marshal(&identity)
	identityReadJson, _ := json.Marshal(&identityRead)
	assert.Equal(t, string(identityJson), string(identityReadJson))

	// Update the identity (this is testing what's possible at the database layer,
	// and does not account for the verification that happens at the higher level)
	identityUpdated := &core.Identity{
		IdentityBase: core.IdentityBase{
			ID:        identityID,
			DID:       "did:firefly:/nodes/2",
			Parent:    fftypes.NewUUID(),
			Type:      core.IdentityTypeNode,
			Namespace: "ns1",
			Name:      "identity2",
		},
		IdentityProfile: core.IdentityProfile{
			Description: "Identity Two",
			Profile:     fftypes.JSONObject{"some": "value"},
		},
		Messages: core.IdentityMessages{
			Claim:        fftypes.NewUUID(),
			Verification: fftypes.NewUUID(),
			Update:       fftypes.NewUUID(),
		},
		Created: identity.Created,
	}
	err = s.UpsertIdentity(context.Background(), identityUpdated, database.UpsertOptimizationExisting)
	assert.NoError(t, err)

	// Check we get the exact same data back - note the removal of one of the identity elements
	identityRead, err = s.GetIdentityByName(ctx, identityUpdated.Type, identityUpdated.Namespace, identityUpdated.Name)
	assert.NoError(t, err)
	identityJson, _ = json.Marshal(&identityUpdated)
	identityReadJson, _ = json.Marshal(&identityRead)
	assert.Equal(t, string(identityJson), string(identityReadJson))

	// Query back the identity
	fb := database.IdentityQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("description", string(identityUpdated.Description)),
		fb.Eq("did", identityUpdated.DID),
	)
	identityRes, res, err := s.GetIdentities(ctx, "ns1", filter.Count(true))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(identityRes))
	assert.Equal(t, int64(1), *res.TotalCount)
	identityReadJson, _ = json.Marshal(identityRes[0])
	assert.Equal(t, string(identityJson), string(identityReadJson))

	s.callbacks.AssertExpectations(t)
}

func TestUpsertIdentityFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertIdentity(context.Background(), &core.Identity{}, database.UpsertOptimizationSkip)
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertIdentityFailSelect(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpsertIdentity(context.Background(), &core.Identity{
		IdentityBase: core.IdentityBase{
			ID: fftypes.NewUUID(),
		},
	}, database.UpsertOptimizationSkip)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertIdentityFailInsert(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{}))
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpsertIdentity(context.Background(), &core.Identity{
		IdentityBase: core.IdentityBase{
			ID: fftypes.NewUUID(),
		},
	}, database.UpsertOptimizationSkip)
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertIdentityFailUpdate(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"identity"}).
		AddRow("id1"))
	mock.ExpectExec("UPDATE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpsertIdentity(context.Background(), &core.Identity{
		IdentityBase: core.IdentityBase{
			ID: fftypes.NewUUID(),
		},
	}, database.UpsertOptimizationSkip)
	assert.Regexp(t, "FF00178", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertIdentityFailCommit(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"identity"}))
	mock.ExpectExec("INSERT .*").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertIdentity(context.Background(), &core.Identity{
		IdentityBase: core.IdentityBase{
			ID: fftypes.NewUUID(),
		},
	}, database.UpsertOptimizationSkip)
	assert.Regexp(t, "FF00180", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetIdentityByIDSelectFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetIdentityByID(context.Background(), "ns1", fftypes.NewUUID())
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetIdentityByNameSelectFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetIdentityByName(context.Background(), core.IdentityTypeOrg, "ff_system", "org1")
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetIdentityByIdentitySelectFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetIdentityByDID(context.Background(), "ns1", "did:firefly:org/org1")
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetIdentityByIDNotFound(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"identity", "identity", "identity"}))
	msg, err := s.GetIdentityByID(context.Background(), "ns1", fftypes.NewUUID())
	assert.NoError(t, err)
	assert.Nil(t, msg)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetIdentityByIDScanFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"identity"}).AddRow("only one"))
	_, err := s.GetIdentityByID(context.Background(), "ns1", fftypes.NewUUID())
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetIdentityQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	f := database.IdentityQueryFactory.NewFilter(context.Background()).Eq("did", "")
	_, _, err := s.GetIdentities(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetIdentityBuildQueryFail(t *testing.T) {
	s, _ := newMockProvider().init()
	f := database.IdentityQueryFactory.NewFilter(context.Background()).Eq("did", map[bool]bool{true: false})
	_, _, err := s.GetIdentities(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00143.*type", err)
}

func TestGetIdentityReadMessageFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"did"}).AddRow("only one"))
	f := database.IdentityQueryFactory.NewFilter(context.Background()).Eq("did", "")
	_, _, err := s.GetIdentities(context.Background(), "ns1", f)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
