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
	"strconv"
	"strings"
)

type templateFormatter struct {
	captureIndexes []int
	captureCount   int
	fmtString      string
}

func newTemplateFormatter(valueExpr string, captureCount int) *templateFormatter {
	matches := templateReplaceCaptureRE.FindAllStringSubmatch(valueExpr, -1)
	if len(matches) == 0 {
		// if no regex reference found, keep it as it is
		return &templateFormatter{captureCount: 0, fmtString: valueExpr}
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
	return &templateFormatter{
		captureIndexes: indexes,
		captureCount:   len(indexes),
		fmtString:      valueFormatter,
	}
}

func (formatter *templateFormatter) format(captures map[int]string) string {
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
