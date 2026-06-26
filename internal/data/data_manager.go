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
	"fmt"
	"io"

	"github.com/hyperledger/firefly-common/pkg/config"
	"github.com/hyperledger/firefly-common/pkg/ffapi"
	"github.com/hyperledger/firefly-common/pkg/fftypes"
	"github.com/hyperledger/firefly-common/pkg/i18n"
	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly/internal/cache"
	"github.com/hyperledger/firefly/internal/coreconfig"
	"github.com/hyperledger/firefly/internal/coremsgs"
	"github.com/hyperledger/firefly/pkg/core"
	"github.com/hyperledger/firefly/pkg/database"
	"github.com/hyperledger/firefly/pkg/dataexchange"
)

type Manager interface {
	CheckDatatype(ctx context.Context, datatype *core.Datatype) error
	ValidateAll(ctx context.Context, data core.DataArray) (valid bool, err error)
	GetMessageWithDataCached(ctx context.Context, msgID *fftypes.UUID, options ...CacheReadOption) (msg *core.Message, data core.DataArray, foundAllData bool, err error)
	GetMessageDataCached(ctx context.Context, msg *core.Message, options ...CacheReadOption) (data core.DataArray, foundAll bool, err error)
	PeekMessageCache(ctx context.Context, id *fftypes.UUID, options ...CacheReadOption) (msg *core.Message, data core.DataArray)
	UpdateMessageCache(msg *core.Message, data core.DataArray)
	UpdateMessageIfCached(ctx context.Context, msg *core.Message)
	UpdateMessageStateIfCached(ctx context.Context, id *fftypes.UUID, state core.MessageState, confirmed *fftypes.FFTime, rejectReason string)
	ResolveInlineData(ctx context.Context, msg *NewMessage) error
	WriteNewMessage(ctx context.Context, newMsg *NewMessage) error
	BlobsEnabled() bool

	UploadJSON(ctx context.Context, inData *core.DataRefOrValue) (*core.Data, error)
	UploadBlob(ctx context.Context, inData *core.DataRefOrValue, blob *ffapi.Multipart, autoMeta bool) (*core.Data, error)
	DownloadBlob(ctx context.Context, dataID string) (*core.Blob, io.ReadCloser, error)
	DeleteData(ctx context.Context, dataID string) error
	HydrateBatch(ctx context.Context, persistedBatch *core.BatchPersisted) (*core.Batch, error)
	Start()
	WaitStop()
}

type dataManager struct {
	blobStore
	namespace      *core.Namespace
	database       database.Plugin
	validatorCache cache.CInterface
	messageCache   cache.CInterface
	messageWriter  *messageWriter
}

type messageCacheEntry struct {
	msg  *core.Message
	data []*core.Data
	size int64
}

func (mce *messageCacheEntry) Size() int64 {
	return mce.size
}

// Messages have fields that are mutable, in two categories
//
// 1) Can change multiple times like state - you cannot rely on the cache for these
// 2) Can go from being un-set, to being set, and once set are immutable.
// For (2) the cache provides a set of CacheReadOption modifiers that makes it safe to query the cache,
// even if the cache we slow to update asynchronously (active/active cluster being the ultimate example here,
// but from code inspection this is possible in the current cache).
type CacheReadOption int

const (
	// If you use CRORequirePublicBlobRefs then the cache will return a miss, if all data blobs do not have a `public` reference string
	CRORequirePublicBlobRefs CacheReadOption = iota
	// If you use CRORequirePins then the cache will return a miss, if the number of pins does not match the number of topics in the message.
	CRORequirePins
	// If you use CRORequestBatchID then the cache will return a miss, if there is no BatchID set.
	CRORequireBatchID
)

func NewDataManager(ctx context.Context, ns *core.Namespace, di database.Plugin, dx dataexchange.Plugin, cacheManager cache.Manager) (Manager, error) {
	if di == nil {
		return nil, i18n.NewError(ctx, coremsgs.MsgInitializationNilDepError, "DataManager")
	}
	dm := &dataManager{
		namespace: ns,
		database:  di,
	}
	dm.blobStore = blobStore{
		dm:       dm,
		database: di,
		exchange: dx,
	}

	validatorCache, err := cacheManager.GetCache(
		cache.NewCacheConfig(
			ctx,
			coreconfig.CacheValidatorSize,
			coreconfig.CacheValidatorTTL,
			ns.Name,
		),
	)
	if err != nil {
		return nil, err
	}
	dm.validatorCache = validatorCache

	messageCache, err := cacheManager.GetCache(
		cache.NewCacheConfig(
			ctx,
			coreconfig.CacheMessageSize,
			coreconfig.CacheMessageTTL,
			ns.Name,
		),
	)
	if err != nil {
		return nil, err
	}
	dm.messageCache = messageCache
	dm.messageWriter = newMessageWriter(ctx, di, &messageWriterConf{
		workerCount:  config.GetInt(coreconfig.MessageWriterCount),
		batchTimeout: config.GetDuration(coreconfig.MessageWriterBatchTimeout),
		maxInserts:   config.GetInt(coreconfig.MessageWriterBatchMaxInserts),
	})
	return dm, nil
}

