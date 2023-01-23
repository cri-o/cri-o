package metric

import "strings"

//AvailabilityRequirement is metric type for Base Metrics
type AvailabilityRequirement int

//Constant of AvailabilityRequirement result
const (
	AvailabilityRequirementNotDefined AvailabilityRequirement = iota
	AvailabilityRequirementLow
	AvailabilityRequirementMedium
	AvailabilityRequirementHigh
)

var AvailabilityRequirementMap = map[AvailabilityRequirement]string{
	AvailabilityRequirementNotDefined: "X",
	AvailabilityRequirementLow:        "L",
	AvailabilityRequirementMedium:     "M",
	AvailabilityRequirementHigh:       "H",
}

var AvailabilityRequirementValueMap = map[AvailabilityRequirement]float64{
	AvailabilityRequirementNotDefined: 1,
	AvailabilityRequirementLow:        0.5,
	AvailabilityRequirementMedium:     1,
	AvailabilityRequirementHigh:       1.5,
}

//GetAvailabilityRequirement returns result of AvailabilityRequirement metric
func GetAvailabilityRequirement(s string) AvailabilityRequirement {
	s = strings.ToUpper(s)
	for k, v := range AvailabilityRequirementMap {
		if s == v {
			return k
		}
	}
	return AvailabilityRequirementNotDefined
}

func (ar AvailabilityRequirement) String() string {
	if s, ok := AvailabilityRequirementMap[ar]; ok {
		return s
	}
	return ""
}

//Value returns value of AvailabilityRequirement metric
func (ar AvailabilityRequirement) Value() float64 {
	if v, ok := AvailabilityRequirementValueMap[ar]; ok {
		return v
	}
	return 0.0
}

//IsDefined returns false if undefined result value of metric
func (ar AvailabilityRequirement) IsDefined() bool {
	_, ok := AvailabilityRequirementValueMap[ar]
	return ok
}

/* Copyright 2022 thejohnbrown */
