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

package core

import "github.com/hyperledger-firefly/common/pkg/fftypes"

type TokenApprovalInput struct {
	TokenApproval
	Message        *MessageInOut  `ffstruct:"TokenApprovalInput" json:"message,omitempty"`
	Pool           string         `ffstruct:"TokenApprovalInput" json:"pool,omitempty" ffexcludeoutput:"true"`
	IdempotencyKey IdempotencyKey `ffstruct:"TokenApprovalInput" json:"idempotencyKey,omitempty" ffexcludeoutput:"true"`
}

type TokenApproval struct {
	LocalID         *fftypes.UUID      `ffstruct:"TokenApproval" json:"localId,omitempty" ffexcludeinput:"true"`
	Pool            *fftypes.UUID      `ffstruct:"TokenApproval" json:"pool,omitempty"`
	Connector       string             `ffstruct:"TokenApproval" json:"connector,omitempty" ffexcludeinput:"true"`
	Key             string             `ffstruct:"TokenApproval" json:"key,omitempty"`
	Operator        string             `ffstruct:"TokenApproval" json:"operator,omitempty"`
	Approved        bool               `ffstruct:"TokenApproval" json:"approved"`
	Info            fftypes.JSONObject `ffstruct:"TokenApproval" json:"info,omitempty" ffexcludeinput:"true"`
	Namespace       string             `ffstruct:"TokenApproval" json:"namespace,omitempty" ffexcludeinput:"true"`
	ProtocolID      string             `ffstruct:"TokenApproval" json:"protocolId,omitempty" ffexcludeinput:"true"`
	Subject         string             `ffstruct:"TokenApproval" json:"subject,omitempty" ffexcludeinput:"true"`
	Active          bool               `ffstruct:"TokenApproval" json:"active,omitempty" ffexcludeinput:"true"`
	Message         *fftypes.UUID      `ffstruct:"TokenApproval" json:"message,omitempty"`
	MessageHash     *fftypes.Bytes32   `ffstruct:"TokenApproval" json:"messageHash,omitempty" ffexcludeinput:"true"`
	Created         *fftypes.FFTime    `ffstruct:"TokenApproval" json:"created,omitempty" ffexcludeinput:"true"`
	TX              TransactionRef     `ffstruct:"TokenApproval" json:"tx" ffexcludeinput:"true"`
	BlockchainEvent *fftypes.UUID      `ffstruct:"TokenApproval" json:"blockchainEvent,omitempty" ffexcludeinput:"true"`
	Config          fftypes.JSONObject `ffstruct:"TokenApproval" json:"config,omitempty" ffexcludeoutput:"true"` // for REST calls only (not stored)
}
