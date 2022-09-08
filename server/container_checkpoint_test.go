package server_test

import (
	"context"
	"os"

	"github.com/containers/podman/v4/pkg/criu"
	cstorage "github.com/containers/storage"
	"github.com/containers/storage/pkg/archive"
	"github.com/cri-o/cri-o/internal/hostport"
	"github.com/cri-o/cri-o/internal/oci"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	specs "github.com/opencontainers/runtime-spec/specs-go"
	"google.golang.org/grpc/status"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

var _ = t.Describe("ContainerCheckpoint", func() {
	mockCheckpointContainer := func() {
		gomock.InOrder(
			multiStoreServerMock.EXPECT().GetStore().Return(multiStoreMock),
			multiStoreMock.EXPECT().GetStoreForContainer(gomock.Any()).Return(storeMock, nil),
			storeMock.EXPECT().Container(gomock.Any()).Return(&cstorage.Container{}, nil),
			multiStoreServerMock.EXPECT().GetStore().Return(multiStoreMock),
			multiStoreMock.EXPECT().GetStoreForContainer(gomock.Any()).Return(storeMock, nil),
			storeMock.EXPECT().Changes(gomock.Any(), gomock.Any()).Return([]archive.Change{}, nil),
			multiStoreServerMock.EXPECT().GetImageServerForContainer(gomock.Any()).Return(imageServerMock, nil),
			imageServerMock.EXPECT().GetStore().Return(storeMock),
			storeMock.EXPECT().Mount(gomock.Any(), gomock.Any()).Return("/tmp/", nil),
			runtimeServerMock.EXPECT().StopContainer(gomock.Any()).Return(nil),
		)
	}
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		createDummyConfig()
		mockRuncInLibConfig()
		if !criu.CheckForCriu(criu.PodCriuVersion) {
			Skip("CRIU is missing or too old.")
		}
		serverConfig.SetCheckpointRestore(true)
		setupSUT()
	})

	AfterEach(func() {
		afterEach()
		os.RemoveAll("config.dump")
		os.RemoveAll("cp.tar")
		os.RemoveAll("dump.log")
		os.RemoveAll("spec.dump")
	})

	t.Describe("ContainerCheckpoint", func() {
		It("should succeed", func() {
			// Given
			addContainerAndSandbox()

			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			testContainer.SetSpec(&specs.Spec{Version: "1.0.0"})

			gomock.InOrder(
				runtimeServerMock.EXPECT().StopContainer(gomock.Any()).
					Return(nil),
			)

			// When
			err := sut.CheckpointContainer(context.Background(),
				&types.CheckpointContainerRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail with invalid container id", func() {
			// Given
			// When
			err := sut.CheckpointContainer(context.Background(),
				&types.CheckpointContainerRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err).NotTo(BeNil())
		})
		It("should fail with valid pod id without archive", func() {
			// Given
			addContainerAndSandbox()
			// When
			err := sut.CheckpointContainer(context.Background(),
				&types.CheckpointContainerRequest{
					ContainerId: testSandbox.ID(),
				})

			// Then
			Expect(status.Convert(err).Message()).To(Equal("Pod checkpointing requires a destination file"))
		})
		It("should succeed with valid pod id and archive", func() {
			// Given
			addContainerAndSandbox()

			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			testContainer.SetSpec(&specs.Spec{Version: "1.0.0"})
			mockCheckpointContainer()

			// When
			err := sut.CheckpointContainer(context.Background(),
				&types.CheckpointContainerRequest{
					ContainerId: testSandbox.ID(),
					Location:    "cp.tar",
				})

			// Then
			Expect(err).To(BeNil())
		})
		It("should succeed with valid pod id and archive and DNSConfig and PortMapping", func() {
			// Given
			addContainerAndSandbox()

			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			testContainer.SetSpec(&specs.Spec{Version: "1.0.0"})
			testSandbox.SetDNSConfig(&types.DNSConfig{
				Servers:  []string{"server1", "server2"},
				Searches: []string{"searche1", "searches"},
				Options:  []string{"option1", "option2"},
			})
			testSandbox.SetPortMappings([]*hostport.PortMapping{
				{
					ContainerPort: 2222,
					HostPort:      1222,
					Protocol:      "TCP",
				},
				{
					ContainerPort: 2222,
					HostPort:      1223,
					Protocol:      "UDP",
				},
				{
					ContainerPort: 2222,
					HostIP:        "127.0.0.2",
					HostPort:      1224,
					Protocol:      "SCTP",
				},
			})

			mockCheckpointContainer()
			// When
			err := sut.CheckpointContainer(context.Background(),
				&types.CheckpointContainerRequest{
					ContainerId: testSandbox.ID(),
					Location:    "cp.tar",
				})

			// Then
			Expect(err).To(BeNil())
		})
		It("should fail with valid pod id and archive (with empty Container())", func() {
			// Given
			addContainerAndSandbox()

			testContainer.SetState(&oci.ContainerState{
				State: specs.State{Status: oci.ContainerStateRunning},
			})
			testContainer.SetSpec(&specs.Spec{Version: "1.0.0"})

			gomock.InOrder(
				multiStoreServerMock.EXPECT().GetStore().Return(multiStoreMock),
				multiStoreMock.EXPECT().GetStoreForContainer(gomock.Any()).Return(storeMock, nil),
				storeMock.EXPECT().Container(gomock.Any()).Return(nil, t.TestError),
			)
			// When
			err := sut.CheckpointContainer(context.Background(),
				&types.CheckpointContainerRequest{
					ContainerId: testSandbox.ID(),
					Location:    "cp.tar",
				})

			// Then
			Expect(err.Error()).To(Equal(`failed to write file system changes of container containerID: error exporting root file-system diff for "containerID": error`))
		})
	})
})

var _ = t.Describe("ContainerCheckpoint with CheckpointRestore set to false", func() {
	// Prepare the sut
	BeforeEach(func() {
		beforeEach()
		createDummyConfig()
		mockRuncInLibConfig()
		serverConfig.SetCheckpointRestore(false)
		setupSUT()
	})

	AfterEach(afterEach)

	t.Describe("ContainerCheckpoint", func() {
		It("should fail with checkpoint/restore support not available", func() {
			// Given
			// When
			err := sut.CheckpointContainer(context.Background(),
				&types.CheckpointContainerRequest{
					ContainerId: testContainer.ID(),
				})

			// Then
			Expect(err.Error()).To(Equal(`checkpoint/restore support not available`))
		})
	})
})
