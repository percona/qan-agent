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
)

func TestTableInfo(t *testing.T) {
	t.Parallel()

	dsn := os.Getenv("PCT_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Fatal("PCT_TEST_MYSQL_DSN is not set")
	}

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
			assert.Nil(t, err)

			tableInfo, ok := got[db+"."+table]
			assert.Equal(t, true, ok)

			t.Logf("%+v\n", tableInfo)

			assert.Equal(t, 0, len(tableInfo.Errors))

			assert.Equal(t, true, strings.HasPrefix(tableInfo.Create, "CREATE TABLE `user` ("))

			assert.NotNil(t, tableInfo.Status)
			assert.Equal(t, table, tableInfo.Status.Name)

			// Indexes are grouped by name (KeyName), so all the index parts of the
			// PRIMARY key should be together.
			assert.NotEmpty(t, tableInfo.Index)
			index, ok := tableInfo.Index["PRIMARY"]
			assert.Equal(t, true, ok)
			assert.Len(t, index, 2)
			assert.Equal(t, "Host", index[0].ColumnName)
			assert.Equal(t, "User", index[1].ColumnName)
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
			assert.Nil(t, err)

			tableInfo, ok := got[db+"."+table]
			assert.Equal(t, true, ok)

			t.Logf("%+v\n", tableInfo)

			assert.Equal(t, 0, len(tableInfo.Errors))

			assert.NotNil(t, tableInfo.Status)
			assert.Equal(t, table, tableInfo.Status.Name)

			var zeroTime time.Time
			assert.Equal(t, zeroTime, tableInfo.Status.CreateTime.Time)
			assert.Equal(t, zeroTime, tableInfo.Status.UpdateTime.Time)
			assert.Equal(t, zeroTime, tableInfo.Status.CheckTime.Time)
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
