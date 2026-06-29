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
	"fmt"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
)

// IdentityType is the type of an identity
type IdentityType = fftypes.FFEnum

var (
	// IdentityTypeOrg is an organization
	IdentityTypeOrg = fftypes.FFEnumValue("identitytype", "org")
	// IdentityTypeNode is a node
	IdentityTypeNode = fftypes.FFEnumValue("identitytype", "node")
	// IdentityTypeCustom is a user defined identity within a namespace
	IdentityTypeCustom = fftypes.FFEnumValue("identitytype", "custom")
)

const (
	DIDPrefix              = "did:"
	FireFlyDIDPrefix       = "did:firefly:"
	FireFlyOrgDIDPrefix    = "did:firefly:org/"
	FireFlyNodeDIDPrefix   = "did:firefly:node/"
	FireFlyCustomDIDPrefix = "did:firefly:"
)

type IdentityMessages struct {
	Claim        *fftypes.UUID `ffstruct:"IdentityMessages" json:"claim"`
	Verification *fftypes.UUID `ffstruct:"IdentityMessages" json:"verification"`
	Update       *fftypes.UUID `ffstruct:"IdentityMessages" json:"update"`
}

// IdentityBase are the immutable fields of an identity that determine what the identity itself is
type IdentityBase struct {
	ID        *fftypes.UUID `ffstruct:"Identity" json:"id" ffexcludeinput:"true"`
	DID       string        `ffstruct:"Identity" json:"did"`
	Type      IdentityType  `ffstruct:"Identity" json:"type" ffenum:"identitytype" ffexcludeinput:"true"`
	Parent    *fftypes.UUID `ffstruct:"Identity" json:"parent,omitempty"`
	Namespace string        `ffstruct:"Identity" json:"namespace"`
	Name      string        `ffstruct:"Identity" json:"name,omitempty"`
}

// IdentityProfile are the field of a profile that can be updated over time
type IdentityProfile struct {
	Description string             `ffstruct:"IdentityProfile" json:"description,omitempty"`
	Profile     fftypes.JSONObject `ffstruct:"IdentityProfile" json:"profile,omitempty"`
}

// Identity is the persisted structure backing all identities, including orgs, nodes and custom identities
type Identity struct {
	IdentityBase
	IdentityProfile
	Messages IdentityMessages `ffstruct:"Identity" json:"messages,omitempty" ffexcludeinput:"true"`
	Created  *fftypes.FFTime  `ffstruct:"Identity" json:"created,omitempty" ffexcludeinput:"true"`
	Updated  *fftypes.FFTime  `ffstruct:"Identity" json:"updated,omitempty"`
}

// IdentityWithVerifiers has an embedded array of verifiers
type IdentityWithVerifiers struct {
	Identity
	Verifiers []*VerifierRef `ffstruct:"IdentityWithVerifiers" json:"verifiers"`
}

// IdentityCreateDTO is the input structure to submit to register an identity.
// The blockchain key that will be used to establish the claim for the identity
// needs to be provided.
type IdentityCreateDTO struct {
	Name   string       `ffstruct:"Identity" json:"name"`
	Type   IdentityType `ffstruct:"Identity" json:"type,omitempty"`
	Parent string       `ffstruct:"IdentityCreateDTO" json:"parent,omitempty"` // can be a DID for resolution, or the UUID directly
	Key    string       `ffstruct:"IdentityCreateDTO" json:"key,omitempty"`
	IdentityProfile
}

// IdentityUpdateDTO is the input structure to submit to update an identityprofile.
// The same key in the claim will be used for the update.
type IdentityUpdateDTO struct {
	IdentityProfile
}

// SignerRef is the nested structure representing the identity that signed a message.
// It might comprise a resolvable by FireFly identity DID, a blockchain signing key, or both.
type SignerRef struct {
	Author string `ffstruct:"SignerRef" json:"author,omitempty"`
	Key    string `ffstruct:"SignerRef" json:"key,omitempty"`
}

// IdentityClaim is the data payload used in a message to broadcast an intent to publish a new identity.
// Most claims (except root orgs, where different requirements apply) require a separate IdentityVerification
// from the parent identity to be published (on the same topic) before the identity is considered valid
// and is stored as a confirmed identity.
type IdentityClaim struct {
	Identity *Identity `ffstruct:"IdentityClaim" json:"identity"`
}

// IdentityVerification is the data payload used in message to broadcast a verification of a child identity.
// Must refer to the UUID and Hash of the IdentityClaim message, and must contain the same base identity data.
type IdentityVerification struct {
	Claim    MessageRef   `ffstruct:"IdentityVerification" json:"claim"`
	Identity IdentityBase `ffstruct:"IdentityVerification" json:"identity"`
}

