package sender

import (
	"fmt"
	"sync"
	"time"

	"github.com/percona/pmm/proto/qan"
	"github.com/percona/qan-agent/data"
	"github.com/percona/qan-agent/pct"
)

func New(reportChan <-chan qan.Report, spool data.Spooler, logger *pct.Logger) *Sender {
	return &Sender{
		reportChan: reportChan,
		spool:      spool,
		logger:     logger,
	}
}

type Sender struct {
	// dependencies
	reportChan <-chan qan.Report
	spool      data.Spooler
	logger     *pct.Logger

	// status
	pingChan chan struct{} //  ping goroutine for status
	pongChan chan status   // receive status from goroutine
	status   map[string]string

	// state
	sync.RWMutex                 // Lock() to protect internal consistency of the service
	running      bool            // Is this service running?
	doneChan     chan struct{}   // close(doneChan) to notify goroutines that they should shutdown
	wg           *sync.WaitGroup // Wait() for goroutines to stop after being notified they should shutdown
}

// Start starts but doesn't wait until it exits
func (self *Sender) Start() error {
	self.Lock()
	defer self.Unlock()
	if self.running {
		return nil
	}

	// create new channels over which we will communicate to...
	// ... inside goroutine to close it
	self.doneChan = make(chan struct{})

	// set status
	self.pingChan = make(chan struct{})
	self.pongChan = make(chan status)
	self.status = map[string]string{}

	// start a goroutine and Add() it to WaitGroup
	// so we could later Wait() for it to finish
	self.wg = &sync.WaitGroup{}
	self.wg.Add(1)
	go start(
		self.wg,
		self.reportChan,
		self.spool,
		self.logger,
		self.pingChan,
		self.pongChan,
		self.doneChan,
	)

	self.running = true
	return nil
}

// Stop stops running
func (self *Sender) Stop() {
	self.Lock()
	defer self.Unlock()
	if !self.running {
		return
	}
	self.running = false

	// notify goroutine to close
	close(self.doneChan)

	// wait for goroutines to exit
	self.wg.Wait()
	return
}

func (self *Sender) Running() bool {
	self.RLock()
	defer self.RUnlock()
	return self.running
}

func (self *Sender) Status() map[string]string {
	if !self.Running() {
		return nil
	}

	go self.sendPing()
	status := self.recvPong()

	self.Lock()
	defer self.Unlock()
	for k, v := range status {
		self.status[k] = v
	}

	return self.status
}

func (self *Sender) Name() string {
	return "sender"
}

func (self *Sender) sendPing() {
	select {
	case self.pingChan <- struct{}{}:
	case <-time.After(1 * time.Second):
		// timeout carry on
	}
}

func (self *Sender) recvPong() map[string]string {
	select {
	case s := <-self.pongChan:
		status := map[string]string{}
		status["out"] = fmt.Sprintf("%d", s.In)
		status["in"] = fmt.Sprintf("%d", s.Out)
		if s.Errors > 0 {
			status["errors"] = fmt.Sprintf("%d", s.Errors)
		}
		return status
	case <-time.After(2 * time.Second):
		// timeout carry on
	}
	return nil
}

func start(
	wg *sync.WaitGroup,
	reportChan <-chan qan.Report,
	spool data.Spooler,
	logger *pct.Logger,
	pingChan <-chan struct{},
	pongChan chan<- status,
	doneChan <-chan struct{},
) {
	// signal WaitGroup when goroutine finished
	defer wg.Done()

	status := status{}
	for {

		select {
		case report, ok := <-reportChan:
			status.In += 1
			// if channel got closed we should exit as there is nothing we can listen to
			if !ok {
				return
			}

			// check if we should shutdown
			select {
			case <-doneChan:
				return
			default:
				// just continue if not
			}

			// check if we got ping
			select {
			case <-pingChan:
				go pong(status, pongChan)
			default:
				// just continue if not
			}

			// sent report
			if err := spool.Write("qan", report); err != nil {
				status.Errors += 1
				logger.Warn("Lost report:", err)
				continue
			}
			status.Out += 1
		case <-doneChan:
			return
		case <-pingChan:
			go pong(status, pongChan)
		}
	}

}

func pong(status status, pongChan chan<- status) {
	select {
	case pongChan <- status:
	case <-time.After(1 * time.Second):
		// timeout carry on
	}
}

type status struct {
	In     int
	Out    int
	Errors int
}
