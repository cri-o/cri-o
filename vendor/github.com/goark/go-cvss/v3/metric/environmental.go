package metric

import (
	"math"
	"strings"

	"github.com/goark/errs"
	"github.com/goark/go-cvss/cvsserr"
)

//Base is Environmental Metrics for CVSSv3
type Environmental struct {
	*Temporal
	CR  ConfidentialityRequirement
	IR  IntegrityRequirement
	AR  AvailabilityRequirement
	MAV ModifiedAttackVector
	MAC ModifiedAttackComplexity
	MPR ModifiedPrivilegesRequired
	MUI ModifiedUserInteraction
	MS  ModifiedScope
	MC  ModifiedConfidentialityImpact
	MI  ModifiedIntegrityImpact
	MA  ModifiedAvailabilityImpact
}

//NewBase returns Base Metrics instance
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
	}
}

func (em *Environmental) Decode(vector string) (*Environmental, error) {
	if em == nil {
		em = NewEnvironmental()
	}
	values := strings.Split(vector, "/")
	if len(values) < 9 { // E, RL, RC metrics are optional.
		return em, errs.Wrap(cvsserr.ErrInvalidVector, errs.WithContext("vector", vector))
	}
	//CVSS version
	ver, err := GetVersion(values[0])
	if err != nil {
		return em, errs.Wrap(err, errs.WithContext("vector", vector))
	}
	if ver == VUnknown {
		return em, errs.Wrap(cvsserr.ErrNotSupportVer, errs.WithContext("vector", vector))
	}
	em.Ver = ver
	//parse vector
	var lastErr error
	for _, value := range values[1:] {
		if err := em.decodeOne(value); err != nil {
			if !errs.Is(err, cvsserr.ErrNotSupportMetric) {
				return em, errs.Wrap(err, errs.WithContext("vector", vector))
			}
			lastErr = err
		}
	}
	if lastErr != nil {
		return em, lastErr
	}
	return em, em.GetError()
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
	if len(m) != 2 {
		return errs.Wrap(cvsserr.ErrInvalidVector, errs.WithContext("metric", str))
	}
	switch strings.ToUpper(m[0]) {
	case "CR": //Exploitability
		em.CR = GetConfidentialityRequirement(m[1])
	case "IR": //RemediationLevel
		em.IR = GetIntegrityRequirement(m[1])
	case "AR": //RemediationLevel
		em.AR = GetAvailabilityRequirement(m[1])
	case "MAV": //RemediationLevel
		em.MAV = GetModifiedAttackVector(m[1])
	case "MAC": //RemediationLevel
		em.MAC = GetModifiedAttackComplexity(m[1])
	case "MPR": //RemediationLevel
		em.MPR = GetModifiedPrivilegesRequired(m[1])
	case "MUI": //RemediationLevel
		em.MUI = GetModifiedUserInteraction(m[1])
	case "MS": //RemediationLevel
		em.MS = GetModifiedScope(m[1])
	case "MC": //RemediationLevel
		em.MC = GetModifiedConfidentialityImpact(m[1])
	case "MI": //RemediationLevel
		em.MI = GetModifiedIntegrityImpact(m[1])
	case "MA": //RemediationLevel
		em.MA = GetModifiedAvailabilityImpact(m[1])
	default:
		return errs.Wrap(cvsserr.ErrNotSupportMetric, errs.WithContext("metric", str))
	}
	return nil
}

//GetError returns error instance if undefined metric
func (em *Environmental) GetError() error {
	if em == nil {
		return errs.Wrap(cvsserr.ErrUndefinedMetric)
	}
	if err := em.Base.GetError(); err != nil {
		return errs.Wrap(err)
	}
	switch true {
	case !em.CR.IsDefined(), !em.IR.IsDefined(), !em.AR.IsDefined(), !em.MAV.IsDefined(), !em.MAC.IsDefined(), !em.MPR.IsDefined(), !em.MUI.IsDefined(),
		!em.MS.IsDefined(), !em.MC.IsDefined(), !em.MI.IsDefined(), !em.MA.IsDefined():
		return errs.Wrap(cvsserr.ErrUndefinedMetric)
	default:
		return nil
	}
}

