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
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/hyperledger/firefly-common/pkg/ffapi"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly/internal/coremsgs"
	"github.com/hyperledger/firefly/internal/orchestrator"
	"github.com/hyperledger/firefly/pkg/core"
)

var postData = &ffapi.Route{
	Name:        "postData",
	Path:        "data",
	Method:      http.MethodPost,
	PathParams:  nil,
	QueryParams: nil,
	FormParams: []*ffapi.FormParam{
		{Name: "autometa", Description: coremsgs.APIParamsAutometa},
		{Name: "metadata", Description: coremsgs.APIParamsMetadata},
		{Name: "validator", Description: coremsgs.APIParamsValidator},
		{Name: "datatype.name", Description: coremsgs.APIParamsDatatypeName},
		{Name: "datatype.version", Description: coremsgs.APIParamsDatatypeVersion},
	},
	Description:     coremsgs.APIEndpointsPostData,
	JSONInputValue:  func() interface{} { return &core.DataRefOrValue{} },
	JSONOutputValue: func() interface{} { return &core.Data{} },
	JSONOutputCodes: []int{http.StatusCreated},
	Extensions: &coreExtensions{
		EnabledIf: func(or orchestrator.Orchestrator) bool {
			return or.Data() != nil
		},
		CoreJSONHandler: func(r *ffapi.APIRequest, cr *coreRequest) (output interface{}, err error) {
			output, err = cr.or.Data().UploadJSON(cr.ctx, r.Input.(*core.DataRefOrValue))
			return output, err
		},
		CoreFormUploadHandler: func(r *ffapi.APIRequest, cr *coreRequest) (output interface{}, err error) {
			if !cr.or.Data().BlobsEnabled() {
				return nil, i18n.NewError(r.Req.Context(), coremsgs.MsgActionNotSupported)
			}
			// Check for file upload
			if r.Part == nil || r.Part.FileName() == "" {
				return nil, i18n.NewError(r.Req.Context(), coremsgs.MsgMissingFileUpload)
			}
			data := &core.DataRefOrValue{}
			validator := r.FP["validator"]
			if len(validator) > 0 {
				data.Validator = core.ValidatorType(validator)
			}
			if r.FP["datatype.name"] != "" {
				data.Datatype = &core.DatatypeRef{
					Name:    r.FP["datatype.name"],
					Version: r.FP["datatype.version"],
				}
			}
			metadata := r.FP["metadata"]
			if len(metadata) > 0 {
				// The metadata might be JSON, or just a simple string. Try to unmarshal and see
				var marshalCheck interface{}
				if err := json.Unmarshal([]byte(metadata), &marshalCheck); err != nil {
					metadata = fmt.Sprintf(`"%s"`, metadata)
				}
				data.Value = fftypes.JSONAnyPtr(metadata)
			}
			output, err = cr.or.Data().UploadBlob(cr.ctx, data, r.Part, strings.EqualFold(r.FP["autometa"], "true"))
			return output, err
		},
	},
}
