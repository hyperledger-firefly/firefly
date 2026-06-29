// Copyright © 2025 Kaleido, Inc.
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
package operations

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/internal/cache"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/txcommon"
	"github.com/hyperledger-firefly/firefly/mocks/cachemocks"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/mocks/datamocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type mockHandler struct {
	Phase     core.OpPhase
	PrepErr   error
	RunErr    error
	Prepared  *core.PreparedOperation
	Outputs   fftypes.JSONObject
	UpdateErr error
}

type mockConflictErr struct {
	err error
}

func (ce *mockConflictErr) Error() string {
	return ce.err.Error()
}

func (ce *mockConflictErr) IsConflictError() bool {
	return true
}

func (m *mockHandler) Name() string {
	return "MockHandler"
}

func (m *mockHandler) PrepareOperation(ctx context.Context, op *core.Operation) (*core.PreparedOperation, error) {
	return m.Prepared, m.PrepErr
}

func (m *mockHandler) RunOperation(ctx context.Context, op *core.PreparedOperation) (outputs fftypes.JSONObject, phase core.OpPhase, err error) {
	return m.Outputs, m.Phase, m.RunErr
}

func (m *mockHandler) OnOperationUpdate(ctx context.Context, op *core.Operation, update *core.OperationUpdate) error {
	return m.UpdateErr
}

func newTestOperations(t *testing.T) (*operationsManager, func()) {
	coreconfig.Reset()
	config.Set(coreconfig.OpUpdateWorkerCount, 1)
	ctx, cancel := context.WithCancel(context.Background())
	mdi := &databasemocks.Plugin{}
	mdi.On("Capabilities").Return(&database.Capabilities{
		Concurrency: true,
	})
	mdm := &datamocks.Manager{}
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := txcommon.NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)

	rag := mdi.On("RunAsGroup", mock.Anything, mock.Anything).Maybe()
	rag.RunFn = func(a mock.Arguments) {
		rag.ReturnArguments = mock.Arguments{
			a[1].(func(context.Context) error)(a[0].(context.Context)),
		}
	}

	ns := "ns1"
	om, err := NewOperationsManager(ctx, ns, mdi, txHelper, cmi)
	assert.NoError(t, err)
	cmi.AssertCalled(t, "GetCache", cache.NewCacheConfig(
		ctx,
		coreconfig.CacheOperationsLimit,
		coreconfig.CacheOperationsTTL,
		ns,
	))
	return om.(*operationsManager), cancel
}

func TestInitFail(t *testing.T) {
	_, err := NewOperationsManager(context.Background(), "ns1", nil, nil, nil)
	assert.Regexp(t, "FF10128", err)
}

func TestCacheInitFail(t *testing.T) {
	cacheInitError := errors.New("Initialization error.")
	coreconfig.Reset()
	config.Set(coreconfig.OpUpdateWorkerCount, 1)
	mdi := &databasemocks.Plugin{}
	mdi.On("Capabilities").Return(&database.Capabilities{
		Concurrency: true,
	})
	mdm := &datamocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := txcommon.NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)
	ns := "ns1"
	ecmi := &cachemocks.Manager{}
	ecmi.On("GetCache", mock.Anything).Return(nil, cacheInitError)
	_, err := NewOperationsManager(ctx, ns, mdi, txHelper, ecmi)
	assert.Equal(t, cacheInitError, err)
}

func TestPrepareOperationNotSupported(t *testing.T) {
	om, cancel := newTestOperations(t)
	defer cancel()

	op := &core.Operation{}

	_, err := om.PrepareOperation(context.Background(), op)
	assert.Regexp(t, "FF10371", err)
}

func TestPrepareOperationSuccess(t *testing.T) {
	om, cancel := newTestOperations(t)
	defer cancel()

	ctx := context.Background()
	op := &core.Operation{
		Type: core.OpTypeBlockchainPinBatch,
	}

	om.RegisterHandler(ctx, &mockHandler{}, []core.OpType{core.OpTypeBlockchainPinBatch})
	_, err := om.PrepareOperation(context.Background(), op)

	assert.NoError(t, err)
}

func TestRunOperationNotSupported(t *testing.T) {
	om, cancel := newTestOperations(t)
	defer cancel()

	op := &core.PreparedOperation{}

	_, err := om.RunOperation(context.Background(), op, true)
	assert.Regexp(t, "FF10371", err)
}

