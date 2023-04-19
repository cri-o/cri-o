package metric

// ConfidentialityRequirement is metric type for Base Metrics
type ConfidentialityRequirement int

// Constant of ConfidentialityRequirement result
const (
	ConfidentialityRequirementInvalid ConfidentialityRequirement = iota
	ConfidentialityRequirementNotDefined
	ConfidentialityRequirementLow
	ConfidentialityRequirementMedium
	ConfidentialityRequirementHigh
)

var ConfidentialityRequirementMap = map[ConfidentialityRequirement]string{
	ConfidentialityRequirementNotDefined: "X",
	ConfidentialityRequirementLow:        "L",
	ConfidentialityRequirementMedium:     "M",
	ConfidentialityRequirementHigh:       "H",
}

var ConfidentialityRequirementValueMap = map[ConfidentialityRequirement]float64{
	ConfidentialityRequirementNotDefined: 1,
	ConfidentialityRequirementLow:        0.5,
	ConfidentialityRequirementMedium:     1,
	ConfidentialityRequirementHigh:       1.5,
}

// GetConfidentialityRequirement returns result of ConfidentalityRequirement metric
func GetConfidentialityRequirement(s string) ConfidentialityRequirement {
	for k, v := range ConfidentialityRequirementMap {
		if s == v {
			return k
		}
	}
	return ConfidentialityRequirementInvalid
}

func (cr ConfidentialityRequirement) String() string {
	if s, ok := ConfidentialityRequirementMap[cr]; ok {
		return s
	}
	return ""
}

// Value returns value of ConfidentialityRequirement metric
func (cr ConfidentialityRequirement) Value() float64 {
	if v, ok := ConfidentialityRequirementValueMap[cr]; ok {
		return v
	}
	return 0.0
}

// IsDefined returns false if undefined result value of metric
func (cr ConfidentialityRequirement) IsValid() bool {
	_, ok := ConfidentialityRequirementValueMap[cr]
	return ok
}

/* Copyright 2022 thejohnbrown */
/* Contributed by Spiegel, 2023 */
