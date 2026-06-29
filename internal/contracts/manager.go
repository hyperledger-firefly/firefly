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

package contracts

import (
	"context"
	"crypto/sha256"
	"database/sql/driver"
	"encoding/hex"
	"fmt"
	"hash"
	"sort"
	"strings"

	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/batch"
	"github.com/hyperledger-firefly/firefly/internal/broadcast"
	"github.com/hyperledger-firefly/firefly/internal/cache"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/internal/data"
	"github.com/hyperledger-firefly/firefly/internal/database/sqlcommon"
	"github.com/hyperledger-firefly/firefly/internal/identity"
	"github.com/hyperledger-firefly/firefly/internal/operations"
	"github.com/hyperledger-firefly/firefly/internal/privatemessaging"
	"github.com/hyperledger-firefly/firefly/internal/syncasync"
	"github.com/hyperledger-firefly/firefly/internal/txcommon"
	"github.com/hyperledger-firefly/firefly/internal/txwriter"
	"github.com/hyperledger-firefly/firefly/pkg/blockchain"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/santhosh-tekuri/jsonschema/v5"
)

type Manager interface {
	core.Named

	GetFFI(ctx context.Context, name, version string) (*fftypes.FFI, error)
	GetFFIWithChildren(ctx context.Context, name, version string) (*fftypes.FFI, error)
	GetFFIByID(ctx context.Context, id *fftypes.UUID) (*fftypes.FFI, error)
	GetFFIByIDWithChildren(ctx context.Context, id *fftypes.UUID) (*fftypes.FFI, error)
	GetFFIMethods(ctx context.Context, id *fftypes.UUID) ([]*fftypes.FFIMethod, error)
	GetFFIEvents(ctx context.Context, id *fftypes.UUID) ([]*fftypes.FFIEvent, error)
	GetFFIs(ctx context.Context, filter ffapi.AndFilter) ([]*fftypes.FFI, *ffapi.FilterResult, error)
	ResolveFFI(ctx context.Context, ffi *fftypes.FFI) error
	ResolveFFIReference(ctx context.Context, ref *fftypes.FFIReference) error
	DeleteFFI(ctx context.Context, id *fftypes.UUID) error

	DeployContract(ctx context.Context, req *core.ContractDeployRequest, waitConfirm bool) (interface{}, error)
	InvokeContract(ctx context.Context, req *core.ContractCallRequest, waitConfirm bool) (interface{}, error)
	InvokeContractAPI(ctx context.Context, apiName, methodPath string, req *core.ContractCallRequest, waitConfirm bool) (interface{}, error)
	GetContractAPI(ctx context.Context, httpServerURL, apiName string) (*core.ContractAPI, error)
	GetContractAPIInterface(ctx context.Context, apiName string) (*fftypes.FFI, error)
	GetContractAPIs(ctx context.Context, httpServerURL string, filter ffapi.AndFilter) ([]*core.ContractAPI, *ffapi.FilterResult, error)
	ResolveContractAPI(ctx context.Context, httpServerURL string, api *core.ContractAPI) error
	DeleteContractAPI(ctx context.Context, apiName string) error

	ConstructContractListenerSignature(ctx context.Context, listener *core.ContractListenerInput) (output *core.ContractListenerSignatureOutput, err error)
	AddContractListener(ctx context.Context, listener *core.ContractListenerInput) (output *core.ContractListener, err error)
	AddContractAPIListener(ctx context.Context, apiName, eventPath string, listener *core.ContractListener) (output *core.ContractListener, err error)
	GetContractListenerByNameOrID(ctx context.Context, nameOrID string) (*core.ContractListener, error)
	GetContractListenerByNameOrIDWithStatus(ctx context.Context, nameOrID string) (*core.ContractListenerWithStatus, error)
	GetContractListeners(ctx context.Context, filter ffapi.AndFilter) ([]*core.ContractListener, *ffapi.FilterResult, error)
	GetContractAPIListeners(ctx context.Context, apiName, eventPath string, filter ffapi.AndFilter) ([]*core.ContractListener, *ffapi.FilterResult, error)
	DeleteContractListenerByNameOrID(ctx context.Context, nameOrID string) error
	GenerateFFI(ctx context.Context, generationRequest *fftypes.FFIGenerationRequest) (*fftypes.FFI, error)

	// From operations.OperationHandler
	PrepareOperation(ctx context.Context, op *core.Operation) (*core.PreparedOperation, error)
	RunOperation(ctx context.Context, op *core.PreparedOperation) (outputs fftypes.JSONObject, phase core.OpPhase, err error)
}

type contractManager struct {
	namespace         string
	database          database.Plugin
	data              data.Manager
	broadcast         broadcast.Manager        // optional
	messaging         privatemessaging.Manager // optional
	batch             batch.Manager            // optional
	txHelper          txcommon.Helper
	txWriter          txwriter.Writer
	identity          identity.Manager
	blockchain        blockchain.Plugin
	ffiParamValidator fftypes.FFIParamValidator
	operations        operations.Manager
	syncasync         syncasync.Bridge
	methodCache       cache.CInterface
}

type methodCacheEntry struct {
	method *fftypes.FFIMethod
	errors []*fftypes.FFIError
}

type schemaValidationEntry struct {
	schema *jsonschema.Schema
}

func NewContractManager(ctx context.Context, ns string, di database.Plugin, bi blockchain.Plugin, dm data.Manager, bm broadcast.Manager, pm privatemessaging.Manager, bp batch.Manager, im identity.Manager, om operations.Manager, txHelper txcommon.Helper, txWriter txwriter.Writer, sa syncasync.Bridge, cacheManager cache.Manager) (Manager, error) {
	if di == nil || im == nil || bi == nil || dm == nil || om == nil || txHelper == nil || txWriter == nil || sa == nil || cacheManager == nil {
		return nil, i18n.NewError(ctx, coremsgs.MsgInitializationNilDepError, "ContractManager")
	}
	v, err := bi.GetFFIParamValidator(ctx)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, coremsgs.MsgPluginInitializationFailed)
	}

	cm := &contractManager{
		namespace:         ns,
		database:          di,
		data:              dm,
		broadcast:         bm,
		messaging:         pm,
		batch:             bp,
		txHelper:          txHelper,
		txWriter:          txWriter,
		identity:          im,
		blockchain:        bi,
		ffiParamValidator: v,
		operations:        om,
		syncasync:         sa,
	}

	cm.methodCache, err = cacheManager.GetCache(
		cache.NewCacheConfig(
			ctx,
			coreconfig.CacheMethodsLimit,
			coreconfig.CacheMethodsTTL,
			ns,
		),
	)
	if err != nil {
		return nil, err
	}

	om.RegisterHandler(ctx, cm, []core.OpType{
		core.OpTypeBlockchainInvoke,
		core.OpTypeBlockchainContractDeploy,
	})

	// Validate all our listeners exist on startup - consistent with the multi-party manager.
	// This means if EVMConnect is cleared, simply a restart of the namespace on core will
	// cause recreation of all the listeners (noting that listeners that were specified to start
	// from latest, will start from the new latest rather than replaying from the block they
	// started from before they were deleted).
	err = cm.verifyListeners(ctx)
	if err != nil {
		return nil, err
	}

	return cm, nil
}

