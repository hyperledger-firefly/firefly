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

package events

import (
	"context"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/pkg/blockchain"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

func (em *eventManager) actionTerminate(ctx context.Context, location *fftypes.JSONAny, event *blockchain.Event) error {
	return em.multiparty.TerminateContract(ctx, location, event)
}

func (em *eventManager) handleBlockchainNetworkAction(ctx context.Context, event *blockchain.NetworkActionEvent, bc *eventBatchContext) error {
	if em.multiparty == nil {
		log.L(ctx).Errorf("Ignoring network action from non-multiparty network!")
		return nil
	}

	// Verify that the action came from a registered root org
	resolvedAuthor, err := em.identity.FindIdentityForVerifier(ctx, []core.IdentityType{core.IdentityTypeOrg}, event.SigningKey)
	if err != nil {
		return err
	}
	if resolvedAuthor == nil {
		log.L(ctx).Errorf("Ignoring network action %s from unknown identity %s", event.Action, event.SigningKey.Value)
		return nil
	}
	if resolvedAuthor.Parent != nil {
		log.L(ctx).Errorf("Ignoring network action %s from non-root identity %s", event.Action, event.SigningKey.Value)
		return nil
	}

	if event.Action == core.NetworkActionTerminate.String() {
		err = em.actionTerminate(ctx, event.Location, event.Event)
	} else {
		log.L(ctx).Errorf("Ignoring unrecognized network action: %s", event.Action)
		return nil
	}

	if err == nil {
		chainEvent := buildBlockchainEvent(em.namespace.Name, nil, event.Event, &core.BlockchainTransactionRef{
			BlockchainID: event.Event.BlockchainTXID,
		})
		bc.addEventToInsert(chainEvent, em.getTopicForChainListener(nil))
	}
	return err
}
