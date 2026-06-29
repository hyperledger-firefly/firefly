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
	"strings"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/test/e2e"
	"github.com/hyperledger-firefly/firefly/test/e2e/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

var expectedERC20Methods = fftypes.JSONAnyPtr(`{
	"approval": {
		"format": "abi",
		"methods": [
		{
			"type": "function",
			"name": "approve",
			"stateMutability": "nonpayable",
			"inputs": [
			{
				"name": "spender",
				"type": "address",
				"internalType": "address"
			},
			{
				"name": "amount",
				"type": "uint256",
				"internalType": "uint256"
			}
			],
			"outputs": [
			{
				"name": "",
				"type": "bool",
				"internalType": "bool"
			}
			]
		}
		]
	},
	"burn": {
		"format": "abi",
		"methods": [
		{
			"type": "function",
			"name": "burn",
			"stateMutability": "nonpayable",
			"inputs": [
			{
				"name": "amount",
				"type": "uint256",
				"internalType": "uint256"
			}
			],
			"outputs": []
		},
		{
			"type": "function",
			"name": "burnFrom",
			"stateMutability": "nonpayable",
			"inputs": [
			{
				"name": "account",
				"type": "address",
				"internalType": "address"
			},
			{
				"name": "amount",
				"type": "uint256",
				"internalType": "uint256"
			}
			],
			"outputs": []
		}
		]
	},
	"mint": {
		"format": "abi",
		"methods": [
		{
			"type": "function",
			"name": "mint",
			"stateMutability": "nonpayable",
			"inputs": [
			{
				"name": "to",
				"type": "address",
				"internalType": "address"
			},
			{
				"name": "amount",
				"type": "uint256",
				"internalType": "uint256"
			}
			],
			"outputs": []
		}
		]
	},
	"transfer": {
		"format": "abi",
		"methods": [
		{
			"type": "function",
			"name": "transfer",
			"stateMutability": "nonpayable",
			"inputs": [
			{
				"name": "to",
				"type": "address",
				"internalType": "address"
			},
			{
				"name": "amount",
				"type": "uint256",
				"internalType": "uint256"
			}
			],
			"outputs": [
			{
				"name": "",
				"type": "bool",
				"internalType": "bool"
			}
			]
		},
		{
			"type": "function",
			"name": "transferFrom",
			"stateMutability": "nonpayable",
			"inputs": [
			{
				"name": "from",
				"type": "address",
				"internalType": "address"
			},
			{
				"name": "to",
				"type": "address",
				"internalType": "address"
			},
			{
				"name": "amount",
				"type": "uint256",
				"internalType": "uint256"
			}
			],
			"outputs": [
			{
				"name": "",
				"type": "bool",
				"internalType": "bool"
			}
			]
		}
		]
	}
}`)

