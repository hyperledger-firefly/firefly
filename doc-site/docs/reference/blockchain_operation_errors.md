---
title: Blockchain Operation Errors
---

## Blockchain Error Messages

The receipt for a FireFly blockchain operation contains an `extraInfo` section that records additional information about the transaction. For example:

```
"receipt": {
  ...
  "extraInfo": [
    {
      {
        "contractAddress":"0x87ae94ab290932c4e6269648bb47c86978af4436",
        "cumulativeGasUsed":"33812",
        "from":"0x2b1c769ef5ad304a4889f2a07a6617cd935849ae",
        "to":"0x302259069aaa5b10dc6f29a9a3f72a8e52837cc3",
        "gasUsed":"33812",
        "status":"0",
        "errorMessage":"Not enough tokens", 
      }
    }
  ],
  ...
},
```

The `errorMessage` field can be be set by a blockchain connector to provide FireFly and the end-user with more information about the reason why a tranasction failed. The blockchain connector can choose what information to include in `errorMessage` field. It may be set to an error message relating to the blockchain connector itself or an error message passed back from the blockchain or smart contract that was invoked.

## Default Error Format With Hyperledger FireFly 1.3 and Hyperledger Besu 24.3.0

If FireFly is configured to connect to a Besu EVM client, and Besu has been configured with the `revert-reason-enabled=true` setting (note - the default value for Besu is `false`) error messages passed to FireFly from the blockchain client itself will be set correctly in the FireFly blockchain operation. For example:

 - `"errorMessage":"Not enough tokens"` for a revert error string from a smart contract

If the smart contract uses a custom error type, Besu will return the revert reason to FireFly as a hexadecimal string but FireFly will be unable to decode it into. In this case the blockchain operation error message and return values will be set to:

 - `"errorMessage":"FF23053: Error return value for custom error: <revert hex string>`
 - `"returnValue":"<revert hex string>"`

A future update to FireFly could be made to automatically decode custom error revert reasons if FireFly knows the ABI for the custom error. See [FireFly issue 1466](https://github.com/hyperledger-firefly/firefly/issues/1466) which describes the current limitation.

If FireFly is configured to connect to Besu without `revert-reason-enabled=true` the error message will be set to:

 - `"errorMessage":"FF23054: Error return value unavailable"`

## Error Retrieval Details

The precise format of the error message in a blockchain operation can vary based on different factors. The sections below describe in detail how the error message is populted, with specific references to the `firefly-evmconnect` blockchain connector.

### Format of a `firefly-evmconnect` Error Message

The following section describes the way that the `firefly-evmconnect` plugin uses the `errorMessage` field. This serves both as an explanation of how EVM-based transaction errors will be formatted, and as a guide that other blockchain connectors may decide to follow.

The `errorMessage` field for a `firefly-evmconnect` transaction may contain one of the following:

1. An error message from the FireFly blockchain connector
  - For example `"FF23054: Error return value unavailable"`
2. A decoded error string from the blockchain transaction
  - For example `Not enough tokens`
  - This could be an error string from a smart contract e.g. `require(requestedTokens <= allowance, "Not enough tokens");`
3. An un-decoded byte string from the blockchain transaction
  - For example 
```
FF23053: Error return value for custom error: 0x1320fa6a00000000000000000000000000000000000000000000000000000000000000640000000000000000000000000000000000000000000000000000000000000010
```
  - This could be a custom error from a smart contract e.g.
```
error AllowanceTooSmall(uint256 requested, uint256 allowance);
...
revert AllowanceTooSmall({ requested: 100, allowance: 20 });
```
  - If an error reason cannot be decoded the `returnValue` of the `extraInfo` will be set to the raw byte string. For example:
```
"receipt": {
  ...
  "extraInfo": [
     {
       {
         "contractAddress":"0x87ae94ab290932c4e6269648bb47c86978af4436",
         "cumulativeGasUsed":"33812",
         "from":"0x2b1c769ef5ad304a4889f2a07a6617cd935849ae",
         "to":"0x302259069aaa5b10dc6f29a9a3f72a8e52837cc3",
         "gasUsed":"33812",
         "status":"0",
         "errorMessage":"FF23053: Error return value for custom error: 0x1320fa6a00000000000000000000000000000000000000000000000000000000000000640000000000000000000000000000000000000000000000000000000000000010", 
         "returnValue":"0x1320fa6a00000000000000000000000000000000000000000000000000000000000000640000000000000000000000000000000000000000000000000000000000000010"
       }
     }
  ],
  ...
},
```

### Retrieving EVM Blockchain Transaction Errors

The ability of a blockchain connector such as `firefly-evmconnect` to retrieve the reason for a transaction failure, is dependent on by the configuration of the blockchain it is connected to. For an EVM blockchain the reason why a transaction failed is recorded with the `REVERT` op code, with a `REASON` set to the reason for the failure. By default, most EVM clients do not store this reason in the transaction receipt. This is typically to reduce resource consumption such as memory usage in the client. It is usually possible to configure an EVM client to store the revert reason in the transaction receipt. For example Hyperledger Besu™ provides the `--revert-reason-enabled` configuration option. If the transaction receipt does not contain the revert reason it is possible to request that an EVM client re-run the transaction and return a trace of all of the op-codes, including the final `REVERT` `REASON`. This can be a resource intensive request to submit to an EVM client, and is only available on archive nodes or for very recent blocks.

The `firefly-evmconnect` blockchain connector attempts to obtain the reason for a transaction revert and include it in the `extraInfo` field. It uses the following mechanisms, in this order:

1. Checks if the blockchain transaction receipt contains the revert reason.
2. If the revert reason is not in the receipt, and the `connector.traceTXForRevertReason` configuration option is set to `true`, calls `debug_traceTransaction` to obtain a full trace of the transaction and extract the revert reason. By default, `connector.traceTXForRevertReason` is set to `false` to avoid submitting high-resource requests to the EVM client.

If the revert reason can be obtained using either mechanism above, the revert reason bytes are decoded in the following way:
  - Attempts to decode the bytes as the standard `Error(string)` signature format and includes the decoded string in the `errorMessage`
  - If the reason is not a standard `Error(String)` error, sets the `errorMessage` to `FF23053: Error return value for custom error: <raw hex string>` and includes the raw byte string in the `returnValue` field.

