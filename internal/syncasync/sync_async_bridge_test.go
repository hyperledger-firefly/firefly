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

package syncasync

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/mocks/datamocks"
	"github.com/hyperledger-firefly/firefly/mocks/operationmocks"
	"github.com/hyperledger-firefly/firefly/mocks/systemeventmocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func newTestSyncAsyncBridge(t *testing.T) (*syncAsyncBridge, func()) {
	ctx, cancel := context.WithCancel(context.Background())
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	mom := &operationmocks.Manager{}
	mse := &systemeventmocks.EventInterface{}
	sa := NewSyncAsyncBridge(ctx, "ns1", mdi, mdm, mom)
	sa.Init(mse)
	return sa.(*syncAsyncBridge), cancel
}

func TestRequestReplyOk(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	requestID := fftypes.NewUUID()
	replyID := fftypes.NewUUID()
	dataID := fftypes.NewUUID()

	mse := sa.sysevents.(*systemeventmocks.EventInterface)
	mse.On("AddSystemEventListener", "ns1", mock.Anything).Return(nil)

	mdi := sa.database.(*databasemocks.Plugin)
	gmid := mdi.On("GetMessageByID", sa.ctx, "ns1", mock.Anything)
	gmid.RunFn = func(a mock.Arguments) {
		gmid.ReturnArguments = mock.Arguments{
			&core.Message{
				Header: core.MessageHeader{
					ID:  replyID,
					CID: requestID,
				},
				Data: core.DataRefs{
					{ID: dataID},
				},
			}, nil,
		}
	}

	mdm := sa.data.(*datamocks.Manager)
	mdm.On("GetMessageDataCached", sa.ctx, mock.Anything).Return(core.DataArray{
		{ID: dataID, Value: fftypes.JSONAnyPtr(`"response data"`)},
	}, true, nil)

	reply, err := sa.WaitForReply(sa.ctx, requestID, func(ctx context.Context) error {
		go func() {
			sa.eventCallback(&core.EventDelivery{
				EnrichedEvent: core.EnrichedEvent{
					Event: core.Event{
						ID:         fftypes.NewUUID(),
						Type:       core.EventTypeMessageConfirmed,
						Reference:  replyID,
						Correlator: requestID,
						Namespace:  "ns1",
					},
				},
			})
		}()
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, *replyID, *reply.Header.ID)
	assert.Equal(t, `"response data"`, reply.InlineData[0].Value.String())

}

func TestAwaitConfirmationOk(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	requestID := fftypes.NewUUID()
	dataID := fftypes.NewUUID()

	mse := sa.sysevents.(*systemeventmocks.EventInterface)
	mse.On("AddSystemEventListener", "ns1", mock.Anything).Return(nil)

	mdi := sa.database.(*databasemocks.Plugin)
	gmid := mdi.On("GetMessageByID", sa.ctx, "ns1", mock.Anything)
	gmid.RunFn = func(a mock.Arguments) {
		msgSent := &core.Message{}
		msgSent.Header.ID = requestID
		msgSent.Confirmed = fftypes.Now()
		msgSent.State = core.MessageStateConfirmed
		gmid.ReturnArguments = mock.Arguments{
			msgSent, nil,
		}
	}

	mdm := sa.data.(*datamocks.Manager)
	mdm.On("GetMessageDataCached", sa.ctx, mock.Anything).Return(core.DataArray{
		{ID: dataID, Value: fftypes.JSONAnyPtr(`"response data"`)},
	}, true, nil)

	reply, err := sa.WaitForMessage(sa.ctx, requestID, func(ctx context.Context) error {
		go func() {
			sa.eventCallback(&core.EventDelivery{
				EnrichedEvent: core.EnrichedEvent{
					Event: core.Event{
						ID:        fftypes.NewUUID(),
						Type:      core.EventTypeMessageConfirmed,
						Reference: requestID,
						Namespace: "ns1",
					},
				},
			})
		}()
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, *requestID, *reply.Header.ID)

}

func TestAwaitConfirmationRejected(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	requestID := fftypes.NewUUID()
	dataID := fftypes.NewUUID()

	mse := sa.sysevents.(*systemeventmocks.EventInterface)
	mse.On("AddSystemEventListener", "ns1", mock.Anything).Return(nil)

	mdi := sa.database.(*databasemocks.Plugin)
	gmid := mdi.On("GetMessageByID", sa.ctx, "ns1", mock.Anything)
	gmid.RunFn = func(a mock.Arguments) {
		msgSent := &core.Message{}
		msgSent.Header.ID = requestID
		msgSent.Confirmed = fftypes.Now()
		msgSent.State = core.MessageStateConfirmed
		gmid.ReturnArguments = mock.Arguments{
			msgSent, nil,
		}
	}

	mdm := sa.data.(*datamocks.Manager)
	mdm.On("GetMessageDataCached", sa.ctx, mock.Anything).Return(core.DataArray{
		{ID: dataID, Value: fftypes.JSONAnyPtr(`"response data"`)},
	}, true, nil)

	_, err := sa.WaitForMessage(sa.ctx, requestID, func(ctx context.Context) error {
		go func() {
			sa.eventCallback(&core.EventDelivery{
				EnrichedEvent: core.EnrichedEvent{
					Event: core.Event{
						ID:        fftypes.NewUUID(),
						Type:      core.EventTypeMessageRejected,
						Reference: requestID,
						Namespace: "ns1",
					},
				},
			})
		}()
		return nil
	})
	assert.Regexp(t, "FF10269", err)
}

func TestRequestReplyTimeout(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	cancel()

	mse := sa.sysevents.(*systemeventmocks.EventInterface)
	mse.On("AddSystemEventListener", "ns1", mock.Anything).Return(nil)

	_, err := sa.WaitForReply(sa.ctx, fftypes.NewUUID(), func(ctx context.Context) error {
		return nil
	})
	assert.Regexp(t, "FF10260", err)
}

func TestRequestSetupSystemListenerFail(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	mse := sa.sysevents.(*systemeventmocks.EventInterface)
	mse.On("AddSystemEventListener", "ns1", mock.Anything).Return(fmt.Errorf("pop"))

	_, err := sa.WaitForReply(sa.ctx, fftypes.NewUUID(), func(ctx context.Context) error {
		return nil
	})
	assert.Regexp(t, "pop", err)

}

func TestEventCallbackNotInflight(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	mse := sa.sysevents.(*systemeventmocks.EventInterface)
	mse.On("AddSystemEventListener", "ns1", mock.Anything).Return(nil)

	err := sa.eventCallback(&core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				Namespace: "ns1",
				ID:        fftypes.NewUUID(),
				Reference: fftypes.NewUUID(),
				Type:      core.EventTypeMessageConfirmed,
			},
		},
	})
	assert.NoError(t, err)

	sa.addInFlight("ns1", fftypes.NewUUID(), messageConfirm)

	for _, eventType := range []core.EventType{
		core.EventTypeMessageConfirmed,
		core.EventTypeMessageRejected,
		core.EventTypePoolConfirmed,
		core.EventTypeTransferConfirmed,
		core.EventTypeApprovalConfirmed,
		core.EventTypePoolOpFailed,
		core.EventTypeTransferOpFailed,
		core.EventTypeApprovalOpFailed,
		core.EventTypeIdentityConfirmed,
		core.EventTypeBlockchainInvokeOpSucceeded,
		core.EventTypeBlockchainInvokeOpFailed,
		core.EventTypeBlockchainContractDeployOpSucceeded,
		core.EventTypeBlockchainContractDeployOpFailed,
	} {
		err := sa.eventCallback(&core.EventDelivery{
			EnrichedEvent: core.EnrichedEvent{
				Event: core.Event{
					Namespace: "ns1",
					ID:        fftypes.NewUUID(),
					Reference: fftypes.NewUUID(),
					Type:      eventType,
				},
			},
		})
		assert.NoError(t, err)

	}
}

