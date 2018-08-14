// Copyright 2013 The Prometheus Authors
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package mapper

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	yaml "gopkg.in/yaml.v2"
)

var (
	statsdMetricRE    = `[a-zA-Z_](-?[a-zA-Z0-9_])+`
	templateReplaceRE = `(\$\{?\d+\}?)`

	metricLineRE = regexp.MustCompile(`^(\*\.|` + statsdMetricRE + `\.)+(\*|` + statsdMetricRE + `)$`)
	metricNameRE = regexp.MustCompile(`^([a-zA-Z_]|` + templateReplaceRE + `)([a-zA-Z0-9_]|` + templateReplaceRE + `)*$`)
	labelNameRE  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]+$`)
)

type mapperConfigDefaults struct {
	TimerType TimerType         `yaml:"timer_type"`
	Buckets   []float64         `yaml:"buckets"`
	Quantiles []metricObjective `yaml:"quantiles"`
	MatchType MatchType         `yaml:"match_type"`
}

type MetricMapper struct {
	Defaults mapperConfigDefaults `yaml:"defaults"`
	Mappings []MetricMapping      `yaml:"mappings"`
	mutex    sync.Mutex

	MappingsCount prometheus.Gauge
}

type matchMetricType string

type MetricMapping struct {
	Match           string `yaml:"match"`
	Name            string `yaml:"name"`
	regex           *regexp.Regexp
	Labels          prometheus.Labels `yaml:"labels"`
	TimerType       TimerType         `yaml:"timer_type"`
	Buckets         []float64         `yaml:"buckets"`
	Quantiles       []metricObjective `yaml:"quantiles"`
	MatchType       MatchType         `yaml:"match_type"`
	HelpText        string            `yaml:"help"`
	Action          ActionType        `yaml:"action"`
	MatchMetricType MetricType        `yaml:"match_metric_type"`
}

type metricObjective struct {
	Quantile float64 `yaml:"quantile"`
	Error    float64 `yaml:"error"`
}

var defaultQuantiles = []metricObjective{
	{Quantile: 0.5, Error: 0.05},
	{Quantile: 0.9, Error: 0.01},
	{Quantile: 0.99, Error: 0.001},
}

func (m *MetricMapper) InitFromYAMLString(fileContents string) error {
	var n MetricMapper

	if err := yaml.Unmarshal([]byte(fileContents), &n); err != nil {
		return err
	}

	if n.Defaults.Buckets == nil || len(n.Defaults.Buckets) == 0 {
		n.Defaults.Buckets = prometheus.DefBuckets
	}

	if n.Defaults.Quantiles == nil || len(n.Defaults.Quantiles) == 0 {
		n.Defaults.Quantiles = defaultQuantiles
	}

	if n.Defaults.MatchType == MatchTypeDefault {
		n.Defaults.MatchType = MatchTypeGlob
	}

	for i := range n.Mappings {
		currentMapping := &n.Mappings[i]

		// check that label is correct
		for k := range currentMapping.Labels {
			if !labelNameRE.MatchString(k) {
				return fmt.Errorf("invalid label key: %s", k)
			}
		}

		if currentMapping.Name == "" {
			return fmt.Errorf("line %d: metric mapping didn't set a metric name", i)
		}

		if !metricNameRE.MatchString(currentMapping.Name) {
			return fmt.Errorf("metric name '%s' doesn't match regex '%s'", currentMapping.Name, metricNameRE)
		}

		if currentMapping.MatchType == "" {
			currentMapping.MatchType = n.Defaults.MatchType
		}

		if currentMapping.Action == "" {
			currentMapping.Action = ActionTypeMap
		}

		if currentMapping.MatchType == MatchTypeGlob {
			if !metricLineRE.MatchString(currentMapping.Match) {
				return fmt.Errorf("invalid match: %s", currentMapping.Match)
			}
			// Translate the glob-style metric match line into a proper regex that we
			// can use to match metrics later on.
			metricRe := strings.Replace(currentMapping.Match, ".", "\\.", -1)
			metricRe = strings.Replace(metricRe, "*", "([^.]*)", -1)
			if regex, err := regexp.Compile("^" + metricRe + "$"); err != nil {
				return fmt.Errorf("invalid match %s. cannot compile regex in mapping: %v", currentMapping.Match, err)
			} else {
				currentMapping.regex = regex
			}
		} else {
			if regex, err := regexp.Compile(currentMapping.Match); err != nil {
				return fmt.Errorf("invalid regex %s in mapping: %v", currentMapping.Match, err)
			} else {
				currentMapping.regex = regex
			}
		}

		if currentMapping.TimerType == "" {
			currentMapping.TimerType = n.Defaults.TimerType
		}

		if currentMapping.Buckets == nil || len(currentMapping.Buckets) == 0 {
			currentMapping.Buckets = n.Defaults.Buckets
		}

		if currentMapping.Quantiles == nil || len(currentMapping.Quantiles) == 0 {
			currentMapping.Quantiles = n.Defaults.Quantiles
		}

	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.Defaults = n.Defaults
	m.Mappings = n.Mappings

	if m.MappingsCount != nil {
		m.MappingsCount.Set(float64(len(n.Mappings)))
	}

	return nil
}

func (m *MetricMapper) InitFromFile(fileName string) error {
	mappingStr, err := ioutil.ReadFile(fileName)
	if err != nil {
		return err
	}
	return m.InitFromYAMLString(string(mappingStr))
}

func (m *MetricMapper) GetMapping(statsdMetric string, statsdMetricType MetricType) (*MetricMapping, prometheus.Labels, bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, mapping := range m.Mappings {
		matches := mapping.regex.FindStringSubmatchIndex(statsdMetric)
		if len(matches) == 0 {
			continue
		}

		mapping.Name = string(mapping.regex.ExpandString(
			[]byte{},
			mapping.Name,
			statsdMetric,
			matches,
		))

		if mt := mapping.MatchMetricType; mt != "" && mt != statsdMetricType {
			continue
		}

		labels := prometheus.Labels{}
		for label, valueExpr := range mapping.Labels {
			value := mapping.regex.ExpandString([]byte{}, valueExpr, statsdMetric, matches)
			labels[label] = string(value)
		}

		return &mapping, labels, true
	}

	return nil, nil, false
}
