package mongo

import (
	"context"
	"testing"

	"github.com/percona/pmm/proto"
	"github.com/percona/qan-agent/pct"
	"github.com/percona/qan-agent/test/mock"
	"github.com/stretchr/testify/assert"
)

func TestMongoAnalyzer_StartStopStatus(t *testing.T) {
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
	expect := map[string]string{
		"plugin-collector-in":          "0",
		"plugin-collector-out":         "1",
		"plugin-parser-docs-in":        "0",
		"plugin-parser-reports-out":    "0",
		"plugin-sender-out":            "0",
		"plugin":                       "Running",
		"plugin-parser-interval-start": "",
		"plugin-parser-interval-end":   "",
		"plugin-sender-in":             "0",
	}
	actual := plugin.Status()
	actual["plugin-parser-interval-start"] = ""
	actual["plugin-parser-interval-end"] = ""
	assert.Equal(t, expect, actual)

	err = plugin.Stop()
	assert.Nil(t, err)
	assert.Equal(t, map[string]string{serviceName: "Not running"}, plugin.Status())
}
