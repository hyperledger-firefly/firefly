// Copyright © 2022 Kaleido, Inc.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in comdiliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or imdilied.
// See the License for the specific language governing permissions and
// limitations under the License.

package assets

import (
	"context"
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/internal/database/sqlcommon"
	"github.com/hyperledger-firefly/firefly/internal/identity"
	"github.com/hyperledger-firefly/firefly/internal/syncasync"
	"github.com/hyperledger-firefly/firefly/mocks/contractmocks"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/mocks/identitymanagermocks"
	"github.com/hyperledger-firefly/firefly/mocks/operationmocks"
	"github.com/hyperledger-firefly/firefly/mocks/syncasyncmocks"
	"github.com/hyperledger-firefly/firefly/mocks/tokenmocks"
	"github.com/hyperledger-firefly/firefly/mocks/txcommonmocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/hyperledger-firefly/firefly/pkg/tokens"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestCreateTokenPoolBadName(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPoolInput{}

	_, err := am.CreateTokenPool(context.Background(), pool, false)
	assert.Regexp(t, "FF00140", err)
}

func TestCreateTokenPoolGetError(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPoolInput{
		TokenPool: core.TokenPool{
			Name: "testpool",
		},
	}

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPool", context.Background(), "ns1", "testpool").Return(nil, fmt.Errorf("pop"))

	_, err := am.CreateTokenPool(context.Background(), pool, false)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestCreateTokenPoolDuplicateName(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPoolInput{
		TokenPool: core.TokenPool{
			Name: "testpool",
		},
	}

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPool", context.Background(), "ns1", "testpool").Return(&core.TokenPool{}, nil)

	_, err := am.CreateTokenPool(context.Background(), pool, false)
	assert.Regexp(t, "FF10275.*testpool", err)

	mdi.AssertExpectations(t)
}

func TestCreateTokenPoolDefaultConnectorSuccess(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPoolInput{
		TokenPool: core.TokenPool{
			Name: "testpool",
		},
		IdempotencyKey: "idem1",
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mdi.On("GetTokenPool", context.Background(), "ns1", "testpool").Return(nil, nil)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("resolved-key", nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenPool, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	mom.On("RunOperation", context.Background(), mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(createPoolData)
		return op.Type == core.OpTypeTokenCreatePool && data.Pool == &pool.TokenPool
	}), true).Return(nil, nil)

	_, err := am.CreateTokenPool(context.Background(), pool, false)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestCreateTokenPoolIdempotentResubmit(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()
	var id = fftypes.NewUUID()

	pool := &core.TokenPoolInput{
		TokenPool: core.TokenPool{
			Name: "testpool",
		},
		IdempotencyKey: "idem1",
	}

	op := &core.Operation{
		ID:     fftypes.NewUUID(),
		Plugin: "blockchain",
		Type:   core.OpTypeBlockchainPinBatch,
		Status: core.OpStatusFailed,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mdi.On("GetTokenPool", context.Background(), "ns1", "testpool").Return(nil, nil)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("resolved-key", nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenPool, core.IdempotencyKey("idem1")).
		Return(id, &sqlcommon.IdempotencyError{
			ExistingTXID:  id,
			OriginalError: i18n.NewError(context.Background(), coremsgs.MsgIdempotencyKeyDuplicateTransaction, "idem1", id)})
	mom.On("ResubmitOperations", context.Background(), id).Return(1, []*core.Operation{op}, nil)

	_, err := am.CreateTokenPool(context.Background(), pool, false)

	// SubmitNewTransction returned 409 idempotency clash, ResubmitOperations returned that it resubmitted an operation. Shouldn't
	// see the original 409 Conflict error
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestCreateTokenPoolIdempotentResubmitAll(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()
	var id = fftypes.NewUUID()

	pool := &core.TokenPoolInput{
		TokenPool: core.TokenPool{
			Name: "testpool",
		},
		IdempotencyKey: "idem1",
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mdi.On("GetTokenPool", context.Background(), "ns1", "testpool").Return(nil, nil)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("resolved-key", nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenPool, core.IdempotencyKey("idem1")).
		Return(id, &sqlcommon.IdempotencyError{
			ExistingTXID:  id,
			OriginalError: i18n.NewError(context.Background(), coremsgs.MsgIdempotencyKeyDuplicateTransaction, "idem1", id)})
	mom.On("ResubmitOperations", context.Background(), id).Return(0, nil, nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	mom.On("RunOperation", context.Background(), mock.Anything, true).Return(nil, nil)

	_, err := am.CreateTokenPool(context.Background(), pool, false)

	// SubmitNewTransction returned 409 idempotency clash, ResubmitOperations returned that it resubmitted an operation. Shouldn't
	// see the original 409 Conflict error
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestCreateTokenPoolIdempotentNoOperationToResubmit(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()
	var id = fftypes.NewUUID()

	pool := &core.TokenPoolInput{
		TokenPool: core.TokenPool{
			Name: "testpool",
		},
		IdempotencyKey: "idem1",
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mdi.On("GetTokenPool", context.Background(), "ns1", "testpool").Return(nil, nil)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("resolved-key", nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenPool, core.IdempotencyKey("idem1")).
		Return(id, &sqlcommon.IdempotencyError{
			ExistingTXID:  id,
			OriginalError: i18n.NewError(context.Background(), coremsgs.MsgIdempotencyKeyDuplicateTransaction, "idem1", id)})
	mom.On("ResubmitOperations", context.Background(), id).Return(1 /* total */, nil /* to resubmit */, nil)

	_, err := am.CreateTokenPool(context.Background(), pool, false)

	// SubmitNewTransction returned 409 idempotency clash, ResubmitOperations returned no resubmitted operations. Expect to
	// see the original 409 Conflict error
	assert.Error(t, err)
	assert.ErrorContains(t, err, "FF10431")

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestCreateTokenPoolIdempotentErrorOnOperationResubmit(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()
	var id = fftypes.NewUUID()

	pool := &core.TokenPoolInput{
		TokenPool: core.TokenPool{
			Name: "testpool",
		},
		IdempotencyKey: "idem1",
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mdi.On("GetTokenPool", context.Background(), "ns1", "testpool").Return(nil, nil)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("resolved-key", nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenPool, core.IdempotencyKey("idem1")).
		Return(id, &sqlcommon.IdempotencyError{
			ExistingTXID:  id,
			OriginalError: i18n.NewError(context.Background(), coremsgs.MsgIdempotencyKeyDuplicateTransaction, "idem1", id)})
	mom.On("ResubmitOperations", context.Background(), id).Return(-1, nil, fmt.Errorf("pop"))

	_, err := am.CreateTokenPool(context.Background(), pool, false)

	// SubmitNewTransction returned 409 idempotency clash, ResubmitOperations failed trying to resubmit an existing operation. Expect
	// to see the resubmit error
	assert.Error(t, err)
	assert.ErrorContains(t, err, "pop")

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestCreateTokenPoolDefaultConnectorNoConnectors(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPoolInput{
		TokenPool: core.TokenPool{
			Name: "testpool",
		},
	}

	am.tokens = make(map[string]tokens.Plugin)

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPool", context.Background(), "ns1", "testpool").Return(nil, nil)

	_, err := am.CreateTokenPool(context.Background(), pool, false)
	assert.Regexp(t, "FF10292", err)

	mdi.AssertExpectations(t)
}

func TestCreateTokenPoolDefaultConnectorMultipleConnectors(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPoolInput{
		TokenPool: core.TokenPool{
			Name: "testpool",
		},
	}

	am.tokens["magic-tokens"] = nil
	am.tokens["magic-tokens2"] = nil

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPool", context.Background(), "ns1", "testpool").Return(nil, nil)

	_, err := am.CreateTokenPool(context.Background(), pool, false)
	assert.Regexp(t, "FF10292", err)

	mdi.AssertExpectations(t)
}

func TestCreateTokenPoolNoConnectors(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()
	am.tokens = nil

	pool := &core.TokenPoolInput{
		TokenPool: core.TokenPool{
			Name: "testpool",
		},
	}

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPool", context.Background(), "ns1", "testpool").Return(nil, nil)

	_, err := am.CreateTokenPool(context.Background(), pool, false)
	assert.Regexp(t, "FF10292", err)

	mdi.AssertExpectations(t)
}

func TestCreateTokenPoolIdentityFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPoolInput{
		TokenPool: core.TokenPool{
			Name: "testpool",
		},
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mdi.On("GetTokenPool", context.Background(), "ns1", "testpool").Return(nil, nil)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("", fmt.Errorf("pop"))

	_, err := am.CreateTokenPool(context.Background(), pool, false)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
}

func TestCreateTokenPoolWrongConnector(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPoolInput{
		TokenPool: core.TokenPool{
			Connector: "wrongun",
			Name:      "testpool",
		},
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mdi.On("GetTokenPool", context.Background(), "ns1", "testpool").Return(nil, nil)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)

	_, err := am.CreateTokenPool(context.Background(), pool, false)
	assert.Regexp(t, "FF10272", err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
}

func TestCreateTokenPoolFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPoolInput{
		TokenPool: core.TokenPool{
			Connector: "magic-tokens",
			Name:      "testpool",
		},
		IdempotencyKey: "idem1",
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mdi.On("GetTokenPool", context.Background(), "ns1", "testpool").Return(nil, nil)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenPool, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	mom.On("RunOperation", context.Background(), mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(createPoolData)
		return op.Type == core.OpTypeTokenCreatePool && data.Pool == &pool.TokenPool
	}), true).Return(nil, fmt.Errorf("pop"))

	_, err := am.CreateTokenPool(context.Background(), pool, false)
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestCreateTokenPoolTransactionFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPoolInput{
		TokenPool: core.TokenPool{
			Connector: "magic-tokens",
			Name:      "testpool",
		},
		IdempotencyKey: "idem1",
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mdi.On("GetTokenPool", context.Background(), "ns1", "testpool").Return(nil, nil)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenPool, core.IdempotencyKey("idem1")).Return(nil, fmt.Errorf("pop"))

	_, err := am.CreateTokenPool(context.Background(), pool, false)
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
}

func TestCreateTokenPoolOpInsertFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPoolInput{
		TokenPool: core.TokenPool{
			Connector: "magic-tokens",
			Name:      "testpool",
		},
		IdempotencyKey: "idem1",
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mdi.On("GetTokenPool", context.Background(), "ns1", "testpool").Return(nil, nil)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenPool, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(fmt.Errorf("pop"))

	_, err := am.CreateTokenPool(context.Background(), pool, false)
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestCreateTokenPoolWithInterfaceFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPoolInput{
		TokenPool: core.TokenPool{
			Connector: "magic-tokens",
			Name:      "testpool",
			Interface: &fftypes.FFIReference{
				ID: fftypes.NewUUID(),
			},
		},
		IdempotencyKey: "idem1",
	}

	mdi := am.database.(*databasemocks.Plugin)
	mcm := am.contracts.(*contractmocks.Manager)
	mdi.On("GetTokenPool", context.Background(), "ns1", "testpool").Return(nil, nil)
	mcm.On("ResolveFFIReference", context.Background(), pool.Interface).Return(fmt.Errorf("pop"))

	_, err := am.CreateTokenPool(context.Background(), pool, false)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
	mcm.AssertExpectations(t)
}

func TestCreateTokenPoolSyncSuccess(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPoolInput{
		TokenPool: core.TokenPool{
			Connector: "magic-tokens",
			Name:      "testpool",
		},
		IdempotencyKey: "idem1",
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mdi.On("GetTokenPool", context.Background(), "ns1", "testpool").Return(nil, nil)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenPool, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	mom.On("RunOperation", context.Background(), mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(createPoolData)
		return op.Type == core.OpTypeTokenCreatePool && data.Pool == &pool.TokenPool
	}), true).Return(nil, nil)

	_, err := am.CreateTokenPool(context.Background(), pool, false)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestCreateTokenPoolAsyncSuccess(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPoolInput{
		TokenPool: core.TokenPool{
			Connector: "magic-tokens",
			Name:      "testpool",
		},
		IdempotencyKey: "idem1",
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mdi.On("GetTokenPool", context.Background(), "ns1", "testpool").Return(nil, nil)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenPool, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	mom.On("RunOperation", context.Background(), mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(createPoolData)
		return op.Type == core.OpTypeTokenCreatePool && data.Pool == &pool.TokenPool
	}), true).Return(nil, nil)

	_, err := am.CreateTokenPool(context.Background(), pool, false)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestCreateTokenPoolConfirm(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPoolInput{
		TokenPool: core.TokenPool{
			Connector: "magic-tokens",
			Name:      "testpool",
		},
		IdempotencyKey: "idem1",
	}

	mdi := am.database.(*databasemocks.Plugin)
	msa := am.syncasync.(*syncasyncmocks.Bridge)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mdi.On("GetTokenPool", context.Background(), "ns1", "testpool").Return(nil, nil)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenPool, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	msa.On("WaitForTokenPool", context.Background(), mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			send := args[2].(syncasync.SendFunction)
			send(context.Background())
		}).
		Return(nil, nil)
	mom.On("RunOperation", context.Background(), mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(createPoolData)
		return op.Type == core.OpTypeTokenCreatePool && data.Pool == &pool.TokenPool
	}), true).Return(nil, nil)

	_, err := am.CreateTokenPool(context.Background(), pool, true)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
	msa.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestActivateTokenPool(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Connector: "magic-tokens",
		TX: core.TransactionRef{
			ID: fftypes.NewUUID(),
		},
	}

	mom := am.operations.(*operationmocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mth.On("FindOperationInTransaction", context.Background(), pool.TX.ID, core.OpTypeTokenActivatePool).Return(nil, nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.MatchedBy(func(op *core.Operation) bool {
		return op.Type == core.OpTypeTokenActivatePool
	})).Return(nil)
	mom.On("RunOperation", context.Background(), mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(activatePoolData)
		return op.Type == core.OpTypeTokenActivatePool && data.Pool == pool
	}), false).Return(nil, nil)

	err := am.ActivateTokenPool(context.Background(), pool)
	assert.NoError(t, err)

	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestActivateTokenPoolBadConnector(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPool{
		Namespace: "ns1",
		Connector: "bad",
	}

	err := am.ActivateTokenPool(context.Background(), pool)
	assert.Regexp(t, "FF10272", err)
}

func TestActivateTokenPoolOpInsertFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPool{
		Namespace: "ns1",
		Connector: "magic-tokens",
		TX: core.TransactionRef{
			ID: fftypes.NewUUID(),
		},
	}

	mom := am.operations.(*operationmocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mth.On("FindOperationInTransaction", context.Background(), pool.TX.ID, core.OpTypeTokenActivatePool).Return(nil, nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.MatchedBy(func(op *core.Operation) bool {
		return op.Type == core.OpTypeTokenActivatePool
	})).Return(fmt.Errorf("pop"))

	err := am.ActivateTokenPool(context.Background(), pool)
	assert.EqualError(t, err, "pop")

	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestActivateTokenPoolFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPool{
		Namespace: "ns1",
		Connector: "magic-tokens",
		TX: core.TransactionRef{
			ID: fftypes.NewUUID(),
		},
	}

	mom := am.operations.(*operationmocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mth.On("FindOperationInTransaction", context.Background(), pool.TX.ID, core.OpTypeTokenActivatePool).Return(nil, nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.MatchedBy(func(op *core.Operation) bool {
		return op.Type == core.OpTypeTokenActivatePool
	})).Return(nil)
	mom.On("RunOperation", context.Background(), mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(activatePoolData)
		return op.Type == core.OpTypeTokenActivatePool && data.Pool == pool
	}), false).Return(nil, fmt.Errorf("pop"))

	err := am.ActivateTokenPool(context.Background(), pool)
	assert.EqualError(t, err, "pop")

	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestActivateTokenPoolExisting(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPool{
		Namespace: "ns1",
		Connector: "magic-tokens",
		TX: core.TransactionRef{
			ID: fftypes.NewUUID(),
		},
	}

	mth := am.txHelper.(*txcommonmocks.Helper)
	mth.On("FindOperationInTransaction", context.Background(), pool.TX.ID, core.OpTypeTokenActivatePool).Return(&core.Operation{}, nil)

	err := am.ActivateTokenPool(context.Background(), pool)
	assert.NoError(t, err)

	mth.AssertExpectations(t)
}

func TestActivateTokenPoolExistingFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPool{
		Namespace: "ns1",
		Connector: "magic-tokens",
		TX: core.TransactionRef{
			ID: fftypes.NewUUID(),
		},
	}

	mth := am.txHelper.(*txcommonmocks.Helper)
	mth.On("FindOperationInTransaction", context.Background(), pool.TX.ID, core.OpTypeTokenActivatePool).Return(nil, fmt.Errorf("pop"))

	err := am.ActivateTokenPool(context.Background(), pool)
	assert.EqualError(t, err, "pop")

	mth.AssertExpectations(t)
}

func TestActivateTokenPoolSyncSuccess(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPool{
		Namespace: "ns1",
		Connector: "magic-tokens",
		TX: core.TransactionRef{
			ID: fftypes.NewUUID(),
		},
	}

	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mth.On("FindOperationInTransaction", context.Background(), pool.TX.ID, core.OpTypeTokenActivatePool).Return(nil, nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.MatchedBy(func(op *core.Operation) bool {
		return op.Type == core.OpTypeTokenActivatePool
	})).Return(nil)
	mom.On("RunOperation", context.Background(), mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(activatePoolData)
		return op.Type == core.OpTypeTokenActivatePool && data.Pool == pool
	}), false).Return(nil, nil)

	err := am.ActivateTokenPool(context.Background(), pool)
	assert.NoError(t, err)

	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestGetTokenPoolByLocator(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPools", context.Background(), "ns1", mock.MatchedBy(func(filter ffapi.AndFilter) bool {
		f, err := filter.Finalize()
		assert.NoError(t, err)
		assert.Len(t, f.Children, 2)
		assert.Equal(t, "connector", f.Children[0].Field)
		val, _ := f.Children[0].Value.Value()
		assert.Equal(t, "magic-tokens", val)
		assert.Equal(t, "locator", f.Children[1].Field)
		val, _ = f.Children[1].Value.Value()
		assert.Equal(t, "abc", val)
		return true
	})).Return([]*core.TokenPool{{}}, nil, nil)
	result, err := am.GetTokenPoolByLocator(context.Background(), "magic-tokens", "abc")
	assert.NoError(t, err)
	assert.NotNil(t, result)

	mdi.AssertExpectations(t)
}

func TestGetTokenPoolByLocatorBadPlugin(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	am.tokens = make(map[string]tokens.Plugin)
	mdi := am.database.(*databasemocks.Plugin)
	_, err := am.GetTokenPoolByLocator(context.Background(), "magic-tokens", "abc")
	assert.Regexp(t, "FF10272", err)

	mdi.AssertExpectations(t)
}

func TestGetTokenPoolByLocatorCached(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	mdi := am.database.(*databasemocks.Plugin)
	am.cache.Set("ns=ns1,connector=magic-tokens,poollocator=abc", &core.TokenPool{})
	_, err := am.GetTokenPoolByLocator(context.Background(), "magic-tokens", "abc")
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestGetTokenPoolByLocatorNotFound(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPools", context.Background(), "ns1", mock.Anything).Return([]*core.TokenPool{}, nil, nil)
	result, err := am.GetTokenPoolByLocator(context.Background(), "magic-tokens", "abc")
	assert.NoError(t, err)
	assert.Nil(t, result)

	mdi.AssertExpectations(t)
}

func TestGetTokenPoolByLocatorFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPools", context.Background(), "ns1", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))
	_, err := am.GetTokenPoolByLocator(context.Background(), "magic-tokens", "abc")
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestGetTokenPoolByID(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	u := fftypes.NewUUID()
	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPoolByID", context.Background(), "ns1", u).Return(&core.TokenPool{}, nil)
	_, err := am.GetTokenPoolByNameOrID(context.Background(), u.String())
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestGetTokenPoolByIDCached(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	u := fftypes.NewUUID()
	mdi := am.database.(*databasemocks.Plugin)

	am.cache.Set(fmt.Sprintf("ns=ns1,poolnameorid=%s", u.String()), &core.TokenPool{})
	_, err := am.GetTokenPoolByNameOrID(context.Background(), u.String())
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestGetTokenPoolByIDBadNamespace(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	_, err := am.GetTokenPoolByNameOrID(context.Background(), "")
	assert.Regexp(t, "FF00140", err)
}

func TestGetTokenPoolByIDBadID(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	u := fftypes.NewUUID()
	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPoolByID", context.Background(), "ns1", u).Return(nil, fmt.Errorf("pop"))
	_, err := am.GetTokenPoolByNameOrID(context.Background(), u.String())
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestGetTokenPoolByIDNilPool(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	u := fftypes.NewUUID()
	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPoolByID", context.Background(), "ns1", u).Return(nil, nil)
	_, err := am.GetTokenPoolByNameOrID(context.Background(), u.String())
	assert.Regexp(t, "FF10109", err)

	mdi.AssertExpectations(t)
}

func TestGetTokenPoolByName(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPool", context.Background(), "ns1", "abc").Return(&core.TokenPool{}, nil)
	_, err := am.GetTokenPoolByNameOrID(context.Background(), "abc")
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestGetTokenPoolByNameBadName(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	_, err := am.GetTokenPoolByNameOrID(context.Background(), "")
	assert.Regexp(t, "FF00140", err)
}

func TestGetTokenPoolByNameNilPool(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPool", context.Background(), "ns1", "abc").Return(nil, fmt.Errorf("pop"))
	_, err := am.GetTokenPoolByNameOrID(context.Background(), "abc")
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestGetTokenPools(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	u := fftypes.NewUUID()
	mdi := am.database.(*databasemocks.Plugin)
	fb := database.TokenPoolQueryFactory.NewFilter(context.Background())
	f := fb.And(fb.Eq("id", u))
	mdi.On("GetTokenPools", context.Background(), "ns1", f).Return([]*core.TokenPool{}, nil, nil)
	_, _, err := am.GetTokenPools(context.Background(), f)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestResolvePoolMethods(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPool{
		Connector: "magic-tokens",
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
	}
	methods := []*fftypes.FFIMethod{{Name: "method1"}}
	resolved := fftypes.JSONAnyPtr(`"resolved"`)

	mcm := am.contracts.(*contractmocks.Manager)
	mcm.On("GetFFIMethods", context.Background(), pool.Interface.ID).Return(methods, nil)

	mti := am.tokens["magic-tokens"].(*tokenmocks.Plugin)
	mti.On("CheckInterface", context.Background(), pool, methods).Return(resolved, nil)

	err := am.ResolvePoolMethods(context.Background(), pool)
	assert.NoError(t, err)
	assert.Equal(t, resolved, pool.Methods)

	mcm.AssertExpectations(t)
	mti.AssertExpectations(t)
}

func TestDeletePoolNotFound(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(nil, nil)

	err := am.DeleteTokenPool(context.Background(), "pool1")
	assert.Regexp(t, "FF10109", err)

	mdi.AssertExpectations(t)
}

func TestDeletePoolFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Connector: "magic-tokens",
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
	}

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mdi.On("DeleteTokenPool", context.Background(), "ns1", pool.ID).Return(fmt.Errorf("pop"))

	err := am.DeleteTokenPool(context.Background(), "pool1")
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestDeletePoolPublished(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Connector: "magic-tokens",
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
		Published: true,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)

	err := am.DeleteTokenPool(context.Background(), "pool1")
	assert.Regexp(t, "FF10449", err)

	mdi.AssertExpectations(t)
}

func TestDeletePoolTransfersFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Connector: "magic-tokens",
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
	}

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mdi.On("DeleteTokenPool", context.Background(), "ns1", pool.ID).Return(nil)
	mdi.On("DeleteTokenTransfers", context.Background(), "ns1", pool.ID).Return(fmt.Errorf("pop"))

	err := am.DeleteTokenPool(context.Background(), "pool1")
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestDeletePoolApprovalsFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Connector: "magic-tokens",
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
	}

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mdi.On("DeleteTokenPool", context.Background(), "ns1", pool.ID).Return(nil)
	mdi.On("DeleteTokenTransfers", context.Background(), "ns1", pool.ID).Return(nil)
	mdi.On("DeleteTokenApprovals", context.Background(), "ns1", pool.ID).Return(fmt.Errorf("pop"))

	err := am.DeleteTokenPool(context.Background(), "pool1")
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestDeletePoolBalancesFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Connector: "magic-tokens",
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
	}

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mdi.On("DeleteTokenPool", context.Background(), "ns1", pool.ID).Return(nil)
	mdi.On("DeleteTokenTransfers", context.Background(), "ns1", pool.ID).Return(nil)
	mdi.On("DeleteTokenApprovals", context.Background(), "ns1", pool.ID).Return(nil)
	mdi.On("DeleteTokenBalances", context.Background(), "ns1", pool.ID).Return(fmt.Errorf("pop"))

	err := am.DeleteTokenPool(context.Background(), "pool1")
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestDeletePoolSuccess(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Connector: "magic-tokens",
		Interface: &fftypes.FFIReference{
			ID: fftypes.NewUUID(),
		},
	}

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mdi.On("DeleteTokenPool", context.Background(), "ns1", pool.ID).Return(nil)
	mdi.On("DeleteTokenTransfers", context.Background(), "ns1", pool.ID).Return(nil)
	mdi.On("DeleteTokenApprovals", context.Background(), "ns1", pool.ID).Return(nil)
	mdi.On("DeleteTokenBalances", context.Background(), "ns1", pool.ID).Return(nil)

	mti := am.tokens["magic-tokens"].(*tokenmocks.Plugin)
	mti.On("DeactivateTokenPool", context.Background(), pool).Return(nil)

	err := am.DeleteTokenPool(context.Background(), "pool1")
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
	mti.AssertExpectations(t)
}

func TestDeletePoolBadPlugin(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	pool := &core.TokenPool{
		ID:        fftypes.NewUUID(),
		Connector: "BAD",
	}

	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)

	err := am.DeleteTokenPool(context.Background(), "pool1")
	assert.Regexp(t, "FF10272", err)

	mdi.AssertExpectations(t)
}
