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

package agent

import (
	"archive/zip"
	"encoding/base64"
	"encoding/json"
	"io/ioutil"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"testing"
	"time"

	"github.com/percona/pmm/proto"
	pc "github.com/percona/pmm/proto/config"
	"github.com/percona/qan-agent/agent/release"
	"github.com/percona/qan-agent/pct"
	pctCmd "github.com/percona/qan-agent/pct/cmd"
	"github.com/percona/qan-agent/test"
	"github.com/percona/qan-agent/test/mock"
	"github.com/percona/qan-agent/test/rootdir"
	"github.com/stretchr/testify/assert"
	. "gopkg.in/check.v1"
)

// Hook gocheck into the "go test" runner.
func Test(t *testing.T) { TestingT(t) }

var sample = rootdir.RootDir() + "/test/agent"

type AgentTestSuite struct {
	tmpDir     string
	configFile string
	// Log
	logger  *pct.Logger
	logChan chan proto.LogEntry
	// Agent
	agent        *Agent
	config       *pc.Agent
	services     map[string]pct.ServiceManager
	servicesMap  map[string]pct.ServiceManager
	client       *mock.WebsocketClient
	sendDataChan chan interface{}
	recvDataChan chan interface{}
	sendChan     chan *proto.Cmd
	recvChan     chan *proto.Reply
	api          *mock.API
	agentRunning bool
	// --
	startWaitGroup *sync.WaitGroup
	traceChan      chan string
	doneChan       chan bool
	stopReason     string
	upgrade        bool
}

var _ = Suite(&AgentTestSuite{})

func (s *AgentTestSuite) SetUpSuite(t *C) {
	var err error
	s.tmpDir, err = ioutil.TempDir("/tmp", "percona-agent-test")
	t.Assert(err, IsNil)

	if err := pct.Basedir.Init(s.tmpDir); err != nil {
		t.Fatal(err)
	}
	s.configFile = filepath.Join(s.tmpDir, pct.CONFIG_DIR, "agent"+pct.CONFIG_FILE_SUFFIX)

	// Log
	// todo: use log.Manager instead
	s.logChan = make(chan proto.LogEntry, 10)
	s.logger = pct.NewLogger(s.logChan, "agent-test")

	// Agent
	s.config = &pc.Agent{
		UUID:        "abc-123-def",
		ApiHostname: "http://localhost",
		Keepalive:   1, // don't send while testing
	}

	s.sendChan = make(chan *proto.Cmd, 5)
	s.recvChan = make(chan *proto.Reply, 5)
	s.sendDataChan = make(chan interface{}, 5)
	s.recvDataChan = make(chan interface{}, 5)
	s.client = mock.NewWebsocketClient(s.sendChan, s.recvChan, s.sendDataChan, s.recvDataChan)
	s.client.ErrChan = make(chan error)

	s.traceChan = make(chan string, 10)
	s.doneChan = make(chan bool, 1)
}

func (s *AgentTestSuite) SetUpTest(t *C) {
	s.startWaitGroup = &sync.WaitGroup{}
	// Before each test, create an agent.  Tests make change the agent,
	// so this ensures each test starts with an agent with known values.
	s.services = make(map[string]pct.ServiceManager)
	for _, service := range []string{"qan", "mm"} {
		s.services[service] = mock.NewMockServiceManager(
			service,
			s.startWaitGroup,
			s.traceChan,
		)
	}

	links := map[string]string{
		"agent":     "http://localhost/agent",
		"instances": "http://localhost/instances",
	}
	s.api = mock.NewAPI("http://localhost", s.config.ApiHostname, s.config.UUID, links)

	s.servicesMap = map[string]pct.ServiceManager{
		"mm":  s.services["mm"],
		"qan": s.services["qan"],
	}

	// Run the agent.
	s.agent = NewAgent(s.config, s.logger, s.client, "http://localhost", s.servicesMap)
	s.agentRunning = true

	go func() {
		s.agent.Run()
		s.doneChan <- true
	}()
}

