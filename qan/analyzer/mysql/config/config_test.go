package config

import (
	"testing"

	pc "github.com/percona/pmm/proto/config"
	"github.com/stretchr/testify/require"
)

func TestValidateConfig(t *testing.T) {
	uuid := "123"
	exampleQueries := true
	cfg := pc.QAN{
		UUID:           uuid,
		Interval:       300,        // 5 min
		MaxSlowLogSize: 1073741824, // 1 GiB
		ExampleQueries: &exampleQueries,
		CollectFrom:    "slowlog",
	}
	_, err := ValidateConfig(cfg)
	require.NoError(t, err)
}
