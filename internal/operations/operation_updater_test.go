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

package operations

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-common/pkg/ffapi"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly/internal/cache"
	"github.com/hyperledger/firefly/internal/coreconfig"
	"github.com/hyperledger/firefly/internal/txcommon"
	"github.com/hyperledger/firefly/mocks/cachemocks"
	"github.com/hyperledger/firefly/mocks/databasemocks"
	"github.com/hyperledger/firefly/mocks/datamocks"
	"github.com/hyperledger/firefly/pkg/core"
	"github.com/hyperledger/firefly/pkg/database"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func newTestOperationUpdater(t *testing.T) *operationUpdater {
	return newTestOperationUpdaterCommon(t, &database.Capabilities{Concurrency: true})
}

func newTestOperationUpdaterNoConcurrency(t *testing.T) *operationUpdater {
	return newTestOperationUpdaterCommon(t, &database.Capabilities{Concurrency: false})
}

func newTestOperationUpdaterCommon(t *testing.T, dbCapabilities *database.Capabilities) *operationUpdater {
	coreconfig.Reset()
	config.Set(coreconfig.OpUpdateWorkerCount, 1)
	config.Set(coreconfig.OpUpdateWorkerBatchTimeout, "1s")
	config.Set(coreconfig.OpUpdateWorkerBatchMaxInserts, 200)
	logrus.SetLevel(logrus.DebugLevel)

	mdi := &databasemocks.Plugin{}
	mdi.On("Capabilities").Return(dbCapabilities)

	mom := &operationsManager{
		namespace: "ns1",
		handlers:  make(map[fftypes.FFEnum]OperationHandler),
		cache:     cache.NewUmanagedCache(context.Background(), 100, 5*time.Minute),
		database:  mdi,
	}
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := txcommon.NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)
	return newOperationUpdater(context.Background(), mom, mdi, txHelper)
}

func updateMatcher(vals [][]string) func(ffapi.Update) bool {
	return func(update ffapi.Update) bool {
		info, _ := update.Finalize()
		if len(info.SetOperations) != len(vals) {
			fmt.Printf("Failed: %d != %d\n", len(info.SetOperations), len(vals))
			return false
		}
		for i, v := range vals {
			field := info.SetOperations[i].Field
			if info.SetOperations[i].Field != v[0] {
				fmt.Printf("Failed: %s != %s\n", field, v[0])
				return false
			}
			updateVal, _ := info.SetOperations[i].Value.Value()
			if s, ok := updateVal.([]byte); ok {
				updateVal = string(s)
			}
			if updateVal != v[1] {
				fmt.Printf("Failed: %v != %v\n", updateVal, v[1])
				return false
			}
		}
		return true
	}
}

func TestNewOperationUpdaterNoConcurrency(t *testing.T) {
	ou := newTestOperationUpdaterNoConcurrency(t)
	defer ou.close()
	assert.Zero(t, ou.conf.workerCount)
}

func TestSubmitUpdateBadIDIgnored(t *testing.T) {
	ou := newTestOperationUpdater(t)
	ou.close()
	ou.workQueues = []chan *core.OperationUpdateAsync{
		make(chan *core.OperationUpdateAsync),
	}
	ou.cancelFunc()
	ou.SubmitOperationUpdate(ou.ctx, &core.OperationUpdateAsync{
		OperationUpdate: core.OperationUpdate{
			NamespacedOpID: "!!!" + fftypes.NewUUID().String(),
		},
	})
}

func TestSubmitUpdateClosed(t *testing.T) {
	ou := newTestOperationUpdater(t)
	ou.close()
	ou.workQueues = []chan *core.OperationUpdateAsync{
		make(chan *core.OperationUpdateAsync),
	}
	ou.cancelFunc()
	ou.SubmitOperationUpdate(ou.ctx, &core.OperationUpdateAsync{
		OperationUpdate: core.OperationUpdate{
			NamespacedOpID: "ns1:" + fftypes.NewUUID().String(),
		},
	})
}

