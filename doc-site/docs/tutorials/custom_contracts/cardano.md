---
title: Cardano
---

# Work with Cardano dApps

This guide describes the steps to author and deploy a Cardano dApp through FireFly.

## What is a Cardano dApp?

Cardano dApps typically have two components: off-chain and on-chain.

 - The off-chain component is written in an ordinary programming language using a Cardano-specific library. It builds transactions, signs them with a private key, and submits them to be published to the chain. The FireFly Cardano connector uses a framework called [Balius](https://github.com/txpipe/balius) for off-chain code. This lets you write transaction-building logic in Rust, which is compiled to WebAssembly and run in response to HTTP requests or new blocks reaching the chain.

 - The on-chain component is a "validator script". Validator scripts are written in domain-specific languages such as [Aiken](https://aiken-lang.org/), and compiled to a bytecode called [UPLC](https://aiken-lang.org/uplc). They take a transaction as input, and return true or false to indicate whether that transaction is valid. ADA and native takens can be locked at the address of one of these scripts; if they are, then they can only be spent by transactions which the script considers valid.

## Writing a dApp

> **NOTE:** The source code to this dApp is also available [on GitHub](https://github.com/hyperledger/firefly-cardano/tree/main/wasm/simple-tx).

First, decide on the contract which your dApp will satisfy. FireFly uses [FireFly Interface Format](https://hyperledger.github.io/firefly/latest/reference/firefly_interface_format/) to describe its contracts. Create a file named `contract.json`. An example is below:

### Contract

```json
{
  "name": "sample-contract",
  "description": "Simple TX submission contract",
  "networkName": "sample-contract",
  "version": "0.1.0",
  "errors": [],
  "methods": [
    {
      "description": "Sends ADA to a wallet",
      "details": {},
      "name": "send_ada",
      "params": [
        {
          "name": "fromAddress",
          "schema": {
            "type": "string"
          }
        },
        {
          "name": "toAddress",
          "schema": {
            "type": "string"
          }
        },
        {
          "name": "amount",
          "schema": {
            "type": "integer"
          }
        }
      ],
      "returns": []
    }
  ],
  "events": [
    {
      "name": "TransactionAccepted",
      "description": "",
      "params": [
        {
          "name": "transactionId",
          "schema": {
            "type": "string"
          }
        }
      ]
    },
    {
      "name": "TransactionRolledBack",
      "description": "",
      "params": [
        {
          "name": "transactionId",
          "schema": {
            "type": "string"
          }
        }
      ]
    },
    {
      "name": "TransactionFinalized",
      "description": "",
      "params": [
        {
          "name": "transactionId",
          "schema": {
            "type": "string"
          }
        }
      ]
    }
  ]
}
```

This is describing a contract with a single method, named `send_ada`. This method takes three parameters: a `fromAddress`, a `toAddress`, and an `amount`.

It also emits three events: 

 - `TransactionAccepted(string)` is emitted when the transaction is included in a block.
 - `TransactionRolledBack(string)` is emitted if the transaction was included in a block, and that block got rolled back. This happens maybe once or twice a day on the Cardano network, which is more likely than some other chains, so your code must be able to gracefully handle rollbacks.
 - `TransactionFinalized(string)` is emitted when the transaction has been on the chain for "long enough" that it is effectively immutable. It is up to your tolerance risk.

These three events are all automatically handled by the connector.

### The dApp itself

The Balius framework requires you to write your dApp in Rust, and compile it to WebAssembly.
Set up a new Rust project with the contents below:

`cargo.toml`:
```toml
[package]
name = "sample-contract"
version = "0.1.0"
edition = "2021"

[dependencies]
# The version of firefly-balius should match the version of firefly-cardano which you are using.
firefly-balius = { git = "https://github.com/hyperledger/firefly-cardano", rev = "<firefly cardano version>" }
pallas-addresses = "0.32"
serde = { version = "1", features = ["derive"] }

[lib]
crate-type = ["cdylib"]
```

Code for a sample contract is below:

`src/lib.rs`:
```rust
use std::collections::HashSet;

use balius_sdk::{
    txbuilder::{
        AddressPattern, BuildError, FeeChangeReturn, OutputBuilder, TxBuilder, UtxoPattern,
        UtxoSource,
    },
    Ack, Config, FnHandler, Params, Worker, WorkerResult,
};
use firefly_balius::{
    balius_sdk::{self, Json},
    kv, CoinSelectionInput, FinalizationCondition, NewMonitoredTx, SubmittedTx, WorkerExt as _,
};
use pallas_addresses::Address;
use serde::{Deserialize, Serialize};

// For each method, define a struct with all its parameters.
// Don't forget the "rename_all = camelCase" annotation.
#[derive(Deserialize)]
#[serde(rename_all = "camelCase")]
struct SendAdaRequest {
    pub from_address: String,
    pub to_address: String,
    pub amount: u64,
}

#[derive(Serialize, Deserialize, Default)]
#[serde(rename_all = "camelCase")]
struct CurrentState {
    submitted_txs: HashSet<String>,
}

/// This function builds a transaction to send ADA from one address to another.
fn send_ada(_: Config<()>, req: Params<SendAdaRequest>) -> WorkerResult<NewMonitoredTx> {
    let from_address =
        Address::from_bech32(&req.from_address).map_err(|_| BuildError::MalformedAddress)?;

    // Build an "address source" describing where the funds to transfer are coming from.
    let address_source = UtxoSource::Search(UtxoPattern {
        address: Some(AddressPattern {
            exact_address: from_address.to_vec(),
        }),
        ..UtxoPattern::default()
    });

    // In Cardano, addresses don't hold ADA or native tokens directly.
    // Instead, they control UTxOs (unspent transaction outputs),
    // and those UTxOs contain some amount of ADA and native tokens.
    // You can't spent part of a UTxO in a transaction; instead, transactions
    // include inputs with more funds than they need, and a "change" output
    // to give any excess funds back to the original sender.

    // Build a transaction with
    //  - One or more inputs containing at least `amount` ADA at the address `from_address`
    //  - One output containing exactly `amount` ADA at the address `to_address`
    //  - One output containing any change at the address `from_address`
    let tx = TxBuilder::new()
        .with_input(CoinSelectionInput(address_source.clone(), req.amount))
        .with_output(
            OutputBuilder::new()
                .address(req.to_address.clone())
                .with_value(req.amount),
        )
        .with_output(FeeChangeReturn(address_source));

    // Return that TX. The framework will sign, submit, and monitor it.
    // By returning a `NewMonitoredTx`, we tell the framework that we want it to monitor this transaction.
    // This enables the TransactionApproved, TransactionRolledBack, and TransactionFinalized events from before.
    // Note that we decide the transaction has been finalized after 4 blocks have reached the chain.
    Ok(NewMonitoredTx(
        Box::new(tx),
        FinalizationCondition::AfterBlocks(4),
    ))
}

/// This function is called when a TX produced by this contract is submitted to the blockchain, but before it has reached a block.
fn handle_submit(_: Config<()>, tx: SubmittedTx) -> WorkerResult<Ack> {
    // Keep track of which TXs have been submitted.
    let mut state: CurrentState = kv::get("current_state")?.unwrap_or_default();
    state.submitted_txs.insert(tx.hash);
    kv::set("current_state", &state)?;

    Ok(Ack)
}

fn query_current_state(_: Config<()>, _: Params<()>) -> WorkerResult<Json<CurrentState>> {
    Ok(Json(kv::get("current_state")?.unwrap_or_default()))
}

#[balius_sdk::main]
fn main() -> Worker {
    Worker::new()
        .with_request_handler("send_ada", FnHandler::from(send_ada))
        .with_request_handler("current_state", FnHandler::from(query_current_state))
        .with_tx_submitted_handler(handle_submit)
}

```

## Deploying the dApp

You can use the `firefly-cardano-deploy` tool to deploy this dApp to your running FireFly instance.
This tool will

 - Compile your dApp to WebAssembly
 - Deploy that WebAssembly to a running FireFly node 
 - Deploy your interface to that FireFly node
 - Create an API for that interface

```sh
# The version here should match the version of firefly-cardano which you are using.
cargo install --git https://github.com/hyperledger/firefly-cardano --version <firefly cardano version> firefly-cardano-deploy

CONTRACT_PATH="/path/to/your/dapp"
FIREFLY_URL="http://localhost:5000"
firefly-cardano-deploy --contract-path "$CONTRACT_PATH" --firefly-url "$FIREFLY_URL"
```

After this runs, you should see output like the following:
```
Contract location: {"address":"sample-contract@0.1.0"}
Interface: {"id":"120d061e-bcda-4c2f-a296-018d7cd62a04"}
API available at http://127.0.0.1:5000/api/v1/namespaces/default/apis/sample-contract-0.1.0
Swagger UI at http://127.0.0.1:5000/api/v1/namespaces/default/apis/sample-contract-0.1.0/api
```

## Invoking the dApp

Now that we've set everything up, let's prove it works by sending 1 ADA back to the faucet.

### Request

`POST` `http://localhost:5000/api/v1/namespaces/default/apis/simple-storage-0.1.0/invoke/send_ada`

```json
{
  "input": {
    "fromAddress": "<wallet address you set up before>",
    "toAddress": "addr_test1vqeux7xwusdju9dvsj8h7mca9aup2k439kfmwy773xxc2hcu7zy99",
    "amount": 1000000
  },
  "key": "<wallet address you set up before>"
}
```

### Response
```json
{
  "id": "d191e6ab-3e9c-4a67-99df-8b96b7026e89"
}
```

## Create a blockchain event listener

Now that we've seen how to submit transactions, let's look at how to receive blockchain events so we know when things are happening in realtime.

Remember that this contract is emitting events when transactions are accepted, rolled back, or finalized. In order to receive these events, we first need to instruct FireFly to listen for this specific type of blockchain event. To do this, we create an **Event Listener**. The `/contracts/listeners` endpoint is RESTful so there are `POST`, `GET`, and `DELETE` methods available on it. To create a new listener, we will make a `POST` request. We are going to tell FireFly to listen to events with name `"TransactionAccepted"`, `"TransactionRolledBack"`, or `"TransactionFinalized"` from the FireFly Interface we defined earlier, referenced by its ID. We will also tell FireFly which contract address we expect to emit these events, and the topic to assign these events to. You can specify multiple filters for a listener, in this case we specify one for each event. Topics are a way for applications to subscribe to events they are interested in.

### Request

```json
{
  "filters": [
    {
      "interface": {"id":"120d061e-bcda-4c2f-a296-018d7cd62a04"},
      "location": {"address":"sample-contract@0.1.0"},
      "eventPath": "TransactionAccepted"
    },
    {
      "interface": {"id":"120d061e-bcda-4c2f-a296-018d7cd62a04"},
      "location": {"address":"sample-contract@0.1.0"},
      "eventPath": "TransactionRolledBack"
    },
    {
      "interface": {"id":"120d061e-bcda-4c2f-a296-018d7cd62a04"},
      "location": {"address":"sample-contract@0.1.0"},
      "eventPath": "TransactionFinalized"
    }
  ],
  "options": {
    "firstEvent": "newest"
  },
  "topic": "sample-contract"
}
```

### Response

```json
{
  "id": "b314d8af-2641-4bf2-b386-2e658f3e76a5",
  "interface": {
    "id": "120d061e-bcda-4c2f-a296-018d7cd62a04"
  },
  "namespace": "default",
  "name": "01JPB97KWQ1ZBPWQDNDMEYDMT2",
  "backendId": "01JPB97KWQ1ZBPWQDNDMEYDMT2",
  "location": {
    "address": "sample-contract@0.1.0"
  },
  "created": "2025-03-14T21:33:44.308362312Z",
  "event": {
    "name": "TransactionAccepted",
    "description": "",
    "params": [
      {
        "name": "transactionId",
        "schema": {
          "type": "string"
        }
      }
    ]
  },
  "signature": "sample-contract@0.1.0:TransactionAccepted(string);sample-contract@0.1.0:TransactionRolledBack(string);sample-contract@0.1.0:TransactionRFinalized(string)",
  "topic": "sample-contract",
  "options": {
    "firstEvent": "newest"
  },
  "filters": [
    {
      "event": {
        "name": "TransactionAccepted",
        "description": "",
        "params": [
          {
            "name": "transactionId",
            "schema": {
              "type": "string"
            }
          }
        ]
      },
      "location": {
        "address": "sample-contract@0.1.0"
      },
      "interface": {
        "id": "120d061e-bcda-4c2f-a296-018d7cd62a04"
      },
      "signature": "sample-contract@0.1.0:TransactionAccepted(string)"
    },
    {
      "event": {
        "name": "TransactionRolledBack",
        "description": "",
        "params": [
          {
            "name": "transactionId",
            "schema": {
              "type": "string"
            }
          }
        ]
      },
      "location": {
        "address": "sample-contract@0.1.0"
      },
      "interface": {
        "id": "120d061e-bcda-4c2f-a296-018d7cd62a04"
      },
      "signature": "sample-contract@0.1.0:TransactionRolledBack(string)"
    }
    {
      "event": {
        "name": "TransactionFinalized",
        "description": "",
        "params": [
          {
            "name": "transactionId",
            "schema": {
              "type": "string"
            }
          }
        ]
      },
      "location": {
        "address": "sample-contract@0.1.0"
      },
      "interface": {
        "id": "120d061e-bcda-4c2f-a296-018d7cd62a04"
      },
      "signature": "sample-contract@0.1.0:TransactionFinalized(string)"
    }
  ]
}
```

We can see in the response, that FireFly pulls all the schema information from the FireFly Interface that we broadcasted earlier and creates the listener with that schema. This is useful so that we don't have to enter all of that data again.

## Subscribe to events from our contract

Now that we've told FireFly that it should listen for specific events on the blockchain, we can set up a **Subscription** for FireFly to send events to our app. To set up our subscription, we will make a `POST` to the `/subscriptions` endpoint.

We will set a friendly name `sample-contract` to identify the Subscription when we are connecting to it in the next step.

We're also going to set up a filter to only send events blockchain events from our listener that we created in the previous step. To do that, we'll **copy the listener ID** from the step above (`b314d8af-2641-4bf2-b386-2e658f3e76a5`) and set that as the value of the `listener` field in the example below:

### Request

`POST` `http://localhost:5000/api/v1/namespaces/default/subscriptions`

```json
{
  "namespace": "default",
  "name": "sample-contract",
  "transport": "websockets",
  "filter": {
    "events": "blockchain_event_received",
    "blockchainevent": {
      "listener": "b314d8af-2641-4bf2-b386-2e658f3e76a5"
    }
  },
  "options": {
    "firstEvent": "oldest"
  }
}
```

### Response

```json
{
  "id": "f826269c-65ed-4634-b24c-4f399ec53a32",
  "namespace": "default",
  "name": "sample-contract",
  "transport": "websockets",
  "filter": {
    "events": "blockchain_event_received",
    "message": {},
    "transaction": {},
    "blockchainevent": {
      "listener": "b314d8af-2641-4bf2-b386-2e658f3e76a5"
    }
  },
  "options": {
    "firstEvent": "-1",
    "withData": false
  },
  "created": "2025-03-15T17:35:30.131698921Z",
  "updated": null
}
```

## Receive custom smart contract events

The last step is to connect a WebSocket client to FireFly to receive the event. You can use any WebSocket client you like, such as [Postman](https://www.postman.com/) or a command line app like [`websocat`](https://github.com/vi/websocat).

Connect your WebSocket client to `ws://localhost:5000/ws`.

After connecting the WebSocket client, send a message to tell FireFly to:

- Start sending events
- For the Subscription named `sample-contract`
- On the `default` namespace
- Automatically "ack" each event which will let FireFly immediately send the next event when available

```json
{
  "type": "start",
  "name": "sample-contract",
  "namespace": "default",
  "autoack": true
}
```

> **NOTE:** Do not use `autoack` in production, as it can cause your application to miss events. For resilience, your app should instead respond with an "ack" message to each incoming event. For more details, see the [Websockets documentation](../../reference/types/subscription/#using-start-and-ack-explicitly).

### WebSocket event

After creating the subscription, you should see an event arrive on the connected WebSocket client that looks something like this:

```json
{
  "id": "0f4a31d6-9743-4537-82df-5a9c76ccbd1e",
  "sequence": 24,
  "type": "blockchain_event_received",
  "namespace": "default",
  "reference": "dd3e1554-c832-47a8-898e-f1ee406bea41",
  "created": "2025-03-15T17:32:27.824417878Z",
  "blockchainevent": {
    "id": "dd3e1554-c832-47a8-898e-f1ee406bea41",
    "sequence": 7,
    "source": "cardano",
    "namespace": "default",
    "name": "TransactionAccepted",
    "listener": "1bfa3b0f-3d90-403e-94a4-af978d8c5b14",
    "protocolId": "000000000010/000000/000000",
    "output": {
      "transactionId": "2fad3b4e560b562d32b2e54e25495d11ed342dafe7eba76bc1c4632bbc23d468"
    },
    "info": {
      "address": "0xa5ea5d0a6b2eaf194716f0cc73981939dca26da1",
      "blockNumber": "10",
      "logIndex": "0",
      "signature": "TransactionAccepted(string)",
      "subId": "sb-724b8416-786d-4e67-4cd3-5bae4a26eb0e",
      "timestamp": "1647365460",
      "transactionHash": "2fad3b4e560b562d32b2e54e25495d11ed342dafe7eba76bc1c4632bbc23d468",
      "transactionIndex": "0x0"
    },
    "timestamp": "2025-03-15T17:31:00Z",
    "tx": {
      "type": ""
    }
  },
  "subscription": {
    "id": "f826269c-65ed-4634-b24c-4f399ec53a32",
    "namespace": "default",
    "name": "sample-contract"
  }
}
```

You can see in the event received over the WebSocket connection, the blockchain event that was emitted from our first transaction, which happened in the past. We received this event, because when we set up both the Listener, and the Subscription, we specified the `"firstEvent"` as `"oldest"`. This tells FireFly to look for this event from the beginning of the blockchain, and that your app is interested in FireFly events since the beginning of FireFly's event history.
