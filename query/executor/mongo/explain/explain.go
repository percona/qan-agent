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
	"fmt"
	"strings"
	"time"

	"github.com/percona/pmgo"
	"github.com/percona/pmm/proto"
	"gopkg.in/mgo.v2/bson"
)

const (
	MgoTimeoutDialInfo      = 1 * time.Second
	MgoTimeoutSessionSync   = 1 * time.Second
	MgoTimeoutSessionSocket = 1 * time.Second
)

type DecodeQueryError struct{ *MongoExplainError }
type DecodeNamespaceError struct{ *MongoExplainError }

func Explain(dsn, namespace, query string) (*proto.ExplainResult, error) {
	// if dsn is incorrect we should exit immediately as this is not gonna correct itself
	dialInfo, err := pmgo.ParseURL(dsn)
	if err != nil {
		return nil, err
	}
	dialInfo.Timeout = MgoTimeoutDialInfo
	dialer := pmgo.NewDialer()

	session, err := dialer.DialWithInfo(dialInfo)
	if err != nil {
		return nil, err
	}
	defer session.Close()
	session.SetSyncTimeout(MgoTimeoutSessionSync)
	session.SetSocketTimeout(MgoTimeoutSessionSocket)

	q := bson.M{}
	err = bson.UnmarshalJSON([]byte(query), &q)
	if err != nil {
		return nil, &DecodeQueryError{&MongoExplainError{err, fmt.Sprintf("unable to decode query %s", query)}}
	}

	s := strings.Split(namespace, ".")
	if len(s) != 2 {
		return nil, &DecodeNamespaceError{&MongoExplainError{nil, fmt.Sprintf("unable to decode db and collection from namespace %s", namespace)}}
	}
	db, collection := s[0], s[1]

	result := bson.M{}
	err = session.DB(db).C(collection).Find(q).Explain(&result)
	if err != nil {
		return nil, err
	}

	resultJson, err := bson.MarshalJSON(result)
	if err != nil {
		return nil, err
	}

	explain := &proto.ExplainResult{
		JSON: string(resultJson),
	}
	return explain, nil
}
