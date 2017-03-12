package collector

import (
	"reflect"
	"testing"

	"github.com/percona/pmgo"
	"github.com/stretchr/testify/assert"
	"gopkg.in/mgo.v2"
	//"gopkg.in/mgo.v2/dbtest"
	//"os"
	//"io/ioutil"
	//"encoding/json"
	"github.com/percona/percona-toolkit/src/go/mongolib/proto"
)

func TestNew(t *testing.T) {
	//t.Parallel()

	dialer := pmgo.NewDialer()
	dialInfo, _ := mgo.ParseURL("127.0.0.1:27017")

	type args struct {
		dialInfo *mgo.DialInfo
		dialer   pmgo.Dialer
	}
	tests := []struct {
		name string
		args args
		want *Collector
	}{
		{
			name: "127.0.0.1:27017",
			args: args{
				dialInfo: dialInfo,
				dialer:   dialer,
			},
			want: &Collector{
				dialInfo: dialInfo,
				dialer:   dialer,
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := New(tt.args.dialInfo, tt.args.dialer); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("New(%v, %v) = %v, want %v", tt.args.dialInfo, tt.args.dialer, got, tt.want)
			}
		})
	}
}

func TestCollector_StartStop(t *testing.T) {
	t.Parallel()

	dialer := pmgo.NewDialer()
	dialInfo, _ := mgo.ParseURL("127.0.0.1:27017")

	collector1 := New(dialInfo, dialer)
	docsChan, err := collector1.Start()
	assert.Nil(t, err)
	assert.NotNil(t, docsChan)

	defer collector1.Stop()
}

func TestCollector_Stop(t *testing.T) {
	t.Parallel()

	dialer := pmgo.NewDialer()
	dialInfo, _ := mgo.ParseURL("127.0.0.1:27017")

	// #1
	notStarted := New(dialInfo, dialer)

	// #2
	started := New(dialInfo, dialer)
	_, err := started.Start()
	assert.Nil(t, err)

	tests := []struct {
		name string
		self *Collector
	}{
		{
			name: "not started",
			self: notStarted,
		},
		{
			name: "started",
			self: started,
		},
		// repeat to be sure Stop() is idempotent
		{
			name: "not started",
			self: notStarted,
		},
		{
			name: "started",
			self: started,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.self.Stop()
		})
	}
}

func TestCollector(t *testing.T) {
	t.Parallel()

	dialer := pmgo.NewDialer()
	dialInfo, _ := mgo.ParseURL("127.0.0.1:27017")

	collector := New(dialInfo, dialer)
	docsChan, err := collector.Start()
	assert.Nil(t, err)
	defer collector.Stop()

	people := []map[string]string{
		{"name": "Kamil"},
		{"name": "Carlos"},
	}
	go func() {
		session, err := dialer.DialWithInfo(dialInfo)
		assert.Nil(t, err)
		for _, person := range people {
			err = session.DB("test").C("people").Insert(&person)
			assert.Nil(t, err)
		}
	}()

	actual := []proto.SystemProfile{}
	for doc := range docsChan {
		if doc.Ns == "test.people" && doc.Query["insert"] == "people" {
			actual = append(actual, doc)
		}
		if len(actual) == len(people) {
			// stopping collector should also close docsChan
			collector.Stop()
		}
	}

}

//var Server dbtest.DBServer
//
//func TestMain(m *testing.M) {
//	return
//	// The tempdir is created so MongoDB has a location to store its files.
//	// Contents are wiped once the server stops
//	os.Setenv("CHECK_SESSIONS", "0")
//	tempDir, _ := ioutil.TempDir("", "testing")
//	Server.SetPath(tempDir)
//
//	dat, err := ioutil.ReadFile("test/sample/system.profile.json")
//	if err != nil {
//		fmt.Printf("cannot load fixtures: %s", err)
//		os.Exit(1)
//	}
//
//	var docs []proto.SystemProfile
//	err = json.Unmarshal(dat, &docs)
//	c := Server.Session().DB("samples").C("system_profile")
//	for _, doc := range docs {
//		c.Insert(doc)
//	}
//
//	retCode := m.Run()
//
//	Server.Session().Close()
//	Server.Session().DB("samples").DropDatabase()
//
//	// Stop shuts down the temporary server and removes data on disk.
//	Server.Stop()
//
//	// call with result of m.Run()
//	os.Exit(retCode)
//}
