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
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func newPoolDefinition() *core.TokenPoolDefinition {
	pool := &core.TokenPool{
		ID:          fftypes.NewUUID(),
		Namespace:   "ns1",
		Name:        "name1",
		NetworkName: "name1",
		Type:        core.TokenTypeFungible,
		Locator:     "12345",
		Symbol:      "COIN",
		TX: core.TransactionRef{
			Type: core.TransactionTypeTokenPool,
			ID:   fftypes.NewUUID(),
		},
		Connector: "remote1",
		Published: true,
	}
	return &core.TokenPoolDefinition{
		Pool: pool,
	}
}

func buildPoolDefinitionMessage(definition *core.TokenPoolDefinition) (*core.Message, core.DataArray, error) {
	msg := &core.Message{
		Header: core.MessageHeader{
			ID:  fftypes.NewUUID(),
			Tag: core.SystemTagDefinePool,
			SignerRef: core.SignerRef{
				Author: "firefly:org1",
			},
		},
	}
	b, err := json.Marshal(definition)
	if err != nil {
		return nil, nil, err
	}
	data := core.DataArray{{
		Value: fftypes.JSONAnyPtrBytes(b),
	}}
	return msg, data, nil
}

func TestHandleDefinitionBroadcastTokenPoolActivateOK(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)

	definition := newPoolDefinition()
	pool := definition.Pool
	msg, data, err := buildPoolDefinitionMessage(definition)
	assert.NoError(t, err)

	dh.mdi.On("InsertOrGetTokenPool", context.Background(), mock.MatchedBy(func(p *core.TokenPool) bool {
		return *p.ID == *pool.ID && p.Message == msg.Header.ID && p.Connector == "connector1"
	})).Return(nil, nil)
	dh.mam.On("ActivateTokenPool", context.Background(), mock.AnythingOfType("*core.TokenPool")).Return(nil)
	dh.mim.On("GetRootOrgDID", context.Background()).Return("firefly:org1", nil)

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, msg, data, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionWait, CustomCorrelator: pool.ID}, action)
	assert.NoError(t, err)

	err = bs.RunPreFinalize(context.Background())
	assert.NoError(t, err)
}

func TestHandleDefinitionBroadcastTokenPoolBadConnector(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)

	definition := newPoolDefinition()
	pool := definition.Pool
	pool.NetworkName = "//bad"
	msg, data, err := buildPoolDefinitionMessage(definition)
	assert.NoError(t, err)

	dh.mim.On("GetRootOrgDID", context.Background()).Return("firefly:org1", nil)

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, msg, data, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionReject, CustomCorrelator: pool.ID}, action)
	assert.Regexp(t, "FF10403", err)

	err = bs.RunPreFinalize(context.Background())
	assert.NoError(t, err)
}

func TestHandleDefinitionBroadcastTokenPoolNameExists(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)

	definition := newPoolDefinition()
	pool := definition.Pool
	msg, data, err := buildPoolDefinitionMessage(definition)
	assert.NoError(t, err)

	existing := &core.TokenPool{
		Name: "name1",
	}

	dh.mdi.On("InsertOrGetTokenPool", context.Background(), mock.MatchedBy(func(p *core.TokenPool) bool {
		return *p.ID == *pool.ID && p.Message == msg.Header.ID && p.Name == "name1"
	})).Return(existing, nil)
	dh.mdi.On("InsertOrGetTokenPool", context.Background(), mock.MatchedBy(func(p *core.TokenPool) bool {
		return *p.ID == *pool.ID && p.Message == msg.Header.ID && p.Name == "name1-1"
	})).Return(nil, nil)
	dh.mim.On("GetRootOrgDID", context.Background()).Return("firefly:org1", nil)

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, msg, data, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionWait, CustomCorrelator: pool.ID}, action)
	assert.NoError(t, err)
}

func TestHandleDefinitionLocalTokenPoolNameExists(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)

	definition := newPoolDefinition()
	pool := definition.Pool
	pool.Published = false

	existing := &core.TokenPool{
		Active: true,
		Name:   "name1",
	}

	dh.mdi.On("InsertOrGetTokenPool", context.Background(), mock.MatchedBy(func(p *core.TokenPool) bool {
		return *p.ID == *pool.ID && p.Name == "name1"
	})).Return(existing, nil)

	action, err := dh.handleTokenPoolDefinition(context.Background(), &bs.BatchState, pool, true)
	assert.Equal(t, HandlerResult{Action: core.ActionReject, CustomCorrelator: pool.ID}, action)
	assert.Error(t, err)

	bs.assertNoFinalizers()
}

