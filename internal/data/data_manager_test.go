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

package data

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/internal/cache"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/mocks/cachemocks"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/mocks/dataexchangemocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestCacheInitFail(t *testing.T) {
	cacheInitError := errors.New("Initialization error.")
	coreconfig.Reset()
	config.Set(coreconfig.MessageWriterCount, 1)
	ctx := context.Background()
	mdi := &databasemocks.Plugin{}
	mdi.On("Capabilities").Return(&database.Capabilities{
		Concurrency: true,
	})
	mdx := &dataexchangemocks.Plugin{}
	ns := &core.Namespace{Name: "ns1", NetworkName: "ns1"}

	vErrcmi := &cachemocks.Manager{}
	vErrcmi.On("GetCache", cache.NewCacheConfig(
		ctx,
		coreconfig.CacheValidatorSize,
		coreconfig.CacheValidatorTTL,
		ns.Name,
	)).Return(nil, cacheInitError).Once()
	defer vErrcmi.AssertExpectations(t)
	_, err := NewDataManager(ctx, ns, mdi, mdx, vErrcmi)
	assert.Equal(t, cacheInitError, err)

	mErrcmi := &cachemocks.Manager{}
	mErrcmi.On("GetCache", cache.NewCacheConfig(
		ctx,
		coreconfig.CacheValidatorSize,
		coreconfig.CacheValidatorTTL,
		ns.Name,
	)).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil).Once()
	mErrcmi.On("GetCache", cache.NewCacheConfig(
		ctx,
		coreconfig.CacheMessageSize,
		coreconfig.CacheMessageTTL,
		ns.Name,
	)).Return(nil, cacheInitError).Once()
	defer mErrcmi.AssertExpectations(t)
	_, err = NewDataManager(ctx, ns, mdi, mdx, mErrcmi)
	assert.Equal(t, cacheInitError, err)
}

func newTestDataManager(t *testing.T) (*dataManager, context.Context, func()) {
	coreconfig.Reset()
	config.Set(coreconfig.MessageWriterCount, 1)
	ctx, cancel := context.WithCancel(context.Background())
	mdi := &databasemocks.Plugin{}
	mdi.On("Capabilities").Return(&database.Capabilities{
		Concurrency: true,
	})
	mdx := &dataexchangemocks.Plugin{}
	ns := &core.Namespace{Name: "ns1", NetworkName: "ns1"}

	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 10000, 5*time.Minute), nil)
	dm, err := NewDataManager(ctx, ns, mdi, mdx, cmi)
	cmi.AssertCalled(t, "GetCache", cache.NewCacheConfig(
		ctx,
		coreconfig.CacheMessageSize,
		coreconfig.CacheMessageTTL,
		ns.Name,
	))
	cmi.AssertCalled(t, "GetCache", cache.NewCacheConfig(
		ctx,
		coreconfig.CacheValidatorSize,
		coreconfig.CacheValidatorTTL,
		ns.Name,
	))
	assert.NoError(t, err)
	assert.True(t, dm.BlobsEnabled())
	dm.Start()
	return dm.(*dataManager), ctx, func() {
		cancel()
		dm.WaitStop()
	}
}

func testNewMessage() (*fftypes.UUID, *fftypes.Bytes32, *NewMessage) {
	dataID := fftypes.NewUUID()
	dataHash := fftypes.NewRandB32()
	return dataID, dataHash, &NewMessage{
		Message: &core.MessageInOut{
			Message: core.Message{
				Header: core.MessageHeader{
					ID:        fftypes.NewUUID(),
					Namespace: "ns1",
				},
			},
			InlineData: core.InlineData{
				{DataRef: core.DataRef{ID: dataID, Hash: dataHash}},
			},
		},
	}
}

func TestValidateE2E(t *testing.T) {

	coreconfig.Reset()
	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdi := dm.database.(*databasemocks.Plugin)
	data := &core.Data{
		Namespace: "ns1",
		Validator: core.ValidatorTypeJSON,
		Datatype: &core.DatatypeRef{
			Name:    "customer",
			Version: "0.0.1",
		},
		Value: fftypes.JSONAnyPtr(`{"some":"json"}`),
	}
	data.Seal(ctx, nil)
	dt := &core.Datatype{
		ID:        fftypes.NewUUID(),
		Validator: core.ValidatorTypeJSON,
		Value: fftypes.JSONAnyPtr(`{
			"properties": {
				"field1": {
					"type": "string"
				}
			},
			"additionalProperties": false
		}`),
		Namespace: "ns1",
		Name:      "customer",
		Version:   "0.0.1",
	}
	mdi.On("GetDatatypeByName", mock.Anything, "ns1", "customer", "0.0.1").Return(dt, nil)
	isValid, err := dm.ValidateAll(ctx, core.DataArray{data})
	assert.Regexp(t, "FF10198", err)
	assert.False(t, isValid)

	v, err := dm.getValidatorForDatatype(ctx, data.Validator, data.Datatype)
	err = v.Validate(ctx, data)
	assert.Regexp(t, "FF10198", err)

	data.Value = fftypes.JSONAnyPtr(`{"field1":"value1"}`)
	data.Seal(context.Background(), nil)
	err = v.Validate(ctx, data)
	assert.NoError(t, err)

	isValid, err = dm.ValidateAll(ctx, core.DataArray{data})
	assert.NoError(t, err)
	assert.True(t, isValid)

}

