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
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type NamespaceAliasSuite struct {
	suite.Suite
	testState *testState
}

func (suite *NamespaceAliasSuite) BeforeTest(suiteName, testName string) {
	suite.testState = beforeE2ETest(suite.T())
}

func (suite *NamespaceAliasSuite) AfterTest(suiteName, testName string) {
	e2e.VerifyAllOperationsSucceeded(suite.T(), []*client.FireFlyClient{suite.testState.client1, suite.testState.client2}, suite.testState.startTime)
	suite.testState.done()
}

func newNamespace(networkNamespace, contractAddress string) (ns, org, node map[string]interface{}) {
	org = make(map[string]interface{})
	node = make(map[string]interface{})
	ns = map[string]interface{}{
		"multiparty": map[string]interface{}{
			"enabled":          true,
			"networkNamespace": networkNamespace,
			"org":              org,
			"node":             node,
			"contract": []map[string]interface{}{
				{
					"location":   map[string]interface{}{"address": contractAddress},
					"firstEvent": "0",
				},
			},
		},
	}
	return ns, org, node
}

func (suite *NamespaceAliasSuite) TestMultiTenancy() {
	defer suite.testState.Done()

	address := deployContract(suite.T(), suite.testState.stackName, "firefly/Firefly.json")
	namespace := e2e.RandomName(suite.T())
	nsAlice := namespace + "-A"
	nsBob := namespace + "-B"
	nsCharlie := namespace + "-C"
	suite.T().Logf("Test namespace: %s", namespace)

	data := &core.DataRefOrValue{Value: fftypes.JSONAnyPtr(`"test"`)}

	// Add 3 local namespaces that all map to the same remote namespace
	// (2 on the first node, 1 on the second)
	data1 := e2e.ReadConfig(suite.T(), suite.testState.configFile1)
	ns, org, node := newNamespace(namespace, address)
	ns["name"] = nsAlice
	org["name"] = suite.testState.org1.Name
	org["key"] = suite.testState.org1key.Value
	node["name"] = "alice"
	e2e.AddNamespace(data1, ns)
	ns, org, node = newNamespace(namespace, address)
	ns["name"] = nsBob
	org["name"] = suite.testState.org1.Name
	org["key"] = suite.testState.org1key.Value
	node["name"] = "bob"
	e2e.AddNamespace(data1, ns)
	e2e.WriteConfig(suite.T(), suite.testState.configFile1, data1)

	data2 := e2e.ReadConfig(suite.T(), suite.testState.configFile2)
	ns, org, node = newNamespace(namespace, address)
	ns["name"] = nsCharlie
	org["name"] = suite.testState.org2.Name
	org["key"] = suite.testState.org2key.Value
	node["name"] = "charlie"
	e2e.AddNamespace(data2, ns)
	e2e.WriteConfig(suite.T(), suite.testState.configFile2, data2)

	admin1 := client.NewResty(suite.T())
	admin2 := client.NewResty(suite.T())
	admin1.SetBaseURL(suite.testState.adminHost1 + "/spi/v1")
	admin2.SetBaseURL(suite.testState.adminHost2 + "/spi/v1")

	clientAlice := client.NewFireFly(suite.T(), suite.testState.client1.Hostname, nsAlice)
	clientBob := client.NewFireFly(suite.T(), suite.testState.client1.Hostname, nsBob)
	clientCharlie := client.NewFireFly(suite.T(), suite.testState.client2.Hostname, nsCharlie)

	// Reset both nodes to pick up the new namespace
	e2e.ResetFireFly(suite.T(), admin1)
	e2e.ResetFireFly(suite.T(), admin2)
	e2e.PollForUp(suite.T(), clientAlice)
	e2e.PollForUp(suite.T(), clientBob)
	e2e.PollForUp(suite.T(), clientCharlie)

	eventNames := "message_confirmed"
	queryString := fmt.Sprintf("namespace=%s&ephemeral&autoack&filter.events=%s", nsAlice, eventNames)
	receivedAlice := e2e.WsReader(clientAlice.WebSocket(suite.T(), queryString, nil))
	queryString = fmt.Sprintf("namespace=%s&ephemeral&autoack&filter.events=%s", nsBob, eventNames)
	receivedBob := e2e.WsReader(clientBob.WebSocket(suite.T(), queryString, nil))
	queryString = fmt.Sprintf("namespace=%s&ephemeral&autoack&filter.events=%s", nsCharlie, eventNames)
	receivedCharlie := e2e.WsReader(clientCharlie.WebSocket(suite.T(), queryString, nil))

	// Register org/node identities on the new namespace
	clientAlice.RegisterSelfOrg(suite.T(), true)
	e2e.WaitForMessageConfirmed(suite.T(), receivedBob, core.MessageTypeDefinition)
	clientAlice.RegisterSelfNode(suite.T(), true)
	clientBob.RegisterSelfNode(suite.T(), true)
	clientCharlie.RegisterSelfOrg(suite.T(), true)
	clientCharlie.RegisterSelfNode(suite.T(), true)

	// Verify that a broadcast on the new namespace succeeds
	resp, err := clientAlice.BroadcastMessage(suite.T(), "topic", "", data, false)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 202, resp.StatusCode())
	e2e.WaitForMessageConfirmed(suite.T(), receivedAlice, core.MessageTypeBroadcast)
	e2e.WaitForMessageConfirmed(suite.T(), receivedBob, core.MessageTypeBroadcast)
	e2e.WaitForMessageConfirmed(suite.T(), receivedCharlie, core.MessageTypeBroadcast)

	toAlice := []core.MemberInput{
		{Identity: suite.testState.org1.Name, Node: "did:firefly:node/alice"},
	}
	toBob := []core.MemberInput{
		{Identity: suite.testState.org1.Name, Node: "did:firefly:node/bob"},
	}
	toCharlie := []core.MemberInput{
		{Identity: suite.testState.org2.Name, Node: "did:firefly:node/charlie"},
	}
	toAll := []core.MemberInput{
		{Identity: suite.testState.org1.Name, Node: "did:firefly:node/alice"},
		{Identity: suite.testState.org1.Name, Node: "did:firefly:node/bob"},
		{Identity: suite.testState.org2.Name, Node: "did:firefly:node/charlie"},
	}

	// Verify that private messages on the new namespace succeed
	// Alice -> Charlie
	resp, err = clientAlice.PrivateMessage("topic", "", data, toCharlie, "tag1", core.TransactionTypeBatchPin, false, suite.testState.startTime)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 202, resp.StatusCode())
	e2e.WaitForMessageConfirmed(suite.T(), receivedAlice, core.MessageTypePrivate)
	e2e.WaitForMessageConfirmed(suite.T(), receivedCharlie, core.MessageTypePrivate)
	// Charlie -> Alice
	resp, err = clientCharlie.PrivateMessage("topic", "", data, toAlice, "tag2", core.TransactionTypeBatchPin, false, suite.testState.startTime)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 202, resp.StatusCode())
	e2e.WaitForMessageConfirmed(suite.T(), receivedAlice, core.MessageTypePrivate)
	e2e.WaitForMessageConfirmed(suite.T(), receivedCharlie, core.MessageTypePrivate)
	// Charlie -> Alice+Bob
	resp, err = clientCharlie.PrivateMessage("topic", "", data, toAll, "tag3", core.TransactionTypeBatchPin, false, suite.testState.startTime)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 202, resp.StatusCode())
	e2e.WaitForMessageConfirmed(suite.T(), receivedAlice, core.MessageTypePrivate)
	e2e.WaitForMessageConfirmed(suite.T(), receivedBob, core.MessageTypePrivate)
	e2e.WaitForMessageConfirmed(suite.T(), receivedCharlie, core.MessageTypePrivate)
	// Alice -> Bob
	resp, err = clientAlice.PrivateMessage("topic", "", data, toBob, "tag4", core.TransactionTypeBatchPin, false, suite.testState.startTime)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 202, resp.StatusCode())
	e2e.WaitForMessageConfirmed(suite.T(), receivedAlice, core.MessageTypePrivate)
	e2e.WaitForMessageConfirmed(suite.T(), receivedBob, core.MessageTypePrivate)
	// Bob -> Alice+Charlie (blob transfer)
	dataRef, resp, err := clientBob.PrivateBlobMessageDatatypeTagged(suite.T(), "topic", toAll, suite.testState.startTime)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 202, resp.StatusCode())
	assert.NotNil(suite.T(), dataRef.Blob.Hash)
	e2e.WaitForMessageConfirmed(suite.T(), receivedAlice, core.MessageTypePrivate)
	e2e.WaitForMessageConfirmed(suite.T(), receivedBob, core.MessageTypePrivate)
	e2e.WaitForMessageConfirmed(suite.T(), receivedCharlie, core.MessageTypePrivate)

	dataRecv := clientAlice.GetDataByID(suite.T(), dataRef.ID)
	assert.Equal(suite.T(), dataRef.Blob.Hash, dataRecv.Blob.Hash)
	dataRecv = clientCharlie.GetDataByID(suite.T(), dataRef.ID)
	assert.Equal(suite.T(), dataRef.Blob.Hash, dataRecv.Blob.Hash)

	messages := clientAlice.GetMessages(suite.T(), suite.testState.startTime, core.MessageTypePrivate, "topic")
	assert.Len(suite.T(), messages, 5)
	messages = clientBob.GetMessages(suite.T(), suite.testState.startTime, core.MessageTypePrivate, "topic")
	assert.Len(suite.T(), messages, 3)
	messages = clientCharlie.GetMessages(suite.T(), suite.testState.startTime, core.MessageTypePrivate, "topic")
	assert.Len(suite.T(), messages, 4)
}
