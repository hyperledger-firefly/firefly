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

package core

import (
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/stretchr/testify/assert"
)

func TestNewEvent(t *testing.T) {

	ref := fftypes.NewUUID()
	tx := fftypes.NewUUID()
	e := NewEvent(EventTypeMessageConfirmed, "ns1", ref, tx, "topic1")
	assert.Equal(t, EventTypeMessageConfirmed, e.Type)
	assert.Equal(t, "ns1", e.Namespace)
	assert.Equal(t, *ref, *e.Reference)
	assert.Equal(t, *tx, *e.Transaction)
	assert.Equal(t, "topic1", e.Topic)

	e.Sequence = 12345
	var ls LocallySequenced = e
	assert.Equal(t, int64(12345), ls.LocalSequence())

}
