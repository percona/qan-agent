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
	"io/ioutil"
	"os"
	"testing"

	"github.com/percona/pmm/proto"
	pc "github.com/percona/pmm/proto/config"
	"github.com/percona/qan-agent/instance"
	"github.com/percona/qan-agent/mysql"
	"github.com/percona/qan-agent/pct"
	"github.com/percona/qan-agent/qan/analyzer"
	"github.com/percona/qan-agent/qan/analyzer/factory"
	"github.com/percona/qan-agent/test"
	"github.com/percona/qan-agent/test/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWithRealMySQL runs tests against real MySQL server
func TestWithRealMySQL(t *testing.T) {
	dsn := os.Getenv("PCT_TEST_MYSQL_DSN")
	require.NotEmpty(t, dsn, "PCT_TEST_MYSQL_DSN is not set")

	// Init pct.Basedir
	tmpDir, err := ioutil.TempDir("/tmp", "agent-test")
	require.Nil(t, err)
	if err := pct.Basedir.Init(tmpDir); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			t.Error(err)
		}
	}()

	// Create dependencies for Manager
	logChan := make(chan proto.LogEntry, 1000)
	dataChan := make(chan interface{})
	spool := mock.NewSpooler(dataChan)
	clock := mock.NewClock()
	mrm := mock.NewMrmsMonitor()
	logger := pct.NewLogger(logChan, "TestManager_GetDefaults")
	links := map[string]string{}
	api := mock.NewAPI("http://localhost", "http://localhost", "abc-123-def", links)
	instanceRepo := instance.NewRepo(logger, "", api)
	protoInstance := proto.Instance{
		Subsystem: "mysql",
		UUID:      "3130000000000000",
		Name:      "db01",
		DSN:       dsn,
	}
	err = instanceRepo.Add(protoInstance, false)
	assert.Nil(t, err)
	defer instanceRepo.Remove(protoInstance.UUID)
	analyzerFactory := factory.New(
		logChan,
		spool,
		clock,
		mrm,
		instanceRepo,
	)

	// Write a realistic qan.conf config to disk.
	pcQANSetExpected := pc.QAN{
		UUID:          protoInstance.UUID,
		CollectFrom:   "slowlog",
		Interval:      300,
		WorkerRunTime: 270,
	}
	err = pct.Basedir.WriteConfig("qan-"+protoInstance.UUID, &pcQANSetExpected)
	assert.Nil(t, err)

	t.Run("real-mysql", func(t *testing.T) {
		testGetDefaultsBoolValues(t, logger, instanceRepo, analyzerFactory, protoInstance)
	})
}

// testGetDefaultsBoolValues verifies if value for MySQL bool variable is properly converted to json bool
// https://jira.percona.com/browse/PMM-949
func testGetDefaultsBoolValues(
	t *testing.T,
	logger *pct.Logger,
	instanceRepo *instance.Repo,
	analyzerFactory analyzer.AnalyzerFactory,
	protoInstance proto.Instance,
) {
	// Create new Manager
	m := NewManager(
		logger,
		instanceRepo,
		analyzerFactory,
	)

	// Create new MySQL connection
	conn := mysql.NewConnection(protoInstance.DSN)
	err := conn.Connect()
	require.Nil(t, err)
	defer conn.Close()

	// Start the manager and analyzer.
	err = m.Start()
	require.Nil(t, err)
	test.WaitStatus(1, m, "qan", "Running")

	type Key struct {
		db         string
		json       string
		constraint string
	}
	keys := []Key{
		{"log_slow_admin_statements", "LogSlowAdminStatements", ">= 5.6.11, != 10.0"},
		{"log_slow_slave_statements", "LogSlowSlaveStatements", ">= 5.6.11, != 10.0"},
	}

	t.Run("variables", func(t *testing.T) {
		for i := range keys {
			// create local variable
			i := i

			t.Run(keys[i].json, func(t *testing.T) {
				t.Parallel()

				// Skip testing variable if it was introduced in higher MySQL version
				if keys[i].constraint != "" {
					variableIsSupported, err := conn.VersionConstraint(keys[i].constraint)
					assert.Nil(t, err)
					if !variableIsSupported {
						t.Skipf("Variable '%s' is unsupported, it's supported in MySQL %s.", keys[i].db, keys[i].constraint)
					}
				}

				// GetDefaults returns current configuration
				// let's be sure log_slow_slave_statements=0 returns `false`
				err = conn.Set([]mysql.Query{
					{
						Set: fmt.Sprintf("SET GLOBAL %s=0", keys[i].db),
					},
				})
				require.Nil(t, err)
				got := m.GetDefaults(protoInstance.UUID)
				assert.Equal(t, false, got[keys[i].json])

				// GetDefaults returns current configuration
				// let's be sure log_slow_slave_statements=1 returns `true`
				err = conn.Set([]mysql.Query{
					{
						Set: fmt.Sprintf("SET GLOBAL %s=1", keys[i].db),
					},
				})
				require.Nil(t, err)
				got = m.GetDefaults(protoInstance.UUID)
				assert.Equal(t, true, got[keys[i].json])
			})
		}
	})

	// Stop the manager.
	err = m.Stop()
	assert.Nil(t, err)
}
