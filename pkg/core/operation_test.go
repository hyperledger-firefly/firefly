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

package core

import (
	"context"
	"reflect"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/stretchr/testify/assert"
)

type fakePlugin struct{}

func (f *fakePlugin) Name() string { return "fake" }

func TestNewInitializedMessageOp(t *testing.T) {

	txID := fftypes.NewUUID()
	op := NewOperation(&fakePlugin{}, "ns1", txID, OpTypeSharedStorageUploadBatch)
	assert.Equal(t, Operation{
		ID:          op.ID,
		Namespace:   "ns1",
		Transaction: txID,
		Plugin:      "fake",
		Type:        OpTypeSharedStorageUploadBatch,
		Status:      OpStatusInitialized,
		Created:     op.Created,
		Updated:     op.Created,
	}, *op)
}

func TestOperationTypes(t *testing.T) {

	txID := fftypes.NewUUID()
	op := NewOperation(&fakePlugin{}, "ns1", txID, OpTypeSharedStorageUploadBatch)
	assert.Equal(t, Operation{
		ID:          op.ID,
		Namespace:   "ns1",
		Transaction: txID,
		Plugin:      "fake",
		Type:        OpTypeSharedStorageUploadBatch,
		Status:      OpStatusInitialized,
		Created:     op.Created,
		Updated:     op.Created,
	}, *op)

	assert.False(t, op.IsBlockchainOperation())
	assert.False(t, op.IsTokenOperation())

	// Blockchain operation types
	op.Type = OpTypeBlockchainContractDeploy
	assert.True(t, op.IsBlockchainOperation())
	assert.False(t, op.IsTokenOperation())

	op.Type = OpTypeBlockchainInvoke
	assert.True(t, op.IsBlockchainOperation())
	assert.False(t, op.IsTokenOperation())

	op.Type = OpTypeBlockchainNetworkAction
	assert.True(t, op.IsBlockchainOperation())
	assert.False(t, op.IsTokenOperation())

	op.Type = OpTypeBlockchainPinBatch
	assert.True(t, op.IsBlockchainOperation())
	assert.False(t, op.IsTokenOperation())

	// Token operation types
	op.Type = OpTypeTokenActivatePool
	assert.True(t, op.IsTokenOperation())
	assert.False(t, op.IsBlockchainOperation())

	op.Type = OpTypeTokenApproval
	assert.True(t, op.IsTokenOperation())
	assert.False(t, op.IsBlockchainOperation())

	op.Type = OpTypeTokenCreatePool
	assert.True(t, op.IsTokenOperation())
	assert.False(t, op.IsBlockchainOperation())

	op.Type = OpTypeTokenTransfer
	assert.True(t, op.IsTokenOperation())
	assert.False(t, op.IsBlockchainOperation())
}

func TestOperationDeepCopy(t *testing.T) {
	op := &Operation{
		ID:          fftypes.NewUUID(),
		Namespace:   "ns1",
		Transaction: fftypes.NewUUID(),
		Type:        OpTypeBlockchainInvoke,
		Status:      OpStatusInitialized,
		Plugin:      "fake",
		Input:       fftypes.JSONObject{"key": "value"},
		Output:      fftypes.JSONObject{"result": "success"},
		Error:       "error message",
		Created:     fftypes.Now(),
		Updated:     fftypes.Now(),
		Retry:       fftypes.NewUUID(),
	}

	copyOp := op.DeepCopy()
	shallowCopy := op // Shallow copy for showcasing that DeepCopy is a deep copy

	// Ensure the data was copied correctly
	assert.Equal(t, op.ID, copyOp.ID)
	assert.Equal(t, op.Namespace, copyOp.Namespace)
	assert.Equal(t, op.Transaction, copyOp.Transaction)
	assert.Equal(t, op.Type, copyOp.Type)
	assert.Equal(t, op.Status, copyOp.Status)
	assert.Equal(t, op.Plugin, copyOp.Plugin)
	assert.Equal(t, op.Input, copyOp.Input)
	assert.Equal(t, op.Output, copyOp.Output)
	assert.Equal(t, op.Error, copyOp.Error)
	assert.Equal(t, op.Created, copyOp.Created)
	assert.Equal(t, op.Updated, copyOp.Updated)
	assert.Equal(t, op.Retry, copyOp.Retry)

	// Modify the original and ensure the copy is not modified
	*op.ID = *fftypes.NewUUID()
	assert.NotEqual(t, copyOp.ID, op.ID)

	*op.Created = *fftypes.Now()
	assert.NotEqual(t, copyOp.Created, op.Created)

	// Ensure the copy is a deep copy by comparing the pointers of the fields
	assert.NotSame(t, copyOp.ID, op.ID)
	assert.NotSame(t, copyOp.Created, op.Created)
	assert.NotSame(t, copyOp.Updated, op.Updated)
	assert.NotSame(t, copyOp.Transaction, op.Transaction)
	assert.NotSame(t, copyOp.Retry, op.Retry)
	assert.NotSame(t, copyOp.Input, op.Input)
	assert.NotSame(t, copyOp.Output, op.Output)

	// showcasing that the shallow copy is a shallow copy and the copied object value changed as well the pointer has the same address as the original
	assert.Equal(t, shallowCopy.ID, op.ID)
	assert.Same(t, shallowCopy.ID, op.ID)

	// Ensure no new fields are added to the Operation struct
	// If a new field is added, this test will fail and the DeepCopy function should be updated
	assert.Equal(t, 12, reflect.TypeOf(Operation{}).NumField())
}
func TestParseNamespacedOpID(t *testing.T) {

	ctx := context.Background()
	u := fftypes.NewUUID()

	_, _, err := ParseNamespacedOpID(ctx, "")
	assert.Regexp(t, "FF10411", err)

	_, _, err = ParseNamespacedOpID(ctx, "a::"+u.String())
	assert.Regexp(t, "FF10411", err)

	_, _, err = ParseNamespacedOpID(ctx, "bad%namespace:"+u.String())
	assert.Regexp(t, "FF00140", err)

	_, _, err = ParseNamespacedOpID(ctx, "ns1:Bad UUID")
	assert.Regexp(t, "FF00138", err)

	po := &PreparedOperation{
		ID:        u,
		Namespace: "ns1",
	}
	ns, u1, err := ParseNamespacedOpID(ctx, po.NamespacedIDString())
	assert.NoError(t, err)
	assert.Equal(t, u, u1)
	assert.Equal(t, "ns1", ns)

}

