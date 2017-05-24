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

package summary

import (
	"github.com/percona/pmgo"
	"github.com/percona/qan-agent/pct/cmd"
)

// Summary executes `pt-mongodb-summary` for given dsn
func Summary(dsn string) (string, error) {
	dialInfo, err := pmgo.ParseURL(dsn)
	if err != nil {
		return "", err
	}

	name := "pt-mongodb-summary"

	args := []string{}
	// add username, password and auth database e.g. `pt-mongodb-summary -u admin -p admin -a admin`
	args = append(args, authArgs(dialInfo.Username, dialInfo.Password, dialInfo.Source)...)
	// add host[:port] e.g. `pt-mongodb-summary localhost:27017`
	args = append(args, addrArgs(dialInfo.Addrs)...)

	return cmd.NewRealCmd(name, args...).Run()
}

// authArgs returns authentication arguments for cmd
func authArgs(username, password, authDatabase string) (args []string) {
	flags := map[string]string{
		"-u": username,     // -u, --username=Username to use for optional MongoDB authentication
		"-p": password,     // -p, --password[=Password to use for optional MongoDB authentication]
		"-a": authDatabase, // -a, --authenticationDatabase=Database to use for optional MongoDB authentication.
	}

	for flag, value := range flags {
		if value != "" {
			args = append(args, flag, value)
		}
	}

	return args
}

// addrArgs returns host[:port] arguments for cmd
func addrArgs(addrs []string) (args []string) {
	if len(addrs) == 0 {
		return nil
	}

	args = append(args, addrs[0])

	return args
}
