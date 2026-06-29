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

package orchestrator

import (
	"context"
	"math"
	"time"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/internal/events/system"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

func (or *orchestrator) CreateSubscription(ctx context.Context, subDef *core.Subscription) (*core.Subscription, error) {
	return or.createUpdateSubscription(ctx, subDef, true)
}

func (or *orchestrator) CreateUpdateSubscription(ctx context.Context, subDef *core.Subscription) (*core.Subscription, error) {
	return or.createUpdateSubscription(ctx, subDef, false)
}

func (or *orchestrator) createUpdateSubscription(ctx context.Context, subDef *core.Subscription, mustNew bool) (*core.Subscription, error) {
	subDef.ID = fftypes.NewUUID()
	subDef.Created = fftypes.Now()
	subDef.Namespace = or.namespace.Name
	subDef.Ephemeral = false
	if err := fftypes.ValidateFFNameFieldNoUUID(ctx, subDef.Name, "name"); err != nil {
		return nil, err
	}
	if subDef.Transport == system.SystemEventsTransport {
		return nil, i18n.NewError(ctx, coremsgs.MsgSystemTransportInternal)
	}
	resolvedTransport, capabilities, err := or.events.ResolveTransportAndCapabilities(ctx, subDef.Transport)
	if err != nil {
		return nil, err
	}
	subDef.Transport = resolvedTransport

	if subDef.Options.TLSConfigName != "" {
		if or.namespace.TLSConfigs[subDef.Options.TLSConfigName] == nil {
			return nil, i18n.NewError(ctx, coremsgs.MsgNotFoundTLSConfig, subDef.Options.TLSConfigName, subDef.Namespace)

		}

		subDef.Options.TLSConfig = or.namespace.TLSConfigs[subDef.Options.TLSConfigName]
	}

	if subDef.Options.BatchTimeout != nil && *subDef.Options.BatchTimeout != "" {
		_, err := fftypes.ParseDurationString(*subDef.Options.BatchTimeout, time.Millisecond)
		if err != nil {
			return nil, err
		}
	}

	if subDef.Options.Batch != nil && *subDef.Options.Batch {
		if subDef.Options.WithData != nil && *subDef.Options.WithData {
			return nil, i18n.NewError(ctx, coremsgs.MsgBatchWithDataNotSupported, subDef.Name)
		}
		if !capabilities.BatchDelivery {
			return nil, i18n.NewError(ctx, coremsgs.MsgBatchDeliveryNotSupported, subDef.Transport)
		}
	}

	return subDef, or.events.CreateUpdateDurableSubscription(ctx, subDef, mustNew)
}

func (or *orchestrator) DeleteSubscription(ctx context.Context, id string) error {
	u, err := fftypes.ParseUUID(ctx, id)
	if err != nil {
		return err
	}
	sub, err := or.database().GetSubscriptionByID(ctx, or.namespace.Name, u)
	if err != nil {
		return err
	}
	if sub == nil {
		return i18n.NewError(ctx, coremsgs.Msg404NotFound)
	}
	return or.events.DeleteDurableSubscription(ctx, sub)
}

func (or *orchestrator) GetSubscriptions(ctx context.Context, filter ffapi.AndFilter) ([]*core.Subscription, *ffapi.FilterResult, error) {
	return or.database().GetSubscriptions(ctx, or.namespace.Name, filter)
}

func (or *orchestrator) GetSubscriptionByID(ctx context.Context, id string) (*core.Subscription, error) {
	u, err := fftypes.ParseUUID(ctx, id)
	if err != nil {
		return nil, err
	}
	return or.database().GetSubscriptionByID(ctx, or.namespace.Name, u)
}

func (or *orchestrator) GetSubscriptionByIDWithStatus(ctx context.Context, id string) (*core.SubscriptionWithStatus, error) {
	sub, err := or.GetSubscriptionByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if sub == nil {
		return nil, nil
	}

	offset, err := or.database().GetOffset(ctx, core.OffsetTypeSubscription, sub.ID.String())
	if err != nil {
		return nil, err
	}

	subWithStatus := &core.SubscriptionWithStatus{
		Subscription: *sub,
	}

	if offset != nil {
		subWithStatus.Status = core.SubscriptionStatus{
			CurrentOffset: offset.Current,
		}
	}

	return subWithStatus, nil
}

func (or *orchestrator) GetSubscriptionEventsHistorical(ctx context.Context, subscription *core.Subscription, filter ffapi.AndFilter, startSequence int, endSequence int) ([]*core.EnrichedEvent, *ffapi.FilterResult, error) {
	if startSequence != -1 && endSequence != -1 && endSequence-startSequence > config.GetInt(coreconfig.SubscriptionMaxHistoricalEventScanLength) {
		return nil, nil, i18n.NewError(ctx, coremsgs.MsgMaxSubscriptionEventScanLimitBreached, startSequence, endSequence)
	}

	requestedFiltering, err := filter.Finalize()
	if err != nil {
		return nil, nil, err
	}

	var unfilteredEvents []*core.EnrichedEvent
	if startSequence == -1 && endSequence == -1 {
		unfilteredEvents, _, err = or.GetEventsWithReferences(ctx, filter)
		if err != nil {
			return nil, nil, err
		}
	} else {
		if startSequence == -1 {
			recordLimit := math.Min(float64(requestedFiltering.Limit), float64(config.GetInt(coreconfig.SubscriptionMaxHistoricalEventScanLength)))
			if endSequence-int(recordLimit) > 0 {
				startSequence = endSequence - int(recordLimit)
			} else {
				startSequence = 0
			}
		}

		if endSequence == -1 {
			// This blind assertion is safe since the DB won't blow up, it'll just return nothing
			endSequence = startSequence + 1000
		}

		unfilteredEvents, _, err = or.GetEventsWithReferencesInSequenceRange(ctx, filter, startSequence, endSequence)
		if err != nil {
			return nil, nil, err
		}
	}

	filteredEvents, err := or.events.FilterHistoricalEventsOnSubscription(ctx, unfilteredEvents, subscription)
	if err != nil {
		return nil, nil, err
	}

	intLimit := len(filteredEvents)
	if requestedFiltering.Limit < uint64(intLimit) {
		//nolint:gosec
		intLimit = int(requestedFiltering.Limit)
	}
	var filteredEventsMatchingSubscription []*core.EnrichedEvent
	if len(filteredEvents) > intLimit {
		filteredEventsMatchingSubscription = filteredEvents[len(filteredEvents)-intLimit:]
	} else {
		filteredEventsMatchingSubscription = filteredEvents
	}

	filterResultLength := int64(len(filteredEventsMatchingSubscription))
	return filteredEventsMatchingSubscription, &ffapi.FilterResult{
		TotalCount: &filterResultLength,
	}, nil
}
