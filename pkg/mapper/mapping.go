// Copyright 2020 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either xpress or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mapper

import (
	"regexp"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/prometheus/statsd_exporter/pkg/mapper/fsm"
)

type MetricMapping struct {
	Match            string `yaml:"match"`
	Name             string `yaml:"name"`
	nameFormatter    *fsm.TemplateFormatter
	regex            *regexp.Regexp
	Labels           prometheus.Labels `yaml:"labels"`
	HonorLabels      bool              `yaml:"honor_labels"`
	labelKeys        []string
	labelFormatters  []*fsm.TemplateFormatter
	ObserverType     ObserverType      `yaml:"observer_type"`
	MatchType        MatchType         `yaml:"match_type"`
	HelpText         string            `yaml:"help"`
	Action           ActionType        `yaml:"action"`
	MatchMetricType  MetricType        `yaml:"match_metric_type"`
	Ttl              time.Duration     `yaml:"ttl"`
	SummaryOptions   *SummaryOptions   `yaml:"summary_options"`
	HistogramOptions *HistogramOptions `yaml:"histogram_options"`
	Scale            MaybeFloat64      `yaml:"scale"`
}

type MaybeFloat64 struct {
	Set bool
	Val float64
}

func (m *MaybeFloat64) MarshalYAML() (interface{}, error) {
	if m.Set {
		return m.Val, nil
	}
	return nil, nil
}

func (m *MaybeFloat64) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var tmp float64
	if err := unmarshal(&tmp); err != nil {
		return err
	}
	m.Val = tmp
	m.Set = true
	return nil
}
