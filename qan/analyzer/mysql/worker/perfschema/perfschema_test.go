/*
   Copyright (c) 2016, Percona LLC and/or its affiliates. All rights reserved.

   This program is free software: you can redistribute it and/or modify
   it under the terms of the GNU Affero General Public License as published by
   the Free Software Foundation, either version 3 of the License, or
   (at your option) any later version.

   This program is distributed in the hope that it will be useful,
   but WITHOUT ANY WARRANTY; without even the implied warranty of
   MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
   GNU Affero General Public License for more details.

   You should have received a copy of the GNU Affero General Public License
   along with this program.  If not, see <http://www.gnu.org/licenses/>
*/

package perfschema

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/percona/go-mysql/event"
	"github.com/percona/pmm/proto"
	"github.com/percona/qan-agent/mysql"
	"github.com/percona/qan-agent/pct"
	"github.com/percona/qan-agent/qan/analyzer/mysql/iter"
	"github.com/percona/qan-agent/qan/analyzer/report"
	"github.com/percona/qan-agent/test/mock"
	. "github.com/percona/qan-agent/test/rootdir"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var inputDir = RootDir() + "/test/qan/perfschema/"

func TestWorker(t *testing.T) {
	logChan := make(chan proto.LogEntry, 100)
	logger := pct.NewLogger(logChan, "qan-worker")

	tests := []func(t *testing.T, logger *pct.Logger){
		testWorkerWithNullMySQL,
		testWorkerWithRealMySQL,
	}

	for _, f := range tests {
		f := f // capture range variable
		fName := runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
		t.Run(fName, func(t *testing.T) {
			t.Parallel()

			f(t, logger)
		})
	}
}

func testWorkerWithRealMySQL(t *testing.T, logger *pct.Logger) {
	dsn := os.Getenv("PCT_TEST_MYSQL_DSN")
	if dsn == "" {
		t.Fatal("PCT_TEST_MYSQL_DSN is not set")
	}

	tests := []func(t *testing.T, logger *pct.Logger, dsn string){
		testRealWorker,
		testIterClockReset,
		testIterOutOfSeq,
	}

	for _, f := range tests {
		f := f // capture range variable
		fName := runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
		t.Run(fName, func(t *testing.T) {
			f(t, logger, dsn)
		})
	}
}

func testWorkerWithNullMySQL(t *testing.T, logger *pct.Logger) {
	tests := []func(t *testing.T, logger *pct.Logger, nullmysql *mock.NullMySQL){
		test001,
		test002,
		test003,
		test005,
		test004EmptyDigest,
	}

	for _, f := range tests {
		f := f // capture range variable
		fName := runtime.FuncForPC(reflect.ValueOf(f).Pointer()).Name()
		t.Run(fName, func(t *testing.T) {
			t.Parallel()

			nullmysql := mock.NewNullMySQL()
			f(t, logger, nullmysql)
		})
	}
}

// --------------------------------------------------------------------------

func loadData(dir string) ([][]*DigestRow, error) {
	files, err := filepath.Glob(filepath.Join(inputDir, dir, "/iter*.json"))
	if err != nil {
		return nil, err
	}
	iters := [][]*DigestRow{}
	for _, file := range files {
		bytes, err := ioutil.ReadFile(file)
		if err != nil {
			return nil, err
		}
		rows := []*DigestRow{}
		if err := json.Unmarshal(bytes, &rows); err != nil {
			return nil, err
		}
		iters = append(iters, rows)
	}
	return iters, nil
}

func loadResult(file string, got *report.Result) (*report.Result, error) {
	file = filepath.Join(inputDir, file)
	updateTestData := os.Getenv("UPDATE_TEST_DATA")
	if updateTestData != "" {
		data, _ := json.MarshalIndent(got, "", "  ")
		ioutil.WriteFile(file, data, 0666)

	}
	bytes, err := ioutil.ReadFile(file)
	if err != nil {
		return nil, err
	}
	res := &report.Result{}
	if err := json.Unmarshal(bytes, &res); err != nil {
		return nil, err
	}
	return res, nil
}

