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

package websockets

import "github.com/hyperledger-firefly/common/pkg/config"

const (
	bufferSizeDefault = "16Kb"
)

const (
	// ReadBufferSize is the read buffer size for the socket
	ReadBufferSize = "readBufferSize"
	// WriteBufferSize is the write buffer size for the socket
	WriteBufferSize = "writeBufferSize"
)

func (ws *WebSockets) InitConfig(config config.Section) {
	config.AddKnownKey(ReadBufferSize, bufferSizeDefault)
	config.AddKnownKey(WriteBufferSize, bufferSizeDefault)
}
