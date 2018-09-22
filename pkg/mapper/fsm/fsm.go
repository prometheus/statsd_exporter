package fsm

import (
	"bufio"
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/common/log"
)

var (
	templateReplaceCaptureRE = regexp.MustCompile(`\$\{?([a-zA-Z0-9_\$]+)\}?`)
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
	root              *mappingState
	needsBacktracking bool
	dumpFSMPath       string
	metricTypes       []string
	disableOrdering   bool
	statesCount       int
}

func (fsm *FSM) SetDumpFSMPath(path string) error {
	fsm.dumpFSMPath = path
	return nil
}

func NewFSM(metricTypes []string, maxPossibleTransitions int, disableOrdering bool) *FSM {
	fsm := FSM{}
	root := &mappingState{}
	root.transitions = make(map[string]*mappingState, 3)

	metricTypes = append(metricTypes, "")
	for _, field := range metricTypes {
		state := &mappingState{}
		(*state).transitions = make(map[string]*mappingState, maxPossibleTransitions)
		root.transitions[string(field)] = state
	}
	fsm.disableOrdering = disableOrdering
	fsm.metricTypes = metricTypes
	fsm.statesCount = 0
	fsm.root = root
	return &fsm
}

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

func (f *FSM) DumpFSM() {
	if f.dumpFSMPath == "" {
		return
	}
	log.Infoln("Start dumping FSM to", f.dumpFSMPath)
	idx := 0
	states := make(map[int]*mappingState)
	states[idx] = f.root

	fd, _ := os.Create(f.dumpFSMPath)
	w := bufio.NewWriter(fd)
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

func (f *FSM) TestIfNeedBacktracking(mappings []string) bool {
	needBacktrack := false
	// rule A and B that has same length and
	// A one has * in rules but is not a superset of B makes it needed for backtracking
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
					if !f.disableOrdering && i1 < i2 {
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

	// backtracking will always be needed if ordering of rules is not disabled
	// since transistions are stored in (unordered) map
	// note: don't move this branch to the beginning of this function
	// since we need logs for superset rules
	f.needsBacktracking = !f.disableOrdering || needBacktrack

	return f.needsBacktracking
}

func (f *FSM) GetMapping(statsdMetric string, statsdMetricType string) (interface{}, string, prometheus.Labels, bool) {
	matchFields := strings.Split(statsdMetric, ".")
	currentState := f.root.transitions[statsdMetricType]

	// the cursor/pointer in the backtrack stack
	var backtrackCursor *fsmBacktrackStackCursor
	resumeFromBacktrack := false

	var finalState *mappingState

	captures := make(map[int]string, len(matchFields))
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
					} else if f.needsBacktracking {
						altState, present := currentState.transitions["*"]
						if !present || fieldsLeft > altState.maxRemainingLength || fieldsLeft < altState.minRemainingLength {
						} else {
							// push to backtracking stack
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
				if f.disableOrdering {
					finalState = state
					// do a double break
					goto formatLabels
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

formatLabels:
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
