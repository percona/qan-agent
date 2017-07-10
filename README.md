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


## Submitting Bug Reports

If you find a bug in Percona QAN Agent or one of the related projects, you should submit a report to that project's [JIRA](https://jira.percona.com) issue tracker.

Your first step should be [to search](https://jira.percona.com/issues/?jql=project%20%3D%20PMM%20AND%20component%20%3D%20%22QAN%20Agent%22) the existing set of open tickets for a similar report. If you find that someone else has already reported your problem, then you can upvote that report to increase its visibility.

If there is no existing report, submit a report following these steps:

1. [Sign in to Percona JIRA.](https://jira.percona.com/login.jsp) You will need to create an account if you do not have one.
2. [Go to the Create Issue screen and select the relevant project.](https://jira.percona.com/secure/CreateIssueDetails!init.jspa?pid=11600&issuetype=1&priority=3&components=11309)
3. Fill in the fields of Summary, Description, Steps To Reproduce, and Affects Version to the best you can. If the bug corresponds to a crash, attach the stack trace from the logs.

An excellent resource is [Elika Etemad's article on filing good bug reports.](http://fantasai.inkedblade.net/style/talks/filing-good-bugs/).

As a general rule of thumb, please try to create bug reports that are:

- *Reproducible.* Include steps to reproduce the problem.
- *Specific.* Include as much detail as possible: which version, what environment, etc.
- *Unique.* Do not duplicate existing tickets.
- *Scoped to a Single Bug.* One bug per report.
