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

package main

import (
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"

	"github.com/percona/pmm/proto"
	pc "github.com/percona/pmm/proto/config"
	"github.com/percona/qan-agent/bin/percona-qan-agent-installer/installer"
	"github.com/percona/qan-agent/instance"
	"github.com/percona/qan-agent/pct"
)

var (
	flagBasedir string
	flagDebug   bool
	// @todo remove this flag, it's currently used by pmm as mysql=false
	flagMySQL bool

	flagServerUser     string
	flagServerPass     string
	flagUseSSL         bool
	flagUseInsecureSSL bool
)

var fs *flag.FlagSet

func init() {
	log.SetFlags(log.Ldate | log.Ltime | log.Lmicroseconds | log.Lshortfile)

	fs = flag.NewFlagSet(os.Args[0], flag.ExitOnError)

	fs.BoolVar(&flagMySQL, "mysql", true, "Create MySQL instance")
	fs.BoolVar(&flagDebug, "debug", false, "Debug")
	fs.StringVar(&flagBasedir, "basedir", pct.DEFAULT_BASEDIR, "Agent basedir")

	fs.StringVar(&flagServerUser, "server-user", "pmm", "Username to use for API auth")
	fs.StringVar(&flagServerPass, "server-pass", "", "Password to use for API auth")
	fs.BoolVar(&flagUseSSL, "use-ssl", false, "Use ssl to connect to the API")
	fs.BoolVar(&flagUseInsecureSSL, "use-insecure-ssl", false, "Use self signed certs when connecting to the API")

}

func main() {
	// It flag is unknown it exist with os.Exit(10),
	// so exit code=10 is strictly reserved for flags
	// Don't use it anywhere else, as shell script install.sh depends on it
	// NOTE: standard flag.Parse() was using os.Exit(2)
	//       which was the same as returned with ctrl+c
	if err := fs.Parse(os.Args[1:]); err != nil {
		if err == flag.ErrHelp {
			return
		}
		log.Fatal(err)
	}

	args := fs.Args()
	if len(args) != 1 {
		fs.PrintDefaults()
		fmt.Printf("Got %d args, expected 1: API_HOST[:PORT]\n", len(args))
		fmt.Fprintf(os.Stderr, "Usage: %s [options] API_HOST[:PORT]\n", os.Args[0])
		os.Exit(1)
	}

	qanAPIURL, err := parseURLParam(args[0])
	if err != nil {
		log.Fatal("expected arg in the form [schema://]host[:port][path]")
	}

	if qanAPIURL.Scheme == "https" {
		flagUseSSL = true
	}
	agentConfig := &pc.Agent{
		ApiHostname:       qanAPIURL.Host,
		ApiPath:           qanAPIURL.Path,
		ServerUser:        flagServerUser,
		ServerPassword:    flagServerPass,
		ServerSSL:         flagUseSSL,
		ServerInsecureSSL: flagUseInsecureSSL,
	}

	flags := installer.Flags{
		Bool: map[string]bool{
			"debug": flagDebug,
		},
	}

	fmt.Println("CTRL-C at any time to quit")

	api := pct.NewAPI(flagServerUser, flagServerPass, flagUseSSL, flagUseInsecureSSL)
	requestURL := qanAPIURL.String()
	fmt.Printf("API host: %s\n", requestURL)

	if _, err := api.Init(requestURL, nil); err != nil {
		fmt.Printf("Cannot connect to API %s: %s\n", requestURL, err)
		os.Exit(1)
	}

	// Agent stores all its files in the basedir.  This must be called first
	// because installer uses pct.Basedir and assumes it's already initialized.
	if err := pct.Basedir.Init(flagBasedir); err != nil {
		log.Printf("Error initializing basedir %s: %s\n", flagBasedir, err)
		os.Exit(1)
	}

	logChan := make(chan proto.LogEntry, 100)
	logger := pct.NewLogger(logChan, "instance-repo")
	instanceRepo := instance.NewRepo(logger, pct.Basedir.Dir("config"), api)
	agentInstaller, err := installer.NewInstaller(flagBasedir, api, instanceRepo, agentConfig, flags)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// todo: catch SIGINT and clean up
	if err := agentInstaller.Run(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	os.Exit(0)
}

func parseURLParam(args0 string) (*url.URL, error) {
	if !strings.HasPrefix(args0, "http://") && !strings.HasPrefix(args0, "https://") {
		args0 = "http://" + args0
	}

	qanAPIURL, err := url.Parse(args0)
	if err != nil {
		return nil, fmt.Errorf("expected arg in the form [schema://]host[:port][path]")
	}
	return qanAPIURL, nil
}