func (s *AgentTestSuite) TearDownTest(t *C) {
	if s.agentRunning {
		s.sendChan <- &proto.Cmd{Cmd: "Stop"} // tell agent to stop itself

		test.WaitReply(s.recvChan)
		select {
		case <-s.doneChan: // wait for goroutine agent.Run() in test
		case <-time.After(5 * time.Second):
			t.Fatal("Agent didn't respond to Stop cmd")
		}
		s.agentRunning = false
	}

	test.DrainLogChan(s.logChan)
	test.DrainSendChan(s.sendChan)
	test.DrainRecvChan(s.recvChan)
	test.DrainTraceChan(s.traceChan)
	test.DrainTraceChan(s.client.TraceChan)
	test.DrainBoolChan(s.client.ConnectChan())
}

func (s *AgentTestSuite) TearDownSuite(t *C) {
	//if err := os.RemoveAll(s.tmpDir); err != nil {
	//	t.Error(err)
	//}
}

type ByService []proto.AgentConfig

func (a ByService) Len() int      { return len(a) }
func (a ByService) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByService) Less(i, j int) bool {
	return a[i].Service < a[j].Service
}

/////////////////////////////////////////////////////////////////////////////
// Test cases
// //////////////////////////////////////////////////////////////////////////

func (s *AgentTestSuite) TestStatus(t *C) {

	// This is what the API would send:
	statusCmd := &proto.Cmd{
		Ts:   time.Now(),
		User: "daniel",
		Cmd:  "Status",
		Id:   "1",
	}
	s.sendChan <- statusCmd

	got := test.WaitStatusReply(s.recvChan)
	t.Assert(got, NotNil)

	expectStatus := map[string]string{
		"agent":             "Idle",
		"agent-cmd-handler": "Idle",
		"mm":                "",
		"qan":               "",
		"ws":                "Connected",
		"ws-link":           "http://localhost",
	}

	t.Assert(got["agent"], DeepEquals, expectStatus["agent"])
	t.Assert(got["qan"], DeepEquals, expectStatus["qan"])
	t.Assert(got["mm"], DeepEquals, expectStatus["mm"])
	t.Assert(got["ws"], DeepEquals, expectStatus["ws"])

	// We asked for all status, so we should get mm too.
	_, ok := got["mm"]
	t.Check(ok, Equals, true)

	/**
	 * Get only agent's status
	 */
	statusCmd = &proto.Cmd{
		Ts:      time.Now(),
		User:    "daniel",
		Cmd:     "Status",
		Service: "agent",
		Id:      "2",
	}
	s.sendChan <- statusCmd
	got = test.WaitStatusReply(s.recvChan)
	t.Assert(got, NotNil)

	// Only asked for agent, so we shouldn't get mm.
	_, ok = got["mm"]
	t.Check(ok, Equals, false)

	/**
	 * Get only sub-service status.
	 */
	statusCmd = &proto.Cmd{
		Ts:      time.Now(),
		User:    "daniel",
		Cmd:     "Status",
		Service: "mm",
		Id:      "3",
	}
	s.sendChan <- statusCmd
	got = test.WaitStatusReply(s.recvChan)
	t.Assert(got, NotNil)

	// Asked for mm, so we get it.
	_, ok = got["mm"]
	t.Check(ok, Equals, true)

	// Didn't ask for all or agent, so we don't get it.
	_, ok = got["agent"]
	t.Check(ok, Equals, false)
}

func (s *AgentTestSuite) TestStatusAfterConnFail(t *C) {
	// Use optional ConnectChan in mock ws client for this test only.
	connectChan := make(chan bool)
	s.client.SetConnectChan(connectChan)
	defer s.client.SetConnectChan(nil)

	// Disconnect agent.
	s.client.Disconnect()

	// Wait for agent to reconnect.
	<-connectChan
	connectChan <- true

	// Send cmd.
	statusCmd := &proto.Cmd{
		Ts:   time.Now(),
		User: "daniel",
		Cmd:  "Status",
	}
	s.sendChan <- statusCmd

	// Get reply.
	got := test.WaitStatusReply(s.recvChan)
	t.Assert(got, NotNil)
	t.Check(got["agent"], Equals, "Idle")
}

