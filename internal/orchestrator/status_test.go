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
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

var (
	pluginsResult = core.NamespaceStatusPlugins{
		Blockchain: []*core.NamespaceStatusPlugin{
			{
				PluginType: "mock-bi",
			},
		},
		Database: []*core.NamespaceStatusPlugin{
			{
				PluginType: "mock-di",
			},
		},
		DataExchange: []*core.NamespaceStatusPlugin{
			{
				PluginType: "mock-dx",
			},
		},
		Events: []*core.NamespaceStatusPlugin{
			{
				PluginType: "mock-ei",
			},
		},
		Identity: []*core.NamespaceStatusPlugin{
			{
				PluginType: "mock-ii",
			},
		},
		SharedStorage: []*core.NamespaceStatusPlugin{
			{
				PluginType: "mock-ps",
			},
		},
		Tokens: []*core.NamespaceStatusPlugin{
			{
				Name:       "token",
				PluginType: "mock-tk",
			},
		},
	}

	mockEventPlugins = []*core.NamespaceStatusPlugin{
		{
			PluginType: "mock-ei",
		},
	}
)

func TestGetStatusRegistered(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)

	coreconfig.Reset()
	config.Set(coreconfig.NamespacesDefault, "default")

	orgID := fftypes.NewUUID()
	nodeID := fftypes.NewUUID()

	or.mim.On("GetRootOrg", or.ctx).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			ID:        orgID,
			Name:      "org1",
			Namespace: "ns",
			DID:       "did:firefly:org/org1",
		},
	}, nil)
	or.mim.On("GetLocalNode", or.ctx).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			ID:     nodeID,
			Name:   "node1",
			Parent: orgID,
		},
	}, nil)
	or.mdi.On("GetVerifiers", or.ctx, "ns", mock.Anything).Return([]*core.Verifier{
		{Hash: fftypes.NewRandB32(), VerifierRef: core.VerifierRef{
			Type:  core.VerifierTypeEthAddress,
			Value: "0x12345",
		}},
	}, nil, nil)

	or.config.Multiparty.Org.Name = "org1"
	or.config.Multiparty.Node.Name = "node1"

	or.mem.On("GetPlugins").Return(mockEventPlugins)

	status, err := or.GetStatus(or.ctx)
	assert.NoError(t, err)

	assert.Equal(t, "ns", status.Namespace.Name)

	assert.Equal(t, "org1", status.Org.Name)
	assert.True(t, status.Org.Registered)
	assert.Equal(t, "did:firefly:org/org1", status.Org.DID)

	assert.Equal(t, *orgID, *status.Org.ID)
	assert.Equal(t, "node1", status.Node.Name)
	assert.True(t, status.Node.Registered)
	assert.Equal(t, *nodeID, *status.Node.ID)
	assert.Equal(t, "0x12345", status.Org.Verifiers[0].Value)

	// Plugins
	assert.ElementsMatch(t, pluginsResult.Blockchain, status.Plugins.Blockchain)
	assert.ElementsMatch(t, pluginsResult.Database, status.Plugins.Database)
	assert.ElementsMatch(t, pluginsResult.DataExchange, status.Plugins.DataExchange)
	assert.ElementsMatch(t, pluginsResult.Events, status.Plugins.Events)
	assert.ElementsMatch(t, pluginsResult.SharedStorage, status.Plugins.SharedStorage)
	assert.ElementsMatch(t, pluginsResult.Tokens, status.Plugins.Tokens)

}

func TestGetStatusVerifierLookupFail(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)

	coreconfig.Reset()
	config.Set(coreconfig.NamespacesDefault, "default")

	orgID := fftypes.NewUUID()

	or.mim.On("GetRootOrg", or.ctx).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			ID:        orgID,
			Name:      "org1",
			Namespace: "ns",
			DID:       "did:firefly:org/org1",
		},
	}, nil)
	or.mdi.On("GetVerifiers", or.ctx, "ns", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	or.mem.On("GetPlugins").Return(mockEventPlugins)

	_, err := or.GetStatus(or.ctx)
	assert.Regexp(t, "pop", err)

}

