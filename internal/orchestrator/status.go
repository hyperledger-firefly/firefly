// Copyright © 2024 Kaleido, Inc.
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
	"database/sql/driver"
	"encoding/json"

	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

func (or *orchestrator) getPlugins() core.NamespaceStatusPlugins {
	// Plugins can have more than one name, so they must be iterated over
	tokensArray := make([]*core.NamespaceStatusPlugin, 0)
	for _, plugin := range or.plugins.Tokens {
		tokensArray = append(tokensArray, &core.NamespaceStatusPlugin{
			Name:       plugin.Name,
			PluginType: plugin.Plugin.Name(),
		})
	}

	blockchainsArray := make([]*core.NamespaceStatusPlugin, 0)
	if or.plugins.Blockchain.Plugin != nil {
		blockchainsArray = append(blockchainsArray, &core.NamespaceStatusPlugin{
			Name:       or.plugins.Blockchain.Name,
			PluginType: or.plugins.Blockchain.Plugin.Name(),
		})
	}

	databasesArray := make([]*core.NamespaceStatusPlugin, 0)
	if or.plugins.Database.Plugin != nil {
		databasesArray = append(databasesArray, &core.NamespaceStatusPlugin{
			Name:       or.plugins.Database.Name,
			PluginType: or.plugins.Database.Plugin.Name(),
		})
	}

	sharedstorageArray := make([]*core.NamespaceStatusPlugin, 0)
	if or.plugins.SharedStorage.Plugin != nil {
		sharedstorageArray = append(sharedstorageArray, &core.NamespaceStatusPlugin{
			Name:       or.plugins.SharedStorage.Name,
			PluginType: or.plugins.SharedStorage.Plugin.Name(),
		})
	}

	dataexchangeArray := make([]*core.NamespaceStatusPlugin, 0)
	if or.plugins.DataExchange.Plugin != nil {
		dataexchangeArray = append(dataexchangeArray, &core.NamespaceStatusPlugin{
			Name:       or.plugins.DataExchange.Name,
			PluginType: or.plugins.DataExchange.Plugin.Name(),
		})
	}

	return core.NamespaceStatusPlugins{
		Blockchain:    blockchainsArray,
		Database:      databasesArray,
		SharedStorage: sharedstorageArray,
		DataExchange:  dataexchangeArray,
		Events:        or.events.GetPlugins(),
		Tokens:        tokensArray,
		Identity:      []*core.NamespaceStatusPlugin{},
	}
}

func (or *orchestrator) GetStatus(ctx context.Context) (status *core.NamespaceStatus, err error) {

	status = &core.NamespaceStatus{
		Namespace: or.namespace,
		Plugins:   or.getPlugins(),
		Multiparty: core.NamespaceStatusMultiparty{
			Enabled: or.config.Multiparty.Enabled,
		},
	}

	if or.config.Multiparty.Enabled {
		status.Node = &core.NamespaceStatusNode{Name: or.config.Multiparty.Node.Name}
		status.Org = &core.NamespaceStatusOrg{Name: or.config.Multiparty.Org.Name}
		status.Multiparty.Contracts = or.namespace.Contracts

		org, err := or.identity.GetRootOrg(ctx)
		if err != nil {
			log.L(ctx).Warnf("Failed to query local org for status: %s", err)
		}

		if org != nil {
			status.Org.Registered = true
			status.Org.ID = org.ID
			status.Org.DID = org.DID

			fb := database.VerifierQueryFactory.NewFilter(ctx)
			verifiers, _, err := or.database().GetVerifiers(ctx, org.Namespace, fb.And(fb.Eq("identity", org.ID)))
			if err != nil {
				return nil, err
			}
			status.Org.Verifiers = make([]*core.VerifierRef, len(verifiers))
			for i, v := range verifiers {
				status.Org.Verifiers[i] = &v.VerifierRef
			}

			node, err := or.identity.GetLocalNode(ctx)
			if err != nil {
				return nil, err
			}
			if node != nil && !node.Parent.Equals(org.ID) {
				log.L(ctx).Errorf("Specified node name is in use by another org: %s", err)
				node = nil
			}
			if node != nil {
				status.Node.Registered = true
				status.Node.ID = node.ID
			}
		}
	}

	return status, nil
}

// Get the earliest incomplete identity claim message for this org, if it exists
func (or *orchestrator) getRegistrationMessage(ctx context.Context) (msg *core.MessageInOut, err error) {
	fb := database.MessageQueryFactory.NewFilter(ctx)
	filter := fb.Sort("created").Limit(1).And(
		fb.Eq("type", core.MessageTypeDefinition),
		fb.Eq("tag", core.SystemTagIdentityClaim),
		fb.Eq("author", core.FireFlyOrgDIDPrefix+or.config.Multiparty.Org.Name),
		fb.In("state", []driver.Value{core.MessageStateStaged, core.MessageStateReady, core.MessageStateSent, core.MessageStatePending}),
	)
	msgs, _, err := or.GetMessagesWithData(ctx, filter)
	if err != nil {
		return nil, err
	}
	if len(msgs) > 0 {
		return msgs[0], nil
	}
	return nil, nil
}

