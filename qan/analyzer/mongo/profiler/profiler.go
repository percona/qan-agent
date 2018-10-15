package profiler

import (
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/percona/pmgo"
	pc "github.com/percona/pmm/proto/config"

	"github.com/percona/qan-agent/data"
	"github.com/percona/qan-agent/pct"
	"github.com/percona/qan-agent/qan/analyzer/mongo/profiler/aggregator"
	"github.com/percona/qan-agent/qan/analyzer/mongo/profiler/sender"
)

func New(
	dialInfo *pmgo.DialInfo,
	dialer pmgo.Dialer,
	logger *pct.Logger,
	spool data.Spooler,
	config pc.QAN,
) *profiler {
	return &profiler{
		dialInfo: dialInfo,
		dialer:   dialer,
		logger:   logger,
		spool:    spool,
		config:   config,
	}
}

type profiler struct {
	// dependencies
	dialInfo *pmgo.DialInfo
	dialer   pmgo.Dialer
	spool    data.Spooler
	logger   *pct.Logger
	config   pc.QAN

	// internal deps
	monitors   *monitors
	session    pmgo.SessionManager
	aggregator *aggregator.Aggregator
	sender     *sender.Sender

	// state
	sync.RWMutex                 // Lock() to protect internal consistency of the service
	running      bool            // Is this service running?
	doneChan     chan struct{}   // close(doneChan) to notify goroutines that they should shutdown
	wg           *sync.WaitGroup // Wait() for goroutines to stop after being notified they should shutdown
}

// Start starts analyzer but doesn't wait until it exits
func (self *profiler) Start() error {
	self.Lock()
	defer self.Unlock()
	if self.running {
		return nil
	}

	// create new session
	session, err := createSession(self.dialInfo, self.dialer)
	if err != nil {
		return err
	}
	self.session = session

	// create aggregator which collects documents and aggregates them into qan report
	self.aggregator = aggregator.New(time.Now(), self.config)
	reportChan := self.aggregator.Start()

	// create sender which sends qan reports and start it
	self.sender = sender.New(reportChan, self.spool, self.logger)
	err = self.sender.Start()
	if err != nil {
		return err
	}

	f := func(
		session pmgo.SessionManager,
		dbName string,
	) *monitor {
		return NewMonitor(
			session,
			dbName,
			self.aggregator,
			self.logger,
			self.spool,
			self.config,
		)
	}

	// create monitors service which we use to periodically scan server for new/removed databases
	self.monitors = NewMonitors(
		session,
		f,
	)

	// create new channel over which
	// we will tell goroutine it should close
	self.doneChan = make(chan struct{})

	// start a goroutine and Add() it to WaitGroup
	// so we could later Wait() for it to finish
	self.wg = &sync.WaitGroup{}
	self.wg.Add(1)

	// create ready sync.Cond so we could know when goroutine actually started getting data from db
	ready := sync.NewCond(&sync.Mutex{})
	ready.L.Lock()
	defer ready.L.Unlock()

	go start(
		self.monitors,
		self.wg,
		self.doneChan,
		ready,
	)

	// wait until we actually fetch data from db
	ready.Wait()

	self.running = true
	return nil
}

// Status returns list of statuses
func (self *profiler) Status() map[string]string {
	self.RLock()
	defer self.RUnlock()
	if !self.running {
		return nil
	}

	statuses := &sync.Map{}
	monitors := self.monitors.GetAll()

	wg := &sync.WaitGroup{}
	wg.Add(len(monitors))
	for dbName, m := range monitors {
		go func(dbName string, m *monitor) {
			defer wg.Done()
			for k, v := range m.Status() {
				key := fmt.Sprintf("%s-%s", k, dbName)
				statuses.Store(key, v)
			}
		}(dbName, m)
	}

	wg.Add(1)
	go func() {
		defer wg.Done()
		for k, v := range self.aggregator.Status() {
			key := fmt.Sprintf("%s-%s", "aggregator", k)
			statuses.Store(key, v)
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		for k, v := range self.sender.Status() {
			key := fmt.Sprintf("%s-%s", "sender", k)
			statuses.Store(key, v)
		}
	}()

	wg.Wait()

	statusesMap := map[string]string{}
	statuses.Range(func(key, value interface{}) bool {
		statusesMap[key.(string)] = value.(string)
		return true
	})
	statusesMap["servers"] = strings.Join(self.session.LiveServers(), ", ")
	return statusesMap
}

// Stop stops running analyzer, waits until it stops
func (self *profiler) Stop() error {
	self.Lock()
	defer self.Unlock()
	if !self.running {
		return nil
	}

	// notify goroutine to close
	close(self.doneChan)

	// wait for goroutine to exit
	self.wg.Wait()

	// stop aggregator; do it after goroutine is closed
	self.aggregator.Stop()

	// stop sender; do it after goroutine is closed
	self.sender.Stop()

	// close the session; do it after goroutine is closed
	self.session.Close()

	// set state to "not running"
	self.running = false
	return nil
}

func start(
	monitors *monitors,
	wg *sync.WaitGroup,
	doneChan <-chan struct{},
	ready *sync.Cond,
) {
	// signal WaitGroup when goroutine finished
	defer wg.Done()

	// stop all monitors
	defer monitors.StopAll()

	// monitor all databases
	monitors.MonitorAll()

	// signal we started monitoring
	signalReady(ready)

	// loop to periodically refresh monitors
	for {
		// check if we should shutdown
		select {
		case <-doneChan:
			return
		case <-time.After(1 * time.Minute):
			// just continue after delay if not
		}

		// update monitors
		monitors.MonitorAll()
	}
}

func signalReady(ready *sync.Cond) {
	ready.L.Lock()
	defer ready.L.Unlock()
	ready.Broadcast()
}
