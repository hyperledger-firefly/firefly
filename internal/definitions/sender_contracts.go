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

	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly/internal/coremsgs"
	"github.com/hyperledger/firefly/pkg/core"
)

func (ds *definitionSender) DefineFFI(ctx context.Context, ffi *fftypes.FFI, waitConfirm bool, topics []string) error {
	ffi.ID = fftypes.NewUUID()
	ffi.Namespace = ds.namespace
	for _, method := range ffi.Methods {
		method.ID = fftypes.NewUUID()
	}
	for _, event := range ffi.Events {
		event.ID = fftypes.NewUUID()
	}
	for _, errorDef := range ffi.Errors {
		errorDef.ID = fftypes.NewUUID()
	}

	if ffi.Published {
		if !ds.multiparty {
			return i18n.NewError(ctx, coremsgs.MsgActionNotSupported)
		}
		_, err := ds.getFFISender(ctx, ffi, topics).send(ctx, waitConfirm)
		return err
	}

	ffi.NetworkName = ""

	return fakeBatch(ctx, func(ctx context.Context, state *core.BatchState) (HandlerResult, error) {
		hr, err := ds.handler.handleFFIDefinition(ctx, state, ffi, nil, true)
		if err != nil {
			if innerErr := errors.Unwrap(err); innerErr != nil {
				return hr, innerErr
			}
		}
		return hr, err
	})
}

func (ds *definitionSender) getFFISender(ctx context.Context, ffi *fftypes.FFI, topics []string) *sendWrapper {
	if err := ds.contracts.ResolveFFI(ctx, ffi); err != nil {
		return wrapSendError(err)
	}

	if ffi.NetworkName == "" {
		ffi.NetworkName = ffi.Name
	}

	existing, err := ds.database.GetFFIByNetworkName(ctx, ds.namespace, ffi.NetworkName, ffi.Version)
	if err != nil {
		return wrapSendError(err)
	} else if existing != nil {
		return wrapSendError(i18n.NewError(ctx, coremsgs.MsgNetworkNameExists))
	}

	// Prepare the FFI definition to be serialized for broadcast
	localName := ffi.Name
	ffi.Name = ""
	ffi.Namespace = ""
	ffi.Published = true

	sender := ds.getSenderDefault(ctx, ffi, core.SystemTagDefineFFI, topics)
	if sender.message != nil {
		ffi.Message = sender.message.Header.ID
	}

	ffi.Name = localName
	ffi.Namespace = ds.namespace
	return sender
}

func (ds *definitionSender) PublishFFI(ctx context.Context, name, version, networkName string, waitConfirm bool, topics []string) (ffi *fftypes.FFI, err error) {
	if !ds.multiparty {
		return nil, i18n.NewError(ctx, coremsgs.MsgActionNotSupported)
	}

	var sender *sendWrapper
	err = ds.database.RunAsGroup(ctx, func(ctx context.Context) error {
		if ffi, err = ds.contracts.GetFFIWithChildren(ctx, name, version); err != nil {
			return err
		}
		if ffi.Published {
			return i18n.NewError(ctx, coremsgs.MsgAlreadyPublished)
		}
		ffi.NetworkName = networkName
		sender = ds.getFFISender(ctx, ffi, topics)
		if sender.err != nil {
			return sender.err
		}
		return sender.sender.Prepare(ctx)
	})
	if err != nil {
		return nil, err
	}

	_, err = sender.send(ctx, waitConfirm)
	return ffi, err
}

func (ds *definitionSender) DefineContractAPI(ctx context.Context, httpServerURL string, api *core.ContractAPI, waitConfirm bool, topics []string) error {
	if api.ID == nil {
		api.ID = fftypes.NewUUID()
	}
	api.Namespace = ds.namespace

	if api.Published {
		if !ds.multiparty {
			return i18n.NewError(ctx, coremsgs.MsgActionNotSupported)
		}
		_, err := ds.getContractAPISender(ctx, httpServerURL, api, topics).send(ctx, waitConfirm)
		return err
	}

	api.NetworkName = ""

	return fakeBatch(ctx, func(ctx context.Context, state *core.BatchState) (HandlerResult, error) {
		return ds.handler.handleContractAPIDefinition(ctx, state, httpServerURL, api, nil, true)
	})
}

func (ds *definitionSender) getContractAPISender(ctx context.Context, httpServerURL string, api *core.ContractAPI, topics []string) *sendWrapper {
	if err := ds.contracts.ResolveContractAPI(ctx, httpServerURL, api); err != nil {
		return wrapSendError(err)
	}

	if api.NetworkName == "" {
		api.NetworkName = api.Name
	}

	existing, err := ds.database.GetContractAPIByNetworkName(ctx, ds.namespace, api.NetworkName)
	if err != nil {
		return wrapSendError(err)
	} else if existing != nil {
		return wrapSendError(i18n.NewError(ctx, coremsgs.MsgNetworkNameExists))
	}
	if api.Interface != nil && api.Interface.ID != nil {
		iface, err := ds.database.GetFFIByID(ctx, ds.namespace, api.Interface.ID)
		switch {
		case err != nil:
			return wrapSendError(err)
		case iface == nil:
			return wrapSendError(i18n.NewError(ctx, coremsgs.MsgContractInterfaceNotFound, api.Interface.ID))
		case !iface.Published:
			return wrapSendError(i18n.NewError(ctx, coremsgs.MsgContractInterfaceNotPublished, api.Interface.ID))
		}
	}

	// Prepare the API definition to be serialized for broadcast
	localName := api.Name
	api.Name = ""
	api.Namespace = ""
	api.Published = true

	sender := ds.getSenderDefault(ctx, api, core.SystemTagDefineContractAPI, topics)
	if sender.message != nil {
		api.Message = sender.message.Header.ID
	}

	api.Name = localName
	api.Namespace = ds.namespace
	return sender
}

func (ds *definitionSender) PublishContractAPI(ctx context.Context, httpServerURL, name, networkName string, waitConfirm bool, topics []string) (api *core.ContractAPI, err error) {
	if !ds.multiparty {
		return nil, i18n.NewError(ctx, coremsgs.MsgActionNotSupported)
	}

	var sender *sendWrapper
	err = ds.database.RunAsGroup(ctx, func(ctx context.Context) error {
		if api, err = ds.contracts.GetContractAPI(ctx, httpServerURL, name); err != nil {
			return err
		}
		if api.Published {
			return i18n.NewError(ctx, coremsgs.MsgAlreadyPublished)
		}
		api.NetworkName = networkName
		sender = ds.getContractAPISender(ctx, httpServerURL, api, topics)
		if sender.err != nil {
			return sender.err
		}
		return sender.sender.Prepare(ctx)
	})
	if err != nil {
		return nil, err
	}

	_, err = sender.send(ctx, waitConfirm)
	return api, err
}
