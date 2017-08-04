package mongo

import (
	"context"
	"fmt"
	"testing"

	"github.com/percona/pmm/proto"
	"github.com/percona/qan-agent/pct"
	"github.com/percona/qan-agent/test/mock"
	"github.com/percona/qan-agent/test/profiling"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMongo_StartStopStatus(t *testing.T) {
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
	}

	dataChan := make(chan interface{})
	logChan := make(chan proto.LogEntry)

	serviceName := "plugin"

	// Expose some global services to plugins
	ctx := context.Background()
	ctx = context.WithValue(ctx, "services", map[string]interface{}{
		"logger": pct.NewLogger(logChan, serviceName),
		"spool":  mock.NewSpooler(dataChan),
		"clock":  mock.NewClock(),
	})

	protoInstance := proto.Instance{}
	plugin := New(ctx, protoInstance)

	assert.Equal(t, map[string]string{serviceName: "Not running"}, plugin.Status())
	err := plugin.Start()
	require.NoError(t, err)
	// some values are unpredictable, e.g. time but they should exist
	shouldExist := "<should exist>"

	expect := map[string]string{
		"plugin": "Running",
	}
	for _, dbName := range dbNames {
		t := map[string]string{
			"plugin-%s-collector-profile":          "Profiling disabled. Please enable profiling for this database or whole MongoDB server (https://docs.mongodb.com/manual/tutorial/manage-the-database-profiler/).",
			"plugin-%s-collector-iterator-counter": "1",
			"plugin-%s-collector-iterator-created": shouldExist,
			"plugin-%s-collector-started":          shouldExist,
			"plugin-%s-parser-started":             shouldExist,
			"plugin-%s-parser-interval-start":      shouldExist,
			"plugin-%s-parser-interval-end":        shouldExist,
			"plugin-%s-sender-started":             shouldExist,
		}
		m := map[string]string{}
		for k, v := range t {
			m[fmt.Sprintf(k, dbName)] = v
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
