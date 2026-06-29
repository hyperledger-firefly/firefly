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

package events

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/internal/cache"
	"github.com/hyperledger-firefly/firefly/internal/txcommon"
	"github.com/hyperledger-firefly/firefly/mocks/cachemocks"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/mocks/datamocks"
	"github.com/hyperledger-firefly/firefly/mocks/operationmocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func newTestEventEnricher() *eventEnricher {
	mdi := &databasemocks.Plugin{}
	mdm := &datamocks.Manager{}
	mom := &operationmocks.Manager{}
	ctx := context.Background()
	cmi := &cachemocks.Manager{}
	cmi.On("GetCache", mock.Anything).Return(cache.NewUmanagedCache(ctx, 100, 5*time.Minute), nil)
	txHelper, _ := txcommon.NewTransactionHelper(ctx, "ns1", mdi, mdm, cmi)
	return newEventEnricher("ns1", mdi, mdm, mom, txHelper)
}

func TestEnrichMessageConfirmed(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	ctx := context.Background()

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()

	// Setup enrichment
	em.mdm.On("GetMessageWithDataCached", mock.Anything, ref1).Return(&core.Message{
		Header: core.MessageHeader{ID: ref1},
	}, nil, true, nil)

	event := &core.Event{
		ID:        ev1,
		Type:      core.EventTypeMessageConfirmed,
		Reference: ref1,
	}

	enriched, err := em.EnrichEvent(ctx, event)
	assert.NoError(t, err)
	assert.Equal(t, ref1, enriched.Message.Header.ID)
}

func TestEnrichEventsMessageConfirmed(t *testing.T) {
	em := newTestEventManager(t)
	defer em.cleanup(t)
	ctx := context.Background()

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()

	// Setup enrichment
	em.mdm.On("GetMessageWithDataCached", mock.Anything, ref1).Return(&core.Message{
		Header: core.MessageHeader{ID: ref1},
	}, nil, true, nil)

	event := []*core.Event{
		{
			ID:        ev1,
			Type:      core.EventTypeMessageConfirmed,
			Reference: ref1,
		},
	}

	enriched, err := em.EnrichEvents(ctx, event)
	assert.NoError(t, err)
	assert.Equal(t, ref1, enriched[0].Message.Header.ID)
}

func TestEnrichMessageFail(t *testing.T) {
	em := newTestEventEnricher()
	ctx := context.Background()

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()

	// Setup enrichment
	mdm := em.data.(*datamocks.Manager)
	mdm.On("GetMessageWithDataCached", mock.Anything, ref1).Return(nil, nil, false, fmt.Errorf("pop"))

	event := &core.Event{
		ID:        ev1,
		Type:      core.EventTypeMessageConfirmed,
		Reference: ref1,
	}

	_, err := em.enrichEvent(ctx, event)
	assert.EqualError(t, err, "pop")
}

func TestEnrichMessageRejected(t *testing.T) {
	em := newTestEventEnricher()
	ctx := context.Background()

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()

	// Setup enrichment
	mdm := em.data.(*datamocks.Manager)
	mdm.On("GetMessageWithDataCached", mock.Anything, ref1).Return(&core.Message{
		Header: core.MessageHeader{ID: ref1},
	}, nil, true, nil)

	event := &core.Event{
		ID:        ev1,
		Type:      core.EventTypeMessageRejected,
		Reference: ref1,
	}

	enriched, err := em.enrichEvent(ctx, event)
	assert.NoError(t, err)
	assert.Equal(t, ref1, enriched.Message.Header.ID)
}

func TestEnrichTxSubmitted(t *testing.T) {
	em := newTestEventEnricher()
	ctx := context.Background()

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()

	// Setup enrichment
	mdi := em.database.(*databasemocks.Plugin)
	mdi.On("GetTransactionByID", mock.Anything, "ns1", ref1).Return(&core.Transaction{
		ID: ref1,
	}, nil)

	event := &core.Event{
		ID:        ev1,
		Type:      core.EventTypeTransactionSubmitted,
		Reference: ref1,
	}

	enriched, err := em.enrichEvent(ctx, event)
	assert.NoError(t, err)
	assert.Equal(t, ref1, enriched.Transaction.ID)
}

func TestEnrichTxFail(t *testing.T) {
	em := newTestEventEnricher()
	ctx := context.Background()

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()

	// Setup enrichment
	mdi := em.database.(*databasemocks.Plugin)
	mdi.On("GetTransactionByID", mock.Anything, "ns1", ref1).Return(nil, fmt.Errorf("pop"))

	event := &core.Event{
		ID:        ev1,
		Type:      core.EventTypeTransactionSubmitted,
		Reference: ref1,
	}

	_, err := em.enrichEvent(ctx, event)
	assert.EqualError(t, err, "pop")
}

