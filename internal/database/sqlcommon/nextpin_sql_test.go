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
)

func TestNextPinsE2EWithDB(t *testing.T) {
	log.SetLevel("debug")

	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new nextpin entry
	nextpin := &core.NextPin{
		Namespace: "ns",
		Context:   fftypes.NewRandB32(),
		Identity:  "0x12345",
		Hash:      fftypes.NewRandB32(),
		Nonce:     int64(12345),
	}
	err := s.InsertNextPin(ctx, nextpin)
	assert.NoError(t, err)

	// Check we get the exact same nextpin back
	nextpinRead, err := s.GetNextPinsForContext(ctx, "ns", nextpin.Context)
	assert.NoError(t, err)
	assert.Len(t, nextpinRead, 1)
	nextpinJson, _ := json.Marshal(nextpin)
	nextpinReadJson, _ := json.Marshal(nextpinRead[0])
	assert.Equal(t, string(nextpinJson), string(nextpinReadJson))

	var nextpinUpdated core.NextPin
	nextpinUpdated = *nextpin
	nextpinUpdated.Nonce = 1111111
	nextpinUpdated.Hash = fftypes.NewRandB32()
	err = s.UpdateNextPin(context.Background(), "ns", nextpin.Sequence, database.NextPinQueryFactory.NewUpdate(ctx).
		Set("hash", nextpinUpdated.Hash).
		Set("nonce", nextpinUpdated.Nonce),
	)
	nextpinJson, _ = json.Marshal(nextpinUpdated)

	// Check we get the exact same data back
	nextpinRead, err = s.GetNextPinsForContext(ctx, "ns", nextpin.Context)
	assert.NoError(t, err)
	assert.Len(t, nextpinRead, 1)
	nextpinReadJson, _ = json.Marshal(nextpinRead[0])
	assert.Equal(t, string(nextpinJson), string(nextpinReadJson))

	// Check we get the exact same data back querying with filter
	nextpinRead, _, err = s.GetNextPins(ctx, "ns", database.NextPinQueryFactory.NewFilter(ctx).Eq(
		"identity", "0x12345",
	))
	assert.NoError(t, err)
	assert.Len(t, nextpinRead, 1)
	nextpinReadJson, _ = json.Marshal(nextpinRead[0])
	assert.Equal(t, string(nextpinJson), string(nextpinReadJson))

}

func TestUpsertNextPinFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.InsertNextPin(context.Background(), &core.NextPin{})
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertNextPinFailInsert(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.InsertNextPin(context.Background(), &core.NextPin{Context: fftypes.NewRandB32()})
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertNextPinFailCommit(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("INSERT .*").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(fmt.Errorf("pop"))
	err := s.InsertNextPin(context.Background(), &core.NextPin{Context: fftypes.NewRandB32()})
	assert.Regexp(t, "FF00180", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetNextPinsForContextQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetNextPinsForContext(context.Background(), "ns", fftypes.NewRandB32())
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetNextPinsForContextReadMessageFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"context"}).AddRow("only one"))
	_, err := s.GetNextPinsForContext(context.Background(), "ns", fftypes.NewRandB32())
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetNextPinsBadFilter(t *testing.T) {
	s, mock := newMockProvider().init()
	_, _, err := s.GetNextPins(context.Background(), "ns", database.NextPinQueryFactory.NewFilter(context.Background()).Eq(
		"wrong", "bad",
	))
	assert.Regexp(t, "FF00142", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetNextPinQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, _, err := s.GetNextPins(context.Background(), "ns", database.NextPinQueryFactory.NewFilter(context.Background()).Eq(
		"identity", "0x12345",
	))
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetNextPinReadMessageFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"context"}).AddRow("only one"))
	_, _, err := s.GetNextPins(context.Background(), "ns", database.NextPinQueryFactory.NewFilter(context.Background()).Eq(
		"identity", "0x12345",
	))
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestNextPinUpdateBeginFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	u := database.NextPinQueryFactory.NewUpdate(context.Background()).Set("context", "anything")
	err := s.UpdateNextPin(context.Background(), "ns", 12345, u)
	assert.Regexp(t, "FF00175", err)
}

func TestNextPinUpdateBuildQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	u := database.NextPinQueryFactory.NewUpdate(context.Background()).Set("context", map[bool]bool{true: false})
	err := s.UpdateNextPin(context.Background(), "ns", 12345, u)
	assert.Regexp(t, "FF00143.*context", err)
}

func TestNextPinUpdateFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	u := database.NextPinQueryFactory.NewUpdate(context.Background()).Set("context", fftypes.NewRandB32())
	err := s.UpdateNextPin(context.Background(), "ns", 12345, u)
	assert.Regexp(t, "FF00178", err)
}
