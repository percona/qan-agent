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
	"bytes"
	"compress/gzip"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
	"github.com/percona/pmm/proto"
	"github.com/percona/qan-agent/agent/release"
)

var requiredEntryLinks = []string{"agents", "instances"}
var requiredAgentLinks = []string{"cmd", "log", "data", "self"}
var reHostPort = regexp.MustCompile("(.*):(\\d+)$")

type ConnectionConfig struct {
	User           string
	Password       string
	UseSSL         bool
	UseInsecureSSL bool
}

type APIConnector interface {
	Connect(hostname, basePath, agentUuid string) error
	Init(hostname string, headers map[string]string) (code int, err error)
	Get(url string) (int, []byte, error)
	Post(url string, data []byte) (*http.Response, []byte, error)
	Put(url string, data []byte) (*http.Response, []byte, error)
	CreateInstance(url string, it interface{}) (bool, error)
	EntryLink(resource string) string
	AgentLink(resource string) string
	Origin() string
	Hostname() string
	AgentUuid() string
	URL(paths ...string) string
	GetConnectionConfig() ConnectionConfig
}

// --------------------------------------------------------------------------

type TimeoutClientConfig struct {
	ConnectTimeout   time.Duration
	ReadWriteTimeout time.Duration
}

var timeoutClientConfig = &TimeoutClientConfig{
	ConnectTimeout:   10 * time.Second,
	ReadWriteTimeout: 10 * time.Second,
}

func TimeoutDialer(config *TimeoutClientConfig) func(net, addr string) (c net.Conn, err error) {
	return func(netw, addr string) (net.Conn, error) {
		conn, err := net.DialTimeout(netw, addr, config.ConnectTimeout)
		if err != nil {
			return nil, err
		}
		conn.SetDeadline(time.Now().Add(config.ReadWriteTimeout))
		return conn, nil
	}
}

// --------------------------------------------------------------------------

type API struct {
	origin     string
	hostname   string
	agentUuid  string
	entryLinks map[string]string
	agentLinks map[string]string
	mux        *sync.RWMutex
	client     *http.Client
	//
	user           string
	pass           string
	useSSL         bool
	useInsecureSSL bool
}

func NewAPI(user string, pass string, useSSL bool, useInsecureSSL bool) *API {
	hostname, _ := os.Hostname()
	client := &http.Client{
		Transport: &http.Transport{
			Dial:            TimeoutDialer(timeoutClientConfig),
			TLSClientConfig: &tls.Config{InsecureSkipVerify: useInsecureSSL},
		},
	}
	a := &API{
		origin:     "http://" + hostname,
		agentLinks: make(map[string]string),
		mux:        new(sync.RWMutex),
		client:     client,
		//
		user:           user,
		pass:           pass,
		useSSL:         useSSL,
		useInsecureSSL: useInsecureSSL,
	}
	return a
}

func (a *API) GetConnectionConfig() ConnectionConfig {
	return ConnectionConfig{
		User:           a.user,
		Password:       a.pass,
		UseSSL:         a.useSSL,
		UseInsecureSSL: a.useInsecureSSL,
	}
}

func Ping(hostname string, headers map[string]string) (int, error) {
	url := hostname + "/ping"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, fmt.Errorf("Ping %s error: http.NewRequest: %s", url, err)
	}

	req.Header.Add("X-Percona-Agent-Version", release.VERSION)
	req.Header.Add("X-Percona-Protocol-Version", proto.VERSION)
	if headers != nil {
		for k, v := range headers {
			req.Header.Add(k, v)
		}
	}

	client := &http.Client{
		Transport: &http.Transport{
			Dial:            TimeoutDialer(timeoutClientConfig),
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	_, err = ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return resp.StatusCode, fmt.Errorf("Ping %s error: ioutil.ReadAll: %s", url, err)
	}
	return resp.StatusCode, nil
}

func URL(hostname string, paths ...string) string {
	paths = append(paths, "")
	if len(paths) > 0 {
		if strings.HasPrefix(paths[0], "/") {
			paths[0] = "." + paths[0]
		} else {
			paths[0] = "./" + paths[0]
		}
	}
	u, _ := url.Parse(hostname)
	relativePath := []string{u.Path}
	relativePath = append(relativePath, paths...)
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	p, _ := url.Parse(strings.Join(paths, "/"))
	if u.Scheme == "" {
		u.Scheme = "http"
	}
	return u.ResolveReference(p).String()
}

