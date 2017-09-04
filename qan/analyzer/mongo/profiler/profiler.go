package profiler

import (
	"fmt"
	"sync"
	"time"

	"github.com/percona/pmgo"
	pc "github.com/percona/pmm/proto/config"
	"github.com/percona/qan-agent/data"
	"github.com/percona/qan-agent/pct"
)

func New(
	dialInfo *pmgo.DialInfo,
	dialer pmgo.Dialer,
	logger *pct.Logger,
	spool data.Spooler,
	config pc.QAN,
) *profiler {
	f := func(
		dialInfo *pmgo.DialInfo,
		dialer pmgo.Dialer,
	) *monitor {
		return NewMonitor(
			dialInfo,
			dialer,
			logger,
			spool,
			config,
		)
	}
	m := NewMonitors(
		dialInfo,
		dialer,
		f,
	)
	return &profiler{
		dialInfo: dialInfo,
		dialer:   dialer,
		logger:   logger,
		spool:    spool,
		config:   config,
		monitors: m,
	}
}

type profiler struct {
	// dependencies
	dialInfo *pmgo.DialInfo
	dialer   pmgo.Dialer
	spool    data.Spooler
	logger   *pct.Logger
	config   pc.QAN

	// monitors
	monitors *monitors

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
	statuses := map[string]string{}
	for dbName, p := range self.monitors.GetAll() {
		for k, v := range p.Status() {
			statuses[fmt.Sprintf("%s-%s", dbName, k)] = v
		}
	}
	return statuses
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

	firstTry := true
	for {
		// update
		monitors.MonitorAll()

		if firstTry {
			// signal we started monitoring
			signalReady(ready)
			firstTry = false
		}

		// check if we should shutdown
		select {
		case <-doneChan:
			return
		case <-time.After(1 * time.Minute):
			// just continue after delay if not
		}
	}
}

func signalReady(ready *sync.Cond) {
	ready.L.Lock()
	defer ready.L.Unlock()
	ready.Broadcast()
}
