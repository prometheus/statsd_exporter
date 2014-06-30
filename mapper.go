// Copyright (c) 2013, Prometheus Team
// All rights reserved.
//
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package main

import (
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
)

var (
	identifierRE = `[a-zA-Z_][a-zA-Z0-9_]+`
	metricLineRE = regexp.MustCompile(`^(\*\.|` + identifierRE + `\.)+(\*|` + identifierRE + `)$`)
	labelLineRE  = regexp.MustCompile(`^(` + identifierRE + `)\s*=\s*"(.*)"$`)
	metricNameRE = regexp.MustCompile(`^` + identifierRE + `$`)
)

type metricMapping struct {
	regex  *regexp.Regexp
	labels prometheus.Labels
}

type metricMapper struct {
	mappings []metricMapping
	mutex    sync.Mutex
}

type configLoadStates int

const (
	SEARCHING configLoadStates = iota
	METRIC_DEFINITION
)

func (m *metricMapper) initFromString(fileContents string) error {
	lines := strings.Split(fileContents, "\n")
	state := SEARCHING

	parsedMappings := []metricMapping{}
	currentMapping := metricMapping{labels: prometheus.Labels{}}
	for i, line := range lines {
		line := strings.TrimSpace(line)

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
			if line == "" {
				if len(currentMapping.labels) == 0 {
					return fmt.Errorf("Line %d: metric mapping didn't set any labels", i)
				}
				if _, ok := currentMapping.labels["name"]; !ok {
					return fmt.Errorf("Line %d: metric mapping didn't set a metric name", i)
				}

				parsedMappings = append(parsedMappings, currentMapping)

				state = SEARCHING
				currentMapping = metricMapping{labels: prometheus.Labels{}}
				continue
			}

			matches := labelLineRE.FindStringSubmatch(line)
			if len(matches) != 3 {
				return fmt.Errorf("Line %d: expected label mapping line, got: %s", i, line)
			}
			label, value := matches[1], matches[2]
			if label == "name" && !metricNameRE.MatchString(value) {
				return fmt.Errorf("Line %d: metric name '%s' doesn't match regex '%s'", i, value, metricNameRE)
			}
			currentMapping.labels[label] = value
		default:
			panic("illegal state")
		}
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()
	m.mappings = parsedMappings

	mappingsCount.Set(float64(len(parsedMappings)))

	return nil
}

func (m *metricMapper) initFromFile(fileName string) error {
	mappingStr, err := ioutil.ReadFile(fileName)
	if err != nil {
		return err
	}
	return m.initFromString(string(mappingStr))
}

func (m *metricMapper) getMapping(statsdMetric string) (labels prometheus.Labels, present bool) {
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, mapping := range m.mappings {
		matches := mapping.regex.FindStringSubmatchIndex(statsdMetric)
		if len(matches) == 0 {
			continue
		}

		labels := prometheus.Labels{}
		for label, valueExpr := range mapping.labels {
			value := mapping.regex.ExpandString([]byte{}, valueExpr, statsdMetric, matches)
			labels[label] = string(value)
		}
		return labels, true
	}

	return nil, false
}
