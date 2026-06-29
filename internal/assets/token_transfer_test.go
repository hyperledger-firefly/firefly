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
	"github.com/hyperledger-firefly/firefly/mocks/broadcastmocks"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/mocks/identitymanagermocks"
	"github.com/hyperledger-firefly/firefly/mocks/operationmocks"
	"github.com/hyperledger-firefly/firefly/mocks/privatemessagingmocks"
	"github.com/hyperledger-firefly/firefly/mocks/syncasyncmocks"
	"github.com/hyperledger-firefly/firefly/mocks/txcommonmocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestGetTokenTransfers(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	mdi := am.database.(*databasemocks.Plugin)
	fb := database.TokenTransferQueryFactory.NewFilter(context.Background())
	f := fb.And()
	mdi.On("GetTokenTransfers", context.Background(), "ns1", f).Return([]*core.TokenTransfer{}, nil, nil)
	_, _, err := am.GetTokenTransfers(context.Background(), f)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestGetTokenTransferByID(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	u := fftypes.NewUUID()
	mdi := am.database.(*databasemocks.Plugin)
	mdi.On("GetTokenTransferByID", context.Background(), "ns1", u).Return(&core.TokenTransfer{}, nil)
	_, err := am.GetTokenTransferByID(context.Background(), u.String())
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestGetTokenTransferByIDBadID(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	_, err := am.GetTokenTransferByID(context.Background(), "badUUID")
	assert.Regexp(t, "FF00138", err)
}

func TestMintTokensSuccess(t *testing.T) {
	am, cancel := newTestAssetsWithMetrics(t)
	defer cancel()

	mint := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
	}
	pool := &core.TokenPool{
		Connector: "magic-tokens",
		Active:    true,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	mom.On("RunOperation", context.Background(), mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(transferData)
		return op.Type == core.OpTypeTokenTransfer && data.Pool == pool && data.Transfer == &mint.TokenTransfer
	}), true).Return(nil, nil)

	_, err := am.MintTokens(context.Background(), mint, false)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestMintTokensIdempotentResubmit(t *testing.T) {
	am, cancel := newTestAssetsWithMetrics(t)
	defer cancel()
	var id = fftypes.NewUUID()

	mint := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
	}

	op := &core.Operation{}

	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(id, &sqlcommon.IdempotencyError{
		ExistingTXID:  id,
		OriginalError: i18n.NewError(context.Background(), coremsgs.MsgIdempotencyKeyDuplicateTransaction, "idem1", id)})
	mom.On("ResubmitOperations", context.Background(), id).Return(1, []*core.Operation{op}, nil)

	// If ResubmitOperations returns an operation it's because it found one to resubmit, so we return 2xx not 409, and don't expect an error
	_, err := am.MintTokens(context.Background(), mint, false)
	assert.NoError(t, err)

	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestMintTokensIdempotentResubmitAll(t *testing.T) {
	am, cancel := newTestAssetsWithMetrics(t)
	defer cancel()
	var id = fftypes.NewUUID()

	mint := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
	}
	pool := &core.TokenPool{
		Name:      "pool1",
		Connector: "magic-tokens",
		Active:    true,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(id, &sqlcommon.IdempotencyError{
		ExistingTXID:  id,
		OriginalError: i18n.NewError(context.Background(), coremsgs.MsgIdempotencyKeyDuplicateTransaction, "idem1", id)})
	mom.On("ResubmitOperations", context.Background(), id).Return(0, nil, nil)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	mom.On("RunOperation", context.Background(), mock.Anything, true).Return(nil, nil)

	// If ResubmitOperations returns an operation it's because it found one to resubmit, so we return 2xx not 409, and don't expect an error
	_, err := am.MintTokens(context.Background(), mint, false)
	assert.NoError(t, err)

	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestMintTokensIdempotentNoOperationToResubmit(t *testing.T) {
	am, cancel := newTestAssetsWithMetrics(t)
	defer cancel()
	var id = fftypes.NewUUID()

	mint := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
	}

	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(id, &sqlcommon.IdempotencyError{
		ExistingTXID:  id,
		OriginalError: i18n.NewError(context.Background(), coremsgs.MsgIdempotencyKeyDuplicateTransaction, "idem1", id)})
	mom.On("ResubmitOperations", context.Background(), id).Return(1 /* total */, nil /* to resumit */, nil)

	// If ResubmitOperations returns nil it's because there was no operation in initialized state, so we expect the regular 409 error back
	_, err := am.MintTokens(context.Background(), mint, false)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "FF10431")

	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestMintTokensIdempotentErrorOnResubmit(t *testing.T) {
	am, cancel := newTestAssetsWithMetrics(t)
	defer cancel()
	var id = fftypes.NewUUID()

	mint := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
	}

	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(id, &sqlcommon.IdempotencyError{
		ExistingTXID:  id,
		OriginalError: i18n.NewError(context.Background(), coremsgs.MsgIdempotencyKeyDuplicateTransaction, "idem1", id)})
	mom.On("ResubmitOperations", context.Background(), id).Return(-1, nil, fmt.Errorf("pop"))

	// If ResubmitOperations returns nil it's because there was no operation in initialized state, so we expect the regular 409 error back
	_, err := am.MintTokens(context.Background(), mint, false)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "pop")

	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestMintTokensBadConnector(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	mint := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
	}
	pool := &core.TokenPool{
		Connector: "bad",
		Active:    true,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)

	_, err := am.MintTokens(context.Background(), mint, false)
	assert.Regexp(t, "FF10272.*bad", err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
}

