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

package sqlcommon

import (
	"context"
	"database/sql"
	"strconv"

	sq "github.com/Masterminds/squirrel"
	"github.com/hyperledger-firefly/common/pkg/config"
	"github.com/hyperledger-firefly/common/pkg/i18n"
	"github.com/hyperledger-firefly/firefly/internal/coreconfig"
	"github.com/hyperledger-firefly/firefly/internal/coremsgs"
	"github.com/hyperledger-firefly/firefly/pkg/core"
	"github.com/hyperledger-firefly/firefly/pkg/database"
)

func (s *SQLCommon) getTableNameFromCollection(ctx context.Context, collection database.CollectionName) (tableName string, fieldMap map[string]string, err error) {
	switch collection {
	case database.CollectionName(database.CollectionMessages):
		return "messages", msgFilterFieldMap, nil
	case database.CollectionName(database.CollectionTransactions):
		return "transactions", transactionFilterFieldMap, nil
	case database.CollectionName(database.CollectionOperations):
		return "operations", opFilterFieldMap, nil
	case database.CollectionName(database.CollectionEvents):
		return "events", eventFilterFieldMap, nil
	case database.CollectionName(database.CollectionTokenTransfers):
		return "tokentransfer", tokenTransferFilterFieldMap, nil
	case database.CollectionName(database.CollectionBlockchainEvents):
		return "blockchainevents", blockchainEventFilterFieldMap, nil
	default:
		return "", nil, i18n.NewError(ctx, coremsgs.MsgUnsupportedCollection, collection)
	}
}

func (s *SQLCommon) getSelectStatements(ns string, tableName string, intervals []core.ChartHistogramInterval, timestampKey string, sql sq.SelectBuilder) (queries []sq.SelectBuilder) {
	for _, interval := range intervals {
		queries = append(queries, sql.
			From(tableName).
			Where(
				sq.And{
					sq.GtOrEq{timestampKey: interval.StartTime},
					sq.Lt{timestampKey: interval.EndTime},
					sq.Eq{"namespace": ns},
				}).
			OrderBy(timestampKey).
			Limit(config.GetUint64(coreconfig.HistogramsMaxChartRows)))
	}

	return queries
}

func (s *SQLCommon) histogramResult(ctx context.Context, tableName string, rows *sql.Rows, onlyTimestamp bool) (map[string]int, string, error) {
	total := 0
	if onlyTimestamp {
		countMap := map[string]int{}
		for rows.Next() {
			var timestamp string
			err := rows.Scan(&timestamp)
			if err != nil {
				return nil, "", i18n.NewError(ctx, coremsgs.MsgDBReadErr, tableName)
			}
			total++
		}
		return countMap, strconv.Itoa(total), nil
	}

	typeMap := map[string]int{}
	for rows.Next() {
		var timestamp string
		var typeStr string
		err := rows.Scan(&timestamp, &typeStr)
		if err != nil {
			return nil, "", i18n.NewError(ctx, coremsgs.MsgDBReadErr, tableName)
		}

		if _, ok := typeMap[typeStr]; !ok {
			typeMap[typeStr] = 1
		} else {
			typeMap[typeStr]++
		}

		total++
	}

	return typeMap, strconv.Itoa(total), nil
}

func (s *SQLCommon) GetChartHistogram(ctx context.Context, ns string, intervals []core.ChartHistogramInterval, collection database.CollectionName) (histogramList []*core.ChartHistogram, err error) {
	tableName, fieldMap, err := s.getTableNameFromCollection(ctx, collection)
	if err != nil {
		return nil, err
	}

	// Timestamp column name
	timestampKey := "created"
	if tableName == "blockchainevents" {
		// Blockchain Events have a `timestamp` column name
		timestampKey = "timestamp"
	}

	// Number of columns to read.
	// Some tables don't have a `type` field and therefore
	// a `type` field will not be queried
	onlyTimestamp := true
	cols := []string{timestampKey}
	if typeStr, ok := fieldMap["type"]; ok {
		cols = append(cols, typeStr)
		onlyTimestamp = false
	}

	queries := s.getSelectStatements(ns, tableName, intervals, timestampKey, sq.Select(cols...))

	for i, query := range queries {
		// Query bucket's data
		rows, _, err := s.Query(ctx, tableName, query)
		if err != nil {
			return nil, err
		}
		defer rows.Close()

		data, total, err := s.histogramResult(ctx, tableName, rows, onlyTimestamp)
		if err != nil {
			return nil, err
		}

		histTypes := make([]*core.ChartHistogramType, 0)
		histBucket := core.ChartHistogram{
			Count:     total,
			Timestamp: intervals[i].StartTime,
			Types:     histTypes,
			IsCapped:  total == config.GetString(coreconfig.HistogramsMaxChartRows),
		}

		// If the bucket has types, add their counts
		if !onlyTimestamp {
			for k, v := range data {
				histBucket.Types = append(histBucket.Types,
					&core.ChartHistogramType{
						Count: strconv.Itoa(v),
						Type:  k,
					})
			}
		}

		histogramList = append(histogramList, &histBucket)
	}

	return histogramList, nil
}
