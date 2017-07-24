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
	"database/sql"
	"errors"
	"time"

	"github.com/percona/pmm/proto"
	"github.com/percona/qan-agent/mysql"
	"sync"
)

var ERR_NOT_FOUND = errors.New("var not found in NullMySQL mock")

type NullMySQL struct {
	set                  []mysql.Query
	SetCond              *sync.Cond
	exec                 []string
	explain              map[string]*proto.ExplainResult
	uptime               int64
	uptimeCount          uint
	boolVars             map[string]sql.NullBool
	stringVars           map[string]sql.NullString
	numericVars          map[string]sql.NullFloat64
	integerVars          map[string]sql.NullInt64
	atLeastVersion       bool
	atLeastVersionErr    error
	Version              string
	CurrentTzOffsetHours int
	SystemTzOffsetHours  int
}

func NewNullMySQL() *NullMySQL {
	n := &NullMySQL{
		set:                  []mysql.Query{},
		exec:                 []string{},
		explain:              make(map[string]*proto.ExplainResult),
		SetCond:              sync.NewCond(&sync.Mutex{}),
		CurrentTzOffsetHours: 6,
		SystemTzOffsetHours:  0,
	}
	return n
}

func (n *NullMySQL) DB() *sql.DB {
	return nil
}

func (n *NullMySQL) DSN() string {
	return "user:pass@tcp(127.0.0.1:3306)/?parseTime=true"
}

func (n *NullMySQL) Connect() error {
	return nil
}

func (n *NullMySQL) Close() {
	return
}

func (n *NullMySQL) Explain(db, query string, convert bool) (*proto.ExplainResult, error) {
	return nil, nil
}

func (n *NullMySQL) TableInfo(tables *proto.TableInfoQuery) (proto.TableInfoResult, error) {
	return proto.TableInfoResult{}, nil
}

func (n *NullMySQL) Set(queries []mysql.Query) error {
	for _, q := range queries {
		n.set = append(n.set, q)
	}

	// broadcast an event
	n.SetCond.L.Lock()
	defer n.SetCond.L.Unlock()
	n.SetCond.Broadcast()
	return nil
}

func (n *NullMySQL) Exec(queries []string) error {
	for _, q := range queries {
		n.exec = append(n.exec, q)
	}

	// broadcast an event
	n.SetCond.L.Lock()
	defer n.SetCond.L.Unlock()
	n.SetCond.Broadcast()
	return nil
}

func (n *NullMySQL) GetSet() []mysql.Query {
	return n.set
}

func (n *NullMySQL) GetExec() []string {
	return n.exec
}

func (n *NullMySQL) Reset() {
	n.set = []mysql.Query{}
	n.exec = []string{}
	n.boolVars = make(map[string]sql.NullBool)
	n.stringVars = make(map[string]sql.NullString)
	n.numericVars = make(map[string]sql.NullFloat64)
	n.integerVars = make(map[string]sql.NullInt64)
}

func (n *NullMySQL) GetGlobalVarBoolean(varName string) (varValue sql.NullBool, err error) {
	varValue, ok := n.boolVars[varName]
	if ok {
		return varValue, nil
	}
	return varValue, ERR_NOT_FOUND
}

func (n *NullMySQL) GetGlobalVarString(varName string) (varValue sql.NullString, err error) {
	varValue, ok := n.stringVars[varName]
	if ok {
		return varValue, nil
	}
	return varValue, ERR_NOT_FOUND
}

func (n *NullMySQL) GetGlobalVarNumeric(varName string) (varValue sql.NullFloat64, err error) {
	varValue, ok := n.numericVars[varName]
	if ok {
		return varValue, nil
	}
	return varValue, ERR_NOT_FOUND
}

func (n *NullMySQL) GetGlobalVarInteger(varName string) (varValue sql.NullInt64, err error) {
	varValue, ok := n.integerVars[varName]
	if ok {
		return varValue, nil
	}
	return varValue, ERR_NOT_FOUND
}

func (n *NullMySQL) SetGlobalVarNumeric(name string, value float64) {
	n.numericVars[name] = sql.NullFloat64{
		Float64: value,
	}
}

func (n *NullMySQL) SetGlobalVarInteger(name string, value int64) {
	n.integerVars[name] = sql.NullInt64{
		Int64: value,
	}
}

func (n *NullMySQL) SetGlobalVarString(name, value string) {
	n.stringVars[name] = sql.NullString{
		String: value,
	}
}

func (n *NullMySQL) Uptime() (int64, error) {
	n.uptimeCount++
	return n.uptime, nil
}

func (n *NullMySQL) AtLeastVersion(v string) (bool, error) {
	n.Version = v
	return n.atLeastVersion, n.atLeastVersionErr
}

func (n *NullMySQL) SetAtLeastVersion(atLeastVersion bool, err error) {
	n.atLeastVersion = atLeastVersion
	n.atLeastVersionErr = err
}

func (n *NullMySQL) GetUptimeCount() uint {
	return n.uptimeCount
}

func (n *NullMySQL) SetUptime(uptime int64) {
	n.uptime = uptime
}

func (n *NullMySQL) UTCOffset() (time.Duration, time.Duration, error) {
	return time.Duration(n.CurrentTzOffsetHours) * time.Hour, time.Duration(n.SystemTzOffsetHours) * time.Hour, nil
}
