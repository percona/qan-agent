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
		"plugin":                   "Running",
		"plugin-collector-profile": "was: 2, slowms: 100",
	}
	actual := plugin.Status()
	delete(actual, "plugin-collector-in")
	delete(actual, "plugin-parser-interval-start")
	delete(actual, "plugin-parser-interval-end")
	assert.Equal(t, expect, actual)

	err = plugin.Stop()
	assert.Nil(t, err)
	assert.Equal(t, map[string]string{serviceName: "Not running"}, plugin.Status())
}
