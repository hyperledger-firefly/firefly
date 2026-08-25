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

package webhooks

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/mux"
	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/ffresty"
	"github.com/hyperledger-firefly/common/pkg/fftls"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	fflog "github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/mocks/eventsmocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/events"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func newTestWebHooks(t *testing.T) (wh *WebHooks, cancel func()) {
	coreconfig.Reset()

	cbs := &eventsmocks.Callbacks{}
	rc := cbs.On("RegisterConnection", mock.Anything, mock.Anything).Return(nil)
	rc.RunFn = func(a mock.Arguments) {
		assert.Equal(t, true, a[1].(events.SubscriptionMatcher)(core.SubscriptionRef{}))
	}
	wh = &WebHooks{}
	ctx, cancelCtx := context.WithCancel(context.Background())
	svrConfig := config.RootSection("ut.webhooks")
	wh.InitConfig(svrConfig)
	wh.Init(ctx, svrConfig)
	wh.SetHandler("ns1", cbs)
	assert.Equal(t, "webhooks", wh.Name())
	assert.NotNil(t, wh.Capabilities())
	return wh, cancelCtx
}

func TestInitBadTLS(t *testing.T) {
	coreconfig.Reset()

	wh := &WebHooks{}
	ctx := context.Background()
	svrConfig := config.RootSection("ut.webhooks")
	wh.InitConfig(svrConfig)
	tlsConfig := svrConfig.SubSection("tls")
	tlsConfig.Set(fftls.HTTPConfTLSEnabled, true)
	tlsConfig.Set(fftls.HTTPConfTLSCAFile, "BADCA")
	err := wh.Init(ctx, svrConfig)
	assert.Regexp(t, "FF00153", err)
}

func TestValidateOptionsWithDataFalse(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	no := false
	opts := &core.SubscriptionOptions{
		SubscriptionCoreOptions: core.SubscriptionCoreOptions{
			WithData: &no,
		},
	}
	opts.TransportOptions()["url"] = "/anything"
	err := wh.ValidateOptions(wh.ctx, opts)
	assert.NoError(t, err)
	assert.False(t, *opts.WithData)
}

func TestValidateOptionsWithDataDefaulTrue(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	opts := &core.SubscriptionOptions{}
	opts.TransportOptions()["url"] = "/anything"
	err := wh.ValidateOptions(wh.ctx, opts)
	assert.NoError(t, err)
	assert.True(t, *opts.WithData)

	wh.SetHandler("ns1", nil)
	assert.Empty(t, wh.callbacks.handlers)
}

func TestValidateOptionsBadURL(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	opts := &core.SubscriptionOptions{}
	opts.TransportOptions()
	err := wh.ValidateOptions(wh.ctx, opts)
	assert.Regexp(t, "FF10242", err)
}

func TestValidateOptionsBadHeaders(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	opts := &core.SubscriptionOptions{}
	opts.TransportOptions()
	opts.TransportOptions()["url"] = "/anything"
	opts.TransportOptions()["headers"] = fftypes.JSONObject{
		"bad": map[bool]bool{false: true},
	}
	err := wh.ValidateOptions(wh.ctx, opts)
	assert.Regexp(t, "FF10243.*headers", err)
}

func TestValidateOptionsBadQuery(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	opts := &core.SubscriptionOptions{}
	opts.TransportOptions()
	opts.TransportOptions()["url"] = "/anything"
	opts.TransportOptions()["query"] = fftypes.JSONObject{
		"bad": map[bool]bool{false: true},
	}
	err := wh.ValidateOptions(wh.ctx, opts)
	assert.Regexp(t, "FF10243.*query", err)
}

func TestValidateOptionsBadInitialDelayDuration(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	opts := &core.SubscriptionOptions{}
	opts.Retry = core.WebhookRetryOptions{
		Enabled:      true,
		InitialDelay: "badinitialdelay",
	}
	err := wh.ValidateOptions(wh.ctx, opts)
	assert.Regexp(t, "FF00137", err)
}

func TestValidateOptionsBadMaxDelayDuration(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	opts := &core.SubscriptionOptions{}
	opts.Retry = core.WebhookRetryOptions{
		Enabled:      true,
		MaximumDelay: "badmaxdelay",
	}
	err := wh.ValidateOptions(wh.ctx, opts)
	assert.Regexp(t, "FF00137", err)
}

func TestValidateOptionsBadHTTPRequestTimeout(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	opts := &core.SubscriptionOptions{}

	opts.HTTPOptions = core.WebhookHTTPOptions{
		HTTPRequestTimeout: "badrequestimeout",
	}
	err := wh.ValidateOptions(wh.ctx, opts)
	assert.Regexp(t, "FF00137", err)
}

func TestValidateOptionsBadHTTPTLSHandshakeTimeout(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	opts := &core.SubscriptionOptions{}

	opts.HTTPOptions = core.WebhookHTTPOptions{
		HTTPTLSHandshakeTimeout: "badtimeout",
	}
	err := wh.ValidateOptions(wh.ctx, opts)
	assert.Regexp(t, "FF00137", err)
}
func TestValidateOptionsBadHTTPIdleConnTimeout(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	opts := &core.SubscriptionOptions{}

	opts.HTTPOptions = core.WebhookHTTPOptions{
		HTTPIdleConnTimeout: "badtimeout",
	}
	err := wh.ValidateOptions(wh.ctx, opts)
	assert.Regexp(t, "FF00137", err)
}
func TestValidateOptionsBadHTTPConnectionTimeout(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	opts := &core.SubscriptionOptions{}

	opts.HTTPOptions = core.WebhookHTTPOptions{
		HTTPConnectionTimeout: "badtimeout",
	}
	err := wh.ValidateOptions(wh.ctx, opts)
	assert.Regexp(t, "FF00137", err)
}
func TestValidateOptionsBadHTTPExpectedContinueTimeout(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	opts := &core.SubscriptionOptions{}

	opts.HTTPOptions = core.WebhookHTTPOptions{
		HTTPExpectContinueTimeout: "badtimeout",
	}
	err := wh.ValidateOptions(wh.ctx, opts)
	assert.Regexp(t, "FF00137", err)
}

func TestValidateOptionsExtraFields(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	wh.ffrestyConfig = &ffresty.Config{
		URL: "test-url",
	}

	opts := &core.SubscriptionOptions{}
	opts.TransportOptions()["url"] = "/anything"

	opts.Retry = core.WebhookRetryOptions{
		Enabled:      true,
		Count:        2,
		InitialDelay: "1s",
		MaximumDelay: "2s",
	}

	proxyURL := "http://myproxy.example.com:8888"
	opts.HTTPOptions = core.WebhookHTTPOptions{
		HTTPMaxIdleConns:          2,
		HTTPTLSHandshakeTimeout:   "2s",
		HTTPRequestTimeout:        "2s",
		HTTPIdleConnTimeout:       "2s",
		HTTPConnectionTimeout:     "2s",
		HTTPExpectContinueTimeout: "2s",
		HTTPProxyURL:              &proxyURL,
	}

	opts.TLSConfig = &tls.Config{}

	err := wh.ValidateOptions(wh.ctx, opts)
	assert.NoError(t, err)

	assert.Equal(t, opts.RestyClient.RetryCount, 2)
	assert.Equal(t, opts.RestyClient.RetryMaxWaitTime, 2*time.Second)
	assert.Equal(t, opts.RestyClient.RetryWaitTime, 1*time.Second)

	expectedDuration := 2 * time.Second
	assert.Equal(t, opts.RestyClient.GetClient().Timeout, expectedDuration)

	transport, ok := opts.RestyClient.GetClient().Transport.(*http.Transport)
	assert.True(t, ok)
	assert.Equal(t, transport.IdleConnTimeout, expectedDuration)
	assert.Equal(t, transport.ExpectContinueTimeout, expectedDuration)
	assert.Equal(t, transport.TLSHandshakeTimeout, expectedDuration)
	assert.Equal(t, transport.MaxIdleConns, 2)
	assert.NotNil(t, transport.TLSClientConfig)

	req := httptest.NewRequest(http.MethodGet, "http://testany.example.com", nil)
	u, err := transport.Proxy(req)
	assert.NoError(t, err)
	assert.Equal(t, "http://myproxy.example.com:8888", u.String())
}