func (dm *dataManager) Start() {
	dm.messageWriter.start()
}

func (dm *dataManager) BlobsEnabled() bool {
	return dm.blobStore.exchange != nil
}

func (dm *dataManager) CheckDatatype(ctx context.Context, datatype *core.Datatype) error {
	_, err := newJSONValidator(ctx, dm.namespace.Name, datatype)
	return err
}

// getValidatorForDatatype only returns database errors - not found (of all kinds) is a nil
func (dm *dataManager) getValidatorForDatatype(ctx context.Context, validator core.ValidatorType, datatypeRef *core.DatatypeRef) (Validator, error) {
	if validator == "" {
		validator = core.ValidatorTypeJSON
	}

	if datatypeRef == nil || datatypeRef.Name == "" || datatypeRef.Version == "" {
		log.L(ctx).Warnf("Invalid datatype reference '%s:%s:%s'", validator, dm.namespace.Name, datatypeRef)
		return nil, nil
	}

	key := fmt.Sprintf("%s:%s:%s", validator, dm.namespace.Name, datatypeRef)
	if cachedValue := dm.validatorCache.Get(key); cachedValue != nil {
		return cachedValue.(Validator), nil
	}

	datatype, err := dm.database.GetDatatypeByName(ctx, dm.namespace.Name, datatypeRef.Name, datatypeRef.Version)
	if err != nil {
		return nil, err
	}
	if datatype == nil {
		return nil, nil
	}
	v, err := newJSONValidator(ctx, dm.namespace.Name, datatype)
	if err != nil {
		log.L(ctx).Errorf("Invalid validator stored for '%s:%s:%s': %s", validator, dm.namespace.Name, datatypeRef, err)
		return nil, nil
	}

	dm.validatorCache.Set(key, v)
	return v, err
}

// GetMessageWithData performs a cached lookup of a message with all of the associated data.
//   - Use this in performance sensitive code, but note mutable fields like the status of the
//     message CANNOT be relied upon (due to the caching).
func (dm *dataManager) GetMessageWithDataCached(ctx context.Context, msgID *fftypes.UUID, options ...CacheReadOption) (msg *core.Message, data core.DataArray, foundAllData bool, err error) {
	if mce := dm.queryMessageCache(ctx, msgID, options...); mce != nil {
		return mce.msg, mce.data, true, nil
	}
	msg, err = dm.database.GetMessageByID(ctx, dm.namespace.Name, msgID)
	if err != nil || msg == nil {
		return nil, nil, false, err
	}
	data, foundAllData, err = dm.dataLookupAndCache(ctx, msg)
	return msg, data, foundAllData, err
}

// GetMessageDataCached looks for all the data attached to the message, including caching.
// It only returns persistence errors.
// For all cases where the data is not found (or the hashes mismatch)
func (dm *dataManager) GetMessageDataCached(ctx context.Context, msg *core.Message, options ...CacheReadOption) (data core.DataArray, foundAll bool, err error) {
	if mce := dm.queryMessageCache(ctx, msg.Header.ID, options...); mce != nil {
		return mce.data, true, nil
	}
	return dm.dataLookupAndCache(ctx, msg)
}

// cachedMessageAndDataLookup is the common function that can lookup and cache a message with its data
func (dm *dataManager) dataLookupAndCache(ctx context.Context, msg *core.Message) (data core.DataArray, foundAllData bool, err error) {
	data, foundAllData, err = dm.getMessageData(ctx, msg)
	if err != nil {
		return nil, false, err
	}
	if !foundAllData {
		return data, false, err
	}
	dm.UpdateMessageCache(msg, data)
	return data, true, nil
}

func (dm *dataManager) PeekMessageCache(ctx context.Context, id *fftypes.UUID, options ...CacheReadOption) (msg *core.Message, data core.DataArray) {
	mce := dm.queryMessageCache(ctx, id, options...)
	if mce != nil {
		return mce.msg, mce.data
	}
	return nil, nil
}

