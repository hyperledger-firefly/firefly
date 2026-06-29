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
	groupColumns = []string{
		"message_id",
		"namespace",
		"namespace_local",
		"name",
		"hash",
		"created",
	}
	groupFilterFieldMap = map[string]string{
		"message": "message_id",
	}
)

const groupsTable = "groups"

func (s *SQLCommon) UpsertGroup(ctx context.Context, group *core.Group, optimization database.UpsertOptimization) (err error) {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	// We use an upsert optimization here for performance, but also to account for the situation where two threads
	// try to perform an insert concurrently and ensure a non-failure outcome.
	optimized := false
	if optimization == database.UpsertOptimizationNew {
		opErr := s.attemptGroupInsert(ctx, tx, group, true /* we want a failure here we can progress past */)
		optimized = opErr == nil
	} else if optimization == database.UpsertOptimizationExisting {
		rowsAffected, opErr := s.attemptGroupUpdate(ctx, tx, group)
		optimized = opErr == nil && rowsAffected == 1
	}

	existing := false
	if !optimized {
		// Do a select within the transaction to determine if the UUID already exists
		groupRows, _, err := s.QueryTx(ctx, groupsTable, tx,
			sq.Select("hash").
				From(groupsTable).
				Where(sq.Eq{"hash": group.Hash, "namespace_local": group.LocalNamespace}),
		)
		if err != nil {
			return err
		}
		existing = groupRows.Next()
		groupRows.Close()

		if existing {
			if _, err = s.attemptGroupUpdate(ctx, tx, group); err != nil {
				return err
			}
		} else {
			if err = s.attemptGroupInsert(ctx, tx, group, false); err != nil {
				return err
			}
		}
	}

	// Note the member list is not allowed to change, as it is part of the hash.
	// So the optimization above relies on the fact these are in a transaction, so the
	// whole group (with members) will have been inserted
	if (optimized && optimization == database.UpsertOptimizationNew) || (!optimized && !existing) {
		if err = s.updateMembers(ctx, tx, group, false); err != nil {
			return err
		}
	}

	return s.CommitTx(ctx, tx, autoCommit)
}

func (s *SQLCommon) attemptGroupUpdate(ctx context.Context, tx *dbsql.TXWrapper, group *core.Group) (int64, error) {
	// Update the group
	return s.UpdateTx(ctx, groupsTable, tx,
		sq.Update(groupsTable).
			Set("message_id", group.Message).
			Set("name", group.Name).
			Set("hash", group.Hash).
			Set("created", group.Created).
			Where(sq.Eq{"hash": group.Hash, "namespace_local": group.LocalNamespace}),
		func() {
			s.callbacks.HashCollectionNSEvent(database.CollectionGroups, core.ChangeEventTypeUpdated, group.LocalNamespace, group.Hash)
		},
	)
}

func (s *SQLCommon) attemptGroupInsert(ctx context.Context, tx *dbsql.TXWrapper, group *core.Group, requestConflictEmptyResult bool) error {
	_, err := s.InsertTxExt(ctx, groupsTable, tx,
		sq.Insert(groupsTable).
			Columns(groupColumns...).
			Values(
				group.Message,
				group.Namespace,
				group.LocalNamespace,
				group.Name,
				group.Hash,
				group.Created,
			),
		func() {
			s.callbacks.HashCollectionNSEvent(database.CollectionGroups, core.ChangeEventTypeCreated, group.LocalNamespace, group.Hash)
		},
		requestConflictEmptyResult,
	)
	return err
}