func (s *AgentTestSuite) TestStartStopService(t *C) {
	// To start a service, first we make a config for the service:
	qanConfig := &pc.QAN{
		Interval:       60,         // seconds
		MaxSlowLogSize: 1073741824, // 1 GiB
		ExampleQueries: true,
		WorkerRunTime:  120, // seconds
	}

	// Second, the service config is encoded and encapsulated in a ServiceData:
	qanConfigData, _ := json.Marshal(qanConfig)
	serviceCmd := &proto.ServiceData{
		Name:   "qan",
		Config: qanConfigData,
	}

	// Third and final, the service data is encoded and encapsulated in a Cmd:
	serviceData, _ := json.Marshal(serviceCmd)
	cmd := &proto.Cmd{
		Ts:      time.Now(),
		User:    "daniel",
		Service: "agent",
		Cmd:     "StartService",
		Data:    serviceData,
	}

	// Send the StartService cmd to the client, then wait for the reply
	// which should not have an error, indicating success.
	s.sendChan <- cmd
	gotReplies := test.WaitReply(s.recvChan)
	if len(gotReplies) != 1 {
		t.Fatal("Got Reply to Cmd:StartService")
	}
	reply := &proto.Reply{}
	_ = json.Unmarshal(gotReplies[0].Data, reply)
	if reply.Error != "" {
		t.Error("No Reply.Error to Cmd:StartService; got ", reply.Error)
	}

	// To double-check that the agent started without error, get its status
	// which should show everything is "Ready" or "Idle".
	status := test.GetStatus(s.sendChan, s.recvChan)
	expectStatus := map[string]string{
		"ws-link":           "http://localhost",
		"agent-cmd-handler": "Idle",
		"mm":                "",
		"qan":               "Ready",
		"ws":                "Connected",
	}
	delete(status, "agent") // unpredictable, can be `Idle` or `Queueing`
	assert.Equal(t, expectStatus, status)

	// Finally, since we're using mock objects, let's double check the
	// execution trace, i.e. what calls the agent made based on all
	// the previous ^.
	got := test.WaitTrace(s.traceChan)
	sort.Strings(got)
	expect := []string{
		`Start qan`,
		`Status mm`,
		`Status qan`,
	}
	assert.Equal(t, expect, got)

	/**
	 * Stop the service.
	 */

	serviceCmd = &proto.ServiceData{
		Name: "qan",
	}
	serviceData, _ = json.Marshal(serviceCmd)
	cmd = &proto.Cmd{
		Ts:      time.Now(),
		User:    "daniel",
		Service: "agent",
		Cmd:     "StopService",
		Data:    serviceData,
	}

	s.sendChan <- cmd
	gotReplies = test.WaitReply(s.recvChan)
	if len(gotReplies) != 1 {
		t.Fatal("Got Reply to Cmd:StopService")
	}
	reply = &proto.Reply{}
	_ = json.Unmarshal(gotReplies[0].Data, reply)
	if reply.Error != "" {
		t.Error("No Reply.Error to Cmd:StopService; got ", reply.Error)
	}

	status = test.GetStatus(s.sendChan, s.recvChan)
	t.Check(status["qan"], Equals, "Stopped")
	t.Check(status["mm"], Equals, "")
}

