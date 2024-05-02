package apparmor_test

import (
	"github.com/cri-o/cri-o/internal/config/apparmor"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	runtimeapi "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// The actual test suite
var _ = t.Describe("Config", func() {
	var sut *apparmor.Config

	BeforeEach(func() {
		sut = apparmor.New()
		Expect(sut).NotTo(BeNil())

		if !sut.IsEnabled() {
			Skip("AppArmor is disabled")
		}
	})

	t.Describe("Apply", func() {
		It("should return default profile if profile is empty or default", func() {
			// When
			profile, err := sut.Apply(&runtimeapi.LinuxContainerSecurityContext{})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(profile).To(Equal("crio-default"))
		})

		It("should return default profile if both fields are empty", func() {
			// When
			profile, err := sut.Apply(&runtimeapi.LinuxContainerSecurityContext{
				ApparmorProfile: "",
				Apparmor:        &runtimeapi.SecurityProfile{},
			})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(profile).To(Equal("crio-default"))
		})

		It("should return profile from new field", func() {
			// When
			profile, err := sut.Apply(&runtimeapi.LinuxContainerSecurityContext{
				ApparmorProfile: "runtime/default",
				Apparmor:        &runtimeapi.SecurityProfile{},
			})

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(profile).To(Equal("crio-default"))
		})

		It("should return error if profile empty", func() {
			// When
			_, err := sut.Apply(&runtimeapi.LinuxContainerSecurityContext{
				ApparmorProfile: "localhost/",
			})

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should not return error if profile provided", func() {
			// When
			profile, err := sut.Apply(&runtimeapi.LinuxContainerSecurityContext{
				ApparmorProfile: "localhost/some-profile",
			})

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(profile).To(Equal("some-profile"))
		})

		It("should not return error if profile provided with Apparmor field", func() {
			// When
			profile, err := sut.Apply(&runtimeapi.LinuxContainerSecurityContext{
				Apparmor: &runtimeapi.SecurityProfile{
					ProfileType:  runtimeapi.SecurityProfile_Localhost,
					LocalhostRef: "localhost/some-profile",
				},
			})

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(profile).To(Equal("some-profile"))
		})

		It("should not return error and respect Apparmor field", func() {
			// When
			profile, err := sut.Apply(&runtimeapi.LinuxContainerSecurityContext{
				ApparmorProfile: "localhost/another-profile",
				Apparmor: &runtimeapi.SecurityProfile{
					ProfileType:  runtimeapi.SecurityProfile_Localhost,
					LocalhostRef: "localhost/some-profile",
				},
			})

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(profile).To(Equal("some-profile"))
		})

		It("should not return error if Apparmor is unconfined", func() {
			// When
			profile, err := sut.Apply(&runtimeapi.LinuxContainerSecurityContext{
				Apparmor: &runtimeapi.SecurityProfile{
					ProfileType: runtimeapi.SecurityProfile_Unconfined,
				},
			})

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(profile).To(Equal("unconfined"))
		})

		It("should not return error if ApparmorProfile is unconfined", func() {
			// When
			profile, err := sut.Apply(&runtimeapi.LinuxContainerSecurityContext{
				ApparmorProfile: "unconfined",
			})

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(profile).To(Equal("unconfined"))
		})
	})

	t.Describe("IsEnabled", func() {
		It("should be true per default", func() {
			// Given
			// When
			res := sut.IsEnabled()

			// Then
			Expect(res).To(BeTrue())
		})
	})

	t.Describe("LoadProfile", func() {
		It("should succeed with unconfined", func() {
			// Given
			// When
			err := sut.LoadProfile("unconfined")

			// Then
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
