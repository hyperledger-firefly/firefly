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

func TestGetTokenApprovals(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	mdi := am.database.(*databasemocks.Plugin)
	fb := database.TokenApprovalQueryFactory.NewFilter(context.Background())
	f := fb.And()
	mdi.On("GetTokenApprovals", context.Background(), "ns1", f).Return([]*core.TokenApproval{}, nil, nil)
	_, _, err := am.GetTokenApprovals(context.Background(), f)
	assert.NoError(t, err)
}

func TestTokenApprovalSuccess(t *testing.T) {
	am, cancel := newTestAssetsWithMetrics(t)
	defer cancel()

	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Approved: true,
			Operator: "operator",
			Key:      "key",
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
	mim.On("ResolveInputSigningKey", context.Background(), "key", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenApproval, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	mom.On("RunOperation", context.Background(), mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(approvalData)
		return op.Type == core.OpTypeTokenApproval && data.Pool == pool && data.Approval == &approval.TokenApproval
	}), true).Return(nil, nil)

	_, err := am.TokenApproval(context.Background(), approval, false)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestTokenApprovalSuccessUnknownIdentity(t *testing.T) {
	am, cancel := newTestAssetsWithMetrics(t)
	defer cancel()

	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Approved: true,
			Operator: "operator",
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
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenApproval, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	mom.On("RunOperation", context.Background(), mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(approvalData)
		return op.Type == core.OpTypeTokenApproval && data.Pool == pool && data.Approval == &approval.TokenApproval
	}), true).Return(nil, nil)

	_, err := am.TokenApproval(context.Background(), approval, false)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestApprovalBadConnector(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Approved: true,
			Operator: "operator",
			Key:      "key",
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
	}
	pool := &core.TokenPool{
		Locator:   "F1",
		Connector: "bad",
		Active:    true,
	}

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mim.On("ResolveInputSigningKey", context.Background(), "key", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenApproval, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)

	_, err := am.TokenApproval(context.Background(), approval, false)
	assert.Regexp(t, "FF10272", err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
}

