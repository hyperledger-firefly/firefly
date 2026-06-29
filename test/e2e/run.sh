#!/bin/bash -x

set -o pipefail

if [[ ! -x `which jq` ]]; then echo "Please install \"jq\" to continue"; exit 1; fi

CWD=$(dirname "$0")
CLI="ff -v --ansi never"
CLI_VERSION=$(cat $CWD/../../manifest.json | jq -r .cli.tag)

create_accounts() {
  if [ "$TEST_SUITE" == "TestEthereumMultipartyE2ESuite" ]; then
      # Create 4 new accounts for use in testing
      for i in {1..5}
      do
          $CLI accounts create $STACK_NAME
      done
  elif [ "$TEST_SUITE" == "TestFabricMultipartyE2ESuite" ]; then
      # Create 4 new accounts for the first org for use in testing
      for i in {1..4}
      do
          $CLI accounts create $STACK_NAME $($CLI accounts list $STACK_NAME | jq --raw-output '.[0].orgName') user_$(openssl rand -hex 3)
      done
      # Create one account for the second org
      $CLI accounts create $STACK_NAME $($CLI accounts list $STACK_NAME | jq --raw-output '.[1].orgName') user_$(openssl rand -hex 3)
  fi
}

checkOk() {
  local rc=$1

  WORKDIR=${GITHUB_WORKSPACE}
  if [ -z "$WORKDIR" ]; then WORKDIR=.; fi

  mkdir -p "${WORKDIR}/containerlogs"
  $CLI logs $STACK_NAME > "${WORKDIR}/containerlogs/logs.txt"
  if [ $rc -eq 0 ]; then rc=$?; fi
  if [ $rc -ne 0 ]; then exit $rc; fi
}

STACK_NAME=${STACK_NAME:-firefly_e2e}
STACKS_DIR=${STACKS_DIR:-~/.firefly/stacks}
STACK_DIR=${STACK_DIR:-$STACKS_DIR/$STACK_NAME}

DOWNLOAD_CLI=${DOWNLOAD_CLI:-true}
CREATE_STACK=${CREATE_STACK:-true}
BUILD_FIREFLY=${BUILD_FIREFLY:-true}
MULTIPARTY_ENABLED=${MULTIPARTY_ENABLED:-true}

DATABASE_TYPE=${DATABASE_TYPE:-sqlite3}
STACK_TYPE=${STACK_TYPE:-ethereum}
TOKENS_PROVIDER=${TOKENS_PROVIDER:-erc20_erc721}

EXTRA_FLAGS=""
if [ -n "${BLOCKCHAIN_CONNECTOR}" ]; then
  EXTRA_FLAGS="${EXTRA_FLAGS} --blockchain-connector ${BLOCKCHAIN_CONNECTOR}"
fi

if [ "${STACK_TYPE}" != "fabric" ]; then
  if [ -n "${BLOCKCHAIN_NODE}" ]; then
    EXTRA_FLAGS="${EXTRA_FLAGS} --blockchain-node ${BLOCKCHAIN_NODE}"
  fi
fi

if [ "${TEST_SUITE}" == "TestFabricMultipartyCustomPinE2ESuite" ]; then
  # Special config for this one test suite
  EXTRA_FLAGS="${EXTRA_FLAGS} --custom-pin-support"
fi

# Pick a default suite if none was explicitly set
if [ -z "${TEST_SUITE}" ]; then
  if [ "${STACK_TYPE}" == "fabric" ]; then
    if [ "${MULTIPARTY_ENABLED}" == "true" ]; then
      TEST_SUITE=TestFabricMultipartyE2ESuite
    else
      TEST_SUITE=TestFabricGatewayE2ESuite
    fi
  else
    if [ "${MULTIPARTY_ENABLED}" == "true" ]; then
      TEST_SUITE=TestEthereumMultipartyE2ESuite
    else
      TEST_SUITE=TestEthereumGatewayE2ESuite
    fi
  fi
fi

cd $CWD

if [ "$BUILD_FIREFLY" == "true" ]; then
  make -C ../.. DOCKER_ARGS="--load" docker
  checkOk $?
fi

if [ "$DOWNLOAD_CLI" == "true" ]; then
  go install github.com/hyperledger-firefly/cli/ff@$CLI_VERSION
  checkOk $?
fi

if [ "$CREATE_STACK" == "true" ]; then
  $CLI remove -f $STACK_NAME
  $CLI init $STACK_TYPE --prometheus-enabled --database $DATABASE_TYPE $STACK_NAME 2 $EXTRA_FLAGS --token-providers $TOKENS_PROVIDER --manifest ../../manifest.json $EXTRA_INIT_ARGS --sandbox-enabled=false --multiparty=$MULTIPARTY_ENABLED
  checkOk $?

  $CLI pull $STACK_NAME -r 3
  checkOk $?

  $CLI start -b $STACK_NAME
  checkOk $?
fi

create_accounts

$CLI info $STACK_NAME
checkOk $?

export STACK_DIR

runTest() {
  go clean -testcache && go test -v -p 1 ./runners -run $TEST_SUITE
  checkOk $?
}
runTest

if [ "$RESTART" == "true" ]; then
  $CLI stop $STACK_NAME
  checkOk $?

  $CLI start $STACK_NAME
  checkOk $?

  create_accounts
  runTest
fi
