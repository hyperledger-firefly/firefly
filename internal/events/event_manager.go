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
	"bytes"
	"context"
	"encoding/json"
	"strconv"

	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly-common/pkg/retry"
	"github.com/hyperledger/firefly/internal/assets"
	"github.com/hyperledger/firefly/internal/broadcast"
	"github.com/hyperledger/firefly/internal/cache"
	"github.com/hyperledger/firefly/internal/coreconfig"
	"github.com/hyperledger/firefly/internal/coremsgs"
	"github.com/hyperledger/firefly/internal/data"
	"github.com/hyperledger/firefly/internal/definitions"
	"github.com/hyperledger/firefly/internal/events/eifactory"
	"github.com/hyperledger/firefly/internal/events/system"
	"github.com/hyperledger/firefly/internal/identity"
	"github.com/hyperledger/firefly/internal/metrics"
	"github.com/hyperledger/firefly/internal/multiparty"
	"github.com/hyperledger/firefly/internal/operations"
	"github.com/hyperledger/firefly/internal/privatemessaging"
	"github.com/hyperledger/firefly/internal/shareddownload"
	"github.com/hyperledger/firefly/internal/txcommon"
	"github.com/hyperledger/firefly/pkg/blockchain"
	"github.com/hyperledger/firefly/pkg/core"
	"github.com/hyperledger/firefly/pkg/database"
	"github.com/hyperledger/firefly/pkg/dataexchange"
	"github.com/hyperledger/firefly/pkg/events"
	"github.com/hyperledger/firefly/pkg/sharedstorage"
	"github.com/hyperledger/firefly/pkg/tokens"
)

type EventManager interface {
	NewPins() chan<- int64
	NewEvents() chan<- int64
	NewSubscriptions() chan<- *fftypes.UUID
	SubscriptionUpdates() chan<- *fftypes.UUID
	DeletedSubscriptions() chan<- *fftypes.UUID
	DeleteDurableSubscription(ctx context.Context, subDef *core.Subscription) (err error)
	CreateUpdateDurableSubscription(ctx context.Context, subDef *core.Subscription, mustNew bool) (err error)
	EnrichEvent(ctx context.Context, event *core.Event) (*core.EnrichedEvent, error)
	EnrichEvents(ctx context.Context, events []*core.Event) ([]*core.EnrichedEvent, error)
	FilterHistoricalEventsOnSubscription(ctx context.Context, events []*core.EnrichedEvent, sub *core.Subscription) ([]*core.EnrichedEvent, error)
	QueueBatchRewind(batchID *fftypes.UUID)
	ResolveTransportAndCapabilities(ctx context.Context, transportName string) (string, *events.Capabilities, error)
	Start() error
	WaitStop()

	// Bound blockchain callbacks
	BlockchainEventBatch(batch []*blockchain.EventToDispatch) error

	// Bound dataexchange callbacks
	DXEvent(plugin dataexchange.Plugin, event dataexchange.DXEvent) error

	// Bound sharedstorage callbacks
	SharedStorageBatchDownloaded(ss sharedstorage.Plugin, payloadRef string, data []byte) (*fftypes.UUID, error)
	SharedStorageBlobDownloaded(ss sharedstorage.Plugin, hash fftypes.Bytes32, size int64, payloadRef string, dataID *fftypes.UUID) error

	// Bound token callbacks
	TokenPoolCreated(ctx context.Context /* allows security context to be propagated when called in-line with the send TX */, ti tokens.Plugin, pool *tokens.TokenPool) error
	TokensTransferred(ti tokens.Plugin, transfer *tokens.TokenTransfer) error
	TokensApproved(ti tokens.Plugin, approval *tokens.TokenApproval) error

	GetPlugins() []*core.NamespaceStatusPlugin

	// Internal events
	system.EventInterface
}

type eventManager struct {
	ctx                context.Context
	namespace          *core.Namespace
	enricher           *eventEnricher
	database           database.Plugin
	txHelper           txcommon.Helper
	identity           identity.Manager
	defsender          definitions.Sender
	defhandler         definitions.Handler
	data               data.Manager
	subManager         *subscriptionManager
	retry              retry.Retry
	aggregator         *aggregator              // optional
	broadcast          broadcast.Manager        // optional
	messaging          privatemessaging.Manager // optional
	assets             assets.Manager
	sharedDownload     shareddownload.Manager // optional
	blobReceiver       *blobReceiver          // optional
	newEventNotifier   *eventNotifier
	newPinNotifier     *eventNotifier
	defaultTransport   string
	internalEvents     *system.Events
	metrics            metrics.Manager
	chainListenerCache cache.CInterface
	multiparty         multiparty.Manager // optional
}

