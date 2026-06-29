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
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/test/e2e"
	"github.com/hyperledger-firefly/firefly/test/e2e/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type IdentityTestSuite struct {
	suite.Suite
	testState *testState
}

func (suite *IdentityTestSuite) BeforeTest(suiteName, testName string) {
	suite.testState = beforeE2ETest(suite.T())
}

func (suite *IdentityTestSuite) AfterTest(suiteName, testName string) {
	e2e.VerifyAllOperationsSucceeded(suite.T(), []*client.FireFlyClient{suite.testState.client1, suite.testState.client2}, suite.testState.startTime)
	suite.testState.done()
}

func (suite *IdentityTestSuite) TestCustomChildIdentityBroadcasts() {
	ctx := context.Background()
	received1 := e2e.WsReader(suite.testState.ws1)
	received2 := e2e.WsReader(suite.testState.ws2)

	totalIdentities := 2
	ts := time.Now().Unix()
	for i := 0; i < totalIdentities; i++ {
		key := getUnregisteredAccount(suite, suite.testState.org1.Name)
		suite.testState.client1.ClaimCustomIdentity(suite.T(),
			key,
			fmt.Sprintf("custom_%d_%d", ts, i),
			fmt.Sprintf("Description %d", i),
			fftypes.JSONObject{"profile": i},
			suite.testState.org1.ID,
			false)
	}

	identityIDs := make(map[fftypes.UUID]bool)
	for i := 0; i < totalIdentities; i++ {
		ed := e2e.WaitForIdentityConfirmed(suite.T(), received1)
		identityIDs[*ed.Reference] = true
		suite.T().Logf("Received node 1 confirmation of identity %s", ed.Reference)
		ed = e2e.WaitForIdentityConfirmed(suite.T(), received2)
		identityIDs[*ed.Reference] = true
		suite.T().Logf("Received node 2 confirmation of identity %s", ed.Reference)
	}
	assert.Len(suite.T(), identityIDs, totalIdentities)

	identities := make(map[string]*core.Identity)
	for identityID := range identityIDs {
		identityNode1 := suite.testState.client1.GetIdentity(suite.T(), &identityID)
		identityNode2 := suite.testState.client2.GetIdentity(suite.T(), &identityID)
		assert.True(suite.T(), identityNode1.IdentityBase.Equals(ctx, &identityNode2.IdentityBase))
		identities[identityNode1.DID] = identityNode1
	}

	// Send a broadcast from each custom identity
	for did := range identities {
		resp, err := suite.testState.client1.BroadcastMessageAsIdentity(suite.T(), did, "identitytest", "", &core.DataRefOrValue{
			Value: fftypes.JSONAnyPtr(`{"some": "data"}`),
		}, false)
		require.NoError(suite.T(), err)
		assert.Equal(suite.T(), 202, resp.StatusCode())
	}
	for range identities {
		e2e.WaitForMessageConfirmed(suite.T(), received1, core.MessageTypeBroadcast)
		e2e.WaitForMessageConfirmed(suite.T(), received2, core.MessageTypeBroadcast)
	}

}

func (suite *IdentityTestSuite) TestCustomChildIdentityPrivate() {
	received1 := e2e.WsReader(suite.testState.ws1)
	received2 := e2e.WsReader(suite.testState.ws2)

	org1key := getUnregisteredAccount(suite, suite.testState.org1.Name)
	org2key := getUnregisteredAccount(suite, suite.testState.org2.Name)

	ts := time.Now().Unix()
	custom1 := suite.testState.client1.ClaimCustomIdentity(suite.T(),
		org1key,
		fmt.Sprintf("custom_%d_org1priv", ts),
		"Description org1priv",
		nil,
		suite.testState.org1.ID,
		true)
	custom2 := suite.testState.client2.ClaimCustomIdentity(suite.T(),
		org2key,
		fmt.Sprintf("custom_%d_org2priv", ts),
		"Description org2priv",
		nil,
		suite.testState.org2.ID,
		true)
	for i := 0; i < 2; i++ {
		e2e.WaitForIdentityConfirmed(suite.T(), received1)
		e2e.WaitForIdentityConfirmed(suite.T(), received2)
	}

	members := []core.MemberInput{
		{Identity: custom1.DID},
		{Identity: custom2.DID},
	}

	resp, err := suite.testState.client1.PrivateMessageWithKey(org1key, "topic1", "", &core.DataRefOrValue{
		Value: fftypes.JSONAnyPtr(`"test private custom identity"`),
	}, members, "tag1", core.TransactionTypeBatchPin, true, suite.testState.startTime)
	require.NoError(suite.T(), err)
	assert.Equal(suite.T(), 200, resp.StatusCode())

	e2e.WaitForMessageConfirmed(suite.T(), received1, core.MessageTypePrivate)
	e2e.WaitForMessageConfirmed(suite.T(), received2, core.MessageTypePrivate)
}

func (suite *IdentityTestSuite) TestInvalidIdentityAlreadyRegistered() {
	received1 := e2e.WsReader(suite.testState.ws1)

	key := getUnregisteredAccount(suite, suite.testState.org1.Name)
	require.NotEqual(suite.T(), "", key)
	ts := time.Now().Unix()

	suite.testState.client1.ClaimCustomIdentity(suite.T(),
		key,
		fmt.Sprintf("custom_already_registered_%d_1", ts),
		"Description 1",
		fftypes.JSONObject{"profile": 1},
		suite.testState.org1.ID,
		false)

	e2e.WaitForIdentityConfirmed(suite.T(), received1)

	suite.testState.client1.ClaimCustomIdentity(suite.T(),
		key,
		fmt.Sprintf("custom_already_registered_%d_2", ts),
		"Description 2",
		fftypes.JSONObject{"profile": 2},
		suite.testState.org1.ID,
		false)

	e2e.WaitForMessageRejected(suite.T(), received1, core.MessageTypeDefinition)
}

func getUnregisteredAccount(suite *IdentityTestSuite, orgName string) string {
	verifiers := suite.testState.client1.GetVerifiers(suite.T())
	suite.T().Logf("checking for account with orgName: %s", orgName)
	for i, account := range suite.testState.unregisteredAccounts {
		alreadyRegistered := false
		accountMap := account.(map[string]interface{})
		suite.T().Logf("account from stackState has orgName: %s", accountMap["orgName"])
		if accountOrgName, ok := accountMap["orgName"]; ok {
			if accountOrgName != orgName {
				// An orgName was present and it wasn't what we were looking for. Skip.
				continue
			}
		}
		var key string
		if k, ok := accountMap["address"]; ok {
			key = k.(string)
		}
		if k, ok := accountMap["name"]; ok {
			key = k.(string)
		}

		suite.T().Logf("checking to make sure key '%s' is not registered", key)
		if len(verifiers) > 0 {
			for _, verifier := range verifiers {

				suite.T().Logf("account name/address: %s", key)
				suite.T().Logf("verifier value: %s", verifier.Value)
				if strings.Contains(verifier.Value, key) {
					// Already registered. Look at the next account
					alreadyRegistered = true
					break
				}
			}
		}
		if !alreadyRegistered {
			suite.testState.unregisteredAccounts = append(suite.testState.unregisteredAccounts[:i], suite.testState.unregisteredAccounts[i+1:]...)
			return key
		}
	}
	suite.T().Logf("could not find an unregistered account!")
	return ""
}
