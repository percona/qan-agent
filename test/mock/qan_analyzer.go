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

package mock

import (
	"time"

	"github.com/percona/pmm/proto"
	pc "github.com/percona/pmm/proto/config"
	"github.com/percona/qan-agent/qan"
)

type QanAnalyzer struct {
	StartChan          chan bool
	StopChan           chan bool
	ErrorChan          chan error
	CrashChan          chan bool
	config             pc.QAN
	name               string
	ValidateConfigMock func(config pc.QAN) (pc.QAN, error)
	Defaults           map[string]interface{}
}

func NewQanAnalyzer(name string) *QanAnalyzer {
	a := &QanAnalyzer{
		StartChan: make(chan bool, 1),
		StopChan:  make(chan bool, 1),
		ErrorChan: make(chan error, 1),
		CrashChan: make(chan bool, 1),
		config:    pc.QAN{},
		name:      name,
		ValidateConfigMock: func(config pc.QAN) (pc.QAN, error) {
			return config, nil
		},
		Defaults: map[string]interface{}{},
	}
	return a
}

func (a *QanAnalyzer) Start() error {
	a.StartChan <- true
	return a.crashOrError()
}

func (a *QanAnalyzer) Stop() error {
	a.StopChan <- true
	return a.crashOrError()
}

func (a *QanAnalyzer) Status() map[string]string {
	return map[string]string{
		"qan-analyzer": "ok",
	}
}

func (a *QanAnalyzer) String() string {
	return a.name
}

func (a *QanAnalyzer) Config() pc.QAN {
	return a.config
}

func (a *QanAnalyzer) SetConfig(config pc.QAN) {
	a.config = config
}

func (a *QanAnalyzer) ValidateConfig(config pc.QAN) (pc.QAN, error) {
	return a.ValidateConfigMock(config)
}

func (a *QanAnalyzer) GetDefaults(uuid string) map[string]interface{} {
	return a.Defaults
}

// --------------------------------------------------------------------------

func (a *QanAnalyzer) crashOrError() error {
	select {
	case <-a.CrashChan:
		panic("mock.QanAnalyzer crash")
	default:
	}
	select {
	case err := <-a.ErrorChan:
		return err
	default:
	}
	return nil
}

/////////////////////////////////////////////////////////////////////////////
// Factory
/////////////////////////////////////////////////////////////////////////////

type AnalyzerArgs struct {
	Type          string
	Name          string
	ProtoInstance proto.Instance
	RestartChan   chan proto.Instance
	TickChan      chan time.Time
}

type QanAnalyzerFactory struct {
	Args      []AnalyzerArgs
	analyzers []qan.Analyzer
	n         int
}

func NewQanAnalyzerFactory(a ...qan.Analyzer) *QanAnalyzerFactory {
	f := &QanAnalyzerFactory{
		Args:      []AnalyzerArgs{},
		analyzers: a,
	}
	return f
}

func (f *QanAnalyzerFactory) Make(
	analyzerType string,
	analyzerName string,
	protoInstance proto.Instance,
) qan.Analyzer {
	if f.n < len(f.analyzers) {
		a := f.analyzers[f.n]
		args := AnalyzerArgs{
			Type:          analyzerType,
			Name:          analyzerName,
			ProtoInstance: protoInstance,
		}
		f.Args = append(f.Args, args)
		f.n++
		return a
	}
	panic("Need more analyzers")
}