// Ensure the given registration message correlates with the expected type
func (or *orchestrator) checkRegistrationType(ctx context.Context, msg *core.MessageInOut, expectedType core.IdentityType) (match bool, err error) {
	if msg != nil && msg.InlineData != nil && len(msg.InlineData) > 0 {
		var identity core.IdentityClaim
		err := json.Unmarshal([]byte(*msg.InlineData[0].Value), &identity)
		if err != nil {
			return false, i18n.WrapError(ctx, err, coremsgs.MsgUnableToParseRegistrationData, err)
		}
		var did string
		switch expectedType {
		case core.IdentityTypeOrg:
			did = core.FireFlyOrgDIDPrefix + or.config.Multiparty.Org.Name
		case core.IdentityTypeNode:
			did = core.FireFlyNodeDIDPrefix + or.config.Multiparty.Node.Name
		default:
			return false, i18n.NewError(ctx, coremsgs.MsgUnexpectedRegistrationType, expectedType)
		}
		return identity.Identity.DID == did && identity.Identity.Type == expectedType, nil
	}
	return false, i18n.NewError(ctx, coremsgs.MsgNoRegistrationMessageData, or.config.Multiparty.Org.Name)
}

func (or *orchestrator) GetMultipartyStatus(ctx context.Context) (mpStatus *core.NamespaceMultipartyStatus, err error) {

	mpStatus = &core.NamespaceMultipartyStatus{
		Enabled: or.config.Multiparty.Enabled,
	}
	if !or.config.Multiparty.Enabled {
		return mpStatus, nil
	}

	status, err := or.GetStatus(ctx)
	if err != nil {
		return nil, err
	}

	mpStatus.Org = core.NamespaceMultipartyStatusOrg{}
	mpStatus.Node = core.NamespaceMultipartyStatusNode{}
	mpStatus.Contracts = &core.MultipartyContractsWithActiveStatus{}
	if or.namespace.Contracts != nil {
		mpStatus.Contracts.Active = &core.MultipartyContractWithStatus{
			MultipartyContract: *or.namespace.Contracts.Active,
			Status:             core.ContractListenerStatusUnknown,
		}
		mpStatus.Contracts.Terminated = or.namespace.Contracts.Terminated
		log.L(ctx).Debugf("Looking up listener status with subscription ID: %s", mpStatus.Contracts.Active.Info.Subscription)
		ok, _, listenerStatus, err := or.blockchain().GetContractListenerStatus(ctx, or.namespace.Name, mpStatus.Contracts.Active.Info.Subscription, false)
		if !ok || err != nil {
			return nil, err
		}
		mpStatus.Contracts.Active.Status = listenerStatus
	}

	if status.Org.Registered {
		mpStatus.Org.Status = core.NamespaceRegistrationStatusRegistered
		if status.Node.Registered {
			mpStatus.Node.Status = core.NamespaceRegistrationStatusRegistered
		} else {
			msg, err := or.getRegistrationMessage(ctx)
			if err != nil {
				return nil, err
			}
			mpStatus.Node.Status = core.NamespaceRegistrationStatusUnregistered
			if msg != nil {
				match, err := or.checkRegistrationType(ctx, msg, core.IdentityTypeNode)
				if err != nil {
					return nil, err
				}
				if match {
					mpStatus.Node.PendingRegistrationMessageID = msg.Header.ID
					mpStatus.Node.Status = core.NamespaceRegistrationStatusRegistering
				} else {
					mpStatus.Node.Status = core.NamespaceRegistrationStatusUnknown
				}
			}
		}
	} else {
		msg, err := or.getRegistrationMessage(ctx)
		if err != nil {
			return nil, err
		}
		mpStatus.Org.Status = core.NamespaceRegistrationStatusUnregistered
		mpStatus.Node.Status = core.NamespaceRegistrationStatusUnregistered
		if msg != nil {
			match, err := or.checkRegistrationType(ctx, msg, core.IdentityTypeOrg)
			if err != nil {
				return nil, err
			}
			if match {
				mpStatus.Org.PendingRegistrationMessageID = msg.Header.ID
				mpStatus.Org.Status = core.NamespaceRegistrationStatusRegistering
			} else {
				mpStatus.Org.Status = core.NamespaceRegistrationStatusUnknown
				mpStatus.Node.Status = core.NamespaceRegistrationStatusUnknown
			}
		}
	}

	return mpStatus, nil
}
