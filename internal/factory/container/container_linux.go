package container

import (
	"strconv"
	"strings"

	"github.com/opencontainers/cgroups"
	"github.com/opencontainers/selinux/go-selinux"

	"github.com/cri-o/cri-o/utils"
)

// SelinuxLabel returns the container's SelinuxLabel
// it takes the sandbox's label, which it falls back upon.
func (c *container) SelinuxLabel(sboxLabel string) ([]string, error) {
	selinuxConfig := c.config.GetLinux().GetSecurityContext().GetSelinuxOptions()

	labels := map[string]string{}

	labelOptions, err := selinux.DupSecOpt(sboxLabel)
	if err != nil {
		return nil, err
	}

	for _, r := range labelOptions {
		k := strings.Split(r, ":")[0]
		labels[k] = r
	}

	if selinuxConfig != nil {
		for _, r := range utils.GetLabelOptions(selinuxConfig) {
			k := strings.Split(r, ":")[0]
			labels[k] = r
		}
	}

	ret := []string{}
	for _, v := range labels {
		ret = append(ret, v)
	}

	return ret, nil
}

// convertCPUSharesToCgroupV2Weight converts CPU shares to cgroup v2 weight using OCI standard conversion.
func convertCPUSharesToCgroupV2Weight(shares uint64) string {
	weight := cgroups.ConvertCPUSharesToCgroupV2Value(shares)

	return strconv.FormatUint(weight, 10)
}
