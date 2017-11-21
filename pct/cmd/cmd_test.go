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

package cmd_test

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/percona/qan-agent/pct/cmd"
	. "gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { TestingT(t) }

type TestSuite struct {
}

var _ = Suite(&TestSuite{})

// --------------------------------------------------------------------------

func (s *TestSuite) TestCmdNotFound(t *C) {
	unknownCmd := cmd.NewRealCmd("unknown-cmd")
	output, err := unknownCmd.Run()
	t.Assert(output, Equals, "")
	t.Assert(err, Equals, cmd.ErrNotFound)
}

func (s *TestSuite) TestCmdRedirectOutput(t *C) {
	// we are going to run cat file > file1
	// 1st step is to create a temp file and put some content in it
	content := []byte("I am the man with no name. Zapp Brannigan, at your service.")
	tmpfile, err := ioutil.TempFile("", "test000")
	t.Assert(err, Equals, nil)

	defer os.Remove(tmpfile.Name())

	_, err = tmpfile.Write(content)
	t.Assert(err, IsNil)

	err = tmpfile.Close()
	t.Assert(err, IsNil)

	// This is the file where we are going to redirect cat's output
	tmpRedirFile, err := ioutil.TempFile("", "test000")
	t.Assert(err, IsNil)

	cat := cmd.NewRealCmd("cat", tmpfile.Name(), "> "+tmpRedirFile.Name())
	output, err := cat.Run()
	t.Assert(output, Not(Equals), "")
	t.Assert(err, IsNil)

	gotContent, err := ioutil.ReadFile(output)
	t.Assert(string(content), Equals, string(gotContent))
	os.Remove(output)
}
