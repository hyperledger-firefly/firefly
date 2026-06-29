// Copyright © 2025 Kaleido, Inc.
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

package ffdx

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/hyperledger-firefly/firefly/internal/metrics"

	"github.com/hyperledger-firefly/firefly/mocks/metricsmocks"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/ffresty"
	"github.com/hyperledger-firefly/common/pkg/fftls"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/retry"
	"github.com/hyperledger-firefly/common/pkg/wsclient"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/mocks/coremocks"
	"github.com/hyperledger-firefly/firefly/mocks/dataexchangemocks"
	"github.com/hyperledger-firefly/firefly/mocks/wsmocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/dataexchange"
	"github.com/jarcoal/httpmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var utConfig = config.RootSection("ffdx_unit_tests")

const (
	// NOTE: the CA cert expires on Monday, February 28, 2035 7:30:57 PM UTC,
	//       and the leaf cert expires on Monday, February 28, 2028 7:30:57 PM UTC
	testCertBundle = `
-----BEGIN CERTIFICATE-----
MIIDqTCCApGgAwIBAgIUbZT+Ds4f2oDmGpgVi+SaQq9gxvcwDQYJKoZIhvcNAQEL
BQAwZDELMAkGA1UEBhMCVVMxEzARBgNVBAgMCkNhbGlmb3JuaWExFjAUBgNVBAcM
DVNhbiBGcmFuY2lzY28xEzARBgNVBAoMCkV4YW1wbGUgQ0ExEzARBgNVBAMMCmV4
YW1wbGUtY2EwHhcNMjUwMjI4MTkzMDM4WhcNMzUwMjI2MTkzMDM4WjBkMQswCQYD
VQQGEwJVUzETMBEGA1UECAwKQ2FsaWZvcm5pYTEWMBQGA1UEBwwNU2FuIEZyYW5j
aXNjbzETMBEGA1UECgwKRXhhbXBsZSBDQTETMBEGA1UEAwwKZXhhbXBsZS1jYTCC
ASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAOWBryFqk0YqQ6pzGJvBDjbV
4BnkMzsv+Fq869Xks09OP4eW44oqfFUmpCFyS3fEmRCz+389t4mKvxcCRIJMW0f5
K9jffG1QKUKL4UuNfEPFpM0MXTwhI+dCdvofdelzc+KBGA6CDYlnWYcCKFSuWeSu
xrb/qCEvhcCaSYt3e2WcRHRuK+OLzM3REeJctC4G/pq858OUV5CZU2B6aGV/9uFL
ZW3TCrOaj+Khzzt5FNvjVdLiUw0FS8VESxFA4kH8p+XUshs9S0e7LfIBSID2NU8+
+5D6HliqNqikbsny1Ps6GhLa+nI37LOVj7nFcG7uk+gb6HUN1+0YvjOJ0/zvnLEC
AwEAAaNTMFEwHQYDVR0OBBYEFJfNoXmIn5S6W7Lcj5G/huW5q1YQMB8GA1UdIwQY
MBaAFJfNoXmIn5S6W7Lcj5G/huW5q1YQMA8GA1UdEwEB/wQFMAMBAf8wDQYJKoZI
hvcNAQELBQADggEBALsdRYJHQMkhjLcrO4Yha1KXh2d+irmi8AqQqQgbLIsSzuqG
bKFiYnJ8PKHaISHlev2xRM9kEjDZ/9q8T4aUELg4eBjj7VK+gs+gSBO6peJ+AcEg
TepsE5GHmhoIIiE/3dIP6XnaM6NBb8q0ewsIg1c5vLlrt8W96LY6Og7f+742VvoV
H31srpGjy7c5nYjBTn/Bu84eb5Lxfvy10sJjnenkXDJvzkUcnfbRzDQ9k5ZuPa05
x+BsxonN0iaeZH91F+Y3kgJidLnU5EhIB/1KXYjuEbl9qUxD6GFHRststPRPeOmj
7C+BtJCIjjavysSqVMvQWLQ6rXms3SpRPAimWqM=
-----END CERTIFICATE-----
-----BEGIN CERTIFICATE-----
MIIDqjCCApKgAwIBAgIUWnobAQ4vq8gWBAXZBf7XZG3oSiUwDQYJKoZIhvcNAQEL
BQAwZDELMAkGA1UEBhMCVVMxEzARBgNVBAgMCkNhbGlmb3JuaWExFjAUBgNVBAcM
DVNhbiBGcmFuY2lzY28xEzARBgNVBAoMCkV4YW1wbGUgQ0ExEzARBgNVBAMMCmV4
YW1wbGUtY2EwHhcNMjUwMjI4MTkzMDU3WhcNMjgwMjI4MTkzMDU3WjB2MQswCQYD
VQQGEwJVUzETMBEGA1UECAwKQ2FsaWZvcm5pYTEWMBQGA1UEBwwNU2FuIEZyYW5j
aXNjbzEcMBoGA1UECgwTSHlwZXJsZWRnZXIgRmlyZWZseTEcMBoGA1UEAwwTaHlw
ZXJsZWRnZXItZmlyZWZseTCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEB
AKlbQ+7cWWS0+QPp03PrdxsnAAtG2tWOk2CEG7HS3AlBU82YImhCidKOw+jQPS68
2f2d0tYBhugqB2Ki6HsfYMGTjHDLbUQ5y+cLk6PFbvhjm39Ayd+WGmhWht5qFtRN
gllTa/SbG8+iGaSPIVFCyvg1IzxsFnBGn+05Gu+KjpL4i0l1RqDmy5ItxKGP77in
RPEUkejiUozg/X3v2TWAGagIVF5+EQ2Cswot9W1faAvyu/QmSGLLfSH22GdEDHXa
U4DV5ArJ2U2eNkOuasSWGKBopa/Wh1SZjKrNsy5Gw84ihAI4k7ARoP+vu1dIPdaX
ElipmGMtUWu0Azn2l9QJZpMCAwEAAaNCMEAwHQYDVR0OBBYEFL798jEmX2+hw70t
SmfJA78PZnnHMB8GA1UdIwQYMBaAFJfNoXmIn5S6W7Lcj5G/huW5q1YQMA0GCSqG
SIb3DQEBCwUAA4IBAQBY1NXTuQJZvjip33dRXyWP6GsSDKbXTSCcSF38P4/m+pcH
r/q/upo+K+8eTtPqUwBsIywH5bypWqoIPtM+rkd3FVBe7uti2FExufpcOruzEGsY
rNTfiFZbc7eHmFRTkKXWW4j6b6ElygrBvV999BhCRNf6NS0/syjqsbALHkFGeIcl
78wdaR+m2XVJBV7SmPmZ/EQzxvhCZONNVyU5zvW2sehI7sRbZt9/FG5U1Ng0LarW
R0gnXX/IZFnLhLh6UpLOBB0KIGENh75EEU7755jMKDKFj16D0uA1Lzrh5YxicTMy
ydFYQLpLycsWl2oV3JB4pO5TIzjY9awkRE0MeMMc
-----END CERTIFICATE-----
`
	// The CA is at the bottom of the bundle and expires same as above,
	// Monday, February 28, 2035 7:30:57 PM. The leaf cert at the top of
	// the bundle is already expired.
	testExpiredCert = `
-----BEGIN CERTIFICATE-----
MIIDqjCCApKgAwIBAgIUWnobAQ4vq8gWBAXZBf7XZG3oSicwDQYJKoZIhvcNAQEL
BQAwZDELMAkGA1UEBhMCVVMxEzARBgNVBAgMCkNhbGlmb3JuaWExFjAUBgNVBAcM
DVNhbiBGcmFuY2lzY28xEzARBgNVBAoMCkV4YW1wbGUgQ0ExEzARBgNVBAMMCmV4
YW1wbGUtY2EwHhcNMjUwMzAyMTQxNDA1WhcNMjUwMzAyMTQxNDA1WjB2MQswCQYD
VQQGEwJVUzETMBEGA1UECAwKQ2FsaWZvcm5pYTEWMBQGA1UEBwwNU2FuIEZyYW5j
aXNjbzEcMBoGA1UECgwTSHlwZXJsZWRnZXIgRmlyZWZseTEcMBoGA1UEAwwTaHlw
ZXJsZWRnZXItZmlyZWZseTCCASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEB
AKlbQ+7cWWS0+QPp03PrdxsnAAtG2tWOk2CEG7HS3AlBU82YImhCidKOw+jQPS68
2f2d0tYBhugqB2Ki6HsfYMGTjHDLbUQ5y+cLk6PFbvhjm39Ayd+WGmhWht5qFtRN
gllTa/SbG8+iGaSPIVFCyvg1IzxsFnBGn+05Gu+KjpL4i0l1RqDmy5ItxKGP77in
RPEUkejiUozg/X3v2TWAGagIVF5+EQ2Cswot9W1faAvyu/QmSGLLfSH22GdEDHXa
U4DV5ArJ2U2eNkOuasSWGKBopa/Wh1SZjKrNsy5Gw84ihAI4k7ARoP+vu1dIPdaX
ElipmGMtUWu0Azn2l9QJZpMCAwEAAaNCMEAwHQYDVR0OBBYEFL798jEmX2+hw70t
SmfJA78PZnnHMB8GA1UdIwQYMBaAFJfNoXmIn5S6W7Lcj5G/huW5q1YQMA0GCSqG
SIb3DQEBCwUAA4IBAQA1dY8UCf1YSLG3Vu/u20ucw5q9tnYYoTi4fHm/g+imTvEQ
KgTQBd5s9EHmj0BSjVFy9eSSTx5XiH6JqzGhCJRSbOIQ8RwrXpUlTuLr7gp0cO9c
Ykxz6wt0k1F+9Iq+K8Eb6jzXoZe/ebMz611zUqY7+9lIl1AIIgx96MoZcDS/LA0e
p/TUQ6q+Mg3W9pSXqLm8jmWNBfDViQF1v9Z3ASFYHUF/yak8jMdBEUpAqDadd/ay
BHm9m8IvFevQjpUw6kyyg77ehEBBn7H/ISTL3HTCpUbkR3qUnFjOyBJ0G02XoozB
I/hI0mpd6y+/JwyvG0smbD2lioiO/JQaUEZGU8pU
-----END CERTIFICATE-----
-----BEGIN CERTIFICATE-----
MIIDqTCCApGgAwIBAgIUbZT+Ds4f2oDmGpgVi+SaQq9gxvcwDQYJKoZIhvcNAQEL
BQAwZDELMAkGA1UEBhMCVVMxEzARBgNVBAgMCkNhbGlmb3JuaWExFjAUBgNVBAcM
DVNhbiBGcmFuY2lzY28xEzARBgNVBAoMCkV4YW1wbGUgQ0ExEzARBgNVBAMMCmV4
YW1wbGUtY2EwHhcNMjUwMjI4MTkzMDM4WhcNMzUwMjI2MTkzMDM4WjBkMQswCQYD
VQQGEwJVUzETMBEGA1UECAwKQ2FsaWZvcm5pYTEWMBQGA1UEBwwNU2FuIEZyYW5j
aXNjbzETMBEGA1UECgwKRXhhbXBsZSBDQTETMBEGA1UEAwwKZXhhbXBsZS1jYTCC
ASIwDQYJKoZIhvcNAQEBBQADggEPADCCAQoCggEBAOWBryFqk0YqQ6pzGJvBDjbV
4BnkMzsv+Fq869Xks09OP4eW44oqfFUmpCFyS3fEmRCz+389t4mKvxcCRIJMW0f5
K9jffG1QKUKL4UuNfEPFpM0MXTwhI+dCdvofdelzc+KBGA6CDYlnWYcCKFSuWeSu
xrb/qCEvhcCaSYt3e2WcRHRuK+OLzM3REeJctC4G/pq858OUV5CZU2B6aGV/9uFL
ZW3TCrOaj+Khzzt5FNvjVdLiUw0FS8VESxFA4kH8p+XUshs9S0e7LfIBSID2NU8+
+5D6HliqNqikbsny1Ps6GhLa+nI37LOVj7nFcG7uk+gb6HUN1+0YvjOJ0/zvnLEC
AwEAAaNTMFEwHQYDVR0OBBYEFJfNoXmIn5S6W7Lcj5G/huW5q1YQMB8GA1UdIwQY
MBaAFJfNoXmIn5S6W7Lcj5G/huW5q1YQMA8GA1UdEwEB/wQFMAMBAf8wDQYJKoZI
hvcNAQELBQADggEBALsdRYJHQMkhjLcrO4Yha1KXh2d+irmi8AqQqQgbLIsSzuqG
bKFiYnJ8PKHaISHlev2xRM9kEjDZ/9q8T4aUELg4eBjj7VK+gs+gSBO6peJ+AcEg
TepsE5GHmhoIIiE/3dIP6XnaM6NBb8q0ewsIg1c5vLlrt8W96LY6Og7f+742VvoV
H31srpGjy7c5nYjBTn/Bu84eb5Lxfvy10sJjnenkXDJvzkUcnfbRzDQ9k5ZuPa05
x+BsxonN0iaeZH91F+Y3kgJidLnU5EhIB/1KXYjuEbl9qUxD6GFHRststPRPeOmj
7C+BtJCIjjavysSqVMvQWLQ6rXms3SpRPAimWqM=
-----END CERTIFICATE-----
`
)

