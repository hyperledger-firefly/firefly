---
title: Fabric Test Network
---

# Work with Fabric-Samples Test Network

This guide will walk you through the steps to create a local FireFly development environment and connect it to the Fabric Test Network from the [Fabric Samples repo](https://github.com/hyperledger/fabric-samples)

## Previous steps: Install the FireFly CLI

If you haven't set up the FireFly CLI already, please go back to the Getting Started guide and read the section on how to [Install the FireFly CLI](../../gettingstarted/firefly_cli.md).

[← ① Install the FireFly CLI](../../gettingstarted/firefly_cli.md){: .md-button .md-button--primary}

## Start Fabric Test Network with Fabric CA

For details about the Fabric Test Network and how to set it up, please see the [Fabric Samples repo](https://github.com/hyperledger/fabric-samples/tree/main/test-network). The one important detail is that you need to start up the Test Network with a Fabric CA. This is because Fabconnect will use the Fabric CA to create an identity for its FireFly node to use. To start up the network with the CA, and create a new channel called `mychannel` run:

```
./network.sh up createChannel -ca
```

> **NOTE**: If you already have the Test Network running, you will need to bring it down first, by running: `./network.sh down`

## Deploy FireFly Chaincode

Next we will need to package and deploy the FireFly chaincode to `mychannel` in our new network. For more details on packaging and deploying chaincode, please see the [Fabric chaincode lifecycle documentation](https://hyperledger-fabric.readthedocs.io/en/latest/chaincode_lifecycle.md). If you already have the [FireFly repo](https://github.com/hyperledger-firefly/firefly) cloned in the same directory as your `fabric-samples` repo, you can run the following script from your `test-network` directory:

> **NOTE**: This script is provided as a convenience only, and you are not required to use it. You are welcome to package and deploy the chaincode to your test-network any way you would like.

```bash
#!/bin/bash

# This file should be run from the test-network directory in the fabric-samples repo
# It also assumes that you have the firefly repo checked out at the same level as the fabric-samples directory
# It also assumes that the test-network is up and running and a channel named 'mychannel' has already been created

cd ../../firefly/smart_contracts/fabric/firefly-go
GO111MODULE=on go mod vendor
cd ../../../../fabric-samples/test-network

export PATH=${PWD}/../bin:$PATH
export FABRIC_CFG_PATH=$PWD/../config/

peer lifecycle chaincode package firefly.tar.gz --path ../../firefly/smart_contracts/fabric/firefly-go --lang golang --label firefly_1.0

export CORE_PEER_TLS_ENABLED=true
export CORE_PEER_LOCALMSPID="Org1MSP"
export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
export CORE_PEER_ADDRESS=localhost:7051

peer lifecycle chaincode install firefly.tar.gz

export CORE_PEER_LOCALMSPID="Org2MSP"
export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt
export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp
export CORE_PEER_ADDRESS=localhost:9051

peer lifecycle chaincode install firefly.tar.gz

export CC_PACKAGE_ID=$(peer lifecycle chaincode queryinstalled --output json | jq --raw-output ".installed_chaincodes[0].package_id")

peer lifecycle chaincode approveformyorg -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --channelID mychannel --name firefly --version 1.0 --package-id $CC_PACKAGE_ID --sequence 1 --tls --cafile "${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem"

export CORE_PEER_LOCALMSPID="Org1MSP"
export CORE_PEER_MSPCONFIGPATH=${PWD}/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp
export CORE_PEER_TLS_ROOTCERT_FILE=${PWD}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt
export CORE_PEER_ADDRESS=localhost:7051

peer lifecycle chaincode approveformyorg -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --channelID mychannel --name firefly --version 1.0 --package-id $CC_PACKAGE_ID --sequence 1 --tls --cafile "${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem"

peer lifecycle chaincode commit -o localhost:7050 --ordererTLSHostnameOverride orderer.example.com --channelID mychannel --name firefly --version 1.0 --sequence 1 --tls --cafile "${PWD}/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/msp/tlscacerts/tlsca.example.com-cert.pem" --peerAddresses localhost:7051 --tlsRootCertFiles "${PWD}/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/ca.crt" --peerAddresses localhost:9051 --tlsRootCertFiles "${PWD}/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/ca.crt"
```

## Create `ccp.yml` documents

Each FireFly Supernode (specifically the Fabconnect instance in each) will need to know how to connect to the Fabric network. Fabconnect will use a [Fabric Connection Profile](https://hyperledger-fabric.readthedocs.io/en/release-2.2/developapps/connectionprofile.md) which describes the network and tells it where the certs and keys are that it needs. Below is a `ccp.yml` for each organization. You will need to fill in one line by replacing the string `FILL_IN_KEY_NAME_HERE`, because the file name of the private key for each user is randomly generated.

### Organization 1 connection profile

Create a new file at `~/org1_ccp.yml` with the contents below. Replace the string `FILL_IN_KEY_NAME_HERE` with the filename in your `fabric-samples/test-network/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp/keystore` directory.

```yml
certificateAuthorities:
  org1.example.com:
    tlsCACerts:
      path: /etc/firefly/organizations/peerOrganizations/org1.example.com/msp/tlscacerts/ca.crt
    url: https://ca_org1:7054
    grpcOptions:
      ssl-target-name-override: org1.example.com
    registrar:
      enrollId: admin
      enrollSecret: adminpw
channels:
  mychannel:
    orderers:
      - fabric_orderer
    peers:
      fabric_peer:
        chaincodeQuery: true
        endorsingPeer: true
        eventSource: true
        ledgerQuery: true
client:
  BCCSP:
    security:
      default:
        provider: SW
      enabled: true
      hashAlgorithm: SHA2
      level: 256
      softVerify: true
  credentialStore:
    cryptoStore:
      path: /etc/firefly/organizations/peerOrganizations/org1.example.com/msp
    path: /etc/firefly/organizations/peerOrganizations/org1.example.com/msp
  cryptoconfig:
    path: /etc/firefly/organizations/peerOrganizations/org1.example.com/msp
  logging:
    level: info
  organization: org1.example.com
  tlsCerts:
    client:
      cert:
        path: /etc/firefly/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp/signcerts/cert.pem
      key:
        path: /etc/firefly/organizations/peerOrganizations/org1.example.com/users/Admin@org1.example.com/msp/keystore/FILL_IN_KEY_NAME_HERE
orderers:
  fabric_orderer:
    tlsCACerts:
      path: /etc/firefly/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/tls/tlscacerts/tls-localhost-9054-ca-orderer.pem
    url: grpcs://orderer.example.com:7050
organizations:
  org1.example.com:
    certificateAuthorities:
      - org1.example.com
    cryptoPath: /tmp/msp
    mspid: Org1MSP
    peers:
      - fabric_peer
peers:
  fabric_peer:
    tlsCACerts:
      path: /etc/firefly/organizations/peerOrganizations/org1.example.com/peers/peer0.org1.example.com/tls/tlscacerts/tls-localhost-7054-ca-org1.pem
    url: grpcs://peer0.org1.example.com:7051
version: 1.1.0%
```

### Organization 2 connection profile

Create a new file at `~/org2_ccp.yml` with the contents below. Replace the string `FILL_IN_KEY_NAME_HERE` with the filename in your `fabric-samples/test-network/organizations/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp/keystore` directory.

```yml
certificateAuthorities:
  org2.example.com:
    tlsCACerts:
      path: /etc/firefly/organizations/peerOrganizations/org2.example.com/msp/tlscacerts/ca.crt
    url: https://ca_org2:8054
    grpcOptions:
      ssl-target-name-override: org2.example.com
    registrar:
      enrollId: admin
      enrollSecret: adminpw
channels:
  mychannel:
    orderers:
      - fabric_orderer
    peers:
      fabric_peer:
        chaincodeQuery: true
        endorsingPeer: true
        eventSource: true
        ledgerQuery: true
client:
  BCCSP:
    security:
      default:
        provider: SW
      enabled: true
      hashAlgorithm: SHA2
      level: 256
      softVerify: true
  credentialStore:
    cryptoStore:
      path: /etc/firefly/organizations/peerOrganizations/org2.example.com/msp
    path: /etc/firefly/organizations/peerOrganizations/org2.example.com/msp
  cryptoconfig:
    path: /etc/firefly/organizations/peerOrganizations/org2.example.com/msp
  logging:
    level: info
  organization: org2.example.com
  tlsCerts:
    client:
      cert:
        path: /etc/firefly/organizations/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp/signcerts/cert.pem
      key:
        path: /etc/firefly/organizations/peerOrganizations/org2.example.com/users/Admin@org2.example.com/msp/keystore/FILL_IN_KEY_NAME_HERE
orderers:
  fabric_orderer:
    tlsCACerts:
      path: /etc/firefly/organizations/ordererOrganizations/example.com/orderers/orderer.example.com/tls/tlscacerts/tls-localhost-9054-ca-orderer.pem
    url: grpcs://orderer.example.com:7050
organizations:
  org2.example.com:
    certificateAuthorities:
      - org2.example.com
    cryptoPath: /tmp/msp
    mspid: Org2MSP
    peers:
      - fabric_peer
peers:
  fabric_peer:
    tlsCACerts:
      path: /etc/firefly/organizations/peerOrganizations/org2.example.com/peers/peer0.org2.example.com/tls/tlscacerts/tls-localhost-8054-ca-org2.pem
    url: grpcs://peer0.org2.example.com:9051
version: 1.1.0%
```

## Create the FireFly stack

Now we can create a FireFly stack and pass in these files as command line flags.

> **NOTE**: The following command should be run in the `test-network` directory as it includes a relative path to the `organizations` directory containing each org's MSP.

```
ff init fabric dev \
  --ccp "${HOME}/org1_ccp.yml" \
  --msp "organizations" \
  --ccp "${HOME}/org2_ccp.yml" \
  --msp "organizations" \
  --channel mychannel \
  --chaincode firefly
```

## Edit `docker-compose.override.yml`

The last step before starting up FireFly is to make sure that our FireFly containers have networking access to the Fabric containers. Because these are in two different Docker Compose networks by default, normally the containers would not be able to connect directly. We can fix this by instructing Docker to also attach our FireFly containers to the Fabric test network Docker Compose network. The easiest way to do that is to edit `~/.firefly/stacks/dev/docker-compose.override.yml` and set its contents to the following:

```yml
# Add custom config overrides here
# See https://docs.docker.com/compose/extends
version: "2.1"
networks:
  default:
    name: fabric_test
    external: true
```

## Start FireFly stack

Now we can start up FireFly!

```
ff start dev
```

After everything starts up, you should have two FireFly nodes that are each mapped to an Organization in your Fabric network. You can that they each use separate signing keys for their Org on messages that each FireFly node sends.

## Connecting to a remote Fabric Network

This same guide can be adapted to connect to a remote Fabric network running somewhere else. They key takeaways are:

- You need the FireFly chaincode deployed on channel in your Fabric network
- You need to pass the channel and chaincode name when you run `ff init`
- You need to provide a connection profile and the correct certs, keys, etc. for each node when you run `ff init`
- Your FireFly containers will need to have network access to your Fabric network

## Troubleshooting

There are quite a few moving parts in this guide and if steps are missed or done out of order it can cause problems. Below are some of the common situations that you might run into while following this guide, and solutions for each.

You may see a message something along the lines of:

```
ERROR: for firefly_core_0  Container "bc04521372aa" is unhealthy.
Encountered errors while bringing up the project.
```

In this case, we need to look at the container logs to get more detail about what happened. To do this, we can run `ff start` and tell it not to clean up the stack after the failure, to let you inspect what went wrong. To do that, you can run:

```
ff start dev --verbose --no-rollback
```

Then we could run `docker logs <container_name>` to see the logs for that container.

### No such host

```
Error: http://127.0.0.1:5102/identities [500] {"error":"enroll failed: enroll failed: POST failure of request: POST https://ca_org1:7054/enroll\n{\"hosts\":null,\"certificate_request\":\"-----BEGIN CERTIFICATE REQUEST-----\\nMIH0MIGcAgEAMBAxDjAMBgNVBAMTBWFkbWluMFkwEwYHKoZIzj0CAQYIKoZIzj0D\\nAQcDQgAE7qJZ5nGt/kxU9IvrEb7EmgNIgn9xXoQUJLl1+U9nXdWB9cnxcmoitnvy\\nYN63kbBuUh0z21vOmO8GLD3QxaRaD6AqMCgGCSqGSIb3DQEJDjEbMBkwFwYDVR0R\\nBBAwDoIMMGQ4NGJhZWIwZGY0MAoGCCqGSM49BAMCA0cAMEQCIBcWb127dVxm/80K\\nB2LtenAY/Jtb2FbZczolrXNCKq+LAiAcGEJ6Mx8LVaPzuSP4uGpEoty6+bEErc5r\\nHVER+0aXiQ==\\n-----END CERTIFICATE REQUEST-----\\n\",\"profile\":\"\",\"crl_override\":\"\",\"label\":\"\",\"NotBefore\":\"0001-01-01T00:00:00Z\",\"NotAfter\":\"0001-01-01T00:00:00Z\",\"ReturnPrecert\":false,\"CAName\":\"\"}: Post \"https://ca_org1:7054/enroll\": dial tcp: lookup ca_org1 on 127.0.0.11:53: no such host"}
```

If you see something in your logs that looks like the above, there could be a couple issues:

1. The hostname for one of your Fabric containers could be wrong in the `ccp.yml`. Check the `ccp.yml` for that member and make sure the hostnames are correct.
1. The FireFly container doesn't have networking connectivity to the Fabric containers. Check the `docker-compose.override.yml` file to make sure you added the `fabric_test` network as instructed above.

### No such file or directory

```
User credentials store creation failed. Failed to load identity configurations: failed to create identity config from backends: failed to load client TLSConfig : failed to load client key: failed to load pem bytes from path /etc/firefly/organizations/peerOrganizations/org1.example.com/users/User1@org1.example.com/msp/keystore/cfc50311e2204f232cfdfaf4eba7731279f2366ec291ca1c1781e2bf7bc75529_sk: open /etc/firefly/organizations/peerOrganizations/org1.example.com/users/User1@org1.example.com/msp/keystore/cfc50311e2204f232cfdfaf4eba7731279f2366ec291ca1c1781e2bf7bc75529_sk: no such file or directory
```

If you see something in your logs that looks like the above, it's likely that your private key file name is not correct in your `ccp.yml` file for that particular member. Check your `ccp.yml` and make sure all the files listed there exist in your `organizations` directory.
