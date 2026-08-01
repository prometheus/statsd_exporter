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
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var templateReplaceCaptureRE = regexp.MustCompile(`\$\{?([a-zA-Z0-9_\$]+)\}?`)

type TemplateFormatter struct {
	captureIndexes []int
	captureCount   int
	fmtString      string
}

// NewTemplateFormatter instantiates a TemplateFormatter
// from given template string and the maximum amount of captures.
// captureCount is the number of captures in the match, not counting $0.
func NewTemplateFormatter(template string, captureCount int) *TemplateFormatter {
	matches := templateReplaceCaptureRE.FindAllStringSubmatch(template, -1)
	if len(matches) == 0 {
		// if no regex reference found, keep it as it is
		return &TemplateFormatter{captureCount: 0, fmtString: template}
	}

	var indexes []int
	valueFormatter := template
	for _, match := range matches {
		idx, err := strconv.Atoi(match[len(match)-1])
		if err != nil || idx > captureCount || idx < 0 {
			// if index larger than captured count or using unsupported named capture group,
			// replace with empty string
			valueFormatter = strings.ReplaceAll(valueFormatter, match[0], "")
		} else {
			valueFormatter = strings.ReplaceAll(valueFormatter, match[0], "%s")
			// $0 is the whole metric name and the captures follow it, so the
			// reference variable $? indexes directly into the captures passed
			// to Format.
			indexes = append(indexes, idx)
		}
	}
	return &TemplateFormatter{
		captureIndexes: indexes,
		captureCount:   len(indexes),
		fmtString:      valueFormatter,
	}
}

// Format accepts a list containing the whole metric name followed by the
// captured strings, and returns the formatted string using the template stored
// in current TemplateFormatter. The metric name is at index 0, so it is
// referenced as $0, matching the behaviour of the regex match type.
func (formatter *TemplateFormatter) Format(captures []string) string {
	if formatter.captureCount == 0 {
		// no label substitution, keep as it is
		return formatter.fmtString
	}
	indexes := formatter.captureIndexes
	vargs := make([]interface{}, formatter.captureCount)
	for i, idx := range indexes {
		vargs[i] = captures[idx]
	}
	return fmt.Sprintf(formatter.fmtString, vargs...)
}