func TestEventCallbackWrongType(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	responseID := fftypes.NewUUID()
	sa.inflight = map[string]map[fftypes.UUID]*inflightRequest{
		"ns1": {
			*responseID: &inflightRequest{},
		},
	}

	err := sa.eventCallback(&core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				Namespace: "ns1",
				ID:        fftypes.NewUUID(),
				Reference: fftypes.NewUUID(),
				Type:      core.EventTypeIdentityUpdated, // We use the message for this one, so no sync/async handler
			},
		},
	})
	assert.NoError(t, err)

}

func TestEventCallbackMsgLookupFail(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	responseID := fftypes.NewUUID()
	sa.inflight = map[string]map[fftypes.UUID]*inflightRequest{
		"ns1": {
			*responseID: &inflightRequest{
				reqType: messageConfirm,
			},
		},
	}

	mdi := sa.database.(*databasemocks.Plugin)
	mdi.On("GetMessageByID", sa.ctx, "ns1", mock.Anything).Return(nil, fmt.Errorf("pop"))

	err := sa.eventCallback(&core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				Namespace: "ns1",
				ID:        fftypes.NewUUID(),
				Reference: responseID,
				Type:      core.EventTypeMessageConfirmed,
			},
		},
	})
	assert.EqualError(t, err, "pop")

}

func TestEventCallbackTokenPoolLookupFail(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	responseID := fftypes.NewUUID()
	sa.inflight = map[string]map[fftypes.UUID]*inflightRequest{
		"ns1": {
			*responseID: &inflightRequest{
				reqType: tokenPoolConfirm,
			},
		},
	}

	mdi := sa.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPoolByID", sa.ctx, "ns1", mock.Anything).Return(nil, fmt.Errorf("pop"))

	err := sa.eventCallback(&core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				Namespace: "ns1",
				ID:        fftypes.NewUUID(),
				Reference: responseID,
				Type:      core.EventTypePoolConfirmed,
			},
		},
	})
	assert.EqualError(t, err, "pop")

}

func TestEventCallbackIdentityLookupFail(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	responseID := fftypes.NewUUID()
	sa.inflight = map[string]map[fftypes.UUID]*inflightRequest{
		"ns1": {
			*responseID: &inflightRequest{
				reqType: identityConfirm,
			},
		},
	}

	mdi := sa.database.(*databasemocks.Plugin)
	mdi.On("GetIdentityByID", sa.ctx, "ns1", mock.Anything).Return(nil, fmt.Errorf("pop"))

	err := sa.eventCallback(&core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				Namespace: "ns1",
				ID:        fftypes.NewUUID(),
				Reference: responseID,
				Type:      core.EventTypeIdentityConfirmed,
			},
		},
	})
	assert.EqualError(t, err, "pop")

}

func TestEventCallbackIdentityLookupNotFound(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	responseID := fftypes.NewUUID()
	sa.inflight = map[string]map[fftypes.UUID]*inflightRequest{
		"ns1": {
			*responseID: &inflightRequest{
				reqType: identityConfirm,
			},
		},
	}

	mdi := sa.database.(*databasemocks.Plugin)
	mdi.On("GetIdentityByID", sa.ctx, "ns1", mock.Anything).Return(nil, nil)

	err := sa.eventCallback(&core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				Namespace: "ns1",
				ID:        fftypes.NewUUID(),
				Reference: responseID,
				Type:      core.EventTypeIdentityConfirmed,
			},
		},
	})
	assert.NoError(t, err)

}

