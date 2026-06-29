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

package namespace

import (
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

func (nm *namespaceManager) OrderedUUIDCollectionNSEvent(resType database.OrderedUUIDCollectionNS, eventType core.ChangeEventType, ns string, id *fftypes.UUID, sequence int64) {
	var ces *int64
	if eventType == core.ChangeEventTypeCreated {
		// Sequence is only provided on create events
		ces = &sequence
	}
	nm.adminEvents.Dispatch(&core.ChangeEvent{
		Collection: string(resType),
		Type:       eventType,
		Namespace:  ns,
		ID:         id,
		Sequence:   ces,
	})
}

func (nm *namespaceManager) OrderedCollectionNSEvent(resType database.OrderedCollectionNS, eventType core.ChangeEventType, ns string, sequence int64) {
	nm.adminEvents.Dispatch(&core.ChangeEvent{
		Collection: string(resType),
		Type:       eventType,
		Namespace:  ns,
		Sequence:   &sequence,
	})
}

func (nm *namespaceManager) UUIDCollectionNSEvent(resType database.UUIDCollectionNS, eventType core.ChangeEventType, ns string, id *fftypes.UUID) {
	nm.adminEvents.Dispatch(&core.ChangeEvent{
		Collection: string(resType),
		Type:       eventType,
		Namespace:  ns,
		ID:         id,
	})
}

func (nm *namespaceManager) HashCollectionNSEvent(resType database.HashCollectionNS, eventType core.ChangeEventType, ns string, hash *fftypes.Bytes32) {
	nm.adminEvents.Dispatch(&core.ChangeEvent{
		Collection: string(resType),
		Type:       eventType,
		Namespace:  ns,
		Hash:       hash,
	})
}
