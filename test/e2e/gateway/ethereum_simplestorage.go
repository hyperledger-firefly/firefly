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
	"os"
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

var simpleStorageContractVersion, _ = nanoid.Generate("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz", nanoid.DefaultSize)

func simpleStorageFFIChanged() *fftypes.FFIEvent {
	return &fftypes.FFIEvent{
		FFIEventDefinition: fftypes.FFIEventDefinition{
			Name: "Changed",
			Params: fftypes.FFIParams{
				{
					Name:   "_from",
					Schema: fftypes.JSONAnyPtr(`{"type": "string", "details": {"type": "address", "indexed": true}}`),
				},
				{
					Name:   "_value",
					Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
				},
			},
		},
	}
}

func simpleStorageFFI() *fftypes.FFI {
	return &fftypes.FFI{
		Name:    "SimpleStorage",
		Version: simpleStorageContractVersion,
		Methods: []*fftypes.FFIMethod{
			simpleStorageFFISet(),
			simpleStorageFFIGet(),
		},
		Events: []*fftypes.FFIEvent{
			simpleStorageFFIChanged(),
		},
	}
}

func simpleStorageFFISet() *fftypes.FFIMethod {
	return &fftypes.FFIMethod{
		Name: "set",
		Params: fftypes.FFIParams{
			{
				Name:   "newValue",
				Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
			},
		},
		Returns: fftypes.FFIParams{},
	}
}