func TestEventCallbackTokenTransferLookupFail(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	responseID := fftypes.NewUUID()
	sa.inflight = map[string]map[fftypes.UUID]*inflightRequest{
		"ns1": {
			*responseID: &inflightRequest{
				reqType: tokenTransferConfirm,
			},
		},
	}

	mdi := sa.database.(*databasemocks.Plugin)
	mdi.On("GetTokenTransferByID", sa.ctx, "ns1", mock.Anything).Return(nil, fmt.Errorf("pop"))

	err := sa.eventCallback(&core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				Namespace: "ns1",
				ID:        fftypes.NewUUID(),
				Reference: responseID,
				Type:      core.EventTypeTransferConfirmed,
			},
		},
	})
	assert.EqualError(t, err, "pop")
}

func TestEventCallbackTokenApprovalLookupFail(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	responseID := fftypes.NewUUID()
	sa.inflight = map[string]map[fftypes.UUID]*inflightRequest{
		"ns1": {
			*responseID: &inflightRequest{
				reqType: tokenApproveConfirm,
			},
		},
	}

	mdi := sa.database.(*databasemocks.Plugin)
	mdi.On("GetTokenApprovalByID", sa.ctx, "ns1", mock.Anything).Return(nil, fmt.Errorf("pop"))

	err := sa.eventCallback(&core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				Namespace: "ns1",
				ID:        fftypes.NewUUID(),
				Reference: responseID,
				Type:      core.EventTypeApprovalConfirmed,
			},
		},
	})
	assert.EqualError(t, err, "pop")

}

func TestEventCallbackMsgNotFound(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	responseID := fftypes.NewUUID()
	sa.inflight = map[string]map[fftypes.UUID]*inflightRequest{
		"ns1": {
			*responseID: &inflightRequest{
				reqType: messageConfirm,
			},
		},
	}

	mdi := sa.database.(*databasemocks.Plugin)
	mdi.On("GetMessageByID", sa.ctx, "ns1", mock.Anything).Return(nil, nil)

	err := sa.eventCallback(&core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				Namespace: "ns1",
				ID:        fftypes.NewUUID(),
				Reference: responseID,
				Type:      core.EventTypeMessageConfirmed,
			},
		},
	})
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestEventCallbackRejectedMsgNotFound(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	responseID := fftypes.NewUUID()
	correlationID := fftypes.NewUUID()
	sa.inflight = map[string]map[fftypes.UUID]*inflightRequest{
		"ns1": {
			*responseID: &inflightRequest{
				reqType: messageConfirm,
			},
			*correlationID: &inflightRequest{
				reqType: tokenPoolConfirm,
			},
		},
	}

	mdi := sa.database.(*databasemocks.Plugin)
	mdi.On("GetMessageByID", sa.ctx, "ns1", mock.Anything).Return(nil, nil)

	err := sa.eventCallback(&core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				Namespace:  "ns1",
				ID:         fftypes.NewUUID(),
				Reference:  responseID,
				Correlator: correlationID,
				Type:       core.EventTypeMessageRejected,
			},
		},
	})
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestEventCallbackTokenPoolNotFound(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	responseID := fftypes.NewUUID()
	sa.inflight = map[string]map[fftypes.UUID]*inflightRequest{
		"ns1": {
			*responseID: &inflightRequest{
				reqType: tokenPoolConfirm,
			},
		},
	}

	mdi := sa.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPoolByID", sa.ctx, "ns1", mock.Anything).Return(nil, nil)

	err := sa.eventCallback(&core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				Namespace: "ns1",
				ID:        fftypes.NewUUID(),
				Reference: responseID,
				Type:      core.EventTypePoolConfirmed,
			},
		},
	})
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestEventCallbackTokenTransferNotFound(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	responseID := fftypes.NewUUID()
	sa.inflight = map[string]map[fftypes.UUID]*inflightRequest{
		"ns1": {
			*responseID: &inflightRequest{
				reqType: tokenTransferConfirm,
			},
		},
	}

	mdi := sa.database.(*databasemocks.Plugin)
	mdi.On("GetTokenTransferByID", sa.ctx, "ns1", mock.Anything).Return(nil, nil)

	err := sa.eventCallback(&core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				Namespace: "ns1",
				ID:        fftypes.NewUUID(),
				Reference: responseID,
				Type:      core.EventTypeTransferConfirmed,
			},
		},
	})
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestEventCallbackTokenApprovalNotFound(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	responseID := fftypes.NewUUID()
	sa.inflight = map[string]map[fftypes.UUID]*inflightRequest{
		"ns1": {
			*responseID: &inflightRequest{
				reqType: tokenApproveConfirm,
			},
		},
	}

	mdi := sa.database.(*databasemocks.Plugin)
	mdi.On("GetTokenApprovalByID", sa.ctx, "ns1", mock.Anything).Return(nil, nil)

	err := sa.eventCallback(&core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				Namespace: "ns1",
				ID:        fftypes.NewUUID(),
				Reference: responseID,
				Type:      core.EventTypeApprovalConfirmed,
			},
		},
	})
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestEventCallbackTokenPoolRejectedNoData(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	responseID := fftypes.NewUUID()
	sa.inflight = map[string]map[fftypes.UUID]*inflightRequest{
		"ns1": {
			*responseID: &inflightRequest{
				reqType: tokenPoolConfirm,
			},
		},
	}

	msg := &core.Message{
		Header: core.MessageHeader{
			ID:   fftypes.NewUUID(),
			Type: core.MessageTypeDefinition,
			Tag:  core.SystemTagDefinePool,
		},
		Data: core.DataRefs{},
	}

	mdi := sa.database.(*databasemocks.Plugin)
	mdi.On("GetMessageByID", sa.ctx, "ns1", mock.Anything).Return(msg, nil)

	err := sa.eventCallback(&core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				Namespace:  "ns1",
				ID:         fftypes.NewUUID(),
				Reference:  fftypes.NewUUID(),
				Correlator: responseID,
				Type:       core.EventTypeMessageRejected,
			},
		},
	})
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestEventCallbackTokenPoolRejectedDataError(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	responseID := fftypes.NewUUID()
	correlationID := fftypes.NewUUID()
	sa.inflight = map[string]map[fftypes.UUID]*inflightRequest{
		"ns1": {
			*responseID: &inflightRequest{
				reqType: messageConfirm,
			},
			*correlationID: &inflightRequest{
				reqType: tokenPoolConfirm,
			},
		},
	}

	dataID := fftypes.NewUUID()
	msg := &core.Message{
		Header: core.MessageHeader{
			ID:   fftypes.NewUUID(),
			Type: core.MessageTypeDefinition,
			Tag:  core.SystemTagDefinePool,
		},
		Data: core.DataRefs{
			{ID: dataID},
		},
	}

	mdi := sa.database.(*databasemocks.Plugin)
	mdi.On("GetMessageByID", sa.ctx, "ns1", mock.Anything).Return(msg, nil)
	mdi.On("GetDataByID", sa.ctx, "ns1", dataID, true).Return(nil, fmt.Errorf("pop"))

	err := sa.eventCallback(&core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				Namespace:  "ns1",
				ID:         fftypes.NewUUID(),
				Reference:  responseID,
				Correlator: correlationID,
				Type:       core.EventTypeMessageRejected,
			},
		},
	})
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestEventCallbackMsgDataLookupFail(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	mdm := sa.data.(*datamocks.Manager)
	mdm.On("GetMessageDataCached", sa.ctx, mock.Anything).Return(nil, false, fmt.Errorf("pop"))

	sa.resolveReply(&inflightRequest{}, &core.Message{
		Header: core.MessageHeader{
			ID:  fftypes.NewUUID(),
			CID: fftypes.NewUUID(),
		},
	})

	mdm.AssertExpectations(t)
}

