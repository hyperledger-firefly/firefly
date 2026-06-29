// Copyright © 2025 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//	http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.
package privatemessaging

import (
	"context"
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/mocks/dataexchangemocks"
	"github.com/hyperledger-firefly/firefly/mocks/datamocks"
	"github.com/hyperledger-firefly/firefly/mocks/identitymanagermocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestPrepareAndRunTransferBlob(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()
	dataID := fftypes.NewUUID()

	op := &core.Operation{
		Type:      core.OpTypeDataExchangeSendBlob,
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	node := &core.Identity{
		IdentityBase: core.IdentityBase{
			ID: fftypes.NewUUID(),
		},
		IdentityProfile: core.IdentityProfile{
			Profile: fftypes.JSONObject{
				"id": "peer1",
			},
		},
	}
	localNode := &core.Identity{
		IdentityBase: core.IdentityBase{
			ID: fftypes.NewUUID(),
		},
		IdentityProfile: core.IdentityProfile{
			Profile: fftypes.JSONObject{
				"id": "local1",
			},
		},
	}
	blob := &core.Blob{
		Namespace:  "ns1",
		Hash:       fftypes.NewRandB32(),
		PayloadRef: "payload",
		DataID:     dataID,
	}
	addTransferBlobInputs(op, node.ID, blob.Hash, dataID)

	mdi := pm.database.(*databasemocks.Plugin)
	mdx := pm.exchange.(*dataexchangemocks.Plugin)
	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupByID", context.Background(), mock.Anything).Return(node, nil)
	mdi.On("GetBlobs", context.Background(), "ns1", mock.Anything).Return([]*core.Blob{blob}, nil, nil)
	mim.On("GetLocalNode", context.Background()).Return(localNode, nil)
	mdx.On("TransferBlob", context.Background(), "ns1:"+op.ID.String(), node.Profile, localNode.Profile, "payload").Return(nil)

	po, err := pm.PrepareOperation(context.Background(), op)
	assert.NoError(t, err)
	assert.Equal(t, node, po.Data.(transferBlobData).Node)
	assert.Equal(t, blob, po.Data.(transferBlobData).Blob)

	_, phase, err := pm.RunOperation(context.Background(), po)

	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
	mdx.AssertExpectations(t)
	mim.AssertExpectations(t)
}

func TestPrepareAndRunBatchSend(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	op := &core.Operation{
		Type:      core.OpTypeDataExchangeSendBatch,
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	node := &core.Identity{
		IdentityBase: core.IdentityBase{
			ID: fftypes.NewUUID(),
		},
		IdentityProfile: core.IdentityProfile{
			Profile: fftypes.JSONObject{
				"id": "peer1",
			},
		},
	}
	localNode := &core.Identity{
		IdentityBase: core.IdentityBase{
			ID: fftypes.NewUUID(),
		},
		IdentityProfile: core.IdentityProfile{
			Profile: fftypes.JSONObject{
				"id": "local1",
			},
		},
	}
	group := &core.Group{
		Hash: fftypes.NewRandB32(),
	}
	bp := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			ID: fftypes.NewUUID(),
		},
	}
	batch := &core.Batch{
		BatchHeader: bp.BatchHeader,
	}
	addBatchSendInputs(op, node.ID, group.Hash, batch.ID)

	mdi := pm.database.(*databasemocks.Plugin)
	mdx := pm.exchange.(*dataexchangemocks.Plugin)
	mdm := pm.data.(*datamocks.Manager)
	mdm.On("HydrateBatch", context.Background(), bp).Return(batch, nil)
	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("GetLocalNode", context.Background()).Return(localNode, nil)
	mim.On("CachedIdentityLookupByID", context.Background(), node.ID).Return(node, nil)
	mdi.On("GetGroupByHash", context.Background(), "ns1", group.Hash).Return(group, nil)
	mdi.On("GetBatchByID", context.Background(), "ns1", batch.ID).Return(bp, nil)
	mdx.On("SendMessage", context.Background(), "ns1:"+op.ID.String(), node.Profile, localNode.Profile, mock.Anything).Return(nil)

	po, err := pm.PrepareOperation(context.Background(), op)
	assert.NoError(t, err)
	assert.Equal(t, node, po.Data.(batchSendData).Node)
	assert.Equal(t, group, po.Data.(batchSendData).Transport.Group)
	assert.Equal(t, batch, po.Data.(batchSendData).Transport.Batch)
	assert.Equal(t, "ns1-remote", po.Data.(batchSendData).Transport.Batch.Namespace) // ensure its set to the network name not the local namespace name

	_, phase, err := pm.RunOperation(context.Background(), po)

	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
	mdx.AssertExpectations(t)
	mdm.AssertExpectations(t)
	mim.AssertExpectations(t)
}

