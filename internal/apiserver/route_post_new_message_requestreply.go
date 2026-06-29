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
	"github.com/hyperledger-firefly/firefly/internal/orchestrator"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

var postNewMessageRequestReply = &ffapi.Route{
	Name:            "postNewMessageRequestReply",
	Path:            "messages/requestreply",
	Method:          http.MethodPost,
	PathParams:      nil,
	QueryParams:     nil,
	Description:     coremsgs.APIEndpointsPostNewMessageRequestReply,
	JSONInputValue:  func() interface{} { return &core.MessageInOut{} },
	JSONOutputValue: func() interface{} { return &core.MessageInOut{} },
	JSONOutputCodes: []int{http.StatusOK}, // Sync operation
	Extensions: &coreExtensions{
		EnabledIf: func(or orchestrator.Orchestrator) bool {
			return or.MultiParty() != nil
		},
		CoreJSONHandler: func(r *ffapi.APIRequest, cr *coreRequest) (output interface{}, err error) {
			output, err = cr.or.RequestReply(cr.ctx, r.Input.(*core.MessageInOut))
			return output, err
		},
	},
}
