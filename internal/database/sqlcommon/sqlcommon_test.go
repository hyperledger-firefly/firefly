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
	"fmt"
	"testing"

	sq "github.com/Masterminds/squirrel"
	"github.com/golang-migrate/migrate/v4"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/stretchr/testify/assert"
)

func TestMigrationUpDown(t *testing.T) {
	tp, cleanup := newSQLiteTestProvider(t)
	defer cleanup()

	driver, err := tp.GetMigrationDriver(tp.DB())
	assert.NoError(t, err)
	assert.NotNil(t, tp.Capabilities())
	var m *migrate.Migrate
	m, err = migrate.NewWithDatabaseInstance(
		"file://../../../db/migrations/sqlite",
		tp.MigrationsDir(), driver)
	assert.NoError(t, err)
	err = m.Down()
	assert.NoError(t, err)
}

func TestTXConcurrency(t *testing.T) {
	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()

	// This test exercise our transaction begin/use/end code for parallel execution.
	// It was originally written to validate the pure Go implementation of SQLite:
	// https://gitlab.com/cznic/sqlite
	// Sadly we found problems with that implementation, and are now only using
	// the well adopted CGO implementation.
	// When the e2e DB tests move to being able to be run against any database, this
	// test should be included.
	// (additional refactor required - see https://github.com/hyperledger-firefly/firefly/issues/119)

	_, err := s.DB().Exec(`
		CREATE TABLE testconc ( seq INTEGER PRIMARY KEY AUTOINCREMENT, val VARCHAR(256) );
	`)
	assert.NoError(t, err)

	racer := func(done chan struct{}, name string) func() {
		return func() {
			defer close(done)
			for i := 0; i < 5; i++ {
				ctx, tx, ac, err := s.BeginOrUseTx(context.Background())
				assert.NoError(t, err)
				val := fmt.Sprintf("%s/%d", name, i)
				sequence, err := s.InsertTx(ctx, "table1", tx, sq.Insert("testconc").Columns("val").Values(val), nil)
				assert.NoError(t, err)
				t.Logf("%s = %d", val, sequence)
				err = s.CommitTx(ctx, tx, ac)
				assert.NoError(t, err)
			}
		}
	}
	flags := make([]chan struct{}, 5)
	for i := 0; i < len(flags); i++ {
		flags[i] = make(chan struct{})
		go racer(flags[i], fmt.Sprintf("racer_%d", i))()
	}
	for i := 0; i < len(flags); i++ {
		<-flags[i]
		t.Logf("Racer %d complete", i)
	}
}

func TestNamespaceCallbacks(t *testing.T) {
	tcb := &databasemocks.Callbacks{}
	s := &SQLCommon{
		callbacks: callbacks{
			handlers: map[string]database.Callbacks{},
		},
	}
	s.SetHandler("ns1", tcb)
	id := fftypes.NewUUID()
	hash := fftypes.NewRandB32()

	tcb.On("OrderedUUIDCollectionNSEvent", database.CollectionMessages, core.ChangeEventTypeCreated, "ns1", id, int64(1)).Return()
	tcb.On("OrderedCollectionNSEvent", database.CollectionPins, core.ChangeEventTypeCreated, "ns1", int64(1)).Return()
	tcb.On("UUIDCollectionNSEvent", database.CollectionOperations, core.ChangeEventTypeCreated, "ns1", id).Return()
	tcb.On("HashCollectionNSEvent", database.CollectionGroups, core.ChangeEventTypeUpdated, "ns1", hash).Return()

	s.callbacks.OrderedUUIDCollectionNSEvent(database.CollectionMessages, core.ChangeEventTypeCreated, "ns1", id, 1)
	s.callbacks.OrderedCollectionNSEvent(database.CollectionPins, core.ChangeEventTypeCreated, "ns1", 1)
	s.callbacks.UUIDCollectionNSEvent(database.CollectionOperations, core.ChangeEventTypeCreated, "ns1", id)
	s.callbacks.HashCollectionNSEvent(database.CollectionGroups, core.ChangeEventTypeUpdated, "ns1", hash)

	s.SetHandler("ns1", nil)
	assert.Empty(t, s.callbacks.handlers)
}
