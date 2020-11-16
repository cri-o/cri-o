package sandbox_test

import (
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = Describe("Sandbox:SetNameAndID", func() {
	t.Describe("SetConfig", func() {
		It("should succeed", func() {
			// Given
			config := &pb.PodSandboxConfig{
				Metadata: &pb.PodSandboxMetadata{
					Name:      "name",
					Uid:       "uid",
					Namespace: "namespace",
				},
			}
			Expect(sut.SetConfig(config)).To(BeNil())

			// When
			err := sut.SetNameAndID()

			// Then
			Expect(err).To(BeNil())
			Expect(len(sut.ID())).To(Equal(64))
			Expect(sut.Name()).To(ContainSubstring("name"))
			Expect(sut.Name()).To(ContainSubstring("uid"))
			Expect(sut.Name()).To(ContainSubstring("namespace"))
		})

		It("should fail with empty config", func() {
			// Given
			// When
			err := sut.SetNameAndID()

			// Then
			Expect(err).NotTo(BeNil())
			Expect(sut.ID()).To(Equal(""))
			Expect(sut.Name()).To(Equal(""))
		})

		It("should fail with empty name in metadata", func() {
			// Given
			config := &pb.PodSandboxConfig{
				Metadata: &pb.PodSandboxMetadata{
					Uid:       "uid",
					Namespace: "namespace",
				},
			}
			Expect(sut.SetConfig(config)).NotTo(BeNil())

			// When
			err := sut.SetNameAndID()

			// Then
			Expect(err).NotTo(BeNil())
			Expect(sut.ID()).To(Equal(""))
			Expect(sut.Name()).To(Equal(""))
		})
		It("should fail with empty namespace in metadata", func() {
			// Given
			config := &pb.PodSandboxConfig{
				Metadata: &pb.PodSandboxMetadata{
					Name: "name",
					Uid:  "uid",
				},
			}
			Expect(sut.SetConfig(config)).To(BeNil())

			// When
			err := sut.SetNameAndID()

			// Then
			Expect(err).NotTo(BeNil())
			Expect(sut.ID()).To(Equal(""))
			Expect(sut.Name()).To(Equal(""))
		})
		It("should succeed with empty uid in metadata", func() {
			// Given
			config := &pb.PodSandboxConfig{
				Metadata: &pb.PodSandboxMetadata{
					Name:      "name",
					Namespace: "namespace",
				},
			}
			Expect(sut.SetConfig(config)).To(BeNil())

			// When
			err := sut.SetNameAndID()

			// Then
			Expect(err).To(BeNil())
			Expect(sut.ID()).NotTo(Equal(""))
			Expect(sut.Name()).NotTo(Equal(""))
		})
	})
})
