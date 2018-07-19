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
	"bufio"
	"fmt"
	"io/ioutil"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
	yaml "gopkg.in/yaml.v2"
)

var (
	statsdMetricRE    = `[a-zA-Z_](-?[a-zA-Z0-9_])+`
	templateReplaceRE = `(\$\{?\d+\}?)`

	metricLineRE          = regexp.MustCompile(`^(\*\.|` + statsdMetricRE + `\.)+(\*|` + statsdMetricRE + `)$`)
	metricNameRE          = regexp.MustCompile(`^([a-zA-Z_]|` + templateReplaceRE + `)([a-zA-Z0-9_]|` + templateReplaceRE + `)*$`)
	labelNameRE           = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]+$`)
	labelValueExpansionRE = regexp.MustCompile(`\${?(\d+)}?`)
)

type mapperConfigDefaults struct {
	TimerType   TimerType         `yaml:"timer_type"`
	Buckets     []float64         `yaml:"buckets"`
	Quantiles   []metricObjective `yaml:"quantiles"`
	MatchType   MatchType         `yaml:"match_type"`
	DumpFSM     string            `yaml:"dump_fsm"`
	FSMFallback MatchType         `yaml:"fsm_fallback"`
}

type mappingState struct {
	transitions map[string]*mappingState
	// result is nil unless there's a metric ends with this state
	result *MetricMapping
}

type MetricMapper struct {
	Defaults mapperConfigDefaults `yaml:"defaults"`
	Mappings []MetricMapping      `yaml:"mappings"`
	FSM      *mappingState
	mutex    sync.Mutex

	MappingsCount prometheus.Gauge
}

type labelFormatter struct {
	captureIdx int
	fmtString  string
}

type matchMetricType string

type MetricMapping struct {
	Match           string `yaml:"match"`
	Name            string `yaml:"name"`
	regex           *regexp.Regexp
	Labels          prometheus.Labels `yaml:"labels"`
	LabelsFormatter map[string]labelFormatter
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

	maxPossibleTransitions := len(n.Mappings)

	n.FSM = &mappingState{}
	n.FSM.transitions = make(map[string]*mappingState, maxPossibleTransitions)

	for i := range n.Mappings {
		maxPossibleTransitions--

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

		if currentMapping.MatchType == MatchTypeFSM {
			// first split by "."
			matchFields := strings.Split(currentMapping.Match, ".")
			// fill into our FSM
			root := n.FSM
			captureCount := 0
			for i, field := range matchFields {
				state, prs := root.transitions[field]
				if !prs {
					state = &mappingState{}
					(*state).transitions = make(map[string]*mappingState, maxPossibleTransitions)
					root.transitions[field] = state
					// if this is last field, set result to currentMapping instance
					if i == len(matchFields)-1 {
						root.transitions[field].result = currentMapping
					}
				}
				if field == "*" {
					captureCount++
				}

				// goto next state
				root = state
			}
			currentLabelFormatter := make(map[string]labelFormatter, captureCount)
			for label, valueExpr := range currentMapping.Labels {
				matches := labelValueExpansionRE.FindAllStringSubmatch(valueExpr, -1)
				if len(matches) == 0 {
					// if no regex expansion found, keep it as it is
					currentLabelFormatter[label] = labelFormatter{captureIdx: -1, fmtString: valueExpr}
					continue
				} else if len(matches) > 1 {
					return fmt.Errorf("multiple captures is not supported in FSM matching type")
				}
				var valueFormatter string
				idx, err := strconv.Atoi(matches[0][1])
				if err != nil {
					return fmt.Errorf("invalid label value expression: %s", valueExpr)
				}
				if idx > captureCount || idx < 1 {
					// index larger than captured count, replace all expansion with empty string
					valueFormatter = labelValueExpansionRE.ReplaceAllString(valueExpr, "")
					idx = 0
				} else {
					valueFormatter = labelValueExpansionRE.ReplaceAllString(valueExpr, "%s")
				}
				currentLabelFormatter[label] = labelFormatter{captureIdx: idx - 1, fmtString: valueFormatter}
			}
			currentMapping.LabelsFormatter = currentLabelFormatter
		}
		if currentMapping.MatchType == MatchTypeGlob || n.Defaults.FSMFallback == MatchTypeGlob {
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
		} else if currentMapping.MatchType == MatchTypeRegex || n.Defaults.FSMFallback == MatchTypeRegex {
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
	if len(n.Defaults.DumpFSM) > 0 {
		m.dumpFSM(n.Defaults.DumpFSM, n.FSM)
	}

	m.mutex.Lock()
	defer m.mutex.Unlock()

	m.Defaults = n.Defaults
	m.Mappings = n.Mappings
	if len(n.FSM.transitions) > 0 {
		m.FSM = n.FSM
	}

	if m.MappingsCount != nil {
		m.MappingsCount.Set(float64(len(n.Mappings)))
	}

	return nil
}

func (m *MetricMapper) dumpFSM(fileName string, root *mappingState) {
	log.Infoln("Start dumping FSM to", fileName)
	idx := 0
	states := make(map[int]*mappingState)
	states[idx] = root

	f, _ := os.Create(fileName)
	w := bufio.NewWriter(f)
	w.WriteString("digraph g {\n")
	w.WriteString("rankdir=LR\n")                                                    // make it vertical
	w.WriteString("node [ label=\"\",style=filled,fillcolor=white,shape=circle ]\n") // remove label of node

	for idx < len(states) {
		for field, transition := range states[idx].transitions {
			states[len(states)] = transition
			w.WriteString(fmt.Sprintf("%d -> %d  [label = \"%s\"];\n", idx, len(states)-1, field))
			if transition.transitions == nil || len(transition.transitions) == 0 {
				w.WriteString(fmt.Sprintf("%d [color=\"#82B366\",fillcolor=\"#D5E8D4\"];\n", len(states)-1))
			}

		}
		idx++
	}
	w.WriteString(fmt.Sprintf("0 [color=\"#D6B656\",fillcolor=\"#FFF2CC\"];\n"))
	w.WriteString("}")
	w.Flush()
	log.Infoln("Finish dumping FSM")
}

func (m *MetricMapper) InitFromFile(fileName string) error {
	mappingStr, err := ioutil.ReadFile(fileName)
	if err != nil {
		return err
	}
	return m.InitFromYAMLString(string(mappingStr))
}

func (m *MetricMapper) GetMapping(statsdMetric string, statsdMetricType MetricType) (*MetricMapping, prometheus.Labels, bool) {
	if root := m.FSM; root != nil {
		matchFields := strings.Split(statsdMetric, ".")
		captures := make(map[int]string, len(matchFields))
		captureIdx := 0
		filedsCount := len(matchFields)
		for i, field := range matchFields {
			if root.transitions == nil {
				break
			}
			state, prs := root.transitions[field]
			if !prs {
				state, prs = root.transitions["*"]
				if !prs {
					break
				}
				captures[captureIdx] = field
				captureIdx++
			}
			if state.result != nil && i == filedsCount-1 {
				// format valueExpr
				mapping := *state.result
				labels := prometheus.Labels{}
				for label := range mapping.Labels {
					formatter := mapping.LabelsFormatter[label]
					idx := formatter.captureIdx
					var value string
					if idx == -1 {
						value = formatter.fmtString
					} else {
						value = fmt.Sprintf(formatter.fmtString, captures[idx])
					}
					labels[label] = string(value)
				}
				return state.result, labels, true
			}
			root = state
		}

		// if fsm_fallback is not defined, return immediately
		if len(m.Defaults.FSMFallback) == 0 {
			log.Infof("%s not matched by fsm\n", statsdMetric)
			return nil, nil, false
		}
	}

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
