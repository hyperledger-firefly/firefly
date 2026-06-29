---
title: Private data exchange
---

## Introduction

Private data exchange is the way most enterprise business-to-business communication
happens today. One party privately sends data to another, over a pipe that has been
agreed as sufficiently secure between the two parties. That might be a REST API,
SOAP Web Service, FTP / EDI, Message Queue (MQ), or other B2B Gateway technology.

![Multi-party Systems](../../images/multiparty_system1.png "Multi-Party System")

The ability to perform these same private data exchanges within
a multi-party system is critical. In fact it's common for the majority of business
data continue to transfer over such interfaces.

So real-time application to application private messaging, and private
transfer of large blobs/documents, are first class constructs in the FireFly API.

## Qualities of service

FireFly recognizes that a multi-party system will need to establish a secure messaging
backbone, with the right qualities of service for their requirements. So the implementation
is pluggable, and the plugin interface embraces the following quality of service
characteristics that differ between different implementations.

- Transport Encryption
  - Technologies like TLS encrypt data while it is in flight, so that it cannot be
    sniffed by a third party that has access to the underlying network.
- Authentication
  - There are many technologies including Mutual TLS, and Java Web Tokens (JWT),
    that can be used to ensure a private data exchange is happening with the
    correct party in the system.
  - Most modern approaches use public/private key encryption to establish the identity
    during the setup phase of a connection. This means a distribution mechanism is required
    for public keys, which might be enhanced with a trust hierarchy (like PKI).
- Request/Response (Sync) vs. Message Queuing (Async)
  - Synchronous transports like HTTPS require both parties to be available at the
    time data is sent, and the transmission must be retried at the application (plugin)
    layer if it fails or times out.
  - Asynchronous transports like AMQP, MQTT or Kafka introduce one or more broker runtimes
    between the parties, that reliably buffer the communications if the target application
    falls behind or is temporarily unavailable.
- Hub & spoke vs. Peer to peer
  - Connectivity might be direct from one party to another within the network, tackling
    the IT security complexity of firewalls between sensitive networks. Or network shared
    infrastructure / as-a-service provider might be used to provide a reliable backbone
    for data exchange between the members.
- End-to-end Payload Encryption
  - Particularly in cases where the networking hops are complex, or involve shared
    shared/third-party infrastructure, end-to-end encryption can be used to additionally
    protect the data while in flight. This technology means data remains encrypted
    from the source to the target, regardless of the number of transport hops taken in-between.
- Large blob / Managed file transfer
  - The optimal approach to transferring real-time small messages (KBs in size) is different
    to the approach to transferring large blobs (MBs/GBs in size). For large blobs chunking,
    compression, and checkpoint restart are common for efficient and reliable transfer.

## FireFly OSS implementation

A reference implementation of a private data exchange is provided as part of the FireFly
project. This implementation uses peer-to-peer transfer over a synchronous HTTPS transport,
backed by Mutual TLS authentication. X509 certificate exchange is orchestrated by FireFly,
such that self-signed certificates can be used (or multiple PKI trust roots) and bound to
the blockchain-backed identities of the organizations in FireFly.

See [hyperledger-firefly/dataexchange-https](https://github.com/hyperledger-firefly/dataexchange-https)
