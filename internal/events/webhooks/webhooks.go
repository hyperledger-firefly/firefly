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

package webhooks

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/ffresty"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/events"
)

type WebHooks struct {
	ctx           context.Context
	capabilities  *events.Capabilities
	callbacks     callbacks
	client        *resty.Client
	connID        string
	ffrestyConfig *ffresty.Config
}

type callbacks struct {
	writeLock sync.Mutex
	handlers  map[string]events.Callbacks
}

type whRequest struct {
	r         *resty.Request
	url       string
	method    string
	forceJSON bool
	replyTx   string
}

type whPayload struct {
	input       fftypes.JSONObject
	parsedData0 fftypes.JSONObject
	data0       *fftypes.JSONAny
	body        interface{}
}

type whResponse struct {
	Status  int                `json:"status"`
	Headers fftypes.JSONObject `json:"headers"`
	Body    *fftypes.JSONAny   `json:"body"`
}

func (wh *WebHooks) Name() string { return "webhooks" }

func (wh *WebHooks) Init(ctx context.Context, config config.Section) (err error) {
	connID := fftypes.ShortID()

	ffrestyConfig, err := ffresty.GenerateConfig(ctx, config)
	if err != nil {
		return err
	}

	client := ffresty.NewWithConfig(ctx, *ffrestyConfig)

	*wh = WebHooks{
		ctx: log.WithLogField(ctx, "webhook", connID),
		capabilities: &events.Capabilities{
			BatchDelivery: true,
		},
		callbacks: callbacks{
			handlers: make(map[string]events.Callbacks),
		},
		client:        client,
		connID:        connID,
		ffrestyConfig: ffrestyConfig,
	}
	return nil
}

func (wh *WebHooks) SetHandler(namespace string, handler events.Callbacks) error {
	wh.callbacks.writeLock.Lock()
	defer wh.callbacks.writeLock.Unlock()
	if handler == nil {
		delete(wh.callbacks.handlers, namespace)
		return nil
	}
	wh.callbacks.handlers[namespace] = handler
	// We have a single logical connection, that matches all subscriptions
	return handler.RegisterConnection(wh.connID, func(sr core.SubscriptionRef) bool { return true })
}

func (wh *WebHooks) Capabilities() *events.Capabilities {
	return wh.capabilities
}

// firstData parses data0 from the data the first time it's needed, and guarantees a non-nil result
func (p *whPayload) firstData() fftypes.JSONObject {
	if p.parsedData0 != nil {
		return p.parsedData0
	}
	if p.data0 == nil {
		p.parsedData0 = fftypes.JSONObject{}
	} else {
		// Use JSONObjectOk instead of JSONObject
		// JSONObject fails for datatypes such as array, string, bool, number etc
		var valid bool
		p.parsedData0, valid = p.data0.JSONObjectOk()
		if !valid {
			p.parsedData0 = fftypes.JSONObject{
				"value": p.parsedData0,
			}
		}
	}
	return p.parsedData0
}

func (wh *WebHooks) buildPayload(ctx context.Context, sub *core.Subscription, event *core.CombinedEventDataDelivery) *whPayload {
	log.L(ctx).Debugf("Webhook-> %s event %s", sub.Options.URL, event.Event.ID)
	withData := sub.Options.WithData != nil && *sub.Options.WithData
	options := sub.Options.TransportOptions()
	p := &whPayload{
		// Options on how to process the input
		input: options.GetObject("input"),
	}

	allData := make([]*fftypes.JSONAny, 0, len(event.Data))
	if withData {
		for _, d := range event.Data {
			if d.Value != nil {
				allData = append(allData, d.Value)
				if p.data0 == nil {
					p.data0 = d.Value
				}
			}
		}
	}

	// Choose to sub-select a field to send as the body
	var bodyFromFirstData *fftypes.JSONAny
	inputBody := p.input.GetString("body")
	if inputBody != "" {
		bodyFromFirstData = fftypes.JSONAnyPtr(p.firstData().GetObject(inputBody).String())
	}

	switch {
	case bodyFromFirstData != nil:
		// We might have been told to extract a body from the first data record
		p.body = bodyFromFirstData.String()
	case len(allData) > 1:
		// We've got an array of data to POST
		p.body = allData
	case len(allData) == 1:
		// Just send the first object, forced into an object per the rules in firstData()
		p.body = p.firstData()
	default:
		// Just send the event itself
		p.body = event.Event
	}
	return p
}