func TestApprovalDefaultPoolSuccess(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Approved: true,
			Operator: "operator",
			Key:      "key",
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
			Locator:   "F1",
			Connector: "magic-tokens",
			Active:    true,
		},
	}
	totalCount := int64(1)
	filterResult := &ffapi.FilterResult{
		TotalCount: &totalCount,
	}
	mim.On("ResolveInputSigningKey", context.Background(), "key", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mdi.On("GetTokenPools", context.Background(), "ns1", mock.MatchedBy((func(f ffapi.AndFilter) bool {
		info, _ := f.Finalize()
		return info.Count && info.Limit == 1
	}))).Return(tokenPools, filterResult, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenApproval, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	mom.On("RunOperation", context.Background(), mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(approvalData)
		return op.Type == core.OpTypeTokenApproval && data.Pool == tokenPools[0] && data.Approval == &approval.TokenApproval
	}), true).Return(nil, nil)

	_, err := am.TokenApproval(context.Background(), approval, false)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestApprovalIdempotentOperationResubmit(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()
	var id = fftypes.NewUUID()

	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Approved: true,
			Operator: "operator",
			Key:      "key",
		},
		IdempotencyKey: "idem1",
	}

	op := &core.Operation{}

	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	fb := database.TokenPoolQueryFactory.NewFilter(context.Background())
	f := fb.And()
	f.Limit(1).Count(true)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenApproval, core.IdempotencyKey("idem1")).Return(id, &sqlcommon.IdempotencyError{
		ExistingTXID:  id,
		OriginalError: i18n.NewError(context.Background(), coremsgs.MsgIdempotencyKeyDuplicateTransaction, "idem1", id)})
	mom.On("ResubmitOperations", context.Background(), id).Return(1, []*core.Operation{op}, nil)

	// If ResubmitOperations returns an operation it's because it found one to resubmit, so we return 2xx not 409, and don't expect an error
	_, err := am.TokenApproval(context.Background(), approval, false)
	assert.NoError(t, err)

	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestApprovalIdempotentOperationResubmitAll(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()
	var id = fftypes.NewUUID()

	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Approved: true,
			Operator: "operator",
			Key:      "key",
		},
		IdempotencyKey: "idem1",
	}

	pool := &core.TokenPool{
		Connector: "magic-tokens",
		Active:    true,
	}

	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)

	fb := database.TokenPoolQueryFactory.NewFilter(context.Background())
	f := fb.And()
	f.Limit(1).Count(true)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenApproval, core.IdempotencyKey("idem1")).Return(id, &sqlcommon.IdempotencyError{
		ExistingTXID:  id,
		OriginalError: i18n.NewError(context.Background(), coremsgs.MsgIdempotencyKeyDuplicateTransaction, "idem1", id)})
	mom.On("ResubmitOperations", context.Background(), id).Return(0, nil, nil)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	mom.On("RunOperation", context.Background(), mock.Anything, true).Return(nil, nil)

	tokenPools := []*core.TokenPool{
		{
			Name:      "pool1",
			Locator:   "F1",
			Connector: "magic-tokens",
			Active:    true,
		},
	}
	totalCount := int64(1)
	filterResult := &ffapi.FilterResult{
		TotalCount: &totalCount,
	}
	mim.On("ResolveInputSigningKey", context.Background(), "key", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mdi.On("GetTokenPools", context.Background(), "ns1", mock.MatchedBy((func(f ffapi.AndFilter) bool {
		info, _ := f.Finalize()
		return info.Count && info.Limit == 1
	}))).Return(tokenPools, filterResult, nil)

	// If ResubmitOperations returns an operation it's because it found one to resubmit, so we return 2xx not 409, and don't expect an error
	_, err := am.TokenApproval(context.Background(), approval, false)
	assert.NoError(t, err)

	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestApprovalIdempotentNoOperationToResubmit(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()
	var id = fftypes.NewUUID()

	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Approved: true,
			Operator: "operator",
			Key:      "key",
		},
		IdempotencyKey: "idem1",
	}

	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	fb := database.TokenPoolQueryFactory.NewFilter(context.Background())
	f := fb.And()
	f.Limit(1).Count(true)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenApproval, core.IdempotencyKey("idem1")).Return(id, &sqlcommon.IdempotencyError{
		ExistingTXID:  id,
		OriginalError: i18n.NewError(context.Background(), coremsgs.MsgIdempotencyKeyDuplicateTransaction, "idem1", id)})
	mom.On("ResubmitOperations", context.Background(), id).Return(1 /* one total */, nil /* none to resubmit */, nil)

	// If ResubmitOperations returns nil it's because there was no operation in initialized state, so we expect the regular 409 error back
	_, err := am.TokenApproval(context.Background(), approval, false)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "FF10431")

	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestApprovalIdempotentOperationErrorOnResubmit(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()
	var id = fftypes.NewUUID()

	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Approved: true,
			Operator: "operator",
			Key:      "key",
		},
		IdempotencyKey: "idem1",
	}

	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	fb := database.TokenPoolQueryFactory.NewFilter(context.Background())
	f := fb.And()
	f.Limit(1).Count(true)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenApproval, core.IdempotencyKey("idem1")).Return(id, &sqlcommon.IdempotencyError{
		ExistingTXID:  id,
		OriginalError: i18n.NewError(context.Background(), coremsgs.MsgIdempotencyKeyDuplicateTransaction, "idem1", id)})
	mom.On("ResubmitOperations", context.Background(), id).Return(-1, nil, fmt.Errorf("pop"))

	// If ResubmitOperations returns an operation it's because it found one to resubmit, so we return 2xx not 409, and don't expect an error
	_, err := am.TokenApproval(context.Background(), approval, false)
	assert.Error(t, err)
	assert.ErrorContains(t, err, "pop")

	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestApprovalDefaultPoolNoPool(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Approved: true,
			Operator: "operator",
			Key:      "key",
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
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenApproval, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)

	_, err := am.TokenApproval(context.Background(), approval, false)
	assert.Regexp(t, "FF10292", err)

	mdi.AssertExpectations(t)
	mth.AssertExpectations(t)
}

func TestApprovalBadPool(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Approved: true,
			Operator: "operator",
			Key:      "key",
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
	}

	mdi := am.database.(*databasemocks.Plugin)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(nil, fmt.Errorf("pop"))
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenApproval, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)

	_, err := am.TokenApproval(context.Background(), approval, false)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
	mth.AssertExpectations(t)
}

func TestApprovalUnconfirmedPool(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Approved: true,
			Operator: "operator",
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
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenApproval, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)

	_, err := am.TokenApproval(context.Background(), approval, false)
	assert.Regexp(t, "FF10293", err)

	mdi.AssertExpectations(t)
	mth.AssertExpectations(t)
}

func TestApprovalIdentityFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Approved: true,
			Operator: "operator",
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
	mim.On("ResolveInputSigningKey", context.Background(), "", identity.KeyNormalizationBlockchainPlugin).Return("", fmt.Errorf("pop"))
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenApproval, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)

	_, err := am.TokenApproval(context.Background(), approval, false)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
}

func TestApprovalFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Approved: true,
			Operator: "operator",
			Key:      "key",
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
	mim.On("ResolveInputSigningKey", context.Background(), "key", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenApproval, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	mom.On("RunOperation", context.Background(), mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(approvalData)
		return op.Type == core.OpTypeTokenApproval && data.Pool == pool && data.Approval == &approval.TokenApproval
	}), true).Return(nil, fmt.Errorf("pop"))

	_, err := am.TokenApproval(context.Background(), approval, false)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestApprovalTransactionFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Approved: true,
			Operator: "operator",
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
	}

	mth := am.txHelper.(*txcommonmocks.Helper)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenApproval, core.IdempotencyKey("idem1")).Return(nil, fmt.Errorf("pop"))

	_, err := am.TokenApproval(context.Background(), approval, false)
	assert.EqualError(t, err, "pop")

	mth.AssertExpectations(t)
}

func TestApprovalWithBroadcastMessage(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	msgID := fftypes.NewUUID()
	hash := fftypes.NewRandB32()
	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Operator: "B",
			Approved: true,
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
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenApproval, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	mbm.On("NewBroadcast", approval.Message).Return(mms)
	mms.On("Prepare", context.Background()).Return(nil)
	mms.On("Send", context.Background()).Return(nil)
	mom.On("RunOperation", context.Background(), mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(approvalData)
		return op.Type == core.OpTypeTokenApproval && data.Pool == pool && data.Approval == &approval.TokenApproval
	}), true).Return(nil, nil)

	_, err := am.TokenApproval(context.Background(), approval, false)
	assert.NoError(t, err)
	assert.Equal(t, *msgID, *approval.TokenApproval.Message)
	assert.Equal(t, *hash, *approval.TokenApproval.MessageHash)

	mbm.AssertExpectations(t)
	mim.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mms.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestApprovalWithBroadcastMessageDisabled(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()
	am.broadcast = nil

	msgID := fftypes.NewUUID()
	hash := fftypes.NewRandB32()
	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Operator: "B",
			Approved: true,
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

	mth := am.txHelper.(*txcommonmocks.Helper)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenApproval, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)

	_, err := am.TokenApproval(context.Background(), approval, false)
	assert.Regexp(t, "FF10415", err)

	mth.AssertExpectations(t)
}

func TestApprovalWithBroadcastMessageSendFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	msgID := fftypes.NewUUID()
	hash := fftypes.NewRandB32()
	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Operator: "B",
			Approved: true,
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
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenApproval, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	mbm.On("NewBroadcast", approval.Message).Return(mms)
	mms.On("Prepare", context.Background()).Return(nil)
	mms.On("Send", context.Background()).Return(fmt.Errorf("pop"))

	_, err := am.TokenApproval(context.Background(), approval, false)
	assert.Regexp(t, "pop", err)
	assert.Equal(t, *msgID, *approval.TokenApproval.Message)
	assert.Equal(t, *hash, *approval.TokenApproval.MessageHash)

	mbm.AssertExpectations(t)
	mim.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mms.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestApprovalWithBroadcastPrepareFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Operator: "B",
			Approved: true,
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
	mbm.On("NewBroadcast", approval.Message).Return(mms)
	mms.On("Prepare", context.Background()).Return(fmt.Errorf("pop"))
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenApproval, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)

	_, err := am.TokenApproval(context.Background(), approval, false)
	assert.EqualError(t, err, "pop")

	mbm.AssertExpectations(t)
	mms.AssertExpectations(t)
	mth.AssertExpectations(t)
}

