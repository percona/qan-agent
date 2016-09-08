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
	"strconv"

	pc "github.com/percona/pmm/proto/config"
	"github.com/percona/qan-agent/mysql"
	"github.com/percona/qan-agent/pct"
)

var (
	DEFAULT_COLLECT_FROM                    = "slowlog"
	DEFAULT_INTERVAL                  uint  = 60         // 1 minute
	DEFAULT_LONG_QUERY_TIME                 = 0.001      // 1ms
	DEFAULT_MAX_SLOW_LOG_SIZE         int64 = 1073741824 // 1G
	DEFAULT_REMOVE_OLD_SLOW_LOGS            = true
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
	err := conn.Connect()
	if err != nil {
		return err
	}

	perfschemaStatus, err := conn.GetGlobalVarString("performance_schema")
	if pct.ToBool(perfschemaStatus) {
		DEFAULT_COLLECT_FROM = "perfschema"
	}

	//
	DEFAULT_LONG_QUERY_TIME, err = conn.GetGlobalVarNumber("long_query_time")
	if err != nil {
		return err
	}

	//
	defaultLogSlowAdminStatements, err := conn.GetGlobalVarString("log_slow_admin_statements")
	if err != nil {
		return err
	}
	DEFAULT_LOG_SLOW_ADMIN_STATEMENTS = pct.ToBool(defaultLogSlowAdminStatements)

	//
	defaultRateLimit, err := conn.GetGlobalVarNumber("log_slow_rate_limit")
	if err != nil {
		return err
	}
	DEFAULT_RATE_LIMIT = uint(defaultRateLimit)

	//
	defaultLogSlowSlaveStatements, err := conn.GetGlobalVarString("log_slow_slave_statements")
	if err != nil {
		return err
	}
	DEFAULT_LOG_SLOW_SLAVE_STATEMENTS = pct.ToBool(defaultLogSlowSlaveStatements)

	//
	DEFAULT_SLOW_LOG_VERBOSITY, err = conn.GetGlobalVarString("log_slow_verbosity")
	if err != nil {
		return err
	}

	return nil
}

func ValidateConfig(setConfig map[string]string) (pc.QAN, error) {
	runConfig := pc.QAN{
		UUID:                    setConfig["UUID"],
		CollectFrom:             DEFAULT_COLLECT_FROM,
		Interval:                DEFAULT_INTERVAL,
		LongQueryTime:           DEFAULT_LONG_QUERY_TIME,
		MaxSlowLogSize:          DEFAULT_MAX_SLOW_LOG_SIZE,
		RemoveOldSlowLogs:       DEFAULT_REMOVE_OLD_SLOW_LOGS,
		ExampleQueries:          DEFAULT_EXAMPLE_QUERIES,
		SlowLogVerbosity:        DEFAULT_SLOW_LOG_VERBOSITY,
		RateLimit:               DEFAULT_RATE_LIMIT,
		LogSlowAdminStatements:  DEFAULT_LOG_SLOW_ADMIN_STATEMENTS,
		LogSlowSlaveStatemtents: DEFAULT_LOG_SLOW_SLAVE_STATEMENTS,
		WorkerRunTime:           DEFAULT_WORKER_RUNTIME,
		ReportLimit:             DEFAULT_REPORT_LIMIT,
	}

	// Strings

	if val, set := setConfig["CollectFrom"]; set {
		if val != "slowlog" && val != "perfschema" {
			return runConfig, fmt.Errorf("CollectFrom must be 'slowlog' or 'perfschema'")
		}
		runConfig.CollectFrom = val
	}

	// Integers

	if val, set := setConfig["Interval"]; set {
		n, err := strconv.ParseUint(val, 10, 32)
		if err != nil {
			return runConfig, fmt.Errorf("invalid Interval: '%s': %s", val, err)
		}
		if n < 0 || n > 3600 {
			return runConfig, fmt.Errorf("Interval must be > 0 and <= 3600 (1 hour)")
		}
		runConfig.Interval = uint(n)
	}
	runConfig.WorkerRunTime = uint(float64(runConfig.Interval) * 0.9) // 90% of interval

	if val, set := setConfig["MaxSlowLogSize"]; set {
		n, err := strconv.ParseInt(val, 10, 64)
		if err != nil {
			return runConfig, fmt.Errorf("invalid MaxSlowLogSize: '%s': %s", val, err)
		}
		if n < 0 {
			return runConfig, fmt.Errorf("MaxSlowLogSize must be > 0")
		}
		runConfig.MaxSlowLogSize = n
	}

	if val, set := setConfig["ExampleQueries"]; set {
		runConfig.ExampleQueries = pct.ToBool(val)
	}

	return runConfig, nil
}
