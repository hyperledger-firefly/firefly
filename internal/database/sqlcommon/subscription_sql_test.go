// Copyright © 2021 Kaleido, Inc.
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
	"encoding/json"
	"fmt"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

func TestSubscriptionsE2EWithDB(t *testing.T) {

	s, cleanup := newSQLiteTestProvider(t)
	defer cleanup()
	ctx := context.Background()

	// Create a new subscription entry
	subscription := &core.Subscription{
		SubscriptionRef: core.SubscriptionRef{
			ID:        nil, // generated for us
			Namespace: "ns1",
			Name:      "subscription1",
		},
		Created: fftypes.Now(),
	}

	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionSubscriptions, core.ChangeEventTypeCreated, "ns1", mock.Anything).Return()

	err := s.UpsertSubscription(ctx, subscription, true)
	assert.NoError(t, err)

	// Check we get the exact same subscription back
	subscriptionRead, err := s.GetSubscriptionByName(ctx, subscription.Namespace, subscription.Name)
	assert.NoError(t, err)
	assert.NotNil(t, subscriptionRead)
	subscriptionJson, _ := json.Marshal(&subscription)
	subscriptionReadJson, _ := json.Marshal(&subscriptionRead)
	assert.Equal(t, string(subscriptionJson), string(subscriptionReadJson))

	// Update the subscription (this is testing what's possible at the database layer,
	// and does not account for the verification that happens at the higher level)
	newest := core.SubOptsFirstEventNewest
	fifty := uint(50)
	subOpts := core.SubscriptionOptions{
		SubscriptionCoreOptions: core.SubscriptionCoreOptions{
			FirstEvent: &newest,
			ReadAhead:  &fifty,
		},
		WebhookSubOptions: core.WebhookSubOptions{
			TLSConfigName: "myconfig",
		},
	}
	subOpts.TransportOptions()["my-transport-option"] = true
	subscriptionUpdated := &core.Subscription{
		SubscriptionRef: core.SubscriptionRef{
			ID:        fftypes.NewUUID(), // will fail with us trying to update this
			Namespace: "ns1",
			Name:      "subscription1",
		},
		Transport: "websockets",
		Filter: core.SubscriptionFilter{
			Events: string(core.EventTypeMessageConfirmed),
			Topic:  "topics.*",
			Message: core.MessageFilter{
				Tag:   "tag.*",
				Group: "group.*",
			},
		},
		Options: subOpts,
		Created: fftypes.Now(),
		Updated: fftypes.Now(),
	}

	// Rejects attempt to update ID
	err = s.UpsertSubscription(context.Background(), subscriptionUpdated, true)
	assert.Equal(t, database.IDMismatch, err)

	// Blank out the ID and retry
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionSubscriptions, core.ChangeEventTypeUpdated, "ns1", subscription.ID).Return()
	subscriptionUpdated.ID = nil
	err = s.UpsertSubscription(context.Background(), subscriptionUpdated, true)
	assert.NoError(t, err)

	// Check we get the exact same data back - note the removal of one of the subscription elements
	subscriptionRead, err = s.GetSubscriptionByID(ctx, "ns1", subscriptionUpdated.ID)
	assert.NoError(t, err)
	subscriptionJson, _ = json.Marshal(&subscriptionUpdated)
	subscriptionReadJson, _ = json.Marshal(&subscriptionRead)
	assert.Equal(t, string(subscriptionJson), string(subscriptionReadJson))
	assert.Equal(t, true, subscriptionRead.Options.TransportOptions()["my-transport-option"])
	assert.Equal(t, "myconfig", subscriptionRead.Options.TLSConfigName)

	// Query back the subscription
	fb := database.SubscriptionQueryFactory.NewFilter(ctx)
	filter := fb.And(
		fb.Eq("name", subscriptionUpdated.Name),
	)
	subscriptionRes, res, err := s.GetSubscriptions(ctx, "ns1", filter.Count(true))
	assert.NoError(t, err)
	assert.Equal(t, 1, len(subscriptionRes))
	assert.Equal(t, int64(1), *res.TotalCount)
	subscriptionReadJson, _ = json.Marshal(subscriptionRes[0])
	assert.Equal(t, string(subscriptionJson), string(subscriptionReadJson))

	// Update
	updateTime := fftypes.Now()
	up := database.SubscriptionQueryFactory.NewUpdate(ctx).Set("created", updateTime)
	err = s.UpdateSubscription(ctx, subscriptionUpdated.Namespace, subscriptionUpdated.Name, up)
	assert.NoError(t, err)

	// Test find updated value
	filter = fb.And(
		fb.Eq("name", subscriptionUpdated.Name),
		fb.Eq("created", updateTime.String()),
	)
	subscriptions, _, err := s.GetSubscriptions(ctx, "ns1", filter)
	assert.NoError(t, err)
	assert.Equal(t, 1, len(subscriptions))

	// Test delete, and refind no return
	s.callbacks.On("UUIDCollectionNSEvent", database.CollectionSubscriptions, core.ChangeEventTypeDeleted, "ns1", subscription.ID).Return()
	err = s.DeleteSubscriptionByID(ctx, "ns1", subscriptionUpdated.ID)
	assert.NoError(t, err)
	subscriptions, _, err = s.GetSubscriptions(ctx, "ns1", filter)
	assert.NoError(t, err)
	assert.Equal(t, 0, len(subscriptions))

	s.callbacks.AssertExpectations(t)
}

