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

package ffdx

import (
	"encoding/json"
	"fmt"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/common/pkg/log"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/dataexchange"
)

type wsEvent struct {
	Type      msgType            `json:"type"`
	EventID   string             `json:"id"`
	Sender    string             `json:"sender"`
	Recipient string             `json:"recipient"`
	RequestID string             `json:"requestId"`
	Path      string             `json:"path"`
	Message   string             `json:"message"`
	Hash      string             `json:"hash"`
	Size      int64              `json:"size"`
	Error     string             `json:"error"`
	Manifest  string             `json:"manifest"`
	Info      fftypes.JSONObject `json:"info"`
}

type dxEvent struct {
	ffdx                *FFDX
	id                  string
	dxType              dataexchange.DXEventType
	messageReceived     *dataexchange.MessageReceived
	privateBlobReceived *dataexchange.PrivateBlobReceived
}

func (e *dxEvent) EventID() string {
	return e.id
}

func (e *dxEvent) Type() dataexchange.DXEventType {
	return e.dxType
}

func (e *dxEvent) AckWithManifest(manifest string) {
	select {
	case e.ffdx.ackChannel <- &ack{
		eventID:  e.id,
		manifest: manifest,
	}:
	case <-e.ffdx.ctx.Done():
		log.L(e.ffdx.ctx).Debugf("Ack received after close: %s", e.id)
	}
}

func (e *dxEvent) Ack() {
	e.AckWithManifest("")
}

func (e *dxEvent) MessageReceived() *dataexchange.MessageReceived {
	return e.messageReceived
}

func (e *dxEvent) PrivateBlobReceived() *dataexchange.PrivateBlobReceived {
	return e.privateBlobReceived
}

func (h *FFDX) dispatchEvent(msg *wsEvent) {
	var dataID string
	var namespace string
	var err error
	e := &dxEvent{ffdx: h, id: msg.EventID}

	switch msg.Type {
	case messageFailed:
		h.callbacks.OperationUpdate(h.ctx, &core.OperationUpdateAsync{
			OperationUpdate: core.OperationUpdate{
				Plugin:         h.Name(),
				NamespacedOpID: msg.RequestID,
				Status:         core.OpStatusFailed,
				ErrorMessage:   msg.Error,
				Output:         msg.Info,
			},
			OnComplete: e.Ack,
		})
		return
	case messageDelivered:
		status := core.OpStatusSucceeded
		if h.capabilities.Manifest {
			status = core.OpStatusPending
		}
		h.callbacks.OperationUpdate(h.ctx, &core.OperationUpdateAsync{
			OperationUpdate: core.OperationUpdate{
				Plugin:         h.Name(),
				NamespacedOpID: msg.RequestID,
				Status:         status,
				Output:         msg.Info,
			},
			OnComplete: e.Ack,
		})
		return
	case messageAcknowledged:
		h.callbacks.OperationUpdate(h.ctx, &core.OperationUpdateAsync{
			OperationUpdate: core.OperationUpdate{
				Plugin:         h.Name(),
				NamespacedOpID: msg.RequestID,
				Status:         core.OpStatusSucceeded,
				VerifyManifest: h.capabilities.Manifest,
				DXManifest:     msg.Manifest,
				Output:         msg.Info,
			},
			OnComplete: e.Ack,
		})
		return
	case blobFailed:
		h.callbacks.OperationUpdate(h.ctx, &core.OperationUpdateAsync{
			OperationUpdate: core.OperationUpdate{
				Plugin:         h.Name(),
				NamespacedOpID: msg.RequestID,
				Status:         core.OpStatusFailed,
				ErrorMessage:   msg.Error,
				Output:         msg.Info,
			},
			OnComplete: e.Ack,
		})
		return
	case blobDelivered:
		status := core.OpStatusSucceeded
		if h.capabilities.Manifest {
			status = core.OpStatusPending
		}
		h.callbacks.OperationUpdate(h.ctx, &core.OperationUpdateAsync{
			OperationUpdate: core.OperationUpdate{
				Plugin:         h.Name(),
				NamespacedOpID: msg.RequestID,
				Status:         status,
				Output:         msg.Info,
			},
			OnComplete: e.Ack,
		})
		return
	case blobAcknowledged:
		h.callbacks.OperationUpdate(h.ctx, &core.OperationUpdateAsync{
			OperationUpdate: core.OperationUpdate{
				Plugin:         h.Name(),
				NamespacedOpID: msg.RequestID,
				Status:         core.OpStatusSucceeded,
				Output:         msg.Info,
				VerifyManifest: h.capabilities.Manifest,
				DXHash:         msg.Hash,
			},
			OnComplete: e.Ack,
		})
		return

	case messageReceived:
		// De-serialize the transport wrapper
		var wrapper *core.TransportWrapper
		err = json.Unmarshal([]byte(msg.Message), &wrapper)
		switch {
		case err != nil:
			err = fmt.Errorf("invalid transmission from peer '%s': %s", msg.Sender, err)
		case wrapper.Batch == nil:
			err = fmt.Errorf("invalid transmission from peer '%s': nil batch", msg.Sender)
		default:
			namespace = wrapper.Batch.Namespace
			e.dxType = dataexchange.DXEventTypeMessageReceived
			e.messageReceived = &dataexchange.MessageReceived{
				PeerID:    msg.Sender,
				Transport: wrapper,
			}
		}

	case blobReceived:
		var hash *fftypes.Bytes32
		hash, err = fftypes.ParseBytes32(h.ctx, msg.Hash)
		if err == nil {
			_, namespace, dataID = splitBlobPath(msg.Path)
			e.dxType = dataexchange.DXEventTypePrivateBlobReceived
			e.privateBlobReceived = &dataexchange.PrivateBlobReceived{
				Namespace:  namespace,
				PeerID:     msg.Sender,
				Hash:       *hash,
				Size:       msg.Size,
				PayloadRef: msg.Path,
				DataID:     dataID,
			}
		}

	default:
		err = i18n.NewError(h.ctx, coremsgs.MsgUnexpectedDXMessageType, msg.Type)
	}

	// If we couldn't dispatch the event we received, we still ack it
	if err != nil {
		log.L(h.ctx).Warnf("Failed to dispatch DX event: %s", err)
		e.Ack()
	} else {
		// Once we're ready to dispatch it to the callback, we use a retry
		// loop because if the namespace isn't ready to consume the event
		// we need to hold onto it - not ack it (there's no nack in the protocol
		// with FFDX that allows us to push it back to the remote microservice).
		h.callbackWithRetry(namespace, msg.Recipient, e)
	}
}

func (h *FFDX) callbackWithRetry(namespace, recipient string, e *dxEvent) {
	_ = h.retry.Do(h.ctx, "dispatch ffdx event", func(attempt int) (retry bool, err error) {
		// Return until success, or the context closes.
		return true, h.callbacks.DXEvent(h.ctx, namespace, recipient, e)
	})
}
