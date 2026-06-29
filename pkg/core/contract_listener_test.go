// Copyright © 2021 Kaleido, Inc.
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
	"testing"

	"github.com/hyperledger-firefly/common/pkg/fftypes"
	"github.com/stretchr/testify/assert"
)

func TestFFISerializedEventScan(t *testing.T) {
	params := &FFISerializedEvent{}
	err := params.Scan([]byte(`{"name":"event1","description":"a super event","params":[{"name":"details","type":"integer","details":{"type":"uint256"}}]}`))
	assert.NoError(t, err)
}

func TestFFISerializedEventScanNil(t *testing.T) {
	params := &FFISerializedEvent{}
	err := params.Scan(nil)
	assert.Nil(t, err)
}

func TestFFISerializedEventScanString(t *testing.T) {
	params := &FFISerializedEvent{}
	err := params.Scan(`{"name":"event1","description":"a super event","params":[{"name":"details","type":"integer","details":{"type":"uint256"}}]}`)
	assert.NoError(t, err)
}

func TestFFISerializedEventScanError(t *testing.T) {
	params := &FFISerializedEvent{}
	err := params.Scan(map[string]interface{}{"this is": "not a supported serialization of a FFISerializedEvent"})
	assert.Regexp(t, "FF00105", err)
}

func TestFFISerializedEventValue(t *testing.T) {
	params := &FFISerializedEvent{
		FFIEventDefinition: fftypes.FFIEventDefinition{
			Name:        "event1",
			Description: "a super event",
			Params: fftypes.FFIParams{
				&fftypes.FFIParam{Name: "details", Schema: fftypes.JSONAnyPtr(`{"type": "integer", "details": {"type": "uint256"}}`)},
			},
		},
	}

	val, err := params.Value()
	assert.NoError(t, err)
	assert.Equal(t, `{"name":"event1","description":"a super event","params":[{"name":"details","schema":{"type":"integer","details":{"type":"uint256"}}}]}`, string(val.([]byte)))
}

func TestContractListenerOptionsScan(t *testing.T) {
	options := &ContractListenerOptions{}
	err := options.Scan([]byte(`{"firstBlock":"newest"}`))
	assert.NoError(t, err)
}

func TestContractListenerOptionsScanNil(t *testing.T) {
	options := &ContractListenerOptions{}
	err := options.Scan(nil)
	assert.Nil(t, err)
}

func TestContractListenerOptionsScanString(t *testing.T) {
	options := &ContractListenerOptions{}
	err := options.Scan(`{"firstBlock":"newest"}`)
	assert.NoError(t, err)
}

func TestContractListenerOptionsScanError(t *testing.T) {
	options := &ContractListenerOptions{}
	err := options.Scan(false)
	assert.Regexp(t, "FF00105", err)
}

func TestContractListenerOptionsValue(t *testing.T) {
	options := &ContractListenerOptions{
		FirstEvent: "newest",
	}

	val, err := options.Value()
	assert.NoError(t, err)
	assert.Equal(t, `{"firstEvent":"newest"}`, string(val.([]byte)))
}

func TestListenerFiltersScan(t *testing.T) {
	filters := ListenerFilters{}
	err := filters.Scan([]byte(`[{"event":{"name":"event1","description":"asuperevent","params":[{"name":"value","schema":{"type":"integer","details":{"type":"uint256","internalType":"uint256"}}}]},"location":{"address":"0x1234"}}]`))
	assert.NoError(t, err)
}

func TestListenerFiltersScanNil(t *testing.T) {
	params := &ListenerFilters{}
	err := params.Scan(nil)
	assert.Nil(t, err)
}

func TestListenerFiltersScanString(t *testing.T) {
	params := &ListenerFilters{}
	err := params.Scan(`[{"event":{"name":"event1","description":"asuperevent","params":[{"name":"value","schema":{"type":"integer","details":{"type":"uint256","internalType":"uint256"}}}]},"location":{"address":"0x1234"},"signature":"changed"}]`)
	assert.NoError(t, err)
}

func TestListenerFiltersScanError(t *testing.T) {
	params := &ListenerFilters{}
	err := params.Scan(map[string]interface{}{"this is": "not a supported serialization of a FFISerializedEvent"})
	assert.Regexp(t, "FF00105", err)
}

func TestListenerFiltersValue(t *testing.T) {
	filtersStr := `[{"event":{"name":"event1","description":"asuperevent","params":[{"name":"value","schema":{"type":"integer","details":{"type":"uint256","internalType":"uint256"}}}]},"location":{"address":"0x1234"},"signature":"changed"}]`
	filters := ListenerFilters{}
	err := filters.Scan(filtersStr)
	assert.NoError(t, err)
	value, err := filters.Value()
	assert.NoError(t, err)
	assert.Equal(t, filtersStr, string(value.([]byte)))
}
