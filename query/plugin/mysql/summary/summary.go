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
	"net"

	"github.com/go-sql-driver/mysql"
	"github.com/percona/qan-agent/pct/cmd"
)

// Summary executes `pt-mysql-summary` for given dsn
func Summary(dsn string) (string, error) {
	cfg, err := mysql.ParseDSN(dsn)
	if err != nil {
		return "", err
	}

	name := "pt-mysql-summary"

	args := []string{}
	args = append(args, authArgs(cfg.User, cfg.Passwd)...)
	a, err := addrArgs(cfg.Net, cfg.Addr)
	if err != nil {
		return "", err
	}
	args = append(args, a...)

	return cmd.NewRealCmd(name, args...).Run()
}

// authArgs returns username and/or password arguments for cmd, e.g:
// `pt-mysql-summary --user root --password root`
func authArgs(username, password string) (args []string) {
	flags := map[string]string{
		"--user":     username,
		"--password": password,
	}

	for flag, value := range flags {
		if value != "" {
			args = append(args, flag, value)
		}
	}

	return args
}

// addrArgs returns host[:port] or socket arguments for cmd, e.g.:
// * `pt-mysql-summary --host localhost --port 27017`
// * `pt-mysql-summary --socket /tmp/mysql.sock`
func addrArgs(protocol, addr string) (args []string, err error) {
	switch protocol {
	case "tcp":
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, err
		}
		args = append(args, "--host", host)
		args = append(args, "--port", port)
	case "unix":
		args = append(args, "--socket", addr)
	}

	return args, nil
}