func TestMintTokenDefaultPoolSuccess(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	mint := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			Amount: *fftypes.NewFFBigInt(5),
		},
		IdempotencyKey: "idem1",
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	fb := database.TokenPoolQueryFactory.NewFilter(context.Background())
	f := fb.And()
	f.Limit(1).Count(true)
	tokenPools := []*core.TokenPool{
		{
			Name:      "pool1",
			Connector: "magic-tokens",
			Active:    true,
		},
	}
	totalCount := int64(1)
	filterResult := &ffapi.FilterResult{
		TotalCount: &totalCount,
	}
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mdi.On("GetTokenPools", context.Background(), "ns1", mock.MatchedBy((func(f ffapi.AndFilter) bool {
		info, _ := f.Finalize()
		return info.Count && info.Limit == 1
	}))).Return(tokenPools, filterResult, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	mom.On("RunOperation", context.Background(), mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(transferData)
		return op.Type == core.OpTypeTokenTransfer && data.Pool == tokenPools[0] && data.Transfer == &mint.TokenTransfer
	}), true).Return(nil, nil)

	_, err := am.MintTokens(context.Background(), mint, false)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestMintTokenDefaultPoolNoPools(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	mint := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			Amount: *fftypes.NewFFBigInt(5),
		},
		IdempotencyKey: "idem1",
	}

	mdi := am.database.(*databasemocks.Plugin)
	mth := am.txHelper.(*txcommonmocks.Helper)
	fb := database.TokenPoolQueryFactory.NewFilter(context.Background())
	f := fb.And()
	f.Limit(1).Count(true)
	tokenPools := []*core.TokenPool{}
	totalCount := int64(0)
	filterResult := &ffapi.FilterResult{
		TotalCount: &totalCount,
	}
	mdi.On("GetTokenPools", context.Background(), "ns1", mock.MatchedBy((func(f ffapi.AndFilter) bool {
		info, _ := f.Finalize()
		return info.Count && info.Limit == 1
	}))).Return(tokenPools, filterResult, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)

	_, err := am.MintTokens(context.Background(), mint, false)
	assert.Regexp(t, "FF10292", err)

	mdi.AssertExpectations(t)
	mth.AssertExpectations(t)
}

func TestMintTokenDefaultPoolMultiplePools(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	mint := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			Amount: *fftypes.NewFFBigInt(5),
		},
		IdempotencyKey: "idem1",
	}

	mdi := am.database.(*databasemocks.Plugin)
	mth := am.txHelper.(*txcommonmocks.Helper)
	fb := database.TokenPoolQueryFactory.NewFilter(context.Background())
	f := fb.And()
	f.Limit(1).Count(true)
	tokenPools := []*core.TokenPool{
		{
			Name: "pool1",
		},
		{
			Name: "pool2",
		},
	}
	totalCount := int64(2)
	filterResult := &ffapi.FilterResult{
		TotalCount: &totalCount,
	}
	mdi.On("GetTokenPools", context.Background(), "ns1", mock.MatchedBy((func(f ffapi.AndFilter) bool {
		info, _ := f.Finalize()
		return info.Count && info.Limit == 1
	}))).Return(tokenPools, filterResult, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)

	_, err := am.MintTokens(context.Background(), mint, false)
	assert.Regexp(t, "FF10292", err)

	mdi.AssertExpectations(t)
	mth.AssertExpectations(t)
}

