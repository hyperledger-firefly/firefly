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

type TokensTestSuite struct {
	suite.Suite
	testState *testState
	connector string
}

func (suite *TokensTestSuite) SetupSuite() {
	suite.testState = beforeE2ETest(suite.T())
	stack := e2e.ReadStack(suite.T())
	suite.connector = stack.TokenProviders[0]
}

func (suite *TokensTestSuite) BeforeTest(suiteName, testName string) {
	suite.testState = beforeE2ETest(suite.T())
}

func (suite *TokensTestSuite) AfterTest(suiteName, testName string) {
	e2e.VerifyAllOperationsSucceeded(suite.T(), []*client.FireFlyClient{suite.testState.client1, suite.testState.client2}, suite.testState.startTime)
	suite.testState.done()
}

func (suite *TokensTestSuite) TestE2EFungibleTokensAsync() {
	received1 := e2e.WsReader(suite.testState.ws1)
	received2 := e2e.WsReader(suite.testState.ws2)

	poolName := fmt.Sprintf("pool_%s", e2e.RandomName(suite.T()))
	suite.T().Logf("Pool name: %s", poolName)

	pool := &core.TokenPool{
		Name:   poolName,
		Type:   core.TokenTypeFungible,
		Config: fftypes.JSONObject{},
	}

	poolResp := suite.testState.client1.CreateTokenPool(suite.T(), pool, true, false)
	poolID := poolResp.ID

	e2e.WaitForEvent(suite.T(), received1, core.EventTypePoolConfirmed, poolID)
	pools := suite.testState.client1.GetTokenPools(suite.T(), suite.testState.startTime)
	assert.Equal(suite.T(), 1, len(pools))
	assert.Equal(suite.T(), suite.testState.namespace, pools[0].Namespace)
	assert.Equal(suite.T(), suite.connector, pools[0].Connector)
	assert.Equal(suite.T(), poolName, pools[0].Name)
	assert.Equal(suite.T(), core.TokenTypeFungible, pools[0].Type)
	assert.NotEmpty(suite.T(), pools[0].Locator)
	assert.NotNil(suite.T(), pools[0].Message)

	e2e.WaitForEvent(suite.T(), received2, core.EventTypePoolConfirmed, poolID)
	pools = suite.testState.client2.GetTokenPools(suite.T(), suite.testState.startTime)
	assert.Equal(suite.T(), 1, len(pools))
	assert.Equal(suite.T(), suite.testState.namespace, pools[0].Namespace)
	assert.Equal(suite.T(), suite.connector, pools[0].Connector)
	assert.Equal(suite.T(), poolName, pools[0].Name)
	assert.Equal(suite.T(), core.TokenTypeFungible, pools[0].Type)
	assert.NotEmpty(suite.T(), pools[0].Locator)
	assert.NotNil(suite.T(), pools[0].Message)

	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Key:      suite.testState.org1key.Value,
			Operator: suite.testState.org2key.Value,
			Approved: true,
		},
		Pool:           poolName,
		IdempotencyKey: core.IdempotencyKey(fftypes.NewUUID().String()),
	}
	approvalOut := suite.testState.client1.TokenApproval(suite.T(), approval, false)
	_ = suite.testState.client1.TokenApproval(suite.T(), approval, false, 409) // check for idempotency key rejection

	e2e.WaitForEvent(suite.T(), received1, core.EventTypeApprovalConfirmed, approvalOut.LocalID)
	approvals := suite.testState.client1.GetTokenApprovals(suite.T(), poolID)
	assert.Equal(suite.T(), 1, len(approvals))
	assert.Equal(suite.T(), suite.connector, approvals[0].Connector)
	assert.Equal(suite.T(), true, approvals[0].Approved)

	transfer := &core.TokenTransferInput{
		TokenTransfer:  core.TokenTransfer{Amount: *fftypes.NewFFBigInt(1)},
		Pool:           poolName,
		IdempotencyKey: core.IdempotencyKey(fftypes.NewUUID().String()),
	}
	transferOut := suite.testState.client1.MintTokens(suite.T(), transfer, false)
	_ = suite.testState.client1.MintTokens(suite.T(), transfer, false, 409) // check for idempotency key rejection

	e2e.WaitForEvent(suite.T(), received1, core.EventTypeTransferConfirmed, transferOut.LocalID)
	transfers := suite.testState.client1.GetTokenTransfers(suite.T(), poolID)
	assert.Equal(suite.T(), 1, len(transfers))
	assert.Equal(suite.T(), suite.connector, transfers[0].Connector)
	assert.Equal(suite.T(), core.TokenTransferTypeMint, transfers[0].Type)
	assert.Equal(suite.T(), int64(1), transfers[0].Amount.Int().Int64())
	e2e.ValidateAccountBalances(suite.T(), suite.testState.client1, poolID, "", map[string]int64{
		suite.testState.org1key.Value: 1,
	})

	e2e.WaitForEvent(suite.T(), received2, core.EventTypeTransferConfirmed, nil)
	transfers = suite.testState.client2.GetTokenTransfers(suite.T(), poolID)
	assert.Equal(suite.T(), 1, len(transfers))
	assert.Equal(suite.T(), suite.connector, transfers[0].Connector)
	assert.Equal(suite.T(), core.TokenTransferTypeMint, transfers[0].Type)
	assert.Equal(suite.T(), int64(1), transfers[0].Amount.Int().Int64())
	e2e.ValidateAccountBalances(suite.T(), suite.testState.client2, poolID, "", map[string]int64{
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
		IdempotencyKey: core.IdempotencyKey(fftypes.NewUUID().String()),
	}
	transferOut = suite.testState.client2.TransferTokens(suite.T(), transfer, false)
	_ = suite.testState.client2.TransferTokens(suite.T(), transfer, false, 409) // check for idempotency key rejection

	e2e.WaitForEvent(suite.T(), received1, core.EventTypeMessageConfirmed, transferOut.Message)
	transfers = suite.testState.client1.GetTokenTransfers(suite.T(), poolID)
	assert.Equal(suite.T(), 2, len(transfers))
	assert.Equal(suite.T(), suite.connector, transfers[0].Connector)
	assert.Equal(suite.T(), core.TokenTransferTypeTransfer, transfers[0].Type)
	assert.Equal(suite.T(), int64(1), transfers[0].Amount.Int().Int64())
	data := suite.testState.client1.GetDataForMessage(suite.T(), suite.testState.startTime, transfers[0].Message)
	assert.Equal(suite.T(), 1, len(data))
	assert.Equal(suite.T(), `"token approval - payment for data"`, data[0].Value.String())
	e2e.ValidateAccountBalances(suite.T(), suite.testState.client1, poolID, "", map[string]int64{
		suite.testState.org1key.Value: 0,
		suite.testState.org2key.Value: 1,
	})

	e2e.WaitForEvent(suite.T(), received2, core.EventTypeMessageConfirmed, transferOut.Message)
	transfers = suite.testState.client2.GetTokenTransfers(suite.T(), poolID)
	assert.Equal(suite.T(), 2, len(transfers))
	assert.Equal(suite.T(), suite.connector, transfers[0].Connector)
	assert.Equal(suite.T(), core.TokenTransferTypeTransfer, transfers[0].Type)
	assert.Equal(suite.T(), int64(1), transfers[0].Amount.Int().Int64())
	e2e.ValidateAccountBalances(suite.T(), suite.testState.client2, poolID, "", map[string]int64{
		suite.testState.org1key.Value: 0,
		suite.testState.org2key.Value: 1,
	})

	transfer = &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{Amount: *fftypes.NewFFBigInt(1)},
		Pool:          poolName,
	}
	transferOut = suite.testState.client2.BurnTokens(suite.T(), transfer, false)

	e2e.WaitForEvent(suite.T(), received2, core.EventTypeTransferConfirmed, transferOut.LocalID)
	transfers = suite.testState.client2.GetTokenTransfers(suite.T(), poolID)
	assert.Equal(suite.T(), 3, len(transfers))
	assert.Equal(suite.T(), suite.connector, transfers[0].Connector)
	assert.Equal(suite.T(), core.TokenTransferTypeBurn, transfers[0].Type)
	assert.Equal(suite.T(), "", transfers[0].TokenIndex)
	assert.Equal(suite.T(), int64(1), transfers[0].Amount.Int().Int64())
	e2e.ValidateAccountBalances(suite.T(), suite.testState.client2, poolID, "", map[string]int64{
		suite.testState.org1key.Value: 0,
		suite.testState.org2key.Value: 0,
	})

	e2e.WaitForEvent(suite.T(), received1, core.EventTypeTransferConfirmed, nil)
	transfers = suite.testState.client1.GetTokenTransfers(suite.T(), poolID)
	assert.Equal(suite.T(), 3, len(transfers))
	assert.Equal(suite.T(), suite.connector, transfers[0].Connector)
	assert.Equal(suite.T(), core.TokenTransferTypeBurn, transfers[0].Type)
	assert.Equal(suite.T(), "", transfers[0].TokenIndex)
	assert.Equal(suite.T(), int64(1), transfers[0].Amount.Int().Int64())
	e2e.ValidateAccountBalances(suite.T(), suite.testState.client1, poolID, "", map[string]int64{
		suite.testState.org1key.Value: 0,
		suite.testState.org2key.Value: 0,
	})

	accounts := suite.testState.client1.GetTokenAccounts(suite.T(), poolID)
	assert.Equal(suite.T(), 2, len(accounts))
	assert.Equal(suite.T(), suite.testState.org2key.Value, accounts[0].Key)
	assert.Equal(suite.T(), suite.testState.org1key.Value, accounts[1].Key)
	accounts = suite.testState.client2.GetTokenAccounts(suite.T(), poolID)
	assert.Equal(suite.T(), 2, len(accounts))
	assert.Equal(suite.T(), suite.testState.org2key.Value, accounts[0].Key)
	assert.Equal(suite.T(), suite.testState.org1key.Value, accounts[1].Key)

	accountPools := suite.testState.client1.GetTokenAccountPools(suite.T(), suite.testState.org1key.Value)
	assert.Equal(suite.T(), *poolID, *accountPools[0].Pool)
	accountPools = suite.testState.client2.GetTokenAccountPools(suite.T(), suite.testState.org2key.Value)
	assert.Equal(suite.T(), *poolID, *accountPools[0].Pool)

	// Cannot delete pools in multiparty mode
	suite.testState.client1.DeleteTokenPool(suite.T(), poolID, 409)
}

