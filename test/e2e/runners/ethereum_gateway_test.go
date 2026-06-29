// Copyright © 2022 Kaleido, Inc.
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

package runners

import (
	"testing"

	"github.com/hyperledger-firefly/firefly/test/e2e/gateway"
	"github.com/stretchr/testify/suite"
)

func TestEthereumGatewayE2ESuite(t *testing.T) {
	suite.Run(t, new(gateway.TokensTestSuite))
	suite.Run(t, new(gateway.EthereumCouponTestSuite))
	suite.Run(t, new(gateway.EthereumSimpleStorageTestSuite))
	suite.Run(t, new(gateway.EthereumRevertTestSuite))
	suite.Run(t, new(gateway.TokensOnlyTestSuite))
}

func TestEthereumGatewayLegacyEthE2ESuite(t *testing.T) {
	// Note EthereumRevertTestSuite does not work with legacy EthConnect, as the
	// submissionRejected boolean is only supported by the EVMConnect (FFTM) generation.
	suite.Run(t, new(gateway.TokensTestSuite))
	suite.Run(t, new(gateway.EthereumCouponTestSuite))
	suite.Run(t, new(gateway.EthereumSimpleStorageTestSuite))
	suite.Run(t, new(gateway.TokensOnlyTestSuite))
}
