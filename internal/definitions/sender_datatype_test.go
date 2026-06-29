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

package definitions

import (
	"context"
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/mocks/syncasyncmocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestDefineDatatypeBadType(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true
	err := ds.DefineDatatype(context.Background(), &core.Datatype{
		Validator: core.ValidatorType("wrong"),
	}, false)
	assert.Regexp(t, "FF00111.*validator", err)
}

func TestBroadcastDatatypeBadValue(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	ds.mdm.On("CheckDatatype", mock.Anything, mock.Anything).Return(nil)
	ds.mim.On("GetRootOrg", context.Background()).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			DID: "firefly:org1",
		},
	}, nil)
	ds.mim.On("ResolveInputSigningIdentity", mock.Anything, mock.Anything).Return(nil)

	err := ds.DefineDatatype(context.Background(), &core.Datatype{
		Namespace: "ns1",
		Name:      "ent1",
		Version:   "0.0.1",
		Value:     fftypes.JSONAnyPtr(`!unparsable`),
	}, false)
	assert.Regexp(t, "FF10137.*value", err)
}

func TestDefineDatatypeInvalid(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true

	ds.mdm.On("CheckDatatype", mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))

	err := ds.DefineDatatype(context.Background(), &core.Datatype{
		Namespace: "ns1",
		Name:      "ent1",
		Version:   "0.0.1",
		Value:     fftypes.JSONAnyPtr(`{"some": "data"}`),
	}, false)
	assert.EqualError(t, err, "pop")
}

func TestBroadcastOk(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = true
	mms := &syncasyncmocks.Sender{}

	ds.mim.On("GetRootOrg", context.Background()).Return(&core.Identity{
		IdentityBase: core.IdentityBase{
			DID: "firefly:org1",
		},
	}, nil)
	ds.mim.On("ResolveInputSigningIdentity", mock.Anything, mock.Anything).Return(nil)
	ds.mdm.On("CheckDatatype", mock.Anything, mock.Anything).Return(nil)
	ds.mbm.On("NewBroadcast", mock.Anything).Return(mms)
	mms.On("Send", context.Background()).Return(nil)

	err := ds.DefineDatatype(context.Background(), &core.Datatype{
		Namespace: "ns1",
		Name:      "ent1",
		Version:   "0.0.1",
		Value:     fftypes.JSONAnyPtr(`{"some": "data"}`),
	}, false)
	assert.NoError(t, err)

	mms.AssertExpectations(t)
}

func TestDefineDatatypeNonMultiparty(t *testing.T) {
	ds := newTestDefinitionSender(t)
	defer ds.cleanup(t)
	ds.multiparty = false

	err := ds.DefineDatatype(context.Background(), &core.Datatype{
		Namespace: "ns1",
		Name:      "ent1",
		Version:   "0.0.1",
		Value:     fftypes.JSONAnyPtr(`{"some": "data"}`),
	}, false)
	assert.Regexp(t, "FF10414", err)
}
