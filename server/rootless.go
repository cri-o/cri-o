package server

import (
	"strings"

	rspec "github.com/opencontainers/runtime-spec/specs-go"
	"github.com/opencontainers/runtime-tools/generate"
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
	g.Config.Linux.Resources = nil
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
