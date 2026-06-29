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
	"errors"
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func testIdentityUpdate(t *testing.T) (*core.Identity, *core.Message, *core.Data, *core.IdentityUpdate) {
	org1 := testOrgIdentity(t, "org1")
	org1.Parent = fftypes.NewUUID() // Not involved in verification for updates, just must not change

	iu := &core.IdentityUpdate{
		Identity: org1.IdentityBase,
		Updates: core.IdentityProfile{
			Profile: fftypes.JSONObject{
				"new": "profile",
			},
			Description: "new description",
		},
	}
	b, err := json.Marshal(&iu)
	assert.NoError(t, err)
	updateData := &core.Data{
		ID:    fftypes.NewUUID(),
		Value: fftypes.JSONAnyPtrBytes(b),
	}

	updateMsg := &core.Message{
		Header: core.MessageHeader{
			ID:     fftypes.NewUUID(),
			Type:   core.MessageTypeDefinition,
			Tag:    core.SystemTagIdentityUpdate,
			Topics: fftypes.FFStringArray{org1.Topic()},
			SignerRef: core.SignerRef{
				Author: org1.DID,
				Key:    "0x12345",
			},
		},
	}

	return org1, updateMsg, updateData, iu
}

func testNodeIdentityUpdate(t *testing.T) (*core.Identity, *core.Identity, *core.Message, *core.Data, *core.IdentityUpdate) {
	org1 := testOrgIdentity(t, "org1")
	org1.Parent = fftypes.NewUUID() // Not involved in verification for updates, just must not change
	node1 := testNodeIdentity(t, "node1", org1)

	iu := &core.IdentityUpdate{
		Identity: node1.IdentityBase,
		Updates: core.IdentityProfile{
			Profile: fftypes.JSONObject{
				"new": "profile",
			},
			Description: "new description",
		},
	}
	b, err := json.Marshal(&iu)
	assert.NoError(t, err)
	updateData := &core.Data{
		ID:    fftypes.NewUUID(),
		Value: fftypes.JSONAnyPtrBytes(b),
	}

	updateMsg := &core.Message{
		Header: core.MessageHeader{
			ID:     fftypes.NewUUID(),
			Type:   core.MessageTypeDefinition,
			Tag:    core.SystemTagIdentityUpdate,
			Topics: fftypes.FFStringArray{node1.Topic()},
			SignerRef: core.SignerRef{
				Author: org1.DID,
				Key:    "0x12345",
			},
		},
	}

	return org1, node1, updateMsg, updateData, iu
}

func TestHandleDefinitionIdentityUpdateOk(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	ctx := context.Background()

	org1, updateMsg, updateData, iu := testIdentityUpdate(t)

	dh.mim.On("CachedIdentityLookupByID", ctx, org1.ID).Return(org1, nil)
	dh.mdi.On("UpsertIdentity", ctx, mock.MatchedBy(func(identity *core.Identity) bool {
		assert.Equal(t, *updateMsg.Header.ID, *identity.Messages.Update)
		assert.Equal(t, org1.IdentityBase, identity.IdentityBase)
		assert.Equal(t, iu.Updates, identity.IdentityProfile)
		return true
	}), database.UpsertOptimizationExisting).Return(nil)
	dh.mdi.On("InsertEvent", mock.Anything, mock.MatchedBy(func(event *core.Event) bool {
		return event.Type == core.EventTypeIdentityUpdated
	})).Return(nil)

	action, err := dh.HandleDefinitionBroadcast(ctx, &bs.BatchState, updateMsg, core.DataArray{updateData}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionConfirm}, action)
	assert.NoError(t, err)

	err = bs.RunFinalize(ctx)
	assert.NoError(t, err)
}

func TestHandleDefinitionIdentityUpdateUpsertFail(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	ctx := context.Background()

	org1, updateMsg, updateData, _ := testIdentityUpdate(t)

	dh.mim.On("CachedIdentityLookupByID", ctx, org1.ID).Return(org1, nil)
	dh.mdi.On("UpsertIdentity", ctx, mock.Anything, database.UpsertOptimizationExisting).Return(fmt.Errorf("pop"))

	action, err := dh.HandleDefinitionBroadcast(ctx, &bs.BatchState, updateMsg, core.DataArray{updateData}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionRetry}, action)
	assert.Regexp(t, "pop", err)

	bs.assertNoFinalizers()
}

