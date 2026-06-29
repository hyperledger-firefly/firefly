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

package ipfs

import (
	"context"
	"encoding/json"
	"fmt"

	"io"

	"github.com/go-resty/resty/v2"
	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/ffresty"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/sharedstorage"
)

type IPFS struct {
	ctx          context.Context
	capabilities *sharedstorage.Capabilities
	apiClient    *resty.Client
	gwClient     *resty.Client
}

type ipfsUploadResponse struct {
	Name string      `json:"Name"`
	Hash string      `json:"Hash"`
	Size json.Number `json:"Size"`
}

func (i *IPFS) Name() string {
	return "ipfs"
}

func (i *IPFS) Init(ctx context.Context, config config.Section) (err error) {

	i.ctx = log.WithLogFields(ctx, "sharedstorage", "ipfs")

	apiConfig := config.SubSection(IPFSConfAPISubconf)
	if apiConfig.GetString(ffresty.HTTPConfigURL) == "" {
		return i18n.NewError(ctx, coremsgs.MsgMissingPluginConfig, apiConfig.Resolve(ffresty.HTTPConfigURL), "ipfs")
	}
	i.apiClient, err = ffresty.New(i.ctx, apiConfig)
	if err != nil {
		return err
	}
	gwConfig := config.SubSection(IPFSConfGatewaySubconf)
	if gwConfig.GetString(ffresty.HTTPConfigURL) == "" {
		return i18n.NewError(ctx, coremsgs.MsgMissingPluginConfig, gwConfig.Resolve(ffresty.HTTPConfigURL), "ipfs")
	}
	i.gwClient, err = ffresty.New(i.ctx, gwConfig)
	if err != nil {
		return err
	}
	i.capabilities = &sharedstorage.Capabilities{}
	return nil
}

func (i *IPFS) SetHandler(_ string, handler sharedstorage.Callbacks) {
}

func (i *IPFS) Capabilities() *sharedstorage.Capabilities {
	return i.capabilities
}

func (i *IPFS) UploadData(ctx context.Context, data io.Reader) (string, error) {
	var ipfsResponse ipfsUploadResponse
	res, err := i.apiClient.R().
		SetContext(ctx).
		SetFileReader("document", "file.bin", data).
		SetResult(&ipfsResponse).
		Post("/api/v0/add")
	if err != nil || !res.IsSuccess() {
		return "", ffresty.WrapRestErr(i.ctx, res, err, coremsgs.MsgIPFSRESTErr)
	}
	log.L(ctx).Infof("IPFS published %s Size=%s", ipfsResponse.Hash, ipfsResponse.Size)
	return ipfsResponse.Hash, err
}

func (i *IPFS) DownloadData(ctx context.Context, payloadRef string) (data io.ReadCloser, err error) {
	res, err := i.gwClient.R().
		SetContext(ctx).
		SetDoNotParseResponse(true).
		Get(fmt.Sprintf("/ipfs/%s", payloadRef))
	ffresty.OnAfterResponse(i.gwClient, res) // required using SetDoNotParseResponse
	if err != nil || !res.IsSuccess() {
		if res != nil && res.RawBody() != nil {
			_ = res.RawBody().Close()
		}
		return nil, ffresty.WrapRestErr(i.ctx, res, err, coremsgs.MsgIPFSRESTErr)
	}
	log.L(ctx).Infof("IPFS retrieved %s", payloadRef)
	return res.RawBody(), nil
}
