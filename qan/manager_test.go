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

package qan_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/percona/pmm/proto"
	pc "github.com/percona/pmm/proto/config"
	"github.com/percona/qan-agent/instance"
	"github.com/percona/qan-agent/pct"
	"github.com/percona/qan-agent/qan"
	"github.com/percona/qan-agent/test"
	"github.com/percona/qan-agent/test/mock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	. "gopkg.in/check.v1"
)

// Hook up gocheck into the "go test" runner.
func Test(t *testing.T) { TestingT(t) }

type ManagerTestSuite struct {
	logChan       chan proto.LogEntry
	logger        *pct.Logger
	dataChan      chan interface{}
	spool         *mock.Spooler
	tmpDir        string
	configDir     string
	im            *instance.Repo
	api           *mock.API
	protoInstance proto.Instance
	instanceUUID  string
}

var _ = Suite(&ManagerTestSuite{})

func (s *ManagerTestSuite) SetUpSuite(t *C) {

	s.logChan = make(chan proto.LogEntry, 1000)
	s.logger = pct.NewLogger(s.logChan, "qan-test")

	s.dataChan = make(chan interface{}, 2)
	s.spool = mock.NewSpooler(s.dataChan)

	var err error
	s.tmpDir, err = ioutil.TempDir("/tmp", "agent-test")
	t.Assert(err, IsNil)

	links := map[string]string{
		"agents":    "/agents",
		"instances": "/instances",
	}
	s.api = mock.NewAPI("localhost", "http://localhost", "212", links)

	if err := pct.Basedir.Init(s.tmpDir); err != nil {
		t.Fatal(err)
	}
	s.configDir = pct.Basedir.Dir("config")
	s.im = instance.NewRepo(pct.NewLogger(s.logChan, "manager-test"), s.configDir, s.api)
	s.instanceUUID = "3130000000000000"
	s.protoInstance = proto.Instance{
		Subsystem: "mysql",
		UUID:      s.instanceUUID,
		Name:      "db01",
		DSN:       "user:pass@tcp(localhost)/",
	}

	err = s.im.Init()
	t.Assert(err, IsNil)
}

func (s *ManagerTestSuite) SetUpTest(t *C) {
	if err := test.ClearDir(pct.Basedir.Dir("config"), "*"); err != nil {
		t.Fatal(err)
	}
	for _, in := range s.im.List("mysql") {
		s.im.Remove(in.UUID)
	}
	err := s.im.Add(s.protoInstance, true)
	t.Assert(err, IsNil)
}

func (s *ManagerTestSuite) TearDownSuite(t *C) {
	if err := os.RemoveAll(s.tmpDir); err != nil {
		t.Error(err)
	}
}

// --------------------------------------------------------------------------

func (s *ManagerTestSuite) TestStarNoConfig(t *C) {
	// Make a qan.Manager with mock factories.
	a := mock.NewQanAnalyzer("qan-analizer-1")
	f := mock.NewQanAnalyzerFactory(a)
	m := qan.NewManager(s.logger, s.im, f)
	t.Assert(m, NotNil)

	// qan.Manager should be able to start without a qan.conf, i.e. no analyzer.
	err := m.Start()
	t.Check(err, IsNil)

	// Wait for qan.Manager.Start() to finish.
	test.WaitStatus(1, m, "qan", "Running")

	// No analyzer is configured, so the mock analyzer should not be started.
	select {
	case <-a.StartChan:
		t.Error("Analyzer.Start() called")
	default:
	}

	// And the mock analyzer's status should not be reported.
	status := m.Status()
	t.Check(status["qan"], Equals, "Running")

	// Stop the manager.
	err = m.Stop()
	t.Assert(err, IsNil)

	// No analyzer is configured, so the mock analyzer should not be stop.
	select {
	case <-a.StartChan:
		t.Error("Analyzer.Stop() called")
	default:
	}
}

