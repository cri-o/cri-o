package metric

// PrivilegesRequired is metric type for Base Metrics
type PrivilegesRequired int

// Constant of PrivilegesRequired result
const (
	PrivilegesRequiredUnknown PrivilegesRequired = iota
	PrivilegesRequiredHigh
	PrivilegesRequiredLow
	PrivilegesRequiredNone
)

var privilegesRequiredMap = map[PrivilegesRequired]string{
	PrivilegesRequiredHigh: "H",
	PrivilegesRequiredLow:  "L",
	PrivilegesRequiredNone: "N",
}

var privilegesRequiredWithUValueMap = map[PrivilegesRequired]float64{
	PrivilegesRequiredHigh: 0.27,
	PrivilegesRequiredLow:  0.62,
	PrivilegesRequiredNone: 0.85,
}
var privilegesRequiredWithCValueMap = map[PrivilegesRequired]float64{
	PrivilegesRequiredHigh: 0.50,
	PrivilegesRequiredLow:  0.68,
	PrivilegesRequiredNone: 0.85,
}

// GetPrivilegesRequired returns result of PrivilegesRequired metric
func GetPrivilegesRequired(s string) PrivilegesRequired {
	for k, v := range privilegesRequiredMap {
		if s == v {
			return k
		}
	}
	return PrivilegesRequiredUnknown
}

func (pr PrivilegesRequired) String() string {
	if s, ok := privilegesRequiredMap[pr]; ok {
		return s
	}
	return ""
}

// Value returns value of PrivilegesRequired metric
func (pr PrivilegesRequired) Value(s Scope) float64 {
	var m map[PrivilegesRequired]float64
	switch s {
	case ScopeUnchanged:
		m = privilegesRequiredWithUValueMap
	case ScopeChanged:
		m = privilegesRequiredWithCValueMap
	default:
		return 0.0
	}
	if v, ok := m[pr]; ok {
		return v
	}
	return 0.0
}

// IsUnknown returns false if undefined result value of metric
func (pr PrivilegesRequired) IsUnknown() bool {
	return pr == PrivilegesRequiredUnknown
}

/* Copyright 2018-2023 Spiegel
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 * 	http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */
