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

package tezos

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-resty/resty/v2"
	"github.com/hyperledger-firefly/common/pkg/ffresty"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/cache"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

type streamManager struct {
	client       *resty.Client
	cache        cache.CInterface
	batchSize    uint
	batchTimeout int64
}

type eventStream struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	ErrorHandling  string `json:"errorHandling"`
	BatchSize      uint   `json:"batchSize"`
	BatchTimeoutMS int64  `json:"batchTimeoutMS"`
	Type           string `json:"type"`
	Timestamps     bool   `json:"timestamps"`
}

type subscription struct {
	ID                 string            `json:"id"`
	Name               string            `json:"name,omitempty"`
	Stream             string            `json:"stream"`
	FromBlock          string            `json:"fromBlock"`
	TezosCompatAddress string            `json:"address,omitempty"`
	Filter             eventFilter       `json:"filter"`
	Filters            []fftypes.JSONAny `json:"filters"`
	subscriptionCheckpoint
}

type eventFilter struct {
	EventFilter string `json:"eventFilter"`
}

func newStreamManager(client *resty.Client, cache cache.CInterface, batchSize uint, batchTimeout int64) *streamManager {
	return &streamManager{
		client:       client,
		cache:        cache,
		batchSize:    batchSize,
		batchTimeout: batchTimeout,
	}
}

func (s *streamManager) getEventStreams(ctx context.Context) (streams []*eventStream, err error) {
	res, err := s.client.R().
		SetContext(ctx).
		SetResult(&streams).
		Get("/eventstreams")
	if err != nil || !res.IsSuccess() {
		return nil, ffresty.WrapRestErr(ctx, res, err, coremsgs.MsgTezosconnectRESTErr)
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
		Timestamps:     true,
	}
}

func (s *streamManager) createEventStream(ctx context.Context, topic string) (*eventStream, error) {
	stream := buildEventStream(topic, s.batchSize, s.batchTimeout)
	res, err := s.client.R().
		SetContext(ctx).
		SetBody(stream).
		SetResult(stream).
		Post("/eventstreams")
	if err != nil || !res.IsSuccess() {
		return nil, ffresty.WrapRestErr(ctx, res, err, coremsgs.MsgTezosconnectRESTErr)
	}
	return stream, nil
}

func (s *streamManager) updateEventStream(ctx context.Context, topic string, batchSize uint, batchTimeout int64, eventStreamID string) (*eventStream, error) {
	stream := buildEventStream(topic, batchSize, batchTimeout)
	res, err := s.client.R().
		SetContext(ctx).
		SetBody(stream).
		SetResult(stream).
		Patch("/eventstreams/" + eventStreamID)
	if err != nil || !res.IsSuccess() {
		return nil, ffresty.WrapRestErr(ctx, res, err, coremsgs.MsgTezosconnectRESTErr)
	}
	return stream, nil
}

func (s *streamManager) ensureEventStream(ctx context.Context, topic string) (*eventStream, error) {
	existingStreams, err := s.getEventStreams(ctx)
	if err != nil {
		return nil, err
	}
	for _, stream := range existingStreams {
		if stream.Name == topic {
			stream, err = s.updateEventStream(ctx, topic, s.batchSize, s.batchTimeout, stream.ID)
			if err != nil {
				return nil, err
			}
			return stream, nil
		}
	}
	return s.createEventStream(ctx, topic)
}

func (s *streamManager) getSubscriptions(ctx context.Context) (subs []*subscription, err error) {
	res, err := s.client.R().
		SetContext(ctx).
		SetResult(&subs).
		Get("/subscriptions")
	if err != nil || !res.IsSuccess() {
		return nil, ffresty.WrapRestErr(ctx, res, err, coremsgs.MsgTezosconnectRESTErr)
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
		return nil, ffresty.WrapRestErr(ctx, res, err, coremsgs.MsgTezosconnectRESTErr)
	}
	return sub, nil
}

func (s *streamManager) createSubscription(ctx context.Context, location *Location, stream, name, event, firstEvent string) (*subscription, error) {
	// Map FireFly "firstEvent" values to Tezos "fromBlock" values
	switch firstEvent {
	case string(core.SubOptsFirstEventOldest):
		firstEvent = "0"
	case string(core.SubOptsFirstEventNewest):
		firstEvent = "latest"
	}

	filters := make([]fftypes.JSONAny, 0)
	filter := eventFilter{
		EventFilter: event,
	}
	filterJSON, _ := json.Marshal(filter)
	filters = append(filters, fftypes.JSONAny(filterJSON))

	sub := subscription{
		Name:      name,
		Stream:    stream,
		Filters:   filters,
		FromBlock: firstEvent,
	}

	if location != nil {
		sub.TezosCompatAddress = location.Address
	}

	res, err := s.client.R().
		SetContext(ctx).
		SetBody(&sub).
		SetResult(&sub).
		Post("/subscriptions")
	if err != nil || !res.IsSuccess() {
		return nil, ffresty.WrapRestErr(ctx, res, err, coremsgs.MsgTezosconnectRESTErr)
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
		return ffresty.WrapRestErr(ctx, res, err, coremsgs.MsgTezosconnectRESTErr)
	}
	return nil
}

func (s *streamManager) ensureFireFlySubscription(ctx context.Context, namespace string, version int, instancePath, firstEvent, stream, event string) (sub *subscription, err error) {
	// Include a hash of the instance path in the subscription, so if we ever point at a different
	// contract configuration, we re-subscribe from block 0.
	// We don't need full strength hashing, so just use the first 16 chars for readability.
	instanceUniqueHash := hex.EncodeToString(sha256.New().Sum([]byte(instancePath)))[0:16]

	existingSubs, err := s.getSubscriptions(ctx)
	if err != nil {
		return nil, err
	}

	v2Name := fmt.Sprintf("%s_%s_%s", namespace, event, instanceUniqueHash)

	for _, s := range existingSubs {
		if s.Stream == stream {
			if s.Name == v2Name {
				return s, nil
			}
		}
	}

	location := &Location{Address: instancePath}
	if sub, err = s.createSubscription(ctx, location, stream, v2Name, event, firstEvent); err != nil {
		return nil, err
	}
	log.L(ctx).Infof("%s subscription: %s", event, sub.ID)
	return sub, nil
}
