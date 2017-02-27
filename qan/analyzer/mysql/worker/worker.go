package worker

import (
	pc "github.com/percona/pmm/proto/config"
	"github.com/percona/qan-agent/qan/analyzer/mysql/iter"
	"github.com/percona/qan-agent/qan/analyzer/mysql/report"
)

// A Worker gets queries, aggregates them, and returns a Result. Workers are ran
// by Analyzers. When ran, MySQL is presumed to be configured and ready.
type Worker interface {
	Setup(*iter.Interval) error
	Run() (*report.Result, error)
	Stop() error
	Cleanup() error
	Status() map[string]string
	SetConfig(pc.QAN)
}
