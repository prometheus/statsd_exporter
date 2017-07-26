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

package main

import (
	"fmt"
	"io/ioutil"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	yaml "gopkg.in/yaml.v2"
)

var (
	identifierRE   = `[a-zA-Z_][a-zA-Z0-9_]+`
	statsdMetricRE = `[a-zA-Z_](-?[a-zA-Z0-9_])+`

	metricLineRE = regexp.MustCompile(`^(\*\.|` + statsdMetricRE + `\.)+(\*|` + statsdMetricRE + `)$`)
	labelLineRE  = regexp.MustCompile(`^(` + identifierRE + `)\s*=\s*"(.*)"$`)
	metricNameRE = regexp.MustCompile(`^` + identifierRE + `$`)
)

type mapperConfigDefaults struct {
	TimerType string    `yaml:"timer_type"`
	Buckets   []float64 `yaml:"buckets"`
}

type metricMapper struct {
	Defaults mapperConfigDefaults `yaml:"defaults"`
	Mappings []metricMapping      `yaml:"mappings"`
	mutex    sync.Mutex
}

type metricMapping struct {
	Match     string `yaml:"match"`
	regex     *regexp.Regexp
	Labels    prometheus.Labels
	TimerType string    `yaml:"timer_type"`
	Buckets   []float64 `yaml:"buckets"`
}

type configLoadStates int

const (
	SEARCHING configLoadStates = iota
	METRIC_DEFINITION
)

func (m *metricMapper) initFromString(fileContents string) error {
	lines := strings.Split(fileContents, "\n")
	numLines := len(lines)
	state := SEARCHING

	parsedMappings := []metricMapping{}
	currentMapping := metricMapping{Labels: prometheus.Labels{}}
	for i, line := range lines {
		line = strings.TrimSpace(line)

		// skip comments
		if strings.HasPrefix(line, "#") {
			continue
		}

		switch state {
		case SEARCHING:
			if line == "" {
				continue
			}
			if !metricLineRE.MatchString(line) {
				return fmt.Errorf("Line %d: expected metric match line, got: %s", i, line)
			}

			// Translate the glob-style metric match line into a proper regex that we
			// can use to match metrics later on.
			metricRe := strings.Replace(line, ".", "\\.", -1)
			metricRe = strings.Replace(metricRe, "*", "([^.]+)", -1)
			currentMapping.regex = regexp.MustCompile("^" + metricRe + "$")

			state = METRIC_DEFINITION

		case METRIC_DEFINITION:
			if (i == numLines-1) && (line != "") {
				return fmt.Errorf("Line %d: missing terminating newline", i)
			}
			if line == "" {
				if len(currentMapping.Labels) == 0 {
					return fmt.Errorf("Line %d: metric mapping didn't set any labels", i)
				}
				if _, ok := currentMapping.Labels["name"]; !ok {
					return fmt.Errorf("Line %d: metric mapping didn't set a metric name", i)
				}
				parsedMappings = append(parsedMappings, currentMapping)
				state = SEARCHING
				currentMapping = metricMapping{Labels: prometheus.Labels{}}
				continue
			}
			if err := m.updateMapping(line, i, &currentMapping); err != nil {
				return err
			}
		default:
			panic("illegal state")
		}
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.Mappings = parsedMappings

	mappingsCount.Set(float64(len(parsedMappings)))

	return nil
}

func (m *metricMapper) initFromYamlString(fileContents string) error {

	var n metricMapper

	if err := yaml.Unmarshal([]byte(fileContents), &n); err != nil {
		return err
	}

	if n.Defaults.Buckets == nil || len(n.Defaults.Buckets) == 0 {
		n.Defaults.Buckets = prometheus.DefBuckets
	}

	for i := range n.Mappings {
		currentMapping := &n.Mappings[i]

		if !metricLineRE.MatchString(currentMapping.Match) {
			return fmt.Errorf("invalid match: %s", currentMapping.Match)
		}

		// Translate the glob-style metric match line into a proper regex that we
		// can use to match metrics later on.
		metricRe := strings.Replace(currentMapping.Match, ".", "\\.", -1)
		metricRe = strings.Replace(metricRe, "*", "([^.]+)", -1)
		currentMapping.regex = regexp.MustCompile("^" + metricRe + "$")

		if currentMapping.TimerType == "" {
			currentMapping.TimerType = n.Defaults.TimerType
		}

		if currentMapping.Buckets == nil || len(currentMapping.Buckets) == 0 {
			currentMapping.Buckets = n.Defaults.Buckets
		}
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.Defaults = n.Defaults
	m.Mappings = n.Mappings

	mappingsCount.Set(float64(len(n.Mappings)))

	return nil
}

func (m *metricMapper) initFromFile(fileName string) error {
	mappingStr, err := ioutil.ReadFile(fileName)
	if err != nil {
		return err
	}
	switch strings.ToLower(filepath.Ext(fileName)) {
	case ".yaml", ".yml":
		return m.initFromYamlString(string(mappingStr))
	default:
		return m.initFromString(string(mappingStr))
	}
}

func (m *metricMapper) getMapping(statsdMetric string) (*metricMapping, prometheus.Labels, bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, mapping := range m.Mappings {
		matches := mapping.regex.FindStringSubmatchIndex(statsdMetric)
		if len(matches) == 0 {
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

func (m *metricMapper) updateMapping(line string, i int, mapping *metricMapping) error {
	matches := labelLineRE.FindStringSubmatch(line)
	if len(matches) != 3 {
		return fmt.Errorf("Line %d: expected label mapping line, got: %s", i, line)
	}
	label, value := matches[1], matches[2]
	if label == "name" && !metricNameRE.MatchString(value) {
		return fmt.Errorf("Line %d: metric name '%s' doesn't match regex '%s'", i, value, metricNameRE)
	}
	(*mapping).Labels[label] = value
	return nil
}