func TestAwaitTokenPoolConfirmation(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	requestID := fftypes.NewUUID()

	mse := sa.sysevents.(*systemeventmocks.EventInterface)
	mse.On("AddSystemEventListener", "ns1", mock.Anything).Return(nil)

	mdi := sa.database.(*databasemocks.Plugin)
	gmid := mdi.On("GetTokenPoolByID", sa.ctx, "ns1", mock.Anything)
	gmid.RunFn = func(a mock.Arguments) {
		pool := &core.TokenPool{
			ID:   requestID,
			Name: "my-pool",
		}
		gmid.ReturnArguments = mock.Arguments{
			pool, nil,
		}
	}

	reply, err := sa.WaitForTokenPool(sa.ctx, requestID, func(ctx context.Context) error {
		go func() {
			sa.eventCallback(&core.EventDelivery{
				EnrichedEvent: core.EnrichedEvent{
					Event: core.Event{
						ID:        fftypes.NewUUID(),
						Type:      core.EventTypePoolConfirmed,
						Reference: requestID,
						Namespace: "ns1",
					},
				},
			})
		}()
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, *requestID, *reply.ID)
	assert.Equal(t, "my-pool", reply.Name)
}

func TestAwaitTokenPoolConfirmationSendFail(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	mse := sa.sysevents.(*systemeventmocks.EventInterface)
	mse.On("AddSystemEventListener", "ns1", mock.Anything).Return(nil)

	_, err := sa.WaitForTokenPool(sa.ctx, fftypes.NewUUID(), func(ctx context.Context) error {
		return fmt.Errorf("pop")
	})
	assert.EqualError(t, err, "pop")
}

func TestAwaitTokenPoolConfirmationRejected(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	pool := &core.TokenPoolDefinition{
		Pool: &core.TokenPool{
			ID: fftypes.NewUUID(),
		},
	}
	poolJSON, _ := json.Marshal(pool)
	data := &core.Data{
		ID:    fftypes.NewUUID(),
		Value: fftypes.JSONAnyPtrBytes(poolJSON),
	}
	msg := &core.Message{
		Header: core.MessageHeader{
			ID:   fftypes.NewUUID(),
			Type: core.MessageTypeDefinition,
			Tag:  core.SystemTagDefinePool,
		},
		Data: core.DataRefs{
			{ID: data.ID},
		},
	}

	mse := sa.sysevents.(*systemeventmocks.EventInterface)
	mse.On("AddSystemEventListener", "ns1", mock.Anything).Return(nil)

	mdi := sa.database.(*databasemocks.Plugin)
	mdi.On("GetMessageByID", sa.ctx, "ns1", msg.Header.ID).Return(msg, nil)
	mdi.On("GetDataByID", sa.ctx, "ns1", data.ID, true).Return(data, nil)

	_, err := sa.WaitForTokenPool(sa.ctx, pool.Pool.ID, func(ctx context.Context) error {
		go func() {
			sa.eventCallback(&core.EventDelivery{
				EnrichedEvent: core.EnrichedEvent{
					Event: core.Event{
						ID:         fftypes.NewUUID(),
						Type:       core.EventTypeMessageRejected,
						Reference:  msg.Header.ID,
						Correlator: pool.Pool.ID,
						Namespace:  "ns1",
					},
				},
			})
		}()
		return nil
	})
	assert.Regexp(t, "FF10276", err)
}

func TestAwaitTokenTransferConfirmation(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	requestID := fftypes.NewUUID()

	mse := sa.sysevents.(*systemeventmocks.EventInterface)
	mse.On("AddSystemEventListener", "ns1", mock.Anything).Return(nil)

	mdi := sa.database.(*databasemocks.Plugin)
	gmid := mdi.On("GetTokenTransferByID", sa.ctx, "ns1", mock.Anything)
	gmid.RunFn = func(a mock.Arguments) {
		transfer := &core.TokenTransfer{
			LocalID:    requestID,
			ProtocolID: "abc",
		}
		gmid.ReturnArguments = mock.Arguments{
			transfer, nil,
		}
	}

	reply, err := sa.WaitForTokenTransfer(sa.ctx, requestID, func(ctx context.Context) error {
		go func() {
			sa.eventCallback(&core.EventDelivery{
				EnrichedEvent: core.EnrichedEvent{
					Event: core.Event{
						ID:        fftypes.NewUUID(),
						Type:      core.EventTypeTransferConfirmed,
						Reference: requestID,
						Namespace: "ns1",
					},
				},
			})
		}()
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, *requestID, *reply.LocalID)
	assert.Equal(t, "abc", reply.ProtocolID)
}

func TestAwaitTokenApprovalConfirmation(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	requestID := fftypes.NewUUID()

	mse := sa.sysevents.(*systemeventmocks.EventInterface)
	mse.On("AddSystemEventListener", "ns1", mock.Anything).Return(nil)

	mdi := sa.database.(*databasemocks.Plugin)
	gmid := mdi.On("GetTokenApprovalByID", sa.ctx, "ns1", mock.Anything)
	gmid.RunFn = func(a mock.Arguments) {
		approval := &core.TokenApproval{
			LocalID: requestID,
			Subject: "abc",
		}
		gmid.ReturnArguments = mock.Arguments{
			approval, nil,
		}
	}

	reply, err := sa.WaitForTokenApproval(sa.ctx, requestID, func(ctx context.Context) error {
		go func() {
			sa.eventCallback(&core.EventDelivery{
				EnrichedEvent: core.EnrichedEvent{
					Event: core.Event{
						ID:        fftypes.NewUUID(),
						Type:      core.EventTypeApprovalConfirmed,
						Reference: requestID,
						Namespace: "ns1",
					},
				},
			})
		}()
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, *requestID, *reply.LocalID)
	assert.Equal(t, "abc", reply.Subject)
}

func TestAwaitTokenApprovalConfirmationSendFail(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	mse := sa.sysevents.(*systemeventmocks.EventInterface)
	mse.On("AddSystemEventListener", "ns1", mock.Anything).Return(nil)

	_, err := sa.WaitForTokenApproval(sa.ctx, fftypes.NewUUID(), func(ctx context.Context) error {
		return fmt.Errorf("pop")
	})
	assert.EqualError(t, err, "pop")
}

func TestAwaitTokenTransferConfirmationSendFail(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	mse := sa.sysevents.(*systemeventmocks.EventInterface)
	mse.On("AddSystemEventListener", "ns1", mock.Anything).Return(nil)

	_, err := sa.WaitForTokenTransfer(sa.ctx, fftypes.NewUUID(), func(ctx context.Context) error {
		return fmt.Errorf("pop")
	})
	assert.EqualError(t, err, "pop")
}

func TestAwaitFailedTokenPool(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	requestID := fftypes.NewUUID()
	op := &core.Operation{
		ID: fftypes.NewUUID(),
		Input: fftypes.JSONObject{
			"localId": requestID.String(),
		},
		Error: "pop",
	}

	mse := sa.sysevents.(*systemeventmocks.EventInterface)
	mse.On("AddSystemEventListener", "ns1", mock.Anything).Return(nil)

	mom := sa.operations.(*operationmocks.Manager)
	mom.On("GetOperationByIDCached", sa.ctx, op.ID).Return(op, nil)

	_, err := sa.WaitForTokenPool(sa.ctx, requestID, func(ctx context.Context) error {
		go func() {
			sa.eventCallback(&core.EventDelivery{
				EnrichedEvent: core.EnrichedEvent{
					Event: core.Event{
						ID:         fftypes.NewUUID(),
						Type:       core.EventTypePoolOpFailed,
						Reference:  op.ID,
						Correlator: requestID,
						Namespace:  "ns1",
					},
				},
			})
		}()
		return nil
	})
	assert.EqualError(t, err, "pop")
}

func TestAwaitFailedTokenTransfer(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	requestID := fftypes.NewUUID()
	op := &core.Operation{
		ID: fftypes.NewUUID(),
		Input: fftypes.JSONObject{
			"localId": requestID.String(),
		},
		Error: "pop",
	}

	mse := sa.sysevents.(*systemeventmocks.EventInterface)
	mse.On("AddSystemEventListener", "ns1", mock.Anything).Return(nil)

	mom := sa.operations.(*operationmocks.Manager)
	mom.On("GetOperationByIDCached", sa.ctx, op.ID).Return(op, nil)

	_, err := sa.WaitForTokenTransfer(sa.ctx, requestID, func(ctx context.Context) error {
		go func() {
			sa.eventCallback(&core.EventDelivery{
				EnrichedEvent: core.EnrichedEvent{
					Event: core.Event{
						ID:         fftypes.NewUUID(),
						Type:       core.EventTypeTransferOpFailed,
						Reference:  op.ID,
						Correlator: requestID,
						Namespace:  "ns1",
					},
				},
			})
		}()
		return nil
	})
	assert.EqualError(t, err, "pop")
}

func TestAwaitFailedTokenApproval(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	requestID := fftypes.NewUUID()
	op := &core.Operation{
		ID: fftypes.NewUUID(),
		Input: fftypes.JSONObject{
			"localId": requestID.String(),
		},
		Error: "pop",
	}

	mse := sa.sysevents.(*systemeventmocks.EventInterface)
	mse.On("AddSystemEventListener", "ns1", mock.Anything).Return(nil)

	mom := sa.operations.(*operationmocks.Manager)
	mom.On("GetOperationByIDCached", sa.ctx, op.ID).Return(op, nil)

	_, err := sa.WaitForTokenApproval(sa.ctx, requestID, func(ctx context.Context) error {
		go func() {
			sa.eventCallback(&core.EventDelivery{
				EnrichedEvent: core.EnrichedEvent{
					Event: core.Event{
						ID:         fftypes.NewUUID(),
						Type:       core.EventTypeApprovalOpFailed,
						Reference:  op.ID,
						Correlator: requestID,
						Namespace:  "ns1",
					},
				},
			})
		}()
		return nil
	})
	assert.EqualError(t, err, "pop")
}

func TestFailedTokenTransferOpError(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	requestID := fftypes.NewUUID()
	sa.inflight = map[string]map[fftypes.UUID]*inflightRequest{
		"ns1": {
			*requestID: &inflightRequest{
				reqType: tokenTransferConfirm,
			},
		},
	}

	op := &core.Operation{
		ID: fftypes.NewUUID(),
		Input: fftypes.JSONObject{
			"localId": requestID.String(),
		},
	}

	mom := sa.operations.(*operationmocks.Manager)
	mom.On("GetOperationByIDCached", sa.ctx, op.ID).Return(nil, fmt.Errorf("pop"))

	err := sa.eventCallback(&core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID:         fftypes.NewUUID(),
				Type:       core.EventTypeTransferOpFailed,
				Reference:  op.ID,
				Correlator: requestID,
				Namespace:  "ns1",
			},
		},
	})
	assert.EqualError(t, err, "pop")

	mom.AssertExpectations(t)
}

