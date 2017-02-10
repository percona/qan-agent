# Percona Query Analytics Agent

[![Travis CI Build Status](https://travis-ci.org/percona/qan-agent.svg?branch=master)](https://travis-ci.org/percona/qan-agent)
[![GoDoc](https://godoc.org/github.com/percona/qan-agent?status.svg)](https://godoc.org/github.com/percona/qan-agent)
[![Report Card](http://goreportcard.com/badge/percona/qan-agent)](http://goreportcard.com/report/percona/qan-agent)

Percona Query Analytics (QAN) Agent is part of Percona Monitoring and Management (PMM).
See the [PMM docs](https://www.percona.com/doc/percona-monitoring-and-management/index.html) for more information.

##Updating dependencies

Install govendor: `go get -u github.com/kardianos/govendor`  
Fetch dependencies from the original repo (not local copy on GOPATH): `govendor sync`  

##Building
  
In the main dir run:  
`go build -o bin/percona-qan-agent/percona-qan-agent bin/percona-qan-agent/main.go`  
or  
`go build -o bin/percona-qan-agent-installer/percona-qan-agent-installer bin/percona-qan-agent-installer/main.go`  

