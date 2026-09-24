---
title: Namespaces
---

# Namespaces

[![FireFly Namespaces Example](../images/hyperledger-firefly-namespaces-example-with-org.png "FireFly namespaces example")](../images/hyperledger-firefly-namespaces-example-with-org.png)

## Introduction to Namespaces

Namespaces are a construct for segregating data and operations within a FireFly supernode. Each namespace is an isolated environment within a FireFly runtime, that allows independent configuration of:

- Plugin and infrastructure components
- API security
- Identity broadcasting
- On-chain data indexing
- How datatypes, locations of on-chain contrats, etc. should be shared

They can be thought of in two basic modes:

### Multi-party Namespaces

This namespace is shared with one or more other FireFly nodes. It requires three types of communication plugins - blockchain, data exchange, and shared storage. Organization and node identities must be claimed with an identity broadcast when joining the namespace, which establishes credentials for blockchain and off-chain communication. Shared objects can be defined in the namespace (such as datatypes and token pools), and details of them will be implicitly broadcast to other members.

This type of namespace is used when multiple parties need to share on- and off-chain data and agree upon the ordering and authenticity of that data. For more information, see the [multi-party system](../overview/multiparty/index.md) overview.

### Gateway Namespaces

Nothing in this namespace will be shared automatically, and no assumptions are made about whether other parties connected through this namespace are also using Hyperledger FireFly. Plugins for data exchange and shared storage are not supported. If any identities or definitions are created in this namespace, they will be stored in the local database, but will not be shared implicitly outside the node.

This type of namespace is mainly used when interacting directly with a blockchain, without assuming that the interaction needs to conform to FireFly's multi-party system model.

## Configuration

FireFly nodes can be configured with one or many namespaces of different modes. This means that a single FireFly node can be used to interact with multiple distinct blockchains, multiple distinct token economies, and multiple business networks.

Below is an example plugin and namespace configuration containing both a multi-party and gateway namespace:

```
plugins:
  database:
  - name: database0
    type: sqlite3
    sqlite3:
      migrations:
        auto: true
      url: /etc/firefly/db?_busy_timeout=5000
  blockchain:
  - name: blockchain0
    type: ethereum
    ethereum:
      ethconnect:
        url: http://ethconnect_0:8080
        topic: "0"
  - name: blockchain1
    type: ethereum
    ethereum:
      ethconnect:
        url: http://ethconnect_01:8080
        topic: "0"
  dataexchange:
  - name: dataexchange0
    type: ffdx
    ffdx:
      url: http://dataexchange_0:3000
  sharedstorage:
  - name: sharedstorage0
    type: ipfs
    ipfs:
      api:
        url: http://ipfs_0:5001
      gateway:
        url: http://ipfs_0:8080
  tokens:
  - name: erc20_erc721
    broadcastName: erc20_erc721
    type: fftokens
    fftokens:
      url: http://tokens_0_0:3000
namespaces:
  default: alpha
  predefined:
  - name: alpha
    description: Default predefined namespace
    defaultKey: 0x123456
    plugins: [database0, blockchain0, dataexchange0, sharedstorage0, erc20_erc721]
    multiparty:
      networkNamespace: alpha
      enabled: true
      org:
        name: org0
        description: org0
        key: 0x123456
      node:
        name: node0
        description: node0
      contract:
        - location:
            address: 0x4ae50189462b0e5d52285f59929d037f790771a6
          firstEvent: 0
        - location:
            address: 0x3c1bef20a7858f5c2f78bda60796758d7cafff27
          firstEvent: 5000
  - name: omega
    defaultkey: 0x48a54f9964d7ceede2d6a8b451bf7ad300c7b09f
    description: Gateway namespace
    plugins: [database0, blockchain1, erc20_erc721]
```

The `namespaces.predefined` object contains the follow sub-keys:

- `defaultKey` is a blockchain key used to sign transactions when none is specified (in multi-party mode,
  defaults to the org key)
- `plugins` is an array of plugin names to be activated for this namespace (defaults to
  all available plugins if omitted)
- `multiparty.networkNamespace` is the namespace name to be sent in plugin calls, if it differs from the
  locally used name (useful for interacting with multiple shared namespaces of the same name -
  defaults to the value of `name`)
- `multiparty.enabled` controls if multi-party mode is enabled (defaults to true if an org key or
  org name is defined on this namespace _or_ in the deprecated `org` section at the root)
- `multiparty.org` is the root org identity for this multi-party namespace (containing `name`,
  `description`, and `key`)
- `multiparty.node` is the local node identity for this multi-party namespace (containing `name` and
  `description`)
- `multiparty.contract` is an array of objects describing the location(s) of a FireFly multi-party
  smart contract. Its children are blockchain-agnostic `location` and `firstEvent` fields, with formats
  identical to the same fields on custom contract interfaces and contract listeners. The blockchain plugin
  will interact with the first contract in the list until instructions are received to terminate it and
  migrate to the next.