func TestDeepCopyMapNil(t *testing.T) {
	original := map[string]interface{}(nil)
	copy := deepCopyMap(original)
	assert.Nil(t, copy)
}

func TestDeepCopyMapEmpty(t *testing.T) {
	original := map[string]interface{}{}
	copy := deepCopyMap(original)
	assert.NotNil(t, copy)
	assert.Empty(t, copy)
}

func TestDeepCopyMapSimple(t *testing.T) {
	original := map[string]interface{}{
		"key1": "value1",
		"key2": 42,
	}
	copy := deepCopyMap(original)
	assert.Equal(t, original, copy)
}

func TestDeepCopyMapNestedMap(t *testing.T) {
	original := map[string]interface{}{
		"key1": map[string]interface{}{
			"nestedKey1": "nestedValue1",
		},
	}
	copy := deepCopyMap(original)
	assert.Equal(t, original, copy)
	assert.NotSame(t, original["key1"], copy["key1"])
}

func TestDeepCopyMapNestedSlice(t *testing.T) {
	original := map[string]interface{}{
		"key1": []interface{}{"value1", 42},
	}
	copy := deepCopyMap(original)
	assert.Equal(t, original, copy)
	assert.NotSame(t, original["key1"], copy["key1"])
}

func TestDeepCopyMapMixed(t *testing.T) {
	original := map[string]interface{}{
		"key1": "value1",
		"key2": map[string]interface{}{
			"nestedKey1": "nestedValue1",
		},
		"key3": []interface{}{"value1", 42},
	}
	copy := deepCopyMap(original)
	assert.Equal(t, original, copy)
	assert.NotSame(t, original["key2"], copy["key2"])
	assert.NotSame(t, original["key3"], copy["key3"])
}

func TestDeepCopySliceNil(t *testing.T) {
	original := []interface{}(nil)
	copy := deepCopySlice(original)
	assert.Nil(t, copy)
}

func TestDeepCopySliceEmpty(t *testing.T) {
	original := []interface{}{}
	copy := deepCopySlice(original)
	assert.NotNil(t, copy)
	assert.Empty(t, copy)
}

func TestDeepCopySliceSimple(t *testing.T) {
	original := []interface{}{"value1", 42}
	copy := deepCopySlice(original)
	assert.Equal(t, original, copy)
}

func TestDeepCopySliceNestedMap(t *testing.T) {
	original := []interface{}{
		map[string]interface{}{
			"nestedKey1": "nestedValue1",
		},
	}
	copy := deepCopySlice(original)
	assert.Equal(t, original, copy)
	assert.NotSame(t, original[0], copy[0])
}

func TestDeepCopySliceNestedSlice(t *testing.T) {
	original := []interface{}{
		[]interface{}{"value1", 42},
	}
	copy := deepCopySlice(original)
	assert.Equal(t, original, copy)
	assert.NotSame(t, original[0], copy[0])
}

func TestDeepCopySliceMixed(t *testing.T) {
	original := []interface{}{
		"value1",
		42,
		map[string]interface{}{
			"nestedKey1": "nestedValue1",
		},
		[]interface{}{"value2", 43},
	}
	copy := deepCopySlice(original)
	assert.Equal(t, original, copy)
	assert.NotSame(t, original[2], copy[2])
	assert.NotSame(t, original[3], copy[3])
}
