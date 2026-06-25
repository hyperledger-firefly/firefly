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
	"strings"

	"github.com/hyperledger/firefly-common/pkg/ffapi"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly/internal/coremsgs"
	"github.com/hyperledger/firefly/internal/orchestrator"
)

var postNewContractInterface = &ffapi.Route{
	Name:       "postNewContractInterface",
	Path:       "contracts/interfaces",
	Method:     http.MethodPost,
	PathParams: nil,
	QueryParams: []*ffapi.QueryParam{
		{Name: "confirm", Description: coremsgs.APIConfirmMsgQueryParam, IsBool: true, Example: "true"},
		{Name: "publish", Description: coremsgs.APIPublishQueryParam, IsBool: true},
		{Name: "topics", Description: coremsgs.APICustomTopicsQueryParam, IsArray: true},
	},
	Description:     coremsgs.APIEndpointsPostNewContractInterface,
	JSONInputValue:  func() interface{} { return &fftypes.FFI{} },
	JSONOutputValue: func() interface{} { return &fftypes.FFI{} },
	JSONOutputCodes: []int{http.StatusOK},
	Extensions: &coreExtensions{
		EnabledIf: func(or orchestrator.Orchestrator) bool {
			return or.Contracts() != nil
		},
		CoreJSONHandler: func(r *ffapi.APIRequest, cr *coreRequest) (output interface{}, err error) {
			waitConfirm := strings.EqualFold(r.QP["confirm"], "true")
			r.SuccessStatus = syncRetcode(waitConfirm)
			ffi := r.Input.(*fftypes.FFI)
			ffi.Published = strings.EqualFold(r.QP["publish"], "true")
			err = cr.or.DefinitionSender().DefineFFI(cr.ctx, ffi, waitConfirm, r.QAP["topics"])
			return ffi, err
		},
	},
}