func TestWriteNewMessageE2E(t *testing.T) {

	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()

	dt := &core.Datatype{
		ID:        fftypes.NewUUID(),
		Validator: core.ValidatorTypeJSON,
		Value:     fftypes.JSONAnyPtr(`{}`),
		Name:      "customer",
		Namespace: "0.0.1",
	}

	mdi := dm.database.(*databasemocks.Plugin)
	mdi.On("GetDatatypeByName", mock.Anything, "ns1", "customer", "0.0.1").Return(dt, nil).Once()
	mdi.On("RunAsGroup", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		err := args[1].(func(context.Context) error)(ctx)
		assert.NoError(t, err)
	}).Return(nil)
	mdi.On("InsertDataArray", mock.Anything, mock.Anything).Return(nil).Once()

	data1, err := dm.UploadJSON(ctx, &core.DataRefOrValue{
		Value:     fftypes.JSONAnyPtr(`"message 1 - data A"`),
		Validator: core.ValidatorTypeJSON,
		Datatype: &core.DatatypeRef{
			Name:    "customer",
			Version: "0.0.1",
		},
	})
	assert.NoError(t, err)

	mdi.On("GetDataByID", mock.Anything, "ns1", data1.ID, true).Return(data1, nil).Once()

	_, _, newMsg1 := testNewMessage()
	newMsg1.Message.InlineData = core.InlineData{
		{DataRef: core.DataRef{
			ID:   data1.ID,
			Hash: data1.Hash,
		}},
		{Value: fftypes.JSONAnyPtr(`"message 1 - data B"`)},
		{Value: fftypes.JSONAnyPtr(`"message 1 - data C"`)},
	}
	_, _, newMsg2 := testNewMessage()
	newMsg2.Message.InlineData = core.InlineData{
		{Value: fftypes.JSONAnyPtr(`"message 2 - data B"`)},
		{Value: fftypes.JSONAnyPtr(`"message 2 - data C"`)},
	}

	err = dm.ResolveInlineData(ctx, newMsg1)
	assert.NoError(t, err)
	err = dm.ResolveInlineData(ctx, newMsg2)
	assert.NoError(t, err)

	allData := append(append(core.DataArray{}, newMsg1.NewData...), newMsg2.NewData...)
	assert.Len(t, allData, 4)

	mdi.On("InsertMessages", mock.Anything, mock.MatchedBy(func(msgs []*core.Message) bool {
		msgsByID := make(map[fftypes.UUID]bool)
		for _, msg := range msgs {
			msgsByID[*msg.Header.ID] = true
		}
		return len(msgs) == 2 &&
			msgsByID[*newMsg1.Message.Header.ID] &&
			msgsByID[*newMsg2.Message.Header.ID]
	})).Return(nil).Once()
	mdi.On("InsertDataArray", mock.Anything, mock.MatchedBy(func(dataArray core.DataArray) bool {
		dataByID := make(map[fftypes.UUID]bool)
		for _, data := range dataArray {
			dataByID[*data.ID] = true
		}
		return len(dataArray) == 4 &&
			dataByID[*newMsg1.AllData[1].ID] &&
			dataByID[*newMsg1.AllData[2].ID] &&
			dataByID[*newMsg2.AllData[0].ID] &&
			dataByID[*newMsg2.AllData[1].ID]
	})).Return(nil).Once()

	results := make(chan error)

	go func() {
		results <- dm.WriteNewMessage(ctx, newMsg1)
	}()
	go func() {
		results <- dm.WriteNewMessage(ctx, newMsg2)
	}()

	assert.NoError(t, <-results)
	assert.NoError(t, <-results)

	mdi.AssertExpectations(t)
}

func TestInitBadDeps(t *testing.T) {
	_, err := NewDataManager(context.Background(), &core.Namespace{}, nil, nil, nil)
	assert.Regexp(t, "FF10128", err)
}

func TestValidatorLookupCached(t *testing.T) {
	coreconfig.Reset()
	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdi := dm.database.(*databasemocks.Plugin)
	ref := &core.DatatypeRef{
		Name:    "customer",
		Version: "0.0.1",
	}
	dt := &core.Datatype{
		ID:        fftypes.NewUUID(),
		Validator: core.ValidatorTypeJSON,
		Value:     fftypes.JSONAnyPtr(`{}`),
		Name:      "customer",
		Namespace: "0.0.1",
	}
	mdi.On("GetDatatypeByName", mock.Anything, "ns1", "customer", "0.0.1").Return(dt, nil).Once()
	lookup1, err := dm.getValidatorForDatatype(ctx, core.ValidatorTypeJSON, ref)
	assert.NoError(t, err)
	assert.Equal(t, "customer", lookup1.(*jsonValidator).datatype.Name)

	lookup2, err := dm.getValidatorForDatatype(ctx, core.ValidatorTypeJSON, ref)
	assert.NoError(t, err)
	assert.Equal(t, lookup1, lookup2)

}

func TestValidateBadHash(t *testing.T) {

	coreconfig.Reset()
	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdi := dm.database.(*databasemocks.Plugin)
	data := &core.Data{
		Namespace: "ns1",
		Validator: core.ValidatorTypeJSON,
		Datatype: &core.DatatypeRef{
			Name:    "customer",
			Version: "0.0.1",
		},
		Value: fftypes.JSONAnyPtr(`{}`),
		Hash:  fftypes.NewRandB32(),
	}
	dt := &core.Datatype{
		ID:        fftypes.NewUUID(),
		Validator: core.ValidatorTypeJSON,
		Value:     fftypes.JSONAnyPtr(`{}`),
		Name:      "customer",
		Namespace: "0.0.1",
	}
	mdi.On("GetDatatypeByName", mock.Anything, "ns1", "customer", "0.0.1").Return(dt, nil).Once()
	_, err := dm.ValidateAll(ctx, core.DataArray{data})
	assert.Regexp(t, "FF10201", err)

}

func TestGetMessageDataDBError(t *testing.T) {

	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdi := dm.database.(*databasemocks.Plugin)
	mdi.On("GetDataByID", mock.Anything, "ns1", mock.Anything, true).Return(nil, fmt.Errorf("pop"))
	data, foundAll, err := dm.GetMessageDataCached(ctx, &core.Message{
		Header: core.MessageHeader{ID: fftypes.NewUUID()},
		Data:   core.DataRefs{{ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32()}},
	})
	assert.Nil(t, data)
	assert.False(t, foundAll)
	assert.EqualError(t, err, "pop")

}

