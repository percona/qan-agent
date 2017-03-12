package mqd

import (
	"crypto/md5"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/montanaflynn/stats"
	pm "github.com/percona/percona-toolkit/src/go/mongolib/proto"
)

const (
	MAX_DEPTH_LEVEL = 10
)

var (
	ErrCannotGetQuery = errors.New("cannot get query field from the profile document (it is not a map)")

	// This is a regexp array to filter out the keys we don't want in the fingerprint
	keyFilters = func() []string {
		return []string{"^shardVersion$", "^\\$"}
	}
)

type statsArray []Stat

func (a statsArray) Len() int           { return len(a) }
func (a statsArray) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a statsArray) Less(i, j int) bool { return a[i].Count < a[j].Count }

type times []time.Time

func (a times) Len() int           { return len(a) }
func (a times) Swap(i, j int)      { a[i], a[j] = a[j], a[i] }
func (a times) Less(i, j int) bool { return a[i].Before(a[j]) }

type Stat struct {
	ID             string
	Operation      string
	Fingerprint    string
	Namespace      string
	Query          map[string]interface{}
	Count          int
	TableScan      bool
	NScanned       []float64
	NReturned      []float64
	QueryTime      []float64 // in milliseconds
	ResponseLength []float64
	LockTime       times
	BlockedTime    times
	FirstSeen      time.Time
	LastSeen       time.Time
}

type GroupKey struct {
	Operation   string
	Fingerprint string
	Namespace   string
}

type statistics struct {
	Pct    float64
	Total  float64
	Min    float64
	Max    float64
	Avg    float64
	Pct95  float64
	StdDev float64
	Median float64
}

type queryInfo struct {
	Count          int
	Operation      string
	Query          string
	Fingerprint    string
	FirstSeen      time.Time
	ID             string
	LastSeen       time.Time
	Namespace      string
	NoVersionCheck bool
	QPS            float64
	QueryTime      statistics
	Rank           int
	Ratio          float64
	ResponseLength statistics
	Returned       statistics
	Scanned        statistics
}

type iter interface {
	All(result interface{}) error
	Close() error
	Err() error
	For(result interface{}, f func() error) (err error)
	Next(result interface{}) bool
	Timeout() bool
}

type options struct {
	AuthDB          string
	Database        string
	Debug           bool
	Help            bool
	Host            string
	Limit           int
	LogLevel        string
	NoVersionCheck  bool
	OrderBy         []string
	Password        string
	SkipCollections []string
	User            string
	Version         bool
}

// This func receives a doc from the profiler and returns:
// true : the document must be considered
// false: the document must be skipped
type DocsFilter func(pm.SystemProfile) bool

func GetData(i iter, filters []DocsFilter) []Stat {
	var doc pm.SystemProfile
	stats := make(map[GroupKey]*Stat)

	for i.Next(&doc) && i.Err() == nil {
		ProcessDoc(&doc, filters, stats)
	}

	// We need to sort the data but a hash cannot be sorted so, convert the hash having
	// the results to a slice
	return ToStatSlice(stats)
}

func ToStatSlice(stats map[GroupKey]*Stat) []Stat {
	sa := statsArray{}
	for _, s := range stats {
		sa = append(sa, *s)
	}

	sort.Sort(sa)
	return sa
}

