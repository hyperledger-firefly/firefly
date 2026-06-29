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

package privatemessaging

import (
	"context"
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/internal/batch"
	"github.com/hyperledger-firefly/firefly/internal/data"
	"github.com/hyperledger-firefly/firefly/internal/syncasync"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/mocks/dataexchangemocks"
	"github.com/hyperledger-firefly/firefly/mocks/datamocks"
	"github.com/hyperledger-firefly/firefly/mocks/identitymanagermocks"
	"github.com/hyperledger-firefly/firefly/mocks/operationmocks"
	"github.com/hyperledger-firefly/firefly/mocks/syncasyncmocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func newTestOrg(name string) *core.Identity {
	identity := &core.Identity{
		IdentityBase: core.IdentityBase{
			ID:        fftypes.NewUUID(),
			Type:      core.IdentityTypeOrg,
			Namespace: "ns1",
			Name:      name,
			Parent:    nil,
		},
	}
	identity.DID, _ = identity.GenerateDID(context.Background())
	return identity
}

func newTestNode(name string, owner *core.Identity) *core.Identity {
	identity := &core.Identity{
		IdentityBase: core.IdentityBase{
			ID:        fftypes.NewUUID(),
			Type:      core.IdentityTypeNode,
			Namespace: "ns1",
			Name:      name,
			Parent:    owner.ID,
		},
		IdentityProfile: core.IdentityProfile{
			Profile: fftypes.JSONObject{
				"id":  fmt.Sprintf("%s-peer", name),
				"url": fmt.Sprintf("https://%s.example.com", name),
			},
		},
	}
	identity.DID, _ = identity.GenerateDID(context.Background())
	return identity
}

func TestSendConfirmMessageE2EOk(t *testing.T) {

	pm, cancel := newTestPrivateMessagingWithMetrics(t)
	defer cancel()

	mim := pm.identity.(*identitymanagermocks.Manager)
	rootOrg := newTestOrg("rootorg")
	intermediateOrg := newTestOrg("localorg")
	intermediateOrg.Parent = rootOrg.ID
	localNode := newTestNode("node1", intermediateOrg)
	mim.On("ResolveInputSigningIdentity", pm.ctx, mock.Anything).Return(nil)
	mim.On("GetRootOrg", pm.ctx).Return(intermediateOrg, nil)
	mim.On("GetLocalNode", pm.ctx).Return(localNode, nil)
	mim.On("CachedIdentityLookupMustExist", pm.ctx, "org1").Return(intermediateOrg, false, nil)
	mim.On("CachedIdentityLookupByID", pm.ctx, rootOrg.ID).Return(rootOrg, nil)

	mdm := pm.data.(*datamocks.Manager)
	mdm.On("ResolveInlineData", pm.ctx, mock.Anything).Return(nil)
	mdm.On("WriteNewMessage", pm.ctx, mock.Anything).Return(nil).Once()

	mdi := pm.database.(*databasemocks.Plugin)
	mdi.On("GetIdentities", pm.ctx, "ns1", mock.Anything).Return([]*core.Identity{}, nil, nil).Once()
	mdi.On("GetIdentities", pm.ctx, "ns1", mock.Anything).Return([]*core.Identity{localNode}, nil, nil).Once()
	mdi.On("GetGroupByHash", pm.ctx, "ns1", mock.Anything, mock.Anything).Return(&core.Group{Hash: fftypes.NewRandB32()}, nil, nil).Once()

	retMsg := &core.Message{
		Header: core.MessageHeader{
			ID: fftypes.NewUUID(),
		},
	}
	msa := pm.syncasync.(*syncasyncmocks.Bridge)
	msa.On("WaitForMessage", pm.ctx, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			send := args[2].(syncasync.SendFunction)
			send(pm.ctx)
		}).
		Return(retMsg, nil).Once()

	msg, err := pm.SendMessage(pm.ctx, &core.MessageInOut{
		InlineData: core.InlineData{
			{Value: fftypes.JSONAnyPtr(`{"some": "data"}`)},
		},
		Group: &core.InputGroup{
			Members: []core.MemberInput{
				{Identity: "org1"},
			},
		},
	}, true)
	assert.NoError(t, err)
	assert.Equal(t, retMsg, msg)

	mim.AssertExpectations(t)
	mdm.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestSendUnpinnedMessageE2EOk(t *testing.T) {

	pm, cancel := newTestPrivateMessagingWithMetrics(t)
	defer cancel()

	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("ResolveInputSigningIdentity", pm.ctx, mock.Anything).Run(func(args mock.Arguments) {
		identity := args[1].(*core.SignerRef)
		identity.Author = "localorg"
		identity.Key = "localkey"
	}).Return(nil)

	groupID := fftypes.NewRandB32()
	mdm := pm.data.(*datamocks.Manager)
	mdm.On("ResolveInlineData", pm.ctx, mock.Anything).Return(nil)
	mdm.On("WriteNewMessage", pm.ctx, mock.Anything).Return(nil).Once()

	mdi := pm.database.(*databasemocks.Plugin)
	mdi.On("GetGroupByHash", pm.ctx, "ns1", groupID).Return(&core.Group{Hash: groupID}, nil)

	msg, err := pm.SendMessage(pm.ctx, &core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				TxType: core.TransactionTypeUnpinned,
				Group:  groupID,
			},
		},
		InlineData: core.InlineData{
			{Value: fftypes.JSONAnyPtr(`{"some": "data"}`)},
		},
		Group: &core.InputGroup{
			Members: []core.MemberInput{
				{Identity: "org1"},
			},
		},
	}, false)
	assert.NoError(t, err)
	assert.NotNil(t, msg.Header.Group)

	mdm.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)

}

