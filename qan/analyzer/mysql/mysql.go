package mysql

import (
	"context"
	"fmt"
	"time"

	"github.com/percona/pmm/proto"
	pc "github.com/percona/pmm/proto/config"
	"github.com/percona/qan-agent/data"
	"github.com/percona/qan-agent/mrms"
	"github.com/percona/qan-agent/mysql"
	"github.com/percona/qan-agent/pct"
	"github.com/percona/qan-agent/qan/analyzer"
	"github.com/percona/qan-agent/qan/analyzer/mysql/config"
	"github.com/percona/qan-agent/qan/analyzer/mysql/factory"
	"github.com/percona/qan-agent/qan/analyzer/mysql/iter"
	"github.com/percona/qan-agent/qan/analyzer/mysql/worker"
	"github.com/percona/qan-agent/qan/analyzer/mysql/worker/perfschema"
	"github.com/percona/qan-agent/qan/analyzer/mysql/worker/slowlog"
	"github.com/percona/qan-agent/ticker"
)

func New(ctx context.Context, protoInstance proto.Instance) analyzer.Analyzer {
	// Get available services from ctx
	services, _ := ctx.Value("services").(map[string]interface{})

	// Get services we need
	logger, _ := services["logger"].(*pct.Logger)
	clock, _ := services["clock"].(ticker.Manager)
	spool, _ := services["spool"].(data.Spooler)
	mrms, _ := services["mrms"].(mrms.Monitor)

	// Create internal services we need
	logChan := logger.LogChan()
	iterFactory := factory.NewRealIntervalIterFactory(logChan)
	slowlogWorkerFactory := slowlog.NewRealWorkerFactory(logChan)
	perfschemaWorkerFactory := perfschema.NewRealWorkerFactory(logChan)
	mysqlConnFactory := &mysql.RealConnectionFactory{}

	// return initialized MySQLAnalyzer
	return &MySQLAnalyzer{
		// on initialization config and analyzer are uninitialized
		config:   pc.QAN{},
		analyzer: nil,
		// initialize
		protoInstance:           protoInstance,
		logger:                  logger,
		clock:                   clock,
		spool:                   spool,
		mrms:                    mrms,
		iterFactory:             iterFactory,
		slowlogWorkerFactory:    slowlogWorkerFactory,
		perfschemaWorkerFactory: perfschemaWorkerFactory,
		mysqlConnFactory:        mysqlConnFactory,
	}
}

// MySQLAnalyzer is a proxy Analyzer between QAN manager
// and MySQL implementations of Slowlog Analyzer and Perfschema Analyzer
type MySQLAnalyzer struct {
	// on initialization config and analyzer are uninitialized
	config   pc.QAN
	analyzer analyzer.Analyzer
	// services initialized in New
	protoInstance           proto.Instance
	logger                  *pct.Logger
	clock                   ticker.Manager
	spool                   data.Spooler
	mrms                    mrms.Monitor
	iterFactory             iter.IntervalIterFactory
	slowlogWorkerFactory    slowlog.WorkerFactory
	perfschemaWorkerFactory perfschema.WorkerFactory
	mysqlConnFactory        mysql.ConnectionFactory
	// real analyzer channels
	tickChan    chan time.Time
	restartChan chan proto.Instance
}

// SetConfig sets the config
func (m *MySQLAnalyzer) SetConfig(setConfig pc.QAN) {
	m.config = setConfig
	if m.analyzer != nil {
		m.analyzer.SetConfig(m.config)
	}
}

// Config returns analyzer running configuration
func (m *MySQLAnalyzer) Config() pc.QAN {
	if m.analyzer != nil {
		m.config = m.analyzer.Config()
	}

	return m.config
}

// Start starts analyzer but doesn't wait until it exits
func (m *MySQLAnalyzer) Start() error {
	setConfig := m.Config()

	// Create a MySQL connection.
	mysqlConn := m.mysqlConnFactory.Make(m.protoInstance.DSN)

	// Validate and transform the set config and into a running config.
	config, err := config.ValidateConfig(setConfig)
	if err != nil {
		return fmt.Errorf("invalid QAN config: %s", err)
	}

	// Add the MySQL DSN to the MySQL restart monitor. If MySQL restarts,
	// the analyzer will stop its worker and re-configure MySQL.
	restartChan := m.mrms.Add(m.protoInstance)

	// Make a chan on which the clock will tick at even intervals:
	// clock -> tickChan -> iter -> analyzer -> worker
	tickChan := make(chan time.Time, 1)
	m.clock.Add(tickChan, config.Interval, true)

	name := m.logger.Service()
	logChan := m.logger.LogChan()
	var worker worker.Worker
	analyzerType := config.CollectFrom
	switch analyzerType {
	case "slowlog":
		worker = m.slowlogWorkerFactory.Make(name+"-worker", config, mysqlConn)
	case "perfschema":
		worker = m.perfschemaWorkerFactory.Make(name+"-worker", mysqlConn)
	default:
		panic("Invalid analyzerType: " + analyzerType)
	}
	worker.SetConfig(config)

	// Create and start a new analyzer. This should return immediately.
	// The analyzer will configure MySQL, start its iter, then run it worker
	// for each interval.
	m.analyzer = NewRealAnalyzer(
		pct.NewLogger(logChan, name),
		config,
		m.iterFactory.Make(analyzerType, mysqlConn, tickChan),
		mysqlConn,
		restartChan,
		worker,
		m.clock,
		m.spool,
	)
	m.restartChan = restartChan

	return m.analyzer.Start()
}

// Status returns list of statuses
func (m *MySQLAnalyzer) Status() map[string]string {
	if m.analyzer != nil {
		return m.analyzer.Status()
	}

	service := m.logger.Service()
	status := "Not running"

	return map[string]string{
		service: status,
	}
}

// Stop stops running analyzer, waits until it stops
func (m *MySQLAnalyzer) Stop() error {
	if m.analyzer == nil {
		return nil
	}

	a := m.analyzer
	tickChan := m.tickChan
	restartChan := m.restartChan
	m.analyzer = nil
	m.tickChan = nil
	m.restartChan = nil

	// Stop ticking on this tickChan. Other services receiving ticks at the same
	// interval are not affected.
	m.clock.Remove(tickChan)

	// Stop watching this MySQL instance. Other services watching this MySQL
	// instance are not affected.
	m.mrms.Remove(m.protoInstance.UUID, restartChan)

	return a.Stop()
}

func (m *MySQLAnalyzer) GetDefaults(uuid string) map[string]interface{} {
	// Configuration
	cfg := map[string]interface{}{
		"CollectFrom":     m.config.CollectFrom,
		"Interval":        m.config.Interval,
		"MaxSlowLogSize":  m.config.MaxSlowLogSize,
		"RetainSlowLogs":  m.config.RetainSlowLogs,
		"SlowLogRotation": m.config.SlowLogRotation,
		"ExampleQueries":  m.config.ExampleQueries,
		"ReportLimit":     m.config.ReportLimit,
	}

	// Info from SHOW GLOBAL STATUS
	mysqlInstance := m.protoInstance
	mysqlConn := m.mysqlConnFactory.Make(mysqlInstance.DSN)
	mysqlConn.Connect()
	defer mysqlConn.Close()
	info := config.ReadInfoFromShowGlobalStatus(mysqlConn) // Read current values
	for k, v := range info {
		cfg[k] = v
	}

	return cfg
}

// String returns human readable identification of Analyzer
func (m *MySQLAnalyzer) String() string {
	return m.analyzer.String()
}
