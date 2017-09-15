package collector

import (
	"fmt"
	"sync"
	"time"

	"github.com/percona/percona-toolkit/src/go/mongolib/proto"
	"github.com/percona/pmgo"
	"github.com/percona/qan-agent/qan/analyzer/mongo/status"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	MgoTimeoutDialInfo      = 5 * time.Second
	MgoTimeoutSessionSync   = 5 * time.Second
	MgoTimeoutSessionSocket = 5 * time.Second
	MgoTimeoutTail          = 1 * time.Second
)

func New(dialInfo *pmgo.DialInfo, dialer pmgo.Dialer) *Collector {
	return &Collector{
		dialInfo: dialInfo,
		dialer:   dialer,
	}
}

type Collector struct {
	// dependencies
	dialInfo *pmgo.DialInfo
	dialer   pmgo.Dialer

	// provides
	docsChan chan proto.SystemProfile

	// status
	status *status.Status

	// state
	sync.RWMutex                 // Lock() to protect internal consistency of the service
	running      bool            // Is this service running?
	doneChan     chan struct{}   // close(doneChan) to notify goroutines that they should shutdown
	wg           *sync.WaitGroup // Wait() for goroutines to stop after being notified they should shutdown
}

// Start starts but doesn't wait until it exits
func (self *Collector) Start() (<-chan proto.SystemProfile, error) {
	self.Lock()
	defer self.Unlock()
	if self.running {
		return nil, nil
	}

	// create new channels over which we will communicate to...
	// ... outside world by sending collected docs
	self.docsChan = make(chan proto.SystemProfile)
	// ... inside goroutine to close it
	self.doneChan = make(chan struct{})

	// set status
	stats := &stats{}
	self.status = status.New(stats)

	// start a goroutine and Add() it to WaitGroup
	// so we could later Wait() for it to finish
	self.wg = &sync.WaitGroup{}
	self.wg.Add(1)

	// create ready sync.Cond so we could know when goroutine actually started getting data from db
	ready := sync.NewCond(&sync.Mutex{})
	ready.L.Lock()
	defer ready.L.Unlock()

	go start(
		self.wg,
		self.dialInfo,
		self.dialer,
		self.docsChan,
		self.doneChan,
		stats,
		ready,
	)

	// wait until we actually fetch data from db
	ready.Wait()

	self.running = true
	return self.docsChan, nil
}

// Stop stops running
func (self *Collector) Stop() {
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
	close(self.docsChan)
	return
}

func (self *Collector) Running() bool {
	self.RLock()
	defer self.RUnlock()
	return self.running
}

func (self *Collector) Status() map[string]string {
	if !self.Running() {
		return nil
	}

	s := self.status.Map()
	s["profile"] = getProfile(self.dialInfo, self.dialer)

	return s
}

func getProfile(
	dialInfo *pmgo.DialInfo,
	dialer pmgo.Dialer,
) string {
	dialInfo.Timeout = MgoTimeoutDialInfo
	session, err := dialer.DialWithInfo(dialInfo)
	if err != nil {
		return fmt.Sprintf("%s", err)
	}
	defer session.Close()
	session.SetMode(mgo.Eventual, true)
	session.SetSyncTimeout(MgoTimeoutSessionSync)
	session.SetSocketTimeout(MgoTimeoutSessionSocket)

	result := struct {
		Was       int
		Slowms    int
		Ratelimit int
	}{}
	err = session.DB(dialInfo.Database).Run(
		bson.M{
			"profile": -1,
		},
		&result,
	)
	if err != nil {
		return fmt.Sprintf("%s", err)
	}

	if result.Was == 0 {
		return "Profiling disabled. Please enable profiling for this database or whole MongoDB server (https://docs.mongodb.com/manual/tutorial/manage-the-database-profiler/)."
	}

	if result.Was == 1 {
		return fmt.Sprintf("Profiling enabled for slow queries only (slowms: %d)", result.Slowms)
	}

	if result.Was == 2 {
		// if result.Ratelimit == 0 we assume ratelimit is not supported
		// so all queries have ratelimit = 1 (log all queries)
		if result.Ratelimit == 0 {
			result.Ratelimit = 1
		}
		return fmt.Sprintf("Profiling enabled for all queries (ratelimit: %d)", result.Ratelimit)
	}
	return fmt.Sprintf("Unknown profiling state: %d", result.Was)
}

