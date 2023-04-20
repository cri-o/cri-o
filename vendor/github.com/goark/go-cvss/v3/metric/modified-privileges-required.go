package metric

// ModifiedPrivilegesRequired is metric type for Base Metrics
type ModifiedPrivilegesRequired int

// Constant of ModifiedPrivilegesRequired result
const (
	ModifiedPrivilegesRequiredInvalid ModifiedPrivilegesRequired = iota
	ModifiedPrivilegesRequiredNotDefined
	ModifiedPrivilegesRequiredHigh
	ModifiedPrivilegesRequiredLow
	ModifiedPrivilegesRequiredNone
)

var ModifiedPrivilegesRequiredMap = map[ModifiedPrivilegesRequired]string{
	ModifiedPrivilegesRequiredNotDefined: "X",
	ModifiedPrivilegesRequiredHigh:       "H",
	ModifiedPrivilegesRequiredLow:        "L",
	ModifiedPrivilegesRequiredNone:       "N",
}

var ModifiedPrivilegesRequiredWithUValueMap = map[ModifiedPrivilegesRequired]float64{
	ModifiedPrivilegesRequiredNotDefined: 0,
	ModifiedPrivilegesRequiredHigh:       0.27,
	ModifiedPrivilegesRequiredLow:        0.62,
	ModifiedPrivilegesRequiredNone:       0.85,
}
var ModifiedPrivilegesRequiredWithCValueMap = map[ModifiedPrivilegesRequired]float64{
	ModifiedPrivilegesRequiredNotDefined: 0,
	ModifiedPrivilegesRequiredHigh:       0.50,
	ModifiedPrivilegesRequiredLow:        0.68,
	ModifiedPrivilegesRequiredNone:       0.85,
}

// GetModifiedPrivilegesRequired returns result of ModifiedPrivilegesRequired metric
func GetModifiedPrivilegesRequired(s string) ModifiedPrivilegesRequired {
	for k, v := range ModifiedPrivilegesRequiredMap {
		if s == v {
			return k
		}
	}
	return ModifiedPrivilegesRequiredInvalid
}

func (mpr ModifiedPrivilegesRequired) String() string {
	if s, ok := ModifiedPrivilegesRequiredMap[mpr]; ok {
		return s
	}
	return ""
}

// Value returns value of ModifiedPrivilegesRequired metric
func (mpr ModifiedPrivilegesRequired) Value(ms ModifiedScope, s Scope, pr PrivilegesRequired) float64 {
	if mpr == ModifiedPrivilegesRequiredNotDefined {
		if ms.IsChanged(s) {
			s = ScopeChanged
		} else {
			s = ScopeUnchanged
		}
		return pr.Value(s)
	} else {
		var m map[ModifiedPrivilegesRequired]float64
		if ms.IsChanged(s) {
			m = ModifiedPrivilegesRequiredWithCValueMap
		} else {
			m = ModifiedPrivilegesRequiredWithUValueMap
		}
		if v, ok := m[mpr]; ok {
			return v
		}
	}
	return 0.0
}

// IsDefined returns false if undefined result value of metric
func (mpr ModifiedPrivilegesRequired) IsValid() bool {
	_, ok := ModifiedPrivilegesRequiredWithCValueMap[mpr]
	return ok
}

/* Copyright 2022 thejohnbrown */
/* Contributed by Spiegel, 2023 */
