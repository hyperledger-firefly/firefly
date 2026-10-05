# Adding a Namespace with the FireFly Helm Chart

When deploying FireFly with the [FireFly Helm chart](https://github.com/hyperledger/firefly-helm-charts), additional predefined namespaces can be configured through `config.extraNamespaces`.

The Helm chart creates the `default` FireFly namespace automatically. To add another namespace, provide the additional namespace definition through `config.extraNamespaces`.

> **Note:** `extraNamespaces` adds FireFly namespaces to the FireFly configuration. It does not create a Kubernetes namespace.

## Prerequisites

This guide assumes that:

- FireFly is already deployed using the FireFly Helm chart.
- Helm is installed and you have access to the Kubernetes cluster running FireFly.
- You have the values file used to deploy your existing FireFly release.

## Minimal configuration

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

The values above assume that `database0` and `eth0` are already configured in the FireFly deployment. The `plugins` list selects which configured plugins are available to the namespace.

For a basic namespace, this is the only additional configuration required.

## Apply the configuration

Save the namespace configuration in a separate values file, for example `extra-namespaces.yaml`.

If you are using the FireFly Helm chart from a local checkout, upgrade the existing FireFly release with your existing values file and the additional namespace values:

```bash
helm upgrade firefly ./charts/firefly \
  -f ./charts/firefly/local-kind-values.yaml \
  -f extra-namespaces.yaml
```

If you are using a different deployment or values file, use the corresponding chart and values files for your environment.

The chart adds the entries from `config.extraNamespaces` to the `namespaces.predefined` section of the FireFly configuration.

## Access the FireFly API

If FireFly is running in Kubernetes and the API is not otherwise exposed, forward the FireFly API port locally:

```bash
kubectl port-forward pod/firefly-0 8080:5000
```

Keep the port-forward running while executing the API commands below.

## Verify the namespace

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

## Verify the namespace configuration

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

## Using the namespace

FireFly APIs that operate within a namespace can be accessed using the namespace-specific API path.

For example, to query messages in the `payments` namespace:

```bash
curl http://127.0.0.1:8080/api/v1/namespaces/payments/messages
```

A newly created namespace will normally return an empty list until messages have been created.