func TestGetMessageDataNilEntry(t *testing.T) {

	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdi := dm.database.(*databasemocks.Plugin)
	mdi.On("GetDataByID", mock.Anything, "ns1", mock.Anything, true).Return(nil, nil)
	data, foundAll, err := dm.GetMessageDataCached(ctx, &core.Message{
		Header: core.MessageHeader{ID: fftypes.NewUUID()},
		Data:   core.DataRefs{nil},
	})
	assert.Empty(t, data)
	assert.False(t, foundAll)
	assert.NoError(t, err)

}

func TestGetMessageDataNotFound(t *testing.T) {

	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdi := dm.database.(*databasemocks.Plugin)
	mdi.On("GetDataByID", mock.Anything, "ns1", mock.Anything, true).Return(nil, nil)
	data, foundAll, err := dm.GetMessageDataCached(ctx, &core.Message{
		Header: core.MessageHeader{ID: fftypes.NewUUID()},
		Data:   core.DataRefs{{ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32()}},
	})
	assert.Empty(t, data)
	assert.False(t, foundAll)
	assert.NoError(t, err)

}

func TestGetMessageDataHashMismatch(t *testing.T) {

	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdi := dm.database.(*databasemocks.Plugin)
	dataID := fftypes.NewUUID()
	mdi.On("GetDataByID", mock.Anything, "ns1", mock.Anything, true).Return(&core.Data{
		ID:   dataID,
		Hash: fftypes.NewRandB32(),
	}, nil)
	data, foundAll, err := dm.GetMessageDataCached(ctx, &core.Message{
		Header: core.MessageHeader{ID: fftypes.NewUUID()},
		Data:   core.DataRefs{{ID: dataID, Hash: fftypes.NewRandB32()}},
	})
	assert.Empty(t, data)
	assert.False(t, foundAll)
	assert.NoError(t, err)

}