func (s *AgentTestSuite) TestStartServiceSlow(t *C) {
	// This test is like TestStartService but simulates a slow starting service.

	qanConfig := &pc.QAN{
		Interval:       60,         // seconds
		MaxSlowLogSize: 1073741824, // 1 GiB
		ExampleQueries: true,
		WorkerRunTime:  120, // seconds
	}
	qanConfigData, _ := json.Marshal(qanConfig)
	serviceCmd := &proto.ServiceData{
		Name:   "qan",
		Config: qanConfigData,
	}
	serviceData, _ := json.Marshal(serviceCmd)
	now := time.Now()
	cmd := &proto.Cmd{
		Ts:      now,
		User:    "daniel",
		Service: "agent",
		Cmd:     "StartService",
		Data:    serviceData,
	}

	s.startWaitGroup.Add(1)
	// Send the cmd to the client, tell the agent to stop, then wait for it.
	s.sendChan <- cmd

	// No replies yet.
	gotReplies := test.WaitReply(s.recvChan)
	if len(gotReplies) != 0 {
		t.Fatal("No reply before StartService")
	}

	// Agent should be able to reply on status chan, indicating that it's
	// still starting the service.
	gotStatus := test.GetStatus(s.sendChan, s.recvChan)
	t.Check(gotStatus["qan"], Equals, "Starting")

	// Make it seem like service has started now.
	s.startWaitGroup.Done()

	// Agent sends reply: no error.
	gotReplies = test.WaitReply(s.recvChan)
	if len(gotReplies) == 0 {
		t.Fatal("Get reply")
	}
	if len(gotReplies) > 1 {
		t.Errorf("One reply, got %+v", gotReplies)
	}

	reply := &proto.Reply{}
	_ = json.Unmarshal(gotReplies[0].Data, reply)
	t.Check(reply.Error, Equals, "")
}

func (s *AgentTestSuite) TestStartStopUnknownService(t *C) {
	// Starting an unknown service should return an error.
	serviceCmd := &proto.ServiceData{
		Name: "foo",
	}
	serviceData, _ := json.Marshal(serviceCmd)
	cmd := &proto.Cmd{
		Ts:      time.Now(),
		User:    "daniel",
		Service: "agent",
		Cmd:     "StartService",
		Data:    serviceData,
	}

	s.sendChan <- cmd
	gotReplies := test.WaitReply(s.recvChan)
	t.Assert(len(gotReplies), Equals, 1)
	t.Check(gotReplies[0].Cmd, Equals, "StartService")
	t.Check(gotReplies[0].Error, Not(Equals), "")

	// Stopp an unknown service should return an error.
	cmd = &proto.Cmd{
		Ts:      time.Now(),
		User:    "daniel",
		Service: "agent",
		Cmd:     "StopService",
		Data:    serviceData,
	}

	s.sendChan <- cmd
	gotReplies = test.WaitReply(s.recvChan)
	t.Assert(len(gotReplies), Equals, 1)
	t.Check(gotReplies[0].Cmd, Equals, "StopService")
	t.Check(gotReplies[0].Error, Not(Equals), "")
}

func (s *AgentTestSuite) TestLoadConfig(t *C) {
	// Load a partial config to make sure LoadConfig() works in general but also
	// when the config has missing options (which is normal).
	os.Remove(s.configFile)
	sampleConfig := sample + "/config001.json"
	err := test.CopyFile(sampleConfig, s.configFile)
	if err != nil {
		t.Fatalf("cannot copy config file %s to %s : %s", sampleConfig, s.configFile, err.Error())
	}

	bytes, err := LoadConfig()
	t.Assert(err, IsNil)
	got := &pc.Agent{}
	if err := json.Unmarshal(bytes, got); err != nil {
		t.Fatal(err)
	}
	expect := &pc.Agent{
		UUID:        "abc-123-def",
		ApiHostname: "localhost",
		Keepalive:   DEFAULT_KEEPALIVE,
	}
	assert.Equal(t, expect, got)

	// Load a config with all options to make sure LoadConfig() hasn't missed any.
	os.Remove(s.configFile)
	err = test.CopyFile(sample+"/full_config.json", s.configFile)
	if err != nil {
		t.Fatalf("cannot copy config file %s to %s : %s", sampleConfig, s.configFile, err.Error())
	}
	bytes, err = LoadConfig()
	t.Assert(err, IsNil)
	got = &pc.Agent{}
	if err := json.Unmarshal(bytes, got); err != nil {
		t.Fatal(err)
	}
	expect = &pc.Agent{
		ApiHostname: "agent hostname",
		UUID:        "agent uuid",
		Keepalive:   DEFAULT_KEEPALIVE,
	}
	assert.Equal(t, expect, got)
}