func NewEventManager(ctx context.Context, ns *core.Namespace, di database.Plugin, bi blockchain.Plugin, im identity.Manager, dh definitions.Handler, dm data.Manager, ds definitions.Sender, bm broadcast.Manager, pm privatemessaging.Manager, am assets.Manager, sd shareddownload.Manager, mm metrics.Manager, om operations.Manager, txHelper txcommon.Helper, transports map[string]events.Plugin, mp multiparty.Manager, cacheManager cache.Manager) (EventManager, error) {
	if di == nil || im == nil || dh == nil || dm == nil || om == nil || ds == nil || am == nil {
		return nil, i18n.NewError(ctx, coremsgs.MsgInitializationNilDepError, "EventManager")
	}
	newPinNotifier := newEventNotifier(ctx, "pins")
	newEventNotifier := newEventNotifier(ctx, "events")

	eventListenerCache, err := cacheManager.GetCache(
		cache.NewCacheConfig(
			ctx,
			coreconfig.CacheEventListenerTopicLimit,
			coreconfig.CacheEventListenerTopicTTL,
			ns.Name,
		),
	)
	if err != nil {
		return nil, err
	}

	em := &eventManager{
		ctx:            log.WithLogField(ctx, "role", "event-manager"),
		namespace:      ns,
		database:       di,
		txHelper:       txHelper,
		identity:       im,
		defsender:      ds,
		defhandler:     dh,
		data:           dm,
		broadcast:      bm,
		messaging:      pm,
		assets:         am,
		sharedDownload: sd,
		multiparty:     mp,
		retry: retry.Retry{
			InitialDelay: config.GetDuration(coreconfig.EventAggregatorRetryInitDelay),
			MaximumDelay: config.GetDuration(coreconfig.EventAggregatorRetryMaxDelay),
			Factor:       config.GetFloat64(coreconfig.EventAggregatorRetryFactor),
		},
		defaultTransport:   config.GetString(coreconfig.EventTransportsDefault),
		newEventNotifier:   newEventNotifier,
		newPinNotifier:     newPinNotifier,
		metrics:            mm,
		chainListenerCache: eventListenerCache,
	}
	ie, _ := eifactory.GetPlugin(ctx, system.SystemEventsTransport)
	em.internalEvents = ie.(*system.Events)
	if bi != nil {
		aggregator, err := newAggregator(ctx, ns.Name, di, bi, pm, dh, im, dm, newPinNotifier, mm, cacheManager)
		if err != nil {
			return nil, err
		}
		em.aggregator = aggregator
		em.blobReceiver = newBlobReceiver(ctx, em.aggregator)
	}

	em.enricher = newEventEnricher(ns.Name, di, dm, om, txHelper)

	if em.subManager, err = newSubscriptionManager(ctx, ns, em.enricher, di, dm, newEventNotifier, bm, pm, txHelper, transports); err != nil {
		return nil, err
	}

	return em, nil
}

func (em *eventManager) Start() (err error) {
	err = em.subManager.start()
	if err == nil {
		if em.aggregator != nil {
			em.aggregator.start()
			em.blobReceiver.start()
		}
	}
	return err
}

func (em *eventManager) NewEvents() chan<- int64 {
	return em.newEventNotifier.newEvents
}

func (em *eventManager) NewPins() chan<- int64 {
	return em.newPinNotifier.newEvents
}

func (em *eventManager) NewSubscriptions() chan<- *fftypes.UUID {
	return em.subManager.newOrUpdatedSubscriptions
}

func (em *eventManager) SubscriptionUpdates() chan<- *fftypes.UUID {
	return em.subManager.newOrUpdatedSubscriptions
}

func (em *eventManager) DeletedSubscriptions() chan<- *fftypes.UUID {
	return em.subManager.deletedSubscriptions
}

func (em *eventManager) ResolveTransportAndCapabilities(ctx context.Context, transportName string) (string, *events.Capabilities, error) {
	if transportName == "" {
		transportName = em.defaultTransport
	}
	t, err := em.subManager.getTransport(ctx, transportName)
	if err != nil {
		return "", nil, err
	}
	return transportName, t.Capabilities(), nil
}

