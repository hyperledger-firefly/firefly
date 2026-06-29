// Copyright © 2023 Kaleido, Inc.
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

import "github.com/hyperledger-firefly/common/pkg/fftypes"

type WSSubscriptionStatus struct {
	StartTime *fftypes.FFTime    `ffstruct:"WSSubscriptionStatus" json:"startTime"`
	Ephemeral bool               `ffstruct:"WSSubscriptionStatus" json:"ephemeral"`
	Namespace string             `ffstruct:"WSSubscriptionStatus" json:"namespace"`
	Name      string             `ffstruct:"WSSubscriptionStatus" json:"name,omitempty"`
	Filter    SubscriptionFilter `ffstruct:"WSSubscriptionStatus" json:"filter"`
}

type WSConnectionStatus struct {
	ID            string                  `ffstruct:"WSConnectionStatus" json:"id"`
	RemoteAddress string                  `ffstruct:"WSConnectionStatus" json:"remoteAddress"`
	UserAgent     string                  `ffstruct:"WSConnectionStatus" json:"userAgent"`
	Subscriptions []*WSSubscriptionStatus `ffstruct:"WSConnectionStatus" json:"subscriptions"`
}

type WebSocketStatus struct {
	Enabled     bool                  `ffstruct:"WebSocketStatus" json:"enabled"`
	Connections []*WSConnectionStatus `ffstruct:"WebSocketStatus" json:"connections"`
}