func TestFailedTokenApprovalOpError(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	requestID := fftypes.NewUUID()
	sa.inflight = map[string]map[fftypes.UUID]*inflightRequest{
		"ns1": {
			*requestID: &inflightRequest{
				reqType: tokenApproveConfirm,
			},
		},
	}

	op := &core.Operation{
		ID: fftypes.NewUUID(),
		Input: fftypes.JSONObject{
			"localId": requestID.String(),
		},
	}

	mom := sa.operations.(*operationmocks.Manager)
	mom.On("GetOperationByIDCached", sa.ctx, op.ID).Return(nil, fmt.Errorf("pop"))

	err := sa.eventCallback(&core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID:         fftypes.NewUUID(),
				Type:       core.EventTypeApprovalOpFailed,
				Reference:  op.ID,
				Correlator: requestID,
				Namespace:  "ns1",
			},
		},
	})
	assert.EqualError(t, err, "pop")

	mom.AssertExpectations(t)
}

func TestFailedTokenPoolOpNotFound(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	requestID := fftypes.NewUUID()
	sa.inflight = map[string]map[fftypes.UUID]*inflightRequest{
		"ns1": {
			*requestID: &inflightRequest{
				reqType: tokenPoolConfirm,
			},
		},
	}

	op := &core.Operation{
		ID: fftypes.NewUUID(),
		Input: fftypes.JSONObject{
			"localId": requestID.String(),
		},
	}

	mom := sa.operations.(*operationmocks.Manager)
	mom.On("GetOperationByIDCached", sa.ctx, op.ID).Return(nil, nil)

	err := sa.eventCallback(&core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID:         fftypes.NewUUID(),
				Type:       core.EventTypePoolOpFailed,
				Reference:  op.ID,
				Correlator: requestID,
				Namespace:  "ns1",
			},
		},
	})
	assert.NoError(t, err)

	mom.AssertExpectations(t)
}

