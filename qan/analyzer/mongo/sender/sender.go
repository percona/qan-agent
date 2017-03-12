package sender

import (
	"sync"

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

	// state
	sync.Mutex                 // Lock() to protect internal consistency of the service
	running    bool            // Is this service running?
	doneChan   chan struct{}   // close(doneChan) to notify goroutines that they should shutdown
	wg         *sync.WaitGroup // Wait() for goroutines to stop after being notified they should shutdown
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

	// start a goroutine and Add() it to WaitGroup
	// so we could later Wait() for it to finish
	self.wg = &sync.WaitGroup{}
	self.wg.Add(1)
	go start(self.wg, self.reportChan, self.spool, self.logger, self.doneChan)

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

func start(
	wg *sync.WaitGroup,
	reportChan <-chan qan.Report,
	spool data.Spooler,
	logger *pct.Logger,
	doneChan <-chan struct{},
) {
	// signal WaitGroup when goroutine finished
	defer wg.Done()

	for {

		select {
		case report, ok := <-reportChan:
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

			// sent report
			if err := spool.Write("qan", report); err != nil {
				logger.Warn("Lost report:", err)
			}
		case <-doneChan:
			return
		}
	}

}
