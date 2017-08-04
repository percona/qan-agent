package mongo_test

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"testing"
	"time"

	"github.com/percona/pmm/proto"
	pc "github.com/percona/pmm/proto/config"
	"github.com/percona/qan-agent/instance"
	"github.com/percona/qan-agent/pct"
	"github.com/percona/qan-agent/qan"
	"github.com/percona/qan-agent/qan/analyzer/factory"
	"github.com/percona/qan-agent/test"
	"github.com/percona/qan-agent/test/mock"
	"github.com/percona/qan-agent/test/profiling"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRealStartTool(t *testing.T) {
	dbNames := []string{
		"admin",
		"local",
		"test",
	}

	// disable profiling as we only want to test if factory works
	for _, dbName := range dbNames {
		url := "/" + dbName
		err := profiling.Disable(url)
		require.NoError(t, err)
		profiling.Drop(url)
		err = profiling.Enable(url)
		require.NoError(t, err)
	}

	logChan := make(chan proto.LogEntry)
	dataChan := make(chan interface{})
	spool := mock.NewSpooler(dataChan)
	clock := mock.NewClock()
	mrm := mock.NewMrmsMonitor()
	logger := pct.NewLogger(logChan, "TestRealStartTool")
	links := map[string]string{}
	api := mock.NewAPI("http://localhost", "http://localhost", "abc-123-def", links)
	instanceRepo := instance.NewRepo(logger, "", api)
	f := factory.New(
		logChan,
		spool,
		clock,
		mrm,
		instanceRepo,
	)
	m := qan.NewManager(logger, instanceRepo, f)
	err := m.Start()
	require.NoError(t, err)

	protoInstance := proto.Instance{
		UUID:      "12345678",
		Subsystem: "mongo",
	}
	err = instanceRepo.Add(protoInstance, false)
	require.NoError(t, err)

	// Create the qan config.
	config := &pc.QAN{
		UUID:           protoInstance.UUID,
		Interval:       1, // 1 second
		ExampleQueries: true,
	}

	// Send a StartTool cmd with the qan config to start an analyzer.
	now := time.Now()
	qanConfig, _ := json.Marshal(config)
	cmd := &proto.Cmd{
		User:      "kdz",
		Ts:        now,
		AgentUUID: "123",
		Service:   "qan",
		Cmd:       "StartTool",
		Data:      qanConfig,
	}
	reply := m.Handle(cmd)
	assert.Equal(t, "", reply.Error)

	// The manager writes the qan config to disk.
	data, err := ioutil.ReadFile(pct.Basedir.ConfigFile("qan-" + config.UUID))
	require.NoError(t, err)
	gotConfig := &pc.QAN{}
	err = json.Unmarshal(data, gotConfig)
	require.NoError(t, err)
	assert.Equal(t, config, gotConfig)

	// Now the manager and analyzer should be running.
	shouldExist := "<should exist>"
	actual := m.Status()

	pluginName := fmt.Sprintf("%s-analyzer-%s-%s", cmd.Service, protoInstance.Subsystem, protoInstance.UUID)
	expect := map[string]string{
		"qan":      "Running",
		pluginName: "Running",
	}
	for _, dbName := range dbNames {
		t := map[string]string{
			"%s-collector-profile":          "Profiling enabled for all queries (ratelimit: 1)",
			"%s-collector-iterator-counter": "1",
			"%s-collector-iterator-created": shouldExist,
			"%s-collector-started":          shouldExist,
			"%s-parser-started":             shouldExist,
			"%s-parser-interval-start":      shouldExist,
			"%s-parser-interval-end":        shouldExist,
			"%s-sender-started":             shouldExist,
		}
		m := map[string]string{}
		for k, v := range t {
			prefix := fmt.Sprintf("%s-%s", pluginName, dbName)
			key := fmt.Sprintf(k, prefix)
			m[key] = v
		}
		expect = merge(expect, m)
	}

	for k, v := range expect {
		if v == shouldExist {
			assert.Contains(t, actual, k)
			delete(actual, k)
			delete(expect, k)
		}
	}
	expectJSON, err := json.Marshal(expect)
	require.NoError(t, err)
	actualJSON, err := json.Marshal(actual)
	require.NoError(t, err)
	assert.JSONEq(t, string(expectJSON), string(actualJSON))

	// Try to start the same analyzer again. It results in an error because
	// double tooling is not allowed.
	reply = m.Handle(cmd)
	assert.Equal(t, "Query Analytics is already running on instance 12345678. To reconfigure or restart Query Analytics, stop then start it again.", reply.Error)

	// Send a StopTool cmd to stop the analyzer.
	now = time.Now()
	cmd = &proto.Cmd{
		User:      "daniel",
		Ts:        now,
		AgentUUID: "123",
		Service:   "qan",
		Cmd:       "StopTool",
		Data:      []byte(protoInstance.UUID),
	}
	reply = m.Handle(cmd)
	assert.Equal(t, "", reply.Error)

	// Now the manager is still running, but the analyzer is not.
	actual = m.Status()
	expect = map[string]string{
		"qan": "Running",
	}
	assert.Equal(t, expect, actual)

	// And the manager has removed the qan config from disk so next time
	// the agent starts the analyzer is not started.
	assert.False(t, test.FileExists(pct.Basedir.ConfigFile("qan-"+protoInstance.UUID)))

	// StopTool should be idempotent, so send it again and expect no error.
	reply = m.Handle(cmd)
	assert.Equal(t, "", reply.Error)

	// Stop the manager.
	err = m.Stop()
	require.NoError(t, err)
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