func TestRunOperationSuccess(t *testing.T) {
	om, cancel := newTestOperations(t)
	defer cancel()

	ctx := context.Background()
	op := &core.PreparedOperation{
		Type: core.OpTypeBlockchainPinBatch,
	}

	om.RegisterHandler(ctx, &mockHandler{Outputs: fftypes.JSONObject{"test": "output"}}, []core.OpType{core.OpTypeBlockchainPinBatch})
	outputs, err := om.RunOperation(context.Background(), op, true)
	assert.Equal(t, "output", outputs.GetString("test"))

	assert.NoError(t, err)
}

func TestRunOperationSyncSuccess(t *testing.T) {
	om, cancel := newTestOperations(t)
	defer cancel()

	om.updater.workQueues = []chan *core.OperationUpdateAsync{
		make(chan *core.OperationUpdateAsync),
	}
	om.updater.cancelFunc()

	ctx := context.Background()
	op := &core.PreparedOperation{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Type:      core.OpTypeBlockchainPinBatch,
	}

	om.RegisterHandler(ctx, &mockHandler{Phase: core.OpPhaseComplete}, []core.OpType{core.OpTypeBlockchainPinBatch})
	_, err := om.RunOperation(ctx, op, true)

	assert.NoError(t, err)
}

func TestRunOperationFailIdempotentInit(t *testing.T) {
	om, cancel := newTestOperations(t)
	defer cancel()

	om.updater.workQueues = []chan *core.OperationUpdateAsync{
		make(chan *core.OperationUpdateAsync, 1),
	}

	ctx := context.Background()
	op := &core.PreparedOperation{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Type:      core.OpTypeBlockchainPinBatch,
	}

	om.RegisterHandler(ctx, &mockHandler{
		RunErr: fmt.Errorf("pop"),
		Phase:  core.OpPhaseInitializing,
	}, []core.OpType{core.OpTypeBlockchainPinBatch})
	_, err := om.RunOperation(ctx, op, true)

	update := <-om.updater.workQueues[0]
	assert.Equal(t, "ns1:"+op.ID.String(), update.NamespacedOpID)
	assert.Equal(t, core.OpStatusInitialized, update.Status)

	assert.EqualError(t, err, "pop")
}

func TestRunOperationFailNonIdempotentInit(t *testing.T) {
	om, cancel := newTestOperations(t)
	defer cancel()

	om.updater.workQueues = []chan *core.OperationUpdateAsync{
		make(chan *core.OperationUpdateAsync, 1),
	}

	ctx := context.Background()
	op := &core.PreparedOperation{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Type:      core.OpTypeBlockchainPinBatch,
	}

	om.RegisterHandler(ctx, &mockHandler{
		RunErr: fmt.Errorf("pop"),
		Phase:  core.OpPhaseInitializing,
	}, []core.OpType{core.OpTypeBlockchainPinBatch})
	_, err := om.RunOperation(ctx, op, false)

	update := <-om.updater.workQueues[0]
	assert.Equal(t, "ns1:"+op.ID.String(), update.NamespacedOpID)
	assert.Equal(t, core.OpStatusFailed, update.Status)

	assert.EqualError(t, err, "pop")
}

func TestRunOperationFailConflict(t *testing.T) {
	om, cancel := newTestOperations(t)
	defer cancel()

	om.updater.workQueues = []chan *core.OperationUpdateAsync{
		make(chan *core.OperationUpdateAsync, 1),
	}

	ctx := context.Background()
	op := &core.PreparedOperation{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Type:      core.OpTypeBlockchainPinBatch,
	}

	om.RegisterHandler(ctx, &mockHandler{RunErr: &mockConflictErr{err: fmt.Errorf("pop")}}, []core.OpType{core.OpTypeBlockchainPinBatch})
	_, err := om.RunOperation(ctx, op, true)

	update := <-om.updater.workQueues[0]
	assert.Equal(t, "ns1:"+op.ID.String(), update.NamespacedOpID)
	assert.Equal(t, core.OpStatusPending, update.Status)

	assert.EqualError(t, err, "pop")
}

