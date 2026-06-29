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

package core

import (
	"context"
	"database/sql/driver"
	"encoding/json"
	"net/url"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
)

// SubscriptionFilter contains regular expressions to match against events. All must match for an event to be dispatched to a subscription
type SubscriptionFilter struct {
	Events           string                `ffstruct:"SubscriptionFilter" json:"events,omitempty"`
	Message          MessageFilter         `ffstruct:"SubscriptionFilter" json:"message,omitempty"`
	Transaction      TransactionFilter     `ffstruct:"SubscriptionFilter" json:"transaction,omitempty"`
	BlockchainEvent  BlockchainEventFilter `ffstruct:"SubscriptionFilter" json:"blockchainevent,omitempty"`
	Topic            string                `ffstruct:"SubscriptionFilter" json:"topic,omitempty"`
	DeprecatedTopics string                `ffstruct:"SubscriptionFilter" json:"topics,omitempty"`
	DeprecatedTag    string                `ffstruct:"SubscriptionFilter" json:"tag,omitempty"`
	DeprecatedGroup  string                `ffstruct:"SubscriptionFilter" json:"group,omitempty"`
	DeprecatedAuthor string                `ffstruct:"SubscriptionFilter" json:"author,omitempty"`
}

func NewSubscriptionFilterFromQuery(query url.Values) SubscriptionFilter {
	return SubscriptionFilter{
		Events: query.Get("filter.events"),
		Message: MessageFilter{
			Group:  query.Get("filter.message.group"),
			Tag:    query.Get("filter.message.tag"),
			Author: query.Get("filter.message.author"),
		},
		BlockchainEvent: BlockchainEventFilter{
			Name:     query.Get("filter.blockchain.name"),
			Listener: query.Get("filter.blockchain.listener"),
		},
		Transaction: TransactionFilter{
			Type: query.Get("filter.transaction.type"),
		},
		Topic:            query.Get("filter.topic"),
		DeprecatedTag:    query.Get("filter.tag"),
		DeprecatedTopics: query.Get("filter.topics"),
		DeprecatedGroup:  query.Get("filter.group"),
		DeprecatedAuthor: query.Get("filter.author"),
	}
}

type MessageFilter struct {
	Tag    string `ffstruct:"SubscriptionMessageFilter" json:"tag,omitempty"`
	Group  string `ffstruct:"SubscriptionMessageFilter" json:"group,omitempty"`
	Author string `ffstruct:"SubscriptionMessageFilter" json:"author,omitempty"`
}

type TransactionFilter struct {
	Type string `ffstruct:"SubscriptionTransactionFilter" json:"type,omitempty"`
}

type BlockchainEventFilter struct {
	Name     string `ffstruct:"SubscriptionBlockchainEventFilter" json:"name,omitempty"`
	Listener string `ffstruct:"SubscriptionBlockchainEventFilter" json:"listener,omitempty"`
}

// SubOptsFirstEvent picks the first event that should be dispatched on the subscription, and can be a string containing an exact sequence as well as one of the enum values
type SubOptsFirstEvent string

const (
	// SubOptsFirstEventOldest indicates all events should be dispatched to the subscription
	SubOptsFirstEventOldest SubOptsFirstEvent = "oldest"
	// SubOptsFirstEventNewest indicates only newly received events should be dispatched to the subscription
	SubOptsFirstEventNewest SubOptsFirstEvent = "newest"
)

// SubscriptionCoreOptions are the core options that apply across all transports
// REMEMBER TO ADD OPTIONS HERE TO MarshalJSON()
type SubscriptionCoreOptions struct {
	FirstEvent   *SubOptsFirstEvent `ffstruct:"SubscriptionCoreOptions" json:"firstEvent,omitempty"`
	ReadAhead    *uint              `ffstruct:"SubscriptionCoreOptions" json:"readAhead,omitempty"`
	WithData     *bool              `ffstruct:"SubscriptionCoreOptions" json:"withData,omitempty"`
	Batch        *bool              `ffstruct:"SubscriptionCoreOptions" json:"batch,omitempty"`
	BatchTimeout *string            `ffstruct:"SubscriptionCoreOptions" json:"batchTimeout,omitempty"`
}

// SubscriptionOptions customize the behavior of subscriptions
type SubscriptionOptions struct {
	SubscriptionCoreOptions
	WebhookSubOptions

	// Extensible by the specific transport - so we serialize/de-serialize via map.
	additionalOptions fftypes.JSONObject
}

