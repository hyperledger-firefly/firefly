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
	"github.com/hyperledger/firefly/internal/coremsgs"
	"github.com/hyperledger/firefly/pkg/core"
)

// @Summary Update an identity
// @ID patchUpdateIdentity
// @Tags identities
// @Accept json
// @Produce json
// @Param iid path string true "Identity ID"  <-- THE CRITICAL FIX
// @Param body body core.IdentityUpdateDTO true "Identity update details"
// @Success 202 {object} core.Identity
// @Router /identities/{iid} [patch]
var patchUpdateIdentity = &ffapi.Route{
	Name:   "patchUpdateIdentity",
	Path:   "identities/{iid}",
	Method: http.MethodPatch,
	PathParams: []*ffapi.PathParam{
		{Name: "iid", Description: coremsgs.APIParamsIdentityID},
	},
	QueryParams: []*ffapi.QueryParam{
		{Name: "confirm", Description: coremsgs.APIConfirmMsgQueryParam, IsBool: true},
	},
	Description:     coremsgs.APIEndpointsPatchUpdateIdentity,
	JSONInputValue:  func() interface{} { return &core.IdentityUpdateDTO{} },
	JSONOutputValue: func() interface{} { return &core.Identity{} },
	JSONOutputCodes: []int{http.StatusAccepted, http.StatusOK},
	Extensions: &coreExtensions{
		CoreJSONHandler: func(r *ffapi.APIRequest, cr *coreRequest) (output interface{}, err error) {
			waitConfirm := strings.EqualFold(r.QP["confirm"], "true")
			r.SuccessStatus = syncRetcode(waitConfirm)
			org, err := cr.or.NetworkMap().UpdateIdentity(cr.ctx, r.PP["iid"], r.Input.(*core.IdentityUpdateDTO), waitConfirm)
			return org, err
		},
	},
}
