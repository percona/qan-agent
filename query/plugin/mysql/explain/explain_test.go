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
	SelectID int       `json:"select_id"`
	CostInfo *CostInfo `json:"cost_info,omitempty"`
	Message  string    `json:"message,omitempty"`
	Table    *Table    `json:"table,omitempty"`
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

	expectedJsonQuery := JsonQuery{
		QueryBlock: QueryBlock{
			SelectID: 1,
		},
	}

	newFormat, err := conn.VersionConstraint(">= 5.7, < 10.1")
	assert.NoError(t, err)
	if newFormat {
		expectedJsonQuery.QueryBlock.Message = "No tables used"
	} else {
		expectedJsonQuery.QueryBlock.Table = &Table{
			Message: "No tables used",
		}
	}
	expectedJSON, err := json.MarshalIndent(&expectedJsonQuery, "", "  ")
	assert.Nil(t, err)

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
	assert.Nil(t, err)
	// Check the json first but only if supported...
	jsonSupported, err := conn.AtLeastVersion("5.6.5")
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

	expectedJsonQuery := JsonQuery{
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
	assert.Nil(t, err)
	if mariaDB101 {
		expectedJsonQuery.QueryBlock.Table.ScannedDatabases = float64(1)
		expectedJsonQuery.QueryBlock.Table.AttachedCondition = "(`tables`.`TABLE_NAME` = 'tables')"
	}

	newFormat, err := conn.VersionConstraint(">= 5.7, < 10.1")
	assert.Nil(t, err)
	if newFormat {
		expectedJsonQuery.QueryBlock.CostInfo = &CostInfo{
			QueryCost: "10.50",
		}
		expectedJsonQuery.QueryBlock.Table.UsedColumns = []string{
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

	expectedJSON, err := json.MarshalIndent(&expectedJsonQuery, "", "  ")
	assert.Nil(t, err)

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

	gotExplainResult, err := Explain(conn, db, query, true)
	assert.Nil(t, err)
	// Check the json first but only if supported...
	jsonSupported, err := conn.AtLeastVersion("5.6.5")
	if jsonSupported {
		assert.JSONEq(t, string(expectedJSON), gotExplainResult.JSON)
	}
	// ... check the rest after, without json
	// you can't compare json as string because properties in json are in undefined order
	expectedExplainResult.JSON = ""
	gotExplainResult.JSON = ""
	assert.Equal(t, expectedExplainResult, gotExplainResult)
}
