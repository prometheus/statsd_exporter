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
	TimerType          TimerType         `yaml:"timer_type"`
	Buckets            []float64         `yaml:"buckets"`
	Quantiles          []metricObjective `yaml:"quantiles"`
	MatchType          MatchType         `yaml:"match_type"`
	GlobDisbleOrdering bool              `yaml:"glob_disable_ordering"`
}

type mappingState struct {
	transitions        map[string]*mappingState
	minRemainingLength int
	maxRemainingLength int
	// result is nil unless there's a metric ends with this state
	result *MetricMapping
}

type MetricMapper struct {
	Defaults             mapperConfigDefaults `yaml:"defaults"`
	Mappings             []MetricMapping      `yaml:"mappings"`
	FSM                  *mappingState
	hasFSM               bool
	FSMNeedsBacktracking bool
	// if doRegex is true,  at least one matching rule is regex type
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

type fsmBacktrackStackCursor struct {
	fieldIndex     int
	captureIndex   int
	currentCapture string
	state          *mappingState
	prev           *fsmBacktrackStackCursor
	next           *fsmBacktrackStackCursor
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
	priority        int
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

func generateFormatter(valueExpr string, captureCount int) templateFormatter {
	matches := templateReplaceCaptureRE.FindAllStringSubmatch(valueExpr, -1)
	if len(matches) == 0 {
		// if no regex reference found, keep it as it is
		return templateFormatter{captureCount: 0, fmtString: valueExpr}
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
	}
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

func min(x, y int) int {
	if x < y {
		return x
	}
	return y
}

func max(x, y int) int {
	if x > y {
		return x
	}
	return y
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
	n.FSM.transitions = make(map[string]*mappingState, 3)
	for _, field := range []MetricType{MetricTypeCounter, MetricTypeTimer, MetricTypeGauge, ""} {
		state := &mappingState{}
		(*state).transitions = make(map[string]*mappingState, maxPossibleTransitions)
		n.FSM.transitions[string(field)] = state
	}

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

		currentMapping.priority = i

		if currentMapping.MatchType == MatchTypeGlob {
			n.hasFSM = true
			if !metricLineRE.MatchString(currentMapping.Match) {
				return fmt.Errorf("invalid match: %s", currentMapping.Match)
			}

			// first split by "."
			matchFields := strings.Split(currentMapping.Match, ".")
			// fill into our FSM
			roots := []*mappingState{}
			if currentMapping.MatchMetricType == "" {
				// if metricType not specified, connect the state from all three types
				for _, metricType := range []MetricType{MetricTypeCounter, MetricTypeTimer, MetricTypeGauge, ""} {
					roots = append(roots, n.FSM.transitions[string(metricType)])
				}
			} else {
				roots = append(roots, n.FSM.transitions[string(currentMapping.MatchMetricType)])
			}
			var captureCount int
			for _, root := range roots {
				captureCount = 0
				for i, field := range matchFields {
					state, prs := root.transitions[field]
					if !prs {
						state = &mappingState{}
						(*state).transitions = make(map[string]*mappingState, maxPossibleTransitions)
						(*state).maxRemainingLength = len(matchFields) - i - 1
						(*state).minRemainingLength = len(matchFields) - i - 1
						root.transitions[field] = state
						// if this is last field, set result to currentMapping instance
						if i == len(matchFields)-1 {
							root.transitions[field].result = currentMapping
						}
					} else {
						(*state).maxRemainingLength = max(len(matchFields)-i-1, (*state).maxRemainingLength)
						(*state).minRemainingLength = min(len(matchFields)-i-1, (*state).minRemainingLength)
					}
					if field == "*" {
						captureCount++
					}

					// goto next state
					root = state
				}
			}
			nameFmt := generateFormatter(currentMapping.Name, captureCount)

			currentMapping.NameFormatter = nameFmt

			currentLabelFormatter := make(map[string]templateFormatter, captureCount)
			for label, valueExpr := range currentMapping.Labels {
				lblFmt := generateFormatter(valueExpr, captureCount)
				currentLabelFormatter[label] = lblFmt
			}
			currentMapping.LabelsFormatter = currentLabelFormatter
		} else {
			if regex, err := regexp.Compile(currentMapping.Match); err != nil {
				return fmt.Errorf("invalid regex %s in mapping: %v", currentMapping.Match, err)
			} else {
				currentMapping.regex = regex
			}
			n.doRegex = true
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
	m.hasFSM = n.hasFSM
	if n.hasFSM {
		m.FSM = n.FSM
		m.doRegex = n.doRegex
		if m.dumpFSMPath != "" {
			dumpFSM(m.dumpFSMPath, m.FSM)
		}

		m.FSMNeedsBacktracking = needBacktracking(&n)
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

func needBacktracking(n *MetricMapper) bool {
	needBacktrack := false
	// rule A and B that has same length and
	// A one has * in rules but is not a superset of B makes it needed for backtracking
	ruleByLength := make(map[int][]string)
	ruleREByLength := make(map[int][]*regexp.Regexp)
	rulesOrderByLength := make(map[int][]int)

	// first sort rules by length
	for _, mapping := range n.Mappings {
		if mapping.MatchType != MatchTypeGlob {
			continue
		}
		l := len(strings.Split(mapping.Match, "."))
		ruleByLength[l] = append(ruleByLength[l], mapping.Match)

		metricRe := strings.Replace(mapping.Match, ".", "\\.", -1)
		metricRe = strings.Replace(metricRe, "*", "([^.]*)", -1)
		regex, err := regexp.Compile("^" + metricRe + "$")
		if err != nil {
			log.Warnf("invalid match %s. cannot compile regex in mapping: %v", mapping.Match, err)
		}
		// put into array no matter there's error or not, we will skip later if regex is nil
		ruleREByLength[l] = append(ruleREByLength[l], regex)
		rulesOrderByLength[l] = append(rulesOrderByLength[l], mapping.priority)
	}

	for l, rules := range ruleByLength {
		if len(rules) == 1 {
			continue
		}
		rulesRE := ruleREByLength[l]
		rulesOrder := rulesOrderByLength[l]
		// for each rule r1 in rules that has * inside, check if r1 is the superset of any rules
		// if not then r1 is a rule that leads to backtrack
		for i1, r1 := range rules {
			currentRuleNeedBacktrack := true
			re1 := rulesRE[i1]
			if re1 == nil || strings.Index(r1, "*") == -1 {
				continue
			}
			for i2, r2 := range rules {
				if i2 != i1 && len(re1.FindStringSubmatchIndex(r2)) > 0 {
					// log if we care about ordering and the superset occurs before
					if !n.Defaults.GlobDisbleOrdering && rulesOrder[i1] < rulesOrder[i2] {
						log.Warnf("match \"%s\" is a super set of match \"%s\" but in a lower order, "+
							"the first will never be matched\n", r1, r2)
					}
					currentRuleNeedBacktrack = false
				}
			}
			for i2, re2 := range rulesRE {
				// especially, if r1 is a subset of other rule, we don't need backtrack
				// because either we turned on ordering
				// or we disabled ordering and can't match it even with backtrack
				if i2 != i1 && re2 != nil && len(re2.FindStringSubmatchIndex(r1)) > 0 {
					currentRuleNeedBacktrack = false
				}
			}
			if currentRuleNeedBacktrack {
				log.Warnf("backtracking required because of match \"%s\", "+
					"matching performance may be degraded\n", r1)
				needBacktrack = true
			}
		}
	}
	if !n.Defaults.GlobDisbleOrdering {
		// backtracking only makes sense when we disbled ordering of rules
		// where transistions are stored in (unordered) map
		// note: don't move this branch to the beginning of this function
		// since we need logs for superset rules
		return true
	} else {
		return needBacktrack
	}
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
		for field, transition := range states[idx].transitions {
			states[len(states)] = transition
			w.WriteString(fmt.Sprintf("%d -> %d  [label = \"%s\"];\n", idx, len(states)-1, field))
			if idx == 0 {
				// color for metric types
				w.WriteString(fmt.Sprintf("%d [color=\"#D6B656\",fillcolor=\"#FFF2CC\"];\n", len(states)-1))
			} else if transition.transitions == nil || len(transition.transitions) == 0 {
				// color for end state
				w.WriteString(fmt.Sprintf("%d [color=\"#82B366\",fillcolor=\"#D5E8D4\"];\n", len(states)-1))
			}
		}
		idx++
	}
	// color for start state
	w.WriteString(fmt.Sprintf("0 [color=\"#a94442\",fillcolor=\"#f2dede\"];\n"))
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
	if m.hasFSM {
		matchFields := strings.Split(statsdMetric, ".")
		root := m.FSM.transitions[string(statsdMetricType)]
		captures := make(map[int]string, len(matchFields))
		captureIdx := 0
		var backtrackCursor *fsmBacktrackStackCursor
		resumeFromBacktrack := false
		var result *MetricMapping
		filedsCount := len(matchFields)
		i := 0
		var state *mappingState
		for {
			for {
				var prs bool
				if !resumeFromBacktrack {
					if len(root.transitions) > 0 {
						field := matchFields[i]
						state, prs = root.transitions[field]
						fieldsLeft := filedsCount - i - 1
						// also compare length upfront to avoid unnecessary loop or backtrack
						if !prs || fieldsLeft > state.maxRemainingLength || fieldsLeft < state.minRemainingLength {
							state, prs = root.transitions["*"]
							if !prs || fieldsLeft > state.maxRemainingLength || fieldsLeft < state.minRemainingLength {
								break
							} else {
								captures[captureIdx] = field
								captureIdx++
							}
						} else if m.FSMNeedsBacktracking {
							altState, prs := root.transitions["*"]
							if !prs || fieldsLeft > altState.maxRemainingLength || fieldsLeft < altState.minRemainingLength {
							} else {
								// push to stack
								newCursor := fsmBacktrackStackCursor{prev: backtrackCursor, state: altState,
									fieldIndex:   i,
									captureIndex: captureIdx, currentCapture: field,
								}
								if backtrackCursor != nil {
									backtrackCursor.next = &newCursor
								}
								backtrackCursor = &newCursor
							}
						}
					} else { // no more transitions for this state
						break
					}
				} // backtrack will resume from here

				// do we reach a final state?
				if state.result != nil && i == filedsCount-1 {
					if m.Defaults.GlobDisbleOrdering {
						result = state.result
						// do a double break
						goto formatLabels
					} else if result == nil || result.priority > state.result.priority {
						// if we care about ordering, try to find a result with highest prioity
						result = state.result
					}
					break
				}

				i++
				if i >= filedsCount {
					break
				}

				resumeFromBacktrack = false
				root = state
			}
			if backtrackCursor == nil {
				// if we are not doing backtracking or all path has been travesaled
				break
			} else {
				// pop one from stack
				state = backtrackCursor.state
				root = state
				i = backtrackCursor.fieldIndex
				captureIdx = backtrackCursor.captureIndex + 1
				// put the * capture back
				captures[captureIdx-1] = backtrackCursor.currentCapture
				backtrackCursor = backtrackCursor.prev
				if backtrackCursor != nil {
					// deref for GC
					backtrackCursor.next = nil
				}
				resumeFromBacktrack = true
			}
		}

	formatLabels:
		// format name and labels
		if result != nil {
			result.Name = formatTemplate(result.NameFormatter, captures)

			labels := prometheus.Labels{}
			for label := range result.Labels {
				labels[label] = formatTemplate(result.LabelsFormatter[label], captures)
			}
			return result, labels, true
		} else if !m.doRegex {
			// if there's no regex match type, return immediately
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