func TestRequestWithBodyReplyEndToEnd(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	r := mux.NewRouter()
	r.HandleFunc("/myapi/my/sub/path?escape_query", func(res http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "myheaderval", req.Header.Get("My-Header"))
		assert.Equal(t, "dynamicheaderval", req.Header.Get("Dynamic-Header"))
		assert.Equal(t, "myqueryval", req.URL.Query().Get("my-query"))
		assert.Equal(t, "dynamicqueryval", req.URL.Query().Get("dynamic-query"))
		var body fftypes.JSONObject
		err := json.NewDecoder(req.Body).Decode(&body)
		assert.NoError(t, err)
		assert.Equal(t, "inputvalue", body.GetString("inputfield"))
		res.Header().Set("my-reply-header", "myheaderval2")
		res.WriteHeader(200)
		res.Write([]byte(`{
			"replyfield": "replyvalue"
		}`))
	}).Methods(http.MethodPut)
	server := httptest.NewServer(r)
	defer server.Close()

	yes := true
	dataID := fftypes.NewUUID()
	msgID := fftypes.NewUUID()
	groupHash := fftypes.NewRandB32()
	sub := &core.Subscription{
		SubscriptionRef: core.SubscriptionRef{
			Namespace: "ns1",
		},
		Options: core.SubscriptionOptions{
			SubscriptionCoreOptions: core.SubscriptionCoreOptions{
				WithData: &yes,
			},
		},
	}
	to := sub.Options.TransportOptions()
	to["reply"] = true
	to["json"] = true
	to["method"] = "PUT"
	to["url"] = fmt.Sprintf("http://%s/myapi/", server.Listener.Addr())
	to["headers"] = map[string]interface{}{
		"my-header": "myheaderval",
	}
	to["query"] = map[string]interface{}{
		"my-query": "myqueryval",
	}
	to["input"] = map[string]interface{}{
		"query":   "in_query",
		"headers": "in_headers",
		"body":    "in_body",
		"path":    "in_path",
		"replytx": "in_replytx",
	}
	event := &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID: fftypes.NewUUID(),
			},
			Message: &core.Message{
				Header: core.MessageHeader{
					ID:    msgID,
					Group: groupHash,
					Type:  core.MessageTypePrivate,
				},
				Data: core.DataRefs{
					{ID: dataID},
				},
			},
		},
		Subscription: core.SubscriptionRef{
			ID: sub.ID,
		},
	}
	data := &core.Data{
		ID: dataID,
		Value: fftypes.JSONAnyPtr(`{
			"in_body": {
				"inputfield": "inputvalue"
			},
			"in_query": {
				"dynamic-query": "dynamicqueryval"
			},
			"in_headers": {
				"dynamic-header": "dynamicheaderval"
			},
			"in_path": "/my/sub/path?escape_query",
			"in_replytx": true
		}`),
	}

	mcb := wh.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)
	mcb.On("DeliveryResponse", mock.Anything, mock.MatchedBy(func(response *core.EventDeliveryResponse) bool {
		assert.Equal(t, *msgID, *response.Reply.Message.Header.CID)
		assert.Equal(t, *groupHash, *response.Reply.Message.Header.Group)
		assert.Equal(t, core.MessageTypePrivate, response.Reply.Message.Header.Type)
		assert.Equal(t, core.TransactionTypeBatchPin, response.Reply.Message.Header.TxType)
		assert.Equal(t, "myheaderval2", response.Reply.InlineData[0].Value.JSONObject().GetObject("headers").GetString("My-Reply-Header"))
		assert.Equal(t, "replyvalue", response.Reply.InlineData[0].Value.JSONObject().GetObject("body").GetString("replyfield"))
		assert.Equal(t, float64(200), response.Reply.InlineData[0].Value.JSONObject()["status"])
		return true
	})).Return(nil)

	err := wh.DeliveryRequest(wh.ctx, mock.Anything, sub, event, core.DataArray{data})
	assert.NoError(t, err)

	mcb.AssertExpectations(t)
}

func TestRequestWithBodyReplyEndToEndWithTLS(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	r := mux.NewRouter()
	r.HandleFunc("/myapi/my/sub/path?escape_query", func(res http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "myheaderval", req.Header.Get("My-Header"))
		assert.Equal(t, "dynamicheaderval", req.Header.Get("Dynamic-Header"))
		assert.Equal(t, "myqueryval", req.URL.Query().Get("my-query"))
		assert.Equal(t, "dynamicqueryval", req.URL.Query().Get("dynamic-query"))
		var body fftypes.JSONObject
		err := json.NewDecoder(req.Body).Decode(&body)
		assert.NoError(t, err)
		assert.Equal(t, "inputvalue", body.GetString("inputfield"))
		res.Header().Set("my-reply-header", "myheaderval2")
		res.WriteHeader(200)
		res.Write([]byte(`{
			"replyfield": "replyvalue"
		}`))
	}).Methods(http.MethodPut)

	// Create an X509 certificate pair
	privatekey, _ := rsa.GenerateKey(rand.Reader, 2048)
	publickey := &privatekey.PublicKey
	var privateKeyBytes []byte = x509.MarshalPKCS1PrivateKey(privatekey)
	privateKeyFile, _ := os.CreateTemp("", "key.pem")
	defer os.Remove(privateKeyFile.Name())
	privateKeyBlock := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: privateKeyBytes}
	pem.Encode(privateKeyFile, privateKeyBlock)
	serialNumber, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	x509Template := &x509.Certificate{
		SerialNumber: serialNumber,
		Subject: pkix.Name{
			Organization: []string{"Unit Tests"},
		},
		NotBefore:             time.Now(),
		NotAfter:              time.Now().Add(1000 * time.Second),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IPAddresses:           []net.IP{net.IPv4(127, 0, 0, 1)},
	}
	derBytes, err := x509.CreateCertificate(rand.Reader, x509Template, x509Template, publickey, privatekey)
	assert.NoError(t, err)
	publicKeyFile, _ := os.CreateTemp("", "cert.pem")
	defer os.Remove(publicKeyFile.Name())
	pem.Encode(publicKeyFile, &pem.Block{Type: "CERTIFICATE", Bytes: derBytes})

	caCert, err := os.ReadFile(publicKeyFile.Name())
	if err != nil {
		log.Fatal(err)
	}
	caCertPool := x509.NewCertPool()
	caCertPool.AppendCertsFromPEM(caCert)

	// Create the TLS Config with the CA pool and enable Client certificate validation
	tlsConfig := &tls.Config{
		ClientCAs:  caCertPool,
		ClientAuth: tls.RequireAndVerifyClientCert,
	}
	// Create a Server instance to listen on port 8443 with the TLS config
	server := &http.Server{
		Addr:      "127.0.0.1:8443",
		TLSConfig: tlsConfig,
	}

	ctx, cancelCtx := context.WithCancel(context.Background())
	go func() {
		<-ctx.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownContext)
	}()

	server.Handler = r
	go func() {
		if err := server.ListenAndServeTLS(publicKeyFile.Name(), privateKeyFile.Name()); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServeTLS(): %v", err)
		}
	}()

	// Wait for the server to be ready
	for {
		conn, err := tls.Dial("tcp", server.Addr, &tls.Config{
			InsecureSkipVerify: true,
		})
		if err == nil {
			conn.Close()
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Build a TLS config for the client and set on the subscription object
	cert, err := tls.LoadX509KeyPair(publicKeyFile.Name(), privateKeyFile.Name())
	assert.NoError(t, err)
	clientTLSConfig := &tls.Config{
		RootCAs:      caCertPool,
		Certificates: []tls.Certificate{cert},
	}

	yes := true
	dataID := fftypes.NewUUID()
	msgID := fftypes.NewUUID()
	groupHash := fftypes.NewRandB32()

	client := ffresty.NewWithConfig(ctx, ffresty.Config{
		HTTPConfig: ffresty.HTTPConfig{
			TLSClientConfig: clientTLSConfig,
		},
	})
	sub := &core.Subscription{
		SubscriptionRef: core.SubscriptionRef{
			Namespace: "ns1",
		},
		Options: core.SubscriptionOptions{
			SubscriptionCoreOptions: core.SubscriptionCoreOptions{
				WithData: &yes,
			},
			WebhookSubOptions: core.WebhookSubOptions{
				RestyClient: client,
			},
		},
	}
	to := sub.Options.TransportOptions()
	to["reply"] = true
	to["json"] = true
	to["method"] = "PUT"
	to["url"] = fmt.Sprintf("https://%s/myapi/", server.Addr)
	to["headers"] = map[string]interface{}{
		"my-header": "myheaderval",
	}
	to["query"] = map[string]interface{}{
		"my-query": "myqueryval",
	}
	to["input"] = map[string]interface{}{
		"query":   "in_query",
		"headers": "in_headers",
		"body":    "in_body",
		"path":    "in_path",
		"replytx": "in_replytx",
	}
	event := &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID: fftypes.NewUUID(),
			},
			Message: &core.Message{
				Header: core.MessageHeader{
					ID:    msgID,
					Group: groupHash,
					Type:  core.MessageTypePrivate,
				},
				Data: core.DataRefs{
					{ID: dataID},
				},
			},
		},
		Subscription: core.SubscriptionRef{
			ID: sub.ID,
		},
	}
	data := &core.Data{
		ID: dataID,
		Value: fftypes.JSONAnyPtr(`{
			"in_body": {
				"inputfield": "inputvalue"
			},
			"in_query": {
				"dynamic-query": "dynamicqueryval"
			},
			"in_headers": {
				"dynamic-header": "dynamicheaderval"
			},
			"in_path": "/my/sub/path?escape_query",
			"in_replytx": true
		}`),
	}

	mcb := wh.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)
	mcb.On("DeliveryResponse", mock.Anything, mock.MatchedBy(func(response *core.EventDeliveryResponse) bool {
		assert.Equal(t, *msgID, *response.Reply.Message.Header.CID)
		assert.Equal(t, *groupHash, *response.Reply.Message.Header.Group)
		assert.Equal(t, core.MessageTypePrivate, response.Reply.Message.Header.Type)
		assert.Equal(t, core.TransactionTypeBatchPin, response.Reply.Message.Header.TxType)
		assert.Equal(t, "myheaderval2", response.Reply.InlineData[0].Value.JSONObject().GetObject("headers").GetString("My-Reply-Header"))
		assert.Equal(t, "replyvalue", response.Reply.InlineData[0].Value.JSONObject().GetObject("body").GetString("replyfield"))
		assert.Equal(t, float64(200), response.Reply.InlineData[0].Value.JSONObject()["status"])
		return true
	})).Return(nil)

	err = wh.DeliveryRequest(wh.ctx, mock.Anything, sub, event, core.DataArray{data})
	assert.NoError(t, err)

	mcb.AssertExpectations(t)

	cancelCtx()

}

