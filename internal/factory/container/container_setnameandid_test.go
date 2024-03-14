package container_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/internal/factory/container"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// The actual test suite
var _ = t.Describe("Container:SetNameAndID", func() {
	// Setup the SUT
	BeforeEach(func() {
	})

	It("should succeed", func() {
		// Given
		const (
			name      = "name"
			namespace = "namespace"
			uid       = "uid"
		)
		metadata := &types.PodSandboxMetadata{
			Name: name, Uid: uid, Namespace: namespace,
		}
		setupContainerWithMetadata(metadata)

		// When
		err := sut.SetNameAndID("")

		// Then
		Expect(err).ToNot(HaveOccurred())
		Expect(sut.ID()).To(HaveLen(64))
		Expect(sut.Name()).To(ContainSubstring(name))
		Expect(sut.Name()).To(ContainSubstring(namespace))
		Expect(sut.Name()).To(ContainSubstring(uid))
	})

	It("should succeed with ID as parameter", func() {
		// Given
		const (
			name      = "name"
			namespace = "namespace"
			uid       = "uid"
		)
		metadata := &types.PodSandboxMetadata{
			Name: name, Uid: uid, Namespace: namespace,
		}
		setupContainerWithMetadata(metadata)

		// When
		err := sut.SetNameAndID("use-this-ID")

		// Then
		Expect(err).ToNot(HaveOccurred())
		Expect(sut.ID()).To(Equal("use-this-ID"))
		Expect(sut.Name()).To(ContainSubstring(name))
		Expect(sut.Name()).To(ContainSubstring(namespace))
		Expect(sut.Name()).To(ContainSubstring(uid))
	})

	It("should succeed with empty sandbox metadata", func() {
		// Given
		metadata := &types.PodSandboxMetadata{}
		setupContainerWithMetadata(metadata)

		// When
		err := sut.SetNameAndID("")

		// Then
		Expect(err).ToNot(HaveOccurred())
	})

	It("should fail with config nil", func() {
		// Given
		// When
		container, err := container.New()
		Expect(err).ToNot(HaveOccurred())

		err = container.SetNameAndID("")

		// Then
		Expect(container).ToNot(BeNil())
		Expect(err).To(HaveOccurred())
	})
})

func setupContainerWithMetadata(md *types.PodSandboxMetadata) {
	config := &types.ContainerConfig{
		Metadata: &types.ContainerMetadata{Name: "name"},
	}
	sboxConfig := &types.PodSandboxConfig{
		Metadata: md,
	}
	Expect(sut.SetConfig(config, sboxConfig)).To(Succeed())
}