func TestGetMessageDataOk(t *testing.T) {

	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdi := dm.database.(*databasemocks.Plugin)
	dataID := fftypes.NewUUID()
	hash := fftypes.NewRandB32()
	msg := &core.Message{
		Header: core.MessageHeader{ID: fftypes.NewUUID()},
		Data:   core.DataRefs{{ID: dataID, Hash: hash}},
	}

	mdi.On("GetDataByID", mock.Anything, "ns1", mock.Anything, true).Return(&core.Data{
		ID:   dataID,
		Hash: hash,
	}, nil).Once()
	data, foundAll, err := dm.GetMessageDataCached(ctx, msg)
	assert.NotEmpty(t, data)
	assert.Equal(t, *dataID, *data[0].ID)
	assert.True(t, foundAll)
	assert.NoError(t, err)

	// Check cache kicks in for second call
	data, foundAll, err = dm.GetMessageDataCached(ctx, msg)
	assert.NotEmpty(t, data)
	assert.Equal(t, *dataID, *data[0].ID)
	assert.True(t, foundAll)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestCheckDatatypeVerifiesTheSchema(t *testing.T) {

	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	err := dm.CheckDatatype(ctx, &core.Datatype{})
	assert.Regexp(t, "FF10196", err)
}

func TestResolveInlineDataEmpty(t *testing.T) {

	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()

	newMsg := &NewMessage{
		Message: &core.MessageInOut{
			Message: core.Message{
				Header: core.MessageHeader{
					ID:        fftypes.NewUUID(),
					Namespace: "ns1",
				},
			},
			InlineData: core.InlineData{},
		},
	}

	err := dm.ResolveInlineData(ctx, newMsg)
	assert.NoError(t, err)
	assert.Empty(t, newMsg.AllData)
	assert.Empty(t, newMsg.Message.Data)

}

func TestResolveInlineDataRefIDOnlyOK(t *testing.T) {
	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdi := dm.database.(*databasemocks.Plugin)

	dataID, dataHash, newMsg := testNewMessage()

	mdi.On("GetDataByID", ctx, "ns1", dataID, true).Return(&core.Data{
		ID:        dataID,
		Namespace: "ns1",
		Hash:      dataHash,
	}, nil)

	err := dm.ResolveInlineData(ctx, newMsg)
	assert.NoError(t, err)
	assert.Len(t, newMsg.AllData, 1)
	assert.Len(t, newMsg.Message.Data, 1)
	assert.Equal(t, dataID, newMsg.AllData[0].ID)
	assert.Equal(t, dataHash, newMsg.AllData[0].Hash)
	assert.Empty(t, newMsg.NewData)
}

func TestResolveInlineDataDataToPublish(t *testing.T) {
	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdi := dm.database.(*databasemocks.Plugin)

	dataID, dataHash, newMsg := testNewMessage()
	blobHash := fftypes.NewRandB32()

	mdi.On("GetDataByID", ctx, "ns1", dataID, true).Return(&core.Data{
		ID:        dataID,
		Namespace: "ns1",
		Hash:      dataHash,
		Blob: &core.BlobRef{
			Hash: blobHash,
		},
	}, nil)
	mdi.On("GetBlobs", ctx, "ns1", mock.Anything).Return([]*core.Blob{{
		Hash:       blobHash,
		PayloadRef: "blob/1",
	}}, nil, nil)

	err := dm.ResolveInlineData(ctx, newMsg)
	assert.NoError(t, err)
	assert.Len(t, newMsg.AllData, 1)
	assert.Len(t, newMsg.Message.Data, 1)
	assert.Empty(t, newMsg.NewData)
	assert.Equal(t, dataID, newMsg.AllData[0].ID)
	assert.Equal(t, dataHash, newMsg.AllData[0].Hash)
}

func TestResolveInlineDataResolveBlobFail(t *testing.T) {
	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdi := dm.database.(*databasemocks.Plugin)

	dataID, dataHash, newMsg := testNewMessage()
	blobHash := fftypes.NewRandB32()

	mdi.On("GetDataByID", ctx, "ns1", dataID, true).Return(&core.Data{
		ID:        dataID,
		Namespace: "ns1",
		Hash:      dataHash,
		Blob: &core.BlobRef{
			Hash: blobHash,
		},
	}, nil)
	mdi.On("GetBlobs", ctx, "ns1", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	err := dm.ResolveInlineData(ctx, newMsg)
	assert.EqualError(t, err, "pop")
}

func TestResolveInlineDataRefBadHash(t *testing.T) {
	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdi := dm.database.(*databasemocks.Plugin)

	dataID, _, newMsg := testNewMessage()

	mdi.On("GetDataByID", ctx, "ns1", dataID, true).Return(&core.Data{
		ID:        dataID,
		Namespace: "ns1",
		Hash:      fftypes.NewRandB32(),
	}, nil)

	err := dm.ResolveInlineData(ctx, newMsg)
	assert.Regexp(t, "FF10204", err)
}

func TestResolveInlineDataNilMsg(t *testing.T) {
	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()

	err := dm.ResolveInlineData(ctx, &NewMessage{})
	assert.Regexp(t, "FF00125", err)
}

func TestResolveInlineDataRefLookkupFail(t *testing.T) {
	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdi := dm.database.(*databasemocks.Plugin)

	dataID, _, newMsg := testNewMessage()

	mdi.On("GetDataByID", ctx, "ns1", dataID, true).Return(nil, fmt.Errorf("pop"))

	err := dm.ResolveInlineData(ctx, newMsg)
	assert.EqualError(t, err, "pop")
}

func TestResolveInlineDataValueNoValidatorOK(t *testing.T) {
	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdi := dm.database.(*databasemocks.Plugin)

	mdi.On("UpsertData", ctx, mock.Anything, database.UpsertOptimizationNew).Return(nil)

	_, _, newMsg := testNewMessage()
	newMsg.Message.InlineData = core.InlineData{
		{Value: fftypes.JSONAnyPtr(`{"some":"json"}`)},
	}

	err := dm.ResolveInlineData(ctx, newMsg)
	assert.NoError(t, err)
	assert.Len(t, newMsg.AllData, 1)
	assert.Len(t, newMsg.NewData, 1)
	assert.Len(t, newMsg.Message.Data, 1)
	assert.NotNil(t, newMsg.AllData[0].ID)
	assert.NotNil(t, newMsg.AllData[0].Hash)
}

func TestResolveInlineDataValueWithValidation(t *testing.T) {
	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdi := dm.database.(*databasemocks.Plugin)

	mdi.On("UpsertData", ctx, mock.Anything, database.UpsertOptimizationNew).Return(nil)
	mdi.On("GetDatatypeByName", ctx, "ns1", "customer", "0.0.1").Return(&core.Datatype{
		ID:        fftypes.NewUUID(),
		Validator: core.ValidatorTypeJSON,
		Namespace: "ns1",
		Name:      "customer",
		Version:   "0.0.1",
		Value: fftypes.JSONAnyPtr(`{
			"properties": {
				"field1": {
					"type": "string"
				}
			},
			"additionalProperties": false
		}`),
	}, nil)

	_, _, newMsg := testNewMessage()
	newMsg.Message.InlineData = core.InlineData{
		{
			Datatype: &core.DatatypeRef{
				Name:    "customer",
				Version: "0.0.1",
			},
			Value: fftypes.JSONAnyPtr(`{"field1":"value1"}`),
		},
	}

	err := dm.ResolveInlineData(ctx, newMsg)
	assert.NoError(t, err)
	assert.Len(t, newMsg.AllData, 1)
	assert.Len(t, newMsg.NewData, 1)
	assert.NotNil(t, newMsg.AllData[0].ID)
	assert.NotNil(t, newMsg.AllData[0].Hash)

	newMsg.Message.InlineData = core.InlineData{
		{
			Datatype: &core.DatatypeRef{
				Name:    "customer",
				Version: "0.0.1",
			},
			Value: fftypes.JSONAnyPtr(`{"not_allowed":"value"}`),
		},
	}
	err = dm.ResolveInlineData(ctx, newMsg)
	assert.Regexp(t, "FF10198", err)
}

func TestResolveInlineDataNoRefOrValue(t *testing.T) {
	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()

	_, _, newMsg := testNewMessage()
	newMsg.Message.InlineData = core.InlineData{
		{ /* missing */ },
	}

	err := dm.ResolveInlineData(ctx, newMsg)
	assert.Regexp(t, "FF10205", err)
}

func TestUploadJSONLoadDatatypeFail(t *testing.T) {
	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdi := dm.database.(*databasemocks.Plugin)

	mdi.On("GetDatatypeByName", ctx, "ns1", "customer", "0.0.1").Return(nil, fmt.Errorf("pop"))
	_, err := dm.UploadJSON(ctx, &core.DataRefOrValue{
		Datatype: &core.DatatypeRef{
			Name:    "customer",
			Version: "0.0.1",
		},
	})
	assert.EqualError(t, err, "pop")
}

func TestUploadJSONLoadInsertDataFail(t *testing.T) {
	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	dm.messageWriter.close()
	_, err := dm.UploadJSON(ctx, &core.DataRefOrValue{
		Value: fftypes.JSONAnyPtr(`{}`),
	})
	assert.Regexp(t, "FF00154", err)
}

func TestValidateAndStoreLoadNilRef(t *testing.T) {
	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()

	_, err := dm.validateInputData(ctx, &core.DataRefOrValue{
		Validator: core.ValidatorTypeJSON,
		Datatype:  nil,
	})
	assert.Regexp(t, "FF00109", err)
}

func TestValidateAndStoreLoadValidatorUnknown(t *testing.T) {

	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdi := dm.database.(*databasemocks.Plugin)
	mdi.On("GetDatatypeByName", mock.Anything, "ns1", "customer", "0.0.1").Return(nil, nil)
	_, err := dm.validateInputData(ctx, &core.DataRefOrValue{
		Validator: "wrong!",
		Datatype: &core.DatatypeRef{
			Name:    "customer",
			Version: "0.0.1",
		},
	})
	assert.Regexp(t, "FF00108.*wrong", err)

}

func TestValidateAndStoreLoadBadRef(t *testing.T) {

	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdi := dm.database.(*databasemocks.Plugin)
	mdi.On("GetDatatypeByName", mock.Anything, "ns1", "customer", "0.0.1").Return(nil, nil)
	_, err := dm.validateInputData(ctx, &core.DataRefOrValue{
		Datatype: &core.DatatypeRef{
			// Missing name
		},
	})
	assert.Regexp(t, "FF10195", err)
}

func TestValidateAndStoreNotFound(t *testing.T) {

	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdi := dm.database.(*databasemocks.Plugin)
	mdi.On("GetDatatypeByName", mock.Anything, "ns1", "customer", "0.0.1").Return(nil, nil)
	_, err := dm.validateInputData(ctx, &core.DataRefOrValue{
		Datatype: &core.DatatypeRef{
			Name:    "customer",
			Version: "0.0.1",
		},
	})
	assert.Regexp(t, "FF10195", err)
}

func TestValidateAndStoreBlobError(t *testing.T) {

	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdi := dm.database.(*databasemocks.Plugin)
	blobHash := fftypes.NewRandB32()
	mdi.On("GetBlobs", ctx, dm.namespace.Name, mock.Anything).Return([]*core.Blob{}, nil, fmt.Errorf("pop"))
	_, err := dm.validateInputData(ctx, &core.DataRefOrValue{
		Blob: &core.BlobRef{
			Hash: blobHash,
		},
	})
	assert.Regexp(t, "pop", err)
}

func TestValidateAndStoreBlobNotFound(t *testing.T) {

	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdi := dm.database.(*databasemocks.Plugin)
	blobHash := fftypes.NewRandB32()
	mdi.On("GetBlobs", ctx, dm.namespace.Name, mock.Anything).Return([]*core.Blob{}, nil, nil)
	_, err := dm.validateInputData(ctx, &core.DataRefOrValue{
		Blob: &core.BlobRef{
			Hash: blobHash,
		},
	})
	assert.Regexp(t, "FF10239", err)
}

func TestValidateAllLookupError(t *testing.T) {

	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdi := dm.database.(*databasemocks.Plugin)
	mdi.On("GetDatatypeByName", mock.Anything, "ns1", "customer", "0.0.1").Return(nil, fmt.Errorf("pop"))
	data := &core.Data{
		Namespace: "ns1",
		Validator: core.ValidatorTypeJSON,
		Datatype: &core.DatatypeRef{
			Name:    "customer",
			Version: "0.0.1",
		},
		Value: fftypes.JSONAnyPtr(`anything`),
	}
	data.Seal(ctx, nil)
	_, err := dm.ValidateAll(ctx, core.DataArray{data})
	assert.Regexp(t, "pop", err)

}

func TestGetValidatorForDatatypeNilRef(t *testing.T) {

	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	v, err := dm.getValidatorForDatatype(ctx, "", nil)
	assert.Nil(t, v)
	assert.NoError(t, err)

}

func TestValidateAllStoredValidatorInvalid(t *testing.T) {

	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdi := dm.database.(*databasemocks.Plugin)
	mdi.On("GetDatatypeByName", mock.Anything, "ns1", "customer", "0.0.1").Return(&core.Datatype{
		Value: fftypes.JSONAnyPtr(`{"not": "a", "schema": true}`),
	}, nil)
	data := &core.Data{
		Namespace: "ns1",
		Datatype: &core.DatatypeRef{
			Name:    "customer",
			Version: "0.0.1",
		},
	}
	isValid, err := dm.ValidateAll(ctx, core.DataArray{data})
	assert.False(t, isValid)
	assert.NoError(t, err)
	mdi.AssertExpectations(t)
}

func TestHydrateBatchOK(t *testing.T) {
	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()

	batchID := fftypes.NewUUID()
	msgID := fftypes.NewUUID()
	msgHash := fftypes.NewRandB32()
	dataID := fftypes.NewUUID()
	dataHash := fftypes.NewRandB32()
	bp := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			Type:      core.BatchTypeBroadcast,
			ID:        batchID,
			Namespace: "ns1",
		},
		Manifest: fftypes.JSONAnyPtr(fmt.Sprintf(`{"id":"%s","messages":[{"id":"%s","hash":"%s"}],"data":[{"id":"%s","hash":"%s"}]}`,
			batchID, msgID, msgHash, dataID, dataHash,
		)),
		TX: core.TransactionRef{
			ID: fftypes.NewUUID(),
		},
	}

	mdi := dm.database.(*databasemocks.Plugin)
	mdi.On("GetMessageByID", ctx, "ns1", msgID).Return(&core.Message{
		Header:    core.MessageHeader{ID: msgID},
		Hash:      msgHash,
		Confirmed: fftypes.Now(),
	}, nil)
	mdi.On("GetDataByID", ctx, "ns1", dataID, true).Return(&core.Data{
		ID:      dataID,
		Hash:    dataHash,
		Created: fftypes.Now(),
	}, nil)

	batch, err := dm.HydrateBatch(ctx, bp)
	assert.NoError(t, err)
	assert.Equal(t, bp.BatchHeader, batch.BatchHeader)
	assert.Equal(t, bp.TX, batch.Payload.TX)
	assert.Equal(t, msgID, batch.Payload.Messages[0].Header.ID)
	assert.Equal(t, msgHash, batch.Payload.Messages[0].Hash)
	assert.Nil(t, batch.Payload.Messages[0].Confirmed)
	assert.Equal(t, dataID, batch.Payload.Data[0].ID)
	assert.Equal(t, dataHash, batch.Payload.Data[0].Hash)
	assert.Equal(t, dataHash, batch.Payload.Data[0].Hash)
	assert.NotNil(t, batch.Payload.Data[0].Created)

	mdi.AssertExpectations(t)
}

