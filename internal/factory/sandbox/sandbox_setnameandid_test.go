package sandbox_test

import (
	libsandbox "github.com/cri-o/cri-o/internal/lib/sandbox"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

var _ = Describe("Sandbox:SetNameAndID", func() {
	t.Describe("SetConfig", func() {
		It("should succeed", func() {
			// Given
			config := &types.PodSandboxConfig{
				Metadata: &types.PodSandboxMetadata{
					Name:      "name",
					Uid:       "uid",
					Namespace: "namespace",
				},
			}
			Expect(sut.SetConfig(config)).To(Succeed())

			// When
			err := sut.SetNameAndID(nil)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.ID()).To(HaveLen(64))
			Expect(sut.Name()).To(ContainSubstring("name"))
			Expect(sut.Name()).To(ContainSubstring("uid"))
			Expect(sut.Name()).To(ContainSubstring("namespace"))
		})

		It("should succeed with lib sandbox", func() {
			// Given
			config := &types.PodSandboxConfig{
				Metadata: &types.PodSandboxMetadata{
					Name:      "name",
					Uid:       "uid",
					Namespace: "namespace",
				},
			}
			Expect(sut.SetConfig(config)).To(Succeed())
			sb := libsandbox.New()

			// When
			err := sut.SetNameAndID(sb)

			// Then
			Expect(err).NotTo(HaveOccurred())
			Expect(sb.ID()).To(Equal(sut.ID()))
			Expect(sb.Name()).To(Equal(sut.Name()))
		})

		It("should fail with empty config", func() {
			// Given
			// When
			err := sut.SetNameAndID(nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(sut.ID()).To(Equal(""))
			Expect(sut.Name()).To(Equal(""))
		})

		It("should fail with empty name in metadata", func() {
			// Given
			config := &types.PodSandboxConfig{
				Metadata: &types.PodSandboxMetadata{
					Uid:       "uid",
					Namespace: "namespace",
				},
			}
			Expect(sut.SetConfig(config)).NotTo(Succeed())

			// When
			err := sut.SetNameAndID(nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(sut.ID()).To(Equal(""))
			Expect(sut.Name()).To(Equal(""))
		})

		It("should fail with empty namespace in metadata", func() {
			// Given
			config := &types.PodSandboxConfig{
				Metadata: &types.PodSandboxMetadata{
					Name: "name",
					Uid:  "uid",
				},
			}
			Expect(sut.SetConfig(config)).To(Succeed())

			// When
			err := sut.SetNameAndID(nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(sut.ID()).To(Equal(""))
			Expect(sut.Name()).To(Equal(""))
		})

		It("should fail with empty uid in metadata", func() {
			// Given
			config := &types.PodSandboxConfig{
				Metadata: &types.PodSandboxMetadata{
					Name:      "name",
					Namespace: "namespace",
				},
			}
			Expect(sut.SetConfig(config)).To(Succeed())

			// When
			err := sut.SetNameAndID(nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(sut.ID()).To(Equal(""))
			Expect(sut.Name()).To(Equal(""))
		})
	})
})
