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
	"encoding/json"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/test/e2e"
	"github.com/hyperledger-firefly/firefly/test/e2e/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func customPinMethod() *fftypes.FFIMethod {
	return &fftypes.FFIMethod{
		Name: "MyCustomPin",
		Params: fftypes.FFIParams{
			{
				Name:   "data",
				Schema: fftypes.JSONAnyPtr(`{"type": "string"}`),
			},
		},
		Returns: fftypes.FFIParams{},
	}
}

type FabricContractWithMessageTestSuite struct {
	suite.Suite
	testState     *testState
	chaincodeName string
}

func (suite *FabricContractWithMessageTestSuite) SetupSuite() {
	stack := e2e.ReadStack(suite.T())
	suite.chaincodeName = deployChaincode(suite.T(), stack.Name, "/contracts/custompin-sample")
}

func (suite *FabricContractWithMessageTestSuite) BeforeTest(suiteName, testName string) {
	suite.testState = beforeE2ETest(suite.T())
}

func (suite *FabricContractWithMessageTestSuite) AfterTest(suiteName, testName string) {
	e2e.VerifyAllOperationsSucceeded(suite.T(), []*client.FireFlyClient{suite.testState.client1, suite.testState.client2}, suite.testState.startTime)
	suite.testState.done()
}

func (suite *FabricContractWithMessageTestSuite) TestCustomContractWithMessage() {
	defer suite.testState.Done()

	received1 := e2e.WsReader(suite.testState.ws1)

	location := map[string]interface{}{
		"chaincode": suite.chaincodeName,
		"channel":   "firefly",
	}
	locationBytes, _ := json.Marshal(location)

	_, err := suite.testState.client1.InvokeContractMethod(suite.T(), &core.ContractCallRequest{
		Method:   customPinMethod(),
		Location: fftypes.JSONAnyPtrBytes(locationBytes),
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
		Method:   customPinMethod(),
		Location: fftypes.JSONAnyPtrBytes(locationBytes),
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