func TestSendMessageBadGroup(t *testing.T) {

	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("ResolveInputSigningIdentity", pm.ctx, mock.Anything).Return(nil)

	_, err := pm.SendMessage(pm.ctx, &core.MessageInOut{
		InlineData: core.InlineData{
			{Value: fftypes.JSONAnyPtr(`{"some": "data"}`)},
		},
		Group: &core.InputGroup{},
	}, true)
	assert.Regexp(t, "FF00115", err)

	mim.AssertExpectations(t)

}

func TestSendMessageBadIdentity(t *testing.T) {

	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("ResolveInputSigningIdentity", pm.ctx, mock.Anything).Return(fmt.Errorf("pop"))

	_, err := pm.SendMessage(pm.ctx, &core.MessageInOut{
		InlineData: core.InlineData{
			{Value: fftypes.JSONAnyPtr(`{"some": "data"}`)},
		},
		Group: &core.InputGroup{
			Members: []core.MemberInput{
				{Identity: "org1"},
			},
		},
	}, false)
	assert.Regexp(t, "FF10206.*pop", err)

	mim.AssertExpectations(t)

}

func TestResolveAndSendBadInlineData(t *testing.T) {

	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	mim := pm.identity.(*identitymanagermocks.Manager)
	localOrg := newTestOrg("localorg")
	localNode := newTestNode("node1", localOrg)
	mim.On("ResolveInputSigningIdentity", pm.ctx, mock.Anything).Return(nil)
	mim.On("GetRootOrg", pm.ctx).Return(localOrg, nil)
	mim.On("GetLocalNode", pm.ctx).Return(localNode, nil)
	mim.On("ResolveInputSigningIdentity", pm.ctx, mock.Anything).Run(func(args mock.Arguments) {
		identity := args[2].(*core.SignerRef)
		identity.Author = "localorg"
		identity.Key = "localkey"
	}).Return(nil)
	mim.On("CachedIdentityLookupMustExist", pm.ctx, "localorg").Return(localOrg, false, nil)

	mdi := pm.database.(*databasemocks.Plugin)
	mdi.On("GetIdentities", pm.ctx, "ns1", mock.Anything).Return([]*core.Identity{localNode}, nil, nil).Once()
	mdi.On("GetGroupByHash", pm.ctx, "ns1", mock.Anything, mock.Anything).Return(&core.Group{Hash: fftypes.NewRandB32()}, nil, nil).Once()

	mdm := pm.data.(*datamocks.Manager)
	mdm.On("ResolveInlineData", pm.ctx, mock.Anything).Return(fmt.Errorf("pop"))

	message := &messageSender{
		mgr: pm,
		msg: &data.NewMessage{
			Message: &core.MessageInOut{
				Message: core.Message{Header: core.MessageHeader{Namespace: "ns1"}},
				Group: &core.InputGroup{
					Members: []core.MemberInput{
						{Identity: "localorg"},
					},
				},
			},
		},
	}

	err := message.resolve(pm.ctx)
	assert.Regexp(t, "pop", err)

	mim.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mdm.AssertExpectations(t)

}

