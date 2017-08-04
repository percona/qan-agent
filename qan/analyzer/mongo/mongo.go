package mongo

import (
	"context"
	"fmt"
	"sync"

	"github.com/percona/pmgo"
	"github.com/percona/pmm/proto"
	pc "github.com/percona/pmm/proto/config"
	"github.com/percona/qan-agent/data"
	"github.com/percona/qan-agent/pct"
	"github.com/percona/qan-agent/qan/analyzer"
	"github.com/percona/qan-agent/qan/analyzer/mongo/profiler"
	"github.com/percona/qan-agent/qan/analyzer/mongo/profiler/parser"
)

func New(ctx context.Context, protoInstance proto.Instance) analyzer.Analyzer {
	// Get available services from ctx
	services, _ := ctx.Value("services").(map[string]interface{})

	// Get services we need
	logger, _ := services["logger"].(*pct.Logger)
	spool, _ := services["spool"].(data.Spooler)

	// return initialized MongoAnalyzer
	return &MongoAnalyzer{
		protoInstance: protoInstance,
		spool:         spool,
		logger:        logger,
	}
}

// MongoAnalyzer
type MongoAnalyzer struct {
	// dependencies
	protoInstance proto.Instance

	// dependencies from ctx
	logger *pct.Logger
	spool  data.Spooler

	// dependency from setter SetConfig
	config pc.QAN

	// profiler
	profiler Profiler

	// state
	sync.RWMutex      // Lock() to protect internal consistency of the service
	running      bool // Is this service running?
}

// SetConfig sets the config
func (m *MongoAnalyzer) SetConfig(setConfig pc.QAN) {
	m.config = setConfig
}

// Config returns analyzer running configuration
func (m *MongoAnalyzer) Config() pc.QAN {
	return m.config
}

// Start starts analyzer but doesn't wait until it exits
func (m *MongoAnalyzer) Start() error {
	m.Lock()
	defer m.Unlock()
	if m.running {
		return nil
	}

	// get the dsn from instance
	dsn := m.protoInstance.DSN

	// if dsn is incorrect we should exit immediately as this is not gonna correct itself
	dialInfo, err := pmgo.ParseURL(dsn)
	if err != nil {
		return err
	}
	dialer := pmgo.NewDialer()

	m.profiler = profiler.New(
		dialInfo,
		dialer,
		m.logger,
		m.spool,
		m.config,
	)
	m.profiler.Start()

	m.running = true
	return nil
}

// Status returns list of statuses
func (m *MongoAnalyzer) Status() map[string]string {
	m.RLock()
	defer m.RUnlock()

	statuses := map[string]string{}
	service := m.logger.Service()

	if !m.running {
		statuses[service] = "Not running"
		return statuses
	}

	for k, v := range m.profiler.Status() {
		statuses[fmt.Sprintf("%s-%s", service, k)] = v
	}

	statuses[service] = "Running"
	return statuses
}

// Stop stops running analyzer, waits until it stops
func (m *MongoAnalyzer) Stop() error {
	m.Lock()
	defer m.Unlock()
	if !m.running {
		return nil
	}

	// stop monitoring databases
	m.profiler.Stop()
	m.profiler = nil

	m.running = false
	return nil
}

func (m *MongoAnalyzer) GetDefaults(uuid string) map[string]interface{} {
	// verify config
	if m.config.Interval == 0 {
		m.config.Interval = parser.DefaultInterval
		m.config.ExampleQueries = parser.DefaultExampleQueries
	}

	return map[string]interface{}{
		"Interval":       m.config.Interval,
		"ExampleQueries": m.config.ExampleQueries,
	}
}

// String returns human readable identification of Analyzer
func (m *MongoAnalyzer) String() string {
	return ""
}

type Profiler interface {
	Start() error
	Stop() error
	Status() map[string]string
}
