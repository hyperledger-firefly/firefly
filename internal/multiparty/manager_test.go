// Copyright © 2022 Kaleido, Inc.
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

package multiparty

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/internal/txcommon"
	"github.com/hyperledger-firefly/firefly/mocks/blockchainmocks"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/mocks/metricsmocks"
	"github.com/hyperledger-firefly/firefly/mocks/operationmocks"
	"github.com/hyperledger-firefly/firefly/mocks/txcommonmocks"
	"github.com/hyperledger-firefly/firefly/pkg/blockchain"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

type testMultipartyManager struct {
	multipartyManager
	mdi *databasemocks.Plugin
	mbi *blockchainmocks.Plugin
	mom *operationmocks.Manager
	mmi *metricsmocks.Manager
	mth *txcommonmocks.Helper
}

func (mp *testMultipartyManager) cleanup(t *testing.T) {
	mp.mdi.AssertExpectations(t)
	mp.mbi.AssertExpectations(t)
	mp.mom.AssertExpectations(t)
	mp.mmi.AssertExpectations(t)
}

func newTestMultipartyManager() *testMultipartyManager {
	nm := &testMultipartyManager{
		mdi: &databasemocks.Plugin{},
		mbi: &blockchainmocks.Plugin{},
		mom: &operationmocks.Manager{},
		mmi: &metricsmocks.Manager{},
		mth: &txcommonmocks.Helper{},
		multipartyManager: multipartyManager{
			namespace: &core.Namespace{
				Name:        "ns1",
				NetworkName: "ns1",
				Contracts: &core.MultipartyContracts{
					Active: &core.MultipartyContract{},
				},
			},
		},
	}

	nm.multipartyManager.database = nm.mdi
	nm.multipartyManager.blockchain = nm.mbi
	nm.multipartyManager.operations = nm.mom
	nm.multipartyManager.metrics = nm.mmi
	nm.multipartyManager.txHelper = nm.mth
	return nm
}

func TestNewMultipartyManager(t *testing.T) {
	mdi := &databasemocks.Plugin{}
	mbi := &blockchainmocks.Plugin{}
	mom := &operationmocks.Manager{}
	mmi := &metricsmocks.Manager{}
	mth := &txcommonmocks.Helper{}
	config := Config{
		Org:       RootOrg{Name: "org1"},
		Node:      LocalNode{Name: "node1"},
		Contracts: []blockchain.MultipartyContract{},
	}
	mom.On("RegisterHandler", mock.Anything, mock.Anything, []core.OpType{
		core.OpTypeBlockchainPinBatch,
		core.OpTypeBlockchainNetworkAction,
	}).Return()
	ns := &core.Namespace{Name: "ns1", NetworkName: "ns1"}
	nm, err := NewMultipartyManager(context.Background(), ns, config, mdi, mbi, mom, mmi, mth)
	assert.NotNil(t, nm)
	assert.NoError(t, err)
	assert.Equal(t, "MultipartyManager", nm.Name())
	assert.Equal(t, config.Org, nm.RootOrg())
	assert.Equal(t, config.Node, nm.LocalNode())
}

func TestInitFail(t *testing.T) {
	config := Config{Contracts: []blockchain.MultipartyContract{}}
	_, err := NewMultipartyManager(context.Background(), &core.Namespace{}, config, nil, nil, nil, nil, nil)
	assert.Regexp(t, "FF10128", err)
}

func TestConfigureContract(t *testing.T) {
	location := fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "0x123",
	}.String())

	mp := newTestMultipartyManager()
	defer mp.cleanup(t)

	mp.mbi.On("GetNetworkVersion", mock.Anything, mock.Anything).Return(1, nil)
	mp.mbi.On("AddFireflySubscription", mock.Anything, mock.Anything, mock.Anything, "001/002/003").Return("test", nil)
	mp.mdi.On("UpsertNamespace", mock.Anything, mock.AnythingOfType("*core.Namespace"), true).Return(nil)
	mp.mdi.On("GetBlockchainEvents", mock.Anything, "ns1", mock.MatchedBy(func(f ffapi.Filter) bool {
		fi, err := f.Finalize()
		assert.NoError(t, err)
		return fi.Limit == 1 && strings.Contains(fi.String(), "listener")
	})).Return([]*core.BlockchainEvent{
		{Namespace: "ns1", ID: fftypes.NewUUID(), ProtocolID: "001/002/003"},
	}, nil, nil).Once()

	mp.multipartyManager.config.Contracts = []blockchain.MultipartyContract{{
		FirstEvent: "0",
		Location:   location,
	}}

	err := mp.ConfigureContract(context.Background())
	assert.NoError(t, err)
}

