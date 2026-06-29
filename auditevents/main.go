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

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "auditevents [flags] host",
	Short: "FireFly event auditor",
	Long:  "Tool for auditing the blockchain events recorded by Hyperledger FireFly",
	Args:  cobra.MinimumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		return run(args[0])
	},
}

func get(host, api string, result interface{}) (err error) {
	resp, err := http.Get(host + api)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	err = json.Unmarshal(body, &result)
	return err
}

func run(host string) error {
	var status core.NamespaceStatus
	err := get(host, "/api/v1/status", &status)
	if err != nil {
		return err
	}

	expectedSource := status.Plugins.Blockchain[0].PluginType
	expectedListener := ""

	limit := 50
	received := limit

	var validated int64
	var lastSequence int64
	var lastProtocolID string

	fmt.Printf("Checking all blockchain events for increasing protocol ID.\n\n")
	fmt.Printf("Configuration:\n")
	fmt.Printf("  host=%s\n", host)
	fmt.Printf("  source=%s\n", expectedSource)
	fmt.Printf("  listener=%s\n\n", expectedListener)
	fmt.Printf("%-10s %s\n", "Sequence", "Protocol ID")

	for received == limit {
		var events []core.EnrichedEvent
		api := fmt.Sprintf("/api/v1/events?type=blockchain_event_received&fetchreferences&sort=sequence&sequence=>%d&limit=%d", lastSequence, limit)
		err := get(host, api, &events)
		if err != nil {
			return err
		}
		received = len(events)

		for _, event := range events {
			lastSequence = event.Sequence
			listener := event.BlockchainEvent.Listener
			src := event.BlockchainEvent.Source
			if listener.String() != expectedListener || src != expectedSource {
				continue
			}
			protocolID := event.BlockchainEvent.ProtocolID
			fmt.Printf("%-10d %s\n", lastSequence, protocolID)
			if protocolID <= lastProtocolID {
				return errors.New("out of order events detected")
			}
			lastProtocolID = protocolID
			validated++
		}
	}

	fmt.Printf("%d events validated\n", validated)
	return nil
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "%s\n", err)
		os.Exit(1)
	}
	os.Exit(0)
}
