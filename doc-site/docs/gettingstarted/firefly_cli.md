---
title: ① Install the FireFly CLI
---

# Install the FireFly CLI

## Prerequisites

In order to run the FireFly CLI, you will need a few things installed on your dev machine:

- [Docker](https://www.docker.com/)
- [Docker Compose](https://docs.docker.com/compose/)
- openssl

### Linux Users

> **NOTE**: For Linux users, it is recommended that you add your user to the `docker` group so that you do not have to run `ff` or `docker` as `root` or with `sudo`. For more information about Docker permissions on Linux, please see [Docker's documentation on the topic](https://docs.docker.com/engine/install/linux-postinstall/).

### Windows Users

> **NOTE**: For Windows users, we recommend that you use [Windows Subsystem for Linux 2 (WSL2)](https://docs.microsoft.com/en-us/windows/wsl/). Binaries provided for Linux will work in this environment.

## Install the CLI

There are several ways to install the FireFly CLI. The easiest way to get up and running with the FireFly CLI is to download a pre-compiled binary of the latest release.

### Install via Binary Package Download

Download the package for your OS by navigating to the [latest release page](https://github.com/hyperledger-firefly/cli/releases/latest) and downloading the appropriate package for your OS and architecture.

#### Unpack and Install the Binary


Assuming you downloaded the package from GitHub into your `Downloads` directory, run the following command to extract the binary and move it to your system path:


```
sudo tar -zxf ~/Downloads/firefly-cli_*.tar.gz -C /usr/local/bin ff && rm ~/Downloads/firefly-cli_*.tar.gz
```

If you downloaded the package into a different directory, adjust the command to point to the correct location of the `firefly-cli_*.tar.gz` file.

#### macOSUsers

> **NOTE**: On recent versions of macOS, default security settings will prevent the FireFly CLI binary from running, because it was downloaded from the internet. You will need to [allow the FireFly CLI in System Preferences](https://github.com/hyperledger-firefly/cli/blob/main/docs/mac_help.md), before it will run.

### Install via Homebrew (macOS)

You can also install the FireFly CLI using Homebrew:

```
brew install firefly
```
 
### Alternative installation method: Install via Go

If you have a local Go development environment, and you have included `${GOPATH}/bin` in your path, you could also use Go to install the FireFly CLI by running:

```sh
go install github.com/hyperledger-firefly/cli/ff@latest
```

## Verify the installation

After using either installation method above, you can verify that the CLI is successfully installed by running `ff version`. This should print the current version like this:

```
{
  "Version": "v0.0.47",
  "License": "Apache-2.0"
}
```

## Next steps: Start your environment

Now that you've got the FireFly CLI set up on your machine, the next step is to create and start a FireFly stack.

[② Start your environment →](setup_env.md){: .md-button .md-button--primary}