func newTestFFDX(t *testing.T, manifestEnabled bool) (h *FFDX, toServer, fromServer chan string, httpURL string, done func()) {
	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)

	toServer, fromServer, wsURL, cancel := wsclient.NewTestWSServer(nil)

	u, _ := url.Parse(wsURL)
	u.Scheme = "http"
	httpURL = u.String()

	coreconfig.Reset()
	h.InitConfig(utConfig)
	utConfig.Set(ffresty.HTTPConfigURL, httpURL)
	utConfig.Set(ffresty.HTTPCustomClient, mockedClient)
	utConfig.Set(DataExchangeManifestEnabled, manifestEnabled)

	h = &FFDX{initialized: true}
	h.InitConfig(utConfig)

	mmm := metricsmocks.NewManager(t)
	dxCtx, dxCancel := context.WithCancel(context.Background())
	err := h.Init(dxCtx, dxCancel, utConfig, mmm)
	assert.NoError(t, err)
	assert.Equal(t, "ffdx", h.Name())
	assert.NotNil(t, h.Capabilities())
	return h, toServer, fromServer, httpURL, func() {
		cancel()
		dxCancel()
		httpmock.DeactivateAndReset()
	}
}

func TestSplitBlobPath(t *testing.T) {
	prefix, namespace, id := splitBlobPath("")
	assert.Equal(t, "", prefix)
	assert.Equal(t, "", namespace)
	assert.Equal(t, "", id)

	prefix, namespace, id = splitBlobPath("123")
	assert.Equal(t, "", prefix)
	assert.Equal(t, "", namespace)
	assert.Equal(t, "123", id)

	prefix, namespace, id = splitBlobPath("ns1/123")
	assert.Equal(t, "", prefix)
	assert.Equal(t, "ns1", namespace)
	assert.Equal(t, "123", id)

	prefix, namespace, id = splitBlobPath("/ns1/123")
	assert.Equal(t, "", prefix)
	assert.Equal(t, "ns1", namespace)
	assert.Equal(t, "123", id)

	prefix, namespace, id = splitBlobPath("/root/test/ns1/123")
	assert.Equal(t, "/root/test", prefix)
	assert.Equal(t, "ns1", namespace)
	assert.Equal(t, "123", id)
}

