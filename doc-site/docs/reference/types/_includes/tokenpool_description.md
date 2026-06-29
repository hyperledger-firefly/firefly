Token pools are a FireFly construct for describing a set of tokens.

The total supply of a particular fungible token, or a group of related non-fungible tokens.

The exact definition of a token pool is dependent on the token connector implementation.

Check the documentation for your chosen connector implementation to see the detailed options
for configuring a new Token Pool.

Note that it is very common to use a Token Pool to teach Hyperledger FireFly about an
_existing token_, so that you can start interacting with a token that already exists.

### Example token pool types

Some examples of how the generic concept of a Token Pool maps to various well-defined Ethereum standards:

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
