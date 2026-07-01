package config_test

import (
	"bytes"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/pkg/config"
)

// The actual test suite.
var _ = t.Describe("Config", func() {
	t.Describe("WriteTemplate", func() {
		BeforeEach(beforeEach)
		It("should succeed", func() {
			// Given
			var wr bytes.Buffer

			// When
			err := sut.WriteTemplate(true, &wr)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should include workload allowed_annotations in template output", func() {
			// Given
			sut.Workloads = config.Workloads{
				"test-workload": &config.WorkloadConfig{
					ActivationAnnotation: "io.test.workload",
					AnnotationPrefix:     "io.test.prefix",
					AllowedAnnotations: []string{
						"io.kubernetes.cri-o.userns-mode",
						"io.kubernetes.cri-o.umask",
						"io.kubernetes.cri-o.Devices",
					},
				},
			}
			var wr bytes.Buffer

			// When
			err := sut.WriteTemplate(true, &wr)

			// Then
			Expect(err).ToNot(HaveOccurred())
			output := wr.String()
			expected := `allowed_annotations = [
						"io.kubernetes.cri-o.userns-mode",
						"io.kubernetes.cri-o.umask",
						"io.kubernetes.cri-o.Devices",
						]`
			Expect(output).To(ContainSubstring(expected))
		})

		It("should not include workload allowed_annotations when empty", func() {
			// Given
			sut.Workloads = config.Workloads{
				"test-workload": &config.WorkloadConfig{
					ActivationAnnotation: "io.test.workload",
					AnnotationPrefix:     "io.test.prefix",
					AllowedAnnotations:   []string{},
				},
			}
			var wr bytes.Buffer

			// When
			err := sut.WriteTemplate(true, &wr)

			// Then
			Expect(err).ToNot(HaveOccurred())
			output := wr.String()

			// Extract just the workload section to verify allowed_annotations is not present
			lines := strings.Split(output, "\n")
			workloadSection := ""
			inWorkloadSection := false
			for _, line := range lines {
				if strings.Contains(line, "[crio.runtime.workloads.test-workload]") {
					inWorkloadSection = true
				} else if strings.HasPrefix(line, "[") && inWorkloadSection {
					break // End of workload section
				}
				if inWorkloadSection {
					workloadSection += line + "\n"
				}
			}
			Expect(workloadSection).ToNot(ContainSubstring("allowed_annotations = ["))
		})
	})
	t.Describe("RuntimesEqual", func() {
		It("not equal if different length", func() {
			// When
			r1 := config.Runtimes{
				"1": &config.RuntimeHandler{},
				"2": &config.RuntimeHandler{},
			}
			r2 := config.Runtimes{
				"1": &config.RuntimeHandler{},
			}

			// Then
			Expect(config.RuntimesEqual(r1, r2)).To(BeFalse())
		})
		It("not equal if different keys", func() {
			// When
			r1 := config.Runtimes{
				"1": &config.RuntimeHandler{},
			}
			r2 := config.Runtimes{
				"2": &config.RuntimeHandler{},
			}

			// Then
			Expect(config.RuntimesEqual(r1, r2)).To(BeFalse())
		})
		It("not equal if different values", func() {
			// When
			r1 := config.Runtimes{
				"1": &config.RuntimeHandler{
					RuntimePath: "1",
				},
			}
			r2 := config.Runtimes{
				"1": &config.RuntimeHandler{
					RuntimePath: "2",
				},
			}

			// Then
			Expect(config.RuntimesEqual(r1, r2)).To(BeFalse())
		})
		It("not equal if different slice values", func() {
			// When
			r1 := config.Runtimes{
				"1": &config.RuntimeHandler{
					AllowedAnnotations: []string{"1"},
				},
			}
			r2 := config.Runtimes{
				"1": &config.RuntimeHandler{
					AllowedAnnotations: []string{"2"},
				},
			}

			// Then
			Expect(config.RuntimesEqual(r1, r2)).To(BeFalse())
		})
		It("equal if same values", func() {
			// When
			r1 := config.Runtimes{
				"1": &config.RuntimeHandler{
					RuntimePath: "1",
				},
			}
			r2 := config.Runtimes{
				"1": &config.RuntimeHandler{
					RuntimePath: "1",
				},
			}

			// Then
			Expect(config.RuntimesEqual(r1, r2)).To(BeTrue())
		})
	})
})
