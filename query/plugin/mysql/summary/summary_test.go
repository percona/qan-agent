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

package summary

import (
	"os"
	"testing"

	"github.com/percona/qan-agent/mysql"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSummary(t *testing.T) {
	// PMM-2569
	// https://github.com/percona/qan-agent/pull/90
	// https://github.com/percona/percona-toolkit/pull/337
	{
		dsn := os.Getenv("PCT_TEST_MYSQL_DSN")
		require.NotEmpty(t, dsn, "PCT_TEST_MYSQL_DSN is not set")
		mysqlConn := mysql.NewConnection(dsn)
		err := mysqlConn.Connect()
		require.NoError(t, err)
		defer mysqlConn.Close()

		ok, err := mysqlConn.VersionConstraint("< 8.0 || >= 10.0")
		require.NoError(t, err)
		if !ok {
			t.Skip(
				"pt-mysql-summary runs host mysql client which doesn't work with MySQL 8.",
			)
		}
	}

	t.Parallel()

	dsn := os.Getenv("PCT_TEST_MYSQL_DSN")
	require.NotEmpty(t, dsn, "PCT_TEST_MYSQL_DSN is not set")

	output, err := Summary(dsn)
	require.NoError(t, err, "output: %s", output)

	assert.Regexp(t, "# Percona Toolkit MySQL Summary Report #", output)
}
