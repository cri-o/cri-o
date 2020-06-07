package container_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"

	"github.com/cri-o/cri-o/pkg/container"
)

// The actual test suite
var _ = t.Describe("Container:SetNameAndID", func() {
	// Setup the SUT
	BeforeEach(func() {
		config := &pb.ContainerConfig{
			Metadata: &pb.ContainerMetadata{Name: "name"},
		}
		sboxConfig := &pb.PodSandboxConfig{}
		Expect(sut.SetConfig(config, sboxConfig)).To(BeNil())
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

		// When
		err := sut.SetNameAndID(metadata)

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

		// When
		err := sut.SetNameAndID(metadata)

		// Then
		Expect(err).To(BeNil())
	})

	It("should fail with config nil", func() {
		// Given
		// When
		err := container.New(context.Background()).SetNameAndID(nil)

		// Then
		Expect(err).NotTo(BeNil())
	})

	It("should fail with sandbox metadata nil", func() {
		// Given
		// When
		err := sut.SetNameAndID(nil)

		// Then
		Expect(err).NotTo(BeNil())
	})
})
