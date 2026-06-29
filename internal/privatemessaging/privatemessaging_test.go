// Copyright © 2025 Kaleido, Inc.
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

package privatemessaging

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/internal/batch"
	"github.com/hyperledger-firefly/firefly/internal/cache"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/mocks/batchmocks"
	"github.com/hyperledger-firefly/firefly/mocks/blockchainmocks"
	"github.com/hyperledger-firefly/firefly/mocks/cachemocks"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/mocks/dataexchangemocks"
	"github.com/hyperledger-firefly/firefly/mocks/datamocks"
	"github.com/hyperledger-firefly/firefly/mocks/identitymanagermocks"
	"github.com/hyperledger-firefly/firefly/mocks/metricsmocks"
	"github.com/hyperledger-firefly/firefly/mocks/multipartymocks"
	"github.com/hyperledger-firefly/firefly/mocks/operationmocks"
	"github.com/hyperledger-firefly/firefly/mocks/syncasyncmocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func newTestPrivateMessagingCommon(t *testing.T, metricsEnabled bool) (*privateMessaging, func()) {
	coreconfig.Reset()
	config.Set(coreconfig.CacheGroupLimit, "1m")
	config.Set(coreconfig.CacheGroupTTL, 10)

	ctx, cancel := context.WithCancel(context.Background())
	mdi := &databasemocks.Plugin{}
	mim := &identitymanagermocks.Manager{}
	mdx := &dataexchangemocks.Plugin{}
	mbi := &blockchainmocks.Plugin{}
	mba := &batchmocks.Manager{}
	mdm := &datamocks.Manager{}
	msa := &syncasyncmocks.Bridge{}
	mmp := &multipartymocks.Manager{}
	mmi := &metricsmocks.Manager{}
	mom := &operationmocks.Manager{}
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	mockRunAsGroupPassthrough(mdi)

	mba.On("RegisterDispatcher",
		pinnedPrivateDispatcherName,
		true,
		[]core.MessageType{
			core.MessageTypeGroupInit,
			core.MessageTypePrivate,
			core.MessageTypeDeprecatedTransferPrivate,
			core.MessageTypeDeprecatedApprovalPrivate,
		}, mock.Anything, mock.Anything).Return()

	mba.On("RegisterDispatcher",
		unpinnedPrivateDispatcherName,
		false,
		[]core.MessageType{
			core.MessageTypePrivate,
		}, mock.Anything, mock.Anything).Return()

	mmi.On("IsMetricsEnabled").Return(metricsEnabled)
	mom.On("RegisterHandler", mock.Anything, mock.Anything, mock.Anything)

	ns := &core.Namespace{Name: "ns1", NetworkName: "ns1-remote"}
	pm, err := NewPrivateMessaging(ctx, ns, mdi, mdx, mbi, mim, mba, mdm, msa, mmp, mmi, mom, cmi)
	assert.NoError(t, err)
	cmi.AssertCalled(t, "GetCache", cache.NewCacheConfig(
		ctx,
		coreconfig.CacheGroupLimit,
		coreconfig.CacheGroupTTL,
		ns.Name,
	))
	// Default mocks to save boilerplate in the tests
	mdx.On("Name").Return("utdx").Maybe()
	mbi.On("Name").Return("utblk").Maybe()

	return pm.(*privateMessaging), cancel
}

func TestCacheInitFail(t *testing.T) {
	coreconfig.Reset()
	config.Set(coreconfig.CacheGroupLimit, "1m")
	config.Set(coreconfig.CacheGroupTTL, 10)

	cacheInitError := errors.New("Initialization error.")
	ctx := context.Background()
	mdi := &databasemocks.Plugin{}
	mim := &identitymanagermocks.Manager{}
	mdx := &dataexchangemocks.Plugin{}
	mbi := &blockchainmocks.Plugin{}
	mba := &batchmocks.Manager{}
	mdm := &datamocks.Manager{}
	msa := &syncasyncmocks.Bridge{}
	mmp := &multipartymocks.Manager{}
	mmi := &metricsmocks.Manager{}
	mom := &operationmocks.Manager{}
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(nil, cacheInitError)

	ns := &core.Namespace{Name: "ns1", NetworkName: "ns1"}
	_, err := NewPrivateMessaging(ctx, ns, mdi, mdx, mbi, mim, mba, mdm, msa, mmp, mmi, mom, cmi)
	assert.Equal(t, cacheInitError, err)
}

