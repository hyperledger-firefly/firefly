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

package namespace

import (
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/mocks/spieventsmocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/stretchr/testify/mock"
)

func TestMessageCreated(t *testing.T) {
	mae := &spieventsmocks.Manager{}
	nm := &namespaceManager{
		adminEvents: mae,
	}
	mae.On("Dispatch", mock.Anything).Return()
	nm.OrderedUUIDCollectionNSEvent(database.CollectionMessages, core.ChangeEventTypeCreated, "ns1", fftypes.NewUUID(), 12345)
	mae.AssertExpectations(t)
}

func TestPinCreated(t *testing.T) {
	mae := &spieventsmocks.Manager{}
	nm := &namespaceManager{
		adminEvents: mae,
	}
	mae.On("Dispatch", mock.Anything).Return()
	nm.OrderedCollectionNSEvent(database.CollectionPins, core.ChangeEventTypeCreated, "ns1", 12345)
	mae.AssertExpectations(t)
}

func TestEventCreated(t *testing.T) {
	mae := &spieventsmocks.Manager{}
	nm := &namespaceManager{
		adminEvents: mae,
	}
	mae.On("Dispatch", mock.Anything).Return()
	nm.OrderedUUIDCollectionNSEvent(database.CollectionEvents, core.ChangeEventTypeCreated, "ns1", fftypes.NewUUID(), 12345)
	mae.AssertExpectations(t)
}

func TestSubscriptionCreated(t *testing.T) {
	mae := &spieventsmocks.Manager{}
	nm := &namespaceManager{
		adminEvents: mae,
	}
	mae.On("Dispatch", mock.Anything).Return()
	nm.UUIDCollectionNSEvent(database.CollectionSubscriptions, core.ChangeEventTypeCreated, "ns1", fftypes.NewUUID())
	mae.AssertExpectations(t)
}

func TestSubscriptionUpdated(t *testing.T) {
	mae := &spieventsmocks.Manager{}
	nm := &namespaceManager{
		adminEvents: mae,
	}
	mae.On("Dispatch", mock.Anything).Return()
	nm.UUIDCollectionNSEvent(database.CollectionSubscriptions, core.ChangeEventTypeUpdated, "ns1", fftypes.NewUUID())
	mae.AssertExpectations(t)
}

func TestSubscriptionDeleted(t *testing.T) {
	mae := &spieventsmocks.Manager{}
	nm := &namespaceManager{
		adminEvents: mae,
	}
	mae.On("Dispatch", mock.Anything).Return()
	nm.UUIDCollectionNSEvent(database.CollectionSubscriptions, core.ChangeEventTypeDeleted, "ns1", fftypes.NewUUID())
	mae.AssertExpectations(t)
}

func TestHashCollectionNSEventOk(t *testing.T) {
	mae := &spieventsmocks.Manager{}
	nm := &namespaceManager{
		adminEvents: mae,
	}
	mae.On("Dispatch", mock.Anything).Return()
	nm.HashCollectionNSEvent(database.CollectionGroups, core.ChangeEventTypeDeleted, "ns1", fftypes.NewRandB32())
	mae.AssertExpectations(t)
}
