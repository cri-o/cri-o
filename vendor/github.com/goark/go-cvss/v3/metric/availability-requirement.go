package metric

// AvailabilityRequirement is metric type for Base Metrics
type AvailabilityRequirement int

// Constant of AvailabilityRequirement result
const (
	AvailabilityRequirementInvalid AvailabilityRequirement = iota
	AvailabilityRequirementNotDefined
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

// GetAvailabilityRequirement returns result of AvailabilityRequirement metric
func GetAvailabilityRequirement(s string) AvailabilityRequirement {
	for k, v := range AvailabilityRequirementMap {
		if s == v {
			return k
		}
	}
	return AvailabilityRequirementInvalid
}

func (ar AvailabilityRequirement) String() string {
	if s, ok := AvailabilityRequirementMap[ar]; ok {
		return s
	}
	return ""
}

// Value returns value of AvailabilityRequirement metric
func (ar AvailabilityRequirement) Value() float64 {
	if v, ok := AvailabilityRequirementValueMap[ar]; ok {
		return v
	}
	return 0.0
}

// IsDefined returns false if undefined result value of metric
func (ar AvailabilityRequirement) IsValid() bool {
	_, ok := AvailabilityRequirementValueMap[ar]
	return ok
}

/* Copyright 2022 thejohnbrown */
/* Contributed by Spiegel, 2023 */
