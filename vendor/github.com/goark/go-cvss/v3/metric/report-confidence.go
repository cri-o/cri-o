package metric

// ReportConfidence is metric type for Temporal Metrics
type ReportConfidence int

// Constant of ReportConfidence result
const (
	ReportConfidenceInvalid ReportConfidence = iota
	ReportConfidenceNotDefined
	ReportConfidenceUnknown
	ReportConfidenceReasonable
	ReportConfidenceConfirmed
)

var reportConfidenceMap = map[ReportConfidence]string{
	ReportConfidenceNotDefined: "X",
	ReportConfidenceUnknown:    "U",
	ReportConfidenceReasonable: "R",
	ReportConfidenceConfirmed:  "C",
}

var reportConfidenceValueMap = map[ReportConfidence]float64{
	ReportConfidenceNotDefined: 1,
	ReportConfidenceUnknown:    0.92,
	ReportConfidenceReasonable: 0.96,
	ReportConfidenceConfirmed:  1,
}

// GetReportConfidence returns result of ReportConfidence metric
func GetReportConfidence(s string) ReportConfidence {
	for k, v := range reportConfidenceMap {
		if s == v {
			return k
		}
	}
	return ReportConfidenceInvalid
}

func (rc ReportConfidence) String() string {
	if s, ok := reportConfidenceMap[rc]; ok {
		return s
	}
	return ""
}

// Value returns value of ReportConfidence metric
func (rc ReportConfidence) Value() float64 {
	if v, ok := reportConfidenceValueMap[rc]; ok {
		return v
	}
	return 1
}

// IsDefined returns false if undefined result value of metric
func (rc ReportConfidence) IsValid() bool {
	_, ok := reportConfidenceValueMap[rc]
	return ok
}

/* Copyright by Florent Viel, 2020 */
/* Contributed by Spiegel, 2020-2023 */
