package container_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/v1/container"
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
		metadata := &pb.PodSandboxMetadata{
			Name: name, Uid: uid, Namespace: namespace,
		}
		setupContainerWithMetadata(metadata)

		// When
		err := sut.SetNameAndID()

		// Then
		Expect(err).To(BeNil())
		Expect(len(sut.ID())).To(Equal(64))
		Expect(sut.Name()).To(ContainSubstring(name))
		Expect(sut.Name()).To(ContainSubstring(namespace))
		Expect(sut.Name()).To(ContainSubstring(uid))
	})

	It("should succeed with empty sandbox metadata", func() {
		// Given
		metadata := &pb.PodSandboxMetadata{}
		setupContainerWithMetadata(metadata)

		// When
		err := sut.SetNameAndID()

		// Then
		Expect(err).To(BeNil())
	})

	It("should fail with config nil", func() {
		// Given
		// When
		container, err := container.New(context.Background())
		Expect(err).To(BeNil())

		err = container.SetNameAndID()

		// Then
		Expect(container).ToNot(BeNil())
		Expect(err).NotTo(BeNil())
	})
})

func setupContainerWithMetadata(md *pb.PodSandboxMetadata) {
	config := &pb.ContainerConfig{
		Metadata: &pb.ContainerMetadata{Name: "name"},
	}
	sboxConfig := &pb.PodSandboxConfig{
		Metadata: md,
	}
	Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())
}
