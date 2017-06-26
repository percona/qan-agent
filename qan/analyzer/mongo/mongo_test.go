package mongo

import (
	"context"
	"testing"

	"github.com/percona/pmm/proto"
	"github.com/percona/qan-agent/pct"
	"github.com/percona/qan-agent/test/mock"
	"github.com/stretchr/testify/assert"
)

func TestMongo_StartStopStatus(t *testing.T) {
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
	assert.Nil(t, err)
	// some values are unpredictable, e.g. time but they should exist
	shouldExist := "<should exist>"
	expect := map[string]string{
		"plugin":                            "Running",
		"plugin-collector-profile":          "Profiling enabled for all queries (ratelimit: 1)",
		"plugin-collector-iterator-counter": "1",
		"plugin-collector-iterator-created": shouldExist,
		"plugin-collector-started":          shouldExist,
		"plugin-parser-started":             shouldExist,
		"plugin-parser-interval-start":      shouldExist,
		"plugin-parser-interval-end":        shouldExist,
		"plugin-parser-docs-in":             "1",
		"plugin-parser-docs-ok":             "1",
		"plugin-sender-started":             shouldExist,
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
	assert.Nil(t, err)
	assert.Equal(t, map[string]string{serviceName: "Not running"}, plugin.Status())
}
