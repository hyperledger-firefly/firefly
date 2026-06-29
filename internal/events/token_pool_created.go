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

package events

import (
	"context"
	"fmt"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/txcommon"
	"github.com/hyperledger-firefly/firefly/pkg/blockchain"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/hyperledger-firefly/firefly/pkg/tokens"
)

func addPoolDetailsFromPlugin(ffPool *core.TokenPool, pluginPool *tokens.TokenPool) error {
	ffPool.Type = pluginPool.Type
	ffPool.Locator = pluginPool.PoolLocator
	ffPool.PluginData = pluginPool.PluginData
	ffPool.Connector = pluginPool.Connector
	ffPool.Standard = pluginPool.Standard
	ffPool.InterfaceFormat = (core.TokenInterfaceFormat)(pluginPool.InterfaceFormat)
	ffPool.Decimals = pluginPool.Decimals
	if pluginPool.TX.ID != nil {
		ffPool.TX = pluginPool.TX
	}
	if pluginPool.Symbol != "" {
		if ffPool.Symbol == "" {
			ffPool.Symbol = pluginPool.Symbol
		} else if ffPool.Symbol != pluginPool.Symbol {
			return fmt.Errorf("token symbol '%s' from blockchain does not match stored symbol '%s'", pluginPool.Symbol, ffPool.Symbol)
		}
	}
	ffPool.Info = pluginPool.Info
	return nil
}

func (em *eventManager) confirmPool(ctx context.Context, pool *core.TokenPool, ev *blockchain.Event) error {
	log.L(ctx).Debugf("Confirming token pool ID='%s' Locator='%s'", pool.ID, pool.Locator)
	var blockchainID string
	if ev != nil {
		// Some pools will not include a blockchain event for creation (such as when indexing a pre-existing pool)
		blockchainID = ev.BlockchainTXID
		chainEvent := buildBlockchainEvent(pool.Namespace, nil, ev, &core.BlockchainTransactionRef{
			ID:           pool.TX.ID,
			Type:         pool.TX.Type,
			BlockchainID: blockchainID,
		})
		created, err := em.maybePersistBlockchainEvent(ctx, chainEvent, nil)
		if err != nil {
			return err
		}
		if created {
			em.emitBlockchainEventMetric(ev)
		}
	}
	if _, err := em.txHelper.PersistTransaction(ctx, pool.TX.ID, pool.TX.Type, blockchainID); err != nil {
		return err
	}
	pool.Active = true
	if err := em.database.UpsertTokenPool(ctx, pool, database.UpsertOptimizationExisting); err != nil {
		return err
	}
	log.L(ctx).Infof("Token pool confirmed, id=%s", pool.ID)
	event := core.NewEvent(core.EventTypePoolConfirmed, pool.Namespace, pool.ID, pool.TX.ID, pool.ID.String())
	return em.database.InsertEvent(ctx, event)
}

func (em *eventManager) getPoolByIDOrLocator(ctx context.Context, id *fftypes.UUID, connector, locator string) (*core.TokenPool, error) {
	if id != nil {
		return em.assets.GetTokenPoolByID(ctx, id)
	}
	return em.assets.GetTokenPoolByLocator(ctx, connector, locator)
}

func (em *eventManager) loadExisting(ctx context.Context, pool *tokens.TokenPool) (existingPool *core.TokenPool, err error) {
	if existingPool, err = em.getPoolByIDOrLocator(ctx, pool.ID, pool.Connector, pool.PoolLocator); err != nil {
		return nil, err
	}

	if existingPool == nil {
		for _, alternateLocator := range pool.AlternateLocators {
			if existingPool, err = em.getPoolByIDOrLocator(ctx, pool.ID, pool.Connector, alternateLocator); err != nil {
				return existingPool, err
			}
			if existingPool != nil {
				log.L(ctx).Debugf("Updating locator for existing pool ns=%s connector=%s oldLocator=%s newLocator=%s", em.namespace.Name, pool.Connector, existingPool.Locator, pool.PoolLocator)
				existingPool.Locator = pool.PoolLocator
				if err := em.database.UpsertTokenPool(ctx, existingPool, database.UpsertOptimizationExisting); err != nil {
					return existingPool, err
				}
				break
			}
		}
	}

	if existingPool == nil {
		log.L(ctx).Debugf("Pool not found with ns=%s connector=%s locator=%s (err=%v)", em.namespace.Name, pool.Connector, pool.PoolLocator, err)
		return existingPool, err
	}

	if err = addPoolDetailsFromPlugin(existingPool, pool); err != nil {
		log.L(ctx).Errorf("Error processing pool for transaction '%s' (%s) - ignoring", pool.TX.ID, err)
		return nil, nil
	}
	return existingPool, nil
}

