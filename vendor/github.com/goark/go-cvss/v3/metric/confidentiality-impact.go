package metric

// ConfidentialityImpact is metric type for Base Metrics
type ConfidentialityImpact int

// Constant of ConfidentialityImpact result
const (
	ConfidentialityImpactUnknown ConfidentialityImpact = iota
	ConfidentialityImpactNone
	ConfidentialityImpactLow
	ConfidentialityImpactHigh
)

var confidentialityImpactMap = map[ConfidentialityImpact]string{
	ConfidentialityImpactNone: "N",
	ConfidentialityImpactLow:  "L",
	ConfidentialityImpactHigh: "H",
}

var confidentialityImpactValueMap = map[ConfidentialityImpact]float64{
	ConfidentialityImpactNone: 0.00,
	ConfidentialityImpactLow:  0.22,
	ConfidentialityImpactHigh: 0.56,
}

// GetConfidentialityImpact returns result of ConfidentialityImpact metric
func GetConfidentialityImpact(s string) ConfidentialityImpact {
	for k, v := range confidentialityImpactMap {
		if s == v {
			return k
		}
	}
	return ConfidentialityImpactUnknown
}

func (ci ConfidentialityImpact) String() string {
	if s, ok := confidentialityImpactMap[ci]; ok {
		return s
	}
	return ""
}

// Value returns value of ConfidentialityImpact metric
func (ci ConfidentialityImpact) Value() float64 {
	if v, ok := confidentialityImpactValueMap[ci]; ok {
		return v
	}
	return 0.0
}

// IsUnknown returns false if undefined result value of metric
func (ci ConfidentialityImpact) IsUnknown() bool {
	return ci == ConfidentialityImpactUnknown
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
