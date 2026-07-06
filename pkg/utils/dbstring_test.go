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
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDBSafeUTF8StringFromPtrNil(t *testing.T) {
	assert.Equal(t, "", DBSafeUTF8StringFromPtr(context.Background(), nil))
}

func TestDBSafeUTF8StringFromPtrValid(t *testing.T) {
	msg := "FF23021: EVM reverted: some normal error message"
	assert.Equal(t, msg, DBSafeUTF8StringFromPtr(context.Background(), &msg))
}

func TestDBSafeUTF8StringFromPtrInvalidUTF8(t *testing.T) {
	msg := "[OCPE]404/98 - \x08\xc3\x79\xa0\x00\x00\x00[TMM]404/16e"
	assert.Equal(t, "5b4f4350455d3430342f3938202d2008c379a00000005b544d4d5d3430342f313665", DBSafeUTF8StringFromPtr(context.Background(), &msg))
}

func TestDBSafeUTF8StringFromPtrNullBytesInValidUTF8(t *testing.T) {
	msg := "hello\x00world"
	assert.Equal(t, "68656c6c6f00776f726c64", DBSafeUTF8StringFromPtr(context.Background(), &msg))
}
