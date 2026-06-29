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

	"github.com/hyperledger-firefly/common/pkg/fftypes"
)

// BatchState tracks the state between definition handlers that run in-line on the pin processing route in the aggregator
// as part of a batch of pins. They might have complex API calls and interdependencies that need to be managed via this state.
type BatchState struct {
	// PreFinalize callbacks may perform blocking actions (possibly to an external connector)
	// - Will execute after all batch messages have been processed
	// - Will execute outside database RunAsGroup
	// - If any PreFinalize callback errors out, batch will be aborted and retried
	PreFinalize []func(ctx context.Context) error

	// Finalize callbacks may perform final, non-idempotent database operations (such as inserting Events)
	// - Will execute after all batch messages have been processed and any PreFinalize callbacks have succeeded
	// - Will execute inside database RunAsGroup
	// - If any Finalize callback errors out, batch will be aborted and retried (small chance of duplicate execution here)
	Finalize []func(ctx context.Context) error

	// PendingConfirms are messages that are pending confirmation after already being processed in this batch
	PendingConfirms map[fftypes.UUID]*Message

	// ConfirmedDIDClaims are DID claims locked in within this batch
	ConfirmedDIDClaims []string
}

func (bs *BatchState) AddPreFinalize(action func(ctx context.Context) error) {
	bs.PreFinalize = append(bs.PreFinalize, action)
}

func (bs *BatchState) AddFinalize(action func(ctx context.Context) error) {
	bs.Finalize = append(bs.Finalize, action)
}

func (bs *BatchState) AddPendingConfirm(id *fftypes.UUID, message *Message) {
	bs.PendingConfirms[*id] = message
}

func (bs *BatchState) AddConfirmedDIDClaim(did string) {
	bs.ConfirmedDIDClaims = append(bs.ConfirmedDIDClaims, did)
}

func (bs *BatchState) RunPreFinalize(ctx context.Context) error {
	for _, action := range bs.PreFinalize {
		if err := action(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (bs *BatchState) RunFinalize(ctx context.Context) error {
	for _, action := range bs.Finalize {
		if err := action(ctx); err != nil {
			return err
		}
	}
	return nil
}
