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

package txcommon

import (
	"context"
	"database/sql/driver"
	"strings"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/cache"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/internal/data"
	"github.com/hyperledger-firefly/firefly/internal/database/sqlcommon"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

type Helper interface {
	SubmitNewTransaction(ctx context.Context, txType core.TransactionType, idempotencyKey core.IdempotencyKey) (*fftypes.UUID, error)
	SubmitNewTransactionBatch(ctx context.Context, namespace string, batch []*BatchedTransactionInsert) error
	PersistTransaction(ctx context.Context, id *fftypes.UUID, txType core.TransactionType, blockchainTXID string) (valid bool, err error)
	AddBlockchainTX(ctx context.Context, tx *core.Transaction, blockchainTXID string) error
	InsertOrGetBlockchainEvent(ctx context.Context, event *core.BlockchainEvent) (existing *core.BlockchainEvent, err error)
	InsertNewBlockchainEvents(ctx context.Context, events []*core.BlockchainEvent) (inserted []*core.BlockchainEvent, err error)
	GetTransactionByIDCached(ctx context.Context, id *fftypes.UUID) (*core.Transaction, error)
	GetBlockchainEventByIDCached(ctx context.Context, id *fftypes.UUID) (*core.BlockchainEvent, error)
	FindOperationInTransaction(ctx context.Context, tx *fftypes.UUID, opType core.OpType) (*core.Operation, error)
}

type transactionHelper struct {
	namespace            string
	database             database.Plugin
	data                 data.Manager
	transactionCache     cache.CInterface
	blockchainEventCache cache.CInterface
}

type BatchedTransactionInsert struct {
	Input  TransactionInsertInput
	Output struct {
		IdempotencyError error
		Transaction      *core.Transaction
	}
}

type TransactionInsertInput struct {
	Type           core.TransactionType
	IdempotencyKey core.IdempotencyKey
}

func NewTransactionHelper(ctx context.Context, ns string, di database.Plugin, dm data.Manager, cacheManager cache.Manager) (Helper, error) {
	t := &transactionHelper{
		namespace: ns,
		database:  di,
		data:      dm,
	}

	transactionCache, err := cacheManager.GetCache(
		cache.NewCacheConfig(
			ctx,
			coreconfig.CacheTransactionSize,
			coreconfig.CacheTransactionTTL,
			ns,
		),
	)

	if err != nil {
		return nil, err
	}

	t.transactionCache = transactionCache

	blockchainEventCache, err := cacheManager.GetCache(
		cache.NewCacheConfig(
			ctx,
			coreconfig.CacheBlockchainEventLimit,
			coreconfig.CacheBlockchainEventTTL,
			ns,
		),
	)

	if err != nil {
		return nil, err
	}
	t.blockchainEventCache = blockchainEventCache
	return t, nil
}

func (t *transactionHelper) updateTransactionsCache(tx *core.Transaction) {
	t.transactionCache.Set(tx.ID.String(), tx)
}

func (t *transactionHelper) GetTransactionByIDCached(ctx context.Context, id *fftypes.UUID) (*core.Transaction, error) {

	if cachedValue := t.transactionCache.Get(id.String()); cachedValue != nil {
		return cachedValue.(*core.Transaction), nil
	}
	tx, err := t.database.GetTransactionByID(ctx, t.namespace, id)
	if err != nil || tx == nil {
		return tx, err
	}
	t.updateTransactionsCache(tx)
	return tx, nil
}

// SubmitNewTransaction is called when there is a new transaction being submitted by the local node
func (t *transactionHelper) SubmitNewTransaction(ctx context.Context, txType core.TransactionType, idempotencyKey core.IdempotencyKey) (*fftypes.UUID, error) {

	tx := &core.Transaction{
		ID:             fftypes.NewUUID(),
		Namespace:      t.namespace,
		Type:           txType,
		IdempotencyKey: idempotencyKey,
	}

	// Note that InsertTransaction is responsible for idempotency key duplicate detection and helpful error creation.
	// (In cases where one or more operations have not yet left 'initialized' state then we need to resubmit them even if
	// we've seen this idempotency key before.)
	if err := t.database.InsertTransaction(ctx, tx); err != nil {
		return nil, err
	}

	if err := t.database.InsertEvent(ctx, core.NewEvent(core.EventTypeTransactionSubmitted, tx.Namespace, tx.ID, tx.ID, tx.Type.String())); err != nil {
		return nil, err
	}

	t.updateTransactionsCache(tx)
	return tx.ID, nil
}

// SubmitNewTransactionBatch is called to do a batch insertion of a set of transactions, and returns an array of the transaction
// result. Each is either a transaction, or an idempotency failure. The overall action fails for DB errors other than idempotency.
func (t *transactionHelper) SubmitNewTransactionBatch(ctx context.Context, namespace string, batch []*BatchedTransactionInsert) error {

	// Sort our transactions into those with/without idempotency keys, and do a pre-check for duplicate
	// idempotency keys within the batch.
	plainTxInserts := make([]*core.Transaction, 0, len(batch))
	idempotentTxInserts := make([]*core.Transaction, 0, len(batch))
	idempotencyKeyMap := make(map[core.IdempotencyKey]*BatchedTransactionInsert)
	for _, t := range batch {
		t.Output.Transaction = &core.Transaction{
			ID:             fftypes.NewUUID(),
			Namespace:      namespace,
			Type:           t.Input.Type,
			IdempotencyKey: t.Input.IdempotencyKey,
		}
		if t.Input.IdempotencyKey == "" {
			plainTxInserts = append(plainTxInserts, t.Output.Transaction)
		} else {
			if existing := idempotencyKeyMap[t.Input.IdempotencyKey]; existing != nil {
				// We've got the same idempotency key twice in our batch. Fail the second one as a dup of the first
				log.L(ctx).Warnf("Idempotency key exists twice in insert batch '%s'", t.Input.IdempotencyKey)
				t.Output.IdempotencyError = &sqlcommon.IdempotencyError{
					ExistingTXID:  existing.Output.Transaction.ID,
					OriginalError: i18n.NewError(ctx, coremsgs.MsgIdempotencyKeyDuplicateTransaction, t.Input.IdempotencyKey, existing.Output.Transaction.ID),
				}
			} else {
				idempotencyKeyMap[t.Input.IdempotencyKey] = t
				idempotentTxInserts = append(idempotentTxInserts, t.Output.Transaction)
			}
		}
	}

	// First attempt to insert any transactions without an idempotency key. These should all work
	if len(plainTxInserts) > 0 {
		if insertErr := t.database.InsertTransactions(ctx, plainTxInserts); insertErr != nil {
			return insertErr
		}
	}

	// Then attempt to insert all the transactions with idempotency keys, which might result in
	// partial success.
	if len(idempotentTxInserts) > 0 {
		if insertErr := t.database.InsertTransactions(ctx, idempotentTxInserts); insertErr != nil {
			// We have either an error, or a mixed result. Do a query to find all the idempotencyKeys.
			// If we find them all, then we're good to continue, after we've used UUID comparison
			// to check which idempotency keys clashed.
			log.L(ctx).Warnf("Insert transaction batch failed. Checking for idempotencyKey duplicates: %s", insertErr)
			idempotencyKeys := make([]driver.Value, len(idempotentTxInserts))
			for i := 0; i < len(idempotentTxInserts); i++ {
				idempotencyKeys[i] = idempotentTxInserts[i].IdempotencyKey
			}
			fb := database.TransactionQueryFactory.NewFilter(ctx)
			resolvedTxns, _, queryErr := t.database.GetTransactions(ctx, namespace, fb.In("idempotencykey", idempotencyKeys))
			if queryErr != nil {
				log.L(ctx).Errorf("idempotencyKey duplicate check abandoned, due to query error (%s). Returning original insert err: %s", queryErr, insertErr)
				return insertErr
			}
			if len(resolvedTxns) != len(idempotencyKeys) {
				log.L(ctx).Errorf("idempotencyKey duplicate check abandoned, due to query not returning all transactions - len=%d, expected=%d. Returning original insert err: %s", len(resolvedTxns), len(idempotencyKeys), insertErr)
				return insertErr
			}
			for _, resolvedTxn := range resolvedTxns {
				// Processing above makes it safe for us to do this
				expectedEntry := idempotencyKeyMap[resolvedTxn.IdempotencyKey]
				if !resolvedTxn.ID.Equals(expectedEntry.Output.Transaction.ID) {
					log.L(ctx).Warnf("Idempotency key '%s' already existed in database for transaction %s", resolvedTxn.IdempotencyKey, resolvedTxn.ID)
					expectedEntry.Output.IdempotencyError = &sqlcommon.IdempotencyError{
						ExistingTXID:  resolvedTxn.ID,
						OriginalError: i18n.NewError(ctx, coremsgs.MsgIdempotencyKeyDuplicateTransaction, resolvedTxn.IdempotencyKey, resolvedTxn.ID),
					}
				}
			}
		}
	}

	// Insert events for all transactions that did not have an idempotency key failure
	// Note event insertion is already optimized within the database layer.
	for _, entry := range batch {
		if entry.Output.IdempotencyError == nil {
			tx := entry.Output.Transaction
			if err := t.database.InsertEvent(ctx, core.NewEvent(core.EventTypeTransactionSubmitted, tx.Namespace, tx.ID, tx.ID, tx.Type.String())); err != nil {
				return err
			}
			t.updateTransactionsCache(tx)
		}
	}

	// Ok - we're done
	return nil
}

// PersistTransaction is called when we need to ensure a transaction exists in the DB, and optionally associate a new BlockchainTXID to it
func (t *transactionHelper) PersistTransaction(ctx context.Context, id *fftypes.UUID, txType core.TransactionType, blockchainTXID string) (valid bool, err error) {

	tx, err := t.GetTransactionByIDCached(ctx, id)
	if err != nil {
		return false, err
	}

	if tx != nil {
		if tx.Type != txType {
			log.L(ctx).Errorf("Type mismatch for transaction '%s' existing=%s new=%s", tx.ID, tx.Type, txType)
			return false, nil
		}

		var changed bool
		tx.BlockchainIDs, changed = tx.BlockchainIDs.AddToSortedSet(blockchainTXID)
		if !changed {
			return true, nil
		}

		if err = t.database.UpdateTransaction(ctx, t.namespace, tx.ID, database.TransactionQueryFactory.NewUpdate(ctx).Set("blockchainids", tx.BlockchainIDs)); err != nil {
			return false, err
		}
	} else {
		tx = &core.Transaction{
			ID:            id,
			Namespace:     t.namespace,
			Type:          txType,
			BlockchainIDs: fftypes.NewFFStringArray(strings.ToLower(blockchainTXID)),
		}
		if err = t.database.InsertTransaction(ctx, tx); err != nil {
			return false, err
		}
	}

	t.updateTransactionsCache(tx)
	return true, nil
}

// AddBlockchainTX is called when we know the transaction should exist, and we don't need any validation
// but just want to bolt on an extra blockchain TXID (if it's not there already).
func (t *transactionHelper) AddBlockchainTX(ctx context.Context, tx *core.Transaction, blockchainTXID string) error {

	var changed bool
	tx.BlockchainIDs, changed = tx.BlockchainIDs.AddToSortedSet(blockchainTXID)
	if !changed {
		return nil
	}

	err := t.database.UpdateTransaction(ctx, t.namespace, tx.ID, database.TransactionQueryFactory.NewUpdate(ctx).Set("blockchainids", tx.BlockchainIDs))
	if err != nil {
		return err
	}

	t.updateTransactionsCache(tx)
	return nil
}

func (t *transactionHelper) addBlockchainEventToCache(chainEvent *core.BlockchainEvent) {
	t.blockchainEventCache.Set(chainEvent.ID.String(), chainEvent)
}

func (t *transactionHelper) GetBlockchainEventByIDCached(ctx context.Context, id *fftypes.UUID) (*core.BlockchainEvent, error) {

	if cachedValue := t.blockchainEventCache.Get(id.String()); cachedValue != nil {
		return cachedValue.(*core.BlockchainEvent), nil
	}
	chainEvent, err := t.database.GetBlockchainEventByID(ctx, t.namespace, id)
	if err != nil || chainEvent == nil {
		return chainEvent, err
	}
	t.addBlockchainEventToCache(chainEvent)
	return chainEvent, nil
}

func (t *transactionHelper) InsertOrGetBlockchainEvent(ctx context.Context, event *core.BlockchainEvent) (existing *core.BlockchainEvent, err error) {
	existing, err = t.database.InsertOrGetBlockchainEvent(ctx, event)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		t.addBlockchainEventToCache(existing)
		return existing, nil
	}
	t.addBlockchainEventToCache(event)
	return nil, nil
}

