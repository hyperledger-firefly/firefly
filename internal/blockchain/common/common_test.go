// Copyright © 2024 Kaleido, Inc.
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

package common

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/go-resty/resty/v2"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/internal/operations"
	"github.com/hyperledger-firefly/firefly/mocks/blockchainmocks"
	"github.com/hyperledger-firefly/firefly/mocks/coremocks"
	"github.com/hyperledger-firefly/firefly/pkg/blockchain"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestCallbackOperationUpdate(t *testing.T) {
	nsOpID := "ns1:" + fftypes.NewUUID().String()

	mbi := &blockchainmocks.Plugin{}
	mcb := &coremocks.OperationCallbacks{}
	cb := NewBlockchainCallbacks()
	cb.SetOperationalHandler("ns1", mcb)

	mbi.On("Name").Return("utblockchain")
	mcb.On("OperationUpdate", mock.MatchedBy(func(update *core.OperationUpdateAsync) bool {
		return update.NamespacedOpID == nsOpID &&
			update.Status == core.OpStatusSucceeded &&
			update.BlockchainTXID == "tx1" &&
			update.ErrorMessage == "err" &&
			update.Plugin == "utblockchain"
	})).Return().Once()
	cb.OperationUpdate(context.Background(), mbi, nsOpID, core.OpStatusSucceeded, "tx1", "err", fftypes.JSONObject{})

	nsOpID = "ns2:" + fftypes.NewUUID().String()
	cb.OperationUpdate(context.Background(), mbi, nsOpID, core.OpStatusSucceeded, "tx1", "err", fftypes.JSONObject{})

	mcb.AssertExpectations(t)
}

func matchBatchWithEvent(protocolID string) interface{} {
	return mock.MatchedBy(func(batch []*blockchain.EventToDispatch) bool {
		return len(batch) == 1 &&
			batch[0].Type == blockchain.EventTypeForListener &&
			batch[0].ForListener.ProtocolID == protocolID
	})
}

func TestCallbackBlockchainEvent(t *testing.T) {
	event := &blockchain.EventForListener{
		Event: &blockchain.Event{
			ProtocolID: "012345",
		},
	}

	mcb := &blockchainmocks.Callbacks{}
	cb := NewBlockchainCallbacks()
	cb.SetHandler("ns1", mcb)

	mcb.On("BlockchainEventBatch", matchBatchWithEvent("012345")).Return(nil).Once()
	events := make(EventsToDispatch)
	cb.PrepareBlockchainEvent(context.Background(), events, "ns1", event)
	err := cb.DispatchBlockchainEvents(context.Background(), events)
	assert.NoError(t, err)

	events = make(EventsToDispatch)
	cb.PrepareBlockchainEvent(context.Background(), events, "ns2", event)
	err = cb.DispatchBlockchainEvents(context.Background(), events)
	assert.NoError(t, err)

	mcb.On("BlockchainEventBatch", matchBatchWithEvent("012345")).Return(fmt.Errorf("pop")).Once()
	events = make(EventsToDispatch)
	cb.PrepareBlockchainEvent(context.Background(), events, "", event)
	err = cb.DispatchBlockchainEvents(context.Background(), events)
	assert.EqualError(t, err, "pop")

	cb.SetHandler("ns1", nil)
	assert.Empty(t, cb.(*callbacks).handlers)
	cb.SetOperationalHandler("ns1", nil)
	assert.Empty(t, cb.(*callbacks).opHandlers)

	mcb.AssertExpectations(t)
}

func TestCallbackBatchPinBadBatch(t *testing.T) {
	event := &blockchain.Event{}
	verifier := &core.VerifierRef{}
	params := &BatchPinParams{}

	mcb := &blockchainmocks.Callbacks{}
	cb := NewBlockchainCallbacks()
	cb.SetHandler("ns1", mcb)

	sub := &SubscriptionInfo{
		Version:     2,
		V2Namespace: "ns1",
	}
	events := make(EventsToDispatch)
	cb.PrepareBatchPinOrNetworkAction(context.Background(), events, sub, fftypes.JSONAnyPtr("{}"), event, verifier, params)
	err := cb.DispatchBlockchainEvents(context.Background(), events)
	assert.NoError(t, err)

	mcb.AssertExpectations(t)
}

func matchBatchPinEvent(ns string, txType core.TransactionType) interface{} {
	return mock.MatchedBy(func(batch []*blockchain.EventToDispatch) bool {
		return len(batch) == 1 &&
			batch[0].Type == blockchain.EventTypeBatchPinComplete &&
			batch[0].BatchPinComplete.Namespace == ns &&
			batch[0].BatchPinComplete.Batch.TransactionType == txType
	})
}

func TestBatchPinContractInvokePin(t *testing.T) {
	event := &blockchain.Event{}
	verifier := &core.VerifierRef{}
	params := &BatchPinParams{
		UUIDs:     "0xe19af8b390604051812d7597d19adfb9847d3bfd074249efb65d3fed15f5b0a6",
		BatchHash: "0xd71eb138d74c229a388eb0e1abc03f4c7cbb21d4fc4b839fbf0ec73e4263f6be",
		Contexts: []string{
			"0x68e4da79f805bca5b912bcda9c63d03e6e867108dabb9b944109aea541ef522a",
			"0x19b82093de5ce92a01e333048e877e2374354bf846dd034864ef6ffbd6438771",
		},
		PayloadRef: "Qmf412jQZiuVUtdgnB36FXFX7xg5V6KEbSJ4dpQuhkLyfD",
		NsOrAction: "firefly:contract_invoke_pin",
	}

	mcb := &blockchainmocks.Callbacks{}
	cb := NewBlockchainCallbacks()
	cb.SetHandler("ns1", mcb)

	mcb.On("BlockchainEventBatch", matchBatchPinEvent("ns1", core.TransactionTypeContractInvokePin)).Return(nil)

	sub := &SubscriptionInfo{
		Version:     2,
		V2Namespace: "ns1",
	}
	events := make(EventsToDispatch)
	cb.PrepareBatchPinOrNetworkAction(context.Background(), events, sub, fftypes.JSONAnyPtr("{}"), event, verifier, params)
	err := cb.DispatchBlockchainEvents(context.Background(), events)
	assert.NoError(t, err)

	mcb.AssertExpectations(t)
}

func TestCallbackBatchPin(t *testing.T) {
	event := &blockchain.Event{}
	verifier := &core.VerifierRef{}
	params := &BatchPinParams{
		UUIDs:     "0xe19af8b390604051812d7597d19adfb9847d3bfd074249efb65d3fed15f5b0a6",
		BatchHash: "0xd71eb138d74c229a388eb0e1abc03f4c7cbb21d4fc4b839fbf0ec73e4263f6be",
		Contexts: []string{
			"0x68e4da79f805bca5b912bcda9c63d03e6e867108dabb9b944109aea541ef522a",
			"0x19b82093de5ce92a01e333048e877e2374354bf846dd034864ef6ffbd6438771",
		},
		PayloadRef: "Qmf412jQZiuVUtdgnB36FXFX7xg5V6KEbSJ4dpQuhkLyfD",
	}

	mcb := &blockchainmocks.Callbacks{}
	cb := NewBlockchainCallbacks()
	cb.SetHandler("ns1", mcb)

	sub := &SubscriptionInfo{
		Version:     2,
		V2Namespace: "ns1",
	}
	mcb.On("BlockchainEventBatch", matchBatchPinEvent("ns1", core.TransactionTypeBatchPin)).Return(nil).Once()
	events := make(EventsToDispatch)
	cb.PrepareBatchPinOrNetworkAction(context.Background(), events, sub, fftypes.JSONAnyPtr("{}"), event, verifier, params)
	err := cb.DispatchBlockchainEvents(context.Background(), events)
	assert.NoError(t, err)

	mcb.On("BlockchainEventBatch", matchBatchPinEvent("ns1", core.TransactionTypeBatchPin)).Return(fmt.Errorf("pop")).Once()
	events = make(EventsToDispatch)
	cb.PrepareBatchPinOrNetworkAction(context.Background(), events, sub, fftypes.JSONAnyPtr("{}"), event, verifier, params)
	err = cb.DispatchBlockchainEvents(context.Background(), events)
	assert.EqualError(t, err, "pop")

	sub = &SubscriptionInfo{
		Version:     1,
		V1Namespace: map[string][]string{"ns2": {"ns1", "ns"}},
	}
	params.NsOrAction = "ns2"
	mcb.On("BlockchainEventBatch", matchBatchPinEvent("ns1", "" /* no tx type for V1 */)).Return(nil).Once()
	events = make(EventsToDispatch)
	cb.PrepareBatchPinOrNetworkAction(context.Background(), events, sub, fftypes.JSONAnyPtr("{}"), event, verifier, params)
	err = cb.DispatchBlockchainEvents(context.Background(), events)
	assert.NoError(t, err)

	params.NsOrAction = "ns3"
	events = make(EventsToDispatch)
	cb.PrepareBatchPinOrNetworkAction(context.Background(), events, sub, fftypes.JSONAnyPtr("{}"), event, verifier, params)
	err = cb.DispatchBlockchainEvents(context.Background(), events)
	assert.NoError(t, err)

	mcb.AssertExpectations(t)
}

