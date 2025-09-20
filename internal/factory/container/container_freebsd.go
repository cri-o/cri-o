package container

func (c *container) SelinuxLabel(sboxLabel string) ([]string, error) {
	return []string{}, nil
}

// convertCPUSharesToCgroupV2Weight is a noop on non-Linux platforms.
func convertCPUSharesToCgroupV2Weight(shares uint64) string {

	return ""
}
