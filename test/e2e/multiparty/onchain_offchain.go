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
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"image/png"
	"math/big"
	"strings"

	image2ascii "github.com/qeesung/image2ascii/convert"

	"github.com/go-resty/resty/v2"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/test/e2e"
	"github.com/hyperledger-firefly/firefly/test/e2e/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type OnChainOffChainTestSuite struct {
	suite.Suite
	testState *testState
}

func (suite *OnChainOffChainTestSuite) BeforeTest(suiteName, testName string) {
	suite.testState = beforeE2ETest(suite.T())
}

func (suite *OnChainOffChainTestSuite) AfterTest(suiteName, testName string) {
	e2e.VerifyAllOperationsSucceeded(suite.T(), []*client.FireFlyClient{suite.testState.client1, suite.testState.client2}, suite.testState.startTime)
	suite.testState.done()
}

func (suite *OnChainOffChainTestSuite) TestE2EBroadcast() {
	received1 := e2e.WsReader(suite.testState.ws1)
	received2 := e2e.WsReader(suite.testState.ws2)

	// Broadcast some messages, that should get batched, across two topics
	testUUID := fftypes.NewUUID()
	totalMessages := 10
	topics := []string{"topicA", "topicB"}
	expectedData := make(map[string][]*core.DataRefOrValue)
	for i := 0; i < 10; i++ {
		value := fftypes.JSONAnyPtr(fmt.Sprintf(`"Hello number %d"`, i))
		data := &core.DataRefOrValue{
			Value: value,
		}
		topic := e2e.PickTopic(i, topics)

		expectedData[topic] = append(expectedData[topic], data)

		idempotencyKey := fmt.Sprintf("%s/%d", testUUID, i)
		resp, err := suite.testState.client1.BroadcastMessage(suite.T(), topic, idempotencyKey, data, false)
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), 202, resp.StatusCode())

		// Ensure idempotency
		resp, err = suite.testState.client1.BroadcastMessage(suite.T(), topic, idempotencyKey, data, false)
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), 409, resp.StatusCode())
	}

	for i := 0; i < totalMessages; i++ {
		// Wait for all the message-confirmed events, from both participants
		e2e.WaitForMessageConfirmed(suite.T(), received1, core.MessageTypeBroadcast)
		e2e.WaitForMessageConfirmed(suite.T(), received2, core.MessageTypeBroadcast)
	}

	for topic, dataArray := range expectedData {
		receiver1data := validateReceivedMessages(suite.testState, suite.testState.client1, topic, core.MessageTypeBroadcast, core.TransactionTypeBatchPin, len(dataArray))
		receiver2data := validateReceivedMessages(suite.testState, suite.testState.client2, topic, core.MessageTypeBroadcast, core.TransactionTypeBatchPin, len(dataArray))
		// Messages should be returned in exactly reverse send order (newest first)
		for i := (len(dataArray) - 1); i >= 0; i-- {
			assert.Equal(suite.T(), dataArray[i].Value, receiver1data[i].Value)
			assert.Equal(suite.T(), dataArray[i].Value, receiver2data[i].Value)
		}
	}

}

func (suite *OnChainOffChainTestSuite) TestStrongDatatypesBroadcast() {
	received1 := e2e.WsReader(suite.testState.ws1)
	received2 := e2e.WsReader(suite.testState.ws2)

	var resp *resty.Response
	value := fftypes.JSONAnyPtr(`"Hello"`)
	randVer, _ := rand.Int(rand.Reader, big.NewInt(100000000))
	version := fmt.Sprintf("0.0.%d", randVer.Int64())
	data := core.DataRefOrValue{
		Value: value,
		Datatype: &core.DatatypeRef{
			Name:    "widget",
			Version: version,
		},
	}

	// Should be rejected as datatype not known
	resp, err := suite.testState.client1.BroadcastMessage(suite.T(), "topic1", "", &data, true)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 400, resp.StatusCode())
	assert.Contains(suite.T(), resp.String(), "FF10195") // datatype not found

	dt := &core.Datatype{
		Name:    "widget",
		Version: version,
		Value:   fftypes.JSONAnyPtrBytes(e2e.WidgetSchemaJSON),
	}
	suite.testState.client1.CreateDatatype(suite.T(), dt, true)

	resp, err = suite.testState.client1.BroadcastMessage(suite.T(), "topic1", "", &data, true)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 400, resp.StatusCode())
	assert.Contains(suite.T(), resp.String(), "FF10198") // does not conform

	data.Value = fftypes.JSONAnyPtr(`{
		"id": "widget12345",
		"name": "mywidget"
	}`)

	resp, err = suite.testState.client1.BroadcastMessage(suite.T(), "topic1", "", &data, true)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 200, resp.StatusCode())

	e2e.WaitForMessageConfirmed(suite.T(), received1, core.MessageTypeBroadcast)
	e2e.WaitForMessageConfirmed(suite.T(), received2, core.MessageTypeBroadcast)
}

