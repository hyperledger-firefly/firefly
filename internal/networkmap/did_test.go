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

package networkmap

import (
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/mocks/identitymanagermocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestDIDGenerationOK(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()

	org1 := testOrg("org1")

	verifierCardano := (&core.Verifier{
		Identity:  org1.ID,
		Namespace: org1.Namespace,
		VerifierRef: core.VerifierRef{
			Type:  core.VerifierTypeCardanoAddress,
			Value: "addr_test1vqhkukz0285zvk0xrwk9jlq0075tx6furuzcjvzpnhtgelsuhhqc4",
		},
		Created: fftypes.Now(),
	})
	verifierEth := (&core.Verifier{
		Identity:  org1.ID,
		Namespace: org1.Namespace,
		VerifierRef: core.VerifierRef{
			Type:  core.VerifierTypeEthAddress,
			Value: "0xc90d94dE1021fD17fAA2F1FC4F4D36Dff176120d",
		},
		Created: fftypes.Now(),
	}).Seal()
	verifierTezos := (&core.Verifier{
		Identity:  org1.ID,
		Namespace: org1.Namespace,
		VerifierRef: core.VerifierRef{
			Type:  core.VerifierTypeTezosAddress,
			Value: "tz1Y6GnVhC4EpcDDSmD3ibcC4WX6DJ4Q1QLN",
		},
		Created: fftypes.Now(),
	}).Seal()
	verifierMSP := (&core.Verifier{
		Identity:  org1.ID,
		Namespace: org1.Namespace,
		VerifierRef: core.VerifierRef{
			Type:  core.VerifierTypeMSPIdentity,
			Value: "mspIdForAcme::x509::CN=fabric-ca::CN=user1",
		},
		Created: fftypes.Now(),
	}).Seal()
	verifierDX := (&core.Verifier{
		Identity:  org1.ID,
		Namespace: org1.Namespace,
		VerifierRef: core.VerifierRef{
			Type:  core.VerifierTypeFFDXPeerID,
			Value: "peer1",
		},
		Created: fftypes.Now(),
	}).Seal()
	verifierUnknown := (&core.Verifier{
		Identity:  org1.ID,
		Namespace: org1.Namespace,
		VerifierRef: core.VerifierRef{
			Type:  core.VerifierType("unknown"),
			Value: "ignore me",
		},
		Created: fftypes.Now(),
	}).Seal()

	mdi := nm.database.(*databasemocks.Plugin)
	mdi.On("GetIdentityByID", nm.ctx, "ns1", mock.Anything).Return(org1, nil)
	mdi.On("GetVerifiers", nm.ctx, "ns1", mock.Anything).Return([]*core.Verifier{
		verifierCardano,
		verifierEth,
		verifierTezos,
		verifierMSP,
		verifierDX,
		verifierUnknown,
	}, nil, nil)

	doc, err := nm.GetDIDDocForIndentityByID(nm.ctx, org1.ID.String())
	assert.NoError(t, err)
	assert.Equal(t, &DIDDocument{
		Context: []string{
			"https://www.w3.org/ns/did/v1",
			"https://w3id.org/security/suites/ed25519-2020/v1",
		},
		ID: org1.DID,
		VerificationMethods: []*VerificationMethod{
			{
				ID:                  verifierCardano.Hash.String(),
				Type:                "PaymentVerificationKeyShelley_ed25519",
				Controller:          org1.DID,
				BlockchainAccountID: verifierCardano.Value,
			},
			{
				ID:                  verifierEth.Hash.String(),
				Type:                "EcdsaSecp256k1VerificationKey2019",
				Controller:          org1.DID,
				BlockchainAccountID: verifierEth.Value,
			},
			{
				ID:                  verifierTezos.Hash.String(),
				Type:                "Ed25519VerificationKey2020",
				Controller:          org1.DID,
				BlockchainAccountID: verifierTezos.Value,
			},
			{
				ID:                verifierMSP.Hash.String(),
				Type:              "HyperledgerFabricMSPIdentity",
				Controller:        org1.DID,
				MSPIdentityString: verifierMSP.Value,
			},
			{
				ID:                 verifierDX.Hash.String(),
				Type:               "FireFlyDataExchangePeerIdentity",
				Controller:         org1.DID,
				DataExchangePeerID: verifierDX.Value,
			},
		},
		Authentication: []string{
			fmt.Sprintf("#%s", verifierCardano.Hash.String()),
			fmt.Sprintf("#%s", verifierEth.Hash.String()),
			fmt.Sprintf("#%s", verifierTezos.Hash.String()),
			fmt.Sprintf("#%s", verifierMSP.Hash.String()),
			fmt.Sprintf("#%s", verifierDX.Hash.String()),
		},
	}, doc)

	mdi.AssertExpectations(t)
}

func TestDIDGenerationGetVerifiersFail(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()

	org1 := testOrg("org1")

	mdi := nm.database.(*databasemocks.Plugin)
	mdi.On("GetIdentityByID", nm.ctx, "ns1", mock.Anything).Return(org1, nil)
	mdi.On("GetVerifiers", nm.ctx, "ns1", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	_, err := nm.GetDIDDocForIndentityByID(nm.ctx, org1.ID.String())
	assert.Regexp(t, "pop", err)
}

func TestDIDGenerationGetIdentityFail(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()

	org1 := testOrg("org1")

	mdi := nm.database.(*databasemocks.Plugin)
	mdi.On("GetIdentityByID", nm.ctx, "ns1", mock.Anything).Return(nil, fmt.Errorf("pop"))

	_, err := nm.GetDIDDocForIndentityByID(nm.ctx, org1.ID.String())
	assert.Regexp(t, "pop", err)
}

func TestDIDGenerationGetIdentityByDIDFail(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()

	org1 := testOrg("org1")

	mii := nm.identity.(*identitymanagermocks.Manager)
	mii.On("CachedIdentityLookupMustExist", nm.ctx, mock.Anything).Return(nil, false, fmt.Errorf("pop"))

	_, err := nm.GetDIDDocForIndentityByDID(nm.ctx, org1.DID)
	assert.Regexp(t, "pop", err)
}

func TestDIDGenerationGetIdentityByDIDFailVerifiers(t *testing.T) {
	nm, cancel := newTestNetworkmap(t)
	defer cancel()

	org1 := testOrg("org1")

	mii := nm.identity.(*identitymanagermocks.Manager)
	mii.On("CachedIdentityLookupMustExist", nm.ctx, mock.Anything).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			ID: fftypes.NewUUID(),
		},
	}, false, nil)
	mdi := nm.database.(*databasemocks.Plugin)
	mdi.On("GetVerifiers", nm.ctx, "ns1", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	_, err := nm.GetDIDDocForIndentityByDID(nm.ctx, org1.DID)
	assert.Regexp(t, "pop", err)
}
