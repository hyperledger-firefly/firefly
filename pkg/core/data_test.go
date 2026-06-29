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

func TestEstimateDataSize(t *testing.T) {
	d := Data{}
	assert.Equal(t, dataSizeEstimateBase, d.EstimateSize())
	d = Data{
		Value:     fftypes.JSONAnyPtr("Test"),
		ValueSize: 4,
	}
	assert.Equal(t, dataSizeEstimateBase+int64(4), d.EstimateSize())
	assert.Equal(t, dataSizeEstimateBase+int64(4), d.EstimateSize())
}

func TestDatatypeReference(t *testing.T) {

	var dr *DatatypeRef
	assert.Equal(t, fftypes.NullString, dr.String())
	dr = &DatatypeRef{
		Name:    "customer",
		Version: "0.0.1",
	}
	assert.Equal(t, "customer/0.0.1", dr.String())

}

func TestValidateBadValidator(t *testing.T) {
	err := CheckValidatorType(context.Background(), "wrong")
	assert.Regexp(t, "FF00108", err)
}

func TestSealNoData(t *testing.T) {
	d := &Data{}
	err := d.Seal(context.Background(), nil)
	assert.Regexp(t, "FF00109", err)
}

func TestSealValueOnly(t *testing.T) {
	d := &Data{
		Value: fftypes.JSONAnyPtr("{}"),
		Blob:  &BlobRef{},
	}
	err := d.Seal(context.Background(), nil)
	assert.NoError(t, err)
	assert.Equal(t, d.Hash.String(), "44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a")
}

func TestSealBlobOnly(t *testing.T) {
	blobHash, _ := fftypes.ParseBytes32(context.Background(), "22440fcf4ee9ac8c1a83de36c3a9ef39f838d960971dc79b274718392f1735f9")
	d := &Data{
		Blob: &BlobRef{
			Hash: blobHash,
		},
	}
	err := d.Seal(context.Background(), &Blob{
		Hash: blobHash,
	})
	assert.NoError(t, err)
	assert.Equal(t, "22440fcf4ee9ac8c1a83de36c3a9ef39f838d960971dc79b274718392f1735f9", d.Hash.String())
}

func TestSealBlobExplictlyNamed(t *testing.T) {
	blobHash, _ := fftypes.ParseBytes32(context.Background(), "bec1b07d757894d5f8b0d6f09530ef89cb2168b3c00df12efbb6cf3d2937e7e1")
	d := &Data{
		Blob: &BlobRef{
			Hash: blobHash,
		},
		Value: fftypes.JSONAnyPtr(`{
			"name": "use this",
			"filename": "ignore this",
			"path": "ignore this too"
		}`),
	}
	err := d.Seal(context.Background(), &Blob{
		Hash: blobHash,
	})
	assert.NoError(t, err)
	assert.Equal(t, "5ed9e8c156d590c44c59dfa3455d556ab6a523c2e62cc5e699398ff7fcfd9313", d.Hash.String())
	assert.Equal(t, "use this", d.Blob.Name)
}

func TestSealBlobPathNamed(t *testing.T) {
	blobHash, _ := fftypes.ParseBytes32(context.Background(), "bec1b07d757894d5f8b0d6f09530ef89cb2168b3c00df12efbb6cf3d2937e7e1")
	d := &Data{
		Blob: &BlobRef{
			Hash: blobHash,
		},
		Value: fftypes.JSONAnyPtr(`{
			"filename": "file.ext",
			"path": "/path/to"
		}`),
	}
	err := d.Seal(context.Background(), &Blob{
		Hash: blobHash,
	})
	assert.NoError(t, err)
	assert.Equal(t, "be66bddaa0fc6e1da4d2989bb55560a36ac36c5b620d81ad1cc5f3ce5f7ee319", d.Hash.String())
	assert.Equal(t, "/path/to/file.ext", d.Blob.Name)
}

func TestSealBlobFileNamed(t *testing.T) {
	blobHash, _ := fftypes.ParseBytes32(context.Background(), "bec1b07d757894d5f8b0d6f09530ef89cb2168b3c00df12efbb6cf3d2937e7e1")
	d := &Data{
		Blob: &BlobRef{
			Hash: blobHash,
		},
		Value: fftypes.JSONAnyPtr(`{
			"filename": "file.ext"
		}`),
	}
	err := d.Seal(context.Background(), &Blob{
		Hash: blobHash,
	})
	assert.NoError(t, err)
	assert.Equal(t, "8c58a0dc3b9d240e6e3066126e58f5853a18161733d2350f9e7488754e7160ec", d.Hash.String())
	assert.Equal(t, "file.ext", d.Blob.Name)
}

func TestSealBlobMismatch1(t *testing.T) {
	blobHash, _ := fftypes.ParseBytes32(context.Background(), "22440fcf4ee9ac8c1a83de36c3a9ef39f838d960971dc79b274718392f1735f9")
	d := &Data{
		Blob: &BlobRef{
			Hash: blobHash,
		},
	}
	err := d.Seal(context.Background(), &Blob{
		Hash: fftypes.NewRandB32(),
	})
	assert.Regexp(t, "FF00110", err)
}