func TestJoinBlobPath(t *testing.T) {
	path := joinBlobPath("ns1", "123")
	assert.Equal(t, "ns1/123", path)
}

func TestInitBadURL(t *testing.T) {
	coreconfig.Reset()
	h := &FFDX{}
	h.InitConfig(utConfig)
	utConfig.Set(ffresty.HTTPConfigURL, "::::////")
	ctx, cancel := context.WithCancel(context.Background())
	err := h.Init(ctx, cancel, utConfig, nil)
	assert.Regexp(t, "FF00149", err)
}

func TestInitBadTLS(t *testing.T) {
	coreconfig.Reset()
	h := &FFDX{}
	h.InitConfig(utConfig)
	utConfig.Set(ffresty.HTTPConfigURL, "http://localhost:12345")
	tlsConfig := utConfig.SubSection("tls")
	tlsConfig.Set(fftls.HTTPConfTLSEnabled, true)
	tlsConfig.Set(fftls.HTTPConfTLSCAFile, "badCA")
	ctx, cancel := context.WithCancel(context.Background())
	err := h.Init(ctx, cancel, utConfig, nil)
	assert.Regexp(t, "FF00153", err)
}

func TestInitMissingURL(t *testing.T) {
	coreconfig.Reset()
	h := &FFDX{}
	h.InitConfig(utConfig)
	ctx, cancel := context.WithCancel(context.Background())
	err := h.Init(ctx, cancel, utConfig, nil)
	assert.Regexp(t, "FF10138", err)
}

func opAcker() func(args mock.Arguments) {
	return func(args mock.Arguments) {
		args[0].(*core.OperationUpdateAsync).OnComplete()
	}
}

func acker() func(args mock.Arguments) {
	return func(args mock.Arguments) {
		args[1].(*dxEvent).Ack()
	}
}

func TestInitWithBackgroundStart(t *testing.T) {
	h, _, _, _, done := newTestFFDX(t, false)
	defer done()
	utConfig.Set(DataExchangeBackgroundStart, true)

	h.InitConfig(utConfig)
	ctx, cancel := context.WithCancel(context.Background())
	err := h.Init(ctx, cancel, utConfig, nil)
	assert.NoError(t, err)

	assert.NotNil(t, h.backgroundRetry)
}

func manifestAcker(manifest string) func(args mock.Arguments) {
	return func(args mock.Arguments) {
		args[1].(dataexchange.DXEvent).AckWithManifest(manifest)
	}
}

func TestGetEndpointInfo(t *testing.T) {
	h, _, _, httpURL, done := newTestFFDX(t, false)
	defer done()

	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/v1/id", httpURL),
		httpmock.NewJsonResponderOrPanic(200, fftypes.JSONObject{
			"id":       "peer1",
			"endpoint": "https://peer1.example.com",
			"cert":     "cert data...",
		}))

	peer, err := h.GetEndpointInfo(context.Background(), "node1")
	assert.NoError(t, err)
	assert.Equal(t, fftypes.JSONObject{
		"id":       "peer1/node1",
		"endpoint": "https://peer1.example.com",
		"cert":     "cert data...",
	}, peer)
}

func TestGetEndpointMissingID(t *testing.T) {
	h, _, _, httpURL, done := newTestFFDX(t, false)
	defer done()

	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/v1/id", httpURL),
		httpmock.NewJsonResponderOrPanic(200, fftypes.JSONObject{
			"endpoint": "https://peer1.example.com",
			"cert":     "cert data...",
		}))

	_, err := h.GetEndpointInfo(context.Background(), "node1")
	assert.Regexp(t, "FF10367", err)
}

func TestGetEndpointInfoError(t *testing.T) {
	h, _, _, httpURL, done := newTestFFDX(t, false)
	defer done()

	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/v1/id", httpURL),
		httpmock.NewJsonResponderOrPanic(500, fftypes.JSONObject{}))

	_, err := h.GetEndpointInfo(context.Background(), "node1")
	assert.Regexp(t, "FF10229", err)
}

func TestAckClosed(t *testing.T) {
	h, _, _, _, done := newTestFFDX(t, false)
	done()

	dxe := &dxEvent{
		ffdx: h,
	}
	dxe.AckWithManifest("")
}

func TestAddPeer(t *testing.T) {
	h, _, _, httpURL, done := newTestFFDX(t, false)
	defer done()

	httpmock.RegisterResponder("PUT", fmt.Sprintf("%s/api/v1/peers/peer1", httpURL),
		httpmock.NewJsonResponderOrPanic(200, fftypes.JSONObject{}))

	err := h.AddNode(context.Background(), "ns1", "node1", fftypes.JSONObject{
		"id":       "peer1",
		"endpoint": "https://peer1.example.com",
		"cert":     "cert...",
	})
	assert.NoError(t, err)
}

func TestAddPeerError(t *testing.T) {
	h, _, _, httpURL, done := newTestFFDX(t, false)
	defer done()

	httpmock.RegisterResponder("PUT", fmt.Sprintf("%s/api/v1/peers/peer1", httpURL),
		httpmock.NewJsonResponderOrPanic(500, fftypes.JSONObject{}))

	err := h.AddNode(context.Background(), "ns1", "node1", fftypes.JSONObject{
		"id": "peer1",
	})
	assert.Regexp(t, "FF10229", err)
}

func TestUploadBlob(t *testing.T) {

	h, _, _, httpURL, done := newTestFFDX(t, false)
	defer done()

	u := fftypes.NewUUID()
	hash := fftypes.NewRandB32()
	httpmock.RegisterResponder("PUT", fmt.Sprintf("%s/api/v1/blobs/ns1/%s", httpURL, u),
		func(r *http.Request) (*http.Response, error) {
			res := &http.Response{
				Body: ioutil.NopCloser(bytes.NewReader([]byte(fmt.Sprintf(`{
					"hash": "%s",
					"size": 12345
				}`, hash)))),
				Header: http.Header{
					"Content-Type": []string{"application/json"},
					"Dx-Hash":      []string{hash.String()},
				},
				StatusCode: 200,
			}
			return res, nil
		})

	payloadRef, hashReturned, sizeReturned, err := h.UploadBlob(context.Background(), "ns1", *u, bytes.NewReader([]byte(`{}`)))
	assert.NoError(t, err)
	assert.Equal(t, fmt.Sprintf("ns1/%s", u.String()), payloadRef)
	assert.Equal(t, *hash, *hashReturned)
	assert.Equal(t, int64(12345), sizeReturned)
}

