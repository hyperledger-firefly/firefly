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
	"fmt"

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
	contractListenerColumns = []string{
		"id",
		"interface_id",
		"event",
		"namespace",
		"name",
		"backend_id",
		"location",
		"signature",
		"topic",
		"options",
		"created",
		"filters",
	}
	contractListenerFilterFieldMap = map[string]string{
		"interface": "interface_id",
		"backendid": "backend_id",
	}
)

const contractlistenersTable = "contractlisteners"

func (s *SQLCommon) UpsertContractListener(ctx context.Context, listener *core.ContractListener, allowExisting bool) (err error) {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	existing := false
	if allowExisting {
		// Do a select within the transaction to detemine if the UUID already exists
		listenerRows, _, err := s.QueryTx(ctx, contractlistenersTable, tx,
			sq.Select("id").
				From(contractlistenersTable).
				Where(sq.Eq{
					"namespace": listener.Namespace,
					"name":      listener.Name,
				}),
		)
		if err != nil {
			return err
		}

		existing = listenerRows.Next()
		if existing {
			var id fftypes.UUID
			_ = listenerRows.Scan(&id)
			if listener.ID != nil {
				if *listener.ID != id {
					listenerRows.Close()
					return database.IDMismatch
				}
			}
			listener.ID = &id // Update on returned object
		}
		listenerRows.Close()
	}

	if existing {
		var interfaceID *fftypes.UUID
		if listener.Interface != nil {
			interfaceID = listener.Interface.ID
		}
		// Update the listener
		if _, err = s.UpdateTx(ctx, contractlistenersTable, tx,
			sq.Update(contractlistenersTable).
				// Note we do not update ID
				Set("backend_id", listener.BackendID).
				Set("filters", listener.Filters).
				Set("event", listener.Event).
				Set("signature", listener.Signature).
				Set("options", listener.Options).
				Set("topic", listener.Topic).
				Set("location", listener.Location).
				Set("interface_id", interfaceID).
				Where(sq.Eq{
					"namespace": listener.Namespace,
					"name":      listener.Name,
				}),
			func() {
				s.callbacks.UUIDCollectionNSEvent(database.CollectionContractListeners, core.ChangeEventTypeUpdated, listener.Namespace, listener.ID)
			},
		); err != nil {
			return err
		}
	} else {
		if listener.ID == nil {
			listener.ID = fftypes.NewUUID()
		}

		err = s.InsertContractListener(ctx, listener)
		if err != nil {
			return err
		}
	}

	return s.CommitTx(ctx, tx, autoCommit)
}

func (s *SQLCommon) InsertContractListener(ctx context.Context, listener *core.ContractListener) (err error) {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	var interfaceID *fftypes.UUID
	if listener.Interface != nil {
		interfaceID = listener.Interface.ID
	}

	listener.Created = fftypes.Now()
	if _, err = s.InsertTx(ctx, contractlistenersTable, tx,
		sq.Insert(contractlistenersTable).
			Columns(contractListenerColumns...).
			Values(
				listener.ID,
				interfaceID,
				listener.Event,
				listener.Namespace,
				listener.Name,
				listener.BackendID,
				listener.Location,
				listener.Signature,
				listener.Topic,
				listener.Options,
				listener.Created,
				listener.Filters,
			),
		func() {
			s.callbacks.UUIDCollectionNSEvent(database.CollectionContractListeners, core.ChangeEventTypeCreated, listener.Namespace, listener.ID)
		},
	); err != nil {
		return err
	}

	return s.CommitTx(ctx, tx, autoCommit)
}

func (s *SQLCommon) contractListenerResult(ctx context.Context, row *sql.Rows) (*core.ContractListener, error) {
	listener := core.ContractListener{
		Interface: &fftypes.FFIReference{},
	}
	err := row.Scan(
		&listener.ID,
		&listener.Interface.ID,
		&listener.Event,
		&listener.Namespace,
		&listener.Name,
		&listener.BackendID,
		&listener.Location,
		&listener.Signature,
		&listener.Topic,
		&listener.Options,
		&listener.Created,
		&listener.Filters,
	)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, contractlistenersTable)
	}

	// Note: If we have a legacy "event" and "address" stored in the DB, it will be returned as before with the event at the top level

	return &listener, nil
}