func (s *AgentTestSuite) TestGetConfig(t *C) {
	cmd := &proto.Cmd{
		Ts:      time.Now(),
		User:    "daniel",
		Cmd:     "GetConfig",
		Service: "agent",
	}
	s.sendChan <- cmd

	got := test.WaitReply(s.recvChan)
	t.Assert(len(got), Equals, 1)
	gotConfig := []proto.AgentConfig{}
	if err := json.Unmarshal(got[0].Data, &gotConfig); err != nil {
		t.Fatal(err)
	}

	config := *s.config
	config.Links = nil
	bytes, _ := json.Marshal(config)
	expect := []proto.AgentConfig{
		{
			Service: "agent",
			Running: string(bytes),
			Updated: time.Time{}.UTC(),
		},
	}

	assert.Equal(t, expect, gotConfig)
}

func (s *AgentTestSuite) TestGetAllConfigs(t *C) {
	cmd := &proto.Cmd{
		Ts:      time.Now(),
		User:    "daniel",
		Cmd:     "GetAllConfigs",
		Service: "agent",
	}
	s.sendChan <- cmd

	got := test.WaitReply(s.recvChan)
	t.Assert(len(got), Equals, 1)
	reply := got[0]
	t.Check(reply.Error, Equals, "")
	t.Assert(reply.Data, Not(HasLen), 0)

	gotConfigs := []proto.AgentConfig{}
	err := json.Unmarshal(reply.Data, &gotConfigs)
	t.Assert(err, IsNil)

	bytes, _ := json.Marshal(s.config)

	sort.Sort(ByService(gotConfigs))
	expectConfigs := []proto.AgentConfig{
		{
			Service: "agent",
			Running: string(bytes),
			Updated: time.Time{}.UTC(),
		},
		{
			Service: "mm",
			Set:     `{"Foo":"bar"}`,
			Updated: time.Time{}.UTC(),
		},
		{
			Service: "qan",
			Set:     `{"Foo":"bar"}`,
			Updated: time.Time{}.UTC(),
		},
	}
	assert.Equal(t, expectConfigs, gotConfigs)
}

func (s *AgentTestSuite) TestGetVersion(t *C) {
	cmd := &proto.Cmd{
		Ts:      time.Now(),
		User:    "daniel",
		Cmd:     "Version",
		Service: "agent",
	}
	s.sendChan <- cmd

	got := test.WaitReply(s.recvChan)
	t.Assert(len(got), Equals, 1)
	version := &proto.Version{}
	json.Unmarshal(got[0].Data, &version)
	t.Check(version.Running, Equals, release.VERSION)
}

func (s *AgentTestSuite) TestGetSystemSummary(t *C) {
	cmd := &proto.Cmd{
		Ts:      time.Now(),
		User:    "zapp brannigan",
		Cmd:     "GetServerSummary",
		Service: "agent",
	}
	s.sendChan <- cmd

	got := test.WaitReplyCmd(s.recvChan, "GetServerSummary")
	t.Assert(len(got), Equals, 1)
	t.Assert(got[0].Error, Equals, "")
	t.Assert(string(got[0].Data), Matches, ".*# Percona Toolkit System Summary Report.*")
}

func (s *AgentTestSuite) TestGetMySQLSummary(t *C) {
	cmd := &proto.Cmd{
		Ts:      time.Now(),
		User:    "zapp brannigan",
		Cmd:     "GetMySQLSummary",
		Service: "agent",
	}
	s.sendChan <- cmd

	got := test.WaitReplyCmd(s.recvChan, "GetMySQLSummary")
	t.Assert(len(got), Equals, 1)
	t.Assert(got[0].Error, Equals, "")
	t.Assert(string(got[0].Data), Matches, ".*# Percona Toolkit MySQL Summary Report.*")

}

func (s *AgentTestSuite) TestGetMongoSummary(t *C) {
	cmd := &proto.Cmd{
		Ts:      time.Now(),
		User:    "zapp brannigan",
		Cmd:     "GetMongoSummary",
		Service: "agent",
	}
	s.sendChan <- cmd

	got := test.WaitReplyCmd(s.recvChan, "GetMongoSummary")
	t.Assert(len(got), Equals, 1)
	t.Assert(got[0].Error, Equals, "")
	// in new version:
	// t.Assert(string(got[0].Data), Matches, ".*# Mongo Executable.*")
	t.Assert(string(got[0].Data), Matches, ".*# Instances.*")
}