func makeGetRowsFunc(iters [][]*DigestRow) GetDigestRowsFunc {
	return func(c chan<- *DigestRow, lastFetchSeconds float64, done chan<- error) error {
		if len(iters) == 0 {
			return fmt.Errorf("No more iters")
		}
		rows := iters[0]
		iters = iters[1:]
		go func() {
			defer func() {
				done <- nil
			}()
			for _, row := range rows {
				c <- row
			}
		}()
		return nil
	}
}

type ByClassId []*event.Class

func (a ByClassId) Len() int      { return len(a) }
func (a ByClassId) Swap(i, j int) { a[i], a[j] = a[j], a[i] }
func (a ByClassId) Less(i, j int) bool {
	return a[i].Id < a[j].Id
}

func normalizeResult(res *report.Result) {
	sort.Sort(ByClassId(res.Class))
	// Perf Schema never has example queries, so remove the empty
	// event.Example struct the json creates.
	for n := range res.Class {
		res.Class[n].Example = nil
	}
}

// --------------------------------------------------------------------------

func test001(t *testing.T, logger *pct.Logger, nullmysql *mock.NullMySQL) {
	// This is the simplest input possible: 1 query in iter 1 and 2. The result
	// is just the increase in its values.

	rows, err := loadData("001")
	require.NoError(t, err)
	getRows := makeGetRowsFunc(rows)
	w := NewWorker(logger, nullmysql, getRows)

	// First run doesn't produce a result because 2 snapshots are required.
	i := &iter.Interval{
		Number:    1,
		StartTime: time.Now().UTC(),
	}
	err = w.Setup(i)
	require.NoError(t, err)

	res, err := w.Run()
	require.NoError(t, err)
	assert.Nil(t, res)

	err = w.Cleanup()
	require.NoError(t, err)

	// The second run produces a result: the diff of 2nd - 1st.
	i = &iter.Interval{
		Number:    2,
		StartTime: time.Now().UTC(),
	}
	err = w.Setup(i)
	require.NoError(t, err)

	res, err = w.Run()
	require.NoError(t, err)
	normalizeResult(res)
	expect, err := loadResult("001/res01.json", res)
	require.NoError(t, err)
	assert.Equal(t, expect, res)

	err = w.Cleanup()
	require.NoError(t, err)

	// Quick side test that Status() works and reports last stats.
	status := w.Status()
	assert.Equal(t, true, strings.HasPrefix(status["qan-worker-last"], "rows: 1"))
}

func test002(t *testing.T, logger *pct.Logger, nullmysql *mock.NullMySQL) {
	// This is the 2nd most simplest input after 001: two queries, same digest,
	// but different schemas. The result is the aggregate of their value diffs
	// from iter 1 to 2.

	rows, err := loadData("002")
	require.NoError(t, err)
	getRows := makeGetRowsFunc(rows)
	w := NewWorker(logger, nullmysql, getRows)

	// First run doesn't produce a result because 2 snapshots are required.
	i := &iter.Interval{
		Number:    1,
		StartTime: time.Now().UTC(),
	}
	err = w.Setup(i)
	require.NoError(t, err)

	res, err := w.Run()
	require.NoError(t, err)
	assert.Nil(t, res)

	err = w.Cleanup()
	require.NoError(t, err)

	// The second run produces a result: the diff of 2nd - 1st.
	i = &iter.Interval{
		Number:    2,
		StartTime: time.Now().UTC(),
	}
	err = w.Setup(i)
	require.NoError(t, err)

	res, err = w.Run()
	require.NoError(t, err)
	normalizeResult(res)
	expect, err := loadResult("002/res01.json", res)
	require.NoError(t, err)
	assert.Equal(t, expect, res)

	err = w.Cleanup()
	require.NoError(t, err)
}