func TestApprovalWithPrivateMessage(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	msgID := fftypes.NewUUID()
	hash := fftypes.NewRandB32()
	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Operator: "B",
			Approved: true,
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
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenApproval, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	mpm.On("NewMessage", approval.Message).Return(mms)
	mms.On("Prepare", context.Background()).Return(nil)
	mms.On("Send", context.Background()).Return(nil)
	mom.On("RunOperation", context.Background(), mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(approvalData)
		return op.Type == core.OpTypeTokenApproval && data.Pool == pool && data.Approval == &approval.TokenApproval
	}), true).Return(nil, nil)

	_, err := am.TokenApproval(context.Background(), approval, false)
	assert.NoError(t, err)
	assert.Equal(t, *msgID, *approval.TokenApproval.Message)
	assert.Equal(t, *hash, *approval.TokenApproval.MessageHash)

	mpm.AssertExpectations(t)
	mim.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mms.AssertExpectations(t)
	mth.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestApprovalWithPrivateMessageDisabled(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()
	am.messaging = nil

	msgID := fftypes.NewUUID()
	hash := fftypes.NewRandB32()
	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Operator: "B",
			Approved: true,
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
		Message: &core.MessageInOut{
			Message: core.Message{
				Header: core.MessageHeader{
					ID:   msgID,
					Type: core.MessageTypeDeprecatedApprovalPrivate,
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
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenApproval, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)

	_, err := am.TokenApproval(context.Background(), approval, false)
	assert.Regexp(t, "FF10415", err)

	mth.AssertExpectations(t)
}

func TestApprovalWithInvalidMessage(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Operator: "B",
			Approved: true,
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
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenApproval, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)

	_, err := am.TokenApproval(context.Background(), approval, false)
	assert.Regexp(t, "FF10287", err)

	mth.AssertExpectations(t)
}

func TestApprovalOperationsFail(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Approved: true,
			Operator: "operator",
			Key:      "key",
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

	mim.On("ResolveInputSigningKey", context.Background(), "key", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenApproval, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(fmt.Errorf("pop"))

	_, err := am.TokenApproval(context.Background(), approval, false)
	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mom.AssertExpectations(t)
	mth.AssertExpectations(t)
}

func TestTokenApprovalConfirm(t *testing.T) {
	am, cancel := newTestAssetsWithMetrics(t)
	defer cancel()

	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Approved: true,
			Operator: "operator",
			Key:      "key",
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
	msa := am.syncasync.(*syncasyncmocks.Bridge)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mom := am.operations.(*operationmocks.Manager)
	mim.On("ResolveInputSigningKey", context.Background(), "key", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenApproval, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	mom.On("RunOperation", context.Background(), mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(approvalData)
		return op.Type == core.OpTypeTokenApproval && data.Pool == pool && data.Approval == &approval.TokenApproval
	}), true).Return(nil, nil)

	msa.On("WaitForTokenApproval", context.Background(), mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			send := args[2].(syncasync.SendFunction)
			send(context.Background())
		}).
		Return(&core.TokenApproval{}, nil)

	_, err := am.TokenApproval(context.Background(), approval, true)
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	msa.AssertExpectations(t)
	mom.AssertExpectations(t)
	mth.AssertExpectations(t)
}

func TestApprovalWithBroadcastConfirm(t *testing.T) {
	am, cancel := newTestAssets(t)
	defer cancel()

	msgID := fftypes.NewUUID()
	hash := fftypes.NewRandB32()
	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Approved: true,
			Operator: "operator",
			Key:      "key",
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
	mim.On("ResolveInputSigningKey", context.Background(), "key", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenApproval, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)
	mbm.On("NewBroadcast", approval.Message).Return(mms)
	mms.On("Prepare", context.Background()).Return(nil)
	mms.On("Send", context.Background()).Return(nil)
	msa.On("WaitForMessage", context.Background(), mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			send := args[2].(syncasync.SendFunction)
			send(context.Background())
		}).
		Return(&core.Message{}, nil)
	msa.On("WaitForTokenApproval", context.Background(), mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			send := args[2].(syncasync.SendFunction)
			send(context.Background())
		}).
		Return(&approval.TokenApproval, nil)
	mom.On("RunOperation", context.Background(), mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(approvalData)
		return op.Type == core.OpTypeTokenApproval && data.Pool == pool && data.Approval == &approval.TokenApproval
	}), true).Return(nil, nil)

	_, err := am.TokenApproval(context.Background(), approval, true)
	assert.NoError(t, err)
	assert.Equal(t, *msgID, *approval.TokenApproval.Message)
	assert.Equal(t, *hash, *approval.TokenApproval.MessageHash)

	mbm.AssertExpectations(t)
	mim.AssertExpectations(t)
	mdi.AssertExpectations(t)
	mms.AssertExpectations(t)
	msa.AssertExpectations(t)
	mom.AssertExpectations(t)
}

func TestApprovalPrepare(t *testing.T) {
	am, cancel := newTestAssetsWithMetrics(t)
	defer cancel()

	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Approved: true,
			Operator: "operator",
			Key:      "key",
		},
		Pool:           "pool1",
		IdempotencyKey: "idem1",
	}
	pool := &core.TokenPool{
		Locator:   "F1",
		Connector: "magic-tokens",
		Active:    true,
	}

	sender := am.NewApproval(approval)

	mdi := am.database.(*databasemocks.Plugin)
	mim := am.identity.(*identitymanagermocks.Manager)
	mth := am.txHelper.(*txcommonmocks.Helper)
	mim.On("ResolveInputSigningKey", context.Background(), "key", identity.KeyNormalizationBlockchainPlugin).Return("0x12345", nil)
	mdi.On("GetTokenPool", context.Background(), "ns1", "pool1").Return(pool, nil)
	mth.On("SubmitNewTransaction", context.Background(), core.TransactionTypeTokenApproval, core.IdempotencyKey("idem1")).Return(fftypes.NewUUID(), nil)

	err := sender.Prepare(context.Background())
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
	mim.AssertExpectations(t)
	mth.AssertExpectations(t)
}
