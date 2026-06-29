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

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
)

type DataRef struct {
	ID   *fftypes.UUID    `ffstruct:"DataRef" json:"id,omitempty"`
	Hash *fftypes.Bytes32 `ffstruct:"DataRef" json:"hash,omitempty" ffexcludeinput:"true"`

	ValueSize int64 `json:"-"` // used internally for message size calculation, without full payload retrieval
}

type BlobRef struct {
	Hash   *fftypes.Bytes32 `ffstruct:"BlobRef" json:"hash"`
	Size   int64            `ffstruct:"BlobRef" json:"size"`
	Name   string           `ffstruct:"BlobRef" json:"name"`
	Path   string           `ffstruct:"BlobRef" json:"path,omitempty"`
	Public string           `ffstruct:"BlobRef" json:"public,omitempty"`
}

type Data struct {
	ID        *fftypes.UUID    `ffstruct:"Data" json:"id,omitempty"`
	Validator ValidatorType    `ffstruct:"Data" json:"validator"`
	Namespace string           `ffstruct:"Data" json:"namespace,omitempty"`
	Hash      *fftypes.Bytes32 `ffstruct:"Data" json:"hash,omitempty"`
	Created   *fftypes.FFTime  `ffstruct:"Data" json:"created,omitempty"`
	Datatype  *DatatypeRef     `ffstruct:"Data" json:"datatype,omitempty"`
	Value     *fftypes.JSONAny `ffstruct:"Data" json:"value"`
	Public    string           `ffstruct:"Data" json:"public,omitempty"`
	Blob      *BlobRef         `ffstruct:"Data" json:"blob,omitempty"`

	ValueSize int64 `json:"-"` // Used internally for message size calculation, without full payload retrieval
}

func (br *BlobRef) BatchBlobRef(batchType BatchType) *BlobRef {
	if br == nil {
		return nil
	}
	switch batchType {
	case BatchTypePrivate:
		// For private we omit the "public" ref in all cases, to avoid an potential for the batch pay to change due
		// to the same data being allocated by the same data being sent in a broadcast batch (thus assigning a public ref).
		return &BlobRef{
			Hash: br.Hash,
			Size: br.Size,
			Name: br.Name,
		}
	default:
		// For broadcast data the blob reference contains the "public" (shared storage) reference, which
		// must have been allocated to this data item before sealing the batch.
		return br
	}
}

// BatchData is the fields in a data record that are assured to be consistent on all parties.
// This is what is transferred and hashed in a batch payload between nodes.
func (d *Data) BatchData(batchType BatchType) *Data {
	return &Data{
		ID:        d.ID,
		Validator: d.Validator,
		Hash:      d.Hash,
		Created:   d.Created,
		Datatype:  d.Datatype,
		Value:     d.Value,
		Blob:      d.Blob.BatchBlobRef(batchType),
		ValueSize: d.ValueSize,
	}
}

type DataAndBlob struct {
	Data *Data
	Blob *Blob
}

type DatatypeRef struct {
	Name    string `ffstruct:"DatatypeRef" json:"name,omitempty"`
	Version string `ffstruct:"DatatypeRef" json:"version,omitempty"`
}

func (dr *DatatypeRef) String() string {
	if dr == nil {
		return fftypes.NullString
	}
	return fmt.Sprintf("%s/%s", dr.Name, dr.Version)
}

type DataRefs []*DataRef

func (d DataRefs) Hash() *fftypes.Bytes32 {
	b, _ := json.Marshal(&d)
	var b32 fftypes.Bytes32 = sha256.Sum256(b)
	return &b32
}

type DataArray []*Data

func (da DataArray) Refs() DataRefs {
	dr := make(DataRefs, len(da))
	for i, d := range da {
		dr[i] = &DataRef{
			ID:        d.ID,
			Hash:      d.Hash,
			ValueSize: d.ValueSize,
		}
	}
	return dr
}

