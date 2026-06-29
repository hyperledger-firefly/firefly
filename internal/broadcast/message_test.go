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

package broadcast

import (
	"context"
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/internal/data"
	"github.com/hyperledger-firefly/firefly/internal/syncasync"
	"github.com/hyperledger-firefly/firefly/mocks/datamocks"
	"github.com/hyperledger-firefly/firefly/mocks/identitymanagermocks"
	"github.com/hyperledger-firefly/firefly/mocks/syncasyncmocks"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestBroadcastMessageOk(t *testing.T) {
	bm, cancel := newTestBroadcastWithMetrics(t)
	defer cancel()
	mdm := bm.data.(*datamocks.Manager)
	mim := bm.identity.(*identitymanagermocks.Manager)

	ctx := context.Background()
	mdm.On("ResolveInlineData", ctx, mock.Anything).Return(nil)
	mdm.On("WriteNewMessage", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	mim.On("ResolveInputSigningIdentity", ctx, mock.Anything).Return(nil)

	msg, err := bm.BroadcastMessage(ctx, &core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				SignerRef: core.SignerRef{
					Author: "did:firefly:org/abcd",
					Key:    "0x12345",
				},
			},
		},
		InlineData: core.InlineData{
			{Value: fftypes.JSONAnyPtr(`{"hello": "world"}`)},
		},
	}, false)
	assert.NoError(t, err)
	assert.Equal(t, "ns1", msg.Header.Namespace)

	mim.AssertExpectations(t)
	mdm.AssertExpectations(t)
}

func TestBroadcastMessageWriteFail(t *testing.T) {
	bm, cancel := newTestBroadcastWithMetrics(t)
	defer cancel()
	mdm := bm.data.(*datamocks.Manager)
	mim := bm.identity.(*identitymanagermocks.Manager)

	ctx := context.Background()
	mdm.On("ResolveInlineData", ctx, mock.Anything).Return(nil)
	mdm.On("WriteNewMessage", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("pop"))
	mim.On("ResolveInputSigningIdentity", ctx, mock.Anything).Return(nil)

	msg, err := bm.BroadcastMessage(ctx, &core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				SignerRef: core.SignerRef{
					Author: "did:firefly:org/abcd",
					Key:    "0x12345",
				},
			},
		},
		InlineData: core.InlineData{
			{Value: fftypes.JSONAnyPtr(`{"hello": "world"}`)},
		},
	}, false)
	assert.EqualError(t, err, "pop")
	assert.Equal(t, "ns1", msg.Header.Namespace)

	mim.AssertExpectations(t)
	mdm.AssertExpectations(t)
}

func TestBroadcastMessageWaitConfirmOk(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()
	mdm := bm.data.(*datamocks.Manager)
	msa := bm.syncasync.(*syncasyncmocks.Bridge)
	mim := bm.identity.(*identitymanagermocks.Manager)

	ctx := context.Background()
	mdm.On("ResolveInlineData", ctx, mock.Anything).Return(nil)
	mim.On("ResolveInputSigningIdentity", ctx, mock.Anything).Return(nil)

	replyMsg := &core.Message{
		Header: core.MessageHeader{
			Namespace: "ns1",
			ID:        fftypes.NewUUID(),
		},
	}
	msa.On("WaitForMessage", ctx, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			send := args[2].(syncasync.SendFunction)
			send(ctx)
		}).
		Return(replyMsg, nil)
	mdm.On("WriteNewMessage", ctx, mock.Anything, mock.Anything).Return(nil)

	msg, err := bm.BroadcastMessage(ctx, &core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				SignerRef: core.SignerRef{
					Author: "did:firefly:org/abcd",
					Key:    "0x12345",
				},
			},
		},
		InlineData: core.InlineData{
			{Value: fftypes.JSONAnyPtr(`{"hello": "world"}`)},
		},
	}, true)
	assert.NoError(t, err)
	assert.Equal(t, replyMsg, msg)
	assert.Equal(t, "ns1", msg.Header.Namespace)

	msa.AssertExpectations(t)
	mdm.AssertExpectations(t)
}

