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

package tableinfo

import (
	"database/sql"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/percona/pmm/proto"
	"github.com/percona/qan-agent/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTableInfo(t *testing.T) {
	t.Parallel()

	dsn := os.Getenv("PCT_TEST_MYSQL_DSN")
	require.NotEmpty(t, dsn, "PCT_TEST_MYSQL_DSN is not set")

	conn := mysql.NewConnection(dsn)
	if err := conn.Connect(); err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	t.Run("", func(t *testing.T) {
		t.Run("full-table-info", func(t *testing.T) {
			t.Parallel()

			db := "mysql"
			table := "user"
			tables := &proto.TableInfoQuery{
				Create: []proto.Table{{db, table}},
				Index:  []proto.Table{{db, table}},
				Status: []proto.Table{{db, table}},
			}

			got, err := TableInfo(conn, tables)
			require.NoError(t, err)

			tableInfo, ok := got[db+"."+table]
			require.True(t, ok)
			assert.Empty(t, tableInfo.Errors, "TableInfo() returned errors: %v", tableInfo.Errors)
			require.True(t, strings.HasPrefix(tableInfo.Create, "CREATE TABLE `user` ("))

			require.NotNil(t, tableInfo.Status)
			assert.Equal(t, table, tableInfo.Status.Name)

			// Indexes are grouped by name (KeyName), so all the index parts of the
			// PRIMARY key should be together.
			require.NotEmpty(t, tableInfo.Index)
			primaryIndex, ok := tableInfo.Index["PRIMARY"]
			require.True(t, ok)
			require.Len(t, primaryIndex, 2, "tableInfo.Index doesn't have PRIMARY key: %v", tableInfo.Index)
			assert.Equal(t, "Host", primaryIndex[0].ColumnName)
			assert.Equal(t, "User", primaryIndex[1].ColumnName)
		})

		t.Run("status-times", func(t *testing.T) {
			t.Parallel()

			err := conn.DB().QueryRow("SELECT 1 FROM mysql.slow_log").Scan()
			if err != nil && err != sql.ErrNoRows {
				t.Log(err)
				t.Skip("mysql.slow_log table does not exist")
			}

			db := "mysql"
			table := "slow_log"
			tables := &proto.TableInfoQuery{
				Status: []proto.Table{{db, table}},
			}

			got, err := TableInfo(conn, tables)
			require.NoError(t, err)

			tableInfo, ok := got[db+"."+table]
			require.True(t, ok)
			assert.Empty(t, tableInfo.Errors, "TableInfo() returned errors: %v", tableInfo.Errors)

			require.NotNil(t, tableInfo.Status)
			assert.Equal(t, table, tableInfo.Status.Name)

			var zeroTime time.Time
			assert.Equal(t, zeroTime, tableInfo.Status.UpdateTime.Time)
			assert.Equal(t, zeroTime, tableInfo.Status.CheckTime.Time)

			zeroCreateTime, err := conn.VersionConstraint("< 8.0 || > 10.0")
			require.NoError(t, err)
			if zeroCreateTime {
				assert.Equal(t, zeroTime, tableInfo.Status.CreateTime.Time)
			}
		})
	})
}

func TestEscapeString(t *testing.T) {
	in := []struct {
		in  string
		out string
	}{
		{`"dbname"`, `\"dbname\"`},
		{"`dbname`", "`dbname`"},
		{`\"dbname\"`, `\\\"dbname\\\"`},
	}

	for _, i := range in {
		got := escapeString(i.in)
		assert.Equal(t, i.out, got)
	}
}