func (cm *contractManager) Name() string {
	return "ContractManager"
}

func (cm *contractManager) newFFISchemaCompiler() *jsonschema.Compiler {
	c := fftypes.NewFFISchemaCompiler()
	if cm.ffiParamValidator != nil {
		c.RegisterExtension(cm.ffiParamValidator.GetExtensionName(), cm.ffiParamValidator.GetMetaSchema(), cm.ffiParamValidator)
	}
	return c
}

func (cm *contractManager) GetFFI(ctx context.Context, name, version string) (*fftypes.FFI, error) {
	ffi, err := cm.database.GetFFI(ctx, cm.namespace, name, version)
	if err != nil {
		return nil, err
	} else if ffi == nil {
		return nil, i18n.NewError(ctx, coremsgs.Msg404NotFound)
	}
	return ffi, nil
}

func (cm *contractManager) GetFFIWithChildren(ctx context.Context, name, version string) (*fftypes.FFI, error) {
	ffi, err := cm.GetFFI(ctx, name, version)
	if err == nil {
		err = cm.getFFIChildren(ctx, ffi)
	}
	return ffi, err
}

func (cm *contractManager) GetFFIByID(ctx context.Context, id *fftypes.UUID) (*fftypes.FFI, error) {
	return cm.database.GetFFIByID(ctx, cm.namespace, id)
}

func (cm *contractManager) GetFFIMethods(ctx context.Context, id *fftypes.UUID) ([]*fftypes.FFIMethod, error) {
	fb := database.FFIMethodQueryFactory.NewFilter(ctx)
	methods, _, err := cm.database.GetFFIMethods(ctx, cm.namespace, fb.Eq("interface", id))
	return methods, err
}

func (cm *contractManager) GetFFIEvents(ctx context.Context, id *fftypes.UUID) ([]*fftypes.FFIEvent, error) {
	fb := database.FFIMethodQueryFactory.NewFilter(ctx)
	events, _, err := cm.database.GetFFIEvents(ctx, cm.namespace, fb.Eq("interface", id))
	if err == nil {
		for _, event := range events {
			event.Signature, err = cm.blockchain.GenerateEventSignature(ctx, &event.FFIEventDefinition)
			if err != nil {
				return nil, err
			}
		}
	}
	return events, err
}

func (cm *contractManager) getFFIChildren(ctx context.Context, ffi *fftypes.FFI) (err error) {
	ffi.Methods, err = cm.GetFFIMethods(ctx, ffi.ID)
	if err != nil {
		return err
	}

	ffi.Events, err = cm.GetFFIEvents(ctx, ffi.ID)
	if err != nil {
		return err
	}

	fb := database.FFIErrorQueryFactory.NewFilter(ctx)
	ffi.Errors, _, err = cm.database.GetFFIErrors(ctx, cm.namespace, fb.Eq("interface", ffi.ID))
	if err != nil {
		return err
	}

	for _, errorDef := range ffi.Errors {
		errorDef.Signature = cm.blockchain.GenerateErrorSignature(ctx, &errorDef.FFIErrorDefinition)
	}
	return nil
}

func (cm *contractManager) GetFFIByIDWithChildren(ctx context.Context, id *fftypes.UUID) (ffi *fftypes.FFI, err error) {
	err = cm.database.RunAsGroup(ctx, func(ctx context.Context) (err error) {
		ffi, err = cm.database.GetFFIByID(ctx, cm.namespace, id)
		if err != nil || ffi == nil {
			return err
		}
		return cm.getFFIChildren(ctx, ffi)
	})
	return ffi, err
}

func (cm *contractManager) GetFFIs(ctx context.Context, filter ffapi.AndFilter) ([]*fftypes.FFI, *ffapi.FilterResult, error) {
	return cm.database.GetFFIs(ctx, cm.namespace, filter)
}

func (cm *contractManager) verifyListeners(ctx context.Context) error {

	var page uint64
	var pageSize uint64 = 50
	verifyCount := 0
	for {
		f := database.ContractListenerQueryFactory.NewFilterLimit(ctx, pageSize).And().Skip(page * pageSize)
		listeners, _, err := cm.database.GetContractListeners(ctx, cm.namespace, f)
		if err != nil {
			return err
		}
		if len(listeners) == 0 {
			log.L(ctx).Infof("Listener restore complete. Verified=%d", verifyCount)
			return nil
		}

		// Migrate and check if listener exists in blockchain plugin
		migratedListeners := []*core.ContractListener{}
		for _, l := range listeners {
			migrated, l, err := cm.MigrateToFiltersIfNeeded(ctx, l)
			if err != nil {
				return err
			}
			if migrated {
				migratedListeners = append(migratedListeners, l)
			}

			if err := cm.checkContractListenerExists(ctx, l); err != nil {
				return err
			}
			verifyCount++
		}

		// Write back the migrations
		if len(migratedListeners) > 0 {
			err := cm.database.RunAsGroup(ctx, func(ctx context.Context) (err error) {
				for _, listener := range migratedListeners {
					err := cm.database.UpsertContractListener(ctx, listener, true)
					if err != nil {
						return err
					}
				}

				return nil
			})
			if err != nil {
				return err
			}
		}

		page++
	}

}