func mockRunAsGroupPassthrough(mdi *databasemocks.Plugin) {
	rag := mdi.On("RunAsGroup", mock.Anything, mock.Anything).Maybe()
	rag.RunFn = func(a mock.Arguments) {
		fn := a[1].(func(context.Context) error)
		rag.ReturnArguments = mock.Arguments{fn(a[0].(context.Context))}
	}
}

func newTestPrivateMessaging(t *testing.T) (*privateMessaging, func()) {
	return newTestPrivateMessagingCommon(t, false)
}

func newTestPrivateMessagingWithMetrics(t *testing.T) (*privateMessaging, func()) {
	pm, cancel := newTestPrivateMessagingCommon(t, true)
	mmi := pm.metrics.(*metricsmocks.Manager)
	mmi.On("MessageSubmitted", mock.Anything).Return()
	return pm, cancel
}

func TestName(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()
	assert.Equal(t, "PrivateMessaging", pm.Name())
}

func TestDispatchBatchWithBlobs(t *testing.T) {

	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	localOrg := newTestOrg("localorg")
	batchID := fftypes.NewUUID()
	groupID := fftypes.NewRandB32()
	pin1 := fftypes.NewRandB32()
	pin2 := fftypes.NewRandB32()
	node1 := newTestNode("node1", localOrg)
	node2 := newTestNode("node2", newTestOrg("remoteorg"))
	txID := fftypes.NewUUID()
	batchHash := fftypes.NewRandB32()
	dataID1 := fftypes.NewUUID()
	blob1 := fftypes.NewRandB32()

	mdi := pm.database.(*databasemocks.Plugin)
	mmp := pm.multiparty.(*multipartymocks.Manager)
	mdx := pm.exchange.(*dataexchangemocks.Plugin)
	mim := pm.identity.(*identitymanagermocks.Manager)
	mom := pm.operations.(*operationmocks.Manager)

	mim.On("GetLocalNode", pm.ctx).Return(node1, nil)
	mdi.On("GetGroupByHash", pm.ctx, "ns1", groupID).Return(&core.Group{
		Hash: fftypes.NewRandB32(),
		GroupIdentity: core.GroupIdentity{
			Name: "group1",
			Members: core.Members{
				{Identity: "org1", Node: node1.ID},
				{Identity: "org2", Node: node2.ID},
			},
		},
	}, nil)
	mim.On("CachedIdentityLookupByID", pm.ctx, node1.ID).Return(node1, nil).Once()
	mim.On("CachedIdentityLookupByID", pm.ctx, node2.ID).Return(node2, nil).Once()
	mdi.On("GetBlobs", pm.ctx, mock.Anything, mock.Anything).Return([]*core.Blob{{
		Hash:       blob1,
		PayloadRef: "/blob/1",
	}}, nil, nil)
	mom.On("AddOrReuseOperation", pm.ctx, mock.MatchedBy(func(op *core.Operation) bool {
		return op.Type == core.OpTypeDataExchangeSendBlob
	})).Return(nil, nil)
	mom.On("AddOrReuseOperation", pm.ctx, mock.MatchedBy(func(op *core.Operation) bool {
		return op.Type == core.OpTypeDataExchangeSendBlob
	})).Return(nil, nil)
	mom.On("AddOrReuseOperation", pm.ctx, mock.MatchedBy(func(op *core.Operation) bool {
		return op.Type == core.OpTypeDataExchangeSendBatch
	})).Return(nil, nil)
	mom.On("AddOrReuseOperation", pm.ctx, mock.MatchedBy(func(op *core.Operation) bool {
		return op.Type == core.OpTypeDataExchangeSendBatch
	})).Return(nil, nil)
	mom.On("RunOperation", pm.ctx, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		if op.Type != core.OpTypeDataExchangeSendBlob {
			return false
		}
		data := op.Data.(transferBlobData)
		return *data.Node.ID == *node2.ID
	}), false).Return(nil, nil)
	mom.On("RunOperation", pm.ctx, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		if op.Type != core.OpTypeDataExchangeSendBatch {
			return false
		}
		data := op.Data.(batchSendData)
		return *data.Node.ID == *node2.ID
	}), false).Return(nil, nil)

	mmp.On("SubmitBatchPin", pm.ctx, mock.Anything, mock.Anything, "", false).Return(nil)

	err := pm.dispatchPinnedBatch(pm.ctx, &batch.DispatchPayload{
		Batch: core.BatchPersisted{
			BatchHeader: core.BatchHeader{
				ID: batchID,
				SignerRef: core.SignerRef{
					Author: "org1",
				},
				Group:     groupID,
				Namespace: "ns1",
			},
			TX: core.TransactionRef{
				Type: core.TransactionTypeUnpinned,
				ID:   txID,
			},
			Hash: batchHash,
		},
		Data: core.DataArray{
			{ID: dataID1, Blob: &core.BlobRef{Hash: blob1}},
		},
		Pins: []*fftypes.Bytes32{pin1, pin2},
	})
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
	mmp.AssertExpectations(t)
	mdx.AssertExpectations(t)
	mim.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestNewPrivateMessagingMissingDeps(t *testing.T) {
	_, err := NewPrivateMessaging(context.Background(), &core.Namespace{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	assert.Regexp(t, "FF10128", err)
}

func TestDispatchErrorFindingGroup(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	mdi := pm.database.(*databasemocks.Plugin)
	mdi.On("GetGroupByHash", pm.ctx, "ns1", mock.Anything).Return(nil, fmt.Errorf("pop"))

	err := pm.dispatchPinnedBatch(pm.ctx, &batch.DispatchPayload{})
	assert.Regexp(t, "pop", err)
}

func TestSendAndSubmitBatchBadID(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	mdi := pm.database.(*databasemocks.Plugin)
	mdi.On("GetGroupByHash", pm.ctx, "ns1", mock.Anything).Return(nil, fmt.Errorf("pop"))

	err := pm.dispatchPinnedBatch(pm.ctx, &batch.DispatchPayload{
		Batch: core.BatchPersisted{
			BatchHeader: core.BatchHeader{
				SignerRef: core.SignerRef{
					Author: "badauthor",
				},
			},
		},
	})
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
}

func TestSendAndSubmitBatchUnregisteredNode(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	groupID := fftypes.NewRandB32()
	node1 := newTestNode("node1", newTestOrg("localorg"))
	node2 := newTestNode("node2", newTestOrg("remoteorg"))

	mdi := pm.database.(*databasemocks.Plugin)
	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupByID", pm.ctx, node1.ID).Return(node1, nil).Once()
	mim.On("CachedIdentityLookupByID", pm.ctx, node2.ID).Return(node2, nil).Once()
	mdi.On("GetGroupByHash", pm.ctx, "ns1", groupID).Return(&core.Group{
		Hash: fftypes.NewRandB32(),
		GroupIdentity: core.GroupIdentity{
			Name: "group1",
			Members: core.Members{
				{Identity: "org1", Node: node1.ID},
				{Identity: "org2", Node: node2.ID},
			},
		},
	}, nil)

	mim.On("GetLocalNode", pm.ctx).Return(nil, fmt.Errorf("pop"))

	err := pm.dispatchPinnedBatch(pm.ctx, &batch.DispatchPayload{
		Batch: core.BatchPersisted{
			BatchHeader: core.BatchHeader{
				Group: groupID,
				SignerRef: core.SignerRef{
					Author: "badauthor",
				},
				Namespace: "ns1",
			},
		},
	})
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
}

func TestSendImmediateFail(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	mdi := pm.database.(*databasemocks.Plugin)
	mdi.On("GetGroupByHash", pm.ctx, "ns1", mock.Anything).Return(nil, fmt.Errorf("pop"))

	err := pm.dispatchPinnedBatch(pm.ctx, &batch.DispatchPayload{
		Batch: core.BatchPersisted{
			BatchHeader: core.BatchHeader{
				SignerRef: core.SignerRef{
					Author: "org1",
				},
			},
		},
	})
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
}

func TestSendSubmitInsertOperationFail(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	localOrg := newTestOrg("localorg")
	groupID := fftypes.NewRandB32()
	node1 := newTestNode("node1", localOrg)
	node2 := newTestNode("node2", newTestOrg("remoteorg"))

	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("GetLocalNode", pm.ctx).Return(node1, nil)

	mdi := pm.database.(*databasemocks.Plugin)
	mim.On("CachedIdentityLookupByID", pm.ctx, node1.ID).Return(node1, nil).Once()
	mim.On("CachedIdentityLookupByID", pm.ctx, node2.ID).Return(node2, nil).Once()
	mdi.On("GetGroupByHash", pm.ctx, "ns1", groupID).Return(&core.Group{
		Hash: fftypes.NewRandB32(),
		GroupIdentity: core.GroupIdentity{
			Name: "group1",
			Members: core.Members{
				{Identity: "org1", Node: node1.ID},
				{Identity: "org2", Node: node2.ID},
			},
		},
	}, nil)
	mom := pm.operations.(*operationmocks.Manager)
	mom.On("AddOrReuseOperation", pm.ctx, mock.Anything).Return(fmt.Errorf("pop"))

	err := pm.dispatchPinnedBatch(pm.ctx, &batch.DispatchPayload{
		Batch: core.BatchPersisted{
			BatchHeader: core.BatchHeader{
				Group: groupID,
				SignerRef: core.SignerRef{
					Author: "org1",
				},
				Namespace: "ns1",
			},
		},
	})
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
}

func TestSendSubmitBlobTransferFail(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	localOrg := newTestOrg("localorg")
	groupID := fftypes.NewRandB32()
	node1 := newTestNode("node1", localOrg)
	node2 := newTestNode("node2", newTestOrg("remoteorg"))
	blob1 := fftypes.NewRandB32()

	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("GetLocalNode", pm.ctx).Return(node1, nil)

	mdi := pm.database.(*databasemocks.Plugin)
	mim.On("CachedIdentityLookupByID", pm.ctx, node1.ID).Return(node1, nil).Once()
	mim.On("CachedIdentityLookupByID", pm.ctx, node2.ID).Return(node2, nil).Once()
	mdi.On("GetGroupByHash", pm.ctx, "ns1", groupID).Return(&core.Group{
		Hash: fftypes.NewRandB32(),
		GroupIdentity: core.GroupIdentity{
			Name: "group1",
			Members: core.Members{
				{Identity: "org1", Node: node1.ID},
				{Identity: "org2", Node: node2.ID},
			},
		},
	}, nil)

	mom := pm.operations.(*operationmocks.Manager)
	mom.On("AddOrReuseOperation", pm.ctx, mock.Anything).Return(nil)
	mdi.On("GetBlobs", pm.ctx, mock.Anything, mock.Anything).Return([]*core.Blob{{
		PayloadRef: "/blob/1",
		Hash:       blob1,
	}}, nil, nil)

	mom.On("RunOperation", pm.ctx, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(transferBlobData)
		return op.Type == core.OpTypeDataExchangeSendBlob && *data.Node.ID == *node2.ID
	}), false).Return(nil, fmt.Errorf("pop"))

	err := pm.dispatchPinnedBatch(pm.ctx, &batch.DispatchPayload{
		Batch: core.BatchPersisted{
			BatchHeader: core.BatchHeader{
				Group: groupID,
				SignerRef: core.SignerRef{
					Author: "org1",
				},
				Namespace: "ns1",
			},
		},
		Data: core.DataArray{
			{ID: fftypes.NewUUID(), Blob: &core.BlobRef{Hash: blob1}},
		},
	})
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestWriteTransactionSubmitBatchPinFail(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	localOrg := newTestOrg("localorg")
	groupID := fftypes.NewRandB32()
	node1 := newTestNode("node1", localOrg)
	node2 := newTestNode("node2", newTestOrg("remoteorg"))
	blob1 := fftypes.NewRandB32()

	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("GetLocalNode", pm.ctx).Return(node1, nil)

	mdi := pm.database.(*databasemocks.Plugin)
	mim.On("CachedIdentityLookupByID", pm.ctx, node1.ID).Return(node1, nil).Once()
	mim.On("CachedIdentityLookupByID", pm.ctx, node2.ID).Return(node2, nil).Once()
	mdi.On("GetGroupByHash", pm.ctx, "ns1", groupID).Return(&core.Group{
		Hash: fftypes.NewRandB32(),
		GroupIdentity: core.GroupIdentity{
			Name: "group1",
			Members: core.Members{
				{Identity: "org1", Node: node1.ID},
				{Identity: "org2", Node: node2.ID},
			},
		},
	}, nil)

	mom := pm.operations.(*operationmocks.Manager)
	mom.On("AddOrReuseOperation", pm.ctx, mock.Anything).Return(nil)
	mom.On("RunOperation", pm.ctx, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		if op.Type != core.OpTypeDataExchangeSendBlob {
			return false
		}
		data := op.Data.(transferBlobData)
		return *data.Node.ID == *node2.ID
	}), false).Return(nil, nil)
	mom.On("RunOperation", pm.ctx, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		if op.Type != core.OpTypeDataExchangeSendBatch {
			return false
		}
		data := op.Data.(batchSendData)
		return *data.Node.ID == *node2.ID
	}), false).Return(nil, nil)

	mdi.On("GetBlobs", pm.ctx, "ns1", mock.Anything).Return([]*core.Blob{{
		Hash:       blob1,
		PayloadRef: "/blob/1",
	}}, nil, nil)

	mmp := pm.multiparty.(*multipartymocks.Manager)
	mmp.On("SubmitBatchPin", pm.ctx, mock.Anything, mock.Anything, "", false).Return(fmt.Errorf("pop"))

	err := pm.dispatchPinnedBatch(pm.ctx, &batch.DispatchPayload{
		Batch: core.BatchPersisted{
			BatchHeader: core.BatchHeader{
				Group: groupID,
				SignerRef: core.SignerRef{
					Author: "org1",
				},
				Namespace: "ns1",
			},
		},
		Data: core.DataArray{
			{Namespace: "ns1", ID: fftypes.NewUUID(), Blob: &core.BlobRef{Hash: blob1}},
		},
	})
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mmp.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestTransferBlobsNoHash(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	_, err := pm.prepareBlobTransfers(pm.ctx, core.DataArray{
		{ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32(), Blob: &core.BlobRef{}},
	}, fftypes.NewUUID(), newTestNode("node1", newTestOrg("org1")))
	assert.Regexp(t, "FF10379", err)

}

