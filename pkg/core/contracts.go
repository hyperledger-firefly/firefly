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

	"github.com/hyperledger-firefly/common/pkg/fftypes"
)

type ContractCallType = fftypes.FFEnum

var (
	// CallTypeInvoke is an invocation that submits a transaction for inclusion in the chain
	CallTypeInvoke = fftypes.FFEnumValue("contractcalltype", "invoke")
	// CallTypeQuery is a query that returns data from the chain
	CallTypeQuery = fftypes.FFEnumValue("contractcalltype", "query")
)

type ContractCallRequest struct {
	Type           ContractCallType       `ffstruct:"ContractCallRequest" json:"type,omitempty" ffenum:"contractcalltype" ffexcludeinput:"true"`
	Interface      *fftypes.UUID          `ffstruct:"ContractCallRequest" json:"interface,omitempty" ffexcludeinput:"postContractAPIInvoke,postContractAPIQuery"`
	Location       *fftypes.JSONAny       `ffstruct:"ContractCallRequest" json:"location,omitempty"`
	Key            string                 `ffstruct:"ContractCallRequest" json:"key,omitempty"`
	Method         *fftypes.FFIMethod     `ffstruct:"ContractCallRequest" json:"method,omitempty" ffexcludeinput:"postContractAPIInvoke,postContractAPIQuery"`
	MethodPath     string                 `ffstruct:"ContractCallRequest" json:"methodPath,omitempty" ffexcludeinput:"postContractAPIInvoke,postContractAPIQuery"`
	Input          map[string]interface{} `ffstruct:"ContractCallRequest" json:"input"`
	Errors         []*fftypes.FFIError    `ffstruct:"ContractCallRequest" json:"errors,omitempty" ffexcludeinput:"postContractAPIInvoke,postContractAPIQuery"`
	Options        map[string]interface{} `ffstruct:"ContractCallRequest" json:"options"`
	Message        *MessageInOut          `ffstruct:"ContractCallRequest" json:"message,omitempty" ffexcludeinput:"postContractQuery,postContractAPIQuery"`
	IdempotencyKey IdempotencyKey         `ffstruct:"ContractCallRequest" json:"idempotencyKey,omitempty" ffexcludeoutput:"true"`
}

type ContractDeployRequest struct {
	Key            string                 `ffstruct:"ContractDeployRequest" json:"key,omitempty"`
	Input          []interface{}          `ffstruct:"ContractDeployRequest" json:"input"`
	Definition     *fftypes.JSONAny       `ffstruct:"ContractDeployRequest" json:"definition"`
	Contract       *fftypes.JSONAny       `ffstruct:"ContractDeployRequest" json:"contract"`
	Options        map[string]interface{} `ffstruct:"ContractDeployRequest" json:"options"`
	IdempotencyKey IdempotencyKey         `ffstruct:"ContractDeployRequest" json:"idempotencyKey,omitempty" ffexcludeoutput:"true"`
}

type ContractURLs struct {
	API     string `ffstruct:"ContractURLs" json:"api"`
	OpenAPI string `ffstruct:"ContractURLs" json:"openapi"`
	UI      string `ffstruct:"ContractURLs" json:"ui"`
}

type ContractAPI struct {
	ID          *fftypes.UUID         `ffstruct:"ContractAPI" json:"id,omitempty" ffexcludeinput:"true"`
	Namespace   string                `ffstruct:"ContractAPI" json:"namespace,omitempty" ffexcludeinput:"true"`
	Interface   *fftypes.FFIReference `ffstruct:"ContractAPI" json:"interface"`
	Location    *fftypes.JSONAny      `ffstruct:"ContractAPI" json:"location,omitempty"`
	Name        string                `ffstruct:"ContractAPI" json:"name"`
	NetworkName string                `ffstruct:"ContractAPI" json:"networkName,omitempty"`
	Message     *fftypes.UUID         `ffstruct:"ContractAPI" json:"message,omitempty" ffexcludeinput:"true"`
	URLs        ContractURLs          `ffstruct:"ContractAPI" json:"urls" ffexcludeinput:"true"`
	Published   bool                  `ffstruct:"ContractAPI" json:"published" ffexcludeinput:"true"`
}

type ContractAPIInput struct {
	ContractAPI
	Topics fftypes.FFStringArray `ffstruct:"ContractAPIInput" json:"topics,omitempty" ffexcludeoutput:"true"`
}

type FFIInput struct {
	fftypes.FFI
	Topics fftypes.FFStringArray `ffstruct:"FFIInput" json:"topics,omitempty" ffexcludeoutput:"true"`
}

func (c *ContractAPI) Validate(ctx context.Context) (err error) {
	if err = fftypes.ValidateFFNameField(ctx, c.Namespace, "namespace"); err != nil {
		return err
	}
	if err = fftypes.ValidateFFNameField(ctx, c.Name, "name"); err != nil {
		return err
	}
	if c.NetworkName != "" {
		if err = fftypes.ValidateFFNameField(ctx, c.NetworkName, "networkName"); err != nil {
			return err
		}
	}
	return nil
}

func (c *ContractAPI) Topic() string {
	return fftypes.TypeNamespaceNameTopicHash("contractapi", c.Namespace, c.NetworkName)
}

func (c *ContractAPI) SetBroadcastMessage(msgID *fftypes.UUID) {
	c.Message = msgID
}

func (c *ContractAPI) LocationAndLedgerEquals(a *ContractAPI) bool {
	if c == nil || a == nil {
		return false
	}
	return c.Location.Hash().Equals(a.Location.Hash())
}
