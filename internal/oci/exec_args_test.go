package oci_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/internal/oci"
)

var _ = t.Describe("NormalizeExecCmdArgs", func() {
	It("strips leading newlines and carriage returns from every argument", func() {
		in := []string{"/bin/sh", "-euc", "\n\recho hello", "\n"}
		Expect(oci.NormalizeExecCmdArgs(in)).To(Equal([]string{"/bin/sh", "-euc", "echo hello", ""}))
		// does not modify the input slice
		Expect(in[2]).To(HavePrefix("\n"))
	})
	It("preserves nil vs empty for a zero-length input", func() {
		Expect(oci.NormalizeExecCmdArgs(nil)).To(BeNil())
		empty := []string{}
		Expect(oci.NormalizeExecCmdArgs(empty)).To(Equal(empty))
	})
})
