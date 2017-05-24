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
	"database/sql"
	"fmt"
	"strings"

	"github.com/percona/pmm/proto"
	"github.com/percona/qan-agent/mysql"
)

func Explain(c mysql.Connector, db, query string, convert bool) (*proto.ExplainResult, error) {
	if db != "" && !strings.HasPrefix(db, "`") {
		db = "`" + db + "`"
	}
	explainResult, err := explain(c, db, query)
	if err != nil {
		// MySQL 5.5 returns syntax error because it doesn't support non-SELECT EXPLAIN.
		// MySQL 5.6 non-SELECT EXPLAIN requires privs for the SQL statement.
		errCode := mysql.MySQLErrorCode(err)
		if convert && (errCode == mysql.ER_SYNTAX_ERROR || errCode == mysql.ER_USER_DENIED) && isDMLQuery(query) {
			query = dmlToSelect(query)
			if query == "" {
				return nil, fmt.Errorf("cannot convert query to SELECT")
			}
			explainResult, err = explain(c, db, query) // query converted to SELECT
		}
		if err != nil {
			return nil, err
		}
	}
	return explainResult, nil
}

// --------------------------------------------------------------------------

func explain(c mysql.Connector, db, query string) (*proto.ExplainResult, error) {
	// Transaction because we need to ensure USE and EXPLAIN are run in one connection
	tx, err := c.DB().Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	// If the query has a default db, use it; else, all tables need to be db-qualified
	// or EXPLAIN will throw an error.
	if db != "" {
		_, err := tx.Exec(fmt.Sprintf("USE %s", db))
		if err != nil {
			return nil, err
		}
	}

	classicExplain, err := classicExplain(c, tx, query)
	if err != nil {
		return nil, err
	}

	jsonExplain, err := jsonExplain(c, tx, query)
	if err != nil {
		return nil, err
	}

	explain := &proto.ExplainResult{
		Classic: classicExplain,
		JSON:    jsonExplain,
	}

	return explain, nil
}

func classicExplain(c mysql.Connector, tx *sql.Tx, query string) (classicExplain []*proto.ExplainRow, err error) {
	// Partitions are introduced since MySQL 5.1
	// We can simply run EXPLAIN /*!50100 PARTITIONS*/ to get this column when it's available
	// without prior check for MySQL version.
	if strings.TrimSpace(query) == "" {
		return nil, fmt.Errorf("cannot run EXPLAIN on an empty query example")
	}
	rows, err := tx.Query(fmt.Sprintf("EXPLAIN %s", query))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Go rows.Scan() expects exact number of columns
	// so when number of columns is undefined then the easiest way to
	// overcome this problem is to count received number of columns
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	nCols := len(columns)

	for rows.Next() {
		explainRow := &proto.ExplainRow{}
		switch nCols {
		case 10:
			err = rows.Scan(
				&explainRow.Id,
				&explainRow.SelectType,
				&explainRow.Table,
				&explainRow.Type,
				&explainRow.PossibleKeys,
				&explainRow.Key,
				&explainRow.KeyLen,
				&explainRow.Ref,
				&explainRow.Rows,
				&explainRow.Extra,
			)
		case 11: // MySQL 5.1 with "partitions"
			err = rows.Scan(
				&explainRow.Id,
				&explainRow.SelectType,
				&explainRow.Table,
				&explainRow.Partitions, // here
				&explainRow.Type,
				&explainRow.PossibleKeys,
				&explainRow.Key,
				&explainRow.KeyLen,
				&explainRow.Ref,
				&explainRow.Rows,
				&explainRow.Extra,
			)
		case 12: // MySQL 5.7 with "filtered"
			err = rows.Scan(
				&explainRow.Id,
				&explainRow.SelectType,
				&explainRow.Table,
				&explainRow.Partitions,
				&explainRow.Type,
				&explainRow.PossibleKeys,
				&explainRow.Key,
				&explainRow.KeyLen,
				&explainRow.Ref,
				&explainRow.Rows,
				&explainRow.Filtered, // here
				&explainRow.Extra,
			)
		}
		if err != nil {
			return nil, err
		}
		classicExplain = append(classicExplain, explainRow)
	}
	err = rows.Err()
	if err != nil {
		return nil, err
	}

	return classicExplain, nil
}

func jsonExplain(c mysql.Connector, tx *sql.Tx, query string) (string, error) {
	// EXPLAIN in JSON format is introduced since MySQL 5.6.5
	ok, err := c.AtLeastVersion("5.6.5")
	if !ok || err != nil {
		return "", err
	}

	explain := ""
	err = tx.QueryRow(fmt.Sprintf("EXPLAIN FORMAT=JSON %s", query)).Scan(&explain)
	if err != nil {
		return "", err
	}

	return explain, nil
}
