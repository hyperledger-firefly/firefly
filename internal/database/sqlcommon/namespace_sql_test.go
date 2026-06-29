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
	"github.com/stretchr/testify/assert"
)

func TestNamespacesE2EWithDB(t *testing.T) {
	log.SetLevel("debug")

	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new namespace entry
	namespace := &core.Namespace{
		Name:        "namespace1",
		NetworkName: "default",
		Created:     fftypes.Now(),
		Contracts: &core.MultipartyContracts{
			Active: &core.MultipartyContract{
				Index: 1,
			},
		},
	}

	err := s.UpsertNamespace(ctx, namespace, true)
	assert.NoError(t, err)

	// Check we get the exact same namespace back
	namespaceRead, err := s.GetNamespace(ctx, namespace.Name)
	assert.NoError(t, err)
	assert.NotNil(t, namespaceRead)
	namespaceJson, _ := json.Marshal(&namespace)
	namespaceReadJson, _ := json.Marshal(&namespaceRead)
	assert.Equal(t, string(namespaceJson), string(namespaceReadJson))

	// Update the namespace (this is testing what's possible at the database layer,
	// and does not account for the verification that happens at the higher level)
	namespaceUpdated := &core.Namespace{
		Name:        "namespace1",
		Description: "description1",
		Created:     fftypes.Now(),
	}
	err = s.UpsertNamespace(context.Background(), namespaceUpdated, true)
	assert.NoError(t, err)

	// Check we get the exact same data back - note the removal of one of the namespace elements
	namespaceRead, err = s.GetNamespace(ctx, namespace.Name)
	assert.NoError(t, err)
	namespaceJson, _ = json.Marshal(&namespaceUpdated)
	namespaceReadJson, _ = json.Marshal(&namespaceRead)
	assert.Equal(t, string(namespaceJson), string(namespaceReadJson))

	s.callbacks.AssertExpectations(t)
}

func TestUpsertNamespaceFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertNamespace(context.Background(), &core.Namespace{}, true)
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertNamespaceFailSelect(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpsertNamespace(context.Background(), &core.Namespace{Name: "name1"}, true)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertNamespaceFailInsert(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{}))
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpsertNamespace(context.Background(), &core.Namespace{Name: "name1"}, true)
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertNamespaceFailUpdate(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"name"}).
		AddRow("name1"))
	mock.ExpectExec("UPDATE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpsertNamespace(context.Background(), &core.Namespace{Name: "name1"}, true)
	assert.Regexp(t, "FF00178", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertNamespaceFailCommit(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"name"}))
	mock.ExpectExec("INSERT .*").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertNamespace(context.Background(), &core.Namespace{Name: "name1"}, true)
	assert.Regexp(t, "FF00180", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetNamespaceByIDSelectFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetNamespace(context.Background(), "name1")
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetNamespaceByNameNotFound(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"ntype", "namespace", "name"}))
	msg, err := s.GetNamespace(context.Background(), "name1")
	assert.NoError(t, err)
	assert.Nil(t, msg)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetNamespaceByNameScanFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"ntype"}).AddRow("only one"))
	_, err := s.GetNamespace(context.Background(), "name1")
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
