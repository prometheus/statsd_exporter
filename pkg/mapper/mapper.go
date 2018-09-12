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

	metricLineRE = regexp.MustCompile(`^(\*\.|` + statsdMetricRE + `\.)+(\*|` + statsdMetricRE + `)$`)
	metricNameRE = regexp.MustCompile(`^([a-zA-Z_]|` + templateReplaceRE + `)([a-zA-Z0-9_]|` + templateReplaceRE + `)*$`)
	labelNameRE  = regexp.MustCompile(`^[a-zA-Z_][a-zA-Z0-9_]+$`)

	templateReplaceCaptureRE = regexp.MustCompile(`\$\{?([a-zA-Z0-9_\$]+)\}?`)
)

type mapperConfigDefaults struct {
	TimerType TimerType         `yaml:"timer_type"`
	Buckets   []float64         `yaml:"buckets"`
	Quantiles []metricObjective `yaml:"quantiles"`
	MatchType MatchType         `yaml:"match_type"`
}

type mappingState struct {
	transitionsMap   map[string]*mappingState
	transitionsArray []*mappingState
	// result is nil unless there's a metric ends with this state
	result *MetricMapping
}

type MetricMapper struct {
	Defaults mapperConfigDefaults `yaml:"defaults"`
	Mappings []MetricMapping      `yaml:"mappings"`
	FSM      *mappingState
	// if this is true, that means at least one matching rule is regex type
	doRegex     bool
	dumpFSMPath string
	mutex       sync.Mutex

	MappingsCount prometheus.Gauge
}

type templateFormatter struct {
	captureIndexes []int
	captureCount   int
	fmtString      string
}

type matchMetricType string

type MetricMapping struct {
	Match           string `yaml:"match"`
	Name            string `yaml:"name"`
	NameFormatter   templateFormatter
	regex           *regexp.Regexp
	Labels          prometheus.Labels `yaml:"labels"`
	LabelsFormatter map[string]templateFormatter
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

func generateFormatter(valueExpr string, captureCount int) (templateFormatter, error) {
	matches := templateReplaceCaptureRE.FindAllStringSubmatch(valueExpr, -1)
	if len(matches) == 0 {
		// if no regex reference found, keep it as it is
		return templateFormatter{captureCount: 0, fmtString: valueExpr}, nil
	}

	var indexes []int
	valueFormatter := valueExpr
	for _, match := range matches {
		idx, err := strconv.Atoi(match[len(match)-1])
		if err != nil || idx > captureCount || idx < 1 {
			// if index larger than captured count or using unsupported named capture group,
			// replace with empty string
			valueFormatter = strings.Replace(valueFormatter, match[0], "", -1)
		} else {
			valueFormatter = strings.Replace(valueFormatter, match[0], "%s", -1)
			// note: the regex reference variable $? starts from 1
			indexes = append(indexes, idx-1)
		}
	}
	return templateFormatter{
		captureIndexes: indexes,
		captureCount:   len(indexes),
		fmtString:      valueFormatter,
	}, nil
}

func formatTemplate(formatter templateFormatter, captures map[int]string) string {
	if formatter.captureCount == 0 {
		// no label substitution, keep as it is
		return formatter.fmtString
	} else {
		indexes := formatter.captureIndexes
		vargs := make([]interface{}, formatter.captureCount)
		for i, idx := range indexes {
			vargs[i] = captures[idx]
		}
		return fmt.Sprintf(formatter.fmtString, vargs...)
	}
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
	n.FSM.transitionsMap = make(map[string]*mappingState, maxPossibleTransitions)

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

		if currentMapping.MatchType == MatchTypeGlob {
			if !metricLineRE.MatchString(currentMapping.Match) {
				return fmt.Errorf("invalid match: %s", currentMapping.Match)
			}

			// first split by "."
			matchFields := strings.Split(currentMapping.Match, ".")
			// fill into our FSM
			root := n.FSM
			captureCount := 0
			for i, field := range matchFields {
				state, prs := root.transitionsMap[field]
				if !prs {
					state = &mappingState{}
					(*state).transitionsMap = make(map[string]*mappingState, maxPossibleTransitions)
					root.transitionsMap[field] = state
					// if this is last field, set result to currentMapping instance
					if i == len(matchFields)-1 {
						root.transitionsMap[field].result = currentMapping
					}
				}
				if field == "*" {
					captureCount++
				}

				// goto next state
				root = state
			}
			nameFmt, err := generateFormatter(currentMapping.Name, captureCount)
			if err != nil {
				return err
			}
			currentMapping.NameFormatter = nameFmt

			currentLabelFormatter := make(map[string]templateFormatter, captureCount)
			for label, valueExpr := range currentMapping.Labels {
				lblFmt, err := generateFormatter(valueExpr, captureCount)
				if err != nil {
					return err
				}
				currentLabelFormatter[label] = lblFmt
			}
			currentMapping.LabelsFormatter = currentLabelFormatter
		} else {
			if regex, err := regexp.Compile(currentMapping.Match); err != nil {
				return fmt.Errorf("invalid regex %s in mapping: %v", currentMapping.Match, err)
			} else {
				currentMapping.regex = regex
			}
			m.doRegex = true
		} /*else if currentMapping.MatchType == MatchTypeGlob {
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
		} */

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
	if len(n.FSM.transitionsMap) > 0 || len(n.FSM.transitionsArray) > 0 {
		m.FSM = n.FSM
		if m.dumpFSMPath != "" {
			dumpFSM(m.dumpFSMPath, m.FSM)
		}
	}

	if m.MappingsCount != nil {
		m.MappingsCount.Set(float64(len(n.Mappings)))
	}

	return nil
}

func (m *MetricMapper) SetDumpFSMPath(path string) error {
	m.dumpFSMPath = path
	return nil
}

func dumpFSM(fileName string, root *mappingState) {
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
		for field, transition := range states[idx].transitionsMap {
			states[len(states)] = transition
			w.WriteString(fmt.Sprintf("%d -> %d  [label = \"%s\"];\n", idx, len(states)-1, field))
			if transition.transitionsMap == nil || len(transition.transitionsMap) == 0 {
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
	// glob matching
	if root := m.FSM; root != nil {
		matchFields := strings.Split(statsdMetric, ".")
		captures := make(map[int]string, len(matchFields))
		captureIdx := 0
		filedsCount := len(matchFields)
		for i, field := range matchFields {
			if root.transitionsMap == nil {
				break
			}
			state, prs := root.transitionsMap[field]
			if !prs {
				state, prs = root.transitionsMap["*"]
				if !prs {
					break
				}
				captures[captureIdx] = field
				captureIdx++
			}
			if state.result != nil && i == filedsCount-1 {
				mapping := *state.result
				state.result.Name = formatTemplate(mapping.NameFormatter, captures)

				labels := prometheus.Labels{}
				for label := range mapping.Labels {
					labels[label] = formatTemplate(mapping.LabelsFormatter[label], captures)
				}
				return state.result, labels, true
			}
			root = state
		}

		// if there's no regex match type, return immediately
		if !m.doRegex {
			return nil, nil, false
		}
	}

	// regex matching
	m.mutex.Lock()
	defer m.mutex.Unlock()

	for _, mapping := range m.Mappings {
		// if a rule don't have regex matching type, the regex field is unset
		if mapping.regex == nil {
			continue
		}
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
