---
title: Subscription
---
{% include-markdown "./_includes/subscription_description.md" %}

### Example

```json
{
    "id": "c38d69fd-442e-4d6f-b5a4-bab1411c7fe8",
    "namespace": "ns1",
    "name": "app1",
    "transport": "websockets",
    "filter": {
        "events": "^(message_.*|token_.*)$",
        "message": {
            "tag": "^(red|blue)$"
        },
        "transaction": {},
        "blockchainevent": {}
    },
    "options": {
        "firstEvent": "newest",
        "readAhead": 50
    },
    "created": "2022-05-16T01:23:15Z",
    "updated": null
}
```

### Field Descriptions

| Field Name | Description | Type |
|------------|-------------|------|
| `id` | The UUID of the subscription | [`UUID`](simpletypes.md#uuid) |
| `namespace` | The namespace of the subscription. A subscription will only receive events generated in the namespace of the subscription | `string` |
| `name` | The name of the subscription. The application specifies this name when it connects, in order to attach to the subscription and receive events that arrived while it was disconnected. If multiple apps connect to the same subscription, events are workload balanced across the connected application instances | `string` |
| `transport` | The transport plugin responsible for event delivery (WebSockets, Webhooks, JMS, NATS etc.) | `string` |
| `filter` | Server-side filter to apply to events | [`SubscriptionFilter`](#subscriptionfilter) |
| `options` | Subscription options | [`SubscriptionOptions`](#subscriptionoptions) |
| `ephemeral` | Ephemeral subscriptions only exist as long as the application is connected, and as such will miss events that occur while the application is disconnected, and cannot be created administratively. You can create one over over a connected WebSocket connection | `bool` |
| `created` | Creation time of the subscription | [`FFTime`](simpletypes.md#fftime) |
| `updated` | Last time the subscription was updated | [`FFTime`](simpletypes.md#fftime) |

## SubscriptionFilter

| Field Name | Description | Type |
|------------|-------------|------|
| `events` | Regular expression to apply to the event type, to subscribe to a subset of event types | `string` |
| `message` | Filters specific to message events. If an event is not a message event, these filters are ignored | [`MessageFilter`](#messagefilter) |
| `transaction` | Filters specific to events with a transaction. If an event is not associated with a transaction, this filter is ignored | [`TransactionFilter`](#transactionfilter) |
| `blockchainevent` | Filters specific to blockchain events. If an event is not a blockchain event, these filters are ignored | [`BlockchainEventFilter`](#blockchaineventfilter) |
| `topic` | Regular expression to apply to the topic of the event, to subscribe to a subset of topics. Note for messages sent with multiple topics, a separate event is emitted for each topic | `string` |
| `topics` | Deprecated: Please use 'topic' instead | `string` |
| `tag` | Deprecated: Please use 'message.tag' instead | `string` |
| `group` | Deprecated: Please use 'message.group' instead | `string` |
| `author` | Deprecated: Please use 'message.author' instead | `string` |

## MessageFilter

| Field Name | Description | Type |
|------------|-------------|------|
| `tag` | Regular expression to apply to the message 'header.tag' field | `string` |
| `group` | Regular expression to apply to the message 'header.group' field | `string` |
| `author` | Regular expression to apply to the message 'header.author' field | `string` |


## TransactionFilter

| Field Name | Description | Type |
|------------|-------------|------|
| `type` | Regular expression to apply to the transaction 'type' field | `string` |


## BlockchainEventFilter

| Field Name | Description | Type |
|------------|-------------|------|
| `name` | Regular expression to apply to the blockchain event 'name' field, which is the name of the event in the underlying blockchain smart contract | `string` |
| `listener` | Regular expression to apply to the blockchain event 'listener' field, which is the UUID of the event listener. So you can restrict your subscription to certain blockchain listeners. Alternatively to avoid your application need to know listener UUIDs you can set the 'topic' field of blockchain event listeners, and use a topic filter on your subscriptions | `string` |



## SubscriptionOptions

| Field Name | Description | Type |
|------------|-------------|------|
| `firstEvent` | Whether your application would like to receive events from the 'oldest' event emitted by your FireFly node (from the beginning of time), or the 'newest' event (from now), or a specific event sequence. Default is 'newest' | `SubOptsFirstEvent` |
| `readAhead` | The number of events to stream ahead to your application, while waiting for confirmation of consumption of those events. At least once delivery semantics are used in FireFly, so if your application crashes/reconnects this is the maximum number of events you would expect to be redelivered after it restarts | `uint` |
| `withData` | Whether message events delivered over the subscription, should be packaged with the full data of those messages in-line as part of the event JSON payload. Or if the application should make separate REST calls to download that data. May not be supported on some transports. | `bool` |
| `batch` | Events are delivered in batches in an ordered array. The batch size is capped to the readAhead limit. The event payload is always an array even if there is a single event in the batch, allowing client-side optimizations when processing the events in a group. Available for both Webhooks and WebSockets. | `bool` |
| `batchTimeout` | When batching is enabled, the optional timeout to send events even when the batch hasn't filled. | `string` |
| `fastack` | Webhooks only: Deprecated - please use a confirmationMode of fastack instead. When true the event will be acknowledged before the webhook is invoked, allowing parallel invocations | `bool` |
| `confirmationMode` | Webhooks only: Determines whether a webhook delivery must be confirmed before the subscription checkpoint advances. Use fastack to acknowledge the event before the webhook is invoked, besteffort (the default) to hold the checkpoint while the invocation - including any configured client-side retries - completes, then acknowledge regardless of its outcome, or assured to acknowledge only on a 2xx response and otherwise hold the checkpoint and redeliver | `FFEnum`:<br/>`"fastack"`<br/>`"besteffort"`<br/>`"assured"` |
| `url` | Webhooks only: HTTP url to invoke. Can be relative if a base URL is set in the webhook plugin config | `string` |
| `method` | Webhooks only: HTTP method to invoke. Default=POST | `string` |
| `json` | Webhooks only: Whether to assume the response body is JSON, regardless of the returned Content-Type | `bool` |
| `reply` | Webhooks only: Whether to automatically send a reply event, using the body returned by the webhook | `bool` |
| `replytag` | Webhooks only: The tag to set on the reply message | `string` |
| `replytx` | Webhooks only: The transaction type to set on the reply message | `string` |
| `headers` | Webhooks only: Static headers to set on the webhook request | `` |
| `query` | Webhooks only: Static query params to set on the webhook request | `` |
| `tlsConfigName` | The name of an existing TLS configuration associated to the namespace to use | `string` |
| `input` | Webhooks only: A set of options to extract data from the first JSON input data in the incoming message. Only applies if withData=true | [`WebhookInputOptions`](#webhookinputoptions) |
| `retry` | Webhooks only: a set of options for retrying the webhook call | [`WebhookRetryOptions`](#webhookretryoptions) |
| `httpOptions` | Webhooks only: a set of options for HTTP | [`WebhookHTTPOptions`](#webhookhttpoptions) |

## WebhookInputOptions

| Field Name | Description | Type |
|------------|-------------|------|
| `query` | A top-level property of the first data input, to use for query parameters | `string` |
| `headers` | A top-level property of the first data input, to use for headers | `string` |
| `body` | A top-level property of the first data input, to use for the request body. Default is the whole first body | `string` |
| `path` | A top-level property of the first data input, to use for a path to append with escaping to the webhook path | `string` |
| `replytx` | A top-level property of the first data input, to use to dynamically set whether to pin the response (so the requester can choose) | `string` |


## WebhookRetryOptions

| Field Name | Description | Type |
|------------|-------------|------|
| `enabled` | Enables retry on HTTP calls, defaults to false | `bool` |
| `count` | Number of times to retry the webhook call in case of failure | `int` |
| `initialDelay` | Initial delay between retries when we retry the webhook call | `string` |
| `maxDelay` | Max delay between retries when we retry the webhookcall | `string` |


## WebhookHTTPOptions

| Field Name | Description | Type |
|------------|-------------|------|
| `proxyURL` | HTTP proxy URL to use for outbound requests to the webhook | `string` |
| `tlsHandshakeTimeout` | The max duration to hold a TLS handshake alive | `string` |
| `requestTimeout` | The max duration to hold a TLS handshake alive | `string` |
| `maxIdleConns` | The max number of idle connections to hold pooled | `int` |
| `idleTimeout` | The max duration to hold a HTTP keepalive connection between calls | `string` |
| `connectionTimeout` | The maximum amount of time that a connection is allowed to remain with no data transmitted. | `string` |
| `expectContinueTimeout` | See [ExpectContinueTimeout in the Go docs](https://pkg.go.dev/net/http#Transport) | `string` |



