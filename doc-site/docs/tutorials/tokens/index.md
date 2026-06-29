---
title: Use tokens
---

## Quick reference

Tokens are a critical building block in many blockchain-backed applications. Fungible tokens can represent a store
of value or a means of rewarding participation in a multi-party system, while non-fungible tokens provide a clear
way to identify and track unique entities across the network. FireFly provides flexible mechanisms to operate on
any type of token and to tie those operations to on- and off-chain data.

- FireFly provides an abstraction layer for multiple types of tokens
- Tokens are grouped into _pools_, which each represent a particular type or class of token
- Each pool is classified as _fungible_ or _non-fungible_
- In the case of _non-fungible_ tokens, the pool is subdivided into individual tokens with a unique _token index_
- Within a pool, you may _mint (issue)_, _transfer_, and _burn (redeem)_ tokens
- Each operation can be optionally accompanied by a broadcast or private message, which will be recorded alongside the transfer on-chain
- FireFly tracks a history of all token operations along with all current token balances
- The blockchain backing each token connector may be the same _or_ different from the one backing FireFly message pinning

## What is a pool?

Token pools are a FireFly construct for describing a set of tokens. The exact definition of a token pool
is dependent on the token connector implementation. Some examples of how pools might map to various well-defined
Ethereum standards:

- **[ERC-1155](https://eips.ethereum.org/EIPS/eip-1155):** a single contract instance can efficiently allocate
  many isolated pools of fungible or non-fungible tokens
- **[ERC-20](https://eips.ethereum.org/EIPS/eip-20) / [ERC-777](https://eips.ethereum.org/EIPS/eip-777):**
  each contract instance represents a single fungible pool of value, e.g. "a coin"
- **[ERC-721](https://eips.ethereum.org/EIPS/eip-721):** each contract instance represents a single pool of NFTs,
  each with unique identities within the pool
- **[ERC-1400](https://github.com/ethereum/eips/issues/1411) / [ERC-1410](https://github.com/ethereum/eips/issues/1410):**
  partially supported in the same manner as ERC-20/ERC-777, but would require new features for working with partitions

These are provided as examples only - a custom token connector could be backed by any token technology (Ethereum or otherwise)
as long as it can support the basic operations described here (create pool, mint, burn, transfer). Other FireFly repos include a sample implementation of a token connector for [ERC-20 and ERC-721](https://github.com/hyperledger-firefly/tokens-erc20-erc721) as well as [ERC-1155](https://github.com/hyperledger-firefly/tokens-erc1155).

<!--nav-->