func TestUploadBlobBadHash(t *testing.T) {

	h, _, _, httpURL, done := newTestFFDX(t, false)
	defer done()

	u := fftypes.NewUUID()
	httpmock.RegisterResponder("PUT", fmt.Sprintf("%s/api/v1/blobs/ns1/%s", httpURL, u),
		func(r *http.Request) (*http.Response, error) {
			res := &http.Response{
				Body: ioutil.NopCloser(bytes.NewReader([]byte(`{}`))),
				Header: http.Header{
					"Dx-Hash": []string{"!hash"},
				},
				StatusCode: 200,
			}
			return res, nil
		})

	_, _, _, err := h.UploadBlob(context.Background(), "ns1", *u, bytes.NewReader([]byte(`{}`)))
	assert.Regexp(t, "FF10237", err)
}

func TestUploadBlobError(t *testing.T) {
	h, _, _, httpURL, done := newTestFFDX(t, false)
	defer done()

	u := fftypes.NewUUID()
	httpmock.RegisterResponder("PUT", fmt.Sprintf("%s/api/v1/blobs/ns1/%s", httpURL, u),
		httpmock.NewJsonResponderOrPanic(500, fftypes.JSONObject{}))

	_, _, _, err := h.UploadBlob(context.Background(), "ns1", *u, bytes.NewReader([]byte(`{}`)))
	assert.Regexp(t, "FF10229", err)
}

func TestDownloadBlob(t *testing.T) {

	h, _, _, httpURL, done := newTestFFDX(t, false)
	defer done()

	u := fftypes.NewUUID()
	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/v1/blobs/ns1/%s", httpURL, u),
		httpmock.NewBytesResponder(200, []byte(`some data`)))

	rc, err := h.DownloadBlob(context.Background(), fmt.Sprintf("ns1/%s", u))
	assert.NoError(t, err)
	b, err := ioutil.ReadAll(rc)
	rc.Close()
	assert.Equal(t, `some data`, string(b))
}

func TestDownloadBlobError(t *testing.T) {
	h, _, _, httpURL, done := newTestFFDX(t, false)
	defer done()

	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/v1/blobs/bad", httpURL),
		httpmock.NewJsonResponderOrPanic(500, fftypes.JSONObject{}))

	_, err := h.DownloadBlob(context.Background(), "bad")
	assert.Regexp(t, "FF10229", err)
}

func TestSendMessage(t *testing.T) {

	h, _, _, httpURL, done := newTestFFDX(t, false)
	defer done()

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/messages", httpURL),
		httpmock.NewJsonResponderOrPanic(200, fftypes.JSONObject{}))

	peer := fftypes.JSONObject{"id": "peer1"}
	sender := fftypes.JSONObject{"id": "sender1"}
	err := h.SendMessage(context.Background(), "ns1:"+fftypes.NewUUID().String(), peer, sender, []byte(`some data`))
	assert.NoError(t, err)
}

func TestSendMessageError(t *testing.T) {
	h, _, _, httpURL, done := newTestFFDX(t, false)
	defer done()

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/message", httpURL),
		httpmock.NewJsonResponderOrPanic(500, fftypes.JSONObject{}))

	peer := fftypes.JSONObject{"id": "peer1"}
	sender := fftypes.JSONObject{"id": "sender1"}
	err := h.SendMessage(context.Background(), "ns1:"+fftypes.NewUUID().String(), peer, sender, []byte(`some data`))
	assert.Regexp(t, "FF10229", err)
}

func TestTransferBlob(t *testing.T) {

	h, _, _, httpURL, done := newTestFFDX(t, false)
	defer done()

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/transfers", httpURL),
		httpmock.NewJsonResponderOrPanic(200, fftypes.JSONObject{}))

	peer := fftypes.JSONObject{"id": "peer1"}
	sender := fftypes.JSONObject{"id": "sender1"}
	err := h.TransferBlob(context.Background(), "ns1:"+fftypes.NewUUID().String(), peer, sender, "ns1/id1")
	assert.NoError(t, err)
}

func TestTransferBlobError(t *testing.T) {
	h, _, _, httpURL, done := newTestFFDX(t, false)
	defer done()

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/transfers", httpURL),
		httpmock.NewJsonResponderOrPanic(500, fftypes.JSONObject{}))

	peer := fftypes.JSONObject{"id": "peer1"}
	sender := fftypes.JSONObject{"id": "sender1"}
	err := h.TransferBlob(context.Background(), "ns1:"+fftypes.NewUUID().String(), peer, sender, "ns1/id1")
	assert.Regexp(t, "FF10229", err)
}

func TestBadEvents(t *testing.T) {

	h, toServer, fromServer, _, done := newTestFFDX(t, false)
	defer done()
	h.AddNode(context.Background(), "ns1", "node1", fftypes.JSONObject{"id": "peer2"})

	err := h.Start()
	assert.NoError(t, err)

	fromServer <- `!}`         // ignored without ack
	fromServer <- `{"id":"0"}` // ignored with ack
	msg := <-toServer
	assert.Equal(t, `{"action":"ack","id":"0"}`, string(msg))

	namespacedID := fmt.Sprintf("ns2:%s", fftypes.NewUUID())
	fromServer <- `{"id":"1","type":"message-failed","requestID":"` + namespacedID + `"}`
	msg = <-toServer
	assert.Equal(t, `{"action":"ack","id":"1"}`, string(msg))

	fromServer <- `{"id":"2","type":"message-received","sender":"peer1","message":"bad!"}`
	msg = <-toServer
	assert.Equal(t, `{"action":"ack","id":"2"}`, string(msg))

	fromServer <- `{"id":"3","type":"message-received","sender":"peer1","message":"{}"}`
	msg = <-toServer
	assert.Equal(t, `{"action":"ack","id":"3"}`, string(msg))

	fromServer <- `{"id":"4","type":"message-received","sender":"peer1","message":"{\"batch\":{\"namespace\":\"ns1\"}}"}`
	msg = <-toServer
	assert.Equal(t, `{"action":"ack","id":"4"}`, string(msg))

	fromServer <- `{"id":"5","type":"message-received","sender":"peer1","recipient":"peer2","message":"{\"batch\":{\"namespace\":\"ns1\"}}"}`
	msg = <-toServer
	assert.Equal(t, `{"action":"ack","id":"5"}`, string(msg))

}

func TestBackgroundStartWSFail(t *testing.T) {
	h := &FFDX{initialized: true}
	coreconfig.Reset()

	u, _ := url.Parse("http://localhost:12345")
	u.Scheme = "http"
	httpURL := u.String()

	h.InitConfig(utConfig)

	utConfig.Set(ffresty.HTTPConfigURL, httpURL)
	utConfig.Set(wsclient.WSConfigKeyInitialConnectAttempts, 1)
	utConfig.Set(DataExchangeBackgroundStart, true)
	h.InitConfig(utConfig)

	dxCtx, dxCancel := context.WithCancel(context.Background())
	defer dxCancel()
	err := h.Init(dxCtx, dxCancel, utConfig, nil)
	assert.NoError(t, err)
	assert.Equal(t, "ffdx", h.Name())
	assert.NotNil(t, h.Capabilities())

	capturedErr := make(chan error)
	h.backgroundRetry = &retry.Retry{
		ErrCallback: func(err error) {
			capturedErr <- err
		},
	}

	err = h.Start()
	assert.NoError(t, err)

	err = <-capturedErr
	assert.Regexp(t, "FF00148", err)
}

