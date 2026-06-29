# FireFly configuration tool

A tool for managing and migrating config files for Hyperledger FireFly.

## Installation

If you have a local Go development environment, and you have included `${GOPATH}/bin` in your path, you can install with:

```sh
go install github.com/hyperledger-firefly/firefly/ffconfig@latest
```

## Usage

### Migration

Parse a config file to find any deprecated items that should be updated to the latest config syntax.
The updated config will be written to stdout (or a file specified with `-o`).
```
ffconfig migrate -f firefly.core.yml [-o new.yml]
```

You may optionally specify `--from` and `--to` versions to run a subset of the migrations.

View the source code for all current migrations at [migrate/migrations.go](migrate/migrations.go).
