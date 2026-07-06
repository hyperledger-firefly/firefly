// Copyright © 2026 Kaleido, Inc.
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

package utils

import (
	"context"
	"encoding/hex"
	"strings"
	"unicode/utf8"

	"github.com/hyperledger/firefly-common/pkg/log"
)

// DBSafeUTF8StringFromPtr returns a DB-safe UTF-8 string from a pointer.
// Nil pointers return "". Strings containing invalid UTF-8 sequences or null
// bytes (which PostgreSQL rejects in text columns even though null bytes are
// technically valid UTF-8) are hex-encoded instead, with a warning logged.
func DBSafeUTF8StringFromPtr(ctx context.Context, s *string) string {
	if s == nil {
		return ""
	}
	if !utf8.ValidString(*s) || strings.ContainsRune(*s, 0) {
		hexString := hex.EncodeToString([]byte(*s))
		log.L(ctx).Warnf("String contains invalid UTF-8 or null bytes - encoding as hex: %s", hexString)
		return hexString
	}
	return *s
}
