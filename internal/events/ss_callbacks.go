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

package events

import (
	"context"
	"encoding/json"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/sharedstorage"
)

func (em *eventManager) SharedStorageBatchDownloaded(ss sharedstorage.Plugin, payloadRef string, data []byte) (*fftypes.UUID, error) {

	l := log.L(em.ctx)

	if em.multiparty == nil {
		l.Errorf("Ignoring batch from non-multiparty network!")
		return nil, nil
	}

	// De-serialize the batch
	var batch *core.Batch
	err := json.Unmarshal(data, &batch)
	if err != nil {
		l.Errorf("Invalid batch downloaded from %s '%s': %s", ss.Name(), payloadRef, err)
		return nil, nil
	}
	l.Infof("Shared storage batch downloaded from %s '%s' id=%s (len=%d)", ss.Name(), payloadRef, batch.ID, len(data))

	if batch.Namespace != em.namespace.NetworkName {
		log.L(em.ctx).Debugf("Ignoring shared storage batch from different namespace '%s'", batch.Namespace)
		return nil, nil // This is not retryable. skip this batch
	}
	batch.Namespace = em.namespace.Name

	err = em.retry.Do(em.ctx, "persist batch", func(attempt int) (bool, error) {
		err := em.database.RunAsGroup(em.ctx, func(ctx context.Context) error {
			_, _, err := em.persistBatch(ctx, batch)
			return err
		})
		if err != nil {
			return true, err
		}
		return false, nil
	})
	if err != nil {
		return nil, err
	}

	// Rewind the aggregator to this batch - after the DB updates are complete
	em.aggregator.queueBatchRewind(batch.ID)
	return batch.ID, nil
}

func (em *eventManager) SharedStorageBlobDownloaded(ss sharedstorage.Plugin, hash fftypes.Bytes32, size int64, payloadRef string, dataID *fftypes.UUID) error {
	l := log.L(em.ctx)
	l.Infof("Blob received event from public storage %s: Hash='%v'", ss.Name(), hash)

	// Dispatch to the blob receiver for efficient batch DB operations
	blobHash := hash
	em.blobReceiver.blobReceived(em.ctx, &blobNotification{
		blob: &core.Blob{
			Namespace:  em.namespace.Name,
			PayloadRef: payloadRef,
			Hash:       &blobHash,
			Size:       size,
			Created:    fftypes.Now(),
			DataID:     dataID,
		},
	})
	return nil
}
