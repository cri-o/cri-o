package lib

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	rspec "github.com/opencontainers/runtime-spec/specs-go"
)

var _ = Describe("shouldRecreateExternalBindMount", func() {
	declared := []rspec.Mount{{Destination: "/data", Source: "/host/data", Type: "bind"}}

	It("recreates a declared mount whose source was not remapped", func() {
		Expect(shouldRecreateExternalBindMount(declared, ExternalBindMount{
			Destination: "/data",
			Source:      "/host/data",
		})).To(BeTrue())
	})

	It("skips a declared mount whose source was already remapped", func() {
		Expect(shouldRecreateExternalBindMount(declared, ExternalBindMount{
			Destination: "/data",
			Source:      "/old/host/data",
		})).To(BeFalse())
	})

	It("skips a bind mount the container never declared", func() {
		Expect(shouldRecreateExternalBindMount(declared, ExternalBindMount{
			Destination: "/not-declared",
			Source:      "/etc/cron.d/evil",
		})).To(BeFalse())
	})

	It("skips a matching mount whose declared type is not bind", func() {
		Expect(shouldRecreateExternalBindMount([]rspec.Mount{
			{Destination: "/run", Source: "tmpfs", Type: "tmpfs"},
		}, ExternalBindMount{
			Destination: "/run",
			Source:      "tmpfs",
		})).To(BeFalse())
	})
})