func TestConfigureContractLocationChanged(t *testing.T) {
	location := fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "0x123",
	}.String())
	location2 := fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "0x456",
	}.String())

	mp := newTestMultipartyManager()
	defer mp.cleanup(t)

	mp.mbi.On("GetNetworkVersion", mock.Anything, mock.Anything).Return(1, nil)
	mp.mbi.On("AddFireflySubscription", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("test", nil)
	mp.mdi.On("UpsertNamespace", mock.Anything, mock.AnythingOfType("*core.Namespace"), true).Return(nil)
	mp.mdi.On("GetBlockchainEvents", mock.Anything, "ns1", mock.Anything).Return(nil, nil, nil).Once()

	mp.multipartyManager.namespace.Contracts = &core.MultipartyContracts{
		Active: &core.MultipartyContract{
			Index:    0,
			Location: location,
		},
	}
	mp.multipartyManager.config.Contracts = []blockchain.MultipartyContract{{
		FirstEvent: "0",
		Location:   location2,
	}}

	err := mp.ConfigureContract(context.Background())
	assert.NoError(t, err)
}

func TestConfigureContractDeprecatedConfig(t *testing.T) {
	mp := newTestMultipartyManager()
	defer mp.cleanup(t)
	mp.namespace.Contracts = nil

	mp.mbi.On("GetAndConvertDeprecatedContractConfig", context.Background()).Return(fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "0x123",
	}.String()), "0", nil)
	mp.mbi.On("GetNetworkVersion", mock.Anything, mock.Anything).Return(1, nil)
	mp.mbi.On("AddFireflySubscription", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("test", nil)
	mp.mdi.On("UpsertNamespace", mock.Anything, mock.AnythingOfType("*core.Namespace"), true).Return(nil)
	mp.mdi.On("GetBlockchainEvents", mock.Anything, "ns1", mock.Anything).Return(nil, nil, nil).Once()

	err := mp.ConfigureContract(context.Background())

	location := fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "0x123",
	}.String())
	assert.Equal(t, location, mp.namespace.Contracts.Active.Location)
	assert.Equal(t, "0", mp.namespace.Contracts.Active.FirstEvent)
	assert.NoError(t, err)
}

func TestResolveContractDeprecatedConfigError(t *testing.T) {
	mp := newTestMultipartyManager()
	defer mp.cleanup(t)

	mp.mbi.On("GetAndConvertDeprecatedContractConfig", context.Background()).Return(nil, "", fmt.Errorf("pop"))

	_, err := mp.resolveFireFlyContract(context.Background(), 0)
	assert.Regexp(t, "pop", err)
}

func TestResolveContractDeprecatedConfigNewestBlock(t *testing.T) {
	mp := newTestMultipartyManager()
	defer mp.cleanup(t)

	mp.mbi.On("GetAndConvertDeprecatedContractConfig", context.Background()).Return(fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "0x123",
	}.String()), "newst", nil)

	_, err := mp.resolveFireFlyContract(context.Background(), 0)
	assert.NoError(t, err)
}

func TestConfigureContractBadIndex(t *testing.T) {
	location := fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "0x123",
	}.String())

	mp := newTestMultipartyManager()
	defer mp.cleanup(t)

	mp.multipartyManager.namespace.Contracts = &core.MultipartyContracts{
		Active: &core.MultipartyContract{Index: 1},
	}
	mp.multipartyManager.config.Contracts = []blockchain.MultipartyContract{{
		FirstEvent: "0",
		Location:   location,
	}}

	err := mp.ConfigureContract(context.Background())
	assert.Regexp(t, "FF10396", err)
}

