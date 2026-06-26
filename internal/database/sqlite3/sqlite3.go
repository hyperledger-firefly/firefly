// Copyright © 2022 Kaleido, Inc.
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

//go:build cgo

package sqlite3

import (
	"context"

	"database/sql"

	sq "github.com/Masterminds/squirrel"
	migratedb "github.com/golang-migrate/migrate/v4/database"
	migratesqlite3 "github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-common/pkg/dbsql"
	"github.com/hyperledger/firefly/internal/database/sqlcommon"
	"github.com/hyperledger/firefly/pkg/database"

	// Import the derivation of SQLite3 CGO suported by golang-migrate
	"github.com/mattn/go-sqlite3"
)

var ffSQLiteRegistered = false

type SQLite3 struct {
	sqlcommon.SQLCommon
}

func connHook(conn *sqlite3.SQLiteConn) error {
	_, err := conn.Exec(`
		PRAGMA case_sensitive_like=ON;
		PRAGMA busy_timeout=1000;
	`, nil)
	return err
}

func (sqlite *SQLite3) Init(ctx context.Context, config config.Section) error {
	capabilities := &database.Capabilities{}
	if !ffSQLiteRegistered {
		sql.Register("sqlite3_ff",
			&sqlite3.SQLiteDriver{
				ConnectHook: connHook,
			})
		ffSQLiteRegistered = true
	}
	return sqlite.SQLCommon.Init(ctx, sqlite, config, capabilities)
}

func (sqlite *SQLite3) SetHandler(namespace string, handler database.Callbacks) {
	sqlite.SQLCommon.SetHandler(namespace, handler)
}

func (sqlite *SQLite3) Name() string {
	return "sqlite3"
}

func (sqlite *SQLite3) MigrationsDir() string {
	return "sqlite"
}

func (sqlite *SQLite3) SequenceColumn() string {
	return "seq"
}

func (sqlite *SQLite3) Features() dbsql.SQLFeatures {
	features := dbsql.DefaultSQLProviderFeatures()
	features.PlaceholderFormat = sq.Dollar
	features.UseILIKE = false // Not supported
	return features
}

func (sqlite *SQLite3) ApplyInsertQueryCustomizations(insert sq.InsertBuilder, requestConflictEmptyResult bool) (sq.InsertBuilder, bool) {
	return insert, false
}

func (sqlite *SQLite3) Open(url string) (*sql.DB, error) {
	return sql.Open("sqlite3_ff", url)
}

func (sqlite *SQLite3) GetMigrationDriver(db *sql.DB) (migratedb.Driver, error) {
	return migratesqlite3.WithInstance(db, &migratesqlite3.Config{})
}
