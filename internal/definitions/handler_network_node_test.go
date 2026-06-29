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

package definitions

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

const oldNodeExample = `
{
    "header": {
      "id": "811de0e6-6c07-445f-99fb-e2f457a9f140",
      "type": "definition",
      "txtype": "batch_pin",
      "author": "did:firefly:org/f08153cc-c605-4239-9087-e08747e1fb4e",
      "key": "0x214840c7c62cddf7a854a830d55018b38e4e78be",
      "created": "2022-02-24T15:31:01.711696467Z",
      "namespace": "ff_system",
      "topics": [
        "ff_organizations"
      ],
      "tag": "ff_define_node",
      "datahash": "e0bfd8cf53524e28d036b971dfca3dfbd1fb93bc0259d32a9874e569fdbcf814"
    },
    "hash": "fdf86a889c0d7377d0c97e654d9fd9f56a9fc462d0c61162df94f902505f5a85",
    "batch": "46015b75-2d90-4055-9c7c-0ca6e0529961",
    "state": "confirmed",
    "confirmed": "2022-02-24T15:31:03.691365677Z",
    "data": [
      {
        "id": "8e03051d-6bf9-4ceb-9985-ed90e60d9334",
        "hash": "8ffd58985cc09fff2c8b3ca92d11e0b9d86847032d05fc98ecf7f2372f421cce",
        "validator": "definition",
        "value": {
          "id": "d0c4f928-943d-49bc-927e-e9eb8fb8dc00",
          "owner": "0x214840c7c62cddf7a854a830d55018b38e4e78be",
          "name": "node_0",
          "dx": {
            "peer": "member_0",
            "endpoint": {
              "cert": "-----BEGIN CERTIFICATE-----\nMIIC1DCCAbwCCQCdQsqbIH663DANBgkqhkiG9w0BAQsFADAsMRcwFQYDVQQDDA5k\nYXRhZXhjaGFuZ2VfMDERMA8GA1UECgwIbWVtYmVyXzAwHhcNMjIwMjI0MTUzMDE1\nWhcNMjMwMjI0MTUzMDE1WjAsMRcwFQYDVQQDDA5kYXRhZXhjaGFuZ2VfMDERMA8G\nA1UECgwIbWVtYmVyXzAwggEiMA0GCSqGSIb3DQEBAQUAA4IBDwAwggEKAoIBAQC9\nItQpszirxOeONjzZQLgnp6iIUcu0v0NYhJ5QQM/a6JkzcTw+ZoxjwQZIAV/WRgVK\ngnp7Z+BXcGB7TqQsY3501tEG6st8zUgH2RHiIdPll9Uavxws2eQlrvW98STST1S8\n41OmIbetC7TWYLYvjtM2d8KjXgU96KtM6G7sVucOFxAkrM1UPrLVZOoUmUyXxery\nTzC16ssvnPHFylWwSD5PzHDRW3H+hYq6O3VE1VztZGmFQ/+9ZrPv3Io7fDyIa0vm\n7WWFiMFqO96vvh5Gnkzailaqs9ViXp4FE5c9ftEmXmzqI5YpVTI70MHlXKXoarD4\nuZnpRRcqACcBFl463WnzAgMBAAEwDQYJKoZIhvcNAQELBQADggEBAJruH13xnlvf\nat2QgeTsxjG4EQK8TDPEIthaA1eXP/69ShHeYNM62H9qP3QCjbY0i8eN9WdEzfGI\nSIWjDdviSNgPeH4KxyRL0Yiv43en8y0E0UcbqiiQrSdqjTDITBxo61cyOEYMmPiE\nynSPnGzt+iP3C64a/dAwfgTRFihgxc9WT+TcvJoZ58vku/Zi2+uA5qn9uLDHb0gF\nKXrACRvrRqOHXKoT1dJPUBnoiEhK4roB4y2yy0CNUP+tEwGLuGpFlek0GruYYEwz\nfAYpvKW5JGdcjD2SgmJ2iWdQQkhh5rNh5pAdSmzYf/x0psHTpVg0JrSC7et2hi6K\njklYSLaI4pI=\n-----END CERTIFICATE-----\n",
              "endpoint": "https://dataexchange_0:3001",
              "id": "member_0"
            }
          },
          "created": "2022-02-24T15:31:01.670896884Z"
        }
      }
    ]
  }
`

func testDeprecatedRootNode(t *testing.T) (*core.DeprecatedNode, *core.Message, *core.Data) {

	var msgInOut core.MessageInOut
	err := json.Unmarshal([]byte(oldNodeExample), &msgInOut)
	assert.NoError(t, err)

	var node core.DeprecatedNode
	err = json.Unmarshal(msgInOut.InlineData[0].Value.Bytes(), &node)
	assert.NoError(t, err)

	return &node, &msgInOut.Message, &core.Data{
		ID:        msgInOut.InlineData[0].ID,
		Validator: msgInOut.InlineData[0].Validator,
		Namespace: msgInOut.Header.Namespace,
		Hash:      msgInOut.InlineData[0].Hash,
		Value:     msgInOut.InlineData[0].Value,
	}
}