func TestMintTokensGetPoolsError(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	mint := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			Amount: *fftypes.NewFFBigInt(5),
		},
		IdempotencyKey: "idem1",
	}

	mdi := am.database.(*databasemocks.Plugin)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mdi.On("GetTokenPools", context.Background(), "ns1", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)

	_, err := am.MintTokens(context.Background(), mint, false)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
	mth.AssertExpectations(t)
}

func TestMintTokensBadPool(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	mint := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
	}

	mdi := am.database.(*databasemocks.Plugin)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(nil, fmt.Errorf("pop"))
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)

	_, err := am.MintTokens(context.Background(), mint, false)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
	mth.AssertExpectations(t)
}

func TestMintTokensIdentityFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	mint := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
	}
	pool := &core.TokenPool{
		Connector: "magic-tokens",
		Active:    true,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("", fmt.Errorf("pop"))
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)

	_, err := am.MintTokens(context.Background(), mint, false)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
}

func TestMintTokensFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	mint := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
	}
	pool := &core.TokenPool{
		Connector: "magic-tokens",
		Active:    true,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	mom.On("RunOperation", context.Background(), mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(transferData)
		return op.Type == core.OpTypeTokenTransfer && data.Pool == pool && data.Transfer == &mint.TokenTransfer
	}), true).Return(nil, fmt.Errorf("pop"))

	_, err := am.MintTokens(context.Background(), mint, false)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestMintTokensOperationFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	mint := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
	}
	pool := &core.TokenPool{
		Locator:   "F1",
		Connector: "magic-tokens",
		Active:    true,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(fmt.Errorf("pop"))

	_, err := am.MintTokens(context.Background(), mint, false)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestMintTokensConfirm(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	mint := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
	}
	pool := &core.TokenPool{
		Connector: "magic-tokens",
		Active:    true,
	}

	mdi := am.database.(*databasemocks.Plugin)
	msa := am.syncasync.(*syncasyncmocks.Bridge)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	msa.On("WaitForTokenTransfer", context.Background(), mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			send := args[2].(syncasync.SendFunction)
			send(context.Background())
		}).
		Return(&core.TokenTransfer{}, nil)
	mom.On("RunOperation", context.Background(), mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(transferData)
		return op.Type == core.OpTypeTokenTransfer && data.Pool == pool && data.Transfer == &mint.TokenTransfer
	}), true).Return(nil, fmt.Errorf("pop"))

	_, err := am.MintTokens(context.Background(), mint, true)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
	msa.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestBurnTokensSuccess(t *testing.T) {
	am, cancel := newTestAssetsWithMetrics(t)
	defer cancel()

	burn := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
	}
	pool := &core.TokenPool{
		Connector: "magic-tokens",
		Active:    true,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	mom.On("RunOperation", context.Background(), mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(transferData)
		return op.Type == core.OpTypeTokenTransfer && data.Pool == pool && data.Transfer == &burn.TokenTransfer
	}), true).Return(nil, nil)

	_, err := am.BurnTokens(context.Background(), burn, false)
	assert.NoError(t, err)

	mim.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestBurnTokensIdentityFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	burn := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
	}
	pool := &core.TokenPool{
		Connector: "magic-tokens",
		Active:    true,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("", fmt.Errorf("pop"))
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)

	_, err := am.BurnTokens(context.Background(), burn, false)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
}

