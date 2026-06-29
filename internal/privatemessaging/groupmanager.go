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

package privatemessaging

import (
	"context"
	"encoding/json"

	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/cache"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/internal/data"
	"github.com/hyperledger-firefly/firefly/internal/identity"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

type GroupManager interface {
	GetGroupByID(ctx context.Context, id string) (*core.Group, error)
	GetGroups(ctx context.Context, filter ffapi.AndFilter) ([]*core.Group, *ffapi.FilterResult, error)
	ResolveInitGroup(ctx context.Context, msg *core.Message, creator *core.Member) (*core.Group, error)
	EnsureLocalGroup(ctx context.Context, group *core.Group, creator *core.Member) (ok bool, err error)
}

type groupManager struct {
	namespace  *core.Namespace
	database   database.Plugin
	identity   identity.Manager
	data       data.Manager
	groupCache cache.CInterface
}

type groupHashEntry struct {
	group *core.Group
	nodes []*core.Identity
}

// EnsureLocalGroup is called for unpinned messages, which carry the full group definition next to the message batch.
// The group simply needs to be validated and stored (or loaded if it exists already).
func (gm *groupManager) EnsureLocalGroup(ctx context.Context, group *core.Group, creator *core.Member) (ok bool, err error) {
	if g, err := gm.database.GetGroupByHash(ctx, gm.namespace.Name, group.Hash); err != nil {
		return false, err
	} else if g != nil {
		return true, nil
	}

	err = group.Validate(ctx, true)
	if err != nil {
		log.L(ctx).Errorf("Attempt to insert invalid group %s: %s", group.Hash, err)
		return false, nil
	}
	if !gm.groupContains(ctx, group, creator) {
		return false, nil
	}

	err = gm.database.UpsertGroup(ctx, group, database.UpsertOptimizationNew /* it could have been created by another thread, but we think we're first */)
	if err != nil {
		return false, err
	}
	return true, nil
}

func (gm *groupManager) groupInit(ctx context.Context, signer *core.SignerRef, group *core.Group) (err error) {

	// Serialize it into a data object, as a piece of data we can write to a message
	data := &core.Data{
		Validator: core.ValidatorTypeSystemDefinition,
		ID:        fftypes.NewUUID(),
		Namespace: gm.namespace.Name, // must go in the same ordering context as the message
		Created:   fftypes.Now(),
	}
	b, err := json.Marshal(&group)
	if err == nil {
		data.Value = fftypes.JSONAnyPtrBytes(b)
		err = group.Validate(ctx, true)
		if err == nil {
			err = data.Seal(ctx, nil)
		}
	}
	if err != nil {
		return i18n.WrapError(ctx, err, coremsgs.MsgSerializationFailed)
	}
	group.LocalNamespace = gm.namespace.Name

	// Ensure all group members are valid
	for _, member := range group.Members {
		node, err := gm.identity.CachedIdentityLookupByID(ctx, member.Node)
		if err != nil {
			return err
		}
		org, _, err := gm.identity.CachedIdentityLookupMustExist(ctx, member.Identity)
		if err != nil {
			return err
		}
		valid, err := gm.identity.ValidateNodeOwner(ctx, node, org)
		if err != nil {
			return err
		} else if !valid {
			return i18n.NewError(ctx, coremsgs.MsgInvalidGroupMember, node.DID, member.Identity)
		}
	}

	// Create a private send message referring to the data
	msg := &core.Message{
		State:          core.MessageStateReady,
		LocalNamespace: gm.namespace.Name, // Must go into the same ordering context as the message itself
		Header: core.MessageHeader{
			Group:     group.Hash,
			Namespace: gm.namespace.NetworkName,
			Type:      core.MessageTypeGroupInit,
			SignerRef: *signer,
			Tag:       core.SystemTagDefineGroup,
			Topics:    fftypes.FFStringArray{group.Topic()},
			TxType:    core.TransactionTypeBatchPin,
		},
		Data: core.DataRefs{
			{ID: data.ID, Hash: data.Hash},
		},
	}
	if err = msg.Seal(ctx); err == nil {
		err = gm.database.RunAsGroup(ctx, func(ctx context.Context) error {
			// Write as data to the local store
			if err = gm.database.UpsertData(ctx, data, database.UpsertOptimizationNew); err != nil {
				return err
			}

			// Store the message - this asynchronously triggers the next step in process
			if err = gm.database.UpsertMessage(ctx, msg, database.UpsertOptimizationNew); err != nil {
				return err
			}

			// Write the unconfirmed group directly to our database, so it can be used straight away.
			// We're able to do this by making the identifier of the group a hash of the identity fields
			// (name, ledger and member list), as that is all the group contains. There's no data in there.
			return gm.database.UpsertGroup(ctx, group, database.UpsertOptimizationNew /* we think we're first */)
		})
		if err == nil {
			log.L(ctx).Infof("Created new group %s", group.Hash)
		}
	}
	return err
}

func (gm *groupManager) GetGroupByID(ctx context.Context, hash string) (*core.Group, error) {
	h, err := fftypes.ParseBytes32(ctx, hash)
	if err != nil {
		return nil, err
	}
	return gm.database.GetGroupByHash(ctx, gm.namespace.Name, h)
}

func (gm *groupManager) GetGroups(ctx context.Context, filter ffapi.AndFilter) ([]*core.Group, *ffapi.FilterResult, error) {
	return gm.database.GetGroups(ctx, gm.namespace.Name, filter)
}

func (gm *groupManager) getGroupNodes(ctx context.Context, groupHash *fftypes.Bytes32, allowNil bool) (*core.Group, []*core.Identity, error) {

	if cachedValue := gm.groupCache.Get(groupHash.String()); cachedValue != nil {
		ghe := cachedValue.(*groupHashEntry)
		return ghe.group, ghe.nodes, nil
	}

	group, err := gm.database.GetGroupByHash(ctx, gm.namespace.Name, groupHash)
	if err != nil || (allowNil && group == nil) {
		return nil, nil, err
	}
	if group == nil {
		return nil, nil, i18n.NewError(ctx, coremsgs.MsgGroupNotFound, groupHash)
	}

	// We de-duplicate nodes in the case that the payload needs to be received by multiple org identities
	// that share a single node.
	nodes := make([]*core.Identity, 0, len(group.Members))
	knownIDs := make(map[fftypes.UUID]bool)
	for _, r := range group.Members {
		node, err := gm.identity.CachedIdentityLookupByID(ctx, r.Node)
		if err != nil {
			return nil, nil, err
		}
		if node == nil || node.Type != core.IdentityTypeNode {
			return nil, nil, i18n.NewError(ctx, coremsgs.MsgNodeNotFound, r.Node)
		}
		if !knownIDs[*node.ID] {
			knownIDs[*node.ID] = true
			nodes = append(nodes, node)
		}
	}

	gm.groupCache.Set(group.Hash.String(), &groupHashEntry{
		group: group,
		nodes: nodes,
	})
	return group, nodes, nil
}

// ResolveInitGroup is called when a message comes in as the first private message on a particular context.
// If the message is a group creation request, then it is validated and the group is created.
// Otherwise, the existing group must exist.
//
// Errors are only returned for database issues. For validation issues, a nil group is returned without an error.
func (gm *groupManager) ResolveInitGroup(ctx context.Context, msg *core.Message, member *core.Member) (*core.Group, error) {
	if msg.Header.Tag == core.SystemTagDefineGroup {
		// Store the new group
		data, foundAll, err := gm.data.GetMessageDataCached(ctx, msg)
		if err != nil || !foundAll || len(data) == 0 {
			log.L(ctx).Warnf("Group %s definition in message %s invalid: missing data", msg.Header.Group, msg.Header.ID)
			return nil, err
		}
		var newGroup core.Group
		err = json.Unmarshal(data[0].Value.Bytes(), &newGroup)
		if err != nil {
			log.L(ctx).Warnf("Group %s definition in message %s invalid: %s", msg.Header.Group, msg.Header.ID, err)
			return nil, nil
		}
		err = newGroup.Validate(ctx, true)
		if err != nil {
			log.L(ctx).Warnf("Group %s definition in message %s invalid: %s", msg.Header.Group, msg.Header.ID, err)
			return nil, nil
		}
		if !newGroup.Hash.Equals(msg.Header.Group) {
			log.L(ctx).Warnf("Group %s definition in message %s invalid: mismatched hash with message '%s'", msg.Header.Group, msg.Header.ID, newGroup.Hash)
			return nil, nil
		}
		newGroup.Message = msg.Header.ID
		newGroup.LocalNamespace = gm.namespace.Name
		err = gm.database.UpsertGroup(ctx, &newGroup, database.UpsertOptimizationNew /* we think we're first to create this */)
		if err != nil {
			return nil, err
		}
		return &newGroup, nil
	}

	// Get the existing group
	group, err := gm.database.GetGroupByHash(ctx, gm.namespace.Name, msg.Header.Group)
	if err != nil {
		return group, err
	}
	if group == nil {
		log.L(ctx).Warnf("Group %s not found for first message in context. type=%s", msg.Header.Group, msg.Header.Type)
		return nil, nil
	}

	if !gm.groupContains(ctx, group, member) {
		return nil, nil
	}
	return group, nil
}

func (gm *groupManager) groupContains(ctx context.Context, group *core.Group, member *core.Member) (valid bool) {
	for _, m := range group.Members {
		if m.Identity == member.Identity && m.Node.Equals(member.Node) {
			return true
		}
	}
	log.L(ctx).Errorf("Group '%s' does not contain member identity=%s node=%s", group.Hash, member.Identity, member.Node)
	return false
}