func test003(t *testing.T, logger *pct.Logger, nullmysql *mock.NullMySQL) {
	// This test has 4 iters:
	//   1: 2 queries
	//   2: 2 queries (res02)
	//   3: 4 queries (res03)
	//   4: 4 queries but 4th has same COUNT_STAR (res04)
	rows, err := loadData("003")
	require.NoError(t, err)
	getRows := makeGetRowsFunc(rows)
	w := NewWorker(logger, nullmysql, getRows)

	// First interval doesn't produce a result because 2 snapshots are required.
	i := &iter.Interval{
		Number:    1,
		StartTime: time.Now().UTC(),
	}
	err = w.Setup(i)
	require.NoError(t, err)

	res, err := w.Run()
	require.NoError(t, err)
	assert.Nil(t, res)

	err = w.Cleanup()
	require.NoError(t, err)

	// Second interval produces a result: the diff of 2nd - 1st.
	i = &iter.Interval{
		Number:    2,
		StartTime: time.Now().UTC(),
	}
	err = w.Setup(i)
	require.NoError(t, err)

	res, err = w.Run()
	require.NoError(t, err)
	normalizeResult(res)
	expect, err := loadResult("003/res02.json", res)
	require.NoError(t, err)
	assert.Equal(t, expect, res)

	err = w.Cleanup()
	require.NoError(t, err)

	// Third interval...
	i = &iter.Interval{
		Number:    3,
		StartTime: time.Now().UTC(),
	}
	err = w.Setup(i)
	require.NoError(t, err)

	res, err = w.Run()
	require.NoError(t, err)
	normalizeResult(res)
	expect, err = loadResult("003/res03.json", res)
	require.NoError(t, err)
	assert.Equal(t, expect, res)

	err = w.Cleanup()
	require.NoError(t, err)

	// Fourth interval...
	i = &iter.Interval{
		Number:    4,
		StartTime: time.Now().UTC(),
	}
	err = w.Setup(i)
	require.NoError(t, err)

	res, err = w.Run()
	require.NoError(t, err)
	normalizeResult(res)
	expect, err = loadResult("003/res04.json", res)
	require.NoError(t, err)
	assert.Equal(t, expect, res)

	err = w.Cleanup()
	require.NoError(t, err)
}

func test004EmptyDigest(t *testing.T, logger *pct.Logger, nullmysql *mock.NullMySQL) {
	// This is the simplest input possible: 1 query in iter 1 and 2. The result
	// is just the increase in its values.

	rows, err := loadData("004")
	require.NoError(t, err)
	getRows := makeGetRowsFunc(rows)
	w := NewWorker(logger, nullmysql, getRows)

	// First run doesn't produce a result because 2 snapshots are required.
	i := &iter.Interval{
		Number:    1,
		StartTime: time.Now().UTC(),
	}
	err = w.Setup(i)
	require.NoError(t, err)

	res, err := w.Run()
	require.NoError(t, err)
	assert.Nil(t, res)

	err = w.Cleanup()
	require.NoError(t, err)

}

// Test005 for `PMM-1081: Performance schema doesn't work for queries that don't show every interval`
func test005(t *testing.T, logger *pct.Logger, nullmysql *mock.NullMySQL) {
	// This test has 3 iters:
	//   1: 2 queries
	//   2: 1 query (res01)
	//   3: 2 queries (res02)
	rows, err := loadData("005")
	require.NoError(t, err)
	getRows := makeGetRowsFunc(rows)
	w := NewWorker(logger, nullmysql, getRows)

	// First interval doesn't produce a result because 2 snapshots are required.
	i := &iter.Interval{
		Number:    1,
		StartTime: time.Now().UTC(),
	}
	err = w.Setup(i)
	require.NoError(t, err)

	res, err := w.Run()
	require.NoError(t, err)
	assert.Nil(t, res)

	err = w.Cleanup()
	require.NoError(t, err)

	// Second interval produces a result: the diff of 2nd - 1st.
	i = &iter.Interval{
		Number:    2,
		StartTime: time.Now().UTC(),
	}
	err = w.Setup(i)
	require.NoError(t, err)

	res, err = w.Run()
	require.NoError(t, err)
	normalizeResult(res)
	expect, err := loadResult("005/res01.json", res)
	require.NoError(t, err)
	assert.Equal(t, expect, res)

	err = w.Cleanup()
	require.NoError(t, err)

	// Third interval...
	i = &iter.Interval{
		Number:    3,
		StartTime: time.Now().UTC(),
	}
	err = w.Setup(i)
	require.NoError(t, err)

	res, err = w.Run()
	require.NoError(t, err)
	normalizeResult(res)
	expect, err = loadResult("005/res02.json", res)
	require.NoError(t, err)
	assert.Equal(t, expect, res)

	err = w.Cleanup()
	require.NoError(t, err)
}

