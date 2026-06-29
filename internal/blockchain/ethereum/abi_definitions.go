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

package ethereum

import "github.com/hyperledger-firefly/signer/pkg/abi"

var batchPinMethodABIV1 = &abi.Entry{
	Name: "pinBatch",
	Type: "function",
	Inputs: abi.ParameterArray{
		{
			InternalType: "string",
			Name:         "namespace",
			Type:         "string",
		},
		{
			InternalType: "bytes32",
			Name:         "uuids",
			Type:         "bytes32",
		},
		{
			InternalType: "bytes32",
			Name:         "batchHash",
			Type:         "bytes32",
		},
		{
			InternalType: "string",
			Name:         "payloadRef",
			Type:         "string",
		},
		{
			InternalType: "bytes32[]",
			Name:         "contexts",
			Type:         "bytes32[]",
		},
	},
}

var batchPinMethodABI = &abi.Entry{
	Name: "pinBatch",
	Type: "function",
	Inputs: abi.ParameterArray{
		{
			InternalType: "bytes32",
			Name:         "uuids",
			Type:         "bytes32",
		},
		{
			InternalType: "bytes32",
			Name:         "batchHash",
			Type:         "bytes32",
		},
		{
			InternalType: "string",
			Name:         "payloadRef",
			Type:         "string",
		},
		{
			InternalType: "bytes32[]",
			Name:         "contexts",
			Type:         "bytes32[]",
		},
	},
}

var networkActionMethodABI = &abi.Entry{
	Name: "networkAction",
	Type: "function",
	Inputs: abi.ParameterArray{
		{
			InternalType: "string",
			Name:         "action",
			Type:         "string",
		},
		{
			InternalType: "string",
			Name:         "payload",
			Type:         "string",
		},
	},
}

var batchPinEventABI = &abi.Entry{
	Name: "BatchPin",
	Type: "event",
	Inputs: abi.ParameterArray{
		{
			Indexed:      false,
			InternalType: "address",
			Name:         "author",
			Type:         "address",
		},
		{
			Indexed:      false,
			InternalType: "uint256",
			Name:         "timestamp",
			Type:         "uint256",
		},
		{
			Indexed:      false,
			InternalType: "string",
			Name:         "namespace",
			Type:         "string",
		},
		{
			Indexed:      false,
			InternalType: "bytes32",
			Name:         "uuids",
			Type:         "bytes32",
		},
		{
			Indexed:      false,
			InternalType: "bytes32",
			Name:         "batchHash",
			Type:         "bytes32",
		},
		{
			Indexed:      false,
			InternalType: "string",
			Name:         "payloadRef",
			Type:         "string",
		},
		{
			Indexed:      false,
			InternalType: "bytes32[]",
			Name:         "contexts",
			Type:         "bytes32[]",
		},
	},
}

var networkVersionMethodABI = &abi.Entry{
	Name:            "networkVersion",
	Type:            "function",
	StateMutability: "pure",
	Inputs:          abi.ParameterArray{},
	Outputs: abi.ParameterArray{
		{
			InternalType: "uint8",
			Type:         "uint8",
		},
	},
}
