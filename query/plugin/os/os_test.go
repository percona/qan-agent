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

package os

import (
	"fmt"
	"testing"

	"github.com/percona/pmm/proto"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHandle(t *testing.T) {
	t.Parallel()

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
		// Summary
		{
			func() (*proto.Cmd, proto.Instance) {
				cmd := &proto.Cmd{
					Cmd: "Summary",
				}
				in := proto.Instance{}
				return cmd, in
			},
			func(data interface{}, err error) {
				require.NoError(t, err)
				assert.Regexp(t, "# Percona Toolkit System Summary Report #", data)
			},
		},
	}

	o := New()
	t.Run("t.Parallel()", func(t *testing.T) {
		for i, f := range fs {
			cmd, in := f.provider()
			name := fmt.Sprintf("%d/%s/%s", i, cmd.Cmd, in.DSN)
			t.Run(name, func(t *testing.T) {
				t.Parallel()

				cmd, in := f.provider()
				f.test(o.Handle(cmd, in))
			})
		}
	})
}