func (cm *contractManager) writeInvokeTransaction(ctx context.Context, req *core.ContractCallRequest) (bool, *core.Operation, error) {
	txtype := core.TransactionTypeContractInvoke
	if req.Message != nil {
		txtype = core.TransactionTypeContractInvokePin
	}
	op := core.NewOperation(
		cm.blockchain,
		cm.namespace,
		nil, // assigned by txwriter
		core.OpTypeBlockchainInvoke)
	if err := addBlockchainReqInputs(op, req); err != nil {
		return false, nil, err
	}

	txn, err := cm.txWriter.WriteTransactionAndOps(ctx, txtype, req.IdempotencyKey, op)
	if err != nil {
		// Check if we've clashed on idempotency key. There might be operations still in "Initialized" state that need
		// submitting to their handlers
		if idemErr, ok := err.(*sqlcommon.IdempotencyError); ok {
			// Note we don't need to worry about re-entering this code zero-ops in this case, as we write everything as a batch in WriteTransactionAndOps.
			_, resubmitted, resubmitErr := cm.operations.ResubmitOperations(ctx, idemErr.ExistingTXID)

			if resubmitErr != nil {
				// Error doing resubmit, return the new error
				err = resubmitErr
			} else if len(resubmitted) > 0 {
				// We successfully resubmitted an initialized operation, return the operation
				// and the idempotent error. The caller will revert the 409 to 2xx
				if req.Message != nil {
					req.Message.Header.TxType = txtype
					req.Message.TransactionID = idemErr.ExistingTXID
				}
				return true, resubmitted[0], nil // only one operation, return existing one
			}
		}
		return false, op, err
	}
	if req.Message != nil {
		req.Message.Header.TxType = txtype
		req.Message.TransactionID = txn.ID
	}

	return false, op, err
}

func (cm *contractManager) writeDeployTransaction(ctx context.Context, req *core.ContractDeployRequest) (bool, *core.Operation, error) {

	op := core.NewOperation(
		cm.blockchain,
		cm.namespace,
		nil, // assigned by txwriter
		core.OpTypeBlockchainContractDeploy)
	if err := addBlockchainReqInputs(op, req); err != nil {
		return false, nil, err
	}
	_, err := cm.txWriter.WriteTransactionAndOps(ctx, core.TransactionTypeContractDeploy, req.IdempotencyKey, op)
	if err != nil {
		// Check if we've clashed on idempotency key. There might be operations still in "Initialized" state that need
		// submitting to their handlers
		if idemErr, ok := err.(*sqlcommon.IdempotencyError); ok {
			// Note we don't need to worry about re-entering this code zero-ops in this case, as we write everything as a batch in WriteTransactionAndOps.
			_, resubmitted, resubmitErr := cm.operations.ResubmitOperations(ctx, idemErr.ExistingTXID)

			if resubmitErr != nil {
				// Error doing resubmit, return the new error
				err = resubmitErr
			} else if len(resubmitted) > 0 {
				// We successfully resubmitted an initialized operation, return the operation
				// and the idempotent error. The caller will revert the 409 to 2xx
				return true, resubmitted[0], nil // only one operation, return existing one
			}
		}
		return false, op, err
	}

	return false, op, err
}

func (cm *contractManager) DeployContract(ctx context.Context, req *core.ContractDeployRequest, waitConfirm bool) (res interface{}, err error) {
	req.Key, err = cm.identity.ResolveInputSigningKey(ctx, req.Key, identity.KeyNormalizationBlockchainPlugin)
	if err != nil {
		return nil, err
	}

	resubmit, op, err := cm.writeDeployTransaction(ctx, req)
	if err != nil {
		return nil, err
	}
	if resubmit {
		return op, nil // nothing more to do
	}

	send := func(ctx context.Context) error {
		_, err := cm.operations.RunOperation(ctx, opBlockchainContractDeploy(op, req), req.IdempotencyKey != "")
		return err
	}
	if waitConfirm {
		return cm.syncasync.WaitForDeployOperation(ctx, op.ID, send)
	}
	err = send(ctx)
	return op, err
}

func (cm *contractManager) InvokeContract(ctx context.Context, req *core.ContractCallRequest, waitConfirm bool) (res interface{}, err error) {
	keyResolver := cm.identity.ResolveInputSigningKey
	if req.Type == core.CallTypeQuery {
		// Special case that we are resolving the key with an intent to query, not sign
		keyResolver = cm.identity.ResolveQuerySigningKey
	}
	req.Key, err = keyResolver(ctx, req.Key, identity.KeyNormalizationBlockchainPlugin)
	if err != nil {
		return nil, err
	}

	var msgSender syncasync.Sender
	if req.Message != nil {
		if msgSender, err = cm.buildInvokeMessage(ctx, req.Message); err != nil {
			return nil, err
		}
	}

	if err := cm.resolveInvokeContractRequest(ctx, req); err != nil {
		return nil, err
	}
	bcParsedMethod, err := cm.validateInvokeContractRequest(ctx, req, true)
	if err != nil {
		return nil, err
	}
	if msgSender != nil {
		if err := msgSender.Prepare(ctx); err != nil {
			return nil, err
		}
		// Clear inline data now that it's been resolved
		// (so as not to store all data values on the operation inputs)
		req.Message.InlineData = nil
	}
	var op *core.Operation
	var resubmit bool
	if req.Type == core.CallTypeInvoke {
		resubmit, op, err = cm.writeInvokeTransaction(ctx, req)
		if err != nil {
			return nil, err
		}
		if resubmit {
			return op, nil
		}
	}

	switch req.Type {
	case core.CallTypeInvoke:
		if msgSender != nil {
			if waitConfirm {
				return op, msgSender.SendAndWait(ctx)
			}
			return op, msgSender.Send(ctx)
		}
		send := func(ctx context.Context) error {
			_, err := cm.operations.RunOperation(ctx, txcommon.OpBlockchainInvoke(op, req, nil), req.IdempotencyKey != "")
			return err
		}
		if waitConfirm {
			return cm.syncasync.WaitForInvokeOperation(ctx, op.ID, send)
		}
		return op, send(ctx)

	case core.CallTypeQuery:
		return cm.blockchain.QueryContract(ctx, req.Key, req.Location, bcParsedMethod, req.Input, req.Options)

	default:
		panic(fmt.Sprintf("unknown call type: %s", req.Type))
	}
}

func (cm *contractManager) InvokeContractAPI(ctx context.Context, apiName, methodPath string, req *core.ContractCallRequest, waitConfirm bool) (interface{}, error) {
	api, err := cm.database.GetContractAPIByName(ctx, cm.namespace, apiName)
	if err != nil {
		return nil, err
	} else if api == nil || api.Interface == nil {
		return nil, i18n.NewError(ctx, coremsgs.Msg404NotFound)
	}
	req.Interface = api.Interface.ID
	req.MethodPath = methodPath
	if api.Location != nil {
		req.Location = api.Location
	}
	return cm.InvokeContract(ctx, req, waitConfirm)
}

