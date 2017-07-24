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

package config

import (
	"database/sql/driver"
	"fmt"
	"strings"

	pc "github.com/percona/pmm/proto/config"
	"github.com/percona/qan-agent/mysql"
)

var (
	DEFAULT_INTERVAL              uint  = 60         // 1 minute
	DEFAULT_MAX_SLOW_LOG_SIZE     int64 = 1073741824 // 1G
	DEFAULT_REMOVE_OLD_SLOW_LOGS        = true       // whether to remove old slow logs after rotation
	DEFAULT_OLD_SLOW_LOGS_TO_KEEP       = 1          // how many slow logs to keep on filesystem
	DEFAULT_EXAMPLE_QUERIES             = true
	// internal
	DEFAULT_WORKER_RUNTIME uint = 55
	DEFAULT_REPORT_LIMIT   uint = 200
)

type MySQLVarType int

const (
	MySQLVarTypeBoolean MySQLVarType = iota
	MySQLVarTypeString
	MySQLVarTypeInteger
	MySQLVarTypeNumeric
)

type MySQLVar struct {
	Name string
	Type MySQLVarType
}

var (
	mysqlVars = []MySQLVar{
		{"log_slow_admin_statements", MySQLVarTypeBoolean},
		{"log_slow_slave_statements", MySQLVarTypeBoolean},
		{"log_slow_rate_limit", MySQLVarTypeInteger},
		{"log_slow_verbosity", MySQLVarTypeString},
		{"long_query_time", MySQLVarTypeNumeric},
		{"performance_schema", MySQLVarTypeBoolean},
	}
)

func ReadInfoFromShowGlobalStatus(conn mysql.Connector) (info map[string]interface{}, err error) {
	info = map[string]interface{}{}
	for _, mysqlVar := range mysqlVars {
		var v driver.Valuer

		switch mysqlVar.Type {
		case MySQLVarTypeNumeric:
			v, err = conn.GetGlobalVarNumeric(mysqlVar.Name)
		case MySQLVarTypeInteger:
			v, err = conn.GetGlobalVarInteger(mysqlVar.Name)
		case MySQLVarTypeBoolean:
			v, err = conn.GetGlobalVarBoolean(mysqlVar.Name)
		case MySQLVarTypeString:
			v, err = conn.GetGlobalVarString(mysqlVar.Name)
		}

		if err != nil {
			return info, err
		}

		info[underscoreToCamelCase(mysqlVar.Name)], _ = v.Value()
	}

	return info, nil
}

func ValidateConfig(setConfig pc.QAN) (pc.QAN, error) {
	runConfig := pc.QAN{
		UUID:           setConfig.UUID,
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

// UnderscoreToCamelCase converts from underscore separated form to camel case form.
// Ex.: my_func => MyFunc
func underscoreToCamelCase(s string) string {
	return strings.Replace(strings.Title(strings.Replace(strings.ToLower(s), "_", " ", -1)), " ", "", -1)
}
