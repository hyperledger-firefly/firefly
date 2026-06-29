// Copyright © 2021 Kaleido, Inc.
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

package orchestrator

import (
	"context"
	"testing"

	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
)

func TestRequestReplyMissingGroup(t *testing.T) {
	or := newTestOrchestrator()
	input := &core.MessageInOut{}
	_, err := or.RequestReply(context.Background(), input)
	assert.Regexp(t, "FF10271", err)
}

func TestRequestReply(t *testing.T) {
	or := newTestOrchestrator()
	input := &core.MessageInOut{
		Group: &core.InputGroup{
			Members: []core.MemberInput{
				{Identity: "org1"},
			},
		},
	}
	or.mpm.On("RequestReply", context.Background(), input).Return(&core.MessageInOut{}, nil)
	_, err := or.RequestReply(context.Background(), input)
	assert.NoError(t, err)
}
