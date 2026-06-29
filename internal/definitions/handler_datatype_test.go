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

package definitions

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestHandleDefinitionBroadcastDatatypeOk(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	dt := &core.Datatype{
		ID:        fftypes.NewUUID(),
		Validator: core.ValidatorTypeJSON,
		Namespace: "ns1",
		Name:      "name1",
		Version:   "ver1",
		Value:     fftypes.JSONAnyPtr(`{}`),
	}
	dt.Hash = dt.Value.Hash()
	b, err := json.Marshal(&dt)
	assert.NoError(t, err)
	data := &core.Data{
		Value: fftypes.JSONAnyPtrBytes(b),
	}

	dh.mdm.On("CheckDatatype", mock.Anything, mock.Anything).Return(nil)
	dh.mdi.On("GetDatatypeByName", mock.Anything, "ns1", "name1", "ver1").Return(nil, nil)
	dh.mdi.On("UpsertDatatype", mock.Anything, mock.Anything, false).Return(nil)
	dh.mdi.On("InsertEvent", mock.Anything, mock.Anything).Return(nil)

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, &core.Message{
		Header: core.MessageHeader{
			Tag: core.SystemTagDefineDatatype,
		},
	}, core.DataArray{data}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionConfirm}, action)
	assert.NoError(t, err)
	err = bs.RunFinalize(context.Background())
	assert.NoError(t, err)
}

func TestHandleDefinitionBroadcastDatatypeEventFail(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	dt := &core.Datatype{
		ID:        fftypes.NewUUID(),
		Validator: core.ValidatorTypeJSON,
		Namespace: "ns1",
		Name:      "name1",
		Version:   "ver1",
		Value:     fftypes.JSONAnyPtr(`{}`),
	}
	dt.Hash = dt.Value.Hash()
	b, err := json.Marshal(&dt)
	assert.NoError(t, err)
	data := &core.Data{
		Value: fftypes.JSONAnyPtrBytes(b),
	}

	dh.mdm.On("CheckDatatype", mock.Anything, mock.Anything).Return(nil)
	dh.mdi.On("GetDatatypeByName", mock.Anything, "ns1", "name1", "ver1").Return(nil, nil)
	dh.mdi.On("UpsertDatatype", mock.Anything, mock.Anything, false).Return(nil)
	dh.mdi.On("InsertEvent", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, &core.Message{
		Header: core.MessageHeader{
			Tag: core.SystemTagDefineDatatype,
		},
	}, core.DataArray{data}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionConfirm}, action)
	assert.NoError(t, err)
	err = bs.RunFinalize(context.Background())
	assert.EqualError(t, err, "pop")
}

func TestHandleDefinitionBroadcastDatatypeMissingID(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	dt := &core.Datatype{
		Validator: core.ValidatorTypeJSON,
		Namespace: "ns1",
		Name:      "name1",
		Version:   "ver1",
		Value:     fftypes.JSONAnyPtr(`{}`),
	}
	dt.Hash = dt.Value.Hash()
	b, err := json.Marshal(&dt)
	assert.NoError(t, err)
	data := &core.Data{
		Value: fftypes.JSONAnyPtrBytes(b),
	}

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, &core.Message{
		Header: core.MessageHeader{
			Tag: core.SystemTagDefineDatatype,
		},
	}, core.DataArray{data}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionReject}, action)
	assert.Error(t, err)
	bs.assertNoFinalizers()
}

func TestHandleDefinitionBroadcastBadSchema(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	dt := &core.Datatype{
		ID:        fftypes.NewUUID(),
		Validator: core.ValidatorTypeJSON,
		Namespace: "ns1",
		Name:      "name1",
		Version:   "ver1",
		Value:     fftypes.JSONAnyPtr(`{}`),
	}
	dt.Hash = dt.Value.Hash()
	b, err := json.Marshal(&dt)
	assert.NoError(t, err)
	data := &core.Data{
		Value: fftypes.JSONAnyPtrBytes(b),
	}

	dh.mdm.On("CheckDatatype", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, &core.Message{
		Header: core.MessageHeader{
			Tag: core.SystemTagDefineDatatype,
		},
	}, core.DataArray{data}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionReject}, action)
	assert.Error(t, err)

	bs.assertNoFinalizers()
}