// IdentityUpdate is the data payload used in message to broadcast an update to an identity profile.
// The broadcast must be on the same identity as the currently established identity claim message for the identity,
// and it must contain the same identity data.
// The profile is replaced in its entirety.
type IdentityUpdate struct {
	Identity IdentityBase    `ffstruct:"IdentityUpdate" json:"identity"`
	Updates  IdentityProfile `ffstruct:"IdentityUpdate" json:"updates,omitempty"`
}

func (ic *IdentityClaim) Topic() string {
	return ic.Identity.Topic()
}

func (ic *IdentityClaim) SetBroadcastMessage(msgID *fftypes.UUID) {
	ic.Identity.Messages.Claim = msgID
}

func (iv *IdentityVerification) Topic() string {
	return iv.Identity.Topic()
}

func (iv *IdentityVerification) SetBroadcastMessage(msgID *fftypes.UUID) {
	// nop-op here, the definition handler of the claim is the one that is responsible for updating
	// the verification message ID on the Identity.
}

func (iu *IdentityUpdate) Topic() string {
	return iu.Identity.Topic()
}

func (iu *IdentityUpdate) SetBroadcastMessage(msgID *fftypes.UUID) {
	// nop-op here, as the IdentityUpdate doesn't have a reference to the original Identity to set this.
}

func (i *IdentityBase) Topic() string {
	h := sha256.New()
	h.Write([]byte(i.DID))
	return fftypes.HashResult(h).String()
}

func (i *IdentityBase) Validate(ctx context.Context) (err error) {
	if i.ID == nil {
		return i18n.NewError(ctx, i18n.MsgNilID)
	}
	if err = fftypes.ValidateFFNameFieldNoUUID(ctx, i.Name, "name"); err != nil {
		return err
	}

	var legacyDID string
	if i.Type == IdentityTypeCustom {
		legacyDID = fmt.Sprintf("%sns/%s/%s", FireFlyCustomDIDPrefix, i.Namespace, i.Name)
	}
	if requiredDID, err := i.GenerateDID(ctx); err != nil {
		return err
	} else if i.DID == "" || (i.DID != requiredDID && i.DID != legacyDID) {
		return i18n.NewError(ctx, i18n.MsgInvalidDIDForType, i.DID, i.Type, i.Namespace, i.Name)
	}
	return nil
}

func (i *IdentityBase) GenerateDID(ctx context.Context) (string, error) {
	switch i.Type {
	case IdentityTypeCustom:
		if i.Namespace == LegacySystemNamespace {
			return "", i18n.NewError(ctx, i18n.MsgCustomIdentitySystemNS, LegacySystemNamespace)
		}
		if i.Parent == nil {
			return "", i18n.NewError(ctx, i18n.MsgNilParentIdentity, i.Type)
		}
		return fmt.Sprintf("%s%s", FireFlyCustomDIDPrefix, i.Name), nil
	case IdentityTypeNode:
		if i.Parent == nil {
			return "", i18n.NewError(ctx, i18n.MsgNilParentIdentity, i.Type)
		}
		return fmt.Sprintf("%s%s", FireFlyNodeDIDPrefix, i.Name), nil
	case IdentityTypeOrg:
		return fmt.Sprintf("%s%s", FireFlyOrgDIDPrefix, i.Name), nil
	default:
		return "", i18n.NewError(ctx, i18n.MsgUnknownIdentityType, i.Type)
	}
}

func (i *IdentityBase) Equals(ctx context.Context, i2 *IdentityBase) bool {
	if err := i.Validate(ctx); err != nil {
		log.L(ctx).Warnf("Comparing invalid identity (source) %s (%v): %s", i.DID, i.ID, err)
		return false
	}
	if err := i2.Validate(ctx); err != nil {
		log.L(ctx).Warnf("Comparing invalid identity (target) %s (%v): %s", i.DID, i2.ID, err)
		return false
	}
	return i.ID.Equals(i2.ID) &&
		i.DID == i2.DID &&
		i.Type == i2.Type &&
		i.Parent.Equals(i2.Parent) &&
		i.Namespace == i2.Namespace &&
		i.Name == i2.Name
}

func (identity *Identity) Validate(ctx context.Context) (err error) {
	if identity == nil {
		return i18n.NewError(ctx, i18n.MsgNilOrNullObject)
	}
	if err = identity.IdentityBase.Validate(ctx); err != nil {
		return err
	}
	if err = fftypes.ValidateLength(ctx, identity.Description, "description", 4096); err != nil {
		return err
	}
	return nil
}
