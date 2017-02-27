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

package interval_iter

import (
	"time"

	"github.com/percona/qan-agent/pct"
	"github.com/percona/qan-agent/qan/analyzer/mysql/iter"
	"github.com/percona/qan-agent/qan/analyzer/mysql/worker/slowlog"
)

type IntervalIterFactory struct {
	Iters     []iter.IntervalIter
	iterNo    int
	TickChans map[iter.IntervalIter]chan time.Time
}

func (tf *IntervalIterFactory) Make(collectFrom string, filename slowlog.FilenameFunc, tickChan chan time.Time) iter.IntervalIter {
	if tf.iterNo >= len(tf.Iters) {
		return tf.Iters[tf.iterNo-1]
	}
	nextIter := tf.Iters[tf.iterNo]
	tf.TickChans[nextIter] = tickChan
	tf.iterNo++
	return nextIter
}

func (tf *IntervalIterFactory) Reset() {
	tf.iterNo = 0
}

// --------------------------------------------------------------------------

type Iter struct {
	testIntervalChan chan *iter.Interval
	intervalChan     chan *iter.Interval
	sync             *pct.SyncChan
	tickChan         chan time.Time
	calls            []string
}

func NewIter(intervalChan chan *iter.Interval) *Iter {
	iter := &Iter{
		testIntervalChan: intervalChan,
		// --
		intervalChan: make(chan *iter.Interval, 1),
		sync:         pct.NewSyncChan(),
		tickChan:     make(chan time.Time),
		calls:        []string{},
	}
	return iter
}

func (i *Iter) Start() {
	i.calls = append(i.calls, "Start")
	go i.run()
}

func (i *Iter) Stop() {
	i.calls = append(i.calls, "Stop")
	i.sync.Stop()
	i.sync.Wait()
}

func (i *Iter) IntervalChan() chan *iter.Interval {
	return i.intervalChan
}

func (i *Iter) TickChan() chan time.Time {
	return i.tickChan
}

func (i *Iter) run() {
	defer func() {
		i.sync.Done()
	}()
	for {
		select {
		case <-i.sync.StopChan:
			return
		case interval := <-i.testIntervalChan:
			i.intervalChan <- interval
		}
	}
}

func (i *Iter) Calls() []string {
	return i.calls
}

func (i *Iter) Reset() {
	i.calls = []string{}
}