func TestRequestWithEmptyStringBodyReplyEndToEnd(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	r := mux.NewRouter()
	r.HandleFunc("/myapi/my/sub/path?escape_query", func(res http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "myheaderval", req.Header.Get("My-Header"))
		assert.Equal(t, "dynamicheaderval", req.Header.Get("Dynamic-Header"))
		assert.Equal(t, "myqueryval", req.URL.Query().Get("my-query"))
		assert.Equal(t, "dynamicqueryval", req.URL.Query().Get("dynamic-query"))
		var body fftypes.JSONObject
		err := json.NewDecoder(req.Body).Decode(&body)
		assert.NoError(t, err)
		assert.Equal(t, "", body.GetString("inputfield"))
		res.Header().Set("my-reply-header", "myheaderval2")
		res.WriteHeader(200)
		res.Write([]byte(`{
			"replyfield": ""
		}`))
	}).Methods(http.MethodPut)
	server := httptest.NewServer(r)
	defer server.Close()

	yes := true
	dataID := fftypes.NewUUID()
	msgID := fftypes.NewUUID()
	groupHash := fftypes.NewRandB32()
	sub := &core.Subscription{
		SubscriptionRef: core.SubscriptionRef{
			Namespace: "ns1",
		},
		Options: core.SubscriptionOptions{
			SubscriptionCoreOptions: core.SubscriptionCoreOptions{
				WithData: &yes,
			},
		},
	}
	to := sub.Options.TransportOptions()
	to["reply"] = true
	to["json"] = true
	to["method"] = "PUT"
	to["url"] = fmt.Sprintf("http://%s/myapi/", server.Listener.Addr())
	to["headers"] = map[string]interface{}{
		"my-header": "myheaderval",
	}
	to["query"] = map[string]interface{}{
		"my-query": "myqueryval",
	}
	to["input"] = map[string]interface{}{
		"query":   "in_query",
		"headers": "in_headers",
		"body":    "in_body",
		"path":    "in_path",
		"replytx": "in_replytx",
	}
	event := &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID: fftypes.NewUUID(),
			},
			Message: &core.Message{
				Header: core.MessageHeader{
					ID:    msgID,
					Group: groupHash,
					Type:  core.MessageTypePrivate,
				},
				Data: core.DataRefs{
					{ID: dataID},
				},
			},
		},
		Subscription: core.SubscriptionRef{
			ID: sub.ID,
		},
	}
	data := &core.Data{
		ID: dataID,
		Value: fftypes.JSONAnyPtr(`{
			"in_body": {
				"inputfield": ""
			},
			"in_query": {
				"dynamic-query": "dynamicqueryval"
			},
			"in_headers": {
				"dynamic-header": "dynamicheaderval"
			},
			"in_path": "/my/sub/path?escape_query",
			"in_replytx": true
		}`),
	}

	mcb := wh.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)
	mcb.On("DeliveryResponse", mock.Anything, mock.MatchedBy(func(response *core.EventDeliveryResponse) bool {
		assert.Equal(t, *msgID, *response.Reply.Message.Header.CID)
		assert.Equal(t, *groupHash, *response.Reply.Message.Header.Group)
		assert.Equal(t, core.MessageTypePrivate, response.Reply.Message.Header.Type)
		assert.Equal(t, core.TransactionTypeBatchPin, response.Reply.Message.Header.TxType)
		assert.Equal(t, "myheaderval2", response.Reply.InlineData[0].Value.JSONObject().GetObject("headers").GetString("My-Reply-Header"))
		assert.Equal(t, "", response.Reply.InlineData[0].Value.JSONObject().GetObject("body").GetString("replyfield"))
		assert.Equal(t, float64(200), response.Reply.InlineData[0].Value.JSONObject()["status"])
		return true
	})).Return(nil)

	err := wh.DeliveryRequest(wh.ctx, mock.Anything, sub, event, core.DataArray{data})
	assert.NoError(t, err)

	mcb.AssertExpectations(t)
}

