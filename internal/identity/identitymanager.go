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

package identity

import (
	"context"
	"fmt"
	"strings"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/cache"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/internal/multiparty"
	"github.com/hyperledger-firefly/firefly/pkg/blockchain"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

const (
	KeyNormalizationBlockchainPlugin = iota
	KeyNormalizationNone
)

type Manager interface {
	ResolveInputSigningIdentity(ctx context.Context, signerRef *core.SignerRef) (err error)
	ResolveInputVerifierRef(ctx context.Context, inputKey *core.VerifierRef, intent blockchain.ResolveKeyIntent) (*core.VerifierRef, error)
	ResolveInputSigningKey(ctx context.Context, inputKey string, keyNormalizationMode int) (signingKey string, err error)
	ResolveQuerySigningKey(ctx context.Context, inputKey string, keyNormalizationMode int) (signingKey string, err error)
	ResolveIdentitySigner(ctx context.Context, identity *core.Identity) (parentSigner *core.SignerRef, err error)
	ResolveMultipartyRootVerifier(ctx context.Context) (*core.VerifierRef, error)

	FindIdentityForVerifier(ctx context.Context, iTypes []core.IdentityType, verifier *core.VerifierRef) (identity *core.Identity, err error)
	CachedIdentityLookupByID(ctx context.Context, id *fftypes.UUID) (identity *core.Identity, err error)
	CachedIdentityLookupMustExist(ctx context.Context, did string) (identity *core.Identity, retryable bool, err error)
	CachedIdentityLookupNilOK(ctx context.Context, did string) (identity *core.Identity, retryable bool, err error)
	GetLocalNode(ctx context.Context) (node *core.Identity, err error)
	GetLocalNodeDID(ctx context.Context) (string, error)
	GetRootOrgDID(ctx context.Context) (string, error)
	GetRootOrg(ctx context.Context) (org *core.Identity, err error)
	VerifyIdentityChain(ctx context.Context, identity *core.Identity) (immediateParent *core.Identity, retryable bool, err error)
	ValidateNodeOwner(ctx context.Context, node *core.Identity, identity *core.Identity) (valid bool, err error)
}

type identityManager struct {
	database      database.Plugin
	blockchain    blockchain.Plugin  // optional
	multiparty    multiparty.Manager // optional
	namespace     string
	defaultKey    string
	identityCache cache.CInterface
}

func NewIdentityManager(ctx context.Context, ns, defaultKey string, di database.Plugin, bi blockchain.Plugin, mp multiparty.Manager, cacheManager cache.Manager) (Manager, error) {
	if di == nil {
		return nil, i18n.NewError(ctx, coremsgs.MsgInitializationNilDepError, "IdentityManager")
	}
	im := &identityManager{
		database:   di,
		blockchain: bi,
		namespace:  ns,
		multiparty: mp,
		defaultKey: defaultKey,
	}

	identityCache, err := cacheManager.GetCache(
		cache.NewCacheConfig(
			ctx,
			coreconfig.CacheIdentityLimit,
			coreconfig.CacheIdentityTTL,
			ns,
		),
	)
	if err != nil {
		return nil, err
	}
	im.identityCache = identityCache

	return im, nil
}

func ParseKeyNormalizationConfig(strConfigVal string) int {
	switch strings.ToLower(strConfigVal) {
	case "blockchain_plugin":
		return KeyNormalizationBlockchainPlugin
	default:
		return KeyNormalizationNone
	}
}

// GetLocalNodeDID returns the expected local node DID based on the multiparty configuration, the node
// identity does not need to be registered yet in order for this to succeed.
func (im *identityManager) GetLocalNodeDID(ctx context.Context) (string, error) {
	nodeName := im.multiparty.LocalNode().Name
	if nodeName == "" {
		return "", i18n.NewError(ctx, coremsgs.MsgLocalNodeNotSet)
	}

	return fmt.Sprintf("%s%s", core.FireFlyNodeDIDPrefix, nodeName), nil
}

// GetLocalNode returns the local node identity, if it has been registered and is in the DB/cache.
func (im *identityManager) GetLocalNode(ctx context.Context) (node *core.Identity, err error) {
	nodeDID, err := im.GetLocalNodeDID(ctx)
	if err != nil {
		return nil, err
	}
	node, _, err = im.CachedIdentityLookupNilOK(ctx, nodeDID)
	return node, err
}

func (im *identityManager) GetRootOrgDID(ctx context.Context) (string, error) {
	orgName := im.multiparty.RootOrg().Name
	if orgName != "" {
		orgDID := fmt.Sprintf("%s%s", core.FireFlyOrgDIDPrefix, orgName)
		return orgDID, nil
	}
	return "", i18n.NewError(ctx, coremsgs.MsgLocalOrgNotSet)
}

func (im *identityManager) GetRootOrg(ctx context.Context) (org *core.Identity, err error) {
	orgDID, err := im.GetRootOrgDID(ctx)
	if err == nil {
		org, _, err = im.CachedIdentityLookupMustExist(ctx, orgDID)
	}
	return org, err
}

// ResolveInputSigningKey takes in only a "key" (which may be empty to use the default) to be resolved and returned.
// This is for cases where keys are used directly without an "author" field alongside them (custom contracts, tokens),
// or when the author is known by the caller and should not / cannot be confirmed prior to sending (identity claims)
func (im *identityManager) ResolveInputSigningKey(ctx context.Context, inputKey string, keyNormalizationMode int) (signingKey string, err error) {
	return im.resolveInputSigningKey(ctx, inputKey, keyNormalizationMode, blockchain.ResolveKeyIntentSign)
}

// ResolveQuerySigningKey does the same resolution as ResolveInputSigningKey, but for the intent of querying the blockchain
// (rather than signing a transaction)
func (im *identityManager) ResolveQuerySigningKey(ctx context.Context, inputKey string, keyNormalizationMode int) (signingKey string, err error) {
	return im.resolveInputSigningKey(ctx, inputKey, keyNormalizationMode, blockchain.ResolveKeyIntentQuery)
}

func (im *identityManager) resolveInputSigningKey(ctx context.Context, inputKey string, keyNormalizationMode int, intent blockchain.ResolveKeyIntent) (signingKey string, err error) {
	if inputKey == "" {
		if im.blockchain == nil {
			if im.defaultKey == "" {
				return "", i18n.NewError(ctx, coremsgs.MsgNodeMissingBlockchainKey)
			}
			// There is no blockchain plugin defined here, so no additional verification possible, or required
			return im.defaultKey, nil
		}

		verifierRef, err := im.getDefaultVerifier(ctx, intent)
		if err != nil {
			return "", err
		}
		return verifierRef.Value, nil
	}
	// If the caller is not confident that the blockchain plugin/connector should be used to resolve,
	// for example it might be a different blockchain (Eth vs Fabric etc.), or it has its own
	// verification/management of keys, it should set `namespaces.predefined[].asset.manager.keyNormalization: "none"` in the config.
	if keyNormalizationMode != KeyNormalizationBlockchainPlugin {
		return inputKey, nil
	}
	signer, err := im.resolveInputKeyViaBlockchainPlugin(ctx, inputKey, intent)
	if err != nil {
		return "", err
	}
	return signer.Value, nil
}

func (im *identityManager) ResolveInputVerifierRef(ctx context.Context, inputKey *core.VerifierRef, intent blockchain.ResolveKeyIntent) (*core.VerifierRef, error) {
	log.L(ctx).Debugf("Resolving input signing key: type='%s' value='%s'", inputKey.Type, inputKey.Value)

	if im.blockchain == nil {
		return nil, i18n.NewError(ctx, coremsgs.MsgBlockchainNotConfigured)
	}

	verifierType := inputKey.Type

	if verifierType.String() == "" {
		verifierType = im.blockchain.VerifierType()
	}

	if verifierType != im.blockchain.VerifierType() {
		return nil, i18n.NewError(ctx, coremsgs.MsgUnknownVerifierType)
	}

	signingKey, err := im.blockchain.ResolveSigningKey(ctx, inputKey.Value, intent)
	if err != nil {
		return nil, err
	}

	return &core.VerifierRef{
		Type:  verifierType,
		Value: signingKey,
	}, nil
}

// ResolveInputSigningIdentity takes in blockchain signing input information from an API call (which may
// include author or key or both), and updates it with fully resolved and normalized values
func (im *identityManager) ResolveInputSigningIdentity(ctx context.Context, signerRef *core.SignerRef) (err error) {
	log.L(ctx).Debugf("Resolving identity input: key='%s' author='%s'", signerRef.Key, signerRef.Author)

	if im.blockchain == nil {
		return i18n.NewError(ctx, coremsgs.MsgBlockchainNotConfigured)
	}

	var verifier *core.VerifierRef
	switch {
	case signerRef.Author == "" && signerRef.Key == "":
		// Nothing specified: use the default node identity
		err = im.resolveDefaultSigningIdentity(ctx, signerRef)
		if err != nil {
			return err
		}

	case signerRef.Key != "":
		// Key specified: normalize it, then check it against author (if specified)
		if verifier, err = im.resolveInputKeyViaBlockchainPlugin(ctx, signerRef.Key, blockchain.ResolveKeyIntentSign); err != nil {
			return err
		}
		signerRef.Key = verifier.Value

		identity, err := im.FindIdentityForVerifier(ctx, []core.IdentityType{
			core.IdentityTypeOrg,
			core.IdentityTypeCustom,
		}, verifier)
		switch {
		case err != nil:
			return err
		case identity != nil:
			// Key matches a registered verifier: author must be unspecified OR must match verifier identity
			if signerRef.Author == identity.Name || signerRef.Author == "" {
				// Resolve author to DID (if blank or bare name)
				signerRef.Author = identity.DID
			}
			if signerRef.Author != identity.DID {
				return i18n.NewError(ctx, coremsgs.MsgAuthorRegistrationMismatch, verifier.Value, signerRef.Author, identity.DID)
			}
		case signerRef.Author != "":
			// Key is unrecognized, but an author was specified: use the key and resolve author to DID
			identity, _, err := im.CachedIdentityLookupMustExist(ctx, signerRef.Author)
			if err != nil {
				return err
			}
			signerRef.Author = identity.DID
		default:
			return i18n.NewError(ctx, coremsgs.MsgAuthorMissingForKey, signerRef.Key)
		}

	case signerRef.Author != "":
		// Author specified (without key): use the first blockchain key associated with it
		identity, _, err := im.CachedIdentityLookupMustExist(ctx, signerRef.Author)
		if err != nil {
			return err
		}
		verifier, _, err = im.firstVerifierForIdentity(ctx, im.blockchain.VerifierType(), identity)
		if err != nil {
			return err
		}
		signerRef.Author = identity.DID
		signerRef.Key = verifier.Value
	}

	log.L(ctx).Debugf("Resolved identity: key='%s' author='%s'", signerRef.Key, signerRef.Author)
	return nil
}

// firstVerifierForIdentity does a lookup of the first verifier of a given type (such as a blockchain signing key) registered to an identity,
// as a convenience to allow you to only specify the org name/DID when sending a message
func (im *identityManager) firstVerifierForIdentity(ctx context.Context, vType core.VerifierType, identity *core.Identity) (verifier *core.VerifierRef, retryable bool, err error) {
	fb := database.VerifierQueryFactory.NewFilterLimit(ctx, 1)
	filter := fb.And(
		fb.Eq("type", vType),
		fb.Eq("identity", identity.ID),
	)
	verifiers, _, err := im.database.GetVerifiers(ctx, identity.Namespace, filter)
	if err != nil {
		return nil, true /* DB Error */, err
	}
	if len(verifiers) == 0 {
		return nil, false, i18n.NewError(ctx, coremsgs.MsgNoVerifierForIdentity, vType, identity.DID)
	}
	return &verifiers[0].VerifierRef, false, nil
}

// resolveDefaultSigningIdentity adds the default signing identity into a message
func (im *identityManager) resolveDefaultSigningIdentity(ctx context.Context, signerRef *core.SignerRef) (err error) {
	verifierRef, err := im.getDefaultVerifier(ctx, blockchain.ResolveKeyIntentSign)
	if err != nil {
		return err
	}
	identity, err := im.GetRootOrg(ctx)
	if err != nil {
		return err
	}
	signerRef.Author = identity.DID
	signerRef.Key = verifierRef.Value
	return nil
}

// getDefaultVerifier gets the default blockchain verifier via the configuration
func (im *identityManager) getDefaultVerifier(ctx context.Context, intent blockchain.ResolveKeyIntent) (verifier *core.VerifierRef, err error) {
	if im.defaultKey != "" {
		return im.resolveInputKeyViaBlockchainPlugin(ctx, im.defaultKey, intent)
	}
	if im.multiparty != nil {
		orgKey := im.multiparty.RootOrg().Key
		if orgKey != "" {
			return im.resolveInputKeyViaBlockchainPlugin(ctx, orgKey, intent)
		}
	}
	return im.resolveInputKeyViaBlockchainPlugin(ctx, "", intent)
}

// ResolveMultipartyRootVerifier gets the blockchain verifier of the root org via the configuration,
// resolving it for use as a signing key for the purpose of signing a child identity
func (im *identityManager) ResolveMultipartyRootVerifier(ctx context.Context) (*core.VerifierRef, error) {
	orgKey := im.multiparty.RootOrg().Key
	if orgKey == "" {
		return nil, i18n.NewError(ctx, coremsgs.MsgNodeMissingBlockchainKey)
	}

	return im.resolveInputKeyViaBlockchainPlugin(ctx, orgKey, blockchain.ResolveKeyIntentSign)
}

// resolveInputKeyViaBlockchainPlugin calls the blockchain plugin to resolve an input key string, to the
// blockchain native representation of that key. Which might involve sophisticated processing.
// See ResolveInputSigningKey on the blockchain connector
//
// Note: Caching is deferred down to the blockchain plugin (prior to v1.2 it was performed in the identity manager)
func (im *identityManager) resolveInputKeyViaBlockchainPlugin(ctx context.Context, inputKey string, intent blockchain.ResolveKeyIntent) (verifier *core.VerifierRef, err error) {

	if im.blockchain == nil {
		return nil, i18n.NewError(ctx, coremsgs.MsgBlockchainNotConfigured)
	}

	keyString, err := im.blockchain.ResolveSigningKey(ctx, inputKey, intent)
	if err != nil {
		return nil, err
	}
	verifier = &core.VerifierRef{
		Type:  im.blockchain.VerifierType(),
		Value: keyString,
	}
	return verifier, nil
}

// FindIdentityForVerifier is a reverse lookup function to look up an identity registered as owner of the specified verifier
func (im *identityManager) FindIdentityForVerifier(ctx context.Context, iTypes []core.IdentityType, verifier *core.VerifierRef) (identity *core.Identity, err error) {
	identity, err = im.cachedIdentityLookupByVerifierRef(ctx, im.namespace, verifier)
	if err != nil || identity != nil {
		return identity, err
	}
	return nil, nil
}

func (im *identityManager) VerifyIdentityChain(ctx context.Context, checkIdentity *core.Identity) (immediateParent *core.Identity, retryable bool, err error) {

	err = checkIdentity.Validate(ctx)
	if err != nil {
		return nil, false, err
	}

	loopDetect := make(map[fftypes.UUID]bool)
	current := checkIdentity
	for {
		loopDetect[*current.ID] = true
		parentID := current.Parent
		if parentID == nil {
			return immediateParent, false, nil
		}
		if _, ok := loopDetect[*parentID]; ok {
			return nil, false, i18n.NewError(ctx, coremsgs.MsgIdentityChainLoop, parentID, current.DID, current.ID)
		}
		parent, err := im.CachedIdentityLookupByID(ctx, parentID)
		if err != nil {
			return nil, true /* DB Error */, err
		}
		if parent == nil {
			return nil, false, i18n.NewError(ctx, coremsgs.MsgParentIdentityNotFound, parentID, current.DID, current.ID)
		}
		if err := im.validateParentType(ctx, current, parent); err != nil {
			return nil, false, err
		}
		if im.multiparty != nil && parent.Messages.Claim == nil {
			return nil, false, i18n.NewError(ctx, coremsgs.MsgParentIdentityMissingClaim, parent.DID, parent.ID)
		}
		current = parent
		if immediateParent == nil {
			immediateParent = parent
		}
	}

}

func (im *identityManager) ResolveIdentitySigner(ctx context.Context, identity *core.Identity) (signer *core.SignerRef, err error) {

	// Find the message that registered the identity
	msg, err := im.database.GetMessageByID(ctx, im.namespace, identity.Messages.Claim)
	if err != nil {
		return nil, err
	}
	if msg == nil {
		return nil, i18n.NewError(ctx, coremsgs.MsgParentIdentityMissingClaim, identity.DID, identity.ID)
	}
	// Return the signing identity from that claim
	return &msg.Header.SignerRef, nil
}

func (im *identityManager) validateParentType(ctx context.Context, child *core.Identity, parent *core.Identity) error {

	switch child.Type {
	case core.IdentityTypeNode, core.IdentityTypeOrg:
		if parent.Type != core.IdentityTypeOrg {
			return i18n.NewError(ctx, coremsgs.MsgInvalidIdentityParentType, parent.DID, parent.ID, parent.Type, child.DID, child.ID, child.Type)
		}
		return nil
	case core.IdentityTypeCustom:
		if parent.Type != core.IdentityTypeOrg && parent.Type != core.IdentityTypeCustom {
			return i18n.NewError(ctx, coremsgs.MsgInvalidIdentityParentType, parent.DID, parent.ID, parent.Type, child.DID, child.ID, child.Type)
		}
		return nil
	default:
		return i18n.NewError(ctx, i18n.MsgUnknownIdentityType, child.Type)
	}

}

func (im *identityManager) cachedIdentityLookupByVerifierRef(ctx context.Context, namespace string, verifierRef *core.VerifierRef) (*core.Identity, error) {
	cacheKey := fmt.Sprintf("ns=%s,type=%s,verifier=%s", namespace, verifierRef.Type, verifierRef.Value)
	if cachedValue := im.identityCache.Get(cacheKey); cachedValue != nil {
		return cachedValue.(*core.Identity), nil
	}
	verifier, err := im.database.GetVerifierByValue(ctx, verifierRef.Type, namespace, verifierRef.Value)
	if err != nil {
		return nil, err
	} else if verifier == nil {
		if namespace != core.LegacySystemNamespace && im.multiparty != nil && im.multiparty.GetNetworkVersion() == 1 {
			// For V1 networks, fall back to LegacySystemNamespace for looking up identities
			// This assumes that the system namespace shares a database with this manager's namespace!
			return im.cachedIdentityLookupByVerifierRef(ctx, core.LegacySystemNamespace, verifierRef)
		}
		return nil, err
	}
	identity, err := im.database.GetIdentityByID(ctx, namespace, verifier.Identity)
	if err != nil {
		return nil, err
	}
	if identity == nil {
		return nil, i18n.NewError(ctx, i18n.MsgEmptyMemberIdentity, verifier.Identity)
	}
	// Cache the result
	im.identityCache.Set(cacheKey, identity)
	return identity, nil
}

func (im *identityManager) cachedIdentityLookup(ctx context.Context, namespace, didLookupStr string) (identity *core.Identity, retryable bool, err error) {
	// Use an LRU cache for the author identity, as it's likely for the same identity to be re-used over and over
	cacheKey := fmt.Sprintf("ns=%s,did=%s", namespace, didLookupStr)
	defer func() {
		didResolved := ""
		var uuidResolved *fftypes.UUID
		if identity != nil {
			didResolved = identity.DID
			uuidResolved = identity.ID
		}
		log.L(ctx).Debugf("Resolved DID '%s' to identity: %s / %s (err=%v)", didLookupStr, uuidResolved, didResolved, err)
	}()
	if cachedValue := im.identityCache.Get(cacheKey); cachedValue != nil {
		identity = cachedValue.(*core.Identity)
	} else {
		if strings.HasPrefix(didLookupStr, core.DIDPrefix) {
			if !strings.HasPrefix(didLookupStr, core.FireFlyDIDPrefix) {
				return nil, false, i18n.NewError(ctx, coremsgs.MsgDIDResolverUnknown, didLookupStr)
			}
			// Look up by the full DID
			if identity, err = im.database.GetIdentityByDID(ctx, namespace, didLookupStr); err != nil {
				return nil, true /* DB Error */, err
			}
			if identity == nil && strings.HasPrefix(didLookupStr, core.FireFlyOrgDIDPrefix) {
				// We allow the UUID to be used to resolve DIDs as an alias to the name
				uuid, err := fftypes.ParseUUID(ctx, strings.TrimPrefix(didLookupStr, core.FireFlyOrgDIDPrefix))
				if err == nil {
					if identity, err = im.database.GetIdentityByID(ctx, namespace, uuid); err != nil {
						return nil, true /* DB Error */, err
					}
				}
			}
		} else {
			// If there is just a name in there, then it could be an Org type identity (from the very original usage of the field)
			if identity, err = im.database.GetIdentityByName(ctx, core.IdentityTypeOrg, namespace, didLookupStr); err != nil {
				return nil, true /* DB Error */, err
			}
		}

		if identity != nil {
			// Cache the result
			im.identityCache.Set(cacheKey, identity)
		} else if namespace != core.LegacySystemNamespace && im.multiparty != nil && im.multiparty.GetNetworkVersion() == 1 {
			// For V1 networks, fall back to LegacySystemNamespace for looking up identities
			// This assumes that the system namespace shares a database with this manager's namespace!
			return im.cachedIdentityLookup(ctx, core.LegacySystemNamespace, didLookupStr)
		}
	}
	return identity, false, nil
}

func (im *identityManager) CachedIdentityLookupNilOK(ctx context.Context, didLookupStr string) (identity *core.Identity, retryable bool, err error) {
	return im.cachedIdentityLookup(ctx, im.namespace, didLookupStr)
}

func (im *identityManager) CachedIdentityLookupMustExist(ctx context.Context, didLookupStr string) (identity *core.Identity, retryable bool, err error) {
	identity, retryable, err = im.CachedIdentityLookupNilOK(ctx, didLookupStr)
	if err != nil {
		return nil, retryable, err
	}
	if identity == nil {
		return nil, false, i18n.NewError(ctx, coremsgs.MsgIdentityNotFoundByString, didLookupStr)
	}
	return identity, false, nil
}

func (im *identityManager) cachedIdentityLookupByID(ctx context.Context, namespace string, id *fftypes.UUID) (identity *core.Identity, err error) {
	// Use an LRU cache for the author identity, as it's likely for the same identity to be re-used over and over
	cacheKey := fmt.Sprintf("ns=%s,id=%s", namespace, id)
	if cachedValue := im.identityCache.Get(cacheKey); cachedValue != nil {
		identity = cachedValue.(*core.Identity)
	} else {
		identity, err = im.database.GetIdentityByID(ctx, namespace, id)
		if err != nil {
			return nil, err
		}
		if identity == nil {
			if namespace != core.LegacySystemNamespace && im.multiparty != nil && im.multiparty.GetNetworkVersion() == 1 {
				// For V1 networks, fall back to LegacySystemNamespace for looking up identities
				// This assumes that the system namespace shares a database with this manager's namespace!
				return im.cachedIdentityLookupByID(ctx, core.LegacySystemNamespace, id)
			}
			return nil, nil
		}
		// Cache the result
		im.identityCache.Set(cacheKey, identity)
	}
	return identity, nil
}

func (im *identityManager) CachedIdentityLookupByID(ctx context.Context, id *fftypes.UUID) (identity *core.Identity, err error) {
	return im.cachedIdentityLookupByID(ctx, im.namespace, id)
}

// Validate that the given identity or one of its ancestors owns the given node.
func (im *identityManager) ValidateNodeOwner(ctx context.Context, node *core.Identity, identity *core.Identity) (valid bool, err error) {
	l := log.L(ctx)
	candidate := identity
	foundOwner := candidate.ID.Equals(node.Parent)
	for !foundOwner && candidate.Parent != nil {
		parent := candidate.Parent
		candidate, err = im.CachedIdentityLookupByID(ctx, parent)
		if err != nil {
			l.Errorf("Failed to retrieve node org '%s': %v", parent, err)
			return false, err // retry for persistence error
		}
		if candidate == nil {
			l.Errorf("Did not find '%s' in chain for identity '%s' (%s)", parent, identity.DID, identity.ID)
			return false, nil
		}
		foundOwner = candidate.ID.Equals(node.Parent)
	}
	if !foundOwner {
		l.Errorf("No identity in the chain matches owner '%s' of node '%s' ('%s')", node.Parent, node.ID, node.Name)
		return false, nil
	}
	return true, nil
}
