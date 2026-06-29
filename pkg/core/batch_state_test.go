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

package core

import (
	"context"
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/stretchr/testify/assert"
)

func TestBatchStateFinalizers(t *testing.T) {
	bs := BatchState{}

	run1 := false
	bs.AddPreFinalize(func(ctx context.Context) error { run1 = true; return nil })
	run2 := false
	bs.AddFinalize(func(ctx context.Context) error { run2 = true; return nil })

	bs.RunPreFinalize(context.Background())
	bs.RunFinalize(context.Background())
	assert.True(t, run1)
	assert.True(t, run2)
}

func TestBatchStateFinalizerErrors(t *testing.T) {
	bs := BatchState{}

	bs.AddPreFinalize(func(ctx context.Context) error { return fmt.Errorf("pop") })
	bs.AddFinalize(func(ctx context.Context) error { return fmt.Errorf("pop") })

	err := bs.RunPreFinalize(context.Background())
	assert.EqualError(t, err, "pop")
	err = bs.RunFinalize(context.Background())
	assert.EqualError(t, err, "pop")
}

func TestBatchStateIdentities(t *testing.T) {
	bs := BatchState{
		PendingConfirms: make(map[fftypes.UUID]*Message),
	}

	id := fftypes.NewUUID()
	msg := &Message{}
	bs.AddPendingConfirm(id, msg)

	did := "did:firefly:id1"
	bs.AddConfirmedDIDClaim(did)

	assert.Equal(t, msg, bs.PendingConfirms[*id])
	assert.Equal(t, bs.ConfirmedDIDClaims[0], did)
}
