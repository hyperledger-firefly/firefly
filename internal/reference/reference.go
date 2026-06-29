// Copyright © 2024 Kaleido, Inc.
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

package reference

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
)

type TypeReferenceDoc struct {
	Example           []byte
	Description       []byte
	FieldDescriptions []byte
	SubFieldTables    []byte
}

/*
 * This function generates a series of markdown pages to document FireFly types, and are
 * designed to be included in the docs. Each page is a []byte value in the map, and the
 * key is the file name of the page. To add additional pages, simply create an example
 * instance of the type you would like to document, then include that in the `types`
 * array which is passed to generateMarkdownPages(). Note: It is the responsibility of
 * some other caller function to actually write the bytes to disk.
 */
func GenerateObjectsReferenceMarkdown(ctx context.Context) (map[string][]byte, error) {

	newest := core.SubOptsFirstEventNewest
	fifty := uint(50)
	falseVar := false

	types := []interface{}{

		&core.Event{
			ID:          fftypes.MustParseUUID("5f875824-b36b-4559-9791-a57a2e2b30dd"),
			Sequence:    168,
			Type:        core.EventTypeTransactionSubmitted,
			Namespace:   "ns1",
			Reference:   fftypes.MustParseUUID("0d12aa75-5ed8-48a7-8b54-45274c6edcb1"),
			Transaction: fftypes.MustParseUUID("0d12aa75-5ed8-48a7-8b54-45274c6edcb1"),
			Topic:       core.TransactionTypeBatchPin.String(),
			Created:     fftypes.UnixTime(1652664195),
		},

		&core.Subscription{
			SubscriptionRef: core.SubscriptionRef{
				ID:        fftypes.MustParseUUID("c38d69fd-442e-4d6f-b5a4-bab1411c7fe8"),
				Namespace: "ns1",
				Name:      "app1",
			},
			Transport: "websockets",
			Filter: core.SubscriptionFilter{
				Events: "^(message_.*|token_.*)$",
				Message: core.MessageFilter{
					Tag: "^(red|blue)$",
				},
			},
			Options: core.SubscriptionOptions{
				SubscriptionCoreOptions: core.SubscriptionCoreOptions{
					FirstEvent: &newest,
					ReadAhead:  &fifty,
				},
			},
			Created: fftypes.UnixTime(1652664195),
		},

		&core.ContractAPI{
			ID:        fftypes.MustParseUUID("0f12317b-85a0-4a77-a722-857ea2b0a5fa"),
			Name:      "my_contract_api",
			Namespace: "ns1",
			Interface: &fftypes.FFIReference{
				ID: fftypes.MustParseUUID("c35d3449-4f24-4676-8e64-91c9e46f06c4"),
			},
			Location: fftypes.JSONAnyPtr(`{
				"address": "0x95a6c4895c7806499ba35f75069198f45e88fc69"
			}`),
			Message: fftypes.MustParseUUID("b09d9f77-7b16-4760-a8d7-0e3c319b2a16"),
			URLs: core.ContractURLs{
				API:     "http://127.0.0.1:5000/api/v1/namespaces/default/apis/my_contract_api",
				OpenAPI: "http://127.0.0.1:5000/api/v1/namespaces/default/apis/my_contract_api/api/swagger.json",
				UI:      "http://127.0.0.1:5000/api/v1/namespaces/default/apis/my_contract_api/api",
			},
		},

		&core.BlockchainEvent{
			ID:         fftypes.MustParseUUID("e9bc4735-a332-4071-9975-b1066e51ab8b"),
			Source:     "ethereum",
			Namespace:  "ns1",
			Name:       "MyEvent",
			Listener:   fftypes.MustParseUUID("c29b4595-03c2-411a-89e3-8b7f27ef17bb"),
			ProtocolID: "000000000048/000000/000000",
			Output: fftypes.JSONObject{
				"addr1":  "0x55860105d6a675dbe6e4d83f67b834377ba677ad",
				"value2": "42",
			},
			Info: fftypes.JSONObject{
				"address":          "0x57A9bE18CCB50D06B7567012AaF6031D669BBcAA",
				"blockHash":        "0xae7382ef2573553f517913b927d8b9691ada8d617266b8b16f74bb37aa78cae8",
				"blockNumber":      "48",
				"logIndex":         "0",
				"signature":        "Changed(address,uint256)",
				"subId":            "sb-e4d5efcd-2eba-4ed1-43e8-24831353fffc",
				"timestamp":        "1653048837",
				"transactionHash":  "0x34b0327567fefed09ac7b4429549bc609302b08a9cbd8f019a078ec44447593d",
				"transactionIndex": "0x0",
			},
			Timestamp: fftypes.UnixTime(1652664195),
			TX: core.BlockchainTransactionRef{
				BlockchainID: "0x34b0327567fefed09ac7b4429549bc609302b08a9cbd8f019a078ec44447593d",
			},
		},

		&core.Transaction{
			ID:            fftypes.MustParseUUID("4e7e0943-4230-4f67-89b6-181adf471edc"),
			Namespace:     "ns1",
			Type:          core.TransactionTypeContractInvoke,
			Created:       fftypes.UnixTime(1652664195),
			BlockchainIDs: fftypes.NewFFStringArray("0x34b0327567fefed09ac7b4429549bc609302b08a9cbd8f019a078ec44447593d"),
		},

		&core.Operation{
			ID:          fftypes.MustParseUUID("04a8b0c4-03c2-4935-85a1-87d17cddc20a"),
			Namespace:   "ns1",
			Type:        core.OpTypeSharedStorageUploadBatch,
			Transaction: fftypes.MustParseUUID("99543134-769b-42a8-8be4-a5f8873f969d"),
			Status:      core.OpStatusSucceeded,
			Plugin:      "ipfs",
			Input: fftypes.JSONObject{
				"id": "80d89712-57f3-48fe-b085-a8cba6e0667d",
			},
			Output: fftypes.JSONObject{
				"payloadRef": "QmWj3tr2aTHqnRYovhS2mQAjYneRtMWJSU4M4RdAJpJwEC",
			},
			Created: fftypes.UnixTime(1652664195),
		},

		&core.OperationWithDetail{
			Operation: core.Operation{
				ID:          fftypes.MustParseUUID("04a8b0c4-03c2-4935-85a1-87d17cddc20a"),
				Namespace:   "ns1",
				Type:        core.OpTypeSharedStorageUploadBatch,
				Transaction: fftypes.MustParseUUID("99543134-769b-42a8-8be4-a5f8873f969d"),
				Status:      core.OpStatusSucceeded,
				Plugin:      "ipfs",
				Input: fftypes.JSONObject{
					"id": "80d89712-57f3-48fe-b085-a8cba6e0667d",
				},
				Output: fftypes.JSONObject{
					"payloadRef": "QmWj3tr2aTHqnRYovhS2mQAjYneRtMWJSU4M4RdAJpJwEC",
				},
				Created: fftypes.UnixTime(1652664195),
			},
			Detail: fftypes.JSONObject{
				"status":          "Succeeded",
				"sequenceId":      "0185f42f-fec8-93df-aeba-387417d477e0",
				"created":         "2023-01-27T17:04:24.26406392Z",
				"firstSubmit":     "2023-01-27T17:04:24.419913295Z",
				"gas":             "4161076",
				"gasPrice":        "0",
				"transactionHash": "0xfb39178fee8e725c03647b8286e6f5cb13f982abf685479a9ee59e8e9d9e51d8",
				"history": []*fftypes.JSONObject{
					{
						"subStatus": "Received",
						"time":      "2023-01-27T17:11:41.122965803Z",
						"actions": []*fftypes.JSONObject{
							{
								"action":         "AssignNonce",
								"count":          1,
								"time":           "",
								"lastOccurrence": "",
							},
							{
								"action":         "RetrieveGasPrice",
								"count":          1,
								"time":           "2023-01-27T17:11:41.161213303Z",
								"lastOccurrence": "2023-01-27T17:11:41.161213303Z",
							},
							{
								"action":         "Submit",
								"count":          1,
								"time":           "2023-01-27T17:11:41.222374636Z",
								"lastOccurrence": "2023-01-27T17:11:41.222374636Z",
							},
						},
					},
					{
						"subStatus": "Tracking",
						"time":      "2023-01-27T17:11:41.222400219Z",
						"actions": []*fftypes.JSONObject{
							{
								"action":         "ReceiveReceipt",
								"count":          1,
								"time":           "2023-01-27T17:11:47.930332625Z",
								"lastOccurrence": "2023-01-27T17:11:47.930332625Z",
							},
							{
								"action":         "Confirm",
								"count":          1,
								"time":           "2023-01-27T17:12:02.660275549Z",
								"lastOccurrence": "2023-01-27T17:12:02.660275549Z",
							},
						},
					},
					{
						"subStatus": "Confirmed",
						"time":      "2023-01-27T17:12:02.660309382Z",
						"actions":   []*fftypes.JSONObject{},
					},
				},
				"historySummary": []*fftypes.JSONObject{
					{
						"subStatus": "Received",
						"count":     1,
					},
					{
						"action": "AssignNonce",
						"count":  1,
					},
					{
						"action": "RetrieveGasPrice",
						"count":  1,
					},
					{
						"action": "Submit",
						"count":  1,
					},
					{
						"subStatus": "Tracking",
						"count":     1,
					},
					{
						"action": "ReceiveReceipt",
						"count":  1,
					},
					{
						"action": "Confirm",
						"count":  1,
					},
					{
						"subStatus": "Confirmed",
						"count":     1,
					},
				},
			},
		},

		&fftypes.FFI{
			ID:          fftypes.MustParseUUID("c35d3449-4f24-4676-8e64-91c9e46f06c4"),
			Namespace:   "ns1",
			Name:        "SimpleStorage",
			Description: "A simple example contract in Solidity",
			Version:     "v0.0.1",
			Message:     fftypes.MustParseUUID("e4ad2077-5714-416e-81f9-7964a6223b6f"),
			Methods: []*fftypes.FFIMethod{
				{
					ID:          fftypes.MustParseUUID("8f3289dd-3a19-4a9f-aab3-cb05289b013c"),
					Interface:   fftypes.MustParseUUID("c35d3449-4f24-4676-8e64-91c9e46f06c4"),
					Name:        "get",
					Namespace:   "ns1",
					Pathname:    "get",
					Description: "Get the current value",
					Params:      fftypes.FFIParams{},
					Returns: fftypes.FFIParams{
						{
							Name: "output",
							Schema: fftypes.JSONAnyPtr(`{
								"type": "integer",
								"details": {
								  "type": "uint256"
								}
							}`),
						},
					},
					Details: fftypes.JSONObject{
						"stateMutability": "viewable",
					},
				},
				{
					ID:          fftypes.MustParseUUID("fc6f54ee-2e3c-4e56-b17c-4a1a0ae7394b"),
					Interface:   fftypes.MustParseUUID("c35d3449-4f24-4676-8e64-91c9e46f06c4"),
					Name:        "set",
					Namespace:   "ns1",
					Pathname:    "set",
					Description: "Set the value",
					Params: fftypes.FFIParams{
						{
							Name: "newValue",
							Schema: fftypes.JSONAnyPtr(`{
								"type": "integer",
								"details": {
								  "type": "uint256"
								}
							}`),
						},
					},
					Returns: fftypes.FFIParams{},
					Details: fftypes.JSONObject{
						"stateMutability": "payable",
					},
				},
			},
			Events: []*fftypes.FFIEvent{
				{
					ID:        fftypes.MustParseUUID("9f653f93-86f4-45bc-be75-d7f5888fbbc0"),
					Interface: fftypes.MustParseUUID("c35d3449-4f24-4676-8e64-91c9e46f06c4"),
					Namespace: "ns1",
					Pathname:  "Changed",
					Signature: "Changed(address,uint256)",
					FFIEventDefinition: fftypes.FFIEventDefinition{
						Name:        "Changed",
						Description: "Emitted when the value changes",
						Params: fftypes.FFIParams{
							{
								Name: "_from",
								Schema: fftypes.JSONAnyPtr(`{
									"type": "string",
									"details": {
									  "type": "address",
									  "indexed": true
									}
								}`),
							},
							{
								Name: "_value",
								Schema: fftypes.JSONAnyPtr(`{
									"type": "integer",
									"details": {
									  "type": "uint256"
									}
								}`),
							},
						},
						Details: fftypes.JSONObject{},
					},
				},
			},
		},

		&core.ContractListener{
			ID: fftypes.MustParseUUID("d61980a9-748c-4c72-baf5-8b485b514d59"),
			Interface: &fftypes.FFIReference{
				ID: fftypes.MustParseUUID("ff1da3c1-f9e7-40c2-8d93-abb8855e8a1d"),
			},
			Namespace: "ns1",
			Name:      "contract1_events",
			BackendID: "sb-dd8795fc-a004-4554-669d-c0cf1ee2c279",
			Location: fftypes.JSONAnyPtr(`{
				"address": "0x596003a91a97757ef1916c8d6c0d42592630d2cf"
			}`),
			Created: fftypes.UnixTime(1652664195),
			Event: &core.FFISerializedEvent{
				FFIEventDefinition: fftypes.FFIEventDefinition{
					Name: "Changed",
					Params: fftypes.FFIParams{
						{
							Name: "x",
							Schema: fftypes.JSONAnyPtr(`{
								"type": "integer",
								"details": {
								  "type": "uint256",
								  "internalType": "uint256"
								}
							}`),
						},
					},
				},
			},
			Signature: "0x596003a91a97757ef1916c8d6c0d42592630d2cf:Changed(uint256)",
			Topic:     "app1_topic",
			Options: &core.ContractListenerOptions{
				FirstEvent: "newest",
			},
			Filters: core.ListenerFilters{
				{
					Event: &core.FFISerializedEvent{
						FFIEventDefinition: fftypes.FFIEventDefinition{
							Name: "Changed",
							Params: fftypes.FFIParams{
								{
									Name: "x",
									Schema: fftypes.JSONAnyPtr(`{
										"type": "integer",
										"details": {
										"type": "uint256",
										"internalType": "uint256"
										}
									}`),
								},
							},
						},
					},
					Signature: "0x596003a91a97757ef1916c8d6c0d42592630d2cf:Changed(uint256)",
					Location: fftypes.JSONAnyPtr(`{
						"address": "0x596003a91a97757ef1916c8d6c0d42592630d2cf"
					}`),
				},
			},
		},

		&core.TokenPool{
			ID:        fftypes.MustParseUUID("90ebefdf-4230-48a5-9d07-c59751545859"),
			Type:      core.TokenTypeFungible,
			Namespace: "ns1",
			Name:      "my_token",
			Standard:  "ERC-20",
			Locator:   "address=0x056df1c53c3c00b0e13d37543f46930b42f71db0&schema=ERC20WithData&type=fungible",
			Decimals:  18,
			Connector: "erc20_erc721",
			Active:    true,
			Message:   fftypes.MustParseUUID("43923040-b1e5-4164-aa20-47636c7177ee"),
			Info: fftypes.JSONObject{
				"address": "0x056df1c53c3c00b0e13d37543f46930b42f71db0",
				"name":    "pool8197",
				"schema":  "ERC20WithData",
			},
			TX: core.TransactionRef{
				Type: core.TransactionTypeTokenPool,
				ID:   fftypes.MustParseUUID("a23ffc87-81a2-4cbc-97d6-f53d320c36cd"),
			},
			Created: fftypes.UnixTime(1652664195),
		},

		&core.TokenTransfer{
			Namespace:       "ns1",
			Connector:       "erc20_erc721",
			URI:             "firefly://token/1",
			Type:            core.TokenTransferTypeTransfer,
			Key:             "0x55860105D6A675dBE6e4d83F67b834377Ba677AD",
			From:            "0x55860105D6A675dBE6e4d83F67b834377Ba677AD",
			To:              "0x55860105D6A675dBE6e4d83F67b834377Ba677AD",
			Amount:          *fftypes.NewFFBigInt(1000000000000000000),
			ProtocolID:      "000000000041/000000/000000",
			Message:         fftypes.MustParseUUID("780b9b90-e3b0-4510-afac-b4b1f2940b36"),
			MessageHash:     fftypes.MustParseBytes32("780204e634364c42779920eddc8d9fecccb33e3607eeac9f53abd1b31184ae4e"),
			Pool:            fftypes.MustParseUUID("1244ecbe-5862-41c3-99ec-4666a18b9dd5"),
			BlockchainEvent: fftypes.MustParseUUID("b57fcaa2-156e-4c3f-9b0b-ddec9ee25933"),
			TX: core.TransactionRef{
				Type: core.TransactionTypeTokenTransfer,
				ID:   fftypes.MustParseUUID("62767ca8-99f9-439c-9deb-d80c6672c158"),
			},
			Created: fftypes.UnixTime(1652664195),
		},

		&core.TokenApproval{
			Namespace: "ns1",
			Connector: "erc20_erc721",
			LocalID:   fftypes.MustParseUUID("1CD3E2E2-DD6A-441D-94C5-02439DE9897B"),
			Pool:      fftypes.MustParseUUID("1244ecbe-5862-41c3-99ec-4666a18b9dd5"),
			Key:       "0x55860105d6a675dbe6e4d83f67b834377ba677ad",
			Operator:  "0x30017fd084715e41aa6536ab777a8f3a2b11a5a1",
			Approved:  true,
			Info: fftypes.JSONObject{
				"owner":   "0x55860105d6a675dbe6e4d83f67b834377ba677ad",
				"spender": "0x30017fd084715e41aa6536ab777a8f3a2b11a5a1",
				"value":   "115792089237316195423570985008687907853269984665640564039457584007913129639935",
			},
			ProtocolID: "000000000032/000000/000000",
			Subject:    "0x55860105d6a675dbe6e4d83f67b834377ba677ad:0x30017fd084715e41aa6536ab777a8f3a2b11a5a1",
			Active:     true,
			TX: core.TransactionRef{
				Type: core.TransactionTypeTokenApproval,
				ID:   fftypes.MustParseUUID("4b6e086d-0e31-482d-9683-cd18b2045031"),
			},
			Created: fftypes.UnixTime(1652664195),
		},

		&core.Identity{
			IdentityBase: core.IdentityBase{
				ID:        fftypes.MustParseUUID("114f5857-9983-46fb-b1fc-8c8f0a20846c"),
				DID:       "did:firefly:org/org_1",
				Parent:    fftypes.MustParseUUID("688072c3-4fa0-436c-a86b-5d89673b8938"),
				Type:      core.IdentityTypeOrg,
				Namespace: "ff_system",
				Name:      "org_1",
			},
			Messages: core.IdentityMessages{
				Claim:        fftypes.MustParseUUID("911b364b-5863-4e49-a3f8-766dbbae7c4c"),
				Verification: fftypes.MustParseUUID("24636f11-c1f9-4bbb-9874-04dd24c7502f"),
			},
			Created: fftypes.UnixTime(1652664195),
		},

		&core.Verifier{
			Identity: fftypes.MustParseUUID("114f5857-9983-46fb-b1fc-8c8f0a20846c"),
			Hash:     fftypes.MustParseBytes32("6818c41093590b862b781082d4df5d4abda6d2a4b71d737779edf6d2375d810b"),
			VerifierRef: core.VerifierRef{
				Type:  core.VerifierTypeEthAddress,
				Value: "0x30017fd084715e41aa6536ab777a8f3a2b11a5a1",
			},
			Created: fftypes.UnixTime(1652664195),
		},

		&core.Message{
			LocalNamespace: "ns1",
			Header: core.MessageHeader{
				ID:     fftypes.MustParseUUID("4ea27cce-a103-4187-b318-f7b20fd87bf3"),
				Type:   core.MessageTypePrivate,
				CID:    fftypes.MustParseUUID("00d20cba-76ed-431d-b9ff-f04b4cbee55c"),
				TxType: core.TransactionTypeBatchPin,
				SignerRef: core.SignerRef{
					Author: "did:firefly:org/acme",
					Key:    "0xD53B0294B6a596D404809b1d51D1b4B3d1aD4945",
				},
				Created:   fftypes.UnixTime(1652664190),
				Group:     fftypes.HashString("testgroup"),
				Namespace: "ns1",
				Topics:    fftypes.NewFFStringArray("topic1"),
				Tag:       "blue_message",
				DataHash:  fftypes.HashString("testmsghash"),
			},
			Data: []*core.DataRef{
				{
					ID:   fftypes.MustParseUUID("fdf9f118-eb81-4086-a63d-b06715b3bb4e"),
					Hash: fftypes.HashString("refhash"),
				},
			},
			State:     core.MessageStateConfirmed,
			Confirmed: fftypes.UnixTime(1652664196),
		},

		&core.Data{
			ID:        fftypes.MustParseUUID("4f11e022-01f4-4c3f-909f-5226947d9ef0"),
			Validator: core.ValidatorTypeJSON,
			Namespace: "ns1",
			Created:   fftypes.UnixTime(1652664195),
			Hash:      fftypes.HashString("testdatahash"),
			Datatype: &core.DatatypeRef{
				Name:    "widget",
				Version: "v1.2.3",
			},
			Value: fftypes.JSONAnyPtr(`{
				"name": "filename.pdf",
				"a": "example",
				"b": { "c": 12345 }
			}`),
			Blob: &core.BlobRef{
				Hash: fftypes.HashString("testblobhash"),
				Size: 12345,
				Name: "filename.pdf",
			},
		},

		&core.Datatype{
			ID:        fftypes.MustParseUUID("3a479f7e-ddda-4bda-aa24-56d06c0bf08e"),
			Message:   fftypes.MustParseUUID("bfcf904c-bdf7-40aa-bbd7-567f625c26c0"),
			Validator: core.ValidatorTypeJSON,
			Namespace: "ns1",
			Name:      "widget",
			Version:   "1.0.0",
			Hash:      fftypes.MustParseBytes32("639cd98c893fa45a9df6fd87bd0393a9b39e31e26fbb1eeefe90cb40c3fa02d2"),
			Created:   fftypes.UnixTime(1652664196),
			Value: fftypes.JSONAnyPtr(`{
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
			}`),
		},

		&core.Group{
			Hash: fftypes.HashString("testgrouphash"),
			GroupIdentity: core.GroupIdentity{
				Namespace: "ns1",
				Members: core.Members{
					{
						Identity: "did:firefly:org/1111",
						Node:     fftypes.MustParseUUID("4f563179-b4bd-4161-86e0-c2c1c0869c4f"),
					},
					{
						Identity: "did:firefly:org/2222",
						Node:     fftypes.MustParseUUID("61a99af8-c1f7-48ea-8fcc-489e4822a0ed"),
					},
				},
			},
			LocalNamespace: "ns1",
			Message:        fftypes.MustParseUUID("0b9dfb76-103d-443d-92fd-b114fe07c54d"),
			Created:        fftypes.UnixTime(1652664196),
		},

		&core.NextPin{
			Namespace: "ns1",
			Context:   fftypes.HashString("testnextpincontext"),
			Identity:  "did:firefly:org/example",
			Hash:      fftypes.HashString("testnextpinhash"),
			Nonce:     12345,
		},

		&core.Batch{
			BatchHeader: core.BatchHeader{
				ID:        fftypes.MustParseUUID("894bc0ea-0c2e-4ca4-bbca-b4c39a816bbb"),
				Type:      core.BatchTypePrivate,
				Namespace: "ns1",
				Node:      fftypes.MustParseUUID("5802ab80-fa71-4f52-9189-fb534de93756"),
				Group:     fftypes.HashString("examplegroup"),
				Created:   fftypes.UnixTime(1652664196),
				SignerRef: core.SignerRef{
					Author: "did:firefly:org/example",
					Key:    "0x0a989907dcd17272257f3ebcf72f4351df65a846",
				},
			},
			Hash: fftypes.HashString("examplebatchhash"),
			Payload: core.BatchPayload{
				TX: core.TransactionRef{
					Type: core.BatchTypePrivate,
					ID:   fftypes.MustParseUUID("04930D84-0227-4044-9D6D-82C2952A0108"),
				},
				Messages: []*core.Message{},
				Data:     core.DataArray{},
			},
		},

		&core.Namespace{
			Name:        "default",
			NetworkName: "default",
			Description: "Default predefined namespace",
			Created:     fftypes.UnixTime(1652664196),
		},

		&core.WSStart{
			WSActionBase: core.WSActionBase{
				Type: core.WSClientActionStart,
			},
			AutoAck:   &falseVar,
			Namespace: "ns1",
			Name:      "app1_subscription",
		},

		&core.WSAck{
			WSActionBase: core.WSActionBase{
				Type: core.WSClientActionAck,
			},
			ID: fftypes.MustParseUUID("f78bf82b-1292-4c86-8a08-e53d855f1a64"),
			Subscription: &core.SubscriptionRef{
				Name:      "app1_subscription",
				Namespace: "ns1",
			},
		},

		&core.WSError{
			Type:  core.WSProtocolErrorEventType,
			Error: i18n.NewError(ctx, coremsgs.MsgWSMsgSubNotMatched).Error(),
		},
	}

	simpleTypes := []interface{}{
		fftypes.UUID{},
		fftypes.FFTime{},
		fftypes.FFBigInt{},
		fftypes.JSONAny(""),
		fftypes.JSONObject{},
	}

	return generateMarkdownPages(ctx, types, simpleTypes, filepath.Join("..", "..", "doc-site", "docs", "reference", "types"))
}

