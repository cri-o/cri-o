package cmdrunner_test

import (
	"os/exec"

	"github.com/cri-o/cri-o/utils/cmdrunner"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = t.Describe("CommandRunner", func() {
	It("command should not prepend if not configured", func() {
		// Given
		cmd := "ls"
		baseline, err := exec.Command(cmd).CombinedOutput()
		Expect(err).To(BeNil())

		// When
		output, err := cmdrunner.CombinedOutput(cmd)
		Expect(err).To(BeNil())

		// Then
		Expect(output).To(Equal(baseline))
	})
	It("command should prepend if configured", func() {
		// Given
		cmd := "ls"
		cmdrunner.PrependCommandsWith("which")
		baseline, err := exec.Command(cmd).CombinedOutput()
		Expect(err).To(BeNil())

		// When
		output, err := cmdrunner.CombinedOutput(cmd)
		Expect(err).To(BeNil())

		// Then
		Expect(output).NotTo(Equal(baseline))
	})
	It("command should not prepend if only args are configured", func() {
		// Given
		cmd := "ls"
		cmdrunner.PrependCommandsWith("", "-l")
		baseline, err := exec.Command(cmd).CombinedOutput()
		Expect(err).To(BeNil())

		// When
		output, err := cmdrunner.CombinedOutput(cmd)
		Expect(err).To(BeNil())

		// Then
		Expect(output).To(Equal(baseline))
	})
})