func TestEnrichBlockchainEventSubmitted(t *testing.T) {
	em := newTestEventEnricher()
	ctx := context.Background()

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()

	// Setup enrichment
	mdi := em.database.(*databasemocks.Plugin)
	mdi.On("GetBlockchainEventByID", mock.Anything, "ns1", ref1).Return(&core.BlockchainEvent{
		ID: ref1,
	}, nil)

	event := &core.Event{
		ID:        ev1,
		Type:      core.EventTypeBlockchainEventReceived,
		Reference: ref1,
	}

	enriched, err := em.enrichEvent(ctx, event)
	assert.NoError(t, err)
	assert.Equal(t, ref1, enriched.BlockchainEvent.ID)
}

func TestEnrichBlockchainEventFail(t *testing.T) {
	em := newTestEventEnricher()
	ctx := context.Background()

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()

	// Setup enrichment
	mdi := em.database.(*databasemocks.Plugin)
	mdi.On("GetBlockchainEventByID", mock.Anything, "ns1", ref1).Return(nil, fmt.Errorf("pop"))

	event := &core.Event{
		ID:        ev1,
		Type:      core.EventTypeBlockchainEventReceived,
		Reference: ref1,
	}

	_, err := em.enrichEvent(ctx, event)
	assert.EqualError(t, err, "pop")
}

func TestEnrichContractAPISubmitted(t *testing.T) {
	em := newTestEventEnricher()
	ctx := context.Background()

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()

	// Setup enrichment
	mdi := em.database.(*databasemocks.Plugin)
	mdi.On("GetContractAPIByID", mock.Anything, "ns1", ref1).Return(&core.ContractAPI{
		ID: ref1,
	}, nil)

	event := &core.Event{
		ID:        ev1,
		Type:      core.EventTypeContractAPIConfirmed,
		Reference: ref1,
	}

	enriched, err := em.enrichEvent(ctx, event)
	assert.NoError(t, err)
	assert.Equal(t, ref1, enriched.ContractAPI.ID)
}

func TestEnrichContractAPItFail(t *testing.T) {
	em := newTestEventEnricher()
	ctx := context.Background()

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()

	// Setup enrichment
	mdi := em.database.(*databasemocks.Plugin)
	mdi.On("GetContractAPIByID", mock.Anything, "ns1", ref1).Return(nil, fmt.Errorf("pop"))

	event := &core.Event{
		ID:        ev1,
		Type:      core.EventTypeContractAPIConfirmed,
		Reference: ref1,
	}

	_, err := em.enrichEvent(ctx, event)
	assert.EqualError(t, err, "pop")
}

func TestEnrichContractInterfaceSubmitted(t *testing.T) {
	em := newTestEventEnricher()
	ctx := context.Background()

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()

	// Setup enrichment
	mdi := em.database.(*databasemocks.Plugin)
	mdi.On("GetFFIByID", mock.Anything, "ns1", ref1).Return(&fftypes.FFI{
		ID: ref1,
	}, nil)

	event := &core.Event{
		ID:        ev1,
		Type:      core.EventTypeContractInterfaceConfirmed,
		Reference: ref1,
	}

	enriched, err := em.enrichEvent(ctx, event)
	assert.NoError(t, err)
	assert.Equal(t, ref1, enriched.ContractInterface.ID)
}

func TestEnrichContractInterfacetFail(t *testing.T) {
	em := newTestEventEnricher()
	ctx := context.Background()

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()

	// Setup enrichment
	mdi := em.database.(*databasemocks.Plugin)
	mdi.On("GetFFIByID", mock.Anything, "ns1", ref1).Return(nil, fmt.Errorf("pop"))

	event := &core.Event{
		ID:        ev1,
		Type:      core.EventTypeContractInterfaceConfirmed,
		Reference: ref1,
	}

	_, err := em.enrichEvent(ctx, event)
	assert.EqualError(t, err, "pop")
}

func TestEnrichDatatypeConfirmed(t *testing.T) {
	em := newTestEventEnricher()
	ctx := context.Background()

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()

	// Setup enrichment
	mdi := em.database.(*databasemocks.Plugin)
	mdi.On("GetDatatypeByID", mock.Anything, "ns1", ref1).Return(&core.Datatype{
		ID: ref1,
	}, nil)

	event := &core.Event{
		ID:        ev1,
		Type:      core.EventTypeDatatypeConfirmed,
		Reference: ref1,
	}

	enriched, err := em.enrichEvent(ctx, event)
	assert.NoError(t, err)
	assert.Equal(t, ref1, enriched.Datatype.ID)
}

