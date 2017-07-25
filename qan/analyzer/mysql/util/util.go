package util

import (
	"fmt"

	pc "github.com/percona/pmm/proto/config"
)

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