func testRealWorker(t *testing.T, logger *pct.Logger, dsn string) {
	mysqlConn := mysql.NewConnection(dsn)
	err := mysqlConn.Connect()
	require.NoError(t, err)
	defer mysqlConn.Close()

	requiredVersion := "5.6.5"
	ok, err := mysqlConn.AtLeastVersion(requiredVersion)
	require.NoError(t, err)
	if !ok {
		t.Skip(
			"Monitoring Performance Schema for this version of MySQL is unsupported.",
			fmt.Sprintf("Required table `events_statements_summary_by_digest` was introduced in MySQL %s.", requiredVersion),
			"https://dev.mysql.com/doc/relnotes/mysql/5.6/en/news-5-6-5.html",
		)
	}

	mysqlWorkerConn := mysql.NewConnection(dsn)
	f := NewRealWorkerFactory(logger.LogChan())
	w := f.Make("qan-worker", mysqlWorkerConn)

	start := []mysql.Query{
		{Verify: "performance_schema", Expect: "1"},
		{Set: "UPDATE performance_schema.setup_consumers SET ENABLED = 'YES' WHERE NAME = 'statements_digest'"},
		{Set: "UPDATE performance_schema.setup_instruments SET ENABLED = 'YES', TIMED = 'YES' WHERE NAME LIKE 'statement/sql/%'"},
		{Set: "TRUNCATE performance_schema.events_statements_summary_by_digest"},
	}
	if err := mysqlConn.Set(start); err != nil {
		t.Fatal(err)
	}
	stop := []mysql.Query{
		{Set: "UPDATE performance_schema.setup_consumers SET ENABLED = 'NO' WHERE NAME = 'statements_digest'"},
		{Set: "UPDATE performance_schema.setup_instruments SET ENABLED = 'NO', TIMED = 'NO' WHERE NAME LIKE 'statement/sql/%'"},
	}
	defer func() {
		if err := mysqlConn.Set(stop); err != nil {
			t.Fatal(err)
		}
	}()

	// SCHEMA_NAME: NULL
	//      DIGEST: fbe070dfb47e4a2401c5be6b5201254e
	// DIGEST_TEXT: SELECT ? FROM DUAL
	_, err = mysqlConn.DB().Exec("SELECT 'teapot' FROM DUAL")

	// First interval.
	err = w.Setup(&iter.Interval{Number: 1, StartTime: time.Now().UTC()})
	require.NoError(t, err)

	res, err := w.Run()
	require.NoError(t, err)
	assert.Nil(t, res)

	err = w.Cleanup()
	require.NoError(t, err)

	// Some query activity between intervals.
	_, err = mysqlConn.DB().Exec("SELECT 'teapot' FROM DUAL")
	time.Sleep(1 * time.Second)

	// Second interval and a result.
	err = w.Setup(&iter.Interval{Number: 2, StartTime: time.Now().UTC()})
	require.NoError(t, err)

	res, err = w.Run()
	require.NoError(t, err)
	require.NotNil(t, res)
	if len(res.Class) == 0 {
		t.Fatal("Expected len(res.Class) > 0")
	}
	var class *event.Class
	for _, c := range res.Class {
		if c.Fingerprint == "SELECT ? FROM DUAL " {
			class = c
			break
		}
	}
	require.NotNil(t, class)
	// Digests on different versions or distros of MySQL don't match
	//assert.Equal(t, "01C5BE6B5201254E", class.Id)
	//assert.Equal(t, "SELECT ? FROM DUAL ", class.Fingerprint)
	queryTime := class.Metrics.TimeMetrics["Query_time"]
	if queryTime.Min == 0 {
		t.Error("Expected Query_time_min > 0")
	}
	if queryTime.Max == 0 {
		t.Error("Expected Query_time_max > 0")
	}
	if queryTime.Avg == 0 {
		t.Error("Expected Query_time_avg > 0")
	}
	if queryTime.Min > queryTime.Max {
		t.Error("Expected Query_time_min >= Query_time_max")
	}
	assert.Equal(t, uint64(0), class.Metrics.NumberMetrics["Rows_affected"].Sum)
	assert.Equal(t, uint64(0), class.Metrics.NumberMetrics["Rows_examined"].Sum)
	assert.Equal(t, uint64(1), class.Metrics.NumberMetrics["Rows_sent"].Sum)

	err = w.Cleanup()
	require.NoError(t, err)
}

