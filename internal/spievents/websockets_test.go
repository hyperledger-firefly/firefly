// Copyright © 2022 Kaleido, Inc.
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

package spievents

import (
	"context"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
)

func TestWriteFail(t *testing.T) {
	_, ws, _, cancel := newTestSPIEventsManager(t)
	defer cancel()

	// Close socket that will break receiver loop, and wait for sender to exit
	ws.wsConn.Close()
	<-ws.senderDone

	// Write some bad data - should swallow the error
	ws.writeObject(map[bool]bool{false: true})

}

func TestBlockedDispatch(t *testing.T) {
	ws := &webSocket{
		ctx:     context.Background(),
		events:  make(chan *core.ChangeEvent, 1),
		manager: &adminEventManager{},
	}
	// Should not block us, and will warn
	ws.dispatch(&core.ChangeEvent{})
	ws.dispatch(&core.ChangeEvent{})
	ws.dispatch(&core.ChangeEvent{})
	// Should unblock if we free up
	<-ws.events
	ws.dispatch(&core.ChangeEvent{})
	<-ws.events
}

func TestBlockedConsume(t *testing.T) {
	_, ws, wsc, cancel := newTestSPIEventsManager(t)
	defer cancel()

	ws.mux.Lock()
	ws.collections = []string{"collection1"}
	ws.blocked = &core.ChangeEvent{
		Type:         core.ChangeEventTypeDropped,
		DroppedSince: fftypes.Now(),
		DroppedCount: 1,
	}
	ws.mux.Unlock()

	// Dispatch an event - will be successful as we faked the block
	ws.dispatch(&core.ChangeEvent{
		Type:       core.ChangeEventTypeCreated,
		Collection: "collection1",
		Namespace:  "ns1",
		ID:         fftypes.NewUUID(),
	})

	msg1 := <-wsc.Receive()
	event1 := unmarshalChangeEvent(t, msg1)
	assert.Equal(t, core.ChangeEventTypeDropped, event1.Type)
	assert.Equal(t, int64(1), event1.DroppedCount)

	msg2 := <-wsc.Receive()
	event2 := unmarshalChangeEvent(t, msg2)
	assert.Equal(t, core.ChangeEventTypeCreated, event2.Type)
}