func getType(v interface{}) reflect.Type {
	if reflect.TypeOf(v).Kind() == reflect.Ptr {
		return reflect.TypeOf(v).Elem()
	}
	return reflect.TypeOf(v)
}

func generateMarkdownPages(ctx context.Context, types []interface{}, simpleTypes []interface{}, outputPath string) (map[string][]byte, error) {
	markdownMap := make(map[string][]byte, len(types))
	rootPageNames := make([]string, len(types))
	for i, v := range types {
		rootPageNames[i] = strings.ToLower(getType(v).Name())
	}

	simpleTypesMarkdown, simpleTypesNames := generateSimpleTypesMarkdown(ctx, simpleTypes, outputPath)
	markdownMap["simpletypes"] = simpleTypesMarkdown

	for i, o := range types {
		pageTitle := getType(types[i]).Name()
		// Page index starts at 1. Simple types will be the first page. Everything else comes after that.
		pageHeader := generatePageHeader(pageTitle)
		b := bytes.NewBuffer([]byte(pageHeader))
		markdown, _, err := generateObjectReferenceMarkdown(ctx, true, o, reflect.TypeOf(o), rootPageNames, simpleTypesNames, []string{}, outputPath)
		if err != nil {
			return nil, err
		}
		b.Write(markdown)
		markdownMap[rootPageNames[i]] = b.Bytes()
	}
	return markdownMap, nil
}

