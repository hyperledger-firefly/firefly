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

package multiparty

import (
	"fmt"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/test/e2e"
	"github.com/hyperledger-firefly/firefly/test/e2e/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type TokensRemoteNameTestSuite struct {
	suite.Suite
	testState *testState
	connector string
}

func (suite *TokensRemoteNameTestSuite) SetupSuite() {
	suite.testState = beforeE2ETest(suite.T())
	stack := e2e.ReadStack(suite.T())
	suite.connector = stack.TokenProviders[0]
}

func (suite *TokensRemoteNameTestSuite) BeforeTest(suiteName, testName string) {
	suite.testState = beforeE2ETest(suite.T())
}

func (suite *TokensRemoteNameTestSuite) AfterTest(suiteName, testName string) {
	e2e.VerifyAllOperationsSucceeded(suite.T(), []*client.FireFlyClient{suite.testState.client1, suite.testState.client2}, suite.testState.startTime)
	suite.testState.done()
}

func (suite *TokensRemoteNameTestSuite) TestE2EFungibleTokensWithRemoteNameAsync() {
	newLocalName := "tokens1"

	config1 := e2e.ReadConfig(suite.T(), suite.testState.configFile1)
	e2e.AddPluginBroadcastName(config1, "tokens", "testremote")

	// Change plugin local name and update plugin list
	e2e.ChangeDefaultNSPluginLocalName(config1, "tokens", newLocalName)
	e2e.WriteConfig(suite.T(), suite.testState.configFile1, config1)

	config2 := e2e.ReadConfig(suite.T(), suite.testState.configFile2)
	e2e.AddPluginBroadcastName(config2, "tokens", "testremote")
	e2e.WriteConfig(suite.T(), suite.testState.configFile2, config2)

	admin1 := client.NewResty(suite.T())
	admin2 := client.NewResty(suite.T())
	admin1.SetBaseURL(suite.testState.adminHost1 + "/spi/v1")
	admin2.SetBaseURL(suite.testState.adminHost2 + "/spi/v1")

	e2e.ResetFireFly(suite.T(), admin1)
	e2e.ResetFireFly(suite.T(), admin2)
	e2e.PollForUp(suite.T(), suite.testState.client1)
	e2e.PollForUp(suite.T(), suite.testState.client2)

	client1 := client.NewFireFly(suite.T(), suite.testState.client1.Hostname, suite.testState.namespace)
	client2 := client.NewFireFly(suite.T(), suite.testState.client2.Hostname, suite.testState.namespace)

	eventNames := "message_confirmed|token_pool_confirmed|token_transfer_confirmed|token_approval_confirmed"
	queryString := fmt.Sprintf("namespace=%s&ephemeral&autoack&filter.events=%s", suite.testState.namespace, eventNames)
	received1 := e2e.WsReader(client1.WebSocket(suite.T(), queryString, nil))
	received2 := e2e.WsReader(client2.WebSocket(suite.T(), queryString, nil))

	poolName := fmt.Sprintf("pool_%s", e2e.RandomName(suite.T()))
	suite.T().Logf("Pool name: %s", poolName)

	pool := &core.TokenPool{
		Name:   poolName,
		Type:   core.TokenTypeFungible,
		Config: fftypes.JSONObject{},
	}

	poolResp := client1.CreateTokenPool(suite.T(), pool, true, false)
	poolID := poolResp.ID

	e2e.WaitForEvent(suite.T(), received1, core.EventTypePoolConfirmed, poolID)
	pools := client1.GetTokenPools(suite.T(), suite.testState.startTime)
	assert.Equal(suite.T(), 1, len(pools))
	assert.Equal(suite.T(), suite.testState.namespace, pools[0].Namespace)
	assert.Equal(suite.T(), newLocalName, pools[0].Connector)
	assert.Equal(suite.T(), poolName, pools[0].Name)
	assert.Equal(suite.T(), core.TokenTypeFungible, pools[0].Type)
	assert.NotEmpty(suite.T(), pools[0].Locator)

	e2e.WaitForEvent(suite.T(), received2, core.EventTypePoolConfirmed, poolID)
	pools = client2.GetTokenPools(suite.T(), suite.testState.startTime)
	assert.Equal(suite.T(), 1, len(pools))
	assert.Equal(suite.T(), suite.testState.namespace, pools[0].Namespace)
	assert.Equal(suite.T(), suite.connector, pools[0].Connector)
	assert.Equal(suite.T(), poolName, pools[0].Name)
	assert.Equal(suite.T(), core.TokenTypeFungible, pools[0].Type)
	assert.NotEmpty(suite.T(), pools[0].Locator)

	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Key:      suite.testState.org1key.Value,
			Operator: suite.testState.org2key.Value,
			Approved: true,
		},
		Pool: poolName,
	}
	approvalOut := client1.TokenApproval(suite.T(), approval, false)

	e2e.WaitForEvent(suite.T(), received1, core.EventTypeApprovalConfirmed, approvalOut.LocalID)
	approvals := client1.GetTokenApprovals(suite.T(), poolID)
	assert.Equal(suite.T(), 1, len(approvals))
	assert.Equal(suite.T(), newLocalName, approvals[0].Connector)
	assert.Equal(suite.T(), true, approvals[0].Approved)

	transfer := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{Amount: *fftypes.NewFFBigInt(1)},
		Pool:          poolName,
	}
	transferOut := client1.MintTokens(suite.T(), transfer, false)

	e2e.WaitForEvent(suite.T(), received1, core.EventTypeTransferConfirmed, transferOut.LocalID)
	transfers := client1.GetTokenTransfers(suite.T(), poolID)
	assert.Equal(suite.T(), 1, len(transfers))
	assert.Equal(suite.T(), newLocalName, transfers[0].Connector)
	assert.Equal(suite.T(), core.TokenTransferTypeMint, transfers[0].Type)
	assert.Equal(suite.T(), int64(1), transfers[0].Amount.Int().Int64())
	e2e.ValidateAccountBalances(suite.T(), client1, poolID, "", map[string]int64{
		suite.testState.org1key.Value: 1,
	})

	e2e.WaitForEvent(suite.T(), received2, core.EventTypeTransferConfirmed, nil)
	transfers = client2.GetTokenTransfers(suite.T(), poolID)
	assert.Equal(suite.T(), 1, len(transfers))
	assert.Equal(suite.T(), suite.connector, transfers[0].Connector)
	assert.Equal(suite.T(), core.TokenTransferTypeMint, transfers[0].Type)
	assert.Equal(suite.T(), int64(1), transfers[0].Amount.Int().Int64())
	e2e.ValidateAccountBalances(suite.T(), client2, poolID, "", map[string]int64{
		suite.testState.org1key.Value: 1,
	})

	transfer = &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			To:     suite.testState.org2key.Value,
			Amount: *fftypes.NewFFBigInt(1),
			From:   suite.testState.org1key.Value,
			Key:    suite.testState.org2key.Value,
		},
		Pool: poolName,
		Message: &core.MessageInOut{
			InlineData: core.InlineData{
				{
					Value: fftypes.JSONAnyPtr(`"token approval - payment for data"`),
				},
			},
		},
	}
	transferOut = client2.TransferTokens(suite.T(), transfer, false)

	e2e.WaitForEvent(suite.T(), received1, core.EventTypeMessageConfirmed, transferOut.Message)
	transfers = client1.GetTokenTransfers(suite.T(), poolID)
	assert.Equal(suite.T(), 2, len(transfers))
	assert.Equal(suite.T(), newLocalName, transfers[0].Connector)
	assert.Equal(suite.T(), core.TokenTransferTypeTransfer, transfers[0].Type)
	assert.Equal(suite.T(), int64(1), transfers[0].Amount.Int().Int64())
	data := client1.GetDataForMessage(suite.T(), suite.testState.startTime, transfers[0].Message)
	assert.Equal(suite.T(), 1, len(data))
	assert.Equal(suite.T(), `"token approval - payment for data"`, data[0].Value.String())
	e2e.ValidateAccountBalances(suite.T(), client1, poolID, "", map[string]int64{
		suite.testState.org1key.Value: 0,
		suite.testState.org2key.Value: 1,
	})

	e2e.WaitForEvent(suite.T(), received2, core.EventTypeMessageConfirmed, transferOut.Message)
	transfers = client2.GetTokenTransfers(suite.T(), poolID)
	assert.Equal(suite.T(), 2, len(transfers))
	assert.Equal(suite.T(), suite.connector, transfers[0].Connector)
	assert.Equal(suite.T(), core.TokenTransferTypeTransfer, transfers[0].Type)
	assert.Equal(suite.T(), int64(1), transfers[0].Amount.Int().Int64())
	e2e.ValidateAccountBalances(suite.T(), client2, poolID, "", map[string]int64{
		suite.testState.org1key.Value: 0,
		suite.testState.org2key.Value: 1,
	})

	transfer = &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{Amount: *fftypes.NewFFBigInt(1)},
		Pool:          poolName,
	}
	transferOut = client2.BurnTokens(suite.T(), transfer, false)

	e2e.WaitForEvent(suite.T(), received2, core.EventTypeTransferConfirmed, transferOut.LocalID)
	transfers = client2.GetTokenTransfers(suite.T(), poolID)
	assert.Equal(suite.T(), 3, len(transfers))
	assert.Equal(suite.T(), suite.connector, transfers[0].Connector)
	assert.Equal(suite.T(), core.TokenTransferTypeBurn, transfers[0].Type)
	assert.Equal(suite.T(), "", transfers[0].TokenIndex)
	assert.Equal(suite.T(), int64(1), transfers[0].Amount.Int().Int64())
	e2e.ValidateAccountBalances(suite.T(), client2, poolID, "", map[string]int64{
		suite.testState.org1key.Value: 0,
		suite.testState.org2key.Value: 0,
	})

	e2e.WaitForEvent(suite.T(), received1, core.EventTypeTransferConfirmed, nil)
	transfers = client1.GetTokenTransfers(suite.T(), poolID)
	assert.Equal(suite.T(), 3, len(transfers))
	assert.Equal(suite.T(), newLocalName, transfers[0].Connector)
	assert.Equal(suite.T(), core.TokenTransferTypeBurn, transfers[0].Type)
	assert.Equal(suite.T(), "", transfers[0].TokenIndex)
	assert.Equal(suite.T(), int64(1), transfers[0].Amount.Int().Int64())
	e2e.ValidateAccountBalances(suite.T(), client1, poolID, "", map[string]int64{
		suite.testState.org1key.Value: 0,
		suite.testState.org2key.Value: 0,
	})

	accounts := client1.GetTokenAccounts(suite.T(), poolID)
	assert.Equal(suite.T(), 2, len(accounts))
	assert.Equal(suite.T(), suite.testState.org2key.Value, accounts[0].Key)
	assert.Equal(suite.T(), suite.testState.org1key.Value, accounts[1].Key)
	accounts = client2.GetTokenAccounts(suite.T(), poolID)
	assert.Equal(suite.T(), 2, len(accounts))
	assert.Equal(suite.T(), suite.testState.org2key.Value, accounts[0].Key)
	assert.Equal(suite.T(), suite.testState.org1key.Value, accounts[1].Key)

	accountPools := client1.GetTokenAccountPools(suite.T(), suite.testState.org1key.Value)
	assert.Equal(suite.T(), *poolID, *accountPools[0].Pool)
	accountPools = client2.GetTokenAccountPools(suite.T(), suite.testState.org2key.Value)
	assert.Equal(suite.T(), *poolID, *accountPools[0].Pool)
}