func TestMessageEventsBackgroundStart(t *testing.T) {

	h, toServer, fromServer, _, done := newTestFFDX(t, false)
	defer done()

	// Starting in background mode and making sure the event loop are started as well
	// to listen to messages
	utConfig.Set(DataExchangeBackgroundStart, true)
	h.Init(h.ctx, h.cancelCtx, utConfig, nil)

	mcb := &dataexchangemocks.Callbacks{}
	h.SetHandler("ns1", "node1", mcb)
	ocb := &coremocks.OperationCallbacks{}
	h.SetOperationHandler("ns1", ocb)
	h.AddNode(context.Background(), "ns1", "node1", fftypes.JSONObject{"id": "peer1"})

	mcb.On("DXConnect", h).Return(nil)

	err := h.Start()
	assert.NoError(t, err)

	namespacedID1 := fmt.Sprintf("ns1:%s", fftypes.NewUUID())
	ocb.On("OperationUpdate", mock.MatchedBy(func(ev *core.OperationUpdateAsync) bool {
		return ev.NamespacedOpID == namespacedID1 &&
			ev.Status == core.OpStatusFailed &&
			ev.ErrorMessage == "pop" &&
			ev.Plugin == "ffdx"
	})).Run(opAcker()).Return(nil)
	fromServer <- `{"id":"1","type":"message-failed","requestID":"` + namespacedID1 + `","error":"pop"}`
	msg := <-toServer
	assert.Equal(t, `{"action":"ack","id":"1"}`, string(msg))

	namespacedID2 := fmt.Sprintf("ns1:%s", fftypes.NewUUID())
	ocb.On("OperationUpdate", mock.MatchedBy(func(ev *core.OperationUpdateAsync) bool {
		return ev.NamespacedOpID == namespacedID2 &&
			ev.Status == core.OpStatusSucceeded &&
			ev.Plugin == "ffdx"
	})).Run(opAcker()).Return(nil)
	fromServer <- `{"id":"2","type":"message-delivered","requestID":"` + namespacedID2 + `"}`
	msg = <-toServer
	assert.Equal(t, `{"action":"ack","id":"2"}`, string(msg))

	namespacedID3 := fmt.Sprintf("ns1:%s", fftypes.NewUUID())
	ocb.On("OperationUpdate", mock.MatchedBy(func(ev *core.OperationUpdateAsync) bool {
		return ev.NamespacedOpID == namespacedID3 &&
			ev.Status == core.OpStatusSucceeded &&
			ev.DXManifest == `{"manifest":true}` &&
			ev.Output.String() == `{"signatures":"and stuff"}` &&
			ev.Plugin == "ffdx"
	})).Run(opAcker()).Return(nil)
	fromServer <- `{"id":"3","type":"message-acknowledged","requestID":"` + namespacedID3 + `","info":{"signatures":"and stuff"},"manifest":"{\"manifest\":true}"}`
	msg = <-toServer
	assert.Equal(t, `{"action":"ack","id":"3"}`, string(msg))

	mcb.On("DXEvent", h, mock.MatchedBy(func(ev dataexchange.DXEvent) bool {
		return ev.EventID() == "4" &&
			ev.Type() == dataexchange.DXEventTypeMessageReceived &&
			ev.MessageReceived().PeerID == "peer2"
	})).Run(manifestAcker(`{"manifest":true}`)).Return(nil)
	fromServer <- `{"id":"4","type":"message-received","sender":"peer2","recipient":"peer1","message":"{\"batch\":{\"namespace\":\"ns1\"}}"}`
	msg = <-toServer
	assert.Equal(t, `{"action":"ack","id":"4","manifest":"{\"manifest\":true}"}`, string(msg))

	mcb.AssertExpectations(t)
	ocb.AssertExpectations(t)
}

func TestMessageEvents(t *testing.T) {

	h, toServer, fromServer, _, done := newTestFFDX(t, false)
	defer done()

	mcb := &dataexchangemocks.Callbacks{}
	h.SetHandler("ns1", "node1", mcb)
	ocb := &coremocks.OperationCallbacks{}
	h.SetOperationHandler("ns1", ocb)
	h.AddNode(context.Background(), "ns1", "node1", fftypes.JSONObject{"id": "peer1"})
	mcb.On("DXConnect", h).Return(nil)

	err := h.Start()
	assert.NoError(t, err)

	namespacedID1 := fmt.Sprintf("ns1:%s", fftypes.NewUUID())
	ocb.On("OperationUpdate", mock.MatchedBy(func(ev *core.OperationUpdateAsync) bool {
		return ev.NamespacedOpID == namespacedID1 &&
			ev.Status == core.OpStatusFailed &&
			ev.ErrorMessage == "pop" &&
			ev.Plugin == "ffdx"
	})).Run(opAcker()).Return(nil)
	fromServer <- `{"id":"1","type":"message-failed","requestID":"` + namespacedID1 + `","error":"pop"}`
	msg := <-toServer
	assert.Equal(t, `{"action":"ack","id":"1"}`, string(msg))

	namespacedID2 := fmt.Sprintf("ns1:%s", fftypes.NewUUID())
	ocb.On("OperationUpdate", mock.MatchedBy(func(ev *core.OperationUpdateAsync) bool {
		return ev.NamespacedOpID == namespacedID2 &&
			ev.Status == core.OpStatusSucceeded &&
			ev.Plugin == "ffdx"
	})).Run(opAcker()).Return(nil)
	fromServer <- `{"id":"2","type":"message-delivered","requestID":"` + namespacedID2 + `"}`
	msg = <-toServer
	assert.Equal(t, `{"action":"ack","id":"2"}`, string(msg))

	namespacedID3 := fmt.Sprintf("ns1:%s", fftypes.NewUUID())
	ocb.On("OperationUpdate", mock.MatchedBy(func(ev *core.OperationUpdateAsync) bool {
		return ev.NamespacedOpID == namespacedID3 &&
			ev.Status == core.OpStatusSucceeded &&
			ev.DXManifest == `{"manifest":true}` &&
			ev.Output.String() == `{"signatures":"and stuff"}` &&
			ev.Plugin == "ffdx"
	})).Run(opAcker()).Return(nil)
	fromServer <- `{"id":"3","type":"message-acknowledged","requestID":"` + namespacedID3 + `","info":{"signatures":"and stuff"},"manifest":"{\"manifest\":true}"}`
	msg = <-toServer
	assert.Equal(t, `{"action":"ack","id":"3"}`, string(msg))

	mcb.On("DXEvent", h, mock.MatchedBy(func(ev dataexchange.DXEvent) bool {
		return ev.EventID() == "4" &&
			ev.Type() == dataexchange.DXEventTypeMessageReceived &&
			ev.MessageReceived().PeerID == "peer2"
	})).Run(manifestAcker(`{"manifest":true}`)).Return(nil)
	fromServer <- `{"id":"4","type":"message-received","sender":"peer2","recipient":"peer1","message":"{\"batch\":{\"namespace\":\"ns1\"}}"}`
	msg = <-toServer
	assert.Equal(t, `{"action":"ack","id":"4","manifest":"{\"manifest\":true}"}`, string(msg))

	h.SetHandler("ns1", "node1", nil)
	assert.Empty(t, h.callbacks.handlers)
	h.SetOperationHandler("ns1", nil)
	assert.Empty(t, h.callbacks.opHandlers)

	mcb.AssertExpectations(t)
	ocb.AssertExpectations(t)
}

