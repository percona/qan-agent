package profiler

import (
	"log"
	"sync"
	"time"

	"github.com/percona/pmgo"
)

const (
	MgoTimeoutDialInfo      = 5 * time.Second
	MgoTimeoutSessionSync   = 5 * time.Second
	MgoTimeoutSessionSocket = 5 * time.Second
)

type newMonitor func(
	session pmgo.SessionManager,
	dbName string,
) *monitor

func NewMonitors(
	session pmgo.SessionManager,
	newMonitor newMonitor,
) *monitors {
	return &monitors{
		session:    session,
		newMonitor: newMonitor,
		monitors:   map[string]*monitor{},
	}
}

type monitors struct {
	// dependencies
	session    pmgo.SessionManager
	newMonitor newMonitor

	// monitors
	monitors map[string]*monitor

	// state
	sync.RWMutex // Lock() to protect internal consistency of the service
}

func (self *monitors) MonitorAll() error {
	databases := map[string]struct{}{}
	databasesSlice, err := self.listDatabases()
	if err != nil {
		return err
	}
	for _, dbName := range databasesSlice {
		// Skip admin and local databases to avoid collecting queries from replication and mongodb_exporter
		//switch dbName {
		//case "admin", "local":
		//	continue
		//default:
		//}

		// change slice to map for easier lookup
		databases[dbName] = struct{}{}

		// if database is already monitored then nothing to do, skip it
		if _, ok := self.monitors[dbName]; ok {
			continue
		}

		// if database is not monitored yet then we need to create new monitor
		m := self.newMonitor(
			self.session,
			dbName,
		)
		// ... and start it
		err := m.Start()
		if err != nil {
			log.Println(err)
			return err
		}

		// add new monitor to list of monitored databases
		self.monitors[dbName] = m
	}

	// if database is no longer present then stop monitoring it
	for dbName := range self.monitors {
		if _, ok := databases[dbName]; !ok {
			self.monitors[dbName].Stop()
			delete(self.monitors, dbName)
		}
	}

	return nil
}

func (self *monitors) StopAll() {
	monitors := self.GetAll()

	for dbName := range monitors {
		self.Stop(dbName)
	}
}

func (self *monitors) Stop(dbName string) {
	m := self.Get(dbName)
	m.Stop()

	self.Lock()
	defer self.Unlock()
	delete(self.monitors, dbName)
}

func (self *monitors) Get(dbName string) *monitor {
	self.RLock()
	defer self.RUnlock()

	return self.monitors[dbName]
}

func (self *monitors) GetAll() map[string]*monitor {
	self.RLock()
	defer self.RUnlock()

	list := map[string]*monitor{}
	for dbName, m := range self.monitors {
		list[dbName] = m
	}

	return list
}

func (self *monitors) listDatabases() ([]string, error) {
	session := self.session.Copy()
	defer session.Close()
	return session.DatabaseNames()
}
