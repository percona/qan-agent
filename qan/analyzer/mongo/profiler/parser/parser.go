package parser

import (
	"sync"
	"time"

	"github.com/percona/percona-toolkit/src/go/mongolib/proto"
	mstats "github.com/percona/percona-toolkit/src/go/mongolib/stats"
	"github.com/percona/pmm/proto/qan"
	"github.com/percona/qan-agent/qan/analyzer/mongo/profiler/aggregator"
	"github.com/percona/qan-agent/qan/analyzer/mongo/status"
)

func New(
	docsChan <-chan proto.SystemProfile,
	aggregator *aggregator.Aggregator,
) *Parser {
	return &Parser{
		docsChan:   docsChan,
		aggregator: aggregator,
	}
}

type Parser struct {
	// dependencies
	docsChan   <-chan proto.SystemProfile
	aggregator *aggregator.Aggregator

	// provides
	reportChan chan *qan.Report

	// status
	status *status.Status

	// state
	sync.RWMutex                 // Lock() to protect internal consistency of the service
	running      bool            // Is this service running?
	doneChan     chan struct{}   // close(doneChan) to notify goroutines that they should shutdown
	wg           *sync.WaitGroup // Wait() for goroutines to stop after being notified they should shutdown
}

// Start starts but doesn't wait until it exits
func (self *Parser) Start() (<-chan *qan.Report, error) {
	self.Lock()
	defer self.Unlock()
	if self.running {
		return self.reportChan, nil
	}

	// create new channels over which we will communicate to...
	// ... outside world by sending collected docs
	self.reportChan = make(chan *qan.Report, 100)
	// ... inside goroutine to close it
	self.doneChan = make(chan struct{})

	// set status
	stats := &stats{}
	self.status = status.New(stats)

	// start a goroutine and Add() it to WaitGroup
	// so we could later Wait() for it to finish
	self.wg = &sync.WaitGroup{}
	self.wg.Add(1)
	go start(
		self.wg,
		self.docsChan,
		self.reportChan,
		self.aggregator,
		self.doneChan,
		stats,
	)

	self.running = true
	return self.reportChan, nil
}

// Stop stops running
func (self *Parser) Stop() {
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

	// we can now safely close channels goroutines write to as goroutine is stopped
	close(self.reportChan)
	return
}

func (self *Parser) Running() bool {
	self.RLock()
	defer self.RUnlock()
	return self.running
}

func (self *Parser) Status() map[string]string {
	if !self.Running() {
		return nil
	}

	return self.status.Map()
}

func (self *Parser) Name() string {
	return "parser"
}

func start(
	wg *sync.WaitGroup,
	docsChan <-chan proto.SystemProfile,
	reportChan chan<- *qan.Report,
	aggregator *aggregator.Aggregator,
	doneChan <-chan struct{},
	stats *stats,
) {
	// signal WaitGroup when goroutine finished
	defer wg.Done()

	// update stats
	stats.IntervalStart.Set(aggregator.TimeStart().Format("2006-01-02 15:04:05"))
	stats.IntervalEnd.Set(aggregator.TimeEnd().Format("2006-01-02 15:04:05"))
	stats.Started.Set(time.Now().UTC().Format("2006-01-02 15:04:05"))
	for {
		// check if we should shutdown
		select {
		case <-doneChan:
			return
		default:
			// just continue if not
		}

		select {
		case doc, ok := <-docsChan:
			// if channel got closed we should exit as there is nothing we can listen to
			if !ok {
				return
			}

			// we got new doc, increase stats
			stats.InDocs.Add(1)

			// aggregate the doc
			report, err := aggregator.Add(doc)
			switch err.(type) {
			case nil:
				stats.OkDocs.Add(1)
			case *mstats.StatsFingerprintError:
				stats.ErrFingerprint.Add(1)
			default:
				stats.ErrParse.Add(1)
			}

			// check if we have new report
			if report != nil {
				// sent report over reportChan.
				select {
				case reportChan <- report:
					stats.OutReports.Add(1)
				// or exit if we can't push over the channel and we should shutdown
				// note that if we can push over the channel then exiting is not guaranteed
				// that's why we have separate `select <-doneChan`
				case <-doneChan:
					return
				}
				// update stats
				stats.IntervalStart.Set(aggregator.TimeStart().Format("2006-01-02 15:04:05"))
				stats.IntervalEnd.Set(aggregator.TimeEnd().Format("2006-01-02 15:04:05"))
			}

		// doneChan needs to be repeated in this select as docsChan can block
		// doneChan needs to be also in separate select statement
		// as docsChan could be always picked since select picks channels pseudo randomly
		case <-doneChan:
			return
		}
	}

}
