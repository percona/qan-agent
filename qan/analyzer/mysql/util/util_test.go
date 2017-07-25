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

package util

import (
	"testing"

	pc "github.com/percona/pmm/proto/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSlowLogMySQLBasic(t *testing.T) {
	on, off, err := GetMySQLConfig(pc.QAN{CollectFrom: "slowlog"})
	require.NoError(t, err)
	assert.Equal(t, []string{
		"SET GLOBAL slow_query_log=OFF",
		"SET GLOBAL log_output='file'",
		"SET GLOBAL slow_query_log=ON",
		"SET time_zone='+0:00'",
	}, on)
	assert.Equal(t, []string{
		"SET GLOBAL slow_query_log=OFF",
	}, off)
}
