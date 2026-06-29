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
	"testing"
	"time"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
	"github.com/stretchr/testify/assert"
)

var msgID = fftypes.NewUUID()
var Message = &core.Message{
	Header: core.MessageHeader{
		ID: msgID,
		SignerRef: core.SignerRef{
			Author: "did:firefly:org/abcd",
			Key:    "0x12345",
		},
		Type: "",
	},
}

var tokenLocalID = fftypes.NewUUID()
var TokenTransfer = &core.TokenTransfer{
	Amount:  *fftypes.NewFFBigInt(1),
	LocalID: tokenLocalID,
	Type:    "",
}

func newTestMetricsManager(t *testing.T) (*metricsManager, func()) {
	coreconfig.Reset()
	Clear()
	Registry()
	ctx, cancel := context.WithCancel(context.Background())
	mmi := NewMetricsManager(ctx)
	mm := mmi.(*metricsManager)
	assert.Equal(t, len(mm.timeMap), 0)
	return mm, cancel
}

func TestCountBatchPin(t *testing.T) {
	mm, cancel := newTestMetricsManager(t)
	defer cancel()
	mm.CountBatchPin("a-ns")
}

func TestMessageSubmittedBroadcast(t *testing.T) {
	mm, cancel := newTestMetricsManager(t)
	defer cancel()
	Message.Header.Type = core.MessageTypeBroadcast
	mm.MessageSubmitted(Message)
	assert.Equal(t, len(mm.timeMap), 1)
	assert.NotNil(t, mm.timeMap[msgID.String()])
}

func TestMessageSubmittedPrivate(t *testing.T) {
	mm, cancel := newTestMetricsManager(t)
	defer cancel()
	Message.Header.Type = core.MessageTypePrivate
	mm.MessageSubmitted(Message)
	assert.Equal(t, len(mm.timeMap), 1)
	assert.NotNil(t, mm.timeMap[msgID.String()])
}

func TestMessageConfirmedBroadcastSuccess(t *testing.T) {
	mm, cancel := newTestMetricsManager(t)
	defer cancel()
	Message.Header.Type = core.MessageTypeBroadcast
	mm.timeMap[msgID.String()] = time.Now()
	mm.MessageConfirmed(Message, core.EventTypeMessageConfirmed)
	assert.Equal(t, len(mm.timeMap), 0)
}

func TestMessageConfirmedBroadcastRejected(t *testing.T) {
	mm, cancel := newTestMetricsManager(t)
	defer cancel()
	Message.Header.Type = core.MessageTypeBroadcast
	mm.timeMap[msgID.String()] = time.Now()
	mm.MessageConfirmed(Message, core.EventTypeMessageRejected)
	assert.Equal(t, len(mm.timeMap), 0)
}

func TestMessageConfirmedPrivateSuccess(t *testing.T) {
	mm, cancel := newTestMetricsManager(t)
	defer cancel()
	Message.Header.Type = core.MessageTypePrivate
	mm.timeMap[msgID.String()] = time.Now()
	mm.MessageConfirmed(Message, core.EventTypeMessageConfirmed)
	assert.Equal(t, len(mm.timeMap), 0)
}

func TestMessageConfirmedPrivateRejected(t *testing.T) {
	mm, cancel := newTestMetricsManager(t)
	defer cancel()
	Message.Header.Type = core.MessageTypePrivate
	mm.timeMap[msgID.String()] = time.Now()
	mm.MessageConfirmed(Message, core.EventTypeMessageRejected)
	assert.Equal(t, len(mm.timeMap), 0)
}

func TestTokenSubmittedMint(t *testing.T) {
	mm, cancel := newTestMetricsManager(t)
	defer cancel()
	TokenTransfer.Type = core.TokenTransferTypeMint
	mm.TransferSubmitted(TokenTransfer)
	assert.Equal(t, len(mm.timeMap), 1)
	assert.NotNil(t, mm.timeMap[tokenLocalID.String()])
}

func TestTransferSubmittedTransfer(t *testing.T) {
	mm, cancel := newTestMetricsManager(t)
	defer cancel()
	TokenTransfer.Type = core.TokenTransferTypeTransfer
	mm.TransferSubmitted(TokenTransfer)
	assert.Equal(t, len(mm.timeMap), 1)
	assert.NotNil(t, mm.timeMap[tokenLocalID.String()])
}

func TestTransferSubmittedBurn(t *testing.T) {
	mm, cancel := newTestMetricsManager(t)
	defer cancel()
	TokenTransfer.Type = core.TokenTransferTypeBurn
	mm.TransferSubmitted(TokenTransfer)
	assert.Equal(t, len(mm.timeMap), 1)
	assert.NotNil(t, mm.timeMap[tokenLocalID.String()])
}

func TestTransferConfirmedMintSuccess(t *testing.T) {
	mm, cancel := newTestMetricsManager(t)
	defer cancel()
	TokenTransfer.Type = core.TokenTransferTypeMint
	mm.timeMap[tokenLocalID.String()] = time.Now()
	mm.TransferConfirmed(TokenTransfer)
	assert.Equal(t, len(mm.timeMap), 0)
}
func TestTransferConfirmedTransferSuccess(t *testing.T) {
	mm, cancel := newTestMetricsManager(t)
	defer cancel()
	TokenTransfer.Type = core.TokenTransferTypeTransfer
	mm.timeMap[tokenLocalID.String()] = time.Now()
	mm.TransferConfirmed(TokenTransfer)
	assert.Equal(t, len(mm.timeMap), 0)
}
func TestTransferConfirmedMintBurn(t *testing.T) {
	mm, cancel := newTestMetricsManager(t)
	defer cancel()
	TokenTransfer.Type = core.TokenTransferTypeBurn
	mm.timeMap[tokenLocalID.String()] = time.Now()
	mm.TransferConfirmed(TokenTransfer)
	assert.Equal(t, len(mm.timeMap), 0)
}

