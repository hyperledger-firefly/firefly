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
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/mocks/identitymanagermocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestResolveMemberListNewGroupE2E(t *testing.T) {

	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	localOrg := newTestOrg("localorg")
	remoteOrg := newTestOrg("remoteorg")
	localNode := newTestNode("node1", localOrg)
	remoteNode := newTestNode("node2", remoteOrg)

	var dataID *fftypes.UUID
	mdi := pm.database.(*databasemocks.Plugin)
	mdi.On("GetIdentities", pm.ctx, "ns1", mock.Anything).Return([]*core.Identity{remoteNode}, nil, nil).Once()
	mdi.On("GetGroupByHash", pm.ctx, "ns1", mock.Anything, mock.Anything).Return(nil, nil).Once()
	mdi.On("UpsertGroup", pm.ctx, mock.Anything, database.UpsertOptimizationNew).Return(nil)

	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupMustExist", pm.ctx, "remoteorg").Return(remoteOrg, false, nil)
	mim.On("CachedIdentityLookupMustExist", pm.ctx, localOrg.DID).Return(localOrg, false, nil)
	mim.On("CachedIdentityLookupMustExist", pm.ctx, remoteOrg.DID).Return(remoteOrg, false, nil)
	mim.On("CachedIdentityLookupByID", pm.ctx, localNode.ID).Return(localNode, nil)
	mim.On("CachedIdentityLookupByID", pm.ctx, remoteNode.ID).Return(remoteNode, nil)
	mim.On("GetRootOrg", pm.ctx).Return(localOrg, nil)
	mim.On("GetLocalNode", pm.ctx).Return(localNode, nil)
	mim.On("ValidateNodeOwner", pm.ctx, localNode, localOrg).Return(true, nil)
	mim.On("ValidateNodeOwner", pm.ctx, remoteNode, remoteOrg).Return(true, nil)

	ud := mdi.On("UpsertData", pm.ctx, mock.Anything, database.UpsertOptimizationNew).Return(nil)
	ud.RunFn = func(a mock.Arguments) {
		data := a[1].(*core.Data)
		assert.Equal(t, core.ValidatorTypeSystemDefinition, data.Validator)
		assert.Equal(t, "ns1", data.Namespace)
		var group core.Group
		err := json.Unmarshal(data.Value.Bytes(), &group)
		assert.NoError(t, err)
		assert.Len(t, group.Members, 2)
		// Group identiy is sorted by group members DIDs so check them in that order
		if localOrg.DID < remoteOrg.DID {
			assert.Equal(t, localOrg.DID, group.Members[0].Identity)
			assert.Equal(t, *localNode.ID, *group.Members[0].Node)
			assert.Equal(t, remoteOrg.DID, group.Members[1].Identity)
			assert.Equal(t, *remoteNode.ID, *group.Members[1].Node)
		} else {
			assert.Equal(t, remoteOrg.DID, group.Members[1].Identity)
			assert.Equal(t, *remoteNode.ID, *group.Members[1].Node)
			assert.Equal(t, localOrg.DID, group.Members[0].Identity)
			assert.Equal(t, *localNode.ID, *group.Members[0].Node)
		}

		dataID = data.ID
	}
	um := mdi.On("UpsertMessage", pm.ctx, mock.Anything, database.UpsertOptimizationNew).Return(nil).Once()
	um.RunFn = func(a mock.Arguments) {
		msg := a[1].(*core.Message)
		assert.Equal(t, core.MessageTypeGroupInit, msg.Header.Type)
		assert.Equal(t, "ns1-remote", msg.Header.Namespace) // note this matches the remote network name, not the local namespace
		assert.Len(t, msg.Data, 1)
		assert.Equal(t, *dataID, *msg.Data[0].ID)
	}

	err := pm.resolveRecipientList(pm.ctx, &core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				Namespace: "ns1",
				SignerRef: core.SignerRef{
					Author: "org1",
				},
			},
		},
		Group: &core.InputGroup{
			Members: []core.MemberInput{
				{Identity: remoteOrg.Name},
			},
		},
	})
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)

}