func simpleStorageFFIGet() *fftypes.FFIMethod {
	return &fftypes.FFIMethod{
		Name:   "get",
		Params: fftypes.FFIParams{},
		Returns: fftypes.FFIParams{
			{
				Name:   "output",
				Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`),
			},
		},
	}
}

func deployTestContractFromCompiledJSON(t *testing.T, stackName, contract string) (*fftypes.JSONAny, string) {
	path := "../../data/contracts/" + contract
	out, err := exec.Command("ff", "deploy", "ethereum", stackName, path).Output()
	var stderr []byte
	if err != nil {
		stderr = err.(*exec.ExitError).Stderr
	}
	require.NoError(t, err, fmt.Sprintf("ff deploy failed: %s", stderr))
	var output map[string]interface{}
	err = json.Unmarshal(out, &output)
	require.NoError(t, err)
	address := output["address"].(string)
	t.Logf("Contract address: %s", address)

	type solcJSON struct {
		Contracts map[string]struct {
			ABI *fftypes.JSONAny `json:"abi"`
		} `json:"contracts"`
	}
	b, err := os.ReadFile(path)
	assert.NoError(t, err)
	var contractJSON solcJSON
	err = json.Unmarshal(b, &contractJSON)
	assert.NoError(t, err)

	var abiBytes *fftypes.JSONAny
	for _, contract := range contractJSON.Contracts {
		abiBytes = contract.ABI
		if abiBytes != nil {
			break
		}
	}
	assert.NotNil(t, abiBytes)
	return abiBytes, address
}

type EthereumSimpleStorageTestSuite struct {
	suite.Suite
	testState       *testState
	contractAddress string
	interfaceID     *fftypes.UUID
	ethClient       *resty.Client
	ethIdentity     string
}

func (suite *EthereumSimpleStorageTestSuite) SetupSuite() {
	suite.testState = beforeE2ETest(suite.T())
	stack := e2e.ReadStack(suite.T())
	stackState := e2e.ReadStackState(suite.T())
	suite.ethClient = client.NewResty(suite.T())
	suite.ethClient.SetBaseURL(fmt.Sprintf("http://localhost:%d", stack.Members[0].ExposedConnectorPort))
	account := stackState.Accounts[0].(map[string]interface{})
	suite.ethIdentity = account["address"].(string)
	_, suite.contractAddress = deployTestContractFromCompiledJSON(suite.T(), stack.Name, "simplestorage/simple_storage.json")

	res, err := suite.testState.client1.CreateFFI(suite.T(), simpleStorageFFI(), false)
	suite.interfaceID = res.ID
	suite.T().Logf("interfaceID: %s", suite.interfaceID)
	assert.NoError(suite.T(), err)
}

func (suite *EthereumSimpleStorageTestSuite) BeforeTest(suiteName, testName string) {
	suite.testState = beforeE2ETest(suite.T())
}

func (suite *EthereumSimpleStorageTestSuite) AfterTest(suiteName, testName string) {
	e2e.VerifyAllOperationsSucceeded(suite.T(), []*client.FireFlyClient{suite.testState.client1}, suite.testState.startTime)
}

func (suite *EthereumSimpleStorageTestSuite) TestDirectInvokeMethod() {
	defer suite.testState.Done()

	received1 := e2e.WsReader(suite.testState.ws1)
	listener := suite.testState.client1.CreateContractListener(suite.T(), simpleStorageFFIChanged(), &fftypes.JSONObject{
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
		Method:   simpleStorageFFISet(),
		Input: map[string]interface{}{
			"newValue": float64(2),
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
			"_value": "2",
			"_from":  suite.ethIdentity,
		},
		"listener": listener.ID.String(),
	}

	event := e2e.WaitForContractEvent(suite.T(), suite.testState.client1, received1, match)
	assert.NotNil(suite.T(), event)

	queryContractRequest := &core.ContractCallRequest{
		Location: fftypes.JSONAnyPtrBytes(locationBytes),
		Method:   simpleStorageFFIGet(),
	}
	queryRes, err := suite.testState.client1.QueryContractMethod(suite.T(), queryContractRequest)
	assert.NoError(suite.T(), err)
	resJSON, err := json.Marshal(queryRes)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), `{"output":"2"}`, string(resJSON))
	suite.testState.client1.DeleteContractListener(suite.T(), listener.ID)
}

func (suite *EthereumSimpleStorageTestSuite) TestFFIInvokeMethod() {
	defer suite.testState.Done()

	received1 := e2e.WsReader(suite.testState.ws1)

	ffiReference := &fftypes.FFIReference{
		ID: suite.interfaceID,
	}
	listener := suite.testState.client1.CreateFFIContractListener(suite.T(), ffiReference, "Changed", &fftypes.JSONObject{
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
		Input: map[string]interface{}{
			"newValue": float64(42),
		},
		Interface:  suite.interfaceID,
		MethodPath: "set",
	}

	res, err := suite.testState.client1.InvokeContractMethod(suite.T(), invokeContractRequest)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), res)

	match := map[string]interface{}{
		"info": map[string]interface{}{
			"address": suite.contractAddress,
		},
		"output": map[string]interface{}{
			"_value": "42",
			"_from":  suite.ethIdentity,
		},
		"listener": listener.ID.String(),
	}
	event := e2e.WaitForContractEvent(suite.T(), suite.testState.client1, received1, match)
	assert.NotNil(suite.T(), event)

	queryContractRequest := &core.ContractCallRequest{
		Location:   fftypes.JSONAnyPtrBytes(locationBytes),
		Interface:  suite.interfaceID,
		MethodPath: "get",
	}
	queryRes, err := suite.testState.client1.QueryContractMethod(suite.T(), queryContractRequest)
	assert.NoError(suite.T(), err)
	resJSON, err := json.Marshal(queryRes)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), `{"output":"42"}`, string(resJSON))
	suite.testState.client1.DeleteContractListener(suite.T(), listener.ID)
}

func (suite *EthereumSimpleStorageTestSuite) TestContractAPIMethod() {
	defer suite.testState.Done()

	received1 := e2e.WsReader(suite.testState.ws1)
	APIName := fftypes.NewUUID().String()

	ffiReference := &fftypes.FFIReference{
		ID: suite.interfaceID,
	}

	location := map[string]interface{}{
		"address": suite.contractAddress,
	}
	locationBytes, _ := json.Marshal(location)

	createContractAPIResult, err := suite.testState.client1.CreateContractAPI(suite.T(), APIName, ffiReference, fftypes.JSONAnyPtr(string(locationBytes)), false)
	assert.NotNil(suite.T(), createContractAPIResult)
	assert.NoError(suite.T(), err)

	listener, err := suite.testState.client1.CreateContractAPIListener(suite.T(), APIName, "Changed", "firefly_e2e")
	assert.NoError(suite.T(), err)

	listeners := suite.testState.client1.GetContractListeners(suite.T(), suite.testState.startTime)
	assert.Equal(suite.T(), 1, len(listeners))
	assert.Equal(suite.T(), listener.BackendID, listeners[0].BackendID)

	input := fftypes.JSONAny(`{"newValue": 42}`)
	invokeResult, err := suite.testState.client1.InvokeContractAPIMethod(suite.T(), APIName, "set", &input)
	assert.NoError(suite.T(), err)
	assert.NotNil(suite.T(), invokeResult)

	match := map[string]interface{}{
		"info": map[string]interface{}{
			"address": suite.contractAddress,
		},
		"output": map[string]interface{}{
			"_value": "42",
			"_from":  suite.ethIdentity,
		},
		"listener": listener.ID.String(),
	}
	event := e2e.WaitForContractEvent(suite.T(), suite.testState.client1, received1, match)
	assert.NotNil(suite.T(), event)
	assert.NoError(suite.T(), err)

	res, err := suite.testState.client1.QueryContractAPIMethod(suite.T(), APIName, "get", fftypes.JSONAnyPtr("{}"))
	assert.NoError(suite.T(), err)
	resJSON, err := json.Marshal(res)
	assert.NoError(suite.T(), err)
	assert.Equal(suite.T(), `{"output":"42"}`, string(resJSON))

	suite.testState.client1.DeleteContractListener(suite.T(), listener.ID)
}
