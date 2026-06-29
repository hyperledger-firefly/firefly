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

package fabric

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-resty/resty/v2"
	"github.com/hyperledger-firefly/common/pkg/ffresty"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/cache"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

type streamManager struct {
	client         *resty.Client
	signer         string
	cache          cache.CInterface
	batchSize      uint
	batchTimeoutMS int64
}

type eventStream struct {
	ID             string               `json:"id"`
	Name           string               `json:"name"`
	ErrorHandling  string               `json:"errorHandling"`
	BatchSize      uint                 `json:"batchSize"`
	BatchTimeoutMS int64                `json:"batchTimeoutMS"`
	Type           string               `json:"type"`
	WebSocket      eventStreamWebsocket `json:"websocket"`
	Timestamps     bool                 `json:"timestamps"`
}

type subscription struct {
	ID        string      `json:"id"`
	Name      string      `json:"name,omitempty"`
	Channel   string      `json:"channel"`
	Signer    string      `json:"signer"`
	Stream    string      `json:"stream"`
	FromBlock string      `json:"fromBlock"`
	Filter    eventFilter `json:"filter"`
}

type eventFilter struct {
	ChaincodeID string `json:"chaincodeId"`
	EventFilter string `json:"eventFilter"`
}

func newStreamManager(client *resty.Client, signer string, cache cache.CInterface, batchSize uint, batchTimeout int64) *streamManager {
	return &streamManager{
		client:         client,
		signer:         signer,
		cache:          cache,
		batchSize:      batchSize,
		batchTimeoutMS: batchTimeout,
	}
}

func (s *streamManager) getEventStreams(ctx context.Context) (streams []*eventStream, err error) {
	res, err := s.client.R().
		SetContext(ctx).
		SetResult(&streams).
		Get("/eventstreams")
	if err != nil || !res.IsSuccess() {
		return nil, ffresty.WrapRestErr(ctx, res, err, coremsgs.MsgFabconnectRESTErr)
	}
	return streams, nil
}

func buildEventStream(topic string, batchSize uint, batchTimeout int64) *eventStream {
	return &eventStream{
		Name:           topic,
		ErrorHandling:  "block",
		BatchSize:      batchSize,
		BatchTimeoutMS: batchTimeout,
		Type:           "websocket",
		// Some implementations require a "topic" to be set separately, while others rely only on the name.
		// We set them to the same thing for cross compatibility.
		WebSocket:  eventStreamWebsocket{Topic: topic},
		Timestamps: true,
	}
}

func (s *streamManager) createEventStream(ctx context.Context, topic string) (*eventStream, error) {
	stream := buildEventStream(topic, s.batchSize, s.batchTimeoutMS)
	res, err := s.client.R().
		SetContext(ctx).
		SetBody(stream).
		SetResult(stream).
		Post("/eventstreams")
	if err != nil || !res.IsSuccess() {
		return nil, ffresty.WrapRestErr(ctx, res, err, coremsgs.MsgFabconnectRESTErr)
	}
	return stream, nil
}

func (s *streamManager) ensureEventStream(ctx context.Context, topic, pluginTopic string) (*eventStream, error) {
	existingStreams, err := s.getEventStreams(ctx)
	if err != nil {
		return nil, err
	}
	for _, stream := range existingStreams {
		if stream.Name == topic {
			return stream, nil
		}
		if stream.Name == pluginTopic {
			// We have an old event stream that needs to get deleted
			if err := s.deleteEventStream(ctx, stream.ID, false); err != nil {
				return nil, err
			}
		}
	}
	return s.createEventStream(ctx, topic)
}

func (s *streamManager) deleteEventStream(ctx context.Context, esID string, okNotFound bool) error {
	res, err := s.client.R().
		SetContext(ctx).
		Delete("/eventstreams/" + esID)
	if err != nil || !res.IsSuccess() {
		if okNotFound && res.StatusCode() == http.StatusNotFound {
			return nil
		}
		return ffresty.WrapRestErr(ctx, res, err, coremsgs.MsgFabconnectRESTErr)
	}
	return nil
}

func (s *streamManager) getSubscriptions(ctx context.Context) (subs []*subscription, err error) {
	res, err := s.client.R().
		SetContext(ctx).
		SetResult(&subs).
		Get("/subscriptions")
	if err != nil || !res.IsSuccess() {
		return nil, ffresty.WrapRestErr(ctx, res, err, coremsgs.MsgFabconnectRESTErr)
	}
	return subs, nil
}

func (s *streamManager) getSubscription(ctx context.Context, subID string, okNotFound bool) (sub *subscription, err error) {
	res, err := s.client.R().
		SetContext(ctx).
		SetResult(&sub).
		Get(fmt.Sprintf("/subscriptions/%s", subID))
	if err != nil || !res.IsSuccess() {
		if okNotFound && res.StatusCode() == http.StatusNotFound {
			return nil, nil
		}
		return nil, ffresty.WrapRestErr(ctx, res, err, coremsgs.MsgFabconnectRESTErr)
	}
	return sub, nil
}