func TestRunOperationFailRemainPending(t *testing.T) {
	om, cancel := newTestOperations(t)
	defer cancel()

	om.updater.workQueues = []chan *core.OperationUpdateAsync{
		make(chan *core.OperationUpdateAsync),
	}
	om.updater.cancelFunc()

	ctx := context.Background()
	op := &core.PreparedOperation{
		ID:        fftypes.NewUUID(),
		Namespace: "ns1",
		Type:      core.OpTypeBlockchainPinBatch,
	}

	om.RegisterHandler(ctx, &mockHandler{
		RunErr: fmt.Errorf("pop"),
		Phase:  core.OpPhasePending,
	}, []core.OpType{core.OpTypeBlockchainPinBatch})
	_, err := om.RunOperation(ctx, op, false)

	assert.EqualError(t, err, "pop")
}

func TestRetryOperationSuccess(t *testing.T) {
	om, cancel := newTestOperations(t)
	defer cancel()

	ctx := context.Background()
	opID := fftypes.NewUUID()
	txID := fftypes.NewUUID()
	op := &core.Operation{
		ID:          opID,
		Namespace:   "ns1",
		Plugin:      "blockchain",
		Transaction: txID,
		Type:        core.OpTypeBlockchainPinBatch,
		Status:      core.OpStatusFailed,
	}
	po := &core.PreparedOperation{
		ID:   op.ID,
		Type: op.Type,
	}

	om.cache = cache.NewUmanagedCache(ctx, 100, 10*time.Minute)
	om.cacheOperation(op)

	mdi := om.database.(*databasemocks.Plugin)
	mdi.On("InsertOperation", ctx, mock.MatchedBy(func(newOp *core.Operation) bool {
		assert.NotEqual(t, opID, newOp.ID)
		assert.Equal(t, "blockchain", newOp.Plugin)
		assert.Equal(t, core.OpStatusInitialized, newOp.Status)
		assert.Equal(t, core.OpTypeBlockchainPinBatch, newOp.Type)
		return true
	})).Return(nil)
	mdi.On("UpdateOperation", ctx, "ns1", op.ID, mock.Anything, mock.MatchedBy(func(update ffapi.Update) bool {
		info, err := update.Finalize()
		assert.NoError(t, err)
		assert.Equal(t, 1, len(info.SetOperations))
		assert.Equal(t, "retry", info.SetOperations[0].Field)
		retryVal, err := info.SetOperations[0].Value.Value()
		assert.NoError(t, err)
		// The retry value of the parent operation should be the new operation ID
		assert.Equal(t, op.Retry.String(), retryVal.(string))
		// The parent ID should not change
		assert.Equal(t, op.ID.String(), opID.String())
		return true
	})).Return(true, nil)
	mdi.On("GetTransactionByID", mock.Anything, "ns1", txID).Return(&core.Transaction{
		ID:             txID,
		Namespace:      "ns1",
		IdempotencyKey: "idem1",
	}, nil)

	om.RegisterHandler(ctx, &mockHandler{Prepared: po}, []core.OpType{core.OpTypeBlockchainPinBatch})
	newOp, err := om.RetryOperation(ctx, op.ID)

	assert.NoError(t, err)
	assert.NotNil(t, newOp)

	mdi.AssertExpectations(t)
}

func TestRetryOperationGetTXFail(t *testing.T) {
	om, cancel := newTestOperations(t)
	defer cancel()

	ctx := context.Background()
	opID := fftypes.NewUUID()
	txID := fftypes.NewUUID()
	op := &core.Operation{
		ID:          opID,
		Namespace:   "ns1",
		Plugin:      "blockchain",
		Transaction: txID,
		Type:        core.OpTypeBlockchainPinBatch,
		Status:      core.OpStatusFailed,
	}
	po := &core.PreparedOperation{
		ID:   op.ID,
		Type: op.Type,
	}

	om.cache = cache.NewUmanagedCache(ctx, 100, 10*time.Minute)
	om.cacheOperation(op)

	mdi := om.database.(*databasemocks.Plugin)
	mdi.On("GetTransactionByID", mock.Anything, "ns1", txID).Return(nil, fmt.Errorf("pop"))

	om.RegisterHandler(ctx, &mockHandler{Prepared: po}, []core.OpType{core.OpTypeBlockchainPinBatch})
	_, err := om.RetryOperation(ctx, op.ID)

	assert.Regexp(t, "pop", err)
	mdi.AssertExpectations(t)
}