func TestSealBlobMismatch2(t *testing.T) {
	d := &Data{
		Blob: &BlobRef{Hash: fftypes.NewRandB32()},
	}
	err := d.Seal(context.Background(), nil)
	assert.Regexp(t, "FF00110", err)
}

func TestSealBlobAndHashOnly(t *testing.T) {
	blobHash, _ := fftypes.ParseBytes32(context.Background(), "22440fcf4ee9ac8c1a83de36c3a9ef39f838d960971dc79b274718392f1735f9")
	d := &Data{
		Blob: &BlobRef{
			Hash: blobHash,
		},
		Value: fftypes.JSONAnyPtr("{}"),
	}
	h := sha256.Sum256([]byte(`44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a22440fcf4ee9ac8c1a83de36c3a9ef39f838d960971dc79b274718392f1735f9`))
	err := d.Seal(context.Background(), &Blob{
		Hash: blobHash,
		Size: 12345,
	})
	assert.NoError(t, err)
	assert.Equal(t, d.Hash[:], h[:])
	assert.Equal(t, int64(12345), d.Blob.Size)
}

func TestHashDataNull(t *testing.T) {

	jd := []byte(`{
		"id": "a64addf8-00e2-4210-9474-477e93b7f8dc",
		"validator": "none",
		"namespace": "default",
		"hash": "3e12f246f0d16ab3fe6d15d15161ca5de8a00991c98fa12cea1b9733ea9832da",
		"created": "2021-09-25T05:07:53.5847572Z",
		"datatype": {
			"name": "myblob"
		},
		"value": null,
		"blob": {
			"hash": "6014cbaf6bde9f9d755f347cb326db88859475e9d1a215d5dc4ccd8ae9caec7c"
		}
	}`)
	var d Data
	err := json.Unmarshal(jd, &d)
	assert.NoError(t, err)

	// Note that the processing of "null" means the value does not contribute to the hash
	expectedHash, err := fftypes.ParseBytes32(context.Background(), "6014cbaf6bde9f9d755f347cb326db88859475e9d1a215d5dc4ccd8ae9caec7c")
	assert.NoError(t, err)
	hash, err := d.CalcHash(context.Background())
	assert.NoError(t, err)
	assert.Equal(t, expectedHash.String(), hash.String())

}

func TestDataImmutable(t *testing.T) {
	data := &Data{
		ID:        fftypes.NewUUID(),
		Validator: ValidatorTypeJSON,
		Namespace: "ns1",
		Hash:      fftypes.NewRandB32(),
		Created:   fftypes.Now(),
	}
	assert.True(t, data.Hash.Equals(data.BatchData(BatchTypeBroadcast).Hash))

	data.Blob = &BlobRef{
		Hash:   fftypes.NewRandB32(),
		Size:   12345,
		Name:   "name.txt",
		Public: "sharedStorageRef",
	}
	assert.Equal(t, data.Blob, data.BatchData(BatchTypeBroadcast).Blob)
	assert.Empty(t, data.BatchData(BatchTypePrivate).Blob.Public)
}

func TestDataArryToRefs(t *testing.T) {
	data1 := &Data{
		ID:        fftypes.NewUUID(),
		Validator: ValidatorTypeJSON,
		Namespace: "ns1",
		Hash:      fftypes.NewRandB32(),
		Created:   fftypes.Now(),
		ValueSize: 12345,
	}
	data2 := &Data{
		ID:        fftypes.NewUUID(),
		Validator: ValidatorTypeJSON,
		Namespace: "ns1",
		Hash:      fftypes.NewRandB32(),
		Created:   fftypes.Now(),
		ValueSize: 23456,
	}

	da := DataArray{data1, data2}
	assert.Equal(t, da.Refs(), DataRefs{
		{ID: data1.ID, Hash: data1.Hash, ValueSize: 12345},
		{ID: data2.ID, Hash: data2.Hash, ValueSize: 23456},
	})

}

func TestPath(t *testing.T) {

	var d *Data
	d.CalcPath() // data nil

	d = &Data{}
	d.CalcPath() // blob nil

	d = &Data{
		Blob: &BlobRef{},
	}
	d.CalcPath()
	assert.Empty(t, d.Blob.Path)

	d = &Data{
		Blob: &BlobRef{
			Name: "root.file",
		},
	}
	d.CalcPath()
	assert.Equal(t, "/", d.Blob.Path)

	d = &Data{
		Blob: &BlobRef{
			Name: "/root.file",
		},
	}
	d.CalcPath()
	assert.Equal(t, "/", d.Blob.Path)

	d = &Data{
		Blob: &BlobRef{
			Name: "/sub/path/to/file",
		},
	}
	d.CalcPath()
	assert.Equal(t, "/sub/path/to", d.Blob.Path)

	d = &Data{
		Blob: &BlobRef{
			Name: "sub/path/to/file",
		},
	}
	d.CalcPath()
	assert.Equal(t, "/sub/path/to", d.Blob.Path)

}
