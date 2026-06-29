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