func (a *API) Connect(hostname, basePath, agentUuid string) error {
	schema := "http"
	if a.useSSL || a.useInsecureSSL {
		schema = "https"
	}
	u := url.URL{
		Scheme: schema,
		Host:   hostname,
		Path:   basePath,
	}
	if a.user != "" {
		u.User = url.UserPassword(a.user, a.pass)
	}

	// Get entry links: GET <API hostname>/
	entryLinks, err := a.getLinks(u.String())
	if err != nil {
		return err
	}
	if err := a.checkLinks(entryLinks, requiredEntryLinks...); err != nil {
		return err
	}

	// Get agent links: <API hostname>/<instances_endpoint>/:uuid
	agentLinks, err := a.getLinks(entryLinks["agents"] + "/" + agentUuid)
	if err != nil {
		return err
	}

	if err := a.checkLinks(agentLinks, requiredAgentLinks...); err != nil {
		return err
	}

	cleanAgentLinks(agentLinks)
	a.prepareAgentLinks(agentLinks)

	// Success: API responds with the links we need.
	a.mux.Lock()
	defer a.mux.Unlock()
	a.hostname = hostname
	a.agentUuid = agentUuid
	a.entryLinks = entryLinks
	a.agentLinks = agentLinks
	return nil
}

func (a *API) Init(hostname string, headers map[string]string) (int, error) {
	code, err := Ping(hostname, headers)
	if err != nil {
		return 0, err
	}

	if code != http.StatusOK {
		return code, fmt.Errorf("Got %d from the API", code)
	}
	a.mux.Lock()
	defer a.mux.Unlock()
	a.hostname = hostname
	return code, nil
}

func (a *API) checkLinks(links map[string]string, req ...string) error {
	for _, link := range req {
		logLink, exist := links[link]
		if !exist || logLink == "" {
			return fmt.Errorf("Missing "+link+" link, got %+v", links)
		}
	}
	return nil
}

/*
API sends a list of links with the form http://host:port/path or ws://host[:port]/path
For websockets, we need to have a port number in the URL because there is no default
port for websocket connections.
This functions checks all links received from the API and fixes ws URLs.
*/
func cleanAgentLinks(agentLinks map[string]string) {
	for key, link := range agentLinks {
		if strings.HasPrefix(link, "ws://") {
			newLink, err := addPortToURL(link, 80)
			if err != nil {
				continue
			}
			agentLinks[key] = newLink
		}
	}
}

func (a *API) prepareAgentLinks(agentLinks map[string]string) {
	if !a.useInsecureSSL && !a.useSSL {
		return
	}
	for key, link := range agentLinks {
		u, err := url.Parse(link)
		if err != nil {
			continue
		}
		if a.useSSL || a.useInsecureSSL {
			if u.Scheme == "http" || u.Scheme == "ws" {
				u.Scheme += "s"
			}
			if a.user != "" {
				u.User = url.UserPassword(a.user, a.pass)
			}
			agentLinks[key] = u.String()
		}
	}
}

func (a *API) setURLSchema(uri string) string {
	if !a.useInsecureSSL && !a.useSSL {
		return uri
	}
	u, err := url.Parse(uri)
	if err != nil {
		return uri
	}
	if a.useSSL || a.useInsecureSSL {
		if u.Scheme == "http" || u.Scheme == "ws" {
			u.Scheme += "s"
		}
	}
	return u.String()
}

// Add a port to an URL if it doesn't have a port
func addPortToURL(uri string, port int) (string, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return "", err
	}

	m := reHostPort.FindStringSubmatch(u.Host)
	if len(m) == 0 {
		u.Host = fmt.Sprintf("%s:%d", u.Host, port)
	}

	return u.String(), nil
}

func (a *API) getLinks(url string) (map[string]string, error) {
	code, data, err := a.Get(url)
	if err != nil {
		return nil, err
	}
	if code != 200 {
		return nil, fmt.Errorf("Failed to get %s from API: status code %d", url, code)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("OK response from %s but no content", url)
	}

	var links proto.Links
	if err := json.Unmarshal(data, &links); err != nil {
		return nil, fmt.Errorf("GET %s error: json.Unmarshal: %s: %s", url, err, string(data))
	}

	return links.Links, nil
}