func (wh *WebHooks) buildRequest(ctx context.Context, restyClient *resty.Client, options fftypes.JSONObject, p *whPayload) (req *whRequest, err error) {
	req = &whRequest{
		r: restyClient.R().
			SetDoNotParseResponse(true).
			SetContext(wh.ctx),
		url:       options.GetString("url"),
		method:    options.GetString("method"),
		forceJSON: options.GetBool("json"),
		replyTx:   options.GetString("replytx"),
	}
	if req.url == "" {
		return nil, i18n.NewError(ctx, coremsgs.MsgWebhookURLEmpty)
	}
	if req.method == "" {
		req.method = http.MethodPost
	}
	headers := options.GetObject("headers")
	for h, v := range headers {
		s, ok := v.(string)
		if !ok {
			return nil, i18n.NewError(ctx, coremsgs.MsgWebhookInvalidStringMap, "headers", h, v)
		}
		_ = req.r.SetHeader(h, s)
	}
	if req.r.Header.Get("Content-Type") == "" {
		req.r.Header.Set("Content-Type", "application/json")
	}
	// Static query support
	query := options.GetObject("query")
	for q, v := range query {
		s, ok := v.(string)
		if !ok {
			return nil, i18n.NewError(ctx, coremsgs.MsgWebhookInvalidStringMap, "query", q, v)
		}
		_ = req.r.SetQueryParam(q, s)
	}
	// p will be nil for a batch delivery
	if p != nil {
		// Dynamic query support from input
		inputQuery := p.input.GetString("query")
		if inputQuery != "" {
			iq := p.firstData().GetObject(inputQuery)
			for q := range iq {
				_ = req.r.SetQueryParam(q, iq.GetString(q))
			}
		}
		// Dynamic header support from input
		inputHeaders := p.input.GetString("headers")
		if inputHeaders != "" {
			ih := p.firstData().GetObject(inputHeaders)
			for h := range ih {
				_ = req.r.SetHeader(h, ih.GetString(h))
			}
		}
		// Choose to add an additional dynamic path
		inputPath := p.input.GetString("path")
		if inputPath != "" {
			extraPath := strings.TrimPrefix(p.firstData().GetString(inputPath), "/")
			if len(extraPath) > 0 {
				pathSegments := strings.Split(extraPath, "/")
				for _, ps := range pathSegments {
					req.url = strings.TrimSuffix(req.url, "/") + "/" + url.PathEscape(ps)
				}
			}
		}
		// Choose to add an additional dynamic path
		inputTxtype := p.input.GetString("replytx")
		if inputTxtype != "" {
			txType := p.firstData().GetString(inputTxtype)
			if len(txType) > 0 {
				req.replyTx = txType
				if strings.EqualFold(txType, "true") {
					req.replyTx = string(core.TransactionTypeBatchPin)
				}
			}
		}
	}
	return req, err
}

