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

	serviceName := "test"

	// Expose some global services to plugins
	ctx := context.Background()
	ctx = context.WithValue(ctx, "services", map[string]interface{}{
		"logger": pct.NewLogger(logChan, serviceName),
		"spool":  mock.NewSpooler(dataChan),
		"clock":  mock.NewClock(),
	})

	protoInstance := proto.Instance{}
	plugin := New(ctx, protoInstance)

	assert.Equal(t, map[string]string{serviceName: "not running"}, plugin.Status())
	err := plugin.Start()
	assert.Nil(t, err)
	assert.Equal(t, map[string]string{serviceName: "running"}, plugin.Status())

	err = plugin.Stop()
	assert.Nil(t, err)
	assert.Equal(t, map[string]string{serviceName: "not running"}, plugin.Status())
}