func TestRequestNoBodyNoReply(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	msgID := fftypes.NewUUID()

	called := false
	r := mux.NewRouter()
	r.HandleFunc("/myapi", func(res http.ResponseWriter, req *http.Request) {
		var body fftypes.JSONObject
		err := json.NewDecoder(req.Body).Decode(&body)
		assert.NoError(t, err)
		assert.Equal(t, msgID.String(), body.GetObject("message").GetObject("header").GetString("id"))
		res.WriteHeader(200)
		called = true
	}).Methods(http.MethodPost)
	server := httptest.NewServer(r)
	defer server.Close()

	dataID := fftypes.NewUUID()
	groupHash := fftypes.NewRandB32()
	sub := &core.Subscription{
		SubscriptionRef: core.SubscriptionRef{
			Namespace: "ns1",
		},
	}
	to := sub.Options.TransportOptions()
	to["url"] = fmt.Sprintf("http://%s/myapi", server.Listener.Addr())
	event := &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID: fftypes.NewUUID(),
			},
			Message: &core.Message{
				Header: core.MessageHeader{
					ID:    msgID,
					Group: groupHash,
					Type:  core.MessageTypePrivate,
				},
				Data: core.DataRefs{
					{ID: dataID},
				},
			},
		},
		Subscription: core.SubscriptionRef{
			ID:        sub.ID,
			Namespace: "ns1",
		},
	}
	data := &core.Data{
		ID: dataID,
		Value: fftypes.JSONAnyPtr(`{
			"inputfield": "inputvalue"
		}`),
	}

	mcb := wh.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)
	mcb.On("DeliveryResponse", mock.Anything, mock.MatchedBy(func(response *core.EventDeliveryResponse) bool {
		return !response.Rejected
	})).Return(nil)

	err := wh.DeliveryRequest(wh.ctx, mock.Anything, sub, event, core.DataArray{data})
	assert.NoError(t, err)
	assert.True(t, called)

	mcb.AssertExpectations(t)
}

func TestRequestReplyEmptyData(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	msgID := fftypes.NewUUID()

	called := false
	r := mux.NewRouter()
	r.HandleFunc("/myapi", func(res http.ResponseWriter, req *http.Request) {
		var body fftypes.JSONObject
		err := json.NewDecoder(req.Body).Decode(&body)
		assert.NoError(t, err)
		res.WriteHeader(200)
		called = true
	}).Methods(http.MethodPost)
	server := httptest.NewServer(r)
	defer server.Close()

	yes := true
	sub := &core.Subscription{
		SubscriptionRef: core.SubscriptionRef{
			Namespace: "ns1",
		},
		Options: core.SubscriptionOptions{
			SubscriptionCoreOptions: core.SubscriptionCoreOptions{
				WithData: &yes,
			},
		},
	}
	to := sub.Options.TransportOptions()
	to["url"] = fmt.Sprintf("http://%s/myapi", server.Listener.Addr())
	to["reply"] = true
	event := &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID: fftypes.NewUUID(),
			},
			Message: &core.Message{
				Header: core.MessageHeader{
					ID:   msgID,
					Type: core.MessageTypeBroadcast,
				},
			},
		},
		Subscription: core.SubscriptionRef{
			ID: sub.ID,
		},
	}

	mcb := wh.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)
	mcb.On("DeliveryResponse", mock.Anything, mock.MatchedBy(func(response *core.EventDeliveryResponse) bool {
		assert.Equal(t, *msgID, *response.Reply.Message.Header.CID)
		assert.Nil(t, response.Reply.Message.Header.Group)
		assert.Equal(t, core.MessageTypeBroadcast, response.Reply.Message.Header.Type)
		return true
	})).Return(nil)

	err := wh.DeliveryRequest(wh.ctx, mock.Anything, sub, event, core.DataArray{})
	assert.NoError(t, err)
	assert.True(t, called)

	mcb.AssertExpectations(t)
}

func TestRequestReplyOneData(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	msgID := fftypes.NewUUID()
	dataID := fftypes.NewUUID()

	r := mux.NewRouter()
	r.HandleFunc("/myapi", func(res http.ResponseWriter, req *http.Request) {
		var body fftypes.JSONObject
		err := json.NewDecoder(req.Body).Decode(&body)
		assert.NoError(t, err)
		res.WriteHeader(200)
	}).Methods(http.MethodPost)
	server := httptest.NewServer(r)
	defer server.Close()

	yes := true
	sub := &core.Subscription{
		SubscriptionRef: core.SubscriptionRef{
			Namespace: "ns1",
		},
		Options: core.SubscriptionOptions{
			SubscriptionCoreOptions: core.SubscriptionCoreOptions{
				WithData: &yes,
			},
		},
	}
	to := sub.Options.TransportOptions()
	to["url"] = fmt.Sprintf("http://%s/myapi", server.Listener.Addr())
	to["reply"] = true
	event := &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID: fftypes.NewUUID(),
			},
			Message: &core.Message{
				Header: core.MessageHeader{
					ID:   msgID,
					Type: core.MessageTypeBroadcast,
				},
				Data: core.DataRefs{
					{ID: dataID},
				},
			},
		},
		Subscription: core.SubscriptionRef{
			ID: sub.ID,
		},
	}

	mcb := wh.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)
	mcb.On("DeliveryResponse", mock.Anything, mock.MatchedBy(func(response *core.EventDeliveryResponse) bool {
		assert.Equal(t, *msgID, *response.Reply.Message.Header.CID)
		assert.Nil(t, response.Reply.Message.Header.Group)
		assert.Equal(t, core.MessageTypeBroadcast, response.Reply.Message.Header.Type)
		return true
	})).Return(nil)

	err := wh.DeliveryRequest(wh.ctx, mock.Anything, sub, event, core.DataArray{{ID: dataID, Value: fftypes.JSONAnyPtr("foo")}})
	assert.NoError(t, err)

	mcb.AssertExpectations(t)
}

func TestRequestReplyBadJSON(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	msgID := fftypes.NewUUID()

	r := mux.NewRouter()
	r.HandleFunc("/myapi", func(res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(200)
		res.Write([]byte(`!badjson`))
	}).Methods(http.MethodPost)
	server := httptest.NewServer(r)
	defer server.Close()

	sub := &core.Subscription{
		SubscriptionRef: core.SubscriptionRef{
			Namespace: "ns1",
		},
	}
	to := sub.Options.TransportOptions()
	to["url"] = fmt.Sprintf("http://%s/myapi", server.Listener.Addr())
	to["reply"] = true
	to["json"] = true
	event := &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID: fftypes.NewUUID(),
			},
			Message: &core.Message{
				Header: core.MessageHeader{
					ID:   msgID,
					Type: core.MessageTypeBroadcast,
				},
			},
		},
		Subscription: core.SubscriptionRef{
			ID: sub.ID,
		},
	}

	mcb := wh.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)
	mcb.On("DeliveryResponse", mock.Anything, mock.MatchedBy(func(response *core.EventDeliveryResponse) bool {
		assert.Equal(t, float64(502), response.Reply.InlineData[0].Value.JSONObject()["status"])
		assert.Regexp(t, "FF10257", response.Reply.InlineData[0].Value.JSONObject().GetObject("body")["error"])
		return true
	})).Return(nil)

	err := wh.DeliveryRequest(wh.ctx, mock.Anything, sub, event, core.DataArray{})
	assert.NoError(t, err)

	mcb.AssertExpectations(t)
}

func TestRequestReplyDataArrayBadStatusB64(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	msgID := fftypes.NewUUID()

	called := false
	r := mux.NewRouter()
	r.HandleFunc("/myapi", func(res http.ResponseWriter, req *http.Request) {
		var body []string
		err := json.NewDecoder(req.Body).Decode(&body)
		assert.NoError(t, err)
		assert.Len(t, body, 2)
		assert.Equal(t, "value1", body[0])
		assert.Equal(t, "value2", body[1])
		res.WriteHeader(500)
		res.Write([]byte(`some bytes`))
		called = true
	}).Methods(http.MethodPost)
	server := httptest.NewServer(r)
	defer server.Close()

	yes := true
	sub := &core.Subscription{
		SubscriptionRef: core.SubscriptionRef{
			Namespace: "ns1",
		},
		Options: core.SubscriptionOptions{
			SubscriptionCoreOptions: core.SubscriptionCoreOptions{
				WithData: &yes,
			},
		},
	}
	to := sub.Options.TransportOptions()
	to["url"] = fmt.Sprintf("http://%s/myapi", server.Listener.Addr())
	to["reply"] = true
	event := &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID: fftypes.NewUUID(),
			},
			Message: &core.Message{
				Header: core.MessageHeader{
					ID:   msgID,
					Type: core.MessageTypeBroadcast,
				},
			},
		},
		Subscription: core.SubscriptionRef{
			ID: sub.ID,
		},
	}

	mcb := wh.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)
	mcb.On("DeliveryResponse", mock.Anything, mock.MatchedBy(func(response *core.EventDeliveryResponse) bool {
		assert.Equal(t, *msgID, *response.Reply.Message.Header.CID)
		assert.Nil(t, response.Reply.Message.Header.Group)
		assert.Equal(t, core.MessageTypeBroadcast, response.Reply.Message.Header.Type)
		assert.Equal(t, float64(500), response.Reply.InlineData[0].Value.JSONObject()["status"])
		assert.Equal(t, `c29tZSBieXRlcw==`, response.Reply.InlineData[0].Value.JSONObject()["body"]) // base64 val
		return true
	})).Return(nil)

	err := wh.DeliveryRequest(wh.ctx, mock.Anything, sub, event, core.DataArray{
		{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"value1"`)},
		{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"value2"`)},
	})
	assert.NoError(t, err)
	assert.True(t, called)

	mcb.AssertExpectations(t)
}