func (s *SQLCommon) getContractListenerPred(ctx context.Context, desc string, pred interface{}) (*core.ContractListener, error) {
	rows, _, err := s.Query(ctx, contractlistenersTable,
		sq.Select(contractListenerColumns...).
			From(contractlistenersTable).
			Where(pred),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		log.L(ctx).Debugf("Contract listener '%s' not found", desc)
		return nil, nil
	}

	sub, err := s.contractListenerResult(ctx, rows)
	if err != nil {
		return nil, err
	}

	return sub, nil
}

func (s *SQLCommon) GetContractListener(ctx context.Context, namespace, name string) (sub *core.ContractListener, err error) {
	return s.getContractListenerPred(ctx, fmt.Sprintf("%s:%s", namespace, name), sq.Eq{"namespace": namespace, "name": name})
}

func (s *SQLCommon) GetContractListenerByID(ctx context.Context, namespace string, id *fftypes.UUID) (sub *core.ContractListener, err error) {
	return s.getContractListenerPred(ctx, id.String(), sq.Eq{"id": id, "namespace": namespace})
}

func (s *SQLCommon) GetContractListenerByBackendID(ctx context.Context, namespace, id string) (sub *core.ContractListener, err error) {
	return s.getContractListenerPred(ctx, id, sq.Eq{"backend_id": id, "namespace": namespace})
}

func (s *SQLCommon) GetContractListeners(ctx context.Context, namespace string, filter ffapi.Filter) ([]*core.ContractListener, *ffapi.FilterResult, error) {
	query, fop, fi, err := s.FilterSelect(ctx, "",
		sq.Select(contractListenerColumns...).From(contractlistenersTable),
		filter, contractListenerFilterFieldMap, []interface{}{"sequence"}, sq.Eq{"namespace": namespace})
	if err != nil {
		return nil, nil, err
	}

	rows, tx, err := s.Query(ctx, contractlistenersTable, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	subs := []*core.ContractListener{}
	for rows.Next() {
		sub, err := s.contractListenerResult(ctx, rows)
		if err != nil {
			return nil, nil, err
		}
		subs = append(subs, sub)
	}

	return subs, s.QueryRes(ctx, contractlistenersTable, tx, fop, nil, fi), err
}

func (s *SQLCommon) UpdateContractListener(ctx context.Context, ns string, id *fftypes.UUID, update ffapi.Update) (err error) {

	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	query, err := s.BuildUpdate(sq.Update(contractlistenersTable), update, contractListenerFilterFieldMap)
	if err != nil {
		return err
	}
	query = query.Where(sq.And{
		sq.Eq{"id": id},
		sq.Eq{"namespace": ns},
	})

	ra, err := s.UpdateTx(ctx, contractlistenersTable, tx, query, func() {
		s.callbacks.UUIDCollectionNSEvent(database.CollectionContractListeners, core.ChangeEventTypeUpdated, ns, id)
	})
	if err != nil {
		return err
	}
	if ra < 1 {
		return i18n.NewError(ctx, coremsgs.Msg404NoResult)
	}

	return s.CommitTx(ctx, tx, autoCommit)
}

func (s *SQLCommon) DeleteContractListenerByID(ctx context.Context, namespace string, id *fftypes.UUID) (err error) {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	sub, err := s.GetContractListenerByID(ctx, namespace, id)
	if err == nil && sub != nil {
		err = s.DeleteTx(ctx, contractlistenersTable, tx, sq.Delete(contractlistenersTable).Where(sq.Eq{"id": id}),
			func() {
				s.callbacks.UUIDCollectionNSEvent(database.CollectionContractListeners, core.ChangeEventTypeDeleted, sub.Namespace, sub.ID)
			},
		)
		if err != nil {
			return err
		}
	}

	return s.CommitTx(ctx, tx, autoCommit)
}
