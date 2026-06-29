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
	subscriptionColumns = []string{
		"id",
		"namespace",
		"name",
		"transport",
		"filters",
		"options",
		"created",
		"updated",
	}
	subscriptionFilterFieldMap = map[string]string{}
)

const subscriptionsTable = "subscriptions"

func (s *SQLCommon) UpsertSubscription(ctx context.Context, subscription *core.Subscription, allowExisting bool) (err error) {
	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	existing := false
	if allowExisting {
		// Do a select within the transaction to detemine if the UUID already exists
		subscriptionRows, _, err := s.QueryTx(ctx, subscriptionsTable, tx,
			sq.Select("id").
				From(subscriptionsTable).
				Where(sq.Eq{
					"namespace": subscription.Namespace,
					"name":      subscription.Name,
				}),
		)
		if err != nil {
			return err
		}

		existing = subscriptionRows.Next()
		if existing {
			var id fftypes.UUID
			_ = subscriptionRows.Scan(&id)
			if subscription.ID != nil {
				if *subscription.ID != id {
					subscriptionRows.Close()
					return database.IDMismatch
				}
			}
			subscription.ID = &id // Update on returned object
		}
		subscriptionRows.Close()
	}

	if existing {
		// Update the subscription
		if _, err = s.UpdateTx(ctx, subscriptionsTable, tx,
			sq.Update(subscriptionsTable).
				// Note we do not update ID
				Set("name", subscription.Name).
				Set("transport", subscription.Transport).
				Set("filters", subscription.Filter).
				Set("options", subscription.Options).
				Set("created", subscription.Created).
				Set("updated", subscription.Updated).
				Where(sq.Eq{
					"namespace": subscription.Namespace,
					"name":      subscription.Name,
				}),
			func() {
				s.callbacks.UUIDCollectionNSEvent(database.CollectionSubscriptions, core.ChangeEventTypeUpdated, subscription.Namespace, subscription.ID)
			},
		); err != nil {
			return err
		}
	} else {
		if subscription.ID == nil {
			subscription.ID = fftypes.NewUUID()
		}

		if _, err = s.InsertTx(ctx, subscriptionsTable, tx,
			sq.Insert(subscriptionsTable).
				Columns(subscriptionColumns...).
				Values(
					subscription.ID,
					subscription.Namespace,
					subscription.Name,
					subscription.Transport,
					subscription.Filter,
					subscription.Options,
					subscription.Created,
					subscription.Updated,
				),
			func() {
				s.callbacks.UUIDCollectionNSEvent(database.CollectionSubscriptions, core.ChangeEventTypeCreated, subscription.Namespace, subscription.ID)
			},
		); err != nil {
			return err
		}

	}

	return s.CommitTx(ctx, tx, autoCommit)
}

func (s *SQLCommon) subscriptionResult(ctx context.Context, row *sql.Rows) (*core.Subscription, error) {
	subscription := core.Subscription{}
	err := row.Scan(
		&subscription.ID,
		&subscription.Namespace,
		&subscription.Name,
		&subscription.Transport,
		&subscription.Filter,
		&subscription.Options,
		&subscription.Created,
		&subscription.Updated,
	)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, coremsgs.MsgDBReadErr, subscriptionsTable)
	}
	return &subscription, nil
}

func (s *SQLCommon) getSubscriptionEq(ctx context.Context, eq sq.Eq, textName string) (message *core.Subscription, err error) {

	rows, _, err := s.Query(ctx, subscriptionsTable,
		sq.Select(subscriptionColumns...).
			From(subscriptionsTable).
			Where(eq),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	if !rows.Next() {
		log.L(ctx).Debugf("Subscription '%s' not found", textName)
		return nil, nil
	}

	subscription, err := s.subscriptionResult(ctx, rows)
	if err != nil {
		return nil, err
	}

	return subscription, nil
}

func (s *SQLCommon) GetSubscriptionByID(ctx context.Context, namespace string, id *fftypes.UUID) (message *core.Subscription, err error) {
	return s.getSubscriptionEq(ctx, sq.Eq{"id": id, "namespace": namespace}, id.String())
}

func (s *SQLCommon) GetSubscriptionByName(ctx context.Context, namespace, name string) (message *core.Subscription, err error) {
	return s.getSubscriptionEq(ctx, sq.Eq{"namespace": namespace, "name": name}, fmt.Sprintf("%s:%s", namespace, name))
}

func (s *SQLCommon) GetSubscriptions(ctx context.Context, namespace string, filter ffapi.Filter) (message []*core.Subscription, fr *ffapi.FilterResult, err error) {

	query, fop, fi, err := s.FilterSelect(
		ctx, "", sq.Select(subscriptionColumns...).From(subscriptionsTable),
		filter, subscriptionFilterFieldMap, []interface{}{"sequence"}, sq.Eq{"namespace": namespace})
	if err != nil {
		return nil, nil, err
	}

	rows, tx, err := s.Query(ctx, subscriptionsTable, query)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	subscription := []*core.Subscription{}
	for rows.Next() {
		d, err := s.subscriptionResult(ctx, rows)
		if err != nil {
			return nil, nil, err
		}
		subscription = append(subscription, d)
	}

	return subscription, s.QueryRes(ctx, subscriptionsTable, tx, fop, nil, fi), err

}

func (s *SQLCommon) UpdateSubscription(ctx context.Context, namespace, name string, update ffapi.Update) (err error) {

	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	subscription, err := s.GetSubscriptionByName(ctx, namespace, name)
	if err != nil {
		return err
	}
	if subscription == nil {
		return i18n.NewError(ctx, coremsgs.Msg404NoResult)
	}

	query, err := s.BuildUpdate(sq.Update(subscriptionsTable), update, subscriptionFilterFieldMap)
	if err != nil {
		return err
	}
	query = query.Where(sq.Eq{"id": subscription.ID})

	_, err = s.UpdateTx(ctx, subscriptionsTable, tx, query,
		func() {
			s.callbacks.UUIDCollectionNSEvent(database.CollectionSubscriptions, core.ChangeEventTypeUpdated, subscription.Namespace, subscription.ID)
		})
	if err != nil {
		return err
	}

	return s.CommitTx(ctx, tx, autoCommit)
}

func (s *SQLCommon) DeleteSubscriptionByID(ctx context.Context, namespace string, id *fftypes.UUID) (err error) {

	ctx, tx, autoCommit, err := s.BeginOrUseTx(ctx)
	if err != nil {
		return err
	}
	defer s.RollbackTx(ctx, tx, autoCommit)

	subscription, err := s.GetSubscriptionByID(ctx, namespace, id)
	if err == nil && subscription != nil {
		err = s.DeleteTx(ctx, subscriptionsTable, tx, sq.Delete(subscriptionsTable).Where(sq.Eq{
			"id": id,
		}),
			func() {
				s.callbacks.UUIDCollectionNSEvent(database.CollectionSubscriptions, core.ChangeEventTypeDeleted, subscription.Namespace, subscription.ID)
			})
		if err != nil {
			return err
		}
	}

	return s.CommitTx(ctx, tx, autoCommit)
}