func TestRetryOperationGetFail(t *testing.T) {
	om, cancel := newTestOperations(t)
	defer cancel()

	ctx := context.Background()
	opID := fftypes.NewUUID()
	op := &core.Operation{
		ID:     opID,
		Plugin: "blockchain",
		Type:   core.OpTypeBlockchainPinBatch,
		Status: core.OpStatusFailed,
	}
	po := &core.PreparedOperation{
		ID:   op.ID,
		Type: op.Type,
	}

	mdi := om.database.(*databasemocks.Plugin)
	mdi.On("GetOperationByID", ctx, "ns1", opID).Return(op, fmt.Errorf("pop"))

	om.RegisterHandler(ctx, &mockHandler{Prepared: po}, []core.OpType{core.OpTypeBlockchainPinBatch})
	_, err := om.RetryOperation(ctx, op.ID)

	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestRetryTwiceOperationInsertFail(t *testing.T) {
	om, cancel := newTestOperations(t)
	defer cancel()

	ctx := context.Background()
	opID := fftypes.NewUUID()
	opID2 := fftypes.NewUUID()
	op := &core.Operation{
		ID:     opID,
		Plugin: "blockchain",
		Type:   core.OpTypeBlockchainPinBatch,
		Status: core.OpStatusFailed,
		Retry:  opID2,
	}
	op2 := &core.Operation{
		ID:     opID2,
		Plugin: "blockchain",
		Type:   core.OpTypeBlockchainPinBatch,
		Status: core.OpStatusFailed,
	}
	po := &core.PreparedOperation{
		ID:   op.ID,
		Type: op.Type,
	}

	mdi := om.database.(*databasemocks.Plugin)
	mdi.On("GetOperationByID", ctx, "ns1", opID).Return(op, nil)
	mdi.On("GetOperationByID", ctx, "ns1", opID2).Return(op2, nil)
	mdi.On("GetTransactionByID", mock.Anything, "ns1", mock.Anything).Return(nil, nil)
	mdi.On("InsertOperation", ctx, mock.Anything).Return(fmt.Errorf("pop"))

	om.RegisterHandler(ctx, &mockHandler{Prepared: po}, []core.OpType{core.OpTypeBlockchainPinBatch})
	_, err := om.RetryOperation(ctx, op.ID)

	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestRetryOperationInsertFail(t *testing.T) {
	om, cancel := newTestOperations(t)
	defer cancel()

	ctx := context.Background()
	opID := fftypes.NewUUID()
	op := &core.Operation{
		ID:     opID,
		Plugin: "blockchain",
		Type:   core.OpTypeBlockchainPinBatch,
		Status: core.OpStatusFailed,
	}
	po := &core.PreparedOperation{
		ID:   op.ID,
		Type: op.Type,
	}

	mdi := om.database.(*databasemocks.Plugin)
	mdi.On("GetOperationByID", ctx, "ns1", opID).Return(op, nil)
	mdi.On("GetTransactionByID", mock.Anything, "ns1", mock.Anything).Return(nil, nil)
	mdi.On("InsertOperation", ctx, mock.Anything).Return(fmt.Errorf("pop"))

	om.RegisterHandler(ctx, &mockHandler{Prepared: po}, []core.OpType{core.OpTypeBlockchainPinBatch})
	_, err := om.RetryOperation(ctx, op.ID)

	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestRetryOperationUpdateFail(t *testing.T) {
	om, cancel := newTestOperations(t)
	defer cancel()

	ctx := context.Background()
	opID := fftypes.NewUUID()
	op := &core.Operation{
		ID:        opID,
		Namespace: "ns1",
		Plugin:    "blockchain",
		Type:      core.OpTypeBlockchainPinBatch,
		Status:    core.OpStatusFailed,
	}
	po := &core.PreparedOperation{
		ID:   op.ID,
		Type: op.Type,
	}

	mdi := om.database.(*databasemocks.Plugin)
	mdi.On("GetOperationByID", ctx, "ns1", opID).Return(op, nil)
	mdi.On("GetTransactionByID", mock.Anything, "ns1", mock.Anything).Return(nil, nil)
	mdi.On("InsertOperation", ctx, mock.Anything).Return(nil)
	mdi.On("UpdateOperation", ctx, "ns1", op.ID, mock.Anything, mock.Anything).Return(false, fmt.Errorf("pop"))

	om.RegisterHandler(ctx, &mockHandler{Prepared: po}, []core.OpType{core.OpTypeBlockchainPinBatch})
	_, err := om.RetryOperation(ctx, op.ID)

	assert.EqualError(t, err, "pop")

	mdi.AssertExpectations(t)
}

func TestResolveOperationByNamespacedIDOk(t *testing.T) {
	om, cancel := newTestOperations(t)
	defer cancel()

	ctx := context.Background()
	opID := fftypes.NewUUID()
	errStr := "my error"
	opUpdate := &core.OperationUpdateDTO{
		Status: core.OpStatusSucceeded,
		Error:  &errStr,
		Output: fftypes.JSONObject{
			"my": "data",
		},
	}

	om.cache.Set(opID.String(), &core.Operation{})

	mdi := om.database.(*databasemocks.Plugin)
	mdi.On("UpdateOperation", ctx, "ns1", opID, mock.Anything, mock.MatchedBy(updateMatcher([][]string{
		{"status", "Succeeded"},
		{"error", errStr},
		{"output", opUpdate.Output.String()},
	}))).Return(true, nil)

	err := om.ResolveOperationByID(ctx, opID, opUpdate)
	assert.NoError(t, err)

	// cache should have been updated
	cached := om.cache.Get(opID.String())
	assert.Equal(t, core.OpStatusSucceeded, cached.(*core.Operation).Status)
	assert.Equal(t, errStr, cached.(*core.Operation).Error)

	mdi.AssertExpectations(t)
}

func TestResolveOperationAlreadyResolved(t *testing.T) {
	om, cancel := newTestOperations(t)
	defer cancel()

	ctx := context.Background()
	opID := fftypes.NewUUID()
	errStr := "my error"
	opUpdate := &core.OperationUpdateDTO{
		Status: core.OpStatusPending,
		Error:  &errStr,
		Output: fftypes.JSONObject{
			"my": "data",
		},
	}

	om.cache.Set(opID.String(), &core.Operation{
		Status: core.OpStatusFailed,
	})

	mdi := om.database.(*databasemocks.Plugin)
	mdi.On("UpdateOperation", ctx, "ns1", opID, mock.Anything, mock.Anything).Return(false, nil)

	err := om.ResolveOperationByID(ctx, opID, opUpdate)
	assert.NoError(t, err)

	// cache should not have been updated
	cached := om.cache.Get(opID.String())
	assert.Equal(t, core.OpStatusFailed, cached.(*core.Operation).Status)
	assert.Equal(t, "", cached.(*core.Operation).Error)

	mdi.AssertExpectations(t)
}

func TestResubmitIdempotentOperation(t *testing.T) {
	om, cancel := newTestOperations(t)
	var id = fftypes.NewUUID()
	defer cancel()

	ctx := context.Background()
	opID := fftypes.NewUUID()
	operations := make([]*core.Operation, 0)
	op := &core.Operation{
		ID:     opID,
		Plugin: "blockchain",
		Type:   core.OpTypeBlockchainPinBatch,
		Status: core.OpStatusInitialized,
	}
	po := &core.PreparedOperation{
		ID:   op.ID,
		Type: op.Type,
	}
	operations = append(operations, op)

	mdi := om.database.(*databasemocks.Plugin)
	fb := database.OperationQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("tx", id),
	)
	om.RegisterHandler(ctx, &mockHandler{Prepared: po}, []core.OpType{core.OpTypeBlockchainPinBatch})
	mdi.On("GetOperations", ctx, "ns1", filter).Return(operations, nil, nil)
	total, resubmitted, err := om.ResubmitOperations(ctx, id)
	assert.NoError(t, err)
	assert.Equal(t, total, 1)
	assert.Len(t, resubmitted, 1)

	mdi.AssertExpectations(t)
}

func TestResubmitIdempotentOperationSkipCached(t *testing.T) {
	om, cancel := newTestOperations(t)
	var id = fftypes.NewUUID()
	defer cancel()

	ctx := context.Background()
	opID := fftypes.NewUUID()
	operations := make([]*core.Operation, 0)
	op := &core.Operation{
		ID:     opID,
		Plugin: "blockchain",
		Type:   core.OpTypeBlockchainPinBatch,
		Status: core.OpStatusInitialized,
	}
	po := &core.PreparedOperation{
		ID:   op.ID,
		Type: op.Type,
	}
	operations = append(operations, op)
	opFlushInFlight := *op
	opFlushInFlight.Status = core.OpStatusFailed
	om.cache.Set(op.ID.String(), &opFlushInFlight)

	mdi := om.database.(*databasemocks.Plugin)
	fb := database.OperationQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("tx", id),
	)
	om.RegisterHandler(ctx, &mockHandler{Prepared: po}, []core.OpType{core.OpTypeBlockchainPinBatch})
	mdi.On("GetOperations", ctx, "ns1", filter).Return(operations, nil, nil)
	total, resubmitted, err := om.ResubmitOperations(ctx, id)
	assert.NoError(t, err)
	assert.Equal(t, total, 1)
	assert.Empty(t, resubmitted)

	mdi.AssertExpectations(t)
}

func TestResubmitIdempotentOperationLookupError(t *testing.T) {
	om, cancel := newTestOperations(t)
	var id = fftypes.NewUUID()
	defer cancel()

	ctx := context.Background()
	opID := fftypes.NewUUID()
	operations := make([]*core.Operation, 0)
	op := &core.Operation{
		ID:     opID,
		Plugin: "blockchain",
		Type:   core.OpTypeBlockchainPinBatch,
		Status: core.OpStatusFailed,
	}
	po := &core.PreparedOperation{
		ID:   op.ID,
		Type: op.Type,
	}
	operations = append(operations, op)

	mdi := om.database.(*databasemocks.Plugin)
	fb := database.OperationQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("tx", id),
	)
	om.RegisterHandler(ctx, &mockHandler{Prepared: po}, []core.OpType{core.OpTypeBlockchainPinBatch})
	mdi.On("GetOperations", ctx, "ns1", filter).Return(operations, nil, fmt.Errorf("pop"))
	_, _, err := om.ResubmitOperations(ctx, id)
	assert.Error(t, err)

	mdi.AssertExpectations(t)
}