func TestBurnTokensConfirm(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	burn := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
	}
	pool := &core.TokenPool{
		Connector: "magic-tokens",
		Active:    true,
	}

	mdi := am.database.(*databasemocks.Plugin)
	msa := am.syncasync.(*syncasyncmocks.Bridge)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	msa.On("WaitForTokenTransfer", context.Background(), mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			send := args[2].(syncasync.SendFunction)
			send(context.Background())
		}).
		Return(&core.TokenTransfer{}, nil)
	mom.On("RunOperation", context.Background(), mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(transferData)
		return op.Type == core.OpTypeTokenTransfer && data.Pool == pool && data.Transfer == &burn.TokenTransfer
	}), true).Return(nil, nil)

	_, err := am.BurnTokens(context.Background(), burn, true)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
	msa.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestTransferTokensSuccess(t *testing.T) {
	am, cancel := newTestAssetsWithMetrics(t)
	defer cancel()

	transfer := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			From:   "A",
			To:     "B",
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
	}
	pool := &core.TokenPool{
		Connector: "magic-tokens",
		Active:    true,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	mom.On("RunOperation", context.Background(), mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(transferData)
		return op.Type == core.OpTypeTokenTransfer && data.Pool == pool && data.Transfer == &transfer.TokenTransfer
	}), true).Return(nil, nil)

	_, err := am.TransferTokens(context.Background(), transfer, false)
	assert.NoError(t, err)

	mim.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestTransferTokensUnconfirmedPool(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	transfer := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			From:   "A",
			To:     "B",
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
	}
	pool := &core.TokenPool{
		Locator:   "F1",
		Connector: "magic-tokens",
		Active:    false,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)

	_, err := am.TransferTokens(context.Background(), transfer, false)
	assert.Regexp(t, "FF10293", err)

	mdi.AssertExpectations(t)
	mth.AssertExpectations(t)
}

func TestTransferTokensIdentityFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	transfer := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			From:   "A",
			To:     "B",
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
	}
	pool := &core.TokenPool{
		Connector: "magic-tokens",
		Active:    true,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("", fmt.Errorf("pop"))
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)

	_, err := am.TransferTokens(context.Background(), transfer, false)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
}

func TestTransferTokensNoFromOrTo(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	transfer := &core.TokenTransferInput{
		Pool:           "pool1",
		IdempotencyKey: "idem1",
	}
	pool := &core.TokenPool{
		Connector: "magic-tokens",
		Active:    true,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)

	_, err := am.TransferTokens(context.Background(), transfer, false)
	assert.Regexp(t, "FF10280", err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
}

func TestTransferTokensTransactionFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	transfer := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			From:   "A",
			To:     "B",
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
	}

	mth := am.txHelper.(*txcommonmocks.Helper)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(nil, fmt.Errorf("pop"))

	_, err := am.TransferTokens(context.Background(), transfer, false)
	assert.EqualError(t, err, "pop")

	mth.AssertExpectations(t)
}

func TestTransferTokensWithBroadcastMessage(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	msgID := fftypes.NewUUID()
	hash := fftypes.NewRandB32()
	transfer := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			From:   "A",
			To:     "B",
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool: "pool1",
		Message: &core.MessageInOut{
			Message: core.Message{
				Header: core.MessageHeader{
					ID: msgID,
				},
				Hash: hash,
			},
			InlineData: core.InlineData{
				{
					Value: fftypes.JSONAnyPtr("test data"),
				},
			},
		},
		IdempotencyKey: "idem1",
	}
	pool := &core.TokenPool{
		Connector: "magic-tokens",
		Active:    true,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mbm := am.broadcast.(*broadcastmocks.Manager)
	mms := &syncasyncmocks.Sender{}
	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	mbm.On("NewBroadcast", transfer.Message).Return(mms)
	mms.On("Prepare", context.Background()).Return(nil)
	mms.On("Send", context.Background()).Return(nil)
	mom.On("RunOperation", context.Background(), mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(transferData)
		return op.Type == core.OpTypeTokenTransfer && data.Pool == pool && data.Transfer == &transfer.TokenTransfer
	}), true).Return(nil, nil)

	_, err := am.TransferTokens(context.Background(), transfer, false)
	assert.NoError(t, err)
	assert.Equal(t, *msgID, *transfer.TokenTransfer.Message)
	assert.Equal(t, *hash, *transfer.TokenTransfer.MessageHash)

	mbm.AssertExpectations(t)
	mim.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mms.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestTransferTokensWithBroadcastMessageDisabled(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()
	am.broadcast = nil

	msgID := fftypes.NewUUID()
	hash := fftypes.NewRandB32()
	transfer := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			From:   "A",
			To:     "B",
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
		Message: &core.MessageInOut{
			Message: core.Message{
				Header: core.MessageHeader{
					ID: msgID,
				},
				Hash: hash,
			},
			InlineData: core.InlineData{
				{
					Value: fftypes.JSONAnyPtr("test data"),
				},
			},
		},
	}

	mth := am.txHelper.(*txcommonmocks.Helper)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)

	_, err := am.TransferTokens(context.Background(), transfer, false)
	assert.Regexp(t, "FF10415", err)

	mth.AssertExpectations(t)
}

