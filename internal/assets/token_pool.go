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

package assets

import (
	"context"
	"fmt"

	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/internal/database/sqlcommon"
	"github.com/hyperledger-firefly/firefly/internal/txcommon"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

func (am *assetManager) CreateTokenPool(ctx context.Context, pool *core.TokenPoolInput, waitConfirm bool) (*core.TokenPool, error) {
	if err := fftypes.ValidateFFNameFieldNoUUID(ctx, pool.Name, "name"); err != nil {
		return nil, err
	}
	if existing, err := am.database.GetTokenPool(ctx, am.namespace, pool.Name); err != nil {
		return nil, err
	} else if existing != nil {
		return nil, i18n.NewError(ctx, coremsgs.MsgTokenPoolDuplicate, pool.Name)
	}
	pool.ID = fftypes.NewUUID()
	pool.Namespace = am.namespace

	if pool.Connector == "" {
		connector, err := am.getDefaultTokenConnector(ctx)
		if err != nil {
			return nil, err
		}
		pool.Connector = connector
	}

	if pool.Interface != nil {
		if err := am.contracts.ResolveFFIReference(ctx, pool.Interface); err != nil {
			return nil, err
		}
	}

	var err error
	pool.Key, err = am.identity.ResolveInputSigningKey(ctx, pool.Key, am.keyNormalization)
	if err != nil {
		return nil, err
	}
	return am.createTokenPoolInternal(ctx, pool, waitConfirm)
}

func (am *assetManager) createTokenPoolInternal(ctx context.Context, pool *core.TokenPoolInput, waitConfirm bool) (*core.TokenPool, error) {
	plugin, err := am.selectTokenPlugin(ctx, pool.Connector)
	if err != nil {
		return nil, err
	}

	if waitConfirm {
		return am.syncasync.WaitForTokenPool(ctx, pool.ID, func(ctx context.Context) error {
			_, err := am.createTokenPoolInternal(ctx, pool, false)
			return err
		})
	}

	var newOperation *core.Operation
	var resubmitted []*core.Operation
	var resubmitErr error
	err = am.database.RunAsGroup(ctx, func(ctx context.Context) (err error) {
		txid, err := am.txHelper.SubmitNewTransaction(ctx, core.TransactionTypeTokenPool, pool.IdempotencyKey)
		if err != nil {
			// Check if we've clashed on idempotency key. There might be operations still in "Initialized" state that need
			// submitting to their handlers.
			resubmitWholeTX := false
			if idemErr, ok := err.(*sqlcommon.IdempotencyError); ok {
				var total int
				total, resubmitted, resubmitErr = am.operations.ResubmitOperations(ctx, idemErr.ExistingTXID)
				if resubmitErr != nil {
					// Error doing resubmit, return the new error
					return resubmitErr
				}
				if total == 0 {
					// We didn't do anything last time - just start again
					txid = idemErr.ExistingTXID
					resubmitWholeTX = true
					err = nil
				} else if len(resubmitted) > 0 {
					pool.TX.ID = idemErr.ExistingTXID
					pool.TX.Type = core.TransactionTypeTokenPool
					err = nil
				}
			}
			if !resubmitWholeTX {
				return err
			}
		}

		pool.TX.ID = txid
		pool.TX.Type = core.TransactionTypeTokenPool

		newOperation = core.NewOperation(
			plugin,
			am.namespace,
			txid,
			core.OpTypeTokenCreatePool)
		if err = txcommon.AddTokenPoolCreateInputs(newOperation, &pool.TokenPool); err == nil {
			err = am.operations.AddOrReuseOperation(ctx, newOperation)
		}
		return err
	})
	if len(resubmitted) > 0 {
		// We resubmitted a previously initialized operation, don't run a new one
		return &pool.TokenPool, nil
	}
	if err != nil {
		// Any other error? Return the error unchanged
		return nil, err
	}

	_, err = am.operations.RunOperation(ctx, opCreatePool(newOperation, &pool.TokenPool), pool.IdempotencyKey != "")
	return &pool.TokenPool, err
}

func (am *assetManager) ActivateTokenPool(ctx context.Context, pool *core.TokenPool) error {
	plugin, err := am.selectTokenPlugin(ctx, pool.Connector)
	if err != nil {
		return err
	}

	var op *core.Operation
	err = am.database.RunAsGroup(ctx, func(ctx context.Context) (err error) {
		if existing, err := am.txHelper.FindOperationInTransaction(ctx, pool.TX.ID, core.OpTypeTokenActivatePool); err != nil {
			return err
		} else if existing != nil {
			log.L(ctx).Debugf("Dropping duplicate token pool activation request for pool %s", pool.ID)
			return nil
		}

		op = core.NewOperation(
			plugin,
			am.namespace,
			pool.TX.ID,
			core.OpTypeTokenActivatePool)
		txcommon.AddTokenPoolActivateInputs(op, pool.ID)
		return am.operations.AddOrReuseOperation(ctx, op)
	})
	if err != nil || op == nil {
		return err
	}

	_, err = am.operations.RunOperation(ctx, opActivatePool(op, pool),
		false, // TODO: this operation should be made idempotent, but cannot inherit this from the TX per our normal semantics
		//              as the transaction is only on the submitting side and this is triggered on all parties.
	)
	return err
}

func (am *assetManager) GetTokenPools(ctx context.Context, filter ffapi.AndFilter) ([]*core.TokenPool, *ffapi.FilterResult, error) {
	return am.database.GetTokenPools(ctx, am.namespace, filter)
}

func (am *assetManager) GetTokenPoolByLocator(ctx context.Context, connector, poolLocator string) (*core.TokenPool, error) {
	cacheKey := fmt.Sprintf("ns=%s,connector=%s,poollocator=%s", am.namespace, connector, poolLocator)
	if cachedValue := am.cache.Get(cacheKey); cachedValue != nil {
		log.L(ctx).Debugf("Token pool cache hit: %s", cacheKey)
		return cachedValue.(*core.TokenPool), nil
	}
	log.L(ctx).Debugf("Token pool cache miss: %s", cacheKey)
	if _, err := am.selectTokenPlugin(ctx, connector); err != nil {
		return nil, err
	}

	fb := database.TokenPoolQueryFactory.NewFilter(ctx)
	results, _, err := am.database.GetTokenPools(ctx, am.namespace, fb.And(
		fb.Eq("connector", connector),
		fb.Eq("locator", poolLocator),
	))
	if err != nil || len(results) == 0 {
		return nil, err
	}

	// Cache the result
	am.cache.Set(cacheKey, results[0])
	return results[0], nil
}

func (am *assetManager) GetTokenPoolByNameOrID(ctx context.Context, poolNameOrID string) (*core.TokenPool, error) {
	var pool *core.TokenPool
	cacheKey := fmt.Sprintf("ns=%s,poolnameorid=%s", am.namespace, poolNameOrID)
	if cachedValue := am.cache.Get(cacheKey); cachedValue != nil {
		log.L(ctx).Debugf("Token pool cache hit: %s", cacheKey)
		return cachedValue.(*core.TokenPool), nil
	}
	log.L(ctx).Debugf("Token pool cache miss: %s", cacheKey)

	poolID, err := fftypes.ParseUUID(ctx, poolNameOrID)
	if err != nil {
		if err := fftypes.ValidateFFNameField(ctx, poolNameOrID, "name"); err != nil {
			return nil, err
		}
		if pool, err = am.database.GetTokenPool(ctx, am.namespace, poolNameOrID); err != nil {
			return nil, err
		}
	} else if pool, err = am.GetTokenPoolByID(ctx, poolID); err != nil {
		return nil, err
	}
	if pool == nil {
		return nil, i18n.NewError(ctx, coremsgs.Msg404NotFound)
	}
	// Cache the result
	am.cache.Set(cacheKey, pool)
	return pool, nil
}

func (am *assetManager) removeTokenPoolFromCache(ctx context.Context, pool *core.TokenPool) {
	cacheKeyName := fmt.Sprintf("ns=%s,poolnameorid=%s", am.namespace, pool.Name)
	cacheKeyID := fmt.Sprintf("ns=%s,poolnameorid=%s", am.namespace, pool.ID)
	cacheKeyLocator := fmt.Sprintf("ns=%s,connector=%s,poollocator=%s", am.namespace, pool.Connector, pool.Locator)
	am.cache.Delete(cacheKeyName)
	am.cache.Delete(cacheKeyID)
	am.cache.Delete(cacheKeyLocator)
}

func (am *assetManager) GetTokenPoolByID(ctx context.Context, poolID *fftypes.UUID) (*core.TokenPool, error) {
	return am.database.GetTokenPoolByID(ctx, am.namespace, poolID)
}

func (am *assetManager) ResolvePoolMethods(ctx context.Context, pool *core.TokenPool) error {
	plugin, err := am.selectTokenPlugin(ctx, pool.Connector)
	if err == nil && pool.Interface != nil && pool.Interface.ID != nil && am.contracts != nil {
		var methods []*fftypes.FFIMethod
		methods, err = am.contracts.GetFFIMethods(ctx, pool.Interface.ID)
		if err == nil {
			pool.Methods, err = plugin.CheckInterface(ctx, pool, methods)
		}
	}
	return err
}

func (am *assetManager) DeleteTokenPool(ctx context.Context, poolNameOrID string) error {
	return am.database.RunAsGroup(ctx, func(ctx context.Context) error {
		pool, err := am.GetTokenPoolByNameOrID(ctx, poolNameOrID)
		if err != nil {
			return err
		}
		if pool.Published {
			return i18n.NewError(ctx, coremsgs.MsgCannotDeletePublished)
		}
		plugin, err := am.selectTokenPlugin(ctx, pool.Connector)
		if err != nil {
			return err
		}
		am.removeTokenPoolFromCache(ctx, pool)
		if err = am.database.DeleteTokenPool(ctx, am.namespace, pool.ID); err != nil {
			return err
		}
		if err = am.database.DeleteTokenTransfers(ctx, am.namespace, pool.ID); err != nil {
			return err
		}
		if err = am.database.DeleteTokenApprovals(ctx, am.namespace, pool.ID); err != nil {
			return err
		}
		if err = am.database.DeleteTokenBalances(ctx, am.namespace, pool.ID); err != nil {
			return err
		}
		return plugin.DeactivateTokenPool(ctx, pool)
	})
}