// SubscriptionRef are the fields that can be used to refer to a subscription
type SubscriptionRef struct {
	ID        *fftypes.UUID `ffstruct:"Subscription" json:"id,omitempty" ffexcludeinput:"true"`
	Namespace string        `ffstruct:"Subscription" json:"namespace"`
	Name      string        `ffstruct:"Subscription" json:"name"`
}

// Subscription is a binding between the stream of events within a namespace, and an event interface - such as an application listening on websockets
type Subscription struct {
	SubscriptionRef

	Transport string              `ffstruct:"Subscription" json:"transport"`
	Filter    SubscriptionFilter  `ffstruct:"Subscription" json:"filter"`
	Options   SubscriptionOptions `ffstruct:"Subscription" json:"options"`
	Ephemeral bool                `ffstruct:"Subscription" json:"ephemeral,omitempty" ffexcludeinput:"true"`
	Created   *fftypes.FFTime     `ffstruct:"Subscription" json:"created" ffexcludeinput:"true"`
	Updated   *fftypes.FFTime     `ffstruct:"Subscription" json:"updated" ffexcludeinput:"true"`
}

type SubscriptionWithStatus struct {
	Subscription
	Status SubscriptionStatus `ffstruct:"SubscriptionWithStatus" json:"status,omitempty" ffexcludeinput:"true"`
}

type SubscriptionStatus struct {
	CurrentOffset int64 `ffstruct:"SubscriptionStatus" json:"currentOffset,omitempty" ffexcludeinout:"true"`
}

func (so *SubscriptionOptions) UnmarshalJSON(b []byte) error {
	so.additionalOptions = fftypes.JSONObject{}
	err := json.Unmarshal(b, &so.additionalOptions)
	if err == nil {
		err = json.Unmarshal(b, &so.SubscriptionCoreOptions)
	}
	if err == nil {
		err = json.Unmarshal(b, &so.WebhookSubOptions)
	}
	if err != nil {
		return err
	}
	delete(so.additionalOptions, "firstEvent")
	delete(so.additionalOptions, "readAhead")
	delete(so.additionalOptions, "withData")
	return nil
}

func (so SubscriptionOptions) MarshalJSON() ([]byte, error) {
	if so.additionalOptions == nil {
		so.additionalOptions = fftypes.JSONObject{}
	}
	if so.WithData != nil {
		so.additionalOptions["withData"] = so.WithData
	}
	if so.FirstEvent != nil {
		so.additionalOptions["firstEvent"] = *so.FirstEvent
	}
	if so.ReadAhead != nil {
		so.additionalOptions["readAhead"] = float64(*so.ReadAhead)
	}
	if so.TLSConfigName != "" {
		so.additionalOptions["tlsConfigName"] = so.TLSConfigName
	}
	if so.Batch != nil {
		so.additionalOptions["batch"] = so.Batch
	}
	if so.BatchTimeout != nil {
		so.additionalOptions["batchTimeout"] = so.BatchTimeout
	}

	return json.Marshal(&so.additionalOptions)
}

func (so *SubscriptionOptions) TransportOptions() fftypes.JSONObject {
	if so.additionalOptions == nil {
		so.additionalOptions = fftypes.JSONObject{}
	}
	return so.additionalOptions
}

// Scan implements sql.Scanner
func (so *SubscriptionOptions) Scan(src interface{}) error {
	switch src := src.(type) {
	case []byte:
		return so.UnmarshalJSON(src)
	case string:
		return so.UnmarshalJSON([]byte(src))
	default:
		return i18n.NewError(context.Background(), i18n.MsgTypeRestoreFailed, src, so)
	}
}

// Value implements sql.Valuer
func (so SubscriptionOptions) Value() (driver.Value, error) {
	return so.MarshalJSON()
}

func (sf SubscriptionFilter) Value() (driver.Value, error) {
	return json.Marshal(&sf)
}

func (sf *SubscriptionFilter) Scan(src interface{}) error {
	switch src := src.(type) {
	case nil:
		return nil

	case []byte:
		return json.Unmarshal(src, &sf)

	case string:
		if src == "" {
			return nil
		}
		return json.Unmarshal([]byte(src), &sf)

	default:
		return i18n.NewError(context.Background(), i18n.MsgTypeRestoreFailed, src, sf)
	}
}
