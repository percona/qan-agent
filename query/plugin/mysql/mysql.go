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

	"github.com/percona/pmm/proto"
	"github.com/percona/qan-agent/mysql"
	"github.com/percona/qan-agent/query/plugin"
	"github.com/percona/qan-agent/query/plugin/mysql/explain"
	"github.com/percona/qan-agent/query/plugin/mysql/summary"
	"github.com/percona/qan-agent/query/plugin/mysql/tableinfo"
)

// verify, at compile time, if main struct implements plugin interface
var _ plugin.Plugin = (*MySQL)(nil)

// MySQL handles cmds related to given instance
type MySQL struct {
	connFactory mysql.ConnectionFactory
	cmds        map[string]execFunc
}

// New returns configured pointer *MySQL
func New() *MySQL {
	m := &MySQL{}
	m.connFactory = &mysql.RealConnectionFactory{}
	m.cmds = map[string]execFunc{
		"Explain":   m.explain,
		"TableInfo": m.tableInfo,
		"Summary":   m.summary,
	}

	return m
}

// Handle executes cmd for given instance and returns resulting data
func (m *MySQL) Handle(cmd *proto.Cmd, in proto.Instance) (interface{}, error) {
	c, ok := m.cmds[cmd.Cmd]
	if !ok {
		return nil, plugin.UnknownCmdError(cmd.Cmd)
	}

	return c(cmd, in)
}

type execFunc func(cmd *proto.Cmd, in proto.Instance) (interface{}, error)

func (m *MySQL) explain(cmd *proto.Cmd, in proto.Instance) (interface{}, error) {
	conn := m.connFactory.Make(in.DSN)
	if err := conn.Connect(); err != nil {
		return nil, err
	}
	defer conn.Close()

	q := &proto.ExplainQuery{}
	if err := json.Unmarshal(cmd.Data, q); err != nil {
		return nil, err
	}

	return explain.Explain(conn, q.Db, q.Query, q.Convert)
}

func (m *MySQL) tableInfo(cmd *proto.Cmd, in proto.Instance) (interface{}, error) {
	conn := m.connFactory.Make(in.DSN)
	if err := conn.Connect(); err != nil {
		return nil, err
	}
	defer conn.Close()

	tableInfo := &proto.TableInfoQuery{}
	if err := json.Unmarshal(cmd.Data, tableInfo); err != nil {
		return nil, err
	}

	return tableinfo.TableInfo(conn, tableInfo)
}

func (m *MySQL) summary(cmd *proto.Cmd, in proto.Instance) (interface{}, error) {
	return summary.Summary(in.DSN)
}
