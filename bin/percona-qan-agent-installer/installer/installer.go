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

package installer

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/nu7hatch/gouuid"
	"github.com/percona/go-mysql/dsn"
	"github.com/percona/pmm/proto"
	pc "github.com/percona/pmm/proto/config"
	"github.com/percona/qan-agent/agent/release"
	"github.com/percona/qan-agent/instance"
	"github.com/percona/qan-agent/pct"
)

type Flags struct {
	Bool   map[string]bool
	String map[string]string
	Int64  map[string]int64
}

type Installer struct {
	basedir      string
	api          pct.APIConnector
	instanceRepo *instance.Repo
	agentConfig  *pc.Agent
	hostname     string
	flags        Flags
	// --
	os       *proto.Instance
	agent    *proto.Instance
	dsnAgent dsn.DSN
	debug    bool
}

func newUUID() string {
	u4, err := uuid.NewV4()
	if err != nil {
		fmt.Printf("Could not create UUID4: %v", err)
		return ""
	}
	return strings.Replace(u4.String(), "-", "", -1)
}

func NewInstaller(basedir string, api pct.APIConnector, instanceRepo *instance.Repo, agentConfig *pc.Agent, hostname string, flags Flags) (*Installer, error) {
	installer := &Installer{
		basedir:      basedir,
		api:          api,
		instanceRepo: instanceRepo,
		agentConfig:  agentConfig,
		hostname:     hostname,
		flags:        flags,
		// --
		debug: flags.Bool["debug"],
	}
	return installer, nil
}

func (i *Installer) Run() (err error) {
	if i.debug {
		fmt.Printf("basedir: %s\n", i.basedir)
		fmt.Printf("Agent.Config: %+v\n", i.agentConfig)
	}

	// Must create OS instance first because agent instance references it.
	if err = i.CreateOSInstance(); err != nil {
		return fmt.Errorf("Failed to create OS instance: %s", err)
	}

	if err := i.CreateAgent(); err != nil {
		return fmt.Errorf("Failed to create agent instance: %s", err)
	}

	configs, err := i.GetDefaultConfigs(i.os)
	if err != nil {
		return err
	}
	if err := i.writeConfigs(configs); err != nil {
		return fmt.Errorf("Failed to write configs: %s", err)
	}

	return nil
}

func (i *Installer) CreateOSInstance() error {
	i.os = &proto.Instance{
		Subsystem: "os",
		UUID:      newUUID(),
		Name:      i.hostname,
	}
	created, err := i.api.CreateInstance("/instances", i.os)
	if err != nil {
		return err
	}

	// todo: distro, version

	if err := i.instanceRepo.Add(*i.os, true); err != nil {
		return err
	}

	if created {
		fmt.Printf("Created OS: name=%s uuid=%s\n", i.os.Name, i.os.UUID)
	} else {
		fmt.Printf("Using existing OS instance: name=%s uuid=%s\n", i.os.Name, i.os.UUID)
	}
	return nil
}

func (i *Installer) CreateAgent() error {
	i.agent = &proto.Instance{
		Subsystem:  "agent",
		UUID:       newUUID(),
		ParentUUID: i.os.UUID,
		Name:       i.os.Name,
		Version:    release.VERSION,
	}
	created, err := i.api.CreateInstance("/instances", i.agent)
	if err != nil {
		return err
	}

	// To save data we need agent config with uuid and links
	i.agentConfig.UUID = i.agent.UUID
	i.agentConfig.Links = i.agent.Links

	if created {
		fmt.Printf("Created agent instance: name=%s uuid=%s\n", i.agent.Name, i.agent.UUID)
	} else {
		fmt.Printf("Using existing agent instance: name=%s uuid=%s\n", i.agent.Name, i.agent.UUID)
	}
	return nil
}

func (i *Installer) GetDefaultConfigs(os *proto.Instance) (configs []proto.AgentConfig, err error) {
	agentConfig, err := i.getAgentConfig()
	if err != nil {
		return nil, err
	}
	configs = append(configs, *agentConfig)

	// We don't need log and data configs. They use all built-in defaults.
	return configs, nil
}

func (i *Installer) writeConfigs(configs []proto.AgentConfig) error {
	for _, config := range configs {
		name := config.Service
		switch name {
		case "qan":
			name += "-" + config.UUID
		}
		if err := pct.Basedir.WriteConfigString(name, config.Set); err != nil {
			return err
		}
	}

	return nil
}

func (i *Installer) getAgentConfig() (*proto.AgentConfig, error) {
	configJson, err := json.Marshal(i.agentConfig)
	if err != nil {
		return nil, err
	}
	agentConfig := &proto.AgentConfig{
		Service: "agent",
		Set:     string(configJson),
	}

	return agentConfig, nil
}
