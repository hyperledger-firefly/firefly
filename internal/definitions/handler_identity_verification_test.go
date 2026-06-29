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

func TestHandleDefinitionIdentityVerificationWithExistingClaimOk(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	ctx := context.Background()

	custom1, org1, claimMsg, claimData, verifyMsg, verifyData := testCustomClaimAndVerification(t)

	dh.mim.On("CachedIdentityLookupByID", ctx, org1.ID).Return(org1, nil)
	dh.mim.On("VerifyIdentityChain", ctx, mock.Anything).Return(custom1, false, nil)
	dh.mdi.On("GetMessageByID", ctx, "ns1", claimMsg.Header.ID).Return(nil, nil) // Simulate pending confirm in same pin batch
	dh.mdi.On("GetIdentityByName", ctx, custom1.Type, custom1.Namespace, custom1.Name).Return(nil, nil)
	dh.mdi.On("GetIdentityByID", ctx, "ns1", custom1.ID).Return(nil, nil)
	dh.mdi.On("GetVerifierByValue", ctx, core.VerifierTypeEthAddress, "ns1", "0x12345").Return(nil, nil)
	dh.mdi.On("UpsertIdentity", ctx, mock.MatchedBy(func(identity *core.Identity) bool {
		assert.Equal(t, *claimMsg.Header.ID, *identity.Messages.Claim)
		assert.Equal(t, *verifyMsg.Header.ID, *identity.Messages.Verification)
		return true
	}), database.UpsertOptimizationNew).Return(nil)
	dh.mdi.On("UpsertVerifier", ctx, mock.MatchedBy(func(verifier *core.Verifier) bool {
		assert.Equal(t, core.VerifierTypeEthAddress, verifier.Type)
		assert.Equal(t, "0x12345", verifier.Value)
		assert.Equal(t, *custom1.ID, *verifier.Identity)
		return true
	}), database.UpsertOptimizationNew).Return(nil)
	dh.mdi.On("InsertEvent", mock.Anything, mock.MatchedBy(func(event *core.Event) bool {
		return event.Type == core.EventTypeIdentityConfirmed
	})).Return(nil)

	dh.mdm.On("GetMessageDataCached", ctx, mock.Anything).Return(core.DataArray{claimData}, true, nil)

	dh.multiparty = true

	bs.AddPendingConfirm(claimMsg.Header.ID, claimMsg)

	action, err := dh.HandleDefinitionBroadcast(ctx, &bs.BatchState, verifyMsg, core.DataArray{verifyData}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionConfirm}, action)
	assert.NoError(t, err)
	assert.Equal(t, bs.ConfirmedDIDClaims, []string{custom1.DID})

	err = bs.RunFinalize(ctx)
	assert.NoError(t, err)
}

func TestHandleDefinitionIdentityVerificationIncompleteClaimData(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	ctx := context.Background()

	_, org1, claimMsg, _, verifyMsg, verifyData := testCustomClaimAndVerification(t)
	claimMsg.State = core.MessageStateConfirmed

	dh.mim.On("CachedIdentityLookupByID", ctx, org1.ID).Return(org1, nil)
	dh.mdi.On("GetMessageByID", ctx, "ns1", claimMsg.Header.ID).Return(claimMsg, nil)
	dh.mdm.On("GetMessageDataCached", ctx, mock.Anything).Return(core.DataArray{}, false, nil)

	action, err := dh.HandleDefinitionBroadcast(ctx, &bs.BatchState, verifyMsg, core.DataArray{verifyData}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionConfirm}, action)
	assert.NoError(t, err)

	bs.assertNoFinalizers()
}

func TestHandleDefinitionIdentityVerificationClaimDataFail(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	ctx := context.Background()

	_, org1, claimMsg, _, verifyMsg, verifyData := testCustomClaimAndVerification(t)
	claimMsg.State = core.MessageStateConfirmed

	dh.mim.On("CachedIdentityLookupByID", ctx, org1.ID).Return(org1, nil)
	dh.mdi.On("GetMessageByID", ctx, "ns1", claimMsg.Header.ID).Return(claimMsg, nil)
	dh.mdm.On("GetMessageDataCached", ctx, mock.Anything).Return(nil, false, fmt.Errorf("pop"))

	action, err := dh.HandleDefinitionBroadcast(ctx, &bs.BatchState, verifyMsg, core.DataArray{verifyData}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionRetry}, action)
	assert.Regexp(t, "pop", err)

	bs.assertNoFinalizers()
}

