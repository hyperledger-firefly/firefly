// Copyright © 2022 Kaleido, Inc.
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
	"fmt"
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/stretchr/testify/mock"
)

func TestBlobReceiverBackgroundDispatchOK(t *testing.T) {

	em := newTestEventManagerWithDBConcurrency(t)
	defer em.cleanup(t)
	em.blobReceiver.start()

	em.mdi.On("GetBlobs", mock.Anything, mock.Anything, mock.Anything).Return([]*core.Blob{}, nil, nil)
	em.mdi.On("InsertBlobs", mock.Anything, mock.Anything).Return(nil, nil)

	blobHash := fftypes.NewRandB32()
	done := make(chan struct{})
	em.blobReceiver.blobReceived(em.ctx, &blobNotification{
		blob: &core.Blob{
			Hash: blobHash,
		},
	})
	em.blobReceiver.blobReceived(em.ctx, &blobNotification{
		blob: &core.Blob{
			Hash: blobHash, // de-dup'd
		},
		onComplete: func() {
			close(done)
		},
	})
	<-done

	em.blobReceiver.stop()

}

func TestBlobReceiverBackgroundDispatchCancelled(t *testing.T) {

	em := newTestEventManagerWithDBConcurrency(t)
	defer em.cleanup(t)
	em.cancel()
	em.blobReceiver.start()

	em.blobReceiver.blobReceived(em.ctx, &blobNotification{
		blob: &core.Blob{
			Hash: fftypes.NewRandB32(),
		},
	})
	em.blobReceiver.stop()

}

func TestBlobReceiverBackgroundDispatchFail(t *testing.T) {

	em := newTestEventManagerWithDBConcurrency(t)
	defer em.cleanup(t)
	em.blobReceiver.start()

	done := make(chan struct{})
	em.mdi.On("GetBlobs", mock.Anything, mock.Anything, mock.Anything).Return(nil, nil, fmt.Errorf("pop")).Run(func(args mock.Arguments) {
		em.cancel()
		close(done)
	})

	em.blobReceiver.blobReceived(em.ctx, &blobNotification{
		blob: &core.Blob{
			Hash: fftypes.NewRandB32(),
		},
	})
	<-done

	em.blobReceiver.stop()

}

func TestBlobReceiverDispatchDup(t *testing.T) {

	em := newTestEventManager(t)
	defer em.cleanup(t)

	blobHash := fftypes.NewRandB32()

	em.mdi.On("GetBlobs", mock.Anything, mock.Anything, mock.Anything).Return([]*core.Blob{
		{Hash: blobHash, PayloadRef: "payload1"},
	}, nil, nil)

	em.blobReceiver.blobReceived(em.ctx, &blobNotification{
		blob: &core.Blob{
			Hash:       blobHash,
			PayloadRef: "payload1",
		},
	})

}