func matchNetworkActionEvent(action string, verifier core.VerifierRef) interface{} {
	return mock.MatchedBy(func(batch []*blockchain.EventToDispatch) bool {
		return len(batch) == 1 &&
			batch[0].Type == blockchain.EventTypeNetworkAction &&
			batch[0].NetworkAction.Action == action &&
			*batch[0].NetworkAction.SigningKey == verifier
	})
}

func TestCallbackNetworkAction(t *testing.T) {
	event := &blockchain.Event{}
	verifier := core.VerifierRef{
		Value: "0x12345",
	}
	params := &BatchPinParams{
		NsOrAction: "firefly:terminate",
	}

	mcb := &blockchainmocks.Callbacks{}
	cb := NewBlockchainCallbacks()
	cb.SetHandler("ns1", mcb)

	sub := &SubscriptionInfo{
		Version:     2,
		V2Namespace: "ns1",
	}
	mcb.On("BlockchainEventBatch", matchNetworkActionEvent("terminate", verifier)).Return(nil).Once()
	events := make(EventsToDispatch)
	cb.PrepareBatchPinOrNetworkAction(context.Background(), events, sub, fftypes.JSONAnyPtr("{}"), event, &verifier, params)
	err := cb.DispatchBlockchainEvents(context.Background(), events)
	assert.NoError(t, err)

	mcb.On("BlockchainEventBatch", matchNetworkActionEvent("terminate", verifier)).Return(fmt.Errorf("pop")).Once()
	events = make(EventsToDispatch)
	cb.PrepareBatchPinOrNetworkAction(context.Background(), events, sub, fftypes.JSONAnyPtr("{}"), event, &verifier, params)
	err = cb.DispatchBlockchainEvents(context.Background(), events)
	assert.EqualError(t, err, "pop")

	sub = &SubscriptionInfo{
		Version:     1,
		V1Namespace: map[string][]string{"ns2": {"ns1", "ns"}},
	}
	mcb.On("BlockchainEventBatch", matchNetworkActionEvent("terminate", verifier)).Return(nil).Once()
	events = make(EventsToDispatch)
	cb.PrepareBatchPinOrNetworkAction(context.Background(), events, sub, fftypes.JSONAnyPtr("{}"), event, &verifier, params)
	err = cb.DispatchBlockchainEvents(context.Background(), events)
	assert.NoError(t, err)

	mcb.AssertExpectations(t)
}