func TestResolveMemberListExistingGroup(t *testing.T) {

	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	localOrg := newTestOrg("org1")
	localNode := newTestNode("node1", localOrg)

	mdi := pm.database.(*databasemocks.Plugin)
	mdi.On("GetIdentities", pm.ctx, "ns1", mock.Anything).Return([]*core.Identity{localNode}, nil, nil)
	mdi.On("GetGroupByHash", pm.ctx, "ns1", mock.Anything, mock.Anything).Return(&core.Group{Hash: fftypes.NewRandB32()}, nil, nil).Once()
	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupMustExist", pm.ctx, "org1").Return(localOrg, false, nil)
	mim.On("GetRootOrg", pm.ctx).Return(localOrg, nil)
	mim.On("GetLocalNode", pm.ctx).Return(localNode, nil)

	err := pm.resolveRecipientList(pm.ctx, &core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				SignerRef: core.SignerRef{
					Author: "org1",
				},
				Namespace: "ns1",
			},
		},
		Group: &core.InputGroup{
			Members: []core.MemberInput{
				{Identity: "org1"},
			},
		},
	})
	assert.NoError(t, err)
	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)

}

func TestResolveMemberListLookupFail(t *testing.T) {

	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	localOrg := newTestOrg("org1")
	localNode := newTestNode("node1", localOrg)

	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupMustExist", pm.ctx, "org1").Return(nil, true, fmt.Errorf("pop"))
	mim.On("GetRootOrg", pm.ctx).Return(localOrg, nil)
	mim.On("GetLocalNode", pm.ctx).Return(localNode, nil)

	err := pm.resolveRecipientList(pm.ctx, &core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				SignerRef: core.SignerRef{
					Author: "org1",
				},
				Namespace: "ns1",
			},
		},
		Group: &core.InputGroup{
			Members: []core.MemberInput{
				{Identity: "org1"},
			},
		},
	})
	assert.Regexp(t, "pop", err)
	mim.AssertExpectations(t)

}

func TestResolveMemberListGetGroupsFail(t *testing.T) {

	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	localOrg := newTestOrg("org1")
	localNode := newTestNode("node1", localOrg)

	mdi := pm.database.(*databasemocks.Plugin)
	mdi.On("GetIdentities", pm.ctx, "ns1", mock.Anything).Return([]*core.Identity{localNode}, nil, nil)
	mdi.On("GetGroupByHash", pm.ctx, "ns1", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))
	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupMustExist", pm.ctx, "org1").Return(localOrg, false, nil)
	mim.On("GetRootOrg", pm.ctx).Return(localOrg, nil)
	mim.On("GetLocalNode", pm.ctx).Return(localNode, nil)

	err := pm.resolveRecipientList(pm.ctx, &core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				SignerRef: core.SignerRef{
					Author: "org1",
				},
				Namespace: "ns1",
			},
		},
		Group: &core.InputGroup{
			Members: []core.MemberInput{
				{Identity: "org1"},
			},
		},
	})
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)

}

func TestResolveMemberListLocalOrgUnregistered(t *testing.T) {

	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("GetRootOrg", pm.ctx).Return(nil, fmt.Errorf("pop"))

	err := pm.resolveRecipientList(pm.ctx, &core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				SignerRef: core.SignerRef{
					Author: "org1",
				},
				Namespace: "ns1",
			},
		},
		Group: &core.InputGroup{
			Members: []core.MemberInput{
				{Identity: "org1"},
			},
		},
	})
	assert.EqualError(t, err, "pop")

	mim.AssertExpectations(t)

}

func TestResolveMemberListLocalMemberLookupFailed(t *testing.T) {

	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	localOrg := newTestOrg("localorg")

	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("GetRootOrg", pm.ctx).Return(localOrg, nil)
	mim.On("GetLocalNode", pm.ctx).Return(nil, fmt.Errorf("pop"))

	err := pm.resolveRecipientList(pm.ctx, &core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				SignerRef: core.SignerRef{
					Author: "org1",
				},
				Namespace: "ns1",
			},
		},
		Group: &core.InputGroup{
			Members: []core.MemberInput{
				{Identity: "org1"},
			},
		},
	})
	assert.Regexp(t, "pop", err)

	mim.AssertExpectations(t)

}

