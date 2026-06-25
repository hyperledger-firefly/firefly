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
	"github.com/hyperledger/firefly/pkg/core"
)

var postContractInterfacePublish = &ffapi.Route{
	Name:   "postContractInterfacePublish",
	Path:   "contracts/interfaces/{name}/{version}/publish",
	Method: http.MethodPost,
	PathParams: []*ffapi.PathParam{
		{Name: "name", Description: coremsgs.APIParamsContractInterfaceName},
		{Name: "version", Description: coremsgs.APIParamsContractInterfaceVersion},
	},
	QueryParams: []*ffapi.QueryParam{
		{Name: "confirm", Description: coremsgs.APIConfirmMsgQueryParam, IsBool: true},
		{Name: "topics", Description: coremsgs.APICustomTopicsQueryParam, IsArray: true},
	},
	Description:     coremsgs.APIEndpointsPostContractInterfacePublish,
	JSONInputValue:  func() interface{} { return &core.DefinitionPublish{} },
	JSONOutputValue: func() interface{} { return &fftypes.FFI{} },
	JSONOutputCodes: []int{http.StatusAccepted, http.StatusOK},
	Extensions: &coreExtensions{
		CoreJSONHandler: func(r *ffapi.APIRequest, cr *coreRequest) (output interface{}, err error) {
			waitConfirm := strings.EqualFold(r.QP["confirm"], "true")
			r.SuccessStatus = syncRetcode(waitConfirm)
			input := r.Input.(*core.DefinitionPublish)
			return cr.or.DefinitionSender().PublishFFI(cr.ctx, r.PP["name"], r.PP["version"], input.NetworkName, waitConfirm, r.QAP["topics"])
		},
	},
}