func TestRequestReplyDataArrayError(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	msgID := fftypes.NewUUID()
	r := mux.NewRouter()
	server := httptest.NewServer(r)
	server.Close()

	sub := &core.Subscription{
		SubscriptionRef: core.SubscriptionRef{
			Namespace: "ns1",
		},
	}
	to := sub.Options.TransportOptions()
	to["url"] = fmt.Sprintf("http://%s/myapi", server.Listener.Addr())
	to["reply"] = true
	event := &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID: fftypes.NewUUID(),
			},
			Message: &core.Message{
				Header: core.MessageHeader{
					ID:   msgID,
					Type: core.MessageTypeBroadcast,
				},
			},
		},
		Subscription: core.SubscriptionRef{
			ID: sub.ID,
		},
	}

	mcb := wh.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)
	mcb.On("DeliveryResponse", mock.Anything, mock.MatchedBy(func(response *core.EventDeliveryResponse) bool {
		assert.Equal(t, *msgID, *response.Reply.Message.Header.CID)
		assert.Nil(t, response.Reply.Message.Header.Group)
		assert.Equal(t, core.MessageTypeBroadcast, response.Reply.Message.Header.Type)
		assert.Equal(t, float64(502), response.Reply.InlineData[0].Value.JSONObject()["status"])
		assert.NotEmpty(t, response.Reply.InlineData[0].Value.JSONObject().GetObject("body")["error"])
		return true
	})).Return(nil)

	err := wh.DeliveryRequest(wh.ctx, mock.Anything, sub, event, core.DataArray{
		{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"value1"`)},
		{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"value2"`)},
	})
	assert.NoError(t, err)

	mcb.AssertExpectations(t)
}

func TestWebhookFailFastAck(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	msgID := fftypes.NewUUID()
	r := mux.NewRouter()
	server := httptest.NewServer(r)
	server.Close()

	sub := &core.Subscription{
		SubscriptionRef: core.SubscriptionRef{
			Namespace: "ns1",
		},
	}
	sub.Options.TransportOptions()["fastack"] = true
	event := &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID: fftypes.NewUUID(),
			},
			Message: &core.Message{
				Header: core.MessageHeader{
					ID:   msgID,
					Type: core.MessageTypeBroadcast,
				},
			},
		},
		Subscription: core.SubscriptionRef{
			ID: sub.ID,
		},
	}

	count := 0
	waiter := make(chan struct{})
	mcb := wh.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)
	mcb.On("DeliveryResponse", mock.Anything, mock.Anything).
		Return(nil).
		Run(func(a mock.Arguments) {
			count++
			if count == 2 {
				close(waiter)
			}
		})

	// Drive two deliveries, waiting for them both to ack (noting both will fail)
	err := wh.DeliveryRequest(wh.ctx, mock.Anything, sub, event, core.DataArray{
		{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"value1"`)},
		{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"value2"`)},
	})
	assert.NoError(t, err)

	err = wh.DeliveryRequest(wh.ctx, mock.Anything, sub, event, core.DataArray{
		{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"value1"`)},
		{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"value2"`)},
	})
	assert.NoError(t, err)

	<-waiter

	mcb.AssertExpectations(t)
}

func TestWebhookFailFastAckBatch(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	msgID := fftypes.NewUUID()
	r := mux.NewRouter()
	server := httptest.NewServer(r)
	server.Close()

	sub := &core.Subscription{
		SubscriptionRef: core.SubscriptionRef{
			Namespace: "ns1",
		},
	}
	sub.Options.TransportOptions()["fastack"] = true
	event := &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID: fftypes.NewUUID(),
			},
			Message: &core.Message{
				Header: core.MessageHeader{
					ID:   msgID,
					Type: core.MessageTypeBroadcast,
				},
			},
		},
		Subscription: core.SubscriptionRef{
			ID: sub.ID,
		},
	}

	count := 0
	waiter := make(chan struct{})
	mcb := wh.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)
	mcb.On("DeliveryResponse", mock.Anything, mock.Anything).
		Return(nil).
		Run(func(a mock.Arguments) {
			count++
			if count == 2 {
				close(waiter)
			}
		})

	// Drive two deliveries, waiting for them both to ack (noting both will fail)
	err := wh.BatchDeliveryRequest(wh.ctx, mock.Anything, sub, []*core.CombinedEventDataDelivery{
		{Event: event, Data: core.DataArray{
			{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"value1"`)},
			{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"value2"`)},
		}},
		{Event: event, Data: core.DataArray{
			{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"value1"`)},
			{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"value2"`)},
		}},
	})
	assert.NoError(t, err)

	<-waiter

	mcb.AssertExpectations(t)
}

func TestDeliveryRequestNilMessage(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()
	mcb := wh.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)
	mcb.On("DeliveryResponse", mock.Anything, mock.Anything).Return("", &core.EventDelivery{})

	yes := true
	sub := &core.Subscription{
		SubscriptionRef: core.SubscriptionRef{
			Namespace: "ns1",
		},
		Options: core.SubscriptionOptions{
			SubscriptionCoreOptions: core.SubscriptionCoreOptions{
				WithData: &yes,
			},
		},
	}
	sub.Options.TransportOptions()["reply"] = true
	event := &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID: fftypes.NewUUID(),
			},
		},
		Subscription: core.SubscriptionRef{
			ID: sub.ID,
		},
	}

	err := wh.DeliveryRequest(wh.ctx, mock.Anything, sub, event, nil)
	assert.NoError(t, err)
	mcb.AssertExpectations(t)
}

func TestDeliveryRequestReplyToReply(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	yes := true
	sub := &core.Subscription{
		SubscriptionRef: core.SubscriptionRef{
			Namespace: "ns1",
		},
		Options: core.SubscriptionOptions{
			SubscriptionCoreOptions: core.SubscriptionCoreOptions{
				WithData: &yes,
			},
		},
	}
	sub.Options.TransportOptions()["reply"] = true
	event := &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID: fftypes.NewUUID(),
			},
			Message: &core.Message{
				Header: core.MessageHeader{
					ID:   fftypes.NewUUID(),
					Type: core.MessageTypeBroadcast,
					CID:  fftypes.NewUUID(),
				},
			},
		},
		Subscription: core.SubscriptionRef{
			ID: sub.ID,
		},
	}

	mcb := wh.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)
	mcb.On("DeliveryResponse", mock.Anything, mock.MatchedBy(func(response *core.EventDeliveryResponse) bool {
		return !response.Rejected // should be accepted as a no-op so we can move on to other events
	}))

	err := wh.DeliveryRequest(wh.ctx, mock.Anything, sub, event, nil)
	assert.NoError(t, err)

	mcb.AssertExpectations(t)
}

