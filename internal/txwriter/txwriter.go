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

package txwriter

import (
	"context"
	"fmt"
	"time"

	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly/internal/coreconfig"
	"github.com/hyperledger/firefly/internal/coremsgs"
	"github.com/hyperledger/firefly/internal/operations"
	"github.com/hyperledger/firefly/internal/txcommon"
	"github.com/hyperledger/firefly/pkg/core"
	"github.com/hyperledger/firefly/pkg/database"
)

type Writer interface {
	Start()
	WriteTransactionAndOps(ctx context.Context, txType core.TransactionType, idempotencyKey core.IdempotencyKey, operations ...*core.Operation) (*core.Transaction, error)
	Close()
}

// request is the dispatched fully validated blockchain transaction, with the previously
// resolved key, and validated signature & inputs ready to generate the operations
// for the blockchain connector.
// Could be a contract deployment, or a transaction invoke.
type request struct {
	txType         core.TransactionType
	idempotencyKey core.IdempotencyKey
	operations     []*core.Operation
	result         chan *result
}

type result struct {
	transaction *core.Transaction
	err         error
}

type txWriterBatch struct {
	id             string
	requests       []*request
	timeoutContext context.Context
	timeoutCancel  func()
}

// txWriter manages writing blockchain transactions and operations to the database.
// Does so with optimized multi-insert database operations, on a pool of routines
// that can manage many concurrent API requests efficiently against the DB.
type txWriter struct {
	bgContext   context.Context
	cancelFunc  func()
	database    database.Plugin
	txHelper    txcommon.Helper
	operations  operations.Manager
	workQueue   chan *request
	workersDone []chan struct{}
	closed      bool
	// Config
	namespace    string
	workerCount  int
	batchTimeout time.Duration
	batchMax     int
}

func NewTransactionWriter(ctx context.Context, ns string, di database.Plugin, txHelper txcommon.Helper, operations operations.Manager) Writer {

	workerCount := config.GetInt(coreconfig.TransactionWriterCount)
	if !di.Capabilities().Concurrency {
		log.L(ctx).Infof("Database plugin not configured for concurrency. Batched transaction writing disabled")
		workerCount = 0
	}
	tw := &txWriter{
		namespace:    ns,
		workerCount:  workerCount,
		batchTimeout: config.GetDuration(coreconfig.TransactionWriterBatchTimeout),
		batchMax:     config.GetInt(coreconfig.TransactionWriterBatchMaxTransactions),

		database:   di,
		txHelper:   txHelper,
		operations: operations,
	}
	tw.bgContext, tw.cancelFunc = context.WithCancel(ctx)
	return tw
}

func (tw *txWriter) WriteTransactionAndOps(ctx context.Context, txType core.TransactionType, idempotencyKey core.IdempotencyKey, operations ...*core.Operation) (*core.Transaction, error) {
	req := &request{
		txType:         txType,
		idempotencyKey: idempotencyKey,
		operations:     operations,
		result:         make(chan *result, 1), // allocate a slot for the result to avoid blocking
	}
	if tw.workerCount == 0 {
		// Workers disabled, execute in-line on the current context
		// (note we provide a slot in the channel to ensure this doesn't block)
		tw.executeBatch(ctx, &txWriterBatch{requests: []*request{req}})
	} else {
		// Dispatch to background worker pool
		select {
		case tw.workQueue <- req:
		case <-ctx.Done(): // caller context is cancelled before dispatch
			return nil, i18n.NewError(ctx, coremsgs.MsgContextCanceled)
		case <-tw.bgContext.Done(): // background context is cancelled before dispatch
			return nil, i18n.NewError(ctx, coremsgs.MsgContextCanceled)
		}
	}
	res := <-req.result
	return res.transaction, res.err
}

func (tw *txWriter) Start() {
	if tw.workerCount > 0 {
		tw.workQueue = make(chan *request)
		tw.workersDone = make([]chan struct{}, tw.workerCount)
		for i := 0; i < tw.workerCount; i++ {
			tw.workersDone[i] = make(chan struct{})
			go tw.writerLoop(i)
		}
	}
}