func TestSubmitUpdateSyncFallbackOpNotFound(t *testing.T) {
	ou := newTestOperationUpdaterNoConcurrency(t)
	defer ou.close()
	customCtx := context.WithValue(context.Background(), "dbtx", "on this context")

	mdi := ou.database.(*databasemocks.Plugin)
	mdi.On("RunAsGroup", customCtx, mock.Anything).Run(func(args mock.Arguments) {
		err := args[1].(func(context.Context) error)(customCtx)
		assert.NoError(t, err)
	}).Return(nil)
	mdi.On("GetOperations", customCtx, mock.Anything, mock.Anything).Return(nil, nil, nil)

	complete := false
	ou.SubmitOperationUpdate(customCtx, &core.OperationUpdateAsync{
		OperationUpdate: core.OperationUpdate{
			NamespacedOpID: "ns1:" + fftypes.NewUUID().String(),
		},
		OnComplete: func() { complete = true },
	})
	assert.True(t, complete)

	mdi.AssertExpectations(t)
}

func TestSubmitUpdateDatabaseError(t *testing.T) {
	ou := newTestOperationUpdaterNoConcurrency(t)
	defer ou.close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	mdi := ou.database.(*databasemocks.Plugin)
	mdi.On("RunAsGroup", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))

	ou.SubmitOperationUpdate(ctx, &core.OperationUpdateAsync{
		OperationUpdate: core.OperationUpdate{
			NamespacedOpID: "ns1:" + fftypes.NewUUID().String(),
		},
	})

	mdi.AssertExpectations(t)
}

func TestSubmitUpdateWrongNS(t *testing.T) {
	ou := newTestOperationUpdaterNoConcurrency(t)
	defer ou.close()
	customCtx := context.WithValue(context.Background(), "dbtx", "on this context")

	ou.SubmitOperationUpdate(customCtx, &core.OperationUpdateAsync{
		OperationUpdate: core.OperationUpdate{
			NamespacedOpID: "ns2:" + fftypes.NewUUID().String(),
		},
	})
}

func TestSubmitUpdateWorkerE2ESuccess(t *testing.T) {
	om, cancel := newTestOperations(t)
	defer om.WaitStop()
	defer cancel()
	om.updater.conf.maxInserts = 2

	opID1 := fftypes.NewUUID()
	opID2 := fftypes.NewUUID()
	opID3 := fftypes.NewUUID()
	tx1 := &core.Transaction{ID: fftypes.NewUUID()}

	om.cache = cache.NewUmanagedCache(context.Background(), 100, 10*time.Minute)
	om.cacheOperation(
		&core.Operation{ID: opID1, Namespace: "ns1", Type: core.OpTypeBlockchainInvoke, Transaction: tx1.ID},
	)

	done := make(chan struct{})

	mdi := om.database.(*databasemocks.Plugin)
	mdi.On("GetOperations", mock.Anything, mock.Anything, mock.Anything).Return([]*core.Operation{
		{ID: opID2, Namespace: "ns1", Type: core.OpTypeTokenTransfer, Input: fftypes.JSONObject{"test": "test"}},
		{ID: opID3, Namespace: "ns1", Type: core.OpTypeTokenApproval, Input: fftypes.JSONObject{"test": "test"}},
	}, nil, nil)
	mdi.On("GetTransactionByID", mock.Anything, "ns1", tx1.ID).Return(tx1, nil)
	mdi.On("UpdateTransaction", mock.Anything, "ns1", tx1.ID, mock.Anything).Return(nil)

	mdi.On("UpdateOperation", mock.Anything, "ns1", opID1, mock.Anything, mock.MatchedBy(updateMatcher([][]string{
		{"status", "Succeeded"},
		{"error", ""},
	}))).Return(true, nil).Once()
	mdi.On("UpdateOperation", mock.Anything, "ns1", opID2, mock.Anything, mock.MatchedBy(updateMatcher([][]string{
		{"status", "Failed"},
		{"error", "err1"},
		{"output", fftypes.JSONObject{"test": true}.String()},
	}))).Return(true, nil).Once()
	mdi.On("UpdateOperation", mock.Anything, "ns1", opID3, mock.Anything, mock.MatchedBy(updateMatcher([][]string{
		{"status", "Failed"},
		{"error", "err2"},
	}))).Return(true, nil).Run(func(args mock.Arguments) {
		close(done)
	}).Once()

	om.Start()
	om.SubmitOperationUpdate(&core.OperationUpdateAsync{
		OperationUpdate: core.OperationUpdate{
			NamespacedOpID: "ns1:" + opID1.String(),
			Status:         core.OpStatusSucceeded,
			BlockchainTXID: "tx12345",
		},
	})
	om.SubmitOperationUpdate(&core.OperationUpdateAsync{
		OperationUpdate: core.OperationUpdate{
			NamespacedOpID: "ns1:" + opID2.String(),
			Status:         core.OpStatusFailed,
			ErrorMessage:   "err1",
			Output:         fftypes.JSONObject{"test": true},
		},
	})
	om.SubmitOperationUpdate(&core.OperationUpdateAsync{
		OperationUpdate: core.OperationUpdate{
			NamespacedOpID: "ns1:" + opID3.String(),
			Status:         core.OpStatusFailed,
			ErrorMessage:   "err2",
		},
	})
	<-done

	mdi.AssertExpectations(t)
}

