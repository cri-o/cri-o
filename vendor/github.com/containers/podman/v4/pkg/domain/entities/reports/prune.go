package reports

type PruneReport struct {
	Id   string `json:"Id"` //nolint
	Err  error  `json:"Err,omitempty"`
	Size uint64 `json:"Size"`
}

func PruneReportsIds(r []*PruneReport) []string {
	ids := make([]string, 0, len(r))
	for _, v := range r {
		if v == nil || v.Id == "" {
			continue
		}
		ids = append(ids, v.Id)
	}
	return ids
}

func PruneReportsErrs(r []*PruneReport) []error {
	errs := make([]error, 0, len(r))
	for _, v := range r {
		if v == nil || v.Err == nil {
			continue
		}
		errs = append(errs, v.Err)
	}
	return errs
}

func PruneReportsSize(r []*PruneReport) uint64 {
	size := uint64(0)
	for _, v := range r {
		if v == nil {
			continue
		}
		size += v.Size
	}
	return size
}
