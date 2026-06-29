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
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/test/e2e"
	"github.com/hyperledger-firefly/firefly/test/e2e/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type EthereumContractWithMessageTestSuite struct {
	suite.Suite
	testState       *testState
	contractAddress string
	contractJSON    fftypes.JSONObject
}

func (suite *EthereumContractWithMessageTestSuite) SetupSuite() {
	stack := e2e.ReadStack(suite.T())
	suite.contractAddress = deployContract(suite.T(), stack.Name, "custompin/CustomPin.json")
	suite.contractJSON = readContractJSON(suite.T(), "custompin/CustomPin.json")
}

func (suite *EthereumContractWithMessageTestSuite) BeforeTest(suiteName, testName string) {
	suite.testState = beforeE2ETest(suite.T())
}

func (suite *EthereumContractWithMessageTestSuite) AfterTest(suiteName, testName string) {
	e2e.VerifyAllOperationsSucceeded(suite.T(), []*client.FireFlyClient{suite.testState.client1, suite.testState.client2}, suite.testState.startTime)
	suite.testState.done()
}

func (suite *EthereumContractWithMessageTestSuite) TestCustomContractWithMessage() {
	defer suite.testState.Done()

	received1 := e2e.WsReader(suite.testState.ws1)

	ffi := suite.testState.client1.GenerateFFIFromABI(suite.T(), &fftypes.FFIGenerationRequest{
		Name:    "CustomPin",
		Version: contractVersion(),
		Input:   fftypes.JSONAnyPtr(`{"abi":` + suite.contractJSON.GetObjectArray("abi").String() + `}`),
	})
	iface, err := suite.testState.client1.CreateFFI(suite.T(), ffi, true)
	assert.NoError(suite.T(), err)

	status, _, err := suite.testState.client1.GetStatus()
	assert.NoError(suite.T(), err)

	_, err = suite.testState.client1.InvokeContractMethod(suite.T(), &core.ContractCallRequest{
		Interface:  iface.ID,
		MethodPath: "setFireFlyAddress",
		Location:   fftypes.JSONAnyPtr(`{"address":"` + suite.contractAddress + `"}`),
		Input: map[string]interface{}{
			"addr": status.Multiparty.Contracts.Active.Location.JSONObject().GetString("address"),
		},
	})
	assert.NoError(suite.T(), err)

	_, err = suite.testState.client1.InvokeContractMethod(suite.T(), &core.ContractCallRequest{
		Interface:  iface.ID,
		MethodPath: "sayHello",
		Location:   fftypes.JSONAnyPtr(`{"address":"` + suite.contractAddress + `"}`),
		Message: &core.MessageInOut{
			InlineData: []*core.DataRefOrValue{
				{
					Value: fftypes.JSONAnyPtr(`"Hello broadcast message"`),
				},
			},
		},
	})
	assert.NoError(suite.T(), err)
	e2e.WaitForMessageConfirmed(suite.T(), received1, core.MessageTypeBroadcast)

	_, err = suite.testState.client1.InvokeContractMethod(suite.T(), &core.ContractCallRequest{
		Interface:  iface.ID,
		MethodPath: "sayHello",
		Location:   fftypes.JSONAnyPtr(`{"address":"` + suite.contractAddress + `"}`),
		Message: &core.MessageInOut{
			Message: core.Message{
				Header: core.MessageHeader{
					Type: core.MessageTypePrivate,
				},
			},
			Group: &core.InputGroup{
				Members: []core.MemberInput{
					{Identity: suite.testState.org1.DID},
					{Identity: suite.testState.org2.DID},
				},
			},
			InlineData: []*core.DataRefOrValue{
				{
					Value: fftypes.JSONAnyPtr(`"Hello private message"`),
				},
			},
		},
	})
	assert.NoError(suite.T(), err)
	e2e.WaitForMessageConfirmed(suite.T(), received1, core.MessageTypePrivate)
}