func TestUpsertSubscriptionFailBegin(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertSubscription(context.Background(), &core.Subscription{}, true)
	assert.Regexp(t, "FF00175", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertSubscriptionFailSelect(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpsertSubscription(context.Background(), &core.Subscription{SubscriptionRef: core.SubscriptionRef{Name: "name1"}}, true)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertSubscriptionFailInsert(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{}))
	mock.ExpectExec("INSERT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpsertSubscription(context.Background(), &core.Subscription{SubscriptionRef: core.SubscriptionRef{Name: "name1"}}, true)
	assert.Regexp(t, "FF00177", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertSubscriptionFailUpdate(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"name"}).
		AddRow("name1"))
	mock.ExpectExec("UPDATE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	err := s.UpsertSubscription(context.Background(), &core.Subscription{SubscriptionRef: core.SubscriptionRef{Name: "name1"}}, true)
	assert.Regexp(t, "FF00178", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestUpsertSubscriptionFailCommit(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"name"}))
	mock.ExpectExec("INSERT .*").WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit().WillReturnError(fmt.Errorf("pop"))
	err := s.UpsertSubscription(context.Background(), &core.Subscription{SubscriptionRef: core.SubscriptionRef{Name: "name1"}}, true)
	assert.Regexp(t, "FF00180", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetSubscriptionByIDSelectFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	_, err := s.GetSubscriptionByName(context.Background(), "ns1", "name1")
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetSubscriptionByIDNotFound(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"namespace", "name"}))
	msg, err := s.GetSubscriptionByName(context.Background(), "ns1", "name1")
	assert.NoError(t, err)
	assert.Nil(t, msg)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetSubscriptionByIDScanFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"namespace"}).AddRow("only one"))
	_, err := s.GetSubscriptionByName(context.Background(), "ns1", "name1")
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetSubscriptionQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	f := database.SubscriptionQueryFactory.NewFilter(context.Background()).Eq("name", "")
	_, _, err := s.GetSubscriptions(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00176", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestGetSubscriptionBuildQueryFail(t *testing.T) {
	s, _ := newMockProvider().init()
	f := database.SubscriptionQueryFactory.NewFilter(context.Background()).Eq("name", map[bool]bool{true: false})
	_, _, err := s.GetSubscriptions(context.Background(), "ns1", f)
	assert.Regexp(t, "FF00143.*type", err)
}

func TestGetSubscriptionReadMessageFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows([]string{"ntype"}).AddRow("only one"))
	f := database.SubscriptionQueryFactory.NewFilter(context.Background()).Eq("name", "")
	_, _, err := s.GetSubscriptions(context.Background(), "ns1", f)
	assert.Regexp(t, "FF10121", err)
	assert.NoError(t, mock.ExpectationsWereMet())
}

func TestSubscriptionUpdateBeginFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	u := database.SubscriptionQueryFactory.NewUpdate(context.Background()).Set("name", "anything")
	err := s.UpdateSubscription(context.Background(), "ns1", "name1", u)
	assert.Regexp(t, "FF00175", err)
}

func TestSubscriptionUpdateBuildQueryFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows(subscriptionColumns).AddRow(
		fftypes.NewUUID(), "ns1", "sub1", "websockets", `{}`, `{}`, fftypes.Now(), fftypes.Now()),
	)
	u := database.SubscriptionQueryFactory.NewUpdate(context.Background()).Set("name", map[bool]bool{true: false})
	err := s.UpdateSubscription(context.Background(), "ns1", "name1", u)
	assert.Regexp(t, "FF00143.*name", err)
}

func TestSubscriptionUpdateSelectFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	u := database.SubscriptionQueryFactory.NewUpdate(context.Background()).Set("name", fftypes.NewUUID())
	err := s.UpdateSubscription(context.Background(), "ns1", "name1", u)
	assert.Regexp(t, "FF00176", err)
}

func TestSubscriptionUpdateSelectNotFound(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows(subscriptionColumns))
	mock.ExpectRollback()
	u := database.SubscriptionQueryFactory.NewUpdate(context.Background()).Set("name", fftypes.NewUUID())
	err := s.UpdateSubscription(context.Background(), "ns1", "name1", u)
	assert.Regexp(t, "FF10143", err)
}

func TestSubscriptionUpdateFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows(subscriptionColumns).AddRow(
		fftypes.NewUUID(), "ns1", "sub1", "websockets", `{}`, `{}`, fftypes.Now(), fftypes.Now()),
	)
	mock.ExpectExec("UPDATE .*").WillReturnError(fmt.Errorf("pop"))
	mock.ExpectRollback()
	u := database.SubscriptionQueryFactory.NewUpdate(context.Background()).Set("name", fftypes.NewUUID())
	err := s.UpdateSubscription(context.Background(), "ns1", "name1", u)
	assert.Regexp(t, "FF00178", err)
}

func TestSubscriptionDeleteBeginFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin().WillReturnError(fmt.Errorf("pop"))
	err := s.DeleteSubscriptionByID(context.Background(), "ns1", fftypes.NewUUID())
	assert.Regexp(t, "FF00175", err)
}

func TestSubscriptionDeleteFail(t *testing.T) {
	s, mock := newMockProvider().init()
	mock.ExpectBegin()
	mock.ExpectQuery("SELECT .*").WillReturnRows(sqlmock.NewRows(subscriptionColumns).AddRow(
		fftypes.NewUUID(), "ns1", "sub1", "websockets", `{}`, `{}`, fftypes.Now(), fftypes.Now()),
	)
	mock.ExpectExec("DELETE .*").WillReturnError(fmt.Errorf("pop"))
	err := s.DeleteSubscriptionByID(context.Background(), "ns1", fftypes.NewUUID())
	assert.Regexp(t, "FF00179", err)
}