func TestFailedTokenApprovalOpNotFound(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	requestID := fftypes.NewUUID()
	sa.inflight = map[string]map[fftypes.UUID]*inflightRequest{
		"ns1": {
			*requestID: &inflightRequest{
				reqType: tokenApproveConfirm,
			},
		},
	}

	op := &core.Operation{
		ID: fftypes.NewUUID(),
		Input: fftypes.JSONObject{
			"localId": requestID.String(),
		},
	}

	mom := sa.operations.(*operationmocks.Manager)
	mom.On("GetOperationByIDCached", sa.ctx, op.ID).Return(nil, nil)

	err := sa.eventCallback(&core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID:         fftypes.NewUUID(),
				Type:       core.EventTypeApprovalOpFailed,
				Reference:  op.ID,
				Correlator: requestID,
				Namespace:  "ns1",
			},
		},
	})
	assert.NoError(t, err)

	mom.AssertExpectations(t)
}

func TestFailedTokenApprovalIDLookupFail(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	requestID := fftypes.NewUUID()
	sa.inflight = map[string]map[fftypes.UUID]*inflightRequest{
		"ns1": {
			*requestID: &inflightRequest{
				reqType: tokenApproveConfirm,
			},
		},
	}

	op := &core.Operation{
		ID:    fftypes.NewUUID(),
		Input: fftypes.JSONObject{},
	}

	mom := sa.operations.(*operationmocks.Manager)
	mom.On("GetOperationByIDCached", sa.ctx, op.ID).Return(op, nil)

	err := sa.eventCallback(&core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID:         fftypes.NewUUID(),
				Type:       core.EventTypeApprovalOpFailed,
				Reference:  op.ID,
				Correlator: requestID,
				Namespace:  "ns1",
			},
		},
	})
	assert.NoError(t, err)

	mom.AssertExpectations(t)
}

