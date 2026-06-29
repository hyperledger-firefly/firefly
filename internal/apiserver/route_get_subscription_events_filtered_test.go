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

package apiserver

import (
	"net/http/httptest"
	"testing"

	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestGetSubscriptionEventsFiltered(t *testing.T) {
	o, r := newTestAPIServer()
	o.On("Authorize", mock.Anything, mock.Anything).Return(nil)
	req := httptest.NewRequest("GET", "/api/v1/namespaces/mynamespace/subscriptions/abcd12345/events?startsequence=100&endsequence=200", nil)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	res := httptest.NewRecorder()

	o.On("GetSubscriptionByID", mock.Anything, "abcd12345").
		Return(&core.Subscription{}, nil)
	o.On("GetSubscriptionEventsHistorical", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return([]*core.EnrichedEvent{}, nil, nil)

	r.ServeHTTP(res, req)
	assert.Equal(t, 200, res.Result().StatusCode)
}

func TestGetSubscriptionEventsFilteredStartSequenceIDDoesNotParse(t *testing.T) {
	o, r := newTestAPIServer()
	o.On("Authorize", mock.Anything, mock.Anything).Return(nil)
	req := httptest.NewRequest("GET", "/api/v1/namespaces/mynamespace/subscriptions/abcd12345/events?startsequence=helloworld", nil)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	res := httptest.NewRecorder()
	o.On("GetSubscriptionByID", mock.Anything, "abcd12345").
		Return(&core.Subscription{}, nil)

	r.ServeHTTP(res, req)
	assert.Equal(t, 400, res.Result().StatusCode)
	assert.Contains(t, res.Body.String(), "helloworld")
}

func TestGetSubscriptionEventsFilteredEndSequenceIDDoesNotParse(t *testing.T) {
	o, r := newTestAPIServer()
	o.On("Authorize", mock.Anything, mock.Anything).Return(nil)
	req := httptest.NewRequest("GET", "/api/v1/namespaces/mynamespace/subscriptions/abcd12345/events?endsequence=helloworld", nil)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	res := httptest.NewRecorder()
	o.On("GetSubscriptionByID", mock.Anything, "abcd12345").
		Return(&core.Subscription{}, nil)

	r.ServeHTTP(res, req)
	assert.Equal(t, 400, res.Result().StatusCode)
	assert.Contains(t, res.Body.String(), "helloworld")
}

func TestGetSubscriptionEventsFilteredNoSequenceIDsProvided(t *testing.T) {
	o, r := newTestAPIServer()
	o.On("Authorize", mock.Anything, mock.Anything).Return(nil)
	req := httptest.NewRequest("GET", "/api/v1/namespaces/mynamespace/subscriptions/abcd12345/events", nil)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	res := httptest.NewRecorder()
	o.On("GetSubscriptionByID", mock.Anything, "abcd12345").
		Return(&core.Subscription{}, nil)
	o.On("GetSubscriptionEventsHistorical", mock.Anything, mock.Anything, mock.Anything, -1, -1).
		Return([]*core.EnrichedEvent{}, nil, nil)

	r.ServeHTTP(res, req)
	assert.Equal(t, 200, res.Result().StatusCode)
}
