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

func TestDataE2EWithDB(t *testing.T) {
	log.SetLevel("trace")

	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new data entry
	dataID := fftypes.NewUUID()
	val := fftypes.JSONObject{
		"some": "data",
		"with": map[string]interface{}{
			"nesting": 12345,
		},
	}
	data := &core.Data{
		ID:        dataID,
		Validator: core.ValidatorTypeSystemDefinition,
		Namespace: "ns1",
		Hash:      fftypes.NewRandB32(),
		Created:   fftypes.Now(),
		Value:     fftypes.JSONAnyPtr(val.String()),
		Public:    "some IPFS ref",
		ValueSize: 12345,
	}

	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionData, core.ChangeEventTypeCreated, "ns1", dataID, mock.Anything).Return()
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionData, core.ChangeEventTypeUpdated, "ns1", dataID, mock.Anything).Return()

	err := s.UpsertData(ctx, data, database.UpsertOptimizationSkip)
	assert.NoError(t, err)

	// Check we get the exact same data back - we should not to return the value first
	dataRead, err := s.GetDataByID(ctx, "ns1", dataID, false)
	assert.NoError(t, err)
	assert.Equal(t, *dataID, *dataRead.ID)
	assert.Nil(t, dataRead.Value)

	// Now with value
	dataRead, err = s.GetDataByID(ctx, "ns1", dataID, true)
	assert.NotNil(t, dataRead)
	dataJson, _ := json.Marshal(&data)
	dataReadJson, _ := json.Marshal(&dataRead)
	assert.Equal(t, string(dataJson), string(dataReadJson))
	assert.Equal(t, int64(12345), dataRead.ValueSize)

	// Update the data (this is testing what's possible at the database layer,
	// and does not account for the verification that happens at the higher level)
	val2 := fftypes.JSONObject{
		"another": "set",
		"of": map[string]interface{}{
			"data": 12345,
			"and":  "stuff",
		},
	}
	dataUpdated := &core.Data{
		ID:        dataID,
		Validator: core.ValidatorTypeJSON,
		Namespace: "ns1",
		Datatype: &core.DatatypeRef{
			Name:    "customer",
			Version: "0.0.1",
		},
		Hash:    fftypes.NewRandB32(),
		Created: fftypes.Now(),
		Value:   fftypes.JSONAnyPtr(val2.String()),
		Blob: &core.BlobRef{
			Hash:   fftypes.NewRandB32(),
			Public: "Qmf412jQZiuVUtdgnB36FXFX7xg5V6KEbSJ4dpQuhkLyfD",
			Name:   "path/to/myfile.ext",
			Size:   12345,
		},
	}

	// Check disallows hash update, regardless of optimization
	err = s.UpsertData(context.Background(), dataUpdated, database.UpsertOptimizationNew)
	assert.Equal(t, database.HashMismatch, err)
	err = s.UpsertData(context.Background(), dataUpdated, database.UpsertOptimizationExisting)
	assert.Equal(t, database.HashMismatch, err)
	assert.Equal(t, "/path/to", dataUpdated.Blob.Path)

	dataUpdated.Hash = data.Hash
	err = s.UpsertData(context.Background(), dataUpdated, database.UpsertOptimizationSkip)
	assert.NoError(t, err)

	// Check we get the exact same message back - note the removal of one of the data elements
	dataRead, err = s.GetDataByID(ctx, "ns1", dataID, true)
	assert.NoError(t, err)
	dataJson, _ = json.Marshal(&dataUpdated)
	dataReadJson, _ = json.Marshal(&dataRead)
	assert.Equal(t, string(dataJson), string(dataReadJson))

	valRestored, ok := dataRead.Value.JSONObjectOk()
	assert.True(t, ok)
	assert.Equal(t, "stuff", valRestored.GetObject("of").GetString("and"))

	// Query back the data
	fb := database.DataQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("id", dataUpdated.ID.String()),
		fb.Eq("validator", string(dataUpdated.Validator)),
		fb.Eq("datatype.name", dataUpdated.Datatype.Name),
		fb.Eq("datatype.version", dataUpdated.Datatype.Version),
		fb.Eq("hash", dataUpdated.Hash),
		fb.Gt("created", 0),
	)
	dataRes, _, err := s.GetData(ctx, "ns1", filter)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(dataRes))
	dataReadJson, _ = json.Marshal(dataRes[0])
	assert.Equal(t, string(dataJson), string(dataReadJson))

	dataRefRes, _, err := s.GetDataRefs(ctx, "ns1", filter)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(dataRefRes))
	assert.Equal(t, *dataUpdated.ID, *dataRefRes[0].ID)
	assert.Equal(t, dataUpdated.Hash, dataRefRes[0].Hash)

	// Update
	v2 := "2.0.0"
	up := database.DataQueryFactory.NewUpdate(ctx).Set("datatype.version", v2)
	err = s.UpdateData(ctx, "ns1", dataID, up)
	assert.NoError(t, err)

	// Test find updated value
	filter = fb.And(
		fb.Eq("id", dataUpdated.ID.String()),
		fb.Eq("datatype.version", v2),
	)
	dataRes, res, err := s.GetData(ctx, "ns1", filter.Count(true))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(dataRes))
	assert.Equal(t, int64(1), *res.TotalCount)

	s.callbacks.AssertExpectations(t)

	// Delete
	err = s.DeleteData(ctx, "ns1", dataID)
	assert.NoError(t, err)
	dataRes, res, err = s.GetData(ctx, "ns1", filter.Count(true))
	assert.NoError(t, err)
	assert.Len(t, dataRes, 0)
}

