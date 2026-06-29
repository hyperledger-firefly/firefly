---
title: Advanced CLI Usage
---

This page details some of the more advanced options of the FireFly CLI

## Understanding how the CLI uses FireFly releases

## The `manifest.json` file

FireFly has a [`manifest.json` file in the root of the repo](https://github.com/hyperledger-firefly/firefly/blob/main/manifest.json). This file contains a list of versions (both tag and sha) for each of the microservices that should be used with this specific commit.

Here is an example of what the `manifest.json` looks like:

```json
{
  "ethconnect": {
    "image": "ghcr.io/hyperledger-firefly/ethconnect",
    "tag": "v3.0.4",
    "sha": "0b7ce0fb175b5910f401ff576ced809fe6f0b83894277c1cc86a73a2d61c6f41"
  },
  "fabconnect": {
    "image": "ghcr.io/hyperledger-firefly/fabconnect",
    "tag": "v0.9.0",
    "sha": "a79a4c66b0a2551d5122d019c15c6426e8cdadd6566ce3cbcb36e008fb7861ca"
  },
  "dataexchange-https": {
    "image": "ghcr.io/hyperledger-firefly/dataexchange-https",
    "tag": "v0.9.0",
    "sha": "0de5b1db891a02871505ba5e0507821416d9fa93c96ccb4b1ba2fac45eb37214"
  },
  "tokens-erc1155": {
    "image": "ghcr.io/hyperledger-firefly/tokens-erc1155",
    "tag": "v0.9.0-20211019-01",
    "sha": "aabc6c483db408896838329dab5f4b9e3c16d1e9fa9fffdb7e1ff05b7b2bbdd4"
  }
}
```

## Default CLI behavior for releases

When creating a new stack, the CLI will by default, check the latest non-pre-release version of FireFly and look at its `manifest.json` file that was part of that commit. It will then use the Docker images referenced in that file to determine which images it should pull for the new stack. The specific image tag and sha is written to the `docker-compose.yml` file for that stack, so restarting or resetting a stack will never pull a newer image.

## Running a specific release of FireFly

If you need to run some other version that is not the latest release of FireFly, you can tell the FireFly CLI which release to use by using the `--release` or `-r` flag. For example, to explicitly use `v0.9.0` run this command to initialize the stack:

```
ff init -r v0.9.0
```

## Running an unreleased version of one or more services

If you need to run an unreleased version of FireFly or one of its microservices, you can point the CLI to a specific `manifest.json` on your local disk. To do this, use the `--manifest` or `-m` flag. For example, if you have a file at `~/firefly/manifest.json`:

```
ff init -m ~/firefly/manifest.json
```

If you need to test a locally built docker image of a specific service, you'll want to edit the `manifest.json` before running `ff init`. Let's look at an example where we want to run a locally built version of fabconnect. The same steps apply to any of FireFly's microservices.

### Build a new image of fabconnect locally

From the fabconnect project directory, build and tag a new Docker image:

```
docker build -t ghcr.io/hyperledger-firefly/fabconnect .
```

### Edit your `manifest.json` file

Next, edit the `fabconnect` section of the `manifest.json` file. You'll want to remove the `tag` and `sha` and a `"local": true` field, so it looks like this:

```json
...
  "fabconnect": {
    "image": "ghcr.io/hyperledger-firefly/fabconnect",
    "local": true
  },
...
```

### Initialize the stack with the custom `manifest.json` file

```
 ff init local-test -b fabric -m ~/Code/hyperledger-firefly/firefly/manifest.json
 ff start local-test
```

If you are iterating on changes locally, you can get the CLI to use an updated image by doing the following:

- Whenever the CLI does its first time setup, it will use your newly built local docker image
- If you've already run the stack, you can run `ff reset <stack_name>` and `ff start <stack_name>` to reset the data, and use the newer image

## Running a locally built FireFly Core image

You may have noticed that FireFly core is actually not listed in the `manifest.json file`. If you want to run a locally built image of FireFly Core, you can follow the same steps above, but instead of editing an existing section in the file, we'll add a new one for FireFly.

### Build a new image of FireFly locally

From the firefly project directory, build and tag a new Docker image:

```
make docker
```

### Initialize the stack with the custom `manifest.json` file

```
 ff init local-test -m ~/Code/hyperledger-firefly/firefly/manifest.json
 ff start local-test
```