func TestBuildBatchPinErrors(t *testing.T) {
	event := &blockchain.Event{}
	params := &BatchPinParams{}

	_, err := buildBatchPin(context.Background(), event, params)
	assert.Regexp(t, "missing data", err)

	params.BatchHash = "0xd71eb138d74c229a388eb0e1abc03f4c7cbb21d4fc4b839fbf0ec73e4263f6be"
	params.UUIDs = "BAD"
	_, err = buildBatchPin(context.Background(), event, params)
	assert.Regexp(t, "bad uuids", err)

	params.BatchHash = "BAD"
	params.UUIDs = "0xe19af8b390604051812d7597d19adfb9847d3bfd074249efb65d3fed15f5b0a6"
	_, err = buildBatchPin(context.Background(), event, params)
	assert.Regexp(t, "odd length hex string", err)

	params.BatchHash = "0xd71eb138d74c229a388eb0e1abc03f4c7cbb21d4fc4b839fbf0ec73e4263f6be"
	params.Contexts = append(params.Contexts, "BAD")
	_, err = buildBatchPin(context.Background(), event, params)
	assert.Regexp(t, "odd length hex string", err)
}

func TestGetNamespaceFromSubName(t *testing.T) {
	ns := GetNamespaceFromSubName("ff-sub-ns1-03071072-079b-4047-b192-a07186fc9db8")
	assert.Equal(t, "ns1", ns)

	ns = GetNamespaceFromSubName("ff-sub-03071072-079b-4047-b192-a07186fc9db8")
	assert.Equal(t, "", ns)

	ns = GetNamespaceFromSubName("ff-sub-ns1-123")
	assert.Equal(t, "", ns)

	ns = GetNamespaceFromSubName("BAD")
	assert.Equal(t, "", ns)
}

func TestSubscriptionsAddRemoveSubscription(t *testing.T) {
	ns := &core.Namespace{Name: "ns1", NetworkName: "ns1"}
	subs := NewFireflySubscriptions()

	subs.AddSubscription(context.Background(), ns, 2, "sub1", nil)
	assert.NotNil(t, subs.GetSubscription("sub1"))
	subs.RemoveSubscription(context.Background(), "sub1")
	assert.Nil(t, subs.GetSubscription("sub1"))
}

func TestSubscriptionsAddRemoveSubscriptionV1(t *testing.T) {
	ns := &core.Namespace{Name: "ns1", NetworkName: "ns1"}
	subs := NewFireflySubscriptions()

	subs.AddSubscription(context.Background(), ns, 1, "sub1", nil)
	assert.NotNil(t, subs.GetSubscription("sub1"))
	subs.RemoveSubscription(context.Background(), "sub1")
	assert.Nil(t, subs.GetSubscription("sub1"))
}

func TestSubscriptionsRemoveInvalid(t *testing.T) {
	subs := NewFireflySubscriptions()
	subs.RemoveSubscription(context.Background(), "sub1")
}

func TestGoodSuccessReceipt(t *testing.T) {
	var reply BlockchainReceiptNotification
	reply.Headers.ReceiptID = "ID"
	reply.Headers.ReplyType = "TransactionSuccess"
	reply.ProtocolID = "123456/098765453"

	mcb := &blockchainmocks.Callbacks{}
	cb := NewBlockchainCallbacks()
	cb.SetHandler("ns1", mcb)
	mcb.On("OperationUpdate", "ns1", mock.Anything).Return()

	err := HandleReceipt(context.Background(), "", nil, &reply, cb)
	assert.NoError(t, err)

	reply.Headers.ReplyType = "TransactionUpdate"
	err = HandleReceipt(context.Background(), "", nil, &reply, cb)
	assert.NoError(t, err)

	reply.Headers.ReplyType = "TransactionFailed"
	err = HandleReceipt(context.Background(), "", nil, &reply, cb)
	assert.NoError(t, err)
}

func TestReceiptMarshallingError(t *testing.T) {
	var reply BlockchainReceiptNotification
	reply.Headers.ReceiptID = "ID"
	reply.Headers.ReplyType = "force-marshall-error"
	reply.ProtocolID = "123456/098765453"

	mcb := &blockchainmocks.Callbacks{}
	cb := NewBlockchainCallbacks()
	cb.SetHandler("ns1", mcb)
	mcb.On("OperationUpdate", "ns1", mock.Anything).Return()

	err := HandleReceipt(context.Background(), "", nil, &reply, cb)
	assert.Error(t, err)
	assert.Regexp(t, ".*[^n]marshalling error.*", err)
}