func TestBlobEvents(t *testing.T) {

	h, toServer, fromServer, _, done := newTestFFDX(t, false)
	defer done()

	mcb := &dataexchangemocks.Callbacks{}
	h.SetHandler("ns1", "node1", mcb)
	ocb := &coremocks.OperationCallbacks{}
	h.SetOperationHandler("ns1", ocb)
	h.AddNode(context.Background(), "ns1", "node1", fftypes.JSONObject{"id": "peer1"})
	mcb.On("DXConnect", h).Return(nil)

	err := h.Start()
	assert.NoError(t, err)

	namespacedID5 := fmt.Sprintf("ns1:%s", fftypes.NewUUID())
	ocb.On("OperationUpdate", mock.MatchedBy(func(ev *core.OperationUpdateAsync) bool {
		return ev.NamespacedOpID == namespacedID5 &&
			ev.Status == core.OpStatusFailed &&
			ev.ErrorMessage == "pop" &&
			ev.Plugin == "ffdx"
	})).Run(opAcker()).Return(nil)
	fromServer <- `{"id":"5","type":"blob-failed","requestID":"` + namespacedID5 + `","error":"pop"}`
	msg := <-toServer
	assert.Equal(t, `{"action":"ack","id":"5"}`, string(msg))

	namespacedID6 := fmt.Sprintf("ns1:%s", fftypes.NewUUID())
	ocb.On("OperationUpdate", mock.MatchedBy(func(ev *core.OperationUpdateAsync) bool {
		return ev.NamespacedOpID == namespacedID6 &&
			ev.Status == core.OpStatusSucceeded &&
			ev.Output.String() == `{"some":"details"}` &&
			ev.Plugin == "ffdx"
	})).Run(opAcker()).Return(nil)
	fromServer <- `{"id":"6","type":"blob-delivered","requestID":"` + namespacedID6 + `","info":{"some":"details"}}`
	msg = <-toServer
	assert.Equal(t, `{"action":"ack","id":"6"}`, string(msg))

	u := fftypes.NewUUID()
	hash := fftypes.NewRandB32()
	mcb.On("DXEvent", h, mock.MatchedBy(func(ev dataexchange.DXEvent) bool {
		return ev.EventID() == "9" &&
			ev.Type() == dataexchange.DXEventTypePrivateBlobReceived &&
			ev.PrivateBlobReceived().Hash.Equals(hash)
	})).Run(acker()).Return(nil)
	fromServer <- fmt.Sprintf(`{"id":"9","type":"blob-received","sender":"peer2","recipient":"peer1","path":"ns1/%s","hash":"%s","size":12345}`, u.String(), hash.String())
	msg = <-toServer
	assert.Equal(t, `{"action":"ack","id":"9"}`, string(msg))

	namespacedID10 := fmt.Sprintf("ns1:%s", fftypes.NewUUID())
	ocb.On("OperationUpdate", mock.MatchedBy(func(ev *core.OperationUpdateAsync) bool {
		return ev.NamespacedOpID == namespacedID10 &&
			ev.Status == core.OpStatusSucceeded &&
			ev.Output.String() == `{"signatures":"and stuff"}` &&
			ev.Plugin == "ffdx"
	})).Run(opAcker()).Return(nil)
	fromServer <- `{"id":"10","type":"blob-acknowledged","requestID":"` + namespacedID10 + `","info":{"signatures":"and stuff"},"manifest":"{\"manifest\":true}"}`
	msg = <-toServer
	assert.Equal(t, `{"action":"ack","id":"10"}`, string(msg))

	mcb.AssertExpectations(t)
	ocb.AssertExpectations(t)
}

func TestEventsWithManifest(t *testing.T) {

	h, toServer, fromServer, _, done := newTestFFDX(t, true)
	defer done()

	mcb := &dataexchangemocks.Callbacks{}
	mcb.On("DXConnect", h).Return(nil)
	h.SetHandler("ns1", "node1", mcb)
	ocb := &coremocks.OperationCallbacks{}
	h.SetOperationHandler("ns1", ocb)

	err := h.Start()
	assert.NoError(t, err)

	fromServer <- `!}`         // ignored without ack
	fromServer <- `{"id":"0"}` // ignored with ack
	msg := <-toServer
	assert.Equal(t, `{"action":"ack","id":"0"}`, string(msg))

	namespacedID1 := fmt.Sprintf("ns1:%s", fftypes.NewUUID())
	ocb.On("OperationUpdate", mock.MatchedBy(func(ev *core.OperationUpdateAsync) bool {
		return ev.NamespacedOpID == namespacedID1 &&
			ev.Status == core.OpStatusPending &&
			ev.Plugin == "ffdx"
	})).Run(opAcker()).Return(nil)
	fromServer <- `{"id":"1","type":"message-delivered","requestID":"` + namespacedID1 + `"}`
	msg = <-toServer
	assert.Equal(t, `{"action":"ack","id":"1"}`, string(msg))

	namespacedID2 := fmt.Sprintf("ns1:%s", fftypes.NewUUID())
	ocb.On("OperationUpdate", mock.MatchedBy(func(ev *core.OperationUpdateAsync) bool {
		return ev.NamespacedOpID == namespacedID2 &&
			ev.Status == core.OpStatusPending &&
			ev.Plugin == "ffdx"
	})).Run(opAcker()).Return(nil)
	fromServer <- `{"id":"2","type":"blob-delivered","requestID":"` + namespacedID2 + `"}`
	msg = <-toServer
	assert.Equal(t, `{"action":"ack","id":"2"}`, string(msg))

	mcb.AssertExpectations(t)
	ocb.AssertExpectations(t)
}

func TestEventLoopReceiveClosed(t *testing.T) {
	dxc := &dataexchangemocks.Callbacks{}
	wsm := &wsmocks.WSClient{}
	called := false
	h := &FFDX{
		ctx:       context.Background(),
		cancelCtx: func() { called = true },
		callbacks: callbacks{handlers: map[string]dataexchange.Callbacks{"ns1": dxc}},
		wsconn:    wsm,
	}
	r := make(chan []byte)
	close(r)
	wsm.On("Close").Return()
	wsm.On("Receive").Return((<-chan []byte)(r))
	h.eventLoop()
	assert.True(t, called)
}

func TestEventLoopSendClosed(t *testing.T) {
	dxc := &dataexchangemocks.Callbacks{}
	wsm := &wsmocks.WSClient{}
	ctx, cancelCtx := context.WithCancel(context.Background())
	h := &FFDX{
		ctx:        ctx,
		callbacks:  callbacks{handlers: map[string]dataexchange.Callbacks{"ns1": dxc}},
		wsconn:     wsm,
		ackChannel: make(chan *ack, 1),
	}
	h.ackChannel <- &ack{
		eventID: "12345",
	}
	wsm.On("Close").Return()
	wsm.On("Send", mock.Anything, mock.Anything).Return(fmt.Errorf("pop")).Run(func(args mock.Arguments) {
		cancelCtx()
	})
	h.ackLoop() // we're simply looking for it exiting
}

