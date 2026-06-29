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
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sort"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
)

type GroupIdentity struct {
	Namespace string  `ffstruct:"Group" json:"namespace,omitempty"`
	Name      string  `ffstruct:"Group" json:"name"`
	Members   Members `ffstruct:"Group" json:"members"`
}

type Group struct {
	GroupIdentity
	LocalNamespace string           `ffstruct:"Group" json:"localNamespace,omitempty"`
	Message        *fftypes.UUID    `ffstruct:"Group" json:"message,omitempty"`
	Hash           *fftypes.Bytes32 `ffstruct:"Group" json:"hash,omitempty"`
	Created        *fftypes.FFTime  `ffstruct:"Group" json:"created,omitempty"`
}

type Members []*Member

func (m Members) Len() int           { return len(m) }
func (m Members) Swap(i, j int)      { m[i], m[j] = m[j], m[i] }
func (m Members) Less(i, j int) bool { return m[i].Identity < m[j].Identity } // Note there's a dupcheck in validate

type Member struct {
	Identity string        `ffstruct:"Member" json:"identity,omitempty"`
	Node     *fftypes.UUID `ffstruct:"Member" json:"node,omitempty"`
}

func (m *Member) Equals(m2 *Member) bool {
	switch {
	case m == nil && m2 == nil:
		return true
	case m == nil || m2 == nil:
		return false
	default:
		return m.Identity == m2.Identity && m.Node.Equals(m2.Node)
	}
}

type MemberInput struct {
	Identity string `ffstruct:"MemberInput" json:"identity,omitempty"`
	Node     string `ffstruct:"MemberInput" json:"node,omitempty"`
}

func (man *GroupIdentity) Hash() *fftypes.Bytes32 {
	b, _ := json.Marshal(&man)
	hash := fftypes.Bytes32(sha256.Sum256(b))
	return &hash
}

func (group *Group) Validate(ctx context.Context, existing bool) (err error) {
	if err = fftypes.ValidateFFNameField(ctx, group.Namespace, "namespace"); err != nil {
		return err
	}
	// We allow a blank name for a group (for auto creation)
	if group.Name != "" {
		if err = fftypes.ValidateFFNameFieldNoUUID(ctx, group.Name, "name"); err != nil {
			return err
		}
	}
	if len(group.Members) == 0 {
		return i18n.NewError(ctx, i18n.MsgGroupMustHaveMembers)
	}
	dupCheck := make(map[string]bool)
	for i, r := range group.Members {
		if r.Identity == "" {
			return i18n.NewError(ctx, i18n.MsgEmptyMemberIdentity, i)
		}
		if err = fftypes.ValidateLength(ctx, r.Identity, "identity", 1024); err != nil {
			return err
		}
		if r.Node == nil {
			return i18n.NewError(ctx, i18n.MsgEmptyMemberNode, i)
		}
		key := fmt.Sprintf("%s:%s", r.Node, r.Identity)
		if dupCheck[key] {
			return i18n.NewError(ctx, i18n.MsgDuplicateMember, i, key)
		}
		dupCheck[key] = true
	}
	if existing {
		hash := group.GroupIdentity.Hash()
		if !group.Hash.Equals(hash) {
			return i18n.NewError(ctx, i18n.MsgGroupInvalidHash, group.Hash, hash)
		}
	}
	return nil
}

func (group *Group) Seal() {
	sort.Sort(group.Members)
	group.Hash = group.GroupIdentity.Hash()
}

func (group *Group) Topic() string {
	return group.Hash.String()
}

func (group *Group) SetBroadcastMessage(msgID *fftypes.UUID) {
	group.Message = msgID
}