func (s *ManagerTestSuite) TestStartWithConfig(t *C) {
	mi2 := s.protoInstance
	mi2.UUID = "2220000000000000"
	mi2.Name = "db02"
	err := s.im.Add(mi2, false)
	t.Assert(err, IsNil)
	defer s.im.Remove(mi2.UUID)

	// Get MySQL instances
	mysqlInstances := s.im.List("mysql")
	t.Assert(len(mysqlInstances), Equals, 2)

	// Make a qan.Manager with mock factories.
	a1 := mock.NewQanAnalyzer(fmt.Sprintf("qan-analyzer-%s", mysqlInstances[0].Name))
	a2 := mock.NewQanAnalyzer(fmt.Sprintf("qan-analyzer-%s", mysqlInstances[1].Name))
	f := mock.NewQanAnalyzerFactory(a1, a2)
	m := qan.NewManager(s.logger, s.im, f)
	t.Assert(m, NotNil)
	configs := make([]pc.QAN, 0)
	for i, analyzerType := range []string{"slowlog", "perfschema"} {
		// We have two analyzerTypes and two MySQL instances in fixture, lets re-use the index
		// as we only need one of each analizer type and they need to be different instances.
		mysqlInstance := mysqlInstances[i]
		// Write a realistic qan.conf config to disk.
		exampleQueries := true
		config := pc.QAN{
			UUID:           mysqlInstance.UUID,
			CollectFrom:    analyzerType,
			Interval:       300,
			WorkerRunTime:  270,
			ExampleQueries: &exampleQueries, // specify optional args
			ReportLimit:    200,             // specify optional args
		}
		err := pct.Basedir.WriteConfig("qan-"+mysqlInstance.UUID, &config)
		t.Assert(err, IsNil)
		configs = append(configs, config)
	}
	// qan.Start() reads qan configs from disk and starts an analyzer for each one.
	err = m.Start()
	t.Check(err, IsNil)

	// Wait until qan.Start() calls analyzer.Start().
	if !test.WaitState(a1.StartChan) {
		t.Fatal("Timeout waiting for <-a1.StartChan")
	}
	if !test.WaitState(a2.StartChan) {
		t.Fatal("Timeout waiting for <-a2.StartChan")
	}

	// After starting, the manager's status should be Running and the analyzer's
	// status should be reported too.
	status := m.Status()
	t.Check(status["qan"], Equals, "Running")
	t.Check(status["qan-analyzer"], Equals, "ok")

	// Check the args passed by the manager to the analyzer factory.
	if len(f.Args) == 0 {
		t.Error("len(f.Args) == 0, expected 2")
	} else {
		t.Check(f.Args, HasLen, 2)

		argConfigs := []pc.QAN{
			a1.Config(),
			a2.Config(),
		}
		assert.Equal(t, configs, argConfigs)
		t.Check(f.Args[0].Name, Equals, "qan-analyzer-mysql-22200000")
		t.Check(f.Args[1].Name, Equals, "qan-analyzer-mysql-31300000")
	}

	// qan.Stop() stops the analyzer and leaves qan.conf on disk.
	err = m.Stop()
	t.Assert(err, IsNil)

	// Wait until qan.Stop() calls analyzer.Stop().
	if !test.WaitState(a1.StopChan) {
		t.Fatal("Timeout waiting for <-a.StopChan")
	}

	// Wait until qan.Stop() calls analyzer.Stop().
	if !test.WaitState(a2.StopChan) {
		t.Fatal("Timeout waiting for <-a.StopChan")
	}

	// qan.conf still exists after qan.Stop().
	for _, mysqlInstance := range s.im.List("mysql") {
		t.Check(test.FileExists(pct.Basedir.ConfigFile("qan-"+mysqlInstance.UUID)), Equals, true)
	}

	// The analyzer is no longer reported in the status because it was stopped
	// and removed when the manager was stopped.
	status = m.Status()
	t.Check(status["qan"], Equals, "Stopped")
}