func (suite *TokensTestSuite) TestE2ENonFungibleTokensSync() {
	received1 := e2e.WsReader(suite.testState.ws1)
	received2 := e2e.WsReader(suite.testState.ws2)

	poolName := fmt.Sprintf("pool_%s", e2e.RandomName(suite.T()))
	suite.T().Logf("Pool name: %s", poolName)

	pool := &core.TokenPool{
		Name:   poolName,
		Type:   core.TokenTypeNonFungible,
		Config: fftypes.JSONObject{},
	}

	poolOut := suite.testState.client1.CreateTokenPool(suite.T(), pool, true, true)
	assert.Equal(suite.T(), suite.testState.namespace, poolOut.Namespace)
	assert.Equal(suite.T(), poolName, poolOut.Name)
	assert.Equal(suite.T(), core.TokenTypeNonFungible, poolOut.Type)
	assert.NotEmpty(suite.T(), poolOut.Locator)
	assert.NotNil(suite.T(), poolOut.Message)

	poolID := poolOut.ID

	e2e.WaitForEvent(suite.T(), received1, core.EventTypePoolConfirmed, poolID)
	e2e.WaitForEvent(suite.T(), received2, core.EventTypePoolConfirmed, poolID)
	pools := suite.testState.client1.GetTokenPools(suite.T(), suite.testState.startTime)
	assert.Equal(suite.T(), 1, len(pools))
	assert.Equal(suite.T(), suite.testState.namespace, pools[0].Namespace)
	assert.Equal(suite.T(), poolName, pools[0].Name)
	assert.Equal(suite.T(), core.TokenTypeNonFungible, pools[0].Type)
	assert.NotEmpty(suite.T(), pools[0].Locator)
	assert.NotNil(suite.T(), pools[0].Message)

	approval := &core.TokenApprovalInput{
		TokenApproval: core.TokenApproval{
			Key:      suite.testState.org1key.Value,
			Operator: suite.testState.org2key.Value,
			Approved: true,
		},
		Pool: poolName,
	}
	approvalOut := suite.testState.client1.TokenApproval(suite.T(), approval, true)

	e2e.WaitForEvent(suite.T(), received1, core.EventTypeApprovalConfirmed, approvalOut.LocalID)
	approvals := suite.testState.client1.GetTokenApprovals(suite.T(), poolID)
	assert.Equal(suite.T(), 1, len(approvals))
	assert.Equal(suite.T(), suite.connector, approvals[0].Connector)
	assert.Equal(suite.T(), true, approvals[0].Approved)

	transfer := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			Amount: *fftypes.NewFFBigInt(1),
		},
		Pool: poolName,
	}
	transferOut := suite.testState.client1.MintTokens(suite.T(), transfer, true)
	assert.Equal(suite.T(), core.TokenTransferTypeMint, transferOut.Type)
	assert.Equal(suite.T(), "1", transferOut.TokenIndex)
	assert.Equal(suite.T(), int64(1), transferOut.Amount.Int().Int64())
	e2e.ValidateAccountBalances(suite.T(), suite.testState.client1, poolID, "1", map[string]int64{
		suite.testState.org1key.Value: 1,
	})

	e2e.WaitForEvent(suite.T(), received1, core.EventTypeTransferConfirmed, transferOut.LocalID)
	e2e.WaitForEvent(suite.T(), received2, core.EventTypeTransferConfirmed, nil)
	transfers := suite.testState.client2.GetTokenTransfers(suite.T(), poolID)
	assert.Equal(suite.T(), 1, len(transfers))
	assert.Equal(suite.T(), core.TokenTransferTypeMint, transfers[0].Type)
	assert.Equal(suite.T(), "1", transfers[0].TokenIndex)
	assert.Equal(suite.T(), int64(1), transfers[0].Amount.Int().Int64())
	e2e.ValidateAccountBalances(suite.T(), suite.testState.client2, poolID, "1", map[string]int64{
		suite.testState.org1key.Value: 1,
	})

	transfer = &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			TokenIndex: "1",
			To:         suite.testState.org2key.Value,
			Amount:     *fftypes.NewFFBigInt(1),
			From:       suite.testState.org1key.Value,
			Key:        suite.testState.org1key.Value,
		},
		Pool: poolName,
		Message: &core.MessageInOut{
			InlineData: core.InlineData{
				{
					Value: fftypes.JSONAnyPtr(`"ownership change"`),
				},
			},
		},
	}
	transferOut = suite.testState.client1.TransferTokens(suite.T(), transfer, true)
	assert.Equal(suite.T(), core.TokenTransferTypeTransfer, transferOut.Type)
	assert.Equal(suite.T(), "1", transferOut.TokenIndex)
	assert.Equal(suite.T(), int64(1), transferOut.Amount.Int().Int64())
	data := suite.testState.client1.GetDataForMessage(suite.T(), suite.testState.startTime, transferOut.Message)
	assert.Equal(suite.T(), 1, len(data))
	assert.Equal(suite.T(), `"ownership change"`, data[0].Value.String())
	e2e.ValidateAccountBalances(suite.T(), suite.testState.client1, poolID, "1", map[string]int64{
		suite.testState.org1key.Value: 0,
		suite.testState.org2key.Value: 1,
	})

	e2e.WaitForEvent(suite.T(), received1, core.EventTypeMessageConfirmed, transferOut.Message)
	e2e.WaitForEvent(suite.T(), received2, core.EventTypeMessageConfirmed, transferOut.Message)
	transfers = suite.testState.client2.GetTokenTransfers(suite.T(), poolID)
	assert.Equal(suite.T(), 2, len(transfers))
	assert.Equal(suite.T(), core.TokenTransferTypeTransfer, transfers[0].Type)
	assert.Equal(suite.T(), "1", transfers[0].TokenIndex)
	assert.Equal(suite.T(), int64(1), transfers[0].Amount.Int().Int64())
	e2e.ValidateAccountBalances(suite.T(), suite.testState.client2, poolID, "1", map[string]int64{
		suite.testState.org1key.Value: 0,
		suite.testState.org2key.Value: 1,
	})

	transfer = &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{
			TokenIndex: "1",
			Amount:     *fftypes.NewFFBigInt(1),
		},
		Pool: poolName,
	}
	transferOut = suite.testState.client2.BurnTokens(suite.T(), transfer, true)
	assert.Equal(suite.T(), core.TokenTransferTypeBurn, transferOut.Type)
	assert.Equal(suite.T(), "1", transferOut.TokenIndex)
	assert.Equal(suite.T(), int64(1), transferOut.Amount.Int().Int64())
	e2e.ValidateAccountBalances(suite.T(), suite.testState.client2, poolID, "1", map[string]int64{
		suite.testState.org1key.Value: 0,
		suite.testState.org2key.Value: 0,
	})

	e2e.WaitForEvent(suite.T(), received2, core.EventTypeTransferConfirmed, transferOut.LocalID)
	e2e.WaitForEvent(suite.T(), received1, core.EventTypeTransferConfirmed, nil)
	transfers = suite.testState.client1.GetTokenTransfers(suite.T(), poolID)
	assert.Equal(suite.T(), 3, len(transfers))
	assert.Equal(suite.T(), core.TokenTransferTypeBurn, transfers[0].Type)
	assert.Equal(suite.T(), "1", transfers[0].TokenIndex)
	assert.Equal(suite.T(), int64(1), transfers[0].Amount.Int().Int64())
	e2e.ValidateAccountBalances(suite.T(), suite.testState.client1, poolID, "1", map[string]int64{
		suite.testState.org1key.Value: 0,
		suite.testState.org2key.Value: 0,
	})

	accounts := suite.testState.client1.GetTokenAccounts(suite.T(), poolID)
	assert.Equal(suite.T(), 2, len(accounts))
	assert.Equal(suite.T(), suite.testState.org2key.Value, accounts[0].Key)
	assert.Equal(suite.T(), suite.testState.org1key.Value, accounts[1].Key)
	accounts = suite.testState.client2.GetTokenAccounts(suite.T(), poolID)
	assert.Equal(suite.T(), 2, len(accounts))
	assert.Equal(suite.T(), suite.testState.org2key.Value, accounts[0].Key)
	assert.Equal(suite.T(), suite.testState.org1key.Value, accounts[1].Key)

	accountPools := suite.testState.client1.GetTokenAccountPools(suite.T(), suite.testState.org1key.Value)
	assert.Equal(suite.T(), *poolID, *accountPools[0].Pool)
	accountPools = suite.testState.client2.GetTokenAccountPools(suite.T(), suite.testState.org2key.Value)
	assert.Equal(suite.T(), *poolID, *accountPools[0].Pool)

	// Cannot delete published pools
	suite.testState.client1.DeleteTokenPool(suite.T(), poolID, 409)
}