func TestGetStatusWrongNodeOwner(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)

	coreconfig.Reset()
	config.Set(coreconfig.NamespacesDefault, "default")

	orgID := fftypes.NewUUID()
	nodeID := fftypes.NewUUID()

	or.mim.On("GetRootOrg", or.ctx).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			ID:        orgID,
			Name:      "org1",
			Namespace: "ns",
			DID:       "did:firefly:org/org1",
		},
	}, nil)
	or.mim.On("GetLocalNode", or.ctx).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			ID:     nodeID,
			Name:   "node1",
			Parent: fftypes.NewUUID(),
		},
	}, nil)
	or.mdi.On("GetVerifiers", or.ctx, "ns", mock.Anything).Return([]*core.Verifier{
		{Hash: fftypes.NewRandB32(), VerifierRef: core.VerifierRef{
			Type:  core.VerifierTypeEthAddress,
			Value: "0x12345",
		}},
	}, nil, nil)

	or.config.Multiparty.Org.Name = "org1"
	or.config.Multiparty.Node.Name = "node1"

	or.mem.On("GetPlugins").Return(mockEventPlugins)

	status, err := or.GetStatus(or.ctx)
	assert.NoError(t, err)

	assert.Equal(t, "ns", status.Namespace.Name)

	assert.Equal(t, "org1", status.Org.Name)
	assert.True(t, status.Org.Registered)
	assert.Equal(t, "did:firefly:org/org1", status.Org.DID)

	assert.Equal(t, *orgID, *status.Org.ID)
	assert.Equal(t, "node1", status.Node.Name)
	assert.False(t, status.Node.Registered)
	assert.Nil(t, status.Node.ID)

}

func TestGetStatusUnregistered(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)

	coreconfig.Reset()
	config.Set(coreconfig.NamespacesDefault, "default")

	or.mim.On("GetRootOrg", or.ctx).Return(nil, fmt.Errorf("pop"))

	or.config.Multiparty.Org.Name = "org1"
	or.config.Multiparty.Node.Name = "node1"

	or.mem.On("GetPlugins").Return(mockEventPlugins)

	status, err := or.GetStatus(or.ctx)
	assert.NoError(t, err)

	assert.Equal(t, "ns", status.Namespace.Name)

	assert.Equal(t, "org1", status.Org.Name)
	assert.False(t, status.Org.Registered)

	assert.Equal(t, "node1", status.Node.Name)
	assert.False(t, status.Node.Registered)

}

func TestGetStatusOrgOnlyRegistered(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)

	coreconfig.Reset()
	config.Set(coreconfig.NamespacesDefault, "default")

	orgID := fftypes.NewUUID()

	or.mim.On("GetRootOrg", or.ctx).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			ID:        orgID,
			Name:      "org1",
			Namespace: "ns",
			DID:       "did:firefly:org/org1",
		},
	}, nil)
	or.mim.On("GetLocalNode", or.ctx).Return(nil, nil)
	or.mdi.On("GetVerifiers", or.ctx, "ns", mock.Anything).Return([]*core.Verifier{
		{Hash: fftypes.NewRandB32(), VerifierRef: core.VerifierRef{
			Type:  core.VerifierTypeEthAddress,
			Value: "0x12345",
		}},
	}, nil, nil)

	or.config.Multiparty.Org.Name = "org1"
	or.config.Multiparty.Node.Name = "node1"

	or.mem.On("GetPlugins").Return(mockEventPlugins)

	status, err := or.GetStatus(or.ctx)
	assert.NoError(t, err)

	assert.Equal(t, "ns", status.Namespace.Name)

	assert.Equal(t, "org1", status.Org.Name)
	assert.True(t, status.Org.Registered)
	assert.Equal(t, "did:firefly:org/org1", status.Org.DID)
	assert.Equal(t, *orgID, *status.Org.ID)

	assert.Equal(t, "node1", status.Node.Name)
	assert.False(t, status.Node.Registered)

	// Plugins
	assert.ElementsMatch(t, pluginsResult.Blockchain, status.Plugins.Blockchain)
	assert.ElementsMatch(t, pluginsResult.Database, status.Plugins.Database)
	assert.ElementsMatch(t, pluginsResult.DataExchange, status.Plugins.DataExchange)
	assert.ElementsMatch(t, pluginsResult.Events, status.Plugins.Events)
	assert.ElementsMatch(t, pluginsResult.SharedStorage, status.Plugins.SharedStorage)
	assert.ElementsMatch(t, pluginsResult.Tokens, status.Plugins.Tokens)
}