//Encode returns CVSSv3 vector string
func (em *Environmental) Encode() (string, error) {
	if err := em.GetError(); err != nil {
		return "", errs.Wrap(err)
	}
	bs, err := em.Base.Encode()
	if err != nil {
		return "", errs.Wrap(err)
	}
	r := &strings.Builder{}
	r.WriteString(bs)                        //Vector of Base metrics
	r.WriteString("/CR:" + em.CR.String())   //Exploitability
	r.WriteString("/IR:" + em.IR.String())   //Remediation Level
	r.WriteString("/AR:" + em.AR.String())   //Report Confidence
	r.WriteString("/MAV:" + em.MAV.String()) //Report Confidence
	r.WriteString("/MAC:" + em.MAC.String()) //Report Confidence
	r.WriteString("/MPR:" + em.MPR.String()) //Report Confidence
	r.WriteString("/MUI:" + em.MUI.String()) //Report Confidence
	r.WriteString("/MS:" + em.MS.String())   //Report Confidence
	r.WriteString("/MC:" + em.MC.String())   //Report Confidence
	r.WriteString("/MI:" + em.MI.String())   //Report Confidence
	r.WriteString("/MA:" + em.MA.String())   //Report Confidence
	return r.String(), nil
}

//Score returns score of Environmental metrics

func (em *Environmental) Score() float64 {
	if err := em.GetError(); err != nil {
		return 0.0
	}
	var score, ModifiedImpact float64
	ModifiedImpactSubScore := math.Min(1-(1-em.CR.Value()*em.MC.Value(em.C))*(1-em.IR.Value()*em.MI.Value(em.I))*(1-em.AR.Value()*em.MA.Value(em.A)), 0.915)

	if em.MS == ModifiedScopeUnchanged {
		ModifiedImpact = 6.42 * ModifiedImpactSubScore
	} else if em.MS == ModifiedScopeChanged {
		ModifiedImpact = 7.52*(ModifiedImpactSubScore-0.029) - 3.25*math.Pow(ModifiedImpactSubScore*0.9731-0.02, 13)
	} else {
		if em.S == ScopeUnchanged {
			ModifiedImpact = 6.42 * ModifiedImpactSubScore
		} else {
			ModifiedImpact = 7.52*(ModifiedImpactSubScore-0.029) - 3.25*math.Pow(ModifiedImpactSubScore*0.9731-0.02, 13)
		}
	}

	ModifiedExploitability := 8.22 * em.MAV.Value(em.AV) * em.MAC.Value(em.AC) * em.MPR.Value(em.MS, em.S, em.PR) * em.MUI.Value(em.UI)

	if ModifiedImpact <= 0 {
		score = 0.0
	} else if em.MS == ModifiedScopeUnchanged {
		score = roundUp(roundUp(math.Min((ModifiedImpact+ModifiedExploitability), 10)) * em.E.Value() * em.RL.Value() * em.RC.Value())
	} else if em.MS == ModifiedScopeChanged {
		score = roundUp(roundUp(math.Min(1.08*(ModifiedImpact+ModifiedExploitability), 10)) * em.E.Value() * em.RL.Value() * em.RC.Value())
	} else {
		if em.S == ScopeUnchanged {
			score = roundUp(roundUp(math.Min((ModifiedImpact+ModifiedExploitability), 10)) * em.E.Value() * em.RL.Value() * em.RC.Value())
		} else {
			score = roundUp(roundUp(math.Min(1.08*(ModifiedImpact+ModifiedExploitability), 10)) * em.E.Value() * em.RL.Value() * em.RC.Value())
		}
	}
	return score
}

//Severity returns severity by score of Environmental metrics
func (em *Environmental) Severity() Severity {
	return severity(em.Score())
}

//BaseMetrics returns Base metrics in Environmental metrics instance
func (em *Environmental) BaseMetrics() *Base {
	if em == nil {
		return nil
	}
	return em.Base
}

func (em *Environmental) TemporalMetrics() *Temporal {
	if em == nil {
		return nil
	}
	return em.Temporal
}

/* Copyright 2022 thejohnbrown */
