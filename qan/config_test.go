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

package qan_test

import (
	pc "github.com/percona/pmm/proto/config"
	"github.com/percona/qan-agent/qan"
	. "gopkg.in/check.v1"
)

type ConfigTestSuite struct{}

var _ = Suite(&ConfigTestSuite{})

func (s *ConfigTestSuite) TestSlowLogMySQLBasic(t *C) {
	on, off, err := qan.GetMySQLConfig(pc.QAN{CollectFrom: "slowlog"})
	t.Assert(err, IsNil)
	t.Check(on, DeepEquals, []string{
		"SET GLOBAL slow_query_log=OFF",
		"SET GLOBAL log_output='file'",
		"SET GLOBAL slow_query_log=ON",
		"SET time_zone='+0:00'",
	})
	t.Check(off, DeepEquals, []string{
		"SET GLOBAL slow_query_log=OFF",
	})
}
