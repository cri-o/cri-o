package metric

import (
	"strings"
)

//ModifiedPrivilegesRequired is metric type for Base Metrics
type ModifiedPrivilegesRequired int

//Constant of ModifiedPrivilegesRequired result
const (
	ModifiedPrivilegesRequiredNotDefined ModifiedPrivilegesRequired = iota
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

//GetModifiedPrivilegesRequired returns result of ModifiedPrivilegesRequired metric
func GetModifiedPrivilegesRequired(s string) ModifiedPrivilegesRequired {
	s = strings.ToUpper(s)
	for k, v := range ModifiedPrivilegesRequiredMap {
		if s == v {
			return k
		}
	}
	return ModifiedPrivilegesRequiredNotDefined
}

func (mpr ModifiedPrivilegesRequired) String() string {
	if s, ok := ModifiedPrivilegesRequiredMap[mpr]; ok {
		return s
	}
	return ""
}

//Value returns value of ModifiedPrivilegesRequired metric
func (mpr ModifiedPrivilegesRequired) Value(ms ModifiedScope, s Scope, pr PrivilegesRequired) float64 {
	var m map[ModifiedPrivilegesRequired]float64
	if mpr.String() == ModifiedPrivilegesRequiredNotDefined.String() {
		switch s {
		case ScopeUnchanged:
			if v, ok := privilegesRequiredWithUValueMap[pr]; ok {
				return v
			}
		case ScopeChanged:
			if v, ok := privilegesRequiredWithCValueMap[pr]; ok {
				return v
			}
		}
	} else {
		switch ms {
		case ModifiedScopeUnchanged:
			m = ModifiedPrivilegesRequiredWithUValueMap
		case ModifiedScopeChanged:
			m = ModifiedPrivilegesRequiredWithCValueMap
		case ModifiedScopeNotDefined:
			if s == ScopeUnchanged {
				m = ModifiedPrivilegesRequiredWithUValueMap
			} else {
				m = ModifiedPrivilegesRequiredWithCValueMap
			}
		default:
			return 0.0
		}
		if v, ok := m[mpr]; ok {
			return v
		}

	}
	return 0.0
}

//IsDefined returns false if undefined result value of metric
func (mpr ModifiedPrivilegesRequired) IsDefined() bool {
	_, ok := ModifiedPrivilegesRequiredWithCValueMap[mpr]
	return ok
}

/* Copyright 2022 thejohnbrown */
