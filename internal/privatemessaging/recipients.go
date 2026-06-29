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
	"fmt"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

func (pm *privateMessaging) resolveRecipientList(ctx context.Context, in *core.MessageInOut) error {
	if in.Header.Group != nil {
		log.L(ctx).Debugf("Group '%s' specified for message", in.Header.Group)
		group, err := pm.database.GetGroupByHash(ctx, pm.namespace.Name, in.Header.Group)
		if err != nil {
			return err
		}
		if group == nil {
			return i18n.NewError(ctx, coremsgs.MsgGroupNotFound, in.Header.Group)
		}
		// We have a group already resolved
		return nil
	}
	if in.Group == nil || len(in.Group.Members) == 0 {
		return i18n.NewError(ctx, i18n.MsgGroupMustHaveMembers)
	}
	group, isNew, err := pm.findOrGenerateGroup(ctx, in)
	if err != nil {
		return err
	}
	log.L(ctx).Debugf("Resolved group '%s' for message. New=%t", group.Hash, isNew)
	in.Message.Header.Group = group.Hash

	// If the group is new, we need to do a group initialization, before we send the message itself.
	if isNew {
		return pm.groupManager.groupInit(ctx, &in.Header.SignerRef, group)
	}
	return err
}

func (pm *privateMessaging) getFirstNodeForOrg(ctx context.Context, identity *core.Identity) (*core.Identity, error) {
	key := fmt.Sprintf("ns=%s,did=%s", identity.Namespace, identity.DID)
	node := pm.orgFirstNodes[key]
	if node == nil && identity.Type == core.IdentityTypeOrg {
		fb := database.IdentityQueryFactory.NewFilterLimit(ctx, 1)
		filter := fb.And(
			fb.Eq("parent", identity.ID),
			fb.Eq("type", core.IdentityTypeNode),
		)
		nodes, _, err := pm.database.GetIdentities(ctx, identity.Namespace, filter)
		if err != nil || len(nodes) == 0 {
			return nil, err
		}
		node = nodes[0]
		pm.orgFirstNodes[key] = node
	}
	return node, nil
}

func (pm *privateMessaging) resolveNode(ctx context.Context, identity *core.Identity, nodeInput string) (node *core.Identity, err error) {
	retryable := true
	if nodeInput != "" {
		node, retryable, err = pm.identity.CachedIdentityLookupMustExist(ctx, nodeInput)
	} else {
		// Find any node owned by this organization
		inputIdentityDebugInfo := fmt.Sprintf("%s (%s)", identity.DID, identity.ID)
		for identity != nil && node == nil {
			node, err = pm.getFirstNodeForOrg(ctx, identity)
			switch {
			case err == nil && node != nil:
				// This is an org, and it owns a node
			case err == nil && identity.Parent != nil:
				// This identity has a parent, maybe that org owns a node
				identity, err = pm.identity.CachedIdentityLookupByID(ctx, identity.Parent)
			default:
				return nil, i18n.NewError(ctx, coremsgs.MsgNodeNotFoundInOrg, inputIdentityDebugInfo)
			}
		}
	}
	if err != nil && retryable {
		return nil, err
	}
	if node == nil {
		return nil, i18n.NewError(ctx, coremsgs.MsgNodeNotFound, nodeInput)
	}
	return node, nil
}

func (pm *privateMessaging) getRecipients(ctx context.Context, in *core.MessageInOut) (gi *core.GroupIdentity, err error) {

	localOrg, err := pm.identity.GetRootOrg(ctx)
	if err != nil {
		return nil, err
	}

	foundLocal := false
	localNode, err := pm.identity.GetLocalNode(ctx)
	if err != nil {
		return nil, err
	}
	gi = &core.GroupIdentity{
		Namespace: in.Message.Header.Namespace,
		Name:      in.Group.Name,
		Members:   make(core.Members, len(in.Group.Members)),
	}
	for i, rInput := range in.Group.Members {
		// Resolve the identity
		identity, _, err := pm.identity.CachedIdentityLookupMustExist(ctx, rInput.Identity)
		if err != nil {
			return nil, err
		}
		// Resolve the node
		node, err := pm.resolveNode(ctx, identity, rInput.Node)
		if err != nil {
			return nil, err
		}
		isLocal := (node.Parent.Equals(localOrg.ID) && node.Name == localNode.Name)
		foundLocal = foundLocal || isLocal
		log.L(ctx).Debugf("Resolved group identity %s node=%s to identity %s node=%s local=%t", rInput.Identity, rInput.Node, identity.DID, node.ID, isLocal)
		gi.Members[i] = &core.Member{
			Identity: identity.DID,
			Node:     node.ID,
		}
	}
	if !foundLocal {
		// Add in the local org/node identity
		gi.Members = append(gi.Members, &core.Member{
			Identity: localOrg.DID,
			Node:     localNode.ID,
		})
	}
	return gi, nil
}

func (pm *privateMessaging) findOrGenerateGroup(ctx context.Context, in *core.MessageInOut) (group *core.Group, isNew bool, err error) {
	gi, err := pm.getRecipients(ctx, in)
	if err != nil {
		return nil, false, err
	}

	// Create the group structure, and seal it - which will sort the members, and
	// generate the deterministic hash. We then search on that group to see if it
	// exists. If it doesn't, we go ahead and create it. If it does - we don't return
	// this candidate - we return the existing group.
	newCandidate := &core.Group{
		GroupIdentity: *gi,
		Created:       fftypes.Now(),
	}
	newCandidate.Seal()

	group, _, err = pm.getGroupNodes(ctx, newCandidate.Hash, true)
	if err != nil {
		return nil, false, err
	}
	if group != nil {
		return group, false, nil
	}
	return newCandidate, true, nil
}
