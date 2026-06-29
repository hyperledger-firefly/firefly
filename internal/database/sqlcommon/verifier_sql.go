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
	verifierColumns = []string{
		"hash",
		"identity",
		"vtype",
		"namespace",
		"value",
		"created",
	}
	verifierFilterFieldMap = map[string]string{
		"type": "vtype",
	}
)

const verifiersTable = "verifiers"

func (s *SQLCommon) attemptVerifierUpdate(ctx context.Context, tx *dbsql.TXWrapper, verifier *core.Verifier) (int64, error) {
	return s.UpdateTx(ctx, verifiersTable, tx,
		sq.Update(verifiersTable).
			Set("identity", verifier.Identity).
			Set("vtype", verifier.Type).
			Set("value", verifier.Value).
			Where(sq.Eq{
				"hash": verifier.Hash,
			}),
		func() {
			s.callbacks.HashCollectionNSEvent(database.CollectionVerifiers, core.ChangeEventTypeUpdated, verifier.Namespace, verifier.Hash)
		})
}

func (s *SQLCommon) attemptVerifierInsert(ctx context.Context, tx *dbsql.TXWrapper, verifier *core.Verifier, requestConflictEmptyResult bool) (err error) {
	verifier.Created = fftypes.Now()
	_, err = s.InsertTxExt(ctx, verifiersTable, tx,
		sq.Insert(verifiersTable).
			Columns(verifierColumns...).
			Values(
				verifier.Hash,
				verifier.Identity,
				verifier.Type,
				verifier.Namespace,
				verifier.Value,
				verifier.Created,
			),
		func() {
			s.callbacks.HashCollectionNSEvent(database.CollectionVerifiers, core.ChangeEventTypeCreated, verifier.Namespace, verifier.Hash)
		}, requestConflictEmptyResult)
	return err
}

func (s *SQLCommon) UpsertVerifier(ctx context.Context, verifier *core.Verifier, optimization database.UpsertOptimization) (err error) {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	optimized := false
	if optimization == database.UpsertOptimizationNew {
		opErr := s.attemptVerifierInsert(ctx, tx, verifier, true /* we want a failure here we can progress past */)
		optimized = opErr == nil
	} else if optimization == database.UpsertOptimizationExisting {
		rowsAffected, opErr := s.attemptVerifierUpdate(ctx, tx, verifier)
		optimized = opErr == nil && rowsAffected == 1
	}

	if !optimized {
		// Do a select within the transaction to detemine if the UUID already exists
		msgRows, _, err := s.QueryTx(ctx, verifiersTable, tx,
			sq.Select("hash").
				From(verifiersTable).
				Where(sq.Eq{
					"namespace": verifier.Namespace,
					"hash":      verifier.Hash,
				}),
		)
		if err != nil {
			return err
		}
		existing := msgRows.Next()
		msgRows.Close()

		if existing {
			// Update the verifier
			if _, err = s.attemptVerifierUpdate(ctx, tx, verifier); err != nil {
				return err
			}
		} else {
			if err = s.attemptVerifierInsert(ctx, tx, verifier, false); err != nil {
				return err
			}
		}
	}

	return s.CommitTx(ctx, tx, autoCommit)
}

func (s *SQLCommon) verifierResult(ctx context.Context, row *sql.Rows) (*core.Verifier, error) {
	verifier := core.Verifier{}
	err := row.Scan(
		&verifier.Hash,
		&verifier.Identity,
		&verifier.Type,
		&verifier.Namespace,
		&verifier.Value,
		&verifier.Created,
	)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, verifiersTable)
	}
	return &verifier, nil
}

func (s *SQLCommon) getVerifierPred(ctx context.Context, desc string, pred interface{}) (verifier *core.Verifier, err error) {

	rows, _, err := s.Query(ctx, verifiersTable,
		sq.Select(verifierColumns...).
			From(verifiersTable).
			Where(pred),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		log.L(ctx).Debugf("Verifier '%s' not found", desc)
		return nil, nil
	}

	return s.verifierResult(ctx, rows)
}

func (s *SQLCommon) GetVerifierByValue(ctx context.Context, vType core.VerifierType, namespace, value string) (verifier *core.Verifier, err error) {
	return s.getVerifierPred(ctx, value, sq.Eq{"vtype": vType, "namespace": namespace, "value": value})
}

func (s *SQLCommon) GetVerifierByHash(ctx context.Context, namespace string, hash *fftypes.Bytes32) (verifier *core.Verifier, err error) {
	return s.getVerifierPred(ctx, hash.String(), sq.Eq{"hash": hash, "namespace": namespace})
}

func (s *SQLCommon) GetVerifiers(ctx context.Context, namespace string, filter ffapi.Filter) (verifiers []*core.Verifier, fr *ffapi.FilterResult, err error) {

	query, fop, fi, err := s.FilterSelect(ctx, "", sq.Select(verifierColumns...).From(verifiersTable), filter, verifierFilterFieldMap, []interface{}{"sequence"}, sq.Eq{"namespace": namespace})
	if err != nil {
		return nil, nil, err
	}

	rows, tx, err := s.Query(ctx, verifiersTable, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	verifiers = []*core.Verifier{}
	for rows.Next() {
		d, err := s.verifierResult(ctx, rows)
		if err != nil {
			return nil, nil, err
		}
		verifiers = append(verifiers, d)
	}

	return verifiers, s.QueryRes(ctx, verifiersTable, tx, fop, nil, fi), err

}