func TestSendUnpinnedMessageTooLarge(t *testing.T) {

	pm, cancel := newTestPrivateMessaging(t)
	pm.maxBatchPayloadLength = 100000
	defer cancel()

	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("ResolveInputSigningIdentity", pm.ctx, mock.Anything).Run(func(args mock.Arguments) {
		identity := args[1].(*core.SignerRef)
		identity.Author = "localorg"
		identity.Key = "localkey"
	}).Return(nil)

	dataID := fftypes.NewUUID()
	groupID := fftypes.NewRandB32()
	mdm := pm.data.(*datamocks.Manager)
	mdm.On("ResolveInlineData", pm.ctx, mock.Anything).Run(func(args mock.Arguments) {
		newMsg := args[1].(*data.NewMessage)
		newMsg.Message.Data = core.DataRefs{
			{ID: dataID, Hash: fftypes.NewRandB32(), ValueSize: 100001},
		}
	}).Return(nil)

	mdi := pm.database.(*databasemocks.Plugin)
	mdi.On("GetGroupByHash", pm.ctx, "ns1", groupID).Return(&core.Group{Hash: groupID}, nil)

	_, err := pm.SendMessage(pm.ctx, &core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				TxType: core.TransactionTypeUnpinned,
				Group:  groupID,
			},
		},
		InlineData: core.InlineData{
			{Value: fftypes.JSONAnyPtr(`{"some": "data"}`)},
		},
		Group: &core.InputGroup{
			Members: []core.MemberInput{
				{Identity: "org1"},
			},
		},
	}, false)
	assert.Regexp(t, "FF10328", err)

	mdm.AssertExpectations(t)
	mim.AssertExpectations(t)

}

func TestSealFail(t *testing.T) {

	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	id1 := fftypes.NewUUID()
	message := pm.NewMessage(&core.MessageInOut{
		Message: core.Message{
			Data: core.DataRefs{
				{ID: id1, Hash: fftypes.NewRandB32()},
				{ID: id1, Hash: fftypes.NewRandB32()}, // duplicate ID
			},
		},
	})

	err := message.(*messageSender).sendInternal(pm.ctx, methodSend)
	assert.Regexp(t, "FF00129", err)

}

func TestMessagePrepare(t *testing.T) {

	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	mim := pm.identity.(*identitymanagermocks.Manager)
	localOrg := newTestOrg("localorg")
	localNode := newTestNode("node1", localOrg)
	mim.On("ResolveInputSigningIdentity", pm.ctx, mock.Anything).Return(nil)
	mim.On("GetRootOrg", pm.ctx).Return(localOrg, nil)
	mim.On("GetLocalNode", pm.ctx).Return(localNode, nil)
	mim.On("ResolveInputSigningIdentity", pm.ctx, mock.Anything).Run(func(args mock.Arguments) {
		identity := args[1].(*core.SignerRef)
		identity.Author = "localorg"
		identity.Key = "localkey"
	}).Return(nil)
	mim.On("CachedIdentityLookupMustExist", pm.ctx, "localorg").Return(localOrg, false, nil)

	mdi := pm.database.(*databasemocks.Plugin)
	mdi.On("GetIdentities", pm.ctx, "ns1", mock.Anything).Return([]*core.Identity{localNode}, nil, nil).Once()
	mdi.On("GetGroupByHash", pm.ctx, "ns1", mock.Anything, mock.Anything).Return(&core.Group{Hash: fftypes.NewRandB32()}, nil, nil).Once()

	mdm := pm.data.(*datamocks.Manager)
	mdm.On("ResolveInlineData", pm.ctx, mock.Anything).Return(nil)

	message := pm.NewMessage(&core.MessageInOut{
		Message: core.Message{
			Data: core.DataRefs{
				{ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32()},
			},
		},
		Group: &core.InputGroup{
			Members: []core.MemberInput{
				{Identity: "localorg"},
			},
		},
	})

	err := message.Prepare(pm.ctx)
	assert.NoError(t, err)

	mim.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mdm.AssertExpectations(t)

}