func TestResubmitIdempotentOperationExecError(t *testing.T) {
	om, cancel := newTestOperations(t)
	var id = fftypes.NewUUID()
	defer cancel()

	ctx := context.Background()
	opID := fftypes.NewUUID()
	operations := make([]*core.Operation, 0)
	op := &core.Operation{
		ID:     opID,
		Plugin: "blockchain",
		Type:   core.OpTypeBlockchainPinBatch,
		Status: core.OpStatusInitialized,
	}
	po := &core.PreparedOperation{
		ID:   op.ID,
		Type: op.Type,
	}
	operations = append(operations, op)

	mdi := om.database.(*databasemocks.Plugin)
	fb := database.OperationQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("tx", id),
	)
	om.RegisterHandler(ctx, &mockHandler{Prepared: po, RunErr: fmt.Errorf("pop")}, []core.OpType{core.OpTypeBlockchainPinBatch})
	mdi.On("GetOperations", ctx, "ns1", filter).Return(operations, nil, nil)
	_, _, err := om.ResubmitOperations(ctx, id)
	assert.Error(t, err)

	mdi.AssertExpectations(t)
}

func TestErrTernaryHelper(t *testing.T) {
	assert.Equal(t, core.OpPhasePending, ErrTernary(nil, core.OpPhaseInitializing, core.OpPhasePending))
	assert.Equal(t, core.OpPhaseInitializing, ErrTernary(fmt.Errorf("pop"), core.OpPhaseInitializing, core.OpPhasePending))
}

