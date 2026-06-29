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

package sqlcommon

import (
	"context"
	"sync"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/dbsql"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"

	// Import migrate file source
	_ "github.com/golang-migrate/migrate/v4/source/file"
)

type SQLCommon struct {
	dbsql.Database
	capabilities *database.Capabilities
	callbacks    callbacks
}

type callbacks struct {
	writeLock sync.Mutex
	handlers  map[string]database.Callbacks
}

func (cb *callbacks) OrderedUUIDCollectionNSEvent(resType database.OrderedUUIDCollectionNS, eventType core.ChangeEventType, ns string, id *fftypes.UUID, sequence int64) {
	if cb, ok := cb.handlers[ns]; ok {
		cb.OrderedUUIDCollectionNSEvent(resType, eventType, ns, id, sequence)
	}
	if cb, ok := cb.handlers[database.GlobalHandler]; ok {
		cb.OrderedUUIDCollectionNSEvent(resType, eventType, ns, id, sequence)
	}
}

func (cb *callbacks) OrderedCollectionNSEvent(resType database.OrderedCollectionNS, eventType core.ChangeEventType, ns string, sequence int64) {
	if cb, ok := cb.handlers[ns]; ok {
		cb.OrderedCollectionNSEvent(resType, eventType, ns, sequence)
	}
	if cb, ok := cb.handlers[database.GlobalHandler]; ok {
		cb.OrderedCollectionNSEvent(resType, eventType, ns, sequence)
	}
}

func (cb *callbacks) UUIDCollectionNSEvent(resType database.UUIDCollectionNS, eventType core.ChangeEventType, ns string, id *fftypes.UUID) {
	if cb, ok := cb.handlers[ns]; ok {
		cb.UUIDCollectionNSEvent(resType, eventType, ns, id)
	}
	if cb, ok := cb.handlers[database.GlobalHandler]; ok {
		cb.UUIDCollectionNSEvent(resType, eventType, ns, id)
	}
}

func (cb *callbacks) HashCollectionNSEvent(resType database.HashCollectionNS, eventType core.ChangeEventType, ns string, hash *fftypes.Bytes32) {
	if cb, ok := cb.handlers[ns]; ok {
		cb.HashCollectionNSEvent(resType, eventType, ns, hash)
	}
	if cb, ok := cb.handlers[database.GlobalHandler]; ok {
		cb.HashCollectionNSEvent(resType, eventType, ns, hash)
	}
}

func (s *SQLCommon) Init(ctx context.Context, provider dbsql.Provider, config config.Section, capabilities *database.Capabilities) (err error) {
	s.capabilities = capabilities
	return s.Database.Init(ctx, provider, config)
}

func (s *SQLCommon) SetHandler(namespace string, handler database.Callbacks) {
	s.callbacks.writeLock.Lock()
	defer s.callbacks.writeLock.Unlock()
	if s.callbacks.handlers == nil {
		s.callbacks.handlers = make(map[string]database.Callbacks)
	}
	if handler == nil {
		delete(s.callbacks.handlers, namespace)
	} else {
		s.callbacks.handlers[namespace] = handler
	}
}

func (s *SQLCommon) Capabilities() *database.Capabilities { return s.capabilities }
