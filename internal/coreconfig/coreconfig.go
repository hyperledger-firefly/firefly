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

package coreconfig

import (
	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/spf13/viper"
)

var ffc = config.AddRootKey

const (
	// PluginConfigName is the user-supplied name for this plugin type
	PluginConfigName = "name"
	// PluginConfigType is the type of the plugin to be loaded
	PluginConfigType = "type"
	// PluginBroadcastName is the plugin name to be sent in multi-party broadcasts, if it differs from the local plugin name
	PluginBroadcastName = "broadcastName"
	// NamespaceName is the short name for a pre-defined namespace
	NamespaceName = "name"
	// NamespaceName is the long description for a pre-defined namespace
	NamespaceDescription = "description"
	// NamespacePlugins is the list of namespace plugins
	NamespacePlugins = "plugins"
	// NamespaceTLSConfigName is the user-supplied name for the TLS Config
	NamespaceTLSConfigName = "name"
	// NamespaceTLSConfigs is the list of tls configs
	NamespaceTLSConfigs = "tlsConfigs"
	// NamespaceTLSConfigTLSSection is the section to provide the paths to CA , cert and key files
	NamespaceTLSConfigTLSSection = "tls"
	// NamespaceDefaultKey is the default signing key for blockchain transactions within this namespace
	NamespaceDefaultKey = "defaultKey"
	// NamespaceAssetKeyNormalization mechanism to normalize keys before using them. Valid options: "blockchain_plugin" - use blockchain plugin (default), "none" - do not attempt normalization
	NamespaceAssetKeyNormalization = "asset.manager.keyNormalization"
	// NamespaceMultiparty contains the multiparty configuration for a namespace
	NamespaceMultiparty = "multiparty"
	// NamespaceMultipartyEnabled specifies if multi-party mode is enabled for a namespace
	NamespaceMultipartyEnabled = "enabled"
	// NamespaceMultipartyNetworkNamespace is the shared namespace name to be used in off-chain messaging
	NamespaceMultipartyNetworkNamespace = "networknamespace"
	// NamespaceMultipartyOrgName is a short name for the local root org within a namespace
	NamespaceMultipartyOrgName = "org.name"
	// NamespaceMultipartyOrgDescription is a description for the local root org within a namespace
	NamespaceMultipartyOrgDescription = "org.description"
	// NamespaceMultipartyOrgKey is the signing key allocated to the local root org within a namespace
	NamespaceMultipartyOrgKey = "org.key"
	// NamespaceMultipartyNodeName is the name for the local node within a namespace
	NamespaceMultipartyNodeName = "node.name"
	// NamespaceMultipartyNodeName is a description for the local node within a namespace
	NamespaceMultipartyNodeDescription = "node.description"
	// NamespaceMultipartyContract is a list of firefly contract configurations for this namespace
	NamespaceMultipartyContract = "contract"
	// NamespaceMultipartyContractFirstEvent is the first event to process for this contract
	NamespaceMultipartyContractFirstEvent = "firstEvent"
	// NamespaceMultipartyContractLocation is an object specifying the blockchain-specific location of the contract
	NamespaceMultipartyContractLocation = "location"
	// NamespaceMultipartyContractOptions is an object of additional blockchain-specific configuration
	NamespaceMultipartyContractOptions = "options"
)