func TestSubmitBulkOperationUpdates(t *testing.T) {
	om, cancel := newTestOperations(t)
	defer cancel()

	ctx := context.Background()

	operations := make([]*core.Operation, 0)
	opID := fftypes.NewUUID()
	op := &core.Operation{
		Namespace: "ns1",
		ID:        opID,
		Plugin:    "blockchain",
		Type:      core.OpTypeBlockchainPinBatch,
		Status:    core.OpStatusInitialized,
	}
	op2ID := fftypes.NewUUID()
	op2 := &core.Operation{
		Namespace: "ns2",
		ID:        op2ID,
		Plugin:    "blockchain",
		Type:      core.OpTypeBlockchainContractDeploy,
		Status:    core.OpStatusInitialized,
	}
	operations = append(operations, op, op2)

	submittedUpdate := &core.OperationUpdate{
		NamespacedOpID: "ns1:" + opID.String(),
		Status:         core.OpStatusSucceeded,
		ErrorMessage:   "my-error-message",
		Plugin:         "blockchain",
	}

	submittedUpdate2 := &core.OperationUpdate{
		NamespacedOpID: "ns1:" + op2ID.String(),
		Status:         core.OpStatusSucceeded,
		ErrorMessage:   "my-error-message",
		Plugin:         "blockchain",
	}

	mdi := om.database.(*databasemocks.Plugin)
	mdi.On("GetOperations", ctx, "ns1", mock.Anything).Return(operations, nil, nil)

	mdi.On("UpdateOperation", ctx, "ns1", opID, mock.Anything, mock.Anything).Return(true, nil)
	mdi.On("UpdateOperation", ctx, "ns2", op2ID, mock.Anything, mock.Anything).Return(true, nil)

	err := om.SubmitBulkOperationUpdates(ctx, []*core.OperationUpdate{submittedUpdate, submittedUpdate2})
	assert.NoError(t, err)
}

