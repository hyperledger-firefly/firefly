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
	"crypto/tls"
	"database/sql/driver"
	"encoding/json"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
)

// Namespace is an isolated set of named resources, to allow multiple applications to co-exist in the same network, with the same named objects.
// Can be used for use case segregation, or multi-tenancy.
type Namespace struct {
	Name        string                 `ffstruct:"Namespace" json:"name"`
	NetworkName string                 `ffstruct:"Namespace" json:"networkName"`
	Description string                 `ffstruct:"Namespace" json:"description"`
	Created     *fftypes.FFTime        `ffstruct:"Namespace" json:"created" ffexcludeinput:"true"`
	Contracts   *MultipartyContracts   `ffstruct:"Namespace" json:"-"`
	TLSConfigs  map[string]*tls.Config `ffstruct:"Namespace" json:"-" ffexcludeinput:"true"`
}

type NamespaceWithInitStatus struct {
	*Namespace
	Initializing        bool   `ffstruct:"NamespaceWithInitStatus" json:"initializing,omitempty"`
	InitializationError string `ffstruct:"NamespaceWithInitStatus" json:"initializationError,omitempty"`
}

// MultipartyContracts represent the currently active and any terminated FireFly multiparty contract(s)
type MultipartyContracts struct {
	Active     *MultipartyContract   `ffstruct:"MultipartyContracts" json:"active"`
	Terminated []*MultipartyContract `ffstruct:"MultipartyContracts" json:"terminated,omitempty"`
}
type MultipartyContractsWithActiveStatus struct {
	Active     *MultipartyContractWithStatus `ffstruct:"MultipartyContracts" json:"active"`
	Terminated []*MultipartyContract         `ffstruct:"MultipartyContracts" json:"terminated,omitempty"`
}

// MultipartyContract represents identifying details about a FireFly multiparty contract, as read from the config file
type MultipartyContract struct {
	Index      int                    `ffstruct:"MultipartyContract" json:"index"`
	Location   *fftypes.JSONAny       `ffstruct:"MultipartyContract" json:"location,omitempty"`
	FirstEvent string                 `ffstruct:"MultipartyContract" json:"firstEvent,omitempty"`
	Info       MultipartyContractInfo `ffstruct:"MultipartyContract" json:"info"`
}

type ContractListenerStatus = fftypes.FFEnum

var (
	// contract is in catch-up mode
	ContractListenerStatusSyncing = fftypes.FFEnumValue("contractlistenerstatus", "syncing")
	// contract is within tolerance of the latest block
	ContractListenerStatusSynced = fftypes.FFEnumValue("contractlistenerstatus", "synced")
	// status couldn't be determined
	ContractListenerStatusUnknown = fftypes.FFEnumValue("contractlistenerstatus", "unknown")
)

type MultipartyContractWithStatus struct {
	MultipartyContract
	Status ContractListenerStatus `ffstruct:"MultipartyContract" json:"status"`
}

// MultipartyContractInfo stores additional info about the FireFly multiparty contract that is computed during node operation
type MultipartyContractInfo struct {
	Subscription string `ffstruct:"MultipartyContract" json:"subscription,omitempty"`
	FinalEvent   string `ffstruct:"MultipartyContract" json:"finalEvent,omitempty"`
	Version      int    `ffstruct:"MultipartyContract" json:"version,omitempty"`
}

// NetworkActionType is a type of action to perform
type NetworkActionType = fftypes.FFEnum

var (
	// NetworkActionTerminate request all network members to stop using the current contract and move to the next one configured
	NetworkActionTerminate = fftypes.FFEnumValue("networkactiontype", "terminate")
)

type NetworkAction struct {
	Type NetworkActionType `ffstruct:"NetworkAction" json:"type" ffenum:"networkactiontype"`
}

// Scan implements sql.Scanner
func (fc *MultipartyContracts) Scan(src interface{}) error {
	switch src := src.(type) {
	case []byte:
		if len(src) == 0 {
			return nil
		}
		return json.Unmarshal(src, fc)
	case string:
		return fc.Scan([]byte(src))
	default:
		return i18n.NewError(context.Background(), i18n.MsgTypeRestoreFailed, src, fc)
	}
}

// Value implements sql.Valuer
func (fc MultipartyContracts) Value() (driver.Value, error) {
	return json.Marshal(fc)
}
