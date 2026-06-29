---
title: Pinning Data
---

# Pin off-chain data to a custom blockchain transaction

This guide describes how to associate an arbitrary off-chain payload with a blockchain transaction on a contract of your own design. A hash of the payload will be recorded as part of the blockchain transaction, and on the receiving side, FireFly will ensure that both the on-chain and off-chain pieces are received and aggregated together.

> **NOTE:** This is an advanced FireFly feature. Before following any of the steps in this guide, you should be very familiar
> and comfortable with the basic features of how [broadcast messages](../broadcast_data.md) and [private messages](../private_send.md)
> work, be proficient at custom contract development on your blockchain of choice, and understand the
> fundamentals of how FireFly interacts with [custom contracts](./index.md).

## Designing a compatible contract

In order to allow pinning a FireFly message batch with a custom contract transaction, your contract must
meet certain criteria.

First, any external method of the contract that will be used for associating with off-chain payloads
must provide an extra parameter for passing the encoded batch data. This must be the last parameter
in the method signature. This convention is chosen partly to align with the Ethereum
[ERC5750](https://eips.ethereum.org/EIPS/eip-5750) standard, but should serve as a straightforward
guideline for nearly any blockchain.

Second, this method must emit a `BatchPin` event that can be received and parsed by FireFly. Exactly how
the data is unpacked and used to emit this event will differ for each blockchain.

### Ethereum

```solidity
import "@hyperledger-firefly/firefly-contracts/contracts/IBatchPin.sol";

contract CustomPin {
    IBatchPin firefly;

    function setFireFlyAddress(address addr) external {
        firefly = IBatchPin(addr);
    }

    function sayHello(bytes calldata data) external {
        require(
            address(firefly) != address(0),
            "CustomPin: FireFly address has not been set"
        );

        /* do custom things */

        firefly.pinBatchData(data);
    }
}
```

- The method in question will receive packed "batch pin" data in its last method parameter (in the
  form of ABI-encoded `bytes`). The method must invoke the `pinBatchData` method of the
  **FireFly Multiparty Contract** and pass along this data payload. It is generally good practice to
  trigger this as a final step before returning, after the method has performed its own logic.
- This also implies that the contract must know the on-chain location of the
  **FireFly Multiparty Contract**. How this is achieved is up to your individual implementation -
  the example above shows exposing a method to set the address. An application may leverage the fact that
  this location is available by querying the FireFly
  `/status` API (under `multiparty.contract.location` as of FireFly v1.1.0). However, the application must
  also consider how appropriately secure this functionality, and how to update this location if a multiparty
  "network action" is used to migrate the network onto a new FireFly multiparty contract.

### Fabric

```go
package chaincode

import (
	"encoding/json"
	"fmt"

	"github.com/hyperledger/fabric-contract-api-go/contractapi"
	"github.com/hyperledger-firefly/firefly/custompin_sample/batchpin"
)

type SmartContract struct {
	contractapi.Contract
}

func (s *SmartContract) MyCustomPin(ctx contractapi.TransactionContextInterface, data string) error {
	event, err := batchpin.BuildEventFromString(ctx, data)
	if err != nil {
		return err
	}
	bytes, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event: %s", err)
	}
	return ctx.GetStub().SetEvent("BatchPin", bytes)
}
```

- The method in question will received packed "batch pin" data in its last method parameter (in the
  form of a JSON-encoded `string`). The method must unpack this argument into a JSON object.
- The contract must directly set a `BatchPin` event in the same format that is used by the
  **FireFly Multiparty Contract**.

## Initializing FireFly

Once you have a contract designed, you can [initialize your environment](../../gettingstarted/setup_env.md)
using the blockchain of your choice.

No special initialization arguments are needed for Ethereum.

If you are using Fabric, you must pass the `--custom-pin-support` argument when initializing your
FireFly stack. This will ensure that the `BatchPin` event listener listens to events from all chaincode
deployed on the default channel, instead of only listening to events from the pre-deployed FireFly chaincode.

## Invoking the contract

You can follow the normal steps for [Ethereum](./ethereum.md) or [Fabric](./fabric.md) to define your contract
interface and API in FireFly. When invoking the contract, you can include a [message payload](../../reference/types/message.md)
alongside the other parameters.

`POST` `http://localhost:5000/api/v1/namespaces/default/apis/custom-pin/invoke/sayHello`

```json
{
  "input": {},
  "message": {
    "data": [
      {
        "value": "payload here"
      }
    ]
  }
}
```

## Listening for events

All parties that receive the message will receive a `message_confirmed` on their [event listeners](../events.md).
This event confirms that the off-chain payload has been received (via data exchange or shared storage) _and_
that the blockchain transaction has been received and sequenced. It is guaranteed that these `message_confirmed`
events will be ordered based on the sequence of the on-chain transactions, regardless of when the off-chain
payload becomes available. This means that all parties will order messages on a given topic in exactly the
same order, allowing for deterministic but decentralized [event-driven architecture](../../reference/events.md).