func TestHydrateBatchDataFail(t *testing.T) {
	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()

	batchID := fftypes.NewUUID()
	msgID := fftypes.NewUUID()
	msgHash := fftypes.NewRandB32()
	dataID := fftypes.NewUUID()
	dataHash := fftypes.NewRandB32()
	bp := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			Type:      core.BatchTypeBroadcast,
			ID:        batchID,
			Namespace: "ns1",
		},
		Manifest: fftypes.JSONAnyPtr(fmt.Sprintf(`{"id":"%s","messages":[{"id":"%s","hash":"%s"}],"data":[{"id":"%s","hash":"%s"}]}`,
			batchID, msgID, msgHash, dataID, dataHash,
		)),
		TX: core.TransactionRef{
			ID: fftypes.NewUUID(),
		},
	}

	mdi := dm.database.(*databasemocks.Plugin)
	mdi.On("GetMessageByID", ctx, "ns1", msgID).Return(&core.Message{
		Header:    core.MessageHeader{ID: msgID},
		Hash:      msgHash,
		Confirmed: fftypes.Now(),
	}, nil)
	mdi.On("GetDataByID", ctx, "ns1", dataID, true).Return(nil, fmt.Errorf("pop"))

	_, err := dm.HydrateBatch(ctx, bp)
	assert.Regexp(t, "FF10372.*pop", err)

	mdi.AssertExpectations(t)
}