func TestEnrichDatatypeConfirmedFail(t *testing.T) {
	em := newTestEventEnricher()
	ctx := context.Background()

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()

	// Setup enrichment
	mdi := em.database.(*databasemocks.Plugin)
	mdi.On("GetDatatypeByID", mock.Anything, "ns1", ref1).Return(nil, fmt.Errorf("pop"))

	event := &core.Event{
		ID:        ev1,
		Type:      core.EventTypeDatatypeConfirmed,
		Reference: ref1,
	}

	_, err := em.enrichEvent(ctx, event)
	assert.EqualError(t, err, "pop")
}

func TestEnrichIdentityConfirmed(t *testing.T) {
	em := newTestEventEnricher()
	ctx := context.Background()

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()

	// Setup enrichment
	mdi := em.database.(*databasemocks.Plugin)
	mdi.On("GetIdentityByID", mock.Anything, "ns1", ref1).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			ID: ref1,
		},
	}, nil)

	event := &core.Event{
		ID:        ev1,
		Type:      core.EventTypeIdentityConfirmed,
		Reference: ref1,
	}

	enriched, err := em.enrichEvent(ctx, event)
	assert.NoError(t, err)
	assert.Equal(t, ref1, enriched.Identity.IdentityBase.ID)
}

func TestEnrichIdentityConfirmedFail(t *testing.T) {
	em := newTestEventEnricher()
	ctx := context.Background()

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()

	// Setup enrichment
	mdi := em.database.(*databasemocks.Plugin)
	mdi.On("GetIdentityByID", mock.Anything, "ns1", ref1).Return(nil, fmt.Errorf("pop"))

	event := &core.Event{
		ID:        ev1,
		Type:      core.EventTypeIdentityConfirmed,
		Reference: ref1,
	}

	_, err := em.enrichEvent(ctx, event)
	assert.EqualError(t, err, "pop")
}

func TestEnrichTokenPoolConfirmed(t *testing.T) {
	em := newTestEventEnricher()
	ctx := context.Background()

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()

	// Setup enrichment
	mdi := em.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPoolByID", mock.Anything, "ns1", ref1).Return(&core.TokenPool{
		ID: ref1,
	}, nil)

	event := &core.Event{
		ID:        ev1,
		Type:      core.EventTypePoolConfirmed,
		Reference: ref1,
	}

	enriched, err := em.enrichEvent(ctx, event)
	assert.NoError(t, err)
	assert.Equal(t, ref1, enriched.TokenPool.ID)
}

func TestEnrichTokenPoolConfirmedFail(t *testing.T) {
	em := newTestEventEnricher()
	ctx := context.Background()

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()

	// Setup enrichment
	mdi := em.database.(*databasemocks.Plugin)
	mdi.On("GetTokenPoolByID", mock.Anything, "ns1", ref1).Return(nil, fmt.Errorf("pop"))

	event := &core.Event{
		ID:        ev1,
		Type:      core.EventTypePoolConfirmed,
		Reference: ref1,
	}

	_, err := em.enrichEvent(ctx, event)
	assert.EqualError(t, err, "pop")
}

func TestEnrichTokenApprovalConfirmed(t *testing.T) {
	em := newTestEventEnricher()
	ctx := context.Background()

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()

	// Setup enrichment
	mdi := em.database.(*databasemocks.Plugin)
	mdi.On("GetTokenApprovalByID", mock.Anything, "ns1", ref1).Return(&core.TokenApproval{
		LocalID: ref1,
	}, nil)

	event := &core.Event{
		ID:        ev1,
		Type:      core.EventTypeApprovalConfirmed,
		Reference: ref1,
	}

	enriched, err := em.enrichEvent(ctx, event)
	assert.NoError(t, err)
	assert.Equal(t, ref1, enriched.TokenApproval.LocalID)
}

func TestEnrichTokenApprovalFailed(t *testing.T) {
	em := newTestEventEnricher()
	ctx := context.Background()

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()

	// Setup enrichment
	mom := em.operations.(*operationmocks.Manager)
	mom.On("GetOperationByIDCached", mock.Anything, ref1).Return(&core.Operation{
		ID: ref1,
	}, nil)

	event := &core.Event{
		ID:        ev1,
		Type:      core.EventTypeApprovalOpFailed,
		Reference: ref1,
	}

	enriched, err := em.enrichEvent(ctx, event)
	assert.NoError(t, err)
	assert.Equal(t, ref1, enriched.Operation.ID)
}

