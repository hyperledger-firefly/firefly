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

	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

var spiPatchOpByID = &ffapi.Route{
	Name:   "spiPatchOpByID",
	Path:   "operations/{nsopid}",
	Method: http.MethodPatch,
	PathParams: []*ffapi.PathParam{
		{Name: "nsopid", Description: coremsgs.APIParamsOperationNamespacedID},
	},
	QueryParams:     nil,
	Description:     coremsgs.APIEndpointsAdminPatchOpByID,
	JSONInputValue:  func() interface{} { return &core.OperationUpdateDTO{} },
	JSONOutputValue: func() interface{} { return &core.EmptyInput{} },
	JSONOutputCodes: []int{http.StatusOK},
	Extensions: &coreExtensions{
		CoreJSONHandler: func(r *ffapi.APIRequest, cr *coreRequest) (output interface{}, err error) {
			err = cr.mgr.ResolveOperationByNamespacedID(cr.ctx, r.PP["nsopid"], r.Input.(*core.OperationUpdateDTO))
			return &core.EmptyInput{}, err
		},
	},
}
