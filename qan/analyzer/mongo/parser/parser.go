package parser

import (
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

	// state
	sync.Mutex                 // Lock() to protect internal consistency of the service
	running    bool            // Is this service running?
	doneChan   chan struct{}   // close(doneChan) to notify goroutines that they should shutdown
	wg         *sync.WaitGroup // Wait() for goroutines to stop after being notified they should shutdown
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

	// start a goroutine and Add() it to WaitGroup
	// so we could later Wait() for it to finish
	self.wg = &sync.WaitGroup{}
	self.wg.Add(1)
	go start(self.wg, self.docsChan, self.reportChan, self.config, self.doneChan)

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

func start(
	wg *sync.WaitGroup,
	docsChan <-chan pm.SystemProfile,
	reportChan chan<- qan.Report,
	config pc.QAN,
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
	for {

		select {
		case doc, ok := <-docsChan:
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

			ts := doc.Ts.UTC()
			if ts.Before(timeStart) {
				continue
			}

			// time to prepare data to sent
			if ts.After(timeEnd) {
				global := event.NewClass("", "", false)
				queries := mqd.ToStatSlice(stats)
				queryStats := mqd.CalcQueryStats(queries, int64(interval))
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

				result := &report.Result{
					Global: global,
					Class:  classes,
				}

				// translate the results into a report and sent over reportChan.
				select {
				case reportChan <- *report.MakeReport(config, timeStart, timeEnd, nil, result):
				// or exit if we can't push over the channel and we should shutdown
				// note that if we can push over the channel then exiting is not guaranteed
				// that's why we have separate `select <-doneChan`
				case <-doneChan:
				}

				// reset stats
				stats = map[mqd.GroupKey]*mqd.Stat{}
				// update time intervals
				timeStart = ts.Truncate(d)
				timeEnd = timeStart.Add(d)
			}

			mqd.ProcessDoc(&doc, filters, stats)
		case <-doneChan:
			return
		}
	}

}
