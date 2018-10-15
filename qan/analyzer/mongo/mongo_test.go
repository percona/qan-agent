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
	// Disable profiling as we only want to test if factory works.
	p := profiling.New("")
	err := p.DisableAll()
	require.NoError(t, err)

	dataChan := make(chan interface{})
	logChan := make(chan proto.LogEntry)

	serviceName := "plugin"

	// Expose some global services to plugins.
	ctx := context.Background()
	ctx = context.WithValue(ctx, "services", map[string]interface{}{
		"logger": pct.NewLogger(logChan, serviceName),
		"spool":  mock.NewSpooler(dataChan),
		"clock":  mock.NewClock(),
	})

	protoInstance := proto.Instance{}
	plugin := New(ctx, protoInstance)

	assert.Equal(t, map[string]string{serviceName: "Not running"}, plugin.Status())
	err = plugin.Start()
	require.NoError(t, err)

	// Some values are unpredictable, e.g. time but they should exist.
	shouldExist := "<should exist>"
	mayExist := "<may exist>"

	pluginName := "plugin"
	expect := map[string]string{
		pluginName: "Running",
		pluginName + "-aggregator-interval-start": shouldExist,
		pluginName + "-aggregator-interval-end":   shouldExist,
		pluginName + "-servers":                   shouldExist,
	}
	dbNames, err := p.DatabaseNames()
	require.NoError(t, err)
	require.NotEmpty(t, dbNames)
	for _, dbName := range dbNames {
		t := map[string]string{
			"%s-collector-profile-%s":                  "Profiling disabled. Please enable profiling for this database or whole MongoDB server (https://docs.mongodb.com/manual/tutorial/manage-the-database-profiler/).",
			"%s-collector-iterator-counter-%s":         "1",
			"%s-collector-iterator-restart-counter-%s": mayExist,
			"%s-collector-iterator-created-%s":         shouldExist,
		}
		m := map[string]string{}
		for k, v := range t {
			key := fmt.Sprintf(k, pluginName, dbName)
			m[key] = v
		}
		expect = merge(expect, m)
	}

	actual := plugin.Status()
	for k, v := range expect {
		switch v {
		case shouldExist:
			assert.Contains(t, actual, k)
		case mayExist:
		default:
			continue
		}
		delete(actual, k)
		delete(expect, k)
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
