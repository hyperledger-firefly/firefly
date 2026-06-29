// Copyright © 2021 Kaleido, Inc.
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
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/ffresty"
	"github.com/hyperledger-firefly/common/pkg/fftls"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/mocks/sharedstoragemocks"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
)

var utConfig = config.RootSection("ipfs_unit_tests")

func resetConf() {
	coreconfig.Reset()
	i := &IPFS{}
	i.InitConfig(utConfig)
}

func TestInitMissingAPIURL(t *testing.T) {
	i := &IPFS{}
	resetConf()

	utConfig.SubSection(IPFSConfGatewaySubconf).Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	err := i.Init(context.Background(), utConfig)
	assert.Regexp(t, "FF10138", err)
}

func TestInitMissingGWURL(t *testing.T) {
	i := &IPFS{}
	resetConf()

	utConfig.SubSection(IPFSConfAPISubconf).Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	err := i.Init(context.Background(), utConfig)
	assert.Regexp(t, "FF10138", err)
}

func TestBadTLSConfigAPI(t *testing.T) {
	i := &IPFS{}
	resetConf()

	apiConf := utConfig.SubSection(IPFSConfAPISubconf)
	apiConf.Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	tlsConf := apiConf.SubSection("tls")
	tlsConf.Set(fftls.HTTPConfTLSEnabled, true)
	tlsConf.Set(fftls.HTTPConfTLSCAFile, "!!!!!badness")
	err := i.Init(context.Background(), utConfig)
	assert.Regexp(t, "FF00153", err)
}

func TestBadTLSConfigGW(t *testing.T) {
	i := &IPFS{}
	resetConf()

	apiConf := utConfig.SubSection(IPFSConfAPISubconf)
	apiConf.Set(ffresty.HTTPConfigURL, "http://localhost:12345")

	gwConf := utConfig.SubSection(IPFSConfGatewaySubconf)
	gwConf.Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	tlsConf := gwConf.SubSection("tls")
	tlsConf.Set(fftls.HTTPConfTLSEnabled, true)
	tlsConf.Set(fftls.HTTPConfTLSCAFile, "!!!!!badness")
	err := i.Init(context.Background(), utConfig)
	assert.Regexp(t, "FF00153", err)
}

func TestInit(t *testing.T) {
	i := &IPFS{}
	resetConf()
	utConfig.SubSection(IPFSConfAPISubconf).Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	utConfig.SubSection(IPFSConfGatewaySubconf).Set(ffresty.HTTPConfigURL, "http://localhost:12345")

	cbs := &sharedstoragemocks.Callbacks{}
	i.SetHandler("ns1", cbs) // no-op

	err := i.Init(context.Background(), utConfig)
	assert.Equal(t, "ipfs", i.Name())
	assert.NoError(t, err)
	assert.NotNil(t, i.Capabilities())
}

func TestIPFSUploadSuccess(t *testing.T) {
	i := &IPFS{}

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	resetConf()
	utConfig.SubSection(IPFSConfAPISubconf).Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	utConfig.SubSection(IPFSConfGatewaySubconf).Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	utConfig.SubSection(IPFSConfAPISubconf).Set(ffresty.HTTPCustomClient, mockedClient)

	err := i.Init(context.Background(), utConfig)
	assert.NoError(t, err)

	httpmock.RegisterResponder("POST", "http://localhost:12345/api/v0/add",
		httpmock.NewJsonResponderOrPanic(200, map[string]interface{}{
			"Hash": "Qmf412jQZiuVUtdgnB36FXFX7xg5V6KEbSJ4dpQuhkLyfD",
		}))

	data := []byte(`hello world`)
	payloadRef, err := i.UploadData(context.Background(), bytes.NewReader(data))
	assert.NoError(t, err)
	assert.Equal(t, `Qmf412jQZiuVUtdgnB36FXFX7xg5V6KEbSJ4dpQuhkLyfD`, payloadRef)

}

