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

package log_test

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"testing"
	"time"

	"github.com/percona/pmm/proto"
	pc "github.com/percona/pmm/proto/config"
	"github.com/percona/qan-agent/log"
	"github.com/percona/qan-agent/pct"
	"github.com/percona/qan-agent/test"
	"github.com/percona/qan-agent/test/mock"
	"github.com/stretchr/testify/assert"
	. "gopkg.in/check.v1"
)

// Hook gocheck into the "go test" runner.
func Test(t *testing.T) { TestingT(t) }

/////////////////////////////////////////////////////////////////////////////
// Relay test suite
/////////////////////////////////////////////////////////////////////////////

type RelayTestSuite struct {
	logChan     chan proto.LogEntry
	logFile     string
	sendChan    chan interface{}
	recvChan    chan interface{}
	connectChan chan bool
	client      *mock.WebsocketClient
	relay       *log.Relay
	logger      *pct.Logger
}

var _ = Suite(&RelayTestSuite{})

func (s *RelayTestSuite) SetUpSuite(t *C) {
	s.logFile = fmt.Sprintf("/tmp/log_test.go.%d", os.Getpid())

	s.sendChan = make(chan interface{}, 5)
	s.recvChan = make(chan interface{}, 5)
	s.connectChan = make(chan bool)
	s.client = mock.NewWebsocketClient(nil, nil, s.sendChan, s.recvChan)

	s.logChan = make(chan proto.LogEntry, log.BUFFER_SIZE*3)
	s.relay = log.NewRelay(s.client, s.logChan, proto.LOG_INFO, false)
	s.logger = pct.NewLogger(s.relay.LogChan(), "test")
	go s.relay.Run() // calls client.Connect()
	got := test.WaitLog(s.recvChan, 1)
	expect := []proto.LogEntry{
		{Ts: test.Ts, Level: proto.LOG_INFO, Service: "log", Msg: "Connected to API"},
	}
	assert.Equal(t, expect, got)
}

func (s *RelayTestSuite) SetUpTest(t *C) {
	s.relay.LogLevelChan() <- proto.LOG_INFO
	s.client.SetConnectChan(nil)

	test.DrainLogChan(s.logChan)
	test.DrainRecvData(s.recvChan)

	if !test.WaitStatus(3, s.relay, "log-buf1", "0") {
		status := s.relay.Status()
		t.Log(status)
		t.Fatal("First buffer full")
	}

	if !test.WaitStatus(3, s.relay, "log-buf2", "0") {
		status := s.relay.Status()
		t.Log(status)
		t.Fatal("Second buffer has overflow")
	}

	test.DrainTraceChan(s.client.TraceChan)
	test.DrainBoolChan(s.connectChan)
}

func (s *RelayTestSuite) TearDownSuite(t *C) {
	s.relay.Stop()
	os.Remove(s.logFile)
}

/////////////////////////////////////////////////////////////////////////////
// Test cases
// //////////////////////////////////////////////////////////////////////////

// @TODO: Log level doesn't work; It was broken with the first commit to this repo
// Below was in relay.go, so logs below log level were skipped
// case entry := <-r.logChan:
// 	Skip if log level too high, too verbose.
// 	if entry.Level > r.logLevel {
// 		continue
// 	}
// above is missing now, so all logs are registered
func (s *RelayTestSuite) TestLogLevel(t *C) {
	r := s.relay
	l := s.logger

	r.LogLevelChan() <- proto.LOG_INFO
	l.Debug("debug")
	l.Info("info")
	l.Warn("warning")
	l.Error("error")
	l.Fatal("fatal")
	got := test.WaitLog(s.recvChan, 4)
	expect := []proto.LogEntry{
		{Ts: test.Ts, Level: proto.LOG_INFO, Service: "test", Msg: "info"},
		{Ts: test.Ts, Level: proto.LOG_WARNING, Service: "test", Msg: "warning"},
		{Ts: test.Ts, Level: proto.LOG_ERROR, Service: "test", Msg: "error"},
		{Ts: test.Ts, Level: proto.LOG_CRITICAL, Service: "test", Msg: "fatal"},
	}
	assert.Equal(t, expect, got)

	r.LogLevelChan() <- proto.LOG_WARNING
	l.Debug("debug")
	l.Info("info")
	l.Warn("warning")
	l.Error("error")
	l.Fatal("fatal")
	got = test.WaitLog(s.recvChan, 4)
	expect = []proto.LogEntry{
		{Ts: test.Ts, Level: proto.LOG_INFO, Service: "test", Msg: "info"},
		{Ts: test.Ts, Level: proto.LOG_WARNING, Service: "test", Msg: "warning"},
		{Ts: test.Ts, Level: proto.LOG_ERROR, Service: "test", Msg: "error"},
		{Ts: test.Ts, Level: proto.LOG_CRITICAL, Service: "test", Msg: "fatal"},
	}
	assert.Equal(t, expect, got)
}

