// Copyright © 2022 Kaleido, Inc.
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
package assets

import (
	"context"
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/internal/txcommon"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/mocks/tokenmocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestPrepareAndRunCreatePool(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	op := &core.Operation{
		Type:      core.OpTypeTokenCreatePool,
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	pool := &core.TokenPool{
		Connector: "magic-tokens",
		Locator:   "F1",
	}
	err := txcommon.AddTokenPoolCreateInputs(op, pool)
	assert.NoError(t, err)

	mti := am.tokens["magic-tokens"].(*tokenmocks.Plugin)
	mti.On("CreateTokenPool", context.Background(), "ns1:"+op.ID.String(), pool).Return(core.OpPhaseComplete, nil)

	po, err := am.PrepareOperation(context.Background(), op)
	assert.NoError(t, err)
	assert.Equal(t, pool, po.Data.(createPoolData).Pool)

	_, phase, err := am.RunOperation(context.Background(), po)

	assert.Equal(t, core.OpPhaseComplete, phase)
	assert.NoError(t, err)

	mti.AssertExpectations(t)
}

func TestPrepareAndRunActivatePool(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	op := &core.Operation{
		Type:      core.OpTypeTokenActivatePool,
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	pool := &core.TokenPool{
		Connector: "magic-tokens",
		ID:        fftypes.NewUUID(),
		Locator:   "F1",
	}
	txcommon.AddTokenPoolActivateInputs(op, pool.ID)

	mti := am.tokens["magic-tokens"].(*tokenmocks.Plugin)
	mdi := am.database.(*databasemocks.Plugin)
	mti.On("ActivateTokenPool", context.Background(), pool).Return(core.OpPhaseComplete, nil)
	mdi.On("GetTokenPoolByID", context.Background(), "ns1", pool.ID).Return(pool, nil)

	po, err := am.PrepareOperation(context.Background(), op)
	assert.NoError(t, err)
	assert.Equal(t, pool, po.Data.(activatePoolData).Pool)

	_, phase, err := am.RunOperation(context.Background(), po)

	assert.Equal(t, core.OpPhaseComplete, phase)
	assert.NoError(t, err)

	mti.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestPrepareAndRunTransfer(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	op := &core.Operation{
		Type:      core.OpTypeTokenTransfer,
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	pool := &core.TokenPool{
		Connector: "magic-tokens",
		Locator:   "F1",
	}
	transfer := &core.TokenTransfer{
		LocalID: fftypes.NewUUID(),
		Pool:    pool.ID,
		Type:    core.TokenTransferTypeTransfer,
	}
	txcommon.AddTokenTransferInputs(op, transfer)

	mti := am.tokens["magic-tokens"].(*tokenmocks.Plugin)
	mdi := am.database.(*databasemocks.Plugin)
	mti.On("TransferTokens", context.Background(), "ns1:"+op.ID.String(), "F1", transfer, (*fftypes.JSONAny)(nil)).Return(nil)
	mdi.On("GetTokenPoolByID", context.Background(), "ns1", pool.ID).Return(pool, nil)

	po, err := am.PrepareOperation(context.Background(), op)
	assert.NoError(t, err)
	assert.Equal(t, pool, po.Data.(transferData).Pool)
	assert.Equal(t, transfer, po.Data.(transferData).Transfer)

	_, phase, err := am.RunOperation(context.Background(), po)

	assert.Equal(t, core.OpPhasePending, phase)
	assert.NoError(t, err)

	mti.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestPrepareAndRunApproval(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	op := &core.Operation{
		Type:      core.OpTypeTokenApproval,
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	pool := &core.TokenPool{
		Connector: "magic-tokens",
		Locator:   "F1",
	}
	approval := &core.TokenApproval{
		LocalID:  fftypes.NewUUID(),
		Pool:     pool.ID,
		Approved: true,
	}
	txcommon.AddTokenApprovalInputs(op, approval)

	mti := am.tokens["magic-tokens"].(*tokenmocks.Plugin)
	mdi := am.database.(*databasemocks.Plugin)
	mti.On("TokensApproval", context.Background(), "ns1:"+op.ID.String(), "F1", approval, (*fftypes.JSONAny)(nil)).Return(nil)
	mdi.On("GetTokenPoolByID", context.Background(), "ns1", pool.ID).Return(pool, nil)

	po, err := am.PrepareOperation(context.Background(), op)
	assert.NoError(t, err)
	assert.Equal(t, pool, po.Data.(approvalData).Pool)
	assert.Equal(t, approval, po.Data.(approvalData).Approval)

	_, phase, err := am.RunOperation(context.Background(), po)

	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.NoError(t, err)

	mti.AssertExpectations(t)
	mdi.AssertExpectations(t)
}

func TestPrepareOperationNotSupported(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	po, err := am.PrepareOperation(context.Background(), &core.Operation{})

	assert.Nil(t, po)
	assert.Regexp(t, "FF10371", err)
}

func TestPrepareOperationCreatePoolBadInput(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	op := &core.Operation{
		Type:  core.OpTypeTokenCreatePool,
		Input: fftypes.JSONObject{"id": "bad"},
	}

	_, err := am.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF00127", err)
}

func TestPrepareOperationActivatePoolBadInput(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	op := &core.Operation{
		Type:  core.OpTypeTokenActivatePool,
		Input: fftypes.JSONObject{"id": "bad"},
	}

	_, err := am.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF00138", err)
}

func TestPrepareOperationActivatePoolError(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	poolID := fftypes.NewUUID()
	op := &core.Operation{
		Type:  core.OpTypeTokenActivatePool,
		Input: fftypes.JSONObject{"id": poolID.String()},
	}

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPoolByID", context.Background(), "ns1", poolID).Return(nil, fmt.Errorf("pop"))

	_, err := am.PrepareOperation(context.Background(), op)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestPrepareOperationActivatePoolNotFound(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	poolID := fftypes.NewUUID()
	op := &core.Operation{
		Type:  core.OpTypeTokenActivatePool,
		Input: fftypes.JSONObject{"id": poolID.String()},
	}

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPoolByID", context.Background(), "ns1", poolID).Return(nil, nil)

	_, err := am.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF10109", err)

	mdi.AssertExpectations(t)
}

func TestPrepareOperationTransferBadInput(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	op := &core.Operation{
		Type:  core.OpTypeTokenTransfer,
		Input: fftypes.JSONObject{"localId": "bad"},
	}

	_, err := am.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF00127", err)
}

func TestPrepareOperationTransferError(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	poolID := fftypes.NewUUID()
	op := &core.Operation{
		Type:  core.OpTypeTokenTransfer,
		Input: fftypes.JSONObject{"pool": poolID.String()},
	}

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPoolByID", context.Background(), "ns1", poolID).Return(nil, fmt.Errorf("pop"))

	_, err := am.PrepareOperation(context.Background(), op)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestPrepareOperationTransferNotFound(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	poolID := fftypes.NewUUID()
	op := &core.Operation{
		Type:  core.OpTypeTokenTransfer,
		Input: fftypes.JSONObject{"pool": poolID.String()},
	}

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPoolByID", context.Background(), "ns1", poolID).Return(nil, nil)

	_, err := am.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF10109", err)

	mdi.AssertExpectations(t)
}

func TestPrepareOperationApprovalBadInput(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	op := &core.Operation{
		Type:  core.OpTypeTokenApproval,
		Input: fftypes.JSONObject{"localId": "bad"},
	}

	_, err := am.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF00127", err)
}

func TestPrepareOperationApprovalError(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	poolID := fftypes.NewUUID()
	op := &core.Operation{
		Type:  core.OpTypeTokenApproval,
		Input: fftypes.JSONObject{"pool": poolID.String()},
	}

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPoolByID", context.Background(), "ns1", poolID).Return(nil, fmt.Errorf("pop"))

	_, err := am.PrepareOperation(context.Background(), op)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestPrepareOperationApprovalNotFound(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	poolID := fftypes.NewUUID()
	op := &core.Operation{
		Type:  core.OpTypeTokenApproval,
		Input: fftypes.JSONObject{"pool": poolID.String()},
	}

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPoolByID", context.Background(), "ns1", poolID).Return(nil, nil)

	_, err := am.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF10109", err)

	mdi.AssertExpectations(t)
}

func TestRunOperationNotSupported(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	_, phase, err := am.RunOperation(context.Background(), &core.PreparedOperation{})

	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.Regexp(t, "FF10378", err)
}

func TestRunOperationCreatePoolBadPlugin(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	op := &core.Operation{}
	pool := &core.TokenPool{}

	_, phase, err := am.RunOperation(context.Background(), opCreatePool(op, pool))

	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.Regexp(t, "FF10272", err)
}

func TestRunOperationCreatePool(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	op := &core.Operation{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	pool := &core.TokenPool{
		Connector: "magic-tokens",
	}

	mti := am.tokens["magic-tokens"].(*tokenmocks.Plugin)
	mti.On("CreateTokenPool", context.Background(), "ns1:"+op.ID.String(), pool).Return(core.OpPhaseInitializing, nil)

	_, phase, err := am.RunOperation(context.Background(), opCreatePool(op, pool))

	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.NoError(t, err)

	mti.AssertExpectations(t)
}

func TestRunOperationActivatePoolBadPlugin(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	op := &core.Operation{}
	pool := &core.TokenPool{}

	_, phase, err := am.RunOperation(context.Background(), opActivatePool(op, pool))

	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.Regexp(t, "FF10272", err)
}

func TestRunOperationTransferBadPlugin(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	op := &core.Operation{}
	pool := &core.TokenPool{}
	transfer := &core.TokenTransfer{}

	_, phase, err := am.RunOperation(context.Background(), opTransfer(op, pool, transfer))

	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.Regexp(t, "FF10272", err)
}

func TestRunOperationApprovalBadPlugin(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	op := &core.Operation{}
	pool := &core.TokenPool{}
	approval := &core.TokenApproval{}

	_, phase, err := am.RunOperation(context.Background(), opApproval(op, pool, approval))

	assert.Equal(t, core.OpPhaseInitializing, phase)
	assert.Regexp(t, "FF10272", err)
}

func TestRunOperationTransferUnknownType(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	op := &core.Operation{
		ID: fftypes.NewUUID(),
	}
	pool := &core.TokenPool{
		Connector: "magic-tokens",
	}
	transfer := &core.TokenTransfer{
		Type: "bad",
	}

	assert.PanicsWithValue(t, "unknown transfer type: bad", func() {
		am.RunOperation(context.Background(), opTransfer(op, pool, transfer))
	})
}

func TestRunOperationTransferMint(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	op := &core.Operation{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	pool := &core.TokenPool{
		Connector: "magic-tokens",
		Locator:   "F1",
	}
	transfer := &core.TokenTransfer{
		Type: core.TokenTransferTypeMint,
	}

	mti := am.tokens["magic-tokens"].(*tokenmocks.Plugin)
	mti.On("MintTokens", context.Background(), "ns1:"+op.ID.String(), "F1", transfer, (*fftypes.JSONAny)(nil)).Return(nil)

	_, phase, err := am.RunOperation(context.Background(), opTransfer(op, pool, transfer))

	assert.Equal(t, core.OpPhasePending, phase)
	assert.NoError(t, err)

	mti.AssertExpectations(t)
}

func TestRunOperationTransferMintWithInterface(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	op := &core.Operation{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	pool := &core.TokenPool{
		Connector: "magic-tokens",
		Locator:   "F1",
		Methods:   fftypes.JSONAnyPtr(`{"mint": "test_interface"}`),
	}
	transfer := &core.TokenTransfer{
		Type: core.TokenTransferTypeMint,
	}

	mti := am.tokens["magic-tokens"].(*tokenmocks.Plugin)
	mti.On("MintTokens", context.Background(), "ns1:"+op.ID.String(), "F1", transfer, pool.Methods).Return(nil)

	_, phase, err := am.RunOperation(context.Background(), opTransfer(op, pool, transfer))

	assert.Equal(t, core.OpPhasePending, phase)
	assert.NoError(t, err)

	mti.AssertExpectations(t)
}

func TestRunOperationTransferBurn(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	op := &core.Operation{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	pool := &core.TokenPool{
		Connector: "magic-tokens",
		Locator:   "F1",
	}
	transfer := &core.TokenTransfer{
		Type: core.TokenTransferTypeBurn,
	}

	mti := am.tokens["magic-tokens"].(*tokenmocks.Plugin)
	mti.On("BurnTokens", context.Background(), "ns1:"+op.ID.String(), "F1", transfer, (*fftypes.JSONAny)(nil)).Return(nil)

	_, phase, err := am.RunOperation(context.Background(), opTransfer(op, pool, transfer))

	assert.Equal(t, core.OpPhasePending, phase)
	assert.NoError(t, err)

	mti.AssertExpectations(t)
}

func TestRunOperationTransfer(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	op := &core.Operation{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	pool := &core.TokenPool{
		Connector: "magic-tokens",
		Locator:   "F1",
	}
	transfer := &core.TokenTransfer{
		Type: core.TokenTransferTypeTransfer,
	}

	mti := am.tokens["magic-tokens"].(*tokenmocks.Plugin)
	mti.On("TransferTokens", context.Background(), "ns1:"+op.ID.String(), "F1", transfer, (*fftypes.JSONAny)(nil)).Return(nil)

	_, phase, err := am.RunOperation(context.Background(), opTransfer(op, pool, transfer))

	assert.Equal(t, core.OpPhasePending, phase)
	assert.NoError(t, err)

	mti.AssertExpectations(t)
}

func TestOperationUpdatePool(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
	}
	op := &core.Operation{
		ID:   fftypes.NewUUID(),
		Type: core.OpTypeTokenCreatePool,
	}
	err := txcommon.AddTokenPoolCreateInputs(op, pool)
	assert.NoError(t, err)

	update := &core.OperationUpdate{
		Status: core.OpStatusFailed,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("InsertEvent", context.Background(), mock.MatchedBy(func(event *core.Event) bool {
		return event.Type == core.EventTypePoolOpFailed && *event.Reference == *op.ID && *event.Correlator == *pool.ID
	})).Return(nil)

	err = am.OnOperationUpdate(context.Background(), op, update)

	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestOperationUpdatePoolBadInput(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	op := &core.Operation{
		ID:   fftypes.NewUUID(),
		Type: core.OpTypeTokenCreatePool,
	}
	update := &core.OperationUpdate{
		Status: core.OpStatusFailed,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("InsertEvent", context.Background(), mock.MatchedBy(func(event *core.Event) bool {
		return event.Type == core.EventTypePoolOpFailed && *event.Reference == *op.ID && event.Correlator == nil
	})).Return(nil)

	err := am.OnOperationUpdate(context.Background(), op, update)

	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestOperationUpdatePoolEventFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	op := &core.Operation{
		ID:   fftypes.NewUUID(),
		Type: core.OpTypeTokenCreatePool,
	}
	update := &core.OperationUpdate{
		Status: core.OpStatusFailed,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("InsertEvent", context.Background(), mock.Anything).Return(fmt.Errorf("pop"))

	err := am.OnOperationUpdate(context.Background(), op, update)

	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestOperationUpdateTransfer(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	transfer := &core.TokenTransfer{
		LocalID: fftypes.NewUUID(),
		Pool:    fftypes.NewUUID(),
		Type:    core.TokenTransferTypeTransfer,
	}
	op := &core.Operation{
		ID:   fftypes.NewUUID(),
		Type: core.OpTypeTokenTransfer,
	}
	err := txcommon.AddTokenTransferInputs(op, transfer)
	assert.NoError(t, err)

	update := &core.OperationUpdate{
		Status: core.OpStatusFailed,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("InsertEvent", context.Background(), mock.MatchedBy(func(event *core.Event) bool {
		return event.Type == core.EventTypeTransferOpFailed && *event.Reference == *op.ID && *event.Correlator == *transfer.LocalID
	})).Return(nil)

	err = am.OnOperationUpdate(context.Background(), op, update)

	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestOperationUpdateTransferBadInput(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	op := &core.Operation{
		ID:   fftypes.NewUUID(),
		Type: core.OpTypeTokenTransfer,
	}
	update := &core.OperationUpdate{
		Status: core.OpStatusFailed,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("InsertEvent", context.Background(), mock.MatchedBy(func(event *core.Event) bool {
		return event.Type == core.EventTypeTransferOpFailed && *event.Reference == *op.ID && event.Correlator == nil
	})).Return(nil)

	err := am.OnOperationUpdate(context.Background(), op, update)

	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestOperationUpdateTransferEventFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	op := &core.Operation{
		ID:   fftypes.NewUUID(),
		Type: core.OpTypeTokenTransfer,
	}
	update := &core.OperationUpdate{
		Status: core.OpStatusFailed,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("InsertEvent", context.Background(), mock.Anything).Return(fmt.Errorf("pop"))

	err := am.OnOperationUpdate(context.Background(), op, update)

	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestOperationUpdateApproval(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	approval := &core.TokenApproval{
		LocalID: fftypes.NewUUID(),
		Pool:    fftypes.NewUUID(),
	}
	op := &core.Operation{
		ID:   fftypes.NewUUID(),
		Type: core.OpTypeTokenApproval,
	}
	err := txcommon.AddTokenApprovalInputs(op, approval)
	assert.NoError(t, err)

	update := &core.OperationUpdate{
		Status: core.OpStatusFailed,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("InsertEvent", context.Background(), mock.MatchedBy(func(event *core.Event) bool {
		return event.Type == core.EventTypeApprovalOpFailed && *event.Reference == *op.ID && *event.Correlator == *approval.LocalID
	})).Return(nil)

	err = am.OnOperationUpdate(context.Background(), op, update)

	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestOperationUpdateApprovalBadInput(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	op := &core.Operation{
		ID:   fftypes.NewUUID(),
		Type: core.OpTypeTokenApproval,
	}
	update := &core.OperationUpdate{
		Status: core.OpStatusFailed,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("InsertEvent", context.Background(), mock.MatchedBy(func(event *core.Event) bool {
		return event.Type == core.EventTypeApprovalOpFailed && *event.Reference == *op.ID && event.Correlator == nil
	})).Return(nil)

	err := am.OnOperationUpdate(context.Background(), op, update)

	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestOperationUpdateApprovalEventFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	op := &core.Operation{
		ID:   fftypes.NewUUID(),
		Type: core.OpTypeTokenApproval,
	}
	update := &core.OperationUpdate{
		Status: core.OpStatusFailed,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("InsertEvent", context.Background(), mock.Anything).Return(fmt.Errorf("pop"))

	err := am.OnOperationUpdate(context.Background(), op, update)

	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}