func TestPrepareAndRunBatchSendHydrateFail(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	op := &core.Operation{
		Type:      core.OpTypeDataExchangeSendBatch,
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	node := &core.Identity{
		IdentityBase: core.IdentityBase{
			ID: fftypes.NewUUID(),
		},
		IdentityProfile: core.IdentityProfile{
			Profile: fftypes.JSONObject{
				"id": "peer1",
			},
		},
	}
	group := &core.Group{
		Hash: fftypes.NewRandB32(),
	}
	bp := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			ID: fftypes.NewUUID(),
		},
	}
	batch := &core.Batch{
		BatchHeader: bp.BatchHeader,
	}
	addBatchSendInputs(op, node.ID, group.Hash, batch.ID)

	mdi := pm.database.(*databasemocks.Plugin)
	mdm := pm.data.(*datamocks.Manager)
	mdm.On("HydrateBatch", context.Background(), bp).Return(nil, fmt.Errorf("pop"))
	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupByID", context.Background(), node.ID).Return(node, nil)
	mdi.On("GetGroupByHash", context.Background(), "ns1", group.Hash).Return(group, nil)
	mdi.On("GetBatchByID", context.Background(), "ns1", batch.ID).Return(bp, nil)

	_, err := pm.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
	mdm.AssertExpectations(t)
	mim.AssertExpectations(t)
}

func TestPrepareOperationNotSupported(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	po, err := pm.PrepareOperation(context.Background(), &core.Operation{})

	assert.Nil(t, po)
	assert.Regexp(t, "FF10371", err)
}

func TestPrepareOperationBlobSendBadInput(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	op := &core.Operation{
		Type:  core.OpTypeDataExchangeSendBlob,
		Input: fftypes.JSONObject{"node": "bad"},
	}

	_, err := pm.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF00138", err)
}

func TestPrepareOperationBlobSendNodeFail(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	nodeID := fftypes.NewUUID()
	blobHash := fftypes.NewRandB32()
	op := &core.Operation{
		Type: core.OpTypeDataExchangeSendBlob,
		Input: fftypes.JSONObject{
			"node":    nodeID.String(),
			"hash":    blobHash.String(),
			"data_id": fftypes.NewUUID().String(),
		},
	}

	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupByID", context.Background(), nodeID).Return(nil, fmt.Errorf("pop"))

	_, err := pm.PrepareOperation(context.Background(), op)
	assert.EqualError(t, err, "pop")

	mim.AssertExpectations(t)
}

func TestPrepareOperationBlobSendNodeNotFound(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	nodeID := fftypes.NewUUID()
	blobHash := fftypes.NewRandB32()
	op := &core.Operation{
		Type: core.OpTypeDataExchangeSendBlob,
		Input: fftypes.JSONObject{
			"node":    nodeID.String(),
			"hash":    blobHash.String(),
			"data_id": fftypes.NewUUID().String(),
		},
	}

	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupByID", context.Background(), nodeID).Return(nil, nil)

	_, err := pm.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF10109", err)

	mim.AssertExpectations(t)
}

