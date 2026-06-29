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

package networkmap

import (
	"context"
	"errors"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/mocks/dataexchangemocks"
	"github.com/hyperledger-firefly/firefly/mocks/identitymanagermocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
)

func TestCheckNodeIdentityStatusSuccess(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	ctx := context.TODO()

	aProfile := fftypes.JSONAnyPtr(`{"cert": "a-cert" }`).JSONObject()
	node := &core.Identity{
		IdentityProfile: core.IdentityProfile{
			Profile: aProfile,
		},
	}
	nm.identity.(*identitymanagermocks.Manager).On("GetLocalNode", ctx).Return(node, nil)
	nm.exchange.(*dataexchangemocks.Plugin).On("CheckNodeIdentityStatus", ctx, node).Return(nil)

	err := nm.CheckNodeIdentityStatus(context.TODO())
	assert.NoError(t, err)

	nm.identity.(*identitymanagermocks.Manager).AssertExpectations(t)
	nm.exchange.(*dataexchangemocks.Plugin).AssertExpectations(t)
}

func TestCheckNodeIdentityStatusLocalNodeFails(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	ctx := context.TODO()

	nm.identity.(*identitymanagermocks.Manager).On("GetLocalNode", ctx).Return(&core.Identity{}, errors.New("local node not set"))

	err := nm.CheckNodeIdentityStatus(ctx)
	assert.Error(t, err)

	nm.identity.(*identitymanagermocks.Manager).AssertExpectations(t)
}

func TestCheckNodeIdentityStatusFails(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()
	ctx := context.TODO()

	aProfile := fftypes.JSONAnyPtr(`{"cert": "a-cert" }`).JSONObject()
	node := &core.Identity{
		IdentityProfile: core.IdentityProfile{
			Profile: aProfile,
		},
	}
	nm.identity.(*identitymanagermocks.Manager).On("GetLocalNode", ctx).Return(node, nil)
	nm.exchange.(*dataexchangemocks.Plugin).On("CheckNodeIdentityStatus", ctx, node).Return(errors.New("identity status check failed"))

	err := nm.CheckNodeIdentityStatus(context.TODO())
	assert.Error(t, err)

	nm.identity.(*identitymanagermocks.Manager).AssertExpectations(t)
	nm.exchange.(*dataexchangemocks.Plugin).AssertExpectations(t)
}
