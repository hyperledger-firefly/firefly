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

package metrics

import (
	"context"
	"sync"
	"time"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

var mutex = &sync.Mutex{}

var namespaceLabels = []string{"ns"}

type Manager interface {
	CountBatchPin(namespace string)
	MessageSubmitted(msg *core.Message)
	MessageConfirmed(msg *core.Message, eventType fftypes.FFEnum)
	TransferSubmitted(transfer *core.TokenTransfer)
	TransferConfirmed(transfer *core.TokenTransfer)
	BlockchainContractDeployment()
	BlockchainTransaction(location, methodName string)
	BlockchainQuery(location, methodName string)
	BlockchainEvent(location, signature string)
	NodeIdentityDXCertMismatch(namespace string, mismatch NodeIdentityDXCertMismatchStatus)
	NodeIdentityDXCertExpiry(namespace string, expiry time.Time)
	AddTime(id string)
	GetTime(id string) time.Time
	DeleteTime(id string)
	IsMetricsEnabled() bool
}

type metricsManager struct {
	ctx            context.Context
	metricsEnabled bool
	timeMap        map[string]time.Time
}

func NewMetricsManager(ctx context.Context) Manager {
	mm := &metricsManager{
		ctx:            ctx,
		metricsEnabled: config.GetBool(coreconfig.DeprecatedMetricsEnabled) || config.GetBool(coreconfig.MonitoringEnabled),
		timeMap:        make(map[string]time.Time),
	}

	return mm
}

func (mm *metricsManager) CountBatchPin(namespace string) {
	BatchPinCounter.WithLabelValues(namespace).Inc()
}

func (mm *metricsManager) MessageSubmitted(msg *core.Message) {
	if len(msg.Header.ID.String()) > 0 {
		switch msg.Header.Type {
		case core.MessageTypeBroadcast:
			BroadcastSubmittedCounter.WithLabelValues(msg.LocalNamespace).Inc()
		case core.MessageTypePrivate:
			PrivateMsgSubmittedCounter.WithLabelValues(msg.LocalNamespace).Inc()
		}
		mm.AddTime(msg.Header.ID.String())
	}
}

func (mm *metricsManager) MessageConfirmed(msg *core.Message, eventType fftypes.FFEnum) {
	eventTime := mm.GetTime(msg.Header.ID.String())
	timeElapsed := time.Since(eventTime).Seconds()
	mm.DeleteTime(msg.Header.ID.String())

	switch msg.Header.Type {
	case core.MessageTypeBroadcast:
		if !eventTime.IsZero() {
			// Check that we recorded the submission
			// as we might not be the party submitting
			BroadcastHistogram.WithLabelValues(msg.LocalNamespace).Observe(timeElapsed)
		}
		if eventType == core.EventTypeMessageConfirmed { // Broadcast Confirmed
			BroadcastConfirmedCounter.WithLabelValues(msg.LocalNamespace).Inc()
		} else if eventType == core.EventTypeMessageRejected { // Broadcast Rejected
			BroadcastRejectedCounter.WithLabelValues(msg.LocalNamespace).Inc()
		}
	case core.MessageTypePrivate:
		if !eventTime.IsZero() {
			// Check that we recorded the submission
			// as we might not be the party submitting
			PrivateMsgHistogram.WithLabelValues(msg.LocalNamespace).Observe(timeElapsed)
		}
		if eventType == core.EventTypeMessageConfirmed { // Private Msg Confirmed
			PrivateMsgConfirmedCounter.WithLabelValues(msg.LocalNamespace).Inc()
		} else if eventType == core.EventTypeMessageRejected { // Private Msg Rejected
			PrivateMsgRejectedCounter.WithLabelValues(msg.LocalNamespace).Inc()
		}
	}
}

func (mm *metricsManager) TransferSubmitted(transfer *core.TokenTransfer) {
	if len(transfer.LocalID.String()) > 0 {
		switch transfer.Type {
		case core.TokenTransferTypeMint: // Mint submitted
			MintSubmittedCounter.WithLabelValues(transfer.Namespace).Inc()
		case core.TokenTransferTypeTransfer: // Transfer submitted
			TransferSubmittedCounter.WithLabelValues(transfer.Namespace).Inc()
		case core.TokenTransferTypeBurn: // Burn submitted
			BurnSubmittedCounter.WithLabelValues(transfer.Namespace).Inc()
		}
		mm.AddTime(transfer.LocalID.String())
	}
}

func (mm *metricsManager) TransferConfirmed(transfer *core.TokenTransfer) {
	transferEvent := mm.GetTime(transfer.LocalID.String())
	timeElapsed := time.Since(transferEvent).Seconds()
	mm.DeleteTime(transfer.LocalID.String())

	switch transfer.Type {
	case core.TokenTransferTypeMint: // Mint confirmed
		if !transferEvent.IsZero() {
			MintHistogram.WithLabelValues(transfer.Namespace).Observe(timeElapsed)
		}
		MintConfirmedCounter.WithLabelValues(transfer.Namespace).Inc()
	case core.TokenTransferTypeTransfer: // Transfer confirmed
		if !transferEvent.IsZero() {
			TransferHistogram.WithLabelValues(transfer.Namespace).Observe(timeElapsed)
		}
		TransferConfirmedCounter.WithLabelValues(transfer.Namespace).Inc()
	case core.TokenTransferTypeBurn: // Burn confirmed
		if !transferEvent.IsZero() {
			BurnHistogram.WithLabelValues(transfer.Namespace).Observe(timeElapsed)
		}
		BurnConfirmedCounter.WithLabelValues(transfer.Namespace).Inc()
	}
}

func (mm *metricsManager) BlockchainContractDeployment() {
	BlockchainTransactionsCounter.WithLabelValues("", "").Inc()
}

func (mm *metricsManager) BlockchainTransaction(location, methodName string) {
	BlockchainTransactionsCounter.WithLabelValues(location, methodName).Inc()
}

func (mm *metricsManager) BlockchainQuery(location, methodName string) {
	BlockchainQueriesCounter.WithLabelValues(location, methodName).Inc()
}

func (mm *metricsManager) BlockchainEvent(location, signature string) {
	BlockchainEventsCounter.WithLabelValues(location, signature).Inc()
}

func (mm *metricsManager) AddTime(id string) {
	mutex.Lock()
	mm.timeMap[id] = time.Now()
	mutex.Unlock()
}

func (mm *metricsManager) GetTime(id string) time.Time {
	mutex.Lock()
	time := mm.timeMap[id]
	mutex.Unlock()
	return time
}

func (mm *metricsManager) DeleteTime(id string) {
	mutex.Lock()
	delete(mm.timeMap, id)
	mutex.Unlock()
}

func (mm *metricsManager) IsMetricsEnabled() bool {
	return mm.metricsEnabled
}
