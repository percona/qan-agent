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
	"github.com/percona/pmm/proto"
	pc "github.com/percona/pmm/proto/config"
)

// AnalyzerFactory makes an Analyzer, real or mock.
type AnalyzerFactory interface {
	Make(
		analyzerType string,
		analyzerName string,
		protoInstance proto.Instance,
	) (Analyzer, error)
}

// Analyzer is a daemon that collects QAN data
type Analyzer interface {
	// Start starts analyzer but doesn't wait until it exits
	Start() error
	// Status returns list of statuses
	Status() map[string]string
	// Stop stops running analyzer, waits until it stops
	Stop() error
	// Config returns analyzer configuration
	Config() pc.QAN
	// SetConfig sets configuration of analyzer
	SetConfig(setConfig pc.QAN)
	// Get default configuration
	GetDefaults(uuid string) map[string]interface{}
	// String returns human readable identification of Analyzer
	String() string
}
