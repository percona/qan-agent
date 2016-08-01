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

package pct

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"

	"github.com/gorilla/mux"
	"github.com/percona/pmm/proto"

	. "gopkg.in/check.v1"
)

/////////////////////////////////////////////////////////////////////////////
// sys.go test suite
/////////////////////////////////////////////////////////////////////////////

type SysTestSuite struct {
}

var _ = Suite(&SysTestSuite{})

func (s *SysTestSuite) TestAddPortToURL(t *C) {

	newURL, err := addPortToURL("ws://some-api-url.com:80/path", 81)
	t.Check(err, IsNil)
	t.Check(newURL, Equals, "ws://some-api-url.com:80/path")

	newURL, err = addPortToURL("ws://some-api-url.com/path", 82)
	t.Check(err, IsNil)
	t.Check(newURL, Equals, "ws://some-api-url.com:82/path")

	newURL, err = addPortToURL("ws://some-api-url.com", 80)
	t.Check(err, IsNil)
	t.Check(newURL, Equals, "ws://some-api-url.com:80")
}

func (s *SysTestSuite) TestCleanAgentLinks(t *C) {
	l := map[string]string{
		"cmd":  "http://hhhh",
		"data": "ws://lllll/path",
	}

	expect := map[string]string{
		"cmd":  "http://hhhh",
		"data": "ws://lllll:80/path",
	}

	cleanAgentLinks(l)
	t.Check(l, DeepEquals, expect)
}

func (s *SysTestSuite) TestPing(t *C) {
	r := http.NewServeMux()
	r.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintln(w, "Pong")
	})
	ts := httptest.NewServer(r)
	defer ts.Close()

	headers := map[string]string{
		"X-something": "a value",
	}
	i, err := Ping(ts.URL, headers)
	t.Check(err, IsNil)
	t.Check(i, Equals, 200)

}

func (s *SysTestSuite) TestConnect(t *C) {
	r := mux.NewRouter()
	ts := httptest.NewServer(r)

	// Fake API handlers
	f := func(w http.ResponseWriter, r *http.Request) {
		links := proto.Links{
			Links: map[string]string{
				"agents":    ts.URL + "/agents",
				"instances": ts.URL + "/instances",
			},
		}
		buf, _ := json.Marshal(links)
		fmt.Fprintln(w, string(buf))
	}

	g := func(w http.ResponseWriter, r *http.Request) {
		links := proto.Links{
			Links: map[string]string{
				"cmd":  "http://percona.com/api",
				"log":  "https://percona.com:443/log",
				"data": "ws://percona.com/wsock",
				"self": "http://percona.com/self/1234",
			},
		}
		buf, _ := json.Marshal(links)
		fmt.Fprintln(w, string(buf))
	}

	r.HandleFunc("/path", f)
	r.HandleFunc("/agents/", g)
	defer ts.Close()

	// Connect method receives a host without http://, just the hostname
	u, _ := url.Parse(ts.URL)

	a := NewAPI()
	err := a.Connect(u.Host, "/path", "")
	t.Check(err, IsNil)

}
