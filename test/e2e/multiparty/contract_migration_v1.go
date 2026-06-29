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
	"github.com/hyperledger-firefly/firefly/test/e2e"
	"github.com/hyperledger-firefly/firefly/test/e2e/client"
)

type ContractMigrationV1TestSuite struct {
	ContractMigrationTestSuite
}

func (suite *ContractMigrationV1TestSuite) BeforeTest(suiteName, testName string) {
	suite.testState = beforeE2ETest(suite.T())
}

func (suite *ContractMigrationV1TestSuite) AfterTest(suiteName, testName string) {
	e2e.VerifyAllOperationsSucceeded(suite.T(), []*client.FireFlyClient{suite.testState.client1, suite.testState.client2}, suite.testState.startTime)
	suite.testState.done()
}

func (suite *ContractMigrationV1TestSuite) TestContractMigration() {
	defer suite.testState.Done()

	address1 := deployContract(suite.T(), suite.testState.stackName, "firefly/FireflyV1.json")
	address2 := deployContract(suite.T(), suite.testState.stackName, "firefly/Firefly.json")
	runMigrationTest(&suite.ContractMigrationTestSuite, address1, address2, true)
}