func (s *RelayTestSuite) TestOfflineBuffering(t *C) {
	l := s.logger

	// We're going to cause the relay's client Send() to get an error
	// which will cause the relay to connect again.  We block this 2nd
	// connect by blocking the connectChan.  End result: relay is offline.
	s.client.SetConnectChan(s.connectChan)

	// Queue the error then log something which will cause the relay to
	// call Send() and get the error.
	s.client.SendError <- io.EOF
	l.Info("I get the Send error")

	// On send error, the relay tries to connect. Wait for it.
	<-s.connectChan

	// Double-check that relay is offline.
	if !test.WaitStatus(1, s.relay, "ws", "Disconnected") {
		t.Fatal("Relay connects")
	}

	// Relay is offline and trying to connect again in another goroutine.
	// These entries should therefore not be sent.  There's a minor race
	// condition: when relay goes offline, it sends an internal log entry.
	// Sometimes we get that here (Service="log") and sometimes not
	// (len(got)==0).  Either condition is correct for this test.
	l.Error("err1")
	l.Error("err2")
	got := test.WaitLog(s.recvChan, -1)
	if len(got) > 0 && got[0].Service != "log" {
		t.Errorf("Log entries are not sent while offline: %+v", got)
	}

	// Unblock the relay's connect attempt.
	s.connectChan <- true
	if !test.WaitStatus(1, s.relay, "ws", "Connected") {
		t.Fatal("Relay connects")
	}

	// Wait for the relay resend what it had ^ buffered.
	got = test.WaitLog(s.recvChan, 5)
	expect := []proto.LogEntry{
		{Ts: test.Ts, Level: proto.LOG_INFO, Service: "test", Msg: "I get the Send error"},
		{Ts: test.Ts, Level: proto.LOG_WARNING, Service: "log", Msg: "Lost connection to API"},
		{Ts: test.Ts, Level: proto.LOG_ERROR, Service: "test", Msg: "err1"},
		{Ts: test.Ts, Level: proto.LOG_ERROR, Service: "test", Msg: "err2"},
		{Ts: test.Ts, Level: proto.LOG_INFO, Service: "log", Msg: "Connected to API"},
	}
	t.Check(got, DeepEquals, expect)
}

func (s *RelayTestSuite) TestOffline1stBufferOverflow(t *C) {
	// Same magic as in TestOfflineBuffering to force relay offline.
	l := s.logger
	s.client.SetConnectChan(s.connectChan)

	// Queue the error then log something which will cause the relay to
	// call Send() and get the error.
	s.client.SendError <- io.EOF
	l.Info("I get the Send error")

	// On send error, the relay tries to connect. Wait for it.
	<-s.connectChan

	// Relay is offline, trying to connect.

	// Overflow the first buffer but not the second.  We should get all
	// log entries back.  We overflow it by 4 entries:
	// +1			I get the Send error
	// +2			Lost connection to API
	// buf size		a:n (loop below)
	// +3			a:n (loop below)
	// +4			Connected to API
	for i := 1; i <= log.BUFFER_SIZE+1; i++ {
		l.Error(fmt.Sprintf("a:%d", i))
	}

	// Wait until the first buf is full.
	if !test.WaitStatus(3, s.relay, "log-buf1", fmt.Sprintf("%d", log.BUFFER_SIZE)) {
		status := s.relay.Status()
		t.Log(status)
		t.Error("First buffer full")
	}

	// Wait until the second buf has the overflow which is 3 not 4 here because
	// "Connected to API" won't be logged until the next block...
	if !test.WaitStatus(3, s.relay, "log-buf2", "3") {
		status := s.relay.Status()
		t.Log(status)
		t.Error("Second buffer has overflow")
	}

	// Unblock the relay's connect attempt.  This causes "Connected to API" to
	// be log (+4 overflow).
	s.connectChan <- true
	if !test.WaitStatus(1, s.relay, "ws", "Connected") {
		t.Fatal("Relay connects")
	}

	// Wait for the relay resend what it had ^ buffered.
	got := test.WaitLog(s.recvChan, log.BUFFER_SIZE+4)

	// Check that we still get all log entries.
	expect := make([]proto.LogEntry, log.BUFFER_SIZE+4)
	// First two msgs (+1 and +2):
	expect[0] = proto.LogEntry{Ts: test.Ts, Level: proto.LOG_INFO, Service: "test", Msg: "I get the Send error"}
	expect[1] = proto.LogEntry{Ts: test.Ts, Level: proto.LOG_WARNING, Service: "log", Msg: "Lost connection to API"}
	// The overflow (buf size and +3):
	for i, n := 1, 2; i <= log.BUFFER_SIZE+1; i, n = i+1, n+1 {
		expect[n] = proto.LogEntry{Ts: test.Ts, Level: proto.LOG_ERROR, Service: "test", Msg: fmt.Sprintf("a:%d", i)}
	}
	// Last msg (+4):
	expect[len(expect)-1] = proto.LogEntry{Ts: test.Ts, Level: proto.LOG_INFO, Service: "log", Msg: "Connected to API"}
	t.Check(got, DeepEquals, expect)

	// Both bufs should be empty now.
	if !test.WaitStatus(2, s.relay, "log-buf1", "0") {
		status := s.relay.Status()
		t.Log(status)
		t.Fatal("1st buf empty")
	}
	if !test.WaitStatus(2, s.relay, "log-buf2", "0") {
		status := s.relay.Status()
		t.Log(status)
		t.Fatal("2nd buf empty")
	}
}