func ProcessDoc(doc *pm.SystemProfile, filters []DocsFilter, stats map[GroupKey]*Stat) {
	// filter out unwanted query
	for _, filter := range filters {
		if filter(*doc) == false {
			return
		}
	}

	// if there is no query then there is nothing to process
	if len(doc.Query) <= 0 {
		return
	}

	fp, err := fingerprint(doc.Query)
	if err != nil {
		return
	}
	var s *Stat
	var ok bool
	key := GroupKey{
		Operation:   doc.Op,
		Fingerprint: fp,
		Namespace:   doc.Ns,
	}
	if s, ok = stats[key]; !ok {
		realQuery, _ := getQueryField(doc.Query)
		s = &Stat{
			ID:          fmt.Sprintf("%x", md5.Sum([]byte(fp+doc.Ns))),
			Operation:   doc.Op,
			Fingerprint: fp,
			Namespace:   doc.Ns,
			TableScan:   false,
			Query:       realQuery,
		}
		stats[key] = s
	}
	s.Count++
	s.NScanned = append(s.NScanned, float64(doc.DocsExamined))
	s.NReturned = append(s.NReturned, float64(doc.Nreturned))
	s.QueryTime = append(s.QueryTime, float64(doc.Millis))
	s.ResponseLength = append(s.ResponseLength, float64(doc.ResponseLength))
	var zeroTime time.Time
	if s.FirstSeen == zeroTime || s.FirstSeen.After(doc.Ts) {
		s.FirstSeen = doc.Ts
	}
	if s.LastSeen == zeroTime || s.LastSeen.Before(doc.Ts) {
		s.LastSeen = doc.Ts
	}
}

func CalcQueryStats(queries []Stat, uptime int64) []queryInfo {
	queryStats := []queryInfo{}
	_, totalScanned, totalReturned, totalQueryTime, totalBytes := calcTotals(queries)
	for rank, query := range queries {
		buf, _ := json.Marshal(query.Query)
		qi := queryInfo{
			Rank:           rank,
			Count:          query.Count,
			ID:             query.ID,
			Operation:      query.Operation,
			Query:          string(buf),
			Fingerprint:    query.Fingerprint,
			Scanned:        calcStats(query.NScanned),
			Returned:       calcStats(query.NReturned),
			QueryTime:      calcStats(query.QueryTime),
			ResponseLength: calcStats(query.ResponseLength),
			FirstSeen:      query.FirstSeen,
			LastSeen:       query.LastSeen,
			Namespace:      query.Namespace,
			QPS:            float64(query.Count) / float64(uptime),
		}
		if totalScanned > 0 {
			qi.Scanned.Pct = qi.Scanned.Total * 100 / totalScanned
		}
		if totalReturned > 0 {
			qi.Returned.Pct = qi.Returned.Total * 100 / totalReturned
		}
		if totalQueryTime > 0 {
			qi.QueryTime.Pct = qi.QueryTime.Total * 100 / totalQueryTime
		}
		if totalBytes > 0 {
			qi.ResponseLength.Pct = qi.ResponseLength.Total / totalBytes
		}
		if qi.Returned.Total > 0 {
			qi.Ratio = qi.Scanned.Total / qi.Returned.Total
		}
		queryStats = append(queryStats, qi)
	}
	return queryStats
}

func CalcTotalQueryStats(queries []Stat, uptime int64) queryInfo {
	qi := queryInfo{}
	qs := Stat{}
	_, totalScanned, totalReturned, totalQueryTime, totalBytes := calcTotals(queries)
	for _, query := range queries {
		qs.NScanned = append(qs.NScanned, query.NScanned...)
		qs.NReturned = append(qs.NReturned, query.NReturned...)
		qs.QueryTime = append(qs.QueryTime, query.QueryTime...)
		qs.ResponseLength = append(qs.ResponseLength, query.ResponseLength...)
		qi.Count += query.Count
	}

	qi.Scanned = calcStats(qs.NScanned)
	qi.Returned = calcStats(qs.NReturned)
	qi.QueryTime = calcStats(qs.QueryTime)
	qi.ResponseLength = calcStats(qs.ResponseLength)

	if totalScanned > 0 {
		qi.Scanned.Pct = qi.Scanned.Total * 100 / totalScanned
	}
	if totalReturned > 0 {
		qi.Returned.Pct = qi.Returned.Total * 100 / totalReturned
	}
	if totalQueryTime > 0 {
		qi.QueryTime.Pct = qi.QueryTime.Total * 100 / totalQueryTime
	}
	if totalBytes > 0 {
		qi.ResponseLength.Pct = qi.ResponseLength.Total / totalBytes
	}
	if qi.Returned.Total > 0 {
		qi.Ratio = qi.Scanned.Total / qi.Returned.Total
	}

	return qi
}

