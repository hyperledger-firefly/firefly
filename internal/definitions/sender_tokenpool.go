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
	"errors"

	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly/internal/coremsgs"
	"github.com/hyperledger/firefly/pkg/core"
)

func (ds *definitionSender) PublishTokenPool(ctx context.Context, poolNameOrID, networkName string, waitConfirm bool) (pool *core.TokenPool, err error) {
	if !ds.multiparty {
		return nil, i18n.NewError(ctx, coremsgs.MsgActionNotSupported)
	}

	var sender *sendWrapper
	err = ds.database.RunAsGroup(ctx, func(ctx context.Context) error {
		if pool, err = ds.assets.GetTokenPoolByNameOrID(ctx, poolNameOrID); err != nil {
			return err
		}
		if pool.Published {
			return i18n.NewError(ctx, coremsgs.MsgAlreadyPublished)
		}
		pool.NetworkName = networkName
		sender = ds.getTokenPoolSender(ctx, pool)
		if sender.err != nil {
			return sender.err
		}
		return sender.sender.Prepare(ctx)
	})
	if err != nil {
		return nil, err
	}

	_, err = sender.send(ctx, waitConfirm)
	return pool, err
}

func (ds *definitionSender) getTokenPoolSender(ctx context.Context, pool *core.TokenPool) *sendWrapper {
	// Map token connector name -> broadcast name
	if broadcastName, exists := ds.tokenBroadcastNames[pool.Connector]; exists {
		pool.Connector = broadcastName
	} else {
		log.L(ctx).Infof("Could not find broadcast name for token connector: %s", pool.Connector)
		return wrapSendError(i18n.NewError(ctx, coremsgs.MsgInvalidConnectorName, broadcastName, "token"))
	}

	if pool.NetworkName == "" {
		pool.NetworkName = pool.Name
	}

	// Validate the pool before sending
	if err := pool.Validate(ctx); err != nil {
		return wrapSendError(err)
	}
	existing, err := ds.database.GetTokenPoolByNetworkName(ctx, ds.namespace, pool.NetworkName)
	if err != nil {
		return wrapSendError(err)
	} else if existing != nil {
		return wrapSendError(i18n.NewError(ctx, coremsgs.MsgNetworkNameExists))
	}
	if pool.Interface != nil && pool.Interface.ID != nil {
		iface, err := ds.database.GetFFIByID(ctx, ds.namespace, pool.Interface.ID)
		switch {
		case err != nil:
			return wrapSendError(err)
		case iface == nil:
			return wrapSendError(i18n.NewError(ctx, coremsgs.MsgContractInterfaceNotFound, pool.Interface.ID))
		case !iface.Published:
			return wrapSendError(i18n.NewError(ctx, coremsgs.MsgContractInterfaceNotPublished, pool.Interface.ID))
		}
	}

	// Prepare the pool definition to be serialized for broadcast
	localName := pool.Name
	pool.Name = ""
	pool.Namespace = ""
	pool.Published = true
	pool.Active = false
	definition := &core.TokenPoolDefinition{Pool: pool}

	sender := ds.getSenderDefault(ctx, definition, core.SystemTagDefinePool, nil)
	if sender.message != nil {
		pool.Message = sender.message.Header.ID
	}

	pool.Name = localName
	pool.Namespace = ds.namespace
	pool.Active = true
	return sender
}

func (ds *definitionSender) DefineTokenPool(ctx context.Context, pool *core.TokenPool, waitConfirm bool) error {
	if pool.Published {
		if !ds.multiparty {
			return i18n.NewError(ctx, coremsgs.MsgActionNotSupported)
		}
		_, err := ds.getTokenPoolSender(ctx, pool).send(ctx, waitConfirm)
		return err
	}

	pool.NetworkName = ""

	return fakeBatch(ctx, func(ctx context.Context, state *core.BatchState) (HandlerResult, error) {
		hr, err := ds.handler.handleTokenPoolDefinition(ctx, state, pool, true)
		if err != nil {
			if innerErr := errors.Unwrap(err); innerErr != nil {
				return hr, innerErr
			}
		}
		return hr, err
	})
}