func TestHandleDefinitionIdentityInvalidIdentity(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	ctx := context.Background()
	dh.multiparty = true

	org1, updateMsg, updateData, _ := testIdentityUpdate(t)
	updateMsg.Header.Author = "wrong"

	dh.mim.On("CachedIdentityLookupByID", ctx, org1.ID).Return(org1, nil)
	dh.mim.On("VerifyIdentityChain", ctx, mock.Anything).Return(nil, false, nil)

	action, err := dh.HandleDefinitionBroadcast(ctx, &bs.BatchState, updateMsg, core.DataArray{updateData}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionReject}, action)
	assert.Error(t, err)

	bs.assertNoFinalizers()
}

func TestHandleDefinitionVerifyFail(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	ctx := context.Background()
	dh.multiparty = true

	org1, updateMsg, updateData, _ := testIdentityUpdate(t)
	updateMsg.Header.Author = "wrong"

	dh.mim.On("CachedIdentityLookupByID", ctx, org1.ID).Return(org1, nil)
	dh.mim.On("VerifyIdentityChain", ctx, mock.Anything).Return(nil, true, fmt.Errorf("pop"))

	action, err := dh.HandleDefinitionBroadcast(ctx, &bs.BatchState, updateMsg, core.DataArray{updateData}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionRetry}, action)
	assert.Error(t, err)

	bs.assertNoFinalizers()
}

func TestHandleDefinitionVerifyWait(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	ctx := context.Background()
	dh.multiparty = true

	org1, updateMsg, updateData, _ := testIdentityUpdate(t)
	updateMsg.Header.Author = "wrong"

	dh.mim.On("CachedIdentityLookupByID", ctx, org1.ID).Return(org1, nil)
	dh.mim.On("VerifyIdentityChain", ctx, mock.Anything).Return(nil, false, fmt.Errorf("pop"))

	action, err := dh.HandleDefinitionBroadcast(ctx, &bs.BatchState, updateMsg, core.DataArray{updateData}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionWait}, action)
	assert.NoError(t, err)

	bs.assertNoFinalizers()
}

func TestHandleDefinitionIdentityNotFound(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	ctx := context.Background()

	org1, updateMsg, updateData, _ := testIdentityUpdate(t)

	dh.mim.On("CachedIdentityLookupByID", ctx, org1.ID).Return(nil, nil)

	action, err := dh.HandleDefinitionBroadcast(ctx, &bs.BatchState, updateMsg, core.DataArray{updateData}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionReject}, action)
	assert.Regexp(t, "FF10408", err)

	bs.assertNoFinalizers()
}

func TestHandleDefinitionIdentityLookupFail(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	ctx := context.Background()

	org1, updateMsg, updateData, _ := testIdentityUpdate(t)

	dh.mim.On("CachedIdentityLookupByID", ctx, org1.ID).Return(nil, fmt.Errorf("pop"))

	action, err := dh.HandleDefinitionBroadcast(ctx, &bs.BatchState, updateMsg, core.DataArray{updateData}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionRetry}, action)
	assert.Regexp(t, "pop", err)

	bs.assertNoFinalizers()
}

func TestHandleDefinitionIdentityValidateFail(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	ctx := context.Background()

	org1 := testOrgIdentity(t, "org1")
	iu := &core.IdentityUpdate{
		Identity: org1.IdentityBase,
	}
	iu.Identity.DID = "wrong"
	b, err := json.Marshal(&iu)
	assert.NoError(t, err)
	updateData := &core.Data{
		ID:    fftypes.NewUUID(),
		Value: fftypes.JSONAnyPtrBytes(b),
	}

	updateMsg := &core.Message{
		Header: core.MessageHeader{
			ID:     fftypes.NewUUID(),
			Type:   core.MessageTypeDefinition,
			Tag:    core.SystemTagIdentityUpdate,
			Topics: fftypes.FFStringArray{org1.Topic()},
			SignerRef: core.SignerRef{
				Author: org1.DID,
				Key:    "0x12345",
			},
		},
	}

	action, err := dh.HandleDefinitionBroadcast(ctx, &bs.BatchState, updateMsg, core.DataArray{updateData}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionReject}, action)
	assert.Error(t, err)

	bs.assertNoFinalizers()
}

