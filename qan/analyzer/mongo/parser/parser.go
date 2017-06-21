package parser

import (
	"sync"
	"time"

	"github.com/percona/go-mysql/event"
	"github.com/percona/percona-toolkit/src/go/mongolib/fingerprinter"
	"github.com/percona/percona-toolkit/src/go/mongolib/proto"
	mstats "github.com/percona/percona-toolkit/src/go/mongolib/stats"
	pc "github.com/percona/pmm/proto/config"
	"github.com/percona/pmm/proto/qan"
	"github.com/percona/qan-agent/qan/analyzer/mongo/status"
	"github.com/percona/qan-agent/qan/analyzer/report"
)

const (
	DefaultInterval       = 60 // in seconds
	DefaultExampleQueries = true
)

func New(docsChan <-chan proto.SystemProfile, config pc.QAN) *Parser {
	return &Parser{
		docsChan: docsChan,
		config:   config,
	}
}

type Parser struct {
	// dependencies
	docsChan <-chan proto.SystemProfile
	config   pc.QAN

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
	self.reportChan = make(chan *qan.Report)
	// ... inside goroutine to close it
	self.doneChan = make(chan struct{})

	// verify config
	if self.config.Interval == 0 {
		self.config.Interval = DefaultInterval
		self.config.ExampleQueries = DefaultExampleQueries
	}

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
		self.config,
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
	config pc.QAN,
	doneChan <-chan struct{},
	stats *stats,
) {
	// signal WaitGroup when goroutine finished
	defer wg.Done()

	fp := fingerprinter.NewFingerprinter(fingerprinter.DEFAULT_KEY_FILTERS)
	s := mstats.New(fp)

	d := time.Duration(config.Interval) * time.Second

	// truncate to the interval e.g 12:15:35 with 1 minute interval is gonna be 12:15:00
	timeStart := time.Now().UTC().Truncate(d)
	// skip first interval as it is partial
	timeStart = timeStart.Add(d)
	// create ending time by adding interval
	timeEnd := timeStart.Add(d)

	// update stats
	stats.IntervalStart.Set(timeStart.Format("2006-01-02 15:04:05"))
	stats.IntervalEnd.Set(timeEnd.Format("2006-01-02 15:04:05"))
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

			ts := doc.Ts.UTC()
			if ts.Before(timeStart) {
				continue
			}

			// time to prepare data to sent
			if ts.After(timeEnd) {
				// create result
				result := createResult(s, int64(config.Interval), config.ExampleQueries)

				// translate result into report
				qanReport := report.MakeReport(config, timeStart, timeEnd, nil, result)

				// sent report over reportChan.
				select {
				case reportChan <- qanReport:
					stats.OutReports.Add(1)
				// or exit if we can't push over the channel and we should shutdown
				// note that if we can push over the channel then exiting is not guaranteed
				// that's why we have separate `select <-doneChan`
				case <-doneChan:
					return
				}

				// reset stats
				s.Reset()
				// update time intervals
				timeStart = ts.Truncate(d)
				timeEnd = timeStart.Add(d)
				// update stats
				stats.IntervalStart.Set(timeStart.Format("2006-01-02 15:04:05"))
				stats.IntervalEnd.Set(timeEnd.Format("2006-01-02 15:04:05"))
			}

			stats.InDocs.Add(1)
			if len(doc.Query) == 0 {
				stats.SkippedDocs.Add(1)
				continue
			}

			err := s.Add(doc)
			switch err.(type) {
			case nil:
				stats.OkDocs.Add(1)
			case *mstats.StatsFingerprintError:
				stats.ErrFingerprint.Add(1)
			case *mstats.StatsGetQueryFieldError:
				stats.ErrGetQuery.Add(1)
			default:
				stats.ErrParse.Add(1)
			}
		// doneChan needs to be repeated in this select as docsChan can block
		// doneChan needs to be also in separate select statement
		// as docsChan could be always picked since select picks channels pseudo randomly
		case <-doneChan:
			return
		}
	}

}

func createResult(s *mstats.Stats, interval int64, exampleQueries bool) *report.Result {
	queries := s.Queries()
	global := event.NewClass("", "", false)
	queryStats := queries.CalcQueriesStats(interval)
	classes := []*event.Class{}
	for _, queryInfo := range queryStats {
		class := event.NewClass(queryInfo.ID, queryInfo.Fingerprint, exampleQueries)
		if exampleQueries {
			class.Example = &event.Example{
				QueryTime: queryInfo.QueryTime.Total,
				Db:        queryInfo.Namespace,
				Query:     queryInfo.Query,
			}
		}

		metrics := event.NewMetrics()

		metrics.TimeMetrics["Query_time"] = newEventTimeStats(queryInfo.QueryTime)

		// @todo we map below metrics to MySQL equivalents according to PMM-830
		metrics.NumberMetrics["Bytes_sent"] = newEventNumberStats(queryInfo.ResponseLength)
		metrics.NumberMetrics["Rows_sent"] = newEventNumberStats(queryInfo.Returned)
		metrics.NumberMetrics["Rows_examined"] = newEventNumberStats(queryInfo.Scanned)

		class.Metrics = metrics
		class.TotalQueries = uint(queryInfo.Count)
		classes = append(classes, class)

		// Add the class to the global metrics.
		global.AddClass(class)
	}

	return &report.Result{
		Global: global,
		Class:  classes,
	}

}

func newEventNumberStats(s mstats.Statistics) *event.NumberStats {
	return &event.NumberStats{
		Sum: uint64(s.Total),
		Min: uint64(s.Min),
		Avg: uint64(s.Avg),
		Med: uint64(s.Median),
		P95: uint64(s.Pct95),
		Max: uint64(s.Max),
	}
}

func newEventTimeStats(s mstats.Statistics) *event.TimeStats {
	return &event.TimeStats{
		Sum: s.Total,
		Min: s.Min,
		Avg: s.Avg,
		Med: s.Median,
		P95: s.Pct95,
		Max: s.Max,
	}
}
