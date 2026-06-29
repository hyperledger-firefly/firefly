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

package apiserver

import (
	"net/http"
	"strconv"

	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

var getChartHistogram = &ffapi.Route{
	Name:   "getChartHistogram",
	Path:   "charts/histogram/{collection}",
	Method: http.MethodGet,
	PathParams: []*ffapi.PathParam{
		{Name: "collection", Description: coremsgs.APIParamsCollectionID},
	},
	QueryParams: []*ffapi.QueryParam{
		{Name: "startTime", Description: coremsgs.APIHistogramStartTimeParam, IsBool: false},
		{Name: "endTime", Description: coremsgs.APIHistogramEndTimeParam, IsBool: false},
		{Name: "buckets", Description: coremsgs.APIHistogramBucketsParam, IsBool: false},
	},
	Description:     coremsgs.APIEndpointsGetChartHistogram,
	JSONInputValue:  nil,
	JSONOutputValue: func() interface{} { return []*core.ChartHistogram{} },
	JSONOutputCodes: []int{http.StatusOK},
	Extensions: &coreExtensions{
		CoreJSONHandler: func(r *ffapi.APIRequest, cr *coreRequest) (output interface{}, err error) {
			startTime, err := fftypes.ParseTimeString(r.QP["startTime"])
			if err != nil {
				return nil, i18n.NewError(cr.ctx, coremsgs.MsgInvalidChartNumberParam, "startTime")
			}
			endTime, err := fftypes.ParseTimeString(r.QP["endTime"])
			if err != nil {
				return nil, i18n.NewError(cr.ctx, coremsgs.MsgInvalidChartNumberParam, "endTime")
			}
			buckets, err := strconv.ParseInt(r.QP["buckets"], 10, 64)
			if err != nil {
				return nil, i18n.NewError(cr.ctx, coremsgs.MsgInvalidChartNumberParam, "buckets")
			}
			return cr.or.GetChartHistogram(cr.ctx, startTime.UnixNano(), endTime.UnixNano(), buckets, database.CollectionName(r.PP["collection"]))
		},
	},
}
