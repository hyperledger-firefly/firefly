---
title: Configuration Reference
---


## admin

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|enabled|Deprecated - use spi.enabled instead|`boolean`|`<nil>`

## api

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|defaultFilterLimit|The maximum number of rows to return if no limit is specified on an API request|`int`|`25`
|dynamicPublicURLHeader|Dynamic header that informs the backend the base public URL for the request, in order to build URL links in OpenAPI/SwaggerUI|`string`|`<nil>`
|maxFilterLimit|The largest value of `limit` that an HTTP client can specify in a request|`int`|`1000`
|passthroughHeaders|A list of HTTP request headers to pass through to dependency microservices|`[]string`|`[]`
|requestMaxTimeout|The maximum amount of time that an HTTP client can specify in a `Request-Timeout` header to keep a specific request open|[`time.Duration`](https://pkg.go.dev/time#Duration)|`10m`
|requestTimeout|The maximum amount of time that a request is allowed to remain open|[`time.Duration`](https://pkg.go.dev/time#Duration)|`120s`

## asset.manager

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|keyNormalization|Mechanism to normalize keys before using them. Valid options are `blockchain_plugin` - use blockchain plugin (default) or `none` - do not attempt normalization (deprecated - use namespaces.predefined[].asset.manager.keyNormalization)|`string`|`blockchain_plugin`

## batch.manager

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|minimumPollDelay|The minimum time the batch manager waits between polls on the DB - to prevent thrashing|[`time.Duration`](https://pkg.go.dev/time#Duration)|`100ms`
|pollTimeout|How long to wait without any notifications of new messages before doing a page query|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|readPageSize|The size of each page of messages read from the database into memory when assembling batches|`int`|`100`

## batch.retry

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|factor|The retry backoff factor|`float32`|`2`
|initDelay|The initial retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`250ms`
|maxDelay|The maximum retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`

## blobreceiver.retry

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|factor|The retry backoff factor|`float32`|`2`
|initialDelay|The initial retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`250ms`
|maxDelay|The maximum retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`1m`

## blobreceiver.worker

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|batchMaxInserts|The maximum number of items the blob receiver worker will insert in a batch|`int`|`200`
|batchTimeout|The maximum amount of the the blob receiver worker will wait|[`time.Duration`](https://pkg.go.dev/time#Duration)|`50ms`
|count|The number of blob receiver workers|`int`|`5`

## broadcast.batch

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|agentTimeout|How long to keep around a batching agent for a sending identity before disposal|`string`|`2m`
|payloadLimit|The maximum payload size of a batch for broadcast messages|[`BytesSize`](https://pkg.go.dev/github.com/docker/go-units#BytesSize)|`800Kb`
|size|The maximum number of messages that can be packed into a batch|`int`|`200`
|timeout|The timeout to wait for a batch to fill, before sending|[`time.Duration`](https://pkg.go.dev/time#Duration)|`1s`

## cache

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|enabled|Enables caching, defaults to true|`boolean`|`true`

## cache.addressresolver

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|limit|Max number of cached items for address resolver|`int`|`1000`
|ttl|Time to live of cached items for address resolver|`string`|`24h`

## cache.batch

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|limit|Max number of cached items for batches|`int`|`100`
|ttl|Time to live of cache items for batches|`string`|`5m`

## cache.blockchain

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|limit|Max number of cached items for blockchain|`int`|`100`
|ttl|Time to live of cached items for blockchain|`string`|`5m`

## cache.blockchainevent

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|limit|Max number of cached blockchain events for transactions|`int`|`1000`
|ttl|Time to live of cached blockchain events for transactions|`string`|`5m`

## cache.eventlistenertopic

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|limit|Max number of cached items for blockchain listener topics|`int`|`100`
|ttl|Time to live of cached items for blockchain listener topics|`string`|`5m`

## cache.group

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|limit|Max number of cached items for groups|`int`|`50`
|ttl|Time to live of cached items for groups|`string`|`1h`

## cache.identity

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|limit|Max number of cached identities for identity manager|`int`|`100`
|ttl|Time to live of cached identities for identity manager|`string`|`1h`

## cache.message

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|size|Max size of cached messages for data manager|[`BytesSize`](https://pkg.go.dev/github.com/docker/go-units#BytesSize)|`50Mb`
|ttl|Time to live of cached messages for data manager|`string`|`5m`

## cache.methods

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|limit|Max number of cached items for schema validations on blockchain methods|`int`|`200`
|ttl|Time to live of cached items for schema validations on blockchain methods|`string`|`5m`

## cache.operations

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|limit|Max number of cached items for operations|`int`|`1000`
|ttl|Time to live of cached items for operations|`string`|`5m`

## cache.tokenpool

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|limit|Max number of cached items for token pools|`int`|`100`
|ttl|Time to live of cached items for token pool|`string`|`1h`

## cache.transaction

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|size|Max size of cached transactions|[`BytesSize`](https://pkg.go.dev/github.com/docker/go-units#BytesSize)|`1Mb`
|ttl|Time to live of cached transactions|`string`|`5m`

## cache.validator

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|size|Max size of cached validators for data manager|[`BytesSize`](https://pkg.go.dev/github.com/docker/go-units#BytesSize)|`1Mb`
|ttl|Time to live of cached validators for data manager|`string`|`1h`

## config

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|autoReload|Monitor the configuration file for changes, and automatically add/remove/reload namespaces and plugins|`boolean`|`<nil>`

## cors

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|credentials|CORS setting to control whether a browser allows credentials to be sent to this API|`boolean`|`true`
|debug|Whether debug is enabled for the CORS implementation|`boolean`|`false`
|enabled|Whether CORS is enabled|`boolean`|`true`
|headers|CORS setting to control the allowed headers|`[]string`|`[*]`
|maxAge|The maximum age a browser should rely on CORS checks|[`time.Duration`](https://pkg.go.dev/time#Duration)|`600`
|methods| CORS setting to control the allowed methods|`[]string`|`[GET POST PUT PATCH DELETE]`
|origins|CORS setting to control the allowed origins|`[]string`|`[*]`

## debug

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|address|The HTTP interface the go debugger binds to|`string`|`localhost`
|port|An HTTP port on which to enable the go debugger|`int`|`-1`

## download.retry

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|factor|The retry backoff factor|`float32`|`2`
|initialDelay|The initial retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`100ms`
|maxAttempts|The maximum number attempts|`int`|`100`
|maxDelay|The maximum retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`1m`

## download.worker

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|count|The number of download workers|`int`|`10`
|queueLength|The length of the work queue in the channel to the workers - defaults to 2x the worker count|`int`|`<nil>`

## event.aggregator

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|batchSize|The maximum number of records to read from the DB before performing an aggregation run|[`BytesSize`](https://pkg.go.dev/github.com/docker/go-units#BytesSize)|`200`
|batchTimeout|How long to wait for new events to arrive before performing aggregation on a page of events|[`time.Duration`](https://pkg.go.dev/time#Duration)|`0ms`
|firstEvent|The first event the aggregator should process, if no previous offest is stored in the DB. Valid options are `oldest` or `newest`|`string`|`oldest`
|pollTimeout|The time to wait without a notification of new events, before trying a select on the table|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|rewindQueryLimit|Safety limit on the maximum number of records to search when performing queries to search for rewinds|`int`|`1000`
|rewindQueueLength|The size of the queue into the rewind dispatcher|`int`|`10`
|rewindTimeout|The minimum time to wait for rewinds to accumulate before resolving them|[`time.Duration`](https://pkg.go.dev/time#Duration)|`50ms`

## event.aggregator.retry

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|factor|The retry backoff factor|`float32`|`2`
|initDelay|The initial retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`100ms`
|maxDelay|The maximum retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`

## event.dbevents

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|bufferSize|The size of the buffer of change events|[`BytesSize`](https://pkg.go.dev/github.com/docker/go-units#BytesSize)|`100`

## event.dispatcher

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|batchTimeout|A short time to wait for new events to arrive before re-polling for new events|[`time.Duration`](https://pkg.go.dev/time#Duration)|`0ms`
|bufferLength|The number of events + attachments an individual dispatcher should hold in memory ready for delivery to the subscription|`int`|`5`
|pollTimeout|The time to wait without a notification of new events, before trying a select on the table|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`

## event.dispatcher.retry

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|factor|The retry backoff factor|`float32`|`<nil>`
|initDelay|The initial retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`<nil>`
|maxDelay|The maximum retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`<nil>`

## event.transports

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|default|The default event transport for new subscriptions|`string`|`websockets`
|enabled|Which event interface plugins are enabled|`boolean`|`[websockets webhooks]`

## events.webhooks

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|connectionTimeout|The maximum amount of time that a connection is allowed to remain with no data transmitted|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|expectContinueTimeout|See [ExpectContinueTimeout in the Go docs](https://pkg.go.dev/net/http#Transport)|[`time.Duration`](https://pkg.go.dev/time#Duration)|`1s`
|headers|Adds custom headers to HTTP requests|`map[string]string`|`<nil>`
|idleTimeout|The max duration to hold a HTTP keepalive connection between calls|[`time.Duration`](https://pkg.go.dev/time#Duration)|`475ms`
|maxConnsPerHost|The max number of connections, per unique hostname. Zero means no limit|`int`|`0`
|maxIdleConns|The max number of idle connections to hold pooled|`int`|`100`
|maxIdleConnsPerHost|The max number of idle connections, per unique hostname. Zero means net/http uses the default of only 2.|`int`|`100`
|passthroughHeadersEnabled|Enable passing through the set of allowed HTTP request headers|`boolean`|`false`
|requestTimeout|The maximum amount of time that a request is allowed to remain open|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|tlsHandshakeTimeout|The maximum amount of time to wait for a successful TLS handshake|[`time.Duration`](https://pkg.go.dev/time#Duration)|`10s`

## events.webhooks.auth

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|password|Password|`string`|`<nil>`
|username|Username|`string`|`<nil>`

## events.webhooks.proxy

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|url|Optional HTTP proxy server to connect through|`string`|`<nil>`

## events.webhooks.retry

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|count|The maximum number of times to retry|`int`|`5`
|enabled|Enables retries|`boolean`|`false`
|errorStatusCodeRegex|The regex that the error response status code must match to trigger retry|`string`|`<nil>`
|initWaitTime|The initial retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`250ms`
|maxWaitTime|The maximum retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`

## events.webhooks.throttle

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|burst|The maximum number of requests that can be made in a short period of time before the throttling kicks in.|`int`|`<nil>`
|requestsPerSecond|The average rate at which requests are allowed to pass through over time.|`int`|`<nil>`

## events.webhooks.tls

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|ca|The TLS certificate authority in PEM format (this option is ignored if caFile is also set)|`string`|`<nil>`
|caFile|The path to the CA file for TLS on this API|`string`|`<nil>`
|cert|The TLS certificate in PEM format (this option is ignored if certFile is also set)|`string`|`<nil>`
|certFile|The path to the certificate file for TLS on this API|`string`|`<nil>`
|clientAuth|Enables or disables client auth for TLS on this API|`string`|`<nil>`
|enabled|Enables or disables TLS on this API|`boolean`|`false`
|insecureSkipHostVerify|When to true in unit test development environments to disable TLS verification. Use with extreme caution|`boolean`|`<nil>`
|key|The TLS certificate key in PEM format (this option is ignored if keyFile is also set)|`string`|`<nil>`
|keyFile|The path to the private key file for TLS on this API|`string`|`<nil>`
|requiredDNAttributes|A set of required subject DN attributes. Each entry is a regular expression, and the subject certificate must have a matching attribute of the specified type (CN, C, O, OU, ST, L, STREET, POSTALCODE, SERIALNUMBER are valid attributes)|`map[string]string`|`<nil>`

## events.websockets

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|readBufferSize|WebSocket read buffer size|[`BytesSize`](https://pkg.go.dev/github.com/docker/go-units#BytesSize)|`16Kb`
|writeBufferSize|WebSocket write buffer size|[`BytesSize`](https://pkg.go.dev/github.com/docker/go-units#BytesSize)|`16Kb`

## histograms

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|maxChartRows|The maximum rows to fetch for each histogram bucket|`int`|`100`

## http

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|address|The IP address on which the HTTP API should listen|IP Address `string`|`127.0.0.1`
|port|The port on which the HTTP API should listen|`int`|`5000`
|publicURL|The fully qualified public URL for the API. This is used for building URLs in HTTP responses and in OpenAPI Spec generation|URL `string`|`<nil>`
|readTimeout|The maximum time to wait when reading from an HTTP connection|[`time.Duration`](https://pkg.go.dev/time#Duration)|`15s`
|shutdownTimeout|The maximum amount of time to wait for any open HTTP requests to finish before shutting down the HTTP server|[`time.Duration`](https://pkg.go.dev/time#Duration)|`10s`
|writeTimeout|The maximum time to wait when writing to an HTTP connection|[`time.Duration`](https://pkg.go.dev/time#Duration)|`15s`

## http.auth

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|type|The auth plugin to use for server side authentication of requests|`string`|`<nil>`

## http.auth.basic

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|passwordfile|The path to a .htpasswd file to use for authenticating requests. Passwords should be hashed with bcrypt.|`string`|`<nil>`

## http.tls

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|ca|The TLS certificate authority in PEM format (this option is ignored if caFile is also set)|`string`|`<nil>`
|caFile|The path to the CA file for TLS on this API|`string`|`<nil>`
|cert|The TLS certificate in PEM format (this option is ignored if certFile is also set)|`string`|`<nil>`
|certFile|The path to the certificate file for TLS on this API|`string`|`<nil>`
|clientAuth|Enables or disables client auth for TLS on this API|`string`|`<nil>`
|enabled|Enables or disables TLS on this API|`boolean`|`false`
|insecureSkipHostVerify|When to true in unit test development environments to disable TLS verification. Use with extreme caution|`boolean`|`<nil>`
|key|The TLS certificate key in PEM format (this option is ignored if keyFile is also set)|`string`|`<nil>`
|keyFile|The path to the private key file for TLS on this API|`string`|`<nil>`
|requiredDNAttributes|A set of required subject DN attributes. Each entry is a regular expression, and the subject certificate must have a matching attribute of the specified type (CN, C, O, OU, ST, L, STREET, POSTALCODE, SERIALNUMBER are valid attributes)|`map[string]string`|`<nil>`

## log

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|compress|Determines if the rotated log files should be compressed using gzip|`boolean`|`<nil>`
|filename|Filename is the file to write logs to.  Backup log files will be retained in the same directory|`string`|`<nil>`
|filesize|MaxSize is the maximum size the log file before it gets rotated|[`BytesSize`](https://pkg.go.dev/github.com/docker/go-units#BytesSize)|`100m`
|forceColor|Force color to be enabled, even when a non-TTY output is detected|`boolean`|`<nil>`
|includeCodeInfo|Enables the report caller for including the calling file and line number, and the calling function. If using text logs, it uses the logrus text format rather than the default prefix format.|`boolean`|`false`
|level|The log level - error, warn, info, debug, trace|`string`|`info`
|maxAge|The maximum time to retain old log files based on the timestamp encoded in their filename|[`time.Duration`](https://pkg.go.dev/time#Duration)|`24h`
|maxBackups|Maximum number of old log files to retain|`int`|`2`
|noColor|Force color to be disabled, event when TTY output is detected|`boolean`|`<nil>`
|timeFormat|Custom time format for logs|[Time format](https://pkg.go.dev/time#pkg-constants) `string`|`2006-01-02T15:04:05.000Z07:00`
|utc|Use UTC timestamps for logs|`boolean`|`false`

## log.json

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|enabled|Enables JSON formatted logs rather than text. All log color settings are ignored when enabled.|`boolean`|`false`

## log.json.fields

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|file|configures the JSON key containing the calling file|`string`|`file`
|func|Configures the JSON key containing the calling function|`string`|`func`
|level|Configures the JSON key containing the log level|`string`|`level`
|message|Configures the JSON key containing the log message|`string`|`message`
|timestamp|Configures the JSON key containing the timestamp of the log|`string`|`@timestamp`

## message.writer

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|batchMaxInserts|The maximum number of database inserts to include when writing a single batch of messages + data|`int`|`200`
|batchTimeout|How long to wait for more messages to arrive before flushing the batch|[`time.Duration`](https://pkg.go.dev/time#Duration)|`10ms`
|count|The number of message writer workers|`int`|`5`

## metrics

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|address|Deprecated - use monitoring.address instead|`int`|`127.0.0.1`
|enabled|Deprecated - use monitoring.enabled instead|`boolean`|`true`
|path|Deprecated - use monitoring.metricsPath instead|`string`|`/metrics`
|port|Deprecated - use monitoring.port instead|`int`|`6000`
|publicURL|Deprecated - use monitoring.publicURL instead|URL `string`|`<nil>`
|readTimeout|Deprecated - use monitoring.readTimeout instead|[`time.Duration`](https://pkg.go.dev/time#Duration)|`15s`
|shutdownTimeout|The maximum amount of time to wait for any open HTTP requests to finish before shutting down the HTTP server|[`time.Duration`](https://pkg.go.dev/time#Duration)|`10s`
|writeTimeout|Deprecated - use monitoring.writeTimeout instead|[`time.Duration`](https://pkg.go.dev/time#Duration)|`15s`

## metrics.auth

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|type|The auth plugin to use for server side authentication of requests|`string`|`<nil>`

## metrics.auth.basic

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|passwordfile|The path to a .htpasswd file to use for authenticating requests. Passwords should be hashed with bcrypt.|`string`|`<nil>`

## metrics.tls

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|ca|The TLS certificate authority in PEM format (this option is ignored if caFile is also set)|`string`|`<nil>`
|caFile|The path to the CA file for TLS on this API|`string`|`<nil>`
|cert|The TLS certificate in PEM format (this option is ignored if certFile is also set)|`string`|`<nil>`
|certFile|The path to the certificate file for TLS on this API|`string`|`<nil>`
|clientAuth|Enables or disables client auth for TLS on this API|`string`|`<nil>`
|enabled|Enables or disables TLS on this API|`boolean`|`false`
|insecureSkipHostVerify|When to true in unit test development environments to disable TLS verification. Use with extreme caution|`boolean`|`<nil>`
|key|The TLS certificate key in PEM format (this option is ignored if keyFile is also set)|`string`|`<nil>`
|keyFile|The path to the private key file for TLS on this API|`string`|`<nil>`
|requiredDNAttributes|A set of required subject DN attributes. Each entry is a regular expression, and the subject certificate must have a matching attribute of the specified type (CN, C, O, OU, ST, L, STREET, POSTALCODE, SERIALNUMBER are valid attributes)|`map[string]string`|`<nil>`

## monitoring

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|address|The IP address on which the metrics HTTP API should listen|`int`|`127.0.0.1`
|enabled|Enables the metrics API|`boolean`|`false`
|metricsPath|The path from which to serve the Prometheus metrics|`string`|`/metrics`
|port|The port on which the metrics HTTP API should listen|`int`|`6000`
|publicURL|The fully qualified public URL for the metrics API. This is used for building URLs in HTTP responses and in OpenAPI Spec generation|URL `string`|`<nil>`
|readTimeout|The maximum time to wait when reading from an HTTP connection|[`time.Duration`](https://pkg.go.dev/time#Duration)|`15s`
|shutdownTimeout|The maximum amount of time to wait for any open HTTP requests to finish before shutting down the HTTP server|[`time.Duration`](https://pkg.go.dev/time#Duration)|`10s`
|writeTimeout|The maximum time to wait when writing to an HTTP connection|[`time.Duration`](https://pkg.go.dev/time#Duration)|`15s`

## monitoring.auth

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|type|The auth plugin to use for server side authentication of requests|`string`|`<nil>`

## monitoring.auth.basic

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|passwordfile|The path to a .htpasswd file to use for authenticating requests. Passwords should be hashed with bcrypt.|`string`|`<nil>`

## monitoring.tls

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|ca|The TLS certificate authority in PEM format (this option is ignored if caFile is also set)|`string`|`<nil>`
|caFile|The path to the CA file for TLS on this API|`string`|`<nil>`
|cert|The TLS certificate in PEM format (this option is ignored if certFile is also set)|`string`|`<nil>`
|certFile|The path to the certificate file for TLS on this API|`string`|`<nil>`
|clientAuth|Enables or disables client auth for TLS on this API|`string`|`<nil>`
|enabled|Enables or disables TLS on this API|`boolean`|`false`
|insecureSkipHostVerify|When to true in unit test development environments to disable TLS verification. Use with extreme caution|`boolean`|`<nil>`
|key|The TLS certificate key in PEM format (this option is ignored if keyFile is also set)|`string`|`<nil>`
|keyFile|The path to the private key file for TLS on this API|`string`|`<nil>`
|requiredDNAttributes|A set of required subject DN attributes. Each entry is a regular expression, and the subject certificate must have a matching attribute of the specified type (CN, C, O, OU, ST, L, STREET, POSTALCODE, SERIALNUMBER are valid attributes)|`map[string]string`|`<nil>`

## namespaces

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|default|The default namespace - must be in the predefined list|`string`|`default`
|predefined|A list of namespaces to ensure exists, without requiring a broadcast from the network|List `string`|`<nil>`

## namespaces.predefined[]

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|defaultKey|A default signing key for blockchain transactions within this namespace|`string`|`<nil>`
|description|A description for the namespace|`string`|`<nil>`
|name|The name of the namespace (must be unique)|`string`|`<nil>`
|plugins|The list of plugins for this namespace|`string`|`<nil>`

## namespaces.predefined[].asset.manager

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|keyNormalization|Mechanism to normalize keys before using them. Valid options are `blockchain_plugin` - use blockchain plugin (default) or `none` - do not attempt normalization|`string`|`<nil>`

## namespaces.predefined[].multiparty

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|enabled|Enables multi-party mode for this namespace (defaults to true if an org name or key is configured, either here or at the root level)|`boolean`|`<nil>`
|networknamespace|The shared namespace name to be sent in multiparty messages, if it differs from the local namespace name|`string`|`<nil>`

## namespaces.predefined[].multiparty.contract[]

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|firstEvent|The first event the contract should process. Valid options are `oldest` or `newest`|`string`|`<nil>`
|location|A blockchain-specific contract location. For example, an Ethereum contract address, or a Fabric chaincode name and channel|`string`|`<nil>`
|options|Blockchain-specific contract options|`string`|`<nil>`

## namespaces.predefined[].multiparty.node

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|description|A description for the node in this namespace|`string`|`<nil>`
|name|The node name for this namespace|`string`|`<nil>`

## namespaces.predefined[].multiparty.org

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|description|A description for the local root organization within this namespace|`string`|`<nil>`
|key|The signing key allocated to the root organization within this namespace|`string`|`<nil>`
|name|A short name for the local root organization within this namespace|`string`|`<nil>`

## namespaces.predefined[].tlsConfigs[]

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|name|Name of the TLS Config|`string`|`<nil>`

## namespaces.predefined[].tlsConfigs[].tls

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|ca|The TLS certificate authority in PEM format (this option is ignored if caFile is also set)|`string`|`<nil>`
|caFile|The path to the CA file for TLS on this API|`string`|`<nil>`
|cert|The TLS certificate in PEM format (this option is ignored if certFile is also set)|`string`|`<nil>`
|certFile|The path to the certificate file for TLS on this API|`string`|`<nil>`
|clientAuth|Enables or disables client auth for TLS on this API|`string`|`<nil>`
|enabled|Enables or disables TLS on this API|`boolean`|`false`
|insecureSkipHostVerify|When to true in unit test development environments to disable TLS verification. Use with extreme caution|`boolean`|`<nil>`
|key|The TLS certificate key in PEM format (this option is ignored if keyFile is also set)|`string`|`<nil>`
|keyFile|The path to the private key file for TLS on this API|`string`|`<nil>`
|requiredDNAttributes|A set of required subject DN attributes. Each entry is a regular expression, and the subject certificate must have a matching attribute of the specified type (CN, C, O, OU, ST, L, STREET, POSTALCODE, SERIALNUMBER are valid attributes)|`map[string]string`|`<nil>`

## namespaces.retry

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|factor|The retry backoff factor|`float32`|`2`
|initDelay|The initial retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`5s`
|maxDelay|The maximum retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`1m`

## node

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|description|The description of this FireFly node|`string`|`<nil>`
|name|The name of this FireFly node|`string`|`<nil>`

## opupdate.retry

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|factor|The retry backoff factor|`float32`|`2`
|initialDelay|The initial retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`250ms`
|maxDelay|The maximum retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`1m`

## opupdate.worker

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|batchMaxInserts|The maximum number of database inserts to include when writing a single batch of messages + data|`int`|`200`
|batchTimeout|How long to wait for more messages to arrive before flushing the batch|[`time.Duration`](https://pkg.go.dev/time#Duration)|`50ms`
|count|The number of operation update works|`int`|`5`
|queueLength|The size of the queue for the Operation Update worker|`int`|`50`

## orchestrator

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|startupAttempts|The number of times to attempt to connect to core infrastructure on startup|`string`|`5`

## org

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|description|A description of the organization to which this FireFly node belongs (deprecated - should be set on each multi-party namespace instead)|`string`|`<nil>`
|key|The signing key allocated to the organization (deprecated - should be set on each multi-party namespace instead)|`string`|`<nil>`
|name|The name of the organization to which this FireFly node belongs (deprecated - should be set on each multi-party namespace instead)|`string`|`<nil>`

## plugins

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|auth|Authorization plugin configuration|`map[string]string`|`<nil>`
|blockchain|The list of configured Blockchain plugins|`string`|`<nil>`
|database|The list of configured Database plugins|`string`|`<nil>`
|dataexchange|The array of configured Data Exchange plugins |`string`|`<nil>`
|identity|The list of available Identity plugins|`string`|`<nil>`
|sharedstorage|The list of configured Shared Storage plugins|`string`|`<nil>`
|tokens|The token plugin configurations|`string`|`<nil>`

## plugins.auth[]

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|name|The name of the auth plugin to use|`string`|`<nil>`
|type|The type of the auth plugin to use|`string`|`<nil>`

## plugins.auth[].basic

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|passwordfile|The path to a .htpasswd file to use for authenticating requests. Passwords should be hashed with bcrypt.|`string`|`<nil>`

## plugins.blockchain[]

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|name|The name of the configured Blockchain plugin|`string`|`<nil>`
|type|The type of the configured Blockchain Connector plugin|`string`|`<nil>`

## plugins.blockchain[].cardano.cardanoconnect

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|batchSize|The number of events Cardanoconnect should batch together for delivery to FireFly core. Only applies when automatically creating a new event stream|`int`|`50`
|batchTimeout|How long Cardanoconnect should wait for new events to arrive and fill a batch, before sending the batch to FireFly core. Only applies when automatically creating a new event stream|[`time.Duration`](https://pkg.go.dev/time#Duration)|`500`
|connectionTimeout|The maximum amount of time that a connection is allowed to remain with no data transmitted|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|expectContinueTimeout|See [ExpectContinueTimeout in the Go docs](https://pkg.go.dev/net/http#Transport)|[`time.Duration`](https://pkg.go.dev/time#Duration)|`1s`
|headers|Adds custom headers to HTTP requests|`map[string]string`|`<nil>`
|idleTimeout|The max duration to hold a HTTP keepalive connection between calls|[`time.Duration`](https://pkg.go.dev/time#Duration)|`475ms`
|maxConnsPerHost|The max number of connections, per unique hostname. Zero means no limit|`int`|`0`
|maxIdleConns|The max number of idle connections to hold pooled|`int`|`100`
|maxIdleConnsPerHost|The max number of idle connections, per unique hostname. Zero means net/http uses the default of only 2.|`int`|`100`
|passthroughHeadersEnabled|Enable passing through the set of allowed HTTP request headers|`boolean`|`false`
|requestTimeout|The maximum amount of time that a request is allowed to remain open|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|tlsHandshakeTimeout|The maximum amount of time to wait for a successful TLS handshake|[`time.Duration`](https://pkg.go.dev/time#Duration)|`10s`
|topic|The websocket listen topic that the node should register on, which is important if there are multiple nodes using a single cardanoconnect|`string`|`<nil>`
|url|The URL of the Cardanoconnect instance|URL `string`|`<nil>`

## plugins.blockchain[].cardano.cardanoconnect.auth

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|password|Password|`string`|`<nil>`
|username|Username|`string`|`<nil>`

## plugins.blockchain[].cardano.cardanoconnect.proxy

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|url|Optional HTTP proxy server to connect through|`string`|`<nil>`

## plugins.blockchain[].cardano.cardanoconnect.retry

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|count|The maximum number of times to retry|`int`|`5`
|enabled|Enables retries|`boolean`|`false`
|errorStatusCodeRegex|The regex that the error response status code must match to trigger retry|`string`|`<nil>`
|factor|The retry backoff factor|`float32`|`2`
|initWaitTime|The initial retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`250ms`
|maxWaitTime|The maximum retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`

## plugins.blockchain[].cardano.cardanoconnect.throttle

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|burst|The maximum number of requests that can be made in a short period of time before the throttling kicks in.|`int`|`<nil>`
|requestsPerSecond|The average rate at which requests are allowed to pass through over time.|`int`|`<nil>`

## plugins.blockchain[].cardano.cardanoconnect.tls

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|ca|The TLS certificate authority in PEM format (this option is ignored if caFile is also set)|`string`|`<nil>`
|caFile|The path to the CA file for TLS on this API|`string`|`<nil>`
|cert|The TLS certificate in PEM format (this option is ignored if certFile is also set)|`string`|`<nil>`
|certFile|The path to the certificate file for TLS on this API|`string`|`<nil>`
|clientAuth|Enables or disables client auth for TLS on this API|`string`|`<nil>`
|enabled|Enables or disables TLS on this API|`boolean`|`false`
|insecureSkipHostVerify|When to true in unit test development environments to disable TLS verification. Use with extreme caution|`boolean`|`<nil>`
|key|The TLS certificate key in PEM format (this option is ignored if keyFile is also set)|`string`|`<nil>`
|keyFile|The path to the private key file for TLS on this API|`string`|`<nil>`
|requiredDNAttributes|A set of required subject DN attributes. Each entry is a regular expression, and the subject certificate must have a matching attribute of the specified type (CN, C, O, OU, ST, L, STREET, POSTALCODE, SERIALNUMBER are valid attributes)|`map[string]string`|`<nil>`

## plugins.blockchain[].cardano.cardanoconnect.ws

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|backgroundConnect|When true the connection is established in the background with infinite reconnect (makes initialConnectAttempts redundant when set)|`boolean`|`false`
|connectionTimeout|The amount of time to wait while establishing a connection (or auto-reconnection)|[`time.Duration`](https://pkg.go.dev/time#Duration)|`45s`
|heartbeatInterval|The amount of time to wait between heartbeat signals on the WebSocket connection|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|initialConnectAttempts|The number of attempts FireFly will make to connect to the WebSocket when starting up, before failing|`int`|`5`
|path|The WebSocket sever URL to which FireFly should connect|WebSocket URL `string`|`<nil>`
|readBufferSize|The size in bytes of the read buffer for the WebSocket connection|[`BytesSize`](https://pkg.go.dev/github.com/docker/go-units#BytesSize)|`16Kb`
|url|URL to use for WebSocket - overrides url one level up (in the HTTP config)|`string`|`<nil>`
|writeBufferSize|The size in bytes of the write buffer for the WebSocket connection|[`BytesSize`](https://pkg.go.dev/github.com/docker/go-units#BytesSize)|`16Kb`

## plugins.blockchain[].ethereum.addressResolver

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|alwaysResolve|Causes the address resolver to be invoked on every API call that submits a signing key, regardless of whether the input string conforms to an 0x address. Also disables any result caching|`boolean`|`<nil>`
|bodyTemplate|The body go template string to use when making HTTP requests. The template input contains '.Key' and '.Intent' string variables.|[Go Template](https://pkg.go.dev/text/template) `string`|`<nil>`
|connectionTimeout|The maximum amount of time that a connection is allowed to remain with no data transmitted|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|expectContinueTimeout|See [ExpectContinueTimeout in the Go docs](https://pkg.go.dev/net/http#Transport)|[`time.Duration`](https://pkg.go.dev/time#Duration)|`1s`
|headers|Adds custom headers to HTTP requests|`string`|`<nil>`
|idleTimeout|The max duration to hold a HTTP keepalive connection between calls|[`time.Duration`](https://pkg.go.dev/time#Duration)|`475ms`
|maxConnsPerHost|The max number of connections, per unique hostname. Zero means no limit|`int`|`0`
|maxIdleConns|The max number of idle connections to hold pooled|`int`|`100`
|maxIdleConnsPerHost|The max number of idle connections, per unique hostname. Zero means net/http uses the default of only 2.|`int`|`100`
|method|The HTTP method to use when making requests to the Address Resolver|`string`|`GET`
|passthroughHeadersEnabled|Enable passing through the set of allowed HTTP request headers|`boolean`|`false`
|requestTimeout|The maximum amount of time that a request is allowed to remain open|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|responseField|The name of a JSON field that is provided in the response, that contains the ethereum address (default `address`)|`string`|`address`
|retainOriginal|When true the original pre-resolved string is retained after the lookup, and passed down to Ethconnect as the from address|`boolean`|`<nil>`
|tlsHandshakeTimeout|The maximum amount of time to wait for a successful TLS handshake|[`time.Duration`](https://pkg.go.dev/time#Duration)|`10s`
|url|The URL of the Address Resolver|`string`|`<nil>`
|urlTemplate|The URL Go template string to use when calling the Address Resolver. The template input contains '.Key' and '.Intent' string variables.|[Go Template](https://pkg.go.dev/text/template) `string`|`<nil>`

## plugins.blockchain[].ethereum.addressResolver.auth

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|password|Password|`string`|`<nil>`
|username|Username|`string`|`<nil>`

## plugins.blockchain[].ethereum.addressResolver.proxy

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|url|Optional HTTP proxy server to use when connecting to the Address Resolver|URL `string`|`<nil>`

## plugins.blockchain[].ethereum.addressResolver.retry

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|count|The maximum number of times to retry|`int`|`5`
|enabled|Enables retries|`boolean`|`false`
|errorStatusCodeRegex|The regex that the error response status code must match to trigger retry|`string`|`<nil>`
|initWaitTime|The initial retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`250ms`
|maxWaitTime|The maximum retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`

## plugins.blockchain[].ethereum.addressResolver.throttle

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|burst|The maximum number of requests that can be made in a short period of time before the throttling kicks in.|`int`|`<nil>`
|requestsPerSecond|The average rate at which requests are allowed to pass through over time.|`int`|`<nil>`

## plugins.blockchain[].ethereum.addressResolver.tls

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|ca|The TLS certificate authority in PEM format (this option is ignored if caFile is also set)|`string`|`<nil>`
|caFile|The path to the CA file for TLS on this API|`string`|`<nil>`
|cert|The TLS certificate in PEM format (this option is ignored if certFile is also set)|`string`|`<nil>`
|certFile|The path to the certificate file for TLS on this API|`string`|`<nil>`
|clientAuth|Enables or disables client auth for TLS on this API|`string`|`<nil>`
|enabled|Enables or disables TLS on this API|`boolean`|`false`
|insecureSkipHostVerify|When to true in unit test development environments to disable TLS verification. Use with extreme caution|`boolean`|`<nil>`
|key|The TLS certificate key in PEM format (this option is ignored if keyFile is also set)|`string`|`<nil>`
|keyFile|The path to the private key file for TLS on this API|`string`|`<nil>`
|requiredDNAttributes|A set of required subject DN attributes. Each entry is a regular expression, and the subject certificate must have a matching attribute of the specified type (CN, C, O, OU, ST, L, STREET, POSTALCODE, SERIALNUMBER are valid attributes)|`map[string]string`|`<nil>`

## plugins.blockchain[].ethereum.ethconnect

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|batchSize|The number of events Ethconnect should batch together for delivery to FireFly core. Only applies when automatically creating a new event stream|`int`|`50`
|batchTimeout|How long Ethconnect should wait for new events to arrive and fill a batch, before sending the batch to FireFly core. Only applies when automatically creating a new event stream|[`time.Duration`](https://pkg.go.dev/time#Duration)|`500`
|connectionTimeout|The maximum amount of time that a connection is allowed to remain with no data transmitted|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|expectContinueTimeout|See [ExpectContinueTimeout in the Go docs](https://pkg.go.dev/net/http#Transport)|[`time.Duration`](https://pkg.go.dev/time#Duration)|`1s`
|fromBlock|The first event this FireFly instance should listen to from the BatchPin smart contract. Default=0. Only affects initial creation of the event stream|Address `string`|`0`
|headers|Adds custom headers to HTTP requests|`map[string]string`|`<nil>`
|idleTimeout|The max duration to hold a HTTP keepalive connection between calls|[`time.Duration`](https://pkg.go.dev/time#Duration)|`475ms`
|instance|The Ethereum address of the FireFly BatchPin smart contract that has been deployed to the blockchain|Address `string`|`<nil>`
|maxConnsPerHost|The max number of connections, per unique hostname. Zero means no limit|`int`|`0`
|maxIdleConns|The max number of idle connections to hold pooled|`int`|`100`
|maxIdleConnsPerHost|The max number of idle connections, per unique hostname. Zero means net/http uses the default of only 2.|`int`|`100`
|passthroughHeadersEnabled|Enable passing through the set of allowed HTTP request headers|`boolean`|`false`
|prefixLong|The prefix that will be used for Ethconnect specific HTTP headers when FireFly makes requests to Ethconnect|`string`|`firefly`
|prefixShort|The prefix that will be used for Ethconnect specific query parameters when FireFly makes requests to Ethconnect|`string`|`fly`
|requestTimeout|The maximum amount of time that a request is allowed to remain open|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|tlsHandshakeTimeout|The maximum amount of time to wait for a successful TLS handshake|[`time.Duration`](https://pkg.go.dev/time#Duration)|`10s`
|topic|The websocket listen topic that the node should register on, which is important if there are multiple nodes using a single ethconnect|`string`|`<nil>`
|url|The URL of the Ethconnect instance|URL `string`|`<nil>`

## plugins.blockchain[].ethereum.ethconnect.auth

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|password|Password|`string`|`<nil>`
|username|Username|`string`|`<nil>`

## plugins.blockchain[].ethereum.ethconnect.backgroundStart

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|enabled|Start the Ethconnect plugin in the background and enter retry loop if failed to start|`boolean`|`<nil>`
|factor|Set the factor by which the delay increases when retrying|`float32`|`2`
|initialDelay|Delay between restarts in the case where we retry to restart the ethereum plugin|[`time.Duration`](https://pkg.go.dev/time#Duration)|`5s`
|maxDelay|Max delay between restarts in the case where we retry to restart the ethereum plugin|[`time.Duration`](https://pkg.go.dev/time#Duration)|`1m`

## plugins.blockchain[].ethereum.ethconnect.proxy

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|url|Optional HTTP proxy server to use when connecting to Ethconnect|URL `string`|`<nil>`

## plugins.blockchain[].ethereum.ethconnect.retry

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|count|The maximum number of times to retry|`int`|`5`
|enabled|Enables retries|`boolean`|`false`
|errorStatusCodeRegex|The regex that the error response status code must match to trigger retry|`string`|`<nil>`
|factor|The retry backoff factor|`float32`|`2`
|initWaitTime|The initial retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`250ms`
|maxWaitTime|The maximum retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`

## plugins.blockchain[].ethereum.ethconnect.throttle

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|burst|The maximum number of requests that can be made in a short period of time before the throttling kicks in.|`int`|`<nil>`
|requestsPerSecond|The average rate at which requests are allowed to pass through over time.|`int`|`<nil>`

## plugins.blockchain[].ethereum.ethconnect.tls

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|ca|The TLS certificate authority in PEM format (this option is ignored if caFile is also set)|`string`|`<nil>`
|caFile|The path to the CA file for TLS on this API|`string`|`<nil>`
|cert|The TLS certificate in PEM format (this option is ignored if certFile is also set)|`string`|`<nil>`
|certFile|The path to the certificate file for TLS on this API|`string`|`<nil>`
|clientAuth|Enables or disables client auth for TLS on this API|`string`|`<nil>`
|enabled|Enables or disables TLS on this API|`boolean`|`false`
|insecureSkipHostVerify|When to true in unit test development environments to disable TLS verification. Use with extreme caution|`boolean`|`<nil>`
|key|The TLS certificate key in PEM format (this option is ignored if keyFile is also set)|`string`|`<nil>`
|keyFile|The path to the private key file for TLS on this API|`string`|`<nil>`
|requiredDNAttributes|A set of required subject DN attributes. Each entry is a regular expression, and the subject certificate must have a matching attribute of the specified type (CN, C, O, OU, ST, L, STREET, POSTALCODE, SERIALNUMBER are valid attributes)|`map[string]string`|`<nil>`

## plugins.blockchain[].ethereum.ethconnect.ws

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|backgroundConnect|When true the connection is established in the background with infinite reconnect (makes initialConnectAttempts redundant when set)|`boolean`|`false`
|connectionTimeout|The amount of time to wait while establishing a connection (or auto-reconnection)|[`time.Duration`](https://pkg.go.dev/time#Duration)|`45s`
|heartbeatInterval|The amount of time to wait between heartbeat signals on the WebSocket connection|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|initialConnectAttempts|The number of attempts FireFly will make to connect to the WebSocket when starting up, before failing|`int`|`5`
|path|The WebSocket sever URL to which FireFly should connect|WebSocket URL `string`|`<nil>`
|readBufferSize|The size in bytes of the read buffer for the WebSocket connection|[`BytesSize`](https://pkg.go.dev/github.com/docker/go-units#BytesSize)|`16Kb`
|url|URL to use for WebSocket - overrides url one level up (in the HTTP config)|`string`|`<nil>`
|writeBufferSize|The size in bytes of the write buffer for the WebSocket connection|[`BytesSize`](https://pkg.go.dev/github.com/docker/go-units#BytesSize)|`16Kb`

## plugins.blockchain[].ethereum.fftm

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|connectionTimeout|The maximum amount of time that a connection is allowed to remain with no data transmitted|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|expectContinueTimeout|See [ExpectContinueTimeout in the Go docs](https://pkg.go.dev/net/http#Transport)|[`time.Duration`](https://pkg.go.dev/time#Duration)|`1s`
|headers|Adds custom headers to HTTP requests|`map[string]string`|`<nil>`
|idleTimeout|The max duration to hold a HTTP keepalive connection between calls|[`time.Duration`](https://pkg.go.dev/time#Duration)|`475ms`
|maxConnsPerHost|The max number of connections, per unique hostname. Zero means no limit|`int`|`0`
|maxIdleConns|The max number of idle connections to hold pooled|`int`|`100`
|maxIdleConnsPerHost|The max number of idle connections, per unique hostname. Zero means net/http uses the default of only 2.|`int`|`100`
|passthroughHeadersEnabled|Enable passing through the set of allowed HTTP request headers|`boolean`|`false`
|requestTimeout|The maximum amount of time that a request is allowed to remain open|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|tlsHandshakeTimeout|The maximum amount of time to wait for a successful TLS handshake|[`time.Duration`](https://pkg.go.dev/time#Duration)|`10s`
|url|The URL of the FireFly Transaction Manager runtime, if enabled|`string`|`<nil>`

## plugins.blockchain[].ethereum.fftm.auth

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|password|Password|`string`|`<nil>`
|username|Username|`string`|`<nil>`

## plugins.blockchain[].ethereum.fftm.proxy

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|url|Optional HTTP proxy server to use when connecting to the Transaction Manager|`string`|`<nil>`

## plugins.blockchain[].ethereum.fftm.retry

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|count|The maximum number of times to retry|`int`|`5`
|enabled|Enables retries|`boolean`|`false`
|errorStatusCodeRegex|The regex that the error response status code must match to trigger retry|`string`|`<nil>`
|initWaitTime|The initial retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`250ms`
|maxWaitTime|The maximum retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`

## plugins.blockchain[].ethereum.fftm.throttle

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|burst|The maximum number of requests that can be made in a short period of time before the throttling kicks in.|`int`|`<nil>`
|requestsPerSecond|The average rate at which requests are allowed to pass through over time.|`int`|`<nil>`

## plugins.blockchain[].ethereum.fftm.tls

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|ca|The TLS certificate authority in PEM format (this option is ignored if caFile is also set)|`string`|`<nil>`
|caFile|The path to the CA file for TLS on this API|`string`|`<nil>`
|cert|The TLS certificate in PEM format (this option is ignored if certFile is also set)|`string`|`<nil>`
|certFile|The path to the certificate file for TLS on this API|`string`|`<nil>`
|clientAuth|Enables or disables client auth for TLS on this API|`string`|`<nil>`
|enabled|Enables or disables TLS on this API|`boolean`|`false`
|insecureSkipHostVerify|When to true in unit test development environments to disable TLS verification. Use with extreme caution|`boolean`|`<nil>`
|key|The TLS certificate key in PEM format (this option is ignored if keyFile is also set)|`string`|`<nil>`
|keyFile|The path to the private key file for TLS on this API|`string`|`<nil>`
|requiredDNAttributes|A set of required subject DN attributes. Each entry is a regular expression, and the subject certificate must have a matching attribute of the specified type (CN, C, O, OU, ST, L, STREET, POSTALCODE, SERIALNUMBER are valid attributes)|`map[string]string`|`<nil>`

## plugins.blockchain[].fabric.fabconnect

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|batchSize|The number of events Fabconnect should batch together for delivery to FireFly core. Only applies when automatically creating a new event stream|`int`|`50`
|batchTimeout|The maximum amount of time to wait for a batch to complete|[`time.Duration`](https://pkg.go.dev/time#Duration)|`500`
|chaincode|The name of the Fabric chaincode that FireFly will use for BatchPin transactions (deprecated - use fireflyContract[].chaincode)|`string`|`<nil>`
|channel|The Fabric channel that FireFly will use for BatchPin transactions|`string`|`<nil>`
|connectionTimeout|The maximum amount of time that a connection is allowed to remain with no data transmitted|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|expectContinueTimeout|See [ExpectContinueTimeout in the Go docs](https://pkg.go.dev/net/http#Transport)|[`time.Duration`](https://pkg.go.dev/time#Duration)|`1s`
|headers|Adds custom headers to HTTP requests|`map[string]string`|`<nil>`
|idleTimeout|The max duration to hold a HTTP keepalive connection between calls|[`time.Duration`](https://pkg.go.dev/time#Duration)|`475ms`
|maxConnsPerHost|The max number of connections, per unique hostname. Zero means no limit|`int`|`0`
|maxIdleConns|The max number of idle connections to hold pooled|`int`|`100`
|maxIdleConnsPerHost|The max number of idle connections, per unique hostname. Zero means net/http uses the default of only 2.|`int`|`100`
|passthroughHeadersEnabled|Enable passing through the set of allowed HTTP request headers|`boolean`|`false`
|prefixLong|The prefix that will be used for Fabconnect specific HTTP headers when FireFly makes requests to Fabconnect|`string`|`firefly`
|prefixShort|The prefix that will be used for Fabconnect specific query parameters when FireFly makes requests to Fabconnect|`string`|`fly`
|requestTimeout|The maximum amount of time that a request is allowed to remain open|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|signer|The Fabric signing key to use when submitting transactions to Fabconnect|`string`|`<nil>`
|tlsHandshakeTimeout|The maximum amount of time to wait for a successful TLS handshake|[`time.Duration`](https://pkg.go.dev/time#Duration)|`10s`
|topic|The websocket listen topic that the node should register on, which is important if there are multiple nodes using a single Fabconnect|`string`|`<nil>`
|url|The URL of the Fabconnect instance|URL `string`|`<nil>`

## plugins.blockchain[].fabric.fabconnect.auth

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|password|Password|`string`|`<nil>`
|username|Username|`string`|`<nil>`

## plugins.blockchain[].fabric.fabconnect.backgroundStart

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|enabled|Start the fabric plugin in the background and enter retry loop if failed to start|`boolean`|`<nil>`
|factor|Set the factor by which the delay increases when retrying|`float32`|`2`
|initialDelay|Delay between restarts in the case where we retry to restart the fabric plugin|[`time.Duration`](https://pkg.go.dev/time#Duration)|`5s`
|maxDelay|Max delay between restarts in the case where we retry to restart the fabric plugin|[`time.Duration`](https://pkg.go.dev/time#Duration)|`1m`

## plugins.blockchain[].fabric.fabconnect.proxy

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|url|Optional HTTP proxy server to use when connecting to Fabconnect|URL `string`|`<nil>`

## plugins.blockchain[].fabric.fabconnect.retry

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|count|The maximum number of times to retry|`int`|`5`
|enabled|Enables retries|`boolean`|`false`
|errorStatusCodeRegex|The regex that the error response status code must match to trigger retry|`string`|`<nil>`
|factor|The retry backoff factor|`float32`|`2`
|initWaitTime|The initial retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`250ms`
|maxWaitTime|The maximum retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`

## plugins.blockchain[].fabric.fabconnect.throttle

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|burst|The maximum number of requests that can be made in a short period of time before the throttling kicks in.|`int`|`<nil>`
|requestsPerSecond|The average rate at which requests are allowed to pass through over time.|`int`|`<nil>`

## plugins.blockchain[].fabric.fabconnect.tls

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|ca|The TLS certificate authority in PEM format (this option is ignored if caFile is also set)|`string`|`<nil>`
|caFile|The path to the CA file for TLS on this API|`string`|`<nil>`
|cert|The TLS certificate in PEM format (this option is ignored if certFile is also set)|`string`|`<nil>`
|certFile|The path to the certificate file for TLS on this API|`string`|`<nil>`
|clientAuth|Enables or disables client auth for TLS on this API|`string`|`<nil>`
|enabled|Enables or disables TLS on this API|`boolean`|`false`
|insecureSkipHostVerify|When to true in unit test development environments to disable TLS verification. Use with extreme caution|`boolean`|`<nil>`
|key|The TLS certificate key in PEM format (this option is ignored if keyFile is also set)|`string`|`<nil>`
|keyFile|The path to the private key file for TLS on this API|`string`|`<nil>`
|requiredDNAttributes|A set of required subject DN attributes. Each entry is a regular expression, and the subject certificate must have a matching attribute of the specified type (CN, C, O, OU, ST, L, STREET, POSTALCODE, SERIALNUMBER are valid attributes)|`map[string]string`|`<nil>`

## plugins.blockchain[].fabric.fabconnect.ws

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|backgroundConnect|When true the connection is established in the background with infinite reconnect (makes initialConnectAttempts redundant when set)|`boolean`|`false`
|connectionTimeout|The amount of time to wait while establishing a connection (or auto-reconnection)|[`time.Duration`](https://pkg.go.dev/time#Duration)|`45s`
|heartbeatInterval|The amount of time to wait between heartbeat signals on the WebSocket connection|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|initialConnectAttempts|The number of attempts FireFly will make to connect to the WebSocket when starting up, before failing|`int`|`5`
|path|The WebSocket sever URL to which FireFly should connect|WebSocket URL `string`|`<nil>`
|readBufferSize|The size in bytes of the read buffer for the WebSocket connection|[`BytesSize`](https://pkg.go.dev/github.com/docker/go-units#BytesSize)|`16Kb`
|url|URL to use for WebSocket - overrides url one level up (in the HTTP config)|`string`|`<nil>`
|writeBufferSize|The size in bytes of the write buffer for the WebSocket connection|[`BytesSize`](https://pkg.go.dev/github.com/docker/go-units#BytesSize)|`16Kb`

## plugins.blockchain[].tezos.addressResolver

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|alwaysResolve|Causes the address resolver to be invoked on every API call that submits a signing key. Also disables any result caching|`boolean`|`<nil>`
|bodyTemplate|The body go template string to use when making HTTP requests|[Go Template](https://pkg.go.dev/text/template) `string`|`<nil>`
|connectionTimeout|The maximum amount of time that a connection is allowed to remain with no data transmitted|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|expectContinueTimeout|See [ExpectContinueTimeout in the Go docs](https://pkg.go.dev/net/http#Transport)|[`time.Duration`](https://pkg.go.dev/time#Duration)|`1s`
|headers|Adds custom headers to HTTP requests|`map[string]string`|`<nil>`
|idleTimeout|The max duration to hold a HTTP keepalive connection between calls|[`time.Duration`](https://pkg.go.dev/time#Duration)|`475ms`
|maxConnsPerHost|The max number of connections, per unique hostname. Zero means no limit|`int`|`0`
|maxIdleConns|The max number of idle connections to hold pooled|`int`|`100`
|maxIdleConnsPerHost|The max number of idle connections, per unique hostname. Zero means net/http uses the default of only 2.|`int`|`100`
|method|The HTTP method to use when making requests to the Address Resolver|`string`|`GET`
|passthroughHeadersEnabled|Enable passing through the set of allowed HTTP request headers|`boolean`|`false`
|requestTimeout|The maximum amount of time that a request is allowed to remain open|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|responseField|The name of a JSON field that is provided in the response, that contains the tezos address (default `address`)|`string`|`address`
|retainOriginal|When true the original pre-resolved string is retained after the lookup, and passed down to Tezosconnect as the from address|`boolean`|`<nil>`
|tlsHandshakeTimeout|The maximum amount of time to wait for a successful TLS handshake|[`time.Duration`](https://pkg.go.dev/time#Duration)|`10s`
|url|The URL of the Address Resolver|`string`|`<nil>`
|urlTemplate|The URL Go template string to use when calling the Address Resolver. The template input contains '.Key' and '.Intent' string variables.|[Go Template](https://pkg.go.dev/text/template) `string`|`<nil>`

## plugins.blockchain[].tezos.addressResolver.auth

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|password|Password|`string`|`<nil>`
|username|Username|`string`|`<nil>`

## plugins.blockchain[].tezos.addressResolver.proxy

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|url|Optional HTTP proxy server to connect through|`string`|`<nil>`

## plugins.blockchain[].tezos.addressResolver.retry

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|count|The maximum number of times to retry|`int`|`5`
|enabled|Enables retries|`boolean`|`false`
|errorStatusCodeRegex|The regex that the error response status code must match to trigger retry|`string`|`<nil>`
|initWaitTime|The initial retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`250ms`
|maxWaitTime|The maximum retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`

## plugins.blockchain[].tezos.addressResolver.throttle

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|burst|The maximum number of requests that can be made in a short period of time before the throttling kicks in.|`int`|`<nil>`
|requestsPerSecond|The average rate at which requests are allowed to pass through over time.|`int`|`<nil>`

## plugins.blockchain[].tezos.addressResolver.tls

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|ca|The TLS certificate authority in PEM format (this option is ignored if caFile is also set)|`string`|`<nil>`
|caFile|The path to the CA file for TLS on this API|`string`|`<nil>`
|cert|The TLS certificate in PEM format (this option is ignored if certFile is also set)|`string`|`<nil>`
|certFile|The path to the certificate file for TLS on this API|`string`|`<nil>`
|clientAuth|Enables or disables client auth for TLS on this API|`string`|`<nil>`
|enabled|Enables or disables TLS on this API|`boolean`|`false`
|insecureSkipHostVerify|When to true in unit test development environments to disable TLS verification. Use with extreme caution|`boolean`|`<nil>`
|key|The TLS certificate key in PEM format (this option is ignored if keyFile is also set)|`string`|`<nil>`
|keyFile|The path to the private key file for TLS on this API|`string`|`<nil>`
|requiredDNAttributes|A set of required subject DN attributes. Each entry is a regular expression, and the subject certificate must have a matching attribute of the specified type (CN, C, O, OU, ST, L, STREET, POSTALCODE, SERIALNUMBER are valid attributes)|`map[string]string`|`<nil>`

## plugins.blockchain[].tezos.tezosconnect

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|batchSize|The number of events Tezosconnect should batch together for delivery to FireFly core. Only applies when automatically creating a new event stream|`int`|`50`
|batchTimeout|How long Tezosconnect should wait for new events to arrive and fill a batch, before sending the batch to FireFly core. Only applies when automatically creating a new event stream|[`time.Duration`](https://pkg.go.dev/time#Duration)|`500`
|connectionTimeout|The maximum amount of time that a connection is allowed to remain with no data transmitted|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|expectContinueTimeout|See [ExpectContinueTimeout in the Go docs](https://pkg.go.dev/net/http#Transport)|[`time.Duration`](https://pkg.go.dev/time#Duration)|`1s`
|headers|Adds custom headers to HTTP requests|`map[string]string`|`<nil>`
|idleTimeout|The max duration to hold a HTTP keepalive connection between calls|[`time.Duration`](https://pkg.go.dev/time#Duration)|`475ms`
|maxConnsPerHost|The max number of connections, per unique hostname. Zero means no limit|`int`|`0`
|maxIdleConns|The max number of idle connections to hold pooled|`int`|`100`
|maxIdleConnsPerHost|The max number of idle connections, per unique hostname. Zero means net/http uses the default of only 2.|`int`|`100`
|passthroughHeadersEnabled|Enable passing through the set of allowed HTTP request headers|`boolean`|`false`
|prefixLong|The prefix that will be used for Tezosconnect specific HTTP headers when FireFly makes requests to Tezosconnect|`string`|`firefly`
|prefixShort|The prefix that will be used for Tezosconnect specific query parameters when FireFly makes requests to Tezosconnect|`string`|`fly`
|requestTimeout|The maximum amount of time that a request is allowed to remain open|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|tlsHandshakeTimeout|The maximum amount of time to wait for a successful TLS handshake|[`time.Duration`](https://pkg.go.dev/time#Duration)|`10s`
|topic|The websocket listen topic that the node should register on, which is important if there are multiple nodes using a single tezosconnect|`string`|`<nil>`
|url|The URL of the Tezosconnect instance|URL `string`|`<nil>`

## plugins.blockchain[].tezos.tezosconnect.auth

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|password|Password|`string`|`<nil>`
|username|Username|`string`|`<nil>`

## plugins.blockchain[].tezos.tezosconnect.backgroundStart

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|enabled|Start the Tezosconnect plugin in the background and enter retry loop if failed to start|`boolean`|`<nil>`
|factor|Set the factor by which the delay increases when retrying|`float32`|`2`
|initialDelay|Delay between restarts in the case where we retry to restart the tezos plugin|[`time.Duration`](https://pkg.go.dev/time#Duration)|`5s`
|maxDelay|Max delay between restarts in the case where we retry to restart the tezos plugin|[`time.Duration`](https://pkg.go.dev/time#Duration)|`1m`

## plugins.blockchain[].tezos.tezosconnect.proxy

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|url|Optional HTTP proxy server to use when connecting to Tezosconnect|URL `string`|`<nil>`

## plugins.blockchain[].tezos.tezosconnect.retry

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|count|The maximum number of times to retry|`int`|`5`
|enabled|Enables retries|`boolean`|`false`
|errorStatusCodeRegex|The regex that the error response status code must match to trigger retry|`string`|`<nil>`
|factor|The retry backoff factor|`float32`|`2`
|initWaitTime|The initial retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`250ms`
|maxWaitTime|The maximum retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`

## plugins.blockchain[].tezos.tezosconnect.throttle

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|burst|The maximum number of requests that can be made in a short period of time before the throttling kicks in.|`int`|`<nil>`
|requestsPerSecond|The average rate at which requests are allowed to pass through over time.|`int`|`<nil>`

## plugins.blockchain[].tezos.tezosconnect.tls

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|ca|The TLS certificate authority in PEM format (this option is ignored if caFile is also set)|`string`|`<nil>`
|caFile|The path to the CA file for TLS on this API|`string`|`<nil>`
|cert|The TLS certificate in PEM format (this option is ignored if certFile is also set)|`string`|`<nil>`
|certFile|The path to the certificate file for TLS on this API|`string`|`<nil>`
|clientAuth|Enables or disables client auth for TLS on this API|`string`|`<nil>`
|enabled|Enables or disables TLS on this API|`boolean`|`false`
|insecureSkipHostVerify|When to true in unit test development environments to disable TLS verification. Use with extreme caution|`boolean`|`<nil>`
|key|The TLS certificate key in PEM format (this option is ignored if keyFile is also set)|`string`|`<nil>`
|keyFile|The path to the private key file for TLS on this API|`string`|`<nil>`
|requiredDNAttributes|A set of required subject DN attributes. Each entry is a regular expression, and the subject certificate must have a matching attribute of the specified type (CN, C, O, OU, ST, L, STREET, POSTALCODE, SERIALNUMBER are valid attributes)|`map[string]string`|`<nil>`

## plugins.blockchain[].tezos.tezosconnect.ws

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|backgroundConnect|When true the connection is established in the background with infinite reconnect (makes initialConnectAttempts redundant when set)|`boolean`|`false`
|connectionTimeout|The amount of time to wait while establishing a connection (or auto-reconnection)|[`time.Duration`](https://pkg.go.dev/time#Duration)|`45s`
|heartbeatInterval|The amount of time to wait between heartbeat signals on the WebSocket connection|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|initialConnectAttempts|The number of attempts FireFly will make to connect to the WebSocket when starting up, before failing|`int`|`5`
|path|The WebSocket sever URL to which FireFly should connect|WebSocket URL `string`|`<nil>`
|readBufferSize|The size in bytes of the read buffer for the WebSocket connection|[`BytesSize`](https://pkg.go.dev/github.com/docker/go-units#BytesSize)|`16Kb`
|url|URL to use for WebSocket - overrides url one level up (in the HTTP config)|`string`|`<nil>`
|writeBufferSize|The size in bytes of the write buffer for the WebSocket connection|[`BytesSize`](https://pkg.go.dev/github.com/docker/go-units#BytesSize)|`16Kb`

## plugins.database[]

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|name|The name of the Database plugin|`string`|`<nil>`
|type|The type of the configured Database plugin|`string`|`<nil>`

## plugins.database[].postgres

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|maxConnIdleTime|The maximum amount of time a database connection can be idle|[`time.Duration`](https://pkg.go.dev/time#Duration)|`1m`
|maxConnLifetime|The maximum amount of time to keep a database connection open|[`time.Duration`](https://pkg.go.dev/time#Duration)|`<nil>`
|maxConns|Maximum connections to the database|`int`|`50`
|maxIdleConns|The maximum number of idle connections to the database|`int`|`<nil>`
|url|The PostgreSQL connection string for the database|`string`|`<nil>`

## plugins.database[].postgres.migrations

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|auto|Enables automatic database migrations|`boolean`|`false`
|directory|The directory containing the numerically ordered migration DDL files to apply to the database|`string`|`./db/migrations/postgres`

## plugins.database[].sqlite3

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|maxConnIdleTime|The maximum amount of time a database connection can be idle|[`time.Duration`](https://pkg.go.dev/time#Duration)|`1m`
|maxConnLifetime|The maximum amount of time to keep a database connection open|[`time.Duration`](https://pkg.go.dev/time#Duration)|`<nil>`
|maxConns|Maximum connections to the database|`int`|`1`
|maxIdleConns|The maximum number of idle connections to the database|`int`|`<nil>`
|url|The SQLite connection string for the database|`string`|`<nil>`

## plugins.database[].sqlite3.migrations

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|auto|Enables automatic database migrations|`boolean`|`false`
|directory|The directory containing the numerically ordered migration DDL files to apply to the database|`string`|`./db/migrations/sqlite`

## plugins.dataexchange[]

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|name|The name of the configured Data Exchange plugin|`string`|`<nil>`
|type|The Data Exchange plugin to use|`string`|`<nil>`

## plugins.dataexchange[].ffdx

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|connectionTimeout|The maximum amount of time that a connection is allowed to remain with no data transmitted|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|expectContinueTimeout|See [ExpectContinueTimeout in the Go docs](https://pkg.go.dev/net/http#Transport)|[`time.Duration`](https://pkg.go.dev/time#Duration)|`1s`
|headers|Adds custom headers to HTTP requests|`map[string]string`|`<nil>`
|idleTimeout|The max duration to hold a HTTP keepalive connection between calls|[`time.Duration`](https://pkg.go.dev/time#Duration)|`475ms`
|initEnabled|Instructs FireFly to always post all current nodes to the `/init` API before connecting or reconnecting to the connector|`boolean`|`false`
|manifestEnabled|Determines whether to require+validate a manifest from other DX instances in the network. Must be supported by the connector|`string`|`false`
|maxConnsPerHost|The max number of connections, per unique hostname. Zero means no limit|`int`|`0`
|maxIdleConns|The max number of idle connections to hold pooled|`int`|`100`
|maxIdleConnsPerHost|The max number of idle connections, per unique hostname. Zero means net/http uses the default of only 2.|`int`|`100`
|passthroughHeadersEnabled|Enable passing through the set of allowed HTTP request headers|`boolean`|`false`
|requestTimeout|The maximum amount of time that a request is allowed to remain open|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|tlsHandshakeTimeout|The maximum amount of time to wait for a successful TLS handshake|[`time.Duration`](https://pkg.go.dev/time#Duration)|`10s`
|url|The URL of the Data Exchange instance|URL `string`|`<nil>`

## plugins.dataexchange[].ffdx.auth

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|password|Password|`string`|`<nil>`
|username|Username|`string`|`<nil>`

## plugins.dataexchange[].ffdx.backgroundStart

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|enabled|Start the data exchange plugin in the background and enter retry loop if failed to start|`boolean`|`false`
|factor|Set the factor by which the delay increases when retrying|`float32`|`2`
|initialDelay|Delay between restarts in the case where we retry to restart the data exchange plugin|[`time.Duration`](https://pkg.go.dev/time#Duration)|`5s`
|maxDelay|Max delay between restarts in the case where we retry to restart the data exchange plugin|[`time.Duration`](https://pkg.go.dev/time#Duration)|`1m`

## plugins.dataexchange[].ffdx.eventRetry

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|factor|The retry backoff factor, for event processing|`float32`|`2`
|initialDelay|The initial retry delay, for event processing|[`time.Duration`](https://pkg.go.dev/time#Duration)|`50ms`
|maxDelay|The maximum retry delay, for event processing|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`

## plugins.dataexchange[].ffdx.proxy

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|url|Optional HTTP proxy server to use when connecting to the Data Exchange|URL `string`|`<nil>`

## plugins.dataexchange[].ffdx.retry

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|count|The maximum number of times to retry|`int`|`5`
|enabled|Enables retries|`boolean`|`false`
|errorStatusCodeRegex|The regex that the error response status code must match to trigger retry|`string`|`<nil>`
|factor|The retry backoff factor|`float32`|`2`
|initWaitTime|The initial retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`250ms`
|maxWaitTime|The maximum retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`

## plugins.dataexchange[].ffdx.throttle

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|burst|The maximum number of requests that can be made in a short period of time before the throttling kicks in.|`int`|`<nil>`
|requestsPerSecond|The average rate at which requests are allowed to pass through over time.|`int`|`<nil>`

## plugins.dataexchange[].ffdx.tls

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|ca|The TLS certificate authority in PEM format (this option is ignored if caFile is also set)|`string`|`<nil>`
|caFile|The path to the CA file for TLS on this API|`string`|`<nil>`
|cert|The TLS certificate in PEM format (this option is ignored if certFile is also set)|`string`|`<nil>`
|certFile|The path to the certificate file for TLS on this API|`string`|`<nil>`
|clientAuth|Enables or disables client auth for TLS on this API|`string`|`<nil>`
|enabled|Enables or disables TLS on this API|`boolean`|`false`
|insecureSkipHostVerify|When to true in unit test development environments to disable TLS verification. Use with extreme caution|`boolean`|`<nil>`
|key|The TLS certificate key in PEM format (this option is ignored if keyFile is also set)|`string`|`<nil>`
|keyFile|The path to the private key file for TLS on this API|`string`|`<nil>`
|requiredDNAttributes|A set of required subject DN attributes. Each entry is a regular expression, and the subject certificate must have a matching attribute of the specified type (CN, C, O, OU, ST, L, STREET, POSTALCODE, SERIALNUMBER are valid attributes)|`map[string]string`|`<nil>`

## plugins.dataexchange[].ffdx.ws

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|backgroundConnect|When true the connection is established in the background with infinite reconnect (makes initialConnectAttempts redundant when set)|`boolean`|`false`
|connectionTimeout|The amount of time to wait while establishing a connection (or auto-reconnection)|[`time.Duration`](https://pkg.go.dev/time#Duration)|`45s`
|heartbeatInterval|The amount of time to wait between heartbeat signals on the WebSocket connection|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|initialConnectAttempts|The number of attempts FireFly will make to connect to the WebSocket when starting up, before failing|`int`|`5`
|path|The WebSocket sever URL to which FireFly should connect|WebSocket URL `string`|`<nil>`
|readBufferSize|The size in bytes of the read buffer for the WebSocket connection|[`BytesSize`](https://pkg.go.dev/github.com/docker/go-units#BytesSize)|`16Kb`
|url|URL to use for WebSocket - overrides url one level up (in the HTTP config)|`string`|`<nil>`
|writeBufferSize|The size in bytes of the write buffer for the WebSocket connection|[`BytesSize`](https://pkg.go.dev/github.com/docker/go-units#BytesSize)|`16Kb`

## plugins.identity[]

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|name|The name of a configured Identity plugin|`string`|`<nil>`
|type|The type of a configured Identity plugin|`string`|`<nil>`

## plugins.sharedstorage[]

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|name|The name of the Shared Storage plugin to use|`string`|`<nil>`
|type|The Shared Storage plugin to use|`string`|`<nil>`

## plugins.sharedstorage[].ipfs.api

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|connectionTimeout|The maximum amount of time that a connection is allowed to remain with no data transmitted|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|expectContinueTimeout|See [ExpectContinueTimeout in the Go docs](https://pkg.go.dev/net/http#Transport)|[`time.Duration`](https://pkg.go.dev/time#Duration)|`1s`
|headers|Adds custom headers to HTTP requests|`map[string]string`|`<nil>`
|idleTimeout|The max duration to hold a HTTP keepalive connection between calls|[`time.Duration`](https://pkg.go.dev/time#Duration)|`475ms`
|maxConnsPerHost|The max number of connections, per unique hostname. Zero means no limit|`int`|`0`
|maxIdleConns|The max number of idle connections to hold pooled|`int`|`100`
|maxIdleConnsPerHost|The max number of idle connections, per unique hostname. Zero means net/http uses the default of only 2.|`int`|`100`
|passthroughHeadersEnabled|Enable passing through the set of allowed HTTP request headers|`boolean`|`false`
|requestTimeout|The maximum amount of time that a request is allowed to remain open|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|tlsHandshakeTimeout|The maximum amount of time to wait for a successful TLS handshake|[`time.Duration`](https://pkg.go.dev/time#Duration)|`10s`
|url|The URL for the IPFS API|URL `string`|`<nil>`

## plugins.sharedstorage[].ipfs.api.auth

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|password|Password|`string`|`<nil>`
|username|Username|`string`|`<nil>`

## plugins.sharedstorage[].ipfs.api.proxy

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|url|Optional HTTP proxy server to use when connecting to the IPFS API|URL `string`|`<nil>`

## plugins.sharedstorage[].ipfs.api.retry

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|count|The maximum number of times to retry|`int`|`5`
|enabled|Enables retries|`boolean`|`false`
|errorStatusCodeRegex|The regex that the error response status code must match to trigger retry|`string`|`<nil>`
|initWaitTime|The initial retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`250ms`
|maxWaitTime|The maximum retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`

## plugins.sharedstorage[].ipfs.api.throttle

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|burst|The maximum number of requests that can be made in a short period of time before the throttling kicks in.|`int`|`<nil>`
|requestsPerSecond|The average rate at which requests are allowed to pass through over time.|`int`|`<nil>`

## plugins.sharedstorage[].ipfs.api.tls

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|ca|The TLS certificate authority in PEM format (this option is ignored if caFile is also set)|`string`|`<nil>`
|caFile|The path to the CA file for TLS on this API|`string`|`<nil>`
|cert|The TLS certificate in PEM format (this option is ignored if certFile is also set)|`string`|`<nil>`
|certFile|The path to the certificate file for TLS on this API|`string`|`<nil>`
|clientAuth|Enables or disables client auth for TLS on this API|`string`|`<nil>`
|enabled|Enables or disables TLS on this API|`boolean`|`false`
|insecureSkipHostVerify|When to true in unit test development environments to disable TLS verification. Use with extreme caution|`boolean`|`<nil>`
|key|The TLS certificate key in PEM format (this option is ignored if keyFile is also set)|`string`|`<nil>`
|keyFile|The path to the private key file for TLS on this API|`string`|`<nil>`
|requiredDNAttributes|A set of required subject DN attributes. Each entry is a regular expression, and the subject certificate must have a matching attribute of the specified type (CN, C, O, OU, ST, L, STREET, POSTALCODE, SERIALNUMBER are valid attributes)|`map[string]string`|`<nil>`

## plugins.sharedstorage[].ipfs.gateway

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|connectionTimeout|The maximum amount of time that a connection is allowed to remain with no data transmitted|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|expectContinueTimeout|See [ExpectContinueTimeout in the Go docs](https://pkg.go.dev/net/http#Transport)|[`time.Duration`](https://pkg.go.dev/time#Duration)|`1s`
|headers|Adds custom headers to HTTP requests|`map[string]string`|`<nil>`
|idleTimeout|The max duration to hold a HTTP keepalive connection between calls|[`time.Duration`](https://pkg.go.dev/time#Duration)|`475ms`
|maxConnsPerHost|The max number of connections, per unique hostname. Zero means no limit|`int`|`0`
|maxIdleConns|The max number of idle connections to hold pooled|`int`|`100`
|maxIdleConnsPerHost|The max number of idle connections, per unique hostname. Zero means net/http uses the default of only 2.|`int`|`100`
|passthroughHeadersEnabled|Enable passing through the set of allowed HTTP request headers|`boolean`|`false`
|requestTimeout|The maximum amount of time that a request is allowed to remain open|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|tlsHandshakeTimeout|The maximum amount of time to wait for a successful TLS handshake|[`time.Duration`](https://pkg.go.dev/time#Duration)|`10s`
|url|The URL for the IPFS Gateway|URL `string`|`<nil>`

## plugins.sharedstorage[].ipfs.gateway.auth

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|password|Password|`string`|`<nil>`
|username|Username|`string`|`<nil>`

## plugins.sharedstorage[].ipfs.gateway.proxy

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|url|Optional HTTP proxy server to use when connecting to the IPFS Gateway|URL `string`|`<nil>`

## plugins.sharedstorage[].ipfs.gateway.retry

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|count|The maximum number of times to retry|`int`|`5`
|enabled|Enables retries|`boolean`|`false`
|errorStatusCodeRegex|The regex that the error response status code must match to trigger retry|`string`|`<nil>`
|initWaitTime|The initial retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`250ms`
|maxWaitTime|The maximum retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`

## plugins.sharedstorage[].ipfs.gateway.throttle

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|burst|The maximum number of requests that can be made in a short period of time before the throttling kicks in.|`int`|`<nil>`
|requestsPerSecond|The average rate at which requests are allowed to pass through over time.|`int`|`<nil>`

## plugins.sharedstorage[].ipfs.gateway.tls

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|ca|The TLS certificate authority in PEM format (this option is ignored if caFile is also set)|`string`|`<nil>`
|caFile|The path to the CA file for TLS on this API|`string`|`<nil>`
|cert|The TLS certificate in PEM format (this option is ignored if certFile is also set)|`string`|`<nil>`
|certFile|The path to the certificate file for TLS on this API|`string`|`<nil>`
|clientAuth|Enables or disables client auth for TLS on this API|`string`|`<nil>`
|enabled|Enables or disables TLS on this API|`boolean`|`false`
|insecureSkipHostVerify|When to true in unit test development environments to disable TLS verification. Use with extreme caution|`boolean`|`<nil>`
|key|The TLS certificate key in PEM format (this option is ignored if keyFile is also set)|`string`|`<nil>`
|keyFile|The path to the private key file for TLS on this API|`string`|`<nil>`
|requiredDNAttributes|A set of required subject DN attributes. Each entry is a regular expression, and the subject certificate must have a matching attribute of the specified type (CN, C, O, OU, ST, L, STREET, POSTALCODE, SERIALNUMBER are valid attributes)|`map[string]string`|`<nil>`

## plugins.tokens[]

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|broadcastName|The name to be used in broadcast messages related to this token plugin, if it differs from the local plugin name|`string`|`<nil>`
|name|A name to identify this token plugin|`string`|`<nil>`
|type|The type of the token plugin to use|`string`|`<nil>`

## plugins.tokens[].fftokens

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|connectionTimeout|The maximum amount of time that a connection is allowed to remain with no data transmitted|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|expectContinueTimeout|See [ExpectContinueTimeout in the Go docs](https://pkg.go.dev/net/http#Transport)|[`time.Duration`](https://pkg.go.dev/time#Duration)|`1s`
|headers|Adds custom headers to HTTP requests|`map[string]string`|`<nil>`
|idleTimeout|The max duration to hold a HTTP keepalive connection between calls|[`time.Duration`](https://pkg.go.dev/time#Duration)|`475ms`
|maxConnsPerHost|The max number of connections, per unique hostname. Zero means no limit|`int`|`0`
|maxIdleConns|The max number of idle connections to hold pooled|`int`|`100`
|maxIdleConnsPerHost|The max number of idle connections, per unique hostname. Zero means net/http uses the default of only 2.|`int`|`100`
|passthroughHeadersEnabled|Enable passing through the set of allowed HTTP request headers|`boolean`|`false`
|requestTimeout|The maximum amount of time that a request is allowed to remain open|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|tlsHandshakeTimeout|The maximum amount of time to wait for a successful TLS handshake|[`time.Duration`](https://pkg.go.dev/time#Duration)|`10s`
|url|The URL of the token connector|URL `string`|`<nil>`

## plugins.tokens[].fftokens.auth

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|password|Password|`string`|`<nil>`
|username|Username|`string`|`<nil>`

## plugins.tokens[].fftokens.backgroundStart

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|enabled|Start the tokens plugin in the background and enter retry loop if failed to start|`boolean`|`false`
|factor|Set the factor by which the delay increases when retrying|`float32`|`2`
|initialDelay|Delay between restarts in the case where we retry to restart the token plugin|[`time.Duration`](https://pkg.go.dev/time#Duration)|`5s`
|maxDelay|Max delay between restarts in the case where we retry to restart the token plugin|[`time.Duration`](https://pkg.go.dev/time#Duration)|`1m`

## plugins.tokens[].fftokens.eventRetry

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|factor|The retry backoff factor, for event processing|`float32`|`2`
|initialDelay|The initial retry delay, for event processing|[`time.Duration`](https://pkg.go.dev/time#Duration)|`50ms`
|maxDelay|The maximum retry delay, for event processing|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`

## plugins.tokens[].fftokens.proxy

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|url|Optional HTTP proxy server to use when connecting to the token connector|URL `string`|`<nil>`

## plugins.tokens[].fftokens.retry

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|count|The maximum number of times to retry|`int`|`5`
|enabled|Enables retries|`boolean`|`false`
|errorStatusCodeRegex|The regex that the error response status code must match to trigger retry|`string`|`<nil>`
|factor|The retry backoff factor|`float32`|`2`
|initWaitTime|The initial retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`250ms`
|maxWaitTime|The maximum retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`

## plugins.tokens[].fftokens.throttle

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|burst|The maximum number of requests that can be made in a short period of time before the throttling kicks in.|`int`|`<nil>`
|requestsPerSecond|The average rate at which requests are allowed to pass through over time.|`int`|`<nil>`

## plugins.tokens[].fftokens.tls

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|ca|The TLS certificate authority in PEM format (this option is ignored if caFile is also set)|`string`|`<nil>`
|caFile|The path to the CA file for TLS on this API|`string`|`<nil>`
|cert|The TLS certificate in PEM format (this option is ignored if certFile is also set)|`string`|`<nil>`
|certFile|The path to the certificate file for TLS on this API|`string`|`<nil>`
|clientAuth|Enables or disables client auth for TLS on this API|`string`|`<nil>`
|enabled|Enables or disables TLS on this API|`boolean`|`false`
|insecureSkipHostVerify|When to true in unit test development environments to disable TLS verification. Use with extreme caution|`boolean`|`<nil>`
|key|The TLS certificate key in PEM format (this option is ignored if keyFile is also set)|`string`|`<nil>`
|keyFile|The path to the private key file for TLS on this API|`string`|`<nil>`
|requiredDNAttributes|A set of required subject DN attributes. Each entry is a regular expression, and the subject certificate must have a matching attribute of the specified type (CN, C, O, OU, ST, L, STREET, POSTALCODE, SERIALNUMBER are valid attributes)|`map[string]string`|`<nil>`

## plugins.tokens[].fftokens.ws

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|backgroundConnect|When true the connection is established in the background with infinite reconnect (makes initialConnectAttempts redundant when set)|`boolean`|`false`
|connectionTimeout|The amount of time to wait while establishing a connection (or auto-reconnection)|[`time.Duration`](https://pkg.go.dev/time#Duration)|`45s`
|heartbeatInterval|The amount of time to wait between heartbeat signals on the WebSocket connection|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`
|initialConnectAttempts|The number of attempts FireFly will make to connect to the WebSocket when starting up, before failing|`int`|`5`
|path|The WebSocket sever URL to which FireFly should connect|WebSocket URL `string`|`<nil>`
|readBufferSize|The size in bytes of the read buffer for the WebSocket connection|[`BytesSize`](https://pkg.go.dev/github.com/docker/go-units#BytesSize)|`16Kb`
|url|URL to use for WebSocket - overrides url one level up (in the HTTP config)|`string`|`<nil>`
|writeBufferSize|The size in bytes of the write buffer for the WebSocket connection|[`BytesSize`](https://pkg.go.dev/github.com/docker/go-units#BytesSize)|`16Kb`

## privatemessaging.batch

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|agentTimeout|How long to keep around a batching agent for a sending identity before disposal|[`time.Duration`](https://pkg.go.dev/time#Duration)|`2m`
|payloadLimit|The maximum payload size of a private message Data Exchange payload|[`BytesSize`](https://pkg.go.dev/github.com/docker/go-units#BytesSize)|`800Kb`
|size|The maximum number of messages in a batch for private messages|`int`|`200`
|timeout|The timeout to wait for a batch to fill, before sending|[`time.Duration`](https://pkg.go.dev/time#Duration)|`1s`

## privatemessaging.retry

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|factor|The retry backoff factor|`float32`|`2`
|initDelay|The initial retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`100ms`
|maxDelay|The maximum retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`

## spi

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|address|The IP address on which the admin HTTP API should listen|IP Address `string`|`127.0.0.1`
|enabled|Enables the admin HTTP API|`boolean`|`false`
|port|The port on which the admin HTTP API should listen|`int`|`5001`
|publicURL|The fully qualified public URL for the admin API. This is used for building URLs in HTTP responses and in OpenAPI Spec generation|URL `string`|`<nil>`
|readTimeout|The maximum time to wait when reading from an HTTP connection|[`time.Duration`](https://pkg.go.dev/time#Duration)|`15s`
|shutdownTimeout|The maximum amount of time to wait for any open HTTP requests to finish before shutting down the HTTP server|[`time.Duration`](https://pkg.go.dev/time#Duration)|`10s`
|writeTimeout|The maximum time to wait when writing to an HTTP connection|[`time.Duration`](https://pkg.go.dev/time#Duration)|`15s`

## spi.auth

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|type|The auth plugin to use for server side authentication of requests|`string`|`<nil>`

## spi.auth.basic

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|passwordfile|The path to a .htpasswd file to use for authenticating requests. Passwords should be hashed with bcrypt.|`string`|`<nil>`

## spi.tls

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|ca|The TLS certificate authority in PEM format (this option is ignored if caFile is also set)|`string`|`<nil>`
|caFile|The path to the CA file for TLS on this API|`string`|`<nil>`
|cert|The TLS certificate in PEM format (this option is ignored if certFile is also set)|`string`|`<nil>`
|certFile|The path to the certificate file for TLS on this API|`string`|`<nil>`
|clientAuth|Enables or disables client auth for TLS on this API|`string`|`<nil>`
|enabled|Enables or disables TLS on this API|`boolean`|`false`
|insecureSkipHostVerify|When to true in unit test development environments to disable TLS verification. Use with extreme caution|`boolean`|`<nil>`
|key|The TLS certificate key in PEM format (this option is ignored if keyFile is also set)|`string`|`<nil>`
|keyFile|The path to the private key file for TLS on this API|`string`|`<nil>`
|requiredDNAttributes|A set of required subject DN attributes. Each entry is a regular expression, and the subject certificate must have a matching attribute of the specified type (CN, C, O, OU, ST, L, STREET, POSTALCODE, SERIALNUMBER are valid attributes)|`map[string]string`|`<nil>`

## spi.ws

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|blockedWarnInterval|How often to log warnings in core, when an admin change event listener falls behind the stream they requested and misses events|[`time.Duration`](https://pkg.go.dev/time#Duration)|`1m`
|eventQueueLength|Server-side queue length for events waiting for delivery over an admin change event listener websocket|`int`|`250`
|readBufferSize|The size in bytes of the read buffer for the WebSocket connection|[`BytesSize`](https://pkg.go.dev/github.com/docker/go-units#BytesSize)|`16Kb`
|writeBufferSize|The size in bytes of the write buffer for the WebSocket connection|[`BytesSize`](https://pkg.go.dev/github.com/docker/go-units#BytesSize)|`16Kb`

## subscription

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|max|The maximum number of pre-defined subscriptions that can exist (note for high fan-out consider connecting a dedicated pub/sub broker to the dispatcher)|`int`|`500`

## subscription.defaults

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|batchSize|Default read ahead to enable for subscriptions that do not explicitly configure readahead|`int`|`50`
|batchTimeout|Default batch timeout|`int`|`50ms`

## subscription.events

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|maxScanLength|The maximum number of events a search for historical events matching a subscription will index from the database|`int`|`1000`

## subscription.retry

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|factor|The retry backoff factor|`float32`|`2`
|initDelay|The initial retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`250ms`
|maxDelay|The maximum retry delay|[`time.Duration`](https://pkg.go.dev/time#Duration)|`30s`

## transaction.writer

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|batchMaxTransactions|The maximum number of transaction inserts to include in a batch|`int`|`100`
|batchTimeout|How long to wait for more transactions to arrive before flushing the batch|[`time.Duration`](https://pkg.go.dev/time#Duration)|`10ms`
|count|The number of message writer workers|`int`|`5`

## ui

|Key|Description|Type|Default Value|
|---|-----------|----|-------------|
|enabled|Enables the web user interface|`boolean`|`true`
|path|The file system path which contains the static HTML, CSS, and JavaScript files for the user interface|`string`|`<nil>`