package collector

import (
	"log"
	"sync"
	"time"

	"github.com/percona/percona-toolkit/src/go/mongolib/proto"
	"github.com/percona/pmgo"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

func New(dialInfo *mgo.DialInfo, dialer pmgo.Dialer) *Collector {
	return &Collector{
		dialInfo: dialInfo,
		dialer:   dialer,
	}
}

type Collector struct {
	// dependencies
	dialInfo *mgo.DialInfo
	dialer   pmgo.Dialer

	// provides
	docsChan chan proto.SystemProfile

	// state
	sync.Mutex                 // Lock() to protect internal consistency of the service
	running    bool            // Is this service running?
	doneChan   chan struct{}   // close(doneChan) to notify goroutines that they should shutdown
	wg         *sync.WaitGroup // Wait() for goroutines to stop after being notified they should shutdown
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

	// start a goroutine and Add() it to WaitGroup
	// so we could later Wait() for it to finish
	self.wg = &sync.WaitGroup{}
	self.wg.Add(1)

	// create ready sync.Cond so we could know when goroutine actually started getting data from db
	ready := sync.NewCond(&sync.Mutex{})
	ready.L.Lock()
	defer ready.L.Unlock()
	go start(self.wg, self.dialInfo, self.dialer, self.docsChan, self.doneChan, ready)

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

func start(
	wg *sync.WaitGroup,
	dialInfo *mgo.DialInfo,
	dialer pmgo.Dialer,
	docsChan chan<- proto.SystemProfile,
	doneChan <-chan struct{},
	ready *sync.Cond,
) {
	// signal WaitGroup when goroutine finished
	defer wg.Done()

	dialInfo.Timeout = 5 * time.Second
	firstTry := true
	for {
		// make a connection and collect data
		connectAndCollect(dialInfo, dialer, docsChan, doneChan, ready)

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
	dialInfo *mgo.DialInfo,
	dialer pmgo.Dialer,
	docsChan chan<- proto.SystemProfile,
	doneChan <-chan struct{},
	ready *sync.Cond,
) {
	session, err := dialer.DialWithInfo(dialInfo)
	if err != nil {
		return
	}
	defer session.Close()

	// @todo
	//session.SetSyncTimeout(1 * time.Minute)
	//session.SetSocketTimeout(1 * time.Minute)

	collection := session.DB(dialInfo.Database).C("system.profile")
	for {
		query := bson.M{
			"ts": bson.M{"$gt": bson.Now()},
			"op": bson.M{"$nin": []string{"getmore", "delete"}},
		}
		collect(collection, query, docsChan, doneChan, ready)

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
	ready *sync.Cond,
) {
	iterator := collection.Find(query).Sort("$natural").Tail(1 * time.Second)
	defer iterator.Close()

	// we got iterator, we are ready
	signalReady(ready)

	for {
		doc := proto.SystemProfile{}
		for iterator.Next(&doc) {
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
				// or exit if we can't push the doc and we should shutdown
				// note that if we can push the doc then exiting is not guaranteed
				// that's why we have separate `select <-doneChan` above
			case <-doneChan:
				return
			}
		}
		if iterator.Err() != nil {
			log.Println(iterator.Err())
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