func TestUpdateLoopExitRetryCancelledContext(t *testing.T) {
	ou := newTestOperationUpdater(t)
	defer ou.close()
	ou.conf.maxInserts = 1
	ou.initQueues()

	mdi := ou.database.(*databasemocks.Plugin)
	mdi.On("RunAsGroup", mock.Anything, mock.Anything).Return(fmt.Errorf("pop")).Run(func(args mock.Arguments) {
		ou.cancelFunc()
	})

	ou.SubmitOperationUpdate(ou.ctx, &core.OperationUpdateAsync{
		OperationUpdate: core.OperationUpdate{
			NamespacedOpID: "ns1:" + fftypes.NewUUID().String(),
		},
	})

	ou.updaterLoop(0)

	mdi.AssertExpectations(t)
}

func TestDoBatchUpdateIgnoreBadID(t *testing.T) {
	ou := newTestOperationUpdaterNoConcurrency(t)
	defer ou.close()

	ou.initQueues()

	err := ou.doBatchUpdate(ou.ctx, []*core.OperationUpdate{
		{NamespacedOpID: "!!Bad", Status: core.OpStatusSucceeded},
	})
	assert.NoError(t, err)

}

func TestDoBatchUpdateFailUpdate(t *testing.T) {
	ou := newTestOperationUpdaterNoConcurrency(t)
	defer ou.close()

	opID1 := fftypes.NewUUID()
	mdi := ou.database.(*databasemocks.Plugin)
	mdi.On("GetOperations", mock.Anything, mock.Anything, mock.Anything).Return([]*core.Operation{
		{ID: opID1, Namespace: "ns1", Type: core.OpTypeBlockchainInvoke},
	}, nil, nil)
	mdi.On("UpdateOperation", mock.Anything, "ns1", opID1, mock.Anything, mock.MatchedBy(updateMatcher([][]string{
		{"status", "Succeeded"},
		{"error", ""},
	}))).Return(false, fmt.Errorf("pop"))

	ou.initQueues()

	err := ou.doBatchUpdate(ou.ctx, []*core.OperationUpdate{
		{NamespacedOpID: "ns1:" + opID1.String(), Status: core.OpStatusSucceeded},
	})
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
}

