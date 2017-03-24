/*
   Copyright (c) 2017, Percona LLC and/or its affiliates. All rights reserved.

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

package main_test

import (
	"fmt"
	"io/ioutil"
	"log"
	"net/url"
	"os"
	"os/exec"
	"testing"

	"github.com/percona/pmm/proto"
	pc "github.com/percona/pmm/proto/config"
	"github.com/percona/qan-agent/pct"
	"github.com/percona/qan-agent/test/cmdtest"
	"github.com/percona/qan-agent/test/fakeapi"
	"github.com/stretchr/testify/suite"
)

type TestSuite struct {
	suite.Suite
	username string
	hostname string
	bin      string
	bindir   string
	fakeApi  *fakeapi.FakeApi
}

func (s *TestSuite) SetupSuite() {
	var err error

	// We can't/shouldn't use /usr/local/percona/ (the default basedir), so use
	// a tmpdir instead with roughly the same structure.
	basedir, err := ioutil.TempDir("/tmp", "agent-installer-test-basedir-")
	s.Nil(err)
	pct.Basedir.Init(basedir)

	s.bindir, err = ioutil.TempDir("/tmp", "agent-installer-test-bin-")
	s.Nil(err)
	s.bin = s.bindir + "/agent-installer"
	cmd := exec.Command("go", "build", "-o", s.bin)
	err = cmd.Run()
	s.Nil(err, "Failed to build installer: %s", err)

	s.username = "root"

	// Default data
	// Hostname must be correct because installer checks that
	// hostname == mysql hostname to enable QAN.
	s.hostname, _ = os.Hostname()
}

func (s *TestSuite) SetupTest() {
	// Create fake api server
	s.fakeApi = fakeapi.NewFakeApi()

	// Remove config dir between tests.
	err := os.RemoveAll(pct.Basedir.Path())
	s.Nil(err)
}

func (s *TestSuite) TearDownTest() {
	// Shutdown fake api server
	s.fakeApi.Close()
}

func (s *TestSuite) TearDownSuite() {
	{
		err := os.RemoveAll(pct.Basedir.Path())
		s.Nil(err)
	}
	{
		err := os.RemoveAll(s.bindir)
		s.Nil(err)
	}
}

// --------------------------------------------------------------------------

func (s *TestSuite) TestDefaultInstall() {
	// Register required api handlers
	s.fakeApi.AppendPing()
	osInstance := &proto.Instance{
		Subsystem: "os",
		Id:        20,
	}
	agentInstance := &proto.Instance{
		Subsystem: "agent",
		Id:        42,
	}
	s.fakeApi.AppendInstances([]*proto.Instance{
		osInstance,
		agentInstance,
	})

	cmd := exec.Command(
		s.bin,
		"-basedir="+pct.Basedir.Path(),
		s.fakeApi.URL(),
	)

	cmdTest := cmdtest.NewCmdTest(cmd)
	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}
	err := cmd.Wait()
	s.Nil(err)

	s.Equal("CTRL-C at any time to quit\n", cmdTest.ReadLine())
	s.Equal(fmt.Sprintf("API host: %s\n", s.fakeApi.URL()), cmdTest.ReadLine())

	s.Regexp(fmt.Sprintf("Created OS: name=%s uuid=%s\n", s.hostname, osInstance.UUID), cmdTest.ReadLine())
	s.Regexp(fmt.Sprintf("Created agent instance: name=%s uuid=%s\n", s.hostname, agentInstance.UUID), cmdTest.ReadLine())

	s.Equal("", cmdTest.ReadLine()) // No more data

	s.expectConfigs(
		[]string{
			"agent.conf",
		},
	)

	s.expectAgentConfig(*agentInstance)
}

func (s *TestSuite) TestInstallMysqlFalse() {
	// Register required api handlers
	s.fakeApi.AppendPing()
	osInstance := &proto.Instance{
		Subsystem: "os",
		Id:        20,
	}
	agentInstance := &proto.Instance{
		Subsystem: "agent",
		Id:        42,
	}
	s.fakeApi.AppendInstances([]*proto.Instance{
		osInstance,
		agentInstance,
	})

	cmd := exec.Command(
		s.bin,
		"-basedir="+pct.Basedir.Path(),
		"-mysql=false",
		s.fakeApi.URL(),
	)

	cmdTest := cmdtest.NewCmdTest(cmd)
	if err := cmd.Start(); err != nil {
		log.Fatal(err)
	}
	err := cmd.Wait()
	s.Nil(err)

	s.Equal("CTRL-C at any time to quit\n", cmdTest.ReadLine())
	s.Equal(fmt.Sprintf("API host: %s\n", s.fakeApi.URL()), cmdTest.ReadLine())

	s.Regexp(fmt.Sprintf("Created OS: name=%s uuid=%s\n", s.hostname, osInstance.UUID), cmdTest.ReadLine())
	s.Regexp(fmt.Sprintf("Created agent instance: name=%s uuid=%s\n", s.hostname, agentInstance.UUID), cmdTest.ReadLine())

	s.Equal("", cmdTest.ReadLine()) // No more data

	s.expectConfigs(
		[]string{
			"agent.conf",
		},
	)

	s.expectAgentConfig(*agentInstance)
}

func (s *TestSuite) expectConfigs(expectedConfigs []string) {
	gotConfigs := []string{}
	fileinfos, err := ioutil.ReadDir(pct.Basedir.Dir("config"))
	s.Nil(err)
	for _, fileinfo := range fileinfos {
		gotConfigs = append(gotConfigs, fileinfo.Name())
	}
	s.Equal(expectedConfigs, gotConfigs)
}

func (s *TestSuite) expectAgentConfig(agentInstance proto.Instance) {
	service := "agent"
	apiURL, err := url.Parse(s.fakeApi.URL())
	s.Nil(err)
	expectedConfig := pc.Agent{
		UUID:        agentInstance.UUID,
		ApiHostname: apiURL.Host,
		ServerUser:  "pmm",
	}
	gotConfig := pc.Agent{}
	if _, err := pct.Basedir.ReadConfig(service, &gotConfig); err != nil {
		s.FailNow("Read %s config: %s", service, gotConfig, err)
	}
	s.Equal(expectedConfig, gotConfig)
}

func TestRunSuite(t *testing.T) {
	suite.Run(t, &TestSuite{})
}
