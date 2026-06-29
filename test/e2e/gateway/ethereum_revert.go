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

	"github.com/go-resty/resty/v2"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/test/e2e"
	"github.com/hyperledger-firefly/firefly/test/e2e/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type EthereumRevertTestSuite struct {
	suite.Suite
	testState       *testState
	contractAddress string
	ethClient       *resty.Client
	ethIdentity     string
	abi             *fftypes.JSONAny
}

func (suite *EthereumRevertTestSuite) SetupSuite() {
	suite.testState = beforeE2ETest(suite.T())
	stack := e2e.ReadStack(suite.T())
	stackState := e2e.ReadStackState(suite.T())
	suite.ethClient = client.NewResty(suite.T())
	suite.ethClient.SetBaseURL(fmt.Sprintf("http://localhost:%d", stack.Members[0].ExposedConnectorPort))
	account := stackState.Accounts[0].(map[string]interface{})
	suite.ethIdentity = account["address"].(string)
	suite.abi, suite.contractAddress = deployTestContractFromCompiledJSON(suite.T(), stack.Name, "reverter/reverter.json")
}

func (suite *EthereumRevertTestSuite) BeforeTest(suiteName, testName string) {
	suite.testState = beforeE2ETest(suite.T())
}

func (suite *EthereumRevertTestSuite) AfterTest(suiteName, testName string) {
	// Important part of the test - the status of the operation must go to Failed - immediately.
	// We should not encounter an "Initialized" status
	e2e.VerifyOperationsAlreadyMarkedFailed(suite.T(), []*client.FireFlyClient{suite.testState.client1}, suite.testState.startTime)
}

func (suite *EthereumRevertTestSuite) TestRevertTransitionsToFailed() {
	defer suite.testState.Done()

	type generateInput struct {
		ABI *fftypes.JSONAny `json:"abi"`
	}
	inputBytes, err := json.Marshal(&generateInput{ABI: suite.abi})
	assert.NoError(suite.T(), err)

	ffi := suite.testState.client1.GenerateFFIFromABI(suite.T(), &fftypes.FFIGenerationRequest{
		Input: fftypes.JSONAnyPtrBytes(inputBytes),
	})
	assert.NoError(suite.T(), err)
	var goBang *fftypes.FFIMethod
	for _, m := range ffi.Methods {
		if m.Name == "goBang" {
			goBang = m
		}
	}
	assert.NotNil(suite.T(), goBang)

	location := map[string]interface{}{
		"address": suite.contractAddress,
	}
	locationBytes, _ := json.Marshal(location)
	invokeContractRequest := &core.ContractCallRequest{
		IdempotencyKey: core.IdempotencyKey(fftypes.NewUUID().String()),
		Location:       fftypes.JSONAnyPtrBytes(locationBytes),
		Method:         goBang,
		Input:          map[string]interface{}{},
	}

	// Check we get the revert error all the way back through the API on the invoke, due to the gas estimation
	_, err = suite.testState.client1.InvokeContractMethod(suite.T(), invokeContractRequest, 500)
	assert.Regexp(suite.T(), "FF10111.*Bang!", err)
}