func TestGetStatusNodeError(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)

	coreconfig.Reset()
	config.Set(coreconfig.NamespacesDefault, "default")

	orgID := fftypes.NewUUID()

	or.mim.On("GetRootOrg", or.ctx).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			ID:        orgID,
			Name:      "org1",
			Namespace: "ns",
			DID:       "did:firefly:org/org1",
		},
	}, nil)
	or.mim.On("GetLocalNode", or.ctx).Return(nil, fmt.Errorf("pop"))
	or.mdi.On("GetVerifiers", or.ctx, "ns", mock.Anything).Return([]*core.Verifier{
		{Hash: fftypes.NewRandB32(), VerifierRef: core.VerifierRef{
			Type:  core.VerifierTypeEthAddress,
			Value: "0x12345",
		}},
	}, nil, nil)

	or.config.Multiparty.Node.Name = "node1"

	or.mem.On("GetPlugins").Return(mockEventPlugins)

	_, err := or.GetStatus(or.ctx)
	assert.EqualError(t, err, "pop")

}

func TestGetMultipartyStatusMultipartyNotEnabled(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)

	coreconfig.Reset()
	config.Set(coreconfig.NamespacesDefault, "default")

	or.config.Multiparty.Enabled = false

	mpStatus, err := or.GetMultipartyStatus(or.ctx)
	assert.NoError(t, err)

	assert.Equal(t, false, mpStatus.Enabled)
}

func TestGetMultipartyStatusUnregistered(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)

	coreconfig.Reset()
	config.Set(coreconfig.NamespacesDefault, "default")

	or.mim.On("GetRootOrg", or.ctx).Return(nil, fmt.Errorf("pop"))

	or.mdi.On("GetMessages", or.ctx, "ns", mock.Anything).Return(nil, nil, nil)

	or.config.Multiparty.Org.Name = "org1"
	or.config.Multiparty.Node.Name = "node1"

	or.mem.On("GetPlugins").Return(mockEventPlugins)

	mpStatus, err := or.GetMultipartyStatus(or.ctx)
	assert.NoError(t, err)

	assert.Equal(t, true, mpStatus.Enabled)
	assert.Equal(t, core.NamespaceRegistrationStatusUnregistered, mpStatus.Org.Status)
	assert.Nil(t, mpStatus.Org.PendingRegistrationMessageID)
	assert.Equal(t, core.NamespaceRegistrationStatusUnregistered, mpStatus.Node.Status)
	assert.Nil(t, mpStatus.Node.PendingRegistrationMessageID)

}

func TestGetMultipartyStatusRegisteringOrg(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)

	coreconfig.Reset()
	config.Set(coreconfig.NamespacesDefault, "default")

	or.mim.On("GetRootOrg", or.ctx).Return(nil, fmt.Errorf("pop"))

	msgID := fftypes.NewUUID()
	msg := &core.Message{
		Header: core.MessageHeader{
			ID: msgID,
		},
	}
	or.mdi.On("GetMessages", or.ctx, "ns", mock.Anything).Return([]*core.Message{msg}, nil, nil)
	or.mdm.On("GetMessageDataCached", or.ctx, mock.Anything).Return(core.DataArray{
		{Value: fftypes.JSONAnyPtr(`{"identity":{"did":"did:firefly:org/org1","type":"org"}}`)},
	}, true, nil)

	or.config.Multiparty.Org.Name = "org1"
	or.config.Multiparty.Node.Name = "node1"

	or.mem.On("GetPlugins").Return(mockEventPlugins)

	mpStatus, err := or.GetMultipartyStatus(or.ctx)
	assert.NoError(t, err)

	assert.Equal(t, true, mpStatus.Enabled)
	assert.Equal(t, core.NamespaceRegistrationStatusRegistering, mpStatus.Org.Status)
	assert.Equal(t, msgID, mpStatus.Org.PendingRegistrationMessageID)
	assert.Equal(t, core.NamespaceRegistrationStatusUnregistered, mpStatus.Node.Status)
	assert.Nil(t, mpStatus.Node.PendingRegistrationMessageID)

}