func (suite *OnChainOffChainTestSuite) TestStrongDatatypesPrivate() {
	received1 := e2e.WsReader(suite.testState.ws1)
	received2 := e2e.WsReader(suite.testState.ws2)

	var resp *resty.Response
	value := fftypes.JSONAnyPtr(`{"foo":"bar"}`)
	randVer, _ := rand.Int(rand.Reader, big.NewInt(100000000))
	version := fmt.Sprintf("0.0.%d", randVer.Int64())
	data := core.DataRefOrValue{
		Value: value,
		Datatype: &core.DatatypeRef{
			Name:    "widget",
			Version: version,
		},
	}

	members := []core.MemberInput{
		{Identity: suite.testState.org1.Name},
		{Identity: suite.testState.org2.Name},
	}

	// Should be rejected as datatype not known
	resp, err := suite.testState.client1.PrivateMessage("topic1", "", &data, members, "", core.TransactionTypeBatchPin, true, suite.testState.startTime)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 400, resp.StatusCode())
	assert.Contains(suite.T(), resp.String(), "FF10195") // datatype not found

	dt := &core.Datatype{
		Name:    "widget",
		Version: version,
		Value:   fftypes.JSONAnyPtrBytes(e2e.WidgetSchemaJSON),
	}
	suite.testState.client1.CreateDatatype(suite.T(), dt, true)

	resp, err = suite.testState.client1.PrivateMessage("topic1", "", &data, members, "", core.TransactionTypeBatchPin, false, suite.testState.startTime)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 400, resp.StatusCode())
	assert.Contains(suite.T(), resp.String(), "FF10198") // does not conform

	data.Value = fftypes.JSONAnyPtr(`{
		"id": "widget12345",
		"name": "mywidget"
	}`)

	resp, err = suite.testState.client1.PrivateMessage("topic1", "", &data, members, "", core.TransactionTypeBatchPin, true, suite.testState.startTime)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 200, resp.StatusCode())

	e2e.WaitForMessageConfirmed(suite.T(), received1, core.MessageTypePrivate)
	e2e.WaitForMessageConfirmed(suite.T(), received2, core.MessageTypePrivate)
}

func (suite *OnChainOffChainTestSuite) TestE2EPrivate() {
	received1 := e2e.WsReader(suite.testState.ws1)
	received2 := e2e.WsReader(suite.testState.ws2)

	members := []core.MemberInput{
		{Identity: suite.testState.org1.Name},
		{Identity: suite.testState.org2.Name},
	}

	// Send 10 messages, that should get batched, across two topics
	totalMessages := 10
	topics := []string{"topicA", "topicB"}
	expectedData := make(map[string][]*core.DataRefOrValue)
	testUUID := fftypes.NewUUID()
	for i := 0; i < 10; i++ {
		value := fftypes.JSONAnyPtr(fmt.Sprintf(`"Hello number %d"`, i))
		data := &core.DataRefOrValue{
			Value: value,
		}
		topic := e2e.PickTopic(i, topics)

		expectedData[topic] = append(expectedData[topic], data)

		idempotencyKey := fmt.Sprintf("%s/%d", testUUID, i)

		resp, err := suite.testState.client1.PrivateMessage(topic, idempotencyKey, data, members, "", core.TransactionTypeBatchPin, false, suite.testState.startTime)
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), 202, resp.StatusCode())

		// Ensure idempotency
		resp, err = suite.testState.client1.PrivateMessage(topic, idempotencyKey, data, members, "", core.TransactionTypeBatchPin, false, suite.testState.startTime)
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), 409, resp.StatusCode())
	}

	for i := 0; i < totalMessages; i++ {
		// Wait for all thel message-confirmed events, from both participants
		e2e.WaitForMessageConfirmed(suite.T(), received1, core.MessageTypePrivate)
		e2e.WaitForMessageConfirmed(suite.T(), received2, core.MessageTypePrivate)
	}

	for topic, dataArray := range expectedData {
		receiver1data := validateReceivedMessages(suite.testState, suite.testState.client1, topic, core.MessageTypePrivate, core.TransactionTypeBatchPin, len(dataArray))
		receiver2data := validateReceivedMessages(suite.testState, suite.testState.client2, topic, core.MessageTypePrivate, core.TransactionTypeBatchPin, len(dataArray))
		// Messages should be returned in exactly reverse send order (newest first)
		for i := (len(dataArray) - 1); i >= 0; i-- {
			assert.Equal(suite.T(), dataArray[i].Value, receiver1data[i].Value)
			assert.Equal(suite.T(), dataArray[i].Value, receiver2data[i].Value)
		}
	}

}

