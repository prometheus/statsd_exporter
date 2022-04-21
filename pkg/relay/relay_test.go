package relay

import (
	"github.com/go-kit/log"
	"github.com/prometheus/statsd_exporter/pkg/clock"
	"github.com/stvp/go-udp-testing"
	"testing"
	"time"
)

func TestRelay_RelayLine(t *testing.T) {
	type args struct {
		lines    []string
		expected string
	}

	tests := []struct {
		name string
		args args
	}{
		{
			name: "single line",
			args: args{
				lines:    []string{"foo5:100|c|#tag1:bar,#tag2:baz"},
				expected: "foo5:100|c|#tag1:bar,#tag2:baz\n",
			},
		},
	}

	for _, tt := range tests {
		udp.SetAddr(":1160")
		t.Run(tt.name, func(t *testing.T) {

			tickerCh := make(chan time.Time)
			clock.ClockInstance = &clock.Clock{
				TickerCh: tickerCh,
			}
			clock.ClockInstance.Instant = time.Unix(0, 0)

			logger := log.NewNopLogger()
			r, err := NewRelay(
				logger,
				"localhost:1160",
				200,
			)

			if err != nil {
				t.Errorf("Did not expect error while creating relay.")
			}

			udp.ShouldReceive(t, tt.args.expected, func() {
				for _, line := range tt.args.lines {
					r.RelayLine(line)
					// Tick time forward to trigger a flush
				}
				clock.ClockInstance.Instant = time.Unix(20000, 0)
				clock.ClockInstance.TickerCh <- time.Unix(20000, 0)
			})
		})
	}
}
