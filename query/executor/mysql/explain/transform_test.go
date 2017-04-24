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

package explain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// --------------------------------------------------------------------------

func TestDMLToSelect(t *testing.T) {
	q := dmlToSelect(`update ignore tabla set nombre = "carlos" where id = 0 limit 2`)
	assert.Equal(t, `SELECT nombre = "carlos" FROM tabla WHERE id = 0`, q)

	q = dmlToSelect(`update ignore tabla set nombre = "carlos" where id = 0`)
	assert.Equal(t, `SELECT nombre = "carlos" FROM tabla WHERE id = 0`, q)

	q = dmlToSelect(`update ignore tabla set nombre = "carlos" limit 1`)
	assert.Equal(t, `SELECT nombre = "carlos" FROM tabla`, q)

	q = dmlToSelect(`update tabla set nombre = "carlos" where id = 0 limit 2`)
	assert.Equal(t, `SELECT nombre = "carlos" FROM tabla WHERE id = 0`, q)

	q = dmlToSelect(`update tabla set nombre = "carlos" where id = 0`)
	assert.Equal(t, `SELECT nombre = "carlos" FROM tabla WHERE id = 0`, q)

	q = dmlToSelect(`update tabla set nombre = "carlos" limit 1`)
	assert.Equal(t, `SELECT nombre = "carlos" FROM tabla`, q)

	q = dmlToSelect(`delete from tabla`)
	assert.Equal(t, `SELECT * FROM tabla`, q)

	q = dmlToSelect(`delete from tabla join tabla2 on tabla.id = tabla2.tabla2_id`)
	assert.Equal(t, `SELECT 1 FROM tabla join tabla2 on tabla.id = tabla2.tabla2_id`, q)

	q = dmlToSelect(`insert into tabla (f1, f2, f3) values (1,2,3)`)
	assert.Equal(t, `SELECT * FROM tabla  WHERE f1=1 and f2=2 and f3=3`, q)

	q = dmlToSelect(`insert into tabla (f1, f2, f3) values (1,2)`)
	assert.Equal(t, `SELECT * FROM tabla  LIMIT 1`, q)

	q = dmlToSelect(`insert into tabla set f1="A1", f2="A2"`)
	assert.Equal(t, `SELECT * FROM tabla WHERE f1="A1" AND  f2="A2"`, q)

	q = dmlToSelect(`replace into tabla set f1="A1", f2="A2"`)
	assert.Equal(t, `SELECT * FROM tabla WHERE f1="A1" AND  f2="A2"`, q)

	q = dmlToSelect("insert into `tabla-1` values(12)")
	assert.Equal(t, "SELECT * FROM `tabla-1` LIMIT 1", q)
}