func TestTransferBlobsNotFound(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	mdi := pm.database.(*databasemocks.Plugin)
	mdi.On("GetBlobs", pm.ctx, mock.Anything, mock.Anything).Return([]*core.Blob{}, nil, nil)

	_, err := pm.prepareBlobTransfers(pm.ctx, core.DataArray{
		{ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32(), Blob: &core.BlobRef{Hash: fftypes.NewRandB32()}},
	}, fftypes.NewUUID(), newTestNode("node1", newTestOrg("org1")))
	assert.Regexp(t, "FF10239", err)

	mdi.AssertExpectations(t)
}

func TestTransferBlobsOpInsertFail(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	mdi := pm.database.(*databasemocks.Plugin)
	mdx := pm.exchange.(*dataexchangemocks.Plugin)
	mom := pm.operations.(*operationmocks.Manager)

	mdi.On("GetBlobs", pm.ctx, mock.Anything, mock.Anything).Return([]*core.Blob{{PayloadRef: "blob/1"}}, nil, nil)
	mdx.On("TransferBlob", pm.ctx, mock.Anything, "peer1", "blob/1").Return(nil)
	mom.On("AddOrReuseOperation", pm.ctx, mock.Anything).Return(fmt.Errorf("pop"))

	_, err := pm.prepareBlobTransfers(pm.ctx, core.DataArray{
		{ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32(), Blob: &core.BlobRef{Hash: fftypes.NewRandB32()}},
	}, fftypes.NewUUID(), newTestNode("node1", newTestOrg("org1")))
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
}
