package metric

import "strings"

//ModifiedAvailabilityImpact is metric type for Base Metrics
type ModifiedAvailabilityImpact int

//Constant of ModifiedAvailabilityImpact result
const (
	ModifiedAvailabilityImpactNotDefined ModifiedAvailabilityImpact = iota
	ModifiedAvailabilityImpactNone
	ModifiedAvailabilityImpactLow
	ModifiedAvailabilityImpactHigh
)

var ModifiedAvailabilityImpactMap = map[ModifiedAvailabilityImpact]string{
	ModifiedAvailabilityImpactNotDefined: "X",
	ModifiedAvailabilityImpactNone:       "N",
	ModifiedAvailabilityImpactLow:        "L",
	ModifiedAvailabilityImpactHigh:       "H",
}

var ModifiedAvailabilityImpactValueMap = map[ModifiedAvailabilityImpact]float64{
	ModifiedAvailabilityImpactNotDefined: 0.00,
	ModifiedAvailabilityImpactNone:       0.00,
	ModifiedAvailabilityImpactLow:        0.22,
	ModifiedAvailabilityImpactHigh:       0.56,
}

//GetModifiedAvailabilityImpact returns result of ModifiedAvailabilityImpact metric
func GetModifiedAvailabilityImpact(s string) ModifiedAvailabilityImpact {
	s = strings.ToUpper(s)
	for k, v := range ModifiedAvailabilityImpactMap {
		if s == v {
			return k
		}
	}
	return ModifiedAvailabilityImpactNotDefined
}

func (mai ModifiedAvailabilityImpact) String() string {
	if s, ok := ModifiedAvailabilityImpactMap[mai]; ok {
		return s
	}
	return ""
}

//Value returns value of ModifiedAvailabilityImpact metric
func (mai ModifiedAvailabilityImpact) Value(ai AvailabilityImpact) float64 {
	if mai.String() == ModifiedAvailabilityImpactNotDefined.String() {
		if v, ok := availabilityImpactValueMap[ai]; ok {
			return v
		}
		return 0.0
	} else {
		if v, ok := ModifiedAvailabilityImpactValueMap[mai]; ok {
			return v
		}
		return 0.0
	}
}

//IsDefined returns false if undefined result value of metric
func (mai ModifiedAvailabilityImpact) IsDefined() bool {
	_, ok := ModifiedAvailabilityImpactValueMap[mai]
	return ok
}

/* Copyright 2022 thejohnbrown */