var expectedERC1155Methods = fftypes.JSONAnyPtr(`{
	"approval": {
	  "format": "abi",
	  "methods": [
		{
		  "type": "function",
		  "name": "setApprovalForAllWithData",
		  "stateMutability": "nonpayable",
		  "inputs": [
			{
			  "name": "operator",
			  "type": "address",
			  "internalType": "address"
			},
			{
			  "name": "approved",
			  "type": "bool",
			  "internalType": "bool"
			},
			{
			  "name": "data",
			  "type": "bytes",
			  "internalType": "bytes"
			}
		  ],
		  "outputs": []
		},
		{
		  "type": "function",
		  "name": "setApprovalForAll",
		  "stateMutability": "nonpayable",
		  "inputs": [
			{
			  "name": "operator",
			  "type": "address",
			  "internalType": "address"
			},
			{
			  "name": "approved",
			  "type": "bool",
			  "internalType": "bool"
			}
		  ],
		  "outputs": []
		}
	  ]
	},
	"burn": {
	  "format": "abi",
	  "methods": [
		{
		  "type": "function",
		  "name": "burn",
		  "stateMutability": "nonpayable",
		  "inputs": [
			{
			  "name": "account",
			  "type": "address",
			  "internalType": "address"
			},
			{
			  "name": "id",
			  "type": "uint256",
			  "internalType": "uint256"
			},
			{
			  "name": "value",
			  "type": "uint256",
			  "internalType": "uint256"
			}
		  ],
		  "outputs": []
		}
	  ]
	},
	"mint": {
	  "format": "abi",
	  "methods": [
		{
		  "type": "function",
		  "name": "mintFungible",
		  "stateMutability": "nonpayable",
		  "inputs": [
			{
			  "name": "type_id",
			  "type": "uint256",
			  "internalType": "uint256"
			},
			{
			  "name": "to",
			  "type": "address[]",
			  "internalType": "address[]"
			},
			{
			  "name": "amounts",
			  "type": "uint256[]",
			  "internalType": "uint256[]"
			},
			{
			  "name": "data",
			  "type": "bytes",
			  "internalType": "bytes"
			}
		  ],
		  "outputs": []
		},
		{
		  "type": "function",
		  "name": "mintNonFungibleWithURI",
		  "stateMutability": "nonpayable",
		  "inputs": [
			{
			  "name": "type_id",
			  "type": "uint256",
			  "internalType": "uint256"
			},
			{
			  "name": "to",
			  "type": "address[]",
			  "internalType": "address[]"
			},
			{
			  "name": "data",
			  "type": "bytes",
			  "internalType": "bytes"
			},
			{
			  "name": "_uri",
			  "type": "string",
			  "internalType": "string"
			}
		  ],
		  "outputs": []
		},
		{
		  "type": "function",
		  "name": "mintNonFungible",
		  "stateMutability": "nonpayable",
		  "inputs": [
			{
			  "name": "type_id",
			  "type": "uint256",
			  "internalType": "uint256"
			},
			{
			  "name": "to",
			  "type": "address[]",
			  "internalType": "address[]"
			},
			{
			  "name": "data",
			  "type": "bytes",
			  "internalType": "bytes"
			}
		  ],
		  "outputs": []
		}
	  ]
	},
	"transfer": {
	  "format": "abi",
	  "methods": [
		{
		  "type": "function",
		  "name": "safeTransferFrom",
		  "stateMutability": "nonpayable",
		  "inputs": [
			{
			  "name": "from",
			  "type": "address",
			  "internalType": "address"
			},
			{
			  "name": "to",
			  "type": "address",
			  "internalType": "address"
			},
			{
			  "name": "id",
			  "type": "uint256",
			  "internalType": "uint256"
			},
			{
			  "name": "amount",
			  "type": "uint256",
			  "internalType": "uint256"
			},
			{
			  "name": "data",
			  "type": "bytes",
			  "internalType": "bytes"
			}
		  ],
		  "outputs": []
		}
	  ]
	}
  }`)

type EthereumTokenContractTestSuite struct {
	suite.Suite
	testState       *testState
	connector       string
	contract        string
	expectedMethods *fftypes.JSONAny
}

func (suite *EthereumTokenContractTestSuite) SetupSuite() {
	stack := e2e.ReadStack(suite.T())
	suite.connector = stack.TokenProviders[0]
	suite.contract = "erc20/ERC20OpenZeppelin.json"
	suite.expectedMethods = expectedERC20Methods
	if suite.connector == "erc1155" {
		suite.contract = "erc1155/ERC1155Sample.json"
		suite.expectedMethods = expectedERC1155Methods
	}
}

func (suite *EthereumTokenContractTestSuite) BeforeTest(suiteName, testName string) {
	suite.testState = beforeE2ETest(suite.T())
}

func (suite *EthereumTokenContractTestSuite) AfterTest(suiteName, testName string) {
	e2e.VerifyAllOperationsSucceeded(suite.T(), []*client.FireFlyClient{suite.testState.client1, suite.testState.client2}, suite.testState.startTime)
	suite.testState.done()
}

