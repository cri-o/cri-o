package cmdrunner_test

import (
	"os/exec"

	"github.com/cri-o/cri-o/utils/cmdrunner"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = t.Describe("CommandRunner", func() {
	It("command prepend should reset on request", func() {
		// Given
		cmdrunner.PrependCommandsWith("which")
		Expect(cmdrunner.GetPrependedCmd()).To(Equal("which"))

		// When
		cmdrunner.ResetPrependedCmd()

		// Then
		Expect(cmdrunner.GetPrependedCmd()).To(Equal(""))
	})
	It("command should not prepend if not configured", func() {
		// Given
		cmdrunner.ResetPrependedCmd()
		cmd := "ls"
		baseline, err := exec.Command(cmd).CombinedOutput()
		Expect(err).To(BeNil())

		// When
		output, err := cmdrunner.CombinedOutput(cmd)
		Expect(err).To(BeNil())

		// Then
		Expect(output).To(Equal(baseline))
		Expect(cmdrunner.GetPrependedCmd()).To(Equal(""))
	})
	It("command should prepend if configured", func() {
		// Given
		cmdrunner.ResetPrependedCmd()
		cmd := "ls"
		cmdrunner.PrependCommandsWith("which")
		baseline, err := exec.Command(cmd).CombinedOutput()
		Expect(err).To(BeNil())

		// When
		output, err := cmdrunner.CombinedOutput(cmd)
		Expect(err).To(BeNil())

		// Then
		Expect(output).NotTo(Equal(baseline))
		Expect(cmdrunner.GetPrependedCmd()).To(Equal("which"))
	})
	It("command should not prepend if only args are configured", func() {
		// Given
		cmdrunner.ResetPrependedCmd()
		cmd := "ls"
		cmdrunner.PrependCommandsWith("", "-l")
		baseline, err := exec.Command(cmd).CombinedOutput()
		Expect(err).To(BeNil())

		// When
		output, err := cmdrunner.CombinedOutput(cmd)
		Expect(err).To(BeNil())

		// Then
		Expect(output).To(Equal(baseline))
		Expect(cmdrunner.GetPrependedCmd()).To(Equal(""))
	})
})
