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
	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

var (
	tokenApprovalColumns = []string{
		"local_id",
		"protocol_id",
		"subject",
		"active",
		"key",
		"operator_key",
		"pool_id",
		"connector",
		"namespace",
		"approved",
		"info",
		"tx_type",
		"tx_id",
		"blockchain_event",
		"created",
		"message_id",
		"message_hash",
	}
	tokenApprovalFilterFieldMap = map[string]string{
		"localid":         "local_id",
		"protocolid":      "protocol_id",
		"pool":            "pool_id",
		"approved":        "approved",
		"key":             "key",
		"operator":        "operator_key",
		"tx.type":         "tx_type",
		"tx.id":           "tx_id",
		"blockchainevent": "blockchain_event",
		"created":         "created",
		"message":         "message_id",
		"messagehash":     "message_hash",
	}
)

const tokenapprovalTable = "tokenapproval"

func (s *SQLCommon) UpsertTokenApproval(ctx context.Context, approval *core.TokenApproval) (err error) {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}

	defer s.RollbackTx(ctx, tx, autoCommit)
	rows, _, err := s.QueryTx(ctx, tokenapprovalTable, tx,
		sq.Select("seq").
			From(tokenapprovalTable).
			Where(sq.Eq{
				"namespace":   approval.Namespace,
				"protocol_id": approval.ProtocolID,
			}),
	)
	if err != nil {
		return err
	}
	existing := rows.Next()
	rows.Close()

	if existing {
		if _, err = s.UpdateTx(ctx, tokenapprovalTable, tx,
			sq.Update(tokenapprovalTable).
				Set("local_id", approval.LocalID).
				Set("subject", approval.Subject).
				Set("active", approval.Active).
				Set("key", approval.Key).
				Set("operator_key", approval.Operator).
				Set("pool_id", approval.Pool).
				Set("connector", approval.Connector).
				Set("approved", approval.Approved).
				Set("info", approval.Info).
				Set("tx_type", approval.TX.Type).
				Set("tx_id", approval.TX.ID).
				Set("blockchain_event", approval.BlockchainEvent).
				Set("message_id", approval.Message).
				Set("message_hash", approval.MessageHash).
				Where(sq.Eq{"protocol_id": approval.ProtocolID}),
			func() {
				s.callbacks.UUIDCollectionNSEvent(database.CollectionTokenApprovals, core.ChangeEventTypeUpdated, approval.Namespace, approval.LocalID)
			},
		); err != nil {
			return err
		}
	} else {
		approval.Created = fftypes.Now()
		if _, err = s.InsertTx(ctx, tokenapprovalTable, tx,
			sq.Insert(tokenapprovalTable).
				Columns(tokenApprovalColumns...).
				Values(
					approval.LocalID,
					approval.ProtocolID,
					approval.Subject,
					approval.Active,
					approval.Key,
					approval.Operator,
					approval.Pool,
					approval.Connector,
					approval.Namespace,
					approval.Approved,
					approval.Info,
					approval.TX.Type,
					approval.TX.ID,
					approval.BlockchainEvent,
					approval.Created,
					approval.Message,
					approval.MessageHash,
				),
			func() {
				s.callbacks.UUIDCollectionNSEvent(database.CollectionTokenApprovals, core.ChangeEventTypeCreated, approval.Namespace, approval.LocalID)
			},
		); err != nil {
			return err
		}
	}
	return s.CommitTx(ctx, tx, autoCommit)
}

func (s *SQLCommon) tokenApprovalResult(ctx context.Context, row *sql.Rows) (*core.TokenApproval, error) {
	approval := core.TokenApproval{}
	err := row.Scan(
		&approval.LocalID,
		&approval.ProtocolID,
		&approval.Subject,
		&approval.Active,
		&approval.Key,
		&approval.Operator,
		&approval.Pool,
		&approval.Connector,
		&approval.Namespace,
		&approval.Approved,
		&approval.Info,
		&approval.TX.Type,
		&approval.TX.ID,
		&approval.BlockchainEvent,
		&approval.Created,
		&approval.Message,
		&approval.MessageHash,
	)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, tokenapprovalTable)
	}
	return &approval, nil
}

func (s *SQLCommon) getTokenApprovalPred(ctx context.Context, desc string, pred interface{}) (*core.TokenApproval, error) {
	rows, _, err := s.Query(ctx, tokenapprovalTable,
		sq.Select(tokenApprovalColumns...).
			From(tokenapprovalTable).
			Where(pred),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		log.L(ctx).Debugf("Token approval '%s' not found", desc)
		return nil, nil
	}

	approval, err := s.tokenApprovalResult(ctx, rows)
	if err != nil {
		return nil, err
	}

	return approval, nil
}

func (s *SQLCommon) GetTokenApprovalByID(ctx context.Context, namespace string, localID *fftypes.UUID) (*core.TokenApproval, error) {
	return s.getTokenApprovalPred(ctx, localID.String(), sq.Eq{"local_id": localID, "namespace": namespace})
}

func (s *SQLCommon) GetTokenApprovalByProtocolID(ctx context.Context, namespace string, poolID *fftypes.UUID, protocolID string) (*core.TokenApproval, error) {
	return s.getTokenApprovalPred(ctx, protocolID, sq.And{
		sq.Eq{"namespace": namespace},
		sq.Eq{"pool_id": poolID},
		sq.Eq{"protocol_id": protocolID},
	})
}

func (s *SQLCommon) GetTokenApprovals(ctx context.Context, namespace string, filter ffapi.Filter) (approvals []*core.TokenApproval, fr *ffapi.FilterResult, err error) {
	query, fop, fi, err := s.FilterSelect(ctx, "", sq.Select(tokenApprovalColumns...).From(tokenapprovalTable),
		filter, tokenApprovalFilterFieldMap, []interface{}{"seq"}, sq.Eq{"namespace": namespace})
	if err != nil {
		return nil, nil, err
	}

	rows, tx, err := s.Query(ctx, tokenapprovalTable, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	approvals = []*core.TokenApproval{}
	for rows.Next() {
		d, err := s.tokenApprovalResult(ctx, rows)
		if err != nil {
			return nil, nil, err
		}
		approvals = append(approvals, d)
	}

	return approvals, s.QueryRes(ctx, tokenapprovalTable, tx, fop, nil, fi), err
}

func (s *SQLCommon) UpdateTokenApprovals(ctx context.Context, filter ffapi.Filter, update ffapi.Update) (err error) {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	query, err := s.BuildUpdate(sq.Update(tokenapprovalTable), update, tokenApprovalFilterFieldMap)
	if err != nil {
		return err
	}

	query, err = s.FilterUpdate(ctx, query, filter, tokenApprovalFilterFieldMap)
	if err != nil {
		return err
	}

	_, err = s.UpdateTx(ctx, tokenapprovalTable, tx, query, nil /* no change events filter based update */)
	if err != nil {
		return err
	}

	return s.CommitTx(ctx, tx, autoCommit)
}

func (s *SQLCommon) DeleteTokenApprovals(ctx context.Context, namespace string, poolID *fftypes.UUID) error {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	err = s.DeleteTx(ctx, tokenapprovalTable, tx, sq.Delete(tokenapprovalTable).Where(sq.Eq{
		"namespace": namespace,
		"pool_id":   poolID,
	}), nil)
	if err != nil && err != fftypes.DeleteRecordNotFound {
		return err
	}

	return s.CommitTx(ctx, tx, autoCommit)
}