func (dm *dataManager) queryMessageCache(ctx context.Context, id *fftypes.UUID, options ...CacheReadOption) *messageCacheEntry {
	cachedValue := dm.messageCache.Get(id.String())
	if cachedValue == nil {
		log.L(context.Background()).Debugf("Cache miss for message %s", id)
		return nil
	}
	mce := cachedValue.(*messageCacheEntry)
	for _, opt := range options {
		switch opt {
		case CRORequirePublicBlobRefs:
			for idx, d := range mce.data {
				if d.Blob != nil && d.Blob.Public == "" {
					log.L(ctx).Debugf("Cache miss for message %s - data %d (%s) is missing public blob ref", mce.msg.Header.ID, idx, d.ID)
					return nil
				}
			}
		case CRORequirePins:
			if len(mce.msg.Header.Topics) != len(mce.msg.Pins) {
				log.L(ctx).Debugf("Cache miss for message %s - missing pins (topics=%d,pins=%d)", mce.msg.Header.ID, len(mce.msg.Header.Topics), len(mce.msg.Pins))
				return nil
			}
		case CRORequireBatchID:
			if mce.msg.BatchID == nil {
				log.L(ctx).Debugf("Cache miss for message %s - missing batch ID", mce.msg.Header.ID)
				return nil
			}
		}
	}
	log.L(ctx).Debugf("Cache hit for message %s", id)
	mce.msg.LocalNamespace = dm.namespace.Name // always populate LocalNamespace on the way out of the cache
	return mce
}

// UpdateMessageCache pushes an entry to the message cache. It is exposed out of the package, so that
// code which generates (or augments) message/data can populate the cache.
func (dm *dataManager) UpdateMessageCache(msg *core.Message, data core.DataArray) {
	cacheEntry := &messageCacheEntry{
		msg:  msg,
		data: data,
		size: msg.EstimateSize(true),
	}
	dm.messageCache.Set(msg.Header.ID.String(), cacheEntry)
	log.L(context.Background()).Debugf("Added to message cache: %s (topics=%d,pins=%d)", msg.Header.ID.String(), len(msg.Header.Topics), len(msg.Pins))
}

// UpdateMessageIfCached is used in order to notify the fields of a message that are not initially filled in, have been filled in.
// It does not guarantee the cache is up to date, and the CacheReadOptions should be used to check you have the updated data.
// But calling this should reduce the possibility of the CROs missing
func (dm *dataManager) UpdateMessageIfCached(ctx context.Context, msg *core.Message) {
	mce := dm.queryMessageCache(ctx, msg.Header.ID)
	if mce != nil {
		dm.UpdateMessageCache(msg, mce.data)
	}
}

func (dm *dataManager) UpdateMessageStateIfCached(ctx context.Context, id *fftypes.UUID, state core.MessageState, confirmed *fftypes.FFTime, rejectReason string) {
	mce := dm.queryMessageCache(ctx, id)
	if mce != nil {
		mce.msg.State = state
		mce.msg.Confirmed = confirmed
		mce.msg.RejectReason = rejectReason
	}
}

func (dm *dataManager) getMessageData(ctx context.Context, msg *core.Message) (data core.DataArray, foundAll bool, err error) {
	// Load all the data - must all be present for us to send
	data = make(core.DataArray, 0, len(msg.Data))
	foundAll = true
	for i, dataRef := range msg.Data {
		d, err := dm.resolveRef(ctx, dataRef)
		if err != nil {
			return nil, false, err
		}
		if d == nil {
			log.L(ctx).Warnf("Message %v data %d missing", msg.Header.ID, i)
			foundAll = false
			continue
		}
		data = append(data, d)
	}
	return data, foundAll, nil
}

func (dm *dataManager) ValidateAll(ctx context.Context, data core.DataArray) (valid bool, err error) {
	for _, d := range data {
		if d.Datatype != nil && d.Validator != core.ValidatorTypeNone {
			v, err := dm.getValidatorForDatatype(ctx, d.Validator, d.Datatype)
			if err != nil {
				return false, err
			}
			if v == nil {
				log.L(ctx).Errorf("Datatype %s:%s:%s not found", d.Validator, d.Namespace, d.Datatype)
				return false, err
			}
			err = v.ValidateValue(ctx, d.Value, d.Hash)
			if err != nil {
				return false, err
			}
		}
	}
	return true, nil
}

