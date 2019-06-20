package server_test

import (
	"io/ioutil"
	"os"
	"strings"

	"github.com/cri-o/cri-o/lib/config"
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

	var defaultConfig = func() *server.Config {
		config, err := server.DefaultConfig()
		Expect(err).To(BeNil())
		return config
	}

	BeforeEach(func() {
		sut = defaultConfig()
	})

	t.Describe("UpdateFromFile", func() {
		It("should succeed", func() {
			// Given
			filePath := t.MustTempFile("config")
			Expect(sut.ToFile(filePath)).To(BeNil())

			// When
			err := sut.UpdateFromFile(filePath)

			// Then
			Expect(err).To(BeNil())
			expected := defaultConfig()
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
			filePath := t.MustTempFile("config")

			// When
			err := sut.ToFile(filePath)

			// Then
			Expect(err).To(BeNil())
			testConfig := &server.Config{}
			Expect(testConfig.UpdateFromFile(filePath))
			Expect(testConfig).To(Equal(sut))
		})

		It("should fail when file not writeable", func() {
			// Given
			// When
			err := sut.ToFile("/proc/invalid")

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("GetData", func() {
		It("should succeed with default config", func() {
			// Given
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
		It("should succeed with default config", func() {
			// Given
			// When
			err := sut.Validate(nil, false)

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed with runtime cheks", func() {
			// Given
			sut.Runtimes["runc"] = config.RuntimeHandler{RuntimePath: validPath}
			sut.Conmon = validPath
			tmpDir := t.MustTempDir("cni-test")
			sut.NetworkConfig.PluginDirs = []string{tmpDir}
			sut.NetworkDir = os.TempDir()
			sut.LogDir = "."

			// When
			err := sut.Validate(nil, true)

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail with invalid network configuration", func() {
			// Given
			sut.Runtimes["runc"] = config.RuntimeHandler{RuntimePath: validPath}
			sut.Conmon = validPath
			sut.PluginDirs = []string{validPath}
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

	t.Describe("ToByte", func() {
		It("should succeed", func() {
			// Given
			// When
			res, err := sut.ToBytes()

			// Then
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
		})
	})

	t.Describe("Reload", func() {
		var modifyDefaultConfig = func(old, new string) string {
			filePath := t.MustTempFile("config")
			Expect(sut.ToFile(filePath)).To(BeNil())

			read, err := ioutil.ReadFile(filePath)
			Expect(err).To(BeNil())

			newContents := strings.ReplaceAll(string(read), old, new)
			err = ioutil.WriteFile(filePath, []byte(newContents), 0)
			Expect(err).To(BeNil())

			return filePath
		}

		It("should succeed without any config change", func() {
			// Given
			filePath := t.MustTempFile("config")
			Expect(sut.ToFile(filePath)).To(BeNil())

			// When
			err := sut.Reload(filePath)

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail with invalid config path", func() {
			// Given
			// When
			err := sut.Reload("")

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with invalid log_level", func() {
			// Given
			filePath := modifyDefaultConfig(
				`log_level = "error"`,
				`log_level = "invalid"`,
			)

			// When
			err := sut.Reload(filePath)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with invalid pause_image_auth_file", func() {
			// Given
			filePath := modifyDefaultConfig(
				`pause_image_auth_file = ""`,
				`pause_image_auth_file = "`+wrongPath+`"`,
			)

			// When
			err := sut.Reload(filePath)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("ReloadLogLevel", func() {
		It("should succeed without any config change", func() {
			// Given
			// When
			err := sut.ReloadLogLevel(sut)

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed with config change", func() {
			// Given
			const newLogLevel = "fatal"
			newConfig := defaultConfig()
			newConfig.LogLevel = newLogLevel

			// When
			err := sut.ReloadLogLevel(newConfig)

			// Then
			Expect(err).To(BeNil())
			Expect(sut.LogLevel).To(Equal(newLogLevel))
		})

		It("should fail with invalid log_level", func() {
			// Given
			newConfig := defaultConfig()
			newConfig.LogLevel = "invalid"

			// When
			err := sut.ReloadLogLevel(newConfig)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("ReloadPauseImage", func() {
		It("should succeed without any config change", func() {
			// Given
			// When
			err := sut.ReloadPauseImage(sut)

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed with pause_image change", func() {
			// Given
			const newPauseImage = "my-pause"
			newConfig := defaultConfig()
			newConfig.PauseImage = newPauseImage

			// When
			err := sut.ReloadPauseImage(newConfig)

			// Then
			Expect(err).To(BeNil())
			Expect(sut.PauseImage).To(Equal(newPauseImage))
		})

		It("should succeed with pause_command change", func() {
			// Given
			const newPauseCommand = "/new-pause"
			newConfig := defaultConfig()
			newConfig.PauseCommand = newPauseCommand

			// When
			err := sut.ReloadPauseImage(newConfig)

			// Then
			Expect(err).To(BeNil())
			Expect(sut.PauseCommand).To(Equal(newPauseCommand))
		})

		It("should succeed with pause_image_auth_file change", func() {
			// Given
			newConfig := defaultConfig()
			newConfig.PauseImageAuthFile = validPath

			// When
			err := sut.ReloadPauseImage(newConfig)

			// Then
			Expect(err).To(BeNil())
			Expect(sut.PauseImageAuthFile).To(Equal(validPath))
		})

		It("should fail with invalid pause_image_auth_file", func() {
			// Given
			newConfig := defaultConfig()
			newConfig.PauseImageAuthFile = wrongPath

			// When
			err := sut.ReloadPauseImage(newConfig)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
