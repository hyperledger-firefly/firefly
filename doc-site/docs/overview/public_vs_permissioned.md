---
title: Public and Permissioned
---

# Public and Permissioned Blockchain

## Public and Permissioned Blockchains

A separate choice to the technology for your blockchain, is what combination
of blockchain ecosystems you will integrate with.

There are a huge variety of options, and increasingly you might find yourself
integrating with multiple ecosystems in your solutions.

A rough (and incomplete) high level classification of the blockchains available is as follows:

- Layer 1 public blockchains
  - This is where most token ecosystems are rooted
- Layer 2 public scaling solutions backed by a Layer 1 blockchain
  - These are increasing where transaction execution takes place that
    needs to be reflected eventually back to a Layer 1 blockchain (due
    to cost/congestion in the Layer 1 chains)
- Permissioned side-chains
  - Historically this has been where the majority of production adoption of
    enterprise blockchain has focussed, due to the predictable cost, performance,
    and ability to manage the validator set and boundary API security
    alongside a business network governance policy
  - These might have their state check-pointed/rolled-up to a Layer 2 or Layer 1 chain

The lines are blurring between these categorizations as the technologies and ecosystems evolve.

## Public blockchain variations

For the public Layer 1 and 2 solutions, there are too many subclassifications to go into in detail here:

- Whether ecosystems supports custom smart contract execution (EVM based is most common, where contracts are supported)
- What types of token standards are supported, or other chain specific embedded smart contracts
- Whether the chain follows an unspent transaction output (UTXO) or Account model
- How value is bridged in-to / out-of the ecosystem
- How the validator set of the chain is established - most common is Proof of Stake (PoS)
- How data availability is maintained - to check the working of the validators ensure the historical state is not lost
- The consensus algorithm, and how it interacts with the consensus of other blockchains
- How state in a Layer 2 is provable in a parent Layer 1 chain (rollup technologies etc.)

## Common public considerations

The thing most consistent across public blockchain technologies, is that the technical decisions are
backed by token economics.

Put simply, creating a system where it's more financially rewarding to behave honestly, than it
is to subvert and cheat the system.

This means that participation costs, and that the mechanisms needed to reliably get your transactions
into these systems are complex. Also that the time it might take to get a transaction onto the chain
can be much longer than for a permissioned blockchain, with the potential to have to make a number
of adjustments/resubmissions.

The choice of whether to run your own node, or use a managed API, to access these blockchain ecosystems
is also a factor in the behavior of the transaction submission and event streaming.

## FireFly architecture for public chains

One of the fastest evolving aspects of the Hyperledger FireFly ecosystem, is how it facilitates
enterprises to participate in these.

[![FireFly Public Transaction Architecture](../images/firefly_transaction_manager.jpg)](https://github.com/hyperledger-firefly/transaction-manager)

The architecture is summarized as follows:

- New **FireFly Transaction Manager** runtime
  - Operates as a microservice extension of the FireFly Core
  - Uses the `operation` resource within FireFly Core to store and update state
  - Runs as a singleton and is responsible for `nonce` assignment
  - Takes as much heavy lifting away from blockchain specific connectors as possible
- Lightweight FireFly Connector API (`ffcapi`)
  - Simple synchronous RPC operations that map to the most common operations supported across public blockchain technologies
  - Examples:
    - Find the next nonce for a given signing key
    - Serialize a transaction from a set of JSON inputs and an interface definition
    - Submit an un-signed transaction with a given gas price to the blockchain, via a signing wallet
    - Establish a new block listener
    - Poll for new blocks
    - Establish a new event log listener
    - Poll for new events
- Pluggable **Policy Engine**
  - Invoked to make decisions on transaction submission
  - Responsible for gas price calculation
  - Able to intervene and adjust the characteristics of signing/submission
  - OSS reference implementation provided with Gas Station REST API integration
- **Confirmation Manager**
  - Extracted from the Ethconnect codebase
  - Coupled to both transaction submission and event confirmation
  - Embeds an efficient block cache
- **Event Streams**
  - Extracted from the Ethconnect codebase
  - Checkpoint restart based reliable at-least-once delivery of events
  - WebSockets interface upstream to FireFly Core

This evolution involves a significant refactoring of components used for production solutions in the FireFly Ethconnect
microservice since mid 2018. This was summarized in [firefly-ethconnect#149](https://github.com/hyperledger-firefly/ethconnect/issues/149),
and cumulated in the creation of a new repository in 2022.

You can follow the progress and contribute in this repo: https://github.com/hyperledger-firefly/transaction-manager
