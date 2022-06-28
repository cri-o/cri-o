/*
Copyright 2022 The Kubernetes Authors.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package query

import (
	"fmt"
	"strconv"
	"strings"
)

type Expression struct {
	Filters []Filter
}

func NewExpression(exp string) (*Expression, error) {
	return parseExpression(exp)
}

func tokenizeExpression(expString string) []string {
	quoted := false
	return strings.FieldsFunc(expString, func(r rune) bool {
		if r == '"' {
			quoted = !quoted
		}
		return !quoted && r == ' '
	})
}

func scanToken(token string) (label, value string) {
	parts := strings.SplitN(token, ":", 2)
	if len(parts) == 1 {
		return parts[0], ""
	}
	if strings.HasPrefix(parts[1], `"`) && strings.HasSuffix(parts[1], `"`) {
		parts[1] = strings.TrimPrefix(strings.TrimSuffix(parts[1], `"`), `"`)
	}
	return parts[0], parts[1]
}

func parseExpression(expString string) (*Expression, error) {
	tokens := tokenizeExpression(expString)
	exp := &Expression{
		Filters: []Filter{},
	}
	for _, token := range tokens {
		label, data := scanToken(token)
		switch label {
		case "all":
			exp.Filters = append(exp.Filters, &AllFilter{})
		case "name":
			exp.Filters = append(exp.Filters, &NameFilter{Pattern: data})
		case "depth":
			i, err := strconv.Atoi(data)
			if err != nil {
				return nil, fmt.Errorf("checking value for depth filter: %w", err)
			}
			exp.Filters = append(exp.Filters, &DepthFilter{
				TargetDepth: i,
			})
		case "purl":
			exp.Filters = append(exp.Filters, &PurlFilter{Pattern: data})
		default:
			return nil, fmt.Errorf("unknown filter: %s", label)
		}
	}
	return exp, nil
}
