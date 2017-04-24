package parser

import (
	"reflect"
	"testing"

	pm "github.com/percona/percona-toolkit/src/go/mongolib/proto"
	pc "github.com/percona/pmm/proto/config"
	"github.com/percona/pmm/proto/qan"
	"github.com/stretchr/testify/assert"
	"time"
)

func TestNew(t *testing.T) {
	docsChan := make(chan pm.SystemProfile)
	pcQan := pc.QAN{
		Interval: 60,
	}

	type args struct {
		docsChan <-chan pm.SystemProfile
		config   pc.QAN
	}
	tests := []struct {
		name string
		args args
		want *Parser
	}{
		{
			name: "TestNew",
			args: args{
				docsChan: docsChan,
				config:   pcQan,
			},
			want: New(docsChan, pcQan),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := New(tt.args.docsChan, tt.args.config); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("New(%v, %v) = %v, want %v", tt.args.docsChan, tt.args.config, got, tt.want)
			}
		})
	}
}

func TestParser_StartStop(t *testing.T) {
	docsChan := make(chan pm.SystemProfile)
	pcQan := pc.QAN{
		Interval: 60,
	}

	parser1 := New(docsChan, pcQan)
	reportChan1, err := parser1.Start()
	assert.Nil(t, err)
	assert.NotNil(t, reportChan1)

	// running multiple Start() should be idempotent
	reportChan2, err := parser1.Start()
	assert.Nil(t, err)
	assert.NotNil(t, reportChan2)

	assert.Exactly(t, reportChan1, reportChan2)

	// running multiple Stop() should be idempotent
	parser1.Stop()
	parser1.Stop()
}

func TestParser_running(t *testing.T) {
	docsChan := make(chan pm.SystemProfile)
	pcQan := pc.QAN{
		Interval: 60,
	}
	d := time.Duration(pcQan.Interval) * time.Second

	parser1 := New(docsChan, pcQan)
	reportChan1, err := parser1.Start()
	assert.Nil(t, err)
	assert.NotNil(t, reportChan1)

	now := time.Now().UTC()
	timeStart := now.Truncate(d).Add(d)
	timeEnd := timeStart.Add(d)

	select {
	case docsChan <- pm.SystemProfile{
		Ts: timeStart,
	}:
	case <-time.After(5 * time.Second):
		t.Error("test timeout")
	}

	select {
	case docsChan <- pm.SystemProfile{
		Ts: timeEnd.Add(1 * time.Second),
	}:
	case <-time.After(5 * time.Second):
		t.Error("test timeout")
	}

	select {
	case actual := <-reportChan1:
		expected := qan.Report{
			StartTs: timeStart,
			EndTs:   timeEnd,
		}
		assert.Equal(t, expected.StartTs, actual.StartTs)
		assert.Equal(t, expected.EndTs, actual.EndTs)
		assert.Equal(t, "", actual)
	case <-time.After(d + 5*time.Second):
		t.Error("test timeout")
	}

	parser1.Stop()
}