func TestResolveMemberListNodeNotFound(t *testing.T) {

	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	localOrg := newTestOrg("localorg")
	localNode := newTestNode("node1", localOrg)

	mdi := pm.database.(*databasemocks.Plugin)
	mdi.On("GetIdentities", pm.ctx, "ns1", mock.Anything).Return([]*core.Identity{}, nil, nil).Once()

	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupMustExist", pm.ctx, "org1").Return(localOrg, false, nil)
	mim.On("GetRootOrg", pm.ctx).Return(localOrg, nil)
	mim.On("GetLocalNode", pm.ctx).Return(localNode, nil)

	err := pm.resolveRecipientList(pm.ctx, &core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				SignerRef: core.SignerRef{
					Author: "org1",
				},
				Namespace: "ns1",
			},
		},
		Group: &core.InputGroup{
			Members: []core.MemberInput{
				{Identity: "org1"},
			},
		},
	})
	assert.Regexp(t, "FF10233", err)
	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)

}

func TestResolveMemberNodeOwnedParentOrg(t *testing.T) {

	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	parentOrg := newTestOrg("localorg")
	childOrg := newTestOrg("org1")
	childOrg.Parent = parentOrg.ID
	localNode := newTestNode("node1", parentOrg)

	mdi := pm.database.(*databasemocks.Plugin)
	mdi.On("GetIdentities", pm.ctx, "ns1", mock.Anything).Return([]*core.Identity{}, nil, nil).Once()
	mdi.On("GetIdentities", pm.ctx, "ns1", mock.Anything).Return([]*core.Identity{localNode}, nil, nil)
	mdi.On("GetGroupByHash", pm.ctx, "ns1", mock.Anything, mock.Anything).Return(&core.Group{Hash: fftypes.NewRandB32()}, nil, nil).Once()
	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("GetRootOrg", pm.ctx).Return(parentOrg, nil)
	mim.On("GetLocalNode", pm.ctx).Return(localNode, nil)
	mim.On("CachedIdentityLookupMustExist", pm.ctx, "org1").Return(childOrg, false, nil)
	mim.On("CachedIdentityLookupByID", pm.ctx, parentOrg.ID).Return(parentOrg, nil)

	err := pm.resolveRecipientList(pm.ctx, &core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				SignerRef: core.SignerRef{
					Author: "org1",
				},
				Namespace: "ns1",
			},
		},
		Group: &core.InputGroup{
			Members: []core.MemberInput{
				{Identity: "org1"},
			},
		},
	})
	assert.NoError(t, err)
	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)

}

func TestGetNodeFail(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupMustExist", pm.ctx, "id-node1").Return(nil, true, fmt.Errorf("pop"))

	_, err := pm.resolveNode(pm.ctx, newTestOrg("org1"), "id-node1")
	assert.Regexp(t, "pop", err)
	mim.AssertExpectations(t)

}

func TestResolveNodeByIDNoResult(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	mdi := pm.database.(*databasemocks.Plugin)
	mdi.On("GetIdentities", pm.ctx, "ns1", mock.Anything).Return([]*core.Identity{}, nil, nil)

	parentOrgID := fftypes.NewUUID()
	mim := pm.identity.(*identitymanagermocks.Manager)
	mim.On("CachedIdentityLookupByID", pm.ctx, parentOrgID).Return(nil, nil)

	childOrg := newTestOrg("test1")
	childOrg.Parent = parentOrgID
	_, err := pm.resolveNode(pm.ctx, childOrg, "")
	assert.Regexp(t, "FF10224", err)
	mdi.AssertExpectations(t)

}

func TestResolveReceipientListExisting(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	groupID := fftypes.NewRandB32()
	mdi := pm.database.(*databasemocks.Plugin)
	mdi.On("GetGroupByHash", pm.ctx, "ns1", groupID).Return(&core.Group{Hash: groupID}, nil)

	err := pm.resolveRecipientList(pm.ctx, &core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				Group: groupID,
			},
		},
	})
	assert.NoError(t, err)
}

func TestResolveReceipientListEmptyList(t *testing.T) {
	pm, cancel := newTestPrivateMessaging(t)
	defer cancel()

	err := pm.resolveRecipientList(pm.ctx, &core.MessageInOut{})
	assert.Regexp(t, "FF00115", err)
}