## Adding a Namespace with the FireFly Helm Chart

When deploying FireFly with the [FireFly Helm chart](https://github.com/hyperledger/firefly-helm-charts), additional predefined namespaces can be configured through `config.extraNamespaces`.

The Helm chart creates the `default` FireFly namespace automatically. To add another namespace, provide the additional namespace definition through `config.extraNamespaces`.

> **Note:** `extraNamespaces` adds FireFly namespaces to the FireFly configuration. It does not create a Kubernetes namespace.

### Minimal configuration

The following example adds a `payments` namespace with a PostgreSQL database and Ethereum blockchain plugin:

```yaml
config:
  extraNamespaces: |
    - name: payments
      description: Payments namespace
      plugins:
        - database0
        - eth0
```

The values above assume that `database0` and `eth0` are already configured by the Helm chart. The `plugins` list selects which configured plugins are available to the namespace.

For a basic namespace, this is the only additional configuration required.

### Apply the configuration

Save the namespace configuration in a separate values file, for example `extra-namespaces.yaml`.

If you are using the FireFly Helm chart from a local checkout, upgrade the existing FireFly release with your existing values file and the additional namespace values:

```bash
helm upgrade firefly ./charts/firefly \
  -f ./charts/firefly/local-kind-values.yaml \
  -f extra-namespaces.yaml
```

If you are using a different deployment or values file, use the corresponding chart and values files for your environment.

The chart adds the entries from `config.extraNamespaces` to the `namespaces.predefined` section of the FireFly configuration.

### Access the FireFly API

If FireFly is running in Kubernetes and the API is not otherwise exposed, forward the FireFly API port locally:

```bash
kubectl port-forward pod/firefly-0 8080:5000
```

Keep the port-forward running while executing the API commands below.

### Verify the namespace

After the Helm upgrade, list the namespaces configured on the FireFly node:

```bash
curl http://127.0.0.1:8080/api/v1/namespaces
```

Verify that the returned list contains the newly configured `payments` namespace.

You can also retrieve the namespace directly:

```bash
curl http://127.0.0.1:8080/api/v1/namespaces/payments
```

The response should contain the namespace name, network name, description, and creation timestamp.

### Verify the namespace configuration

Use the namespace status endpoint to verify which plugins are active:

```bash
curl http://127.0.0.1:8080/api/v1/namespaces/payments/status
```

For the example above, the status should show:

- `database0` as the PostgreSQL database plugin
- `eth0` as the Ethereum blockchain plugin
- Multi-party mode disabled
- No data exchange or shared storage plugin configured

This confirms that the namespace is using the plugins specified in `config.extraNamespaces`.

### Using the namespace

FireFly APIs that operate within a namespace can be accessed using the namespace-specific API path.

For example, to query messages in the `payments` namespace:

```bash
curl http://127.0.0.1:8080/api/v1/namespaces/payments/messages
```

A newly created namespace will normally return an empty list until messages have been created.

### Advanced namespace configuration

The example above intentionally uses the minimum configuration needed for a basic namespace.

If your namespace requires additional plugins, multi-party configuration, token connectors, or other namespace options, use the configuration options and restrictions described above.

For example, a multi-party namespace requires the appropriate blockchain, data exchange, and shared storage plugins and additional `multiparty` configuration. See the [Multi-party system](../overview/multiparty/index.md) documentation for the concepts and configuration involved in multi-party operation.

### Config Restrictions

- `name` must be unique on this node
- for historical reasons, "ff_system" is a reserved string and cannot be used as a `name` or `multiparty.networkNamespace`
- a `database` plugin is required for every namespace
- if `multiparty.enabled` is true, plugins _must_ include one each of `blockchain`, `dataexchange`, and
  `sharedstorage`
- if `multiparty.enabled` is false, plugins _must not_ include `dataexchange` or `sharedstorage`
- at most one of each type of plugin is allowed per namespace, except for tokens (which
  may have many per namespace)

All namespaces must be called out in the FireFly config file in order to be valid. Namespaces found in
the database but _not_ represented in the config file will be ignored.

## Definitions

In FireFly, definitions are immutable payloads that are used to define identities, datatypes, smart contract interfaces, token pools, and other constructs. Each type of definition in FireFly has a schema that it must adhere to. Some definitions also have a name and a version which must be unique within a namespace. In a multiparty namespace, definitions are broadcasted to other organizations.

## Local Definitions

The following are all "definition" types in FireFly:

- datatype
- group
- token pool
- contract interface
- contract API
- organization (deprecated)
- node (deprecated)
- identity claim
- identity verification
- identity update

For gateway namespaces, the APIs which create these definitions will become an immediate
local database insert, instead of performing a broadcast. Additional caveats:

- identities in this mode will not undergo any claim/verification process,
  but will be created and stored locally
- datatypes and groups will not be supported, as they are only useful in the context
  of messaging (which is disabled in gateway namespaces)
