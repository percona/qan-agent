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

package tableinfo

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/percona/pmm/proto"
	"github.com/percona/qan-agent/mysql"
)

func TableInfo(c mysql.Connector, tables *proto.TableInfoQuery) (proto.TableInfoResult, error) {
	res := make(proto.TableInfoResult)

	if len(tables.Create) > 0 {
		for _, t := range tables.Create {
			dbTable := t.Db + "." + t.Table
			tableInfo, ok := res[dbTable]
			if !ok {
				res[dbTable] = &proto.TableInfo{}
				tableInfo = res[dbTable]
			}

			db := escapeString(t.Db)
			table := escapeString(t.Table)
			def, err := showCreate(c, ident(db, table))
			if err != nil {
				if tableInfo.Errors == nil {
					tableInfo.Errors = []string{}
				}
				tableInfo.Errors = append(tableInfo.Errors, fmt.Sprintf("SHOW CREATE TABLE %s: %s", t.Table, err))
				continue
			}
			tableInfo.Create = def
		}
	}

	if len(tables.Index) > 0 {
		for _, t := range tables.Index {
			dbTable := t.Db + "." + t.Table
			tableInfo, ok := res[dbTable]
			if !ok {
				res[dbTable] = &proto.TableInfo{}
				tableInfo = res[dbTable]
			}

			db := escapeString(t.Db)
			table := escapeString(t.Table)
			indexes, err := showIndex(c, ident(db, table))
			if err != nil {
				if tableInfo.Errors == nil {
					tableInfo.Errors = []string{}
				}
				tableInfo.Errors = append(tableInfo.Errors, fmt.Sprintf("SHOW INDEX FROM %s.%s: %s", t.Db, t.Table, err))
				continue
			}
			tableInfo.Index = indexes
		}
	}

	if len(tables.Status) > 0 {
		for _, t := range tables.Status {
			dbTable := t.Db + "." + t.Table
			tableInfo, ok := res[dbTable]
			if !ok {
				res[dbTable] = &proto.TableInfo{}
				tableInfo = res[dbTable]
			}

			// SHOW TABLE STATUS does not accept db.tbl so pass them separately.
			db := escapeString(t.Db)
			table := escapeString(t.Table)
			status, err := showStatus(c, ident(db, ""), table)
			if err != nil {
				if tableInfo.Errors == nil {
					tableInfo.Errors = []string{}
				}
				tableInfo.Errors = append(tableInfo.Errors, fmt.Sprintf("SHOW TABLE STATUS FROM %s WHERE Name='%s': %s", t.Db, t.Table, err))
				continue
			}
			tableInfo.Status = status
		}
	}

	return res, nil
}

// --------------------------------------------------------------------------

func showCreate(c mysql.Connector, dbTable string) (string, error) {
	// Result from SHOW CREATE TABLE includes two columns, "Table" and
	// "Create Table", we ignore the first one as we need only "Create Table".
	var tableName string
	var tableDef string
	err := c.DB().QueryRow("SHOW CREATE TABLE "+dbTable).Scan(&tableName, &tableDef)
	if err == sql.ErrNoRows {
		err = fmt.Errorf("table %s doesn't exist ", dbTable)
	}
	return tableDef, err
}

func showIndex(c mysql.Connector, dbTable string) (map[string][]proto.ShowIndexRow, error) {
	rows, err := c.DB().Query("SHOW INDEX FROM " + dbTable)
	if err != nil && err != sql.ErrNoRows {
		return nil, err
	}
	defer rows.Close()
	if err == sql.ErrNoRows {
		err = fmt.Errorf("table %s doesn't exist", dbTable)
		return nil, err
	}

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	indexes := map[string][]proto.ShowIndexRow{} // keyed on KeyName
	prevKeyName := ""
	for rows.Next() {
		indexRow := proto.ShowIndexRow{}
		dest := []interface{}{
			&indexRow.Table,
			&indexRow.NonUnique,
			&indexRow.KeyName,
			&indexRow.SeqInIndex,
			&indexRow.ColumnName,
			&indexRow.Collation,
			&indexRow.Cardinality,
			&indexRow.SubPart,
			&indexRow.Packed,
			&indexRow.Null,
			&indexRow.IndexType,
			&indexRow.Comment,
			&indexRow.IndexComment,
			&indexRow.Visible,
		}

		// Cut dest to number of columns.
		// Some columns are not available at earlier versions of MySQL.
		if len(columns) < len(dest) {
			dest = dest[:len(columns)]
		}

		err := rows.Scan(dest...)
		if err != nil {
			return nil, err
		}
		if indexRow.KeyName != prevKeyName {
			indexes[indexRow.KeyName] = []proto.ShowIndexRow{}
			prevKeyName = indexRow.KeyName
		}
		indexes[indexRow.KeyName] = append(indexes[indexRow.KeyName], indexRow)
	}
	err = rows.Err()
	if err != nil {
		return nil, err
	}

	return indexes, nil
}

func showStatus(c mysql.Connector, db, table string) (*proto.ShowTableStatus, error) {
	status := &proto.ShowTableStatus{}
	err := c.DB().QueryRow(fmt.Sprintf("SHOW TABLE STATUS FROM %s WHERE Name='%s'", db, table)).Scan(
		&status.Name,
		&status.Engine,
		&status.Version,
		&status.RowFormat,
		&status.Rows,
		&status.AvgRowLength,
		&status.DataLength,
		&status.MaxDataLength,
		&status.IndexLength,
		&status.DataFree,
		&status.AutoIncrement,
		&status.CreateTime,
		&status.UpdateTime,
		&status.CheckTime,
		&status.Collation,
		&status.Checksum,
		&status.CreateOptions,
		&status.Comment,
	)
	if err == sql.ErrNoRows {
		err = fmt.Errorf("table %s.%s doesn't exist", db, table)
	}
	return status, err
}

func ident(db, table string) string {
	// Wrap the idents in ` to handle space and weird chars.
	if db != "" {
		db = "`" + db + "`"
	}
	if table != "" {
		table = "`" + table + "`"
	}
	// Join the idents if there's two, else return whichever was given.
	if db != "" && table != "" {
		return db + "." + table
	} else if table != "" {
		return table
	} else {
		return db
	}
}

func escapeString(v string) string {
	return strings.NewReplacer(
		"\x00", "\\0",
		"\n", "\\n",
		"\r", "\\r",
		"\x1a", "\\Z",
		"'", "\\'",
		"\"", "\\\"",
		"\\", "\\\\",
	).Replace(v)
}
