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

package core

import (
	"context"
	"strings"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
)

// OpType describes mechanical steps in the process that have to be performed,
// might be asynchronous, and have results in the back-end systems that might need
// to be correlated with messages by operators.
type OpType = fftypes.FFEnum

var (
	// OpTypeBlockchainPinBatch is a blockchain transaction to pin a batch
	OpTypeBlockchainPinBatch = fftypes.FFEnumValue("optype", "blockchain_pin_batch")
	// OpTypeBlockchainNetworkAction is an administrative action on a multiparty blockchain network
	OpTypeBlockchainNetworkAction = fftypes.FFEnumValue("optype", "blockchain_network_action")
	// OpTypeBlockchainContractDeploy is a smart contract deploy
	OpTypeBlockchainContractDeploy = fftypes.FFEnumValue("optype", "blockchain_deploy")
	// OpTypeBlockchainInvoke is a smart contract invoke
	OpTypeBlockchainInvoke = fftypes.FFEnumValue("optype", "blockchain_invoke")
	// OpTypeSharedStorageUploadBatch is a shared storage operation to upload broadcast data
	OpTypeSharedStorageUploadBatch = fftypes.FFEnumValue("optype", "sharedstorage_upload_batch")
	// OpTypeSharedStorageUploadBlob is a shared storage operation to upload blob data
	OpTypeSharedStorageUploadBlob = fftypes.FFEnumValue("optype", "sharedstorage_upload_blob")
	// OpTypeSharedStorageUploadValue is a shared storage operation to upload the value JSON from a data record directly
	OpTypeSharedStorageUploadValue = fftypes.FFEnumValue("optype", "sharedstorage_upload_value")
	// OpTypeSharedStorageDownloadBatch is a shared storage operation to download broadcast data
	OpTypeSharedStorageDownloadBatch = fftypes.FFEnumValue("optype", "sharedstorage_download_batch")
	// OpTypeSharedStorageDownloadBlob is a shared storage operation to download broadcast data
	OpTypeSharedStorageDownloadBlob = fftypes.FFEnumValue("optype", "sharedstorage_download_blob")
	// OpTypeDataExchangeSendBatch is a private send of a batch
	OpTypeDataExchangeSendBatch = fftypes.FFEnumValue("optype", "dataexchange_send_batch")
	// OpTypeDataExchangeSendBlob is a private send of a blob
	OpTypeDataExchangeSendBlob = fftypes.FFEnumValue("optype", "dataexchange_send_blob")
	// OpTypeTokenCreatePool is a token pool creation
	OpTypeTokenCreatePool = fftypes.FFEnumValue("optype", "token_create_pool")
	// OpTypeTokenActivatePool is a token pool activation
	OpTypeTokenActivatePool = fftypes.FFEnumValue("optype", "token_activate_pool")
	// OpTypeTokenTransfer is a token transfer
	OpTypeTokenTransfer = fftypes.FFEnumValue("optype", "token_transfer")
	// OpTypeTokenApproval is a token approval
	OpTypeTokenApproval = fftypes.FFEnumValue("optype", "token_approval")
)

func (op *Operation) IsBlockchainOperation() bool {
	return op.Type == OpTypeBlockchainInvoke ||
		op.Type == OpTypeBlockchainNetworkAction ||
		op.Type == OpTypeBlockchainPinBatch ||
		op.Type == OpTypeBlockchainContractDeploy
}

func (op *Operation) IsTokenOperation() bool {
	return op.Type == OpTypeTokenActivatePool || op.Type == OpTypeTokenApproval || op.Type == OpTypeTokenCreatePool || op.Type == OpTypeTokenTransfer
}

func (op *Operation) DeepCopy() *Operation {
	cop := &Operation{
		Namespace: op.Namespace,
		Type:      op.Type,
		Status:    op.Status,
		Plugin:    op.Plugin,
		Error:     op.Error,
	}
	if op.ID != nil {
		idCopy := *op.ID
		cop.ID = &idCopy
	}
	if op.Transaction != nil {
		txCopy := *op.Transaction
		cop.Transaction = &txCopy
	}
	if op.Created != nil {
		createdCopy := *op.Created
		cop.Created = &createdCopy
	}
	if op.Updated != nil {
		updatedCopy := *op.Updated
		cop.Updated = &updatedCopy
	}
	if op.Retry != nil {
		retryCopy := *op.Retry
		cop.Retry = &retryCopy
	}
	if op.Input != nil {
		cop.Input = deepCopyMap(op.Input)
	}
	if op.Output != nil {
		cop.Output = deepCopyMap(op.Output)
	}
	return cop
}

func deepCopyMap(original map[string]interface{}) map[string]interface{} {
	if original == nil {
		return nil
	}
	copy := make(map[string]interface{}, len(original))
	for key, value := range original {
		switch v := value.(type) {
		case map[string]interface{}:
			copy[key] = deepCopyMap(v)
		case []interface{}:
			copy[key] = deepCopySlice(v)
		default:
			copy[key] = v
		}
	}
	return copy
}

func deepCopySlice(original []interface{}) []interface{} {
	if original == nil {
		return nil
	}
	copy := make([]interface{}, len(original))
	for i, value := range original {
		switch v := value.(type) {
		case map[string]interface{}:
			copy[i] = deepCopyMap(v)
		case []interface{}:
			copy[i] = deepCopySlice(v)
		default:
			copy[i] = v
		}
	}
	return copy
}

// OpStatus is the current status of an operation
type OpStatus string