func calcTotals(queries []Stat) (totalCount int, totalScanned, totalReturned, totalQueryTime, totalBytes float64) {

	for _, query := range queries {
		totalCount += query.Count

		scanned, _ := stats.Sum(query.NScanned)
		totalScanned += scanned

		returned, _ := stats.Sum(query.NReturned)
		totalReturned += returned

		queryTime, _ := stats.Sum(query.QueryTime)
		totalQueryTime += queryTime

		bytes, _ := stats.Sum(query.ResponseLength)
		totalBytes += bytes
	}
	return
}

func calcStats(samples []float64) statistics {
	var s statistics
	s.Total, _ = stats.Sum(samples)
	s.Min, _ = stats.Min(samples)
	s.Max, _ = stats.Max(samples)
	s.Avg, _ = stats.Mean(samples)
	s.Pct95, _ = stats.PercentileNearestRank(samples, 95)
	s.StdDev, _ = stats.StandardDeviation(samples)
	s.Median, _ = stats.Median(samples)
	return s
}

func getQueryField(query map[string]interface{}) (map[string]interface{}, error) {
	// MongoDB 3.0
	if squery, ok := query["$query"]; ok {
		// just an extra check to ensure this type assertion won't fail
		if ssquery, ok := squery.(map[string]interface{}); ok {
			return ssquery, nil
		}
		return nil, ErrCannotGetQuery
	}
	// MongoDB 3.2+
	if squery, ok := query["filter"]; ok {
		if ssquery, ok := squery.(map[string]interface{}); ok {
			return ssquery, nil
		}
		return nil, ErrCannotGetQuery
	}
	return query, nil
}

// Query is the top level map query element
// Example for MongoDB 3.2+
//     "query" : {
//        "find" : "col1",
//        "filter" : {
//            "s2" : {
//                "$lt" : "54701",
//                "$gte" : "73754"
//            }
//        },
//        "sort" : {
//            "user_id" : 1
//        }
//     }
func fingerprint(query map[string]interface{}) (string, error) {

	realQuery, err := getQueryField(query)
	if err != nil {
		// Try to encode doc.Query as json for prettiness
		if buf, err := json.Marshal(realQuery); err == nil {
			return "", fmt.Errorf("%v for query %s", err, string(buf))
		}
		// If we cannot encode as json, return just the error message without the query
		return "", err
	}
	retKeys := keys(realQuery, 0)

	sort.Strings(retKeys)

	// if there is a sort clause in the query, we have to add all fields in the sort
	// fields list that are not in the query keys list (retKeys)
	if sortKeys, ok := query["sort"]; ok {
		if sortKeysMap, ok := sortKeys.(map[string]interface{}); ok {
			sortKeys := mapKeys(sortKeysMap, 0)
			for _, sortKey := range sortKeys {
				if !inSlice(sortKey, retKeys) {
					retKeys = append(retKeys, sortKey)
				}
			}
		}
	}

	return strings.Join(retKeys, ","), nil
}

func inSlice(str string, list []string) bool {
	for _, v := range list {
		if v == str {
			return true
		}
	}
	return false
}

func keys(query map[string]interface{}, level int) []string {
	ks := []string{}
	for key, value := range query {
		if shouldSkipKey(key) {
			continue
		}
		ks = append(ks, key)
		if m, ok := value.(map[string]interface{}); ok {
			level++
			if level <= MAX_DEPTH_LEVEL {
				ks = append(ks, keys(m, level)...)
			}
		}
	}
	sort.Strings(ks)
	return ks
}

func mapKeys(query map[string]interface{}, level int) []string {
	ks := []string{}
	for key, value := range query {
		ks = append(ks, key)
		if m, ok := value.(map[string]interface{}); ok {
			level++
			if level <= MAX_DEPTH_LEVEL {
				ks = append(ks, keys(m, level)...)
			}
		}
	}
	sort.Strings(ks)
	return ks
}

func shouldSkipKey(key string) bool {
	for _, filter := range keyFilters() {
		if matched, _ := regexp.MatchString(filter, key); matched {
			return true
		}
	}
	return false
}