func TestHandleDefinitionIdentityVerificationClaimHashMismatchl(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	ctx := context.Background()

	_, org1, claimMsg, _, verifyMsg, verifyData := testCustomClaimAndVerification(t)
	claimMsg.State = core.MessageStateConfirmed
	claimMsg.Hash = fftypes.NewRandB32()

	dh.mim.On("CachedIdentityLookupByID", ctx, org1.ID).Return(org1, nil)
	dh.mdi.On("GetMessageByID", ctx, "ns1", claimMsg.Header.ID).Return(claimMsg, nil)

	action, err := dh.HandleDefinitionBroadcast(ctx, &bs.BatchState, verifyMsg, core.DataArray{verifyData}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionReject}, action)
	assert.Error(t, err)

	bs.assertNoFinalizers()
}

func TestHandleDefinitionIdentityVerificationBeforeClaim(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	ctx := context.Background()

	_, org1, claimMsg, _, verifyMsg, verifyData := testCustomClaimAndVerification(t)

	dh.mim.On("CachedIdentityLookupByID", ctx, org1.ID).Return(org1, nil)
	dh.mdi.On("GetMessageByID", ctx, "ns1", claimMsg.Header.ID).Return(nil, nil)

	action, err := dh.HandleDefinitionBroadcast(ctx, &bs.BatchState, verifyMsg, core.DataArray{verifyData}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionConfirm}, action)
	assert.NoError(t, err)

	bs.assertNoFinalizers()
}

func TestHandleDefinitionIdentityVerificationClaimLookupFail(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	ctx := context.Background()

	_, org1, claimMsg, _, verifyMsg, verifyData := testCustomClaimAndVerification(t)

	dh.mim.On("CachedIdentityLookupByID", ctx, org1.ID).Return(org1, nil)
	dh.mdi.On("GetMessageByID", ctx, "ns1", claimMsg.Header.ID).Return(nil, fmt.Errorf("pop"))

	action, err := dh.HandleDefinitionBroadcast(ctx, &bs.BatchState, verifyMsg, core.DataArray{verifyData}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionRetry}, action)
	assert.Regexp(t, "pop", err)

	bs.assertNoFinalizers()
}

func TestHandleDefinitionIdentityVerificationWrongSigner(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	ctx := context.Background()

	_, org1, _, _, verifyMsg, verifyData := testCustomClaimAndVerification(t)
	verifyMsg.Header.Author = "wrong"

	dh.mim.On("CachedIdentityLookupByID", ctx, org1.ID).Return(org1, nil)

	action, err := dh.HandleDefinitionBroadcast(ctx, &bs.BatchState, verifyMsg, core.DataArray{verifyData}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionReject}, action)
	assert.Error(t, err)

	bs.assertNoFinalizers()
}

func TestHandleDefinitionIdentityVerificationCheckParentNotFound(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	ctx := context.Background()

	_, org1, _, _, verifyMsg, verifyData := testCustomClaimAndVerification(t)

	dh.mim.On("CachedIdentityLookupByID", ctx, org1.ID).Return(nil, nil)

	action, err := dh.HandleDefinitionBroadcast(ctx, &bs.BatchState, verifyMsg, core.DataArray{verifyData}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionReject}, action)
	assert.Error(t, err)

	bs.assertNoFinalizers()
}

func TestHandleDefinitionIdentityVerificationCheckParentFail(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	ctx := context.Background()

	_, org1, _, _, verifyMsg, verifyData := testCustomClaimAndVerification(t)

	dh.mim.On("CachedIdentityLookupByID", ctx, org1.ID).Return(nil, fmt.Errorf("pop"))

	action, err := dh.HandleDefinitionBroadcast(ctx, &bs.BatchState, verifyMsg, core.DataArray{verifyData}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionRetry}, action)
	assert.Regexp(t, "pop", err)

	bs.assertNoFinalizers()
}

func TestHandleDefinitionIdentityVerificationInvalidPayload(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	ctx := context.Background()

	iv := core.IdentityVerification{
		Identity: testOrgIdentity(t, "org1").IdentityBase,
		// Missing message claim info
	}
	b, err := json.Marshal(&iv)
	assert.NoError(t, err)
	emptyObjectData := &core.Data{
		Value: fftypes.JSONAnyPtrBytes(b),
	}

	action, err := dh.HandleDefinitionBroadcast(ctx, &bs.BatchState, &core.Message{
		Header: core.MessageHeader{
			ID:   fftypes.NewUUID(),
			Type: core.MessageTypeBroadcast,
			Tag:  core.SystemTagIdentityVerification,
		},
	}, core.DataArray{emptyObjectData}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionReject}, action)
	assert.Error(t, err)

	bs.assertNoFinalizers()
}

func TestHandleDefinitionIdentityVerificationInvalidData(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	ctx := context.Background()

	action, err := dh.HandleDefinitionBroadcast(ctx, &bs.BatchState, &core.Message{
		Header: core.MessageHeader{
			ID:   fftypes.NewUUID(),
			Type: core.MessageTypeBroadcast,
			Tag:  core.SystemTagIdentityVerification,
		},
	}, core.DataArray{}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionReject}, action)
	assert.Error(t, err)

	bs.assertNoFinalizers()
}
