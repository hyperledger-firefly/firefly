---
title: fftokens
---

# fftokens

## Overview

fftokens is a protocol that can be implemented by token connector runtimes in order to be usable by the [fftokens](https://github.com/hyperledger-firefly/firefly/blob/main/internal/tokens/fftokens/fftokens.go) plugin in FireFly.

The connector runtime must expose an **HTTP and websocket server**, along with a minimum set of HTTP APIs and websocket events. Each connector will be strongly coupled to a specific ledger technology and token standard(s), but **no assumptions are made** in the fftokens spec about what these technologies must be, as long as they can satisfy the basic requirements laid out here.

Note that this is an _internal_ protocol in the FireFly ecosystem - application developers working against FireFly should never need to care about or directly
interact with a token connector runtime. The audience for this document is only developers interested in creating new token connectors (or editing/forking
existing ones).

Two implementations of this specification have been created to date (both based on common Ethereum token standards) - [firefly-tokens-erc1155](https://github.com/hyperledger-firefly/tokens-erc1155) and [firefly-tokens-erc20-erc721](https://github.com/hyperledger-firefly/tokens-erc20-erc721).

## HTTP APIs

This is the minimum set of APIs that must be implemented by a conforming token connector. A connector may choose to expose other APIs for its own purposes. All requests and responses to the APIs below are encoded as JSON. The APIs are currently understood to live under a `/api/v1` prefix.

### `POST /createpool`

Create a new token pool. The exact meaning of this is flexible - it may mean invoking a contract or contract factory to actually define a new set of tokens via a blockchain transaction, or it may mean indexing a set of tokens that already exists (depending on the options a connector accepts in `config`).

In a multiparty network, this operation will only be performed by one of the parties, and FireFly will broadcast the result to the others.

FireFly will store a "pending" token pool after a successful creation, but will replace it with a "confirmed" token pool after a successful activation (see below).

**Request**

```
{
  "type": "fungible",
  "signer": "0x0Ef1D0Dd56a8FB1226C0EaC374000B81D6c8304A",
  "namespace": "default",
  "name": "FFCoin",
  "symbol": "FFC",
  "data": "pool-metadata",
  "requestId": "1",
  "config": {}
}
```

| Parameter | Type        | Description                                                                                                                                                                                                                       |
| --------- | ----------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| type      | string enum | The type of pool to create. Currently supported types are "fungible" and "nonfungible". It is recommended (but not required) that token connectors support both. Unrecognized/unsupported types should be rejected with HTTP 400. |
| signer    | string      | The signing identity to be used for the blockchain transaction, in a format understood by this connector.                                                                                                                         |
| namespace | string      | The namespace of the token pool                                                                                                                                                                                                   |
| name      | string      | (OPTIONAL) If supported by this token contract, this is a requested name for the token pool. May be ignored at the connector's discretion.                                                                                        |
| symbol    | string      | (OPTIONAL) If supported by this token contract, this is a requested symbol for the token pool. May be ignored at the connector's discretion.                                                                                      |
| requestId | string      | (OPTIONAL) A unique identifier for this request. Will be included in the "receipt" websocket event to match receipts to requests.                                                                                                 |
| data      | string      | (OPTIONAL) A data string that should be returned in the connector's response to this creation request.                                                                                                                            |
| config    | object      | (OPTIONAL) An arbitrary JSON object where the connector may accept additional parameters if desired. Each connector may define its own valid options to influence how the token pool is created.                                  |

**Response**

HTTP 200: pool creation was successful, and the pool details are returned in the response.

_See [Response Types: Token Pool](#token-pool_1)_

HTTP 202: request was accepted, but pool will be created asynchronously, with "receipt" and "token-pool" events sent later on the websocket.

_See [Response Types: Async Request](#async-request)_

### `POST /activatepool`

Activate a token pool to begin receiving events. Generally this means the connector will create blockchain event listeners for transfer and approval events related to the set of tokens encompassed by this token pool.

In a multiparty network, this step will be performed by every member after a successful token pool broadcast. It therefore also serves the purpose of validating the broadcast info - if the connector does not find a valid pool given the `poolLocator` and `config` information passed in to this call, the pool should not get confirmed.

**Request**

```
{
  "namespace": "default",
  "poolLocator": "id=F1",
  "poolData": "extra-pool-info",
  "config": {}
}
```

| Parameter   | Type   | Description                                                                                                                                                                             |
| ----------- | ------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| namespace   | string | The namespace of the token pool                                                                                                                                                         |
| poolLocator | string | The locator of the pool, as supplied by the output of the pool creation.                                                                                                                |
| poolData    | string | (OPTIONAL) A data string that should be permanently attached to this pool and returned in all events.                                                                                   |
| config      | object | (OPTIONAL) An arbitrary JSON object where the connector may accept additional parameters if desired. This should be the same `config` object that was passed when the pool was created. |

**Response**

HTTP 200: pool activation was successful, and the pool details are returned in the response.

_See [Response Types: Token Pool](#token-pool_1)_

HTTP 202: request was accepted, but pool will be activated asynchronously, with "receipt" and "token-pool" events sent later on the websocket.

_See [Response Types: Async Request](#async-request)_

HTTP 204: activation was successful - no separate receipt will be delivered, but "token-pool" event will be sent later on the websocket.

_No body_

### `POST /deactivatepool`

Deactivate a token pool to stop receiving events and delete all blockchain listeners related to that pool.

**Request**

```
{
  "namespace": "default",
  "poolLocator": "id=F1",
  "poolData": "extra-pool-info",
  "config": {}
}
```

| Parameter   | Type   | Description                                                                                          |
| ----------- | ------ | ---------------------------------------------------------------------------------------------------- |
| namespace   | string | The namespace of the token pool                                                                      |
| poolLocator | string | The locator of the pool, as supplied by the output of the pool creation.                             |
| poolData    | string | (OPTIONAL) The data string that was attached to this pool at activation.                             |
| config      | object | (OPTIONAL) An arbitrary JSON object where the connector may accept additional parameters if desired. |

**Response**

HTTP 204: deactivation was successful, and one or more listeners were deleted.

_No body_

HTTP 404: no blockchain listeners were found for the given pool information.

_No body_

### `POST /checkinterface`

This is an optional (but recommended) API for token connectors. If implemented, support will be indicated by
the presence of the `interfaceFormat` field in all [Token Pool](#token-pool_1) responses.

In the case that a connector supports multiple variants of a given token standard (such as many different ways to
structure "mint" or "burn" calls on an underlying smart contract), this API allows the connector to be provided with a full
description of the interface methods in use for a given token pool, so the connector can determine which methods it knows
how to invoke.

**Request**

```
{
  "poolLocator": "id=F1",
  "format": "abi",
  "methods": [
    {
      "name": "burn",
      "type": "function",
      "inputs": [
        {
          "internalType": "uint256",
          "name": "tokenId",
          "type": "uint256"
        }
      ],
      "outputs": [],
      "stateMutability": "nonpayable"
    },
    ...
  ]
}
```

| Parameter   | Type         | Description                                                                                                                     |
| ----------- | ------------ | ------------------------------------------------------------------------------------------------------------------------------- |
| poolLocator | string       | The locator of the pool, as supplied by the output of the pool creation.                                                        |
| format      | string enum  | The format of the data in this payload. Should match the `interfaceFormat` as supplied by the output of the pool creation.      |
| methods     | object array | A list of all the methods available on the interface underpinning this token pool, encoded in the format specified by `format`. |

**Response**

HTTP 200: interface was successfully parsed, and methods of interest are returned in the body.

The response body includes a section for each type of token operation (burn/mint/transfer/approval), which
specifies a subset of the input body useful to that operation. The caller (FireFly) can then store and
provide the proper subset of the interface for every future token operation (via the `interface` parameter).

```
{
  "burn": {
    "format": "abi",
    "methods": [
      {
        "name": "burn",
        "type": "function",
        "inputs": [
          {
            "internalType": "uint256",
            "name": "tokenId",
            "type": "uint256"
          }
        ],
        "outputs": [],
        "stateMutability": "nonpayable"
      }
    ]
  },
  "mint": { ... },
  "transfer": { ... },
  "approval": { ... }
}
```

### `POST /mint`

Mint new tokens.

**Request**

```
{
  "namespace": "default",
  "poolLocator": "id=F1",
  "signer": "0x0Ef1D0Dd56a8FB1226C0EaC374000B81D6c8304A",
  "to": "0x0Ef1D0Dd56a8FB1226C0EaC374000B81D6c8304A",
  "amount": "10",
  "tokenIndex": "1",
  "uri": "ipfs://000000",
  "requestId": "1",
  "data": "transfer-metadata",
  "config": {},
  "interface": {}
}
```

| Parameter   | Type          | Description                                                                                                                                                                                    |
| ----------- | ------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| namespace   | string        | The namespace of the token pool                                                                                                                                                                |
| poolLocator | string        | The locator of the pool, as supplied by the output of the pool creation.                                                                                                                       |
| signer      | string        | The signing identity to be used for the blockchain transaction, in a format understood by this connector.                                                                                      |
| to          | string        | The identity to receive the minted tokens, in a format understood by this connector.                                                                                                           |
| amount      | number string | The amount of tokens to mint.                                                                                                                                                                  |
| tokenIndex  | string        | (OPTIONAL) For non-fungible tokens that require choosing an index at mint time, the index of the specific token to mint.                                                                       |
| uri         | string        | (OPTIONAL) For non-fungible tokens that support choosing a URI at mint time, the URI to be attached to the token.                                                                              |
| requestId   | string        | (OPTIONAL) A unique identifier for this request. Will be included in the "receipt" websocket event to match receipts to requests.                                                              |
| data        | string        | (OPTIONAL) A data string that should be returned in the connector's response to this mint request.                                                                                             |
| config      | object        | (OPTIONAL) An arbitrary JSON object where the connector may accept additional parameters if desired. Each connector may define its own valid options to influence how the mint is carried out. |
| interface   | object        | (OPTIONAL) Details on interface methods that are useful to this operation, as negotiated previously by a `/checkinterface` call.                                                               |

**Response**

HTTP 202: request was accepted, but mint will occur asynchronously, with "receipt" and "token-mint" events sent later on the websocket.

_See [Response Types: Async Request](#async-request)_

### `POST /burn`

Burn tokens.

**Request**

```
{
  "namespace": "default",
  "poolLocator": "id=F1",
  "signer": "0x0Ef1D0Dd56a8FB1226C0EaC374000B81D6c8304A",
  "from": "0x0Ef1D0Dd56a8FB1226C0EaC374000B81D6c8304A",
  "amount": "10",
  "tokenIndex": "1",
  "requestId": "1",
  "data": "transfer-metadata",
  "config": {},
  "interface": {}
}
```

| Parameter   | Type          | Description                                                                                                                                                                                    |
| ----------- | ------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| namespace   | string        | The namespace of the token pool                                                                                                                                                                |
| poolLocator | string        | The locator of the pool, as supplied by the output of the pool creation.                                                                                                                       |
| signer      | string        | The signing identity to be used for the blockchain transaction, in a format understood by this connector.                                                                                      |
| from        | string        | The identity that currently owns the tokens to be burned, in a format understood by this connector.                                                                                            |
| amount      | number string | The amount of tokens to burn.                                                                                                                                                                  |
| tokenIndex  | string        | (OPTIONAL) For non-fungible tokens, the index of the specific token to burn.                                                                                                                   |
| requestId   | string        | (OPTIONAL) A unique identifier for this request. Will be included in the "receipt" websocket event to match receipts to requests.                                                              |
| data        | string        | (OPTIONAL) A data string that should be returned in the connector's response to this burn request.                                                                                             |
| config      | object        | (OPTIONAL) An arbitrary JSON object where the connector may accept additional parameters if desired. Each connector may define its own valid options to influence how the burn is carried out. |
| interface   | object        | (OPTIONAL) Details on interface methods that are useful to this operation, as negotiated previously by a `/checkinterface` call.                                                               |

**Response**

HTTP 202: request was accepted, but burn will occur asynchronously, with "receipt" and "token-burn" events sent later on the websocket.

_See [Response Types: Async Request](#async-request)_

### `POST /transfer`

Transfer tokens from one address to another.

**Request**

```
{
  "namespace": "default",
  "poolLocator": "id=F1",
  "signer": "0x0Ef1D0Dd56a8FB1226C0EaC374000B81D6c8304A",
  "from": "0x0Ef1D0Dd56a8FB1226C0EaC374000B81D6c8304A",
  "to": "0xb107ed9caa1323b7bc36e81995a4658ec2251951",
  "amount": "1",
  "tokenIndex": "1",
  "requestId": "1",
  "data": "transfer-metadata",
  "config": {},
  "interface": {}
}
```

| Parameter   | Type          | Description                                                                                                                                                                                        |
| ----------- | ------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| namespace   | string        | The namespace of the token pool                                                                                                                                                                    |
| poolLocator | string        | The locator of the pool, as supplied by the output of the pool creation.                                                                                                                           |
| signer      | string        | The signing identity to be used for the blockchain transaction, in a format understood by this connector.                                                                                          |
| from        | string        | The identity to be used for the source of the transfer, in a format understood by this connector.                                                                                                  |
| to          | string        | The identity to be used for the destination of the transfer, in a format understood by this connector.                                                                                             |
| amount      | number string | The amount of tokens to transfer.                                                                                                                                                                  |
| tokenIndex  | string        | (OPTIONAL) For non-fungible tokens, the index of the specific token to transfer.                                                                                                                   |
| requestId   | string        | (OPTIONAL) A unique identifier for this request. Will be included in the "receipt" websocket event to match receipts to requests.                                                                  |
| data        | string        | (OPTIONAL) A data string that should be returned in the connector's response to this transfer request.                                                                                             |
| config      | object        | (OPTIONAL) An arbitrary JSON object where the connector may accept additional parameters if desired. Each connector may define its own valid options to influence how the transfer is carried out. |
| interface   | object        | (OPTIONAL) Details on interface methods that are useful to this operation, as negotiated previously by a `/checkinterface` call.                                                                   |

**Response**

HTTP 202: request was accepted, but transfer will occur asynchronously, with "receipt" and "token-transfer" events sent later on the websocket.

_See [Response Types: Async Request](#async-request)_

### `POST /approval`

Approve another identity to manage tokens.

**Request**

```
{
  "namespace": "default",
  "poolLocator": "id=F1",
  "signer": "0x0Ef1D0Dd56a8FB1226C0EaC374000B81D6c8304A",
  "operator": "0xb107ed9caa1323b7bc36e81995a4658ec2251951",
  "approved": true,
  "requestId": "1",
  "data": "approval-metadata",
  "config": {},
  "interface": {}
}
```

| Parameter   | Type    | Description                                                                                                                                                                                        |
| ----------- | ------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| namespace   | string  | The namespace of the token pool                                                                                                                                                                    |
| poolLocator | string  | The locator of the pool, as supplied by the output of the pool creation.                                                                                                                           |
| signer      | string  | The signing identity to be used for the blockchain transaction, in a format understood by this connector.                                                                                          |
| operator    | string  | The identity to be approved (or unapproved) for managing the signer's tokens.                                                                                                                      |
| approved    | boolean | Whether to approve (the default) or unapprove.                                                                                                                                                     |
| requestId   | string  | (OPTIONAL) A unique identifier for this request. Will be included in the "receipt" websocket event to match receipts to requests.                                                                  |
| data        | string  | (OPTIONAL) A data string that should be returned in the connector's response to this approval request.                                                                                             |
| config      | object  | (OPTIONAL) An arbitrary JSON object where the connector may accept additional parameters if desired. Each connector may define its own valid options to influence how the approval is carried out. |
| interface   | object  | (OPTIONAL) Details on interface methods that are useful to this operation, as negotiated previously by a `/checkinterface` call.                                                                   |

**Response**

HTTP 202: request was accepted, but approval will occur asynchronously, with "receipt" and "token-approval" events sent later on the websocket.

_See [Response Types: Async Request](#async-request)_

## Websocket Commands

In order to start listening for events on a certain namespace, the client needs to send the `start` command. Clients should send this command every time they connect, or after an automatic reconnect.

```
{
  "type": "start",
  "namespace": "default"
}
```

## Websocket Events

A connector should expose a websocket at `/api/ws`. All emitted websocket events are a JSON string of the form:

```
{
  "id": "event-id",
  "event": "event-name",
  "data": {}
}
```

The `event` name will match one of the names listed below, and the `data` payload will correspond to the linked response object.

All events _except the receipt event_ must be acknowledged by sending an ack of the form:

```
{
  "event": "ack",
  "data": {
    "id": "event-id"
  }
}
```

Many messages may also be batched into a single websocket event of the form:

```
{
  "id": "event-id",
  "event": "batch",
  "data": {
    "events": [
      {
        "event": "event-name",
        "data": {}
      },
      ...
    ]
  }
}
```

Batched messages must be acked all at once using the ID of the batch.

### `receipt`

An asynchronous operation has completed.

_See [Response Types: Receipt](#receipt_1)_

### `token-pool`

A new token pool has been created or activated.

_See [Response Types: Token Pool](#token-pool_1)_

### `token-mint`

Tokens have been minted.

_See [Response Types: Token Transfer](#token-transfer_1)_

### `token-burn`

Tokens have been burned.

_See [Response Types: Token Transfer](#token-transfer_1)_

### `token-transfer`

Tokens have been transferred.

_See [Response Types: Token Transfer](#token-transfer_1)_

### `token-approval`

Token approvals have changed.

_See [Response Types: Token Approval](#token-approval_1)_

## Response Types

### Async Request

Many operations may happen asynchronously in the background, and will return only a request ID. This may be a request ID that was passed in, or if none was passed, will be randomly assigned. This ID can be used to correlate with a receipt event later received on the websocket.

```
{
  "id": "b84ab27d-0d50-42a6-9c26-2fda5eb901ba"
}
```

### Receipt

```
  "headers": {
    "type": "",
    "requestId": ""
  }
  "transactionHash": "",
  "errorMessage": ""
}
```

| Parameter         | Type        | Description                                                                                             |
| ----------------- | ----------- | ------------------------------------------------------------------------------------------------------- |
| headers.type      | string enum | The type of this response. Should be "TransactionSuccess", "TransactionUpdate", or "TransactionFailed". |
| headers.requestId | string      | The ID of the request to which this receipt should correlate.                                           |
| transactionHash   | string      | The unique identifier for the blockchain transaction which generated this receipt.                      |
| errorMessage      | string      | (OPTIONAL) If this is a failure, contains details on the reason for the failure.                        |

### Token Pool

```
{
  "namespace": "default",
  "type": "fungible",
  "data": "pool-metadata",
  "poolLocator": "id=F1",
  "standard": "ERC20",
  "interfaceFormat": "abi",
  "symbol": "FFC",
  "decimals": 18,
  "info": {},
  "signer": "0x0Ef1D0Dd56a8FB1226C0EaC374000B81D6c8304A",
  "blockchain": {}
}
```

| Parameter       | Type        | Description                                                                                                                                                                                                                                                                                 |
| --------------- | ----------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| namespace       | string      | The namespace of the token pool                                                                                                                                                                                                                                                             |
| type            | string enum | The type of pool that was created.                                                                                                                                                                                                                                                          |
| data            | string      | A copy of the data that was passed in on the creation request.                                                                                                                                                                                                                              |
| poolLocator     | string      | A string to identify this pool, generated by the connector. Must be unique for each pool created by this connector. Will be passed back on all operations within this pool, and may be packed with relevant data about the pool for later usage (such as the address and type of the pool). |
| standard        | string      | (OPTIONAL) The name of a well-defined token standard to which this pool conforms.                                                                                                                                                                                                           |
| interfaceFormat | string enum | (OPTIONAL) If this connector supports the `/checkinterface` API, this is the interface format to be used for describing the interface underpinning this pool. Must be "abi" or "ffi".                                                                                                       |
| symbol          | string      | (OPTIONAL) The symbol for this token pool, if applicable.                                                                                                                                                                                                                                   |
| decimals        | number      | (OPTIONAL) The number of decimals used for balances in this token pool, if applicable.                                                                                                                                                                                                      |
| info            | object      | (OPTIONAL) Additional information about the pool. Each connector may define the format for this object.                                                                                                                                                                                     |
| signer          | string      | (OPTIONAL) If this operation triggered a blockchain transaction, the signing identity used for the transaction.                                                                                                                                                                             |
| blockchain      | object      | (OPTIONAL) If this operation triggered a blockchain transaction, contains details on the blockchain event in FireFly's standard blockchain event format.                                                                                                                                    |

### Token Transfer

Note that mint and burn operations are just specialized versions of transfer. A mint will omit the "from" field,
while a burn will omit the "to" field.

```
{
  "namespace": "default",
  "id": "1",
  "data": "transfer-metadata",
  "poolLocator": "id=F1",
  "poolData": "extra-pool-info",
  "from": "0x0Ef1D0Dd56a8FB1226C0EaC374000B81D6c8304A",
  "to": "0xb107ed9caa1323b7bc36e81995a4658ec2251951",
  "amount": "1",
  "tokenIndex": "1",
  "uri": "ipfs://000000",
  "signer": "0x0Ef1D0Dd56a8FB1226C0EaC374000B81D6c8304A",
  "blockchain": {}
}
```

| Parameter   | Type          | Description                                                                                                                                                                                                                                                          |
| ----------- | ------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| namespace   | string        | The namespace of the token pool                                                                                                                                                                                                                                      |
| id          | string        | An identifier for this transfer. Must be unique for every transfer within this pool.                                                                                                                                                                                 |
| data        | string        | A copy of the data that was passed in on the mint/burn/transfer request. May be omitted if the token contract does not support a method of attaching extra data (will result in reduced ability for FireFly to correlate the inputs and outputs of the transaction). |
| poolLocator | string        | The locator of the pool, as supplied by the output of the pool creation.                                                                                                                                                                                             |
| poolData    | string        | The extra data associated with the pool at pool activation.                                                                                                                                                                                                          |
| from        | string        | The identity used for the source of the transfer.                                                                                                                                                                                                                    |
| to          | string        | The identity used for the destination of the transfer.                                                                                                                                                                                                               |
| amount      | number string | The amount of tokens transferred.                                                                                                                                                                                                                                    |
| tokenIndex  | string        | (OPTIONAL) For non-fungible tokens, the index of the specific token transferred.                                                                                                                                                                                     |
| uri         | string        | (OPTIONAL) For non-fungible tokens, the URI attached to the token.                                                                                                                                                                                                   |
| signer      | string        | (OPTIONAL) If this operation triggered a blockchain transaction, the signing identity used for the transaction.                                                                                                                                                      |
| blockchain  | object        | (OPTIONAL) If this operation triggered a blockchain transaction, contains details on the blockchain event in FireFly's standard blockchain event format.                                                                                                             |

### Token Approval

```
{
  "namespace": "default",
  "id": "1",
  "data": "transfer-metadata",
  "poolLocator": "id=F1",
  "poolData": "extra-pool-info",
  "operator": "0xb107ed9caa1323b7bc36e81995a4658ec2251951",
  "approved": true,
  "subject": "0x0Ef1D0Dd56a8FB1226C0EaC374000B81D6c8304A:0xb107ed9caa1323b7bc36e81995a4658ec2251951",
  "info": {},
  "signer": "0x0Ef1D0Dd56a8FB1226C0EaC374000B81D6c8304A",
  "blockchain": {}
}
```

| Parameter   | Type    | Description                                                                                                                                                                                                                                                                                 |
| ----------- | ------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| namespace   | string  | The namespace of the token pool                                                                                                                                                                                                                                                             |
| id          | string  | An identifier for this approval. Must be unique for every approval within this pool.                                                                                                                                                                                                        |
| data        | string  | A copy of the data that was passed in on the approval request. May be omitted if the token contract does not support a method of attaching extra data (will result in reduced ability for FireFly to correlate the inputs and outputs of the transaction).                                  |
| poolLocator | string  | The locator of the pool, as supplied by the output of the pool creation.                                                                                                                                                                                                                    |
| poolData    | string  | The extra data associated with the pool at pool activation.                                                                                                                                                                                                                                 |
| operator    | string  | The identity that was approved (or unapproved) for managing tokens.                                                                                                                                                                                                                         |
| approved    | boolean | Whether this was an approval or unapproval.                                                                                                                                                                                                                                                 |
| subject     | string  | A string identifying the scope of the approval, generated by the connector. Approvals with the same subject are understood replace one another, so that a previously-recorded approval becomes inactive. This string may be a combination of the identities involved, the token index, etc. |
| info        | object  | (OPTIONAL) Additional information about the approval. Each connector may define the format for this object.                                                                                                                                                                                 |
| signer      | string  | (OPTIONAL) If this operation triggered a blockchain transaction, the signing identity used for the transaction.                                                                                                                                                                             |
| blockchain  | object  | (OPTIONAL) If this operation triggered a blockchain transaction, contains details on the blockchain event in FireFly's standard blockchain event format.                                                                                                                                    |