func (wh *WebHooks) ValidateOptions(ctx context.Context, options *core.SubscriptionOptions) error {
	if options.WithData == nil {
		defaultTrue := true
		options.WithData = &defaultTrue
	}

	newFFRestyConfig := ffresty.Config{}
	if wh.ffrestyConfig != nil {
		// Take a copy of the webhooks global resty config
		newFFRestyConfig = *wh.ffrestyConfig
	}
	if options.Retry.Enabled {
		newFFRestyConfig.Retry = true
		if options.Retry.Count > 0 {
			newFFRestyConfig.RetryCount = options.Retry.Count
		}

		if options.Retry.InitialDelay != "" {
			ffd, err := fftypes.ParseDurationString(options.Retry.InitialDelay, time.Millisecond)
			if err != nil {
				return err
			}
			newFFRestyConfig.RetryInitialDelay = fftypes.FFDuration(time.Duration(ffd))
		}

		if options.Retry.MaximumDelay != "" {
			ffd, err := fftypes.ParseDurationString(options.Retry.MaximumDelay, time.Millisecond)
			if err != nil {
				return err
			}
			newFFRestyConfig.RetryMaximumDelay = fftypes.FFDuration(time.Duration(ffd))
		}
	}

	if options.HTTPOptions.HTTPMaxIdleConns > 0 {
		newFFRestyConfig.HTTPMaxIdleConns = options.HTTPOptions.HTTPMaxIdleConns
	}

	if options.HTTPOptions.HTTPRequestTimeout != "" {
		ffd, err := fftypes.ParseDurationString(options.HTTPOptions.HTTPRequestTimeout, time.Millisecond)
		if err != nil {
			return err
		}
		newFFRestyConfig.HTTPRequestTimeout = fftypes.FFDuration(time.Duration(ffd))
	}

	if options.HTTPOptions.HTTPIdleConnTimeout != "" {
		ffd, err := fftypes.ParseDurationString(options.HTTPOptions.HTTPIdleConnTimeout, time.Millisecond)
		if err != nil {
			return err
		}
		newFFRestyConfig.HTTPIdleConnTimeout = fftypes.FFDuration(time.Duration(ffd))
	}

	if options.HTTPOptions.HTTPExpectContinueTimeout != "" {
		ffd, err := fftypes.ParseDurationString(options.HTTPOptions.HTTPExpectContinueTimeout, time.Millisecond)
		if err != nil {
			return err
		}
		newFFRestyConfig.HTTPExpectContinueTimeout = fftypes.FFDuration(time.Duration(ffd))
	}

	if options.HTTPOptions.HTTPConnectionTimeout != "" {
		ffd, err := fftypes.ParseDurationString(options.HTTPOptions.HTTPConnectionTimeout, time.Millisecond)
		if err != nil {
			return err
		}
		newFFRestyConfig.HTTPConnectionTimeout = fftypes.FFDuration(time.Duration(ffd))
	}

	if options.HTTPOptions.HTTPTLSHandshakeTimeout != "" {
		ffd, err := fftypes.ParseDurationString(options.HTTPOptions.HTTPTLSHandshakeTimeout, time.Millisecond)
		if err != nil {
			return err
		}
		newFFRestyConfig.HTTPTLSHandshakeTimeout = fftypes.FFDuration(time.Duration(ffd))
	}

	if options.HTTPOptions.HTTPProxyURL != nil {
		newFFRestyConfig.ProxyURL = *options.HTTPOptions.HTTPProxyURL
	}

	if options.TLSConfig != nil {
		newFFRestyConfig.TLSClientConfig = options.TLSConfig
	}

	// NOTE: this is the plugin context, as the context passed through can be terminated as part of a
	// API call or anything else and we want to use this client later on!!
	// So these clients should live as long as the plugin exists
	options.RestyClient = ffresty.NewWithConfig(wh.ctx, newFFRestyConfig)

	_, err := wh.buildRequest(ctx, options.RestyClient, options.TransportOptions(), nil)
	return err
}