func TestBatchDeliveryRequestReplyToReply(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	yes := true
	sub := &core.Subscription{
		SubscriptionRef: core.SubscriptionRef{
			Namespace: "ns1",
		},
		Options: core.SubscriptionOptions{
			SubscriptionCoreOptions: core.SubscriptionCoreOptions{
				WithData: &yes,
			},
		},
	}
	sub.Options.TransportOptions()["reply"] = true
	event := &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID: fftypes.NewUUID(),
			},
			Message: &core.Message{
				Header: core.MessageHeader{
					ID:   fftypes.NewUUID(),
					Type: core.MessageTypeBroadcast,
					CID:  fftypes.NewUUID(),
				},
			},
		},
		Subscription: core.SubscriptionRef{
			ID: sub.ID,
		},
	}

	mcb := wh.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)
	mcb.On("DeliveryResponse", mock.Anything, mock.MatchedBy(func(response *core.EventDeliveryResponse) bool {
		return !response.Rejected // should be accepted as a no-op so we can move on to other events
	}))

	err := wh.BatchDeliveryRequest(wh.ctx, mock.Anything, sub, []*core.CombinedEventDataDelivery{{Event: event, Data: nil}})
	assert.NoError(t, err)

	mcb.AssertExpectations(t)
}

func TestNamespaceRestarted(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	wh.NamespaceRestarted("ns1", time.Now())
}

func TestRequestWithBodyReplyEndToEndWithBatch(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	r := mux.NewRouter()
	r.HandleFunc("/myapi", func(res http.ResponseWriter, req *http.Request) {
		assert.Equal(t, "myheaderval", req.Header.Get("My-Header"))
		assert.Equal(t, "myqueryval", req.URL.Query().Get("my-query"))
		var data []fftypes.JSONObject
		err := json.NewDecoder(req.Body).Decode(&data)
		assert.NoError(t, err)
		assert.Equal(t, len(data), 2)
		assert.Equal(t, "inputvalue", data[0].GetObject("in_body").GetString("inputfield"))
		res.Header().Set("my-reply-header", "myheaderval2")
		res.WriteHeader(200)
		res.Write([]byte(`{
			"replyfield": "replyvalue"
		}`))
	}).Methods(http.MethodPut)
	server := httptest.NewServer(r)
	defer server.Close()

	yes := true
	dataID := fftypes.NewUUID()
	msgID := fftypes.NewUUID()
	groupHash := fftypes.NewRandB32()
	sub := &core.Subscription{
		SubscriptionRef: core.SubscriptionRef{
			Namespace: "ns1",
		},
		Options: core.SubscriptionOptions{
			SubscriptionCoreOptions: core.SubscriptionCoreOptions{
				WithData: &yes,
			},
		},
	}
	to := sub.Options.TransportOptions()
	to["reply"] = true
	to["json"] = true
	to["method"] = "PUT"
	to["url"] = fmt.Sprintf("http://%s/myapi", server.Listener.Addr())
	to["headers"] = map[string]interface{}{
		"my-header": "myheaderval",
	}
	to["query"] = map[string]interface{}{
		"my-query": "myqueryval",
	}
	event1 := &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID: fftypes.NewUUID(),
			},
			Message: &core.Message{
				Header: core.MessageHeader{
					ID:    msgID,
					Group: groupHash,
					Type:  core.MessageTypePrivate,
				},
				Data: core.DataRefs{
					{ID: dataID},
				},
			},
		},
		Subscription: core.SubscriptionRef{
			ID: sub.ID,
		},
	}

	event2 := &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID: fftypes.NewUUID(),
			},
			Message: &core.Message{
				Header: core.MessageHeader{
					ID:    msgID,
					Group: groupHash,
					Type:  core.MessageTypePrivate,
				},
				Data: core.DataRefs{
					{ID: dataID},
				},
			},
		},
		Subscription: core.SubscriptionRef{
			ID: sub.ID,
		},
	}

	data1 := core.DataArray{&core.Data{
		ID: dataID,
		Value: fftypes.JSONAnyPtr(`{
			"in_body": {
				"inputfield": "inputvalue"
			},
			"in_query": {
				"dynamic-query": "dynamicqueryval"
			},
			"in_headers": {
				"dynamic-header": "dynamicheaderval"
			},
			"in_path": "/my/sub/path?escape_query",
			"in_replytx": true
		}`),
	}}

	data2 := core.DataArray{&core.Data{
		ID: dataID,
		Value: fftypes.JSONAnyPtr(`{
			"in_body": {
				"inputfield": "inputvalue"
			},
			"in_query": {
				"dynamic-query": "dynamicqueryval"
			},
			"in_headers": {
				"dynamic-header": "dynamicheaderval"
			},
			"in_path": "/my/sub/path?escape_query",
			"in_replytx": true
		}`),
	}}

	mcb := wh.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)
	mcb.On("DeliveryResponse", mock.Anything, mock.MatchedBy(func(response *core.EventDeliveryResponse) bool {
		assert.Equal(t, *msgID, *response.Reply.Message.Header.CID)
		assert.Equal(t, *groupHash, *response.Reply.Message.Header.Group)
		assert.Equal(t, core.MessageTypePrivate, response.Reply.Message.Header.Type)
		assert.Equal(t, "myheaderval2", response.Reply.InlineData[0].Value.JSONObject().GetObject("headers").GetString("My-Reply-Header"))
		assert.Equal(t, "replyvalue", response.Reply.InlineData[0].Value.JSONObject().GetObject("body").GetString("replyfield"))
		assert.Equal(t, float64(200), response.Reply.InlineData[0].Value.JSONObject()["status"])
		return true
	})).Return(nil)

	err := wh.BatchDeliveryRequest(wh.ctx, mock.Anything, sub, []*core.CombinedEventDataDelivery{{Event: event1, Data: data1}, {Event: event2, Data: data2}})
	assert.NoError(t, err)

	mcb.AssertExpectations(t)
}

func TestFirstDataNeverNil(t *testing.T) {
	assert.NotNil(t, (&whPayload{}).firstData())
}

func TestBreqCorrelatorSharedBetweenRequestAndResponseLogs(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	logger := logrus.StandardLogger()
	origHooks := logger.Hooks
	hook := &testHook{}
	logger.AddHook(hook)
	logrus.SetLevel(logrus.DebugLevel)
	defer logger.ReplaceHooks(origHooks)

	r := mux.NewRouter()
	r.HandleFunc("/ping", func(res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(200)
		_, _ = res.Write([]byte(`ok`))
	}).Methods(http.MethodPost)
	server := httptest.NewServer(r)
	defer server.Close()

	subID := fftypes.NewUUID()
	sub := &core.Subscription{
		SubscriptionRef: core.SubscriptionRef{ID: subID, Namespace: "ns1"},
	}
	sub.Options.TransportOptions()["url"] = fmt.Sprintf("http://%s/ping", server.Listener.Addr())
	event := &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{Event: core.Event{ID: fftypes.NewUUID()}},
		Subscription:  core.SubscriptionRef{ID: subID},
	}

	mcb := wh.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)
	mcb.On("DeliveryResponse", mock.Anything, mock.MatchedBy(func(resp *core.EventDeliveryResponse) bool {
		return !resp.Rejected
	})).Return(nil)

	err := wh.DeliveryRequest(wh.ctx, mock.Anything, sub, event, nil)
	assert.NoError(t, err)

	var reqBreq, respBreq interface{}
	for _, e := range hook.entries {
		if strings.HasPrefix(e.Message, "==> POST") {
			reqBreq = e.Data["breq"]
		}
		if strings.HasPrefix(e.Message, "Webhook<-") {
			respBreq = e.Data["breq"]
		}
	}

	assert.NotNil(t, reqBreq, "outbound request log should carry a breq field")
	assert.NotNil(t, respBreq, "response log should carry a breq field")
	assert.Equal(t, reqBreq, respBreq, "request and response logs must share the same breq correlator")

	mcb.AssertExpectations(t)
}

type testHook struct{ entries []*logrus.Entry }

func (h *testHook) Levels() []logrus.Level { return logrus.AllLevels }
func (h *testHook) Fire(e *logrus.Entry) error {
	h.entries = append(h.entries, e)
	return nil
}