func (s *ManagerTestSuite) TestStart2RemoteQAN(t *C) {
	mi2 := s.protoInstance
	mi2.UUID = "2220000000000000"
	mi2.Name = "db02"
	err := s.im.Add(mi2, false)
	t.Assert(err, IsNil)
	defer s.im.Remove(mi2.UUID)

	// Get MYySQL instances
	mysqlInstances := s.im.List("mysql")
	t.Assert(len(mysqlInstances), Equals, 2)

	// Make a qan.Manager with mock factories.
	a1 := mock.NewQanAnalyzer(fmt.Sprintf("qan-analyzer-mysql-%s", mysqlInstances[0].Name))
	a2 := mock.NewQanAnalyzer(fmt.Sprintf("qan-analyzer-mysql-%s", mysqlInstances[1].Name))
	f := mock.NewQanAnalyzerFactory(a1, a2)
	m := qan.NewManager(s.logger, s.im, f)
	t.Assert(m, NotNil)
	configs := make([]pc.QAN, 0)
	for _, mysqlInstance := range mysqlInstances {
		// Write a realistic qan.conf config to disk.
		exampleQueries := true
		config := pc.QAN{
			UUID:           mysqlInstance.UUID,
			CollectFrom:    "perfschema",
			Interval:       300,
			WorkerRunTime:  270,
			ExampleQueries: &exampleQueries, // specify optional args
			ReportLimit:    200,             // specify optional args
		}
		err := pct.Basedir.WriteConfig("qan-"+mysqlInstance.UUID, &config)
		t.Assert(err, IsNil)
		configs = append(configs, config)
	}
	// qan.Start() reads qan configs from disk and starts an analyzer for each one.
	err = m.Start()
	t.Check(err, IsNil)

	// Wait until qan.Start() calls analyzer.Start().
	if !test.WaitState(a1.StartChan) {
		t.Fatal("Timeout waiting for <-a1.StartChan")
	}
	if !test.WaitState(a2.StartChan) {
		t.Fatal("Timeout waiting for <-a2.StartChan")
	}

	// After starting, the manager's status should be Running and the analyzer's
	// status should be reported too.
	status := m.Status()
	t.Check(status["qan"], Equals, "Running")
	t.Check(status["qan-analyzer"], Equals, "ok")

	// Check the args passed by the manager to the analyzer factory.
	if len(f.Args) == 0 {
		t.Error("len(f.Args) == 0, expected 2")
	} else {
		t.Check(f.Args, HasLen, 2)

		argConfigs := []pc.QAN{
			a1.Config(),
			a2.Config(),
		}
		assert.Equal(t, configs, argConfigs)
		t.Check(f.Args[0].Name, Equals, "qan-analyzer-mysql-22200000")
		t.Check(f.Args[1].Name, Equals, "qan-analyzer-mysql-31300000")
	}

	// qan.Stop() stops the analyzer and leaves qan.conf on disk.
	err = m.Stop()
	t.Assert(err, IsNil)

	// Wait until qan.Stop() calls analyzer.Stop().
	if !test.WaitState(a1.StopChan) {
		t.Fatal("Timeout waiting for <-a.StopChan")
	}

	// Wait until qan.Stop() calls analyzer.Stop().
	if !test.WaitState(a2.StopChan) {
		t.Fatal("Timeout waiting for <-a.StopChan")
	}

	// qan.conf still exists after qan.Stop().
	for _, mysqlInstance := range s.im.List("mysql") {
		t.Check(test.FileExists(pct.Basedir.ConfigFile("qan-"+mysqlInstance.UUID)), Equals, true)
	}

	// The analyzer is no longer reported in the status because it was stopped
	// and removed when the manager was stopped.
	status = m.Status()
	t.Check(status["qan"], Equals, "Stopped")

}

func (s *ManagerTestSuite) TestGetConfig(t *C) {

	// Make a qan.Manager with mock factories.
	a := mock.NewQanAnalyzer("qan-analizer-1")
	f := mock.NewQanAnalyzerFactory(a)
	m := qan.NewManager(s.logger, s.im, f)
	t.Assert(m, NotNil)

	mysqlInstances := s.im.List("mysql")
	t.Assert(len(mysqlInstances), Equals, 1)
	mysqlUUID := mysqlInstances[0].UUID

	// Write a realistic qan.conf config to disk.
	pcQANSetExpected := pc.QAN{
		UUID:          mysqlUUID,
		CollectFrom:   "slowlog",
		Interval:      300,
		WorkerRunTime: 270,
	}
	err := pct.Basedir.WriteConfig("qan-"+mysqlUUID, &pcQANSetExpected)
	t.Assert(err, IsNil)

	// Start the manager and analyzer.
	err = m.Start()
	t.Check(err, IsNil)
	test.WaitStatus(1, m, "qan", "Running")

	// Set different config in an Analyzer mock
	// to emulate that config changed after running manager
	// This tests that manager can return initial setConfig and runningConfig
	exampleQueries := true
	pcQANRunningExpected := pcQANSetExpected
	pcQANRunningExpected.ReportLimit = 10
	pcQANRunningExpected.ExampleQueries = &exampleQueries
	a.SetConfig(pcQANRunningExpected)

	// Get the manager config which should be just the analyzer config.
	gotConfig, errs := m.GetConfig()
	t.Assert(errs, HasLen, 0)
	t.Assert(gotConfig, HasLen, 1)

	pcQANSet := pc.QAN{}
	err = json.Unmarshal([]byte(gotConfig[0].Set), &pcQANSet)
	require.NoError(t, err)
	assert.Equal(t, pcQANSetExpected, pcQANSet)

	pcQANRunning := pc.QAN{}
	err = json.Unmarshal([]byte(gotConfig[0].Running), &pcQANRunning)
	require.NoError(t, err)
	assert.Equal(t, pcQANRunningExpected, pcQANRunning)

	// We checked json structure earlier, we don't compare json as string because properties can be in unknown order
	gotConfig[0].Set = ""
	gotConfig[0].Running = ""

	expect := []proto.AgentConfig{
		{
			Service: "qan",
			UUID:    mysqlUUID,
		},
	}
	assert.Equal(t, expect, gotConfig)

	// Stop the manager.
	err = m.Stop()
	t.Assert(err, IsNil)
}