func (suite *TokensTestSuite) TestE2ETokenPoolPublish() {
	received1 := e2e.WsReader(suite.testState.ws1)
	received2 := e2e.WsReader(suite.testState.ws2)

	poolName := fmt.Sprintf("pool_%s", e2e.RandomName(suite.T()))
	localName := poolName + "-local"
	networkName := poolName + "-shared"
	suite.T().Logf("Pool local name: %s", localName)
	suite.T().Logf("Pool network name: %s", networkName)

	pool := &core.TokenPool{
		Name:   localName,
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
	assert.Equal(suite.T(), localName, pools[0].Name) // sending party uses local name
	assert.Equal(suite.T(), core.TokenTypeFungible, pools[0].Type)
	assert.NotEmpty(suite.T(), pools[0].Locator)
	assert.Nil(suite.T(), pools[0].Message)

	suite.testState.client1.PublishTokenPool(suite.T(), poolID, networkName, false)

	e2e.WaitForMessageConfirmed(suite.T(), received1, core.MessageTypeDefinition)
	e2e.WaitForEvent(suite.T(), received2, core.EventTypePoolConfirmed, poolID)
	pools = suite.testState.client2.GetTokenPools(suite.T(), suite.testState.startTime)
	assert.Equal(suite.T(), 1, len(pools))
	assert.Equal(suite.T(), suite.testState.namespace, pools[0].Namespace)
	assert.Equal(suite.T(), suite.connector, pools[0].Connector)
	assert.Equal(suite.T(), networkName, pools[0].Name) // receiving party gets network name
	assert.Equal(suite.T(), core.TokenTypeFungible, pools[0].Type)
	assert.NotEmpty(suite.T(), pools[0].Locator)
	assert.NotNil(suite.T(), pools[0].Message)

	// Cannot delete published pools
	suite.testState.client1.DeleteTokenPool(suite.T(), poolID, 409)
}
