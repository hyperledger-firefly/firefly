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
	"context"
	"encoding/json"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/stretchr/testify/assert"
)

func TestMemberEquals(t *testing.T) {
	member1 := &Member{
		Identity: "id1",
		Node:     fftypes.NewUUID(),
	}
	member2 := &Member{
		Identity: "id1",
		Node:     fftypes.NewUUID(),
	}
	assert.False(t, member1.Equals(member2))

	*member2.Node = *member1.Node
	assert.True(t, member1.Equals(member2))

	member2 = nil
	assert.False(t, member1.Equals(member2))
	assert.True(t, member2.Equals(nil))
}

func TestGroupValidation(t *testing.T) {

	group := &Group{
		GroupIdentity: GroupIdentity{
			Namespace: "ok",
			Name:      "!wrong",
		},
	}
	assert.Regexp(t, "FF00140.*name", group.Validate(context.Background(), false))

	group = &Group{
		GroupIdentity: GroupIdentity{
			Name:      "ok",
			Namespace: "!wrong",
		},
	}
	assert.Regexp(t, "FF00140.*namespace", group.Validate(context.Background(), false))

	group = &Group{
		GroupIdentity: GroupIdentity{
			Name:      "ok",
			Namespace: "ok",
		},
	}
	assert.Regexp(t, "FF00115.*member", group.Validate(context.Background(), false))

	group = &Group{
		GroupIdentity: GroupIdentity{
			Name:      "ok",
			Namespace: "ok",
			Members:   Members{{Node: fftypes.NewUUID()}},
		},
	}
	assert.Regexp(t, "FF00116.*member", group.Validate(context.Background(), false))

	group = &Group{
		GroupIdentity: GroupIdentity{
			Name:      "ok",
			Namespace: "ok",
			Members:   Members{{Identity: "0x12345"}},
		},
	}
	assert.Regexp(t, "FF00117.*member", group.Validate(context.Background(), false))

	group = &Group{
		GroupIdentity: GroupIdentity{
			Name:      "ok",
			Namespace: "ok",
			Members:   Members{{Identity: string(make([]byte, 1025)), Node: fftypes.NewUUID()}},
		},
	}
	assert.Regexp(t, "FF00135.*identity", group.Validate(context.Background(), false))

	group = &Group{
		GroupIdentity: GroupIdentity{
			Name:      "ok",
			Namespace: "ok",
			Members:   Members{{Identity: "0x12345", Node: fftypes.NewUUID()}},
		},
	}
	assert.NoError(t, group.Validate(context.Background(), false))

	assert.Regexp(t, "FF00119", group.Validate(context.Background(), true))
	group.Seal()
	assert.NoError(t, group.Validate(context.Background(), true))

	group = &Group{
		GroupIdentity: GroupIdentity{
			Name:      "ok",
			Namespace: "ok",
			Members:   Members{{ /* blank */ }},
		},
	}
	assert.Regexp(t, "FF00116", group.Validate(context.Background(), false))

	nodeID := fftypes.MustParseUUID("8b5c0d39-925f-4579-9c60-54f3e846ab99")
	group = &Group{
		GroupIdentity: GroupIdentity{
			Name:      "ok",
			Namespace: "ok",
			Members: Members{
				{Node: nodeID, Identity: "0x12345"},
				{Node: nodeID, Identity: "0x12345"},
			},
		},
	}
	assert.Regexp(t, "FF00118", group.Validate(context.Background(), false))

	group.Members = Members{
		{Node: nodeID, Identity: "0x12345"},
	}
	b, _ := json.Marshal(&group.Members)
	assert.Equal(t, `[{"identity":"0x12345","node":"8b5c0d39-925f-4579-9c60-54f3e846ab99"}]`, string(b))
	group.Seal()
	assert.Equal(t, "c2e5a42207ce2b48d67a4682a96f914ec0386acc13615550aa503b0841da8824", group.Hash.String())

	var def Definition = group
	assert.Equal(t, group.Hash.String(), def.Topic())
	def.SetBroadcastMessage(fftypes.NewUUID())
	assert.NotNil(t, group.Message)
}

func TestGroupSealSorting(t *testing.T) {

	m1 := &Member{Node: fftypes.NewUUID(), Identity: "0x11111"}
	m2 := &Member{Node: fftypes.NewUUID(), Identity: "0x22222"}
	m3 := &Member{Node: fftypes.NewUUID(), Identity: "0x33333"}
	m4 := &Member{Node: fftypes.NewUUID(), Identity: "0x44444"}
	m5 := &Member{Node: fftypes.NewUUID(), Identity: "0x55555"}

	group1 := &Group{
		GroupIdentity: GroupIdentity{
			Name:      "name1",
			Namespace: "ns1",
			Members:   Members{m1, m2, m3, m4, m5},
		},
	}
	group2 := &Group{
		GroupIdentity: GroupIdentity{
			Name:      "name1",
			Namespace: "ns1",
			Members:   Members{m4, m3, m1, m2, m5},
		},
	}
	group1.Seal()
	group2.Seal()

	assert.Equal(t, *group1.Hash, *group2.Hash)

}
