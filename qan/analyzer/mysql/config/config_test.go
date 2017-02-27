package config

import (
	"testing"

	pc "github.com/percona/pmm/proto/config"
	"github.com/stretchr/testify/assert"
)

func TestValidateConfig(t *testing.T) {
	uuid := "123"
	cfg := pc.QAN{
		UUID:           uuid,
		Interval:       300,        // 5 min
		MaxSlowLogSize: 1073741824, // 1 GiB
		ExampleQueries: true,
		WorkerRunTime:  600, // 10 min
		CollectFrom:    "slowlog",
	}
	_, err := ValidateConfig(cfg)
	assert.Nil(t, err)
}
