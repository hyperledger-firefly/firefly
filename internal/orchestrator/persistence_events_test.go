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

package orchestrator

import (
	"context"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/mocks/batchmocks"
	"github.com/hyperledger-firefly/firefly/mocks/eventmocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

func TestMessageCreated(t *testing.T) {
	mb := &batchmocks.Manager{}
	o := &orchestrator{
		namespace: &core.Namespace{Name: "ns1", NetworkName: "ns1"},
		batch:     mb,
	}
	mb.On("NewMessages").Return((chan<- int64)(make(chan int64, 1)))
	o.OrderedUUIDCollectionNSEvent(database.CollectionMessages, core.ChangeEventTypeCreated, "ns1", fftypes.NewUUID(), 12345)
	mb.AssertExpectations(t)
}

func TestPinCreated(t *testing.T) {
	mem := &eventmocks.EventManager{}
	o := &orchestrator{
		namespace: &core.Namespace{Name: "ns1", NetworkName: "ns1"},
		events:    mem,
	}
	mem.On("NewPins").Return((chan<- int64)(make(chan int64, 1)))
	o.OrderedCollectionNSEvent(database.CollectionPins, core.ChangeEventTypeCreated, "ns1", 12345)
	mem.AssertExpectations(t)
}

func TestEventCreated(t *testing.T) {
	mem := &eventmocks.EventManager{}
	o := &orchestrator{
		namespace: &core.Namespace{Name: "ns1", NetworkName: "ns1"},
		events:    mem,
	}
	mem.On("NewEvents").Return((chan<- int64)(make(chan int64, 1)))
	o.OrderedUUIDCollectionNSEvent(database.CollectionEvents, core.ChangeEventTypeCreated, "ns1", fftypes.NewUUID(), 12345)
	mem.AssertExpectations(t)
}

func TestSubscriptionCreated(t *testing.T) {
	mem := &eventmocks.EventManager{}
	o := &orchestrator{
		namespace: &core.Namespace{Name: "ns1", NetworkName: "ns1"},
		events:    mem,
	}
	mem.On("NewSubscriptions").Return((chan<- *fftypes.UUID)(make(chan *fftypes.UUID, 1)))
	o.UUIDCollectionNSEvent(database.CollectionSubscriptions, core.ChangeEventTypeCreated, "ns1", fftypes.NewUUID())
	mem.AssertExpectations(t)
}

func TestSubscriptionUpdated(t *testing.T) {
	mem := &eventmocks.EventManager{}
	o := &orchestrator{
		namespace: &core.Namespace{Name: "ns1", NetworkName: "ns1"},
		events:    mem,
	}
	mem.On("SubscriptionUpdates").Return((chan<- *fftypes.UUID)(make(chan *fftypes.UUID, 1)))
	o.UUIDCollectionNSEvent(database.CollectionSubscriptions, core.ChangeEventTypeUpdated, "ns1", fftypes.NewUUID())
	mem.AssertExpectations(t)
}

func TestSubscriptionDeleted(t *testing.T) {
	mem := &eventmocks.EventManager{}
	o := &orchestrator{
		namespace: &core.Namespace{Name: "ns1", NetworkName: "ns1"},
		events:    mem,
	}
	mem.On("DeletedSubscriptions").Return((chan<- *fftypes.UUID)(make(chan *fftypes.UUID, 1)))
	o.UUIDCollectionNSEvent(database.CollectionSubscriptions, core.ChangeEventTypeDeleted, "ns1", fftypes.NewUUID())
	mem.AssertExpectations(t)
}

func TestGroupCreatedNOOP(t *testing.T) {
	mem := &eventmocks.EventManager{}
	o := &orchestrator{
		namespace: &core.Namespace{Name: "ns1", NetworkName: "ns1"},
		events:    mem,
	}
	o.HashCollectionNSEvent(database.CollectionGroups, core.ChangeEventTypeCreated, "ns1", fftypes.NewRandB32())
	mem.AssertExpectations(t)
}

func TestOrderedUUIDCollectionWrongNS(t *testing.T) {
	o := &orchestrator{
		ctx:       context.Background(),
		namespace: &core.Namespace{Name: "ns1", NetworkName: "ns1"},
	}
	o.OrderedUUIDCollectionNSEvent(database.CollectionMessages, core.ChangeEventTypeCreated, "ns2", fftypes.NewUUID(), 1)
}

func TestOrderedCollectionWrongNS(t *testing.T) {
	o := &orchestrator{
		ctx:       context.Background(),
		namespace: &core.Namespace{Name: "ns1", NetworkName: "ns1"},
	}
	o.OrderedCollectionNSEvent(database.CollectionPins, core.ChangeEventTypeCreated, "ns2", 1)
}

func TestUUIDCollectionWrongNS(t *testing.T) {
	o := &orchestrator{
		ctx:       context.Background(),
		namespace: &core.Namespace{Name: "ns1", NetworkName: "ns1"},
	}
	o.UUIDCollectionNSEvent(database.CollectionSubscriptions, core.ChangeEventTypeCreated, "ns2", fftypes.NewUUID())
}
