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
	"context"
	"crypto/sha256"
	"encoding/json"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/stretchr/testify/assert"
)

func TestEstimateMessageSize(t *testing.T) {
	msg := Message{}
	assert.Equal(t, messageSizeEstimateBase, msg.EstimateSize(false))
	assert.Equal(t, messageSizeEstimateBase, msg.EstimateSize(true))
	msg.Data = DataRefs{
		{ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32(), ValueSize: 1000},
	}
	assert.Equal(t, messageSizeEstimateBase, msg.EstimateSize(false))
	assert.Equal(t, messageSizeEstimateBase+int64(1000), msg.EstimateSize(true))
}

func TestSealBareMessage(t *testing.T) {
	msg := Message{}
	err := msg.Seal(context.Background())
	assert.NoError(t, err)
	assert.NotNil(t, msg.Header.ID)
	assert.NotNil(t, msg.Header.DataHash)
	assert.NotNil(t, msg.Hash)
}

func TestSealEmptyTopicString(t *testing.T) {
	msg := Message{
		Header: MessageHeader{
			Topics: []string{""},
		},
	}
	err := msg.Seal(context.Background())
	assert.Regexp(t, `FF00140.*header.topics\[0\]`, err)
}

func TestSealBadTagString(t *testing.T) {
	msg := Message{
		Header: MessageHeader{
			Tag: "!wrong",
		},
	}
	err := msg.Seal(context.Background())
	assert.Regexp(t, `FF00140.*header.tag`, err)
}

func TestVerifyTXType(t *testing.T) {
	msg := Message{
		Header: MessageHeader{
			TxType: TransactionTypeBatchPin,
			Topics: []string{"topic1"},
		},
	}
	err := msg.Seal(context.Background())
	assert.NoError(t, err)
	err = msg.Verify(context.Background())
	assert.NoError(t, err)

	msg.Header.TxType = TransactionTypeUnpinned
	err = msg.Seal(context.Background())
	assert.NoError(t, err)
	err = msg.Verify(context.Background())
	assert.NoError(t, err)

	msg.Header.TxType = TransactionTypeTokenPool
	err = msg.Seal(context.Background())
	assert.Regexp(t, "FF00130", err)
}

func TestVerifyEmptyTopicString(t *testing.T) {
	msg := Message{
		Header: MessageHeader{
			TxType: TransactionTypeBatchPin,
			Topics: []string{""},
		},
	}
	err := msg.Verify(context.Background())
	assert.Regexp(t, `FF00140.*header.topics\[0\]`, err)
}

func TestVerifyBadTagString(t *testing.T) {
	msg := Message{
		Header: MessageHeader{
			TxType: TransactionTypeBatchPin,
			Tag:    "!wrong",
		},
	}
	err := msg.Verify(context.Background())
	assert.Regexp(t, `FF00140.*header.tag`, err)
}

func TestSealNilDataID(t *testing.T) {
	msg := Message{
		Header: MessageHeader{
			TxType: TransactionTypeBatchPin,
		},
		Data: DataRefs{
			{},
		},
	}
	err := msg.Seal(context.Background())
	assert.Regexp(t, "FF00128.*0", err)
}

func TestVerifyNilDataHash(t *testing.T) {
	msg := Message{
		Header: MessageHeader{
			TxType: TransactionTypeBatchPin,
		},
		Data: DataRefs{
			{ID: fftypes.NewUUID()},
		},
	}
	err := msg.Verify(context.Background())
	assert.Regexp(t, "FF00128.*0", err)
}

func TestSealDupDataID(t *testing.T) {
	id1 := fftypes.NewUUID()
	hash1 := fftypes.NewRandB32()
	hash2 := fftypes.NewRandB32()
	msg := Message{
		Data: DataRefs{
			{ID: id1, Hash: hash1},
			{ID: id1, Hash: hash2},
		},
	}
	err := msg.Seal(context.Background())
	assert.Regexp(t, "FF00129.*1", err)
}

func TestVerifyNilHashes(t *testing.T) {
	msg := Message{
		Header: MessageHeader{
			TxType: TransactionTypeBatchPin,
		},
	}
	err := msg.Verify(context.Background())
	assert.Regexp(t, "FF00131", err)
}

func TestVerifyNilMisMatchedHashes(t *testing.T) {
	msg := Message{
		Header: MessageHeader{
			TxType:   TransactionTypeBatchPin,
			DataHash: fftypes.NewRandB32(),
		},
		Hash: fftypes.NewRandB32(),
	}
	err := msg.Verify(context.Background())
	assert.Regexp(t, "FF00132", err)
}