// The following keys can be access from the root configuration.
// Plugins are responsible for defining their own keys using the Config interface
var (
	// APIDefaultFilterLimit is the default limit that will be applied to filtered queries on the API
	APIDefaultFilterLimit = ffc("api.defaultFilterLimit")
	// APIMaxFilterLimit is the maximum limit that can be specified by an API call
	APIMaxFilterLimit = ffc("api.maxFilterLimit")
	// APIMaxFilterSkip is the maximum skip value that can be specified on the API
	APIMaxFilterSkip = ffc("api.maxFilterLimit")
	// APIRequestTimeout is the server side timeout for API calls (context timeout), to avoid the server continuing processing when the client gives up
	APIRequestTimeout = ffc("api.requestTimeout")
	// APIRequestMaxTimeout is the maximum timeout an application can set using a Request-Timeout header
	APIRequestMaxTimeout = ffc("api.requestMaxTimeout")
	// APIDynamicPublicURLHeader is a header that can be used on requests to generate Swagger to influence the PublicURL on a per-request basis
	APIDynamicPublicURLHeader = ffc("api.dynamicPublicURLHeader")
	// APIOASPanicOnMissingDescription controls whether the OpenAPI Spec generator will strongly enforce descriptions on every field or not
	APIOASPanicOnMissingDescription = ffc("api.oas.panicOnMissingDescription")
	// APIPassThroughHeaders is a list of HTTP request headers to pass through to requests made to dependency microservices
	APIPassthroughHeaders = ffc("api.passthroughHeaders")
	// BatchManagerReadPageSize is the size of each page of messages read from the database into memory when assembling batches
	BatchManagerReadPageSize = ffc("batch.manager.readPageSize")
	// BatchManagerReadPollTimeout is how long without any notifications of new messages to wait, before doing a page query
	BatchManagerReadPollTimeout = ffc("batch.manager.pollTimeout")
	// BatchManagerMinimumPollDelay is the minimum time the batch manager waits between polls on the DB - to prevent thrashing
	BatchManagerMinimumPollDelay = ffc("batch.manager.minimumPollDelay")
	// BatchRetryFactor is the retry backoff factor for database operations performed by the batch manager
	BatchRetryFactor = ffc("batch.retry.factor")
	// BatchRetryInitDelay is the retry initial delay for database operations
	BatchRetryInitDelay = ffc("batch.retry.initDelay")
	// BatchRetryMaxDelay is the maximum delay between retry attempts
	BatchRetryMaxDelay = ffc("batch.retry.maxDelay")
	// BlobReceiverRetryInitDelay is the initial retry delay
	BlobReceiverRetryInitDelay = ffc("blobreceiver.retry.initialDelay")
	// BlobReceiverRetryMaxDelay is the maximum retry delay
	BlobReceiverRetryMaxDelay = ffc("blobreceiver.retry.maxDelay")
	// BlobReceiverRetryFactor is the backoff factor to use for retries
	BlobReceiverRetryFactor = ffc("blobreceiver.retry.factor")
	// BlobReceiverWorkerCount
	BlobReceiverWorkerCount = ffc("blobreceiver.worker.count")
	// BlobReceiverWorkerBatchTimeout
	BlobReceiverWorkerBatchTimeout = ffc("blobreceiver.worker.batchTimeout")
	// BlobReceiverWorkerBatchMaxInserts
	BlobReceiverWorkerBatchMaxInserts = ffc("blobreceiver.worker.batchMaxInserts")

	// BroadcastBatchAgentTimeout how long to keep around a batching agent for a sending identity before disposal
	BroadcastBatchAgentTimeout = ffc("broadcast.batch.agentTimeout")
	// BroadcastBatchSize is the maximum number of messages that can be packed into a batch
	BroadcastBatchSize = ffc("broadcast.batch.size")
	// BroadcastBatchPayloadLimit is the maximum payload size of a batch for broadcast messages
	BroadcastBatchPayloadLimit = ffc("broadcast.batch.payloadLimit")
	// BroadcastBatchTimeout is the timeout to wait for a batch to fill, before sending
	BroadcastBatchTimeout = ffc("broadcast.batch.timeout")

	// ConfigAutoReload starts a filesystem listener against the config file, and if it changes analyzes the config file for changes that require individual namespaces to restart
	ConfigAutoReload = ffc("config.autoReload")

	// CacheEnabled determines whether cache will be enabled or not, default to true
	CacheEnabled = ffc("cache.enabled")

	// AddressResolver cache config
	CacheAddressResolverLimit = ffc("cache.addressresolver.limit")
	CacheAddressResolverTTL   = ffc("cache.addressresolver.ttl")

	// Batch cache config
	CacheBatchLimit = ffc("cache.batch.limit")
	CacheBatchTTL   = ffc("cache.batch.ttl")

	// Transaction - BlockchainEvent cache config
	CacheBlockchainEventLimit = ffc("cache.blockchainevent.limit")
	CacheBlockchainEventTTL   = ffc("cache.blockchainevent.ttl")
	// Transaction cache config
	CacheTransactionSize = ffc("cache.transaction.size")
	CacheTransactionTTL  = ffc("cache.transaction.ttl")

	// EventListenerTopic cache config
	CacheEventListenerTopicLimit = ffc("cache.eventlistenertopic.limit")
	CacheEventListenerTopicTTL   = ffc("cache.eventlistenertopic.ttl")

	// Group cache config
	CacheGroupLimit = ffc("cache.group.limit")
	CacheGroupTTL   = ffc("cache.group.ttl")

	// IdentityManager identity cache config
	CacheIdentityLimit = ffc("cache.identity.limit")
	CacheIdentityTTL   = ffc("cache.identity.ttl")

	// DataManager Message cache config
	CacheMessageSize = ffc("cache.message.size")
	CacheMessageTTL  = ffc("cache.message.ttl")

	// Token pool cache config
	CacheTokenPoolTTL   = ffc("cache.tokenpool.ttl")
	CacheTokenPoolLimit = ffc("cache.tokenpool.limit")

	// DataManager Validator cache config
	CacheValidatorSize = ffc("cache.validator.size")
	CacheValidatorTTL  = ffc("cache.validator.ttl")

	// Blockchain cache config
	CacheBlockchainTTL   = ffc("cache.blockchain.ttl")
	CacheBlockchainLimit = ffc("cache.blockchain.limit")

	// Operations cache config
	CacheOperationsLimit = ffc("cache.operations.limit")
	CacheOperationsTTL   = ffc("cache.operations.ttl")

	// Invoke methods cache config
	CacheMethodsLimit = ffc("cache.methods.limit")
	CacheMethodsTTL   = ffc("cache.methods.ttl")

	// DownloadWorkerCount is the number of download workers created to pull data from shared storage to the local DX
	DownloadWorkerCount = ffc("download.worker.count")
	// DownloadWorkerQueueLength is the length of the work queue in the channel to the workers - defaults to 2x the worker count
	DownloadWorkerQueueLength = ffc("download.worker.queueLength")
	// DownloadRetryMaxAttempts is the maximum number of automatic attempts to make for each shared storage download before failing the operation
	DownloadRetryMaxAttempts = ffc("download.retry.maxAttempts")
	// DownloadRetryInitDelay is the initial retry delay
	DownloadRetryInitDelay = ffc("download.retry.initialDelay")
	// DownloadRetryMaxDelay is the maximum retry delay
	DownloadRetryMaxDelay = ffc("download.retry.maxDelay")
	// DownloadRetryFactor is the backoff factor to use for retries
	DownloadRetryFactor = ffc("download.retry.factor")
	// PrivateMessagingBatchAgentTimeout how long to keep around a batching agent for a sending identity before disposal
	PrivateMessagingBatchAgentTimeout = ffc("privatemessaging.batch.agentTimeout")
	// PrivateMessagingBatchSize is the maximum size of a batch for broadcast messages
	PrivateMessagingBatchSize = ffc("privatemessaging.batch.size")
	// PrivateMessagingBatchPayloadLimit is the maximum payload size of a private message data exchange payload
	PrivateMessagingBatchPayloadLimit = ffc("privatemessaging.batch.payloadLimit")
	// PrivateMessagingBatchTimeout is the timeout to wait for a batch to fill, before sending
	PrivateMessagingBatchTimeout = ffc("privatemessaging.batch.timeout")
	// PrivateMessagingRetryFactor the backoff factor to use for retry of database operations
	PrivateMessagingRetryFactor = ffc("privatemessaging.retry.factor")
	// PrivateMessagingRetryInitDelay the initial delay to use for retry of data base operations
	PrivateMessagingRetryInitDelay = ffc("privatemessaging.retry.initDelay")
	// PrivateMessagingRetryMaxDelay the maximum delay to use for retry of data base operations
	PrivateMessagingRetryMaxDelay = ffc("privatemessaging.retry.maxDelay")
	// DatabaseType the type of the database interface plugin to use
	HistogramsMaxChartRows = ffc("histograms.maxChartRows")
	// TokensList is the root key containing a list of supported token connectors
	TokensList = ffc("tokens")
	// PluginsTokensList is the key containing a list of supported tokens plugins
	PluginsTokensList = ffc("plugins.tokens")
	// PluginsAuthList is the key containing a list of supported auth plugins
	PluginsAuthList = ffc("plugins.auth")
	// PluginsBlockchainList is the key containing a list of configured blockchain plugins
	PluginsBlockchainList = ffc("plugins.blockchain")
	// PluginsSharedStorageList is the key containing a list of configured shared storage plugins
	PluginsSharedStorageList = ffc("plugins.sharedstorage")
	// PluginsDatabaseList is the key containing a list of configured database plugins
	PluginsDatabaseList = ffc("plugins.database")
	// PluginsDataExchangeList is the key containing a list of configured database plugins
	PluginsDataExchangeList = ffc("plugins.dataexchange")
	// PluginsIdentityList is the key containing a list of configured identity plugins
	PluginsIdentityList = ffc("plugins.identity")
	// DebugPort a HTTP port on which to enable the go debugger
	DebugPort = ffc("debug.port")
	// DebugAddress the HTTP interface for the debugger to listen on
	DebugAddress = ffc("debug.address")
	// EventTransportsDefault the default event transport for new subscriptions
	EventTransportsDefault = ffc("event.transports.default")
	// EventTransportsEnabled which event interface plugins are enabled
	EventTransportsEnabled = ffc("event.transports.enabled")
	// EventAggregatorFirstEvent the first event the aggregator should process, if no previous offest is stored in the DB
	EventAggregatorFirstEvent = ffc("event.aggregator.firstEvent")
	// EventAggregatorBatchSize the maximum number of records to read from the DB before performing an aggregation run
	EventAggregatorBatchSize = ffc("event.aggregator.batchSize")
	// EventAggregatorBatchTimeout how long to wait for new events to arrive before performing aggregation on a page of events
	EventAggregatorBatchTimeout = ffc("event.aggregator.batchTimeout")
	// EventAggregatorPollTimeout the time to wait without a notification of new events, before trying a select on the table
	EventAggregatorPollTimeout = ffc("event.aggregator.pollTimeout")
	// EventAggregatorRewindTimeout the minimum time to wait for rewinds to accumulate before resolving them
	EventAggregatorRewindTimeout = ffc("event.aggregator.rewindTimeout")
	// EventAggregatorRewindQueueLength the size of the queue into the rewind dispatcher
	EventAggregatorRewindQueueLength = ffc("event.aggregator.rewindQueueLength")
	// EventAggregatorRewindQueryLimit safety limit on the maximum number of records to search when performing queries to search for rewinds
	EventAggregatorRewindQueryLimit = ffc("event.aggregator.rewindQueryLimit")
	// EventAggregatorRetryFactor the backoff factor to use for retry of database operations
	EventAggregatorRetryFactor = ffc("event.aggregator.retry.factor")
	// EventAggregatorRetryInitDelay the initial delay to use for retry of data base operations
	EventAggregatorRetryInitDelay = ffc("event.aggregator.retry.initDelay")
	// EventAggregatorRetryMaxDelay the maximum delay to use for retry of data base operations
	EventAggregatorRetryMaxDelay = ffc("event.aggregator.retry.maxDelay")
	// EventDispatcherPollTimeout the time to wait without a notification of new events, before trying a select on the table
	EventDispatcherPollTimeout = ffc("event.dispatcher.pollTimeout")
	// EventDispatcherBufferLength the number of events + attachments an individual dispatcher should hold in memory ready for delivery to the subscription
	EventDispatcherBufferLength = ffc("event.dispatcher.bufferLength")
	// EventDispatcherBatchTimeout a short time to wait for new events to arrive before re-polling for new events
	EventDispatcherBatchTimeout = ffc("event.dispatcher.batchTimeout")
	// EventDispatcherRetryFactor the backoff factor to use for retry of database operations
	EventDispatcherRetryFactor = ffc("event.dispatcher.retry.factor")
	// EventDispatcherRetryInitDelay he initial delay to use for retry of data base operations
	EventDispatcherRetryInitDelay = ffc("event.dispatcher.retry.initDelay")
	// EventDispatcherRetryMaxDelay he maximum delay to use for retry of data base operations
	EventDispatcherRetryMaxDelay = ffc("event.dispatcher.retry.maxDelay")
	// EventDBEventsBufferSize the size of the buffer of change events
	EventDBEventsBufferSize = ffc("event.dbevents.bufferSize")
	// LegacyAdminEnabled is the deprecated key that pre-dates spi.enabled
	LegacyAdminEnabled = ffc("admin.enabled")
	// SPIEnabled determines whether the admin interface will be enabled or not
	SPIEnabled = ffc("spi.enabled")
	// SPIWebSocketEventQueueLength is the maximum number of events that will queue up on the server side of each WebSocket connection before events start being dropped
	SPIWebSocketEventQueueLength = ffc("spi.ws.eventQueueLength")
	// SPIWebSocketBlockedWarnInterval how often to emit a warning if an admin.ws is blocked and not receiving events
	SPIWebSocketBlockedWarnInterval = ffc("spi.ws.blockedWarnInterval")
	// SPIWebSocketReadBufferSize is the WebSocket read buffer size for the admin change-event WebSocket
	SPIWebSocketReadBufferSize = ffc("spi.ws.readBufferSize")
	// SPIWebSocketWriteBufferSize is the WebSocket write buffer size for the admin change-event WebSocket
	SPIWebSocketWriteBufferSize = ffc("spi.ws.writeBufferSize")
	// MessageWriterCount
	MessageWriterCount = ffc("message.writer.count")
	// MessageWriterBatchTimeout
	MessageWriterBatchTimeout = ffc("message.writer.batchTimeout")
	// MessageWriterBatchMaxInserts
	MessageWriterBatchMaxInserts = ffc("message.writer.batchMaxInserts")
	// MetricsEnabled - deprecated, use monitoring.enabled
	DeprecatedMetricsEnabled = ffc("metrics.enabled")
	// MetricsPath - deprecated, use monitoring.metricsPath
	DeprecatedMetricsPath = ffc("metrics.path")
	// Monitoring determines whether monitoring routes will be enabled, which contains metrics instruments
	MonitoringEnabled = ffc("monitoring.enabled")
	// MetricsPath determines what path to serve the Prometheus metrics from
	MonitoringMetricsPath = ffc("monitoring.metricsPath")
	// NamespacesDefault is the default namespace - must be in the predefines list
	NamespacesDefault = ffc("namespaces.default")
	// NamespacesPredefined is a list of namespaces to ensure exists, without requiring a broadcast from the network
	NamespacesPredefined = ffc("namespaces.predefined")
	// NamespacesRetryFactor is the retry backoff factor for starting/restarting individual namespaces
	NamespacesRetryFactor = ffc("namespaces.retry.factor")
	// NamespacesRetryInitDelay is the retry initial delay for starting/restarting individual namespaces
	NamespacesRetryInitDelay = ffc("namespaces.retry.initDelay")
	// NamespacesRetryMaxDelay is the maximum delay between retry attempts
	NamespacesRetryMaxDelay = ffc("namespaces.retry.maxDelay")
	// NodeName is the short name for the node
	NodeName = ffc("node.name")
	// NodeDescription is a description for the node
	NodeDescription = ffc("node.description")
	// OpUpdateRetryInitDelay is the initial retry delay
	OpUpdateRetryInitDelay = ffc("opupdate.retry.initialDelay")
	// OpUpdatedRetryMaxDelay is the maximum retry delay
	OpUpdateRetryMaxDelay = ffc("opupdate.retry.maxDelay")
	// OpUpdateRetryFactor is the backoff factor to use for retries
	OpUpdateRetryFactor = ffc("opupdate.retry.factor")
	// OpUpdateWorkerCount
	OpUpdateWorkerCount = ffc("opupdate.worker.count")
	// OpUpdateWorkerBatchTimeout
	OpUpdateWorkerBatchTimeout = ffc("opupdate.worker.batchTimeout")
	// OpUpdateWorkerBatchMaxInserts
	OpUpdateWorkerBatchMaxInserts = ffc("opupdate.worker.batchMaxInserts")
	// OpUpdateWorkerQueueLength
	OpUpdateWorkerQueueLength = ffc("opupdate.worker.queueLength")
	// OrgName is the short name for the org
	OrgName = ffc("org.name")
	// OrgKey is the signing identity allocated to the organization (can be the same as the nodes)
	OrgKey = ffc("org.key")
	// OrgDescription is a description for the org
	OrgDescription = ffc("org.description")
	// OrchestratorStartupAttempts is how many time to attempt to connect to core infrastructure on startup
	OrchestratorStartupAttempts = ffc("orchestrator.startupAttempts")
	// SubscriptionDefaultsBatchSize default read ahead to enable for subscriptions that do not explicitly configure readahead
	SubscriptionDefaultsBatchSize = ffc("subscription.defaults.batchSize")
	// SubscriptionDefaultsBatchTimeout default batch timeout
	SubscriptionDefaultsBatchTimeout = ffc("subscription.defaults.batchTimeout")
	// SubscriptionMax maximum number of pre-defined subscriptions that can exist (note for high fan-out consider connecting a dedicated pub/sub broker to the dispatcher)
	SubscriptionMax = ffc("subscription.max")
	// SubscriptionsRetryInitialDelay is the initial retry delay
	SubscriptionsRetryInitialDelay = ffc("subscription.retry.initDelay")
	// SubscriptionsRetryMaxDelay is the initial retry delay
	SubscriptionsRetryMaxDelay = ffc("subscription.retry.maxDelay")
	// SubscriptionsRetryFactor the backoff factor to use for retry of database operations
	SubscriptionsRetryFactor = ffc("subscription.retry.factor")
	// SubscriptionMaxHistoricalEventScanLength the maximum amount of historical events we scan for in the DB when indexing through old events against a subscription
	SubscriptionMaxHistoricalEventScanLength = ffc("subscription.events.maxScanLength")
	// TransactionWriterCount
	TransactionWriterCount = ffc("transaction.writer.count")
	// TransactionWriterBatchTimeout
	TransactionWriterBatchTimeout = ffc("transaction.writer.batchTimeout")
	// TransactionWriterBatchMaxTransactions
	TransactionWriterBatchMaxTransactions = ffc("transaction.writer.batchMaxTransactions")

	// AssetManagerKeyNormalization mechanism to normalize keys before using them. Valid options: "blockchain_plugin" - use blockchain plugin (default), "none" - do not attempt normalization
	AssetManagerKeyNormalization = ffc("asset.manager.keyNormalization")
	// UIEnabled set to false to disable the UI (default is true, so UI will be enabled if ui.path is valid)
	UIEnabled = ffc("ui.enabled")
	// UIPath the path on which to serve the UI
	UIPath = ffc("ui.path")
)

