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

package qan

import (
	"fmt"

	pc "github.com/percona/pmm/proto/config"
	"github.com/percona/qan-agent/mysql"
	"github.com/percona/qan-agent/pct"
)

var (
	DEFAULT_COLLECT_FROM                    = "slowlog"
	DEFAULT_INTERVAL                  uint  = 60         // 1 minute
	DEFAULT_LONG_QUERY_TIME                 = 0.001      // 1ms
	DEFAULT_MAX_SLOW_LOG_SIZE         int64 = 1073741824 // 1G
	DEFAULT_REMOVE_OLD_SLOW_LOGS            = true       // whether to remove old slow logs after rotation
	DEFAULT_OLD_SLOW_LOGS_TO_KEEP           = 1          // how many slow logs to keep on filesystem
	DEFAULT_EXAMPLE_QUERIES                 = true
	DEFAULT_SLOW_LOG_VERBOSITY              = "full" // all metrics, Percona Server
	DEFAULT_RATE_LIMIT                uint  = 100    // 1%, Percona Server
	DEFAULT_LOG_SLOW_ADMIN_STATEMENTS       = true   // Percona Server
	DEFAULT_LOG_SLOW_SLAVE_STATEMENTS       = true   // Percona Server
	// internal
	DEFAULT_WORKER_RUNTIME uint = 55
	DEFAULT_REPORT_LIMIT   uint = 200
)

func ReadMySQLConfig(conn mysql.Connector) error {
	if _, err := conn.Uptime(); err != nil {
		err := conn.Connect()
		if err != nil {
			return err
		}
		defer conn.Close()
	}

	perfschemaStatus, _ := conn.GetGlobalVarString("performance_schema")
	if pct.ToBool(perfschemaStatus) {
		DEFAULT_COLLECT_FROM = "perfschema"
	}

	//
	DEFAULT_LONG_QUERY_TIME, _ = conn.GetGlobalVarNumber("long_query_time")

	//
	defaultLogSlowAdminStatements, _ := conn.GetGlobalVarString("log_slow_admin_statements")
	DEFAULT_LOG_SLOW_ADMIN_STATEMENTS = pct.ToBool(defaultLogSlowAdminStatements)

	//
	defaultRateLimit, _ := conn.GetGlobalVarNumber("log_slow_rate_limit")
	DEFAULT_RATE_LIMIT = uint(defaultRateLimit)

	//
	defaultLogSlowSlaveStatements, _ := conn.GetGlobalVarString("log_slow_slave_statements")
	DEFAULT_LOG_SLOW_SLAVE_STATEMENTS = pct.ToBool(defaultLogSlowSlaveStatements)

	//
	DEFAULT_SLOW_LOG_VERBOSITY, _ = conn.GetGlobalVarString("log_slow_verbosity")
	return nil
}

func ValidateConfig(setConfig pc.QAN) (pc.QAN, error) {
	runConfig := pc.QAN{
		UUID:           setConfig.UUID,
		CollectFrom:    DEFAULT_COLLECT_FROM,
		Interval:       DEFAULT_INTERVAL,
		MaxSlowLogSize: DEFAULT_MAX_SLOW_LOG_SIZE,
		ExampleQueries: DEFAULT_EXAMPLE_QUERIES,
		WorkerRunTime:  DEFAULT_WORKER_RUNTIME,
		ReportLimit:    DEFAULT_REPORT_LIMIT,
	}

	// Strings
	if setConfig.CollectFrom != "slowlog" && setConfig.CollectFrom != "perfschema" {
		return runConfig, fmt.Errorf("CollectFrom must be 'slowlog' or 'perfschema'")
	}
	runConfig.CollectFrom = setConfig.CollectFrom

	// Integers
	if setConfig.Interval < 0 || setConfig.Interval > 3600 {
		return runConfig, fmt.Errorf("Interval must be > 0 and <= 3600 (1 hour)")
	}
	if setConfig.Interval > 0 {
		runConfig.Interval = uint(setConfig.Interval)
	}

	runConfig.WorkerRunTime = uint(float64(runConfig.Interval) * 0.9) // 90% of interval

	return runConfig, nil
}

func GetMySQLConfig(config pc.QAN) ([]string, []string, error) {
	switch config.CollectFrom {
	case "slowlog":
		return makeSlowLogConfig()
	case "perfschema":
		return makePerfSchemaConfig()
	default:
		return nil, nil, fmt.Errorf("invalid CollectFrom: '%s'; expected 'slowlog' or 'perfschema'", config.CollectFrom)
	}
}

func makeSlowLogConfig() ([]string, []string, error) {
	on := []string{
		"SET GLOBAL slow_query_log=OFF",
		"SET GLOBAL log_output='file'", // as of MySQL 5.1.6
	}
	off := []string{
		"SET GLOBAL slow_query_log=OFF",
	}

	on = append(on,
		"SET GLOBAL slow_query_log=ON",
		"SET time_zone='+0:00'",
	)
	return on, off, nil
}

func makePerfSchemaConfig() ([]string, []string, error) {
	return []string{"SET time_zone='+0:00'"}, []string{}, nil
}