func generateSimpleTypesMarkdown(ctx context.Context, simpleTypes []interface{}, outputPath string) ([]byte, []string) {
	simpleTypeNames := make([]string, len(simpleTypes))
	for i, v := range simpleTypes {
		simpleTypeNames[i] = strings.ToLower(getType(v).Name())
	}

	pageHeader := generatePageHeader("Simple Types")

	b := bytes.NewBuffer([]byte(pageHeader))
	for _, simpleType := range simpleTypes {
		b.WriteString(fmt.Sprintf("## %s\n\n", reflect.TypeOf(simpleType).Name()))
		markdown, _, _ := generateObjectReferenceMarkdown(ctx, true, nil, reflect.TypeOf(simpleType), []string{}, simpleTypeNames, []string{}, outputPath)
		b.Write(markdown)
	}
	return b.Bytes(), simpleTypeNames
}

func generateObjectReferenceMarkdown(ctx context.Context, descRequired bool, example interface{}, t reflect.Type, rootPageNames, simpleTypeNames, generatedTableNames []string, outputPath string) ([]byte, []string, error) {
	typeReferenceDoc := TypeReferenceDoc{}

	if t.Kind() == reflect.Ptr {
		t = reflect.TypeOf(example).Elem()
	}
	// generatedTableNames is where we keep track of all the tables we've generated (recursively)
	// for creating hyperlinks within the markdown
	generatedTableNames = append(generatedTableNames, strings.ToLower(t.Name()))

	// If a detailed type_description.md file exists, include that in a Description section here
	filename, _ := filepath.Abs(filepath.Join(outputPath, "_includes", fmt.Sprintf("%s_description.md", strings.ToLower(t.Name()))))
	_, err := os.Stat(filename)
	if err != nil {
		if descRequired {
			return nil, nil, i18n.NewError(ctx, coremsgs.MsgReferenceMarkdownMissing, filename)
		}
	} else {
		typeReferenceDoc.Description = []byte(fmt.Sprintf("{%% include-markdown \"./_includes/%s_description.md\" %%}\n\n", strings.ToLower(t.Name())))
	}

	// Include an example JSON representation if we have one available
	if example != nil {
		exampleJSON, err := json.MarshalIndent(example, "", "    ")
		if err != nil {
			return nil, nil, err
		}
		typeReferenceDoc.Example = exampleJSON
	}

	// If the type is a struct, look into each field inside it
	if t.Kind() == reflect.Struct {
		typeReferenceDoc.FieldDescriptions, typeReferenceDoc.SubFieldTables, generatedTableNames = generateFieldDescriptionsForStruct(ctx, t, rootPageNames, simpleTypeNames, generatedTableNames, outputPath)
	}

	// buff is the main buffer where we will write the markdown for this page
	buff := bytes.NewBuffer([]byte{})

	// If we only have one section, we will not write H3 headers
	sectionCount := 0
	if typeReferenceDoc.Description != nil {
		sectionCount++
	}
	if typeReferenceDoc.Example != nil {
		sectionCount++
	}
	if typeReferenceDoc.FieldDescriptions != nil {
		sectionCount++
	}

	if typeReferenceDoc.Description != nil {
		buff.Write(typeReferenceDoc.Description)
	}
	if len(typeReferenceDoc.Example) > 0 {
		if sectionCount > 1 {
			buff.WriteString("### Example\n\n```json\n")
		}
		buff.Write(typeReferenceDoc.Example)
		buff.WriteString("\n```\n\n")
	}
	if len(typeReferenceDoc.FieldDescriptions) > 0 {
		if sectionCount > 1 {
			buff.WriteString("### Field Descriptions\n\n")
		}
		buff.Write(typeReferenceDoc.FieldDescriptions)
		buff.WriteString("\n")
	}

	if len(typeReferenceDoc.SubFieldTables) > 0 {
		buff.Write(typeReferenceDoc.SubFieldTables)
	}

	return buff.Bytes(), generatedTableNames, nil
}

