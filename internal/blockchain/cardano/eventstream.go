// Copyright © 2025 IOG Singapore and SundaeSwap, Inc.
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

package cardano

import (
	"context"
	"fmt"

	"github.com/go-resty/resty/v2"
	"github.com/hyperledger/firefly-common/pkg/ffresty"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly/internal/coremsgs"
)

type streamManager struct {
	client       *resty.Client
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

type listener struct {
	ID   string `json:"id"`
	Name string `json:"name,omitempty"`
}

type filter struct {
	Event eventfilter `json:"event"`
}

type eventfilter struct {
	Contract  string `json:"contract"`
	EventPath string `json:"eventPath"`
}

func newStreamManager(client *resty.Client, batchSize uint, batchTimeout int64) *streamManager {
	return &streamManager{
		client:       client,
		batchSize:    batchSize,
		batchTimeout: batchTimeout,
	}
}

func (s *streamManager) getEventStreams(ctx context.Context) (streams []*eventStream, err error) {
	res, err := s.client.R().
		SetContext(ctx).
		SetResult(&streams).
		Get("/api/v1/eventstreams")
	if err != nil || !res.IsSuccess() {
		return nil, ffresty.WrapRestErr(ctx, res, err, coremsgs.MsgCardanoconnectRESTErr)
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
		Post("/api/v1/eventstreams")
	if err != nil || !res.IsSuccess() {
		return nil, ffresty.WrapRestErr(ctx, res, err, coremsgs.MsgCardanoconnectRESTErr)
	}
	return stream, nil
}

func (s *streamManager) updateEventStream(ctx context.Context, topic string, batchSize uint, batchTimeout int64, eventStreamID string) (*eventStream, error) {
	stream := buildEventStream(topic, batchSize, batchTimeout)
	res, err := s.client.R().
		SetContext(ctx).
		SetBody(stream).
		SetResult(stream).
		Patch("/api/v1/eventstreams/" + eventStreamID)
	if err != nil || !res.IsSuccess() {
		return nil, ffresty.WrapRestErr(ctx, res, err, coremsgs.MsgCardanoconnectRESTErr)
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

func (s *streamManager) getListener(ctx context.Context, streamID string, listenerID string, okNotFound bool) (listener *listener, err error) {
	res, err := s.client.R().
		SetContext(ctx).
		SetResult(&listener).
		Get(fmt.Sprintf("/api/v1/eventstreams/%s/listeners/%s", streamID, listenerID))
	if err != nil || !res.IsSuccess() {
		if okNotFound && res.StatusCode() == 404 {
			return nil, nil
		}
		return nil, ffresty.WrapRestErr(ctx, res, err, coremsgs.MsgCardanoconnectRESTErr)
	}
	return listener, nil
}

func (s *streamManager) getListeners(ctx context.Context, streamID string) (listeners *[]listener, err error) {
	res, err := s.client.R().
		SetContext(ctx).
		SetResult(&listeners).
		Get(fmt.Sprintf("/api/v1/eventstreams/%s/listeners", streamID))
	if err != nil || !res.IsSuccess() {
		return nil, ffresty.WrapRestErr(ctx, res, err, coremsgs.MsgCardanoconnectRESTErr)
	}
	return listeners, nil
}

func (s *streamManager) createListener(ctx context.Context, streamID, name, lastEvent string, filters []filter) (listener *listener, err error) {
	body := map[string]interface{}{
		"name":      name,
		"type":      "events",
		"fromBlock": lastEvent,
		"filters":   filters,
	}

	res, err := s.client.R().
		SetContext(ctx).
		SetBody(body).
		SetResult(&listener).
		Post(fmt.Sprintf("/api/v1/eventstreams/%s/listeners", streamID))

	if err != nil || !res.IsSuccess() {
		return nil, ffresty.WrapRestErr(ctx, res, err, coremsgs.MsgCardanoconnectRESTErr)
	}

	return listener, nil
}

func (s *streamManager) deleteListener(ctx context.Context, streamID, listenerID string) error {
	res, err := s.client.R().
		SetContext(ctx).
		Delete(fmt.Sprintf("/api/v1/eventstreams/%s/listeners/%s", streamID, listenerID))

	if err != nil || !res.IsSuccess() {
		return ffresty.WrapRestErr(ctx, res, err, coremsgs.MsgCardanoconnectRESTErr)
	}
	return nil
}

func (s *streamManager) ensureFireFlyListener(ctx context.Context, namespace string, version int, address, firstEvent, streamID string) (l *listener, err error) {
	existingListeners, err := s.getListeners(ctx, streamID)
	if err != nil {
		return nil, err
	}

	name := fmt.Sprintf("%s_%d_BatchPin", namespace, version)
	for _, l := range *existingListeners {
		if l.Name == name {
			return &l, nil
		}
	}

	filters := []filter{{
		eventfilter{
			Contract:  address,
			EventPath: "BatchPin",
		},
	}}
	if l, err = s.createListener(ctx, streamID, name, firstEvent, filters); err != nil {
		return nil, err
	}
	log.L(ctx).Infof("BatchPin subscription: %s", l.ID)
	return l, nil
}