func TestDoBatchUpdateFailGetTransactions(t *testing.T) {
	ou := newTestOperationUpdaterNoConcurrency(t)
	defer ou.close()

	opID1 := fftypes.NewUUID()
	txID1 := fftypes.NewUUID()
	mdi := ou.database.(*databasemocks.Plugin)
	mdi.On("GetOperations", mock.Anything, mock.Anything, mock.Anything).Return([]*core.Operation{
		{Namespace: "ns1", ID: opID1, Type: core.OpTypeBlockchainInvoke, Transaction: txID1},
	}, nil, nil)
	mdi.On("GetTransactionByID", mock.Anything, "ns1", txID1).Return(nil, fmt.Errorf("pop"))

	ou.initQueues()

	err := ou.doBatchUpdate(ou.ctx, []*core.OperationUpdate{
		{NamespacedOpID: "ns1:" + opID1.String(), Status: core.OpStatusSucceeded},
	})
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
}

func TestDoBatchUpdateFailGetOperations(t *testing.T) {
	ou := newTestOperationUpdaterNoConcurrency(t)
	defer ou.close()

	opID1 := fftypes.NewUUID()
	mdi := ou.database.(*databasemocks.Plugin)
	mdi.On("GetOperations", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	ou.initQueues()

	err := ou.doBatchUpdate(ou.ctx, []*core.OperationUpdate{
		{NamespacedOpID: "ns1:" + opID1.String(), Status: core.OpStatusSucceeded},
	})
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
}

func TestDoUpdateIgnoreBadID(t *testing.T) {
	ou := newTestOperationUpdaterNoConcurrency(t)
	defer ou.close()

	ou.initQueues()

	err := ou.doUpdate(ou.ctx, &core.OperationUpdate{
		NamespacedOpID: "!!!bad", Status: core.OpStatusSucceeded, BlockchainTXID: "0x12345",
	}, []*core.Operation{}, []*core.Transaction{})
	assert.NoError(t, err)

}

func TestDoUpdateFailWrongPlugin(t *testing.T) {
	ou := newTestOperationUpdaterNoConcurrency(t)
	defer ou.close()

	opID1 := fftypes.NewUUID()
	txID1 := fftypes.NewUUID()

	ou.initQueues()

	err := ou.doUpdate(ou.ctx, &core.OperationUpdate{
		NamespacedOpID: "ns1:" + opID1.String(), Status: core.OpStatusSucceeded, BlockchainTXID: "0x12345", Plugin: "plugin2",
	}, []*core.Operation{
		{Namespace: "ns1", ID: opID1, Type: core.OpTypeBlockchainInvoke, Transaction: txID1, Plugin: "plugin1"},
	}, []*core.Transaction{
		{ID: txID1},
	})
	assert.NoError(t, err)
}

func TestDoUpdateFailTransactionUpdate(t *testing.T) {
	ou := newTestOperationUpdaterNoConcurrency(t)
	defer ou.close()

	opID1 := fftypes.NewUUID()
	txID1 := fftypes.NewUUID()
	mdi := ou.database.(*databasemocks.Plugin)
	mdi.On("UpdateTransaction", mock.Anything, "ns1", txID1, mock.Anything).Return(fmt.Errorf("pop"))

	ou.initQueues()

	err := ou.doUpdate(ou.ctx, &core.OperationUpdate{
		NamespacedOpID: "ns1:" + opID1.String(), Status: core.OpStatusSucceeded, BlockchainTXID: "0x12345",
	}, []*core.Operation{
		{Namespace: "ns1", ID: opID1, Type: core.OpTypeBlockchainInvoke, Transaction: txID1},
	}, []*core.Transaction{
		{ID: txID1},
	})
	assert.Regexp(t, "pop", err)

	mdi.AssertExpectations(t)
}

func TestDoUpdateFailExternalHandler(t *testing.T) {
	ou := newTestOperationUpdaterNoConcurrency(t)
	defer ou.close()

	opID1 := fftypes.NewUUID()
	txID1 := fftypes.NewUUID()
	ou.manager.handlers[core.OpTypeBlockchainInvoke] = &mockHandler{UpdateErr: fmt.Errorf("pop")}

	ou.initQueues()

	err := ou.doUpdate(ou.ctx, &core.OperationUpdate{
		NamespacedOpID: "ns1:" + opID1.String(), Status: core.OpStatusSucceeded,
	}, []*core.Operation{
		{Namespace: "ns1", ID: opID1, Type: core.OpTypeBlockchainInvoke, Transaction: txID1},
	}, []*core.Transaction{})
	assert.Regexp(t, "pop", err)
}

func TestDoUpdateVerifyBatchManifest(t *testing.T) {
	ou := newTestOperationUpdaterNoConcurrency(t)
	defer ou.close()

	opID1 := fftypes.NewUUID()
	txID1 := fftypes.NewUUID()
	batchID := fftypes.NewUUID()
	ou.manager.handlers[core.OpTypeDataExchangeSendBatch] = &mockHandler{}

	ou.initQueues()

	mdi := ou.database.(*databasemocks.Plugin)
	mdi.On("GetBatchByID", mock.Anything, "ns1", batchID).Return(&core.BatchPersisted{
		Manifest: fftypes.JSONAnyPtr(`"test-manifest"`),
	}, nil)
	mdi.On("UpdateOperation", mock.Anything, "ns1", opID1, mock.Anything, mock.MatchedBy(updateMatcher([][]string{
		{"status", "Succeeded"},
		{"error", ""},
	}))).Return(true, nil)

	err := ou.doUpdate(ou.ctx, &core.OperationUpdate{
		NamespacedOpID: "ns1:" + opID1.String(),
		Status:         core.OpStatusSucceeded,
		VerifyManifest: true,
		DXManifest:     `"test-manifest"`,
	}, []*core.Operation{{
		Namespace:   "ns1",
		ID:          opID1,
		Type:        core.OpTypeDataExchangeSendBatch,
		Transaction: txID1,
		Input: fftypes.JSONObject{
			"batch": batchID.String(),
		},
	}}, []*core.Transaction{})

	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestDoUpdateVerifyBatchManifestQuery(t *testing.T) {
	ou := newTestOperationUpdaterNoConcurrency(t)
	defer ou.close()

	opID1 := fftypes.NewUUID()
	txID1 := fftypes.NewUUID()
	batchID := fftypes.NewUUID()
	ou.manager.handlers[core.OpTypeDataExchangeSendBatch] = &mockHandler{}

	ou.initQueues()

	mdi := ou.database.(*databasemocks.Plugin)
	mdi.On("GetBatchByID", mock.Anything, "ns1", batchID).Return(nil, fmt.Errorf("pop"))

	err := ou.doUpdate(ou.ctx, &core.OperationUpdate{
		NamespacedOpID: "ns1:" + opID1.String(),
		Status:         core.OpStatusSucceeded,
		VerifyManifest: true,
		DXManifest:     `"test-manifest"`,
	}, []*core.Operation{{
		Namespace:   "ns1",
		ID:          opID1,
		Type:        core.OpTypeDataExchangeSendBatch,
		Transaction: txID1,
		Input: fftypes.JSONObject{
			"batch": batchID.String(),
		},
	}}, []*core.Transaction{})

	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestDoUpdateVerifyBatchManifestFail(t *testing.T) {
	ou := newTestOperationUpdaterNoConcurrency(t)
	defer ou.close()

	opID1 := fftypes.NewUUID()
	txID1 := fftypes.NewUUID()
	batchID := fftypes.NewUUID()
	ou.manager.handlers[core.OpTypeDataExchangeSendBatch] = &mockHandler{}

	ou.initQueues()

	mdi := ou.database.(*databasemocks.Plugin)
	mdi.On("GetBatchByID", mock.Anything, "ns1", batchID).Return(&core.BatchPersisted{
		Manifest: fftypes.JSONAnyPtr(`"test-manifest"`),
	}, nil)
	mdi.On("UpdateOperation", mock.Anything, "ns1", opID1, mock.Anything, mock.MatchedBy(updateMatcher([][]string{
		{"status", "Failed"},
		{"error", "FF10329: Manifest mismatch overriding 'Succeeded' status as failure: '\"BAD\"'"},
	}))).Return(true, nil)

	err := ou.doUpdate(ou.ctx, &core.OperationUpdate{
		NamespacedOpID: "ns1:" + opID1.String(),
		Status:         core.OpStatusSucceeded,
		VerifyManifest: true,
		DXManifest:     `"BAD"`,
	}, []*core.Operation{{
		Namespace:   "ns1",
		ID:          opID1,
		Type:        core.OpTypeDataExchangeSendBatch,
		Transaction: txID1,
		Input: fftypes.JSONObject{
			"batch": batchID.String(),
		},
	}}, []*core.Transaction{})

	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestResolveOperationValidUTF8ErrorPassesThrough(t *testing.T) {
	ou := newTestOperationUpdaterNoConcurrency(t)
	defer ou.close()

	opID1 := fftypes.NewUUID()
	mdi := ou.database.(*databasemocks.Plugin)
	mdi.On("GetOperations", mock.Anything, mock.Anything, mock.Anything).Return([]*core.Operation{
		{ID: opID1, Namespace: "ns1", Type: core.OpTypeBlockchainInvoke},
	}, nil, nil)
	mdi.On("UpdateOperation", mock.Anything, "ns1", opID1, mock.Anything, mock.MatchedBy(updateMatcher([][]string{
		{"status", "Failed"},
		{"error", "FF23021: EVM reverted: some normal error message"},
	}))).Return(true, nil)

	ou.initQueues()

	err := ou.doBatchUpdate(ou.ctx, []*core.OperationUpdate{
		{NamespacedOpID: "ns1:" + opID1.String(), Status: core.OpStatusFailed, ErrorMessage: "FF23021: EVM reverted: some normal error message"},
	})
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestResolveOperationInvalidUTF8ErrorHexEncoded(t *testing.T) {
	ou := newTestOperationUpdaterNoConcurrency(t)
	defer ou.close()

	opID1 := fftypes.NewUUID()

	// Simulate the actual revert scenario: readable text with embedded ABI-encoded Error(string)
	// selector bytes (0x08, 0xc3, 0x79, 0xa0) and null byte padding, which is invalid UTF-8
	invalidMsg := "[OCPE]404/98 - \x08\xc3\x79\xa0\x00\x00\x00[TMM]404/16e"
	expectedHex := "5b4f4350455d3430342f3938202d2008c379a00000005b544d4d5d3430342f313665"

	mdi := ou.database.(*databasemocks.Plugin)
	mdi.On("GetOperations", mock.Anything, mock.Anything, mock.Anything).Return([]*core.Operation{
		{ID: opID1, Namespace: "ns1", Type: core.OpTypeBlockchainInvoke},
	}, nil, nil)
	mdi.On("UpdateOperation", mock.Anything, "ns1", opID1, mock.Anything, mock.MatchedBy(updateMatcher([][]string{
		{"status", "Failed"},
		{"error", expectedHex},
	}))).Return(true, nil)

	ou.initQueues()

	err := ou.doBatchUpdate(ou.ctx, []*core.OperationUpdate{
		{NamespacedOpID: "ns1:" + opID1.String(), Status: core.OpStatusFailed, ErrorMessage: invalidMsg},
	})
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestResolveOperationNullBytesOnlyInvalidUTF8(t *testing.T) {
	ou := newTestOperationUpdaterNoConcurrency(t)
	defer ou.close()

	opID1 := fftypes.NewUUID()

	// Null bytes mixed with non-continuation bytes that break UTF-8 validity
	invalidMsg := "error\x00with\x80null"
	expectedHex := "6572726f720077697468806e756c6c"

	mdi := ou.database.(*databasemocks.Plugin)
	mdi.On("GetOperations", mock.Anything, mock.Anything, mock.Anything).Return([]*core.Operation{
		{ID: opID1, Namespace: "ns1", Type: core.OpTypeBlockchainInvoke},
	}, nil, nil)
	mdi.On("UpdateOperation", mock.Anything, "ns1", opID1, mock.Anything, mock.MatchedBy(updateMatcher([][]string{
		{"status", "Failed"},
		{"error", expectedHex},
	}))).Return(true, nil)

	ou.initQueues()

	err := ou.doBatchUpdate(ou.ctx, []*core.OperationUpdate{
		{NamespacedOpID: "ns1:" + opID1.String(), Status: core.OpStatusFailed, ErrorMessage: invalidMsg},
	})
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestResolveOperationNullBytesInValidUTF8HexEncoded(t *testing.T) {
	ou := newTestOperationUpdaterNoConcurrency(t)
	defer ou.close()

	opID1 := fftypes.NewUUID()

	// Pure null bytes embedded in otherwise valid UTF-8 text.
	// utf8.ValidString returns true for this, but PostgreSQL rejects 0x00 in text columns.
	invalidMsg := "hello\x00world"
	expectedHex := "68656c6c6f00776f726c64"

	mdi := ou.database.(*databasemocks.Plugin)
	mdi.On("GetOperations", mock.Anything, mock.Anything, mock.Anything).Return([]*core.Operation{
		{ID: opID1, Namespace: "ns1", Type: core.OpTypeBlockchainInvoke},
	}, nil, nil)
	mdi.On("UpdateOperation", mock.Anything, "ns1", opID1, mock.Anything, mock.MatchedBy(updateMatcher([][]string{
		{"status", "Failed"},
		{"error", expectedHex},
	}))).Return(true, nil)

	ou.initQueues()

	err := ou.doBatchUpdate(ou.ctx, []*core.OperationUpdate{
		{NamespacedOpID: "ns1:" + opID1.String(), Status: core.OpStatusFailed, ErrorMessage: invalidMsg},
	})
	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}

func TestDoUpdateVerifyBlobManifestFail(t *testing.T) {
	ou := newTestOperationUpdaterNoConcurrency(t)
	defer ou.close()

	opID1 := fftypes.NewUUID()
	txID1 := fftypes.NewUUID()
	blobHash := fftypes.MustParseBytes32("940b97ffd499d5e8c30009f82de9aeee8f5dec222e3d7543535156f40be94cca")
	ou.manager.handlers[core.OpTypeDataExchangeSendBlob] = &mockHandler{}

	ou.initQueues()

	mdi := ou.database.(*databasemocks.Plugin)
	mdi.On("UpdateOperation", mock.Anything, "ns1", opID1, mock.Anything, mock.MatchedBy(updateMatcher([][]string{
		{"status", "Failed"},
		{"error", "FF10348: Blob hash mismatch sent=940b97ffd499d5e8c30009f82de9aeee8f5dec222e3d7543535156f40be94cca received=BAD"},
	}))).Return(true, nil)

	err := ou.doUpdate(ou.ctx, &core.OperationUpdate{
		NamespacedOpID: "ns1:" + opID1.String(),
		Status:         core.OpStatusSucceeded,
		VerifyManifest: true,
		DXHash:         "BAD",
	}, []*core.Operation{{
		Namespace:   "ns1",
		ID:          opID1,
		Type:        core.OpTypeDataExchangeSendBlob,
		Transaction: txID1,
		Input: fftypes.JSONObject{
			"hash": blobHash.String(),
		},
	}}, []*core.Transaction{})

	assert.NoError(t, err)

	mdi.AssertExpectations(t)
}
