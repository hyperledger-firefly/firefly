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

package definitions

import (
	"context"
	"encoding/json"

	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly/internal/assets"
	"github.com/hyperledger/firefly/internal/broadcast"
	"github.com/hyperledger/firefly/internal/contracts"
	"github.com/hyperledger/firefly/internal/coremsgs"
	"github.com/hyperledger/firefly/internal/data"
	"github.com/hyperledger/firefly/internal/identity"
	"github.com/hyperledger/firefly/internal/syncasync"
	"github.com/hyperledger/firefly/pkg/blockchain"
	"github.com/hyperledger/firefly/pkg/core"
	"github.com/hyperledger/firefly/pkg/database"
	"github.com/hyperledger/firefly/pkg/dataexchange"
)

type Sender interface {
	core.Named

	ClaimIdentity(ctx context.Context, def *core.IdentityClaim, signingIdentity *core.SignerRef, parentSigner *core.SignerRef) error
	UpdateIdentity(ctx context.Context, identity *core.Identity, def *core.IdentityUpdate, signingIdentity *core.SignerRef, waitConfirm bool) error
	DefineDatatype(ctx context.Context, datatype *core.Datatype, waitConfirm bool) error
	DefineTokenPool(ctx context.Context, pool *core.TokenPool, waitConfirm bool) error
	PublishTokenPool(ctx context.Context, poolNameOrID, networkName string, waitConfirm bool) (*core.TokenPool, error)
	DefineFFI(ctx context.Context, ffi *fftypes.FFI, waitConfirm bool, topics []string) error
	PublishFFI(ctx context.Context, name, version, networkName string, waitConfirm bool, topics []string) (*fftypes.FFI, error)
	DefineContractAPI(ctx context.Context, httpServerURL string, api *core.ContractAPI, waitConfirm bool, topics []string) error
	PublishContractAPI(ctx context.Context, httpServerURL, name, networkName string, waitConfirm bool, topics []string) (api *core.ContractAPI, err error)
}

type definitionSender struct {
	ctx                 context.Context
	namespace           string
	multiparty          bool
	database            database.Plugin
	broadcast           broadcast.Manager // optional
	identity            identity.Manager
	data                data.Manager
	contracts           contracts.Manager // optional
	assets              assets.Manager
	handler             *definitionHandler
	tokenBroadcastNames map[string]string // mapping of token connector name => broadcast name
}

// Definitions that get processed immediately will create a temporary batch state and then finalize it inline
func fakeBatch(ctx context.Context, handler func(context.Context, *core.BatchState) (HandlerResult, error)) (err error) {
	var state core.BatchState
	_, err = handler(ctx, &state)
	if err == nil {
		err = state.RunPreFinalize(ctx)
	}
	if err == nil {
		err = state.RunFinalize(ctx)
	}
	return err
}

func NewDefinitionSender(ctx context.Context, ns *core.Namespace, multiparty bool, di database.Plugin, bi blockchain.Plugin, dx dataexchange.Plugin, bm broadcast.Manager, im identity.Manager, dm data.Manager, am assets.Manager, cm contracts.Manager, tokenBroadcastNames map[string]string) (Sender, Handler, error) {
	if di == nil || im == nil || dm == nil {
		return nil, nil, i18n.NewError(ctx, coremsgs.MsgInitializationNilDepError, "DefinitionSender")
	}
	ds := &definitionSender{
		ctx:                 ctx,
		namespace:           ns.Name,
		multiparty:          multiparty,
		database:            di,
		broadcast:           bm,
		identity:            im,
		data:                dm,
		contracts:           cm,
		assets:              am,
		tokenBroadcastNames: tokenBroadcastNames,
	}
	dh, err := newDefinitionHandler(ctx, ns, multiparty, di, bi, dx, dm, im, am, cm, reverseMap(tokenBroadcastNames))
	if err != nil {
		return nil, nil, err
	}

	ds.handler = dh
	return ds, dh, nil
}

// reverseMap reverses the key/values of a given map
func reverseMap(orderedMap map[string]string) map[string]string {
	reverseMap := make(map[string]string, len(orderedMap))
	for k, v := range orderedMap {
		reverseMap[v] = k
	}
	return reverseMap
}

func (ds *definitionSender) Name() string {
	return "DefinitionSender"
}

type sendWrapper struct {
	sender  syncasync.Sender
	message *core.Message
	err     error
}

func wrapSendError(err error) *sendWrapper {
	return &sendWrapper{err: err}
}

func (w *sendWrapper) send(ctx context.Context, waitConfirm bool) (*core.Message, error) {
	switch {
	case w.err != nil:
		return nil, w.err
	case waitConfirm:
		return w.message, w.sender.SendAndWait(ctx)
	default:
		return w.message, w.sender.Send(ctx)
	}
}

func (ds *definitionSender) getSenderDefault(ctx context.Context, def core.Definition, tag string, customTopics []string) *sendWrapper {
	org, err := ds.identity.GetRootOrg(ctx)
	if err != nil {
		return wrapSendError(err)
	}
	return ds.getSender(ctx, def, &core.SignerRef{ /* resolve to node default */
		Author: org.DID,
	}, tag, customTopics)
}

func (ds *definitionSender) getSender(ctx context.Context, def core.Definition, signingIdentity *core.SignerRef, tag string, customTopics []string) *sendWrapper {
	err := ds.identity.ResolveInputSigningIdentity(ctx, signingIdentity)
	if err != nil {
		return wrapSendError(err)
	}
	return ds.getSenderResolved(ctx, def, signingIdentity, tag, customTopics)
}

func (ds *definitionSender) getSenderResolved(ctx context.Context, def core.Definition, signingIdentity *core.SignerRef, tag string, customTopics []string) *sendWrapper {
	b, err := json.Marshal(&def)
	if err != nil {
		return wrapSendError(i18n.WrapError(ctx, err, coremsgs.MsgSerializationFailed))
	}
	topics := fftypes.FFStringArray{def.Topic()}

	var filtered []string
	for _, t := range customTopics {
		if t != "" {
			filtered = append(filtered, t)
		}
	}
	if len(filtered) > 0 {
		topics = fftypes.FFStringArray(filtered)
	}
	dataValue := fftypes.JSONAnyPtrBytes(b)
	message := &core.MessageInOut{
		Message: core.Message{
			Header: core.MessageHeader{
				Type:      core.MessageTypeDefinition,
				Topics:    topics,
				Tag:       tag,
				TxType:    core.TransactionTypeBatchPin,
				SignerRef: *signingIdentity,
			},
		},
		InlineData: core.InlineData{
			&core.DataRefOrValue{Value: dataValue},
		},
	}
	return &sendWrapper{
		message: &message.Message,
		sender:  ds.broadcast.NewBroadcast(message),
	}
}