func TestConfigureContractNetworkVersionFail(t *testing.T) {
	location := fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "0x123",
	}.String())

	mp := newTestMultipartyManager()
	defer mp.cleanup(t)

	mp.mbi.On("GetNetworkVersion", mock.Anything, mock.Anything).Return(0, fmt.Errorf("pop"))

	mp.multipartyManager.namespace.Contracts = &core.MultipartyContracts{
		Active: &core.MultipartyContract{Index: 0},
	}
	mp.multipartyManager.config.Contracts = []blockchain.MultipartyContract{{
		FirstEvent: "0",
		Location:   location,
	}}

	err := mp.ConfigureContract(context.Background())
	assert.Regexp(t, "pop", err)
}

func TestSubmitNetworkAction(t *testing.T) {
	location := fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "0x123",
	}.String())
	txid := fftypes.NewUUID()

	mp := newTestMultipartyManager()
	defer mp.cleanup(t)

	mp.multipartyManager.namespace.Contracts = &core.MultipartyContracts{
		Active: &core.MultipartyContract{Index: 0},
	}
	mp.multipartyManager.config.Contracts = []blockchain.MultipartyContract{{
		FirstEvent: "0",
		Location:   location,
	}}

	mp.mbi.On("GetNetworkVersion", mock.Anything, mock.Anything).Return(1, nil)
	mp.mbi.On("AddFireflySubscription", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("test", nil)
	mp.mdi.On("UpsertNamespace", mock.Anything, mock.AnythingOfType("*core.Namespace"), true).Return(nil)
	mp.mdi.On("GetBlockchainEvents", mock.Anything, "ns1", mock.Anything).Return(nil, nil, nil).Once()
	mp.mth.On("SubmitNewTransaction", mock.Anything, core.TransactionTypeNetworkAction, core.IdempotencyKey("")).Return(txid, nil)
	mp.mbi.On("Name").Return("ut")
	mp.mom.On("AddOrReuseOperation", context.Background(), mock.MatchedBy(func(op *core.Operation) bool {
		assert.Equal(t, core.OpTypeBlockchainNetworkAction, op.Type)
		assert.Equal(t, "ut", op.Plugin)
		assert.Equal(t, *txid, *op.Transaction)
		assert.Equal(t, "terminate", op.Input.GetString("type"))
		return true
	})).Return(nil)
	mp.mom.On("RunOperation", mock.Anything, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(networkActionData)
		return op.Type == core.OpTypeBlockchainNetworkAction && data.Type == core.NetworkActionTerminate
	}), false).Return(nil, nil)

	err := mp.ConfigureContract(context.Background())
	assert.NoError(t, err)
	err = mp.SubmitNetworkAction(context.Background(), "0x123", &core.NetworkAction{Type: core.NetworkActionTerminate}, false)
	assert.Nil(t, err)

	mp.mbi.AssertExpectations(t)
	mp.mth.AssertExpectations(t)
	mp.mom.AssertExpectations(t)
}

func TestSubmitNetworkActionTXFail(t *testing.T) {
	location := fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "0x123",
	}.String())

	mp := newTestMultipartyManager()
	defer mp.cleanup(t)

	mp.multipartyManager.namespace.Contracts = &core.MultipartyContracts{
		Active: &core.MultipartyContract{Index: 0},
	}
	mp.multipartyManager.config.Contracts = []blockchain.MultipartyContract{{
		FirstEvent: "0",
		Location:   location,
	}}

	mp.mbi.On("GetNetworkVersion", mock.Anything, mock.Anything).Return(1, nil)
	mp.mbi.On("AddFireflySubscription", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("test", nil)
	mp.mdi.On("UpsertNamespace", mock.Anything, mock.AnythingOfType("*core.Namespace"), true).Return(nil)
	mp.mdi.On("GetBlockchainEvents", mock.Anything, "ns1", mock.Anything).Return(nil, nil, nil).Once()
	mp.mth.On("SubmitNewTransaction", mock.Anything, core.TransactionTypeNetworkAction, core.IdempotencyKey("")).Return(nil, fmt.Errorf("pop"))

	err := mp.ConfigureContract(context.Background())
	assert.NoError(t, err)
	err = mp.SubmitNetworkAction(context.Background(), "0x123", &core.NetworkAction{Type: core.NetworkActionTerminate}, false)
	assert.EqualError(t, err, "pop")

	mp.mbi.AssertExpectations(t)
	mp.mth.AssertExpectations(t)
}