func (a *API) Get(url string) (int, []byte, error) {
	url = a.setURLSchema(url)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Add("X-Percona-Agent-Version", release.VERSION)
	req.Header.Add("X-Percona-Protocol-Version", proto.VERSION)

	// todo: timeout
	resp, err := a.client.Do(req)
	if err != nil {
		return 0, nil, fmt.Errorf("GET %s error: client.Do: %s", url, err)
	}
	defer resp.Body.Close()

	var data []byte
	if resp.Header.Get("Content-Type") == "application/x-gzip" {
		buf := new(bytes.Buffer)
		gz, err := gzip.NewReader(resp.Body)
		if err != nil {
			return 0, nil, err
		}
		if _, err := io.Copy(buf, gz); err != nil {
			return resp.StatusCode, nil, err
		}
		data = buf.Bytes()
	} else {
		data, err = ioutil.ReadAll(resp.Body)
		if err != nil {
			return resp.StatusCode, nil, fmt.Errorf("GET %s error: ioutil.ReadAll: %s", url, err)
		}
	}
	return resp.StatusCode, data, nil
}

func (a *API) EntryLink(resource string) string {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return a.entryLinks[resource]
}

func (a *API) AgentLink(resource string) string {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return a.agentLinks[resource]
}

func (a *API) Origin() string {
	// No lock because origin doesn't change.
	return a.origin
}

func (a *API) Hostname() string {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return a.hostname
}

func (a *API) URL(paths ...string) string {
	return URL(a.Hostname(), paths...)
}

func (a *API) AgentUuid() string {
	a.mux.RLock()
	defer a.mux.RUnlock()
	return a.agentUuid
}

func (a *API) Post(url string, data []byte) (*http.Response, []byte, error) {
	return a.send("POST", url, data)
}

func (a *API) Put(url string, data []byte) (*http.Response, []byte, error) {
	return a.send("PUT", url, data)
}

func (a *API) send(method, url string, data []byte) (*http.Response, []byte, error) {
	url = a.setURLSchema(url)
	req, err := http.NewRequest(method, url, bytes.NewReader(data))
	header := http.Header{}
	header.Set("X-Percona-Agent-Version", release.VERSION)
	header.Set("X-Percona-Protocol-Version", proto.VERSION)
	req.Header = header

	resp, err := a.client.Do(req)
	if err != nil {
		return resp, nil, err
	}
	content, err := ioutil.ReadAll(resp.Body)
	resp.Body.Close()
	if err != nil {
		return resp, nil, err
	}
	return resp, content, nil
}

func (a *API) CreateInstance(url string, in interface{}) (bool, error) {
	created := false

	data, err := json.Marshal(in)
	if err != nil {
		return created, err
	}

	url = a.URL(url)
	resp, _, err := a.Post(url, data)
	if err != nil {
		return created, err
	}
	if resp.StatusCode == http.StatusCreated {
		created = true
	} else {
		switch resp.StatusCode {
		case http.StatusConflict:
			url = resp.Header.Get("Location")
			if url == "" {
				return created, fmt.Errorf("API did not return Location header value for existing instance")
			}
			// fixme: this cause a null-op update like:
			//   UPDATE instances SET parent_uuid = 'fe49800aaac24e5c65db45c80e79e6f1', dsn = '', name = 'beatrice.local'
			//   WHERE uuid = 'e5f4e6b5aee34f8177afe23d89435660'
			// The UUID in the WHERE clause is the old UUID generated by the installer.
			resp, _, err := a.Put(url, data)
			if err != nil {
				return created, err
			}
			if resp.StatusCode >= 300 {
				return created, fmt.Errorf("PUT %s (update instance) returned HTTP status code %d", url, resp.StatusCode)
			}
		default:
			return created, fmt.Errorf("POST %s (create instance) returned HTTP status code %d", url, resp.StatusCode)
		}
	}

	// API returns URI of new resource in Location header
	uri := resp.Header.Get("Location")
	if uri == "" {
		return created, fmt.Errorf("API did not return Location header value for new instance")
	}

	// GET <api>/instances/:uuid
	code, data, err := a.Get(uri)
	if err != nil {
		return created, err
	}
	if code != http.StatusOK {
		return created, fmt.Errorf("GET %s (get instance) returned HTTP status code %d", uri, code)
	}

	if err := json.Unmarshal(data, in); err != nil {
		return created, fmt.Errorf("GET %s (get instance) returned invalid data: %s", err)
	}

	return created, nil
}