func (s *RelayTestSuite) TestOffline2ndBufferOverflow(t *C) {
	// This test is like TestOffline1stBufferOverflow but now we'll overflow
	// the 2nd buf too which causes us to lose some log entries and get a
	// "Lost N log entries" warning.

	// Same magic as in TestOfflineBuffering to force relay offline.
	l := s.logger
	s.client.SetConnectChan(s.connectChan)

	// Queue the error then log something which will cause the relay to
	// call Send() and get the error.
	s.client.SendError <- io.EOF
	l.Info("I get the Send error")

	// On send error, the relay tries to connect. Wait for it.
	<-s.connectChan

	// Relay is offline, trying to connect.

	// Overflow the 1st and 2nd buffs.  Note: there's already "I get the Send error" (+1)
	// and "Lost connection to API" (+2) in the 1st buf.  The +1 here makes a +3 overflow.
	for i := 1; i <= (log.BUFFER_SIZE*2)+1; i++ {
		l.Error(fmt.Sprintf("b:%d", i))
	}

	// Wait until the first buf is full.
	if !test.WaitStatus(3, s.relay, "log-buf1", fmt.Sprintf("%d", log.BUFFER_SIZE)) {
		status := s.relay.Status()
		t.Log(status)
		t.Error("First buffer full")
	}

	// For for the 2nd buf to be full.
	if !test.WaitStatus(3, s.relay, "log-buf2", "3") {
		status := s.relay.Status()
		t.Log(status)
		t.Fatal("2nd buf full")
	}

	// Unblock the relay's connect attempt.  This adds "Connected to API" (+4).
	s.connectChan <- true
	if !test.WaitStatus(1, s.relay, "ws", "Connected") {
		t.Fatal("Relay connects")
	}

	nSum := log.BUFFER_SIZE + 5
	got := test.WaitLog(s.recvChan, nSum)
	if len(got) < nSum {
		status := s.relay.Status()
		t.Log(status)
		t.Fatalf("Expected %d log entrires, got %d", nSum, len(got))
	}

	// When the 2nd buf overflows, it's reset and the overflow msg becomes buf2[0].
	// After re-connecting, a "Lost N log entries" msg is sent (+5).  So if buf size
	// is 10, then we should get:
	/**
	 * buf1:
	 *  1		I get the Send error (+1)
	 *  2		Lost connection to API (+2)
	 *  3		entry 1
	 *  4		entry 2
	 *  5		entry 3
	 *  6		entry 4
	 *  7		entry 5
	 *  8		entry 6
	 *  9		entry 7
	 * 10		entry 8
	 * ---		Lost 10 log entries (+5)
	 * 9-10 and 11-18 go into buf2, 19 overlows and resets buf2:
	 * buf2:
	 *  1		entry 19
	 *  2		entry 20
	 *  3       entry 21 (+3)
	 * ---		Connected to API (+4)
	 */
	expect := make([]proto.LogEntry, nSum)
	expect[0] = proto.LogEntry{Ts: test.Ts, Level: proto.LOG_INFO, Service: "test", Msg: "I get the Send error"}
	expect[1] = proto.LogEntry{Ts: test.Ts, Level: proto.LOG_WARNING, Service: "log", Msg: "Lost connection to API"}
	n := 2
	// entries 1-8 (buf size 10 - 2 = 8)
	for i := 1; i <= log.BUFFER_SIZE-2; i++ {
		expect[n] = proto.LogEntry{Ts: test.Ts, Level: proto.LOG_ERROR, Service: "test", Msg: fmt.Sprintf("b:%d", i)}
		n++
	}
	// n=10 (buf2[0] if buf size = 10)
	// This is logged after resending buf1 and before resending buf2:
	expect[n] = proto.LogEntry{Ts: test.Ts, Level: proto.LOG_WARNING, Service: "log", Msg: fmt.Sprintf("Lost %d log entries", log.BUFFER_SIZE)}
	n++
	// buf2:
	for i := log.BUFFER_SIZE*2 - 1; n < len(expect)-1; i++ {
		expect[n] = proto.LogEntry{Ts: test.Ts, Level: proto.LOG_ERROR, Service: "test", Msg: fmt.Sprintf("b:%d", i)}
		n++
	}
	// Last msg (+4):
	expect[len(expect)-1] = proto.LogEntry{Ts: test.Ts, Level: proto.LOG_INFO, Service: "log", Msg: "Connected to API"}

	t.Check(got, DeepEquals, expect)
}

