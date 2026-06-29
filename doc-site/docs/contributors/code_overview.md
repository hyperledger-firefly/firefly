---
title: Code Overview
---

# FireFly Code Overview

## Developer Intro

FireFly is a second generation implementation re-engineered from the ground up to improve developer experience, runtime performance, and extensibility.

This means a simplified REST/WebSocket programming model for app development, and a wider range of infrastructure options for deployment.

It also means a focus on an architecture and code structure for a vibrant open source community.

A few highlights:

- Golang codebase
  - Strong coding standards, including unit test coverage, translation support, logging and more
  - Fast starting, low memory footprint, multi-threaded runtime
- OpenAPI 3.0 API specification (Swagger)
  - Generated from the API router code, to avoid divergence with the implementation
- Active/active HA architecture for the core runtime
  - Deferring to the core database for state high availability
  - Exploiting leader election where required
- Fully pluggable architecture
  - Everything from Database through to Blockchain, and Compute
  - Golang plugin infrastructure to decouple the core code from the implementation
  - Remote Agent model to decouple code languages, and HA designs
- Updated API resource model
  - `Asset`, `Data`, `Message`, `Event`, `Topic`, `Transaction`
- Added flexibility, with simplified the developer experience:

  - Versioning of data definitions
  - Introducing a first class `Context` construct link related events into a single sequence
  - Allow many pieces of data to be attached to a single message, and be automatically re-assembled on arrival
  - Clearer separation of concerns between the FireFly DB and the Application DB
  - Better search, filter and query support

  ## Directories

- [internal](https://github.com/hyperledger-firefly/firefly/tree/main/internal): The core Golang implementation code
- [pkg](https://github.com/hyperledger-firefly/firefly/tree/main/pkg): Interfaces intended for external project use
- [cmd](https://github.com/hyperledger-firefly/firefly/tree/main/cmd): The command line entry point
- [smart_contracts](https://github.com/hyperledger-firefly/firefly/tree/main/smart_contracts): smart contract code for FireFly's onchain logic