func TestConfigureContractLookupBlockchainEventFail(t *testing.T) {
	location := fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "0x123",
	}.String())

	mp := newTestMultipartyManager()
	defer mp.cleanup(t)

	mp.multipartyManager.namespace.Contracts = &core.MultipartyContracts{
		Active: &core.MultipartyContract{Index: 0},
	}
	mp.multipartyManager.config.Contracts = []blockchain.MultipartyContract{{
		FirstEvent: "0",
		Location:   location,
	}}

	mp.mbi.On("GetNetworkVersion", mock.Anything, mock.Anything).Return(1, nil)
	mp.mdi.On("GetBlockchainEvents", mock.Anything, "ns1", mock.Anything).Return(nil, nil, fmt.Errorf("pop")).Once()

	err := mp.ConfigureContract(context.Background())
	assert.EqualError(t, err, "pop")

	mp.mbi.AssertExpectations(t)
	mp.mth.AssertExpectations(t)
}

func TestSubmitNetworkActionOpFail(t *testing.T) {
	location := fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "0x123",
	}.String())
	txid := fftypes.NewUUID()

	mp := newTestMultipartyManager()
	defer mp.cleanup(t)

	mp.multipartyManager.namespace.Contracts = &core.MultipartyContracts{
		Active: &core.MultipartyContract{Index: 0},
	}
	mp.multipartyManager.config.Contracts = []blockchain.MultipartyContract{{
		FirstEvent: "0",
		Location:   location,
	}}

	mp.mbi.On("GetNetworkVersion", mock.Anything, mock.Anything).Return(1, nil)
	mp.mbi.On("AddFireflySubscription", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("test", nil)
	mp.mdi.On("UpsertNamespace", mock.Anything, mock.AnythingOfType("*core.Namespace"), true).Return(nil)
	mp.mdi.On("GetBlockchainEvents", mock.Anything, "ns1", mock.Anything).Return(nil, nil, nil).Once()
	mp.mth.On("SubmitNewTransaction", mock.Anything, core.TransactionTypeNetworkAction, core.IdempotencyKey("")).Return(txid, nil)
	mp.mbi.On("Name").Return("ut")
	mp.mom.On("AddOrReuseOperation", context.Background(), mock.Anything).Return(fmt.Errorf("pop"))

	err := mp.ConfigureContract(context.Background())
	assert.NoError(t, err)
	err = mp.SubmitNetworkAction(context.Background(), "0x123", &core.NetworkAction{Type: core.NetworkActionTerminate}, false)
	assert.EqualError(t, err, "pop")

	mp.mbi.AssertExpectations(t)
	mp.mth.AssertExpectations(t)
	mp.mom.AssertExpectations(t)
}

func TestSubmitNetworkActionBadType(t *testing.T) {
	location := fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "0x123",
	}.String())

	mp := newTestMultipartyManager()
	defer mp.cleanup(t)

	mp.mbi.On("GetNetworkVersion", mock.Anything, mock.Anything).Return(1, nil)
	mp.mbi.On("AddFireflySubscription", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("test", nil)
	mp.mdi.On("UpsertNamespace", mock.Anything, mock.AnythingOfType("*core.Namespace"), true).Return(nil)
	mp.mdi.On("GetBlockchainEvents", mock.Anything, "ns1", mock.Anything).Return(nil, nil, nil).Once()

	mp.multipartyManager.namespace.Contracts = &core.MultipartyContracts{
		Active: &core.MultipartyContract{Index: 0},
	}
	mp.multipartyManager.config.Contracts = []blockchain.MultipartyContract{{
		FirstEvent: "0",
		Location:   location,
	}}

	err := mp.ConfigureContract(context.Background())
	assert.NoError(t, err)
	err = mp.SubmitNetworkAction(context.Background(), "0x123", &core.NetworkAction{Type: "BAD"}, false)
	assert.Regexp(t, "FF10397", err)

	mp.mbi.AssertExpectations(t)
}

