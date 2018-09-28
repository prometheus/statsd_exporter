// Copyright 2018 The Prometheus Authors
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

package fsm

import (
	"regexp"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
)

type mappingState struct {
	transitions        map[string]*mappingState
	minRemainingLength int
	maxRemainingLength int
	// result* members are nil unless there's a metric ends with this state
	result                interface{}
	resultPriority        int
	resultNameFormatter   *templateFormatter
	resultLabelsFormatter map[string]*templateFormatter
}

type fsmBacktrackStackCursor struct {
	fieldIndex     int
	captureIndex   int
	currentCapture string
	state          *mappingState
	prev           *fsmBacktrackStackCursor
	next           *fsmBacktrackStackCursor
}

type FSM struct {
	root               *mappingState
	metricTypes        []string
	statesCount        int
	BacktrackingNeeded bool
	OrderingDisabled   bool
}

// NewFSM creates a new FSM instance
func NewFSM(metricTypes []string, maxPossibleTransitions int, orderingDisabled bool) *FSM {
	fsm := FSM{}
	root := &mappingState{}
	root.transitions = make(map[string]*mappingState, len(metricTypes))

	metricTypes = append(metricTypes, "")
	for _, field := range metricTypes {
		state := &mappingState{}
		(*state).transitions = make(map[string]*mappingState, maxPossibleTransitions)
		root.transitions[string(field)] = state
	}
	fsm.OrderingDisabled = orderingDisabled
	fsm.metricTypes = metricTypes
	fsm.statesCount = 0
	fsm.root = root
	return &fsm
}

// AddState adds a state into the existing FSM
func (f *FSM) AddState(match string, name string, labels prometheus.Labels, matchMetricType string,
	maxPossibleTransitions int, result interface{}) {
	// first split by "."
	matchFields := strings.Split(match, ".")
	// fill into our FSM
	roots := []*mappingState{}
	if matchMetricType == "" {
		// if metricType not specified, connect the state from all three types
		for _, metricType := range f.metricTypes {
			roots = append(roots, f.root.transitions[string(metricType)])
		}
	} else {
		roots = append(roots, f.root.transitions[matchMetricType])
	}
	var captureCount int
	var finalStates []*mappingState
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
					root.transitions[field].result = result
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
		finalStates = append(finalStates, root)
	}
	nameFmt := newTemplateFormatter(name, captureCount)

	currentLabelFormatter := make(map[string]*templateFormatter, captureCount)
	for label, valueExpr := range labels {
		lblFmt := newTemplateFormatter(valueExpr, captureCount)
		currentLabelFormatter[label] = lblFmt
	}

	for _, state := range finalStates {
		state.resultNameFormatter = nameFmt
		state.resultLabelsFormatter = currentLabelFormatter
		state.resultPriority = f.statesCount
	}

	f.statesCount++

}

