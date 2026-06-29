---
title: Setting up a FireFly Core Development Environment
---

# Setting up a FireFly Core Development Environment

This guide will walk you through setting up your machine for contributing to FireFly, specifically the FireFly core.

---

## Dependencies

You will need a few prerequisites set up on your machine before you can build FireFly from source. We recommend doing development on macOS, Linux, or WSL 2.0.

- [Go 1.22](https://golang.org/dl/)
- make
- GCC
- openssl

### Install the FireFly CLI

The first step to setting up a local development environment is to install the FireFly CLI. Please section of the [Getting Started Guide to install The FireFly CLI](../gettingstarted/firefly_cli.md).

### Installing Go and setting up your `GOPATH`

We recommend following the [instructions on golang.org](https://golang.org/doc/install) to install Go, rather than installing Go from another package magager such as `brew`. Although it is possible to install Go any way you'd like, setting up your `GOPATH` may differ from the following instructions.

After installing Go, you will need to add a few environment variables to your shell run commands file. This is usually a hidden file in your home directory called `.bashrc` or `.zshrc`, depending on which shell you're using.

Add the following lines to your `.bashrc` or `.zshrc` file:

```
export GOPATH=$HOME/go
export GOROOT="/usr/local/go"
export PATH="$PATH:${GOPATH}/bin:${GOROOT}/bin"
```

## Building FireFly

After installing dependencies, building FireFly from source is very easy. Just clone the repo:

```
git clone git@github.com:hyperledger-firefly/firefly.git && cd firefly
```

And run the `Makefile` to run tests, and compile the app

```
make
```

If you want to install the binary on your path (assuming your Go Home is already on your path), from inside the project directory you can simply run:

```
go install
```

## Install the CLI

Please check the CLI Installation instructions for the best way to install the CLI on your machine:
https://github.com/hyperledger-firefly/cli#install-the-cli

## Set up a development stack

Now that you have both FireFly and the FireFly CLI installed, it's time to create a development stack. The CLI can be used to create a docker-compose environment that runs the entirety of a FireFly network. This will include several different processes for each member of the network. This is very useful for people that want to build apps that use FireFly's API. It can also be useful if you want to make changes to FireFly itself, however we need to set up the stack slightly differently in that case.

Essentially what we are going to do is have docker-compose run everything in the FireFly network _except_ one FireFly core process. We'll run this FireFly core process on our host machine, and configure it to connect to the rest of the microservices running in docker-compose. This means we could launch FireFly from Visual Studio Code or some other IDE and use a debugger to see what's going on inside FireFly as it's running.

We'll call this stack `dev`. We're also going to add `--external 1` to the end of our command to create the new stack:

```
ff init dev --external 1
```

This tells the CLI that we want to manage one of the FireFly core processes outside the docker-compose stack. For convenience, the CLI will still generate a config file for this process though.

### Start the stack

To start your new stack simply run:

```
ff start dev
```

At a certain point in the startup process, the CLI will pause and wait for up to two minutes for you to start the other FireFly node. There are two different ways you can run the external FireFly core process.

### 1) From another terminal

The CLI will print out the command line which can be copied and pasted into another terminal window to run FireFly. _This command should be run from the `firefly` core project directory._ Here is an example of the command that the CLI will tell you to run:

```
firefly -f ~/.firefly/stacks/dev/runtime/config/firefly_core_0.yml
```

> **NOTE**: The first time you run FireFly with a fresh database, it will need a directory of database migrations to apply to the empty database. If you run FireFly from the `firefly` project directory you cloned from GitHub, it will automatically find these and apply them. If you run it from some other directory, you will have to point FireFly to the migrations on your own.

### 2) Using an IDE

If you named your stack `dev` there is a `launch.json` file for Visual Studio code already in the project directory. If you have the project open in Visual Studio Code, you can either press the F5 key to run it, or go to the "Run and Debug" view in Visual Studio code, and click "Run FireFly Core".

![Launch config](../images/launch_config.png "Launch config")

Now you should have a full FireFly stack up and running, and be able to debug FireFly using your IDE. Happy hacking!

> **NOTE**: Because `firefly-ui` is a separate repo, unless you also start a UI dev server for the external FireFly core, the default UI path will not load. This is expected, and if you're just working on FireFly core itself, you don't need to worry about it.`

## Set up dev environment for other components

Refer to [Advanced CLI Usage](./advanced_cli_usage.md).