func (tw *txWriter) writerLoop(writerIndex int) {
	defer close(tw.workersDone[writerIndex])

	ctx := log.WithLogField(tw.bgContext, "job", fmt.Sprintf("txwriter_%.3d", writerIndex))
	var batchNumber int
	var batch *txWriterBatch
	for !tw.closed {
		var timeoutContext context.Context
		var timedOut bool
		if batch != nil {
			timeoutContext = batch.timeoutContext
		} else {
			timeoutContext = ctx
		}
		select {
		case work := <-tw.workQueue:
			if batch == nil {
				batchNumber++
				batch = &txWriterBatch{id: fmt.Sprintf("txw_%.3d_%.10d", writerIndex, batchNumber)}
				batch.timeoutContext, batch.timeoutCancel = context.WithTimeout(ctx, tw.batchTimeout)
			}
			batch.requests = append(batch.requests, work)
		case <-timeoutContext.Done():
			timedOut = true
		}

		if batch != nil && (timedOut || (len(batch.requests) >= tw.batchMax)) {
			batch.timeoutCancel()

			tw.executeBatch(ctx, batch)
			batch = nil
		}
	}
}

func (tw *txWriter) executeBatch(ctx context.Context, batch *txWriterBatch) {
	ctx = log.WithLogField(ctx, "batch", batch.id)
	err := tw.database.RunAsGroup(ctx, func(ctx context.Context) error {
		return tw.processBatch(ctx, batch)
	})
	if err != nil {
		for _, req := range batch.requests {
			req.result <- &result{err: err}
		}
	}
}

func (tw *txWriter) processBatch(ctx context.Context, batch *txWriterBatch) error {
	// First we try to insert all the transactions
	txInserts := make([]*txcommon.BatchedTransactionInsert, len(batch.requests))
	for i, req := range batch.requests {
		txInserts[i] = &txcommon.BatchedTransactionInsert{
			Input: txcommon.TransactionInsertInput{
				Type:           req.txType,
				IdempotencyKey: req.idempotencyKey,
			},
		}
	}
	if err := tw.txHelper.SubmitNewTransactionBatch(ctx, tw.namespace, txInserts); err != nil {
		return err
	}
	// Then we work out the actual number of new ones
	results := make([]*result, len(batch.requests))
	operations := make([]*core.Operation, 0, len(batch.requests))
	for i, insertResult := range txInserts {
		req := batch.requests[i]
		if insertResult.Output.IdempotencyError != nil {
			results[i] = &result{err: insertResult.Output.IdempotencyError}
		} else {
			txn := insertResult.Output.Transaction
			results[i] = &result{transaction: txn}
			// Set the transaction ID on all ops, and add to list for insertion
			for _, op := range req.operations {
				op.Transaction = txn.ID
				operations = append(operations, op)

				// This is a key log line, where we can provide all pieces of correlation data a user needs:
				// - The type of the operation
				// - The plugin/connector
				// - The idempotencyKey
				// - The FF Transaction ID
				// - The Operation ID
				log.L(ctx).Infof("FF_NEW_TRANSACTION_OPERATION: namespace=%s plugin=%s type=%s operationId=%s transactionId=%s idempotencyKey='%s'", op.Namespace, op.Plugin, op.Type, op.ID, txn.ID, txn.IdempotencyKey)
			}
		}
	}

	// Insert all the operations - these must be unique, as this is a brand new transaction
	if len(operations) > 0 {
		err := tw.operations.BulkInsertOperations(ctx, operations...)
		if err != nil {
			return err
		}
	}

	// Ok we're done. We're assured not to return an err, so we can dispatch the results to each
	for i, res := range results {
		batch.requests[i].result <- res
	}
	return nil
}

func (tw *txWriter) Close() {
	if !tw.closed {
		tw.closed = true
		tw.cancelFunc()
		for _, workerDone := range tw.workersDone {
			<-workerDone
		}
	}
}
