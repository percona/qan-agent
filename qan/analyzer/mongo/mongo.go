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
	"github.com/percona/qan-agent/qan"
	"github.com/percona/qan-agent/qan/analyzer/mongo/collector"
	"github.com/percona/qan-agent/qan/analyzer/mongo/parser"
	"github.com/percona/qan-agent/qan/analyzer/mongo/sender"
	"gopkg.in/mgo.v2"
)

func New(ctx context.Context, protoInstance proto.Instance) qan.Analyzer {
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
	spool  data.Spooler
	logger *pct.Logger

	// dependency from setter SetConfig
	config pc.QAN

	// internal services
	services []services

	// state
	running bool
	sync.RWMutex
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

	defer func() {
		// if we failed to start
		if !m.running {
			// be sure that any started internal service is shutdown
			for _, s := range m.services {
				s.Stop()
			}
			m.services = nil
		}
	}()

	// get the dsn from instance
	dsn := m.protoInstance.DSN

	// if dsn is incorrect we should exit immediately as this is not gonna correct itself
	dialInfo, err := mgo.ParseURL(dsn)
	if err != nil {
		return err
	}
	dialer := pmgo.NewDialer()

	// create collector and start it
	collector := collector.New(dialInfo, dialer)
	docsChan, err := collector.Start()
	if err != nil {
		return err
	}
	m.services = append(m.services, collector)

	// create parser and start it
	parser := parser.New(docsChan, m.config)
	reportChan, err := parser.Start()
	if err != nil {
		return err
	}
	m.services = append(m.services, parser)

	// create sender and start it
	sender := sender.New(reportChan, m.spool, m.logger)
	err = sender.Start()
	if err != nil {
		return err
	}
	m.services = append(m.services, sender)

	m.running = true
	return nil
}

// Status returns list of statuses
func (m *MongoAnalyzer) Status() map[string]string {
	m.RLock()
	defer m.RUnlock()

	service := m.logger.Service()
	status := "Not running"
	if m.running {
		status = "Running"
	}

	statuses := map[string]string{}

	for _, s := range m.services {
		for k, v := range s.Status() {
			statuses[fmt.Sprintf("%s-%s-%s", service, s.Name(), k)] = v
		}
	}

	statuses[service] = status
	return statuses
}

// Stop stops running analyzer, waits until it stops
func (m *MongoAnalyzer) Stop() error {
	m.Lock()
	m.Unlock()
	if !m.running {
		return nil
	}

	// stop internal services
	for _, s := range m.services {
		s.Stop()
	}
	m.services = nil

	m.running = false
	return nil
}

func (m *MongoAnalyzer) GetDefaults(uuid string) map[string]interface{} {
	return map[string]interface{}{}
}

// String returns human readable identification of Analyzer
func (m *MongoAnalyzer) String() string {
	return ""
}

type services interface {
	Status() map[string]string
	Stop()
	Name() string
}
