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

package mysql

import (
	"encoding/json"
	"fmt"

	"github.com/percona/pmm/proto"
	"github.com/percona/qan-agent/mysql"
	"github.com/percona/qan-agent/pct"
	"github.com/percona/qan-agent/query/executor/mysql/explain"
	"github.com/percona/qan-agent/query/executor/mysql/tableinfo"
)

type QueryExecutor struct {
	connFactory mysql.ConnectionFactory
}

func New() *QueryExecutor {
	mysqlConnFactory := &mysql.RealConnectionFactory{}

	e := &QueryExecutor{
		connFactory: mysqlConnFactory,
	}
	return e
}

func (m *QueryExecutor) Handle(cmd *proto.Cmd, in proto.Instance) *proto.Reply {
	conn := m.connFactory.Make(in.DSN)
	if err := conn.Connect(); err != nil {
		return cmd.Reply(nil, err)
	}
	defer conn.Close()

	// Execute the query.
	switch cmd.Cmd {
	case "Explain":
		q := &proto.ExplainQuery{}
		if err := json.Unmarshal(cmd.Data, q); err != nil {
			return cmd.Reply(nil, err)
		}
		res, err := explain.Explain(conn, q.Db, q.Query, q.Convert)
		if err != nil {
			return cmd.Reply(nil, fmt.Errorf("EXPLAIN failed: %s", err))
		}
		return cmd.Reply(res, nil)
	case "TableInfo":
		tableInfo := &proto.TableInfoQuery{}
		if err := json.Unmarshal(cmd.Data, tableInfo); err != nil {
			return cmd.Reply(nil, err)
		}
		res, err := tableinfo.TableInfo(conn, tableInfo)
		if err != nil {
			return cmd.Reply(nil, fmt.Errorf("Table Info failed: %s", err))
		}
		return cmd.Reply(res, nil)
	default:
		return cmd.Reply(nil, pct.UnknownCmdError{Cmd: cmd.Cmd})
	}
}
