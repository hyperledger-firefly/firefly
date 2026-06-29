// Copyright © 2021 Kaleido, Inc.
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

package events

import (
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestFlushPinsFailUpdatePins(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)
	bs := newBatchState(&ag.aggregator)

	ag.mdi.On("UpdatePins", ag.ctx, "ns1", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))

	bs.markMessageDispatched(fftypes.NewUUID(), &core.Message{
		Header: core.MessageHeader{
			ID:     fftypes.NewUUID(),
			Topics: fftypes.FFStringArray{"topic1"},
		},
		Pins: fftypes.FFStringArray{"pin1"},
	}, 0, core.MessageStateConfirmed)

	err := bs.flushPins(ag.ctx)
	assert.Regexp(t, "pop", err)
}

func TestFlushPinsFailUpdateMessages(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)
	bs := newBatchState(&ag.aggregator)
	msgID := fftypes.NewUUID()

	ag.mdi.On("UpdatePins", ag.ctx, "ns1", mock.Anything, mock.Anything).Return(nil)
	ag.mdi.On("UpdateMessages", ag.ctx, "ns1", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
	ag.mdm.On("UpdateMessageStateIfCached", ag.ctx, msgID, core.MessageStateConfirmed, mock.Anything, "").Return()

	bs.markMessageDispatched(fftypes.NewUUID(), &core.Message{
		Header: core.MessageHeader{
			ID:     msgID,
			Topics: fftypes.FFStringArray{"topic1"},
		},
		Pins: fftypes.FFStringArray{"pin1"},
	}, 0, core.MessageStateConfirmed)

	err := bs.flushPins(ag.ctx)
	assert.Regexp(t, "pop", err)
}

func TestSetContextBlockedByNoState(t *testing.T) {
	ag := newTestAggregator()
	defer ag.cleanup(t)
	bs := newBatchState(&ag.aggregator)

	unmaskedContext := fftypes.NewRandB32()
	bs.SetContextBlockedBy(ag.ctx, *unmaskedContext, 10)

	ready, err := bs.checkUnmaskedContextReady(ag.ctx, unmaskedContext, &core.Message{}, 1)
	assert.NoError(t, err)
	assert.False(t, ready)
}