func TestGetMultipartyStatusMismatchedOrgRegistration(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)

	coreconfig.Reset()
	config.Set(coreconfig.NamespacesDefault, "default")

	or.mim.On("GetRootOrg", or.ctx).Return(nil, fmt.Errorf("pop"))

	msgID := fftypes.NewUUID()
	msg := &core.Message{
		Header: core.MessageHeader{
			ID: msgID,
		},
	}
	or.mdi.On("GetMessages", or.ctx, "ns", mock.Anything).Return([]*core.Message{msg}, nil, nil)
	or.mdm.On("GetMessageDataCached", or.ctx, mock.Anything).Return(core.DataArray{
		{Value: fftypes.JSONAnyPtr(`{"identity":{"did":"did:firefly:org/node1","type":"node"}}`)},
	}, true, nil)

	or.config.Multiparty.Org.Name = "org1"
	or.config.Multiparty.Node.Name = "node1"

	or.mem.On("GetPlugins").Return(mockEventPlugins)

	mpStatus, err := or.GetMultipartyStatus(or.ctx)
	assert.NoError(t, err)

	assert.Equal(t, true, mpStatus.Enabled)
	assert.Equal(t, core.NamespaceRegistrationStatusUnknown, mpStatus.Org.Status)
	assert.Nil(t, mpStatus.Org.PendingRegistrationMessageID)
	assert.Equal(t, core.NamespaceRegistrationStatusUnknown, mpStatus.Node.Status)
	assert.Nil(t, mpStatus.Node.PendingRegistrationMessageID)
}

func TestGetMultipartyStatusOrgBadMessage(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)

	coreconfig.Reset()
	config.Set(coreconfig.NamespacesDefault, "default")

	or.mim.On("GetRootOrg", or.ctx).Return(nil, fmt.Errorf("pop"))

	msgID := fftypes.NewUUID()
	msg := &core.Message{
		Header: core.MessageHeader{
			ID: msgID,
		},
	}
	or.mdi.On("GetMessages", or.ctx, "ns", mock.Anything).Return([]*core.Message{msg}, nil, nil)
	or.mdm.On("GetMessageDataCached", or.ctx, mock.Anything).Return(core.DataArray{
		{Value: fftypes.JSONAnyPtr(`invalid`)},
	}, true, nil)

	or.config.Multiparty.Org.Name = "org1"
	or.config.Multiparty.Node.Name = "node1"

	or.mem.On("GetPlugins").Return(mockEventPlugins)

	_, err := or.GetMultipartyStatus(or.ctx)
	assert.Regexp(t, "FF10471", err)

}

func TestGetMultipartyStatusMessageErrorRegisteringOrg(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)

	coreconfig.Reset()
	config.Set(coreconfig.NamespacesDefault, "default")

	or.mim.On("GetRootOrg", or.ctx).Return(nil, fmt.Errorf("pop"))

	or.mdi.On("GetMessages", or.ctx, "ns", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	or.config.Multiparty.Org.Name = "org1"
	or.config.Multiparty.Node.Name = "node1"

	or.mem.On("GetPlugins").Return(mockEventPlugins)

	_, err := or.GetMultipartyStatus(or.ctx)
	assert.Regexp(t, "pop", err)

}