func TestSendUnpinnedMessageGroupLookupFail(t *testing.T) {

	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	groupID := fftypes.NewRandB32()
	mdi := pm.database.(*databasemocks.Plugin)
	mdi.On("GetGroupByHash", pm.ctx, "ns1", groupID).Return(nil, fmt.Errorf("pop")).Once()

	err := pm.dispatchUnpinnedBatch(pm.ctx, &batch.DispatchPayload{
		Batch: core.BatchPersisted{
			BatchHeader: core.BatchHeader{
				ID:    fftypes.NewUUID(),
				Group: groupID,
			},
		},
		Messages: []*core.Message{
			{
				Header: core.MessageHeader{
					SignerRef: core.SignerRef{
						Author: "org1",
					},
					TxType: core.TransactionTypeUnpinned,
					Group:  groupID,
				},
			},
		},
	})
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)

}

func TestSendUnpinnedMessageInsertFail(t *testing.T) {

	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("ResolveInputSigningIdentity", pm.ctx, mock.MatchedBy(func(identity *core.SignerRef) bool {
		assert.Empty(t, identity.Author)
		return true
	})).Return(nil)

	groupID := fftypes.NewRandB32()
	mdm := pm.data.(*datamocks.Manager)
	mdm.On("ResolveInlineData", pm.ctx, mock.Anything).Return(nil)
	mdm.On("WriteNewMessage", pm.ctx, mock.Anything).Return(fmt.Errorf("pop")).Once()

	mdi := pm.database.(*databasemocks.Plugin)
	mdi.On("GetGroupByHash", pm.ctx, "ns1", groupID).Return(&core.Group{Hash: groupID}, nil)

	_, err := pm.SendMessage(pm.ctx, &core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				TxType: core.TransactionTypeUnpinned,
				Group:  groupID,
			},
		},
		InlineData: core.InlineData{
			{Value: fftypes.JSONAnyPtr(`{"some": "data"}`)},
		},
		Group: &core.InputGroup{
			Members: []core.MemberInput{
				{Identity: "org1"},
			},
		},
	}, false)
	assert.EqualError(t, err, "pop")

	mdm.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)

}

func TestSendUnpinnedMessageConfirmFail(t *testing.T) {

	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("ResolveInputSigningIdentity", pm.ctx, mock.Anything).Return(fmt.Errorf("pop"))

	_, err := pm.SendMessage(pm.ctx, &core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				TxType: core.TransactionTypeUnpinned,
				Group:  fftypes.NewRandB32(),
			},
		},
		Group: &core.InputGroup{
			Members: []core.MemberInput{
				{Identity: "org1"},
			},
		},
	}, true)
	assert.Regexp(t, "pop", err)

	mim.AssertExpectations(t)

}

func TestSendUnpinnedMessageResolveGroupFail(t *testing.T) {

	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("ResolveInputSigningIdentity", pm.ctx, mock.Anything).Return(nil)

	groupID := fftypes.NewRandB32()

	mdi := pm.database.(*databasemocks.Plugin)
	mdi.On("GetGroupByHash", pm.ctx, "ns1", groupID).Return(nil, fmt.Errorf("pop")).Once()

	mdx := pm.exchange.(*dataexchangemocks.Plugin)
	mdx.On("SendMessage", pm.ctx, mock.Anything, "peer2-remote", mock.Anything).Return("tracking1", nil).Once()

	_, err := pm.SendMessage(pm.ctx, &core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				TxType: core.TransactionTypeUnpinned,
				Group:  groupID,
			},
		},
		InlineData: core.InlineData{
			{Value: fftypes.JSONAnyPtr(`{"some": "data"}`)},
		},
		Group: &core.InputGroup{
			Members: []core.MemberInput{
				{Identity: "org1"},
			},
		},
	}, false)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)

}

func TestSendUnpinnedMessageResolveGroupNotFound(t *testing.T) {

	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("ResolveInputSigningIdentity", pm.ctx, mock.Anything).Return(nil)

	groupID := fftypes.NewRandB32()

	mdi := pm.database.(*databasemocks.Plugin)
	mdi.On("GetGroupByHash", pm.ctx, "ns1", groupID).Return(nil, nil)

	mdx := pm.exchange.(*dataexchangemocks.Plugin)
	mdx.On("SendMessage", pm.ctx, mock.Anything, "peer2-remote", mock.Anything).Return(nil).Once()

	_, err := pm.SendMessage(pm.ctx, &core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				TxType: core.TransactionTypeUnpinned,
				Group:  groupID,
			},
		},
		InlineData: core.InlineData{
			{Value: fftypes.JSONAnyPtr(`{"some": "data"}`)},
		},
		Group: &core.InputGroup{
			Members: []core.MemberInput{
				{Identity: "org1"},
			},
		},
	}, false)
	assert.Regexp(t, "FF10226", err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)

}

