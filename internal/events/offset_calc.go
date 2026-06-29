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

package events

import (
	"context"
	"strconv"

	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

func calcFirstOffset(ctx context.Context, ns string, di database.Plugin, pfe *core.SubOptsFirstEvent) (firstOffset int64, err error) {
	firstEvent := core.SubOptsFirstEventNewest
	if pfe != nil {
		firstEvent = *pfe
	}
	firstOffset = -1
	var useNewest bool
	switch firstEvent {
	case "", core.SubOptsFirstEventNewest:
		useNewest = true
	case core.SubOptsFirstEventOldest:
		useNewest = false
	default:
		specificSequence, err := strconv.ParseInt(string(firstEvent), 10, 64)
		if err != nil {
			return -1, i18n.WrapError(ctx, err, coremsgs.MsgInvalidFirstEvent, firstEvent)
		}
		if specificSequence < -1 {
			return -1, i18n.NewError(ctx, coremsgs.MsgNumberMustBeGreaterEqual, -1)
		}
		firstOffset = specificSequence
		useNewest = false
	}
	if useNewest {
		f := database.EventQueryFactory.NewFilter(ctx).And().Sort("sequence").Descending().Limit(1)
		newestEvents, _, err := di.GetEvents(ctx, ns, f)
		if err != nil {
			return firstOffset, err
		}
		if len(newestEvents) > 0 {
			return newestEvents[0].Sequence, nil
		}
	}
	log.L(ctx).Debugf("Event poller initial offest: %d (newest=%t)", firstOffset, useNewest)
	return firstOffset, err
}
