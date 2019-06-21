package server_test

import (
	"context"
	"os"

	"github.com/cri-o/cri-o/internal/pkg/storage"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
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
		// TODO(sgrunert): refactor the internal function to reduce the
		// cyclomatic complexity and test it separately
		It("should fail when container creation erros", func() {
			// Given
			addContainerAndSandbox()
			sut.SetRuntime(ociRuntimeMock)
			gomock.InOrder(
				imageServerMock.EXPECT().ResolveNames(
					gomock.Any(), gomock.Any()).
					Return([]string{"image"}, nil),
				imageServerMock.EXPECT().ImageStatus(gomock.Any(),
					gomock.Any()).Return(&storage.ImageResult{}, nil),
				runtimeServerMock.EXPECT().CreateContainer(
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any()).
					Return(storage.ContainerInfo{
						Config: &v1.Image{}}, nil),
				runtimeServerMock.EXPECT().StartContainer(gomock.Any()).
					Return("testfolder", nil),
				ociRuntimeMock.EXPECT().CreateContainer(gomock.Any(),
					gomock.Any()).Return(nil),
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(nil),
			)
			defer os.RemoveAll("testfolder")

			// When
			response, err := sut.CreateContainer(context.Background(),
				&pb.CreateContainerRequest{PodSandboxId: testSandbox.ID(),
					Config: &pb.ContainerConfig{
						Metadata: &pb.ContainerMetadata{
							Name: "name",
						},
						Image:   &pb.ImageSpec{Image: "{}"},
						Command: []string{"cmd"},
						Args:    []string{"arg"},
					}})

			// Then
			Expect(err).To(BeNil())
			Expect(response).NotTo(BeNil())
		})

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
			testSandbox.SetStopped()

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