func generateEnumList(f reflect.StructField) string {
	enumName := f.Tag.Get("ffenum")
	enumOptions := fftypes.FFEnumValues(enumName)
	buff := new(strings.Builder)
	buff.WriteString("`FFEnum`:")
	for _, v := range enumOptions {
		buff.WriteString(fmt.Sprintf("<br/>`\"%s\"`", v))
	}
	return buff.String()
}

func generateFieldDescriptionsForStruct(ctx context.Context, t reflect.Type, rootPageNames, simpleTypeNames, generatedTableNames []string, outputPath string) ([]byte, []byte, []string) {
	fieldDescriptionsBytes := []byte{}
	// subFieldBuff is where we write any additional tables for sub fields that may be on this struct
	subFieldBuff := bytes.NewBuffer([]byte{})
	if t.NumField() > 0 {
		// Write the table to a temporary buffer - we will throw it away if there are no
		// public JSON serializable fields on the struct
		tableRowCount := 0
		tableBuff := bytes.NewBuffer([]byte{})
		tableBuff.WriteString("| Field Name | Description | Type |\n")
		tableBuff.WriteString("|------------|-------------|------|\n")

		tableRowCount = writeStructFields(ctx, t, rootPageNames, simpleTypeNames, generatedTableNames, outputPath, subFieldBuff, tableBuff, tableRowCount)

		if tableRowCount > 0 {
			fieldDescriptionsBytes = tableBuff.Bytes()
		}
	}
	return fieldDescriptionsBytes, subFieldBuff.Bytes(), generatedTableNames
}

