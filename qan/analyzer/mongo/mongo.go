package mysql

import (
	"context"

	"github.com/percona/pmm/proto"
	pc "github.com/percona/pmm/proto/config"
	"github.com/percona/qan-agent/qan"
)

func New(ctx context.Context, protoInstance proto.Instance) qan.Analyzer {
	// return initialized MongoAnalyzer
	return &MongoAnalyzer{}
}

// MongoAnalyzer
type MongoAnalyzer struct {
}

// SetConfig sets the config
func (m *MongoAnalyzer) SetConfig(setConfig pc.QAN) {
}

// Config returns analyzer running configuration
func (m *MongoAnalyzer) Config() pc.QAN {
	return pc.QAN{}
}

// Start starts analyzer but doesn't wait until it exits
func (m *MongoAnalyzer) Start() error {
	return nil
}

// Status returns list of statuses
func (m *MongoAnalyzer) Status() map[string]string {
	return map[string]string{}
}

// Stop stops running analyzer, waits until it stops
func (m *MongoAnalyzer) Stop() error {
	return nil
}

func (m *MongoAnalyzer) GetDefaults(uuid string) map[string]interface{} {
	return
}

// String returns human readable identification of Analyzer
func (m *MongoAnalyzer) String() string {
	return ""
}