func TestRequestReplyMissingTag(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	msa := pm.syncasync.(*syncasyncmocks.Bridge)
	msa.On("WaitForReply", pm.ctx, mock.Anything).Return(nil, nil)

	_, err := pm.RequestReply(pm.ctx, &core.MessageInOut{})
	assert.Regexp(t, "FF10261", err)
}

func TestRequestReplyInvalidCID(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	msa := pm.syncasync.(*syncasyncmocks.Bridge)
	msa.On("WaitForReply", pm.ctx, mock.Anything).Return(nil, nil)

	_, err := pm.RequestReply(pm.ctx, &core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				Tag:   "mytag",
				CID:   fftypes.NewUUID(),
				Group: fftypes.NewRandB32(),
			},
		},
	})
	assert.Regexp(t, "FF10262", err)
}

func TestRequestReplySuccess(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("ResolveInputSigningIdentity", pm.ctx, mock.Anything).Return(nil)

	msa := pm.syncasync.(*syncasyncmocks.Bridge)
	msa.On("WaitForReply", pm.ctx, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			send := args[2].(syncasync.SendFunction)
			send(pm.ctx)
		}).
		Return(nil, nil)

	mdm := pm.data.(*datamocks.Manager)
	mdm.On("ResolveInlineData", pm.ctx, mock.Anything).Return(nil)
	mdm.On("WriteNewMessage", pm.ctx, mock.Anything).Return(nil).Once()

	groupID := fftypes.NewRandB32()

	mdi := pm.database.(*databasemocks.Plugin)
	mdi.On("GetGroupByHash", pm.ctx, "ns1", groupID).Return(&core.Group{Hash: groupID}, nil)

	_, err := pm.RequestReply(pm.ctx, &core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				Tag:   "mytag",
				Group: groupID,
				SignerRef: core.SignerRef{
					Author: "org1",
				},
			},
		},
	})
	assert.NoError(t, err)
}

func TestDispatchedUnpinnedMessageOK(t *testing.T) {

	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	localOrg := newTestOrg("localorg")
	groupID := fftypes.NewRandB32()
	node1 := newTestNode("node1", localOrg)
	node2 := newTestNode("node2", newTestOrg("remoteorg"))

	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("GetLocalNode", pm.ctx).Return(node1, nil)

	mdx := pm.exchange.(*dataexchangemocks.Plugin)
	mdx.On("SendMessage", pm.ctx, mock.Anything, "node2-peer", mock.Anything).Return(nil)

	mdi := pm.database.(*databasemocks.Plugin)
	mom := pm.operations.(*operationmocks.Manager)
	mdi.On("GetGroupByHash", pm.ctx, "ns1", groupID).Return(&core.Group{
		Hash: groupID,
		GroupIdentity: core.GroupIdentity{
			Members: core.Members{
				{Node: node1.ID, Identity: "localorg"},
				{Node: node2.ID, Identity: "remoteorg"},
			},
		},
	}, nil).Once()
	mim.On("CachedIdentityLookupByID", pm.ctx, node1.ID).Return(node1, nil).Once()
	mim.On("CachedIdentityLookupByID", pm.ctx, node2.ID).Return(node2, nil).Once()

	mom.On("AddOrReuseOperation", pm.ctx, mock.Anything).Return(nil)
	mom.On("RunOperation", pm.ctx, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(batchSendData)
		return op.Type == core.OpTypeDataExchangeSendBatch && *data.Node.ID == *node2.ID
	}), false).Return(nil, nil)

	err := pm.dispatchUnpinnedBatch(pm.ctx, &batch.DispatchPayload{
		Batch: core.BatchPersisted{
			BatchHeader: core.BatchHeader{
				ID:        fftypes.NewUUID(),
				Group:     groupID,
				Namespace: "ns1",
			},
		},
		Messages: []*core.Message{
			{
				Header: core.MessageHeader{
					Tag:   "mytag",
					Group: groupID,
					SignerRef: core.SignerRef{
						Author: "org1",
					},
				},
			},
		},
	})
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
	mom.AssertExpectations(t)
	mim.AssertExpectations(t)

}