func TestHandleDefinitionBroadcastMissingData(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	dt := &core.Datatype{
		ID:        fftypes.NewUUID(),
		Validator: core.ValidatorTypeJSON,
		Namespace: "ns1",
		Name:      "name1",
		Version:   "ver1",
		Value:     fftypes.JSONAnyPtr(`{}`),
	}
	dt.Hash = dt.Value.Hash()

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, &core.Message{
		Header: core.MessageHeader{
			Tag: core.SystemTagDefineDatatype,
		},
	}, core.DataArray{}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionReject}, action)
	assert.Error(t, err)
	bs.assertNoFinalizers()
}

func TestHandleDefinitionBroadcastDatatypeLookupFail(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	dt := &core.Datatype{
		ID:        fftypes.NewUUID(),
		Validator: core.ValidatorTypeJSON,
		Namespace: "ns1",
		Name:      "name1",
		Version:   "ver1",
		Value:     fftypes.JSONAnyPtr(`{}`),
	}
	dt.Hash = dt.Value.Hash()
	b, err := json.Marshal(&dt)
	assert.NoError(t, err)
	data := &core.Data{
		Value: fftypes.JSONAnyPtrBytes(b),
	}

	dh.mdm.On("CheckDatatype", mock.Anything, mock.Anything).Return(nil)
	dh.mdi.On("GetDatatypeByName", mock.Anything, "ns1", "name1", "ver1").Return(nil, fmt.Errorf("pop"))
	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, &core.Message{
		Header: core.MessageHeader{
			Namespace: "ns1",
			Tag:       core.SystemTagDefineDatatype,
		},
	}, core.DataArray{data}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionRetry}, action)
	assert.EqualError(t, err, "pop")

	bs.assertNoFinalizers()
}

func TestHandleDefinitionBroadcastUpsertFail(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	dt := &core.Datatype{
		ID:        fftypes.NewUUID(),
		Validator: core.ValidatorTypeJSON,
		Namespace: "ns1",
		Name:      "name1",
		Version:   "ver1",
		Value:     fftypes.JSONAnyPtr(`{}`),
	}
	dt.Hash = dt.Value.Hash()
	b, err := json.Marshal(&dt)
	assert.NoError(t, err)
	data := &core.Data{
		Value: fftypes.JSONAnyPtrBytes(b),
	}

	dh.mdm.On("CheckDatatype", mock.Anything, mock.Anything).Return(nil)
	dh.mdi.On("GetDatatypeByName", mock.Anything, "ns1", "name1", "ver1").Return(nil, nil)
	dh.mdi.On("UpsertDatatype", mock.Anything, mock.Anything, false).Return(fmt.Errorf("pop"))
	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, &core.Message{
		Header: core.MessageHeader{
			Tag: core.SystemTagDefineDatatype,
		},
	}, core.DataArray{data}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionRetry}, action)
	assert.EqualError(t, err, "pop")

	bs.assertNoFinalizers()
}

func TestHandleDefinitionBroadcastDatatypeDuplicate(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	dt := &core.Datatype{
		ID:        fftypes.NewUUID(),
		Validator: core.ValidatorTypeJSON,
		Namespace: "ns1",
		Name:      "name1",
		Version:   "ver1",
		Value:     fftypes.JSONAnyPtr(`{}`),
	}
	dt.Hash = dt.Value.Hash()
	b, err := json.Marshal(&dt)
	assert.NoError(t, err)
	data := &core.Data{
		Value: fftypes.JSONAnyPtrBytes(b),
	}

	dh.mdm.On("CheckDatatype", mock.Anything, mock.Anything).Return(nil)
	dh.mdi.On("GetDatatypeByName", mock.Anything, "ns1", "name1", "ver1").Return(dt, nil)
	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, &core.Message{
		Header: core.MessageHeader{
			Tag: core.SystemTagDefineDatatype,
		},
	}, core.DataArray{data}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionReject}, action)
	assert.Error(t, err)

	bs.assertNoFinalizers()
}
