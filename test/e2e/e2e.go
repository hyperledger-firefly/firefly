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

package e2e

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/go-resty/resty/v2"
	"github.com/gorilla/websocket"
	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/test/e2e/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type TestState interface {
	T() *testing.T
	StartTime() time.Time
	Done() func()
}

var WidgetSchemaJSON = []byte(`{
	"$id": "https://example.com/widget.schema.json",
	"$schema": "https://json-schema.org/draft/2020-12/schema",
	"title": "Widget",
	"type": "object",
	"properties": {
		"id": {
			"type": "string",
			"description": "The unique identifier for the widget."
		},
		"name": {
			"type": "string",
			"description": "The person's last name."
		}
	},
	"additionalProperties": false
}`)

func PollForUp(t *testing.T, client *client.FireFlyClient) {
	var resp *resty.Response
	var err error
	for i := 0; i < 12; i++ {
		_, resp, err = client.GetStatus()
		if err == nil && resp.StatusCode() == http.StatusOK {
			break
		}
		time.Sleep(5 * time.Second)
	}
	require.NoError(t, err)
	require.Equal(t, 200, resp.StatusCode())
}

func ValidateAccountBalances(t *testing.T, client *client.FireFlyClient, poolID *fftypes.UUID, tokenIndex string, balances map[string]int64) {
	for key, balance := range balances {
		account := client.GetTokenBalance(t, poolID, tokenIndex, key)
		assert.Equal(t, balance, account.Balance.Int().Int64())
	}
}

func PickTopic(i int, options []string) string {
	return options[i%len(options)]
}

func ReadStack(t *testing.T) *Stack {
	stackDir := os.Getenv("STACK_DIR")
	if stackDir == "" {
		t.Fatal("STACK_DIR must be set")
	}
	stack, err := ReadStackFile(filepath.Join(stackDir, "stack.json"))
	assert.NoError(t, err)
	return stack
}

func ReadStackState(t *testing.T) *StackState {
	stackDir := os.Getenv("STACK_DIR")
	if stackDir == "" {
		t.Fatal("STACK_DIR must be set")
	}
	stackState, err := ReadStackStateFile(filepath.Join(stackDir, "runtime", "stackState.json"))
	assert.NoError(t, err)
	return stackState
}

func WsReader(conn *websocket.Conn) chan *core.EventDelivery {
	events := make(chan *core.EventDelivery, 100)
	go func() {
		for {
			_, b, err := conn.ReadMessage()
			if err != nil {
				fmt.Printf("Websocket %s closing, error: %s\n", conn.RemoteAddr(), err)
				return
			}
			var wsa core.WSActionBase
			err = json.Unmarshal(b, &wsa)
			if err != nil {
				panic(fmt.Errorf("invalid JSON received on WebSocket: %s", err))
			}
			var ed core.EventDelivery
			err = json.Unmarshal(b, &ed)
			if err != nil {
				panic(fmt.Errorf("invalid JSON received on WebSocket: %s", err))
			}
			if err == nil {
				fmt.Printf("Websocket %s event: %s/%s/%s -> %s (tx=%s)\n", conn.RemoteAddr(), ed.Namespace, ed.Type, ed.ID, ed.Reference, ed.Transaction)
				events <- &ed
			}
		}
	}()
	return events
}

func WaitForEvent(t *testing.T, c chan *core.EventDelivery, eventType core.EventType, ref *fftypes.UUID) {
	for {
		ed := <-c
		if ed.Type == eventType && (ref == nil || *ref == *ed.Reference) {
			t.Logf("Detected '%s' event for ref '%s'", ed.Type, ed.Reference)
			return
		}
		t.Logf("Ignored event '%s' [%s]", ed.ID, ed.Type)
	}
}

func WaitForMessageConfirmed(t *testing.T, c chan *core.EventDelivery, msgType core.MessageType) *core.EventDelivery {
	for {
		ed := <-c
		if ed.Type == core.EventTypeMessageConfirmed && ed.Message != nil && ed.Message.Header.Type == msgType {
			t.Logf("Detected '%s' event for message '%s' of type '%s'", ed.Type, ed.Message.Header.ID, msgType)
			return ed
		}
		t.Logf("Ignored event '%s' [%s]", ed.ID, ed.Type)
	}
}