func (dm *dataManager) resolveRef(ctx context.Context, dataRef *core.DataRef) (*core.Data, error) {
	if dataRef == nil || dataRef.ID == nil {
		log.L(ctx).Warnf("data is nil")
		return nil, nil
	}
	d, err := dm.database.GetDataByID(ctx, dm.namespace.Name, dataRef.ID, true)
	if err != nil {
		return nil, err
	}
	switch {
	case d == nil:
		log.L(ctx).Warnf("Data %s not found", dataRef.ID)
		return nil, nil
	case d.Hash == nil || (dataRef.Hash != nil && *d.Hash != *dataRef.Hash):
		log.L(ctx).Warnf("Data hash does not match. Hash=%v Expected=%v", d.Hash, dataRef.Hash)
		return nil, nil
	default:
		return d, nil
	}
}

func (dm *dataManager) resolveBlob(ctx context.Context, namespace string, blobRef *core.BlobRef, dataID *fftypes.UUID) (*core.Blob, error) {
	if blobRef != nil && blobRef.Hash != nil {
		fb := database.BlobQueryFactory.NewFilter(ctx)
		blobs, _, err := dm.database.GetBlobs(ctx, dm.dm.namespace.Name, fb.And(fb.Eq("data_id", dataID), fb.Eq("hash", blobRef.Hash)))
		if err != nil {
			return nil, err
		}
		if len(blobs) == 0 || blobs[0] == nil {
			return nil, i18n.NewError(ctx, coremsgs.MsgBlobNotFound, blobRef.Hash)
		}
		return blobs[0], nil
	}
	return nil, nil
}