func TestTransferTokensWithBroadcastMessageSendFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	msgID := fftypes.NewUUID()
	hash := fftypes.NewRandB32()
	transfer := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			From:   "A",
			To:     "B",
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool: "pool1",
		Message: &core.MessageInOut{
			Message: core.Message{
				Header: core.MessageHeader{
					ID: msgID,
				},
				Hash: hash,
			},
			InlineData: core.InlineData{
				{
					Value: fftypes.JSONAnyPtr("test data"),
				},
			},
		},
		IdempotencyKey: "idem1",
	}
	pool := &core.TokenPool{
		Connector: "magic-tokens",
		Active:    true,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mbm := am.broadcast.(*broadcastmocks.Manager)
	mms := &syncasyncmocks.Sender{}
	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	mbm.On("NewBroadcast", transfer.Message).Return(mms)
	mms.On("Prepare", context.Background()).Return(nil)
	mms.On("Send", context.Background()).Return(fmt.Errorf("pop"))

	_, err := am.TransferTokens(context.Background(), transfer, false)
	assert.Regexp(t, "pop", err)
	assert.Equal(t, *msgID, *transfer.TokenTransfer.Message)
	assert.Equal(t, *hash, *transfer.TokenTransfer.MessageHash)

	mbm.AssertExpectations(t)
	mim.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mms.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestTransferTokensWithBroadcastPrepareFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	transfer := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			From:   "A",
			To:     "B",
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
		Message: &core.MessageInOut{
			InlineData: core.InlineData{
				{
					Value: fftypes.JSONAnyPtr("test data"),
				},
			},
		},
	}

	mbm := am.broadcast.(*broadcastmocks.Manager)
	mms := &syncasyncmocks.Sender{}
	mth := am.txHelper.(*txcommonmocks.Helper)
	mbm.On("NewBroadcast", transfer.Message).Return(mms)
	mms.On("Prepare", context.Background()).Return(fmt.Errorf("pop"))
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)

	_, err := am.TransferTokens(context.Background(), transfer, false)
	assert.EqualError(t, err, "pop")

	mbm.AssertExpectations(t)
	mms.AssertExpectations(t)
	mth.AssertExpectations(t)
}

func TestTransferTokensWithPrivateMessage(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	msgID := fftypes.NewUUID()
	hash := fftypes.NewRandB32()
	transfer := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			From:   "A",
			To:     "B",
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool: "pool1",
		Message: &core.MessageInOut{
			Message: core.Message{
				Header: core.MessageHeader{
					ID:   msgID,
					Type: core.MessageTypePrivate,
				},
				Hash: hash,
			},
			InlineData: core.InlineData{
				{
					Value: fftypes.JSONAnyPtr("test data"),
				},
			},
		},
		IdempotencyKey: "idem1",
	}
	pool := &core.TokenPool{
		Connector: "magic-tokens",
		Active:    true,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mpm := am.messaging.(*privatemessagingmocks.Manager)
	mms := &syncasyncmocks.Sender{}
	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	mpm.On("NewMessage", transfer.Message).Return(mms)
	mms.On("Prepare", context.Background()).Return(nil)
	mms.On("Send", context.Background()).Return(nil)
	mom.On("RunOperation", context.Background(), mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(transferData)
		return op.Type == core.OpTypeTokenTransfer && data.Pool == pool && data.Transfer == &transfer.TokenTransfer
	}), true).Return(nil, nil)

	_, err := am.TransferTokens(context.Background(), transfer, false)
	assert.NoError(t, err)
	assert.Equal(t, *msgID, *transfer.TokenTransfer.Message)
	assert.Equal(t, *hash, *transfer.TokenTransfer.MessageHash)

	mpm.AssertExpectations(t)
	mim.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mms.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestTransferTokensWithPrivateMessageDisabled(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()
	am.messaging = nil

	msgID := fftypes.NewUUID()
	hash := fftypes.NewRandB32()
	transfer := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			From:   "A",
			To:     "B",
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
		Message: &core.MessageInOut{
			Message: core.Message{
				Header: core.MessageHeader{
					ID:   msgID,
					Type: core.MessageTypeDeprecatedTransferPrivate,
				},
				Hash: hash,
			},
			InlineData: core.InlineData{
				{
					Value: fftypes.JSONAnyPtr("test data"),
				},
			},
		},
	}

	mth := am.txHelper.(*txcommonmocks.Helper)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)

	_, err := am.TransferTokens(context.Background(), transfer, false)
	assert.Regexp(t, "FF10415", err)

	mth.AssertExpectations(t)
}

