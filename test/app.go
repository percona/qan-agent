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

package test

import (
	"encoding/json"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/percona/pmm/proto"
)

func FileExists(file string) bool {
	_, err := os.Stat(file)
	if err == nil {
		return true
	}
	if os.IsNotExist(err) {
		return false
	}
	return true
}

func GetStatus(sendChan chan *proto.Cmd, recvChan chan *proto.Reply) map[string]string {
	statusCmd := &proto.Cmd{
		Ts:   time.Now(),
		User: "user",
		Cmd:  "Status",
	}
	sendChan <- statusCmd

	select {
	case reply := <-recvChan:
		status := make(map[string]string)
		if err := json.Unmarshal(reply.Data, &status); err != nil {
			// This shouldn't happen.
			log.Fatal(err)
		}
		return status
	case <-time.After(100 * time.Millisecond):
	}

	return map[string]string{}
}

func DrainLogChan(c chan proto.LogEntry) {
	for {
		select {
		case <-c:
		default:
			return
		}
	}
}

func DrainSendChan(c chan *proto.Cmd) {
	for {
		select {
		case <-c:
		default:
			return
		}
	}
}

func DrainRecvChan(c chan *proto.Reply) {
	for {
		select {
		case <-c:
		default:
			return
		}
	}
}

func DrainTraceChan(c chan string) []string {
	trace := []string{}
DRAIN:
	for {
		select {
		case funcCalled := <-c:
			trace = append(trace, funcCalled)
		default:
			break DRAIN
		}
	}
	return trace
}

func DrainBoolChan(c chan bool) {
	for {
		select {
		case <-c:
		default:
			return
		}
	}
}

func DrainRecvData(c chan interface{}) {
	for {
		select {
		case <-c:
		default:
			return
		}
	}
}

func DrainDataChan(c chan []byte) {
	for {
		select {
		case <-c:
		default:
			return
		}
	}
}

func CopyFile(src, dst string) error {
	cmd := exec.Command("cp", src, dst)
	return cmd.Run()
}

func ClearDir(path ...string) error {
	dir := filepath.Join(path...)
	files, err := filepath.Glob(dir)
	if err != nil {
		return err
	}
	for _, file := range files {
		if err := os.Remove(file); err != nil {
			return err
		}
	}
	return nil
}

func LoadReport(file string, v interface{}, got interface{}) error {
	updateTestData := os.Getenv("UPDATE_TEST_DATA")
	if updateTestData != "" {
		data, _ := json.MarshalIndent(&got, "", "  ")
		ioutil.WriteFile(file, data, 0666)

	}
	bytes, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}
	if err = json.Unmarshal(bytes, v); err != nil {
		return err
	}
	return nil
}