func (wh *WebHooks) attemptRequest(ctx context.Context, sub *core.Subscription, events []*core.CombinedEventDataDelivery, batch bool) (req *whRequest, res *whResponse, err error) {

	var payloadForBuildingRequest *whPayload // only set for a single event delivery
	var requestBody interface{}
	if len(events) == 1 && !batch {
		payloadForBuildingRequest = wh.buildPayload(ctx, sub, events[0])
		// Payload for POST/PATCH/PUT is what is calculated for a single event in buildPayload
		requestBody = payloadForBuildingRequest.body
	} else {
		batchBody := make([]interface{}, len(events))
		for i, event := range events {
			// We only use the body itself from the whPayload - then discard it.
			p := wh.buildPayload(ctx, sub, event)
			batchBody[i] = p.body
		}
		// Payload for POST/PATCH/PUT is the array of outputs calculated for a each event in buildPayload
		requestBody = batchBody
	}

	client := wh.client
	if sub.Options.RestyClient != nil {
		client = sub.Options.RestyClient
	}

	req, err = wh.buildRequest(ctx, client, sub.Options.TransportOptions(), payloadForBuildingRequest)
	if err != nil {
		return nil, nil, err
	}

	if req.method == http.MethodPost || req.method == http.MethodPatch || req.method == http.MethodPut {
		req.r.SetBody(requestBody)
	}

	resp, err := req.r.Execute(req.method, req.url)
	if err != nil {
		log.L(ctx).Errorf("Webhook<- %s %s failed: %s", req.method, req.url, err)
		return nil, nil, err
	}
	defer func() { _ = resp.RawBody().Close() }()

	res = &whResponse{
		Status:  resp.StatusCode(),
		Headers: fftypes.JSONObject{},
	}
	log.L(ctx).Debugf("Webhook<- %s %s returned %d", req.method, req.url, res.Status)
	header := resp.Header()
	for h := range header {
		res.Headers[h] = header.Get(h)
	}
	contentType := header.Get("Content-Type")
	log.L(ctx).Debugf("Response content-type '%s' forceJSON=%t", contentType, req.forceJSON)
	if req.forceJSON {
		contentType = "application/json"
	}
	res.Headers["Content-Type"] = contentType
	if req.forceJSON || strings.HasPrefix(contentType, "application/json") {
		var resData interface{}
		err = json.NewDecoder(resp.RawBody()).Decode(&resData)
		if err != nil {
			return nil, nil, i18n.WrapError(ctx, err, coremsgs.MsgWebhooksReplyBadJSON)
		}
		b, _ := json.Marshal(&resData) // we know we can re-marshal It
		res.Body = fftypes.JSONAnyPtrBytes(b)
	} else {
		// Anything other than JSON, gets returned as a JSON string in base64 encoding
		buf := &bytes.Buffer{}
		buf.WriteByte('"')
		b64Encoder := base64.NewEncoder(base64.StdEncoding, buf)
		_, _ = io.Copy(b64Encoder, resp.RawBody())
		_ = b64Encoder.Close()
		buf.WriteByte('"')
		res.Body = fftypes.JSONAnyPtrBytes(buf.Bytes())
	}

	return req, res, nil
}

func (wh *WebHooks) doDelivery(ctx context.Context, connID string, reply bool, sub *core.Subscription, events []*core.CombinedEventDataDelivery, fastAck, batched bool) {
	req, res, gwErr := wh.attemptRequest(ctx, sub, events, batched)
	if gwErr != nil {
		// Generate a bad-gateway error response - we always want to send something back,
		// rather than just causing timeouts
		log.L(ctx).Errorf("Failed to invoke webhook: %s", gwErr)
		b, _ := json.Marshal(&fftypes.RESTError{
			Error: gwErr.Error(),
		})
		res = &whResponse{
			Status: http.StatusBadGateway,
			Headers: fftypes.JSONObject{
				"Content-Type": "application/json",
			},
			Body: fftypes.JSONAnyPtrBytes(b),
		}
	}
	b, _ := json.Marshal(&res)
	log.L(ctx).Tracef("Webhook response: %s", string(b))

	// For each event emit a response
	for _, combinedEvent := range events {
		event := combinedEvent.Event
		// Emit the response
		if reply && event.Message != nil {
			txType := fftypes.FFEnum(strings.ToLower(sub.Options.TransportOptions().GetString("replytx")))
			if req != nil && req.replyTx != "" {
				txType = fftypes.FFEnum(strings.ToLower(req.replyTx))
			}
			if cb, ok := wh.callbacks.handlers[sub.Namespace]; ok {
				log.L(ctx).Debugf("Sending reply message for %s CID=%s", event.ID, event.Message.Header.ID)
				cb.DeliveryResponse(connID, &core.EventDeliveryResponse{
					ID:           event.ID,
					Rejected:     false,
					Subscription: event.Subscription,
					Reply: &core.MessageInOut{
						Message: core.Message{
							Header: core.MessageHeader{
								CID:    event.Message.Header.ID,
								Group:  event.Message.Header.Group,
								Type:   event.Message.Header.Type,
								Topics: event.Message.Header.Topics,
								Tag:    sub.Options.TransportOptions().GetString("replytag"),
								TxType: txType,
							},
						},
						InlineData: core.InlineData{
							{Value: fftypes.JSONAnyPtrBytes(b)},
						},
					},
				})
			}
		} else if !fastAck {
			if cb, ok := wh.callbacks.handlers[sub.Namespace]; ok {
				cb.DeliveryResponse(connID, &core.EventDeliveryResponse{
					ID:           event.ID,
					Rejected:     false,
					Subscription: event.Subscription,
				})
			}
		}
	}

}

