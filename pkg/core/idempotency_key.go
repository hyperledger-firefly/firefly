// Copyright © 2022 Kaleido, Inc.
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

	"github.com/hyperledger-firefly/common/pkg/i18n"
)

// IdempotencyKey is accessed in Go as a string, but when persisted to storage it will be stored as a null
// to allow multiple entries in a unique index to exist with the same un-set idempotency key.
type IdempotencyKey string

func (ik IdempotencyKey) Value() (driver.Value, error) {
	if ik == "" {
		return nil, nil
	}
	return (string)(ik), nil
}

func (ik *IdempotencyKey) Scan(src interface{}) error {
	switch src := src.(type) {
	case nil:
		return nil
	case []byte:
		*ik = IdempotencyKey(src)
		return nil
	case string:
		*ik = IdempotencyKey(src)
		return nil
	default:
		return i18n.NewError(context.Background(), i18n.MsgTypeRestoreFailed, src, ik)
	}
}