func TestBlockchainContractDeployment(t *testing.T) {
	mm, cancel := newTestMetricsManager(t)
	defer cancel()
	mm.timeMap[tokenLocalID.String()] = time.Now()
	mm.BlockchainContractDeployment()
	m, err := BlockchainTransactionsCounter.GetMetricWith(prometheus.Labels{LocationLabelName: "", MethodNameLabelName: ""})
	assert.NoError(t, err)
	v := testutil.ToFloat64(m)
	assert.Equal(t, float64(1), v)
}

func TestBlockchainTransaction(t *testing.T) {
	mm, cancel := newTestMetricsManager(t)
	defer cancel()
	mm.timeMap[tokenLocalID.String()] = time.Now()
	mm.BlockchainTransaction("location", "methodName")
	m, err := BlockchainTransactionsCounter.GetMetricWith(prometheus.Labels{LocationLabelName: "location", MethodNameLabelName: "methodName"})
	assert.NoError(t, err)
	v := testutil.ToFloat64(m)
	assert.Equal(t, float64(1), v)
}

func TestBlockchainQuery(t *testing.T) {
	mm, cancel := newTestMetricsManager(t)
	defer cancel()
	mm.timeMap[tokenLocalID.String()] = time.Now()
	mm.BlockchainQuery("location", "methodName")
	m, err := BlockchainQueriesCounter.GetMetricWith(prometheus.Labels{LocationLabelName: "location", MethodNameLabelName: "methodName"})
	assert.NoError(t, err)
	v := testutil.ToFloat64(m)
	assert.Equal(t, float64(1), v)
}

func TestBlockchainEvents(t *testing.T) {
	mm, cancel := newTestMetricsManager(t)
	defer cancel()
	mm.timeMap[tokenLocalID.String()] = time.Now()
	mm.BlockchainEvent("location", "signature")
	m, err := BlockchainEventsCounter.GetMetricWith(prometheus.Labels{LocationLabelName: "location", SignatureLabelName: "signature"})
	assert.NoError(t, err)
	v := testutil.ToFloat64(m)
	assert.Equal(t, float64(1), v)
}

func TestIsMetricsEnabledTrue(t *testing.T) {
	mm, cancel := newTestMetricsManager(t)
	defer cancel()
	mm.metricsEnabled = true
	assert.Equal(t, mm.IsMetricsEnabled(), true)
}

func TestIsMetricsEnabledFalse(t *testing.T) {
	mm, cancel := newTestMetricsManager(t)
	defer cancel()
	mm.metricsEnabled = false
	assert.Equal(t, mm.IsMetricsEnabled(), false)
}

func TestNodeIdentityDXCertMismatchSetsMismatchedState(t *testing.T) {
	mm, cancel := newTestMetricsManager(t)
	defer cancel()
	mm.NodeIdentityDXCertMismatch("test-namespace", NodeIdentityDXCertMismatchStatusMismatched)
	gaugeValue := testutil.ToFloat64(NodeIdentityDXCertMismatchGauge.WithLabelValues("test-namespace"))
	assert.Equal(t, 1.0, gaugeValue)
}

func TestNodeIdentityDXCertMismatchSetsHealthyState(t *testing.T) {
	mm, cancel := newTestMetricsManager(t)
	defer cancel()
	mm.NodeIdentityDXCertMismatch("test-namespace", NodeIdentityDXCertMismatchStatusHealthy)
	gaugeValue := testutil.ToFloat64(NodeIdentityDXCertMismatchGauge.WithLabelValues("test-namespace"))
	assert.Equal(t, 0.0, gaugeValue)
}

func TestNodeIdentityDXCertMismatchSetsUnknownState(t *testing.T) {
	mm, cancel := newTestMetricsManager(t)
	defer cancel()
	mm.NodeIdentityDXCertMismatch("test-namespace", NodeIdentityDXCertMismatchStatusUnknown)
	gaugeValue := testutil.ToFloat64(NodeIdentityDXCertMismatchGauge.WithLabelValues("test-namespace"))
	assert.Equal(t, -1.0, gaugeValue)
}

func TestNodeIdentityDXCertMismatchSetsDefaultState(t *testing.T) {
	mm, cancel := newTestMetricsManager(t)
	defer cancel()
	mm.NodeIdentityDXCertMismatch("test-namespace", "invalid-state")
	gaugeValue := testutil.ToFloat64(NodeIdentityDXCertMismatchGauge.WithLabelValues("test-namespace"))
	assert.Equal(t, -1.0, gaugeValue)
}

func TestNodeIdentityDXCertExpiry(t *testing.T) {
	mm, cancel := newTestMetricsManager(t)
	defer cancel()
	now := time.Now().UTC()
	mm.NodeIdentityDXCertExpiry("test-namespace", now)
	gaugeValue := testutil.ToFloat64(NodeIdentityDXCertExpiryGauge.WithLabelValues("test-namespace"))
	assert.Equal(t, float64(now.Unix()), gaugeValue)
}
