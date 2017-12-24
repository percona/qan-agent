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

package factory

import (
	"fmt"
	"testing"

	"github.com/percona/pmgo"
	"github.com/percona/pmm/proto"
	pc "github.com/percona/pmm/proto/config"
	"github.com/percona/qan-agent/instance"
	"github.com/percona/qan-agent/pct"
	"github.com/percona/qan-agent/test/mock"
	"github.com/percona/qan-agent/test/profiling"
	"github.com/percona/qan-agent/test/version"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/mgo.v2"
)

func TestFactory_MakeMongo(t *testing.T) {
	t.Parallel()

	dialer := pmgo.NewDialer()
	dialInfo, _ := pmgo.ParseURL("")

	session, err := dialer.DialWithInfo(dialInfo)
	require.NoError(t, err)
	defer session.Close()
	session.SetMode(mgo.Eventual, true)
	bi, err := session.BuildInfo()
	require.NoError(t, err)
	atLeast34, err := version.Constraint(">= 3.4", bi.Version)
	require.NoError(t, err)

	dbNames := []string{
		"local",
		"test",
	}
	if atLeast34 {
		dbNames = append(dbNames, "admin")
	}

	// disable profiling as we only want to test if factory works
	for _, dbName := range dbNames {
		url := "/" + dbName
		err := profiling.Disable(url)
		require.NoError(t, err)
	}

	logChan := make(chan proto.LogEntry)
	dataChan := make(chan interface{})
	spool := mock.NewSpooler(dataChan)
	clock := mock.NewClock()
	mrm := mock.NewMrmsMonitor()
	logger := pct.NewLogger(logChan, "TestFactory_Make")
	links := map[string]string{}
	api := mock.NewAPI("http://localhost", "http://localhost", "abc-123-def", links)
	instanceRepo := instance.NewRepo(logger, "", api)
	factory := New(
		logChan,
		spool,
		clock,
		mrm,
		instanceRepo,
	)
	protoInstance := proto.Instance{}
	serviceName := "plugin"
	plugin, err := factory.Make(
		"mongo",
		serviceName,
		protoInstance,
	)
	require.NoError(t, err)

	assert.Equal(t, map[string]string{serviceName: "Not running"}, plugin.Status())
	err = plugin.Start()
	require.NoError(t, err)
	// some values are unpredictable, e.g. time but they should exist
	shouldExist := "<should exist>"

	pluginName := "plugin"
	expect := map[string]string{
		pluginName: "Running",
	}
	for _, dbName := range dbNames {
		t := map[string]string{
			"%s-collector-profile":                  "Profiling disabled. Please enable profiling for this database or whole MongoDB server (https://docs.mongodb.com/manual/tutorial/manage-the-database-profiler/).",
			"%s-collector-iterator-counter":         "1",
			"%s-collector-iterator-restart-counter": shouldExist,
			"%s-collector-iterator-created":         shouldExist,
			"%s-collector-started":                  shouldExist,
			"%s-collector-servers":                  shouldExist,
			"%s-parser-started":                     shouldExist,
			"%s-parser-interval-start":              shouldExist,
			"%s-parser-interval-end":                shouldExist,
			"%s-sender-started":                     shouldExist,
		}
		m := map[string]string{}
		for k, v := range t {
			prefix := fmt.Sprintf("%s-%s", pluginName, dbName)
			key := fmt.Sprintf(k, prefix)
			m[key] = v
		}
		expect = merge(expect, m)
	}

	actual := plugin.Status()
	for k, v := range expect {
		if v == shouldExist {
			assert.Contains(t, actual, k)
			delete(actual, k)
			delete(expect, k)
		}
	}
	assert.Equal(t, expect, actual)
	err = plugin.Stop()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{serviceName: "Not running"}, plugin.Status())
}

func TestFactory_MakeMySQL(t *testing.T) {
	t.Parallel()

	logChan := make(chan proto.LogEntry)
	dataChan := make(chan interface{})
	spool := mock.NewSpooler(dataChan)
	clock := mock.NewClock()
	mrm := mock.NewMrmsMonitor()
	logger := pct.NewLogger(logChan, "TestFactory_Make")
	links := map[string]string{}
	api := mock.NewAPI("http://localhost", "http://localhost", "abc-123-def", links)
	instanceRepo := instance.NewRepo(logger, "", api)
	factory := New(
		logChan,
		spool,
		clock,
		mrm,
		instanceRepo,
	)
	protoInstance := proto.Instance{}
	serviceName := "plugin"
	plugin, err := factory.Make(
		"mysql",
		serviceName,
		protoInstance,
	)
	require.NoError(t, err)

	pcQan := pc.QAN{
		CollectFrom: "perfschema",
	}
	plugin.SetConfig(pcQan)

	assert.Equal(t, map[string]string{serviceName: "Not running"}, plugin.Status())
	err = plugin.Start()
	require.NoError(t, err)
	expected := map[string]string{
		"plugin":                "",
		"plugin-last-interval":  "",
		"plugin-next-interval":  "0.0s",
		"plugin-worker":         "",
		"plugin-worker-last":    "",
		"plugin-worker-digests": "",
	}
	assert.Equal(t, expected, plugin.Status())
	err = plugin.Stop()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{serviceName: "Not running"}, plugin.Status())
}

// merge merges map[string]string maps
func merge(maps ...map[string]string) map[string]string {
	result := make(map[string]string)
	for _, m := range maps {
		for k, v := range m {
			result[k] = v
		}
	}
	return result
}
