---
title: Basic Auth
---

## Quick reference

FireFly has a pluggable auth system which can be enabled at two different layers of the stack. At the top, auth can be enabled at the HTTP listener level. This will protect all requests to the given listener. FireFly has three different HTTP listeners, which could each use a different auth scheme:

1. The main API
1. The SPI (for internal or admin use)
1. The metrics API.

Auth can also be enabled at the namespace level within FireFly as well. This enables several different use cases. For example, you might have two different teams that want to use the same FireFly node, each with different sets of authorized users. You could configure them to use separate namespaces and create separate auth schemes on each.

FireFly has a basic auth plugin built in, which we will be configuring in this tutorial.

> **NOTE**: This guide assumes that you have already gone through the [Getting Started Guide](../gettingstarted/index.md) and have set up and run a stack at least once.

## Additional info

- Config Reference: [HTTP Auth](../reference/config.md#httpauth)
- [Auth plugin interface](https://github.com/hyperledger-firefly/common/blob/main/pkg/auth/plugin.go)
- [Basic auth plugin implementation](https://github.com/hyperledger-firefly/common/blob/main/pkg/auth/basic/basic_auth.go)

## Create a password file

FireFly's built in basic auth plugin uses a password hash file to store the list of authorized users. FireFly uses the bcrypt algorithm to compare passwords against the stored hash. You can use `htpasswd` on a command line to generate a hash file.

### Create the `test_users` password hash file

```
touch test_users
```

### Create a user named `firefly`

```
htpasswd -B test_users firefly
```

You will be prompted to type the password for the new user twice. **Optional**: You can continue to add new users by running this command with a different username.

```
htpasswd -B test_users <username>
```

## Enable basic auth at the Namespace level

To enable auth at the HTTP listener level we will need to edit the FireFly core config file. You can find the config file for the first node in your stack at the following path:

```
~/.firefly/stacks/<stack_name>/runtime/config/firefly_core_0.yml
```

Open the config file in your favorite editor and add the `auth` section to the `plugins` list:

```
plugins:
  auth:
  - name: test_user_auth
    type: basic
    basic:
      passwordfile: /etc/firefly/test_users
```

You will also need to add `test_user_auth` to the list of plugins used by the `default` namespace:

```
namespaces:
  predefined:
  - plugins:
    - database0
    - blockchain0
    - dataexchange0
    - sharedstorage0
    - erc20_erc721
    - test_user_auth
```

### Mount the password hash file in the Docker container

If you set up your FireFly stack using the FireFly CLI we will need to mount the password hash file in the Docker container, so that FireFly can actually read the file. This can be done by editing the `docker-compose.override.yml` file at:

```
~/.firefly/stacks/<stack_name>/docker-compose.override.yml
```

Edit the file to look like this, replacing the path to your `test_users` file:

```yaml
# Add custom config overrides here
# See https://docs.docker.com/compose/extends
version: "2.1"
services:
  firefly_core_0:
    volumes:
      - PATH_TO_YOUR_TEST_USERS_FILE:/etc/firefly/test_users
```

### Restart your FireFly Core container

To restart your FireFly stack and have Docker pick up the new volume, run:

```
ff stop <stack_name>
ff start <stack_name>
```

> **NOTE**: The FireFly basic auth plugin reads this file at startup and will not read it again during runtime. If you add any users or change passwords, restarting the node will be necessary to use an updated file.

### Test basic auth

After FireFly starts back up, you should be able to test that auth is working correctly by making an unauthenticated request to the API:

```
curl http://localhost:5000/api/v1/status
{"error":"FF00169: Unauthorized"}
```

However, if we add the username and password that we created above, the request should still work:

```
curl -u "firefly:firefly" http://localhost:5000/api/v1/status
{"namespace":{"name":"default","networkName":"default","description":"Default predefined namespace","created":"2022-10-18T16:35:57.603205507Z"},"node":{"name":"node_0","registered":false},"org":{"name":"org_0","registered":false},"plugins":{"blockchain":[{"name":"blockchain0","pluginType":"ethereum"}],"database":[{"name":"database0","pluginType":"sqlite3"}],"dataExchange":[{"name":"dataexchange0","pluginType":"ffdx"}],"events":[{"pluginType":"websockets"},{"pluginType":"webhooks"},{"pluginType":"system"}],"identity":[],"sharedStorage":[{"name":"sharedstorage0","pluginType":"ipfs"}],"tokens":[{"name":"erc20_erc721","pluginType":"fftokens"}]},"multiparty":{"enabled":true,"contract":{"active":{"index":0,"location":{"address":"0xa750e2647e24828f4fec2e6e6d61fc08ccca5efa"},"info":{"subscription":"sb-d0642f14-f89a-41bb-6fd4-ae74b9501b6c","version":2}}}}}
```

## Enable auth at the HTTP listener level

You may also want to enable auth at the HTTP listener level, for instance on the SPI (Service Provider Interface) to limit administrative actions. To enable auth at the HTTP listener level we will need to edit the FireFly core config file. You can find the config file for the first node in your stack at the following path:

```
~/.firefly/stacks/<stack_name>/runtime/config/firefly_core_0.yml
```

Open the config file in your favorite editor and change the `spi` section to look like the following:

```
spi:
  address: 0.0.0.0
  enabled: true
  port: 5101
  publicURL: http://127.0.0.1:5101
  auth:
    type: basic
    basic:
      passwordfile: /etc/firefly/test_users
```

### Restart FireFly to apply the changes

> **NOTE** You will need to mount the password hash file following the [instructions above](#mount-the-password-hash-file-in-the-docker-container) if you have not already.

You can run the following to restart your stack:

```
ff stop <stack_name>
ff start <stack_name>
```

### Test basic auth

After FireFly starts back up, you should be able to query the SPI and the request should be unauthorized.

```
curl http://127.0.0.1:5101/spi/v1/namespaces
{"error":"FF00169: Unauthorized"}
```

Adding the username and password that we set earlier, should make the request succeed.

```
curl -u "firefly:firefly" http://127.0.0.1:5101/spi/v1/namespaces
[{"name":"default","networkName":"default","description":"Default predefined namespace","created":"2022-10-18T16:35:57.603205507Z"}]
```
