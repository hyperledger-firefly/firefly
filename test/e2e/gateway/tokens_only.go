// Copyright © 2023 Kaleido, Inc.
//
// SPDX-License-Identifier: Apache-2.0
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package gateway

import (
	"fmt"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/test/e2e"
	"github.com/hyperledger-firefly/firefly/test/e2e/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type TokensOnlyTestSuite struct {
	suite.Suite
	testState *testState
	connector string
	db        string
	key       string
}

func (suite *TokensOnlyTestSuite) SetupSuite() {
	suite.testState = beforeE2ETest(suite.T())
	stack := e2e.ReadStack(suite.T())
	stackState := e2e.ReadStackState(suite.T())

	suite.connector = stack.TokenProviders[0]
	suite.db = stack.Database
	account := stackState.Accounts[0].(map[string]interface{})
	suite.key = account["address"].(string)
}

func (suite *TokensOnlyTestSuite) BeforeTest(suiteName, testName string) {
	suite.testState = beforeE2ETest(suite.T())
}

func (suite *TokensOnlyTestSuite) AfterTest(suiteName, testName string) {
	e2e.VerifyAllOperationsSucceeded(suite.T(), []*client.FireFlyClient{suite.testState.client1}, suite.testState.startTime)
}

func (suite *TokensOnlyTestSuite) TestTokensOnlyNamespaces() {
	defer suite.testState.Done()
	testNamespace := e2e.RandomName(suite.T())
	suite.T().Logf("Test namespace: %s", testNamespace)

	namespaceInfo := map[string]interface{}{
		"name": testNamespace,
		"asset": map[string]interface{}{
			"manager": map[string]interface{}{
				"keyNormalization": "none",
			},
		},
		"plugins": []string{"database0", suite.connector},
	}

	// Add the new namespace
	data1 := e2e.ReadConfig(suite.T(), suite.testState.configFile1)
	e2e.AddPluginBroadcastName(data1, "tokens", "testremote")
	e2e.AddNamespace(data1, namespaceInfo)
	e2e.WriteConfig(suite.T(), suite.testState.configFile1, data1)

	admin1 := client.NewResty(suite.T())
	admin1.SetBaseURL(suite.testState.adminHost1 + "/spi/v1")

	client1 := client.NewFireFly(suite.T(), suite.testState.client1.Hostname, testNamespace)

	e2e.ResetFireFly(suite.T(), admin1)
	e2e.PollForUp(suite.T(), client1)

	eventNames := "token_pool_confirmed|token_transfer_confirmed"
	queryString := fmt.Sprintf("namespace=%s&ephemeral&autoack&filter.events=%s", testNamespace, eventNames)
	received1 := e2e.WsReader(client1.WebSocket(suite.T(), queryString, nil))

	// Attempt async token operations on new namespace
	poolName := fmt.Sprintf("pool_%s", e2e.RandomName(suite.T()))
	suite.T().Logf("Pool name: %s", poolName)

	pool := &core.TokenPool{
		Name: poolName,
		Key:  suite.key,
		Type: core.TokenTypeFungible,
	}
	poolResp := client1.CreateTokenPool(suite.T(), pool, false, false)
	poolID := poolResp.ID

	e2e.WaitForEvent(suite.T(), received1, core.EventTypePoolConfirmed, poolID)
	pools := client1.GetTokenPools(suite.T(), suite.testState.startTime)
	assert.Equal(suite.T(), 1, len(pools))
	assert.Equal(suite.T(), testNamespace, pools[0].Namespace)
	assert.Equal(suite.T(), suite.connector, pools[0].Connector)
	assert.Equal(suite.T(), poolName, pools[0].Name)
	assert.Equal(suite.T(), core.TokenTypeFungible, pools[0].Type)
	assert.NotEmpty(suite.T(), pools[0].Locator)

	transfer := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{Amount: *fftypes.NewFFBigInt(1), Key: suite.key},
		Pool:          poolName,
	}
	transferOut := client1.MintTokens(suite.T(), transfer, false)
	e2e.WaitForEvent(suite.T(), received1, core.EventTypeTransferConfirmed, transferOut.LocalID)
	e2e.ValidateAccountBalances(suite.T(), client1, poolID, "", map[string]int64{
		suite.key: 1,
	})

	transfer = &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{Amount: *fftypes.NewFFBigInt(1), Key: suite.key},
		Pool:          poolName,
	}
	transferOut = client1.BurnTokens(suite.T(), transfer, false)
	e2e.WaitForEvent(suite.T(), received1, core.EventTypeTransferConfirmed, transferOut.LocalID)
	e2e.ValidateAccountBalances(suite.T(), client1, poolID, "", map[string]int64{
		suite.key: 0,
	})
}
