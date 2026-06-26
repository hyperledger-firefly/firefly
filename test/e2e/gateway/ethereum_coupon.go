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
	"encoding/json"
	"fmt"
	"os/exec"
	"testing"

	"github.com/aidarkhanov/nanoid"
	"github.com/go-resty/resty/v2"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly/pkg/core"
	"github.com/hyperledger/firefly/test/e2e"
	"github.com/hyperledger/firefly/test/e2e/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

var couponContractVersion, _ = nanoid.Generate("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz", nanoid.DefaultSize)

func couponFFICreated() *fftypes.FFIEvent {
	return &fftypes.FFIEvent{
		FFIEventDefinition: fftypes.FFIEventDefinition{
			Name: "CouponCreated",
			Params: fftypes.FFIParams{
				{
					Name: "_tokenId",
					Schema: fftypes.JSONAnyPtr(`{
                        "oneOf": [
                            {
                                "type": "string"
                            },
                            {
                                "type": "integer"
                            }
                        ],
                        "details": {
                            "type": "uint256",
                            "internalType": "uint256",
                            "indexed": true
                        },
                        "description": "An integer. You are recommended to use a JSON string. A JSON number can be used for values up to the safe maximum."
                    }`),
				},
				{
					Name: "_ifpsURL",
					Schema: fftypes.JSONAnyPtr(`{
                        "type": "string",
                        "details": {
                            "type": "string",
                            "internalType": "string",
                            "indexed": true
                        }
                    }`),
				},
				{
					Name: "_status",
					Schema: fftypes.JSONAnyPtr(`{
                        "type": "string",
                        "details": {
                            "type": "string",
                            "internalType": "string",
                            "indexed": true
                        }
                    }`),
				},
			},
		},
	}
}

func couponFFI() *fftypes.FFI {
	return &fftypes.FFI{
		Name:    "SimpleStorage",
		Version: couponContractVersion,
		Methods: []*fftypes.FFIMethod{
			couponFFICreateCoupon(),
			couponFFIGetAllCouponIDs(),
		},
		Events: []*fftypes.FFIEvent{
			couponFFICreated(),
		},
	}
}

func couponFFICreateCoupon() *fftypes.FFIMethod {
	return &fftypes.FFIMethod{
		Name: "createCoupon",
		Params: fftypes.FFIParams{
			{
				Name: "_id",
				Schema: fftypes.JSONAnyPtr(`{
					"oneOf": [
						{
							"type": "string"
						},
						{
							"type": "integer"
						}
					],
					"details": {
						"type": "uint256",
						"internalType": "uint256"
					},
					"description": "An integer. You are recommended to use a JSON string. A JSON number can be used for values up to the safe maximum."
				}`),
			},
			{
				Name: "_ipfsUrl",
				Schema: fftypes.JSONAnyPtr(`{
					"type": "string",
					"details": {
						"type": "string",
						"internalType": "string"
					}
				}`),
			},
			{
				Name: "_start",
				Schema: fftypes.JSONAnyPtr(`{
					"oneOf": [
						{
							"type": "string"
						},
						{
							"type": "integer"
						}
					],
					"details": {
						"type": "uint256",
						"internalType": "uint256"
					},
					"description": "An integer. You are recommended to use a JSON string. A JSON number can be used for values up to the safe maximum."
				}`),
			},
			{
				Name: "_end",
				Schema: fftypes.JSONAnyPtr(`{
					"oneOf": [
						{
							"type": "string"
						},
						{
							"type": "integer"
						}
					],
					"details": {
						"type": "uint256",
						"internalType": "uint256"
					},
					"description": "An integer. You are recommended to use a JSON string. A JSON number can be used for values up to the safe maximum."
				}`),
			},
		},
		Returns: fftypes.FFIParams{},
	}
}

func couponFFIGetAllCouponIDs() *fftypes.FFIMethod {
	return &fftypes.FFIMethod{
		Name:   "allCouponIds",
		Params: fftypes.FFIParams{},
		Returns: fftypes.FFIParams{
			{
				Name: "allCreatedIds",
				Schema: fftypes.JSONAnyPtr(`{
					"type": "array",
					"details": {
						"type": "uint256[]",
						"internalType": "uint256[]"
					},
					"items": {
						"oneOf": [
							{
								"type": "string"
							},
							{
								"type": "integer"
							}
						],
						"description": "An integer. You are recommended to use a JSON string. A JSON number can be used for values up to the safe maximum."
					}
				}`),
			},
		},
	}
}