func (s *ManagerTestSuite) TestAddInstance(t *C) {
	// FAIL: manager_test.go:446: ManagerTestSuite.TestAddInstance
	// manager_test.go:494:
	// t.Assert(reply.Error, Equals, "")
	// ... obtained string = "cannot get MySQL instance 3130000000009999: Cannot read instance file: /tmp/agent-test398904814/config/3130000000009999.json: open /tmp/agent-test398904814/config/3130000000009999.json: no such file or directory"
	// ... expected string = ""
	t.Skip("'Make PMM great again!' No automated testing and this test was failing on 9 Feburary 2017: https://github.com/percona/qan-agent/pull/37")

	// Make and start a qan.Manager with mock factories, no analyzer yet.
	a := mock.NewQanAnalyzer("qan-analizer-1")
	f := mock.NewQanAnalyzerFactory(a)
	m := qan.NewManager(s.logger, s.im, f)
	t.Assert(m, NotNil)
	err := m.Start()
	t.Check(err, IsNil)
	test.WaitStatus(1, m, "qan", "Running")

	mysqlInstances := s.im.List("mysql")
	t.Assert(len(mysqlInstances), Equals, 1)
	// This is a 'new' instance ID to force Handle to call the API because it doesn't
	// know this instance so it will call the API to get it's information.
	mysqlUUID := "3130000000009999"

	// Create the qan config.
	exampleQueries := true
	config := &pc.QAN{
		UUID: mysqlUUID,
		Start: []string{
			"SET GLOBAL slow_query_log=OFF",
			"SET GLOBAL long_query_time=0.123",
			"SET GLOBAL slow_query_log=ON",
		},
		Stop: []string{
			"SET GLOBAL slow_query_log=OFF",
			"SET GLOBAL long_query_time=10",
		},
		Interval:       300,        // 5 min
		MaxSlowLogSize: 1073741824, // 1 GiB
		ExampleQueries: &exampleQueries,
		WorkerRunTime:  600, // 10 min
		CollectFrom:    "slowlog",
	}

	// Send a StartTool cmd with the qan config to start an analyzer.
	now := time.Now()
	qanConfig, _ := json.Marshal(config)
	cmd := &proto.Cmd{
		User:      "daniel",
		Ts:        now,
		AgentUUID: "123",
		Service:   "qan",
		Cmd:       "StartTool",
		Data:      qanConfig,
	}
	reply := m.Handle(cmd)
	t.Assert(reply.Error, Equals, "")

	// The manager writes the qan config to disk.
	data, err := ioutil.ReadFile(pct.Basedir.ConfigFile("qan-" + mysqlUUID))
	t.Check(err, IsNil)
	gotConfig := &pc.QAN{}
	err = json.Unmarshal(data, gotConfig)
	t.Check(err, IsNil)
	t.Check(gotConfig, DeepEquals, config)

	// Now the manager and analyzer should be running.
	status := m.Status()
	t.Check(status["qan"], Equals, "Running")
	t.Check(status["qan-analyzer"], Equals, "ok")

	// Try to start the same analyzer again. It results in an error because
	// double tooling is not allowed.
	reply = m.Handle(cmd)
	t.Check(reply.Error, Equals, a.String()+" service is running")

	// Send a StopTool cmd to stop the analyzer.
	now = time.Now()
	cmd = &proto.Cmd{
		User:      "daniel",
		Ts:        now,
		AgentUUID: "123",
		Service:   "qan",
		Cmd:       "StopTool",
		Data:      []byte(mysqlUUID),
	}
	reply = m.Handle(cmd)
	t.Assert(reply.Error, Equals, "")

	// Now the manager is still running, but the analyzer is not.
	status = m.Status()
	t.Check(status["qan"], Equals, "Running")

	// And the manager has removed the qan config from disk so next time
	// the agent starts the analyzer is not started.
	t.Check(test.FileExists(pct.Basedir.ConfigFile("qan-"+mysqlUUID)), Equals, false)

	// StopTool should be idempotent, so send it again and expect no error.
	reply = m.Handle(cmd)
	t.Assert(reply.Error, Equals, "")

	// Stop the manager.
	err = m.Stop()
	t.Assert(err, IsNil)
}

