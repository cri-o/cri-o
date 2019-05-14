package oci_test

import (
	"github.com/cri-o/cri-o/oci"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("Oci", func() {
	t.Describe("New", func() {
		It("should succeed with default runtime", func() {
			// Given
			// When
			runtime, err := oci.New("runc",
				map[string]oci.RuntimeHandler{"runc": {"/bin/sh", "", "/run/runc"}},
				"", []string{}, "", "", "", "", 0, false, false, 0)

			// Then
			Expect(err).To(BeNil())
			Expect(runtime).NotTo(BeNil())
		})

		It("should fail if no runtime configured for default runtime", func() {
			// Given
			// When
			runtime, err := oci.New("",
				map[string]oci.RuntimeHandler{}, "", []string{},
				"", "", "", "", 0, false, false, 0)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(runtime).To(BeNil())
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
		runtimes := map[string]oci.RuntimeHandler{
			defaultRuntime: {"/bin/sh", "", "/run/runc"}, invalidRuntime: {},
		}

		BeforeEach(func() {
			var err error
			sut, err = oci.New(defaultRuntime,
				runtimes,
				"", []string{}, "", "", "", "", 0, false, false, 0)

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

		It("should fail to validate an inexisting runtime handler", func() {
			// Given
			// When
			handler, err := sut.ValidateRuntimeHandler("not_existing")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(handler).To(Equal(oci.RuntimeHandler{}))
		})

		It("should fail to validate an invalid runtime path", func() {
			// Given
			// When
			handler, err := sut.ValidateRuntimeHandler(invalidRuntime)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(handler).To(Equal(oci.RuntimeHandler{}))
		})

		It("should fail to validate an empty runtime handler", func() {
			// Given
			// When
			handler, err := sut.ValidateRuntimeHandler("")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(handler).To(Equal(oci.RuntimeHandler{}))
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