func TestSubmitBatchPinOk(t *testing.T) {
	mp := newTestMultipartyManager()
	defer mp.cleanup(t)
	ctx := context.Background()

	batch := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			ID: fftypes.NewUUID(),
			SignerRef: core.SignerRef{
				Author: "id1",
				Key:    "0x12345",
			},
		},
		TX: core.TransactionRef{
			ID: fftypes.NewUUID(),
		},
	}
	contexts := []*fftypes.Bytes32{}

	mp.mbi.On("Name").Return("ut")
	mp.mom.On("AddOrReuseOperation", ctx, mock.MatchedBy(func(op *core.Operation) bool {
		assert.Equal(t, core.OpTypeBlockchainPinBatch, op.Type)
		assert.Equal(t, "ut", op.Plugin)
		assert.Equal(t, *batch.TX.ID, *op.Transaction)
		assert.Equal(t, "payload1", op.Input.GetString("payloadRef"))
		return true
	})).Return(nil)
	mp.mmi.On("IsMetricsEnabled").Return(false)
	mp.mom.On("RunOperation", mock.Anything, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(txcommon.BatchPinData)
		return op.Type == core.OpTypeBlockchainPinBatch && data.Batch == batch
	}), false).Return(nil, nil)

	err := mp.SubmitBatchPin(ctx, batch, contexts, "payload1", false)
	assert.NoError(t, err)
}

func TestSubmitPinnedBatchWithMetricsOk(t *testing.T) {
	mp := newTestMultipartyManager()
	defer mp.cleanup(t)
	ctx := context.Background()

	batch := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			ID: fftypes.NewUUID(),
			SignerRef: core.SignerRef{
				Author: "id1",
				Key:    "0x12345",
			},
		},
		TX: core.TransactionRef{
			ID: fftypes.NewUUID(),
		},
	}
	contexts := []*fftypes.Bytes32{}

	mp.mbi.On("Name").Return("ut")
	mp.mom.On("AddOrReuseOperation", ctx, mock.MatchedBy(func(op *core.Operation) bool {
		assert.Equal(t, core.OpTypeBlockchainPinBatch, op.Type)
		assert.Equal(t, "ut", op.Plugin)
		assert.Equal(t, *batch.TX.ID, *op.Transaction)
		assert.Equal(t, "payload1", op.Input.GetString("payloadRef"))
		return true
	})).Return(nil)
	mp.mmi.On("IsMetricsEnabled").Return(true)
	mp.mmi.On("CountBatchPin", "ns1").Return()
	mp.mom.On("RunOperation", mock.Anything, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(txcommon.BatchPinData)
		return op.Type == core.OpTypeBlockchainPinBatch && data.Batch == batch
	}), false).Return(nil, nil)

	err := mp.SubmitBatchPin(ctx, batch, contexts, "payload1", false)
	assert.NoError(t, err)
}

func TestSubmitBatchPinWithBatchOk(t *testing.T) {
	mp := newTestMultipartyManager()
	defer mp.cleanup(t)
	ctx := context.Background()

	batch := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			ID: fftypes.NewUUID(),
			SignerRef: core.SignerRef{
				Author: "id1",
				Key:    "0x12345",
			},
		},
		TX: core.TransactionRef{
			ID:   fftypes.NewUUID(),
			Type: core.TransactionTypeContractInvokePin,
		},
	}
	contexts := []*fftypes.Bytes32{fftypes.NewRandB32()}
	invokeOp := &core.Operation{
		Type: core.OpTypeBlockchainInvoke,
	}

	mp.mth.On("FindOperationInTransaction", ctx, batch.TX.ID, core.OpTypeBlockchainInvoke).Return(invokeOp, nil)
	mp.mom.On("RunOperation", mock.Anything, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(txcommon.BlockchainInvokeData)
		assert.Equal(t, contexts, data.BatchPin.Contexts)
		assert.Equal(t, "payload1", data.BatchPin.PayloadRef)
		return op.Type == core.OpTypeBlockchainInvoke && data.BatchPin.Batch == batch
	}), false).Return(nil, nil)

	err := mp.SubmitBatchPin(ctx, batch, contexts, "payload1", false)
	assert.NoError(t, err)
}

