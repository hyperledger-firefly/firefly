// Copyright © 2023 Kaleido, Inc.
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

	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

var spiGetNamespaces = &ffapi.Route{
	Name:   "spiGetNamespaces",
	Path:   "namespaces",
	Method: http.MethodGet,
	QueryParams: []*ffapi.QueryParam{
		{Name: "includeinitializing", Example: "true", Description: coremsgs.APIParamsNSIncludeInitializing, IsBool: true},
	},
	FilterFactory:   nil,
	Description:     coremsgs.APIEndpointsAdminGetNamespaces,
	JSONInputValue:  nil,
	JSONOutputValue: func() interface{} { return []*core.NamespaceWithInitStatus{} },
	JSONOutputCodes: []int{http.StatusOK},
	Extensions: &coreExtensions{
		CoreJSONHandler: func(r *ffapi.APIRequest, cr *coreRequest) (output interface{}, err error) {
			return cr.mgr.GetNamespaces(cr.ctx, strings.EqualFold(r.QP["includeinitializing"], "true"))
		},
	},
}
