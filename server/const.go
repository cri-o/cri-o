package server

const (
	// According to http://man7.org/linux/man-pages/man5/resolv.conf.5.html:
	// "The search list is currently limited to six domains with a total of 256 characters."
	maxDNSSearches = 6
)

const (
	// by default, cpu.cfs_period_us is set to be 1000000 (i.e., 1s).
	defaultCPUCFSPeriod = 1000000
	// the upper limit of cpu.cfs_quota_us is 1000000.
	maxCPUCFSQuota = 1000000
	// the lower limit of cpu.cfs_quota_us is 1000.
	minCPUCFSQuota = 1000
)
