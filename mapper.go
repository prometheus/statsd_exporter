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
)

var (
	identifierRE = `[a-zA-Z_][a-zA-Z0-9_]+`
	metricLineRE = regexp.MustCompile(`^(\*\.|` + identifierRE + `\.)+(\*|` + identifierRE + `)$`)
	labelLineRE  = regexp.MustCompile(`^(` + identifierRE + `)\s*=\s*"(.*)"$`)
)

type metricMapping struct {
	regex  *regexp.Regexp
	labels map[string]string
}

type metricMapper struct {
	mappings []metricMapping
}

type configLoadStates int

const (
	SEARCHING configLoadStates = iota
	METRIC_DEFINITION
)

func (l *metricMapper) initFromString(fileContents string) error {
	lines := strings.Split(fileContents, "\n")
	state := SEARCHING

	mapping := metricMapping{labels: map[string]string{}}
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
			metricRe := strings.Replace(line, ".", "\\.", -1)
			metricRe = strings.Replace(metricRe, "*", "([^.]+)", -1)
			mapping.regex = regexp.MustCompile("^" + metricRe + "$")
			state = METRIC_DEFINITION

		case METRIC_DEFINITION:
			if line == "" {
				if len(mapping.labels) == 0 {
					return fmt.Errorf("Line %d: metric mapping didn't set any labels", i)
				}
				if _, ok := mapping.labels["name"]; !ok {
					return fmt.Errorf("Line %d: metric mapping didn't set a metric name", i)
				}

				l.mappings = append(l.mappings, mapping)

				state = SEARCHING
				mapping = metricMapping{labels: map[string]string{}}
				continue
			}

			matches := labelLineRE.FindStringSubmatch(line)
			if len(matches) != 3 {
				return fmt.Errorf("Line %d: expected label mapping line, got: %s", i, line)
			}
			mapping.labels[matches[1]] = matches[2]
		default:
			panic("illegal state")
		}
	}
	return nil
}

func (l *metricMapper) initFromFile(fileName string) error {
	mappingStr, err := ioutil.ReadFile(fileName)
	if err != nil {
		return err
	}
	return l.initFromString(string(mappingStr))
}

func (l *metricMapper) getMapping(statsdMetric string) (labels map[string]string, present bool) {
	for _, mapping := range l.mappings {
		matches := mapping.regex.FindStringSubmatchIndex(statsdMetric)
		if len(matches) == 0 {
			continue
		}

		labels := map[string]string{}
		for label, valueExpr := range mapping.labels {
			value := mapping.regex.ExpandString([]byte{}, valueExpr, statsdMetric, matches)
			labels[label] = string(value)
		}
		return labels, true
	}

	return nil, false
}
