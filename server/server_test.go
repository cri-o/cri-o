package server_test

import (
	"context"
	"os"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	cstorage "go.podman.io/storage"
	"go.uber.org/mock/gomock"

	"github.com/cri-o/cri-o/server"
)

// The actual test suite.
var _ = t.Describe("Server", func() {
	// Prepare the sut
	BeforeEach(beforeEach)
	AfterEach(afterEach)

	// Test constants
	const (
		invalidDir = "/proc/invalid"
		invalid    = "invalid"
	)

	t.Describe("New", func() {
		It("should succeed", func() {
			// Given
			mockNewServer()

			// When
			server, err := server.New(context.Background(), libMock)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(server).NotTo(BeNil())
			Expect(server.StreamingServerCloseChan()).NotTo(BeNil())
		})

		It("should succeed with valid config path", func() {
			// Given
			mockNewServer()

			// When
			server, err := server.New(context.Background(), libMock)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(server).NotTo(BeNil())
			Expect(server.StreamingServerCloseChan()).NotTo(BeNil())
		})

		It("should succeed with valid GID/UID mappings", func() {
			// Given
			mockNewServer()
			serverConfig.UIDMappings = "1:1:1"
			serverConfig.GIDMappings = "1:1:1"

			// When
			server, err := server.New(context.Background(), libMock)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(server).NotTo(BeNil())
		})

		It("should succeed with enabled TLS", func() {
			// Given
			mockNewServer()
			serverConfig.StreamEnableTLS = true
			serverConfig.StreamTLSKey = "../test/testdata/key.pem"
			serverConfig.StreamTLSCert = "../test/testdata/cert.pem"

			// When
			server, err := server.New(context.Background(), libMock)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(server).NotTo(BeNil())
		})

		It("should succeed with container restore", func() {
			// Given
			gomock.InOrder(
				cniPluginMock.EXPECT().Status().Return(nil),
				libMock.EXPECT().GetData().Times(2).Return(serverConfig),
				libMock.EXPECT().GetStore().Return(storeMock, nil),
				libMock.EXPECT().GetData().Return(serverConfig),
				storeMock.EXPECT().Containers().
					Return([]cstorage.Container{
						{
							ID:      "1111111111111111111111111111111111111111111111111111111111111111",
							ImageID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
						},
						{
							ID:      "2222222222222222222222222222222222222222222222222222222222222222",
							ImageID: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
						},
						{
							ID:      "3333333333333333333333333333333333333333333333333333333333333333",
							ImageID: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb",
						},
					}, nil),
				storeMock.EXPECT().Metadata(gomock.Any()).
					Return(`{"Pod": false, "pod-name": "name", "pod-id": "id" }`, nil),
				storeMock.EXPECT().Metadata(gomock.Any()).
					Return(`{"Pod": true, "pod-name": "name", "pod-id": "id" }`, nil),
				storeMock.EXPECT().Metadata(gomock.Any()).
					Return("", t.TestError),
				storeMock.EXPECT().
					FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return([]byte{}, nil),
				storeMock.EXPECT().
					FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return([]byte{}, nil),
				cniPluginMock.EXPECT().GC(gomock.Any(), gomock.Len(0)).
					Return(nil),
			)
			Expect(serverConfig.SetCNIPlugin(cniPluginMock)).To(Succeed())

			// When
			server, err := server.New(context.Background(), libMock)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(server).NotTo(BeNil())
		})

		It("should fail when provided config is nil", func() {
			// Given
			// When
			server, err := server.New(context.Background(), nil)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(server).To(BeNil())
		})

		It("should fail when socket dir creation errors", func() {
			// Given
			gomock.InOrder(
				libMock.EXPECT().GetData().Times(2).Return(serverConfig),
			)
			serverConfig.ContainerAttachSocketDir = invalidDir

			// When
			server, err := server.New(context.Background(), libMock)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(server).To(BeNil())
		})

		It("should fail when container exits dir creation errors", func() {
			// Given
			gomock.InOrder(
				libMock.EXPECT().GetData().Times(2).Return(serverConfig),
			)
			serverConfig.ContainerExitsDir = invalidDir

			// When
			server, err := server.New(context.Background(), libMock)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(server).To(BeNil())
		})

		DescribeTable("should fail with wrong ID mappings", func(u, g string) {
			// Given
			gomock.InOrder(
				libMock.EXPECT().GetData().Times(2).Return(serverConfig),
				libMock.EXPECT().GetStore().Return(storeMock, nil),
				libMock.EXPECT().GetData().Return(serverConfig),
			)

			serverConfig.UIDMappings = u
			serverConfig.GIDMappings = g

			// When
			sut, err := server.New(context.Background(), libMock)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(sut).To(BeNil())
		},
			Entry("cid", "w:1:1", "w:1:1"),
			Entry("hid", "1:w:1", "1:w:1"),
			Entry("sz", "1:1:w", "1:1:w"),
		)

		It("should fail with invalid stream address and port", func() {
			// Given
			mockNewServer()
			serverConfig.StreamAddress = invalid
			serverConfig.StreamPort = invalid

			// When
			server, err := server.New(context.Background(), libMock)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(server).To(BeNil())
		})

		It("should fail with invalid TLS certificates", func() {
			// Given
			mockNewServer()
			serverConfig.StreamEnableTLS = true
			serverConfig.StreamTLSCert = invalid
			serverConfig.StreamTLSKey = invalid

			// When
			server, err := server.New(context.Background(), libMock)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(server).To(BeNil())
		})
		It("should fail with invalid timeout duration", func() {
			mockNewServer()
			serverConfig.StreamIdleTimeout = "invalid duration"

			server, err := server.New(context.Background(), libMock)
			Expect(err).To(HaveOccurred())
			Expect(server).To(BeNil())
		})
		It("should succeed to set a valid timeout duration", func() {
			mockNewServer()
			serverConfig.StreamIdleTimeout = "200ms"

			server, err := server.New(context.Background(), libMock)
			Expect(err).ToNot(HaveOccurred())
			Expect(server).ToNot(BeNil())
		})
		It("should succeed with hostport mapping disabled", func() {
			mockNewServer()
			serverConfig.DisableHostPortMapping = true

			server, err := server.New(context.Background(), libMock)
			Expect(err).ToNot(HaveOccurred())
			Expect(server).ToNot(BeNil())
		})
	})

	t.Describe("StartExitMonitor", func() {
		// Prepare the sut
		BeforeEach(setupSUT)

		It("should succeed", func() {
			// Given
			go sut.StartExitMonitor(context.Background())
			closeChan := sut.MonitorsCloseChan()
			Expect(closeChan).NotTo(BeNil())

			// When
			closeChan <- struct{}{}

			// Then
			Expect(closeChan).NotTo(BeNil())
		})
	})

	t.Describe("Shutdown", func() {
		// Prepare the sut
		BeforeEach(setupSUT)

		It("should succeed", func() {
			// Given
			gomock.InOrder(
				storeMock.EXPECT().Shutdown(gomock.Any()).Return(nil, nil),
				storeMock.EXPECT().GraphRoot().Return(emptyDir),
			)

			// When
			err := sut.Shutdown(context.Background())

			// Then
			Expect(err).ToNot(HaveOccurred())

			// expect cri-o to have created the clean shutdown file
			_, err = os.Stat(sut.Config().CleanShutdownFile)
			Expect(err).ToNot(HaveOccurred())
		})
	})

	t.Describe("StopStreamServer", func() {
		// Prepare the sut
		BeforeEach(setupSUT)

		It("should succeed", func() {
			// Given
			// When
			err := sut.StopStreamServer()

			// Then
			Expect(err).ToNot(HaveOccurred())
		})
	})
})
