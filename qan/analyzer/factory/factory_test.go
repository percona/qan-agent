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
	"testing"

	"github.com/percona/pmm/proto"
	pc "github.com/percona/pmm/proto/config"
	"github.com/percona/qan-agent/instance"
	"github.com/percona/qan-agent/pct"
	"github.com/percona/qan-agent/test/mock"
	"github.com/stretchr/testify/assert"
)

func TestFactory_MakeMongo(t *testing.T) {
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
	analyzer, err := factory.Make(
		"mongo",
		serviceName,
		protoInstance,
	)
	assert.Nil(t, err)

	assert.Equal(t, map[string]string{serviceName: "Not running"}, analyzer.Status())
	err = analyzer.Start()
	assert.Nil(t, err)
	expect := map[string]string{
		"plugin":                   "Running",
		"plugin-collector-profile": "was: 2, slowms: 100",
	}
	actual := analyzer.Status()
	delete(actual, "plugin-collector-in")
	delete(actual, "plugin-parser-interval-start")
	delete(actual, "plugin-parser-interval-end")
	assert.Equal(t, expect, actual)
	err = analyzer.Stop()
	assert.Nil(t, err)
	assert.Equal(t, map[string]string{serviceName: "Not running"}, analyzer.Status())
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
	analyzer, err := factory.Make(
		"mysql",
		serviceName,
		protoInstance,
	)
	assert.Nil(t, err)

	pcQan := pc.QAN{
		CollectFrom: "perfschema",
	}
	analyzer.SetConfig(pcQan)

	assert.Equal(t, map[string]string{serviceName: "Not running"}, analyzer.Status())
	err = analyzer.Start()
	assert.Nil(t, err)
	expected := map[string]string{
		"plugin":               "",
		"plugin-last-interval": "",
		"plugin-next-interval": "0.0s",
		"plugin-worker":        "",
		"plugin-worker-last":   "",
	}
	assert.Equal(t, expected, analyzer.Status())
	err = analyzer.Stop()
	assert.Nil(t, err)
	assert.Equal(t, map[string]string{serviceName: "Not running"}, analyzer.Status())
}