func TestSendDataTransferBlobsFail(t *testing.T) {

	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	localOrg := newTestOrg("localorg")
	groupID := fftypes.NewRandB32()
	node1 := newTestNode("node1", localOrg)
	node2 := newTestNode("node2", newTestOrg("remoteorg"))
	nodes := []*core.Identity{node2}

	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("ResolveInputIdentity", pm.ctx, mock.MatchedBy(func(identity *core.SignerRef) bool {
		assert.Equal(t, "localorg", identity.Author)
		return true
	})).Return(nil)
	mim.On("GetRootOrg", pm.ctx).Return(localOrg, nil)
	mim.On("GetLocalNode", pm.ctx).Return(node1, nil)

	mdi := pm.database.(*databasemocks.Plugin)
	mdi.On("GetBlobs", pm.ctx, mock.Anything, mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	err := pm.sendData(pm.ctx, &core.TransportWrapper{
		Batch: &core.Batch{
			BatchHeader: core.BatchHeader{
				ID:        fftypes.NewUUID(),
				Group:     groupID,
				Namespace: "ns1",
			},
			Payload: core.BatchPayload{
				Messages: []*core.Message{
					{
						Header: core.MessageHeader{
							Tag:   "mytag",
							Group: groupID,
							SignerRef: core.SignerRef{
								Author: "org1",
							},
						},
					},
				},
				Data: core.DataArray{
					{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr("{}"), Blob: &core.BlobRef{
						Hash: fftypes.NewRandB32(),
					}},
				},
			},
		},
	}, nodes)
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)

}

func TestSendDataTransferFail(t *testing.T) {

	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	localOrg := newTestOrg("localorg")
	groupID := fftypes.NewRandB32()
	node1 := newTestNode("node1", localOrg)
	node2 := newTestNode("node2", newTestOrg("remoteorg"))
	nodes := []*core.Identity{node2}

	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("GetLocalNode", pm.ctx).Return(node1, nil)

	mom := pm.operations.(*operationmocks.Manager)
	mom.On("AddOrReuseOperation", pm.ctx, mock.Anything).Return(nil)
	mom.On("RunOperation", pm.ctx, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(batchSendData)
		return op.Type == core.OpTypeDataExchangeSendBatch && *data.Node.ID == *node2.ID
	}), false).Return(nil, fmt.Errorf("pop"))

	err := pm.sendData(pm.ctx, &core.TransportWrapper{
		Batch: &core.Batch{
			BatchHeader: core.BatchHeader{
				ID:        fftypes.NewUUID(),
				Group:     groupID,
				Namespace: "ns1",
			},
			Payload: core.BatchPayload{
				Messages: []*core.Message{
					{
						Header: core.MessageHeader{
							Tag:   "mytag",
							Group: groupID,
							SignerRef: core.SignerRef{
								Author: "org1",
							},
						},
					},
				},
			},
		},
	}, nodes)
	assert.Regexp(t, "pop", err)

	mim.AssertExpectations(t)
	mom.AssertExpectations(t)

}

func TestSendDataTransferInsertOperationFail(t *testing.T) {

	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	localOrg := newTestOrg("localorg")
	groupID := fftypes.NewRandB32()
	node1 := newTestNode("node1", localOrg)
	node2 := newTestNode("node2", newTestOrg("remoteorg"))
	nodes := []*core.Identity{node2}

	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("ResolveInputIdentity", pm.ctx, mock.MatchedBy(func(identity *core.SignerRef) bool {
		assert.Equal(t, "localorg", identity.Author)
		return true
	})).Return(nil)
	mim.On("GetRootOrg", pm.ctx).Return(localOrg, nil)
	mim.On("GetLocalNode", pm.ctx).Return(node1, nil)

	mom := pm.operations.(*operationmocks.Manager)
	mom.On("AddOrReuseOperation", pm.ctx, mock.Anything).Return(fmt.Errorf("pop"))

	err := pm.sendData(pm.ctx, &core.TransportWrapper{
		Batch: &core.Batch{
			BatchHeader: core.BatchHeader{
				ID:        fftypes.NewUUID(),
				Group:     groupID,
				Namespace: "ns1",
			},
			Payload: core.BatchPayload{
				Messages: []*core.Message{
					{
						Header: core.MessageHeader{
							Tag:   "mytag",
							Group: groupID,
							SignerRef: core.SignerRef{
								Author: "org1",
							},
						},
					},
				},
			},
		},
	}, nodes)
	assert.Regexp(t, "pop", err)

}

func TestNewMessageContractInvoke(t *testing.T) {

	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	sender := pm.NewMessage(&core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				TxType: core.TransactionTypeContractInvokePin,
			},
		},
	})
	assert.NotNil(t, sender)

}