func TestSubmitBatchPinWithBatchOpFailure(t *testing.T) {
	mp := newTestMultipartyManager()
	defer mp.cleanup(t)
	ctx := context.Background()

	batch := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			ID: fftypes.NewUUID(),
			SignerRef: core.SignerRef{
				Author: "id1",
				Key:    "0x12345",
			},
		},
		TX: core.TransactionRef{
			ID:   fftypes.NewUUID(),
			Type: core.TransactionTypeContractInvokePin,
		},
	}
	contexts := []*fftypes.Bytes32{fftypes.NewRandB32()}

	mp.mth.On("FindOperationInTransaction", ctx, batch.TX.ID, core.OpTypeBlockchainInvoke).Return(nil, fmt.Errorf("pop"))

	err := mp.SubmitBatchPin(ctx, batch, contexts, "payload1", false)
	assert.EqualError(t, err, "pop")
}

func TestSubmitBatchPinWithBatchOpMalformed(t *testing.T) {
	mp := newTestMultipartyManager()
	defer mp.cleanup(t)
	ctx := context.Background()

	batch := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			ID: fftypes.NewUUID(),
			SignerRef: core.SignerRef{
				Author: "id1",
				Key:    "0x12345",
			},
		},
		TX: core.TransactionRef{
			ID:   fftypes.NewUUID(),
			Type: core.TransactionTypeContractInvokePin,
		},
	}
	contexts := []*fftypes.Bytes32{fftypes.NewRandB32()}
	invokeOp := &core.Operation{
		Type: core.OpTypeBlockchainInvoke,
		Input: fftypes.JSONObject{
			"interface": "NOT-A-UUID",
		},
	}

	mp.mth.On("FindOperationInTransaction", ctx, batch.TX.ID, core.OpTypeBlockchainInvoke).Return(invokeOp, nil)

	err := mp.SubmitBatchPin(ctx, batch, contexts, "payload1", false)
	assert.Regexp(t, "FF00127", err)
}

func TestSubmitBatchPinWithBatchOpNotFound(t *testing.T) {
	mp := newTestMultipartyManager()
	defer mp.cleanup(t)
	ctx := context.Background()

	batch := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			ID: fftypes.NewUUID(),
			SignerRef: core.SignerRef{
				Author: "id1",
				Key:    "0x12345",
			},
		},
		TX: core.TransactionRef{
			ID:   fftypes.NewUUID(),
			Type: core.TransactionTypeContractInvokePin,
		},
	}
	contexts := []*fftypes.Bytes32{fftypes.NewRandB32()}

	mp.mth.On("FindOperationInTransaction", ctx, batch.TX.ID, core.OpTypeBlockchainInvoke).Return(nil, nil)
	mp.mbi.On("Name").Return("ut")
	mp.mom.On("AddOrReuseOperation", ctx, mock.MatchedBy(func(op *core.Operation) bool {
		assert.Equal(t, core.OpTypeBlockchainPinBatch, op.Type)
		assert.Equal(t, "ut", op.Plugin)
		assert.Equal(t, *batch.TX.ID, *op.Transaction)
		assert.Equal(t, "payload1", op.Input.GetString("payloadRef"))
		return true
	})).Return(nil)
	mp.mmi.On("IsMetricsEnabled").Return(true)
	mp.mmi.On("CountBatchPin", "ns1").Return()
	mp.mom.On("RunOperation", mock.Anything, mock.MatchedBy(func(op *core.PreparedOperation) bool {
		data := op.Data.(txcommon.BatchPinData)
		return op.Type == core.OpTypeBlockchainPinBatch && data.Batch == batch
	}), false).Return(nil, nil)

	err := mp.SubmitBatchPin(ctx, batch, contexts, "payload1", false)
	assert.NoError(t, err)
}

