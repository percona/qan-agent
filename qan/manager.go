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

package qan

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/percona/pmm/proto"
	pc "github.com/percona/pmm/proto/config"
	"github.com/percona/qan-agent/instance"
	"github.com/percona/qan-agent/pct"
)

const (
	pkg = "qan"
)

var (
	ErrNotRunning     = errors.New("not running")
	ErrAlreadyRunning = errors.New("already running")
)

// An AnalyzerInstance is an Analyzer ran by a Manager, one per MySQL instance
// as configured.
type AnalyzerInstance struct {
	setConfig pc.QAN
	analyzer  Analyzer
}

// A Manager runs AnalyzerInstances, one per MySQL instance as configured.
type Manager struct {
	logger          *pct.Logger
	instanceRepo    *instance.Repo
	analyzerFactory AnalyzerFactory
	// --
	mux       *sync.RWMutex
	running   bool
	analyzers map[string]AnalyzerInstance
	status    *pct.Status
}

func NewManager(
	logger *pct.Logger,
	instanceRepo *instance.Repo,
	analyzerFactory AnalyzerFactory,
) *Manager {
	m := &Manager{
		logger:          logger,
		instanceRepo:    instanceRepo,
		analyzerFactory: analyzerFactory,
		// --
		mux:       &sync.RWMutex{},
		analyzers: make(map[string]AnalyzerInstance),
		status:    pct.NewStatus([]string{pkg}),
	}
	return m
}

/////////////////////////////////////////////////////////////////////////////
// Interface
/////////////////////////////////////////////////////////////////////////////

func (m *Manager) Start() error {
	m.logger.Debug("Start:call")
	defer m.logger.Debug("Start:return")

	m.mux.Lock()
	defer m.mux.Unlock()

	m.status.Update(pkg, "Starting")
	defer func() {
		m.logger.Info("Started")
		m.status.Update(pkg, "Running")
	}()

	filepathGlob := fmt.Sprintf("%s/%s-*%s", pct.Basedir.Dir("config"), pkg, pct.CONFIG_FILE_SUFFIX)
	files, err := filepath.Glob(filepathGlob)
	if err != nil {
		return err
	}
	for _, file := range files {
		data, err := ioutil.ReadFile(file)
		if err != nil && !os.IsNotExist(err) {
			m.logger.Warn(fmt.Sprintf("Cannot read %s: %s", file, err))
			continue
		}

		if len(data) == 0 {
			m.logger.Warn(fmt.Sprintf("%s is empty, removing", file))
			pct.RemoveFile(file)
			continue
		}

		setConfig := pc.QAN{}
		if err := json.Unmarshal(data, &setConfig); err != nil {
			m.logger.Warn(fmt.Sprintf("Cannot decode %s: %s", file, err))
			continue
		}

		// Start the analyzer. If it fails that's ok for
		// the manager itself (i.e. don't fail this func) because user can fix
		// or reconfigure this analyzer instance later and have manager try
		// again to start it.
		if err := m.startAnalyzer(setConfig); err != nil {
			errMsg := fmt.Sprintf("Cannot start Query Analytics on instance %s: %s", setConfig.UUID, err)
			m.logger.Error(errMsg)
			continue
		}
	}

	return nil // success
}

func (m *Manager) Stop() error {
	m.logger.Debug("Stop:call")
	defer m.logger.Debug("Stop:return")

	m.mux.Lock()
	defer m.mux.Unlock()

	for uuid := range m.analyzers {
		if err := m.stopAnalyzer(uuid); err != nil {
			m.logger.Error(err)
		}
	}

	m.logger.Info("Stopped")
	m.status.Update(pkg, "Stopped")
	return nil
}

func (m *Manager) Status() map[string]string {
	m.mux.RLock()
	defer m.mux.RUnlock()
	status := m.status.All()
	for _, a := range m.analyzers {
		for k, v := range a.analyzer.Status() {
			status[k] = v
		}
	}
	return status
}

func (m *Manager) Handle(cmd *proto.Cmd) *proto.Reply {
	m.logger.Debug("Handle:call")
	defer m.logger.Debug("Handle:return")

	m.status.UpdateRe(pkg, "Handling", cmd)
	defer m.status.Update(pkg, "Running")

	m.mux.Lock()
	defer m.mux.Unlock()

	switch cmd.Cmd {
	case "StartTool":
		setConfig := pc.QAN{}
		if err := json.Unmarshal(cmd.Data, &setConfig); err != nil {
			return cmd.Reply(nil, err)
		}
		uuid := setConfig.UUID
		if err := m.startAnalyzer(setConfig); err != nil {
			switch err {
			case ErrAlreadyRunning:
				// App reports this error message to user.
				err = fmt.Errorf("Query Analytics is already running on instance %s."+
					"To reconfigure or restart Query Analytics, stop then start it again.",
					uuid)
				return cmd.Reply(nil, err)
			default:
				return cmd.Reply(nil, err)
			}
		}

		// Write instance config to disk so agent runs instance on restart.
		if err := pct.Basedir.WriteConfig(configName(uuid), setConfig); err != nil {
			return cmd.Reply(nil, err)
		}

		a := m.analyzers[uuid]
		runningConfig := a.analyzer.Config()

		return cmd.Reply(runningConfig) // success
	case "RestartTool":
		setConfig := pc.QAN{}
		if err := json.Unmarshal(cmd.Data, &setConfig); err != nil {
			return cmd.Reply(nil, err)
		}
		uuid := setConfig.UUID
		if err := m.restartAnalyzer(setConfig); err != nil {
			return cmd.Reply(nil, err)
		}

		// Write instance config to disk so agent runs instance on restart.
		if err := pct.Basedir.WriteConfig(configName(uuid), setConfig); err != nil {
			return cmd.Reply(nil, err)
		}

		a := m.analyzers[uuid]
		runningConfig := a.analyzer.Config()

		return cmd.Reply(runningConfig) // success
	case "StopTool":
		errs := []error{}
		uuid := string(cmd.Data)

		if err := m.stopAnalyzer(uuid); err != nil {
			switch err {
			case ErrNotRunning:
				// StopTool is idempotent so this isn't an error, but log it
				// in case user isn't expecting this.
				m.logger.Info("Not running Query Analytics on MySQL", uuid)
			default:
				errs = append(errs, err)
			}
		}

		// Remove instance config from disk so agent doesn't runs instance on restart.
		if err := pct.Basedir.RemoveConfig(configName(uuid)); err != nil {
			errs = append(errs, err)
		}

		// Remove local, cached instance info so if tool is started again we will
		// fetch the latest instance info.
		m.instanceRepo.Remove(uuid)

		return cmd.Reply(nil, errs...)
	case "GetConfig":
		config, errs := m.GetConfig()
		return cmd.Reply(config, errs...)
	default:
		return cmd.Reply(nil, pct.UnknownCmdError{Cmd: cmd.Cmd})
	}
}