func TestEventLoopClosedContext(t *testing.T) {
	dxc := &dataexchangemocks.Callbacks{}
	wsm := &wsmocks.WSClient{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	h := &FFDX{
		ctx:       ctx,
		callbacks: callbacks{handlers: map[string]dataexchange.Callbacks{"ns1": dxc}},
		wsconn:    wsm,
	}
	r := make(chan []byte, 1)
	r <- []byte(`{}`)
	wsm.On("Close").Return()
	wsm.On("Receive").Return((<-chan []byte)(r))
	wsm.On("Send", mock.Anything, mock.Anything).Return(nil)
	h.eventLoop() // we're simply looking for it exiting
}

func TestWebsocketWithReinit(t *testing.T) {
	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	_, _, wsURL, cancel := wsclient.NewTestWSServer(nil)
	defer cancel()

	u, _ := url.Parse(wsURL)
	u.Scheme = "http"
	httpURL := u.String()
	h := &FFDX{}

	coreconfig.Reset()
	h.InitConfig(utConfig)
	utConfig.Set(ffresty.HTTPConfigURL, httpURL)
	utConfig.Set(ffresty.HTTPCustomClient, mockedClient)
	utConfig.Set(DataExchangeInitEnabled, true)

	count := 0
	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/init", httpURL),
		func(req *http.Request) (*http.Response, error) {
			var reqNodes []fftypes.JSONObject
			err := json.NewDecoder(req.Body).Decode(&reqNodes)
			assert.NoError(t, err)
			assert.Equal(t, 1, len(reqNodes))

			assert.False(t, h.initialized)

			count++
			if count == 1 {
				return httpmock.NewJsonResponse(200, fftypes.JSONObject{
					"status": "notready",
				})
			}
			if count == 2 {
				return nil, fmt.Errorf("pop")
			}
			return httpmock.NewJsonResponse(200, fftypes.JSONObject{
				"status": "ready",
			})
		})

	h.InitConfig(utConfig)
	ctx, cancel := context.WithCancel(context.Background())
	err := h.Init(ctx, cancel, utConfig, nil)
	assert.NoError(t, err)
	h.AddNode(context.Background(), "ns1", "node1", fftypes.JSONObject{})

	err = h.Start()
	assert.NoError(t, err)

	assert.Equal(t, 3, httpmock.GetTotalCallCount())
	assert.True(t, h.initialized)
}

func TestWebsocketWithEmptyNodesInit(t *testing.T) {
	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	_, _, wsURL, cancel := wsclient.NewTestWSServer(nil)
	defer cancel()

	u, _ := url.Parse(wsURL)
	u.Scheme = "http"
	httpURL := u.String()
	h := &FFDX{}

	coreconfig.Reset()
	h.InitConfig(utConfig)
	utConfig.Set(ffresty.HTTPConfigURL, httpURL)
	utConfig.Set(ffresty.HTTPCustomClient, mockedClient)
	utConfig.Set(DataExchangeInitEnabled, true)

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/init", httpURL),
		func(req *http.Request) (*http.Response, error) {
			var reqNodes []fftypes.JSONObject

			// we want to make sure when theres are no peer nodes, an empty list is being
			// passed as the req, not "null"
			err := json.NewDecoder(req.Body).Decode(&reqNodes)
			assert.NoError(t, err)
			assert.Empty(t, reqNodes)
			assert.NotNil(t, reqNodes)

			return httpmock.NewJsonResponse(200, fftypes.JSONObject{
				"status": "ready",
			})
		})

	h.InitConfig(utConfig)
	ctx, cancel := context.WithCancel(context.Background())
	err := h.Init(ctx, cancel, utConfig, nil)
	assert.NoError(t, err)
	dxc := &dataexchangemocks.Callbacks{}
	h.callbacks = callbacks{handlers: map[string]dataexchange.Callbacks{"ns1": dxc}}
	dxc.On("DXConnect", h).Return(nil)

	err = h.Start()
	assert.NoError(t, err)

	assert.Equal(t, 1, httpmock.GetTotalCallCount())
	assert.True(t, h.initialized)
}

func TestDXUninitialized(t *testing.T) {
	h, _, _, _, done := newTestFFDX(t, false)
	defer done()

	h.initialized = false

	peer := fftypes.JSONObject{"id": "peer1"}
	sender := fftypes.JSONObject{"id": "sender1"}
	err := h.TransferBlob(context.Background(), "ns1:"+fftypes.NewUUID().String(), peer, sender, "ns1/id1")
	assert.Regexp(t, "FF10342", err)

	err = h.SendMessage(context.Background(), "ns1:"+fftypes.NewUUID().String(), peer, sender, []byte(`some data`))
	assert.Regexp(t, "FF10342", err)
}

func TestDeleteBlob(t *testing.T) {

	h, _, _, httpURL, done := newTestFFDX(t, false)
	defer done()

	u := fftypes.NewUUID()
	httpmock.RegisterResponder("DELETE", fmt.Sprintf("%s/api/v1/blobs/ns1/%s", httpURL, u),
		httpmock.NewBytesResponder(204, []byte(``)))

	err := h.DeleteBlob(context.Background(), fmt.Sprintf("ns1/%s", u))
	assert.NoError(t, err)
}

func TestDeleteBlobFail(t *testing.T) {

	h, _, _, httpURL, done := newTestFFDX(t, false)
	defer done()

	u := fftypes.NewUUID()
	httpmock.RegisterResponder("DELETE", fmt.Sprintf("%s/api/v1/blobs/ns1/%s", httpURL, u),
		httpmock.NewBytesResponder(500, []byte(`ERROR`)))

	err := h.DeleteBlob(context.Background(), fmt.Sprintf("ns1/%s", u))
	assert.Regexp(t, "FF10229", err)
}

type mockDXCallbacks struct {
	connectCalls int
}

func (m *mockDXCallbacks) DXConnect(plugin dataexchange.Plugin) {
	m.connectCalls++
}

func (m *mockDXCallbacks) DXEvent(plugin dataexchange.Plugin, event dataexchange.DXEvent) error {
	panic("implement me")
}

func TestWebsocketDXConnect(t *testing.T) {
	mockedClient := &http.Client{}
	httpmock.ActivateNonDefault(mockedClient)
	defer httpmock.DeactivateAndReset()

	_, _, wsURL, cancel := wsclient.NewTestWSServer(nil)
	defer cancel()

	u, _ := url.Parse(wsURL)
	u.Scheme = "http"
	httpURL := u.String()
	h := &FFDX{}

	coreconfig.Reset()
	h.InitConfig(utConfig)
	utConfig.Set(ffresty.HTTPConfigURL, httpURL)
	utConfig.Set(ffresty.HTTPCustomClient, mockedClient)
	utConfig.Set(DataExchangeInitEnabled, true)

	httpmock.RegisterResponder("POST", fmt.Sprintf("%s/api/v1/init", httpURL),
		func(req *http.Request) (*http.Response, error) {
			var reqNodes []fftypes.JSONObject

			// we want to make sure when theres are no peer nodes, an empty list is being
			// passed as the req, not "null"
			err := json.NewDecoder(req.Body).Decode(&reqNodes)
			assert.NoError(t, err)
			assert.Empty(t, reqNodes)
			assert.NotNil(t, reqNodes)

			return httpmock.NewJsonResponse(200, fftypes.JSONObject{
				"status": "ready",
			})
		})

	h.InitConfig(utConfig)
	ctx, cancel := context.WithCancel(context.Background())
	err := h.Init(ctx, cancel, utConfig, nil)
	assert.NoError(t, err)
	dxc := &mockDXCallbacks{
		connectCalls: 0,
	}
	h.callbacks = callbacks{handlers: map[string]dataexchange.Callbacks{"ns1": dxc}}

	err = h.Start()
	assert.NoError(t, err)

	assert.Equal(t, 1, httpmock.GetTotalCallCount())
	assert.True(t, h.initialized)
	assert.Equal(t, 1, dxc.connectCalls)
}

func TestExtractSoonestExpiryFromCertBundleEmpty(t *testing.T) {
	_, err := extractSoonestExpiryFromCertBundle("")
	assert.ErrorContains(t, err, "no valid certificate found")

}