func CheckValidatorType(ctx context.Context, validator ValidatorType) error {
	switch validator {
	case ValidatorTypeJSON, ValidatorTypeNone, ValidatorTypeSystemDefinition:
		return nil
	default:
		return i18n.NewError(ctx, i18n.MsgUnknownValidatorType, validator)
	}
}

const dataSizeEstimateBase = int64(256)

func (d *Data) EstimateSize() int64 {
	// For now we have a static estimate for the size of the serialized outer structure.
	if d.ValueSize <= 0 {
		d.ValueSize = d.Value.Length()
	}
	// As long as this has been persisted, the value size will represent the length
	return dataSizeEstimateBase + d.ValueSize
}

func (d *Data) CalcHash(ctx context.Context) (*fftypes.Bytes32, error) {
	if d.Value == nil {
		d.Value = fftypes.JSONAnyPtr(fftypes.NullString)
	}
	valueIsNull := d.Value.String() == fftypes.NullString
	if valueIsNull && (d.Blob == nil || d.Blob.Hash == nil) {
		return nil, i18n.NewError(ctx, i18n.MsgDataValueIsNull)
	}
	// The hash is either the blob hash, the value hash, or if both are supplied
	// (e.g. a blob with associated metadata) it a hash of the two HEX hashes
	// concattenated together (no spaces or separation).
	switch {
	case !valueIsNull && (d.Blob == nil || d.Blob.Hash == nil):
		return d.Value.Hash(), nil
	case valueIsNull && d.Blob != nil && d.Blob.Hash != nil:
		return d.Blob.Hash, nil
	default:
		hash := sha256.New()
		hash.Write([]byte(d.Value.Hash().String()))
		hash.Write([]byte(d.Blob.Hash.String()))
		return fftypes.HashResult(hash), nil
	}
}

func (d *Data) Seal(ctx context.Context, blob *Blob) (err error) {
	if d.Validator == "" {
		d.Validator = ValidatorTypeJSON
	}
	if d.ID == nil {
		d.ID = fftypes.NewUUID()
	}
	if d.Created == nil {
		d.Created = fftypes.Now()
	}
	if blob != nil {
		if d.Blob == nil || !d.Blob.Hash.Equals(blob.Hash) {
			return i18n.NewError(ctx, i18n.MsgBlobMismatchSealingData)
		}
		d.Blob.Size = blob.Size
		if d.Value != nil {
			valJSON := d.Value.JSONObjectNowarn()
			jName := valJSON.GetString("name")
			if jName != "" {
				d.Blob.Name = jName
			} else {
				jPath := valJSON.GetString("path")
				jFilename := valJSON.GetString("filename")
				if jPath != "" && jFilename != "" {
					d.Blob.Name = fmt.Sprintf("%s/%s", jPath, jFilename)
				} else if jFilename != "" {
					d.Blob.Name = jFilename
				}
			}
		}
	} else if d.Blob != nil && d.Blob.Hash != nil {
		return i18n.NewError(ctx, i18n.MsgBlobMismatchSealingData)
	}
	if d.ValueSize <= 0 {
		d.ValueSize = d.Value.Length()
	}
	d.Hash, err = d.CalcHash(ctx)
	if err == nil {
		err = CheckValidatorType(ctx, d.Validator)
	}
	return err
}

// Path processes any non-empty "blob.name" to create a '/' prefixed/separated
// path, to the last segment of the path.
// - ""                   -> ""
// - "file.name"          -> "/"
// - "/file.name"         -> "/"
// - "path/to/file.name"  -> "/path/to"
// - "/path/to/file.name" -> "/path/to"
// The path is stored for query.
func (d *Data) CalcPath() {
	if d == nil || d.Blob == nil || d.Blob.Name == "" {
		return
	}
	segments := strings.Split(strings.TrimPrefix(d.Blob.Name, "/"), "/")
	sb := &strings.Builder{}
	sb.WriteRune('/')
	for i, segment := range segments {
		if i < len(segments)-1 {
			if i > 0 {
				sb.WriteRune('/')
			}
			sb.WriteString(segment)
		}
	}
	d.Blob.Path = sb.String()
}
