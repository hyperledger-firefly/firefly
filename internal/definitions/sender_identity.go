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

package definitions

import (
	"context"

	"github.com/hyperledger/firefly/internal/identity"
	"github.com/hyperledger/firefly/pkg/core"
)

// ClaimIdentity is a special form of CreateDefinition where the signing identity does not need to have been pre-registered
// The blockchain "key" will be normalized, but the "author" will pass through unchecked
func (ds *definitionSender) ClaimIdentity(ctx context.Context, claim *core.IdentityClaim, signingIdentity *core.SignerRef, parentSigner *core.SignerRef) error {
	if ds.multiparty {
		var err error
		signingIdentity.Key, err = ds.identity.ResolveInputSigningKey(ctx, signingIdentity.Key, identity.KeyNormalizationBlockchainPlugin)
		if err != nil {
			return err
		}

		claim.Identity.Namespace = ""
		claimMsg, err := ds.getSenderResolved(ctx, claim, signingIdentity, core.SystemTagIdentityClaim, nil).send(ctx, false)
		if err != nil {
			return err
		}
		claim.Identity.Messages.Claim = claimMsg.Header.ID

		// Send the verification if one is required.
		if parentSigner != nil {
			verifyMsg, err := ds.getSender(ctx, &core.IdentityVerification{
				Claim: core.MessageRef{
					ID:   claimMsg.Header.ID,
					Hash: claimMsg.Hash,
				},
				Identity: claim.Identity.IdentityBase,
			}, parentSigner, core.SystemTagIdentityVerification, nil).send(ctx, false)
			if err != nil {
				return err
			}
			claim.Identity.Messages.Verification = verifyMsg.Header.ID
		}

		return nil
	}

	claim.Identity.Namespace = ds.namespace
	return fakeBatch(ctx, func(ctx context.Context, state *core.BatchState) (HandlerResult, error) {
		return ds.handler.handleIdentityClaim(ctx, state, &identityMsgInfo{SignerRef: *signingIdentity}, claim)
	})
}

func (ds *definitionSender) UpdateIdentity(ctx context.Context, identity *core.Identity, def *core.IdentityUpdate, signingIdentity *core.SignerRef, waitConfirm bool) error {
	if ds.multiparty {
		updateMsg, err := ds.getSender(ctx, def, signingIdentity, core.SystemTagIdentityUpdate, nil).send(ctx, waitConfirm)
		identity.Messages.Update = updateMsg.Header.ID
		return err
	}

	return fakeBatch(ctx, func(ctx context.Context, state *core.BatchState) (HandlerResult, error) {
		return ds.handler.handleIdentityUpdate(ctx, state, &identityUpdateMsgInfo{}, def)
	})
}