func TestHandleDefinitionBroadcastTokenPoolNetworkNameExists(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)

	definition := newPoolDefinition()
	pool := definition.Pool
	msg, data, err := buildPoolDefinitionMessage(definition)
	assert.NoError(t, err)

	existing := &core.TokenPool{
		NetworkName: "name1",
	}

	dh.mdi.On("InsertOrGetTokenPool", context.Background(), mock.MatchedBy(func(p *core.TokenPool) bool {
		return *p.ID == *pool.ID && p.Message == msg.Header.ID
	})).Return(existing, nil)
	dh.mim.On("GetRootOrgDID", context.Background()).Return("firefly:org1", nil)

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, msg, data, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionReject, CustomCorrelator: pool.ID}, action)
	assert.Error(t, err)

	bs.assertNoFinalizers()
}

func TestHandleDefinitionBroadcastTokenPoolExistingConfirmed(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)

	definition := newPoolDefinition()
	pool := definition.Pool
	msg, data, err := buildPoolDefinitionMessage(definition)
	assert.NoError(t, err)

	existing := &core.TokenPool{
		ID:      pool.ID,
		Active:  true,
		Message: msg.Header.ID,
	}

	dh.mdi.On("InsertOrGetTokenPool", context.Background(), mock.MatchedBy(func(p *core.TokenPool) bool {
		return *p.ID == *pool.ID && p.Message == msg.Header.ID
	})).Return(existing, nil)
	dh.mim.On("GetRootOrgDID", context.Background()).Return("firefly:org1", nil)

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, msg, data, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionConfirm, CustomCorrelator: pool.ID}, action)
	assert.NoError(t, err)
}

func TestHandleDefinitionBroadcastTokenPoolExistingWaiting(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)

	definition := newPoolDefinition()
	pool := definition.Pool
	msg, data, err := buildPoolDefinitionMessage(definition)
	assert.NoError(t, err)

	existing := &core.TokenPool{
		ID:      pool.ID,
		Active:  false,
		Message: msg.Header.ID,
	}

	dh.mdi.On("InsertOrGetTokenPool", context.Background(), mock.MatchedBy(func(p *core.TokenPool) bool {
		return *p.ID == *pool.ID && p.Message == msg.Header.ID
	})).Return(existing, nil)
	dh.mim.On("GetRootOrgDID", context.Background()).Return("firefly:org1", nil)

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, msg, data, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionWait, CustomCorrelator: pool.ID}, action)
	assert.NoError(t, err)
}

func TestHandleDefinitionBroadcastTokenPoolExistingPublish(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)

	definition := newPoolDefinition()
	pool := definition.Pool
	msg, data, err := buildPoolDefinitionMessage(definition)
	assert.NoError(t, err)

	existing := &core.TokenPool{
		ID:     pool.ID,
		Active: true,
		Name:   "existing-pool",
	}
	newPool := *pool
	newPool.Name = existing.Name
	newPool.Connector = "connector1"
	newPool.Published = true
	newPool.Message = msg.Header.ID
	newPool.Active = false

	dh.mdi.On("InsertOrGetTokenPool", context.Background(), mock.MatchedBy(func(p *core.TokenPool) bool {
		return *p.ID == *pool.ID && p.Message == msg.Header.ID
	})).Return(existing, nil)
	dh.mdi.On("UpsertTokenPool", context.Background(), &newPool, database.UpsertOptimizationExisting).Return(nil)
	dh.mim.On("GetRootOrgDID", context.Background()).Return("firefly:org1", nil)

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, msg, data, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionConfirm, CustomCorrelator: pool.ID}, action)
	assert.NoError(t, err)
}

func TestHandleDefinitionBroadcastTokenPoolExistingPublishUpsertFail(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)

	definition := newPoolDefinition()
	pool := definition.Pool
	msg, data, err := buildPoolDefinitionMessage(definition)
	assert.NoError(t, err)

	existing := &core.TokenPool{
		ID:     pool.ID,
		Active: true,
		Name:   "existing-pool",
	}
	newPool := *pool
	newPool.Name = existing.Name
	newPool.Connector = "connector1"
	newPool.Published = true
	newPool.Message = msg.Header.ID
	newPool.Active = false

	dh.mdi.On("InsertOrGetTokenPool", context.Background(), mock.MatchedBy(func(p *core.TokenPool) bool {
		return *p.ID == *pool.ID && p.Message == msg.Header.ID
	})).Return(existing, nil)
	dh.mdi.On("UpsertTokenPool", context.Background(), &newPool, database.UpsertOptimizationExisting).Return(fmt.Errorf("pop"))
	dh.mim.On("GetRootOrgDID", context.Background()).Return("firefly:org1", nil)

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, msg, data, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionRetry, CustomCorrelator: pool.ID}, action)
	assert.EqualError(t, err, "pop")
}

