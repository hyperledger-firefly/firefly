// Copyright © 2025 Kaleido, Inc.
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

package shareddownload

import (
	"context"
	"fmt"

	"github.com/hyperledger/firefly-common/pkg/log"
	"github.com/hyperledger/firefly/pkg/core"
)

type downloadWorker struct {
	ctx  context.Context
	done chan struct{}
	dm   *downloadManager
}

func newDownloadWorker(dm *downloadManager, idx int) *downloadWorker {
	dw := &downloadWorker{
		ctx:  log.WithLogField(dm.ctx, "downloadworker", fmt.Sprintf("dw_%.3d", idx)),
		done: make(chan struct{}),
		dm:   dm,
	}
	go dw.downloadWorkerLoop()
	return dw
}

func (dw *downloadWorker) downloadWorkerLoop() {
	defer close(dw.done)

	l := log.L(dw.ctx)
	for {
		select {
		case <-dw.ctx.Done():
			l.Debugf("Download worker shutting down")
			return
		case work := <-dw.dm.work:
			dw.attemptWork(work)
		}
	}
}

func (dw *downloadWorker) attemptWork(work *downloadWork) {
	attempts := int(work.attempts.Add(1))
	isLastAttempt := attempts >= dw.dm.retryMaxAttempts
	_, err := dw.dm.operations.RunOperation(dw.ctx, work.preparedOp, work.idempotentSubmit)
	if err != nil {
		log.L(dw.ctx).Errorf("Download operation %s/%s attempt=%d/%d failed: %s", work.preparedOp.Type, work.preparedOp.ID, attempts, dw.dm.retryMaxAttempts, err)
		if isLastAttempt {
			dw.dm.operations.SubmitOperationUpdate(&core.OperationUpdateAsync{
				OperationUpdate: core.OperationUpdate{
					NamespacedOpID: work.preparedOp.NamespacedIDString(),
					Plugin:         work.preparedOp.Plugin,
					Status:         core.OpStatusFailed,
					ErrorMessage:   err.Error(),
				},
			})
		} else {
			go dw.dm.waitAndRetryDownload(work)
		}
	}
}