type TestReceipt BlockchainReceiptNotification

func (receipt BlockchainReceiptNotification) MarshalJSON() ([]byte, error) {
	if receipt.Headers.ReplyType == "force-marshall-error" {
		return []byte(`null`), fmt.Errorf("json error")
	}
	return json.Marshal(TestReceipt(receipt))
}

func TestBadReceipt(t *testing.T) {
	var reply BlockchainReceiptNotification
	data := fftypes.JSONAnyPtr(`{}`)
	err := json.Unmarshal(data.Bytes(), &reply)
	assert.NoError(t, err)
	err = HandleReceipt(context.Background(), "", nil, &reply, nil)
	assert.Error(t, err)
}

func TestWrongNamespaceReceipt(t *testing.T) {
	var reply BlockchainReceiptNotification
	data := fftypes.JSONAnyPtr(`{}`)
	err := json.Unmarshal(data.Bytes(), &reply)
	assert.NoError(t, err)
	err = HandleReceipt(context.Background(), "wrong", nil, &reply, nil)
	assert.NoError(t, err)
}

type MockPlugin struct{}

func (m *MockPlugin) Name() string {
	return "Mock"
}

func TestGoodSuccessReceiptBatch(t *testing.T) {
	var plugin MockPlugin
	var reply BlockchainReceiptNotification
	reply.Headers.ReceiptID = "ID"
	reply.Headers.ReplyType = "TransactionSuccess"
	reply.ProtocolID = "123456/098765453"

	updates := []*core.OperationUpdate{}

	err := AddReceiptToBatch(context.Background(), "", &plugin, &reply, &updates)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(updates))

	reply.Headers.ReplyType = "TransactionUpdate"
	err = AddReceiptToBatch(context.Background(), "", &plugin, &reply, &updates)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(updates))

	reply.Headers.ReplyType = "TransactionFailed"
	err = AddReceiptToBatch(context.Background(), "", &plugin, &reply, &updates)
	assert.NoError(t, err)
	assert.Equal(t, 3, len(updates))
}

func TestReceiptMarshallingErrorBatch(t *testing.T) {
	var plugin MockPlugin
	var reply BlockchainReceiptNotification
	reply.Headers.ReceiptID = "ID"
	reply.Headers.ReplyType = "force-marshall-error"
	reply.ProtocolID = "123456/098765453"

	updates := []*core.OperationUpdate{}

	err := AddReceiptToBatch(context.Background(), "", &plugin, &reply, &updates)
	assert.Error(t, err)
	assert.Regexp(t, ".*[^n]marshalling error.*", err)
	assert.Equal(t, 0, len(updates))
}

func TestBadReceiptBatch(t *testing.T) {
	var plugin MockPlugin
	var reply BlockchainReceiptNotification
	data := fftypes.JSONAnyPtr(`{}`)
	err := json.Unmarshal(data.Bytes(), &reply)
	assert.NoError(t, err)

	updates := []*core.OperationUpdate{}

	err = AddReceiptToBatch(context.Background(), "", &plugin, &reply, &updates)
	assert.Error(t, err)
	assert.Equal(t, 0, len(updates))
}

func TestWrongNamespaceReceiptBatch(t *testing.T) {
	var plugin MockPlugin
	var reply BlockchainReceiptNotification
	data := fftypes.JSONAnyPtr(`{}`)
	err := json.Unmarshal(data.Bytes(), &reply)
	assert.NoError(t, err)
	updates := []*core.OperationUpdate{}

	err = AddReceiptToBatch(context.Background(), "wrong", &plugin, &reply, &updates)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(updates))
}

func TestErrorWrappingConflict(t *testing.T) {
	ctx := context.Background()
	res := &resty.Response{
		RawResponse: &http.Response{StatusCode: 409},
	}
	err := WrapRESTError(ctx, nil, res, fmt.Errorf("pop"), coremsgs.MsgEthConnectorRESTErr)
	assert.Regexp(t, "FF10458", err)
	assert.Regexp(t, "pop", err)

	conflictInterface, conforms := err.(operations.ConflictError)
	assert.True(t, conforms)
	assert.True(t, conflictInterface.IsConflictError())
}