func (suite *EthereumTokenContractTestSuite) TestTokensWithInterface() {
	defer suite.testState.Done()

	received1 := e2e.WsReader(suite.testState.ws1)

	suite.T().Logf("contract: %s", suite.contract)
	contractAddress := deployContract(suite.T(), suite.testState.stackName, suite.contract)
	contractJSON := readContractJSON(suite.T(), suite.contract)
	version := contractVersion()

	ffi := suite.testState.client1.GenerateFFIFromABI(suite.T(), &fftypes.FFIGenerationRequest{
		Name:    "token_contract_test",
		Version: version,
		Input:   fftypes.JSONAnyPtr(`{"abi":` + contractJSON.GetObjectArray("abi").String() + `}`),
	})
	_, err := suite.testState.client1.CreateFFI(suite.T(), ffi, true)
	assert.NoError(suite.T(), err)

	poolName := fmt.Sprintf("pool_%s", e2e.RandomName(suite.T()))
	pool := &core.TokenPool{
		Name: poolName,
		Type: core.TokenTypeFungible,
		Config: fftypes.JSONObject{
			"address": contractAddress,
		},
		Interface: &fftypes.FFIReference{
			Name:    "token_contract_test",
			Version: version,
		},
	}
	poolResp := suite.testState.client1.CreateTokenPool(suite.T(), pool, true, false)
	e2e.WaitForEvent(suite.T(), received1, core.EventTypePoolConfirmed, poolResp.ID)

	poolResp = suite.testState.client1.GetTokenPool(suite.T(), poolResp.ID)
	assert.Equal(suite.T(), core.TokenInterfaceFormatABI, poolResp.InterfaceFormat)
	assert.Equal(suite.T(), suite.expectedMethods.JSONObject(), poolResp.Methods.JSONObject())

	transfer := &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{Amount: *fftypes.NewFFBigInt(1)},
		Pool:          poolName,
	}
	suite.testState.client1.MintTokens(suite.T(), transfer, false)
	e2e.WaitForEvent(suite.T(), received1, core.EventTypeTransferConfirmed, nil)

	transfers := suite.testState.client1.GetTokenTransfers(suite.T(), poolResp.ID)
	assert.Len(suite.T(), transfers, 1)
	blockchainEvent := suite.testState.client1.GetBlockchainEvent(suite.T(), transfers[0].BlockchainEvent.String())
	assert.Equal(suite.T(), strings.ToLower(contractAddress), strings.ToLower(blockchainEvent.Info["address"].(string)))

	e2e.ValidateAccountBalances(suite.T(), suite.testState.client1, poolResp.ID, "", map[string]int64{
		suite.testState.org1key.Value: 1,
	})

	transfer = &core.TokenTransferInput{
		TokenTransfer: core.TokenTransfer{Amount: *fftypes.NewFFBigInt(1)},
		Pool:          poolName,
	}
	suite.testState.client1.BurnTokens(suite.T(), transfer, false)
	e2e.WaitForEvent(suite.T(), received1, core.EventTypeTransferConfirmed, nil)

	transfers = suite.testState.client1.GetTokenTransfers(suite.T(), poolResp.ID)
	assert.Len(suite.T(), transfers, 2)

	e2e.ValidateAccountBalances(suite.T(), suite.testState.client1, poolResp.ID, "", map[string]int64{
		suite.testState.org1key.Value: 0,
	})

	if suite.connector == "erc1155" {
		// Create another pool and pass startId/endId explicitly
		pool.Name = fmt.Sprintf("pool_%s", e2e.RandomName(suite.T()))
		pool.Config["startId"] = "0x1" // intentionally does not match a pool generated by the factory
		pool.Config["endId"] = "0x1"
		poolResp = suite.testState.client1.CreateTokenPool(suite.T(), pool, true, false)
		e2e.WaitForEvent(suite.T(), received1, core.EventTypePoolConfirmed, poolResp.ID)
	}
}
