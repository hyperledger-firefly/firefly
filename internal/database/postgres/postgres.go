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

package postgres

import (
	"context"
	"fmt"
	"math/big"

	"database/sql"

	sq "github.com/Masterminds/squirrel"
	migratedb "github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/dbsql"
	"github.com/hyperledger-firefly/firefly/internal/database/sqlcommon"
	"github.com/hyperledger-firefly/firefly/pkg/database"

	// Import pq driver
	_ "github.com/lib/pq"
)

type Postgres struct {
	sqlcommon.SQLCommon
}

func (psql *Postgres) Init(ctx context.Context, config config.Section) error {
	capabilities := &database.Capabilities{}
	if config.GetInt(dbsql.SQLConfMaxConnections) > 1 {
		capabilities.Concurrency = true
	}
	return psql.SQLCommon.Init(ctx, psql, config, capabilities)
}

func (psql *Postgres) SetHandler(namespace string, handler database.Callbacks) {
	psql.SQLCommon.SetHandler(namespace, handler)
}

func (psql *Postgres) Name() string {
	return "postgres"
}

func (psql *Postgres) SequenceColumn() string {
	return "seq"
}

func (psql *Postgres) MigrationsDir() string {
	return psql.Name()
}

// Attempt to create a unique 64-bit int from the given name, by selecting 4 bytes from the
// beginning and end of the string.
func lockIndex(lockName string) int64 {
	if len(lockName) >= 4 {
		lockName = lockName[0:4] + lockName[len(lockName)-4:]
	}
	return big.NewInt(0).SetBytes([]byte(lockName)).Int64()
}

func (psql *Postgres) Features() dbsql.SQLFeatures {
	features := dbsql.DefaultSQLProviderFeatures()
	features.PlaceholderFormat = sq.Dollar
	features.UseILIKE = false // slower than lower()
	features.AcquireLock = func(lockName string) string {
		return fmt.Sprintf(`SELECT pg_advisory_xact_lock(%d);`, lockIndex(lockName))
	}
	features.MultiRowInsert = true
	return features
}

func (psql *Postgres) ApplyInsertQueryCustomizations(insert sq.InsertBuilder, requestConflictEmptyResult bool) (sq.InsertBuilder, bool) {
	suffix := " RETURNING seq"
	if requestConflictEmptyResult {
		// Caller wants us to return an empty result set on insert conflict, rather than an error
		suffix = fmt.Sprintf(" ON CONFLICT DO NOTHING%s", suffix)
	}
	return insert.Suffix(suffix), true
}

func (psql *Postgres) Open(url string) (*sql.DB, error) {
	return sql.Open(psql.Name(), url)
}

func (psql *Postgres) GetMigrationDriver(db *sql.DB) (migratedb.Driver, error) {
	return postgres.WithInstance(db, &postgres.Config{})
}
