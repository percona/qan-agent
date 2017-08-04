package profiler

import (
	"sync"
	"time"

	"github.com/percona/pmgo"
	"gopkg.in/mgo.v2"
)

const (
	MgoTimeoutSessionSync   = 5 * time.Second
	MgoTimeoutSessionSocket = 5 * time.Second
)

type newMonitor func(
	dialInfo *pmgo.DialInfo,
	dialer pmgo.Dialer,
) *monitor

func NewMonitors(
	dialInfo *pmgo.DialInfo,
	dialer pmgo.Dialer,
	newMonitor newMonitor,
) *monitors {
	return &monitors{
		dialInfo:   dialInfo,
		dialer:     dialer,
		monitors:   map[string]*monitor{},
		newMonitor: newMonitor,
	}
}

type monitors struct {
	// dependencies
	dialInfo   *pmgo.DialInfo
	dialer     pmgo.Dialer
	newMonitor newMonitor

	// monitors
	monitors map[string]*monitor

	// state
	sync.RWMutex // Lock() to protect internal consistency of the service
}

func (self *monitors) MonitorAll() error {
	databases := map[string]struct{}{}
	databasesSlice, err := listDatabases(self.dialInfo, self.dialer)
	if err != nil {
		return err
	}
	for _, dbName := range databasesSlice {
		// change slice to map for easier lookup
		databases[dbName] = struct{}{}

		// if database is already monitored then nothing to do, skip it
		if _, ok := self.monitors[dbName]; ok {
			continue
		}

		// if database is not monitored yet then we need to create new profiler

		// create copy of dialInfo
		dialInfo := &pmgo.DialInfo{}
		*dialInfo = *self.dialInfo

		// set database name for connection
		dialInfo.Database = dbName

		// create new monitor and start it
		m := self.newMonitor(
			dialInfo,
			self.dialer,
		)
		err := m.Start()
		if err != nil {
			return err
		}

		// add new monitor to list of monitored databases
		self.monitors[dbName] = m
	}

	// if databases is no longer present then stop monitoring it
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

func listDatabases(dialInfo *pmgo.DialInfo, dialer pmgo.Dialer) ([]string, error) {
	session, err := dialer.DialWithInfo(dialInfo)
	if err != nil {
		return nil, err

	}
	defer session.Close()

	session.SetMode(mgo.Eventual, true)
	session.SetSyncTimeout(MgoTimeoutSessionSync)
	session.SetSocketTimeout(MgoTimeoutSessionSocket)
	return session.DatabaseNames()
}
