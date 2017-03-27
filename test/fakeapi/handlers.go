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

package fakeapi

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"net/http"

	"github.com/percona/pmm/proto"
)

func (f *FakeApi) AppendPing() {
	f.Append("/ping", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(600)
		}
	})
}

func (f *FakeApi) AppendInstancesId(id uint, protoInstance *proto.Instance) {
	f.Append(fmt.Sprintf("/instances/%d/", id), func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		data, _ := json.Marshal(&protoInstance)
		w.Write(data)
	})
}

func (f *FakeApi) AppendInstances(protoInstances []*proto.Instance) {
	instances := map[string]*proto.Instance{}
	for i := range protoInstances {
		instances[protoInstances[i].Subsystem] = protoInstances[i]
	}
	f.Append("/instances/", func(w http.ResponseWriter, r *http.Request) {
		gotInstance := proto.Instance{}
		body, err := ioutil.ReadAll(r.Body)
		if err != nil {
			panic(err)
		}
		err = json.Unmarshal(body, &gotInstance)
		if err != nil {
			panic(err)
		}
		if _, ok := instances[gotInstance.Subsystem]; !ok {
			panic(fmt.Sprintf("Not registered instance %s", gotInstance))
		}

		gotInstance.Id = instances[gotInstance.Subsystem].Id
		w.Header().Set("Location", fmt.Sprintf("%s/instances/%d", f.URL(), gotInstance.Id))
		w.WriteHeader(http.StatusCreated)
		f.AppendInstancesId(gotInstance.Id, &gotInstance)
		*instances[gotInstance.Subsystem] = gotInstance
	})
}