func setDefaults() {
	// Set defaults
	viper.SetDefault(string(APIDefaultFilterLimit), 25)
	viper.SetDefault(string(APIRequestTimeout), "120s")
	viper.SetDefault(string(APIRequestMaxTimeout), "10m")
	viper.SetDefault(string(APIMaxFilterLimit), 250)
	viper.SetDefault(string(APIMaxFilterSkip), 1000) // protects database (skip+limit pagination is not for bulk operations)
	viper.SetDefault(string(APIRequestTimeout), "120s")
	viper.SetDefault(string(APIPassthroughHeaders), []string{})
	viper.SetDefault(string(AssetManagerKeyNormalization), "blockchain_plugin")
	viper.SetDefault(string(CacheBatchLimit), 100)
	viper.SetDefault(string(CacheBatchTTL), "5m")
	viper.SetDefault(string(BatchManagerReadPageSize), 100)
	viper.SetDefault(string(BatchManagerReadPollTimeout), "30s")
	viper.SetDefault(string(BatchManagerMinimumPollDelay), "100ms")
	viper.SetDefault(string(BatchRetryFactor), 2.0)
	viper.SetDefault(string(BatchRetryFactor), 2.0)
	viper.SetDefault(string(BatchRetryInitDelay), "250ms")
	viper.SetDefault(string(BatchRetryInitDelay), "250ms")
	viper.SetDefault(string(BatchRetryMaxDelay), "30s")
	viper.SetDefault(string(BatchRetryMaxDelay), "30s")
	viper.SetDefault(string(BlobReceiverRetryInitDelay), "250ms")
	viper.SetDefault(string(BlobReceiverRetryMaxDelay), "1m")
	viper.SetDefault(string(BlobReceiverRetryFactor), 2.0)
	viper.SetDefault(string(BlobReceiverWorkerBatchTimeout), "50ms")
	viper.SetDefault(string(BlobReceiverWorkerCount), 5)
	viper.SetDefault(string(BlobReceiverWorkerBatchMaxInserts), 200)
	viper.SetDefault(string(CacheBlockchainEventLimit), 1000)
	viper.SetDefault(string(CacheBlockchainEventTTL), "5m")
	viper.SetDefault(string(BroadcastBatchAgentTimeout), "2m")
	viper.SetDefault(string(BroadcastBatchSize), 200)
	viper.SetDefault(string(BroadcastBatchPayloadLimit), "800Kb")
	viper.SetDefault(string(BroadcastBatchTimeout), "1s")
	viper.SetDefault(string(CacheBlockchainLimit), 100)
	viper.SetDefault(string(CacheBlockchainTTL), "5m")
	viper.SetDefault(string(CacheAddressResolverLimit), 1000)
	viper.SetDefault(string(CacheAddressResolverTTL), "24h")
	viper.SetDefault(string(CacheEnabled), true)
	viper.SetDefault(string(CacheOperationsLimit), 1000)
	viper.SetDefault(string(CacheOperationsTTL), "5m")
	viper.SetDefault(string(CacheMethodsLimit), 200)
	viper.SetDefault(string(CacheMethodsTTL), "5m")
	viper.SetDefault(string(HistogramsMaxChartRows), 100)
	viper.SetDefault(string(DebugPort), -1)
	viper.SetDefault(string(DebugAddress), "localhost")
	viper.SetDefault(string(DownloadWorkerCount), 10)
	viper.SetDefault(string(DownloadRetryMaxAttempts), 100)
	viper.SetDefault(string(DownloadRetryInitDelay), "100ms")
	viper.SetDefault(string(DownloadRetryMaxDelay), "1m")
	viper.SetDefault(string(DownloadRetryFactor), 2.0)
	viper.SetDefault(string(EventAggregatorFirstEvent), core.SubOptsFirstEventOldest)
	viper.SetDefault(string(EventAggregatorBatchSize), 200)
	viper.SetDefault(string(EventAggregatorBatchTimeout), "0ms")
	viper.SetDefault(string(EventAggregatorPollTimeout), "30s")
	viper.SetDefault(string(EventAggregatorRewindTimeout), "50ms")
	viper.SetDefault(string(EventAggregatorRewindQueueLength), 10)
	viper.SetDefault(string(EventAggregatorRewindQueryLimit), 1000)
	viper.SetDefault(string(EventAggregatorRetryFactor), 2.0)
	viper.SetDefault(string(EventAggregatorRetryInitDelay), "100ms")
	viper.SetDefault(string(EventAggregatorRetryMaxDelay), "30s")
	viper.SetDefault(string(EventDBEventsBufferSize), 100)
	viper.SetDefault(string(EventDispatcherBufferLength), 5)
	viper.SetDefault(string(EventDispatcherBatchTimeout), "0ms")
	viper.SetDefault(string(EventDispatcherPollTimeout), "30s")
	viper.SetDefault(string(EventTransportsEnabled), []string{"websockets", "webhooks"})
	viper.SetDefault(string(EventTransportsDefault), "websockets")
	viper.SetDefault(string(CacheEventListenerTopicLimit), 100)
	viper.SetDefault(string(CacheEventListenerTopicTTL), "5m")
	viper.SetDefault(string(CacheGroupLimit), 50)
	viper.SetDefault(string(CacheGroupTTL), "1h")
	viper.SetDefault(string(SPIEnabled), false)
	viper.SetDefault(string(SPIWebSocketReadBufferSize), "16Kb")
	viper.SetDefault(string(SPIWebSocketWriteBufferSize), "16Kb")
	viper.SetDefault(string(SPIWebSocketBlockedWarnInterval), "1m")
	viper.SetDefault(string(SPIWebSocketEventQueueLength), 250)
	viper.SetDefault(string(CacheMessageSize), "50Mb")
	viper.SetDefault(string(CacheMessageTTL), "5m")
	viper.SetDefault(string(MessageWriterBatchMaxInserts), 200)
	viper.SetDefault(string(MessageWriterBatchTimeout), "10ms")
	viper.SetDefault(string(MessageWriterCount), 5)
	viper.SetDefault(string(NamespacesDefault), "default")
	viper.SetDefault(string(NamespacesRetryFactor), 2.0)
	viper.SetDefault(string(NamespacesRetryMaxDelay), "1m")
	viper.SetDefault(string(NamespacesRetryInitDelay), "5s")
	viper.SetDefault(string(OrchestratorStartupAttempts), 5)
	viper.SetDefault(string(OpUpdateRetryInitDelay), "250ms")
	viper.SetDefault(string(OpUpdateRetryMaxDelay), "1m")
	viper.SetDefault(string(OpUpdateRetryFactor), 2.0)
	viper.SetDefault(string(OpUpdateWorkerBatchTimeout), "50ms")
	viper.SetDefault(string(OpUpdateWorkerCount), 5)
	viper.SetDefault(string(OpUpdateWorkerBatchMaxInserts), 200)
	viper.SetDefault(string(OpUpdateWorkerQueueLength), 50)
	viper.SetDefault(string(PrivateMessagingRetryFactor), 2.0)
	viper.SetDefault(string(PrivateMessagingRetryInitDelay), "100ms")
	viper.SetDefault(string(PrivateMessagingRetryMaxDelay), "30s")
	viper.SetDefault(string(PrivateMessagingBatchAgentTimeout), "2m")
	viper.SetDefault(string(PrivateMessagingBatchSize), 200)
	viper.SetDefault(string(PrivateMessagingBatchTimeout), "1s")
	viper.SetDefault(string(PrivateMessagingBatchPayloadLimit), "800Kb")
	viper.SetDefault(string(SubscriptionDefaultsBatchSize), 50)
	viper.SetDefault(string(SubscriptionDefaultsBatchTimeout), "50ms")
	viper.SetDefault(string(SubscriptionMax), 500)
	viper.SetDefault(string(SubscriptionsRetryInitialDelay), "250ms")
	viper.SetDefault(string(SubscriptionsRetryMaxDelay), "30s")
	viper.SetDefault(string(SubscriptionsRetryFactor), 2.0)
	viper.SetDefault(string(SubscriptionMaxHistoricalEventScanLength), 1000)
	viper.SetDefault(string(TransactionWriterBatchMaxTransactions), 100)
	viper.SetDefault(string(TransactionWriterBatchTimeout), "10ms")
	viper.SetDefault(string(TransactionWriterCount), 5)
	viper.SetDefault(string(CacheTransactionSize), "1Mb")
	viper.SetDefault(string(CacheTransactionTTL), "5m")
	viper.SetDefault(string(UIEnabled), true)
	viper.SetDefault(string(CacheValidatorSize), "1Mb")
	viper.SetDefault(string(CacheValidatorTTL), "1h")
	viper.SetDefault(string(CacheIdentityLimit), 100)
	viper.SetDefault(string(CacheIdentityTTL), "1h")
	viper.SetDefault(string(CacheTokenPoolLimit), 100)
	viper.SetDefault(string(CacheTokenPoolTTL), "1h")
}

func Reset() {
	config.RootConfigReset(setDefaults)
}