func TestHydrateBatchMsgNotFound(t *testing.T) {
	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()

	batchID := fftypes.NewUUID()
	msgID := fftypes.NewUUID()
	msgHash := fftypes.NewRandB32()
	dataID := fftypes.NewUUID()
	dataHash := fftypes.NewRandB32()
	bp := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			Type:      core.BatchTypeBroadcast,
			ID:        batchID,
			Namespace: "ns1",
		},
		Manifest: fftypes.JSONAnyPtr(fmt.Sprintf(`{"id":"%s","messages":[{"id":"%s","hash":"%s"}],"data":[{"id":"%s","hash":"%s"}]}`,
			batchID, msgID, msgHash, dataID, dataHash,
		)),
		TX: core.TransactionRef{
			ID: fftypes.NewUUID(),
		},
	}

	mdi := dm.database.(*databasemocks.Plugin)
	mdi.On("GetMessageByID", ctx, "ns1", msgID).Return(nil, nil)

	_, err := dm.HydrateBatch(ctx, bp)
	assert.Regexp(t, "FF10372", err)

	mdi.AssertExpectations(t)
}

func TestHydrateBatchMsgBadManifest(t *testing.T) {
	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()

	bp := &core.BatchPersisted{
		Manifest: fftypes.JSONAnyPtr(`!json`),
	}

	_, err := dm.HydrateBatch(ctx, bp)
	assert.Regexp(t, "FF00127", err)
}