func TestFailedTokenTransferOpNotFound(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	requestID := fftypes.NewUUID()
	sa.inflight = map[string]map[fftypes.UUID]*inflightRequest{
		"ns1": {
			*requestID: &inflightRequest{
				reqType: tokenTransferConfirm,
			},
		},
	}

	op := &core.Operation{
		ID: fftypes.NewUUID(),
		Input: fftypes.JSONObject{
			"localId": requestID.String(),
		},
	}

	mom := sa.operations.(*operationmocks.Manager)
	mom.On("GetOperationByIDCached", sa.ctx, op.ID).Return(nil, nil)

	err := sa.eventCallback(&core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID:         fftypes.NewUUID(),
				Type:       core.EventTypeTransferOpFailed,
				Reference:  op.ID,
				Correlator: requestID,
				Namespace:  "ns1",
			},
		},
	})
	assert.NoError(t, err)

	mom.AssertExpectations(t)
}

func TestFailedTokenTransferIDLookupFail(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	requestID := fftypes.NewUUID()
	sa.inflight = map[string]map[fftypes.UUID]*inflightRequest{
		"ns1": {
			*requestID: &inflightRequest{
				reqType: tokenTransferConfirm,
			},
		},
	}

	op := &core.Operation{
		ID:    fftypes.NewUUID(),
		Input: fftypes.JSONObject{},
	}

	mom := sa.operations.(*operationmocks.Manager)
	mom.On("GetOperationByIDCached", sa.ctx, op.ID).Return(op, nil)

	err := sa.eventCallback(&core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID:         fftypes.NewUUID(),
				Type:       core.EventTypeTransferOpFailed,
				Reference:  op.ID,
				Correlator: requestID,
				Namespace:  "ns1",
			},
		},
	})
	assert.NoError(t, err)

	mom.AssertExpectations(t)
}

func TestAwaitIdentityConfirmed(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	requestID := fftypes.NewUUID()
	identity := &core.Identity{
		IdentityBase: core.IdentityBase{
			ID: requestID,
		},
	}

	mse := sa.sysevents.(*systemeventmocks.EventInterface)
	mse.On("AddSystemEventListener", "ns1", mock.Anything).Return(nil)

	mdi := sa.database.(*databasemocks.Plugin)
	mdi.On("GetIdentityByID", sa.ctx, "ns1", requestID).Return(identity, nil)

	retIdentity, err := sa.WaitForIdentity(sa.ctx, requestID, func(ctx context.Context) error {
		go func() {
			sa.eventCallback(&core.EventDelivery{
				EnrichedEvent: core.EnrichedEvent{
					Event: core.Event{
						ID:        fftypes.NewUUID(),
						Type:      core.EventTypeIdentityConfirmed,
						Reference: requestID,
						Namespace: "ns1",
					},
				},
			})
		}()
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, retIdentity, identity)
}

func TestAwaitIdentityFail(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	requestID := fftypes.NewUUID()

	mse := sa.sysevents.(*systemeventmocks.EventInterface)
	mse.On("AddSystemEventListener", "ns1", mock.Anything).Return(nil)

	_, err := sa.WaitForIdentity(sa.ctx, requestID, func(ctx context.Context) error {
		return fmt.Errorf("pop")
	})
	assert.Regexp(t, "pop", err)
}

func TestAwaitDeployOpSucceeded(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	requestID := fftypes.NewUUID()
	op := &core.Operation{
		ID:     requestID,
		Status: core.OpStatusSucceeded,
	}

	mse := sa.sysevents.(*systemeventmocks.EventInterface)
	mse.On("AddSystemEventListener", "ns1", mock.Anything).Return(nil)

	mom := sa.operations.(*operationmocks.Manager)
	mom.On("GetOperationByIDCached", sa.ctx, op.ID).Return(op, nil)

	ret, err := sa.WaitForDeployOperation(sa.ctx, requestID, func(ctx context.Context) error {
		go func() {
			sa.eventCallback(&core.EventDelivery{
				EnrichedEvent: core.EnrichedEvent{
					Event: core.Event{
						ID:        fftypes.NewUUID(),
						Type:      core.EventTypeBlockchainContractDeployOpSucceeded,
						Reference: requestID,
						Namespace: "ns1",
					},
				},
			})
		}()
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, ret, op)
}

func TestAwaitDeployOpSucceededLookupFail(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	requestID := fftypes.NewUUID()
	sa.inflight = map[string]map[fftypes.UUID]*inflightRequest{
		"ns1": {
			*requestID: &inflightRequest{
				reqType: deployOperationConfirm,
			},
		},
	}

	mse := sa.sysevents.(*systemeventmocks.EventInterface)
	mse.On("AddSystemEventListener", "ns1", mock.Anything).Return(nil)

	mom := sa.operations.(*operationmocks.Manager)
	mom.On("GetOperationByIDCached", sa.ctx, requestID).Return(nil, fmt.Errorf("pop"))

	err := sa.eventCallback(&core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID:        fftypes.NewUUID(),
				Type:      core.EventTypeBlockchainContractDeployOpSucceeded,
				Reference: requestID,
				Namespace: "ns1",
			},
		},
	})
	assert.EqualError(t, err, "pop")
}

