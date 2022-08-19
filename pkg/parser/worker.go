package parser

import (
	"github.com/go-kit/log"
	"github.com/go-kit/log/level"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/statsd_exporter/pkg/event"
	"github.com/prometheus/statsd_exporter/pkg/line"
	"github.com/prometheus/statsd_exporter/pkg/relay"
	"strings"
)

type Worker struct {
	EventHandler event.EventHandler
	Logger       log.Logger
	LineParser   *line.Parser
	Relay        *relay.Relay

	LinesReceived   prometheus.Counter
	SampleErrors    prometheus.CounterVec
	SamplesReceived prometheus.Counter
	TagErrors       prometheus.Counter
	TagsReceived    prometheus.Counter
}

func NewWorker(
	logger log.Logger,
	eventHandler event.EventHandler,
	lineParser *line.Parser,
	relay *relay.Relay,
	linesReceived prometheus.Counter,
	sampleErrors prometheus.CounterVec,
	samplesReceived prometheus.Counter,
	tagErrors prometheus.Counter,
	tagsReceived prometheus.Counter,
) *Worker {
	return &Worker{
		EventHandler:    eventHandler,
		Logger:          logger,
		LineParser:      lineParser,
		Relay:           relay,
		LinesReceived:   linesReceived,
		SampleErrors:    sampleErrors,
		SamplesReceived: samplesReceived,
		TagErrors:       tagErrors,
		TagsReceived:    tagsReceived,
	}
}

func (w *Worker) Consume(c <-chan string) {
	for {
		select {
		case bytes, ok := <-c:
			if !ok {
				level.Debug(w.Logger).Log("msg", "channel closed, exiting consume loop")
				return
			}
			w.handle(bytes)
		}
	}
}

func (w *Worker) handle(packet string) {
	lines := strings.Split(packet, "\n")
	for _, l := range lines {
		level.Debug(w.Logger).Log("msg", "Incoming line", "sample", l)
		w.LinesReceived.Inc()
		if w.Relay != nil && len(l) > 0 {
			w.Relay.RelayLine(l)
		}
		w.EventHandler.Queue(w.LineParser.LineToEvents(l, w.SampleErrors, w.SamplesReceived, w.TagErrors, w.TagsReceived, w.Logger))
	}
}
