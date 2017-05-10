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
	"fmt"

	"github.com/percona/pmm/proto"
	"github.com/percona/qan-agent/pct"
	"github.com/percona/qan-agent/query/executor/mongo/explain"
)

type MongoExecutor struct {
}

func New() *MongoExecutor {
	e := &MongoExecutor{}
	return e
}

func (e *MongoExecutor) Handle(cmd *proto.Cmd, in proto.Instance) *proto.Reply {
	// get the dsn from instance
	dsn := in.DSN

	switch cmd.Cmd {
	case "Explain":
		q := &proto.ExplainQuery{}
		if err := json.Unmarshal(cmd.Data, q); err != nil {
			return cmd.Reply(nil, err)
		}
		res, err := explain.Explain(dsn, q.Db, q.Query)
		if err != nil {
			return cmd.Reply(nil, fmt.Errorf("EXPLAIN failed: %s", err))
		}
		return cmd.Reply(res, nil)
	default:
		return cmd.Reply(nil, pct.UnknownCmdError{Cmd: cmd.Cmd})
	}
}