func TestLoggingContextPreserved(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	logger := logrus.StandardLogger()
	origHooks := logger.Hooks
	hook := &testHook{}
	logger.AddHook(hook)
	logrus.SetLevel(logrus.DebugLevel)
	defer logger.ReplaceHooks(origHooks)

	r := mux.NewRouter()
	r.HandleFunc("/ping", func(res http.ResponseWriter, req *http.Request) {
		res.WriteHeader(200)
		_, _ = res.Write([]byte(`ok`))
	}).Methods(http.MethodPost)
	server := httptest.NewServer(r)
	defer server.Close()

	subID := fftypes.NewUUID()
	sub := &core.Subscription{
		SubscriptionRef: core.SubscriptionRef{ID: subID, Namespace: "ns1"},
	}
	to := sub.Options.TransportOptions()
	to["url"] = fmt.Sprintf("http://%s/ping", server.Listener.Addr())
	event := &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{Event: core.Event{ID: fftypes.NewUUID()}},
		Subscription:  core.SubscriptionRef{ID: subID},
	}

	parentCtx := fflog.WithLogFields(context.Background(), "httpReq", "req-123")

	mcb := wh.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)
	mcb.On("DeliveryResponse", mock.Anything, mock.MatchedBy(func(resp *core.EventDeliveryResponse) bool {
		return !resp.Rejected
	})).Return(nil)

	err := wh.DeliveryRequest(parentCtx, mock.Anything, sub, event, nil)
	assert.NoError(t, err)

	found := false
	for _, e := range hook.entries {
		if e.Data["httpReq"] == "req-123" && e.Data["webhook"] != nil && e.Data["sub"] == subID.String() {
			found = true
			break
		}
	}
	assert.True(t, found, "expected log entry with preserved httpReq, webhook, and sub fields")

	mcb.AssertExpectations(t)
}

// --- confirmationMode -------------------------------------------------------------------------
//
// See https://github.com/hyperledger-firefly/firefly/issues/1770. The critical property under
// test is that nothing changes for a subscription that does not set `confirmationMode`, and that
// only the new "assured" mode ever returns an error to the dispatcher (which is what holds the
// subscription checkpoint and drives redelivery).

// newConfirmationModeTestSub builds a webhook subscription pointing at a test server that always
// responds with the given status, plus a single event to deliver over it.
func newConfirmationModeTestSub(t *testing.T, status int) (sub *core.Subscription, event *core.EventDelivery, called *bool, closeServer func()) {
	t.Helper()
	wasCalled := false
	r := mux.NewRouter()
	r.HandleFunc("/myapi", func(res http.ResponseWriter, req *http.Request) {
		wasCalled = true
		res.WriteHeader(status)
		_, _ = res.Write([]byte(`{}`))
	}).Methods(http.MethodPost)
	server := httptest.NewServer(r)

	subID := fftypes.NewUUID()
	sub = &core.Subscription{
		SubscriptionRef: core.SubscriptionRef{
			ID:        subID,
			Namespace: "ns1",
		},
	}
	sub.Options.TransportOptions()["url"] = fmt.Sprintf("http://%s/myapi", server.Listener.Addr())
	event = &core.EventDelivery{
		EnrichedEvent: core.EnrichedEvent{
			Event: core.Event{
				ID:       fftypes.NewUUID(),
				Sequence: 12345,
			},
			Message: &core.Message{
				Header: core.MessageHeader{
					ID:   fftypes.NewUUID(),
					Type: core.MessageTypeBroadcast,
				},
			},
		},
		Subscription: core.SubscriptionRef{
			ID:        subID,
			Namespace: "ns1",
		},
	}
	return sub, event, &wasCalled, server.Close
}

func TestConfirmationModeForUnsetIsBestEffort(t *testing.T) {
	sub := &core.Subscription{}
	// Today's default, and the whole reason this is a non-breaking change
	assert.Equal(t, core.WebhookConfirmationModeBestEffort, confirmationModeFor(sub))
}

func TestConfirmationModeForDeprecatedFastackFallback(t *testing.T) {
	// The migration case: a subscription created before confirmationMode existed
	sub := &core.Subscription{}
	sub.Options.TransportOptions()["fastack"] = true
	assert.Equal(t, core.WebhookConfirmationModeFastAck, confirmationModeFor(sub))

	// An explicit false is not a signal of anything - still the default
	sub2 := &core.Subscription{}
	sub2.Options.TransportOptions()["fastack"] = false
	assert.Equal(t, core.WebhookConfirmationModeBestEffort, confirmationModeFor(sub2))
}

func TestConfirmationModeForExplicitModes(t *testing.T) {
	for _, mode := range core.WebhookConfirmationModes {
		sub := &core.Subscription{}
		sub.Options.TransportOptions()["confirmationMode"] = string(mode)
		assert.Equal(t, mode, confirmationModeFor(sub))
	}

	// An explicit mode takes precedence over the deprecated boolean (only legal when they agree,
	// per ValidateOptions, but resolution must not depend on that)
	sub := &core.Subscription{}
	sub.Options.TransportOptions()["fastack"] = true
	sub.Options.TransportOptions()["confirmationMode"] = string(core.WebhookConfirmationModeFastAck)
	assert.Equal(t, core.WebhookConfirmationModeFastAck, confirmationModeFor(sub))
}

func TestValidateOptionsConfirmationModeValid(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	for _, mode := range append([]core.WebhookConfirmationMode{""}, core.WebhookConfirmationModes...) {
		opts := &core.SubscriptionOptions{}
		opts.TransportOptions()["url"] = "/anything"
		if mode != "" {
			opts.TransportOptions()["confirmationMode"] = string(mode)
		}
		err := wh.ValidateOptions(wh.ctx, opts)
		assert.NoError(t, err, "confirmationMode '%s' should be accepted", mode)
	}
}

func TestValidateOptionsConfirmationModeInvalid(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	opts := &core.SubscriptionOptions{}
	opts.TransportOptions()["url"] = "/anything"
	opts.TransportOptions()["confirmationMode"] = "strict" // plausible typo, not a real mode
	err := wh.ValidateOptions(wh.ctx, opts)
	assert.Regexp(t, "FF10487", err)
}

func TestValidateOptionsFastackConfirmationModeConflict(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	// fastack:true says "ack before delivery", the mode says otherwise - refuse to pick a winner
	for _, mode := range []core.WebhookConfirmationMode{core.WebhookConfirmationModeBestEffort, core.WebhookConfirmationModeAssured} {
		opts := &core.SubscriptionOptions{}
		opts.TransportOptions()["url"] = "/anything"
		opts.TransportOptions()["fastack"] = true
		opts.TransportOptions()["confirmationMode"] = string(mode)
		err := wh.ValidateOptions(wh.ctx, opts)
		assert.Regexp(t, "FF10488", err, "fastack + confirmationMode '%s' should conflict", mode)
	}
}

func TestValidateOptionsFastackConfirmationModeAgree(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	// Saying the same thing twice is redundant, but not contradictory
	opts := &core.SubscriptionOptions{}
	opts.TransportOptions()["url"] = "/anything"
	opts.TransportOptions()["fastack"] = true
	opts.TransportOptions()["confirmationMode"] = string(core.WebhookConfirmationModeFastAck)
	err := wh.ValidateOptions(wh.ctx, opts)
	assert.NoError(t, err)

	// As is the deprecated option on its own - every existing subscription looks like this
	opts2 := &core.SubscriptionOptions{}
	opts2.TransportOptions()["url"] = "/anything"
	opts2.TransportOptions()["fastack"] = true
	err = wh.ValidateOptions(wh.ctx, opts2)
	assert.NoError(t, err)
}

func TestDeliveryRequestDefaultAcksOnNon2XX(t *testing.T) {
	// No confirmationMode set: a 500 is still acknowledged and the checkpoint still advances.
	// This is the no-regression test - if this one changes, the change is breaking.
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	sub, event, called, closeServer := newConfirmationModeTestSub(t, 500)
	defer closeServer()

	mcb := wh.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)
	mcb.On("DeliveryResponse", mock.Anything, mock.MatchedBy(func(response *core.EventDeliveryResponse) bool {
		return !response.Rejected
	})).Return(nil)

	err := wh.DeliveryRequest(wh.ctx, mock.Anything, sub, event, core.DataArray{})
	assert.NoError(t, err)
	assert.True(t, *called)

	mcb.AssertExpectations(t)
}

