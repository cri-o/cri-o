package server_test

import (
	"context"
	"io/ioutil"
	"os"

	"github.com/containers/image/v5/types"
	cstorage "github.com/containers/storage"
	"github.com/cri-o/cri-o/internal/pkg/signals"
	"github.com/cri-o/cri-o/server"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

// The actual test suite
var _ = t.Describe("Server", func() {
	// Prepare the sut
	BeforeEach(beforeEach)
	AfterEach(afterEach)

	// Test constants
	const (
		invalidDir = "/proc/invalid"
		invalid    = "invalid"
	)

	getTmpFile := func() string {
		tmpfile, err := ioutil.TempFile(os.TempDir(), "config")
		Expect(err).To(BeNil())
		return tmpfile.Name()
	}

	t.Describe("New", func() {
		It("should succeed", func() {
			// Given
			mockNewServer()

			// When
			server, err := server.New(context.Background(), nil, "", libMock)

			// Then
			Expect(err).To(BeNil())
			Expect(server).NotTo(BeNil())
			Expect(server.StreamingServerCloseChan()).NotTo(BeNil())
		})

		It("should succeed with valid config path", func() {
			// Given
			mockNewServer()
			tmpFile := getTmpFile()
			defer os.RemoveAll(tmpFile)

			// When
			server, err := server.New(
				context.Background(), nil, tmpFile, libMock,
			)

			// Then
			Expect(err).To(BeNil())
			Expect(server).NotTo(BeNil())
			Expect(server.StreamingServerCloseChan()).NotTo(BeNil())
		})

		It("should succeed with valid GID/UID mappings", func() {
			// Given
			mockNewServer()
			serverConfig.UIDMappings = "1:1:1"
			serverConfig.GIDMappings = "1:1:1"

			// When
			server, err := server.New(context.Background(), nil, "", libMock)

			// Then
			Expect(err).To(BeNil())
			Expect(server).NotTo(BeNil())
		})

		It("should succeed with enabled TLS", func() {
			// Given
			mockNewServer()
			serverConfig.StreamEnableTLS = true
			serverConfig.StreamTLSKey = "../test/testdata/key.pem"
			serverConfig.StreamTLSCert = "../test/testdata/cert.pem"

			// When
			server, err := server.New(context.Background(), nil, "", libMock)

			// Then
			Expect(err).To(BeNil())
			Expect(server).NotTo(BeNil())
		})

		It("should succeed with container restore", func() {
			// Given
			testError := errors.Wrap(errors.New("/dev/null"), "error")
			gomock.InOrder(
				libMock.EXPECT().GetData().Times(2).Return(serverConfig),
				libMock.EXPECT().GetStore().Return(storeMock, nil),
				libMock.EXPECT().GetData().Return(serverConfig),
				storeMock.EXPECT().Containers().
					Return([]cstorage.Container{
						{},
						{},
						{},
					}, testError),
				storeMock.EXPECT().Metadata(gomock.Any()).
					Return(`{"Pod": false}`, nil),
				storeMock.EXPECT().Metadata(gomock.Any()).
					Return(`{"Pod": true}`, nil),
				storeMock.EXPECT().Metadata(gomock.Any()).
					Return("", t.TestError),
				storeMock.EXPECT().
					FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return([]byte{}, nil),
				storeMock.EXPECT().
					FromContainerDirectory(gomock.Any(), gomock.Any()).
					Return([]byte{}, nil),
			)

			// When
			server, err := server.New(context.Background(), nil, "", libMock)

			// Then
			Expect(err).To(BeNil())
			Expect(server).NotTo(BeNil())
		})

		It("should fail when provided config is nil", func() {
			// Given
			// When
			server, err := server.New(context.Background(), nil, "", nil)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(server).To(BeNil())
		})

		It("should fail when socket dir creation erros", func() {
			// Given
			gomock.InOrder(
				libMock.EXPECT().GetData().Times(2).Return(serverConfig),
			)
			serverConfig.ContainerAttachSocketDir = invalidDir

			// When
			server, err := server.New(context.Background(), nil, "", libMock)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(server).To(BeNil())
		})

		It("should fail when container exits dir creation erros", func() {
			// Given
			gomock.InOrder(
				libMock.EXPECT().GetData().Times(2).Return(serverConfig),
			)
			serverConfig.ContainerExitsDir = invalidDir

			// When
			server, err := server.New(context.Background(), nil, "", libMock)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(server).To(BeNil())
		})

		It("should fail when CNI init errors", func() {
			// Given
			gomock.InOrder(
				libMock.EXPECT().GetData().Times(2).Return(serverConfig),
				libMock.EXPECT().GetStore().Return(storeMock, nil),
				libMock.EXPECT().GetData().Return(serverConfig),
			)
			serverConfig.NetworkDir = invalidDir

			// When
			server, err := server.New(context.Background(), nil, "", libMock)

			// Then
			Expect(err).NotTo(BeNil())
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
			sut, err := server.New(context.Background(), nil, "", libMock)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(sut).To(BeNil())
		},
			Entry("cid", "w:1:1", "w:1:1"),
			Entry("hid", "1:w:1", "1:w:1"),
			Entry("sz", "1:1:w", "1:1:w"),
		)

		It("should fail with inavailable seccomp profile", func() {
			// Given
			gomock.InOrder(
				libMock.EXPECT().GetData().Times(2).Return(serverConfig),
				libMock.EXPECT().GetStore().Return(storeMock, nil),
				libMock.EXPECT().GetData().Return(serverConfig),
			)
			serverConfig.SeccompProfile = invalidDir

			// When
			server, err := server.New(context.Background(), nil, "", libMock)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(server).To(BeNil())
		})

		It("should fail with wrong seccomp profile", func() {
			// Given
			gomock.InOrder(
				libMock.EXPECT().GetData().Times(2).Return(serverConfig),
				libMock.EXPECT().GetStore().Return(storeMock, nil),
				libMock.EXPECT().GetData().Return(serverConfig),
			)
			serverConfig.SeccompProfile = "/dev/null"

			// When
			server, err := server.New(context.Background(), nil, "", libMock)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(server).To(BeNil())
		})

		It("should fail with invalid stream address and port", func() {
			// Given
			mockNewServer()
			serverConfig.StreamAddress = invalid
			serverConfig.StreamPort = invalid

			// When
			server, err := server.New(context.Background(), nil, "", libMock)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(server).To(BeNil())
		})

		It("should fail with invalid TLS certificates", func() {
			// Given
			mockNewServer()
			serverConfig.StreamEnableTLS = true
			serverConfig.StreamTLSCert = invalid
			serverConfig.StreamTLSKey = invalid

			// When
			server, err := server.New(context.Background(), nil, "", libMock)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(server).To(BeNil())
		})
	})

	t.Describe("CreateMetricsEndpoint", func() {
		// Prepare the sut
		BeforeEach(setupSUT)

		It("should succeed", func() {
			// Given

			// When
			mux, err := sut.CreateMetricsEndpoint()

			// Then
			Expect(err).To(BeNil())
			Expect(mux).NotTo(BeNil())
			sut.StopMonitors()
		})
	})

	t.Describe("StartExitMonitor", func() {
		// Prepare the sut
		BeforeEach(setupSUT)

		It("should succeed", func() {
			// Given
			go sut.StartExitMonitor()
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
			)

			// When
			err := sut.Shutdown(context.Background())

			// Then
			Expect(err).To(BeNil())
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
			Expect(err).To(BeNil())
		})
	})

	t.Describe("StartConfigWatcher", func() {
		// Prepare the sut
		BeforeEach(setupSUT)

		It("should succeed", func() {
			// Given
			tmpFile := getTmpFile()
			defer os.RemoveAll(tmpFile)

			// When
			ch, err := sut.StartConfigWatcher(
				tmpFile, func(fileName string) error {
					return nil
				},
			)
			ch <- signals.Hup

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed with failing reload closure", func() {
			// Given
			tmpFile := getTmpFile()
			defer os.RemoveAll(tmpFile)

			// When
			ch, err := sut.StartConfigWatcher(
				tmpFile, func(fileName string) error {
					return t.TestError
				},
			)
			ch <- signals.Hup

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail when fileName does not exist", func() {
			// Given
			// When
			_, err := sut.StartConfigWatcher("", nil)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail when reload closure is nil", func() {
			// Given
			tmpFile := getTmpFile()
			defer os.RemoveAll(tmpFile)

			// When
			_, err := sut.StartConfigWatcher(tmpFile, nil)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("ReloadRegistries", func() {
		// The test registries file
		regConf := ""

		// Prepare the sut
		BeforeEach(func() {
			regConf = t.MustTempFile("reload-registries")
			ctx := &types.SystemContext{SystemRegistriesConfPath: regConf}
			setupSUTWithContext(ctx)
		})

		It("should succeed to reload registries", func() {
			// Given
			// When
			err := sut.ReloadRegistries(regConf)

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail if registries file got deleted", func() {
			// Given
			Expect(os.Remove(regConf)).To(BeNil())

			// When
			err := sut.ReloadRegistries(regConf)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail if registries file is invalid", func() {
			// Given
			Expect(ioutil.WriteFile(regConf, []byte("invalid"), 0755)).To(BeNil())

			// When
			err := sut.ReloadRegistries(regConf)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