func TestHandleDefinitionBroadcastTokenPoolExistingPublishOrgFail(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)

	definition := newPoolDefinition()
	pool := definition.Pool
	msg, data, err := buildPoolDefinitionMessage(definition)
	assert.NoError(t, err)

	existing := &core.TokenPool{
		ID:     pool.ID,
		Active: true,
		Name:   "existing-pool",
	}
	newPool := *pool
	newPool.Name = existing.Name
	newPool.Connector = "connector1"
	newPool.Published = true
	newPool.Message = msg.Header.ID
	newPool.Active = false

	dh.mim.On("GetRootOrgDID", context.Background()).Return("", fmt.Errorf("pop"))

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, msg, data, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionRetry}, action)
	assert.EqualError(t, err, "pop")
}

func TestHandleDefinitionBroadcastTokenPoolExistingPublishOrgMismatch(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)

	definition := newPoolDefinition()
	pool := definition.Pool
	msg, data, err := buildPoolDefinitionMessage(definition)
	assert.NoError(t, err)

	existing := &core.TokenPool{
		ID:     pool.ID,
		Active: true,
		Name:   "existing-pool",
	}
	newPool := *pool
	newPool.Name = existing.Name
	newPool.Connector = "connector1"
	newPool.Published = true
	newPool.Message = msg.Header.ID
	newPool.Active = false

	dh.mdi.On("InsertOrGetTokenPool", context.Background(), mock.MatchedBy(func(p *core.TokenPool) bool {
		return *p.ID == *pool.ID && p.Message == msg.Header.ID
	})).Return(existing, nil)
	dh.mim.On("GetRootOrgDID", context.Background()).Return("firefly:org2", nil)

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, msg, data, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionReject, CustomCorrelator: pool.ID}, action)
	assert.Regexp(t, "FF10407", err)
}

func TestHandleDefinitionBroadcastTokenPoolFailUpsert(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)

	definition := newPoolDefinition()
	pool := definition.Pool
	msg, data, err := buildPoolDefinitionMessage(definition)
	assert.NoError(t, err)

	dh.mdi.On("InsertOrGetTokenPool", context.Background(), mock.MatchedBy(func(p *core.TokenPool) bool {
		return *p.ID == *pool.ID && p.Message == msg.Header.ID
	})).Return(nil, fmt.Errorf("pop"))
	dh.mim.On("GetRootOrgDID", context.Background()).Return("firefly:org1", nil)

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, msg, data, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionRetry}, action)
	assert.EqualError(t, err, "pop")

	bs.assertNoFinalizers()
}

func TestHandleDefinitionBroadcastTokenPoolActivateFail(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)

	definition := newPoolDefinition()
	pool := definition.Pool
	msg, data, err := buildPoolDefinitionMessage(definition)
	assert.NoError(t, err)

	dh.mdi.On("InsertOrGetTokenPool", context.Background(), mock.MatchedBy(func(p *core.TokenPool) bool {
		return *p.ID == *pool.ID && p.Message == msg.Header.ID
	})).Return(nil, nil)
	dh.mam.On("ActivateTokenPool", context.Background(), mock.AnythingOfType("*core.TokenPool")).Return(fmt.Errorf("pop"))
	dh.mim.On("GetRootOrgDID", context.Background()).Return("firefly:org1", nil)

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, msg, data, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionWait, CustomCorrelator: pool.ID}, action)
	assert.NoError(t, err)

	err = bs.RunPreFinalize(context.Background())
	assert.EqualError(t, err, "pop")
}

func TestHandleDefinitionBroadcastTokenPoolValidateFail(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)

	definition := &core.TokenPoolDefinition{
		Pool: &core.TokenPool{},
	}
	msg, data, err := buildPoolDefinitionMessage(definition)
	assert.NoError(t, err)

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, msg, data, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionReject}, action)
	assert.Error(t, err)
	bs.assertNoFinalizers()
}

func TestHandleDefinitionBroadcastTokenPoolBadMessage(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)

	msg := &core.Message{
		Header: core.MessageHeader{
			ID:  fftypes.NewUUID(),
			Tag: core.SystemTagDefinePool,
		},
	}

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, msg, nil, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionReject}, action)
	assert.Error(t, err)
	bs.assertNoFinalizers()
}