func (cm *contractManager) resolveInvokeContractRequest(ctx context.Context, req *core.ContractCallRequest) (err error) {
	if req.Method == nil {
		if req.MethodPath == "" || req.Interface == nil {
			return i18n.NewError(ctx, coremsgs.MsgContractMethodNotSet)
		}
		cacheKey := fmt.Sprintf("method_%s_%s", req.Interface, req.MethodPath)
		cached := cm.methodCache.Get(cacheKey)
		if cached != nil {
			cMethodDetails := cached.(*methodCacheEntry)
			req.Method = cMethodDetails.method
			req.Errors = cMethodDetails.errors
			return nil
		}
		req.Method, err = cm.database.GetFFIMethod(ctx, cm.namespace, req.Interface, req.MethodPath)
		if err != nil || req.Method == nil {
			return i18n.NewError(ctx, coremsgs.MsgContractMethodResolveError, req.MethodPath, err)
		}
		fb := database.FFIErrorQueryFactory.NewFilter(ctx)
		req.Errors, _, err = cm.database.GetFFIErrors(ctx, cm.namespace, fb.Eq("interface", req.Interface))
		if err != nil {
			return i18n.NewError(ctx, coremsgs.MsgContractErrorsResolveError, err)
		}
		cm.methodCache.Set(cacheKey, &methodCacheEntry{
			method: req.Method,
			errors: req.Errors,
		})
	}
	return nil
}

func (cm *contractManager) addContractURLs(httpServerURL string, api *core.ContractAPI) {
	if api != nil {
		// These URLs must match the actual routes in apiserver.createMuxRouter()!
		// Note the httpServerURL includes the namespace
		baseURL := fmt.Sprintf("%s/apis/%s", httpServerURL, api.Name)
		api.URLs.OpenAPI = baseURL + "/api/swagger.json"
		api.URLs.UI = baseURL + "/api"
		api.URLs.API = baseURL
	}
}

func (cm *contractManager) GetContractAPI(ctx context.Context, httpServerURL, apiName string) (*core.ContractAPI, error) {
	api, err := cm.database.GetContractAPIByName(ctx, cm.namespace, apiName)
	cm.addContractURLs(httpServerURL, api)
	return api, err
}

func (cm *contractManager) GetContractAPIInterface(ctx context.Context, apiName string) (*fftypes.FFI, error) {
	api, err := cm.GetContractAPI(ctx, "", apiName)
	if err != nil || api == nil {
		return nil, err
	}
	return cm.GetFFIByIDWithChildren(ctx, api.Interface.ID)
}

func (cm *contractManager) GetContractAPIs(ctx context.Context, httpServerURL string, filter ffapi.AndFilter) ([]*core.ContractAPI, *ffapi.FilterResult, error) {
	apis, fr, err := cm.database.GetContractAPIs(ctx, cm.namespace, filter)
	for _, api := range apis {
		cm.addContractURLs(httpServerURL, api)
	}
	return apis, fr, err
}

