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

//go:build !cgo

package difactory

import (
	"github.com/hyperledger/firefly/internal/database/postgres"
	"github.com/hyperledger/firefly/pkg/database"
)

var pluginsByName = map[string]func() database.Plugin{
	(*postgres.Postgres)(nil).Name(): func() database.Plugin { return &postgres.Postgres{} },
}
