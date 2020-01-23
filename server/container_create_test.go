package server_test

import (
	"context"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// The actual test suite
var _ = t.Describe("ContainerCreate", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ContainerCreate", func() {
		It("should fail when container config image is nil", func() {
			// Given
			addContainerAndSandbox()

			// When
			response, err := sut.CreateContainer(context.Background(),
				&pb.CreateContainerRequest{PodSandboxId: testSandbox.ID(),
					Config: &pb.ContainerConfig{
						Metadata: &pb.ContainerMetadata{
							Name: "name",
						},
					}})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when container config metadata name is empty", func() {
			// Given
			addContainerAndSandbox()

			// When
			response, err := sut.CreateContainer(context.Background(),
				&pb.CreateContainerRequest{PodSandboxId: testSandbox.ID(),
					Config: &pb.ContainerConfig{
						Metadata: &pb.ContainerMetadata{},
					}})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when container config metadata is nil", func() {
			// Given
			addContainerAndSandbox()

			// When
			response, err := sut.CreateContainer(context.Background(),
				&pb.CreateContainerRequest{PodSandboxId: testSandbox.ID(),
					Config: &pb.ContainerConfig{}})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when container config is nil", func() {
			// Given
			addContainerAndSandbox()

			// When
			response, err := sut.CreateContainer(context.Background(),
				&pb.CreateContainerRequest{PodSandboxId: testSandbox.ID()})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when container is stopped", func() {
			// Given
			addContainerAndSandbox()
			testSandbox.SetStopped(false)

			// When
			response, err := sut.CreateContainer(context.Background(),
				&pb.CreateContainerRequest{PodSandboxId: testSandbox.ID()})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when sandbox not found", func() {
			// Given
			Expect(sut.PodIDIndex().Add(testSandbox.ID())).To(BeNil())

			// When
			response, err := sut.CreateContainer(context.Background(),
				&pb.CreateContainerRequest{PodSandboxId: testSandbox.ID()})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail on invalid pod sandbox ID", func() {
			// Given
			// When
			response, err := sut.CreateContainer(context.Background(),
				&pb.CreateContainerRequest{PodSandboxId: testSandbox.ID()})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail on empty pod sandbox ID", func() {
			// Given
			// When
			response, err := sut.CreateContainer(context.Background(),
				&pb.CreateContainerRequest{})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})
})
