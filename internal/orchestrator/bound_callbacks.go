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

package orchestrator

import (
	"context"
	"sync"

	"github.com/hyperledger-firefly/common/pkg/log"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/blockchain"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/dataexchange"
	"github.com/hyperledger-firefly/firefly/pkg/tokens"
)

type boundCallbacks struct {
	sync.Mutex
	o *orchestrator
}

func (bc *boundCallbacks) checkStopped() error {
	if !bc.o.isStarted() {
		return i18n.NewError(bc.o.ctx, coremsgs.MsgNamespaceNotStarted, bc.o.namespace.Name)
	}
	return nil
}

func (bc *boundCallbacks) BulkOperationUpdates(ctx context.Context, updates []*core.OperationUpdate) error {
	return bc.o.operations.SubmitBulkOperationUpdates(ctx, updates)
}

func (bc *boundCallbacks) OperationUpdate(update *core.OperationUpdateAsync) {
	bc.o.operations.SubmitOperationUpdate(update)
}

func (bc *boundCallbacks) SharedStorageBatchDownloaded(payloadRef string, data []byte) (*fftypes.UUID, error) {
	if err := bc.checkStopped(); err != nil {
		return nil, err
	}
	return bc.o.events.SharedStorageBatchDownloaded(bc.o.sharedstorage(), payloadRef, data)
}

func (bc *boundCallbacks) SharedStorageBlobDownloaded(hash fftypes.Bytes32, size int64, payloadRef string, dataID *fftypes.UUID) error {
	if err := bc.checkStopped(); err != nil {
		return err
	}
	return bc.o.events.SharedStorageBlobDownloaded(bc.o.sharedstorage(), hash, size, payloadRef, dataID)
}

func (bc *boundCallbacks) BlockchainEventBatch(batch []*blockchain.EventToDispatch) error {
	if err := bc.checkStopped(); err != nil {
		return err
	}
	return bc.o.events.BlockchainEventBatch(batch)
}

func (bc *boundCallbacks) DXEvent(plugin dataexchange.Plugin, event dataexchange.DXEvent) error {
	if err := bc.checkStopped(); err != nil {
		return err
	}
	return bc.o.events.DXEvent(plugin, event)
}

func (bc *boundCallbacks) TokenPoolCreated(ctx context.Context, plugin tokens.Plugin, pool *tokens.TokenPool) error {
	if err := bc.checkStopped(); err != nil {
		return err
	}
	return bc.o.events.TokenPoolCreated(ctx, plugin, pool)
}

func (bc *boundCallbacks) TokensTransferred(plugin tokens.Plugin, transfer *tokens.TokenTransfer) error {
	if err := bc.checkStopped(); err != nil {
		return err
	}
	return bc.o.events.TokensTransferred(plugin, transfer)
}

func (bc *boundCallbacks) TokensApproved(plugin tokens.Plugin, approval *tokens.TokenApproval) error {
	if err := bc.checkStopped(); err != nil {
		return err
	}
	return bc.o.events.TokensApproved(plugin, approval)
}

func (bc *boundCallbacks) DXConnect(plugin dataexchange.Plugin) {
	err := bc.checkStopped()
	if err == nil {
		err = bc.o.networkmap.CheckNodeIdentityStatus(bc.o.ctx)
	}
	if err != nil {
		log.L(bc.o.ctx).Errorf("Error handling DX connect callback: %s", err)
	}
}