func (s *AgentTestSuite) TestCollectInfo(t *C) {
	cmd := &proto.Cmd{
		Ts:      time.Now(),
		User:    "zapp brannigan",
		Cmd:     "CollectServicesData",
		Service: "agent",
	}
	s.sendChan <- cmd

	got := test.WaitReplyCmd(s.recvChan, "CollectServicesData")
	t.Assert(len(got), Equals, 1)

	dst := struct {
		Filename string
		Data     []byte
	}{}

	err := json.Unmarshal(got[0].Data, &dst)
	t.Assert(err, IsNil)

	// Te response has a zip file encoded as base64 to be able to send
	// binary data as a json payload.
	// Let's write that to a file for the zip library
	tmpfile64, err := ioutil.TempFile("", "decoder_")
	ioutil.WriteFile(tmpfile64.Name(), dst.Data, os.ModePerm)
	tmpfile64.Close()

	tmpfile, err := ioutil.TempFile("", "test000")
	t.Assert(err, IsNil)

	dbuf := make([]byte, base64.StdEncoding.DecodedLen(len(dst.Data)))

	size, err := base64.StdEncoding.Decode(dbuf, dst.Data)
	t.Assert(err, IsNil)

	// DecodedLen(len(dst.Data)) returns the MAXIMUM possible size but
	// the real size is the one returned by the Decode method so, we need
	// to write only those bytes.
	ioutil.WriteFile(tmpfile.Name(), dbuf[:size], os.ModePerm)
	tmpfile.Close()

	z, err := zip.OpenReader(tmpfile.Name())
	t.Assert(err, IsNil)
	defer z.Close()

	// Check the zip file has the files we requested.
	wantFiles := []string{"pt-summary.out", "pt-mysql-summary.out"}
	gotFiles := []string{}
	for _, f := range z.File {
		gotFiles = append(gotFiles, f.Name)
	}
	t.Assert(wantFiles, DeepEquals, gotFiles)
}

func (s *AgentTestSuite) TestSetConfigApiHostname(t *C) {
	newConfig := *s.config
	newConfig.ApiHostname = "http://localhost"
	data, err := json.Marshal(newConfig)
	t.Assert(err, IsNil)

	cmd := &proto.Cmd{
		Ts:      time.Now(),
		User:    "daniel",
		Cmd:     "SetConfig",
		Service: "agent",
		Data:    data,
	}
	s.sendChan <- cmd

	got := test.WaitReply(s.recvChan)
	t.Assert(len(got), Equals, 1)
	gotConfig := &pc.Agent{}
	if err := json.Unmarshal(got[0].Data, gotConfig); err != nil {
		t.Fatal(err)
	}

	/**
	 * Verify new agent config in memory.
	 */
	expect := *s.config
	expect.ApiHostname = "http://localhost"
	expect.Links = nil
	assert.Equal(t, &expect, gotConfig)

	/**
	 * Verify new agent config in API connector.
	 */
	t.Check(s.api.Hostname(), Equals, "http://localhost")

	/**
	 * Verify new agent config on disk.
	 */
	data, err = ioutil.ReadFile(s.configFile)
	t.Assert(err, IsNil)
	gotConfig = &pc.Agent{}
	if err := json.Unmarshal(data, gotConfig); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, &expect, gotConfig)

	// After changing the API host, the agent's ws should NOT reconnect yet,
	// but status should show that its link has changed, so sending a Reconnect
	// cmd will cause agent to reconnect its ws.
	gotCalled := test.WaitTrace(s.client.TraceChan)
	expectCalled := []string{"Start", "Connect"}
	assert.Equal(t, expectCalled, gotCalled)

	/**
	 * Test Reconnect here since it's usually done after changing ApiHostname/
	 */

	// There is NO reply after reconnect because we can't recv cmd on one connection
	// and reply on another.  Instead, we should see agent try to reconnect:
	connectChan := make(chan bool)
	s.client.SetConnectChan(connectChan)
	defer s.client.SetConnectChan(nil)

	cmd = &proto.Cmd{
		Ts:      time.Now(),
		User:    "daniel",
		Cmd:     "Reconnect",
		Service: "agent",
	}
	s.sendChan <- cmd

	// Wait for agent to reconnect.
	<-connectChan
	connectChan <- true

	gotCalled = test.WaitTrace(s.client.TraceChan)
	expectCalled = []string{"Disconnect", "Connect"}
	assert.Equal(t, expectCalled, gotCalled)
}

