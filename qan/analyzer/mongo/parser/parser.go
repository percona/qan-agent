package parser

import (
	"fmt"
	"sync"
	"time"

	"github.com/percona/go-mysql/event"
	pm "github.com/percona/percona-toolkit/src/go/mongolib/proto"
	pc "github.com/percona/pmm/proto/config"
	"github.com/percona/pmm/proto/qan"
	"github.com/percona/qan-agent/qan/analyzer/mongo/mqd"
	"github.com/percona/qan-agent/qan/analyzer/report"
)

const (
	defaultInterval = 60 // in seconds
)

func New(docsChan <-chan pm.SystemProfile, config pc.QAN) *Parser {
	return &Parser{
		docsChan: docsChan,
		config:   config,
	}
}

type Parser struct {
	// dependencies
	docsChan <-chan pm.SystemProfile
	config   pc.QAN

	// provides
	reportChan chan qan.Report

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
func (self *Parser) Start() (<-chan qan.Report, error) {
	self.Lock()
	defer self.Unlock()
	if self.running {
		return self.reportChan, nil
	}

	// create new channels over which we will communicate to...
	// ... outside world by sending collected docs
	self.reportChan = make(chan qan.Report)
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
		self.docsChan,
		self.reportChan,
		self.config,
		self.pingChan,
		self.pongChan,
		self.doneChan,
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

	go self.sendPing()
	status := self.recvPong()

	self.Lock()
	defer self.Unlock()
	for k, v := range status {
		self.status[k] = v
	}

	return self.status
}

func (self *Parser) Name() string {
	return "parser"
}

func (self *Parser) sendPing() {
	select {
	case self.pingChan <- struct{}{}:
	case <-time.After(1 * time.Second):
		// timeout carry on
	}
}

func (self *Parser) recvPong() map[string]string {
	select {
	case s := <-self.pongChan:
		status := map[string]string{}
		status["reports-out"] = fmt.Sprintf("%d", s.OutReports)
		status["docs-in"] = fmt.Sprintf("%d", s.InDocs)
		status["interval-start"] = s.IntervalStart
		status["interval-end"] = s.IntervalEnd
		return status
	case <-time.After(1 * time.Second):
		// timeout carry on
	}

	return nil
}

func start(
	wg *sync.WaitGroup,
	docsChan <-chan pm.SystemProfile,
	reportChan chan<- qan.Report,
	config pc.QAN,
	pingChan <-chan struct{},
	pongChan chan<- status,
	doneChan <-chan struct{},
) {
	// signal WaitGroup when goroutine finished
	defer wg.Done()

	stats := map[mqd.GroupKey]*mqd.Stat{}
	filters := []mqd.DocsFilter{}

	interval := config.Interval
	if interval == 0 {
		interval = defaultInterval
	}
	d := time.Duration(interval) * time.Second

	// truncate to the interval e.g 12:15:35 with 1 minute interval is gonna be 12:15:00
	timeStart := time.Now().UTC().Truncate(d)
	// skip first interval as it is partial
	timeStart = timeStart.Add(d)
	// create ending time by adding interval
	timeEnd := timeStart.Add(d)

	status := status{}
	for {
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
			status.IntervalStart = timeStart.Format("2006-01-02 15:04:05")
			status.IntervalEnd = timeEnd.Format("2006-01-02 15:04:05")
			go pong(status, pongChan)
		default:
			// just continue if not
		}

		select {
		case doc, ok := <-docsChan:
			// if channel got closed we should exit as there is nothing we can listen to
			if !ok {
				return
			}

			ts := doc.Ts.UTC()
			if ts.Before(timeStart) {
				continue
			}

			// time to prepare data to sent
			if ts.After(timeEnd) {
				// create result
				result := createResult(stats, int64(interval))

				// translate result into report
				report := report.MakeReport(config, timeStart, timeEnd, nil, result)

				// sent report over reportChan.
				select {
				case reportChan <- *report:
					status.OutReports += 1
				// or exit if we can't push over the channel and we should shutdown
				// note that if we can push over the channel then exiting is not guaranteed
				// that's why we have separate `select <-doneChan`
				case <-doneChan:
					return
				}

				// reset stats
				stats = map[mqd.GroupKey]*mqd.Stat{}
				// update time intervals
				timeStart = ts.Truncate(d)
				timeEnd = timeStart.Add(d)
			}

			mqd.ProcessDoc(&doc, filters, stats)
			status.InDocs += 1
		// doneChan and pingChan needs to be repeated in this select as docsChan can block
		// doneChan and pingChan needs to be also in separate select statements as docsChan
		// could be always picked since select picks channels pseudo randomly
		case <-doneChan:
			return
		case <-pingChan:
			status.IntervalStart = timeStart.Format("2006-01-02 15:04:05")
			status.IntervalEnd = timeEnd.Format("2006-01-02 15:04:05")
			go pong(status, pongChan)
		}
	}

}

func createResult(stats map[mqd.GroupKey]*mqd.Stat, interval int64) *report.Result {
	queries := mqd.ToStatSlice(stats)
	global := event.NewClass("", "", false)
	queryStats := mqd.CalcQueryStats(queries, interval)
	classes := []*event.Class{}
	for _, queryInfo := range queryStats {
		class := event.NewClass(queryInfo.ID, queryInfo.Fingerprint, true)

		metrics := event.NewMetrics()

		// Time metrics are in picoseconds, so multiply by 10^-12 to convert to seconds.
		metrics.TimeMetrics["Query_time"] = &event.TimeStats{
			Sum: queryInfo.QueryTime.Total,
			Min: queryInfo.QueryTime.Min,
			Avg: queryInfo.QueryTime.Avg,
			Max: queryInfo.QueryTime.Max,
		}

		class.Metrics = metrics
		classes = append(classes, class)

		// Add the class to the global metrics.
		global.AddClass(class)
	}

	return &report.Result{
		Global: global,
		Class:  classes,
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
	InDocs        int
	OutReports    int
	IntervalStart string
	IntervalEnd   string
}
