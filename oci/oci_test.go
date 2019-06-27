package oci_test

import (
	"github.com/cri-o/cri-o/lib/config"
	"github.com/cri-o/cri-o/oci"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("Oci", func() {
	t.Describe("New", func() {
		It("should succeed with default config", func() {
			// Given
			c, err := config.DefaultConfig()
			Expect(err).To(BeNil())

			// When
			runtime := oci.New(c)

			// Then
			Expect(runtime).NotTo(BeNil())
		})
	})

	t.Describe("Oci", func() {
		// The system under test
		var sut *oci.Runtime

		// Test constants
		const (
			invalidRuntime = "invalid"
			defaultRuntime = "runc"
		)
		runtimes := config.Runtimes{
			defaultRuntime: {
				RuntimePath: "/bin/sh",
				RuntimeType: "",
				RuntimeRoot: "/run/runc",
			}, invalidRuntime: {},
		}

		BeforeEach(func() {
			c, err := config.DefaultConfig()
			Expect(err).To(BeNil())
			c.DefaultRuntime = defaultRuntime
			c.Runtimes = runtimes

			sut = oci.New(c)
			Expect(sut).NotTo(BeNil())
		})

		It("should succeed to retrieve the runtimes", func() {
			// Given
			// When
			result := sut.Runtimes()

			// Then
			Expect(result).To(Equal(runtimes))
		})

		It("should succeed to validate a runtime handler", func() {
			// Given
			// When
			handler, err := sut.ValidateRuntimeHandler(defaultRuntime)

			// Then
			Expect(err).To(BeNil())
			Expect(handler).To(Equal(runtimes[defaultRuntime]))
		})
	})

	t.Describe("ExecSyncError", func() {
		It("should succeed to get the exec sync error", func() {
			// Given
			sut := oci.ExecSyncError{}

			// When
			result := sut.Error()

			// Then
			Expect(result).To(ContainSubstring("error"))
		})
	})
})