// GetMapping implements a mapping algorithm for Glob pattern
func (f *FSM) GetMapping(statsdMetric string, statsdMetricType string) (interface{}, string, prometheus.Labels, bool) {
	matchFields := strings.Split(statsdMetric, ".")
	currentState := f.root.transitions[statsdMetricType]

	// the cursor/pointer in the backtrack stack implemented as a double-linked list
	var backtrackCursor *fsmBacktrackStackCursor
	resumeFromBacktrack := false

	// the return variable
	var finalState *mappingState

	captures := make(map[int]string, len(matchFields))
	// keep track of captured group so we don't need to do append() on captures
	captureIdx := 0
	filedsCount := len(matchFields)
	i := 0
	var state *mappingState
	for { // the loop for backtracking
		for { // the loop for a single "depth only" search
			var present bool
			// if we resume from backtrack, we should skip this branch in this case
			// since the state that were saved at the end of this branch
			if !resumeFromBacktrack {
				if len(currentState.transitions) > 0 {
					field := matchFields[i]
					state, present = currentState.transitions[field]
					fieldsLeft := filedsCount - i - 1
					// also compare length upfront to avoid unnecessary loop or backtrack
					if !present || fieldsLeft > state.maxRemainingLength || fieldsLeft < state.minRemainingLength {
						state, present = currentState.transitions["*"]
						if !present || fieldsLeft > state.maxRemainingLength || fieldsLeft < state.minRemainingLength {
							break
						} else {
							captures[captureIdx] = field
							captureIdx++
						}
					} else if f.BacktrackingNeeded {
						// if backtracking is needed, also check for alternative transition
						altState, present := currentState.transitions["*"]
						if !present || fieldsLeft > altState.maxRemainingLength || fieldsLeft < altState.minRemainingLength {
						} else {
							// push to backtracking stack
							newCursor := fsmBacktrackStackCursor{prev: backtrackCursor, state: altState,
								fieldIndex:   i,
								captureIndex: captureIdx, currentCapture: field,
							}
							// if this is not the first time, connect to the previous cursor
							if backtrackCursor != nil {
								backtrackCursor.next = &newCursor
							}
							backtrackCursor = &newCursor
						}
					}
				} else {
					// no more transitions for this state
					break
				}
			} // backtrack will resume from here

			// do we reach a final state?
			if state.result != nil && i == filedsCount-1 {
				if f.OrderingDisabled {
					finalState = state
					return formatLabels(finalState, captures)
				} else if finalState == nil || finalState.resultPriority > state.resultPriority {
					// if we care about ordering, try to find a result with highest prioity
					finalState = state
				}
				break
			}

			i++
			if i >= filedsCount {
				break
			}

			resumeFromBacktrack = false
			currentState = state
		}
		if backtrackCursor == nil {
			// if we are not doing backtracking or all path has been travesaled
			break
		} else {
			// pop one from stack
			state = backtrackCursor.state
			currentState = state
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

	return formatLabels(finalState, captures)
}

// TestIfNeedBacktracking test if backtrack is needed for current mappings
func TestIfNeedBacktracking(mappings []string, orderingDisabled bool) bool {
	backtrackingNeeded := false
	// A has * in rules there's other transisitions at the same state
	// this makes A the cause of backtracking
	ruleByLength := make(map[int][]string)
	ruleREByLength := make(map[int][]*regexp.Regexp)

	// first sort rules by length
	for _, mapping := range mappings {
		l := len(strings.Split(mapping, "."))
		ruleByLength[l] = append(ruleByLength[l], mapping)

		metricRe := strings.Replace(mapping, ".", "\\.", -1)
		metricRe = strings.Replace(metricRe, "*", "([^.]*)", -1)
		regex, err := regexp.Compile("^" + metricRe + "$")
		if err != nil {
			log.Warnf("invalid match %s. cannot compile regex in mapping: %v", mapping, err)
		}
		// put into array no matter there's error or not, we will skip later if regex is nil
		ruleREByLength[l] = append(ruleREByLength[l], regex)
	}

	for l, rules := range ruleByLength {
		if len(rules) == 1 {
			continue
		}
		rulesRE := ruleREByLength[l]
		for i1, r1 := range rules {
			currentRuleNeedBacktrack := false
			re1 := rulesRE[i1]
			if re1 == nil || strings.Index(r1, "*") == -1 {
				continue
			}
			// if a rule is A.B.C.*.E.*, is there a rule A.B.C.D.x.x or A.B.C.*.E.F? (x is any string or *)
			for index := 0; index < len(r1); index++ {
				if r1[index] != '*' {
					continue
				}
				reStr := strings.Replace(r1[:index], ".", "\\.", -1)
				reStr = strings.Replace(reStr, "*", "\\*", -1)
				re := regexp.MustCompile("^" + reStr)
				for i2, r2 := range rules {
					if i2 == i1 {
						continue
					}
					if len(re.FindStringSubmatchIndex(r2)) > 0 {
						currentRuleNeedBacktrack = true
						break
					}
				}
			}

			for i2, r2 := range rules {
				if i2 != i1 && len(re1.FindStringSubmatchIndex(r2)) > 0 {
					// log if we care about ordering and the superset occurs before
					if !orderingDisabled && i1 < i2 {
						log.Warnf("match \"%s\" is a super set of match \"%s\" but in a lower order, "+
							"the first will never be matched", r1, r2)
					}
					currentRuleNeedBacktrack = false
				}
			}
			for i2, re2 := range rulesRE {
				if i2 == i1 || re2 == nil {
					continue
				}
				// if r1 is a subset of other rule, we don't need backtrack
				// because either we turned on ordering
				// or we disabled ordering and can't match it even with backtrack
				if len(re2.FindStringSubmatchIndex(r1)) > 0 {
					currentRuleNeedBacktrack = false
				}
			}

			if currentRuleNeedBacktrack {
				log.Warnf("backtracking required because of match \"%s\", "+
					"matching performance may be degraded", r1)
				backtrackingNeeded = true
			}
		}
	}

	// backtracking will always be needed if ordering of rules is not disabled
	// since transistions are stored in (unordered) map
	// note: don't move this branch to the beginning of this function
	// since we need logs for superset rules

	return !orderingDisabled || backtrackingNeeded
}

func formatLabels(finalState *mappingState, captures map[int]string) (interface{}, string, prometheus.Labels, bool) {
	// format name and labels
	if finalState != nil {
		name := finalState.resultNameFormatter.format(captures)

		labels := prometheus.Labels{}
		for key, formatter := range finalState.resultLabelsFormatter {
			labels[key] = formatter.format(captures)
		}
		return finalState.result, name, labels, true
	}
	return nil, "", nil, false
}
