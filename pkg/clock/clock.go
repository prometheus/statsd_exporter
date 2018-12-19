package clock

import (
	"time"
)

var ClockInstance *Clock

type Clock struct {
	Instant  time.Time
	TickerCh chan time.Time
}

func Now() time.Time {
	if ClockInstance == nil {
		return time.Now()
	}
	return ClockInstance.Instant
}

func NewTicker(d time.Duration) *time.Ticker {
	if ClockInstance == nil || ClockInstance.TickerCh == nil {
		return time.NewTicker(d)
	}
	return &time.Ticker{
		C: ClockInstance.TickerCh,
	}
}