/////////////////////////////////////////////////////////////////////////////
// Manager test suite
/////////////////////////////////////////////////////////////////////////////

type ManagerTestSuite struct {
	tmpDir      string
	sendChan    chan interface{}
	recvChan    chan interface{}
	connectChan chan bool
	client      *mock.WebsocketClient
	logFile     string
	logChan     chan proto.LogEntry
}

var _ = Suite(&ManagerTestSuite{})

func (s *ManagerTestSuite) SetUpSuite(t *C) {
	var err error
	s.tmpDir, err = ioutil.TempDir("/tmp", "agent-test")
	t.Assert(err, IsNil)

	if err := pct.Basedir.Init(s.tmpDir); err != nil {
		t.Fatal(err)
	}

	s.sendChan = make(chan interface{}, log.BUFFER_SIZE*3)
	s.recvChan = make(chan interface{}, log.BUFFER_SIZE*3)
	s.connectChan = make(chan bool)
	s.client = mock.NewWebsocketClient(nil, nil, s.sendChan, s.recvChan)

	s.logChan = make(chan proto.LogEntry, log.BUFFER_SIZE*3)
}

func (s *ManagerTestSuite) SetUpTest(t *C) {
	test.DrainLogChan(s.logChan)
	test.DrainRecvData(s.recvChan)
	test.DrainTraceChan(s.client.TraceChan)
	test.DrainBoolChan(s.connectChan)
	s.client.SetConnectChan(nil)
}

func (s *ManagerTestSuite) TearDownSuite(t *C) {
	if err := os.RemoveAll(s.tmpDir); err != nil {
		t.Error(err)
	}
}

// --------------------------------------------------------------------------

