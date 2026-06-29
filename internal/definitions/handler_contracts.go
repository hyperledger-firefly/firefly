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
	"fmt"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

func (dh *definitionHandler) persistFFI(ctx context.Context, ffi *fftypes.FFI, isAuthor bool) (retry bool, err error) {
	for i := 1; ; i++ {
		if err = dh.contracts.ResolveFFI(ctx, ffi); err != nil {
			return false, i18n.WrapError(ctx, err, coremsgs.MsgDefRejectedValidateFail, "contract interface", ffi.ID)
		}

		// Check if this conflicts with an existing FFI
		existing, err := dh.database.InsertOrGetFFI(ctx, ffi)
		if err != nil {
			return true, err
		}

		if existing == nil {
			// No conflict - new FFI was inserted successfully
			break
		}

		if ffi.Published {
			if existing.ID.Equals(ffi.ID) {
				// ID conflict - check if this matches (or should overwrite) the existing record
				return dh.reconcilePublishedFFI(ctx, existing, ffi, isAuthor)
			}

			if existing.Name == ffi.Name && existing.Version == ffi.Version {
				// Local name conflict - generate a unique name and try again
				ffi.Name = fmt.Sprintf("%s-%d", ffi.NetworkName, i)
				continue
			}
		}

		// Any other conflict - reject
		return false, i18n.NewError(ctx, coremsgs.MsgDefRejectedConflict, "contract interface", ffi.ID, existing.ID)
	}

	for _, method := range ffi.Methods {
		if err := dh.database.UpsertFFIMethod(ctx, method); err != nil {
			return true, err
		}
	}
	for _, event := range ffi.Events {
		if err := dh.database.UpsertFFIEvent(ctx, event); err != nil {
			return true, err
		}
	}
	for _, errorDef := range ffi.Errors {
		if err := dh.database.UpsertFFIError(ctx, errorDef); err != nil {
			return true, err
		}
	}

	return false, nil
}

func (dh *definitionHandler) reconcilePublishedFFI(ctx context.Context, existing, ffi *fftypes.FFI, isAuthor bool) (retry bool, err error) {
	if existing.Message.Equals(ffi.Message) {
		// Message already recorded
		return false, nil
	}

	if existing.Message == nil && isAuthor {
		// FFI was previously unpublished - if it was now published by this node, upsert the new version
		ffi.Name = existing.Name
		if err := dh.database.UpsertFFI(ctx, ffi, database.UpsertOptimizationExisting); err != nil {
			return true, err
		}
		return false, nil
	}

	return false, i18n.NewError(ctx, coremsgs.MsgDefRejectedConflict, "contract interface", ffi.ID, existing.ID)
}

func (dh *definitionHandler) persistContractAPI(ctx context.Context, httpServerURL string, api *core.ContractAPI, isAuthor bool) (retry bool, err error) {
	l := log.L(ctx)
	for i := 1; ; i++ {
		if err := dh.contracts.ResolveContractAPI(ctx, httpServerURL, api); err != nil {
			return false, i18n.WrapError(ctx, err, coremsgs.MsgDefRejectedValidateFail, "contract API", api.ID)
		}

		// Check if this conflicts with an existing API
		existing, err := dh.database.InsertOrGetContractAPI(ctx, api)
		if err != nil {
			l.Errorf("Failed to InsertOrGetContractAPI due to err: %+v", err)
			return true, err
		}

		if existing == nil {
			// No conflict - new API was inserted successfully
			l.Tracef("Successfully inserted the new contract API with ID %s", api.ID)
			break
		}

		if existing.ID.Equals(api.ID) {
			// the matching record has the same ID, perform an update
			l.Trace("Found an existing contract API with the same ID, reconciling the contract API")
			// ID conflict - check if this matches (or should overwrite) the existing record
			return dh.reconcileContractAPI(ctx, existing, api, isAuthor)
		} else if api.Published {

			if existing.Name == api.Name {
				l.Trace("Local name conflict, generating a unique name to retry")
				// Local name conflict - generate a unique name and try again
				api.Name = fmt.Sprintf("%s-%d", api.NetworkName, i)
				continue
			}
		}

		// Any other conflict - reject
		return false, i18n.NewError(ctx, coremsgs.MsgDefRejectedConflict, "contract API", api.ID, existing.ID)

	}

	return false, nil
}