func (s *ManagerTestSuite) TestStartTool(t *C) {
	// Make and start a qan.Manager with mock factories, no analyzer yet.
	a := mock.NewQanAnalyzer("qan-analizer-1")
	f := mock.NewQanAnalyzerFactory(a)
	m := qan.NewManager(s.logger, s.im, f)
	t.Assert(m, NotNil)
	err := m.Start()
	t.Check(err, IsNil)
	test.WaitStatus(1, m, "qan", "Running")

	mysqlInstances := s.im.List("mysql")
	t.Assert(len(mysqlInstances), Equals, 1)
	mysqlUUID := mysqlInstances[0].UUID

	// Create the qan config.
	exampleQueries := true
	config := &pc.QAN{
		UUID: mysqlUUID,
		Start: []string{
			"SET GLOBAL slow_query_log=OFF",
			"SET GLOBAL long_query_time=0.123",
			"SET GLOBAL slow_query_log=ON",
		},
		Stop: []string{
			"SET GLOBAL slow_query_log=OFF",
			"SET GLOBAL long_query_time=10",
		},
		Interval:       300,        // 5 min
		MaxSlowLogSize: 1073741824, // 1 GiB
		ExampleQueries: &exampleQueries,
		WorkerRunTime:  600, // 10 min
		CollectFrom:    "slowlog",
	}

	// Send a StartTool cmd with the qan config to start an analyzer.
	now := time.Now()
	qanConfig, _ := json.Marshal(config)
	cmd := &proto.Cmd{
		User:      "daniel",
		Ts:        now,
		AgentUUID: "123",
		Service:   "qan",
		Cmd:       "StartTool",
		Data:      qanConfig,
	}
	reply := m.Handle(cmd)
	t.Assert(reply.Error, Equals, "")

	// The manager writes the qan config to disk.
	data, err := ioutil.ReadFile(pct.Basedir.ConfigFile("qan-" + mysqlUUID))
	t.Check(err, IsNil)
	gotConfig := &pc.QAN{}
	err = json.Unmarshal(data, gotConfig)
	t.Check(err, IsNil)
	// For some reasons MaxSlowLogSize is explicitly marked to not be saved in config file
	// type QAN struct {
	//      ...
	// 	MaxSlowLogSize int64  `json:"-"` // bytes, 0 = DEFAULT_MAX_SLOW_LOG_SIZE. Don't write it to the config
	//      ...
	// }
	t.Check(gotConfig.MaxSlowLogSize, Equals, int64(0))
	gotConfig.MaxSlowLogSize = config.MaxSlowLogSize
	t.Check(gotConfig, DeepEquals, config)

	// Now the manager and analyzer should be running.
	status := m.Status()
	t.Check(status["qan"], Equals, "Running")
	t.Check(status["qan-analyzer"], Equals, "ok")

	// Try to start the same analyzer again. It results in an error because
	// double tooling is not allowed.
	reply = m.Handle(cmd)
	t.Check(reply.Error, Equals, fmt.Sprintf("Query Analytics is already running on instance %s. To reconfigure or restart Query Analytics, stop then start it again.", mysqlUUID))

	// Send a StopTool cmd to stop the analyzer.
	now = time.Now()
	cmd = &proto.Cmd{
		User:      "daniel",
		Ts:        now,
		AgentUUID: "123",
		Service:   "qan",
		Cmd:       "StopTool",
		Data:      []byte(mysqlUUID),
	}
	reply = m.Handle(cmd)
	t.Assert(reply.Error, Equals, "")

	// Now the manager is still running, but the analyzer is not.
	status = m.Status()
	t.Check(status["qan"], Equals, "Running")

	// And the manager has removed the qan config from disk so next time
	// the agent starts the analyzer is not started.
	t.Check(test.FileExists(pct.Basedir.ConfigFile("qan-"+mysqlUUID)), Equals, false)

	// StopTool should be idempotent, so send it again and expect no error.
	reply = m.Handle(cmd)
	t.Assert(reply.Error, Equals, "")

	// Stop the manager.
	err = m.Stop()
	t.Assert(err, IsNil)
}

func (s *ManagerTestSuite) TestBadCmd(t *C) {
	a := mock.NewQanAnalyzer("qan-analizer-1")
	f := mock.NewQanAnalyzerFactory(a)
	m := qan.NewManager(s.logger, s.im, f)
	t.Assert(m, NotNil)
	err := m.Start()
	t.Check(err, IsNil)
	defer m.Stop()
	test.WaitStatus(1, m, "qan", "Running")
	cmd := &proto.Cmd{
		User:      "daniel",
		Ts:        time.Now(),
		AgentUUID: "123",
		Service:   "qan",
		Cmd:       "foo", // bad cmd
	}
	reply := m.Handle(cmd)
	t.Assert(reply.Error, Equals, "Unknown command: foo")
}