func TestGetMultipartyStatusRegisteringNode(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)

	coreconfig.Reset()
	config.Set(coreconfig.NamespacesDefault, "default")

	orgID := fftypes.NewUUID()

	or.mim.On("GetRootOrg", or.ctx).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			ID:        orgID,
			Name:      "org1",
			Namespace: "ns",
			DID:       "did:firefly:org/org1",
		},
	}, nil)
	or.mim.On("GetLocalNode", or.ctx).Return(nil, nil)
	or.mdi.On("GetVerifiers", or.ctx, "ns", mock.Anything).Return([]*core.Verifier{
		{Hash: fftypes.NewRandB32(), VerifierRef: core.VerifierRef{
			Type:  core.VerifierTypeEthAddress,
			Value: "0x12345",
		}},
	}, nil, nil)

	or.config.Multiparty.Org.Name = "org1"
	or.config.Multiparty.Node.Name = "node1"

	or.mem.On("GetPlugins").Return(mockEventPlugins)

	msgID := fftypes.NewUUID()
	or.mdi.On("GetMessages", or.ctx, "ns", mock.Anything).Return([]*core.Message{{
		Header: core.MessageHeader{
			ID: msgID,
		},
	}}, nil, nil)
	or.mdm.On("GetMessageDataCached", or.ctx, mock.Anything).Return(core.DataArray{
		{Value: fftypes.JSONAnyPtr(`{"identity":{"did":"did:firefly:node/node1","type":"node"}}`)},
	}, true, nil)

	mpStatus, err := or.GetMultipartyStatus(or.ctx)
	assert.NoError(t, err)

	assert.Equal(t, true, mpStatus.Enabled)
	assert.Equal(t, core.NamespaceRegistrationStatusRegistered, mpStatus.Org.Status)
	assert.Nil(t, mpStatus.Org.PendingRegistrationMessageID)
	assert.Equal(t, core.NamespaceRegistrationStatusRegistering, mpStatus.Node.Status)
	assert.Equal(t, msgID, mpStatus.Node.PendingRegistrationMessageID)

}

func TestGetMultipartyStatusMismatchedNodeRegistration(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)

	coreconfig.Reset()
	config.Set(coreconfig.NamespacesDefault, "default")

	orgID := fftypes.NewUUID()

	or.mim.On("GetRootOrg", or.ctx).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			ID:        orgID,
			Name:      "org1",
			Namespace: "ns",
			DID:       "did:firefly:org/org1",
		},
	}, nil)
	or.mim.On("GetLocalNode", or.ctx).Return(nil, nil)
	or.mdi.On("GetVerifiers", or.ctx, "ns", mock.Anything).Return([]*core.Verifier{
		{Hash: fftypes.NewRandB32(), VerifierRef: core.VerifierRef{
			Type:  core.VerifierTypeEthAddress,
			Value: "0x12345",
		}},
	}, nil, nil)

	or.config.Multiparty.Org.Name = "org1"
	or.config.Multiparty.Node.Name = "node1"

	or.mem.On("GetPlugins").Return(mockEventPlugins)

	msgID := fftypes.NewUUID()
	or.mdi.On("GetMessages", or.ctx, "ns", mock.Anything).Return([]*core.Message{{
		Header: core.MessageHeader{
			ID: msgID,
		},
	}}, nil, nil)
	or.mdm.On("GetMessageDataCached", or.ctx, mock.Anything).Return(core.DataArray{
		{Value: fftypes.JSONAnyPtr(`{"identity":{"did":"did:firefly:org/org1","type":"org"}}`)},
	}, true, nil)

	mpStatus, err := or.GetMultipartyStatus(or.ctx)
	assert.NoError(t, err)

	assert.Equal(t, true, mpStatus.Enabled)
	assert.Equal(t, core.NamespaceRegistrationStatusRegistered, mpStatus.Org.Status)
	assert.Equal(t, core.NamespaceRegistrationStatusUnknown, mpStatus.Node.Status)
	assert.Nil(t, mpStatus.Node.PendingRegistrationMessageID)
}
func TestGetMultipartyStatusNodeBadMessage(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)

	coreconfig.Reset()
	config.Set(coreconfig.NamespacesDefault, "default")

	orgID := fftypes.NewUUID()

	or.mim.On("GetRootOrg", or.ctx).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			ID:        orgID,
			Name:      "org1",
			Namespace: "ns",
			DID:       "did:firefly:org/org1",
		},
	}, nil)
	or.mim.On("GetLocalNode", or.ctx).Return(nil, nil)
	or.mdi.On("GetVerifiers", or.ctx, "ns", mock.Anything).Return([]*core.Verifier{
		{Hash: fftypes.NewRandB32(), VerifierRef: core.VerifierRef{
			Type:  core.VerifierTypeEthAddress,
			Value: "0x12345",
		}},
	}, nil, nil)

	or.config.Multiparty.Org.Name = "org1"
	or.config.Multiparty.Node.Name = "node1"

	or.mem.On("GetPlugins").Return(mockEventPlugins)

	msgID := fftypes.NewUUID()
	or.mdi.On("GetMessages", or.ctx, "ns", mock.Anything).Return([]*core.Message{{
		Header: core.MessageHeader{
			ID: msgID,
		},
	}}, nil, nil)
	or.mdm.On("GetMessageDataCached", or.ctx, mock.Anything).Return(core.DataArray{
		{Value: fftypes.JSONAnyPtr(`invalid`)},
	}, true, nil)

	_, err := or.GetMultipartyStatus(or.ctx)
	assert.Regexp(t, "FF10471", err)

}

