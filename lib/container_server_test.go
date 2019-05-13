package lib_test

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"time"

	cstorage "github.com/containers/storage"
	"github.com/cri-o/cri-o/lib"
	"github.com/cri-o/cri-o/oci"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// The actual test suite
var _ = t.Describe("ContainerServer", func() {
	// Prepare the sut
	BeforeEach(beforeEach)

	t.Describe("New", func() {
		It("should succeed with default config", func() {
			// Given
			// Create temp lockfile
			tmpfile, err := ioutil.TempFile("", "lockfile")
			Expect(err).To(BeNil())
			defer os.Remove(tmpfile.Name())

			// Setup config
			config, err := lib.DefaultConfig(nil)
			Expect(err).To(BeNil())
			config.FileLockingPath = tmpfile.Name()
			config.HooksDir = []string{}

			// Specify mocks
			gomock.InOrder(
				libMock.EXPECT().GetStore().Return(storeMock, nil),
				libMock.EXPECT().GetData().Return(config),
			)

			// When
			server, err := lib.New(context.Background(), nil, libMock)

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
			server, err := lib.New(context.Background(), nil, libMock)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(server).To(BeNil())
		})

		It("should fail when Store is nil", func() {
			config, err := lib.DefaultConfig(nil)
			Expect(err).To(BeNil())
			// Given
			gomock.InOrder(
				libMock.EXPECT().GetStore().Return(nil, nil),
				libMock.EXPECT().GetData().Return(config),
			)

			// When
			server, err := lib.New(context.Background(), nil, libMock)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(server).To(BeNil())
		})

		It("should fail when config is nil", func() {
			// Given
			// When
			server, err := lib.New(context.Background(), nil, nil)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(server).To(BeNil())
		})

		It("should fail with invalid default runtime", func() {
			// Given
			config, err := lib.DefaultConfig(nil)
			Expect(err).To(BeNil())
			config.DefaultRuntime = "invalid-runtime"
			gomock.InOrder(
				libMock.EXPECT().GetStore().Return(storeMock, nil),
				libMock.EXPECT().GetData().Return(config),
			)

			// When
			server, err := lib.New(context.Background(), nil, libMock)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(server).To(BeNil())
		})

		It("should fail with invalid lockfile", func() {
			// Given
			config, err := lib.DefaultConfig(nil)
			Expect(err).To(BeNil())
			config.FileLocking = true
			config.FileLockingPath = "/invalid/file"
			gomock.InOrder(
				libMock.EXPECT().GetStore().Return(storeMock, nil),
				libMock.EXPECT().GetData().Return(config),
			)

			// When
			server, err := lib.New(context.Background(), nil, libMock)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(server).To(BeNil())
		})

		It("should fail with invalid hooks dir", func() {
			// Given
			config, err := lib.DefaultConfig(nil)
			Expect(err).To(BeNil())
			config.FileLocking = false
			config.HooksDir = []string{"/invalid-dir"}
			gomock.InOrder(
				libMock.EXPECT().GetStore().Return(storeMock, nil),
				libMock.EXPECT().GetData().Return(config),
			)

			// When
			server, err := lib.New(context.Background(), nil, libMock)

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

		It("should succeed to get the CtrNameIndex", func() {
			// Given
			// When
			res := sut.CtrNameIndex()

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

		It("should succeed to get the PodNameIndex", func() {
			// Given
			// When
			res := sut.PodNameIndex()

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

	t.Describe("Update", func() {
		It("should succeed", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Containers().
					Return([]cstorage.Container{}, nil),
			)

			// When
			err := sut.Update()

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed with containers", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Containers().
					Return([]cstorage.Container{
						{},
					}, nil),
				storeMock.EXPECT().Metadata(gomock.Any()).
					Return(`{"Pod": false}`, nil),
				storeMock.EXPECT().
					FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return([]byte{}, nil),
			)

			// When
			err := sut.Update()

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed with container pod metadata", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Containers().
					Return([]cstorage.Container{
						{},
					}, nil),
				storeMock.EXPECT().Metadata(gomock.Any()).
					Return(`{"Pod": true}`, nil),
				storeMock.EXPECT().
					FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return([]byte{}, nil),
			)

			// When
			err := sut.Update()

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed with sandbox", func() {
			// Given
			sut.AddSandbox(mySandbox)
			gomock.InOrder(
				storeMock.EXPECT().Containers().
					Return([]cstorage.Container{{ID: sandboxID}}, nil),
			)

			// When
			err := sut.Update()

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed with container", func() {
			// Given
			sut.AddSandbox(mySandbox)
			sut.AddContainer(myContainer)
			gomock.InOrder(
				storeMock.EXPECT().Containers().
					Return([]cstorage.Container{{ID: containerID}}, nil),
			)

			// When
			err := sut.Update()

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed when metadata retrieval fails", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Containers().
					Return([]cstorage.Container{
						{},
					}, nil),
				storeMock.EXPECT().Metadata(gomock.Any()).
					Return("", t.TestError),
			)

			// When
			err := sut.Update()

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed with removed container", func() {
			// Given
			mockDirs(testManifest)
			createDummyState()
			gomock.InOrder(
				storeMock.EXPECT().Containers().
					Return([]cstorage.Container{{}}, nil),
				storeMock.EXPECT().Metadata(gomock.Any()).
					Return("", t.TestError),
			)
			err := sut.LoadSandbox("id")
			Expect(err).To(BeNil())

			// When
			err = sut.Update()

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed with already added sandbox", func() {
			// Given
			sut.AddSandbox(mySandbox)
			createDummyState()
			mockDirs(testManifest)
			gomock.InOrder(
				storeMock.EXPECT().Containers().
					Return([]cstorage.Container{{ID: sandboxID}}, nil),
			)
			err := sut.LoadSandbox("id")
			Expect(err).To(BeNil())

			// When
			err = sut.Update()

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail when storage fails", func() {
			// Given
			createDummyState()
			gomock.InOrder(
				storeMock.EXPECT().Containers().Return(nil, t.TestError),
			)

			// When
			err := sut.Update()

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("LoadSandbox", func() {
		It("should succeed", func() {
			// Given
			createDummyState()
			mockDirs(testManifest)

			// When
			err := sut.LoadSandbox("id")

			// Then
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
			err := sut.LoadSandbox("id")

			// Then
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
			err := sut.LoadSandbox("id")

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail with empty pod ID", func() {
			// Given
			mockDirs(testManifest)

			// When
			err := sut.LoadSandbox("")

			// Then
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
			err := sut.LoadSandbox("id")

			// Then
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
			err := sut.LoadSandbox("id")

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with wrong creation time", func() {
			// Given
			manifest := bytes.Replace(testManifest,
				[]byte(`"io.kubernetes.cri-o.Created": "2006-01-02T15:04:05.999999999Z",`),
				[]byte(`"io.kubernetes.cri-o.Created": "wrong",`), 1,
			)
			mockDirs(manifest)

			// When
			err := sut.LoadSandbox("id")

			// Then
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
			err := sut.LoadSandbox("id")

			// Then
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
			err := sut.LoadSandbox("id")

			// Then
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
			err := sut.LoadSandbox("id")

			// Then
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
			err := sut.LoadSandbox("id")

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
			err := sut.LoadSandbox("id")

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
			err := sut.LoadSandbox("id")

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
			err := sut.LoadSandbox("id")

			// Then
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
			err := sut.LoadSandbox("id")

			// Then
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
			err := sut.LoadSandbox("id")

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("LoadContainer", func() {
		It("should succeed", func() {
			// Given
			createDummyState()
			sut.AddSandbox(mySandbox)
			mockDirs(testManifest)

			// When
			err := sut.LoadContainer("id")

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
			err := sut.LoadContainer("id")

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with failing ContainerRunDirectory", func() {
			// Given
			sut.AddSandbox(mySandbox)
			gomock.InOrder(
				storeMock.EXPECT().
					FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return(testManifest, nil),
				storeMock.EXPECT().ContainerRunDirectory(gomock.Any()).
					Return("", t.TestError),
			)

			// When
			err := sut.LoadContainer("id")

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with failing ContainerDirectory", func() {
			// Given
			sut.AddSandbox(mySandbox)
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
			err := sut.LoadContainer("id")

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
			err := sut.LoadContainer("id")

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
			err := sut.LoadContainer("id")

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
			err := sut.LoadContainer("id")

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
			err := sut.LoadContainer("id")

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("ContainerStateFromDisk", func() {
		It("should succeed", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			gomock.InOrder(
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Times(2).Return(nil),
			)
			Expect(sut.ContainerStateToDisk(myContainer)).To(BeNil())
			defer os.Remove(myContainer.StatePath())

			// When
			err := sut.ContainerStateFromDisk(myContainer)

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail when file not found", func() {
			// Given
			// When
			err := sut.ContainerStateFromDisk(myContainer)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("ContainerStateToDisk", func() {
		It("should succeed", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			gomock.InOrder(
				ociRuntimeMock.EXPECT().UpdateContainerStatus(gomock.Any()).
					Return(nil),
			)

			// When
			err := sut.ContainerStateToDisk(myContainer)
			defer os.Remove(myContainer.StatePath())

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail when state path invalid", func() {
			// Given
			container, err := oci.NewContainer(containerID, "", "", "", "",
				make(map[string]string), make(map[string]string),
				make(map[string]string), "", "", "",
				&pb.ContainerMetadata{}, sandboxID, false, false,
				false, false, "", "/invalid", time.Now(), "")
			Expect(err).To(BeNil())

			// When
			err = sut.ContainerStateToDisk(container)

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
			sut.AddSandbox(mySandbox)
			sut.AddContainer(myContainer)

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
			sut.AddContainer(myContainer)

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
			sut.AddSandbox(mySandbox)
			sut.AddContainer(myContainer)
			sut.RemoveContainer(myContainer)
			sut.RemoveSandbox(mySandbox.ID())

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
			sut.AddSandbox(mySandbox)
			sut.AddContainer(myContainer)
			sut.RemoveSandbox(mySandbox.ID())
			sut.RemoveContainer(myContainer)

			// When
			hasSandbox := sut.HasSandbox(mySandbox.ID())
			hasContainer := sut.HasContainer(myContainer.ID())

			// Then
			Expect(hasSandbox).To(BeFalse())
			Expect(hasContainer).To(BeTrue())
		})

		It("should fail to remove sandbox when not available", func() {
			// Given
			sut.RemoveSandbox(mySandbox.ID())

			// When
			hasSandbox := sut.HasSandbox(mySandbox.ID())

			// Then
			Expect(hasSandbox).To(BeFalse())
		})
	})

	t.Describe("ListContainer/ListSandbox", func() {
		It("should succeed", func() {
			// Given
			sut.AddSandbox(mySandbox)
			sut.AddContainer(myContainer)

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
			sut.AddSandbox(mySandbox)
			sut.AddContainer(myContainer)

			// When
			sandboxes := sut.ListSandboxes()
			containers, err := sut.ListContainers(
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
			sut.AddInfraContainer(myContainer)

			// When
			container := sut.GetInfraContainer(myContainer.ID())

			// Then
			Expect(container).NotTo(BeNil())
		})
	})

	t.Describe("RemoveInfraContainer", func() {
		It("should succeed", func() {
			// Given
			sut.AddInfraContainer(myContainer)
			sut.RemoveInfraContainer(myContainer)

			// When
			container := sut.GetInfraContainer(myContainer.ID())

			// Then
			Expect(container).To(BeNil())
		})
	})
})