func writeStructFields(ctx context.Context, t reflect.Type, rootPageNames, simpleTypeNames, generatedTableNames []string, outputPath string, subFieldBuff, tableBuff *bytes.Buffer, tableRowCount int) int {
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		jsonTag := field.Tag.Get("json")
		ffstructTag := field.Tag.Get("ffstruct")
		ffexcludeTag := field.Tag.Get("ffexclude")

		// If this is a nested struct, we need to recurse into it
		if field.Anonymous {
			tableRowCount = writeStructFields(ctx, field.Type, rootPageNames, simpleTypeNames, generatedTableNames, outputPath, subFieldBuff, tableBuff, tableRowCount)
			continue
		}

		// If the field is specifically excluded, or doesn't have a json tag, skip it
		if ffexcludeTag != "" || jsonTag == "" || jsonTag == "-" {
			continue
		}

		jsonFieldName := strings.Split(jsonTag, ",")[0]
		messageKeyName := fmt.Sprintf("%s.%s", ffstructTag, jsonFieldName)
		description := i18n.Expand(ctx, i18n.MessageKey(messageKeyName))
		isArray := false

		fieldType := field.Type
		fireflyType := fieldType.Name()

		if fieldType.Kind() == reflect.Slice {
			fieldType = fieldType.Elem()
			fireflyType = fieldType.Name()
			isArray = true
		}

		if fieldType.Kind() == reflect.Ptr {
			fieldType = fieldType.Elem()
			fireflyType = fieldType.Name()
		}

		if isArray {
			fireflyType = fmt.Sprintf("%s[]", fireflyType)
		}

		fireflyType = fmt.Sprintf("`%s`", fireflyType)

		isStruct := fieldType.Kind() == reflect.Struct
		isEnum := strings.ToLower(fieldType.Name()) == "ffenum"

		fieldInRootPages := false
		fieldInSimpleTypes := false
		if slices.Contains(rootPageNames, strings.ToLower(fieldType.Name())) {
			fieldInRootPages = true
		}
		if slices.Contains(simpleTypeNames, strings.ToLower(fieldType.Name())) {
			fieldInSimpleTypes = true
		}

		link := ""
		switch {
		case isEnum:
			fireflyType = generateEnumList(field)
		case fieldInRootPages:
			link = fmt.Sprintf("%s.md#%s", strings.ToLower(fieldType.Name()), strings.ToLower(fieldType.Name()))
		case fieldInSimpleTypes:
			link = fmt.Sprintf("simpletypes.md#%s", strings.ToLower(fieldType.Name()))
		case isStruct:
			link = fmt.Sprintf("#%s", strings.ToLower(fieldType.Name()))
		}
		if link != "" {
			fireflyType = fmt.Sprintf("[%s](%s)", fireflyType, link)

			// Generate the table for the sub type
			tableAlreadyGenerated := slices.Contains(generatedTableNames, strings.ToLower(fieldType.Name()))
			if isStruct && !tableAlreadyGenerated && !fieldInRootPages && !fieldInSimpleTypes {
				subFieldBuff.WriteString(fmt.Sprintf("## %s\n\n", fieldType.Name()))
				subFieldMarkdown, newTableNames, _ := generateObjectReferenceMarkdown(ctx, false, nil, fieldType, rootPageNames, simpleTypeNames, generatedTableNames, outputPath)
				generatedTableNames = newTableNames
				subFieldBuff.Write(subFieldMarkdown)
				subFieldBuff.WriteString("\n")
			}
		}

		tableBuff.WriteString(fmt.Sprintf("| `%s` | %s | %s |\n", jsonFieldName, description, fireflyType))
		tableRowCount++
	}
	return tableRowCount
}

func generatePageHeader(pageTitle string) string {
	return fmt.Sprintf(`---
title: %s
---
`, pageTitle)
}
