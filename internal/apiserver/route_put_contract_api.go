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
	"github.com/hyperledger/firefly/pkg/core"
)

var putContractAPI = &ffapi.Route{
	Name:   "putContractAPI",
	Path:   "apis/{id}",
	Method: http.MethodPut,
	PathParams: []*ffapi.PathParam{
		{Name: "id", Example: "id", Description: coremsgs.APIParamsContractAPIID},
	},
	QueryParams: []*ffapi.QueryParam{
		{Name: "confirm", Description: coremsgs.APIConfirmMsgQueryParam, IsBool: true, Example: "true"},
	},
	Description:     coremsgs.APIParamsContractAPIID,
	JSONInputValue:  func() interface{} { return &core.ContractAPI{} },
	JSONOutputValue: func() interface{} { return &core.ContractAPI{} },
	JSONOutputCodes: []int{http.StatusOK, http.StatusAccepted},
	Extensions: &coreExtensions{
		EnabledIf: func(or orchestrator.Orchestrator) bool {
			return or.Contracts() != nil
		},
		CoreJSONHandler: func(r *ffapi.APIRequest, cr *coreRequest) (output interface{}, err error) {
			waitConfirm := strings.EqualFold(r.QP["confirm"], "true")
			r.SuccessStatus = syncRetcode(waitConfirm)
			api := r.Input.(*core.ContractAPI)
			api.ID, err = fftypes.ParseUUID(cr.ctx, r.PP["id"])
			if err == nil {
				err = cr.or.DefinitionSender().DefineContractAPI(cr.ctx, cr.apiBaseURL, api, waitConfirm)
			}
			return api, err
		},
	},
}
