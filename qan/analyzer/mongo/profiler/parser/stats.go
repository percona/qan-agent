package parser

import (
	"expvar"
)

type stats struct {
	Started        *expvar.String `name:"started"`
	InDocs         *expvar.Int    `name:"docs-in"`
	OkDocs         *expvar.Int    `name:"docs-ok"`
	OutReports     *expvar.Int    `name:"reports-out"`
	IntervalStart  *expvar.String `name:"interval-start"`
	IntervalEnd    *expvar.String `name:"interval-end"`
	ErrFingerprint *expvar.Int    `name:"err-fingerprint"`
	ErrParse       *expvar.Int    `name:"err-parse"`
	ErrGetQuery    *expvar.Int    `name:"err-get-query"`
	SkippedDocs    *expvar.Int    `name:"skipped-docs"`
}