func TestEnrichTokenApprovalConfirmedFail(t *testing.T) {
	em := newTestEventEnricher()
	ctx := context.Background()

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()

	// Setup enrichment
	mdi := em.database.(*databasemocks.Plugin)
	mdi.On("GetTokenApprovalByID", mock.Anything, "ns1", ref1).Return(nil, fmt.Errorf("pop"))

	event := &core.Event{
		ID:        ev1,
		Type:      core.EventTypeApprovalConfirmed,
		Reference: ref1,
	}

	_, err := em.enrichEvent(ctx, event)
	assert.EqualError(t, err, "pop")
}

func TestEnrichTokenTransferConfirmed(t *testing.T) {
	em := newTestEventEnricher()
	ctx := context.Background()

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()

	// Setup enrichment
	mdi := em.database.(*databasemocks.Plugin)
	mdi.On("GetTokenTransferByID", mock.Anything, "ns1", ref1).Return(&core.TokenTransfer{
		LocalID: ref1,
	}, nil)

	event := &core.Event{
		ID:        ev1,
		Type:      core.EventTypeTransferConfirmed,
		Reference: ref1,
	}

	enriched, err := em.enrichEvent(ctx, event)
	assert.NoError(t, err)
	assert.Equal(t, ref1, enriched.TokenTransfer.LocalID)
}

func TestEnrichTokenTransferFailed(t *testing.T) {
	em := newTestEventEnricher()
	ctx := context.Background()

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()

	// Setup enrichment
	mom := em.operations.(*operationmocks.Manager)
	mom.On("GetOperationByIDCached", mock.Anything, ref1).Return(&core.Operation{
		ID: ref1,
	}, nil)

	event := &core.Event{
		ID:        ev1,
		Type:      core.EventTypeTransferOpFailed,
		Reference: ref1,
	}

	enriched, err := em.enrichEvent(ctx, event)
	assert.NoError(t, err)
	assert.Equal(t, ref1, enriched.Operation.ID)
}

func TestEnrichTokenTransferConfirmedFail(t *testing.T) {
	em := newTestEventEnricher()
	ctx := context.Background()

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()

	// Setup enrichment
	mdi := em.database.(*databasemocks.Plugin)
	mdi.On("GetTokenTransferByID", mock.Anything, "ns1", ref1).Return(nil, fmt.Errorf("pop"))

	event := &core.Event{
		ID:        ev1,
		Type:      core.EventTypeTransferConfirmed,
		Reference: ref1,
	}

	_, err := em.enrichEvent(ctx, event)
	assert.EqualError(t, err, "pop")
}

func TestEnrichOperationFail(t *testing.T) {
	em := newTestEventEnricher()
	ctx := context.Background()

	// Setup the IDs
	ref1 := fftypes.NewUUID()
	ev1 := fftypes.NewUUID()

	// Setup enrichment
	mom := em.operations.(*operationmocks.Manager)
	mom.On("GetOperationByIDCached", mock.Anything, ref1).Return(nil, fmt.Errorf("pop"))

	event := &core.Event{
		ID:        ev1,
		Type:      core.EventTypeApprovalOpFailed,
		Reference: ref1,
	}

	_, err := em.enrichEvent(ctx, event)
	assert.EqualError(t, err, "pop")
}

func TestEnrichEventsFails(t *testing.T) {
	em := newTestEventEnricher()
	ctx := context.Background()

	ev1 := fftypes.NewUUID()
	ev2 := fftypes.NewUUID()
	ref1 := fftypes.NewUUID()

	// Setup enrichment
	mom := em.operations.(*operationmocks.Manager)
	mom.On("GetOperationByIDCached", mock.Anything, mock.Anything).Return(&core.Operation{
		ID: ref1,
	}, nil).Once()
	mom.On("GetOperationByIDCached", mock.Anything, mock.Anything).Return(nil, fmt.Errorf("pop"))

	events := []*core.Event{
		{
			ID:   ev1,
			Type: core.EventTypeApprovalOpFailed,
		},
		{
			ID:   ev2,
			Type: core.EventTypeApprovalOpFailed,
		},
	}

	_, err := em.enrichEvents(ctx, events)
	assert.EqualError(t, err, "pop")
}

func TestEnrichEventsOK(t *testing.T) {
	em := newTestEventEnricher()
	ctx := context.Background()

	ev1 := fftypes.NewUUID()
	ev2 := fftypes.NewUUID()
	ref1 := fftypes.NewUUID()

	// Setup enrichment
	mom := em.operations.(*operationmocks.Manager)
	mom.On("GetOperationByIDCached", mock.Anything, mock.Anything).Return(&core.Operation{
		ID: ref1,
	}, nil)

	events := []*core.Event{
		{
			ID:   ev1,
			Type: core.EventTypeApprovalOpFailed,
		},
		{
			ID:   ev2,
			Type: core.EventTypeApprovalOpFailed,
		},
	}

	result, err := em.enrichEvents(ctx, events)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(result))
}
