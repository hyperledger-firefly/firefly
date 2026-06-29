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

package data

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"

	"github.com/docker/go-units"
	"github.com/hyperledger-firefly/common/pkg/ffapi"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
	"github.com/hyperledger-firefly/firefly/pkg/dataexchange"
)

type blobStore struct {
	dm       *dataManager
	database database.Plugin
	exchange dataexchange.Plugin // optional
}

func (bs *blobStore) uploadVerifyBlob(ctx context.Context, id *fftypes.UUID, reader io.Reader) (hash *fftypes.Bytes32, written int64, payloadRef string, err error) {
	hashCalc := sha256.New()
	dxReader, dx := io.Pipe()
	storeAndHash := io.MultiWriter(hashCalc, dx)

	copyDone := make(chan error, 1)
	go func() {
		var err error
		written, err = io.Copy(storeAndHash, reader)
		log.L(ctx).Debugf("Upload Blob streamed %d bytes (err=%v)", written, err)
		_ = dx.Close()
		copyDone <- err
	}()

	payloadRef, uploadHash, uploadSize, dxErr := bs.exchange.UploadBlob(ctx, bs.dm.namespace.NetworkName, *id, dxReader)
	dxReader.Close()
	copyErr := <-copyDone
	if dxErr != nil {
		return nil, -1, "", dxErr
	}
	if copyErr != nil {
		return nil, -1, "", i18n.WrapError(ctx, copyErr, coremsgs.MsgBlobStreamingFailed)
	}

	hash = fftypes.HashResult(hashCalc)
	log.L(ctx).Debugf("Upload Blob size=%d hashes: calculated=%s upload=%s (expected=%v) size=%d", written, hash, uploadHash, uploadSize, written)

	if !uploadHash.Equals(hash) {
		return nil, -1, "", i18n.NewError(ctx, coremsgs.MsgDXBadHash, uploadHash, hash)
	}
	if uploadSize > 0 && uploadSize != written {
		return nil, -1, "", i18n.NewError(ctx, coremsgs.MsgDXBadSize, uploadSize, written)
	}

	return hash, written, payloadRef, nil

}

func (bs *blobStore) UploadBlob(ctx context.Context, inData *core.DataRefOrValue, mpart *ffapi.Multipart, autoMeta bool) (*core.Data, error) {

	if bs.exchange == nil {
		return nil, i18n.NewError(ctx, coremsgs.MsgActionNotSupported)
	}

	data := &core.Data{
		ID:        fftypes.NewUUID(),
		Namespace: bs.dm.namespace.Name,
		Created:   fftypes.Now(),
		Validator: inData.Validator,
		Datatype:  inData.Datatype,
		Value:     inData.Value,
	}

	hash, blobSize, payloadRef, err := bs.uploadVerifyBlob(ctx, data.ID, mpart.Data)
	if err != nil {
		return nil, err
	}
	data.Blob = &core.BlobRef{Hash: hash}

	// autoMeta will create/update JSON metadata with the upload details
	if autoMeta {
		do := data.Value.JSONObject()
		do["filename"] = mpart.Filename
		do["mimetype"] = mpart.Mimetype
		b, _ := json.Marshal(&do)
		data.Value = fftypes.JSONAnyPtrBytes(b)
	}
	if data.Validator == "" {
		data.Validator = core.ValidatorTypeJSON
	}

	blob := &core.Blob{
		Namespace:  bs.dm.namespace.Name,
		DataID:     data.ID,
		Hash:       hash,
		Size:       blobSize,
		PayloadRef: payloadRef,
		Created:    fftypes.Now(),
	}

	err = bs.dm.checkValidation(ctx, data.Validator, data.Datatype, data.Value)
	if err == nil {
		err = data.Seal(ctx, blob)
	}
	if err != nil {
		return nil, err
	}
	log.L(ctx).Infof("Uploaded Blob blobhash=%s hash=%s (%s)", data.Blob.Hash, data.Hash, units.HumanSizeWithPrecision(float64(blobSize), 2))

	err = bs.database.RunAsGroup(ctx, func(ctx context.Context) error {
		err := bs.database.UpsertData(ctx, data, database.UpsertOptimizationNew)
		if err == nil {
			err = bs.database.InsertBlob(ctx, blob)
		}
		return err
	})
	if err != nil {
		return nil, err
	}

	return data, nil
}

func (bs *blobStore) DownloadBlob(ctx context.Context, dataID string) (*core.Blob, io.ReadCloser, error) {

	if bs.exchange == nil {
		return nil, nil, i18n.NewError(ctx, coremsgs.MsgActionNotSupported)
	}

	id, err := fftypes.ParseUUID(ctx, dataID)
	if err != nil {
		return nil, nil, err
	}

	data, err := bs.database.GetDataByID(ctx, bs.dm.namespace.Name, id, false)
	if err != nil {
		return nil, nil, err
	}
	if data == nil {
		return nil, nil, i18n.NewError(ctx, coremsgs.Msg404NoResult)
	}
	if data.Blob == nil || data.Blob.Hash == nil {
		return nil, nil, i18n.NewError(ctx, coremsgs.MsgDataDoesNotHaveBlob)
	}
	fb := database.BlobQueryFactory.NewFilter(ctx)
	blobs, _, err := bs.database.GetBlobs(ctx, bs.dm.namespace.Name, fb.And(fb.Eq("data_id", data.ID), fb.Eq("hash", data.Blob.Hash)))
	if err != nil {
		return nil, nil, err
	}
	if len(blobs) == 0 || blobs[0] == nil {
		return nil, nil, i18n.NewError(ctx, coremsgs.MsgBlobNotFound, data.Blob.Hash)
	}
	blob := blobs[0]

	reader, err := bs.exchange.DownloadBlob(ctx, blob.PayloadRef)
	return blob, reader, err
}

func (bs *blobStore) DeleteBlob(ctx context.Context, blob *core.Blob) error {
	if bs.exchange == nil {
		return i18n.NewError(ctx, coremsgs.MsgActionNotSupported)
	}

	// Compatibility check: Previous versions of FireFly could have had multiple
	// data items pointing at the same blob. We should NOT delete the blob if other
	// data items still reference this blob! Look at the payloadRef to determine
	// uniqueness, as of FireFly 1.2.x this will be unique per data item.
	fb := database.BlobQueryFactory.NewFilter(ctx)
	blobs, _, err := bs.database.GetBlobs(ctx, bs.dm.namespace.Name, fb.Eq("payloadref", blob.PayloadRef))
	if err != nil {
		return err
	}
	if len(blobs) <= 1 {

		err := bs.exchange.DeleteBlob(ctx, blob.PayloadRef)
		if err != nil {
			return err
		}
	}
	err = bs.database.DeleteBlob(ctx, blob.Sequence)
	if err != nil {
		return err
	}
	return nil
}