func TestHandleDeprecatedNodeDefinitionOK(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	ctx := context.Background()

	node, msg, data := testDeprecatedRootNode(t)
	parent, _, _ := testDeprecatedRootOrg(t)

	dh.mim.On("FindIdentityForVerifier", ctx, []core.IdentityType{core.IdentityTypeOrg}, &core.VerifierRef{
		Type:  core.VerifierTypeEthAddress,
		Value: node.Owner,
	}).Return(parent.Migrated().Identity, nil)
	dh.mim.On("VerifyIdentityChain", ctx, mock.Anything).Return(parent.Migrated().Identity, false, nil)
	dh.mdi.On("GetIdentityByName", ctx, core.IdentityTypeNode, "ns1", node.Name).Return(nil, nil)
	dh.mdi.On("GetIdentityByID", ctx, "ns1", node.ID).Return(nil, nil)
	dh.mdi.On("GetVerifierByValue", ctx, core.VerifierTypeFFDXPeerID, "ns1", "member_0").Return(nil, nil)
	dh.mdi.On("UpsertIdentity", ctx, mock.MatchedBy(func(identity *core.Identity) bool {
		assert.Equal(t, *msg.Header.ID, *identity.Messages.Claim)
		return true
	}), database.UpsertOptimizationNew).Return(nil)
	dh.mdi.On("UpsertVerifier", ctx, mock.MatchedBy(func(verifier *core.Verifier) bool {
		assert.Equal(t, core.VerifierTypeFFDXPeerID, verifier.Type)
		assert.Equal(t, "member_0", verifier.Value)
		assert.Equal(t, *node.ID, *verifier.Identity)
		return true
	}), database.UpsertOptimizationNew).Return(nil)
	dh.mdi.On("InsertEvent", mock.Anything, mock.MatchedBy(func(event *core.Event) bool {
		return event.Type == core.EventTypeIdentityConfirmed
	})).Return(nil)
	dh.mdx.On("GetPeerID", node.DX.Endpoint).Return("member_0")
	dh.mdx.On("AddNode", ctx, "ns1", node.Name, node.DX.Endpoint).Return(nil)
	dh.mim.On("GetLocalNodeDID", ctx).Return("different node", nil)

	dh.multiparty = true

	action, err := dh.HandleDefinitionBroadcast(ctx, &bs.BatchState, msg, core.DataArray{data}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionConfirm}, action)
	assert.NoError(t, err)

	err = bs.RunPreFinalize(ctx)
	assert.NoError(t, err)
	err = bs.RunFinalize(ctx)
	assert.NoError(t, err)
}

func TestHandleDeprecatedNodeDefinitionBadData(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	ctx := context.Background()

	action, err := dh.handleDeprecatedNodeBroadcast(ctx, &bs.BatchState, &core.Message{}, core.DataArray{})
	assert.Equal(t, HandlerResult{Action: core.ActionReject}, action)
	assert.Error(t, err)

	bs.assertNoFinalizers()
}

func TestHandleDeprecatedNodeDefinitionFailOrgLookup(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	ctx := context.Background()

	node, msg, data := testDeprecatedRootNode(t)

	dh.mim.On("FindIdentityForVerifier", ctx, []core.IdentityType{core.IdentityTypeOrg}, &core.VerifierRef{
		Type:  core.VerifierTypeEthAddress,
		Value: node.Owner,
	}).Return(nil, fmt.Errorf("pop"))

	action, err := dh.handleDeprecatedNodeBroadcast(ctx, &bs.BatchState, msg, core.DataArray{data})
	assert.Equal(t, HandlerResult{Action: core.ActionRetry}, action)
	assert.Regexp(t, "pop", err)

	bs.assertNoFinalizers()

}

func TestHandleDeprecatedNodeDefinitionOrgNotFound(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	ctx := context.Background()

	node, msg, data := testDeprecatedRootNode(t)

	dh.mim.On("FindIdentityForVerifier", ctx, []core.IdentityType{core.IdentityTypeOrg}, &core.VerifierRef{
		Type:  core.VerifierTypeEthAddress,
		Value: node.Owner,
	}).Return(nil, nil)

	action, err := dh.handleDeprecatedNodeBroadcast(ctx, &bs.BatchState, msg, core.DataArray{data})
	assert.Equal(t, HandlerResult{Action: core.ActionReject}, action)
	assert.Error(t, err)

	bs.assertNoFinalizers()

}