func (em *eventManager) loadFromOperation(ctx context.Context, pool *tokens.TokenPool) (stagedPool *core.TokenPool, err error) {
	op, err := em.txHelper.FindOperationInTransaction(ctx, pool.TX.ID, core.OpTypeTokenCreatePool)
	if err != nil {
		return nil, err
	} else if op == nil {
		log.L(ctx).Debugf("No pool found for tx=%s status=%s", pool.TX.ID, core.OpTypeTokenCreatePool)
		return nil, nil
	}

	stagedPool, err = txcommon.RetrieveTokenPoolCreateInputs(ctx, op)
	if err != nil || stagedPool.ID == nil || stagedPool.Namespace == "" || stagedPool.Name == "" {
		log.L(ctx).Errorf("Error loading pool info for transaction '%s' (%s) - ignoring: %v", pool.TX.ID, err, op.Input)
		return nil, nil
	}

	if err = addPoolDetailsFromPlugin(stagedPool, pool); err != nil {
		log.L(ctx).Errorf("Error processing pool for transaction '%s' (%s) - ignoring", pool.TX.ID, err)
		return nil, nil
	}
	return stagedPool, nil
}

// It is expected that this method might be invoked twice for each pool, depending on the behavior of the connector.
// It will be at least invoked on the submitter when the pool is first created, to trigger the submitter to publish it.
// It will be invoked on every node (including the submitter) after the pool is published+activated, to trigger confirmation of the pool.
// When received in any other scenario, it should be ignored.
//
// The context passed to this callback is dependent on what phase it is called in.
// In the case that it is called synchronously on the submitter, in order to trigger the publish, the original context
// of the REST API will be propagated (so it can be used for the resolution of the org signing key).
func (em *eventManager) TokenPoolCreated(ctx context.Context, ti tokens.Plugin, pool *tokens.TokenPool) (err error) {
	var msgIDforRewind *fftypes.UUID
	var stagedPool *core.TokenPool

	err = em.retry.Do(ctx, "persist token pool transaction", func(attempt int) (bool, error) {
		err := em.database.RunAsGroup(ctx, func(ctx context.Context) error {
			// See if this is the result of activating an existing pool
			existingPool, err := em.loadExisting(ctx, pool)
			if err != nil {
				return err
			}
			if existingPool != nil {
				if existingPool.Active {
					log.L(ctx).Debugf("Token pool already active ID='%s' Locator='%s'", existingPool.ID, pool.PoolLocator)
					return nil // already active
				}
				msgIDforRewind = existingPool.Message
				return em.confirmPool(ctx, existingPool, pool.Event)
			}

			if pool.TX.ID == nil {
				// Transaction ID is required if the pool doesn't exist yet
				// (it can be omitted above when only activating)
				log.L(ctx).Errorf("Invalid token pool transaction - ID is nil")
				return nil // move on
			}

			// See if this pool was submitted locally and needs to be published
			if stagedPool, err = em.loadFromOperation(ctx, pool); err != nil {
				return err
			} else if stagedPool != nil {
				return nil // trigger publish after completion of database transaction
			}

			// Otherwise this event can be ignored
			var protoID string
			if pool.Event != nil {
				protoID = pool.Event.ProtocolID
			}
			log.L(ctx).Debugf("Ignoring token pool created notification. No matching pool definition found Event='%s' Locator='%s'", protoID, pool.PoolLocator)
			return nil
		})
		return err != nil, err
	})

	if err == nil {
		// Initiate a rewind if a batch was potentially completed by the arrival of this transaction
		if msgIDforRewind != nil {
			em.aggregator.queueMessageRewind(msgIDforRewind)
		}

		if stagedPool != nil {
			// If the pool is tied to a contract interface, resolve the methods to be used for later operations
			if stagedPool.Interface != nil && stagedPool.Interface.ID != nil && stagedPool.InterfaceFormat != "" {
				log.L(ctx).Infof("Querying token connector interface, id=%s", stagedPool.ID)
				if err := em.assets.ResolvePoolMethods(ctx, stagedPool); err != nil {
					return err
				}
			}

			// Publish the details of the new token pool
			// Other nodes will pass these details to their own token connector for validation/activation of the pool
			log.L(ctx).Infof("Defining token pool, id=%s", stagedPool.ID)
			err = em.defsender.DefineTokenPool(ctx, stagedPool, false)
		}
	}

	return err
}
