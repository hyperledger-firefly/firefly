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

package orchestrator

import (
	"context"
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func makeTestIntervals(start int, numIntervals int) (intervals []core.ChartHistogramInterval) {
	for i := 0; i < numIntervals; i++ {
		intervals = append(intervals, core.ChartHistogramInterval{
			StartTime: fftypes.UnixTime(int64(start + i)),
			EndTime:   fftypes.UnixTime(int64(start + i + 1)),
		})
	}
	return intervals
}

func TestGetHistogramBadIntervalMin(t *testing.T) {
	or := newTestOrchestrator()
	_, err := or.GetChartHistogram(context.Background(), 1234567890, 9876543210, core.ChartHistogramMinBuckets-1, database.CollectionName("test"))
	assert.Regexp(t, "FF10298", err)
}

func TestGetHistogramBadIntervalMax(t *testing.T) {
	or := newTestOrchestrator()
	_, err := or.GetChartHistogram(context.Background(), 1234567890, 9876543210, core.ChartHistogramMaxBuckets+1, database.CollectionName("test"))
	assert.Regexp(t, "FF10298", err)
}

func TestGetHistogramBadStartEndTimes(t *testing.T) {
	or := newTestOrchestrator()
	_, err := or.GetChartHistogram(context.Background(), 9876543210, 1234567890, 10, database.CollectionName("test"))
	assert.Regexp(t, "FF10300", err)
}

func TestGetHistogramFailDB(t *testing.T) {
	or := newTestOrchestrator()
	intervals := makeTestIntervals(1000000000, 10)
	or.mdi.On("GetChartHistogram", mock.Anything, "ns", intervals, database.CollectionName("test")).Return(nil, fmt.Errorf("pop"))
	_, err := or.GetChartHistogram(context.Background(), 1000000000, 1000000010, 10, database.CollectionName("test"))
	assert.EqualError(t, err, "pop")
}

func TestGetHistogramSuccess(t *testing.T) {
	or := newTestOrchestrator()
	intervals := makeTestIntervals(1000000000, 10)
	mockHistogram := []*core.ChartHistogram{}

	or.mdi.On("GetChartHistogram", mock.Anything, "ns", intervals, database.CollectionName("test")).Return(mockHistogram, nil)
	_, err := or.GetChartHistogram(context.Background(), 1000000000, 1000000010, 10, database.CollectionName("test"))
	assert.NoError(t, err)
}