func (em *eventManager) WaitStop() {
	em.subManager.close()
	if em.blobReceiver != nil {
		em.blobReceiver.stop()
		em.blobReceiver = nil
	}
	if em.aggregator != nil {
		<-em.aggregator.eventPoller.closed
	}
}

func (em *eventManager) CreateUpdateDurableSubscription(ctx context.Context, subDef *core.Subscription, mustNew bool) (err error) {
	if subDef.Namespace == "" || subDef.Name == "" || subDef.ID == nil {
		return i18n.NewError(ctx, coremsgs.MsgInvalidSubscription)
	}

	// Check it can be parsed before inserting (the submanager will check again when processing the creation, so we discard the result)
	if _, err = em.subManager.parseSubscriptionDef(ctx, subDef); err != nil {
		return err
	}

	// Do a check first for existence, to give a nice 409 if we find one
	existing, _ := em.database.GetSubscriptionByName(ctx, subDef.Namespace, subDef.Name)
	if existing != nil {
		if mustNew {
			return i18n.NewError(ctx, coremsgs.MsgAlreadyExists, "subscription", subDef.Namespace, subDef.Name)
		}
		// Copy over the generated fields, so we can do a compare
		subDef.Created = existing.Created
		subDef.ID = existing.ID
		subDef.Updated = fftypes.Now()
		subDef.Options.FirstEvent = existing.Options.FirstEvent // we do not reset the sub position
		existing.Updated = subDef.Updated
		def1, _ := json.Marshal(existing)
		def2, _ := json.Marshal(subDef)
		if bytes.Equal(def1, def2) {
			log.L(ctx).Infof("Subscription already exists, and is identical")
			return nil
		}
	} else {
		// We lock in the starting sequence at creation time, rather than when the first dispatcher
		// starts, as that's a more obvious behavior for users
		sequence, err := calcFirstOffset(ctx, em.namespace.Name, em.database, subDef.Options.FirstEvent)
		if err != nil {
			return err
		}
		lockedInFirstEvent := core.SubOptsFirstEvent(strconv.FormatInt(sequence, 10))
		subDef.Options.FirstEvent = &lockedInFirstEvent
	}

	// The event in the database for the creation of the susbscription, will asynchronously update the submanager
	return em.database.UpsertSubscription(ctx, subDef, !mustNew)
}

func (em *eventManager) DeleteDurableSubscription(ctx context.Context, subDef *core.Subscription) (err error) {
	// The event in the database for the deletion of the susbscription, will asynchronously update the submanager
	return em.database.DeleteSubscriptionByID(ctx, em.namespace.Name, subDef.ID)
}

func (em *eventManager) AddSystemEventListener(ns string, el system.EventListener) error {
	return em.internalEvents.AddListener(ns, el)
}

func (em *eventManager) GetPlugins() []*core.NamespaceStatusPlugin {
	eventsArray := make([]*core.NamespaceStatusPlugin, 0)
	plugins := em.subManager.transports

	for name := range plugins {
		eventsArray = append(eventsArray, &core.NamespaceStatusPlugin{
			PluginType: name,
		})
	}

	return eventsArray
}

func (em *eventManager) EnrichEvent(ctx context.Context, event *core.Event) (*core.EnrichedEvent, error) {
	return em.enricher.enrichEvent(ctx, event)
}

func (em *eventManager) EnrichEvents(ctx context.Context, events []*core.Event) ([]*core.EnrichedEvent, error) {
	return em.enricher.enrichEvents(ctx, events)
}

func (em *eventManager) QueueBatchRewind(batchID *fftypes.UUID) {
	em.aggregator.queueBatchRewind(batchID)
}

func (em *eventManager) FilterHistoricalEventsOnSubscription(ctx context.Context, events []*core.EnrichedEvent, sub *core.Subscription) ([]*core.EnrichedEvent, error) {
	subscriptionDef, err := em.subManager.parseSubscriptionDef(ctx, sub)
	if err != nil {
		return nil, err
	}

	matchingEvents := []*core.EnrichedEvent{}
	for _, event := range events {
		if subscriptionDef.MatchesEvent(event) {
			matchingEvents = append(matchingEvents, event)
		}
	}

	return matchingEvents, nil
}
