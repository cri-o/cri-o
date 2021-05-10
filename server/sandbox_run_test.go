package server_test

import (
	"context"

	"github.com/cri-o/cri-o/internal/storage"
	"github.com/cri-o/cri-o/server/cri/types"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// The actual test suite
var _ = t.Describe("RunPodSandbox", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("RunPodSandbox", func() {
		// TODO(sgrunert): refactor the internal function to reduce the
		// cyclomatic complexity and test it separately
		It("should fail when container creation errors", func() {
			// Given
			gomock.InOrder(
				runtimeServerMock.EXPECT().CreatePodSandbox(gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any()).
					Return(storage.ContainerInfo{
						RunDir: "/tmp",
						Config: &v1.Image{Config: v1.ImageConfig{}},
					}, nil),
				runtimeServerMock.EXPECT().GetContainerMetadata(gomock.Any()).
					Return(storage.RuntimeContainerMetadata{}, nil),
				runtimeServerMock.EXPECT().SetContainerMetadata(gomock.Any(),
					gomock.Any()).Return(nil),
				runtimeServerMock.EXPECT().DeleteContainer(gomock.Any()).
					Return(nil),
			)

			// When
			response, err := sut.RunPodSandbox(context.Background(),
				&types.RunPodSandboxRequest{Config: &types.PodSandboxConfig{
					Metadata: &types.PodSandboxMetadata{
						Name:      "name",
						Namespace: "default",
						UID:       "uid",
					},
					LogDirectory: "/tmp",
					Linux: &types.LinuxPodSandboxConfig{
						SecurityContext: &types.LinuxSandboxSecurityContext{
							NamespaceOptions: &types.NamespaceOption{
								Ipc: types.NamespaceModeNODE,
							},
						},
					},
				}})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when metadata is nil", func() {
			// Given
			// When
			response, err := sut.RunPodSandbox(context.Background(),
				&types.RunPodSandboxRequest{Config: &types.PodSandboxConfig{}})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when metadata kubeName is nil", func() {
			// Given
			// When
			response, err := sut.RunPodSandbox(context.Background(),
				&types.RunPodSandboxRequest{Config: &types.PodSandboxConfig{
					Metadata: &types.PodSandboxMetadata{},
				}})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail when metadata namespace is not provided", func() {
			// Given
			// When
			response, err := sut.RunPodSandbox(context.Background(),
				&types.RunPodSandboxRequest{Config: &types.PodSandboxConfig{
					Metadata: &types.PodSandboxMetadata{
						Name: "name",
					},
				}})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})

		It("should fail with relative log path", func() {
			// Given
			gomock.InOrder(
				runtimeServerMock.EXPECT().CreatePodSandbox(gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(),
					gomock.Any()).
					Return(storage.ContainerInfo{}, nil),
				runtimeServerMock.EXPECT().DeleteContainer(gomock.Any()).
					Return(nil),
			)

			// When
			response, err := sut.RunPodSandbox(context.Background(),
				&types.RunPodSandboxRequest{Config: &types.PodSandboxConfig{
					Metadata: &types.PodSandboxMetadata{
						Name:      "name",
						Namespace: "default",
						UID:       "uid",
					},
					Linux: &types.LinuxPodSandboxConfig{
						SecurityContext: types.NewLinuxSandboxSecurityContext(),
					},
				}})

			// Then
			Expect(err).NotTo(BeNil())
			Expect(response).To(BeNil())
		})
	})
})