func (s *SQLCommon) updateMembers(ctx context.Context, tx *dbsql.TXWrapper, group *core.Group, existing bool) error {

	if existing {
		if err := s.DeleteTx(ctx, groupsTable, tx,
			sq.Delete("members").
				Where(sq.And{
					sq.Eq{"group_hash": group.Hash},
				}),
			nil, // no db change event for this sub update
		); err != nil && err != database.DeleteRecordNotFound {
			return err
		}
	}

	// Run through the ones in the group, finding ones that already exist, and ones that need to be created
	for requiredIdx, requiredMember := range group.Members {
		if requiredMember == nil || requiredMember.Identity == "" {
			return i18n.NewError(ctx, i18n.MsgEmptyMemberIdentity, requiredIdx)
		}
		if requiredMember.Node == nil {
			return i18n.NewError(ctx, i18n.MsgEmptyMemberNode, requiredIdx)
		}
		if _, err := s.InsertTx(ctx, groupsTable, tx,
			sq.Insert("members").
				Columns(
					"group_hash",
					"identity",
					"node_id",
					"idx",
				).
				Values(
					group.Hash,
					requiredMember.Identity,
					requiredMember.Node,
					requiredIdx,
				),
			nil, // no db change event for this sub update
		); err != nil {
			return err
		}
	}

	return nil

}

func (s *SQLCommon) loadMembers(ctx context.Context, groups []*core.Group) error {

	groupIDs := make([]string, len(groups))
	for i, m := range groups {
		if m != nil {
			groupIDs[i] = m.Hash.String()
		}
	}

	members, _, err := s.Query(ctx, groupsTable,
		sq.Select(
			"group_hash",
			"identity",
			"node_id",
			"idx",
		).
			From("members").
			Where(sq.Eq{"group_hash": groupIDs}).
			OrderBy("idx"),
	)
	if err != nil {
		return err
	}
	defer members.Close()

	for members.Next() {
		var groupID fftypes.Bytes32
		member := &core.Member{}
		var idx int
		if err = members.Scan(&groupID, &member.Identity, &member.Node, &idx); err != nil {
			return i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, "members")
		}
		for _, g := range groups {
			if g.Hash.Equals(&groupID) {
				g.Members = append(g.Members, member)
			}
		}
	}
	// Ensure we return an empty array if no entries, and a consistent order for the data
	for _, g := range groups {
		if g.Members == nil {
			g.Members = core.Members{}
		}
	}

	return nil
}

func (s *SQLCommon) groupResult(ctx context.Context, row *sql.Rows) (*core.Group, error) {
	var group core.Group
	err := row.Scan(
		&group.Message,
		&group.Namespace,
		&group.LocalNamespace,
		&group.Name,
		&group.Hash,
		&group.Created,
	)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, groupsTable)
	}
	return &group, nil
}

func (s *SQLCommon) GetGroupByHash(ctx context.Context, namespace string, hash *fftypes.Bytes32) (group *core.Group, err error) {

	rows, _, err := s.Query(ctx, groupsTable,
		sq.Select(groupColumns...).
			From(groupsTable).
			Where(sq.Eq{"hash": hash, "namespace_local": namespace}),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		log.L(ctx).Debugf("Group '%s' not found", hash)
		return nil, nil
	}

	group, err = s.groupResult(ctx, rows)
	if err != nil {
		return nil, err
	}

	rows.Close()
	if err = s.loadMembers(ctx, []*core.Group{group}); err != nil {
		return nil, err
	}

	return group, nil
}

func (s *SQLCommon) GetGroups(ctx context.Context, namespace string, filter ffapi.Filter) (group []*core.Group, res *ffapi.FilterResult, err error) {
	query, fop, fi, err := s.FilterSelect(ctx, "", sq.Select(groupColumns...).From(groupsTable),
		filter, groupFilterFieldMap, []interface{}{"sequence"}, sq.Eq{"namespace_local": namespace})
	if err != nil {
		return nil, nil, err
	}

	rows, tx, err := s.Query(ctx, groupsTable, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	groups := []*core.Group{}
	for rows.Next() {
		group, err := s.groupResult(ctx, rows)
		if err != nil {
			return nil, nil, err
		}
		groups = append(groups, group)
	}

	rows.Close()
	if len(groups) > 0 {
		if err = s.loadMembers(ctx, groups); err != nil {
			return nil, nil, err
		}
	}

	return groups, s.QueryRes(ctx, groupsTable, tx, fop, nil, fi), err
}