func WaitForMessageRejected(t *testing.T, c chan *core.EventDelivery, msgType core.MessageType) *core.EventDelivery {
	for {
		ed := <-c
		if ed.Type == core.EventTypeMessageRejected && ed.Message != nil && ed.Message.Header.Type == msgType {
			t.Logf("Detected '%s' event for message '%s' of type '%s'", ed.Type, ed.Message.Header.ID, msgType)
			return ed
		}
		t.Logf("Ignored event '%s' [%s]", ed.ID, ed.Type)
	}
}

func WaitForIdentityConfirmed(t *testing.T, c chan *core.EventDelivery) *core.EventDelivery {
	for {
		ed := <-c
		if ed.Type == core.EventTypeIdentityConfirmed {
			t.Logf("Detected '%s' event for identity '%s'", ed.Type, ed.Reference)
			return ed
		}
		t.Logf("Ignored event '%s' [%s]", ed.ID, ed.Type)
	}
}

func WaitForContractEvent(t *testing.T, client *client.FireFlyClient, c chan *core.EventDelivery, match map[string]interface{}) map[string]interface{} {
	for {
		eventDelivery := <-c
		if eventDelivery.Type == core.EventTypeBlockchainEventReceived {
			event := client.GetBlockchainEvent(t, eventDelivery.Event.Reference.String())
			eventJSON, err := json.Marshal(&event)
			require.NoError(t, err)
			var eventMap map[string]interface{}
			err = json.Unmarshal(eventJSON, &eventMap)
			require.NoError(t, err)
			if checkObject(t, match, eventMap) {
				return eventMap
			}
		}
	}
}

func checkObject(t *testing.T, expected interface{}, actual interface{}) bool {
	match := true

	// check if this is a nested object
	expectedObject, expectedIsObject := expected.(map[string]interface{})
	actualObject, actualIsObject := actual.(map[string]interface{})

	t.Logf("Matching blockchain event: %s", fftypes.JSONObject(actualObject).String())

	// check if this is an array
	expectedArray, expectedIsArray := expected.([]interface{})
	actualArray, actualIsArray := actual.([]interface{})
	switch {
	case expectedIsObject && actualIsObject:
		for expectedKey, expectedValue := range expectedObject {
			if !checkObject(t, expectedValue, actualObject[expectedKey]) {
				return false
			}
		}
	case expectedIsArray && actualIsArray:
		for _, expectedItem := range expectedArray {
			for j, actualItem := range actualArray {
				if checkObject(t, expectedItem, actualItem) {
					break
				}
				if j == len(actualArray)-1 {
					return false
				}
			}
		}
	default:
		expectedString, expectedIsString := expected.(string)
		actualString, actualIsString := actual.(string)
		if expectedIsString && actualIsString {
			return strings.EqualFold(expectedString, actualString)
		}
		return expected == actual
	}
	return match
}

func VerifyAllOperationsSucceeded(t *testing.T, clients []*client.FireFlyClient, startTime time.Time) {
	tries := 30
	delay := 2 * time.Second

	var pending string
	for i := 0; i < tries; i++ {
		pending = ""
		for _, client := range clients {
			for _, op := range client.GetOperations(t, startTime) {
				if op.Status != core.OpStatusSucceeded {
					pending += fmt.Sprintf("Operation '%s' (%s) on '%s' status=%s\n", op.ID, op.Type, client.Client.BaseURL, op.Status)
				}
			}
		}
		if pending == "" {
			return
		}
		t.Logf("Waiting on operations:\n%s", pending)
		time.Sleep(delay)
	}

	assert.Fail(t, pending)
}

func VerifyOperationsAlreadyMarkedFailed(t *testing.T, clients []*client.FireFlyClient, startTime time.Time) {
	// Note we do NOT wait in this function - use this function when failure should already have been recorded,
	// and the work has been done in FF Core to ensure that is reflected in the operation cache.
	pending := ""
	for _, client := range clients {
		for _, op := range client.GetOperations(t, startTime) {
			if op.Status != core.OpStatusFailed {
				pending += fmt.Sprintf("Operation '%s' (%s) on '%s' status=%s\n", op.ID, op.Type, client.Client.BaseURL, op.Status)
			}
		}
	}
	assert.Empty(t, pending, pending)
}
