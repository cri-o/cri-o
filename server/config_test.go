package server_test

import (
	"os"
	"path"

	"github.com/cri-o/cri-o/oci"
	"github.com/cri-o/cri-o/server"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

var _ = t.Describe("Config", func() {
	// The system under test
	var sut *server.Config

	const (
		validPath = "/bin/sh"
		wrongPath = "/wrong"
	)

	BeforeEach(func() {
		sut = &server.Config{}
	})

	t.Describe("UpdateFromFile", func() {
		It("should succeed", func() {
			// Given
			sut, err := server.DefaultConfig(nil)
			Expect(err).To(BeNil())
			const filePath = "crio-test.conf"
			Expect(sut.ToFile(filePath)).To(BeNil())
			defer os.RemoveAll(filePath)

			// When
			err = sut.UpdateFromFile(filePath)

			// Then
			Expect(err).To(BeNil())
			expected, err := server.DefaultConfig(nil)
			Expect(err).To(BeNil())
			Expect(sut).To(Equal(expected))
		})

		It("should fail when file not readable", func() {
			// Given
			// When
			err := sut.UpdateFromFile("/proc/invalid")

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail when toml decode errors", func() {
			// Given
			// When
			err := sut.UpdateFromFile("config.go")

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("ToFile", func() {
		It("should succeed", func() {
			// Given
			const filePath = "crio-tofile.conf"
			sut, err := server.DefaultConfig(nil)
			Expect(err).To(BeNil())
			defer os.RemoveAll(filePath)

			// When
			err = sut.ToFile(filePath)

			// Then
			Expect(err).To(BeNil())
			testConfig := &server.Config{}
			Expect(testConfig.UpdateFromFile(filePath))
			Expect(testConfig).To(Equal(sut))
		})

		It("should fail when file not writeable", func() {
			// Given
			sut, err := server.DefaultConfig(nil)
			Expect(err).To(BeNil())

			// When
			err = sut.ToFile("/proc/invalid")

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("GetData", func() {
		It("should succeed with default config", func() {
			// Given
			sut, err := server.DefaultConfig(nil)
			Expect(err).To(BeNil())

			// When
			serverConfig := sut.GetData()

			// Then
			Expect(serverConfig).NotTo(BeNil())
		})

		It("should succeed with empty config", func() {
			// Given
			// When
			serverConfig := sut.GetData()

			// Then
			Expect(serverConfig).NotTo(BeNil())
		})
	})

	t.Describe("GetLibConfigIface", func() {
		It("should succeed with default config", func() {
			// Given
			sut, err := server.DefaultConfig(nil)
			Expect(err).To(BeNil())

			// When
			libConfig := sut.GetLibConfigIface()

			// Then
			Expect(libConfig).NotTo(BeNil())
		})

		It("should succeed with empty config", func() {
			// Given
			// When
			libConfig := sut.GetLibConfigIface()

			// Then
			Expect(libConfig).NotTo(BeNil())
		})
	})

	t.Describe("Validate", func() {
		// Setup the system under test
		BeforeEach(func() {
			var err error
			sut, err = server.DefaultConfig(nil)
			Expect(err).To(BeNil())
		})

		It("should succeed with default config", func() {
			// Given
			// When
			err := sut.Validate(nil, false)

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed with runtime cheks", func() {
			// Given
			sut.Runtimes["runc"] = oci.RuntimeHandler{RuntimePath: validPath}
			sut.Conmon = validPath
			tmpDir := path.Join(os.TempDir(), "cni-test")
			sut.NetworkConfig.PluginDir = []string{tmpDir}
			sut.NetworkDir = os.TempDir()
			sut.LogDir = "."
			defer os.RemoveAll(tmpDir)

			// When
			err := sut.Validate(nil, true)

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail with invalid network configuration", func() {
			// Given
			sut.Runtimes["runc"] = oci.RuntimeHandler{RuntimePath: validPath}
			sut.Conmon = validPath
			sut.PluginDir = []string{validPath}
			sut.NetworkConfig.NetworkDir = wrongPath

			// When
			err := sut.Validate(nil, true)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on unrecognized image volume type", func() {
			// Given
			sut.ImageVolumes = wrongPath

			// When
			err := sut.Validate(nil, false)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on wrong default ulimits", func() {
			// Given
			sut.DefaultUlimits = []string{wrongPath}

			// When
			err := sut.Validate(nil, false)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail wrong UID mappings", func() {
			// Given
			sut.UIDMappings = "value"
			sut.ManageNetworkNSLifecycle = true

			// When
			err := sut.Validate(nil, false)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail wrong GID mappings", func() {
			// Given
			sut.GIDMappings = "value"
			sut.ManageNetworkNSLifecycle = true

			// When
			err := sut.Validate(nil, false)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail wrong max log size", func() {
			// Given
			sut.LogSizeMax = 1

			// When
			err := sut.Validate(nil, false)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
