//go:build !linux

package node

func CgroupIsV2() bool {
	return false
}

// CgroupHasMemorySwap returns whether the memory swap controller is present
func CgroupHasMemorySwap() bool {
	return false
}

// CgroupHasHugetlb returns whether the hugetlb controller is present
func CgroupHasHugetlb() bool {
	return false
}

// CgroupHasPid returns whether the pid controller is present
func CgroupHasPid() bool {
	return false
}