func (cm *contractManager) ResolveContractAPI(ctx context.Context, httpServerURL string, api *core.ContractAPI) (err error) {
	if err := api.Validate(ctx); err != nil {
		return err
	}

	if api.Location != nil {
		if api.Location, err = cm.blockchain.NormalizeContractLocation(ctx, blockchain.NormalizeCall, api.Location); err != nil {
			return err
		}
	}

	err = cm.database.RunAsGroup(ctx, func(ctx context.Context) (err error) {
		existing, err := cm.database.GetContractAPIByName(ctx, api.Namespace, api.Name)
		if existing != nil && err == nil {
			if !api.LocationAndLedgerEquals(existing) {
				return i18n.NewError(ctx, coremsgs.MsgContractLocationExists)
			}
		}

		if err := cm.ResolveFFIReference(ctx, api.Interface); err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return err
	}

	if httpServerURL != "" {
		cm.addContractURLs(httpServerURL, api)
	}
	return nil
}

func (cm *contractManager) ResolveFFIReference(ctx context.Context, ref *fftypes.FFIReference) error {
	switch {
	case ref == nil:
		return i18n.NewError(ctx, coremsgs.MsgContractInterfaceNotFound, "")

	case ref.ID != nil:
		ffi, err := cm.database.GetFFIByID(ctx, cm.namespace, ref.ID)
		if err != nil {
			return err
		} else if ffi == nil {
			return i18n.NewError(ctx, coremsgs.MsgContractInterfaceNotFound, ref.ID)
		}
		return nil

	case ref.Name != "" && ref.Version != "":
		ffi, err := cm.database.GetFFI(ctx, cm.namespace, ref.Name, ref.Version)
		if err != nil {
			return err
		} else if ffi == nil {
			return i18n.NewError(ctx, coremsgs.MsgContractInterfaceNotFound, ref.Name)
		}
		ref.ID = ffi.ID
		return nil

	default:
		return i18n.NewError(ctx, coremsgs.MsgContractInterfaceNotFound, ref.Name)
	}
}

func (cm *contractManager) uniquePathName(name string, usedNames map[string]bool) string {
	pathName := name
	for counter := 1; ; counter++ {
		if _, existing := usedNames[pathName]; existing {
			pathName = fmt.Sprintf("%s_%d", name, counter)
		} else {
			usedNames[pathName] = true
			return pathName
		}
	}
}

func (cm *contractManager) ResolveFFI(ctx context.Context, ffi *fftypes.FFI) error {
	if err := ffi.Validate(ctx); err != nil {
		return err
	}

	methodPathNames := map[string]bool{}
	for _, method := range ffi.Methods {
		method.Interface = ffi.ID
		method.Namespace = ffi.Namespace
		method.Pathname = cm.uniquePathName(method.Name, methodPathNames)
		if _, _, err := cm.validateFFIMethod(ctx, method); err != nil {
			return err
		}
	}

	eventPathNames := map[string]bool{}
	for _, event := range ffi.Events {
		event.Interface = ffi.ID
		event.Namespace = ffi.Namespace
		event.Pathname = cm.uniquePathName(event.Name, eventPathNames)
		if err := cm.validateFFIEvent(ctx, &event.FFIEventDefinition); err != nil {
			return err
		}
	}

	errorPathNames := map[string]bool{}
	for _, errorDef := range ffi.Errors {
		errorDef.Interface = ffi.ID
		errorDef.Namespace = ffi.Namespace
		errorDef.Pathname = cm.uniquePathName(errorDef.Name, errorPathNames)
		if _, err := cm.validateFFIError(ctx, &errorDef.FFIErrorDefinition); err != nil {
			return err
		}
	}
	return nil
}

func (cm *contractManager) validateFFIMethod(ctx context.Context, method *fftypes.FFIMethod) (hash.Hash, map[string]*jsonschema.Schema, error) {
	if method.Name == "" {
		return nil, nil, i18n.NewError(ctx, coremsgs.MsgMethodNameMustBeSet)
	}
	paramSchemas := make(map[string]*jsonschema.Schema)
	paramUniqueHash := sha256.New()
	for _, param := range method.Params {
		paramSchemaHash, schema, err := cm.validateFFIParam(ctx, param)
		if err != nil {
			return nil, nil, err
		}
		paramSchemas[param.Name] = schema
		// The input parsing is dependent on the parameter name, so it's important those are included in the hash
		paramUniqueHash.Write([]byte(param.Name))
		paramUniqueHash.Write([]byte(paramSchemaHash))
	}
	for _, param := range method.Returns {
		returnHash, _, err := cm.validateFFIParam(ctx, param)
		if err != nil {
			return nil, nil, err
		}
		paramUniqueHash.Write([]byte(param.Name))
		paramUniqueHash.Write([]byte(returnHash))
	}
	return paramUniqueHash, paramSchemas, nil
}

func (cm *contractManager) validateFFIParam(ctx context.Context, param *fftypes.FFIParam) (string, *jsonschema.Schema, error) {
	schemaString := param.Schema.String()
	schemaHash := hex.EncodeToString(fftypes.HashString(schemaString)[:])
	cacheKey := fmt.Sprintf("schema_%s", schemaHash)
	cached := cm.methodCache.Get(cacheKey)
	if cached != nil {
		// Cached validation result
		return schemaHash, cached.(*schemaValidationEntry).schema, nil
	}
	c := cm.newFFISchemaCompiler()
	if err := c.AddResource(param.Name, strings.NewReader(schemaString)); err != nil {
		return "", nil, i18n.WrapError(ctx, err, coremsgs.MsgFFISchemaParseFail, param.Name)
	}
	schema, err := c.Compile(param.Name)
	if err != nil || schema == nil {
		return "", nil, i18n.WrapError(ctx, err, coremsgs.MsgFFISchemaCompileFail, param.Name)
	}
	cm.methodCache.Set(cacheKey, &schemaValidationEntry{schema: schema})
	return schemaHash, schema, nil
}

func (cm *contractManager) validateFFIEvent(ctx context.Context, event *fftypes.FFIEventDefinition) error {
	if event.Name == "" {
		return i18n.NewError(ctx, coremsgs.MsgEventNameMustBeSet)
	}
	for _, param := range event.Params {
		if _, _, err := cm.validateFFIParam(ctx, param); err != nil {
			return err
		}
	}
	return nil
}

func (cm *contractManager) validateFFIError(ctx context.Context, errorDef *fftypes.FFIErrorDefinition) (string, error) {
	if errorDef.Name == "" {
		return "", i18n.NewError(ctx, coremsgs.MsgErrorNameMustBeSet)
	}
	cacheKeyBuff := new(strings.Builder) // Build a big string of aggregate hashes
	for _, param := range errorDef.Params {
		paramCacheKey, _, err := cm.validateFFIParam(ctx, param)
		if err != nil {
			return "", err
		}
		cacheKeyBuff.WriteString(param.Name)
		cacheKeyBuff.WriteString(paramCacheKey)
	}
	return cacheKeyBuff.String(), nil
}

func (cm *contractManager) validateInvokeContractRequest(ctx context.Context, req *core.ContractCallRequest, blockchainValidation bool) (interface{}, error) {
	paramUniqueHash, paramSchemas, err := cm.validateFFIMethod(ctx, req.Method)
	if err != nil {
		return nil, err
	}
	for _, errDef := range req.Errors {
		errorCacheKey, err := cm.validateFFIError(ctx, &errDef.FFIErrorDefinition)
		if err != nil {
			return nil, err
		}
		paramUniqueHash.Write([]byte(errorCacheKey))
	}

	// Validate that all parameters are specified and are of reasonable JSON types to match the FFI
	lastIndex := len(req.Method.Params)
	if req.Message != nil {
		// If a message is included, skip validation of the last parameter
		// (assume it will be used for sending the batch pin)
		lastIndex--
		if lastIndex < 0 {
			return nil, i18n.NewError(ctx, coremsgs.MsgMethodDoesNotSupportPinning)
		}

		// Also verify that the user didn't pass in a value for this last parameter
		lastParam := req.Method.Params[lastIndex]
		if _, ok := req.Input[lastParam.Name]; ok {
			return nil, i18n.NewError(ctx, coremsgs.MsgCannotSetParameterWithMessage, lastParam.Name)
		}
	}

	for _, param := range req.Method.Params[:lastIndex] {
		schema, schemaOk := paramSchemas[param.Name]
		value, valueOk := req.Input[param.Name]
		if !valueOk || !schemaOk {
			return nil, i18n.NewError(ctx, coremsgs.MsgContractMissingInputArgument, param.Name)
		}
		if err := cm.checkParamSchema(ctx, param.Name, value, schema); err != nil {
			return nil, err
		}
	}

	// Now we need to ask the blockchain connector to do its own validation of the FFI.
	// This is cached by the aggregate cache key we just built
	cacheKey := "methodhash_" + req.Method.Name + "_" + hex.EncodeToString(paramUniqueHash.Sum(nil))
	bcParsedMethod := cm.methodCache.Get(cacheKey)
	cacheMiss := bcParsedMethod == nil
	if cacheMiss {
		bcParsedMethod, err = cm.blockchain.ParseInterface(ctx, req.Method, req.Errors)
		if err != nil {
			return nil, err
		}
		cm.methodCache.Set(cacheKey, bcParsedMethod)
	}
	log.L(ctx).Debugf("Validating method '%s' (cacheMiss=%t)", cacheKey, cacheMiss)

	if blockchainValidation {
		// Allow the blockchain plugin to perform additional blockchain-specific parameter validation.
		// We only do this on API on the way in, not when this function is called later as part of the operation.
		return bcParsedMethod, cm.blockchain.ValidateInvokeRequest(ctx, bcParsedMethod, req.Input, req.Message != nil)
	}
	return bcParsedMethod, nil
}

func (cm *contractManager) resolveEvent(ctx context.Context, ffi *fftypes.FFIReference, eventPath string) (*core.FFISerializedEvent, error) {
	if err := cm.ResolveFFIReference(ctx, ffi); err != nil {
		return nil, err
	}
	event, err := cm.database.GetFFIEvent(ctx, cm.namespace, ffi.ID, eventPath)
	if err != nil {
		return nil, err
	} else if event == nil {
		return nil, i18n.NewError(ctx, coremsgs.MsgEventNotFound, eventPath)
	}
	return &core.FFISerializedEvent{FFIEventDefinition: event.FFIEventDefinition}, nil
}

func (cm *contractManager) checkContractListenerExists(ctx context.Context, listener *core.ContractListener) error {
	found, _, _, err := cm.blockchain.GetContractListenerStatus(ctx, listener.Namespace, listener.BackendID, true)
	if err != nil {
		log.L(ctx).Errorf("Validating listener %s:%s (BackendID=%s) failed: %s", listener.Signature, listener.ID, listener.BackendID, err)
		return err
	}
	if found {
		log.L(ctx).Debugf("Validated listener %s:%s (BackendID=%s)", listener.Signature, listener.ID, listener.BackendID)
		return nil
	}

	// For the case that we're establishing a listener from "latest" we obtain the protocol ID
	// of the latest event confirmed from the blockchain for a given subscription.
	// This protocolID should be parsed and used by the blockchain plugin if SubOptsFirstEventNewest
	// is passed through, and the listener does not exist.
	fb := database.BlockchainEventQueryFactory.NewFilter(ctx).Sort("-protocolid").Limit(1)
	latestEvents, _, err := cm.database.GetBlockchainEvents(ctx, cm.namespace, fb.Eq(
		"listener", listener.ID,
	))
	if err != nil {
		return err
	}
	lastProtocolID := ""
	if len(latestEvents) > 0 {
		lastProtocolID = latestEvents[0].ProtocolID
	}

	if err = cm.blockchain.AddContractListener(ctx, listener, lastProtocolID); err != nil {
		return err
	}
	return cm.database.UpdateContractListener(ctx, cm.namespace, listener.ID,
		database.ContractListenerQueryFactory.NewUpdate(ctx).Set("backendid", listener.BackendID))
}

func (cm *contractManager) parseContractListenerFilters(ctx context.Context, listener *core.ContractListenerInput) (err error) {
	// Handle deprecated root event
	if len(listener.Filters) == 0 {
		// Copy the deprecated interface into the first element in the filters array
		listener.Filters = append(listener.Filters, &core.ListenerFilterInput{
			ListenerFilter: core.ListenerFilter{
				Event:     listener.Event,
				Location:  listener.Location,
				Interface: listener.Interface,
			},
			EventPath: listener.EventPath,
		})
	}

	// This map check the whole signature + location
	duplicateSignatureChecker := map[string]bool{}
	// This will check the event signature + overlapping locations
	duplicateEventSignatureChecker := map[string]*fftypes.JSONAny{}

	for _, filter := range listener.Filters {
		if filter.Event == nil {
			if filter.EventPath == "" || filter.Interface == nil {
				return i18n.NewError(ctx, coremsgs.MsgListenerNoEvent)
			}
			// Copy the event definition into the filter
			if filter.Event, err = cm.resolveEvent(ctx, filter.Interface, filter.EventPath); err != nil {
				return err
			}
		} else {
			filter.Interface = nil
		}

		if err := cm.validateFFIEvent(ctx, &filter.Event.FFIEventDefinition); err != nil {
			return err
		}

		if filter.Location != nil {
			if filter.Location, err = cm.blockchain.NormalizeContractLocation(ctx, blockchain.NormalizeListener, filter.Location); err != nil {
				return err
			}
		}

		filter.Signature, err = cm.blockchain.GenerateEventSignatureWithLocation(ctx, &filter.Event.FFIEventDefinition, filter.Location)
		if err != nil {
			return err
		}

		// Check if we have parsed a filter with the same signature including location
		if duplicateSignatureChecker[filter.Signature] {
			return i18n.NewError(ctx, coremsgs.MsgDuplicateContractListenerFilterLocation)
		}

		eventSignature, err := cm.blockchain.GenerateEventSignature(ctx, &filter.Event.FFIEventDefinition)
		if err != nil {
			return err
		}

		// Check if we have parsed a filter with the same signature but not location
		if location, ok := duplicateEventSignatureChecker[eventSignature]; ok {
			// If this duplicate filter is looking at all locations it's a superset of the previous one
			// or the previous filter was also looking at all locations then we are trying to add a subset
			if filter.Location == nil || location == nil {
				return i18n.NewError(ctx, coremsgs.MsgDuplicateContractListenerFilterLocation)
			}

			// Have to call the specific blockchain plugin to compare locations
			isOverLapping, err := cm.blockchain.CheckOverlappingLocations(ctx, location, filter.Location)
			if err != nil {
				return err
			}

			if isOverLapping {
				return i18n.NewError(ctx, coremsgs.MsgDuplicateContractListenerFilterLocation)
			}
		}

		listener.ContractListener.Filters = append(listener.ContractListener.Filters, &core.ListenerFilter{
			Event:     filter.Event,
			Location:  filter.Location,
			Interface: filter.Interface,
			Signature: filter.Signature,
		})

		duplicateSignatureChecker[filter.Signature] = true
		duplicateEventSignatureChecker[eventSignature] = filter.Location
	}

	// Sort to keep consistent and generate the same order of signatures
	sort.Slice(listener.ContractListener.Filters, func(i, j int) bool {
		return listener.ContractListener.Filters[i].Signature < listener.ContractListener.Filters[j].Signature
	})

	// Don't need to initialize it.
	var sb strings.Builder
	for i, filter := range listener.ContractListener.Filters {
		sb.WriteString(filter.Signature)
		if i+1 < len(listener.Filters) {
			// Separator between filter signatures if more after this one
			sb.WriteString(";")
		}
	}

	listener.Signature = sb.String()

	// To preserve compatibility copy the first element in the
	// filters arary to the top if not present already
	if listener.Event == nil && len(listener.Filters) > 0 {
		listener.Event = listener.Filters[0].Event
		listener.Location = listener.Filters[0].Location
		listener.Interface = listener.Filters[0].Interface
	}

	return nil
}

func (cm *contractManager) ConstructContractListenerSignature(ctx context.Context, listener *core.ContractListenerInput) (output *core.ContractListenerSignatureOutput, err error) {
	output = &core.ContractListenerSignatureOutput{}

	err = cm.parseContractListenerFilters(ctx, listener)
	if err != nil {
		return nil, err
	}

	output.Signature = listener.Signature

	return output, nil
}

func (cm *contractManager) verifyContractListener(ctx context.Context, listener *core.ContractListenerInput) (output *core.ContractListener, err error) {
	listener.ID = fftypes.NewUUID()
	listener.Namespace = cm.namespace

	if listener.Name != "" {
		if err := fftypes.ValidateFFNameField(ctx, listener.Name, "name"); err != nil {
			return nil, err
		}
	}
	if err := fftypes.ValidateFFNameField(ctx, listener.Topic, "topic"); err != nil {
		return nil, err
	}

	// Check that both the new filters and deprecated fields are not specified
	if len(listener.Filters) > 0 && (listener.Event != nil || listener.EventPath != "") {
		return nil, i18n.NewError(ctx, coremsgs.MsgFiltersAndRootEventError, cm.namespace, listener.Name)
	}

	// This location only applies to the root event and will be ignore as part of filters
	if listener.Location != nil {
		if listener.Location, err = cm.blockchain.NormalizeContractLocation(ctx, blockchain.NormalizeListener, listener.Location); err != nil {
			return nil, err
		}
	}

	if listener.Options == nil {
		listener.Options = cm.getDefaultContractListenerOptions()
	} else if listener.Options.FirstEvent == "" {
		listener.Options.FirstEvent = cm.getDefaultContractListenerOptions().FirstEvent
	}

	_, err = cm.ConstructContractListenerSignature(ctx, listener)
	if err != nil {
		return nil, err
	}

	err = cm.database.RunAsGroup(ctx, func(ctx context.Context) (err error) {
		// Namespace + Name must be unique
		if listener.Name != "" {
			if existing, err := cm.database.GetContractListener(ctx, cm.namespace, listener.Name); err != nil {
				return err
			} else if existing != nil {
				return i18n.NewError(ctx, coremsgs.MsgContractListenerNameExists, cm.namespace, listener.Name)
			}
		}

		// Above we only call NormalizeContractLocation if the listener is non-nil, and that means
		// for an unset location we will have a nil value. Using an fftypes.JSONAny in a query
		// of nil does not yield the right result, so we need to do an explicit nil query.
		var locationLookup driver.Value = nil
		if !listener.Location.IsNil() {
			locationLookup = listener.Location.String()
		}
		if len(listener.Filters) == 1 && listener.Filters[0].Location != nil {
			// For backwards compatibility with existing contract listeners with one filter
			// We need to set the location to not find clashes
			listener.Location = listener.Filters[0].Location
			locationLookup = listener.Filters[0].Location.String()
		}
		fb := database.ContractListenerQueryFactory.NewFilter(ctx)
		if existing, _, err := cm.database.GetContractListeners(ctx, cm.namespace, fb.And(
			fb.Eq("topic", listener.Topic),
			fb.Eq("location", locationLookup),
			fb.Eq("signature", listener.Signature),
		)); err != nil {
			return err
		} else if len(existing) > 0 {
			return i18n.NewError(ctx, coremsgs.MsgContractListenerExists)
		}

		// Check for existense of an older listener with the old signature
		// Only valid for one filter
		if listener.Event != nil && len(listener.Filters) == 1 {
			// Note the event signature has been extended with more information in some blockchain plugins
			// That is why we do not add the signature in the query but instead iterate over the listeners
			// and compare the signatures
			signature, err := cm.blockchain.GenerateEventSignature(ctx, &listener.Event.FFIEventDefinition)
			if err != nil {
				return err
			}
			filter := database.ContractListenerQueryFactory.NewFilter(ctx)
			if existing, _, err := cm.database.GetContractListeners(ctx, cm.namespace, filter.And(
				filter.Eq("topic", listener.Topic),
				filter.Eq("location", locationLookup),
			)); err != nil {
				return err
			} else if len(existing) > 0 {
				for _, listener := range existing {
					// We have extended the event signature to add more information
					// So we compare the start with is not guaranteed to be the same
					// but it's the best comparison
					if strings.HasPrefix(signature, listener.Signature) {
						return i18n.NewError(ctx, coremsgs.MsgContractListenerExists)
					}

				}
			}
		}

		return nil
	})
	if err != nil {
		return nil, err
	}

	return &listener.ContractListener, nil
}

func (cm *contractManager) AddContractListener(ctx context.Context, listener *core.ContractListenerInput) (output *core.ContractListener, err error) {
	verifiedContractListener, err := cm.verifyContractListener(ctx, listener)
	if err != nil {
		return nil, err
	}

	if err = cm.blockchain.AddContractListener(ctx, &listener.ContractListener, ""); err != nil {
		return nil, err
	}
	if listener.Name == "" {
		listener.Name = listener.BackendID
	}
	if err = cm.database.InsertContractListener(ctx, verifiedContractListener); err != nil {
		return nil, err
	}

	return verifiedContractListener, err
}

func (cm *contractManager) AddContractAPIListener(ctx context.Context, apiName, eventPath string, listener *core.ContractListener) (output *core.ContractListener, err error) {
	api, err := cm.database.GetContractAPIByName(ctx, cm.namespace, apiName)
	if err != nil {
		return nil, err
	} else if api == nil || api.Interface == nil {
		return nil, i18n.NewError(ctx, coremsgs.Msg404NotFound)
	}

	input := &core.ContractListenerInput{ContractListener: *listener}
	input.Interface = &fftypes.FFIReference{ID: api.Interface.ID}
	input.EventPath = eventPath
	if api.Location != nil {
		input.Location = api.Location
	}

	return cm.AddContractListener(ctx, input)
}

func (cm *contractManager) MigrateToFiltersIfNeeded(ctx context.Context, listener *core.ContractListener) (bool, *core.ContractListener, error) {
	migrated := false
	if len(listener.Filters) == 0 && listener.Event != nil {
		// Blockchain plugin has changed the signature
		newSignature, err := cm.blockchain.GenerateEventSignature(ctx, &listener.Event.FFIEventDefinition)
		if err != nil {
			// This is safe to do because all listeners previously inserted
			// verified that the event was valid before creating the signature
			return false, nil, err
		}
		listener.Filters = append(listener.Filters, &core.ListenerFilter{
			Event:     listener.Event,
			Location:  listener.Location,
			Interface: listener.Interface,
			Signature: newSignature,
		})
		// Note not migrating the root signature as that would not allow rolling back
		migrated = true
	}

	return migrated, listener, nil
}

func (cm *contractManager) GetContractListenerByNameOrID(ctx context.Context, nameOrID string) (listener *core.ContractListener, err error) {
	id, err := fftypes.ParseUUID(ctx, nameOrID)
	if err != nil {
		if err := fftypes.ValidateFFNameField(ctx, nameOrID, "name"); err != nil {
			return nil, err
		}
		if listener, err = cm.database.GetContractListener(ctx, cm.namespace, nameOrID); err != nil {
			return nil, err
		}
	} else if listener, err = cm.database.GetContractListenerByID(ctx, cm.namespace, id); err != nil {
		return nil, err
	}
	if listener == nil {
		return nil, i18n.NewError(ctx, coremsgs.Msg404NotFound)
	}

	return listener, nil
}

func (cm *contractManager) GetContractListenerByNameOrIDWithStatus(ctx context.Context, nameOrID string) (enrichedListener *core.ContractListenerWithStatus, err error) {
	listener, err := cm.GetContractListenerByNameOrID(ctx, nameOrID)
	if err != nil {
		return nil, err
	}
	_, status, _, err := cm.blockchain.GetContractListenerStatus(ctx, listener.Namespace, listener.BackendID, false)
	if err != nil {
		status = core.ListenerStatusError{
			StatusError: err.Error(),
		}
	}
	enrichedListener = &core.ContractListenerWithStatus{
		ContractListener: *listener,
		Status:           status,
	}
	return enrichedListener, nil
}

func (cm *contractManager) GetContractListeners(ctx context.Context, filter ffapi.AndFilter) ([]*core.ContractListener, *ffapi.FilterResult, error) {
	return cm.database.GetContractListeners(ctx, cm.namespace, filter)
}

func (cm *contractManager) GetContractAPIListeners(ctx context.Context, apiName, eventPath string, filter ffapi.AndFilter) ([]*core.ContractListener, *ffapi.FilterResult, error) {
	api, err := cm.database.GetContractAPIByName(ctx, cm.namespace, apiName)
	if err != nil {
		return nil, nil, err
	} else if api == nil || api.Interface == nil {
		return nil, nil, i18n.NewError(ctx, coremsgs.Msg404NotFound)
	}
	event, err := cm.resolveEvent(ctx, api.Interface, eventPath)
	if err != nil {
		return nil, nil, err
	}
	signature, err := cm.blockchain.GenerateEventSignatureWithLocation(ctx, &event.FFIEventDefinition, api.Location)
	if err != nil {
		return nil, nil, err
	}

	oldSignature, err := cm.blockchain.GenerateEventSignature(ctx, &event.FFIEventDefinition)
	if err != nil {
		return nil, nil, err
	}

	fb := database.ContractListenerQueryFactory.NewFilter(ctx)
	f := fb.And(
		fb.Eq("interface", api.Interface.ID),
		fb.Or(fb.Contains("signature", signature), fb.Eq("signature", oldSignature)),
		filter,
	)
	return cm.database.GetContractListeners(ctx, cm.namespace, f)
}

func (cm *contractManager) DeleteContractListenerByNameOrID(ctx context.Context, nameOrID string) error {
	return cm.database.RunAsGroup(ctx, func(ctx context.Context) (err error) {
		listener, err := cm.GetContractListenerByNameOrID(ctx, nameOrID)
		if err != nil {
			return err
		}
		if err = cm.blockchain.DeleteContractListener(ctx, listener, true /* ok if not found */); err != nil {
			return err
		}
		return cm.database.DeleteContractListenerByID(ctx, cm.namespace, listener.ID)
	})
}

func (cm *contractManager) checkParamSchema(ctx context.Context, name string, input interface{}, schema *jsonschema.Schema) error {
	if err := schema.Validate(input); err != nil {
		return i18n.WrapError(ctx, err, coremsgs.MsgFFIValidationFail, name)
	}
	return nil
}

func (cm *contractManager) GenerateFFI(ctx context.Context, generationRequest *fftypes.FFIGenerationRequest) (*fftypes.FFI, error) {
	generationRequest.Namespace = cm.namespace
	if generationRequest.Name == "" {
		generationRequest.Name = "generated"
	}
	if generationRequest.Version == "" {
		generationRequest.Version = "0.0.1"
	}
	ffi, err := cm.blockchain.GenerateFFI(ctx, generationRequest)
	if err == nil {
		err = cm.ResolveFFI(ctx, ffi)
	}
	return ffi, err
}

func (cm *contractManager) getDefaultContractListenerOptions() *core.ContractListenerOptions {
	return &core.ContractListenerOptions{
		FirstEvent: string(core.SubOptsFirstEventNewest),
	}
}

func (cm *contractManager) buildInvokeMessage(ctx context.Context, in *core.MessageInOut) (syncasync.Sender, error) {
	allowedTypes := []fftypes.FFEnum{
		core.MessageTypeBroadcast,
		core.MessageTypePrivate,
	}
	if in.Header.Type == "" {
		in.Header.Type = core.MessageTypeBroadcast
	}
	switch in.Header.Type {
	case core.MessageTypeBroadcast:
		if cm.broadcast == nil {
			return nil, i18n.NewError(ctx, coremsgs.MsgMessagesNotSupported)
		}
		return cm.broadcast.NewBroadcast(in), nil
	case core.MessageTypePrivate:
		if cm.messaging == nil {
			return nil, i18n.NewError(ctx, coremsgs.MsgMessagesNotSupported)
		}
		return cm.messaging.NewMessage(in), nil
	default:
		return nil, i18n.NewError(ctx, coremsgs.MsgInvalidMessageType, allowedTypes)
	}
}

func (cm *contractManager) DeleteFFI(ctx context.Context, id *fftypes.UUID) error {
	return cm.database.RunAsGroup(ctx, func(ctx context.Context) error {
		ffi, err := cm.GetFFIByID(ctx, id)
		if err != nil {
			return err
		}
		if ffi == nil {
			return i18n.NewError(ctx, coremsgs.Msg404NotFound)
		}
		if ffi.Published {
			return i18n.NewError(ctx, coremsgs.MsgCannotDeletePublished)
		}
		return cm.database.DeleteFFI(ctx, cm.namespace, id)
	})
}

func (cm *contractManager) DeleteContractAPI(ctx context.Context, apiName string) error {
	return cm.database.RunAsGroup(ctx, func(ctx context.Context) error {
		api, err := cm.GetContractAPI(ctx, "", apiName)
		if err != nil {
			return err
		}
		if api == nil {
			return i18n.NewError(ctx, coremsgs.Msg404NotFound)
		}
		if api.Published {
			return i18n.NewError(ctx, coremsgs.MsgCannotDeletePublished)
		}
		return cm.database.DeleteContractAPI(ctx, cm.namespace, api.ID)
	})
}
