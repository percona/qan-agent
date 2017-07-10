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

package mongo

import (
	"encoding/json"

	"github.com/percona/pmm/proto"
	"github.com/percona/qan-agent/query/plugin"
	"github.com/percona/qan-agent/query/plugin/mongo/explain"
	"github.com/percona/qan-agent/query/plugin/mongo/summary"
)

// verify, at compile time, if main struct implements plugin interface
var _ plugin.Plugin = (*Mongo)(nil)

var (
	// available cmds
	cmds = map[string]execFunc{
		"Explain": execExplain,
		"Summary": execSummary,
	}
)

// Mongo handles cmds related to given instance
type Mongo struct {
	cmds map[string]execFunc
}

// New returns configured pointer *Mongo
func New() *Mongo {
	return &Mongo{
		cmds: cmds,
	}
}

// Handle executes cmd for given instance and returns resulting data
func (m *Mongo) Handle(cmd *proto.Cmd, in proto.Instance) (interface{}, error) {
	c, ok := m.cmds[cmd.Cmd]
	if !ok {
		return nil, plugin.UnknownCmdError(cmd.Cmd)
	}

	return c(cmd, in)
}

type execFunc func(cmd *proto.Cmd, in proto.Instance) (interface{}, error)

func execExplain(cmd *proto.Cmd, in proto.Instance) (interface{}, error) {
	q := &proto.ExplainQuery{}
	if err := json.Unmarshal(cmd.Data, q); err != nil {
		return nil, err
	}

	return explain.Explain(in.DSN, q.Db, q.Query)
}

func execSummary(cmd *proto.Cmd, in proto.Instance) (interface{}, error) {
	return summary.Summary(in.DSN)
}