func TestIPFSUploadFail(t *testing.T) {
	i := &IPFS{}

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	resetConf()
	utConfig.SubSection(IPFSConfAPISubconf).Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	utConfig.SubSection(IPFSConfGatewaySubconf).Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	utConfig.SubSection(IPFSConfAPISubconf).Set(ffresty.HTTPCustomClient, mockedClient)

	err := i.Init(context.Background(), utConfig)
	assert.NoError(t, err)

	httpmock.RegisterResponder("POST", "http://localhost:12345/api/v0/add",
		httpmock.NewJsonResponderOrPanic(500, map[string]interface{}{"error": "pop"}))

	data := []byte(`hello world`)
	_, err = i.UploadData(context.Background(), bytes.NewReader(data))
	assert.Regexp(t, "FF10136", err)

}

func TestIPFSDownloadSuccess(t *testing.T) {
	i := &IPFS{}

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	resetConf()
	utConfig.SubSection(IPFSConfAPISubconf).Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	utConfig.SubSection(IPFSConfGatewaySubconf).Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	utConfig.SubSection(IPFSConfGatewaySubconf).Set(ffresty.HTTPCustomClient, mockedClient)

	err := i.Init(context.Background(), utConfig)
	assert.NoError(t, err)

	data := []byte(`{"hello": "world"}`)
	httpmock.RegisterResponder("GET", "http://localhost:12345/ipfs/QmRAQfHNnknnz8S936M2yJGhhVNA6wXJ4jTRP3VXtptmmL",
		httpmock.NewBytesResponder(200, data))

	r, err := i.DownloadData(context.Background(), "QmRAQfHNnknnz8S936M2yJGhhVNA6wXJ4jTRP3VXtptmmL")
	assert.NoError(t, err)
	defer r.Close()

	var resJSON fftypes.JSONObject
	json.NewDecoder(r).Decode(&resJSON)
	assert.Equal(t, "world", resJSON["hello"])

}

func TestIPFSDownloadFail(t *testing.T) {
	i := &IPFS{}

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	resetConf()
	utConfig.SubSection(IPFSConfAPISubconf).Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	utConfig.SubSection(IPFSConfGatewaySubconf).Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	utConfig.SubSection(IPFSConfGatewaySubconf).Set(ffresty.HTTPCustomClient, mockedClient)

	err := i.Init(context.Background(), utConfig)
	assert.NoError(t, err)

	httpmock.RegisterResponder("GET", "http://localhost:12345/ipfs/QmRAQfHNnknnz8S936M2yJGhhVNA6wXJ4jTRP3VXtptmmL",
		httpmock.NewJsonResponderOrPanic(500, map[string]interface{}{"error": "pop"}))

	_, err = i.DownloadData(context.Background(), "QmRAQfHNnknnz8S936M2yJGhhVNA6wXJ4jTRP3VXtptmmL")
	assert.Regexp(t, "FF10136", err)

}

func TestIPFSDownloadError(t *testing.T) {
	i := &IPFS{}

	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	resetConf()
	utConfig.SubSection(IPFSConfAPISubconf).Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	utConfig.SubSection(IPFSConfGatewaySubconf).Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	utConfig.SubSection(IPFSConfGatewaySubconf).Set(ffresty.HTTPCustomClient, mockedClient)

	err := i.Init(context.Background(), utConfig)
	assert.NoError(t, err)

	httpmock.RegisterResponder("GET", "http://localhost:12345/ipfs/QmRAQfHNnknnz8S936M2yJGhhVNA6wXJ4jTRP3VXtptmmL",
		httpmock.NewErrorResponder(fmt.Errorf("pop")))

	_, err = i.DownloadData(context.Background(), "QmRAQfHNnknnz8S936M2yJGhhVNA6wXJ4jTRP3VXtptmmL")
	assert.Regexp(t, "FF10136", err)

}
