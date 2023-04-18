package metric

import (
	"fmt"
	"math"
	"strings"

	"github.com/goark/errs"
	"github.com/goark/go-cvss/cvsserr"
)

const (
	metricCR  = "CR"
	metricIR  = "IR"
	metricAR  = "AR"
	metricMAV = "MAV"
	metricMAC = "MAC"
	metricMPR = "MPR"
	metricMUI = "MUI"
	metricMS  = "MS"
	metricMC  = "MC"
	metricMI  = "MI"
	metricMA  = "MA"
)

// Base is Environmental Metrics for CVSSv3
type Environmental struct {
	*Temporal
	CR    ConfidentialityRequirement
	IR    IntegrityRequirement
	AR    AvailabilityRequirement
	MAV   ModifiedAttackVector
	MAC   ModifiedAttackComplexity
	MPR   ModifiedPrivilegesRequired
	MUI   ModifiedUserInteraction
	MS    ModifiedScope
	MC    ModifiedConfidentialityImpact
	MI    ModifiedIntegrityImpact
	MA    ModifiedAvailabilityImpact
	names map[string]bool
}

// NewBase returns Environmental Metrics instance
func NewEnvironmental() *Environmental {
	return &Environmental{
		Temporal: NewTemporal(),
		CR:       ConfidentialityRequirementNotDefined,
		IR:       IntegrityRequirementNotDefined,
		AR:       AvailabilityRequirementNotDefined,
		MAV:      ModifiedAttackVectorNotDefined,
		MAC:      ModifiedAttackComplexityNotDefined,
		MPR:      ModifiedPrivilegesRequiredNotDefined,
		MUI:      ModifiedUserInteractionNotDefined,
		MS:       ModifiedScopeNotDefined,
		MC:       ModifiedConfidentialityImpactNotDefined,
		MI:       ModifiedIntegrityImpactNotDefined,
		MA:       ModifiedAvailabilityImpactNotDefined,
		names:    map[string]bool{},
	}
}

