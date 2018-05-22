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
	"encoding/json"
	"fmt"
	"strings"

	pc "github.com/percona/pmm/proto/config"
	"github.com/percona/qan-agent/mysql"
)

var (
	DefaultRemoveOldSlowLogs = true // whether to remove old slow logs after rotation
)

type MySQLVarType int

const (
	MySQLVarTypeBoolean MySQLVarType = iota
	MySQLVarTypeString
	MySQLVarTypeInteger
	MySQLVarTypeNumeric
)

var (
	mysqlVars = map[string]MySQLVarType{
		// Slowlog
		"log_slow_admin_statements":              MySQLVarTypeBoolean,
		"log_slow_slave_statements":              MySQLVarTypeBoolean,
		"log_queries_not_using_indexes":          MySQLVarTypeBoolean,
		"log_throttle_queries_not_using_indexes": MySQLVarTypeInteger,
		"log_output":                             MySQLVarTypeString, // @todo it's a set, not string
		"log_timestamps":                         MySQLVarTypeString, // @todo it's a enumeration, not string
		"slow_query_log":                         MySQLVarTypeBoolean,
		"slow_query_log_file":                    MySQLVarTypeString,
		// Percona Slowlog
		"log_slow_filter":                   MySQLVarTypeString, // @todo set, not string
		"log_slow_rate_type":                MySQLVarTypeString,
		"log_slow_rate_limit":               MySQLVarTypeInteger,
		"log_slow_sp_statements":            MySQLVarTypeBoolean,
		"log_slow_verbosity":                MySQLVarTypeString, // @todo set, not string
		"slow_query_log_use_global_control": MySQLVarTypeString, // @todo set, not string
		"slow_query_log_always_write_time":  MySQLVarTypeNumeric,

		// Performance Schema
		"performance_schema":                   MySQLVarTypeBoolean,
		"performance_schema_digests_size":      MySQLVarTypeInteger, // increments "Performance_schema_digest_lost" https://dev.mysql.com/doc/refman/5.7/en/performance-schema-status-variables.html#statvar_Performance_schema_digest_lost
		"performance_schema_max_digest_length": MySQLVarTypeInteger,

		// Common for Slowlog and Performance Schema
		"long_query_time":        MySQLVarTypeNumeric,
		"min_examined_row_limit": MySQLVarTypeInteger,
	}
)

func ReadInfoFromShowGlobalStatus(conn mysql.Connector) (info map[string]interface{}) {
	info = map[string]interface{}{}
	for mysqlVarName, mysqlVarType := range mysqlVars {
		var err error
		var v driver.Valuer

		switch mysqlVarType {
		case MySQLVarTypeNumeric:
			v, err = conn.GetGlobalVarNumeric(mysqlVarName)
		case MySQLVarTypeInteger:
			v, err = conn.GetGlobalVarInteger(mysqlVarName)
		case MySQLVarTypeBoolean:
			v, err = conn.GetGlobalVarBoolean(mysqlVarName)
		case MySQLVarTypeString:
			v, err = conn.GetGlobalVarString(mysqlVarName)
		}

		var msg interface{}
		switch err {
		case nil:
			msg, _ = v.Value()
		default:
			msg = fmt.Errorf("unable to read global variable: %s", err)
		}
		info[underscoreToCamelCase(mysqlVarName)] = msg
	}

	return info
}

func ValidateConfig(setConfig pc.QAN) (pc.QAN, error) {
	runConfig := pc.NewQAN()
	fmt.Printf("%+v\n", runConfig)

	// Marshal setConfig and unmarshal it back on default config.
	// This way we keep defaults if they are not set in setConfig.
	b, err := json.Marshal(setConfig)
	if err != nil {
		return runConfig, err
	}
	err = json.Unmarshal(b, &runConfig)
	if err != nil {
		return runConfig, err
	}
	fmt.Printf("%+v\n", runConfig)

	// Set UUID.
	runConfig.UUID = setConfig.UUID

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
		runConfig.Interval = setConfig.Interval
	}

	return runConfig, nil
}

// UnderscoreToCamelCase converts from underscore separated form to camel case form.
// Ex.: my_func => MyFunc
func underscoreToCamelCase(s string) string {
	return strings.Replace(strings.Title(strings.Replace(strings.ToLower(s), "_", " ", -1)), " ", "", -1)
}