func (dh *definitionHandler) reconcileContractAPI(ctx context.Context, existing, api *core.ContractAPI, isAuthor bool) (retry bool, err error) {
	l := log.L(ctx)

	if api.Published {
		l.Trace("Reconciling a published API")
		if existing.Message.Equals(api.Message) {
			l.Trace("Reconciling a published API: message already recorded, no action required")
			// Message already recorded
			return false, nil
		}

		if existing.Message == nil && isAuthor {
			// API was previously unpublished - if it was now published by this node, upsert the new version
			api.Name = existing.Name
			l.Tracef("Reconciling a published API: update API name from '%s' to the existing name '%s'", api.Name, existing.Name)
			if err := dh.database.UpsertContractAPI(ctx, api, database.UpsertOptimizationExisting); err != nil {
				return true, err
			}
			return false, nil
		}
	} else {
		if err := dh.database.UpsertContractAPI(ctx, api, database.UpsertOptimizationExisting); err != nil {
			return true, err
		}
		return false, nil
	}

	return false, i18n.NewError(ctx, coremsgs.MsgDefRejectedConflict, "contract API", api.ID, existing.ID)
}

func (dh *definitionHandler) handleFFIBroadcast(ctx context.Context, state *core.BatchState, msg *core.Message, data core.DataArray, tx *fftypes.UUID) (HandlerResult, error) {
	var ffi fftypes.FFI
	if valid := dh.getSystemBroadcastPayload(ctx, msg, data, &ffi); !valid {
		return HandlerResult{Action: core.ActionReject}, i18n.NewError(ctx, coremsgs.MsgDefRejectedBadPayload, "contract interface", msg.Header.ID)
	}

	org, err := dh.identity.GetRootOrgDID(ctx)
	if err != nil {
		return HandlerResult{Action: core.ActionRetry}, err
	}
	isAuthor := org == msg.Header.Author

	ffi.Message = msg.Header.ID
	ffi.Name = ffi.NetworkName
	ffi.Published = true
	return dh.handleFFIDefinition(ctx, state, &ffi, tx, isAuthor)
}

func (dh *definitionHandler) handleFFIDefinition(ctx context.Context, state *core.BatchState, ffi *fftypes.FFI, tx *fftypes.UUID, isAuthor bool) (HandlerResult, error) {
	l := log.L(ctx)

	ffi.Namespace = dh.namespace.Name
	if retry, err := dh.persistFFI(ctx, ffi, isAuthor); err != nil {
		if retry {
			return HandlerResult{Action: core.ActionRetry}, err
		}
		return HandlerResult{Action: core.ActionReject}, err
	}

	l.Infof("Contract interface created id=%s", ffi.ID)
	state.AddFinalize(func(ctx context.Context) error {
		event := core.NewEvent(core.EventTypeContractInterfaceConfirmed, ffi.Namespace, ffi.ID, tx, ffi.Topic())
		return dh.database.InsertEvent(ctx, event)
	})
	return HandlerResult{Action: core.ActionConfirm}, nil
}

func (dh *definitionHandler) handleContractAPIBroadcast(ctx context.Context, state *core.BatchState, msg *core.Message, data core.DataArray, tx *fftypes.UUID) (HandlerResult, error) {
	var api core.ContractAPI
	if valid := dh.getSystemBroadcastPayload(ctx, msg, data, &api); !valid {
		return HandlerResult{Action: core.ActionReject}, i18n.NewError(ctx, coremsgs.MsgDefRejectedBadPayload, "contract API", msg.Header.ID)
	}

	org, err := dh.identity.GetRootOrgDID(ctx)
	if err != nil {
		return HandlerResult{Action: core.ActionRetry}, err
	}
	isAuthor := org == msg.Header.Author

	api.Message = msg.Header.ID
	api.Name = api.NetworkName
	api.Published = true
	return dh.handleContractAPIDefinition(ctx, state, "", &api, tx, isAuthor)
}

func (dh *definitionHandler) handleContractAPIDefinition(ctx context.Context, state *core.BatchState, httpServerURL string, api *core.ContractAPI, tx *fftypes.UUID, isAuthor bool) (HandlerResult, error) {
	l := log.L(ctx)

	api.Namespace = dh.namespace.Name
	if retry, err := dh.persistContractAPI(ctx, httpServerURL, api, isAuthor); err != nil {
		if retry {
			return HandlerResult{Action: core.ActionRetry}, err
		}
		return HandlerResult{Action: core.ActionReject}, err
	}

	l.Infof("Contract API created id=%s", api.ID)
	state.AddFinalize(func(ctx context.Context) error {
		event := core.NewEvent(core.EventTypeContractAPIConfirmed, api.Namespace, api.ID, tx, core.SystemTopicDefinitions)
		return dh.database.InsertEvent(ctx, event)
	})
	return HandlerResult{Action: core.ActionConfirm}, nil
}