func TestBroadcastMessageTooLarge(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	bm.maxBatchPayloadLength = 1000000
	defer cancel()
	mdm := bm.data.(*datamocks.Manager)
	mim := bm.identity.(*identitymanagermocks.Manager)

	ctx := context.Background()
	mdm.On("ResolveInlineData", ctx, mock.Anything).Run(
		func(args mock.Arguments) {
			newMsg := args[1].(*data.NewMessage)
			newMsg.Message.Data = core.DataRefs{
				{ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32(), ValueSize: 1000001},
			}
		}).
		Return(nil)
	mim.On("ResolveInputSigningIdentity", ctx, mock.Anything).Return(nil)

	_, err := bm.BroadcastMessage(ctx, &core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				SignerRef: core.SignerRef{
					Author: "did:firefly:org/abcd",
					Key:    "0x12345",
				},
			},
		},
		InlineData: core.InlineData{
			{Value: fftypes.JSONAnyPtr(`{"hello": "world"}`)},
		},
	}, true)
	assert.Regexp(t, "FF10327", err)

	mdm.AssertExpectations(t)
}

func TestBroadcastMessageBadInput(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()
	mdm := bm.data.(*datamocks.Manager)
	mim := bm.identity.(*identitymanagermocks.Manager)

	ctx := context.Background()
	mdm.On("ResolveInlineData", ctx, mock.Anything).Return(fmt.Errorf("pop"))
	mim.On("ResolveInputSigningIdentity", ctx, mock.Anything).Return(nil)

	_, err := bm.BroadcastMessage(ctx, &core.MessageInOut{
		InlineData: core.InlineData{
			{Value: fftypes.JSONAnyPtr(`{"hello": "world"}`)},
		},
	}, false)
	assert.EqualError(t, err, "pop")

	mdm.AssertExpectations(t)
}

func TestBroadcastMessageBadIdentity(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()

	ctx := context.Background()
	mim := bm.identity.(*identitymanagermocks.Manager)
	mim.On("ResolveInputSigningIdentity", ctx, mock.Anything).Return(fmt.Errorf("pop"))

	_, err := bm.BroadcastMessage(ctx, &core.MessageInOut{
		InlineData: core.InlineData{
			{Value: fftypes.JSONAnyPtr(`{"hello": "world"}`)},
		},
	}, false)
	assert.Regexp(t, "FF10206", err)

	mim.AssertExpectations(t)
}

func TestBroadcastPrepare(t *testing.T) {
	bm, cancel := newTestBroadcast(t)
	defer cancel()
	mdm := bm.data.(*datamocks.Manager)
	mim := bm.identity.(*identitymanagermocks.Manager)

	ctx := context.Background()
	mdm.On("ResolveInlineData", ctx, mock.Anything).Return(nil)
	mim.On("ResolveInputSigningIdentity", ctx, mock.Anything).Return(nil)

	msg := &core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				SignerRef: core.SignerRef{
					Author: "did:firefly:org/abcd",
					Key:    "0x12345",
				},
			},
		},
		InlineData: core.InlineData{
			{Value: fftypes.JSONAnyPtr(`{"hello": "world"}`)},
		},
	}
	sender := bm.NewBroadcast(msg)
	err := sender.Prepare(ctx)

	assert.NoError(t, err)
	assert.Equal(t, "ns1", msg.Header.Namespace)

	mdm.AssertExpectations(t)
}

func TestNewMessageContractInvoke(t *testing.T) {

	bm, cancel := newTestBroadcast(t)
	defer cancel()

	sender := bm.NewBroadcast(&core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				TxType: core.TransactionTypeContractInvokePin,
			},
		},
	})
	assert.NotNil(t, sender)

}
