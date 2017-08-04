package profiler

import (
	"fmt"
	"sync"

	"github.com/percona/pmgo"
	pc "github.com/percona/pmm/proto/config"
	"github.com/percona/qan-agent/data"
	"github.com/percona/qan-agent/pct"
	"github.com/percona/qan-agent/qan/analyzer/mongo/profiler/collector"
	"github.com/percona/qan-agent/qan/analyzer/mongo/profiler/parser"
	"github.com/percona/qan-agent/qan/analyzer/mongo/profiler/sender"
)

func NewMonitor(
	dialInfo *pmgo.DialInfo,
	dialer pmgo.Dialer,
	logger *pct.Logger,
	spool data.Spooler,
	config pc.QAN,
) *monitor {
	return &monitor{
		dialInfo: dialInfo,
		dialer:   dialer,
		logger:   logger,
		spool:    spool,
		config:   config,
	}
}

type monitor struct {
	// dependencies
	dialInfo *pmgo.DialInfo
	dialer   pmgo.Dialer
	spool    data.Spooler
	logger   *pct.Logger
	config   pc.QAN

	// internal services
	services []services

	// state
	sync.Mutex      // Lock() to protect internal consistency of the service
	running    bool // Is this service running?
}

func (self *monitor) Start() error {
	self.Lock()
	defer self.Unlock()

	if self.running {
		return nil
	}

	defer func() {
		// if we failed to start
		if !self.running {
			// be sure that any started internal service is shutdown
			for _, s := range self.services {
				s.Stop()
			}
			self.services = nil
		}
	}()

	// create collector and start it
	c := collector.New(self.dialInfo, self.dialer)
	docsChan, err := c.Start()
	if err != nil {
		return err
	}
	self.services = append(self.services, c)

	// create parser and start it
	p := parser.New(docsChan, self.config)
	reportChan, err := p.Start()
	if err != nil {
		return err
	}
	self.services = append(self.services, p)

	// create sender and start it
	s := sender.New(reportChan, self.spool, self.logger)
	err = s.Start()
	if err != nil {
		return err
	}
	self.services = append(self.services, s)

	self.running = true
	return nil
}

func (self *monitor) Stop() {
	self.Lock()
	defer self.Unlock()

	if !self.running {
		return
	}

	// stop internal services
	for _, s := range self.services {
		s.Stop()
	}

	self.running = false
}

// Status returns list of statuses
func (self *monitor) Status() map[string]string {
	statuses := map[string]string{}

	for _, s := range self.services {
		for k, v := range s.Status() {
			statuses[fmt.Sprintf("%s-%s", s.Name(), k)] = v
		}
	}

	return statuses
}

type services interface {
	Status() map[string]string
	Stop()
	Name() string
}