func TestHandleDefinitionIdentityMissingData(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	ctx := context.Background()

	org1 := testOrgIdentity(t, "org1")
	updateMsg := &core.Message{
		Header: core.MessageHeader{
			ID:     fftypes.NewUUID(),
			Type:   core.MessageTypeDefinition,
			Tag:    core.SystemTagIdentityUpdate,
			Topics: fftypes.FFStringArray{org1.Topic()},
			SignerRef: core.SignerRef{
				Author: org1.DID,
				Key:    "0x12345",
			},
		},
	}

	action, err := dh.HandleDefinitionBroadcast(ctx, &bs.BatchState, updateMsg, core.DataArray{}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionReject}, action)
	assert.Error(t, err)

	bs.assertNoFinalizers()
}

func TestHandleDefinitionIdentityUpdateLocalNodeOk(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	ctx := context.Background()

	org1, node1, updateMsg, updateData, iu := testNodeIdentityUpdate(t)

	dh.mim.On("VerifyIdentityChain", ctx, node1).Return(org1, false, nil)
	dh.mim.On("CachedIdentityLookupByID", ctx, node1.ID).Return(node1, nil)
	dh.mdi.On("UpsertIdentity", ctx, mock.MatchedBy(func(identity *core.Identity) bool {
		assert.Equal(t, *updateMsg.Header.ID, *identity.Messages.Update)
		assert.Equal(t, node1.IdentityBase, identity.IdentityBase)
		assert.Equal(t, iu.Updates, identity.IdentityProfile)
		return true
	}), database.UpsertOptimizationExisting).Return(nil)
	dh.mdi.On("InsertEvent", mock.Anything, mock.MatchedBy(func(event *core.Event) bool {
		return event.Type == core.EventTypeIdentityUpdated
	})).Return(nil)
	dh.mim.On("GetLocalNodeDID", ctx).Return(node1.DID, nil)
	dh.mdx.On("CheckNodeIdentityStatus", ctx, node1).Return(errors.New("failed to check status but no worries"))

	dh.multiparty = true
	action, err := dh.HandleDefinitionBroadcast(ctx, &bs.BatchState, updateMsg, core.DataArray{updateData}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionConfirm}, action)
	assert.NoError(t, err)

	err = bs.RunFinalize(ctx)
	assert.NoError(t, err)
}

func TestHandleDefinitionIdentityUpdateLocalNodeMisconfigured(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	ctx := context.Background()

	org1, node1, updateMsg, updateData, iu := testNodeIdentityUpdate(t)

	dh.mim.On("VerifyIdentityChain", ctx, node1).Return(org1, false, nil)
	dh.mim.On("CachedIdentityLookupByID", ctx, node1.ID).Return(node1, nil)
	dh.mdi.On("UpsertIdentity", ctx, mock.MatchedBy(func(identity *core.Identity) bool {
		assert.Equal(t, *updateMsg.Header.ID, *identity.Messages.Update)
		assert.Equal(t, node1.IdentityBase, identity.IdentityBase)
		assert.Equal(t, iu.Updates, identity.IdentityProfile)
		return true
	}), database.UpsertOptimizationExisting).Return(nil)
	dh.mdi.On("InsertEvent", mock.Anything, mock.MatchedBy(func(event *core.Event) bool {
		return event.Type == core.EventTypeIdentityUpdated
	})).Return(nil)
	dh.mim.On("GetLocalNodeDID", ctx).Return(node1.DID, errors.New("no local node but somehow we got this far"))

	dh.multiparty = true
	action, err := dh.HandleDefinitionBroadcast(ctx, &bs.BatchState, updateMsg, core.DataArray{updateData}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionRetry}, action)
	assert.Error(t, err)
}
