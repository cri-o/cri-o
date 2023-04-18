package metric

import (
	"fmt"
	"strings"

	"github.com/goark/errs"
	"github.com/goark/go-cvss/cvsserr"
)

const (
	metricE  = "E"
	metricRL = "RL"
	metricRC = "RC"
)

// Base is Temporal Metrics for CVSSv3
type Temporal struct {
	*Base
	E     Exploitability
	RL    RemediationLevel
	RC    ReportConfidence
	names map[string]bool
}

// NewTemporal returns Temporal Metrics instance
func NewTemporal() *Temporal {
	return &Temporal{
		Base:  NewBase(),
		E:     ExploitabilityNotDefined,
		RL:    RemediationLevelNotDefined,
		RC:    ReportConfidenceNotDefined,
		names: map[string]bool{},
	}
}

func (tm *Temporal) Decode(vector string) (*Temporal, error) {
	if tm == nil {
		tm = NewTemporal()
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
	tm.Ver = ver
	//parse vector
	var lastErr error
	for _, value := range values[1:] {
		if err := tm.decodeOne(value); err != nil {
			if !errs.Is(err, cvsserr.ErrNotSupportMetric) {
				return nil, errs.Wrap(err, errs.WithContext("vector", vector))
			}
			lastErr = err
		}
	}
	if lastErr != nil {
		return nil, lastErr
	}
	if err := tm.GetError(); err != nil {
		return nil, err
	}
	return tm, nil
}
func (tm *Temporal) decodeOne(str string) error {
	if err := tm.Base.decodeOne(str); err != nil {
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
	if tm.names[name] {
		return errs.Wrap(cvsserr.ErrSameMetric, errs.WithContext("metric", str))
	}
	switch name {
	case metricE: //Exploitability
		tm.E = GetExploitability(m[1])
		if tm.E == ExploitabilityInvalid {
			return errs.Wrap(cvsserr.ErrInvalidValue, errs.WithContext("metric", str))
		}
	case metricRL: //RemediationLevel
		tm.RL = GetRemediationLevel(m[1])
		if tm.RL == RemediationLevelInvalid {
			return errs.Wrap(cvsserr.ErrInvalidValue, errs.WithContext("metric", str))
		}
	case metricRC: //RemediationLevel
		tm.RC = GetReportConfidence(m[1])
		if tm.RC == ReportConfidenceInvalid {
			return errs.Wrap(cvsserr.ErrInvalidValue, errs.WithContext("metric", str))
		}
	default:
		return errs.Wrap(cvsserr.ErrNotSupportMetric, errs.WithContext("metric", str))
	}
	tm.names[name] = true
	return nil
}

// GetError returns error instance if undefined metric
func (tm *Temporal) GetError() error {
	if tm == nil {
		return errs.Wrap(cvsserr.ErrNoTemporalMetrics)
	}
	if err := tm.Base.GetError(); err != nil {
		return errs.Wrap(err)
	}
	switch true {
	case !tm.E.IsValid(), !tm.RL.IsValid(), !tm.RC.IsValid():
		return errs.Wrap(cvsserr.ErrInvalidValue)
	default:
		return nil
	}
}

// Encode returns CVSSv3 vector string
func (tm *Temporal) Encode() (string, error) {
	if tm == nil {
		return "", errs.Wrap(cvsserr.ErrNoTemporalMetrics)
	}
	bs, _ := tm.Base.Encode()
	r := &strings.Builder{}
	r.WriteString(bs)                                     //Vector of Base metrics
	r.WriteString(fmt.Sprintf("/%v:%v", metricE, tm.E))   //Exploitability
	r.WriteString(fmt.Sprintf("/%v:%v", metricRL, tm.RL)) //Remediation Level
	r.WriteString(fmt.Sprintf("/%v:%v", metricRC, tm.RC)) //Report Confidence
	return r.String(), tm.GetError()
}

// String is stringer method.
func (tm *Temporal) String() string {
	s, _ := tm.Encode()
	return s
}

// Score returns score of Temporal metrics
func (tm *Temporal) Score() float64 {
	if err := tm.GetError(); err != nil {
		return 0.0
	}
	return roundUp(tm.Base.Score() * tm.E.Value() * tm.RL.Value() * tm.RC.Value())
}

// Severity returns severity by score of Temporal metrics
func (tm *Temporal) Severity() Severity {
	return severity(tm.Score())
}

// BaseMetrics returns Base metrics in Temporal metrics instance
func (tm *Temporal) BaseMetrics() *Base {
	if tm == nil {
		return nil
	}
	return tm.Base
}

/* Copyright by Florent Viel, 2020 */
/* Contributed by Spiegel, 2020-2023 */