func TestDataSubPaths(t *testing.T) {
	log.SetLevel("trace")

	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	newData := func(blobName string) *core.Data {
		return &core.Data{
			ID:        fftypes.NewUUID(),
			Namespace: "ns1",
			Hash:      fftypes.NewRandB32(),
			Created:   fftypes.Now(),
			Blob: &core.BlobRef{
				Name: blobName,
				Hash: fftypes.NewRandB32(),
			},
		}
	}
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionData, core.ChangeEventTypeCreated, "ns1", mock.Anything, mock.Anything).Return()

	err := s.UpsertData(ctx, newData("dir1/file1.txt"), database.UpsertOptimizationSkip)
	assert.NoError(t, err)
	err = s.UpsertData(ctx, newData("/dir1/dir2/file2.txt"), database.UpsertOptimizationSkip)
	assert.NoError(t, err)
	err = s.UpsertData(ctx, newData("/dir1/dir2/file3.txt"), database.UpsertOptimizationSkip)
	assert.NoError(t, err)
	err = s.UpsertData(ctx, newData("dir1/dir3/file4.txt"), database.UpsertOptimizationSkip)
	assert.NoError(t, err)
	err = s.UpsertData(ctx, newData("dir2/dir3/file5.txt"), database.UpsertOptimizationSkip)
	assert.NoError(t, err)
	err = s.UpsertData(ctx, newData("dir2/dir3/dir4/file6.txt"), database.UpsertOptimizationSkip)
	assert.NoError(t, err)
	err = s.UpsertData(ctx, newData("dir2/file7.txt"), database.UpsertOptimizationSkip)
	assert.NoError(t, err)

	subPaths, err := s.GetDataSubPaths(ctx, "ns1", "")
	assert.NoError(t, err)
	assert.Equal(t, []string{
		"/dir1",
		"/dir2",
	}, subPaths)

	subPaths2, err := s.GetDataSubPaths(ctx, "ns1", "/")
	assert.NoError(t, err)
	assert.Equal(t, subPaths, subPaths2)

	subPaths, err = s.GetDataSubPaths(ctx, "ns1", "dir1")
	assert.NoError(t, err)
	assert.Equal(t, []string{
		"/dir1/dir2",
		"/dir1/dir3",
	}, subPaths)

	subPaths2, err = s.GetDataSubPaths(ctx, "ns1", "/dir1")
	assert.NoError(t, err)
	assert.Equal(t, subPaths, subPaths2)

}

func TestUpsertDataFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertData(context.Background(), &core.Data{}, database.UpsertOptimizationSkip)
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertDataFailSelect(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	dataID := fftypes.NewUUID()
	err := s.UpsertData(context.Background(), &core.Data{ID: dataID}, database.UpsertOptimizationSkip)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertDataFailInsert(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{}))
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	dataID := fftypes.NewUUID()
	err := s.UpsertData(context.Background(), &core.Data{ID: dataID}, database.UpsertOptimizationSkip)
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertDataFailUpdate(t *testing.T) {
	s, mock := newMockProvider().init()
	dataID := fftypes.NewUUID()
	dataHash := fftypes.NewRandB32()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"hash"}).AddRow(dataHash.String()))
	mock.ExpectExec("UPDATE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpsertData(context.Background(), &core.Data{ID: dataID, Hash: dataHash}, database.UpsertOptimizationSkip)
	assert.Regexp(t, "FF00178", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertDataFailCommit(t *testing.T) {
	s, mock := newMockProvider().init()
	dataID := fftypes.NewUUID()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	mock.ExpectExec("INSERT .*").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertData(context.Background(), &core.Data{ID: dataID}, database.UpsertOptimizationSkip)
	assert.Regexp(t, "FF00180", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestInsertDataArrayBeginFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.InsertDataArray(context.Background(), core.DataArray{})
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

func TestInsertDataArrayMultiRowOK(t *testing.T) {
	s := newMockProvider()
	s.multiRowInsert = true
	s.fakePSQLInsert = true
	s, mock := s.init()

	data1 := &core.Data{ID: fftypes.NewUUID(), Namespace: "ns1"}
	data2 := &core.Data{ID: fftypes.NewUUID(), Namespace: "ns1"}
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionData, core.ChangeEventTypeCreated, "ns1", data1.ID)
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionData, core.ChangeEventTypeCreated, "ns1", data2.ID)

	mock.ExpectBegin()
	mock.ExpectQuery("INSERT.*").WillReturnRows(sqlmock.NewRows([]string{s.SequenceColumn()}).
		AddRow(int64(1001)).
		AddRow(int64(1002)),
	)
	mock.ExpectCommit()
	err := s.InsertDataArray(context.Background(), core.DataArray{data1, data2})
	assert.NoError(t, err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

func TestInsertDataArrayMultiRowFail(t *testing.T) {
	s := newMockProvider()
	s.multiRowInsert = true
	s.fakePSQLInsert = true
	s, mock := s.init()
	data1 := &core.Data{ID: fftypes.NewUUID(), Namespace: "ns1"}
	mock.ExpectBegin()
	mock.ExpectQuery("INSERT.*").WillReturnError(fmt.Errorf("pop"))
	err := s.InsertDataArray(context.Background(), core.DataArray{data1})
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

func TestInsertDataArraySingleRowFail(t *testing.T) {
	s, mock := newMockProvider().init()
	data1 := &core.Data{ID: fftypes.NewUUID(), Namespace: "ns1"}
	mock.ExpectBegin()
	mock.ExpectExec("INSERT.*").WillReturnError(fmt.Errorf("pop"))
	err := s.InsertDataArray(context.Background(), core.DataArray{data1})
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
	s.callbacks.AssertExpectations(t)
}

func TestGetDataByIDSelectFail(t *testing.T) {
	s, mock := newMockProvider().init()
	dataID := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetDataByID(context.Background(), "ns1", dataID, false)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetDataByIDNotFound(t *testing.T) {
	s, mock := newMockProvider().init()
	dataID := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}))
	msg, err := s.GetDataByID(context.Background(), "ns1", dataID, true)
	assert.NoError(t, err)
	assert.Nil(t, msg)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetDataByIDScanFail(t *testing.T) {
	s, mock := newMockProvider().init()
	dataID := fftypes.NewUUID()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("only one"))
	_, err := s.GetDataByID(context.Background(), "ns1", dataID, true)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetDataQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	f := database.DataQueryFactory.NewFilter(context.Background()).Eq("id", "")
	_, _, err := s.GetData(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetDataBuildQueryFail(t *testing.T) {
	s, _ := newMockProvider().init()
	f := database.DataQueryFactory.NewFilter(context.Background()).Eq("id", map[bool]bool{true: false})
	_, _, err := s.GetData(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00143.*id", err)
}

func TestGetDataReadMessageFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("only one"))
	f := database.DataQueryFactory.NewFilter(context.Background()).Eq("id", "")
	_, _, err := s.GetData(context.Background(), "ns1", f)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetDataRefsQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	f := database.DataQueryFactory.NewFilter(context.Background()).Eq("id", "")
	_, _, err := s.GetDataRefs(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetDataRefsBuildQueryFail(t *testing.T) {
	s, _ := newMockProvider().init()
	f := database.DataQueryFactory.NewFilter(context.Background()).Eq("id", map[bool]bool{true: false})
	_, _, err := s.GetDataRefs(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00143.*id", err)
}

func TestGetDataRefsReadMessageFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"id"}).AddRow("only one"))
	f := database.DataQueryFactory.NewFilter(context.Background()).Eq("id", "")
	_, _, err := s.GetDataRefs(context.Background(), "ns1", f)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDataUpdateBeginFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	u := database.DataQueryFactory.NewUpdate(context.Background()).Set("id", "anything")
	err := s.UpdateData(context.Background(), "ns1", fftypes.NewUUID(), u)
	assert.Regexp(t, "FF00175", err)
}

func TestDataUpdateBuildQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	u := database.DataQueryFactory.NewUpdate(context.Background()).Set("id", map[bool]bool{true: false})
	err := s.UpdateData(context.Background(), "ns1", fftypes.NewUUID(), u)
	assert.Regexp(t, "FF00143.*id", err)
}

func TestDataUpdateFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("UPDATE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	u := database.DataQueryFactory.NewUpdate(context.Background()).Set("id", fftypes.NewUUID())
	err := s.UpdateData(context.Background(), "ns1", fftypes.NewUUID(), u)
	assert.Regexp(t, "FF00178", err)
}

func TestDeleteDataFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.DeleteData(context.Background(), "ns1", fftypes.NewUUID())
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestDataDeleteFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectExec("DELETE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.DeleteData(context.Background(), "ns1", fftypes.NewUUID())
	assert.Regexp(t, "FF00179", err)
}

func TestGetDataSubPathsSelectFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetDataSubPaths(context.Background(), "ns1", "/any/path")
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetDataSubPathsReadFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{}).AddRow())
	_, err := s.GetDataSubPaths(context.Background(), "ns1", "/any/path")
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}