func TestSubmitBulkOperationUpdatesErrorNoPlugin(t *testing.T) {
	om, cancel := newTestOperations(t)
	defer cancel()

	ctx := context.Background()

	operations := make([]*core.Operation, 0)
	opID := fftypes.NewUUID()
	op := &core.Operation{
		ID:     opID,
		Plugin: "blockchain",
		Type:   core.OpTypeBlockchainPinBatch,
		Status: core.OpStatusInitialized,
	}

	operations = append(operations, op)

	submittedUpdate := &core.OperationUpdate{
		NamespacedOpID: "ns1:" + opID.String(),
		Status:         core.OpStatusSucceeded,
		ErrorMessage:   "my-error-message",
	}

	mdi := om.database.(*databasemocks.Plugin)
	mdi.On("GetOperations", ctx, "ns1", mock.Anything).Return(operations, nil, nil)

	err := om.SubmitBulkOperationUpdates(ctx, []*core.OperationUpdate{submittedUpdate})
	assert.Error(t, err)
	assert.Regexp(t, "FF10479", err.Error())
}

func TestSubmitBulkOperationUpdatesErrorWrongNamespace(t *testing.T) {
	om, cancel := newTestOperations(t)
	defer cancel()

	ctx := context.Background()

	operations := make([]*core.Operation, 0)
	opID := fftypes.NewUUID()
	op := &core.Operation{
		ID:     opID,
		Plugin: "blockchain",
		Type:   core.OpTypeBlockchainPinBatch,
		Status: core.OpStatusInitialized,
	}
	operations = append(operations, op)

	submittedUpdate := &core.OperationUpdate{
		NamespacedOpID: "different-namespace:" + opID.String(),
		Status:         core.OpStatusSucceeded,
		ErrorMessage:   "my-error-message",
		Plugin:         "blockchain",
	}

	err := om.SubmitBulkOperationUpdates(ctx, []*core.OperationUpdate{submittedUpdate})
	assert.Error(t, err)
	assert.Regexp(t, "FF10478", err.Error())
}

func TestSubmitBulkOperationUpdatesIgnoredBadID(t *testing.T) {
	om, cancel := newTestOperations(t)
	defer cancel()

	ctx := context.Background()

	submittedUpdate := &core.OperationUpdate{
		NamespacedOpID: "ns1:BAD-UUID",
		Status:         core.OpStatusSucceeded,
		ErrorMessage:   "my-error-message",
		Plugin:         "blockchain",
	}

	err := om.SubmitBulkOperationUpdates(ctx, []*core.OperationUpdate{submittedUpdate})
	assert.Error(t, err)
	assert.Regexp(t, "FF00138", err.Error())
}

func TestSubmitBulkOperationUpdatesError(t *testing.T) {
	om, cancel := newTestOperations(t)
	defer cancel()

	ctx := context.Background()

	operations := make([]*core.Operation, 0)
	opID := fftypes.NewUUID()
	op := &core.Operation{
		ID:     opID,
		Plugin: "blockchain",
		Type:   core.OpTypeBlockchainPinBatch,
		Status: core.OpStatusInitialized,
	}
	operations = append(operations, op)

	submittedUpdate := &core.OperationUpdate{
		NamespacedOpID: "ns1:" + opID.String(),
		Status:         core.OpStatusSucceeded,
		ErrorMessage:   "my-error-message",
		Plugin:         "blockchain",
	}

	mdi := om.database.(*databasemocks.Plugin)
	mdi.On("GetOperations", ctx, "ns1", mock.Anything).Return(operations, nil, errors.New("Failed to get operations"))

	err := om.SubmitBulkOperationUpdates(ctx, []*core.OperationUpdate{submittedUpdate})
	assert.Error(t, err)
}