func TestDeliveryRequestBestEffortAcksOnNon2XX(t *testing.T) {
	// Naming today's behavior explicitly must behave identically to leaving it unset
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	sub, event, called, closeServer := newConfirmationModeTestSub(t, 500)
	defer closeServer()
	sub.Options.TransportOptions()["confirmationMode"] = string(core.WebhookConfirmationModeBestEffort)

	mcb := wh.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)
	mcb.On("DeliveryResponse", mock.Anything, mock.MatchedBy(func(response *core.EventDeliveryResponse) bool {
		return !response.Rejected
	})).Return(nil)

	err := wh.DeliveryRequest(wh.ctx, mock.Anything, sub, event, core.DataArray{})
	assert.NoError(t, err)
	assert.True(t, *called)

	mcb.AssertExpectations(t)
}

func TestDeliveryRequestAssuredHoldsCheckpointOnNon2XX(t *testing.T) {
	// The new behavior: no ack at all, and an error back to the dispatcher, which nacks and
	// rewinds the polling offset so these events are redelivered.
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	sub, event, called, closeServer := newConfirmationModeTestSub(t, 500)
	defer closeServer()
	sub.Options.TransportOptions()["confirmationMode"] = string(core.WebhookConfirmationModeAssured)

	// Deliberately no DeliveryResponse expectation - any acknowledgement here is a failure,
	// because the dispatcher is about to nack this event on the back of the error we return.
	mcb := wh.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)

	err := wh.DeliveryRequest(wh.ctx, mock.Anything, sub, event, core.DataArray{})
	assert.Regexp(t, "FF10486", err)
	assert.True(t, *called)

	mcb.AssertExpectations(t)
}

func TestDeliveryRequestAssuredAcksOn2XX(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	sub, event, called, closeServer := newConfirmationModeTestSub(t, 200)
	defer closeServer()
	sub.Options.TransportOptions()["confirmationMode"] = string(core.WebhookConfirmationModeAssured)

	mcb := wh.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)
	mcb.On("DeliveryResponse", mock.Anything, mock.MatchedBy(func(response *core.EventDeliveryResponse) bool {
		return !response.Rejected
	})).Return(nil)

	err := wh.DeliveryRequest(wh.ctx, mock.Anything, sub, event, core.DataArray{})
	assert.NoError(t, err)
	assert.True(t, *called)

	mcb.AssertExpectations(t)
}

func TestDeliveryRequestAssuredHoldsCheckpointOnNetworkFailure(t *testing.T) {
	// A connection failure is synthesized into a 502 before it reaches the mode check, so it must
	// hold the checkpoint too - nothing was delivered.
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	sub, event, _, closeServer := newConfirmationModeTestSub(t, 200)
	closeServer() // nothing is listening on that address any more
	sub.Options.TransportOptions()["confirmationMode"] = string(core.WebhookConfirmationModeAssured)

	mcb := wh.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)

	err := wh.DeliveryRequest(wh.ctx, mock.Anything, sub, event, core.DataArray{})
	assert.Regexp(t, "FF10486", err)

	mcb.AssertExpectations(t)
}

func TestDeliveryRequestAssuredReplyModeStillAcksOnNon2XX(t *testing.T) {
	// Reply mode is exempt from confirmationMode: the webhook response *is* the payload relayed
	// back to the original caller, whatever its status, so there is nothing to hold open.
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	sub, event, called, closeServer := newConfirmationModeTestSub(t, 500)
	defer closeServer()
	sub.Options.TransportOptions()["confirmationMode"] = string(core.WebhookConfirmationModeAssured)
	sub.Options.TransportOptions()["reply"] = true

	mcb := wh.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)
	mcb.On("DeliveryResponse", mock.Anything, mock.MatchedBy(func(response *core.EventDeliveryResponse) bool {
		assert.False(t, response.Rejected)
		assert.Equal(t, float64(500), response.Reply.InlineData[0].Value.JSONObject()["status"])
		return true
	})).Return(nil)

	err := wh.DeliveryRequest(wh.ctx, mock.Anything, sub, event, core.DataArray{})
	assert.NoError(t, err)
	assert.True(t, *called)

	mcb.AssertExpectations(t)
}

func TestDeliveryRequestConfirmationModeFastackIsDetached(t *testing.T) {
	// confirmationMode: fastack must behave exactly like the deprecated boolean - ack up front,
	// deliver detached, and never surface the outcome.
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	sub, event, _, closeServer := newConfirmationModeTestSub(t, 500)
	defer closeServer()
	sub.Options.TransportOptions()["confirmationMode"] = string(core.WebhookConfirmationModeFastAck)

	acked := make(chan struct{})
	mcb := wh.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)
	mcb.On("DeliveryResponse", mock.Anything, mock.MatchedBy(func(response *core.EventDeliveryResponse) bool {
		return !response.Rejected
	})).Return(nil).Run(func(a mock.Arguments) {
		close(acked)
	})

	err := wh.DeliveryRequest(wh.ctx, mock.Anything, sub, event, core.DataArray{})
	assert.NoError(t, err)
	<-acked

	mcb.AssertExpectations(t)
}

func TestBatchDeliveryRequestDefaultAcksOnNon2XX(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	sub, event, called, closeServer := newConfirmationModeTestSub(t, 500)
	defer closeServer()

	mcb := wh.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)
	mcb.On("DeliveryResponse", mock.Anything, mock.MatchedBy(func(response *core.EventDeliveryResponse) bool {
		return !response.Rejected
	})).Return(nil).Twice()

	err := wh.BatchDeliveryRequest(wh.ctx, mock.Anything, sub, []*core.CombinedEventDataDelivery{
		{Event: event, Data: core.DataArray{}},
		{Event: event, Data: core.DataArray{}},
	})
	assert.NoError(t, err)
	assert.True(t, *called)

	mcb.AssertExpectations(t)
}

func TestBatchDeliveryRequestAssuredHoldsCheckpointOnNon2XX(t *testing.T) {
	// A failed batch nacks the whole batch, matching the existing batch nack semantics
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	sub, event, called, closeServer := newConfirmationModeTestSub(t, 500)
	defer closeServer()
	sub.Options.TransportOptions()["confirmationMode"] = string(core.WebhookConfirmationModeAssured)

	// Again, no DeliveryResponse expectation - nothing in the batch may be acknowledged
	mcb := wh.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)

	err := wh.BatchDeliveryRequest(wh.ctx, mock.Anything, sub, []*core.CombinedEventDataDelivery{
		{Event: event, Data: core.DataArray{}},
		{Event: event, Data: core.DataArray{}},
	})
	assert.Regexp(t, "FF10486", err)
	assert.True(t, *called)

	mcb.AssertExpectations(t)
}

func TestBatchDeliveryRequestAssuredAcksOn2XX(t *testing.T) {
	wh, cancel := newTestWebHooks(t)
	defer cancel()

	sub, event, called, closeServer := newConfirmationModeTestSub(t, 200)
	defer closeServer()
	sub.Options.TransportOptions()["confirmationMode"] = string(core.WebhookConfirmationModeAssured)

	mcb := wh.callbacks.handlers["ns1"].(*eventsmocks.Callbacks)
	mcb.On("DeliveryResponse", mock.Anything, mock.MatchedBy(func(response *core.EventDeliveryResponse) bool {
		return !response.Rejected
	})).Return(nil).Twice()

	err := wh.BatchDeliveryRequest(wh.ctx, mock.Anything, sub, []*core.CombinedEventDataDelivery{
		{Event: event, Data: core.DataArray{}},
		{Event: event, Data: core.DataArray{}},
	})
	assert.NoError(t, err)
	assert.True(t, *called)

	mcb.AssertExpectations(t)
}