func TestAwaitDeployOpFailed(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	requestID := fftypes.NewUUID()
	op := &core.Operation{
		ID:     requestID,
		Status: core.OpStatusFailed,
		Error:  "pop",
	}

	mse := sa.sysevents.(*systemeventmocks.EventInterface)
	mse.On("AddSystemEventListener", "ns1", mock.Anything).Return(nil)

	mom := sa.operations.(*operationmocks.Manager)
	mom.On("GetOperationByIDCached", sa.ctx, op.ID).Return(op, nil)

	_, err := sa.WaitForDeployOperation(sa.ctx, requestID, func(ctx context.Context) error {
		go func() {
			sa.eventCallback(&core.EventDelivery{
				EnrichedEvent: core.EnrichedEvent{
					Event: core.Event{
						ID:        fftypes.NewUUID(),
						Type:      core.EventTypeBlockchainContractDeployOpFailed,
						Reference: requestID,
						Namespace: "ns1",
					},
				},
			})
		}()
		return nil
	})
	assert.EqualError(t, err, "pop")
}

func TestAwaitDeployOpFailedLookupFail(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	requestID := fftypes.NewUUID()
	sa.inflight = map[string]map[fftypes.UUID]*inflightRequest{
		"ns1": {
			*requestID: &inflightRequest{
				reqType: deployOperationConfirm,
			},
		},
	}

	mse := sa.sysevents.(*systemeventmocks.EventInterface)
	mse.On("AddSystemEventListener", "ns1", mock.Anything).Return(nil)

	mom := sa.operations.(*operationmocks.Manager)
	mom.On("GetOperationByIDCached", sa.ctx, requestID).Return(nil, fmt.Errorf("pop"))

	err := sa.eventCallback(&core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID:        fftypes.NewUUID(),
				Type:      core.EventTypeBlockchainContractDeployOpFailed,
				Reference: requestID,
				Namespace: "ns1",
			},
		},
	})
	assert.EqualError(t, err, "pop")
}

func TestAwaitInvokeOpSucceeded(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	requestID := fftypes.NewUUID()
	op := &core.Operation{
		ID:     requestID,
		Status: core.OpStatusSucceeded,
	}

	mse := sa.sysevents.(*systemeventmocks.EventInterface)
	mse.On("AddSystemEventListener", "ns1", mock.Anything).Return(nil)

	mom := sa.operations.(*operationmocks.Manager)
	mom.On("GetOperationByIDCached", sa.ctx, op.ID).Return(op, nil)

	ret, err := sa.WaitForInvokeOperation(sa.ctx, requestID, func(ctx context.Context) error {
		go func() {
			sa.eventCallback(&core.EventDelivery{
				EnrichedEvent: core.EnrichedEvent{
					Event: core.Event{
						ID:        fftypes.NewUUID(),
						Type:      core.EventTypeBlockchainInvokeOpSucceeded,
						Reference: requestID,
						Namespace: "ns1",
					},
				},
			})
		}()
		return nil
	})
	assert.NoError(t, err)
	assert.Equal(t, ret, op)
}

func TestAwaitInvokeOpSucceededLookupFail(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	requestID := fftypes.NewUUID()
	sa.inflight = map[string]map[fftypes.UUID]*inflightRequest{
		"ns1": {
			*requestID: &inflightRequest{
				reqType: invokeOperationConfirm,
			},
		},
	}

	mse := sa.sysevents.(*systemeventmocks.EventInterface)
	mse.On("AddSystemEventListener", "ns1", mock.Anything).Return(nil)

	mom := sa.operations.(*operationmocks.Manager)
	mom.On("GetOperationByIDCached", sa.ctx, requestID).Return(nil, fmt.Errorf("pop"))

	err := sa.eventCallback(&core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID:        fftypes.NewUUID(),
				Type:      core.EventTypeBlockchainInvokeOpSucceeded,
				Reference: requestID,
				Namespace: "ns1",
			},
		},
	})
	assert.EqualError(t, err, "pop")
}

func TestAwaitInvokeOpFailed(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	requestID := fftypes.NewUUID()
	op := &core.Operation{
		ID:     requestID,
		Status: core.OpStatusFailed,
		Error:  "pop",
	}

	mse := sa.sysevents.(*systemeventmocks.EventInterface)
	mse.On("AddSystemEventListener", "ns1", mock.Anything).Return(nil)

	mom := sa.operations.(*operationmocks.Manager)
	mom.On("GetOperationByIDCached", sa.ctx, op.ID).Return(op, nil)

	_, err := sa.WaitForInvokeOperation(sa.ctx, requestID, func(ctx context.Context) error {
		go func() {
			sa.eventCallback(&core.EventDelivery{
				EnrichedEvent: core.EnrichedEvent{
					Event: core.Event{
						ID:        fftypes.NewUUID(),
						Type:      core.EventTypeBlockchainInvokeOpFailed,
						Reference: requestID,
						Namespace: "ns1",
					},
				},
			})
		}()
		return nil
	})
	assert.EqualError(t, err, "pop")
}

func TestAwaitInvokeOpFailedLookupFail(t *testing.T) {

	sa, cancel := newTestSyncAsyncBridge(t)
	defer cancel()

	requestID := fftypes.NewUUID()
	sa.inflight = map[string]map[fftypes.UUID]*inflightRequest{
		"ns1": {
			*requestID: &inflightRequest{
				reqType: invokeOperationConfirm,
			},
		},
	}

	mse := sa.sysevents.(*systemeventmocks.EventInterface)
	mse.On("AddSystemEventListener", "ns1", mock.Anything).Return(nil)

	mom := sa.operations.(*operationmocks.Manager)
	mom.On("GetOperationByIDCached", sa.ctx, requestID).Return(nil, fmt.Errorf("pop"))

	err := sa.eventCallback(&core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID:        fftypes.NewUUID(),
				Type:      core.EventTypeBlockchainInvokeOpFailed,
				Reference: requestID,
				Namespace: "ns1",
			},
		},
	})
	assert.EqualError(t, err, "pop")
}