func TestGetMultipartyStatusMessageErrorRegisteringNode(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)

	coreconfig.Reset()
	config.Set(coreconfig.NamespacesDefault, "default")

	orgID := fftypes.NewUUID()

	or.mim.On("GetRootOrg", or.ctx).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			ID:        orgID,
			Name:      "org1",
			Namespace: "ns",
			DID:       "did:firefly:org/org1",
		},
	}, nil)
	or.mim.On("GetLocalNode", or.ctx).Return(nil, nil)
	or.mdi.On("GetVerifiers", or.ctx, "ns", mock.Anything).Return([]*core.Verifier{
		{Hash: fftypes.NewRandB32(), VerifierRef: core.VerifierRef{
			Type:  core.VerifierTypeEthAddress,
			Value: "0x12345",
		}},
	}, nil, nil)

	or.config.Multiparty.Org.Name = "org1"
	or.config.Multiparty.Node.Name = "node1"

	or.mem.On("GetPlugins").Return(mockEventPlugins)

	or.mdi.On("GetMessages", or.ctx, "ns", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	_, err := or.GetMultipartyStatus(or.ctx)
	assert.Regexp(t, "pop", err)

}

func TestGetMultipartyStatusUnregisteredNode(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)

	coreconfig.Reset()
	config.Set(coreconfig.NamespacesDefault, "default")

	orgID := fftypes.NewUUID()

	or.mim.On("GetRootOrg", or.ctx).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			ID:        orgID,
			Name:      "org1",
			Namespace: "ns",
			DID:       "did:firefly:org/org1",
		},
	}, nil)
	or.mim.On("GetLocalNode", or.ctx).Return(nil, nil)
	or.mdi.On("GetVerifiers", or.ctx, "ns", mock.Anything).Return([]*core.Verifier{
		{Hash: fftypes.NewRandB32(), VerifierRef: core.VerifierRef{
			Type:  core.VerifierTypeEthAddress,
			Value: "0x12345",
		}},
	}, nil, nil)

	or.config.Multiparty.Org.Name = "org1"
	or.config.Multiparty.Node.Name = "node1"

	or.mem.On("GetPlugins").Return(mockEventPlugins)

	or.mdi.On("GetMessages", or.ctx, "ns", mock.Anything).Return(nil, nil, nil)

	mpStatus, err := or.GetMultipartyStatus(or.ctx)
	assert.NoError(t, err)

	assert.Equal(t, true, mpStatus.Enabled)
	assert.Equal(t, core.NamespaceRegistrationStatusRegistered, mpStatus.Org.Status)
	assert.Nil(t, mpStatus.Org.PendingRegistrationMessageID)
	assert.Equal(t, core.NamespaceRegistrationStatusUnregistered, mpStatus.Node.Status)
	assert.Nil(t, mpStatus.Node.PendingRegistrationMessageID)

}

