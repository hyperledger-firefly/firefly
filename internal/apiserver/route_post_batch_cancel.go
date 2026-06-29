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

package apiserver

import (
	"net/http"

	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
)

var postBatchCancel = &ffapi.Route{
	Name:   "postBatchCancel",
	Path:   "batches/{batchid}/cancel",
	Method: http.MethodPost,
	PathParams: []*ffapi.PathParam{
		{Name: "batchid", Description: coremsgs.APIParamsBatchID},
	},
	QueryParams:     nil,
	Description:     coremsgs.APIEndpointsPostBatchCancel,
	JSONInputValue:  nil,
	JSONOutputValue: nil,
	JSONOutputCodes: []int{http.StatusNoContent},
	Extensions: &coreExtensions{
		CoreJSONHandler: func(r *ffapi.APIRequest, cr *coreRequest) (output interface{}, err error) {
			return nil, cr.or.BatchManager().CancelBatch(cr.ctx, r.PP["batchid"])
		},
	},
}
