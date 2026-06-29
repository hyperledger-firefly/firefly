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

package orchestrator

import (
	"context"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

func (or *orchestrator) getHistogramIntervals(startTime int64, endTime int64, numBuckets int64) (intervals []core.ChartHistogramInterval) {
	timeIntervalLength := (endTime - startTime) / numBuckets

	for i := startTime; i < endTime; i += timeIntervalLength {
		intervals = append(intervals, core.ChartHistogramInterval{
			StartTime: fftypes.UnixTime(i),
			EndTime:   fftypes.UnixTime(i + timeIntervalLength),
		})
	}

	return intervals
}

func (or *orchestrator) GetChartHistogram(ctx context.Context, startTime int64, endTime int64, buckets int64, collection database.CollectionName) ([]*core.ChartHistogram, error) {
	if buckets > core.ChartHistogramMaxBuckets || buckets < core.ChartHistogramMinBuckets {
		return nil, i18n.NewError(ctx, coremsgs.MsgInvalidNumberOfIntervals, core.ChartHistogramMinBuckets, core.ChartHistogramMaxBuckets)
	}
	if startTime > endTime {
		return nil, i18n.NewError(ctx, coremsgs.MsgHistogramInvalidTimes)
	}

	intervals := or.getHistogramIntervals(startTime, endTime, buckets)

	histogram, err := or.database().GetChartHistogram(ctx, or.namespace.Name, intervals, collection)
	if err != nil {
		return nil, err
	}

	return histogram, nil
}
