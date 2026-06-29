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
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/mocks/assetmocks"
	"github.com/hyperledger-firefly/firefly/mocks/blockchainmocks"
	"github.com/hyperledger-firefly/firefly/mocks/contractmocks"
	"github.com/hyperledger-firefly/firefly/mocks/databasemocks"
	"github.com/hyperledger-firefly/firefly/mocks/dataexchangemocks"
	"github.com/hyperledger-firefly/firefly/mocks/datamocks"
	"github.com/hyperledger-firefly/firefly/mocks/identitymanagermocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
)

type testDefinitionHandler struct {
	definitionHandler

	mdi *databasemocks.Plugin
	mbi *blockchainmocks.Plugin
	mdx *dataexchangemocks.Plugin
	mim *identitymanagermocks.Manager
	mdm *datamocks.Manager
	mam *assetmocks.Manager
	mcm *contractmocks.Manager
}

func (tdh *testDefinitionHandler) cleanup(t *testing.T) {
	tdh.mdi.AssertExpectations(t)
	tdh.mbi.AssertExpectations(t)
	tdh.mdx.AssertExpectations(t)
	tdh.mim.AssertExpectations(t)
	tdh.mdm.AssertExpectations(t)
	tdh.mam.AssertExpectations(t)
	tdh.mcm.AssertExpectations(t)
}

func newTestDefinitionHandler(t *testing.T) (*testDefinitionHandler, *testDefinitionBatchState) {
	mdi := &databasemocks.Plugin{}
	mbi := &blockchainmocks.Plugin{}
	mdx := &dataexchangemocks.Plugin{}
	mdm := &datamocks.Manager{}
	mim := &identitymanagermocks.Manager{}
	mam := &assetmocks.Manager{}
	mcm := &contractmocks.Manager{}
	tokenNames := make(map[string]string)
	tokenNames["remote1"] = "connector1"
	mbi.On("VerifierType").Return(core.VerifierTypeEthAddress).Maybe()
	ns := &core.Namespace{Name: "ns1", NetworkName: "ns1"}
	dh, _ := newDefinitionHandler(context.Background(), ns, false, mdi, mbi, mdx, mdm, mim, mam, mcm, tokenNames)
	return &testDefinitionHandler{
		definitionHandler: *dh,
		mdi:               mdi,
		mbi:               mbi,
		mdx:               mdx,
		mim:               mim,
		mdm:               mdm,
		mam:               mam,
		mcm:               mcm,
	}, newTestDefinitionBatchState(t)
}

type testDefinitionBatchState struct {
	core.BatchState
	t *testing.T
}

func newTestDefinitionBatchState(t *testing.T) *testDefinitionBatchState {
	return &testDefinitionBatchState{
		BatchState: core.BatchState{
			PendingConfirms: make(map[fftypes.UUID]*core.Message),
			PreFinalize:     make([]func(ctx context.Context) error, 0),
			Finalize:        make([]func(ctx context.Context) error, 0),
		},
		t: t,
	}
}

func (bs *testDefinitionBatchState) assertNoFinalizers() {
	assert.Empty(bs.t, bs.PreFinalize)
	assert.Empty(bs.t, bs.Finalize)
}

func TestInitFail(t *testing.T) {
	_, err := newDefinitionHandler(context.Background(), &core.Namespace{}, false, nil, nil, nil, nil, nil, nil, nil, nil)
	assert.Regexp(t, "FF10128", err)
}

func TestHandleDefinitionBroadcastUnknown(t *testing.T) {
	dh, bs := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	action, err := dh.HandleDefinitionBroadcast(context.Background(), &bs.BatchState, &core.Message{
		Header: core.MessageHeader{
			Tag: "unknown",
		},
	}, core.DataArray{}, fftypes.NewUUID())
	assert.Equal(t, HandlerResult{Action: core.ActionReject}, action)
	assert.Error(t, err)
	bs.assertNoFinalizers()
}

func TestGetSystemBroadcastPayloadMissingData(t *testing.T) {
	dh, _ := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	valid := dh.getSystemBroadcastPayload(context.Background(), &core.Message{
		Header: core.MessageHeader{
			Tag: "unknown",
		},
	}, core.DataArray{}, nil)
	assert.False(t, valid)
}

func TestGetSystemBroadcastPayloadBadJSON(t *testing.T) {
	dh, _ := newTestDefinitionHandler(t)
	defer dh.cleanup(t)

	valid := dh.getSystemBroadcastPayload(context.Background(), &core.Message{
		Header: core.MessageHeader{
			Tag: "unknown",
		},
	}, core.DataArray{}, nil)
	assert.False(t, valid)
}

func TestActionEnum(t *testing.T) {
	assert.Equal(t, "confirm", fmt.Sprintf("%s", core.ActionConfirm))
	assert.Equal(t, "reject", fmt.Sprintf("%s", core.ActionReject))
	assert.Equal(t, "retry", fmt.Sprintf("%s", core.ActionRetry))
	assert.Equal(t, "wait", fmt.Sprintf("%s", core.ActionWait))
	assert.Equal(t, "unknown", fmt.Sprintf("%s", core.MessageAction(999)))
}