func (suite *OnChainOffChainTestSuite) TestE2EBroadcastBlob() {
	received1 := e2e.WsReader(suite.testState.ws1)
	received2 := e2e.WsReader(suite.testState.ws2)

	var resp *resty.Response

	data, resp, err := suite.testState.client1.BroadcastBlobMessage(suite.T(), "topic1")
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 202, resp.StatusCode())

	e2e.WaitForMessageConfirmed(suite.T(), received1, core.MessageTypeBroadcast)
	val1 := validateReceivedMessages(suite.testState, suite.testState.client1, "topic1", core.MessageTypeBroadcast, core.TransactionTypeBatchPin, 1)
	assert.Regexp(suite.T(), "myfile.txt", val1[0].Value.String())
	assert.Equal(suite.T(), "myfile.txt", val1[0].Blob.Name)
	assert.Equal(suite.T(), data.Blob.Size, val1[0].Blob.Size)

	e2e.WaitForMessageConfirmed(suite.T(), received2, core.MessageTypeBroadcast)
	val2 := validateReceivedMessages(suite.testState, suite.testState.client2, "topic1", core.MessageTypeBroadcast, core.TransactionTypeBatchPin, 1)
	assert.Regexp(suite.T(), "myfile.txt", val2[0].Value.String())
	assert.Equal(suite.T(), "myfile.txt", val2[0].Blob.Name)
	assert.Equal(suite.T(), data.Blob.Size, val2[0].Blob.Size)

}

func (suite *OnChainOffChainTestSuite) TestE2EPrivateBlobDatatypeTagged() {
	received1 := e2e.WsReader(suite.testState.ws1)
	received2 := e2e.WsReader(suite.testState.ws2)

	var resp *resty.Response

	members := []core.MemberInput{
		{Identity: suite.testState.org1.Name},
		{Identity: suite.testState.org2.Name},
	}

	data, resp, err := suite.testState.client1.PrivateBlobMessageDatatypeTagged(suite.T(), "topic1", members, suite.testState.startTime)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 202, resp.StatusCode())
	assert.Empty(suite.T(), data.Blob.Name)
	assert.NotNil(suite.T(), data.Blob.Hash)

	e2e.WaitForMessageConfirmed(suite.T(), received1, core.MessageTypePrivate)
	res1 := validateReceivedMessages(suite.testState, suite.testState.client1, "topic1", core.MessageTypePrivate, core.TransactionTypeBatchPin, 1)
	assert.Equal(suite.T(), data.Blob.Hash.String(), res1[0].Blob.Hash.String())
	assert.Empty(suite.T(), res1[0].Blob.Name)
	assert.Equal(suite.T(), data.Blob.Size, res1[0].Blob.Size)

	e2e.WaitForMessageConfirmed(suite.T(), received2, core.MessageTypePrivate)
	res2 := validateReceivedMessages(suite.testState, suite.testState.client2, "topic1", core.MessageTypePrivate, core.TransactionTypeBatchPin, 1)
	assert.Equal(suite.T(), data.Blob.Hash.String(), res2[0].Blob.Hash.String())
	assert.Empty(suite.T(), res2[0].Blob.Name)
	assert.Equal(suite.T(), data.Blob.Size, res2[0].Blob.Size)
}