func deployCouponContract(t *testing.T, stackName, contract string, address string) string {
	path := "../../data/contracts/" + contract
	out, err := exec.Command("ff", "deploy", "ethereum", stackName, path, address).Output()
	require.NoError(t, err)
	var output map[string]interface{}
	err = json.Unmarshal(out, &output)
	require.NoError(t, err)
	contractAddress := output["address"].(string)
	t.Logf("Contract address: %s", address)
	return contractAddress
}

type EthereumCouponTestSuite struct {
	suite.Suite
	testState       *testState
	contractAddress string
	interfaceID     *fftypes.UUID
	ethClient       *resty.Client
	ethIdentity     string
}

func (suite *EthereumCouponTestSuite) SetupSuite() {
	suite.testState = beforeE2ETest(suite.T())
	stack := e2e.ReadStack(suite.T())
	stackState := e2e.ReadStackState(suite.T())
	suite.ethClient = client.NewResty(suite.T())
	suite.ethClient.SetBaseURL(fmt.Sprintf("http://localhost:%d", stack.Members[0].ExposedConnectorPort))
	account := stackState.Accounts[0].(map[string]interface{})
	suite.ethIdentity = account["address"].(string)
	suite.contractAddress = deployCouponContract(suite.T(), stack.Name, "coupon/coupon.json", suite.ethIdentity)

	res, err := suite.testState.client1.CreateFFI(suite.T(), couponFFI(), false)
	suite.interfaceID = res.ID
	suite.T().Logf("interfaceID: %s", suite.interfaceID)
	assert.NoError(suite.T(), err)
}

func (suite *EthereumCouponTestSuite) BeforeTest(suiteName, testName string) {
	suite.testState = beforeE2ETest(suite.T())
}

func (suite *EthereumCouponTestSuite) AfterTest(suiteName, testName string) {
	e2e.VerifyAllOperationsSucceeded(suite.T(), []*client.FireFlyClient{suite.testState.client1}, suite.testState.startTime)
}

func (suite *EthereumCouponTestSuite) TestDirectInvokeMethod() {
	defer suite.testState.Done()

	received1 := e2e.WsReader(suite.testState.ws1)
	listener := suite.testState.client1.CreateContractListener(suite.T(), couponFFICreated(), &fftypes.JSONObject{
		"address": suite.contractAddress,
	})

	listeners := suite.testState.client1.GetContractListeners(suite.T(), suite.testState.startTime)
	assert.Equal(suite.T(), 1, len(listeners))
	assert.Equal(suite.T(), listener.BackendID, listeners[0].BackendID)

	location := map[string]interface{}{
		"address": suite.contractAddress,
	}
	locationBytes, _ := json.Marshal(location)
	invokeContractRequest := &core.ContractCallRequest{
		Location: fftypes.JSONAnyPtrBytes(locationBytes),
		Method:   couponFFICreateCoupon(),
		Input: map[string]interface{}{
			"_id":      "1",
			"_ipfsUrl": "https://ipfs.io/ipfs/Qmc5gCcjYypU7y28oCALwfSvxCBskLuPKWpK4qpterKC7z",
			"_start":   "2",
			"_end":     "3",
		},
	}

	res, err := suite.testState.client1.InvokeContractMethod(suite.T(), invokeContractRequest)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), res)

	match := map[string]interface{}{
		"info": map[string]interface{}{
			"address": suite.contractAddress,
		},
		"output": map[string]interface{}{
			"_tokenId": "1",
			"_ifpsURL": "0xaa430c2f4b1a970b28fcf799f16f539663ad0148aa133f88d622d51f36877f63",
		},
		"listener": listener.ID.String(),
	}

	event := e2e.WaitForContractEvent(suite.T(), suite.testState.client1, received1, match)
	assert.NotNil(suite.T(), event)

	queryContractRequest := &core.ContractCallRequest{
		Location: fftypes.JSONAnyPtrBytes(locationBytes),
		Method:   couponFFIGetAllCouponIDs(),
	}
	queryRes, err := suite.testState.client1.QueryContractMethod(suite.T(), queryContractRequest)
	assert.NoError(suite.T(), err)
	resJSON, err := json.Marshal(queryRes)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), `{"allCreatedIds":["1"]}`, string(resJSON))
	suite.testState.client1.DeleteContractListener(suite.T(), listener.ID)
}