func TestExtractSoonestExpiryFromCertBundleBadBundle(t *testing.T) {
	nonCertPEMBundle := `
-----BEGIN NON-CERTIFICATE-----
MIIDXTCCAkWgAwIBAgIJALa6+u2k5u2kMA0GCSqGSIb3DQEBCwUAMEUxCzAJBgNV
BAYTAkFVMRMwEQYDVQQIDApxdWVlbnNsYW5kMREwDwYDVQQHDAhCcm9va2ZpZWxk
-----END NON-CERTIFICATE-----
`

	_, err := extractSoonestExpiryFromCertBundle(nonCertPEMBundle)
	assert.ErrorContains(t, err, "failed to parse non-certificate within bundle")
}

func TestCheckNodeIdentityStatusNodeNil(t *testing.T) {
	mmm := metricsmocks.NewManager(t)
	h := &FFDX{metrics: mmm}
	err := h.CheckNodeIdentityStatus(context.Background(), nil)
	assert.Error(t, err)
}

func TestCheckNodeIdentityStatusReturnsNilWhenCertIsEmpty(t *testing.T) {
	h, _, _, httpURL, done := newTestFFDX(t, false)
	defer done()

	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/v1/id", httpURL),
		httpmock.NewJsonResponderOrPanic(200, fftypes.JSONObject{
			"id":       "peer1",
			"endpoint": "https://peer1.example.com",
			"cert":     "",
		}))

	node := &core.Identity{
		IdentityBase: core.IdentityBase{
			Namespace: "ns1",
		},
	}

	h.metrics.(*metricsmocks.Manager).On("IsMetricsEnabled").Return(true)
	h.metrics.(*metricsmocks.Manager).On("NodeIdentityDXCertMismatch", "ns1", metrics.NodeIdentityDXCertMismatchStatusUnknown).Return(true)

	err := h.CheckNodeIdentityStatus(context.Background(), node)
	assert.NoError(t, err)
}

func TestCheckNodeIdentityStatusReturnsErrorWhenNodeProfileIsNil(t *testing.T) {
	h, _, _, httpURL, done := newTestFFDX(t, false)
	defer done()

	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/v1/id", httpURL),
		httpmock.NewJsonResponderOrPanic(200, fftypes.JSONObject{
			"id":       "peer1",
			"endpoint": "https://peer1.example.com",
			"cert":     "a-cert",
		}))

	node := &core.Identity{
		IdentityBase: core.IdentityBase{
			Namespace: "ns1",
		},
	}
	h.metrics.(*metricsmocks.Manager).On("IsMetricsEnabled").Return(false)

	err := h.CheckNodeIdentityStatus(context.Background(), node)
	assert.Error(t, err)
}

func TestCheckNodeIdentityStatusEndpointInfoFails(t *testing.T) {
	h, _, _, httpURL, done := newTestFFDX(t, false)
	defer done()

	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/v1/id", httpURL),
		httpmock.NewErrorResponder(errors.New("failed to get peer info")))

	h.metrics.(*metricsmocks.Manager).On("IsMetricsEnabled").Return(true)
	h.metrics.(*metricsmocks.Manager).On("NodeIdentityDXCertMismatch", "ns1", metrics.NodeIdentityDXCertMismatchStatusUnknown).Return(true)

	node := &core.Identity{
		IdentityBase: core.IdentityBase{
			Namespace: "ns1",
		},
	}
	err := h.CheckNodeIdentityStatus(context.Background(), node)
	assert.Error(t, err)
}

func TestCheckNodeIdentityStatusSetsMismatchStateWhenCertsDiffer(t *testing.T) {
	h, _, _, httpURL, done := newTestFFDX(t, false)
	defer done()

	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/v1/id", httpURL),
		httpmock.NewJsonResponderOrPanic(200, fftypes.JSONObject{
			"id":       "peer1",
			"endpoint": "https://peer1.example.com",
			"cert":     "a-cert",
		}))

	node := &core.Identity{
		IdentityBase: core.IdentityBase{
			Namespace: "ns1",
		},
		IdentityProfile: core.IdentityProfile{
			Profile: fftypes.JSONObject{"cert": "b-cert"},
		},
	}

	h.metrics.(*metricsmocks.Manager).On("IsMetricsEnabled").Return(true)
	h.metrics.(*metricsmocks.Manager).On("NodeIdentityDXCertMismatch", "ns1", metrics.NodeIdentityDXCertMismatchStatusMismatched).Return(true)
	err := h.CheckNodeIdentityStatus(context.Background(), node)
	assert.NoError(t, err)
}

func TestCheckNodeIdentityStatusSetsHealthyStateWhenCertsMatch(t *testing.T) {
	h, _, _, httpURL, done := newTestFFDX(t, false)
	defer done()

	jsonFriendlyCert := strings.ReplaceAll(testCertBundle, "\n", `\n`)

	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/v1/id", httpURL),
		httpmock.NewJsonResponderOrPanic(200, fftypes.JSONObject{
			"id":       "peer1",
			"endpoint": "https://peer1.example.com",
			"cert":     jsonFriendlyCert,
		}))

	node := &core.Identity{
		IdentityBase: core.IdentityBase{
			Namespace: "ns1",
		},
		IdentityProfile: core.IdentityProfile{
			Profile: fftypes.JSONObject{"cert": jsonFriendlyCert},
		},
	}

	h.metrics.(*metricsmocks.Manager).On("IsMetricsEnabled").Return(true)
	h.metrics.(*metricsmocks.Manager).On("NodeIdentityDXCertMismatch", "ns1", metrics.NodeIdentityDXCertMismatchStatusHealthy).Return(true)
	expiry := time.Unix(1835379057, 0).UTC()
	h.metrics.(*metricsmocks.Manager).On("NodeIdentityDXCertExpiry", "ns1", expiry).Return(true)

	err := h.CheckNodeIdentityStatus(context.Background(), node)
	assert.NoError(t, err)
}

func TestCheckNodeIdentityStatusSetsHealthyStateWhenCertsExpire(t *testing.T) {
	h, _, _, httpURL, done := newTestFFDX(t, false)
	defer done()

	jsonFriendlyCert := strings.ReplaceAll(testExpiredCert, "\n", `\n`)

	httpmock.RegisterResponder("GET", fmt.Sprintf("%s/api/v1/id", httpURL),
		httpmock.NewJsonResponderOrPanic(200, fftypes.JSONObject{
			"id":       "peer1",
			"endpoint": "https://peer1.example.com",
			"cert":     jsonFriendlyCert,
		}))
	node := &core.Identity{
		IdentityBase: core.IdentityBase{
			Namespace: "ns1",
		},
		IdentityProfile: core.IdentityProfile{
			Profile: fftypes.JSONObject{"cert": jsonFriendlyCert},
		},
	}

	h.metrics.(*metricsmocks.Manager).On("IsMetricsEnabled").Return(true)
	h.metrics.(*metricsmocks.Manager).On("NodeIdentityDXCertMismatch", "ns1", metrics.NodeIdentityDXCertMismatchStatusHealthy).Return(true)
	expiry := time.Unix(1740924845, 0).UTC()
	h.metrics.(*metricsmocks.Manager).On("NodeIdentityDXCertExpiry", "ns1", expiry).Return(true)

	err := h.CheckNodeIdentityStatus(context.Background(), node)
	assert.NoError(t, err)
}
