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

package events

import (
	"context"

	"github.com/hyperledger-firefly/firefly/internal/data"
	"github.com/hyperledger-firefly/firefly/internal/operations"
	"github.com/hyperledger-firefly/firefly/internal/txcommon"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

type eventEnricher struct {
	namespace  string
	data       data.Manager
	database   database.Plugin
	operations operations.Manager
	txHelper   txcommon.Helper
}

func newEventEnricher(ns string, di database.Plugin, dm data.Manager, om operations.Manager, txHelper txcommon.Helper) *eventEnricher {
	return &eventEnricher{
		namespace:  ns,
		data:       dm,
		database:   di,
		operations: om,
		txHelper:   txHelper,
	}
}

func (em *eventEnricher) enrichEvents(ctx context.Context, events []*core.Event) ([]*core.EnrichedEvent, error) {
	enriched := make([]*core.EnrichedEvent, len(events))
	for i, event := range events {
		enrichedEvent, err := em.enrichEvent(ctx, event)
		if err != nil {
			return nil, err
		}
		enriched[i] = enrichedEvent
	}
	return enriched, nil
}

func (em *eventEnricher) enrichEvent(ctx context.Context, event *core.Event) (*core.EnrichedEvent, error) {
	e := &core.EnrichedEvent{
		Event: *event,
	}

	switch event.Type {
	case core.EventTypeTransactionSubmitted:
		tx, err := em.txHelper.GetTransactionByIDCached(ctx, event.Reference)
		if err != nil {
			return nil, err
		}
		e.Transaction = tx
	case core.EventTypeMessageConfirmed, core.EventTypeMessageRejected:
		msg, _, _, err := em.data.GetMessageWithDataCached(ctx, event.Reference)
		if err != nil {
			return nil, err
		}
		e.Message = msg
	case core.EventTypeBlockchainEventReceived:
		be, err := em.txHelper.GetBlockchainEventByIDCached(ctx, event.Reference)
		if err != nil {
			return nil, err
		}
		e.BlockchainEvent = be
	case core.EventTypeContractAPIConfirmed:
		contractAPI, err := em.database.GetContractAPIByID(ctx, em.namespace, event.Reference)
		if err != nil {
			return nil, err
		}
		e.ContractAPI = contractAPI
	case core.EventTypeContractInterfaceConfirmed:
		contractInterface, err := em.database.GetFFIByID(ctx, em.namespace, event.Reference)
		if err != nil {
			return nil, err
		}
		e.ContractInterface = contractInterface
	case core.EventTypeDatatypeConfirmed:
		dt, err := em.database.GetDatatypeByID(ctx, em.namespace, event.Reference)
		if err != nil {
			return nil, err
		}
		e.Datatype = dt
	case core.EventTypeIdentityConfirmed, core.EventTypeIdentityUpdated:
		identity, err := em.database.GetIdentityByID(ctx, em.namespace, event.Reference)
		if err != nil {
			return nil, err
		}
		e.Identity = identity
	case core.EventTypePoolConfirmed:
		tokenPool, err := em.database.GetTokenPoolByID(ctx, em.namespace, event.Reference)
		if err != nil {
			return nil, err
		}
		e.TokenPool = tokenPool
	case core.EventTypeApprovalConfirmed:
		approval, err := em.database.GetTokenApprovalByID(ctx, em.namespace, event.Reference)
		if err != nil {
			return nil, err
		}
		e.TokenApproval = approval
	case core.EventTypeTransferConfirmed:
		transfer, err := em.database.GetTokenTransferByID(ctx, em.namespace, event.Reference)
		if err != nil {
			return nil, err
		}
		e.TokenTransfer = transfer
	case core.EventTypeApprovalOpFailed,
		core.EventTypeTransferOpFailed,
		core.EventTypePoolOpFailed,
		core.EventTypeBlockchainInvokeOpFailed,
		core.EventTypeBlockchainInvokeOpSucceeded,
		core.EventTypeBlockchainContractDeployOpFailed,
		core.EventTypeBlockchainContractDeployOpSucceeded:
		operation, err := em.operations.GetOperationByIDCached(ctx, event.Reference)
		if err != nil {
			return nil, err
		}
		e.Operation = operation
	}
	return e, nil
}