func (em *Environmental) Decode(vector string) (*Environmental, error) {
	if em == nil {
		em = NewEnvironmental()
	}
	values := strings.Split(vector, "/")
	//CVSS version
	ver, err := GetVersion(values[0])
	if err != nil {
		return nil, errs.Wrap(err, errs.WithContext("vector", vector))
	}
	if ver == VUnknown {
		return nil, errs.Wrap(cvsserr.ErrNotSupportVer, errs.WithContext("vector", vector))
	}
	em.Ver = ver
	//parse vector
	var lastErr error
	for _, value := range values[1:] {
		if err := em.decodeOne(value); err != nil {
			if !errs.Is(err, cvsserr.ErrNotSupportMetric) {
				return nil, errs.Wrap(err, errs.WithContext("vector", vector))
			}
			lastErr = err
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	if err := em.GetError(); err != nil {
		return nil, err
	}
	return em, nil
}
func (em *Environmental) decodeOne(str string) error {
	if err := em.Temporal.decodeOne(str); err != nil {
		if !errs.Is(err, cvsserr.ErrNotSupportMetric) {
			return errs.Wrap(err, errs.WithContext("metric", str))
		}
	} else {
		return nil
	}
	m := strings.Split(str, ":")
	if len(m) != 2 || len(m[0]) == 0 || len(m[1]) == 0 {
		return errs.Wrap(cvsserr.ErrInvalidVector, errs.WithContext("metric", str))
	}
	name := m[0]
	if em.names[name] {
		return errs.Wrap(cvsserr.ErrSameMetric, errs.WithContext("metric", str))
	}
	switch name {
	case metricCR: //ConfidentialityRequirement
		em.CR = GetConfidentialityRequirement(m[1])
		if em.CR == ConfidentialityRequirementInvalid {
			return errs.Wrap(cvsserr.ErrInvalidValue, errs.WithContext("metric", str))
		}
	case metricIR: //IntegrityRequirement
		em.IR = GetIntegrityRequirement(m[1])
		if em.IR == IntegrityRequirementInvalid {
			return errs.Wrap(cvsserr.ErrInvalidValue, errs.WithContext("metric", str))
		}
	case metricAR: //AvailabilityRequirement
		em.AR = GetAvailabilityRequirement(m[1])
		if em.AR == AvailabilityRequirementInvalid {
			return errs.Wrap(cvsserr.ErrInvalidValue, errs.WithContext("metric", str))
		}
	case metricMAV: //ModifiedAttackVector
		em.MAV = GetModifiedAttackVector(m[1])
		if em.MAV == ModifiedAttackVectorInvalid {
			return errs.Wrap(cvsserr.ErrInvalidValue, errs.WithContext("metric", str))
		}
	case metricMAC: //ModifiedAttackComplexity
		em.MAC = GetModifiedAttackComplexity(m[1])
		if em.MAC == ModifiedAttackComplexityInvalid {
			return errs.Wrap(cvsserr.ErrInvalidValue, errs.WithContext("metric", str))
		}
	case metricMPR: //ModifiedPrivilegesRequired
		em.MPR = GetModifiedPrivilegesRequired(m[1])
		if em.MPR == ModifiedPrivilegesRequiredInvalid {
			return errs.Wrap(cvsserr.ErrInvalidValue, errs.WithContext("metric", str))
		}
	case metricMUI: //ModifiedUserInteraction
		em.MUI = GetModifiedUserInteraction(m[1])
		if em.MUI == ModifiedUserInteractionInvalid {
			return errs.Wrap(cvsserr.ErrInvalidValue, errs.WithContext("metric", str))
		}
	case metricMS: //ModifiedScope
		em.MS = GetModifiedScope(m[1])
		if em.MS == ModifiedScopeInvalid {
			return errs.Wrap(cvsserr.ErrInvalidValue, errs.WithContext("metric", str))
		}
	case metricMC: //ModifiedConfidentialityImpact
		em.MC = GetModifiedConfidentialityImpact(m[1])
		if em.MC == ModifiedConfidentialityImpactInvalid {
			return errs.Wrap(cvsserr.ErrInvalidValue, errs.WithContext("metric", str))
		}
	case metricMI: //ModifiedIntegrityImpact
		em.MI = GetModifiedIntegrityImpact(m[1])
		if em.MI == ModifiedIntegrityImpactInvalid {
			return errs.Wrap(cvsserr.ErrInvalidValue, errs.WithContext("metric", str))
		}
	case metricMA: //ModifiedAvailabilityImpact
		em.MA = GetModifiedAvailabilityImpact(m[1])
		if em.MA == ModifiedAvailabilityInvalid {
			return errs.Wrap(cvsserr.ErrInvalidValue, errs.WithContext("metric", str))
		}
	default:
		return errs.Wrap(cvsserr.ErrNotSupportMetric, errs.WithContext("metric", str))
	}
	em.names[name] = true
	return nil
}

// GetError returns error instance if undefined metric
func (em *Environmental) GetError() error {
	if em == nil {
		return errs.Wrap(cvsserr.ErrNoEnvironmentalMetrics)
	}
	if err := em.Temporal.GetError(); err != nil {
		return errs.Wrap(err)
	}
	switch true {
	case !em.CR.IsValid(), !em.IR.IsValid(), !em.AR.IsValid(), !em.MAV.IsValid(), !em.MAC.IsValid(), !em.MPR.IsValid(), !em.MUI.IsValid(),
		!em.MS.IsValid(), !em.MC.IsValid(), !em.MI.IsValid(), !em.MA.IsValid():
		return errs.Wrap(cvsserr.ErrInvalidValue)
	default:
		return nil
	}
}

// Encode returns CVSSv3 vector string
func (em *Environmental) Encode() (string, error) {
	if em == nil {
		return "", errs.Wrap(cvsserr.ErrNoEnvironmentalMetrics)
	}
	if err := em.GetError(); err != nil {
		return "", errs.Wrap(err)
	}
	ts, _ := em.Temporal.Encode()
	r := &strings.Builder{}
	r.WriteString(ts)                                       //Vector of Temporal metrics
	r.WriteString(fmt.Sprintf("/%v:%v", metricCR, em.CR))   //Confidentiality Requirement
	r.WriteString(fmt.Sprintf("/%v:%v", metricIR, em.IR))   //Integrity Requirement
	r.WriteString(fmt.Sprintf("/%v:%v", metricAR, em.AR))   //Availability Requirement
	r.WriteString(fmt.Sprintf("/%v:%v", metricMAV, em.MAV)) //Modified Attack Vector
	r.WriteString(fmt.Sprintf("/%v:%v", metricMAC, em.MAC)) //Modified Attack Complexity
	r.WriteString(fmt.Sprintf("/%v:%v", metricMPR, em.MPR)) //Modified Privileges Required
	r.WriteString(fmt.Sprintf("/%v:%v", metricMUI, em.MUI)) //Modified User Interaction
	r.WriteString(fmt.Sprintf("/%v:%v", metricMS, em.MS))   //Modified Scope
	r.WriteString(fmt.Sprintf("/%v:%v", metricMC, em.MC))   //Modified Confidentiality Impact
	r.WriteString(fmt.Sprintf("/%v:%v", metricMI, em.MI))   //Modified Integrity Impact
	r.WriteString(fmt.Sprintf("/%v:%v", metricMA, em.MA))   //Modified Availability Impact
	return r.String(), em.GetError()
}

// String is stringer method.
func (em *Environmental) String() string {
	s, _ := em.Encode()
	return s
}

// Score returns score of Environmental metrics
func (em *Environmental) Score() float64 {
	if err := em.GetError(); err != nil {
		return 0.0
	}
	ModifiedImpactSubScore := math.Min(1-((1-em.CR.Value()*em.MC.Value(em.C))*(1-em.IR.Value()*em.MI.Value(em.I))*(1-em.AR.Value()*em.MA.Value(em.A))), 0.915)
	changes := em.MS.IsChanged(em.S)
	var ModifiedImpact float64
	if changes {
		if em.Ver == V3_1 {
			ModifiedImpact = 7.52*(ModifiedImpactSubScore-0.029) - 3.25*math.Pow(ModifiedImpactSubScore*0.9731-0.02, 13)
		} else {
			ModifiedImpact = 7.52*(ModifiedImpactSubScore-0.029) - 3.25*math.Pow(ModifiedImpactSubScore-0.02, 15)
		}
	} else {
		ModifiedImpact = 6.42 * ModifiedImpactSubScore
	}
	if ModifiedImpact <= 0 {
		return 0.0
	}

	ModifiedExploitability := 8.22 * em.MAV.Value(em.AV) * em.MAC.Value(em.AC) * em.MPR.Value(em.MS, em.S, em.PR) * em.MUI.Value(em.UI)

	if changes {
		return roundUp(roundUp(math.Min(1.08*(ModifiedImpact+ModifiedExploitability), 10)) * em.E.Value() * em.RL.Value() * em.RC.Value())
	}
	return roundUp(roundUp(math.Min((ModifiedImpact+ModifiedExploitability), 10)) * em.E.Value() * em.RL.Value() * em.RC.Value())
}

// Severity returns severity by score of Environmental metrics
func (em *Environmental) Severity() Severity {
	return severity(em.Score())
}

// BaseMetrics returns Base metrics in Environmental metrics instance
func (em *Environmental) BaseMetrics() *Base {
	if em == nil {
		return nil
	}
	return em.Base
}

// TemporalMetrics returns Temporal metrics in Environmental metrics instance
func (em *Environmental) TemporalMetrics() *Temporal {
	if em == nil {
		return nil
	}
	return em.Temporal
}

/* Copyright 2022 thejohnbrown */
/* Contributed by Spiegel, 2023 */
