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

package sqlcommon

import (
	"context"
	"database/sql"

	sq "github.com/Masterminds/squirrel"
	"github.com/hyperledger-firefly/common/pkg/dbsql"
	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

var (
	tokenTransferColumns = []string{
		"type",
		"local_id",
		"pool_id",
		"token_index",
		"uri",
		"connector",
		"namespace",
		"key",
		"from_key",
		"to_key",
		"amount",
		"protocol_id",
		"message_id",
		"message_hash",
		"tx_type",
		"tx_id",
		"blockchain_event",
		"created",
	}
	tokenTransferFilterFieldMap = map[string]string{
		"type":            "type",
		"localid":         "local_id",
		"pool":            "pool_id",
		"tokenindex":      "token_index",
		"from":            "from_key",
		"to":              "to_key",
		"protocolid":      "protocol_id",
		"message":         "message_id",
		"messagehash":     "message_hash",
		"tx.type":         "tx_type",
		"tx.id":           "tx_id",
		"blockchainevent": "blockchain_event",
	}
)

const tokentransferTable = "tokentransfer"

func (s *SQLCommon) setTokenTransferEventInsertValues(query sq.InsertBuilder, transfer *core.TokenTransfer) sq.InsertBuilder {
	return query.Values(
		transfer.Type,
		transfer.LocalID,
		transfer.Pool,
		transfer.TokenIndex,
		transfer.URI,
		transfer.Connector,
		transfer.Namespace,
		transfer.Key,
		transfer.From,
		transfer.To,
		transfer.Amount,
		transfer.ProtocolID,
		transfer.Message,
		transfer.MessageHash,
		transfer.TX.Type,
		transfer.TX.ID,
		transfer.BlockchainEvent,
		transfer.Created,
	)
}

func (s *SQLCommon) attemptTokenTransferEventInsert(ctx context.Context, tx *dbsql.TXWrapper, transfer *core.TokenTransfer, requestConflictEmptyResult bool) (err error) {
	_, err = s.InsertTxExt(ctx, tokentransferTable, tx,
		s.setTokenTransferEventInsertValues(sq.Insert(tokentransferTable).Columns(tokenTransferColumns...), transfer),
		func() {
			s.callbacks.UUIDCollectionNSEvent(database.CollectionTokenTransfers, core.ChangeEventTypeCreated, transfer.Namespace, transfer.LocalID)
		}, requestConflictEmptyResult)
	return err
}

func (s *SQLCommon) InsertOrGetTokenTransfer(ctx context.Context, transfer *core.TokenTransfer) (existing *core.TokenTransfer, err error) {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return nil, err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	if transfer.Created == nil {
		transfer.Created = fftypes.Now()
	}
	opErr := s.attemptTokenTransferEventInsert(ctx, tx, transfer, true /* we want a failure here we can progress past */)
	if opErr == nil {
		return nil, s.CommitTx(ctx, tx, autoCommit)
	}

	// Do a select within the transaction to determine if the protocolID already exists
	existing, err = s.GetTokenTransferByProtocolID(ctx, transfer.Namespace, transfer.Pool, transfer.ProtocolID)
	if err != nil || existing != nil {
		return existing, err
	}
	// Error was apparently not a protocolID conflict - must have been something else
	return nil, opErr
}

func (s *SQLCommon) tokenTransferResult(ctx context.Context, row *sql.Rows) (*core.TokenTransfer, error) {
	transfer := core.TokenTransfer{}
	err := row.Scan(
		&transfer.Type,
		&transfer.LocalID,
		&transfer.Pool,
		&transfer.TokenIndex,
		&transfer.URI,
		&transfer.Connector,
		&transfer.Namespace,
		&transfer.Key,
		&transfer.From,
		&transfer.To,
		&transfer.Amount,
		&transfer.ProtocolID,
		&transfer.Message,
		&transfer.MessageHash,
		&transfer.TX.Type,
		&transfer.TX.ID,
		&transfer.BlockchainEvent,
		&transfer.Created,
	)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, tokentransferTable)
	}
	return &transfer, nil
}

func (s *SQLCommon) getTokenTransferPred(ctx context.Context, desc string, pred interface{}) (*core.TokenTransfer, error) {
	rows, _, err := s.Query(ctx, tokentransferTable,
		sq.Select(tokenTransferColumns...).
			From(tokentransferTable).
			Where(pred),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		log.L(ctx).Debugf("Token transfer '%s' not found", desc)
		return nil, nil
	}

	transfer, err := s.tokenTransferResult(ctx, rows)
	if err != nil {
		return nil, err
	}

	return transfer, nil
}

func (s *SQLCommon) GetTokenTransferByID(ctx context.Context, namespace string, localID *fftypes.UUID) (*core.TokenTransfer, error) {
	return s.getTokenTransferPred(ctx, localID.String(), sq.Eq{"local_id": localID, "namespace": namespace})
}

func (s *SQLCommon) GetTokenTransferByProtocolID(ctx context.Context, namespace string, poolID *fftypes.UUID, protocolID string) (*core.TokenTransfer, error) {
	return s.getTokenTransferPred(ctx, protocolID, sq.And{
		sq.Eq{"namespace": namespace},
		sq.Eq{"pool_id": poolID},
		sq.Eq{"protocol_id": protocolID},
	})
}

func (s *SQLCommon) GetTokenTransfers(ctx context.Context, namespace string, filter ffapi.Filter) (message []*core.TokenTransfer, fr *ffapi.FilterResult, err error) {
	query, fop, fi, err := s.FilterSelect(ctx, "", sq.Select(tokenTransferColumns...).From(tokentransferTable),
		filter, tokenTransferFilterFieldMap, []interface{}{"seq"}, sq.Eq{"namespace": namespace})
	if err != nil {
		return nil, nil, err
	}

	rows, tx, err := s.Query(ctx, tokentransferTable, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	transfers := []*core.TokenTransfer{}
	for rows.Next() {
		d, err := s.tokenTransferResult(ctx, rows)
		if err != nil {
			return nil, nil, err
		}
		transfers = append(transfers, d)
	}

	return transfers, s.QueryRes(ctx, tokentransferTable, tx, fop, nil, fi), err
}

func (s *SQLCommon) DeleteTokenTransfers(ctx context.Context, namespace string, poolID *fftypes.UUID) error {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	err = s.DeleteTx(ctx, tokentransferTable, tx, sq.Delete(tokentransferTable).Where(sq.Eq{
		"namespace": namespace,
		"pool_id":   poolID,
	}), nil)
	if err != nil && err != fftypes.DeleteRecordNotFound {
		return err
	}

	return s.CommitTx(ctx, tx, autoCommit)
}
