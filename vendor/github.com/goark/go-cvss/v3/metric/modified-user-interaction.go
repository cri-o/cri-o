package metric

import "strings"

//ModifiedUserInteraction is metric type for Base Metrics
type ModifiedUserInteraction int

//Constant of ModifiedUserInteraction result
const (
	ModifiedUserInteractionNotDefined ModifiedUserInteraction = iota
	ModifiedUserInteractionRequired
	ModifiedUserInteractionNone
)

var ModifiedUserInteractionMap = map[ModifiedUserInteraction]string{
	ModifiedUserInteractionNotDefined: "X",
	ModifiedUserInteractionRequired:   "R",
	ModifiedUserInteractionNone:       "N",
}

var ModifiedUserInteractionValueMap = map[ModifiedUserInteraction]float64{
	ModifiedUserInteractionNotDefined: 0,
	ModifiedUserInteractionRequired:   0.62,
	ModifiedUserInteractionNone:       0.85,
}

//GetModifiedUserInteraction returns result of ModifiedUserInteraction metric
func GetModifiedUserInteraction(s string) ModifiedUserInteraction {
	s = strings.ToUpper(s)
	for k, v := range ModifiedUserInteractionMap {
		if s == v {
			return k
		}
	}
	return ModifiedUserInteractionNotDefined
}

func (mui ModifiedUserInteraction) String() string {
	if s, ok := ModifiedUserInteractionMap[mui]; ok {
		return s
	}
	return ""
}

//Value returns value of ModifiedUserInteraction metric
func (mui ModifiedUserInteraction) Value(ui UserInteraction) float64 {
	if mui.String() == ModifiedUserInteractionNotDefined.String() {
		if v, ok := userInteractionValueMap[ui]; ok {
			return v
		}
		return 0.0
	} else {
		if v, ok := ModifiedUserInteractionValueMap[mui]; ok {
			return v
		}
		return 0.0
	}
}

//IsDefined returns false if undefined result value of metric
func (mui ModifiedUserInteraction) IsDefined() bool {
	_, ok := ModifiedUserInteractionValueMap[mui]
	return ok
}

/* Copyright 2022 thejohnbrown */