func (suite *OnChainOffChainTestSuite) TestE2EWebhookExchange() {
	received1 := e2e.WsReader(suite.testState.ws1)
	received2 := e2e.WsReader(suite.testState.ws2)

	subJSON := fmt.Sprintf(`{
		"transport": "webhooks",
		"namespace": "%s",
		"name": "myhook",
		"options": {
			"withData": true,
			"url": "https://raw.githubusercontent.com/hyperledger-firefly/firefly/main/test/data/config/firefly.core.yaml",
			"reply": true,
			"replytag": "myreply",
			"method": "GET"
		},
		"filter": {
			"tag": "myrequest"
		}
	}`, suite.testState.namespace)
	suite.testState.client2.CleanupExistingSubscription(suite.T(), suite.testState.namespace, "myhook")
	sub := suite.testState.client2.CreateSubscription(suite.T(), subJSON, 201)
	assert.NotNil(suite.T(), sub.ID)

	data := core.DataRefOrValue{
		Value: fftypes.JSONAnyPtr(`{}`),
	}

	members := []core.MemberInput{
		{Identity: suite.testState.org1.Name},
		{Identity: suite.testState.org2.Name},
	}

	var resp *resty.Response
	resp, err := suite.testState.client1.PrivateMessage("topic1", "", &data, members, "myrequest", core.TransactionTypeBatchPin, false, suite.testState.startTime)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 202, resp.StatusCode())

	e2e.WaitForMessageConfirmed(suite.T(), received1, core.MessageTypePrivate) // request 1
	e2e.WaitForMessageConfirmed(suite.T(), received2, core.MessageTypePrivate) // request 2
	e2e.WaitForMessageConfirmed(suite.T(), received1, core.MessageTypePrivate) // response 1
	e2e.WaitForMessageConfirmed(suite.T(), received2, core.MessageTypePrivate) // response 2

	// When we query the confirmed messages for each receiver, we will see the requests and responses.
	// We just check the reponses (index 1)

	receiver1vals := validateReceivedMessages(suite.testState, suite.testState.client1, "topic1", core.MessageTypePrivate, core.TransactionTypeBatchPin, 2)
	assert.Equal(suite.T(), float64(200), receiver1vals[1].Value.JSONObject()["status"])
	decoded1, err := base64.StdEncoding.DecodeString(receiver1vals[1].Value.JSONObject().GetString("body"))
	assert.NoError(suite.T(), err)
	assert.Regexp(suite.T(), "Example YAML", string(decoded1))

	receiver2vals := validateReceivedMessages(suite.testState, suite.testState.client2, "topic1", core.MessageTypePrivate, core.TransactionTypeBatchPin, 2)
	assert.Equal(suite.T(), float64(200), receiver2vals[1].Value.JSONObject()["status"])
	decoded2, err := base64.StdEncoding.DecodeString(receiver2vals[1].Value.JSONObject().GetString("body"))
	assert.NoError(suite.T(), err)
	assert.Regexp(suite.T(), "Example YAML", string(decoded2))
}

func (suite *OnChainOffChainTestSuite) TestE2EWebhookRequestReplyNoTx() {
	subJSON := fmt.Sprintf(`{
		"transport": "webhooks",
		"namespace": "%s",
		"name": "myhook",
		"options": {
			"withData": true,
			"url": "https://github.com/hyperledger-firefly/firefly/raw/main/resources/ff-logo-32.png",
			"reply": true,
			"replytag": "myreply",
			"replytx": "none",
			"method": "GET"
		},
		"filter": {
			"tag": "myrequest"
		}
	}`, suite.testState.namespace)
	suite.testState.client2.CleanupExistingSubscription(suite.T(), suite.testState.namespace, "myhook")
	sub := suite.testState.client2.CreateSubscription(suite.T(), subJSON, 201)
	assert.NotNil(suite.T(), sub.ID)

	data := core.DataRefOrValue{
		Value: fftypes.JSONAnyPtr(`{}`),
	}

	reply := suite.testState.client1.RequestReply(suite.T(), &data, []string{
		suite.testState.org1.Name,
		suite.testState.org2.Name,
	}, "myrequest", core.TransactionTypeUnpinned, suite.testState.startTime)
	assert.NotNil(suite.T(), reply)

	bodyData := reply.InlineData[0].Value.JSONObject().GetString("body")
	b, err := base64.StdEncoding.DecodeString(bodyData)
	assert.NoError(suite.T(), err)
	ffImg, err := png.Decode(bytes.NewReader(b))
	assert.NoError(suite.T(), err)

	// Verify we got the right data back by parsing it
	convertOptions := image2ascii.DefaultOptions
	convertOptions.FixedWidth = 100
	convertOptions.FixedHeight = 60
	convertOptions.Colored = false
	converter := image2ascii.NewImageConverter()
	str := converter.Image2ASCIIString(ffImg, &convertOptions)
	for _, s := range strings.Split(str, "\n") {
		if len(strings.TrimSpace(s)) > 0 {
			fmt.Println(s)
		}
	}

}
