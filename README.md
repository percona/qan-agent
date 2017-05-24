# Percona Query Analytics Agent

[![Travis CI Build Status](https://travis-ci.org/percona/qan-agent.svg?branch=master)](https://travis-ci.org/percona/qan-agent)
[![GoDoc](https://godoc.org/github.com/percona/qan-agent?status.svg)](https://godoc.org/github.com/percona/qan-agent)
[![Report Card](http://goreportcard.com/badge/percona/qan-agent)](http://goreportcard.com/report/percona/qan-agent)

Percona Query Analytics (QAN) Agent is part of Percona Monitoring and Management (PMM).
See the [PMM docs](https://www.percona.com/doc/percona-monitoring-and-management/index.html) for more information.


## Building

1. Setup [`GOPATH`](https://golang.org/doc/code.html#GOPATH).
2. Clone repository to `GOPATH`: `go get -v github.com/percona/qan-agent`.
3. Install [`glide`](https://github.com/Masterminds/glide#install):
   * `curl https://glide.sh/get | sh` or
   * `brew install glide` or
   * `go get -u github.com/Masterminds/glide` for development version (`master` branch).
4. Fetch dependencies: `glide install`.
5. Install agent and installer: `go install -v github.com/percona/qan-agent/bin/...`. Binaries will be created in `$GOPATH/bin`.