const (
	// OpStatusInitialized indicates the operation has been initialized, but successful confirmation of submission to the
	// relevant plugin has not yet been received
	OpStatusInitialized OpStatus = "Initialized"
	// OpStatusPending indicates the operation has been submitted, but is not yet confirmed as successful or failed
	OpStatusPending OpStatus = "Pending"
	// OpStatusSucceeded the infrastructure runtime has returned success for the operation
	OpStatusSucceeded OpStatus = "Succeeded"
	// OpStatusFailed happens when an error is reported by the infrastructure runtime
	OpStatusFailed OpStatus = "Failed"
)

type Named interface {
	Name() string
}

// NewOperation creates a new operation in a transaction. It always starts in "Initialized" state
// and only moves to "Pending" when successful submission to the respective plugin has been confirmed.
func NewOperation(plugin Named, namespace string, tx *fftypes.UUID, opType OpType) *Operation {
	now := fftypes.Now()
	return &Operation{
		ID:          fftypes.NewUUID(),
		Namespace:   namespace,
		Plugin:      plugin.Name(),
		Transaction: tx,
		Type:        opType,
		Status:      OpStatusInitialized,
		Created:     now,
		Updated:     now,
	}
}

// Operation is a description of an action performed as part of a transaction submitted by this node
type Operation struct {
	ID          *fftypes.UUID      `ffstruct:"Operation" json:"id" ffexcludeinput:"true"`
	Namespace   string             `ffstruct:"Operation" json:"namespace" ffexcludeinput:"true"`
	Transaction *fftypes.UUID      `ffstruct:"Operation" json:"tx" ffexcludeinput:"true"`
	Type        OpType             `ffstruct:"Operation" json:"type" ffenum:"optype" ffexcludeinput:"true"`
	Status      OpStatus           `ffstruct:"Operation" json:"status"`
	Plugin      string             `ffstruct:"Operation" json:"plugin" ffexcludeinput:"true"`
	Input       fftypes.JSONObject `ffstruct:"Operation" json:"input,omitempty" ffexcludeinput:"true"`
	Output      fftypes.JSONObject `ffstruct:"Operation" json:"output,omitempty"`
	Error       string             `ffstruct:"Operation" json:"error,omitempty"`
	Created     *fftypes.FFTime    `ffstruct:"Operation" json:"created,omitempty" ffexcludeinput:"true"`
	Updated     *fftypes.FFTime    `ffstruct:"Operation" json:"updated,omitempty" ffexcludeinput:"true"`
	Retry       *fftypes.UUID      `ffstruct:"Operation" json:"retry,omitempty" ffexcludeinput:"true"`
}

// OperationUpdateDTO is the subset of fields on an operation that are mutable, via the SPI
type OperationUpdateDTO struct {
	Status OpStatus           `ffstruct:"Operation" json:"status"`
	Output fftypes.JSONObject `ffstruct:"Operation" json:"output,omitempty"`
	Error  *string            `ffstruct:"Operation" json:"error,omitempty"`
}

// PreparedOperation is an operation that has gathered all the raw data ready to send to a plugin
// It is never stored, but it should always be possible for the owning Manager to generate a
// PreparedOperation from an Operation. Data is defined by the Manager, but should be JSON-serializable
// to support inspection and debugging.
type PreparedOperation struct {
	ID        *fftypes.UUID `json:"id"`
	Namespace string        `json:"namespace"`
	Plugin    string        `json:"plugin"`
	Type      OpType        `json:"type" ffenum:"optype"`
	Data      interface{}   `json:"data"`
}

type OpPhase int

const (
	OpPhaseComplete OpPhase = iota
	OpPhasePending
	OpPhaseInitializing
)

func (po *PreparedOperation) NamespacedIDString() string {
	return po.Namespace + ":" + po.ID.String()
}

func ParseNamespacedOpID(ctx context.Context, nsIDStr string) (string, *fftypes.UUID, error) {
	nsIDSplit := strings.Split(nsIDStr, ":")
	if len(nsIDSplit) != 2 {
		return "", nil, i18n.NewError(ctx, coremsgs.MsgInvalidNamespaceUUID, nsIDStr)
	}
	ns := nsIDSplit[0]
	uuidStr := nsIDSplit[1]
	if err := fftypes.ValidateFFNameField(ctx, ns, "namespace"); err != nil {
		return "", nil, err
	}
	u, err := fftypes.ParseUUID(ctx, uuidStr)
	return ns, u, err
}

type OperationCallbacks interface {
	// This is an asynchronous update and you can use onComplete to signal the update has been committed to the DB through a function call
	OperationUpdate(update *OperationUpdateAsync)
	BulkOperationUpdates(ctx context.Context, updates []*OperationUpdate) error
}

// OperationUpdate notifies FireFly of an update to an operation.
// Only success/failure and errorMessage (for errors) are modeled.
// Output can be used to add opaque protocol-specific JSON from the plugin (protocol transaction ID etc.)
// Note this is an optional hook information, and stored separately to the confirmation of the actual event that was being submitted/sequenced.
// Only the party submitting the transaction will see this data.
type OperationUpdate struct {
	Plugin         string
	NamespacedOpID string
	Status         OpStatus
	BlockchainTXID string
	ErrorMessage   string
	Output         fftypes.JSONObject
	VerifyManifest bool
	DXManifest     string
	DXHash         string
}

type OperationUpdateAsync struct {
	OperationUpdate
	OnComplete func()
}

type OperationDetailError struct {
	StatusError string `ffstruct:"OperationDetail" json:"error,omitempty"`
}

type OperationWithDetail struct {
	Operation
	Detail interface{} `ffstruct:"OperationWithDetail" json:"detail,omitempty" ffexcludeinput:"true"`
}