func TestGetMessageWithDataOk(t *testing.T) {
	logrus.SetLevel(logrus.DebugLevel)

	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdi := dm.database.(*databasemocks.Plugin)
	dataID := fftypes.NewUUID()
	hash := fftypes.NewRandB32()
	msg := &core.Message{
		Header: core.MessageHeader{ID: fftypes.NewUUID()},
		Data:   core.DataRefs{{ID: dataID, Hash: hash}},
	}

	mdi.On("GetMessageByID", mock.Anything, "ns1", mock.Anything).Return(msg, nil).Once()
	mdi.On("GetDataByID", mock.Anything, "ns1", mock.Anything, true).Return(&core.Data{
		ID:   dataID,
		Hash: hash,
	}, nil).Once()
	msgRet, data, foundAll, err := dm.GetMessageWithDataCached(ctx, msg.Header.ID)
	assert.Equal(t, msg, msgRet)
	assert.NotEmpty(t, data)
	assert.Equal(t, *dataID, *data[0].ID)
	assert.True(t, foundAll)
	assert.NoError(t, err)

	// Check cache kicks in for second call
	msgRet, data, foundAll, err = dm.GetMessageWithDataCached(ctx, msg.Header.ID)
	assert.Equal(t, msg, msgRet)
	assert.NotEmpty(t, data)
	assert.Equal(t, *dataID, *data[0].ID)
	assert.True(t, foundAll)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestGetMessageWithDataCRORequirePublicBlobRefs(t *testing.T) {

	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdi := dm.database.(*databasemocks.Plugin)
	dataID := fftypes.NewUUID()
	hash := fftypes.NewRandB32()
	msg := &core.Message{
		Header: core.MessageHeader{ID: fftypes.NewUUID()},
		Data:   core.DataRefs{{ID: dataID, Hash: hash}},
	}

	mdi.On("GetMessageByID", mock.Anything, "ns1", mock.Anything).Return(msg, nil).Twice()
	mdi.On("GetDataByID", mock.Anything, "ns1", mock.Anything, true).Return(&core.Data{
		ID:   dataID,
		Hash: hash,
		Blob: &core.BlobRef{
			Hash: fftypes.NewRandB32(),
		},
	}, nil).Twice()
	msgRet, data, foundAll, err := dm.GetMessageWithDataCached(ctx, msg.Header.ID)
	assert.Equal(t, msg, msgRet)
	assert.NotEmpty(t, data)
	assert.Equal(t, *dataID, *data[0].ID)
	assert.True(t, foundAll)
	assert.NoError(t, err)

	// Check cache does not kick in as we have missing blob ref
	msgRet, data, foundAll, err = dm.GetMessageWithDataCached(ctx, msg.Header.ID, CRORequirePublicBlobRefs)
	assert.Equal(t, msg, msgRet)
	assert.NotEmpty(t, data)
	assert.Equal(t, *dataID, *data[0].ID)
	assert.True(t, foundAll)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestGetMessageWithDataReadDataFail(t *testing.T) {

	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdi := dm.database.(*databasemocks.Plugin)
	dataID := fftypes.NewUUID()
	hash := fftypes.NewRandB32()
	msg := &core.Message{
		Header: core.MessageHeader{ID: fftypes.NewUUID()},
		Data:   core.DataRefs{{ID: dataID, Hash: hash}},
	}

	mdi.On("GetMessageByID", mock.Anything, "ns1", mock.Anything).Return(msg, nil)
	mdi.On("GetDataByID", mock.Anything, "ns1", mock.Anything, true).Return(nil, fmt.Errorf("pop"))
	_, _, _, err := dm.GetMessageWithDataCached(ctx, msg.Header.ID)
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
}

func TestGetMessageWithDataReadMessageFail(t *testing.T) {

	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdi := dm.database.(*databasemocks.Plugin)
	dataID := fftypes.NewUUID()
	hash := fftypes.NewRandB32()
	msg := &core.Message{
		Header: core.MessageHeader{ID: fftypes.NewUUID()},
		Data:   core.DataRefs{{ID: dataID, Hash: hash}},
	}

	mdi.On("GetMessageByID", mock.Anything, "ns1", mock.Anything).Return(msg, fmt.Errorf("pop"))
	_, _, _, err := dm.GetMessageWithDataCached(ctx, msg.Header.ID)
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
}

func TestUpdateMessageCacheCRORequirePins(t *testing.T) {

	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()

	data := core.DataArray{
		{ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32()},
	}
	msgNoPins := &core.Message{
		Header: core.MessageHeader{
			ID:     fftypes.NewUUID(),
			Topics: fftypes.FFStringArray{"topic1"},
		},
		Data: data.Refs(),
	}
	msgWithPins := &core.Message{
		Header: msgNoPins.Header,
		Data:   data.Refs(),
		Pins:   fftypes.FFStringArray{"pin1"},
	}

	msg, _ := dm.PeekMessageCache(ctx, msgWithPins.Header.ID)
	assert.Nil(t, msg)

	dm.UpdateMessageCache(msgNoPins, data)

	msg, _ = dm.PeekMessageCache(ctx, msgWithPins.Header.ID)
	assert.NotNil(t, msg)

	mce := dm.queryMessageCache(ctx, msgNoPins.Header.ID, CRORequirePins)
	assert.Nil(t, mce)

	dm.UpdateMessageIfCached(ctx, msgWithPins)
	for mce == nil {
		mce = dm.queryMessageCache(ctx, msgNoPins.Header.ID, CRORequirePins)
	}

	now := fftypes.Now()
	dm.UpdateMessageStateIfCached(ctx, msgWithPins.Header.ID, core.MessageStateConfirmed, now, "")
	assert.Equal(t, core.MessageStateConfirmed, msgWithPins.State)
	assert.Equal(t, now, msgWithPins.Confirmed)

}

func TestUpdateMessageCacheCRORequireBatchID(t *testing.T) {

	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()

	data := core.DataArray{
		{ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32()},
	}
	msgNoPins := &core.Message{
		Header: core.MessageHeader{
			ID:     fftypes.NewUUID(),
			Topics: fftypes.FFStringArray{"topic1"},
		},
		Data: data.Refs(),
	}
	msgWithBatch := &core.Message{
		Header:  msgNoPins.Header,
		Data:    data.Refs(),
		BatchID: fftypes.NewUUID(),
	}

	dm.UpdateMessageCache(msgNoPins, data)

	mce := dm.queryMessageCache(ctx, msgNoPins.Header.ID, CRORequireBatchID)
	assert.Nil(t, mce)

	dm.UpdateMessageIfCached(ctx, msgWithBatch)
	for mce == nil {
		mce = dm.queryMessageCache(ctx, msgNoPins.Header.ID, CRORequireBatchID)
	}

}

func TestWriteNewMessageFailNil(t *testing.T) {

	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()

	err := dm.WriteNewMessage(ctx, &NewMessage{})
	assert.Regexp(t, "FF00125", err)
}

func TestWriteNewMessageFailClosed(t *testing.T) {

	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	dm.messageWriter.close()

	err := dm.WriteNewMessage(ctx, &NewMessage{
		Message: &core.MessageInOut{},
	})
	assert.Regexp(t, "FF00154", err)
}

func TestDeleteData(t *testing.T) {
	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdb := dm.database.(*databasemocks.Plugin)
	mdx := dm.exchange.(*dataexchangemocks.Plugin)

	msgID := fftypes.NewUUID()
	dataID := fftypes.NewUUID()
	hash := fftypes.NewRandB32()
	payloadRef := "payloadRef"

	data := &core.Data{
		ID:        dataID,
		Namespace: dm.namespace.Name,
		Blob: &core.BlobRef{
			Hash: hash,
		},
	}

	blob := &core.Blob{
		Sequence:   0,
		Namespace:  dm.namespace.Name,
		PayloadRef: payloadRef,
		Hash:       hash,
		DataID:     dataID,
	}

	mdb.On("GetDataByID", ctx, dm.namespace.Name, dataID, false).Return(data, nil)
	mdb.On("GetBlobs", ctx, mock.Anything, mock.Anything).Return([]*core.Blob{blob}, &ffapi.FilterResult{}, nil)
	mdx.On("DeleteBlob", ctx, payloadRef).Return(nil)
	mdb.On("DeleteBlob", ctx, int64(0)).Return(nil)
	mdb.On("GetMessagesForData", ctx, dm.namespace.Name, dataID, mock.Anything).Return([]*core.Message{
		{
			Header: core.MessageHeader{
				ID: msgID,
			},
		},
	}, &ffapi.FilterResult{}, nil)
	mdb.On("DeleteData", ctx, dm.namespace.Name, dataID).Return(nil)

	err := dm.DeleteData(ctx, dataID.String())

	assert.NoError(t, err)
	mdb.AssertExpectations(t)
}

func TestDeleteDataFailParseUUID(t *testing.T) {
	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	err := dm.DeleteData(ctx, "NOT_A_UUID")
	assert.Regexp(t, "FF00138", err)
}

func TestDeleteDataFailGetData(t *testing.T) {
	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdb := dm.database.(*databasemocks.Plugin)

	dataID := fftypes.NewUUID()

	mdb.On("GetDataByID", ctx, dm.namespace.Name, dataID, false).Return(nil, fmt.Errorf("pop"))

	err := dm.DeleteData(ctx, dataID.String())

	assert.Regexp(t, "pop", err)
	mdb.AssertExpectations(t)
}

func TestDeleteDataNotFound(t *testing.T) {
	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdb := dm.database.(*databasemocks.Plugin)
	dataID := fftypes.NewUUID()

	mdb.On("GetDataByID", ctx, dm.namespace.Name, dataID, false).Return(nil, nil)

	err := dm.DeleteData(ctx, dataID.String())

	assert.Regexp(t, "FF10143", err)
	mdb.AssertExpectations(t)
}

func TestDeleteDataFailGetBlob(t *testing.T) {
	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdb := dm.database.(*databasemocks.Plugin)

	dataID := fftypes.NewUUID()
	hash := fftypes.NewRandB32()

	data := &core.Data{
		ID:        dataID,
		Namespace: dm.namespace.Name,
		Blob: &core.BlobRef{
			Hash: hash,
		},
	}

	mdb.On("GetDataByID", ctx, dm.namespace.Name, dataID, false).Return(data, nil)
	mdb.On("GetBlobs", ctx, mock.Anything, mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	err := dm.DeleteData(ctx, dataID.String())

	assert.Regexp(t, "pop", err)
	mdb.AssertExpectations(t)
}

func TestDeleteDataFailDeleteBlob(t *testing.T) {
	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdb := dm.database.(*databasemocks.Plugin)
	mdx := dm.exchange.(*dataexchangemocks.Plugin)

	dataID := fftypes.NewUUID()
	hash := fftypes.NewRandB32()
	payloadRef := "payloadRef"

	data := &core.Data{
		ID:        dataID,
		Namespace: dm.namespace.Name,
		Blob: &core.BlobRef{
			Hash: hash,
		},
	}

	blob := &core.Blob{
		Sequence:   0,
		Namespace:  dm.namespace.Name,
		PayloadRef: payloadRef,
		Hash:       hash,
		DataID:     dataID,
	}

	mdb.On("GetDataByID", ctx, dm.namespace.Name, dataID, false).Return(data, nil)
	mdb.On("GetBlobs", ctx, mock.Anything, mock.Anything).Return([]*core.Blob{blob}, &ffapi.FilterResult{}, nil)
	mdx.On("DeleteBlob", ctx, payloadRef).Return(nil)
	mdb.On("DeleteBlob", ctx, int64(0)).Return(fmt.Errorf("pop"))

	err := dm.DeleteData(ctx, dataID.String())

	assert.Regexp(t, "pop", err)
	mdb.AssertExpectations(t)
}

func TestDeleteDataFailGetBlobs(t *testing.T) {
	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdb := dm.database.(*databasemocks.Plugin)

	dataID := fftypes.NewUUID()
	hash := fftypes.NewRandB32()

	data := &core.Data{
		ID:        dataID,
		Namespace: dm.namespace.Name,
		Blob: &core.BlobRef{
			Hash: hash,
		},
	}

	mdb.On("GetDataByID", ctx, dm.namespace.Name, dataID, false).Return(data, nil)
	mdb.On("GetBlobs", ctx, mock.Anything, mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	err := dm.DeleteData(ctx, dataID.String())

	assert.Regexp(t, "pop", err)
	mdb.AssertExpectations(t)
}

func TestDeleteDataFailGetMessages(t *testing.T) {
	dm, ctx, cancel := newTestDataManager(t)
	defer cancel()
	mdb := dm.database.(*databasemocks.Plugin)
	mdx := dm.exchange.(*dataexchangemocks.Plugin)

	dataID := fftypes.NewUUID()
	hash := fftypes.NewRandB32()
	payloadRef := "payloadRef"

	data := &core.Data{
		ID:        dataID,
		Namespace: dm.namespace.Name,
		Blob: &core.BlobRef{
			Hash: hash,
		},
	}

	blob := &core.Blob{
		Sequence:   0,
		Namespace:  dm.namespace.Name,
		PayloadRef: payloadRef,
		Hash:       hash,
		DataID:     dataID,
	}

	mdb.On("GetDataByID", ctx, dm.namespace.Name, dataID, false).Return(data, nil)
	mdb.On("GetBlobs", ctx, mock.Anything, mock.Anything).Return([]*core.Blob{blob}, &ffapi.FilterResult{}, nil)
	mdx.On("DeleteBlob", ctx, payloadRef).Return(nil)
	mdb.On("DeleteBlob", ctx, int64(0)).Return(nil)
	mdb.On("GetMessagesForData", ctx, dm.namespace.Name, dataID, mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	err := dm.DeleteData(ctx, dataID.String())

	assert.Regexp(t, "pop", err)
	mdb.AssertExpectations(t)
}
