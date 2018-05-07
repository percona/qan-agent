/*
   Copyright (c) 2016, Percona LLC and/or its affiliates. All rights reserved.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published by
   the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <http://www.gnu.org/licenses/>
*/

package explain

import (
	"database/sql"
	"encoding/json"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/percona/pmm/proto"
	"github.com/percona/qan-agent/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --------------------------------------------------------------------------

type JsonQuery struct {
	QueryBlock QueryBlock `json:"query_block"`
}

type QueryBlock struct {
	SelectID   int        `json:"select_id"`
	CostInfo   *CostInfo  `json:"cost_info,omitempty"`
	Message    string     `json:"message,omitempty"`
	Table      *Table     `json:"table,omitempty"`
	NestedLoop NestedLoop `json:"nested_loop,omitempty"`
}

type Table struct {
	TableName         string      `json:"table_name,omitempty"`
	AccessType        string      `json:"access_type,omitempty"`
	Key               string      `json:"key,omitempty"`
	SkipOpenTable     bool        `json:"skip_open_table,omitempty"`
	UsedColumns       []string    `json:"used_columns,omitempty"`
	ScannedDatabases  interface{} `json:"scanned_databases,omitempty"`
	AttachedCondition string      `json:"attached_condition,omitempty"`
	Message           string      `json:"message,omitempty"`
}

type CostInfo struct {
	QueryCost string `json:"query_cost,omitempty"`
}

// Table80 for MySQL 8.0
type Table80 struct {
	AccessType          string      `json:"access_type,omitempty"`
	AttachedCondition   string      `json:"attached_condition,omitempty"`
	CostInfo            *CostInfo80 `json:"cost_info,omitempty"`
	Filtered            string      `json:"filtered"`
	Key                 string      `json:"key"`
	KeyLength           string      `json:"key_length"`
	PossibleKeys        []string    `json:"possible_keys"`
	Ref                 []string    `json:"ref,omitempty"`
	RowsExaminedPerScan float64     `json:"rows_examined_per_scan"`
	RowsProducedPerJoin float64     `json:"rows_produced_per_join"`
	TableName           string      `json:"table_name,omitempty"`
	UsedColumns         []string    `json:"used_columns,omitempty"`
	UsedKeyParts        []string    `json:"used_key_parts,omitempty"`
	UsingIndex          bool        `json:"using_index,omitempty"`
}

type CostInfo80 struct {
	DataReadPerJoin string `json:"data_read_per_join"`
	EvalCost        string `json:"eval_cost"`
	PrefixCost      string `json:"prefix_cost"`
	ReadCost        string `json:"read_cost"`
}

type NestedLoop []NestedLoopItem

type NestedLoopItem struct {
	Table *Table80 `json:"table,omitempty"`
}

func TestExplain(t *testing.T) {
	t.Parallel()

	dsn := os.Getenv("PCT_TEST_MYSQL_DSN")
	require.NotEmpty(t, dsn, "PCT_TEST_MYSQL_DSN is not set")

	conn := mysql.NewConnection(dsn)
	if err := conn.Connect(); err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	tests := []func(*testing.T, mysql.Connector){
		testExplainWithDb,
		testExplainWithoutDb,
		testExplainWithoutQuery,
	}
	t.Run("explain", func(t *testing.T) {
		for _, f := range tests {
			f := f // capture range variable
			fName := runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
			t.Run(fName, func(t *testing.T) {
				// t.Parallel()
				f(t, conn)
			})
		}
	})
}

func testExplainWithoutQuery(t *testing.T, conn mysql.Connector) {
	db := ""
	query := "  "

	_, err := Explain(conn, db, query, true)
	assert.NotNil(t, err)

	// This is not a good practice. We should not care about the error type but in this case, this is
	// the only way to check we catched an empty query when calling 'explain'
	isEmptyMessage := strings.Contains(err.Error(), "cannot run EXPLAIN on an empty query example")
	assert.Equal(t, true, isEmptyMessage)

}

func testExplainWithoutDb(t *testing.T, conn mysql.Connector) {
	db := ""
	query := "SELECT 1"

	expectedJSONQuery := JsonQuery{
		QueryBlock: QueryBlock{
			SelectID: 1,
		},
	}

	newFormat, err := conn.VersionConstraint(">= 5.7, < 10.1")
	assert.NoError(t, err)
	if newFormat {
		expectedJSONQuery.QueryBlock.Message = "No tables used"
	} else {
		expectedJSONQuery.QueryBlock.Table = &Table{
			Message: "No tables used",
		}
	}
	expectedJSON, err := json.MarshalIndent(&expectedJSONQuery, "", "  ")
	require.NoError(t, err)

	expectedExplainResult := &proto.ExplainResult{
		Classic: []*proto.ExplainRow{
			{
				Id: proto.NullInt64{
					NullInt64: sql.NullInt64{
						Int64: 1,
						Valid: true,
					},
				},
				SelectType: proto.NullString{
					NullString: sql.NullString{
						String: "SIMPLE",
						Valid:  true,
					},
				},
				Table: proto.NullString{
					NullString: sql.NullString{
						String: "",
						Valid:  false,
					},
				},
				Type: proto.NullString{
					NullString: sql.NullString{
						String: "",
						Valid:  false,
					},
				},
				PossibleKeys: proto.NullString{
					NullString: sql.NullString{
						String: "",
						Valid:  false,
					},
				},
				Key: proto.NullString{
					NullString: sql.NullString{
						String: "",
						Valid:  false,
					},
				},
				KeyLen: proto.NullString{
					NullString: sql.NullString{
						String: "",
						Valid:  false,
					},
				},
				Ref: proto.NullString{
					NullString: sql.NullString{
						String: "",
						Valid:  false,
					},
				},
				Rows: proto.NullInt64{
					NullInt64: sql.NullInt64{
						Int64: 0,
						Valid: false,
					},
				},
				Extra: proto.NullString{
					NullString: sql.NullString{
						String: "No tables used",
						Valid:  true,
					},
				},
			},
		},
		JSON: string(expectedJSON),
	}

	gotExplainResult, err := Explain(conn, db, query, true)
	require.NoError(t, err)

	// Check the json first but only if supported...
	// EXPLAIN in JSON format is introduced since MySQL 5.6.5 and MariaDB 10.1.2
	// https://mariadb.com/kb/en/mariadb/explain-format-json/
	jsonSupported, err := conn.VersionConstraint(">= 5.6.5, < 10.0.0 || >= 10.1.2")
	if jsonSupported {
		assert.JSONEq(t, string(expectedJSON), gotExplainResult.JSON)
	}
	// ... check the rest after, without json
	// you can't compare json as string because properties in json are in undefined order
	expectedExplainResult.JSON = ""
	gotExplainResult.JSON = ""
	assert.Equal(t, expectedExplainResult, gotExplainResult)
}

func testExplainWithDb(t *testing.T, conn mysql.Connector) {
	db := "information_schema"
	query := "SELECT table_name FROM tables WHERE table_name='tables'"

	gotExplainResult, err := Explain(conn, db, query, true)
	require.NoError(t, err)

	expectedJSONQuery := JsonQuery{
		QueryBlock: QueryBlock{
			SelectID: 1,
			Table: &Table{
				TableName:         "tables",
				AccessType:        "ALL",
				Key:               "TABLE_NAME",
				SkipOpenTable:     true,
				ScannedDatabases:  "1",
				AttachedCondition: "(`information_schema`.`tables`.`TABLE_NAME` = 'tables')",
			},
		},
	}

	mariaDB101, err := conn.VersionConstraint(">= 10.1")
	assert.NoError(t, err)
	if mariaDB101 {
		expectedJSONQuery.QueryBlock.Table.ScannedDatabases = float64(1)
		expectedJSONQuery.QueryBlock.Table.AttachedCondition = "(`tables`.`TABLE_NAME` = 'tables')"
	}

	mariaDB103, err := conn.VersionConstraint(">= 10.3")
	assert.NoError(t, err)
	if mariaDB103 {
		expectedJSONQuery.QueryBlock.Table.AttachedCondition = "`tables`.`TABLE_NAME` = 'tables'"
	}

	mysql57, err := conn.VersionConstraint(">= 5.7, < 8.0")
	require.NoError(t, err)
	if mysql57 {
		expectedJSONQuery.QueryBlock.CostInfo = &CostInfo{
			QueryCost: "10.50",
		}
		expectedJSONQuery.QueryBlock.Table.UsedColumns = []string{
			"TABLE_CATALOG",
			"TABLE_SCHEMA",
			"TABLE_NAME",
			"TABLE_TYPE",
			"ENGINE",
			"VERSION",
			"ROW_FORMAT",
			"TABLE_ROWS",
			"AVG_ROW_LENGTH",
			"DATA_LENGTH",
			"MAX_DATA_LENGTH",
			"INDEX_LENGTH",
			"DATA_FREE",
			"AUTO_INCREMENT",
			"CREATE_TIME",
			"UPDATE_TIME",
			"CHECK_TIME",
			"TABLE_COLLATION",
			"CHECKSUM",
			"CREATE_OPTIONS",
			"TABLE_COMMENT",
		}
	}

	mysql80, err := conn.VersionConstraint(">= 8.0, < 10.0")
	require.NoError(t, err)
	if mysql80 {
		expectedJSONQuery.QueryBlock.CostInfo = &CostInfo{
			QueryCost: "9.85",
		}
		expectedJSONQuery.QueryBlock.Table = nil
		expectedJSONQuery.QueryBlock.NestedLoop = NestedLoop{
			{
				Table: &Table80{
					AccessType: "index",
					CostInfo: &CostInfo80{
						DataReadPerJoin: "224",
						EvalCost:        "0.10",
						PrefixCost:      "0.35",
						ReadCost:        "0.25",
					},
					Filtered:  "100.00",
					Key:       "name",
					KeyLength: "194",
					PossibleKeys: []string{
						"PRIMARY",
					},
					RowsExaminedPerScan: 1,
					RowsProducedPerJoin: 1,
					TableName:           "cat",
					UsedColumns: []string{
						"id",
					},
					UsedKeyParts: []string{
						"name",
					},
					UsingIndex: true,
				},
			},
			{
				Table: &Table80{
					AccessType: "ref",
					CostInfo: &CostInfo80{
						DataReadPerJoin: "1K",
						EvalCost:        "0.50",
						PrefixCost:      "1.12",
						ReadCost:        "0.28",
					},
					Filtered:  "100.00",
					Key:       "catalog_id",
					KeyLength: "8",
					PossibleKeys: []string{
						"PRIMARY",
						"catalog_id",
					},
					Ref: []string{
						"mysql.cat.id",
					},
					RowsExaminedPerScan: 5,
					RowsProducedPerJoin: 5,
					TableName:           "sch",
					UsedColumns: []string{
						"id",
						"catalog_id",
						"name",
					},
					UsedKeyParts: []string{
						"catalog_id",
					},
					UsingIndex: true,
				},
			},
			{
				Table: &Table80{
					AccessType:        "eq_ref",
					AttachedCondition: "(can_access_table(`mysql`.`sch`.`name`,`mysql`.`tbl`.`name`) and is_visible_dd_object(`mysql`.`tbl`.`hidden`))",
					CostInfo: &CostInfo80{
						DataReadPerJoin: "154K",
						EvalCost:        "0.50",
						PrefixCost:      "4.60",
						ReadCost:        "2.97",
					},
					Filtered:  "100.00",
					Key:       "schema_id",
					KeyLength: "202",
					PossibleKeys: []string{
						"schema_id",
					},
					Ref: []string{
						"mysql.sch.id",
						"const",
					},
					RowsExaminedPerScan: 1,
					RowsProducedPerJoin: 5,
					TableName:           "tbl",
					UsedColumns: []string{
						"schema_id",
						"name",
						"collation_id",
						"hidden",
						"tablespace_id",
					},
					UsedKeyParts: []string{
						"schema_id",
						"name",
					},
				},
			},
			{
				Table: &Table80{
					AccessType: "eq_ref",
					CostInfo: &CostInfo80{
						DataReadPerJoin: "2K",
						EvalCost:        "0.50",
						PrefixCost:      "6.35",
						ReadCost:        "1.25",
					},
					Filtered:  "100.00",
					Key:       "PRIMARY",
					KeyLength: "388",
					PossibleKeys: []string{
						"PRIMARY",
					},
					Ref: []string{
						"mysql.sch.name",
						"const",
					},
					RowsExaminedPerScan: 1,
					RowsProducedPerJoin: 5,
					TableName:           "stat",
					UsedColumns: []string{
						"schema_name",
						"table_name",
					},
					UsedKeyParts: []string{
						"schema_name",
						"table_name",
					},
					UsingIndex: true,
				},
			},
			{
				Table: &Table80{
					AccessType: "eq_ref",
					CostInfo: &CostInfo80{
						DataReadPerJoin: "34K",
						EvalCost:        "0.50",
						PrefixCost:      "8.10",
						ReadCost:        "1.25",
					},
					Filtered:  "100.00",
					Key:       "PRIMARY",
					KeyLength: "8",
					PossibleKeys: []string{
						"PRIMARY",
					},
					Ref: []string{
						"mysql.tbl.tablespace_id",
					},
					RowsExaminedPerScan: 1,
					RowsProducedPerJoin: 5,
					TableName:           "ts",
					UsedColumns: []string{
						"id",
					},
					UsedKeyParts: []string{
						"id",
					},
					UsingIndex: true,
				},
			},
			{
				Table: &Table80{
					AccessType: "eq_ref",
					CostInfo: &CostInfo80{
						DataReadPerJoin: "1K",
						EvalCost:        "0.50",
						PrefixCost:      "9.85",
						ReadCost:        "1.25",
					},
					Filtered:  "100.00",
					Key:       "PRIMARY",
					KeyLength: "8",
					PossibleKeys: []string{
						"PRIMARY",
					},
					Ref: []string{
						"mysql.tbl.collation_id",
					},
					RowsExaminedPerScan: 1,
					RowsProducedPerJoin: 5,
					TableName:           "col",
					UsedColumns: []string{
						"id",
					},
					UsedKeyParts: []string{
						"id",
					},
					UsingIndex: true,
				},
			},
		}

		// Some values are unpredictable in MySQL 8.
		{
			gotJSONQuery := JsonQuery{}
			err = json.Unmarshal([]byte(gotExplainResult.JSON), &gotJSONQuery)
			require.NoError(t, err)

			expectedJSONQuery.QueryBlock.CostInfo = gotJSONQuery.QueryBlock.CostInfo
			for i := range expectedJSONQuery.QueryBlock.NestedLoop {
				expectedJSONQuery.QueryBlock.NestedLoop[i].Table.CostInfo.PrefixCost = gotJSONQuery.QueryBlock.NestedLoop[i].Table.CostInfo.PrefixCost
				expectedJSONQuery.QueryBlock.NestedLoop[i].Table.CostInfo.ReadCost = gotJSONQuery.QueryBlock.NestedLoop[i].Table.CostInfo.ReadCost
			}
		}
	}

	expectedJSON, err := json.MarshalIndent(&expectedJSONQuery, "", "  ")
	require.NoError(t, err)

	expectedExplainResult := &proto.ExplainResult{
		Classic: []*proto.ExplainRow{
			{
				Id: proto.NullInt64{
					NullInt64: sql.NullInt64{
						Int64: 1,
						Valid: true,
					},
				},
				SelectType: proto.NullString{
					NullString: sql.NullString{
						String: "SIMPLE",
						Valid:  true,
					},
				},
				Table: proto.NullString{
					NullString: sql.NullString{
						String: "tables",
						Valid:  true,
					},
				},
				Type: proto.NullString{
					NullString: sql.NullString{
						String: "ALL",
						Valid:  true,
					},
				},
				PossibleKeys: proto.NullString{
					NullString: sql.NullString{
						String: "",
						Valid:  false,
					},
				},
				Key: proto.NullString{
					NullString: sql.NullString{
						String: "TABLE_NAME",
						Valid:  true,
					},
				},
				KeyLen: proto.NullString{
					NullString: sql.NullString{
						String: "",
						Valid:  false,
					},
				},
				Ref: proto.NullString{
					NullString: sql.NullString{
						String: "",
						Valid:  false,
					},
				},
				Rows: proto.NullInt64{
					NullInt64: sql.NullInt64{
						Int64: 0,
						Valid: false,
					},
				},
				Extra: proto.NullString{
					NullString: sql.NullString{
						String: "Using where; Skip_open_table; Scanned 1 database",
						Valid:  true,
					},
				},
			},
		},
		JSON: string(expectedJSON),
	}

	if mysql80 {
		expectedExplainResult = &proto.ExplainResult{
			Classic: []*proto.ExplainRow{
				{
					Id: proto.NullInt64{
						NullInt64: sql.NullInt64{
							Int64: 1,
							Valid: true,
						},
					},
					SelectType: proto.NullString{
						NullString: sql.NullString{
							String: "SIMPLE",
							Valid:  true,
						},
					},
					Table: proto.NullString{
						NullString: sql.NullString{
							String: "cat",
							Valid:  true,
						},
					},
					Partitions: proto.NullString{
						NullString: sql.NullString{
							String: "",
							Valid:  false,
						},
					},
					CreateTable: proto.NullString{
						NullString: sql.NullString{
							String: "",
							Valid:  false,
						},
					},
					Type: proto.NullString{
						NullString: sql.NullString{
							String: "index",
							Valid:  true,
						},
					},
					PossibleKeys: proto.NullString{
						NullString: sql.NullString{
							String: "PRIMARY",
							Valid:  true,
						},
					},
					Key: proto.NullString{
						NullString: sql.NullString{
							String: "name",
							Valid:  true,
						},
					},
					KeyLen: proto.NullString{
						NullString: sql.NullString{
							String: "194",
							Valid:  true,
						},
					},
					Ref: proto.NullString{
						NullString: sql.NullString{
							String: "",
							Valid:  false,
						},
					},
					Rows: proto.NullInt64{
						NullInt64: sql.NullInt64{
							Int64: 1,
							Valid: true,
						},
					},
					Filtered: proto.NullFloat64{
						NullFloat64: sql.NullFloat64{
							Float64: 100,
							Valid:   true,
						},
					},
					Extra: proto.NullString{
						NullString: sql.NullString{
							String: "Using index",
							Valid:  true,
						},
					},
				},
				{
					Id: proto.NullInt64{
						NullInt64: sql.NullInt64{
							Int64: 1,
							Valid: true,
						},
					},
					SelectType: proto.NullString{
						NullString: sql.NullString{
							String: "SIMPLE",
							Valid:  true,
						},
					},
					Table: proto.NullString{
						NullString: sql.NullString{
							String: "sch",
							Valid:  true,
						},
					},
					Partitions: proto.NullString{
						NullString: sql.NullString{
							String: "",
							Valid:  false,
						},
					},
					CreateTable: proto.NullString{
						NullString: sql.NullString{
							String: "",
							Valid:  false,
						},
					},
					Type: proto.NullString{
						NullString: sql.NullString{
							String: "ref",
							Valid:  true,
						},
					},
					PossibleKeys: proto.NullString{
						NullString: sql.NullString{
							String: "PRIMARY,catalog_id",
							Valid:  true,
						},
					},
					Key: proto.NullString{
						NullString: sql.NullString{
							String: "catalog_id",
							Valid:  true,
						},
					},
					KeyLen: proto.NullString{
						NullString: sql.NullString{
							String: "8",
							Valid:  true,
						},
					},
					Ref: proto.NullString{
						NullString: sql.NullString{
							String: "mysql.cat.id",
							Valid:  true,
						},
					},
					Rows: proto.NullInt64{
						NullInt64: sql.NullInt64{Int64: 5,
							Valid: true,
						},
					},
					Filtered: proto.NullFloat64{
						NullFloat64: sql.NullFloat64{
							Float64: 100,
							Valid:   true,
						},
					},
					Extra: proto.NullString{
						NullString: sql.NullString{
							String: "Using index",
							Valid:  true,
						},
					},
				},
				{
					Id: proto.NullInt64{
						NullInt64: sql.NullInt64{Int64: 1,
							Valid: true,
						},
					},
					SelectType: proto.NullString{
						NullString: sql.NullString{
							String: "SIMPLE",
							Valid:  true,
						},
					},
					Table: proto.NullString{
						NullString: sql.NullString{
							String: "tbl",
							Valid:  true,
						},
					},
					Partitions: proto.NullString{
						NullString: sql.NullString{
							String: "",
							Valid:  false,
						},
					},
					CreateTable: proto.NullString{
						NullString: sql.NullString{
							String: "",
							Valid:  false,
						},
					},
					Type: proto.NullString{
						NullString: sql.NullString{
							String: "eq_ref",
							Valid:  true,
						},
					},
					PossibleKeys: proto.NullString{
						NullString: sql.NullString{
							String: "schema_id",
							Valid:  true,
						},
					},
					Key: proto.NullString{
						NullString: sql.NullString{
							String: "schema_id",
							Valid:  true,
						},
					},
					KeyLen: proto.NullString{
						NullString: sql.NullString{
							String: "202",
							Valid:  true,
						},
					},
					Ref: proto.NullString{
						NullString: sql.NullString{
							String: "mysql.sch.id,const",
							Valid:  true,
						},
					},
					Rows: proto.NullInt64{
						NullInt64: sql.NullInt64{Int64: 1,
							Valid: true,
						},
					},
					Filtered: proto.NullFloat64{
						NullFloat64: sql.NullFloat64{
							Float64: 100,
							Valid:   true,
						},
					},
					Extra: proto.NullString{
						NullString: sql.NullString{
							String: "Using where",
							Valid:  true,
						},
					},
				},
				{
					Id: proto.NullInt64{
						NullInt64: sql.NullInt64{Int64: 1,
							Valid: true,
						},
					},
					SelectType: proto.NullString{
						NullString: sql.NullString{
							String: "SIMPLE",
							Valid:  true,
						},
					},
					Table: proto.NullString{
						NullString: sql.NullString{
							String: "stat",
							Valid:  true,
						},
					},
					Partitions: proto.NullString{
						NullString: sql.NullString{
							String: "",
							Valid:  false,
						},
					},
					CreateTable: proto.NullString{
						NullString: sql.NullString{
							String: "",
							Valid:  false,
						},
					},
					Type: proto.NullString{
						NullString: sql.NullString{
							String: "eq_ref",
							Valid:  true,
						},
					},
					PossibleKeys: proto.NullString{
						NullString: sql.NullString{
							String: "PRIMARY",
							Valid:  true,
						},
					},
					Key: proto.NullString{
						NullString: sql.NullString{
							String: "PRIMARY",
							Valid:  true,
						},
					},
					KeyLen: proto.NullString{
						NullString: sql.NullString{
							String: "388",
							Valid:  true,
						},
					},
					Ref: proto.NullString{
						NullString: sql.NullString{
							String: "mysql.sch.name,const",
							Valid:  true,
						},
					},
					Rows: proto.NullInt64{
						NullInt64: sql.NullInt64{Int64: 1,
							Valid: true,
						},
					},
					Filtered: proto.NullFloat64{
						NullFloat64: sql.NullFloat64{
							Float64: 100,
							Valid:   true,
						},
					},
					Extra: proto.NullString{
						NullString: sql.NullString{
							String: "Using index",
							Valid:  true,
						},
					},
				},
				{
					Id: proto.NullInt64{
						NullInt64: sql.NullInt64{Int64: 1,
							Valid: true,
						},
					},
					SelectType: proto.NullString{
						NullString: sql.NullString{
							String: "SIMPLE",
							Valid:  true,
						},
					},
					Table: proto.NullString{
						NullString: sql.NullString{
							String: "ts",
							Valid:  true,
						},
					},
					Partitions: proto.NullString{
						NullString: sql.NullString{
							String: "",
							Valid:  false,
						},
					},
					CreateTable: proto.NullString{
						NullString: sql.NullString{
							String: "",
							Valid:  false,
						},
					},
					Type: proto.NullString{
						NullString: sql.NullString{
							String: "eq_ref",
							Valid:  true,
						},
					},
					PossibleKeys: proto.NullString{
						NullString: sql.NullString{
							String: "PRIMARY",
							Valid:  true,
						},
					},
					Key: proto.NullString{
						NullString: sql.NullString{
							String: "PRIMARY",
							Valid:  true,
						},
					},
					KeyLen: proto.NullString{
						NullString: sql.NullString{
							String: "8",
							Valid:  true,
						},
					},
					Ref: proto.NullString{
						NullString: sql.NullString{
							String: "mysql.tbl.tablespace_id",
							Valid:  true,
						},
					},
					Rows: proto.NullInt64{
						NullInt64: sql.NullInt64{Int64: 1,
							Valid: true,
						},
					},
					Filtered: proto.NullFloat64{
						NullFloat64: sql.NullFloat64{
							Float64: 100,
							Valid:   true,
						},
					},
					Extra: proto.NullString{
						NullString: sql.NullString{
							String: "Using index",
							Valid:  true,
						},
					},
				},
				{
					Id: proto.NullInt64{
						NullInt64: sql.NullInt64{Int64: 1,
							Valid: true,
						},
					},
					SelectType: proto.NullString{
						NullString: sql.NullString{
							String: "SIMPLE",
							Valid:  true,
						},
					},
					Table: proto.NullString{
						NullString: sql.NullString{
							String: "col",
							Valid:  true,
						},
					},
					Partitions: proto.NullString{
						NullString: sql.NullString{
							String: "",
							Valid:  false,
						},
					},
					CreateTable: proto.NullString{
						NullString: sql.NullString{
							String: "",
							Valid:  false,
						},
					},
					Type: proto.NullString{
						NullString: sql.NullString{
							String: "eq_ref",
							Valid:  true,
						},
					},
					PossibleKeys: proto.NullString{
						NullString: sql.NullString{
							String: "PRIMARY",
							Valid:  true,
						},
					},
					Key: proto.NullString{
						NullString: sql.NullString{
							String: "PRIMARY",
							Valid:  true,
						},
					},
					KeyLen: proto.NullString{
						NullString: sql.NullString{
							String: "8",
							Valid:  true,
						},
					},
					Ref: proto.NullString{
						NullString: sql.NullString{
							String: "mysql.tbl.collation_id",
							Valid:  true,
						},
					},
					Rows: proto.NullInt64{
						NullInt64: sql.NullInt64{Int64: 1,
							Valid: true,
						},
					},
					Filtered: proto.NullFloat64{
						NullFloat64: sql.NullFloat64{
							Float64: 100,
							Valid:   true,
						},
					},
					Extra: proto.NullString{
						NullString: sql.NullString{
							String: "Using index",
							Valid:  true,
						},
					},
				},
			},
		}
	}

	// Check the json first but only if supported...
	// EXPLAIN in JSON format is introduced since MySQL 5.6.5 and MariaDB 10.1.2
	// https://mariadb.com/kb/en/mariadb/explain-format-json/
	jsonSupported, err := conn.VersionConstraint(">= 5.6.5, < 10.0.0 || >= 10.1.2")
	if jsonSupported {
		assert.JSONEq(t, string(expectedJSON), gotExplainResult.JSON)
	}
	// ... check the rest after, without json
	// you can't compare json as string because properties in json are in undefined order
	expectedExplainResult.JSON = ""
	gotExplainResult.JSON = ""
	assert.Equal(t, expectedExplainResult, gotExplainResult, "%#+v", gotExplainResult)
}
