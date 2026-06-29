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

package core

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/stretchr/testify/assert"
)

func TestSQLSerializedManifest(t *testing.T) {

	msgID1 := fftypes.NewUUID()
	msgID2 := fftypes.NewUUID()
	batch := &Batch{
		BatchHeader: BatchHeader{
			ID: fftypes.NewUUID(),
		},
		Payload: BatchPayload{
			TX: TransactionRef{
				ID: fftypes.NewUUID(),
			},
			Messages: []*Message{
				{Header: MessageHeader{ID: msgID1}},
				{Header: MessageHeader{ID: msgID2}},
			},
		},
	}

	bp, manifest := batch.Confirmed()
	mfString := manifest.String()
	assert.Equal(t, batch.BatchHeader, bp.BatchHeader)
	assert.Equal(t, batch.Payload.TX, bp.TX)
	assert.Equal(t, mfString, bp.Manifest.String())
	assert.NotNil(t, bp.Confirmed)

	var mf *BatchManifest
	err := json.Unmarshal([]byte(mfString), &mf)
	assert.NoError(t, err)
	assert.Equal(t, msgID1, mf.Messages[0].ID)
	assert.Equal(t, msgID2, mf.Messages[1].ID)
	mfHash := sha256.Sum256([]byte(mfString))
	assert.Equal(t, fftypes.HashString(bp.GenManifest(batch.Payload.Messages, batch.Payload.Data).String()).String(), hex.EncodeToString(mfHash[:]))

	assert.Equal(t, batch, bp.GenInflight(batch.Payload.Messages, batch.Payload.Data))

	assert.NotEqual(t, batch.Payload.Hash().String(), hex.EncodeToString(mfHash[:]))

}