func (t *transactionHelper) InsertNewBlockchainEvents(ctx context.Context, events []*core.BlockchainEvent) (inserted []*core.BlockchainEvent, err error) {
	// First we try and insert the whole bundle using batch insert
	err = t.database.InsertBlockchainEvents(ctx, events, func() {
		for _, event := range events {
			t.addBlockchainEventToCache(event)
		}
	})
	if err == nil {
		// happy path worked - all new events
		return events, nil
	}

	// Fall back to insert-or-get
	log.L(ctx).Warnf("Blockchain event insert-many optimization failed: %s", err)
	inserted = make([]*core.BlockchainEvent, 0, len(events))
	for _, event := range events {
		existing, err := t.database.InsertOrGetBlockchainEvent(ctx, event)
		if err != nil {
			return nil, err
		}

		if existing != nil {
			// It's possible the batch insert was partially successful, and this is actually a "new" row.
			// Look to see if the corresponding entry also exists in the "events" table.
			fb := database.EventQueryFactory.NewFilter(ctx)
			notifications, _, err := t.database.GetEvents(ctx, t.namespace, fb.And(
				fb.Eq("type", core.EventTypeBlockchainEventReceived),
				fb.Eq("reference", existing.ID),
			))
			if err != nil {
				return nil, err
			}
			if len(notifications) == 0 {
				log.L(ctx).Debugf("Detected partial success from batch insert on blockchain event %s", existing.ProtocolID)
				inserted = append(inserted, existing) // notify caller that this is actually a new row
			} else {
				log.L(ctx).Debugf("Ignoring duplicate blockchain event %s", existing.ProtocolID)
			}
			t.addBlockchainEventToCache(existing)
		} else {
			inserted = append(inserted, event)
			t.addBlockchainEventToCache(event)
		}
	}

	return inserted, nil
}

func (t *transactionHelper) FindOperationInTransaction(ctx context.Context, tx *fftypes.UUID, opType core.OpType) (*core.Operation, error) {
	fb := database.OperationQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("tx", tx),
		fb.Eq("type", opType),
	)
	ops, _, err := t.database.GetOperations(ctx, t.namespace, filter)
	if err != nil || len(ops) == 0 {
		return nil, err
	}
	return ops[0], nil
}