func TestErrorWrappingConflictErrorInBody(t *testing.T) {
	ctx := context.Background()
	res := &resty.Response{
		RawResponse: &http.Response{StatusCode: 409},
	}
	err := WrapRESTError(ctx, &BlockchainRESTError{Error: "snap"}, res, fmt.Errorf("pop"), coremsgs.MsgEthConnectorRESTErr)
	assert.Regexp(t, "FF10458", err)
	assert.Regexp(t, "snap", err)

	conflictInterface, conforms := err.(operations.ConflictError)
	assert.True(t, conforms)
	assert.True(t, conflictInterface.IsConflictError())
}

func TestErrorWrappingError(t *testing.T) {
	ctx := context.Background()
	err := WrapRESTError(ctx, nil, nil, fmt.Errorf("pop"), coremsgs.MsgEthConnectorRESTErr)
	assert.Regexp(t, "pop", err)

	_, conforms := err.(operations.ConflictError)
	assert.False(t, conforms)
}

func TestErrorWrappingErrorRes(t *testing.T) {
	ctx := context.Background()

	err := WrapRESTError(ctx, &BlockchainRESTError{Error: "snap"}, nil, fmt.Errorf("pop"), coremsgs.MsgEthConnectorRESTErr)
	assert.Regexp(t, "snap", err)

	_, conforms := err.(operations.ConflictError)
	assert.False(t, conforms)
}

func TestErrorWrappingNonConflict(t *testing.T) {
	ctx := context.Background()
	res := &resty.Response{
		RawResponse: &http.Response{StatusCode: 500},
	}
	err := WrapRESTError(ctx, nil, res, fmt.Errorf("pop"), coremsgs.MsgEthConnectorRESTErr)
	assert.Regexp(t, "pop", err)

	_, conforms := err.(operations.ConflictError)
	assert.False(t, conforms)
}

func TestCallbackBulkOperationUpdate(t *testing.T) {
	nsOpID := "ns1:" + fftypes.NewUUID().String()
	nsOpID2 := "ns1:" + fftypes.NewUUID().String()

	mbi := &blockchainmocks.Plugin{}
	mcb := &coremocks.OperationCallbacks{}
	cb := NewBlockchainCallbacks()
	cb.SetOperationalHandler("ns1", mcb)

	mbi.On("Name").Return("utblockchain")
	mcb.On("BulkOperationUpdates", mock.Anything, mock.MatchedBy(func(updates []*core.OperationUpdate) bool {
		assert.True(t, updates[0].NamespacedOpID == nsOpID &&
			updates[0].Status == core.OpStatusSucceeded &&
			updates[0].BlockchainTXID == "tx1" &&
			updates[0].ErrorMessage == "err" &&
			updates[0].Plugin == "utblockchain")

		assert.True(t, updates[1].NamespacedOpID == nsOpID2 &&
			updates[1].Status == core.OpStatusSucceeded &&
			updates[1].BlockchainTXID == "tx2" &&
			updates[1].ErrorMessage == "err" &&
			updates[1].Plugin == "utblockchain")

		return true
	})).Return(nil).Once()

	cb.BulkOperationUpdates(context.Background(), "ns1", []*core.OperationUpdate{
		{
			NamespacedOpID: nsOpID,
			Status:         core.OpStatusSucceeded,
			BlockchainTXID: "tx1",
			ErrorMessage:   "err",
			Output:         fftypes.JSONObject{},
			Plugin:         "utblockchain",
		},
		{
			NamespacedOpID: nsOpID2,
			Status:         core.OpStatusSucceeded,
			BlockchainTXID: "tx2",
			ErrorMessage:   "err",
			Output:         fftypes.JSONObject{},
			Plugin:         "utblockchain",
		},
	})

	// No Handler
	cb.BulkOperationUpdates(context.Background(), "ns2", []*core.OperationUpdate{})

	mcb.AssertExpectations(t)
}
