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
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type ContractMigrationTestSuite struct {
	suite.Suite
	testState *testState
}

func (suite *ContractMigrationTestSuite) BeforeTest(suiteName, testName string) {
	suite.testState = beforeE2ETest(suite.T())
}

func (suite *ContractMigrationTestSuite) AfterTest(suiteName, testName string) {
	e2e.VerifyAllOperationsSucceeded(suite.T(), []*client.FireFlyClient{suite.testState.client1, suite.testState.client2}, suite.testState.startTime)
	suite.testState.done()
}

func (suite *ContractMigrationTestSuite) TestContractMigration() {
	defer suite.testState.Done()

	address1 := deployContract(suite.T(), suite.testState.stackName, "firefly/Firefly.json")
	address2 := deployContract(suite.T(), suite.testState.stackName, "firefly/Firefly.json")
	runMigrationTest(suite, address1, address2, false)
}

func runMigrationTest(suite *ContractMigrationTestSuite, address1, address2 string, startOnV1 bool) {
	testNamespace := e2e.RandomName(suite.T())
	suite.T().Logf("Test namespace: %s", testNamespace)

	org := map[string]interface{}{}
	node := map[string]interface{}{}
	namespaceInfo := map[string]interface{}{
		"name": testNamespace,
		"multiparty": map[string]interface{}{
			"enabled": true,
			"org":     org,
			"node":    node,
			"contract": []map[string]interface{}{
				{
					"location": map[string]interface{}{"address": address1},
				},
				{
					"location": map[string]interface{}{"address": address2},
				},
			},
		},
	}
	data := &core.DataRefOrValue{Value: fftypes.JSONAnyPtr(`"test"`)}
	members := []core.MemberInput{
		{Identity: suite.testState.org1.Name},
		{Identity: suite.testState.org2.Name},
	}

	// Add the new namespace to both config files
	data1 := e2e.ReadConfig(suite.T(), suite.testState.configFile1)
	org["name"] = suite.testState.org1.Name
	org["key"] = suite.testState.org1key.Value
	node["name"] = fmt.Sprintf("%s_node", suite.testState.org1.Name)
	e2e.AddNamespace(data1, namespaceInfo)
	e2e.WriteConfig(suite.T(), suite.testState.configFile1, data1)

	data2 := e2e.ReadConfig(suite.T(), suite.testState.configFile2)
	org["name"] = suite.testState.org2.Name
	org["key"] = suite.testState.org2key.Value
	node["name"] = fmt.Sprintf("%s_node", suite.testState.org2.Name)
	e2e.AddNamespace(data2, namespaceInfo)
	e2e.WriteConfig(suite.T(), suite.testState.configFile2, data2)

	admin1 := client.NewResty(suite.T())
	admin2 := client.NewResty(suite.T())
	admin1.SetBaseURL(suite.testState.adminHost1 + "/spi/v1")
	admin2.SetBaseURL(suite.testState.adminHost2 + "/spi/v1")

	// Reset both nodes to pick up the new namespace
	e2e.ResetFireFly(suite.T(), admin1)
	e2e.ResetFireFly(suite.T(), admin2)

	client1 := client.NewFireFly(suite.T(), suite.testState.client1.Hostname, testNamespace)
	client2 := client.NewFireFly(suite.T(), suite.testState.client2.Hostname, testNamespace)
	e2e.PollForUp(suite.T(), client1)
	e2e.PollForUp(suite.T(), client2)

	eventNames := "message_confirmed|blockchain_event_received|identity_confirmed"
	queryString := fmt.Sprintf("namespace=%s&ephemeral&autoack&filter.events=%s", testNamespace, eventNames)
	received1 := e2e.WsReader(client1.WebSocket(suite.T(), queryString, nil))
	received2 := e2e.WsReader(client2.WebSocket(suite.T(), queryString, nil))

	if startOnV1 {
		systemClient1 := client.NewFireFly(suite.T(), suite.testState.client1.Hostname, "ff_system")
		systemClient2 := client.NewFireFly(suite.T(), suite.testState.client2.Hostname, "ff_system")
		e2e.PollForUp(suite.T(), systemClient1)
		e2e.PollForUp(suite.T(), systemClient2)

		// Register org/node identities on ff_system if not registered (but not on the new namespace)
		if systemClient1.GetOrganization(suite.T(), suite.testState.org1.Name) == nil {
			systemClient1.RegisterSelfOrg(suite.T(), true)
			systemClient1.RegisterSelfNode(suite.T(), true)
		}
		if systemClient2.GetOrganization(suite.T(), suite.testState.org2.Name) == nil {
			systemClient2.RegisterSelfOrg(suite.T(), true)
			systemClient2.RegisterSelfNode(suite.T(), true)
		}

		// Verify that a broadcast and private message on the new namespace succeed under the first contract
		resp, err := client1.BroadcastMessage(suite.T(), "topic", "", data, false)
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), 202, resp.StatusCode())
		e2e.WaitForMessageConfirmed(suite.T(), received1, core.MessageTypeBroadcast)
		e2e.WaitForMessageConfirmed(suite.T(), received2, core.MessageTypeBroadcast)

		resp, err = client1.PrivateMessage("topic1", "", data, members, "", core.TransactionTypeBatchPin, false, suite.testState.startTime)
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), 202, resp.StatusCode())
		e2e.WaitForMessageConfirmed(suite.T(), received1, core.MessageTypePrivate)
		e2e.WaitForMessageConfirmed(suite.T(), received2, core.MessageTypePrivate)
	}

	// Register org/node identities on the new namespace
	client1.RegisterSelfOrg(suite.T(), true)
	client1.RegisterSelfNode(suite.T(), true)
	client2.RegisterSelfOrg(suite.T(), true)
	client2.RegisterSelfNode(suite.T(), true)

	// Wait for the orgs to propagate (org2 to node1, and org1 to node 2)
	orgConfs := append([]*core.EventDelivery{}, e2e.WaitForIdentityConfirmed(suite.T(), received1))
	orgConfs = append(orgConfs, e2e.WaitForIdentityConfirmed(suite.T(), received1))
	orgConfs = append(orgConfs, e2e.WaitForIdentityConfirmed(suite.T(), received2))
	orgConfs = append(orgConfs, e2e.WaitForIdentityConfirmed(suite.T(), received2))
	for _, oc := range orgConfs {
		assert.Regexp(suite.T(), fmt.Sprintf("%s|%s", suite.testState.org1.Name, suite.testState.org2.Name), oc.Identity.DID)
	}

	// Migrate to the new contract
	client1.NetworkAction(suite.T(), core.NetworkActionTerminate)
	e2e.WaitForContractEvent(suite.T(), client1, received1, map[string]interface{}{
		"output": map[string]interface{}{
			"namespace": "firefly:terminate",
		},
	})
	e2e.WaitForContractEvent(suite.T(), client2, received2, map[string]interface{}{
		"output": map[string]interface{}{
			"namespace": "firefly:terminate",
		},
	})

	// Verify that a broadcast on the new namespace succeeds under the second contract
	resp, err := client1.BroadcastMessage(suite.T(), "topic", "", data, false)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 202, resp.StatusCode())
	e2e.WaitForMessageConfirmed(suite.T(), received1, core.MessageTypeBroadcast)
	e2e.WaitForMessageConfirmed(suite.T(), received2, core.MessageTypeBroadcast)

	// Verify the contract addresses for both blockchain events
	events := client1.GetBlockchainEvents(suite.T(), suite.testState.startTime)
	assert.Equal(suite.T(), address2, strings.ToLower(events[0].Info["address"].(string)))
	assert.Equal(suite.T(), address1, strings.ToLower(events[1].Info["address"].(string)))

	// Verify that a private message on the new namespace succeeds under the second contract
	resp, err = client1.PrivateMessage("topic1", "", data, members, "", core.TransactionTypeBatchPin, false, suite.testState.startTime)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 202, resp.StatusCode())
	e2e.WaitForMessageConfirmed(suite.T(), received1, core.MessageTypePrivate)
	e2e.WaitForMessageConfirmed(suite.T(), received2, core.MessageTypePrivate)
}