func (s *ManagerTestSuite) TestLogService(t *C) {
	config := &pc.Log{
		Level: "info",
	}
	pct.Basedir.WriteConfig("log", config)

	m := log.NewManager(s.client, s.logChan)
	err := m.Start()
	t.Assert(err, IsNil)

	defer m.Stop()

	relay := m.Relay()
	t.Assert(relay, NotNil)

	logger := pct.NewLogger(relay.LogChan(), "log-svc-test")
	logger.Info("i'm a log entry")

	// Log entry should be sent to API.
	got := test.WaitLog(s.recvChan, 3)
	if len(got) == 0 {
		t.Fatal("No log entries")
	}
	var gotLog proto.LogEntry
	for _, l := range got {
		if l.Service == "log-svc-test" {
			gotLog = l
			break
		}
	}
	t.Assert(gotLog, NotNil)
	expectLog := proto.LogEntry{Ts: test.Ts, Level: proto.LOG_INFO, Service: "log-svc-test", Msg: "i'm a log entry"}
	t.Check(gotLog, DeepEquals, expectLog)

	// Change log level and file
	newLogFile := s.logFile + "-2"
	defer os.Remove(newLogFile)

	config = &pc.Log{
		Level: "warning",
	}
	configData, err := json.Marshal(config)
	t.Assert(err, IsNil)

	cmd := &proto.Cmd{
		User:    "daniel",
		Service: "log",
		Cmd:     "SetConfig",
		Data:    configData,
	}

	gotReply := m.Handle(cmd)
	expectReply := cmd.Reply(config)
	t.Check(gotReply, DeepEquals, expectReply)

	logger.Warn("blah")
	got = test.WaitLog(s.recvChan, 3)
	gotLog = proto.LogEntry{}
	for _, l := range got {
		if l.Service == "log-svc-test" {
			gotLog = l
			break
		}
	}
	expectLog = proto.LogEntry{Ts: test.Ts, Level: proto.LOG_WARNING, Service: "log-svc-test", Msg: "blah"}
	t.Check(gotLog, DeepEquals, expectLog)

	// Verify new log config on disk.
	data, err := ioutil.ReadFile(pct.Basedir.ConfigFile("log"))
	t.Assert(err, IsNil)
	gotConfig := &pc.Log{}
	if err := json.Unmarshal(data, gotConfig); err != nil {
		t.Fatal(err)
	}
	assert.Equal(t, config, gotConfig)

	// GetConfig
	cmd = &proto.Cmd{
		User:    "daniel",
		Service: "log",
		Cmd:     "GetConfig",
	}
	reply := m.Handle(cmd)
	t.Assert(reply.Error, Equals, "")
	t.Assert(reply.Data, NotNil)
	gotConfigRes := []proto.AgentConfig{}
	if err := json.Unmarshal(reply.Data, &gotConfigRes); err != nil {
		t.Fatal(err)
	}
	expectConfigRes := []proto.AgentConfig{
		{
			Service: "log",
			Set:     "{\"Level\":\"warning\"}",
			Running: "{\"Level\":\"warning\"}",
			Updated: time.Time{}.UTC(),
		},
	}
	assert.Equal(t, expectConfigRes, gotConfigRes)

	// Status (internal status of log and relay)
	status := m.Status()
	t.Check(status["ws"], Equals, "Connected")
	t.Check(status["log-level"], Equals, "warning")
}

func (s *ManagerTestSuite) TestReconnect(t *C) {
	config := &pc.Log{
		Level: "info",
	}
	if err := pct.Basedir.WriteConfig("log", config); err != nil {
		t.Fatal(err)
	}

	m := log.NewManager(s.client, s.logChan)
	err := m.Start()
	t.Assert(err, IsNil)

	defer m.Stop()

	// Wait for relay to start.
	got := test.WaitLog(s.recvChan, 2)
	expect := []proto.LogEntry{
		{Ts: test.Ts, Level: proto.LOG_INFO, Service: "log", Msg: "Started"},
		{Ts: test.Ts, Level: proto.LOG_INFO, Service: "log", Msg: "Connected to API"},
	}
	t.Check(got, DeepEquals, expect)

	// Log something before we reconnect.
	relay := m.Relay()
	t.Assert(relay, NotNil)
	logger := pct.NewLogger(relay.LogChan(), "log-svc-test")
	logger.Info("before reconnect")

	s.client.SetConnectChan(s.connectChan)

	// Tell relay to disconnect and reconnect.
	cmd := &proto.Cmd{
		User:    "daniel",
		Service: "log",
		Cmd:     "Reconnect",
	}
	reply := m.Handle(cmd)
	t.Check(reply.Error, Equals, "")

	// Wait for relay to reconnect.
	select {
	case <-s.connectChan:
	case <-time.After(2 * time.Second):
		t.Fatal("Relay did not reconnect")
	}

	// Let relay reconnect
	s.connectChan <- true
	if !test.WaitStatus(1, relay, "ws", "Connected") {
		t.Fatal("Relay connects")
	}

	logger.Info("after reconnect")

	got = test.WaitLog(s.recvChan, 4)
	expect = []proto.LogEntry{
		{Ts: test.Ts, Level: proto.LOG_INFO, Service: "log-svc-test", Msg: "before reconnect"},
		{Ts: test.Ts, Level: proto.LOG_WARNING, Service: "log", Msg: "Lost connection to API"},
		{Ts: test.Ts, Level: proto.LOG_INFO, Service: "log", Msg: "Connected to API"},
		{Ts: test.Ts, Level: proto.LOG_INFO, Service: "log-svc-test", Msg: "after reconnect"},
	}
	t.Check(got, DeepEquals, expect)
}