func (m *Manager) GetConfig() ([]proto.AgentConfig, []error) {
	m.logger.Debug("GetConfig:call")
	defer m.logger.Debug("GetConfig:return")

	m.mux.RLock()
	defer m.mux.RUnlock()

	// Configs are always returned as array of AgentConfig resources.
	configs := []proto.AgentConfig{}
	for uuid, a := range m.analyzers {
		setConfigBytes, _ := json.Marshal(a.setConfig)
		runConfigBytes, err := json.Marshal(a.analyzer.Config())
		if err != nil {
			m.logger.Warn(err)
		}
		configs = append(configs, proto.AgentConfig{
			Service: pkg,
			UUID:    uuid,
			Set:     string(setConfigBytes),
			Running: string(runConfigBytes),
		})
	}
	return configs, nil
}

func (m *Manager) GetDefaults(uuid string) map[string]interface{} {
	// Check if an analyzer for this instance is running
	if a, exist := m.analyzers[uuid]; exist {
		return a.analyzer.GetDefaults(uuid)
	}

	return map[string]interface{}{}
}

/////////////////////////////////////////////////////////////////////////////
// Implementation
/////////////////////////////////////////////////////////////////////////////
func (m *Manager) restartAnalyzer(setConfig pc.QAN) error {
	// XXX Assume caller has locked m.mux.

	m.logger.Debug("restartAnalyzer:call")
	defer m.logger.Debug("restartAnalyzer:return")

	uuid := setConfig.UUID

	// Check if an analyzer for this instance is running
	_, ok := m.analyzers[uuid]
	if !ok {
		m.logger.Debug("restartAnalyzer:not-running", uuid)
		return ErrNotRunning
	}

	if err := m.stopAnalyzer(uuid); err != nil {
		return err
	}

	return m.startAnalyzer(setConfig)

}

func (m *Manager) startAnalyzer(setConfig pc.QAN) (err error) {
	/*
		XXX Assume caller has locked m.mux.
	*/

	m.logger.Debug("startAnalyzer:call")
	defer m.logger.Debug("startAnalyzer:return")

	uuid := setConfig.UUID

	// Check if an analyzer for this instance already exists.
	if _, ok := m.analyzers[uuid]; ok {
		return ErrAlreadyRunning
	}

	// Get the instance from repo.
	protoInstance, err := m.instanceRepo.Get(uuid, false) // true = cache (write to disk)
	if err != nil {
		return fmt.Errorf("cannot get instance %s: %s", uuid, err)
	}

	analyzerType := protoInstance.Subsystem
	analyzerName := strings.Join(
		[]string{
			pkg,
			"analyzer",
			analyzerType,
			protoInstance.UUID[0:8],
		},
		"-",
	)

	// Create and start a new analyzer. This should return immediately.
	analyzer, err := m.analyzerFactory.Make(
		analyzerType,
		analyzerName,
		protoInstance,
	)
	if err != nil {
		return fmt.Errorf("cannot create analyzer %s: %s", uuid, err)
	}

	// Set the configuration
	analyzer.SetConfig(setConfig)

	if err := analyzer.Start(); err != nil {
		return fmt.Errorf("Cannot start analyzer: %s", err)
	}

	// Save the new analyzer and its associated parts.
	m.analyzers[uuid] = AnalyzerInstance{
		setConfig: setConfig,
		analyzer:  analyzer,
	}

	return nil // success
}

func (m *Manager) stopAnalyzer(uuid string) error {
	/*
		XXX Assume caller has locked m.mux.
	*/

	m.logger.Debug("stopAnalyzer:call")
	defer m.logger.Debug("stopAnalyzer:return")

	a, ok := m.analyzers[uuid]
	if !ok {
		m.logger.Debug("stopAnalyzer:not-running", uuid)
		return ErrNotRunning
	}

	m.status.Update(pkg, fmt.Sprintf("Stopping %s", a.analyzer))
	m.logger.Info(fmt.Sprintf("Stopping %s", a.analyzer))

	// Stop the analyzer. It stops its iter and worker and un-configures MySQL.
	if err := a.analyzer.Stop(); err != nil {
		return err
	}

	// Stop managing this analyzer.
	delete(m.analyzers, uuid)

	return nil // success
}

func configName(uuid string) string {
	return fmt.Sprintf("%s-%s", pkg, uuid)
}