func (s *streamManager) getSubscriptionName(ctx context.Context, subID string, okNotFound bool) (string, error) {
	if cachedValue := s.cache.GetString("sub:" + subID); cachedValue != "" {
		return cachedValue, nil
	}
	sub, err := s.getSubscription(ctx, subID, okNotFound)
	if err != nil {
		return "", err
	}
	s.cache.SetString("sub:"+subID, sub.Name)
	return sub.Name, nil
}

func resolveFromBlock(ctx context.Context, firstEvent, lastProtocolID string) (string, error) {
	// Parse the lastProtocolID if supplied
	var blockBeforeNewestEvent *uint64
	if len(lastProtocolID) > 0 {
		blockStr := strings.Split(lastProtocolID, "/")[0]
		parsedUint, err := strconv.ParseUint(blockStr, 10, 64)
		if err != nil {
			return "", i18n.NewError(ctx, coremsgs.MsgInvalidLastEventProtocolID, lastProtocolID)
		}
		if parsedUint > 0 {
			// We jump back on block from the last event, to minimize re-delivery while ensuring
			// we get all events since the last delivered (including subsequent events in the same block)
			parsedUint--
			blockBeforeNewestEvent = &parsedUint
		}
	}

	// If the user requested newest, then we use the last block number if we have one,
	// or we pass the request for newest down to the connector
	if firstEvent == "" || firstEvent == string(core.SubOptsFirstEventNewest) || firstEvent == "latest" {
		if blockBeforeNewestEvent != nil {
			return strconv.FormatUint(*blockBeforeNewestEvent, 10), nil
		}
		return "newest", nil
	}

	// Otherwise we expect to be able to parse the block, with "oldest" being the same as "0"
	if firstEvent == string(core.SubOptsFirstEventOldest) {
		firstEvent = "0"
	}
	blockNumber, err := strconv.ParseUint(firstEvent, 10, 64)
	if err != nil {
		return "", i18n.NewError(ctx, coremsgs.MsgInvalidFromBlockNumber, firstEvent)
	}
	// If the last event is already dispatched after this block, recreate the listener from that block
	if blockBeforeNewestEvent != nil && *blockBeforeNewestEvent > blockNumber {
		blockNumber = *blockBeforeNewestEvent
	}
	return strconv.FormatUint(blockNumber, 10), nil
}

func (s *streamManager) createSubscription(ctx context.Context, location *Location, stream, name, event, firstEvent, lastProtocolID string) (*subscription, error) {

	fromBlock, err := resolveFromBlock(ctx, firstEvent, lastProtocolID)
	if err != nil {
		return nil, err
	}

	sub := subscription{
		Name:    name,
		Channel: location.Channel,
		Signer:  s.signer,
		Stream:  stream,
		Filter: eventFilter{
			EventFilter: event,
		},
		FromBlock: fromBlock,
	}

	if location.Chaincode != "" {
		sub.Filter.ChaincodeID = location.Chaincode
	}

	res, err := s.client.R().
		SetContext(ctx).
		SetBody(&sub).
		SetResult(&sub).
		Post("/subscriptions")
	if err != nil || !res.IsSuccess() {
		return nil, ffresty.WrapRestErr(ctx, res, err, coremsgs.MsgFabconnectRESTErr)
	}
	return &sub, nil
}

func (s *streamManager) deleteSubscription(ctx context.Context, subID string, okNotFound bool) error {
	res, err := s.client.R().
		SetContext(ctx).
		Delete("/subscriptions/" + subID)
	if err != nil || !res.IsSuccess() {
		if okNotFound && res.StatusCode() == http.StatusNotFound {
			return nil
		}
		return ffresty.WrapRestErr(ctx, res, err, coremsgs.MsgFabconnectRESTErr)
	}
	return nil
}

func (s *streamManager) ensureFireFlySubscription(ctx context.Context, namespace string, version int, location *Location, firstEvent, stream, event, lastProtocolID string) (sub *subscription, err error) {
	existingSubs, err := s.getSubscriptions(ctx)
	if err != nil {
		return nil, err
	}

	v1Name := event
	v2Name := fmt.Sprintf("%s_%s", namespace, event)

	for _, s := range existingSubs {
		if s.Stream == stream {
			if version == 1 {
				if s.Name == v1Name {
					return s, nil
				}
			} else {
				if s.Name == v1Name {
					return nil, i18n.NewError(ctx, coremsgs.MsgInvalidSubscriptionForNetwork, s.Name, version)
				} else if s.Name == v2Name {
					return s, nil
				}
			}
		}
	}

	name := v2Name
	if version == 1 {
		name = v1Name
	}
	if sub, err = s.createSubscription(ctx, location, stream, name, event, firstEvent, lastProtocolID); err != nil {
		return nil, err
	}
	log.L(ctx).Infof("%s subscription: %s", event, sub.ID)
	return sub, nil
}
