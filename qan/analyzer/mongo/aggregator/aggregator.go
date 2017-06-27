package aggregator

import (
	"sync"
	"time"

	"github.com/percona/go-mysql/event"
	"github.com/percona/percona-toolkit/src/go/mongolib/fingerprinter"
	"github.com/percona/percona-toolkit/src/go/mongolib/proto"
	"github.com/percona/percona-toolkit/src/go/mongolib/stats"
	pc "github.com/percona/pmm/proto/config"
	"github.com/percona/pmm/proto/qan"
	"github.com/percona/qan-agent/qan/analyzer/report"
)

func New(timeStart time.Time, config pc.QAN) *Aggregator {
	fp := fingerprinter.NewFingerprinter(fingerprinter.DEFAULT_KEY_FILTERS)
	s := stats.New(fp)

	d := time.Duration(config.Interval) * time.Second

	// truncate to the interval e.g 12:15:35 with 1 minute interval it will be 12:15:00
	timeStart = timeStart.UTC().Truncate(d)
	// create ending time by adding interval
	timeEnd := timeStart.Add(d)

	return &Aggregator{
		// dependencies
		timeStart: timeStart,
		config:    config,

		// internal
		timeEnd: timeEnd,
		d:       d,
		stats:   s,
	}
}

type Aggregator struct {
	// dependencies
	config pc.QAN

	// interval
	timeStart time.Time
	timeEnd   time.Time
	d         time.Duration

	stats *stats.Stats

	// state
	sync.RWMutex // Lock() to protect internal consistency of the service
}

func (self *Aggregator) Add(doc proto.SystemProfile) (*qan.Report, error) {
	ts := doc.Ts.UTC()

	// skip old metrics
	if ts.Before(self.timeStart) {
		return nil, nil
	}

	return self.NextReport(ts), self.stats.Add(doc)
}

func (self *Aggregator) NextReport(ts time.Time) *qan.Report {
	// if time is greater than interval then we are done with this interval
	if !ts.Before(self.timeEnd) {
		// reset stats
		defer self.resetStats(ts)

		// let's check if we have anything to send
		if len(self.stats.Queries()) > 0 {
			// create result
			result := self.createResult()

			// translate result into report
			return report.MakeReport(self.config, self.timeStart, self.timeEnd, nil, result)
		}
	}

	// we are not done with the interval so no report yet
	return nil
}

func (self *Aggregator) TimeStart() time.Time {
	self.RLock()
	defer self.RUnlock()
	return self.timeStart
}

func (self *Aggregator) TimeEnd() time.Time {
	self.RLock()
	defer self.RUnlock()
	return self.timeEnd
}

func (self *Aggregator) resetStats(ts time.Time) {
	// reset stats
	self.stats.Reset()

	// update time intervals
	self.timeStart = ts.Truncate(self.d)
	self.timeEnd = self.timeStart.Add(self.d)
}

func (self *Aggregator) createResult() *report.Result {
	queries := self.stats.Queries()
	global := event.NewClass("", "", false)
	queryStats := queries.CalcQueriesStats(int64(self.config.Interval))
	classes := []*event.Class{}
	for _, queryInfo := range queryStats {
		class := event.NewClass(queryInfo.ID, queryInfo.Fingerprint, self.config.ExampleQueries)
		if self.config.ExampleQueries {
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
		class.UniqueQueries = 1
		classes = append(classes, class)

		// Add the class to the global metrics.
		global.AddClass(class)
	}

	return &report.Result{
		Global: global,
		Class:  classes,
	}

}

func newEventNumberStats(s stats.Statistics) *event.NumberStats {
	return &event.NumberStats{
		Sum: uint64(s.Total),
		Min: uint64(s.Min),
		Avg: uint64(s.Avg),
		Med: uint64(s.Median),
		P95: uint64(s.Pct95),
		Max: uint64(s.Max),
	}
}

func newEventTimeStats(s stats.Statistics) *event.TimeStats {
	return &event.TimeStats{
		Sum: s.Total,
		Min: s.Min,
		Avg: s.Avg,
		Med: s.Median,
		P95: s.Pct95,
		Max: s.Max,
	}
}
