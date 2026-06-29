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
	"database/sql"
	"fmt"

	"github.com/DATA-DOG/go-sqlmock"
	sq "github.com/Masterminds/squirrel"
	migratedb "github.com/golang-migrate/migrate/v4/database"
	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/dbsql"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

// testProvider uses the datadog mocking framework
type mockProvider struct {
	SQLCommon
	callbacks    *databasemocks.Callbacks
	capabilities *database.Capabilities
	config       config.Section

	mockDB *sql.DB
	mdb    sqlmock.Sqlmock

	fakePSQLInsert          bool
	openError               error
	getMigrationDriverError error
	individualSort          bool
	multiRowInsert          bool
}

func newMockProvider() *mockProvider {
	coreconfig.Reset()
	conf := config.RootSection("unittest.db")
	conf.AddKnownKey("url", "test")
	mp := &mockProvider{
		capabilities: &database.Capabilities{},
		callbacks:    &databasemocks.Callbacks{},
		config:       conf,
	}
	mp.SQLCommon.InitConfig(mp, mp.config)
	mp.config.Set(SQLConfMaxConnections, 10)
	mp.mockDB, mp.mdb, _ = sqlmock.New()
	mp.multiRowInsert = false
	return mp
}

// init is a convenience to init for tests that aren't testing init itself
func (mp *mockProvider) init() (*mockProvider, sqlmock.Sqlmock) {
	_ = mp.Init(context.Background(), mp, mp.config, mp.capabilities)
	mp.SetHandler(database.GlobalHandler, mp.callbacks)
	return mp, mp.mdb
}

func (mp *mockProvider) Name() string {
	return "mockdb"
}

func (mp *mockProvider) SequenceColumn() string {
	return "seq"
}

func (mp *mockProvider) MigrationsDir() string {
	return mp.Name()
}

func (psql *mockProvider) Features() dbsql.SQLFeatures {
	features := dbsql.DefaultSQLProviderFeatures()
	features.UseILIKE = true
	features.MultiRowInsert = psql.multiRowInsert
	features.AcquireLock = func(lockName string) string {
		return fmt.Sprintf(`<acquire lock %s>`, lockName)
	}
	return features
}

func (mp *mockProvider) ApplyInsertQueryCustomizations(insert sq.InsertBuilder, requestConflictEmptyResult bool) (sq.InsertBuilder, bool) {
	if mp.fakePSQLInsert {
		return insert.Suffix(" RETURNING seq"), true
	}
	return insert, false
}

func (mp *mockProvider) Open(url string) (*sql.DB, error) {
	return mp.mockDB, mp.openError
}

func (mp *mockProvider) GetMigrationDriver(db *sql.DB) (migratedb.Driver, error) {
	return nil, mp.getMigrationDriverError
}