func testIterOutOfSeq(t *testing.T, logger *pct.Logger, dsn string) {
	mysqlConn := mysql.NewConnection(dsn)
	err := mysqlConn.Connect()
	require.NoError(t, err)
	defer mysqlConn.Close()

	requiredVersion := "5.6.5"
	ok, err := mysqlConn.AtLeastVersion(requiredVersion)
	require.NoError(t, err)
	if !ok {
		t.Skip(
			"Monitoring Performance Schema for this version of MySQL is unsupported.",
			fmt.Sprintf("Required table `events_statements_summary_by_digest` was introduced in MySQL %s.", requiredVersion),
			"https://dev.mysql.com/doc/relnotes/mysql/5.6/en/news-5-6-5.html",
		)
	}

	mysqlWorkerConn := mysql.NewConnection(dsn)
	f := NewRealWorkerFactory(logger.LogChan())
	w := f.Make("qan-worker", mysqlWorkerConn)

	start := []mysql.Query{
		{Verify: "performance_schema", Expect: "1"},
		{Set: "UPDATE performance_schema.setup_consumers SET ENABLED = 'YES' WHERE NAME = 'statements_digest'"},
		{Set: "UPDATE performance_schema.setup_instruments SET ENABLED = 'YES', TIMED = 'YES' WHERE NAME LIKE 'statement/sql/%'"},
		{Set: "TRUNCATE performance_schema.events_statements_summary_by_digest"},
	}
	if err := mysqlConn.Set(start); err != nil {
		t.Fatal(err)
	}
	stop := []mysql.Query{
		{Set: "UPDATE performance_schema.setup_consumers SET ENABLED = 'NO' WHERE NAME = 'statements_digest'"},
		{Set: "UPDATE performance_schema.setup_instruments SET ENABLED = 'NO', TIMED = 'NO' WHERE NAME LIKE 'statement/sql/%'"},
	}
	defer func() {
		if err := mysqlConn.Set(stop); err != nil {
			t.Fatal(err)
		}
	}()

	// SCHEMA_NAME: NULL
	//      DIGEST: fbe070dfb47e4a2401c5be6b5201254e
	// DIGEST_TEXT: SELECT ? FROM DUAL
	_, err = mysqlConn.DB().Exec("SELECT 'teapot' FROM DUAL")

	// First interval.
	err = w.Setup(&iter.Interval{Number: 1, StartTime: time.Now().UTC()})
	require.NoError(t, err)

	res, err := w.Run()
	require.NoError(t, err)
	assert.Nil(t, res)

	err = w.Cleanup()
	require.NoError(t, err)

	// Some query activity between intervals.
	_, err = mysqlConn.DB().Exec("SELECT 'teapot' FROM DUAL")
	time.Sleep(1 * time.Second)

	// Simulate the ticker being reset which results in it resetting
	// its internal interval number, so instead of 2 here we have 1 again.
	// Second interval and a result.
	err = w.Setup(&iter.Interval{Number: 1, StartTime: time.Now().UTC()})
	require.NoError(t, err)

	res, err = w.Run()
	require.NoError(t, err)
	assert.Nil(t, res) // no result due to out of sequence interval

	err = w.Cleanup()
	require.NoError(t, err)

	// Simulate normal operation resuming, i.e. interval 2.
	err = w.Setup(&iter.Interval{Number: 2, StartTime: time.Now().UTC()})
	require.NoError(t, err)

	// Now there should be a result.
	res, err = w.Run()
	require.NoError(t, err)
	require.NotNil(t, res)
	if len(res.Class) == 0 {
		t.Error("Expected len(res.Class) > 0")
	}
}