func (wh *WebHooks) DeliveryRequest(ctx context.Context, connID string, sub *core.Subscription, event *core.EventDelivery, data core.DataArray) error {
	ctx = log.WithLogField(log.WithLogField(ctx, "webhook", wh.connID), "sub", sub.ID.String())
	reply := sub.Options.TransportOptions().GetBool("reply")
	if reply && event.Message != nil && event.Message.Header.CID != nil {
		// We cowardly refuse to dispatch a message that is itself a reply, as it's hard for users to
		// avoid loops - and there's no way for us to detect here if a user has configured correctly
		// to avoid a loop.
		log.L(ctx).Debugf("Webhook subscription with reply enabled called with reply event '%s'", event.ID)
		if cb, ok := wh.callbacks.handlers[sub.Namespace]; ok {
			cb.DeliveryResponse(connID, &core.EventDeliveryResponse{
				ID:           event.ID,
				Rejected:     false,
				Subscription: event.Subscription,
			})
		}
		return nil
	}

	// In fastack mode we drive calls in parallel to the backend, immediately acknowledging the event
	// NOTE: We cannot use this with reply mode, as when we're sending a reply the `DeliveryResponse`
	//       callback must include the reply in-line.
	if !reply && sub.Options.TransportOptions().GetBool("fastack") {
		if cb, ok := wh.callbacks.handlers[sub.Namespace]; ok {
			cb.DeliveryResponse(connID, &core.EventDeliveryResponse{
				ID:           event.ID,
				Rejected:     false,
				Subscription: event.Subscription,
			})
		}
		go wh.doDelivery(ctx, connID, reply, sub, []*core.CombinedEventDataDelivery{{Event: event, Data: data}}, true, false)
		return nil
	}

	// NOTE: We could check here for batching and accumulate but we can't return because this causes the offset to jump...

	// TODO we don't look at the error here?
	wh.doDelivery(ctx, connID, reply, sub, []*core.CombinedEventDataDelivery{{Event: event, Data: data}}, false, false)
	return nil
}

func (wh *WebHooks) BatchDeliveryRequest(ctx context.Context, connID string, sub *core.Subscription, events []*core.CombinedEventDataDelivery) error {
	ctx = log.WithLogField(log.WithLogField(ctx, "webhook", wh.connID), "sub", sub.ID.String())
	reply := sub.Options.TransportOptions().GetBool("reply")
	if reply {
		nonReplyEvents := []*core.CombinedEventDataDelivery{}
		for _, combinedEvent := range events {
			event := combinedEvent.Event
			// We cowardly refuse to dispatch a message that is itself a reply, as it's hard for users to
			// avoid loops - and there's no way for us to detect here if a user has configured correctly
			// to avoid a loop.
			if event.Message != nil && event.Message.Header.CID != nil {
				log.L(ctx).Debugf("Webhook subscription with reply enabled called with reply event '%s'", event.ID)
				if cb, ok := wh.callbacks.handlers[sub.Namespace]; ok {
					cb.DeliveryResponse(connID, &core.EventDeliveryResponse{
						ID:           event.ID,
						Rejected:     false,
						Subscription: event.Subscription,
					})
				}
				continue
			}

			nonReplyEvents = append(nonReplyEvents, combinedEvent)
		}
		// Override the events to send without the reply events
		events = nonReplyEvents
	}

	// // In fastack mode we drive calls in parallel to the backend, immediately acknowledging the event
	// NOTE: We cannot use this with reply mode, as when we're sending a reply the `DeliveryResponse`
	//       callback must include the reply in-line.
	if !reply && sub.Options.TransportOptions().GetBool("fastack") {
		for _, combinedEvent := range events {
			event := combinedEvent.Event
			if cb, ok := wh.callbacks.handlers[sub.Namespace]; ok {
				cb.DeliveryResponse(connID, &core.EventDeliveryResponse{
					ID:           event.ID,
					Rejected:     false,
					Subscription: event.Subscription,
				})
			}
		}
		go wh.doDelivery(ctx, connID, reply, sub, events, true, true)
		return nil
	}

	wh.doDelivery(ctx, connID, reply, sub, events, false, true)
	return nil
}

func (wh *WebHooks) NamespaceRestarted(ns string, startTime time.Time) {
	// no-op
}
