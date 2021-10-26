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
			// so we have permission to make a directory within it
			c.ContainerAttachSocketDir = t.MustTempDir("crio")

			// When
			runtime, err := oci.New(c)
			Expect(err).To(BeNil())

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
			vmRuntime          = "kata"
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
			vmRuntime: {
				RuntimePath:                  "/usr/bin/containerd-shim-kata-v2",
				RuntimeType:                  "vm",
				RuntimeRoot:                  "/run/vc",
				PrivilegedWithoutHostDevices: true,
				RuntimeConfigPath:            "/opt/kata-containers/config.toml",
			},
		}

		BeforeEach(func() {
			c, err := config.DefaultConfig()
			Expect(err).To(BeNil())
			c.DefaultRuntime = defaultRuntime
			c.Runtimes = runtimes
			// so we have permission to make a directory within it
			c.ContainerAttachSocketDir = t.MustTempDir("crio")

			sut, err = oci.New(c)
			Expect(err).To(BeNil())
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
		It("should return an OCI runtime type if none is set", func() {
			// Given
			// When
			runtimeType, err := sut.RuntimeType(defaultRuntime)

			// Then
			Expect(err).To(BeNil())
			Expect(runtimeType).To(Equal(""))
		})
		It("should return a VM runtime type when it is set", func() {
			// Given
			// When
			runtimeType, err := sut.RuntimeType(vmRuntime)

			// Then
			Expect(err).To(BeNil())
			Expect(runtimeType).To(Equal(config.RuntimeTypeVM))
		})
		Context("AllowedAnnotations", func() {
			It("should succeed to return allowed annotation", func() {
				// Given
				Expect(runtimes[performanceRuntime].ValidateRuntimeAllowedAnnotations()).To(BeNil())

				// When
				foundAnn, err := sut.AllowedAnnotations(performanceRuntime)

				// Then
				Expect(err).To(BeNil())
				Expect(foundAnn).NotTo(ContainElement(annotations.DevicesAnnotation))
				Expect(foundAnn).To(ContainElement(annotations.IRQLoadBalancingAnnotation))
			})
			It("should fail to return allowed annotation of unknown runtime", func() {
				// Given
				// When
				_, err := sut.AllowedAnnotations("invalid")

				// Then
				Expect(err).NotTo(BeNil())
			})
		})

		It("PrivilegedWithoutHostDevices should be true when set", func() {
			// Given
			// When
			privileged, err := sut.PrivilegedWithoutHostDevices(vmRuntime)

			// Then
			Expect(err).To(BeNil())
			Expect(privileged).To(Equal(true))
		})
		It("PrivilegedWithoutHostDevices should be false when runtime invalid", func() {
			// Given
			// When
			privileged, err := sut.PrivilegedWithoutHostDevices(invalidRuntime)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(privileged).To(Equal(false))
		})
		It("PrivilegedWithoutHostDevices should be false when runtime is the default", func() {
			// Given
			// When
			privileged, err := sut.PrivilegedWithoutHostDevices(defaultRuntime)

			// Then
			Expect(err).To(BeNil())
			Expect(privileged).To(Equal(false))
		})
	})

	t.Describe("BuildContainerdBinaryName", func() {
		It("Simple binary name (containerd-shim-kata-v2)", func() {
			binaryName := oci.BuildContainerdBinaryName("containerd-shim-kata-v2")
			Expect(binaryName).To(Equal("containerd.shim.kata.v2"))
		})

		It("Full binary path with a simple binary name (/usr/bin/containerd-shim-kata-v2)", func() {
			binaryName := oci.BuildContainerdBinaryName("/usr/bin/containerd-shim-kata-v2")
			Expect(binaryName).To(Equal("/usr/bin/containerd.shim.kata.v2"))
		})

		It("Composed binary name (containerd-shim-kata-qemu-with-dax-support-v2)", func() {
			binaryName := oci.BuildContainerdBinaryName("containerd-shim-kata-qemu-with-dax-support-v2")
			Expect(binaryName).To(Equal("containerd.shim.kata-qemu-with-dax-support.v2"))
		})

		It("Full binary path with a composed binary name (/usr/bin/containerd-shim-kata-v2)", func() {
			binaryName := oci.BuildContainerdBinaryName("/usr/bin/containerd-shim-kata-qemu-with-dax-support-v2")
			Expect(binaryName).To(Equal("/usr/bin/containerd.shim.kata-qemu-with-dax-support.v2"))
		})
	})
})