func TestTransferTokensWithInvalidMessage(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	transfer := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			From:   "A",
			To:     "B",
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
		Message: &core.MessageInOut{
			Message: core.Message{
				Header: core.MessageHeader{
					Type: core.MessageTypeDefinition,
				},
			},
			InlineData: core.InlineData{
				{
					Value: fftypes.JSONAnyPtr("test data"),
				},
			},
		},
	}

	mth := am.txHelper.(*txcommonmocks.Helper)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)

	_, err := am.TransferTokens(context.Background(), transfer, false)
	assert.Regexp(t, "FF10287", err)

	mth.AssertExpectations(t)
}

func TestTransferTokensConfirm(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	transfer := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			From:   "A",
			To:     "B",
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
	}
	pool := &core.TokenPool{
		Connector: "magic-tokens",
		Active:    true,
	}

	mdi := am.database.(*databasemocks.Plugin)
	msa := am.syncasync.(*syncasyncmocks.Bridge)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	msa.On("WaitForTokenTransfer", context.Background(), mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			send := args[2].(syncasync.SendFunction)
			send(context.Background())
		}).
		Return(&core.TokenTransfer{}, nil)
	mom.On("RunOperation", context.Background(), mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(transferData)
		return op.Type == core.OpTypeTokenTransfer && data.Pool == pool && data.Transfer == &transfer.TokenTransfer
	}), true).Return(nil, nil)

	_, err := am.TransferTokens(context.Background(), transfer, true)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
	msa.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestTransferTokensWithBroadcastConfirm(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	msgID := fftypes.NewUUID()
	hash := fftypes.NewRandB32()
	transfer := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			From:   "A",
			To:     "B",
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool: "pool1",
		Message: &core.MessageInOut{
			Message: core.Message{
				Header: core.MessageHeader{
					ID: msgID,
				},
				Hash: hash,
			},
			InlineData: core.InlineData{
				{
					Value: fftypes.JSONAnyPtr("test data"),
				},
			},
		},
		IdempotencyKey: "idem1",
	}
	pool := &core.TokenPool{
		Connector: "magic-tokens",
		Active:    true,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mbm := am.broadcast.(*broadcastmocks.Manager)
	mms := &syncasyncmocks.Sender{}
	msa := am.syncasync.(*syncasyncmocks.Bridge)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mbm.On("NewBroadcast", transfer.Message).Return(mms)
	mms.On("Prepare", context.Background()).Return(nil)
	mms.On("Send", context.Background()).Return(nil)
	msa.On("WaitForMessage", context.Background(), mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			send := args[2].(syncasync.SendFunction)
			send(context.Background())
		}).
		Return(&core.Message{}, nil)
	msa.On("WaitForTokenTransfer", context.Background(), mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			send := args[2].(syncasync.SendFunction)
			send(context.Background())
		}).
		Return(&transfer.TokenTransfer, nil)
	mom.On("RunOperation", context.Background(), mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(transferData)
		return op.Type == core.OpTypeTokenTransfer && data.Pool == pool && data.Transfer == &transfer.TokenTransfer
	}), true).Return(nil, nil)

	_, err := am.TransferTokens(context.Background(), transfer, true)
	assert.NoError(t, err)
	assert.Equal(t, *msgID, *transfer.TokenTransfer.Message)
	assert.Equal(t, *hash, *transfer.TokenTransfer.MessageHash)

	mbm.AssertExpectations(t)
	mim.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mms.AssertExpectations(t)
	msa.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestTransferTokensPoolNotFound(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	transfer := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			From:   "A",
			To:     "B",
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
	}

	mdi := am.database.(*databasemocks.Plugin)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(nil, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)

	_, err := am.TransferTokens(context.Background(), transfer, false)
	assert.Regexp(t, "FF10109", err)

	mdi.AssertExpectations(t)
	mth.AssertExpectations(t)
}

func TestTransferPrepare(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	transfer := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			Type:   core.TokenTransferTypeTransfer,
			From:   "A",
			To:     "B",
			Amount: *fftypes.NewFFBigInt(5),
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
	}
	pool := &core.TokenPool{
		Connector: "magic-tokens",
		Active:    true,
	}

	sender := am.NewTransfer(transfer)

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenTransfer, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)

	err := sender.Prepare(context.Background())
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
}
