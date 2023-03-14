package metric

import "strings"

//ModifiedAttackVector is metric type for Base Metrics
type ModifiedAttackVector int

//Constant of ModifiedAttackVector result
const (
	ModifiedAttackVectorNotDefined ModifiedAttackVector = iota
	ModifiedAttackVectorPhysical
	ModifiedAttackVectorLocal
	ModifiedAttackVectorAdjacent
	ModifiedAttackVectorNetwork
)

var ModifiedAttackVectorMap = map[ModifiedAttackVector]string{
	ModifiedAttackVectorNotDefined: "X",
	ModifiedAttackVectorPhysical:   "P",
	ModifiedAttackVectorLocal:      "L",
	ModifiedAttackVectorAdjacent:   "A",
	ModifiedAttackVectorNetwork:    "N",
}

var ModifiedAttackVectorValueMap = map[ModifiedAttackVector]float64{
	ModifiedAttackVectorNotDefined: 0,
	ModifiedAttackVectorPhysical:   0.20,
	ModifiedAttackVectorLocal:      0.55,
	ModifiedAttackVectorAdjacent:   0.62,
	ModifiedAttackVectorNetwork:    0.85,
}

//GetModifiedAttackVector returns result of ModifiedAttackVector metric
func GetModifiedAttackVector(s string) ModifiedAttackVector {
	s = strings.ToUpper(s)
	for k, v := range ModifiedAttackVectorMap {
		if s == v {
			return k
		}
	}
	return ModifiedAttackVectorNotDefined
}

func (mav ModifiedAttackVector) String() string {
	if s, ok := ModifiedAttackVectorMap[mav]; ok {
		return s
	}
	return ""
}

//Value returns value of ModifiedAttackVector metric
func (mav ModifiedAttackVector) Value(av AttackVector) float64 {
	if mav.String() == ModifiedAttackVectorNotDefined.String() {
		if v, ok := attackVectorValueMap[av]; ok {
			return v
		}
		return 0.0
	} else {
		if v, ok := ModifiedAttackVectorValueMap[mav]; ok {
			return v
		}
		return 0.0
	}
}

//IsDefined returns false if undefined result value of metric
func (mav ModifiedAttackVector) IsDefined() bool {
	_, ok := ModifiedAttackVectorValueMap[mav]
	return ok
}

/* Copyright 2022 thejohnbrown */
