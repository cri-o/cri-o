package oci_test

import (
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/cri-o/cri-o/pkg/annotations"
	"github.com/cri-o/cri-o/pkg/config"
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
			invalidRuntime     = "invalid"
			defaultRuntime     = "runc"
			usernsRuntime      = "userns"
			performanceRuntime = "high-performance"
		)
		runtimes := config.Runtimes{
			defaultRuntime: {
				RuntimePath: "/bin/sh",
				RuntimeType: "",
				RuntimeRoot: "/run/runc",
			},
			invalidRuntime: {},
			usernsRuntime: {
				RuntimePath:        "/bin/sh",
				RuntimeType:        "",
				RuntimeRoot:        "/run/runc",
				AllowedAnnotations: []string{annotations.UsernsModeAnnotation},
			},
			performanceRuntime: {
				RuntimePath: "/bin/sh",
				RuntimeType: "",
				RuntimeRoot: "/run/runc",
				AllowedAnnotations: []string{
					annotations.CPULoadBalancingAnnotation,
					annotations.IRQLoadBalancingAnnotation,
					annotations.CPUQuotaAnnotation,
					annotations.OCISeccompBPFHookAnnotation,
				},
			},
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
		It("AllowUsernsAnnotation should be true when set", func() {
			// Given
			// When
			allowed, err := sut.AllowUsernsAnnotation(usernsRuntime)

			// Then
			Expect(err).To(BeNil())
			Expect(allowed).To(Equal(true))
		})
		It("AllowUsernsAnnotation should be false when not set", func() {
			// Given
			// When
			allowed, err := sut.AllowUsernsAnnotation(defaultRuntime)

			// Then
			Expect(err).To(BeNil())
			Expect(allowed).To(Equal(false))
		})
		It("AllowUsernsAnnotation should be false when runtime invalid", func() {
			// Given
			// When
			allowed, err := sut.AllowUsernsAnnotation(invalidRuntime)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(allowed).To(Equal(false))
		})
		It("AllowCPULoadBalancingAnnotation should be true when set", func() {
			// Given
			// When
			allowed, err := sut.AllowCPULoadBalancingAnnotation(performanceRuntime)

			// Then
			Expect(err).To(BeNil())
			Expect(allowed).To(Equal(true))
		})
		It("AllowCPUQuotaAnnotation should be true when set", func() {
			// Given
			// When
			allowed, err := sut.AllowCPUQuotaAnnotation(performanceRuntime)

			// Then
			Expect(err).To(BeNil())
			Expect(allowed).To(Equal(true))
		})
		It("AllowIRQLoadBalancingAnnotation should be true when set", func() {
			// Given
			// When
			allowed, err := sut.AllowIRQLoadBalancingAnnotation(performanceRuntime)

			// Then
			Expect(err).To(BeNil())
			Expect(allowed).To(Equal(true))
		})
		It("AllowOCISeccompBPFHookAnnotation should be true when set", func() {
			// Given
			// When
			allowed, err := sut.AllowOCISeccompBPFHookAnnotation(performanceRuntime)

			// Then
			Expect(err).To(BeNil())
			Expect(allowed).To(Equal(true))
		})
		It("AllowOCISeccompBPFHookAnnotation should be false when runtime invalid", func() {
			// Given
			// When
			allowed, err := sut.AllowOCISeccompBPFHookAnnotation(invalidRuntime)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(allowed).To(Equal(false))
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
