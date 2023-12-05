package lib_test

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"time"

	"github.com/containers/podman/v4/pkg/annotations"
	"github.com/cri-o/cri-o/internal/lib"
	"github.com/cri-o/cri-o/internal/oci"
	libconfig "github.com/cri-o/cri-o/pkg/config"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"
)

// The actual test suite
var _ = t.Describe("ContainerServer", func() {
	ctx := context.TODO()
	// Prepare the sut
	BeforeEach(beforeEach)

	t.Describe("New", func() {
		It("should succeed with default config", func() {
			// Given
			// Create temp lockfile
			tmpfile, err := os.CreateTemp("", "lockfile")
			Expect(err).To(BeNil())
			defer os.Remove(tmpfile.Name())

			// Setup config
			config, err := libconfig.DefaultConfig()
			Expect(err).To(BeNil())
			config.HooksDir = []string{}
			// so we have permission to make a directory within it
			config.ContainerAttachSocketDir = t.MustTempDir("crio")

			// Specify mocks
			gomock.InOrder(
				libMock.EXPECT().GetStore().Return(storeMock, nil),
				libMock.EXPECT().GetData().Return(config),
			)

			// When
			server, err := lib.New(context.Background(), libMock)

			// Then
			Expect(err).To(BeNil())
			Expect(server).NotTo(BeNil())
		})

		It("should fail when GetStore fails", func() {
			// Given
			gomock.InOrder(
				libMock.EXPECT().GetStore().Return(nil, t.TestError),
			)

			// When
			server, err := lib.New(context.Background(), libMock)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(server).To(BeNil())
		})

		It("should fail when config is nil", func() {
			// Given
			// When
			server, err := lib.New(context.Background(), nil)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(server).To(BeNil())
		})
	})

	t.Describe("Getter", func() {
		It("should succeed to get the Runtime", func() {
			// Given
			// When
			res := sut.Runtime()

			// Then
			Expect(res).NotTo(BeNil())
		})

		It("should succeed to get the Store", func() {
			// Given
			// When
			res := sut.Store()

			// Then
			Expect(res).NotTo(BeNil())
			Expect(res).To(Equal(storeMock))
		})

		It("should succeed to get the StorageImageServer", func() {
			// Given
			// When
			res := sut.StorageImageServer()

			// Then
			Expect(res).NotTo(BeNil())
		})

		It("should succeed to get the CtrIDIndex", func() {
			// Given
			// When
			res := sut.CtrIDIndex()

			// Then
			Expect(res).NotTo(BeNil())
		})

		It("should succeed to get the PodIDIndex", func() {
			// Given
			// When
			res := sut.PodIDIndex()

			// Then
			Expect(res).NotTo(BeNil())
		})

		It("should succeed to get the Config", func() {
			// Given
			// When
			res := sut.Config()

			// Then
			Expect(res).NotTo(BeNil())
		})

		It("should succeed to get the StorageRuntimeServer", func() {
			// Given
			// When
			res := sut.StorageRuntimeServer()

			// Then
			Expect(res).NotTo(BeNil())
		})
	})

	t.Describe("LoadSandbox", func() {
		It("should succeed", func() {
			// Given
			createDummyState()
			mockDirs(testManifest)

			// When
			sb, err := sut.LoadSandbox(context.Background(), "id")

			// Then
			Expect(sb).NotTo(BeNil())
			Expect(err).To(BeNil())
		})

		It("should succeed with invalid network namespace", func() {
			// Given
			createDummyState()
			manifest := bytes.Replace(testManifest,
				[]byte(`{"type": "network", "path": "default"}`),
				[]byte(`{"type": "", "path": ""},{"type": "network", "path": ""}`), 1,
			)
			mockDirs(manifest)

			// When
			sb, err := sut.LoadSandbox(context.Background(), "id")

			// Then
			Expect(sb).NotTo(BeNil())
			Expect(err).To(BeNil())
		})

		It("should succeed with missing network namespace", func() {
			// Given
			createDummyState()
			manifest := bytes.Replace(testManifest,
				[]byte(`{"type": "network", "path": "default"}`),
				[]byte(`{}`), 1,
			)
			mockDirs(manifest)

			// When
			sb, err := sut.LoadSandbox(context.Background(), "id")

			// Then
			Expect(sb).NotTo(BeNil())
			Expect(err).To(BeNil())
		})

		It("should fail with empty pod ID", func() {
			// Given
			mockDirs(testManifest)

			// When
			sb, err := sut.LoadSandbox(context.Background(), "")

			// Then
			Expect(sb).NotTo(BeNil())
			Expect(err).NotTo(BeNil())
		})

		It("should fail with wrong container ID", func() {
			// Given
			manifest := bytes.Replace(testManifest,
				[]byte(`"io.kubernetes.cri-o.ContainerID": "sandboxID",`),
				[]byte(`"io.kubernetes.cri-o.ContainerID": "",`), 1,
			)
			mockDirs(manifest)

			// When
			sb, err := sut.LoadSandbox(context.Background(), "id")

			// Then
			Expect(sb).NotTo(BeNil())
			Expect(err).NotTo(BeNil())
		})

		It("should fail with wrong container volumes", func() {
			// Given
			manifest := bytes.Replace(testManifest,
				[]byte(`"io.kubernetes.cri-o.Volumes": "[{}]",`),
				[]byte(`"io.kubernetes.cri-o.Volumes": "wrong",`), 1,
			)
			mockDirs(manifest)

			// When
			sb, err := sut.LoadSandbox(context.Background(), "id")

			// Then
			Expect(sb).NotTo(BeNil())
			Expect(err).NotTo(BeNil())
		})

		It("should fail with failing container directory", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().
					FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return(testManifest, nil),
				storeMock.EXPECT().ContainerRunDirectory(gomock.Any()).
					Return("", nil),
				storeMock.EXPECT().ContainerDirectory(gomock.Any()).
					Return("", t.TestError),
			)

			// When
			sb, err := sut.LoadSandbox(context.Background(), "id")

			// Then
			Expect(sb).NotTo(BeNil())
			Expect(err).NotTo(BeNil())
		})

		It("should fail with failing container run directory", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().
					FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return(testManifest, nil),
				storeMock.EXPECT().ContainerRunDirectory(gomock.Any()).
					Return("", t.TestError),
			)

			// When
			sb, err := sut.LoadSandbox(context.Background(), "id")

			// Then
			Expect(sb).NotTo(BeNil())
			Expect(err).NotTo(BeNil())
		})

		It("should fail with invalid namespace options", func() {
			// Given
			manifest := bytes.Replace(testManifest,
				[]byte(`"io.kubernetes.cri-o.NamespaceOptions": "{}",`),
				[]byte(`"io.kubernetes.cri-o.NamespaceOptions": "",`), 1,
			)
			gomock.InOrder(
				storeMock.EXPECT().
					FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return(manifest, nil),
			)

			// When
			sb, err := sut.LoadSandbox(context.Background(), "id")

			// Then
			Expect(sb).To(BeNil())
			Expect(err).NotTo(BeNil())
		})

		It("should fail with invalid port mappings", func() {
			// Given
			manifest := bytes.Replace(testManifest,
				[]byte(`"io.kubernetes.cri-o.PortMappings": "[]",`),
				[]byte(`"io.kubernetes.cri-o.PortMappings": "{}",`), 1,
			)
			gomock.InOrder(
				storeMock.EXPECT().
					FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return(manifest, nil),
			)

			// When
			sb, err := sut.LoadSandbox(context.Background(), "id")

			// Then
			Expect(sb).To(BeNil())
			Expect(err).NotTo(BeNil())
		})

		It("should fail with invalid kube annotations", func() {
			// Given
			manifest := bytes.Replace(testManifest,
				[]byte(`"io.kubernetes.cri-o.Annotations": "{}",`),
				[]byte(`"io.kubernetes.cri-o.Annotations": "",`), 1,
			)
			gomock.InOrder(
				storeMock.EXPECT().
					FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return(manifest, nil),
			)

			// When
			sb, err := sut.LoadSandbox(context.Background(), "id")

			// Then
			Expect(sb).To(BeNil())
			Expect(err).NotTo(BeNil())
		})

		It("should fail with invalid metadata", func() {
			// Given
			manifest := bytes.Replace(testManifest,
				[]byte(`"io.kubernetes.cri-o.Metadata": "{}",`),
				[]byte(`"io.kubernetes.cri-o.Metadata": "",`), 1,
			)
			gomock.InOrder(
				storeMock.EXPECT().
					FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return(manifest, nil),
			)

			// When
			sb, err := sut.LoadSandbox(context.Background(), "id")

			// Then
			Expect(sb).To(BeNil())
			Expect(err).NotTo(BeNil())
		})

		It("should fail with invalid labels", func() {
			// Given
			manifest := bytes.Replace(testManifest,
				[]byte(`"io.kubernetes.cri-o.Labels": "{}",`),
				[]byte(`"io.kubernetes.cri-o.Labels": "",`), 1,
			)
			gomock.InOrder(
				storeMock.EXPECT().
					FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return(manifest, nil),
			)

			// When
			sb, err := sut.LoadSandbox(context.Background(), "id")

			// Then
			Expect(sb).To(BeNil())
			Expect(err).NotTo(BeNil())
		})

		It("should fail with invalid network selinux labels", func() {
			// Given
			manifest := bytes.Replace(testManifest,
				[]byte(`"selinuxLabel": "system_u:system_r:container_runtime_t:s0"`),
				[]byte(`"selinuxLabel": "wrong"`), 1,
			)
			gomock.InOrder(
				storeMock.EXPECT().
					FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return(manifest, nil),
			)

			// When
			sb, err := sut.LoadSandbox(context.Background(), "id")

			// Then
			Expect(sb).NotTo(BeNil())
			Expect(err).NotTo(BeNil())
		})

		It("should fail with container directory", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().
					FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			sb, err := sut.LoadSandbox(context.Background(), "id")

			// Then
			Expect(sb).To(BeNil())
			Expect(err).NotTo(BeNil())
		})

		It("should fail with wrong PodLinuxOverhead", func() {
			// Given
			manifest := bytes.Replace(testManifest,
				[]byte(`"io.kubernetes.cri-o.PodLinuxOverhead": "{}",`),
				[]byte(`"io.kubernetes.cri-o.PodLinuxOverhead": "wrong",`), 1,
			)
			gomock.InOrder(
				storeMock.EXPECT().
					FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return(manifest, nil),
			)

			// When
			sb, err := sut.LoadSandbox(context.Background(), "id")

			// Then
			Expect(sb).To(BeNil())
			Expect(err).NotTo(BeNil())
		})

		It("should fail with wrong PodLinuxResources", func() {
			// Given
			manifest := bytes.Replace(testManifest,
				[]byte(`"io.kubernetes.cri-o.PodLinuxResources": "{}",`),
				[]byte(`"io.kubernetes.cri-o.PodLinuxResources": "wrong",`), 1,
			)
			gomock.InOrder(
				storeMock.EXPECT().
					FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return(manifest, nil),
			)

			// When
			sb, err := sut.LoadSandbox(context.Background(), "id")

			// Then
			Expect(sb).To(BeNil())
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("LoadContainer", func() {
		It("should succeed", func() {
			// Given
			createDummyState()
			Expect(sut.AddSandbox(ctx, mySandbox)).To(BeNil())
			mockDirs(testManifest)

			// When
			err := sut.LoadContainer(context.Background(), "id")

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail with failing FromContainerDirectory", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().
					FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			err := sut.LoadContainer(context.Background(), "id")

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with failing ContainerRunDirectory", func() {
			// Given
			Expect(sut.AddSandbox(ctx, mySandbox)).To(BeNil())
			gomock.InOrder(
				storeMock.EXPECT().
					FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return(testManifest, nil),
				storeMock.EXPECT().ContainerRunDirectory(gomock.Any()).
					Return("", t.TestError),
			)

			// When
			err := sut.LoadContainer(context.Background(), "id")

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with failing ContainerDirectory", func() {
			// Given
			Expect(sut.AddSandbox(ctx, mySandbox)).To(BeNil())
			gomock.InOrder(
				storeMock.EXPECT().
					FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return(testManifest, nil),
				storeMock.EXPECT().ContainerRunDirectory(gomock.Any()).
					Return("", nil),
				storeMock.EXPECT().ContainerDirectory(gomock.Any()).
					Return("", t.TestError),
			)

			// When
			err := sut.LoadContainer(context.Background(), "id")

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with invalid manifest", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().
					FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return([]byte{}, nil),
			)

			// When
			err := sut.LoadContainer(context.Background(), "id")

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with invalid labels", func() {
			// Given
			manifest := bytes.Replace(testManifest,
				[]byte(`"io.kubernetes.cri-o.Labels": "{}",`),
				[]byte(`"io.kubernetes.cri-o.Labels": "",`), 1,
			)
			gomock.InOrder(
				storeMock.EXPECT().
					FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return(manifest, nil),
			)

			// When
			err := sut.LoadContainer(context.Background(), "id")

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with invalid metadata", func() {
			// Given
			manifest := bytes.Replace(testManifest,
				[]byte(`"io.kubernetes.cri-o.Metadata": "{}",`),
				[]byte(`"io.kubernetes.cri-o.Metadata": "",`), 1,
			)
			gomock.InOrder(
				storeMock.EXPECT().
					FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return(manifest, nil),
			)

			// When
			err := sut.LoadContainer(context.Background(), "id")

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with invalid kube annotations", func() {
			// Given
			manifest := bytes.Replace(testManifest,
				[]byte(`"io.kubernetes.cri-o.Annotations": "{}",`),
				[]byte(`"io.kubernetes.cri-o.Annotations": "",`), 1,
			)
			gomock.InOrder(
				storeMock.EXPECT().
					FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return(manifest, nil),
			)

			// When
			err := sut.LoadContainer(context.Background(), "id")

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with non CRI-O managed container", func() {
			// Given
			manifest := bytes.Replace(testManifest,
				[]byte(`"io.kubernetes.cri-o.Annotations": "{}",`),
				[]byte(fmt.Sprintf("%q: %q,", annotations.ContainerManager,
					annotations.ContainerManagerLibpod)), 1,
			)
			gomock.InOrder(
				storeMock.EXPECT().
					FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return(manifest, nil),
			)

			// When
			err := sut.LoadContainer(context.Background(), "id")

			// Then
			Expect(err).To(Equal(lib.ErrIsNonCrioContainer))
		})
	})

	t.Describe("ContainerStateToDisk", func() {
		It("should fail when state path invalid", func() {
			// Given
			container, err := oci.NewContainer(containerID, "", "", "",
				make(map[string]string), make(map[string]string),
				make(map[string]string), "", nil, nil,
				&types.ContainerMetadata{}, sandboxID, false,
				false, false, "", "/invalid", time.Now(), "")
			Expect(err).To(BeNil())

			// When
			err = sut.ContainerStateToDisk(context.Background(), container)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("ReserveContainerName", func() {
		It("should succeed", func() {
			// Given
			// When
			name, err := sut.ReserveContainerName("id", "name")

			// Then
			Expect(err).To(BeNil())
			Expect(name).To(Equal("name"))
		})

		It("should fail when reserved twice", func() {
			// Given
			_, err := sut.ReserveContainerName("someID", "name")
			Expect(err).To(BeNil())

			// When
			_, err = sut.ReserveContainerName("anotherID", "name")

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("ReservePodName", func() {
		It("should succeed", func() {
			// Given
			// When
			name, err := sut.ReservePodName("id", "name")

			// Then
			Expect(err).To(BeNil())
			Expect(name).To(Equal("name"))
		})

		It("should fail when reserved twice", func() {
			// Given
			_, err := sut.ReservePodName("someID", "name")
			Expect(err).To(BeNil())

			// When
			_, err = sut.ReservePodName("anotherID", "name")

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("Shutdown", func() {
		It("should succeed", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Shutdown(gomock.Any()).
					Return([]string{}, nil),
			)

			// When
			err := sut.Shutdown()

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail when storage shutdown fails", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Shutdown(gomock.Any()).
					Return(nil, t.TestError),
			)

			// When
			err := sut.Shutdown()

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("AddContainer/AddSandbox", func() {
		It("should succeed", func() {
			// Given
			Expect(sut.AddSandbox(ctx, mySandbox)).To(BeNil())
			sut.AddContainer(ctx, myContainer)

			// When
			hasSandbox := sut.HasSandbox(mySandbox.ID())
			hasContainer := sut.HasContainer(myContainer.ID())

			// Then
			Expect(hasSandbox).To(BeTrue())
			Expect(sut.GetSandboxContainer(mySandbox.ID())).To(BeNil())
			Expect(hasContainer).To(BeTrue())
		})

		It("should fail when sandbox not available", func() {
			// Given
			sut.AddContainer(ctx, myContainer)

			// When
			hasSandbox := sut.HasSandbox(mySandbox.ID())
			hasContainer := sut.HasContainer(myContainer.ID())

			// Then
			Expect(hasSandbox).To(BeFalse())
			Expect(hasContainer).To(BeFalse())
		})
	})

	t.Describe("RemoveContainer/RemoveSandbox", func() {
		It("should succeed", func() {
			// Given
			Expect(sut.AddSandbox(ctx, mySandbox)).To(BeNil())
			sut.AddContainer(ctx, myContainer)
			sut.RemoveContainer(ctx, myContainer)
			Expect(sut.RemoveSandbox(ctx, mySandbox.ID())).To(BeNil())

			// When
			hasSandbox := sut.HasSandbox(mySandbox.ID())
			hasContainer := sut.HasContainer(myContainer.ID())

			// Then
			Expect(hasSandbox).To(BeFalse())
			Expect(sut.GetSandboxContainer(mySandbox.ID())).To(BeNil())
			Expect(hasContainer).To(BeFalse())
		})

		It("should fail to remove container when sandbox not available", func() {
			// Given
			Expect(sut.AddSandbox(ctx, mySandbox)).To(BeNil())
			sut.AddContainer(ctx, myContainer)
			Expect(sut.RemoveSandbox(ctx, mySandbox.ID())).To(BeNil())
			sut.RemoveContainer(ctx, myContainer)

			// When
			hasSandbox := sut.HasSandbox(mySandbox.ID())
			hasContainer := sut.HasContainer(myContainer.ID())

			// Then
			Expect(hasSandbox).To(BeFalse())
			Expect(hasContainer).To(BeTrue())
		})

		It("should fail to remove sandbox when not available", func() {
			// Given
			Expect(sut.RemoveSandbox(ctx, mySandbox.ID())).To(BeNil())

			// When
			hasSandbox := sut.HasSandbox(mySandbox.ID())

			// Then
			Expect(hasSandbox).To(BeFalse())
		})
	})

	t.Describe("ListContainer/ListSandbox", func() {
		It("should succeed", func() {
			// Given
			Expect(sut.AddSandbox(ctx, mySandbox)).To(BeNil())
			sut.AddContainer(ctx, myContainer)

			// When
			sandboxes := sut.ListSandboxes()
			containers, err := sut.ListContainers()

			// Then
			Expect(err).To(BeNil())
			Expect(len(sandboxes)).To(Equal(1))
			Expect(len(containers)).To(Equal(1))
		})

		It("should succeed filtered", func() {
			// Given
			Expect(sut.AddSandbox(ctx, mySandbox)).To(BeNil())
			sut.AddContainer(ctx, myContainer)

			// When
			sandboxes := sut.ListSandboxes()
			containers, err := sut.ListContainers(
				func(container *oci.Container) bool {
					return true
				},
				func(container *oci.Container) bool {
					return true
				})

			// Then
			Expect(err).To(BeNil())
			Expect(len(sandboxes)).To(Equal(1))
			Expect(len(containers)).To(Equal(1))
		})
	})

	t.Describe("AddInfraContainer", func() {
		It("should succeed", func() {
			// Given
			sut.AddInfraContainer(ctx, myContainer)

			// When
			container := sut.GetInfraContainer(ctx, myContainer.ID())

			// Then
			Expect(container).NotTo(BeNil())
		})
	})

	t.Describe("RemoveInfraContainer", func() {
		It("should succeed", func() {
			// Given
			sut.AddInfraContainer(ctx, myContainer)
			sut.RemoveInfraContainer(ctx, myContainer)

			// When
			container := sut.GetInfraContainer(ctx, myContainer.ID())

			// Then
			Expect(container).To(BeNil())
		})
	})
})
