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

package perfschema

// NewDigests returns ready to use *Digests
func NewDigests() *Digests {
	d := &Digests{}
	d.Reset()
	return d
}

// Digests represents digests retrieved from performance_schema
type Digests struct {
	// All digests collected from performance_schema since creation of Digests or Reset()
	All Snapshot
	// Curr digests collected from performance_schema
	Curr Snapshot
}

// MergeCurr merges current snapshot into all collected digests so far
func (d *Digests) MergeCurr() {
	for i := range d.Curr {
		if _, ok := d.All[i]; !ok {
			d.All[i] = d.Curr[i]
			continue
		}

		for j := range d.Curr[i].Rows {
			d.All[i].Rows[j] = d.Curr[i].Rows[j]
		}
	}
}

// Reset drops all collected data
func (d *Digests) Reset() {
	d.All = Snapshot{}
	d.All = Snapshot{}
}