func (dm *dataManager) checkValidation(ctx context.Context, validator core.ValidatorType, datatype *core.DatatypeRef, value *fftypes.JSONAny) error {
	if validator == "" {
		validator = core.ValidatorTypeJSON
	}
	if err := core.CheckValidatorType(ctx, validator); err != nil {
		return err
	}
	// If a datatype is specified, we need to verify the payload conforms
	if datatype != nil && validator != core.ValidatorTypeNone {
		if datatype.Name == "" || datatype.Version == "" {
			return i18n.NewError(ctx, coremsgs.MsgDatatypeNotFound, datatype)
		}
		if validator != core.ValidatorTypeNone {
			v, err := dm.getValidatorForDatatype(ctx, validator, datatype)
			if err != nil {
				return err
			}
			if v == nil {
				return i18n.NewError(ctx, coremsgs.MsgDatatypeNotFound, datatype)
			}
			err = v.ValidateValue(ctx, value, nil)
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func (dm *dataManager) validateInputData(ctx context.Context, inData *core.DataRefOrValue) (data *core.Data, err error) {

	validator := inData.Validator
	datatype := inData.Datatype
	value := inData.Value
	blobRef := inData.Blob

	if err := dm.checkValidation(ctx, validator, datatype, value); err != nil {
		return nil, err
	}

	blob, err := dm.resolveBlob(ctx, dm.namespace.Name, blobRef, inData.ID)
	if err != nil {
		return nil, err
	}

	// Ok, we're good to generate the full data payload and save it
	data = &core.Data{
		Validator: validator,
		Datatype:  datatype,
		Namespace: dm.namespace.Name,
		Value:     value,
		Blob:      blobRef,
	}
	err = data.Seal(ctx, blob)
	if err != nil {
		return nil, err
	}
	return data, nil
}

func (dm *dataManager) UploadJSON(ctx context.Context, inData *core.DataRefOrValue) (*core.Data, error) {
	data, err := dm.validateInputData(ctx, inData)
	if err != nil {
		return nil, err
	}
	if err = dm.messageWriter.WriteData(ctx, data); err != nil {
		return nil, err
	}
	return data, err
}

// ResolveInlineData processes an input message that is going to be stored, to see which of the data
// elements are new, and which are existing. It verifies everything that points to an existing
// reference, and returns a list of what data is new separately - so that it can be stored by the
// message writer when the sending code is ready.
func (dm *dataManager) ResolveInlineData(ctx context.Context, newMessage *NewMessage) (err error) {

	if newMessage.Message == nil {
		return i18n.NewError(ctx, i18n.MsgNilOrNullObject)
	}

	inData := newMessage.Message.InlineData
	newMessage.AllData = make(core.DataArray, len(newMessage.Message.InlineData))
	for i, dataOrValue := range inData {
		var d *core.Data
		switch {
		case dataOrValue.ID != nil:
			// If an ID is supplied, then it must be a reference to existing data
			d, err = dm.resolveRef(ctx, &dataOrValue.DataRef)
			if err != nil {
				return err
			}
			if d == nil {
				return i18n.NewError(ctx, coremsgs.MsgDataReferenceUnresolvable, i)
			}
			if _, err = dm.resolveBlob(ctx, dm.namespace.Name, d.Blob, d.ID); err != nil {
				return err
			}
		case dataOrValue.Value != nil || dataOrValue.Blob != nil:
			// We've got a Value, so we can validate + store it
			if d, err = dm.validateInputData(ctx, dataOrValue); err != nil {
				return err
			}
			newMessage.NewData = append(newMessage.NewData, d)
		default:
			// We have nothing - this must be a mistake
			return i18n.NewError(ctx, coremsgs.MsgDataMissing, i)
		}
		newMessage.AllData[i] = d

	}
	newMessage.Message.Data = newMessage.AllData.Refs()
	return nil
}

// HydrateBatch fetches the full messages for a persisted batch, ready for transmission
func (dm *dataManager) HydrateBatch(ctx context.Context, persistedBatch *core.BatchPersisted) (*core.Batch, error) {

	var manifest core.BatchManifest
	err := persistedBatch.Manifest.Unmarshal(ctx, &manifest)
	if err != nil {
		return nil, i18n.WrapError(ctx, err, i18n.MsgJSONObjectParseFailed, fmt.Sprintf("batch %s manifest", persistedBatch.ID))
	}

	batch := persistedBatch.GenInflight(make([]*core.Message, len(manifest.Messages)), make(core.DataArray, len(manifest.Data)))

	for i, mr := range manifest.Messages {
		m, err := dm.database.GetMessageByID(ctx, dm.namespace.Name, mr.ID)
		if err != nil || m == nil {
			return nil, i18n.WrapError(ctx, err, coremsgs.MsgFailedToRetrieve, "message", mr.ID)
		}
		// BatchMessage removes any fields that could change after the batch was first assembled on the sender
		batch.Payload.Messages[i] = m.BatchMessage()
	}
	for i, dr := range manifest.Data {
		d, err := dm.database.GetDataByID(ctx, dm.namespace.Name, dr.ID, true)
		if err != nil || d == nil {
			return nil, i18n.WrapError(ctx, err, coremsgs.MsgFailedToRetrieve, "data", dr.ID)
		}
		// BatchData removes any fields that could change after the batch was first assembled on the sender
		batch.Payload.Data[i] = d.BatchData(persistedBatch.Type)
	}

	return batch, nil
}

// WriteNewMessage dispatches the writing of the message and assocated data, then blocks until the background
// worker (or foreground if no DB concurrency) has written. The caller MUST NOT call this inside of a
// DB RunAsGroup - because if a large number of routines enter the same function they could starve the background
// worker of the spare connection required to execute (and thus deadlock).
func (dm *dataManager) WriteNewMessage(ctx context.Context, newMsg *NewMessage) error {

	if newMsg.Message == nil {
		return i18n.NewError(ctx, i18n.MsgNilOrNullObject)
	}

	// We add the message to the cache before we write it, because the batch aggregator might
	// pick up our message from the message-writer before we return. The batch processor
	// writes a more authoritative cache entry, with pings/batchID etc.
	dm.UpdateMessageCache(&newMsg.Message.Message, newMsg.AllData)

	err := dm.messageWriter.WriteNewMessage(ctx, newMsg)
	if err != nil {
		return err
	}
	return nil
}

func (dm *dataManager) WaitStop() {
	dm.messageWriter.close()
}

func (dm *dataManager) DeleteData(ctx context.Context, dataID string) error {
	id, err := fftypes.ParseUUID(ctx, dataID)
	if err != nil {
		return err
	}

	data, err := dm.database.GetDataByID(ctx, dm.namespace.Name, id, false)
	if err != nil {
		return err
	}
	if data == nil {
		return i18n.NewError(ctx, coremsgs.Msg404NoResult)
	}
	if data.Blob != nil && data.Blob.Hash != nil {
		fb := database.BlobQueryFactory.NewFilter(ctx)
		blobs, _, err := dm.database.GetBlobs(ctx, dm.namespace.Name, fb.And(fb.Eq("data_id", data.ID), fb.Eq("hash", data.Blob.Hash)))
		if err != nil {
			return err
		}
		for _, blob := range blobs {
			if blob != nil {
				err = dm.DeleteBlob(ctx, blob)
				if err != nil {
					return err
				}
			}
		}
	}

	// Invalidate cache entries for any messages that had these data refs
	msgs, _, err := dm.database.GetMessagesForData(ctx, dm.namespace.Name, data.ID, database.MessageQueryFactory.NewFilter(ctx).And())
	if err != nil {
		return err
	}
	for _, msg := range msgs {
		dm.messageCache.Set(msg.Header.ID.String(), nil)
	}

	return dm.database.DeleteData(ctx, data.Namespace, data.ID)
}