func TestGetMultipartyStatusRegistered(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)

	coreconfig.Reset()
	config.Set(coreconfig.NamespacesDefault, "default")

	orgID := fftypes.NewUUID()

	or.mim.On("GetRootOrg", or.ctx).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			ID:        orgID,
			Name:      "org1",
			Namespace: "ns",
			DID:       "did:firefly:org/org1",
		},
	}, nil)
	or.mim.On("GetLocalNode", or.ctx).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			Parent: orgID,
		},
	}, nil)
	or.mdi.On("GetVerifiers", or.ctx, "ns", mock.Anything).Return([]*core.Verifier{
		{Hash: fftypes.NewRandB32(), VerifierRef: core.VerifierRef{
			Type:  core.VerifierTypeEthAddress,
			Value: "0x12345",
		}},
	}, nil, nil)

	or.config.Multiparty.Org.Name = "org1"
	or.config.Multiparty.Node.Name = "node1"

	or.mem.On("GetPlugins").Return(mockEventPlugins)

	mpStatus, err := or.GetMultipartyStatus(or.ctx)
	assert.NoError(t, err)

	assert.Equal(t, true, mpStatus.Enabled)
	assert.Equal(t, core.NamespaceRegistrationStatusRegistered, mpStatus.Org.Status)
	assert.Nil(t, mpStatus.Org.PendingRegistrationMessageID)
	assert.Equal(t, core.NamespaceRegistrationStatusRegistered, mpStatus.Node.Status)
	assert.Nil(t, mpStatus.Node.PendingRegistrationMessageID)

}

func TestGetMultipartyStatusErrorStatus(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)

	coreconfig.Reset()
	config.Set(coreconfig.NamespacesDefault, "default")

	orgID := fftypes.NewUUID()

	or.mim.On("GetRootOrg", or.ctx).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			ID:        orgID,
			Name:      "org1",
			Namespace: "ns",
			DID:       "did:firefly:org/org1",
		},
	}, nil)
	or.mdi.On("GetVerifiers", or.ctx, "ns", mock.Anything).Return(nil, nil, fmt.Errorf("pop"))

	or.config.Multiparty.Org.Name = "org1"
	or.config.Multiparty.Node.Name = "node1"

	or.mem.On("GetPlugins").Return(mockEventPlugins)

	_, err := or.GetMultipartyStatus(or.ctx)
	assert.Regexp(t, "pop", err)

}

func TestCheckRegistrationType(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)

	coreconfig.Reset()
	config.Set(coreconfig.NamespacesDefault, "default")

	or.config.Multiparty.Org.Name = "org1"
	or.config.Multiparty.Node.Name = "node1"

	msg := core.MessageInOut{
		InlineData: []*core.DataRefOrValue{{
			Value: fftypes.JSONAnyPtr(`{"identity":{"did":"did:firefly:org/org1","type":"org"}}`),
		}},
	}
	match, err := or.checkRegistrationType(or.ctx, &msg, core.IdentityTypeOrg)
	assert.NoError(t, err)
	assert.True(t, match)

	msg = core.MessageInOut{
		InlineData: []*core.DataRefOrValue{{
			Value: fftypes.JSONAnyPtr(`{"identity":{"did":"did:firefly:node/node1","type":"node"}}`),
		}},
	}
	match, err = or.checkRegistrationType(or.ctx, &msg, core.IdentityTypeNode)
	assert.NoError(t, err)
	assert.True(t, match)

	msg = core.MessageInOut{
		InlineData: []*core.DataRefOrValue{{
			Value: fftypes.JSONAnyPtr(`{"identity":{"did":"did:firefly:node/node1","type":"org"}}`),
		}},
	}
	match, err = or.checkRegistrationType(or.ctx, &msg, core.IdentityTypeNode)
	assert.NoError(t, err)
	assert.False(t, match)

	msg = core.MessageInOut{
		InlineData: []*core.DataRefOrValue{{
			Value: fftypes.JSONAnyPtr(`{"identity":{"did":"did:firefly:node/node1","type":"custom"}}`),
		}},
	}
	match, err = or.checkRegistrationType(or.ctx, &msg, core.IdentityTypeCustom)
	assert.Regexp(t, "FF10470", err)
	assert.False(t, match)

	msg = core.MessageInOut{
		InlineData: []*core.DataRefOrValue{{
			Value: fftypes.JSONAnyPtr(`{"identity":{"did":"did:firefly:node/node1","type":"custom"}}`),
		}},
	}
	match, err = or.checkRegistrationType(or.ctx, &msg, core.IdentityTypeCustom)
	assert.Regexp(t, "FF10470", err)
	assert.False(t, match)

	msg = core.MessageInOut{
		InlineData: []*core.DataRefOrValue{{
			Value: fftypes.JSONAnyPtr(`notvalid`),
		}},
	}
	match, err = or.checkRegistrationType(or.ctx, &msg, core.IdentityTypeCustom)
	assert.Regexp(t, "FF10471", err)
	assert.False(t, match)

	msg = core.MessageInOut{
		Message: core.Message{},
	}
	match, err = or.checkRegistrationType(or.ctx, &msg, core.IdentityTypeCustom)
	assert.Regexp(t, "FF10469", err)
	assert.False(t, match)

	match, err = or.checkRegistrationType(or.ctx, nil, core.IdentityTypeCustom)
	assert.Regexp(t, "FF10469", err)
	assert.False(t, match)

}

