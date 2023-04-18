package metric

// Scope is metric type for Base Metrics
type Scope int

// Constant of Scope result
const (
	ScopeUnknown Scope = iota
	ScopeUnchanged
	ScopeChanged
)

var scopeMap = map[Scope]string{
	ScopeUnchanged: "U",
	ScopeChanged:   "C",
}

// GetScope returns result of Scope metric
func GetScope(s string) Scope {
	for k, v := range scopeMap {
		if s == v {
			return k
		}
	}
	return ScopeUnknown
}

// IsChanged returns true if Scope value is ScopeChanged.
func (sc Scope) IsChanged() bool {
	return sc == ScopeChanged
}

func (sc Scope) String() string {
	if s, ok := scopeMap[sc]; ok {
		return s
	}
	return ""
}

// IsUnknown returns false if undefined result value of metric
func (sc Scope) IsUnknown() bool {
	return sc == ScopeUnknown
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
