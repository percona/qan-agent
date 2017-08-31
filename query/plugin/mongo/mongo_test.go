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
	"testing"

	"github.com/percona/pmm/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/mgo.v2/bson"
)

func TestHandle(t *testing.T) {
	t.Parallel()

	m := New()

	in := proto.Instance{}

	cmdUnknow := &proto.Cmd{
		Cmd: "Unknown",
	}

	cmdSummary := &proto.Cmd{
		Cmd: "Summary",
	}

	q := &proto.ExplainQuery{
		Db:    "test",
		Query: `{"ns":"test.col1","op":"query","query":{"find":"col1","filter":{"name":"Alicja"}}}`,
	}
	data, err := json.Marshal(q)
	require.NoError(t, err)
	cmdExplain := &proto.Cmd{
		Cmd:  "Explain",
		Data: data,
	}

	fs := []struct {
		cmd  *proto.Cmd
		in   proto.Instance
		test func(data interface{}, err error)
	}{
		// Unknown cmd
		{
			cmdUnknow, in,
			func(data interface{}, err error) {
				assert.Error(t, err, "cmd Unknown doesn't exist")
				assert.Nil(t, data)
			},
		},
		// Summary
		{
			cmdSummary, in,
			func(data interface{}, err error) {
				require.NoError(t, err)
				assert.Regexp(t, "# Instances #", data)
			},
		},
		// Explain
		{
			cmdExplain,
			proto.Instance{
				DSN: "127.0.0.1:27017",
			},
			func(data interface{}, err error) {
				require.NoError(t, err)

				// unpack data
				explainResult := data.(*proto.ExplainResult)
				got := bson.M{}
				err = bson.UnmarshalJSON([]byte(explainResult.JSON), &got)

				// check structure of the result
				assert.NotEmpty(t, got["executionStats"])
				assert.NotEmpty(t, got["queryPlanner"])
				assert.NotEmpty(t, got["serverInfo"])
				assert.NotEmpty(t, got["ok"])
				assert.Len(t, got, 4)
			},
		},
	}
	for _, f := range fs {
		f.test(m.Handle(f.cmd, f.in))
	}
}
