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

type TokensTestSuite struct {
	suite.Suite
	testState *testState
	connector string
	key       string
}

func (suite *TokensTestSuite) SetupSuite() {
	suite.testState = beforeE2ETest(suite.T())
	stack := e2e.ReadStack(suite.T())
	stackState := e2e.ReadStackState(suite.T())
	suite.connector = stack.TokenProviders[0]
	account := stackState.Accounts[0].(map[string]interface{})
	suite.key = account["address"].(string)
}

func (suite *TokensTestSuite) BeforeTest(suiteName, testName string) {
	suite.testState = beforeE2ETest(suite.T())
}

func (suite *TokensTestSuite) AfterTest(suiteName, testName string) {
	e2e.VerifyAllOperationsSucceeded(suite.T(), []*client.FireFlyClient{suite.testState.client1}, suite.testState.startTime)
}

func (suite *TokensTestSuite) TestE2EFungibleTokensAsync() {
	defer suite.testState.done()

	received1 := e2e.WsReader(suite.testState.ws1)

	poolName := fmt.Sprintf("pool_%s", e2e.RandomName(suite.T()))
	suite.T().Logf("Pool name: %s", poolName)

	pool := &core.TokenPool{
		Name:   poolName,
		Type:   core.TokenTypeFungible,
		Config: fftypes.JSONObject{},
	}

	poolResp := suite.testState.client1.CreateTokenPool(suite.T(), pool, false, false)
	poolID := poolResp.ID

	e2e.WaitForEvent(suite.T(), received1, core.EventTypePoolConfirmed, poolID)
	pools := suite.testState.client1.GetTokenPools(suite.T(), suite.testState.startTime)
	assert.Equal(suite.T(), 1, len(pools))
	assert.Equal(suite.T(), suite.testState.namespace, pools[0].Namespace)
	assert.Equal(suite.T(), suite.connector, pools[0].Connector)
	assert.Equal(suite.T(), poolName, pools[0].Name)
	assert.Equal(suite.T(), core.TokenTypeFungible, pools[0].Type)
	assert.NotEmpty(suite.T(), pools[0].Locator)

	transfer := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{Amount: *fftypes.NewFFBigInt(1)},
		Pool:          poolName,
	}
	transferOut := suite.testState.client1.MintTokens(suite.T(), transfer, false)
	e2e.WaitForEvent(suite.T(), received1, core.EventTypeTransferConfirmed, transferOut.LocalID)
	e2e.ValidateAccountBalances(suite.T(), suite.testState.client1, poolID, "", map[string]int64{
		suite.key: 1,
	})

	transfer = &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{Amount: *fftypes.NewFFBigInt(1)},
		Pool:          poolName,
	}
	transferOut = suite.testState.client1.BurnTokens(suite.T(), transfer, false)
	e2e.WaitForEvent(suite.T(), received1, core.EventTypeTransferConfirmed, transferOut.LocalID)
	e2e.ValidateAccountBalances(suite.T(), suite.testState.client1, poolID, "", map[string]int64{
		suite.key: 0,
	})

	suite.testState.client1.DeleteTokenPool(suite.T(), poolID, 204)
	pools = suite.testState.client1.GetTokenPools(suite.T(), suite.testState.startTime)
	assert.Equal(suite.T(), 0, len(pools))
}
