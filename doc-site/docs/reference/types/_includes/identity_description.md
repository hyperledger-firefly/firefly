FireFly contains an address book of identities, which is managed in a decentralized
way across a multi-party system through `claim` and `verification` system.

> See [FIR-12](https://github.com/hyperledger-firefly/fir/pull/12) for evolution
> that is happening to Hyperledger FireFly to allow:
>
> - Private address books that are not shared with other participants
> - Multiple address books backed by different chains, in the same node

Root identities are registered with only a `claim` - which is a signed
transaction from a particular blockchain account, to bind a DID with a
`name` that is unique within the network, to that signing key.

The signing key then becomes a [Verifier](./verifier.md) for that identity.
Using that key the root identity can be used to register a new FireFly node
in the network, send and receive messages, and register child identities.

When child identities are registered, a `claim` using a key that is going
to be the [Verifier](./verifier.md) for that child identity is required.
However, this is insufficient to establish that identity as a child identity
of the parent. There must be an additional `verification` that references
the `claim` (by UUID) using the key verifier of the parent identity.

### DIDs

FireFly has adopted the [DID](https://www.w3.org/TR/did-core/) standard for
representing identities. A "DID Method" name of `firefly` is used to represent
that the built-in identity system of Hyperledger FireFly is being used
to resolve these DIDs.

So an example FireFly DID for organization `abcd1234` is:

- `did:firefly:org/abcd1234`

> The adoption of DIDs in Hyperledger FireFly v1.0 is also a stepping stone
> to allowing pluggable DID based identity resolvers into FireFly in the future.

You can also download a [DID Document](https://www.w3.org/TR/did-core/#dfn-did-documents)
for a FireFly identity, which represents the verifiers and other information about
that identity according to the JSON format in the DID standard.