func TestSealKnownMessage(t *testing.T) {
	msgid := fftypes.MustParseUUID("2cd37805-5f40-4e12-962e-67868cde3049")
	cid := fftypes.MustParseUUID("39296b6e-91b9-4a61-b279-833c85b04d94")
	data1 := fftypes.MustParseUUID("e3a3b714-7e49-4c73-a4ea-87a50b19961a")
	data2 := fftypes.MustParseUUID("cc66b23f-d340-4333-82d5-b63adc1c3c07")
	data3 := fftypes.MustParseUUID("189c8185-2b92-481a-847a-e57595ab3541")
	var gid fftypes.Bytes32
	gid.UnmarshalText([]byte("3fcc7e07069e441f07c9f6b26f16fcb2dc896222d72888675082fd308440d9ae"))
	var hash1, hash2, hash3 fftypes.Bytes32
	hash1.UnmarshalText([]byte("3fcc7e07069e441f07c9f6b26f16fcb2dc896222d72888675082fd308440d9ae"))
	hash2.UnmarshalText([]byte("1d1462e02d7acee49a8448267c65067e0bec893c9a0c050b9835efa376fec046"))
	hash3.UnmarshalText([]byte("284b535da66aa0734af56c708426d756331baec3bce3079e508003bcf4738ee6"))
	msg := Message{
		Header: MessageHeader{
			ID:     msgid,
			CID:    cid,
			Type:   MessageTypePrivate,
			TxType: TransactionTypeBatchPin,
			SignerRef: SignerRef{
				Author: "0x12345",
			},
			Namespace: "ns1",
			Topics:    []string{"topic1", "topic2"},
			Tag:       "tag1",
			Created:   fftypes.UnixTime(1620104103123456789),
			Group:     &gid,
		},
		Data: DataRefs{
			{ID: data1, Hash: &hash1},
			{Hash: &hash2, ID: data2}, // Demonstrating we hash in order id,hash order, even if Go definition is re-ordered
			{ID: data3, Hash: &hash3},
		},
	}
	err := msg.Seal(context.Background())
	assert.NoError(t, err)

	// Data IDs are hashed the the sequence of the message - that is preserved as the message moves through
	dataHashData, _ := json.Marshal(&msg.Data)
	var dataHash fftypes.Bytes32 = sha256.Sum256([]byte(dataHashData))
	assert.Equal(t, `[{"id":"e3a3b714-7e49-4c73-a4ea-87a50b19961a","hash":"3fcc7e07069e441f07c9f6b26f16fcb2dc896222d72888675082fd308440d9ae"},{"id":"cc66b23f-d340-4333-82d5-b63adc1c3c07","hash":"1d1462e02d7acee49a8448267c65067e0bec893c9a0c050b9835efa376fec046"},{"id":"189c8185-2b92-481a-847a-e57595ab3541","hash":"284b535da66aa0734af56c708426d756331baec3bce3079e508003bcf4738ee6"}]`, string(dataHashData))
	assert.Equal(t, `2468d5c26cc85968acaf8b96d09476453916ea4eab41632a31d09efc7ab297d2`, dataHash.String())
	assert.Equal(t, dataHash, *msg.Header.DataHash)

	// Header contains the data hash, and is hashed into the message hash
	actualHeader, _ := json.Marshal(&msg.Header)
	expectedHeader := `{"id":"2cd37805-5f40-4e12-962e-67868cde3049","cid":"39296b6e-91b9-4a61-b279-833c85b04d94","type":"private","txtype":"batch_pin","author":"0x12345","created":"2021-05-04T04:55:03.123456789Z","namespace":"ns1","group":"3fcc7e07069e441f07c9f6b26f16fcb2dc896222d72888675082fd308440d9ae","topics":["topic1","topic2"],"tag":"tag1","datahash":"2468d5c26cc85968acaf8b96d09476453916ea4eab41632a31d09efc7ab297d2"}`
	var msgHash fftypes.Bytes32 = sha256.Sum256([]byte(expectedHeader))
	assert.Equal(t, expectedHeader, string(actualHeader))
	assert.Equal(t, `b5b9da88ed83403754bf78ed596eab70b803fd02108635fb932c2416d9e379b7`, msgHash.String())
	assert.Equal(t, msgHash, *msg.Hash)

	// Verify also returns good
	err = msg.Verify(context.Background())
	assert.NoError(t, err)

	msg.Sequence = 12345
	var ls LocallySequenced = &msg
	assert.Equal(t, int64(12345), ls.LocalSequence())
}

func TestSetInlineData(t *testing.T) {
	msg := &MessageInOut{}
	msg.SetInlineData([]*Data{
		{ID: fftypes.NewUUID(), Value: fftypes.JSONAnyPtr(`"some data"`)},
	})
	b, err := json.Marshal(&msg)
	assert.NoError(t, err)
	assert.Regexp(t, "some data", string(b))
}

func TestMessageImmutable(t *testing.T) {
	msg := &Message{
		Header: MessageHeader{
			ID: fftypes.NewUUID(),
		},
		BatchID:   fftypes.NewUUID(),
		Hash:      fftypes.NewRandB32(),
		State:     MessageStateConfirmed,
		Confirmed: fftypes.Now(),
		Data: DataRefs{
			{ID: fftypes.NewUUID(), Hash: fftypes.NewRandB32()},
		},
		Pins: fftypes.NewFFStringArray("pin1", "pin2"),
	}
	assert.True(t, msg.Hash.Equals(msg.BatchMessage().Hash))
}

func TestMessageActions(t *testing.T) {
	assert.Equal(t, "reject", ActionReject.String())
	assert.Equal(t, "confirm", ActionConfirm.String())
	assert.Equal(t, "retry", ActionRetry.String())
	assert.Equal(t, "wait", ActionWait.String())
	assert.Equal(t, "unknown", MessageAction(99999).String())
}