func TestGetMultipartyActiveListenerStatus(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)

	coreconfig.Reset()
	config.Set(coreconfig.NamespacesDefault, "default")

	orgID := fftypes.NewUUID()

	or.mim.On("GetRootOrg", or.ctx).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			ID:        orgID,
			Name:      "org1",
			Namespace: "ns",
			DID:       "did:firefly:org/org1",
		},
	}, nil)
	or.mim.On("GetLocalNode", or.ctx).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			Parent: orgID,
		},
	}, nil)
	or.mdi.On("GetVerifiers", or.ctx, "ns", mock.Anything).Return([]*core.Verifier{
		{Hash: fftypes.NewRandB32(), VerifierRef: core.VerifierRef{
			Type:  core.VerifierTypeEthAddress,
			Value: "0x12345",
		}},
	}, nil, nil)

	or.config.Multiparty.Org.Name = "org1"
	or.config.Multiparty.Node.Name = "node1"
	or.namespace.Contracts = &core.MultipartyContracts{
		Active: &core.MultipartyContract{
			Info: core.MultipartyContractInfo{
				Subscription: "sub1",
			},
		},
		Terminated: []*core.MultipartyContract{},
	}

	or.mem.On("GetPlugins").Return(mockEventPlugins)

	or.mbi.On("GetContractListenerStatus", or.ctx, "ns", "sub1", false).Return(true, nil, core.ContractListenerStatusSynced, nil)

	mpStatus, err := or.GetMultipartyStatus(or.ctx)
	assert.NoError(t, err)

	assert.Equal(t, core.ContractListenerStatusSynced, mpStatus.Contracts.Active.Status)

}

func TestGetMultipartyErrorActiveListenerStatus(t *testing.T) {
	or := newTestOrchestrator()
	defer or.cleanup(t)

	coreconfig.Reset()
	config.Set(coreconfig.NamespacesDefault, "default")

	orgID := fftypes.NewUUID()

	or.mim.On("GetRootOrg", or.ctx).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			ID:        orgID,
			Name:      "org1",
			Namespace: "ns",
			DID:       "did:firefly:org/org1",
		},
	}, nil)
	or.mim.On("GetLocalNode", or.ctx).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			Parent: orgID,
		},
	}, nil)
	or.mdi.On("GetVerifiers", or.ctx, "ns", mock.Anything).Return([]*core.Verifier{
		{Hash: fftypes.NewRandB32(), VerifierRef: core.VerifierRef{
			Type:  core.VerifierTypeEthAddress,
			Value: "0x12345",
		}},
	}, nil, nil)

	or.config.Multiparty.Org.Name = "org1"
	or.config.Multiparty.Node.Name = "node1"
	or.namespace.Contracts = &core.MultipartyContracts{
		Active: &core.MultipartyContract{
			Info: core.MultipartyContractInfo{
				Subscription: "sub1",
			},
		},
		Terminated: []*core.MultipartyContract{},
	}

	or.mem.On("GetPlugins").Return(mockEventPlugins)

	or.mbi.On("GetContractListenerStatus", or.ctx, "ns", "sub1", false).Return(false, nil, core.ContractListenerStatusUnknown, fmt.Errorf("pop"))

	_, err := or.GetMultipartyStatus(or.ctx)
	assert.Regexp(t, "pop", err)

}
