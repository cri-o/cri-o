package metric

import "strings"

//ModifiedScope is metric type for Base Metrics
type ModifiedScope int

//Constant of ModifiedScope result
const (
	ModifiedScopeNotDefined ModifiedScope = iota
	ModifiedScopeUnchanged
	ModifiedScopeChanged
)

var ModifiedScopeValueMap = map[ModifiedScope]string{
	ModifiedScopeNotDefined: "X",
	ModifiedScopeUnchanged:  "U",
	ModifiedScopeChanged:    "C",
}

//GetModifiedScope returns result of ModifiedScope metric
func GetModifiedScope(s string) ModifiedScope {
	s = strings.ToUpper(s)
	for k, v := range ModifiedScopeValueMap {
		if s == v {
			return k
		}
	}
	return ModifiedScopeNotDefined
}

func (msc ModifiedScope) String() string {
	if s, ok := ModifiedScopeValueMap[msc]; ok {
		return s
	}
	return ""
}

//IsDefined returns false if undefined result value of metric
func (msc ModifiedScope) IsDefined() bool {
	_, ok := ModifiedScopeValueMap[msc]
	return ok
}

/* Copyright 2022 thejohnbrown */