func TestSubmitPinnedBatchOpFail(t *testing.T) {
	mp := newTestMultipartyManager()
	defer mp.cleanup(t)
	ctx := context.Background()

	batch := &core.BatchPersisted{
		BatchHeader: core.BatchHeader{
			ID: fftypes.NewUUID(),
			SignerRef: core.SignerRef{
				Author: "id1",
				Key:    "0x12345",
			},
		},
		TX: core.TransactionRef{
			ID: fftypes.NewUUID(),
		},
	}
	contexts := []*fftypes.Bytes32{}

	mp.mbi.On("Name").Return("ut")
	mp.mom.On("AddOrReuseOperation", ctx, mock.Anything).Return(fmt.Errorf("pop"))
	err := mp.SubmitBatchPin(ctx, batch, contexts, "payload1", false)
	assert.Regexp(t, "pop", err)
}

func TestGetNetworkVersion(t *testing.T) {
	location := fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "0x123",
	}.String())

	mp := newTestMultipartyManager()
	defer mp.cleanup(t)

	mp.mbi.On("GetNetworkVersion", mock.Anything, mock.Anything).Return(1, nil)
	mp.mbi.On("AddFireflySubscription", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("test", nil)
	mp.mdi.On("UpsertNamespace", mock.Anything, mock.AnythingOfType("*core.Namespace"), true).Return(nil)
	mp.mdi.On("GetBlockchainEvents", mock.Anything, "ns1", mock.Anything).Return(nil, nil, nil).Once()

	mp.multipartyManager.namespace.Contracts = &core.MultipartyContracts{
		Active: &core.MultipartyContract{Index: 0},
	}
	mp.multipartyManager.config.Contracts = []blockchain.MultipartyContract{{
		FirstEvent: "0",
		Location:   location,
	}}

	err := mp.ConfigureContract(context.Background())
	assert.NoError(t, err)

	version := mp.GetNetworkVersion()
	assert.Equal(t, 1, version)
}

func TestConfgureAndTerminateContract(t *testing.T) {
	location := fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "0x123",
	}.String())

	mp := newTestMultipartyManager()
	defer mp.cleanup(t)

	mp.mbi.On("GetNetworkVersion", mock.Anything, mock.Anything).Return(1, nil)
	mp.mbi.On("AddFireflySubscription", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return("test", nil)
	mp.mdi.On("UpsertNamespace", mock.Anything, mock.AnythingOfType("*core.Namespace"), true).Return(nil)
	mp.mbi.On("RemoveFireflySubscription", mock.Anything, mock.Anything).Return(nil)
	mp.mdi.On("GetBlockchainEvents", mock.Anything, "ns1", mock.Anything).Return(nil, nil, nil)

	mp.multipartyManager.namespace.Contracts = &core.MultipartyContracts{
		Active: &core.MultipartyContract{Index: 0},
	}
	mp.multipartyManager.config.Contracts = []blockchain.MultipartyContract{{
		FirstEvent: "0",
		Location:   location,
	}, {
		FirstEvent: "0",
		Location:   location,
	}}

	err := mp.ConfigureContract(context.Background())
	assert.NoError(t, err)

	err = mp.TerminateContract(context.Background(), location, &blockchain.Event{})
	assert.NoError(t, err)
}

func TestTerminateContractError(t *testing.T) {
	mp := newTestMultipartyManager()
	defer mp.cleanup(t)

	mp.mbi.On("RemoveFireflySubscription", mock.Anything, mock.Anything).Return()

	location := fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "0x123",
	}.String())

	mp.multipartyManager.namespace.Contracts = &core.MultipartyContracts{
		Active: &core.MultipartyContract{Index: 0, Location: location},
	}

	err := mp.TerminateContract(context.Background(), location, &blockchain.Event{})
	assert.Regexp(t, "FF10396", err)
}

func TestTerminateContractWrongAddress(t *testing.T) {
	mp := newTestMultipartyManager()
	defer mp.cleanup(t)

	location := fftypes.JSONAnyPtr(fftypes.JSONObject{
		"address": "0x123",
	}.String())

	mp.multipartyManager.namespace.Contracts = &core.MultipartyContracts{
		Active: &core.MultipartyContract{Index: 0, Location: location},
	}

	err := mp.TerminateContract(context.Background(), fftypes.JSONAnyPtr("{}"), &blockchain.Event{})
	assert.NoError(t, err)
}
