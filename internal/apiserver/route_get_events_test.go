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

package apiserver

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestGetEvents(t *testing.T) {
	o, r := newTestAPIServer()
	o.On("Authorize", mock.Anything, mock.Anything).Return(nil)
	req := httptest.NewRequest("GET", "/api/v1/namespaces/mynamespace/events", nil)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	res := httptest.NewRecorder()

	o.On("GetEvents", mock.Anything, mock.Anything).
		Return([]*core.Event{}, nil, nil)
	r.ServeHTTP(res, req)

	assert.Equal(t, 200, res.Result().StatusCode)
}

func TestGetEventsWithReferences(t *testing.T) {
	o, r := newTestAPIServer()
	o.On("Authorize", mock.Anything, mock.Anything).Return(nil)
	req := httptest.NewRequest("GET", "/api/v1/namespaces/mynamespace/events?fetchreferences", nil)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	res := httptest.NewRecorder()

	var ten int64 = 10
	o.On("GetEventsWithReferences", mock.Anything, mock.Anything).
		Return([]*core.EnrichedEvent{}, &ffapi.FilterResult{
			TotalCount: &ten,
		}, nil)
	r.ServeHTTP(res, req)

	assert.Equal(t, 200, res.Result().StatusCode)
	var resWithCount ffapi.FilterResultsWithCount
	err := json.NewDecoder(res.Body).Decode(&resWithCount)
	assert.NoError(t, err)
	assert.NotNil(t, resWithCount.Items)
	assert.Equal(t, int64(0), resWithCount.Count)
	assert.Equal(t, int64(10), *resWithCount.Total)
}

func TestGetEventsWithFetchReference(t *testing.T) {
	o, r := newTestAPIServer()
	o.On("Authorize", mock.Anything, mock.Anything).Return(nil)
	req := httptest.NewRequest("GET", "/api/v1/namespaces/mynamespace/events?fetchreference", nil)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	res := httptest.NewRecorder()

	var ten int64 = 10
	o.On("GetEventsWithReferences", mock.Anything, mock.Anything).
		Return([]*core.EnrichedEvent{}, &ffapi.FilterResult{
			TotalCount: &ten,
		}, nil)
	r.ServeHTTP(res, req)

	assert.Equal(t, 200, res.Result().StatusCode)
	var resWithCount ffapi.FilterResultsWithCount
	err := json.NewDecoder(res.Body).Decode(&resWithCount)
	assert.NoError(t, err)
	assert.NotNil(t, resWithCount.Items)
	assert.Equal(t, int64(0), resWithCount.Count)
	assert.Equal(t, int64(10), *resWithCount.Total)
}
