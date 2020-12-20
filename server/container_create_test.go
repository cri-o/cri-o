package server_test

import (
	"context"

	"github.com/cri-o/cri-o/server/cri/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
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
				&types.CreateContainerRequest{
					PodSandboxID: testSandbox.ID(),
					Config: &types.ContainerConfig{
						Metadata: &types.ContainerMetadata{
							Name: "name",
						},
					},
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when container config metadata name is empty", func() {
			// Given
			addContainerAndSandbox()

			// When
			response, err := sut.CreateContainer(context.Background(),
				&types.CreateContainerRequest{
					PodSandboxID: testSandbox.ID(),
					Config: &types.ContainerConfig{
						Metadata: &types.ContainerMetadata{},
					},
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when container config metadata is nil", func() {
			// Given
			addContainerAndSandbox()

			// When
			response, err := sut.CreateContainer(context.Background(),
				&types.CreateContainerRequest{
					PodSandboxID: testSandbox.ID(),
					Config:       &types.ContainerConfig{},
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when container config is nil", func() {
			// Given
			addContainerAndSandbox()

			// When
			response, err := sut.CreateContainer(context.Background(),
				&types.CreateContainerRequest{
					PodSandboxID:  testSandbox.ID(),
					Config:        types.NewContainerConfig(),
					SandboxConfig: types.NewPodSandboxConfig(),
				})

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
				&types.CreateContainerRequest{
					PodSandboxID:  testSandbox.ID(),
					Config:        types.NewContainerConfig(),
					SandboxConfig: types.NewPodSandboxConfig(),
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when sandbox not found", func() {
			// Given
			Expect(sut.PodIDIndex().Add(testSandbox.ID())).To(BeNil())

			// When
			response, err := sut.CreateContainer(context.Background(),
				&types.CreateContainerRequest{
					PodSandboxID:  testSandbox.ID(),
					Config:        types.NewContainerConfig(),
					SandboxConfig: types.NewPodSandboxConfig(),
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail on invalid pod sandbox ID", func() {
			// Given
			// When
			response, err := sut.CreateContainer(context.Background(),
				&types.CreateContainerRequest{
					PodSandboxID:  testSandbox.ID(),
					Config:        types.NewContainerConfig(),
					SandboxConfig: types.NewPodSandboxConfig(),
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail on empty pod sandbox ID", func() {
			// Given
			// When
			response, err := sut.CreateContainer(context.Background(),
				&types.CreateContainerRequest{
					Config:        types.NewContainerConfig(),
					SandboxConfig: types.NewPodSandboxConfig(),
				})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})
})
