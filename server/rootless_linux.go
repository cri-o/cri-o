package server

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/opencontainers/cgroups"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
	"github.com/sirupsen/logrus"
)

func hasNetworkNamespace(config *rspec.Spec) bool {
	for _, n := range config.Linux.Namespaces {
		if n.Type == rspec.NetworkNamespace {
			return true
		}
	}

	return false
}

func makeOCIConfigurationRootless(g *generate.Generator) {
	// Resource limitations requires cgroup v2 delegation (https://rootlesscontaine.rs/getting-started/common/cgroup2/).
	if r := g.Config.Linux.Resources; r != nil {
		// cannot control device eBPF with rootless
		r.Devices = nil
		if r.Memory != nil || r.CPU != nil || r.Pids != nil || r.BlockIO != nil || r.Rdma != nil || r.HugepageLimits != nil {
			v2Controllers := getAvailableV2Controllers()
			if _, ok := v2Controllers["memory"]; !ok && r.Memory != nil {
				logrus.Warn("rootless: cgroup v2 memory controller is not delegated. Discarding memory limit.")

				r.Memory = nil
			}

			if _, ok := v2Controllers["cpu"]; !ok && r.CPU != nil {
				logrus.Warn("rootless: cgroup v2 cpu controller is not delegated. Discarding cpu limit.")

				r.CPU = nil
			}

			if _, ok := v2Controllers["cpuset"]; !ok && r.CPU != nil {
				logrus.Warn("rootless: cgroup v2 cpuset controller is not delegated. Discarding cpuset limit.")

				r.CPU.Cpus = ""
				r.CPU.Mems = ""
			}

			if _, ok := v2Controllers["pids"]; !ok && r.Pids != nil {
				logrus.Warn("rootless: cgroup v2 pids controller is not delegated. Discarding pids limit.")

				r.Pids = nil
			}

			if _, ok := v2Controllers["io"]; !ok && r.BlockIO != nil {
				logrus.Warn("rootless: cgroup v2 io controller is not delegated. Discarding block I/O limit.")

				r.BlockIO = nil
			}

			if _, ok := v2Controllers["rdma"]; !ok && r.Rdma != nil {
				logrus.Warn("rootless: cgroup v2 rdma controller is not delegated. Discarding RDMA limit.")

				r.Rdma = nil
			}

			if _, ok := v2Controllers["hugetlb"]; !ok && r.HugepageLimits != nil {
				logrus.Warn("rootless: cgroup v2 hugetlb controller is not delegated. Discarding RDMA limit.")

				r.HugepageLimits = nil
			}
		}
	}

	g.Config.Process.OOMScoreAdj = nil
	g.Config.Process.ApparmorProfile = ""

	for i := range g.Config.Mounts {
		var newOptions []string

		for _, o := range g.Config.Mounts[i].Options {
			if strings.HasPrefix(o, "gid=") {
				continue
			}

			newOptions = append(newOptions, o)
		}

		g.Config.Mounts[i].Options = newOptions
	}

	if !hasNetworkNamespace(g.Config) {
		g.RemoveMount("/sys")

		sysMnt := rspec.Mount{
			Destination: "/sys",
			Type:        "bind",
			Source:      "/sys",
			Options:     []string{"nosuid", "noexec", "nodev", "ro", "rbind"},
		}
		g.AddMount(sysMnt)
	}

	g.SetLinuxCgroupsPath("")
}

// getAvailableV2Controllers returns the entries in /sys/fs/cgroup/<SELF>/cgroup.controllers.
func getAvailableV2Controllers() map[string]struct{} {
	procSelfCgroup, err := cgroups.ParseCgroupFile("/proc/self/cgroup")
	if err != nil {
		logrus.Error(err)

		return nil
	}

	v2Group := procSelfCgroup[""]
	if v2Group == "" {
		return nil
	}

	controllersPath := filepath.Join("/sys/fs/cgroup", v2Group, "cgroup.controllers")

	controllersBytes, err := os.ReadFile(controllersPath)
	if err != nil {
		logrus.Error(err)

		return nil
	}

	result := make(map[string]struct{})
	for _, controller := range strings.Split(strings.TrimSpace(string(controllersBytes)), " ") {
		result[controller] = struct{}{}
	}

	return result
}
