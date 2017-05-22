package collector

import (
	"fmt"
	"sync"
	"time"

	"github.com/percona/percona-toolkit/src/go/mongolib/proto"
	"github.com/percona/pmgo"
	"github.com/percona/qan-agent/qan/analyzer/mongo/state"
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
	self.pingChan = make(chan struct{})
	self.pongChan = make(chan status)
	self.status = map[string]string{}

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
		self.pingChan,
		self.pongChan,
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

	go self.sendPing()
	profile := getProfile(self.dialInfo, self.dialer)

	status := self.recvPong()

	self.Lock()
	defer self.Unlock()
	for k, v := range status {
		self.status[k] = v
	}
	self.status["profile"] = profile

	return self.status
}

func getProfile(
	dialInfo *pmgo.DialInfo,
	dialer pmgo.Dialer,
) string {
	session, err := dialer.DialWithInfo(dialInfo)
	if err != nil {
		return fmt.Sprintf("%s", err)
	}
	defer session.Close()
	session.SetSyncTimeout(MgoTimeoutSessionSync)
	session.SetSocketTimeout(MgoTimeoutSessionSocket)

	result := struct {
		Was    int
		Slowms int
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

	return fmt.Sprintf("was: %d, slowms: %d", result.Was, result.Slowms)
}

func (self *Collector) Name() string {
	return "collector"
}

func (self *Collector) sendPing() {
	select {
	case self.pingChan <- struct{}{}:
	case <-time.After(1 * time.Second):
		// timeout carry on
	}
}

func (self *Collector) recvPong() map[string]string {
	select {
	case s := <-self.pongChan:
		return state.StatusToMap(s)
	case <-time.After(1 * time.Second):
		// timeout carry on
	}

	return nil
}

func start(
	wg *sync.WaitGroup,
	dialInfo *pmgo.DialInfo,
	dialer pmgo.Dialer,
	docsChan chan<- proto.SystemProfile,
	doneChan <-chan struct{},
	pingChan <-chan struct{},
	pongChan chan<- status,
	ready *sync.Cond,
) {
	// signal WaitGroup when goroutine finished
	defer wg.Done()

	dialInfo.Timeout = MgoTimeoutDialInfo
	firstTry := true
	status := status{}
	for {
		// make a connection and collect data
		connectAndCollect(
			dialInfo,
			dialer,
			docsChan,
			doneChan,
			pingChan,
			pongChan,
			status,
			ready,
		)

		// After first failure in connection we signal that we are ready anyway
		// this way service starts, and will automatically connect when db is available.
		if firstTry {
			signalReady(ready)
			firstTry = false
		}

		select {
		// check if we should shutdown
		case <-doneChan:
			return
		// wait some time before reconnecting
		case <-time.After(1 * time.Second):
		}
	}
}

func connectAndCollect(
	dialInfo *pmgo.DialInfo,
	dialer pmgo.Dialer,
	docsChan chan<- proto.SystemProfile,
	doneChan <-chan struct{},
	pingChan <-chan struct{},
	pongChan chan<- status,
	status status,
	ready *sync.Cond,
) {
	session, err := dialer.DialWithInfo(dialInfo)
	if err != nil {
		return
	}
	defer session.Close()

	// set timeouts or otherwise iter.Next() might hang forever
	session.SetSyncTimeout(MgoTimeoutSessionSync)
	session.SetSocketTimeout(MgoTimeoutSessionSocket)

	collection := session.DB(dialInfo.Database).C("system.profile")
	for {
		query := bson.M{
			"ts": bson.M{"$gt": bson.Now()},
			"op": bson.M{"$nin": []string{"getmore", "delete"}},
		}
		collect(
			collection,
			query,
			docsChan,
			doneChan,
			pingChan,
			pongChan,
			status,
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
	pingChan <-chan struct{},
	pongChan chan<- status,
	status status,
	ready *sync.Cond,
) {
	iterator := collection.Find(query).Sort("$natural").Tail(MgoTimeoutTail)
	defer iterator.Close()

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

		// check if we got ping
		select {
		case <-pingChan:
			go pong(status, pongChan)
		default:
			// just continue if not
		}

		doc := proto.SystemProfile{}
		for iterator.Next(&doc) {
			status.In += 1

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
				go pong(status, pongChan)
			default:
				// just continue if not
			}

			// try to push doc
			select {
			case docsChan <- doc:
				status.Out += 1
				// or exit if we can't push the doc and we should shutdown
				// note that if we can push the doc then exiting is not guaranteed
				// that's why we have separate `select <-doneChan` above
			case <-doneChan:
				return
			}
		}
		if iterator.Err() != nil {
			status.ErrIter += 1
			return
		}
		if iterator.Timeout() {
			continue
		}
	}
}

func signalReady(ready *sync.Cond) {
	ready.L.Lock()
	defer ready.L.Unlock()
	ready.Broadcast()
}

func pong(status status, pongChan chan<- status) {
	select {
	case pongChan <- status:
	case <-time.After(1 * time.Second):
		// timeout carry on
	}
}

type status struct {
	In      uint `name:"in"`
	Out     uint `name:"out"`
	ErrIter uint `name:"err-iter"`
}
