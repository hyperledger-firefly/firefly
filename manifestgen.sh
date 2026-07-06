#!/bin/bash

# Copyright © 2022 Kaleido, Inc.
#
# SPDX-License-Identifier: Apache-2.0
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# This script will automatically update the manifest.json file with the
# latest releases of all FireFly microservice dependencies

 if [[ ! -x `which jq` ]]; then echo "Please install \"jq\" to continue"; exit 1; fi
 
USE_HEAD=false

# If you pass the string "head" as an argument to this script, it will 
# get the latest build, straight off the main branch. Otherwise it will
# get the most recent release that is not a pre-release.
if [[ $# -eq 1 ]] ; then
    if [[ $1 -eq "head" ]] ; then
        echo 'using head'
        USE_HEAD=true
    fi
fi

# These sections are currently not pulled from GitHub automatically
# so these sections are copied over from the existing file
BUILD_SECTION=$(cat manifest.json | jq .build)
UI_SECTION=$(cat manifest.json | jq .ui)
CLI_SECTION=$(cat manifest.json | jq .cli)

rm -f manifest.json

function repository_url() {
    service=$1
    case $service in
        cardano*)
            echo "https://api.github.com/repos/hyperledger/firefly-cardano"
            ;;
        *)
            echo "https://api.github.com/repos/hyperledger/firefly-$service"
            ;;
    esac
}

SERVICES=(
    "cardanoconnect"
    "cardanosigner"
    "ethconnect"
    "evmconnect"
    "fabconnect"
    "tezosconnect"
    "dataexchange-https"
    "tokens-erc1155"
    "tokens-erc20-erc721"
    "signer"
)
SERVICE_COUNT=${#SERVICES[@]}

echo "{" >> manifest.json

for (( i=0; i<${SERVICE_COUNT}; i++ ))
do
    echo "  \"${SERVICES[$i]}\": {" >> manifest.json
    echo "    \"image\": \"ghcr.io/hyperledger/firefly-${SERVICES[$i]}\"," >> manifest.json

    if [ $USE_HEAD = false ] ; then
        # Query GitHub API the latest release version
        TAG=$(curl "$(repository_url "${SERVICES[$i]}")/releases/latest" -s | jq .tag_name -r)
    else
        # Otherwise, pull the newest built image straight off the main branch
        TAG="head"
    fi
    
    # Attempt to pull the image from GitHub Container Repository
    docker pull ghcr.io/hyperledger/firefly-${SERVICES[$i]}:$TAG
    # Get the SHA of the downloaded image
    SHA=$(docker inspect --format='{{index .RepoDigests 0}}' ghcr.io/hyperledger/firefly-${SERVICES[$i]}:$TAG | cut -d ':' -f 2)
    # Get the tag / build number name of this image from its label
    TAG_LABEL=$(docker inspect --format='{{index .Config.Labels "tag"}}' ghcr.io/hyperledger/firefly-${SERVICES[$i]}:$TAG)

    # If the tag / build number wasn't set in the label, use whatever docker tag we fetched
    # This is done for backwards compatability, because not all images have labels yet
    if [ -z "$TAG_LABEL" ] || [ "$TAG_LABEL" == "null" ]; then
        TAG_LABEL=$TAG
    fi

    echo "    \"tag\": \"$TAG_LABEL\"," >> manifest.json
    echo "    \"sha\": \"$SHA\"" >> manifest.json
    echo "  }," >> manifest.json
done

# Add the build and UI sections, piping to sed to get proper indentation
echo "\"build\": $BUILD_SECTION," | sed 's/^/  /' >> manifest.json
echo "\"ui\": $UI_SECTION," | sed 's/^/  /' >> manifest.json
echo "\"cli\": $CLI_SECTION" | sed 's/^/  /' >> manifest.json

echo "}" >> manifest.json