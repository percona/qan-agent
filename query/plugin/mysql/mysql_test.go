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
	"os"
	"testing"

	"github.com/percona/pmm/proto"
	"github.com/stretchr/testify/assert"
)

func TestHandle(t *testing.T) {
	t.Parallel()

	dsn := os.Getenv("PCT_TEST_MYSQL_DSN")
	assert.NotEmpty(t, dsn, "PCT_TEST_MYSQL_DSN is not set")

	m := New()

	fs := []struct {
		provider func() (cmd *proto.Cmd, in proto.Instance)
		test     func(data interface{}, err error)
	}{
		// Unknown cmd
		{
			func() (*proto.Cmd, proto.Instance) {
				cmd := &proto.Cmd{
					Cmd: "Unknown",
				}
				in := proto.Instance{}
				return cmd, in
			},
			func(data interface{}, err error) {
				assert.Error(t, err, "cmd Unknown doesn't exist")
				assert.Nil(t, data)
			},
		},
		// Explain
		{
			func() (*proto.Cmd, proto.Instance) {
				q := &proto.ExplainQuery{
					Db:    "mysql",
					Query: `SELECT 1`,
				}
				data, err := json.Marshal(q)
				assert.Nil(t, err)
				cmd := &proto.Cmd{
					Cmd:  "Explain",
					Data: data,
				}
				in := proto.Instance{
					DSN: dsn,
				}
				return cmd, in
			},
			func(data interface{}, err error) {
				assert.Nil(t, err)

				// unpack data
				explainResult := data.(*proto.ExplainResult)
				assert.NotEmpty(t, explainResult.JSON)
				assert.NotEmpty(t, explainResult.Classic)
			},
		},
		// Summary
		{
			func() (*proto.Cmd, proto.Instance) {
				cmd := &proto.Cmd{
					Cmd: "Summary",
				}
				in := proto.Instance{
					DSN: dsn,
				}
				return cmd, in
			},
			func(data interface{}, err error) {
				assert.Nil(t, err)
				assert.Regexp(t, "# Percona Toolkit MySQL Summary Report #", data)
			},
		},
		// TableInfo
		{
			func() (*proto.Cmd, proto.Instance) {
				db := "mysql"
				table := "user"
				tables := &proto.TableInfoQuery{
					Create: []proto.Table{{db, table}},
					Index:  []proto.Table{{db, table}},
					Status: []proto.Table{{db, table}},
				}
				data, err := json.Marshal(tables)
				assert.Nil(t, err)
				cmd := &proto.Cmd{
					Cmd:  "TableInfo",
					Data: data,
				}

				in := proto.Instance{
					DSN: dsn,
				}
				return cmd, in
			},
			func(data interface{}, err error) {
				assert.Nil(t, err)
				assert.NotEmpty(t, data)
			},
		},
	}

	t.Run("t.Parallel()", func(t *testing.T) {
		for i, f := range fs {
			cmd, in := f.provider()
			name := fmt.Sprintf("%d/%s/%s", i, cmd.Cmd, in.DSN)
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				cmd, in := f.provider()
				f.test(m.Handle(cmd, in))
			})
		}
	})
}
