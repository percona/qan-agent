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
	"time"

	"github.com/percona/percona-toolkit/src/go/mongolib/explain"
	"github.com/percona/pmgo"
	"github.com/percona/pmm/proto"
)

const (
	MgoTimeoutDialInfo      = 5 * time.Second
	MgoTimeoutSessionSync   = 5 * time.Second
	MgoTimeoutSessionSocket = 5 * time.Second
)

func Explain(dsn, db, query string) (*proto.ExplainResult, error) {
	// if dsn is incorrect we should exit immediately as this is not gonna correct itself
	dialInfo, err := pmgo.ParseURL(dsn)
	if err != nil {
		return nil, err
	}
	dialer := pmgo.NewDialer()

	dialInfo.Timeout = MgoTimeoutDialInfo
	session, err := dialer.DialWithInfo(dialInfo)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	session.SetSyncTimeout(MgoTimeoutSessionSync)
	session.SetSocketTimeout(MgoTimeoutSessionSocket)

	ex := explain.New(session)
	resultJson, err := ex.Explain(db, []byte(query))
	if err != nil {
		return nil, err
	}

	explainResult := &proto.ExplainResult{
		JSON: string(resultJson),
	}
	return explainResult, nil
}