func (self *Collector) Name() string {
	return "collector"
}

func start(
	wg *sync.WaitGroup,
	dialInfo *pmgo.DialInfo,
	dialer pmgo.Dialer,
	docsChan chan<- proto.SystemProfile,
	doneChan <-chan struct{},
	stats *stats,
	ready *sync.Cond,
) {
	// signal WaitGroup when goroutine finished
	defer wg.Done()

	firstTry := true
	for {
		// make a connection and collect data
		connectAndCollect(
			dialInfo,
			dialer,
			docsChan,
			doneChan,
			stats,
			ready,
		)

		select {
		// check if we should shutdown
		case <-doneChan:
			return
		// wait some time before reconnecting
		case <-time.After(1 * time.Second):
		}

		// After first failure in connection we signal that we are ready anyway
		// this way service starts, and will automatically connect when db is available.
		if firstTry {
			signalReady(ready)
			firstTry = false
		}
	}
}

func connectAndCollect(
	dialInfo *pmgo.DialInfo,
	dialer pmgo.Dialer,
	docsChan chan<- proto.SystemProfile,
	doneChan <-chan struct{},
	stats *stats,
	ready *sync.Cond,
) {
	dialInfo.Timeout = MgoTimeoutDialInfo
	session, err := dialer.DialWithInfo(dialInfo)
	if err != nil {
		return
	}
	defer session.Close()
	session.SetMode(mgo.Eventual, true)
	session.SetSyncTimeout(MgoTimeoutSessionSync)
	session.SetSocketTimeout(MgoTimeoutSessionSocket)

	now := bson.Now()
	stats.Started.Set(time.Now().UTC().Format("2006-01-02 15:04:05"))

	collection := session.DB(dialInfo.Database).C("system.profile")
	for {
		query := bson.M{
			"ts": bson.M{"$gt": now},
			"op": bson.M{"$nin": []string{"getmore"}},
		}
		collect(
			collection,
			query,
			docsChan,
			doneChan,
			stats,
			ready,
		)

		select {
		// check if we should shutdown
		case <-doneChan:
			return
		// wait some time before retrying
		case <-time.After(1 * time.Second):
		}
	}
}

func collect(
	collection pmgo.CollectionManager,
	query bson.M,
	docsChan chan<- proto.SystemProfile,
	doneChan <-chan struct{},
	stats *stats,
	ready *sync.Cond,
) {
	iterator := collection.Find(query).Sort("$natural").Tail(MgoTimeoutTail)
	defer iterator.Close()

	stats.IteratorCreated.Set(time.Now().UTC().Format("2006-01-02 15:04:05"))
	stats.IteratorCounter.Add(1)

	// we got iterator, we are ready
	signalReady(ready)

	for {
		// check if we should shutdown
		select {
		case <-doneChan:
			return
		default:
			// just continue if not
		}

		doc := proto.SystemProfile{}
		for iterator.Next(&doc) {
			stats.In.Add(1)

			// check if we should shutdown
			select {
			case <-doneChan:
				return
			default:
				// just continue if not
			}

			// try to push doc
			select {
			case docsChan <- doc:
				stats.Out.Add(1)
			// or exit if we can't push the doc and we should shutdown
			// note that if we can push the doc then exiting is not guaranteed
			// that's why we have separate `select <-doneChan` above
			case <-doneChan:
				return
			}
		}
		if err := iterator.Err(); err != nil {
			stats.IteratorErrCounter.Add(1)
			stats.IteratorErrLast.Set(err.Error())
			return
		}
		if iterator.Timeout() {
			stats.IteratorTimeout.Add(1)
			continue
		}
	}
}

func signalReady(ready *sync.Cond) {
	ready.L.Lock()
	defer ready.L.Unlock()
	ready.Broadcast()
}