func testIterClockReset(t *testing.T, logger *pct.Logger, dsn string) {
	var err error

	mysqlConn := mysql.NewConnection(dsn)
	err = mysqlConn.Connect()
	require.NoError(t, err)
	defer mysqlConn.Close()

	requiredVersion := "5.6.5"
	ok, err := mysqlConn.AtLeastVersion(requiredVersion)
	require.NoError(t, err)
	if !ok {
		t.Skip(
			"Monitoring Performance Schema for this version of MySQL is unsupported.",
			fmt.Sprintf("Required table `events_statements_summary_by_digest` was introduced in MySQL %s.", requiredVersion),
			"https://dev.mysql.com/doc/relnotes/mysql/5.6/en/news-5-6-5.html",
		)
	}

	mysqlWorkerConn := mysql.NewConnection(dsn)
	f := NewRealWorkerFactory(logger.LogChan())
	w := f.Make("qan-worker", mysqlWorkerConn)

	start := []mysql.Query{
		{Verify: "performance_schema", Expect: "1"},
		{Set: "UPDATE performance_schema.setup_consumers SET ENABLED = 'YES' WHERE NAME = 'statements_digest'"},
		{Set: "UPDATE performance_schema.setup_instruments SET ENABLED = 'YES', TIMED = 'YES' WHERE NAME LIKE 'statement/sql/%'"},
		{Set: "TRUNCATE performance_schema.events_statements_summary_by_digest"},
	}
	if err := mysqlConn.Set(start); err != nil {
		t.Fatal(err)
	}
	stop := []mysql.Query{
		{Set: "UPDATE performance_schema.setup_consumers SET ENABLED = 'NO' WHERE NAME = 'statements_digest'"},
		{Set: "UPDATE performance_schema.setup_instruments SET ENABLED = 'NO', TIMED = 'NO' WHERE NAME LIKE 'statement/sql/%'"},
	}
	defer func() {
		if err := mysqlConn.Set(stop); err != nil {
			t.Fatal(err)
		}
	}()

	// Generate some perf schema data.
	_, err = mysqlConn.DB().Exec("SELECT 'teapot' FROM DUAL")

	// First interval.
	now := time.Now().UTC()
	err = w.Setup(&iter.Interval{Number: 1, StartTime: now})
	require.NoError(t, err)

	res, err := w.Run()
	require.NoError(t, err)
	assert.Nil(t, res)

	err = w.Cleanup()
	require.NoError(t, err)

	// Simulate the ticker sending a time that's earlier than the previous
	// tick, which shouldn't happen.
	now = now.Add(-1 * time.Minute)
	err = w.Setup(&iter.Interval{Number: 2, StartTime: now})
	require.NoError(t, err)

	res, err = w.Run()
	require.NoError(t, err)
	assert.Nil(t, res) // no result due to out of sequence interval

	err = w.Cleanup()
	require.NoError(t, err)

	// Simulate normal operation resuming.
	now = now.Add(1 * time.Minute)
	err = w.Setup(&iter.Interval{Number: 3, StartTime: now})
	require.NoError(t, err)

	// Now there should be a result.
	res, err = w.Run()
	require.NoError(t, err)
	require.NotNil(t, res)
	if len(res.Class) == 0 {
		t.Error("Expected len(res.Class) > 0")
	}
}

func TestIter(t *testing.T) {
	t.Parallel()

	logChan := make(chan proto.LogEntry, 100)
	tickChan := make(chan time.Time, 1)
	i := NewIter(pct.NewLogger(logChan, "iter"), tickChan)
	require.NotNil(t, i)

	iterChan := i.IntervalChan()
	require.NotNil(t, iterChan)

	i.Start()
	defer i.Stop()

	t1, _ := time.Parse("2006-01-02 15:04:05", "2015-01-01 00:01:00")
	t2, _ := time.Parse("2006-01-02 15:04:05", "2015-01-01 00:02:00")
	t3, _ := time.Parse("2006-01-02 15:04:05", "2015-01-01 00:03:00")

	tickChan <- t1
	got := <-iterChan
	assert.Equal(t, &iter.Interval{Number: 1, StartTime: time.Time{}, StopTime: t1}, got)

	tickChan <- t2
	got = <-iterChan
	assert.Equal(t, &iter.Interval{Number: 2, StartTime: t1, StopTime: t2}, got)

	tickChan <- t3
	got = <-iterChan
	assert.Equal(t, &iter.Interval{Number: 3, StartTime: t2, StopTime: t3}, got)
}