func TestPrepareOperationBlobSendBlobFail(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	blobHash := fftypes.NewRandB32()
	node := &core.Identity{
		IdentityBase: core.IdentityBase{
			ID: fftypes.NewUUID(),
		},
		IdentityProfile: core.IdentityProfile{
			Profile: fftypes.JSONObject{
				"id": "peer1",
			},
		},
	}
	op := &core.Operation{
		Type: core.OpTypeDataExchangeSendBlob,
		Input: fftypes.JSONObject{
			"node":    node.ID.String(),
			"hash":    blobHash.String(),
			"data_id": fftypes.NewUUID().String(),
		},
	}

	mdi := pm.database.(*databasemocks.Plugin)
	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupByID", context.Background(), node.ID).Return(node, nil)
	mdi.On("GetBlobs", context.Background(), mock.Anything, mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	_, err := pm.PrepareOperation(context.Background(), op)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
}

func TestPrepareOperationBlobSendBlobNotFound(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	blobHash := fftypes.NewRandB32()
	node := &core.Identity{
		IdentityBase: core.IdentityBase{
			ID: fftypes.NewUUID(),
		},
		IdentityProfile: core.IdentityProfile{
			Profile: fftypes.JSONObject{
				"id": "peer1",
			},
		},
	}
	op := &core.Operation{
		Type: core.OpTypeDataExchangeSendBlob,
		Input: fftypes.JSONObject{
			"node":    node.ID.String(),
			"hash":    blobHash.String(),
			"data_id": fftypes.NewUUID().String(),
		},
	}

	mdi := pm.database.(*databasemocks.Plugin)
	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupByID", context.Background(), node.ID).Return(node, nil)
	mdi.On("GetBlobs", context.Background(), mock.Anything, mock.Anything).Return([]*core.Blob{}, nil, nil)

	_, err := pm.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF10109", err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
}

func TestPrepareOperationBatchSendBadInput(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	op := &core.Operation{
		Type:  core.OpTypeDataExchangeSendBatch,
		Input: fftypes.JSONObject{"node": "bad"},
	}

	_, err := pm.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF00138", err)
}

func TestPrepareOperationBatchSendNodeFail(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	nodeID := fftypes.NewUUID()
	groupHash := fftypes.NewRandB32()
	batchID := fftypes.NewUUID()
	op := &core.Operation{
		Type: core.OpTypeDataExchangeSendBatch,
		Input: fftypes.JSONObject{
			"node":  nodeID.String(),
			"group": groupHash.String(),
			"batch": batchID.String(),
		},
	}

	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupByID", context.Background(), nodeID).Return(nil, fmt.Errorf("pop"))

	_, err := pm.PrepareOperation(context.Background(), op)
	assert.EqualError(t, err, "pop")

	mim.AssertExpectations(t)
}

func TestPrepareOperationBatchSendNodeNotFound(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	nodeID := fftypes.NewUUID()
	groupHash := fftypes.NewRandB32()
	batchID := fftypes.NewUUID()
	op := &core.Operation{
		Type: core.OpTypeDataExchangeSendBatch,
		Input: fftypes.JSONObject{
			"node":  nodeID.String(),
			"group": groupHash.String(),
			"batch": batchID.String(),
		},
	}

	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupByID", context.Background(), nodeID).Return(nil, nil)

	_, err := pm.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF10109", err)

	mim.AssertExpectations(t)
}

func TestPrepareOperationBatchSendGroupFail(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	groupHash := fftypes.NewRandB32()
	batchID := fftypes.NewUUID()
	node := &core.Identity{
		IdentityBase: core.IdentityBase{
			ID: fftypes.NewUUID(),
		},
	}
	op := &core.Operation{
		Type: core.OpTypeDataExchangeSendBatch,
		Input: fftypes.JSONObject{
			"node":  node.ID.String(),
			"group": groupHash.String(),
			"batch": batchID.String(),
		},
	}

	mdi := pm.database.(*databasemocks.Plugin)
	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupByID", context.Background(), node.ID).Return(node, nil)
	mdi.On("GetGroupByHash", context.Background(), "ns1", groupHash).Return(nil, fmt.Errorf("pop"))

	_, err := pm.PrepareOperation(context.Background(), op)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
}

func TestPrepareOperationBatchSendGroupNotFound(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	groupHash := fftypes.NewRandB32()
	batchID := fftypes.NewUUID()
	node := &core.Identity{
		IdentityBase: core.IdentityBase{
			ID: fftypes.NewUUID(),
		},
	}
	op := &core.Operation{
		Type: core.OpTypeDataExchangeSendBatch,
		Input: fftypes.JSONObject{
			"node":  node.ID.String(),
			"group": groupHash.String(),
			"batch": batchID.String(),
		},
	}

	mdi := pm.database.(*databasemocks.Plugin)
	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupByID", context.Background(), node.ID).Return(node, nil)
	mdi.On("GetGroupByHash", context.Background(), "ns1", groupHash).Return(nil, nil)

	_, err := pm.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF10109", err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
}

func TestPrepareOperationBatchSendBatchFail(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	batchID := fftypes.NewUUID()
	node := &core.Identity{
		IdentityBase: core.IdentityBase{
			ID: fftypes.NewUUID(),
		},
	}
	group := &core.Group{
		Hash: fftypes.NewRandB32(),
	}
	op := &core.Operation{
		Type: core.OpTypeDataExchangeSendBatch,
		Input: fftypes.JSONObject{
			"node":  node.ID.String(),
			"group": group.Hash.String(),
			"batch": batchID.String(),
		},
	}

	mdi := pm.database.(*databasemocks.Plugin)
	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupByID", context.Background(), node.ID).Return(node, nil)
	mdi.On("GetGroupByHash", context.Background(), "ns1", group.Hash).Return(group, nil)
	mdi.On("GetBatchByID", context.Background(), "ns1", batchID).Return(nil, fmt.Errorf("pop"))

	_, err := pm.PrepareOperation(context.Background(), op)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
}

func TestPrepareOperationBatchSendBatchNotFound(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	batchID := fftypes.NewUUID()
	node := &core.Identity{
		IdentityBase: core.IdentityBase{
			ID: fftypes.NewUUID(),
		},
	}
	group := &core.Group{
		Hash: fftypes.NewRandB32(),
	}
	op := &core.Operation{
		Type: core.OpTypeDataExchangeSendBatch,
		Input: fftypes.JSONObject{
			"node":  node.ID.String(),
			"group": group.Hash.String(),
			"batch": batchID.String(),
		},
	}

	mdi := pm.database.(*databasemocks.Plugin)
	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupByID", context.Background(), node.ID).Return(node, nil)
	mdi.On("GetGroupByHash", context.Background(), "ns1", group.Hash).Return(group, nil)
	mdi.On("GetBatchByID", context.Background(), "ns1", batchID).Return(nil, nil)

	_, err := pm.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF10109", err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
}

func TestRunOperationNotSupported(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	_, phase, err := pm.RunOperation(context.Background(), &core.PreparedOperation{})

	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.Regexp(t, "FF10378", err)
}

func TestRunOperationBatchSendInvalidData(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	op := &core.Operation{}
	node := &core.Identity{
		IdentityBase: core.IdentityBase{
			ID: fftypes.NewUUID(),
		},
	}
	localNode := &core.Identity{
		IdentityBase: core.IdentityBase{
			ID: fftypes.NewUUID(),
		},
		IdentityProfile: core.IdentityProfile{
			Profile: fftypes.JSONObject{
				"id": "local1",
			},
		},
	}
	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("GetLocalNode", context.Background()).Return(localNode, nil)
	transport := &core.TransportWrapper{
		Group: &core.Group{},
		Batch: &core.Batch{
			Payload: core.BatchPayload{
				Data: core.DataArray{
					{Value: fftypes.JSONAnyPtr(`!json`)},
				},
			},
		},
	}

	_, phase, err := pm.RunOperation(context.Background(), opSendBatch(op, node, transport))

	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.Regexp(t, "FF10137", err)
}

func TestRunOperationBatchSendNodeFail(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	op := &core.Operation{}
	node := &core.Identity{
		IdentityBase: core.IdentityBase{
			ID: fftypes.NewUUID(),
		},
	}
	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("GetLocalNode", context.Background()).Return(nil, fmt.Errorf("pop"))
	transport := &core.TransportWrapper{
		Group: &core.Group{},
		Batch: &core.Batch{
			Payload: core.BatchPayload{
				Data: core.DataArray{
					{Value: fftypes.JSONAnyPtr(`!json`)},
				},
			},
		},
	}

	_, phase, err := pm.RunOperation(context.Background(), opSendBatch(op, node, transport))

	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.EqualError(t, err, "pop")
}

func TestRunOperationBlobSendNodeFail(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	op := &core.Operation{}
	node := &core.Identity{
		IdentityBase: core.IdentityBase{
			ID: fftypes.NewUUID(),
		},
	}
	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("GetLocalNode", context.Background()).Return(nil, fmt.Errorf("pop"))

	_, phase, err := pm.RunOperation(context.Background(), opSendBlob(op, node, &core.Blob{}))

	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.EqualError(t, err, "pop")
}

func TestOperationUpdate(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()
	assert.NoError(t, pm.OnOperationUpdate(context.Background(), nil, nil))
}

func TestRetrieveBSendBlobInputs(t *testing.T) {

	nodeID := fftypes.NewUUID()
	dataID := fftypes.NewUUID()
	hash := fftypes.NewRandB32()

	op := &core.Operation{
		Input: fftypes.JSONObject{
			"node":    nodeID.String(),
			"hash":    hash.String(),
			"data_id": dataID.String(),
		},
	}
	n, h, d, err := retrieveSendBlobInputs(context.Background(), op)
	assert.NoError(t, err)
	assert.Equal(t, n, nodeID)
	assert.Equal(t, h, hash)
	assert.Equal(t, d, dataID)

	op = &core.Operation{
		Input: fftypes.JSONObject{
			"hash":    hash.String(),
			"data_id": dataID.String(),
		},
	}
	n, h, d, err = retrieveSendBlobInputs(context.Background(), op)
	assert.Regexp(t, "FF00138", err)

	op = &core.Operation{
		Input: fftypes.JSONObject{
			"node":    nodeID.String(),
			"data_id": dataID.String(),
		},
	}
	n, h, d, err = retrieveSendBlobInputs(context.Background(), op)
	assert.Regexp(t, "FF00107", err)

	op = &core.Operation{
		Input: fftypes.JSONObject{
			"node": nodeID.String(),
			"hash": hash.String(),
		},
	}
	n, h, d, err = retrieveSendBlobInputs(context.Background(), op)
	assert.Regexp(t, "FF00138", err)
}