func (s *AgentTestSuite) TestKeepalive(t *C) {
	// Agent should be sending a Pong every 1s now which is sent as a
	// reply to no cmd (it's a platypus).
	<-time.After(2 * time.Second)
	reply := test.WaitReply(s.recvChan)
	if len(reply) < 1 {
		t.Fatal("No Pong recieved")
	}
	t.Check(reply[0].Cmd, Equals, "Pong")

	// Disconnect and keepalives should stop.
	connectChan := make(chan bool)
	s.client.SetConnectChan(connectChan)
	defer s.client.SetConnectChan(nil)
	s.client.Disconnect()
	<-connectChan

	<-time.After(2 * time.Second)
	reply = test.WaitReply(s.recvChan)
	t.Check(reply, HasLen, 0)

	// Let agent reconnect and keepalives should resume.
	connectChan <- true
	<-time.After(2 * time.Second)
	reply = test.WaitReply(s.recvChan)
	if len(reply) < 1 {
		t.Fatal("No Pong recieved after reconnect")
	}
	t.Check(reply[0].Cmd, Equals, "Pong")
}

func (s *AgentTestSuite) TestRestart(t *C) {
	// Stop the default agent.  We need our own to check its return value.
	s.TearDownTest(t)

	cmdFactory := &mock.CmdFactory{}
	pctCmd.Factory = cmdFactory

	defer func() {
		os.Remove(pct.Basedir.File("start-lock"))
		os.Remove(pct.Basedir.File("start-script"))
	}()

	newAgent := NewAgent(s.config, s.logger, s.client, "localhost", s.servicesMap)
	doneChan := make(chan error, 1)
	go func() {
		doneChan <- newAgent.Run()
	}()

	cmd := &proto.Cmd{
		Service: "agent",
		Cmd:     "Restart",
	}
	s.sendChan <- cmd

	replies := test.WaitReply(s.recvChan)
	t.Assert(replies, HasLen, 1)
	t.Check(replies[0].Error, Equals, "")

	var err error
	select {
	case err = <-doneChan:
	case <-time.After(2 * time.Second):
		t.Fatal("Agent did not restart")
	}

	// Agent should return without an error.
	t.Check(err, IsNil)

	// Agent should create the start-lock file and start-script.
	t.Check(pct.FileExists(pct.Basedir.File("start-lock")), Equals, true)
	t.Check(pct.FileExists(pct.Basedir.File("start-script")), Equals, true)

	// Agent should make a command to run the start-script.
	t.Assert(cmdFactory.Cmds, HasLen, 1)
	t.Check(cmdFactory.Cmds[0].Name, Equals, pct.Basedir.File("start-script"))
	t.Check(cmdFactory.Cmds[0].Args, IsNil)
}

func (s *AgentTestSuite) TestCmdToService(t *C) {
	cmd := &proto.Cmd{
		Service: "mm",
		Cmd:     "Hello",
	}
	s.sendChan <- cmd

	reply := test.WaitReply(s.recvChan)
	t.Assert(reply, HasLen, 1)
	t.Check(reply[0].Error, Equals, "")
	t.Check(reply[0].Cmd, Equals, "Hello")

	t.Assert(s.services["mm"].(*mock.MockServiceManager).Cmds, HasLen, 1)
	t.Check(s.services["mm"].(*mock.MockServiceManager).Cmds[0].Cmd, Equals, "Hello")
}
