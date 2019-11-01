package config_test

import (
	"io/ioutil"
	"os"
	"path"

	"github.com/cri-o/cri-o/internal/lib/config"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("Config", func() {
	BeforeEach(beforeEach)

	var runtimeValidConfig = func() *config.Config {
		sut.Runtimes["runc"] = &config.RuntimeHandler{
			RuntimePath: validFilePath, RuntimeType: config.DefaultRuntimeType,
		}
		sut.Conmon = validFilePath
		tmpDir := t.MustTempDir("cni-test")
		sut.NetworkConfig.PluginDirs = []string{tmpDir}
		sut.NetworkDir = os.TempDir()
		sut.LogDir = "/"
		sut.Listen = t.MustTempFile("crio.sock")
		return sut
	}

	t.Describe("ValidateConfig", func() {
		It("should succeed with default config", func() {
			// Given
			// When
			err := sut.Validate(nil, false)

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed with runtime checks", func() {
			// Given
			sut = runtimeValidConfig()

			// When
			err := sut.Validate(nil, true)

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail with invalid log_dir", func() {
			// Given
			sut.RootConfig.LogDir = "/dev/null"

			// When
			err := sut.Validate(nil, true)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with invalid runtime config", func() {
			// Given
			sut = runtimeValidConfig()
			sut.Listen = "/proc"

			// When
			err := sut.Validate(nil, true)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with invalid api config", func() {
			// Given
			sut.AdditionalDevices = []string{invalidPath}

			// When
			err := sut.Validate(nil, true)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with invalid network config", func() {
			// Given
			sut.Runtimes["runc"] = &config.RuntimeHandler{RuntimePath: validDirPath}
			sut.Conmon = validFilePath
			sut.NetworkConfig.NetworkDir = invalidPath

			// When
			err := sut.Validate(nil, true)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on unrecognized image volume type", func() {
			// Given
			sut.ImageVolumes = invalidPath

			// When
			err := sut.Validate(nil, false)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on wrong default ulimits", func() {
			// Given
			sut.DefaultUlimits = []string{"invalid=-1:-1"}

			// When
			err := sut.Validate(nil, false)

			// Then
			Expect(err).NotTo(BeNil())
		})

	})

	t.Describe("ValidateAPIConfig", func() {
		It("should succeed with negative GRPCMaxSendMsgSize", func() {
			// Given
			sut.GRPCMaxSendMsgSize = -100

			// When
			err := sut.APIConfig.Validate(false)

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed with negative GRPCMaxRecvMsgSize", func() {
			// Given
			sut.GRPCMaxRecvMsgSize = -100

			// When
			err := sut.APIConfig.Validate(false)

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail on invalid Listen directory", func() {
			// Given
			sut = runtimeValidConfig()
			sut.Listen = "/proc/dir/crio.sock"

			// When
			err := sut.APIConfig.Validate(true)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail if socket removal fails", func() {
			// Given
			sut = runtimeValidConfig()
			sut.Listen = "/proc"

			// When
			err := sut.APIConfig.Validate(true)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with invalid host IP", func() {
			// Given
			sut = runtimeValidConfig()
			sut.HostIP = []string{"1.2.3.4", "invalid"}

			// When
			err := sut.APIConfig.Validate(true)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with more than two host IPs", func() {
			// Given
			sut = runtimeValidConfig()
			sut.HostIP = []string{"1.2.3.4", "10.1.2.3", "3300::1"}

			// When
			err := sut.APIConfig.Validate(true)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("ValidateRuntimeConfig", func() {
		It("should succeed with default config", func() {
			// Given
			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed during runtime", func() {
			// Given
			sut.Runtimes["runc"] = &config.RuntimeHandler{
				RuntimePath: validFilePath,
				RuntimeType: config.DefaultRuntimeType,
			}
			sut.Conmon = validFilePath

			// When
			err := sut.RuntimeConfig.Validate(nil, true)

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed with additional devices", func() {
			// Given
			sut.AdditionalDevices = []string{"/dev/null:/dev/null:rw"}
			sut.Runtimes["runc"] = &config.RuntimeHandler{
				RuntimePath: validFilePath,
				RuntimeType: config.DefaultRuntimeType,
			}
			sut.Conmon = validFilePath

			// When
			err := sut.RuntimeConfig.Validate(nil, true)

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed with hooks directories", func() {
			// Given
			sut.Runtimes["runc"] = &config.RuntimeHandler{
				RuntimePath: validFilePath,
				RuntimeType: config.DefaultRuntimeType,
			}
			sut.Conmon = validFilePath
			sut.HooksDir = []string{validDirPath}

			// When
			err := sut.RuntimeConfig.Validate(nil, true)

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail on invalid hooks directory", func() {
			// Given
			sut.Runtimes["runc"] = &config.RuntimeHandler{RuntimePath: validFilePath}
			sut.Conmon = validFilePath
			sut.HooksDir = []string{invalidPath}

			// When
			err := sut.RuntimeConfig.Validate(nil, true)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail if the hooks directory is not a directory", func() {
			// Given
			sut.Runtimes["runc"] = &config.RuntimeHandler{RuntimePath: validFilePath}
			sut.Conmon = validFilePath
			sut.HooksDir = []string{validFilePath}

			// When
			err := sut.RuntimeConfig.Validate(nil, true)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on invalid conmon path", func() {
			// Given
			sut.Runtimes["runc"] = &config.RuntimeHandler{RuntimePath: validFilePath}
			sut.Conmon = invalidPath
			sut.HooksDir = []string{validDirPath}

			// When
			err := sut.RuntimeConfig.Validate(nil, true)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on wrong DefaultUlimits", func() {
			// Given
			sut.DefaultUlimits = []string{invalidPath}

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on wrong invalid device specification", func() {
			// Given
			sut.AdditionalDevices = []string{"::::"}

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on invalid device", func() {
			// Given
			sut.AdditionalDevices = []string{invalidPath}

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on invalid device mode", func() {
			// Given
			sut.AdditionalDevices = []string{"/dev/null:/dev/null:abc"}

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on invalid first device", func() {
			// Given
			sut.AdditionalDevices = []string{"wrong:/dev/null:rw"}

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on invalid second device", func() {
			// Given
			sut.AdditionalDevices = []string{"/dev/null:wrong:rw"}

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on no default runtime", func() {
			// Given
			sut.Runtimes = config.Runtimes{}

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on non existing runtime binary", func() {
			// Given
			sut.Runtimes["runc"] = &config.RuntimeHandler{RuntimePath: "not-existing"}

			// When
			err := sut.RuntimeConfig.Validate(nil, true)

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

		It("should fail on invalid conmon cgroup", func() {
			// Given
			sut.ConmonCgroup = "wrong"

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should succeed without defaultRuntime set", func() {
			// Given
			sut.DefaultRuntime = ""

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).To(BeNil())
			Expect(sut.DefaultRuntime).To(Equal("runc"))
		})

		It("should succeed without Runtimes and DefaultRuntime set", func() {
			// Given
			sut.DefaultRuntime = ""
			sut.Runtimes = config.Runtimes{}

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).To(BeNil())
			Expect(sut.DefaultRuntime).To(Equal("runc"))
		})
	})

	t.Describe("ValidateRuntimes", func() {
		It("should succeed with default config", func() {
			// Given
			// When
			err := sut.RuntimeConfig.ValidateRuntimes()

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed with empty runtime_type", func() {
			// Given
			sut.Runtimes["runc"] = &config.RuntimeHandler{
				RuntimePath: validFilePath,
			}

			// When
			err := sut.RuntimeConfig.ValidateRuntimes()

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail if executable not in $PATH", func() {
			// Given
			sut.Runtimes[invalidPath] = &config.RuntimeHandler{RuntimePath: ""}

			// When
			err := sut.RuntimeConfig.ValidateRuntimes()

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with wrong but set runtime_path", func() {
			// Given
			sut.Runtimes["runc"] = &config.RuntimeHandler{RuntimePath: invalidPath}

			// When
			err := sut.RuntimeConfig.ValidateRuntimes()

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with wrong runtime_type", func() {
			// Given
			sut.Runtimes["runc"] = &config.RuntimeHandler{
				RuntimePath: validFilePath,
				RuntimeType: "wrong",
			}

			// When
			err := sut.RuntimeConfig.ValidateRuntimes()

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("ValidateConmonPath", func() {
		It("should succeed with valid file in $PATH", func() {
			// Given
			sut.RuntimeConfig.Conmon = ""

			// When
			err := sut.RuntimeConfig.ValidateConmonPath(validFilePath)

			// Then
			Expect(err).To(BeNil())
			Expect(sut.RuntimeConfig.Conmon).To(Equal(validFilePath))
		})

		It("should fail with invalid file in $PATH", func() {
			// Given
			sut.RuntimeConfig.Conmon = ""

			// When
			err := sut.RuntimeConfig.ValidateConmonPath(invalidPath)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should succeed with valid file outside $PATH", func() {
			// Given
			sut.RuntimeConfig.Conmon = validDirPath

			// When
			err := sut.RuntimeConfig.ValidateConmonPath("")

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail with invalid file outside $PATH", func() {
			// Given
			sut.RuntimeConfig.Conmon = invalidPath

			// When
			err := sut.RuntimeConfig.ValidateConmonPath("")

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("ValidateNetworkConfig", func() {
		It("should succeed with default config", func() {
			// Given
			// When
			err := sut.NetworkConfig.Validate(false)

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed during runtime", func() {
			// Given
			sut.NetworkConfig.NetworkDir = validDirPath
			tmpDir := path.Join(os.TempDir(), "cni-test")
			sut.NetworkConfig.PluginDirs = []string{tmpDir}
			defer os.RemoveAll(tmpDir)

			// When
			err := sut.NetworkConfig.Validate(true)

			// Then
			Expect(err).To(BeNil())
		})

		It("should create the  NetworkDir", func() {
			// Given
			tmpDir := path.Join(os.TempDir(), invalidPath)
			sut.NetworkConfig.NetworkDir = tmpDir
			sut.NetworkConfig.PluginDirs = []string{validDirPath}

			// When
			err := sut.NetworkConfig.Validate(true)

			// Then
			Expect(err).To(BeNil())
			os.RemoveAll(tmpDir)
		})

		It("should fail on invalid NetworkDir", func() {
			// Given
			tmpfile := path.Join(os.TempDir(), "wrong-file")
			file, err := os.Create(tmpfile)
			Expect(err).To(BeNil())
			file.Close()
			defer os.Remove(tmpfile)
			sut.NetworkConfig.NetworkDir = tmpfile
			sut.NetworkConfig.PluginDirs = []string{}

			// When
			err = sut.NetworkConfig.Validate(true)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on invalid PluginDirs", func() {
			// Given
			sut.NetworkConfig.NetworkDir = validDirPath
			sut.NetworkConfig.PluginDirs = []string{invalidPath}

			// When
			err := sut.NetworkConfig.Validate(true)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should succeed on having PluginDir", func() {
			// Given
			sut.NetworkConfig.NetworkDir = validDirPath
			sut.NetworkConfig.PluginDir = validDirPath
			sut.NetworkConfig.PluginDirs = []string{}

			// When
			err := sut.NetworkConfig.Validate(true)

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed in appending PluginDir to PluginDirs", func() {
			// Given
			sut.NetworkConfig.NetworkDir = validDirPath
			sut.NetworkConfig.PluginDir = validDirPath
			sut.NetworkConfig.PluginDirs = []string{}

			// When
			err := sut.NetworkConfig.Validate(true)

			// Then
			Expect(err).To(BeNil())
			Expect(sut.NetworkConfig.PluginDirs[0]).To(Equal(validDirPath))
		})

		It("should fail in validating invalid PluginDir", func() {
			// Given
			sut.NetworkConfig.NetworkDir = validDirPath
			sut.NetworkConfig.PluginDir = invalidPath
			sut.NetworkConfig.PluginDirs = []string{}

			// When
			err := sut.NetworkConfig.Validate(true)

			// Then
			Expect(err).ToNot(BeNil())
		})
	})

	t.Describe("ValidateRootConfig", func() {
		It("should succeed with default config", func() {
			// Given
			// When
			err := sut.RootConfig.Validate(false)

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed during runtime", func() {
			// Given
			sut = runtimeValidConfig()

			// When
			err := sut.RootConfig.Validate(true)

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail on invalid LogDir", func() {
			// Given
			sut.RootConfig.LogDir = "/dev/null"

			// When
			err := sut.RootConfig.Validate(true)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with non absolute log_dir", func() {
			// Given
			sut.RootConfig.LogDir = "test"

			// When
			err := sut.Validate(nil, true)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("ToFile", func() {
		It("should succeed with default config", func() {
			// Given
			tmpfile, err := ioutil.TempFile("", "config")
			Expect(err).To(BeNil())
			defer os.Remove(tmpfile.Name())

			// When
			err = sut.ToFile(tmpfile.Name())

			// Then
			Expect(err).To(BeNil())
			_, err = os.Stat(tmpfile.Name())
			Expect(err).To(BeNil())
		})

		It("should fail with invalid path", func() {
			// Given
			// When
			err := sut.ToFile(invalidPath)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("UpdateFromFile", func() {
		It("should succeed with default config", func() {
			// Given
			// When
			err := sut.UpdateFromFile("testdata/config.toml")

			// Then
			Expect(err).To(BeNil())
			Expect(sut.Storage).To(Equal("overlay2"))
			Expect(sut.PidsLimit).To(BeEquivalentTo(2048))
		})

		It("should fail when file does not exist", func() {
			// Given
			// When
			err := sut.UpdateFromFile("/invalid/file")

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail when toml decode fails", func() {
			// Given
			// When
			err := sut.UpdateFromFile("config.go")

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("GetData", func() {
		It("should succeed with default config", func() {
			// Given
			// When
			config := sut.GetData()

			// Then
			Expect(config).NotTo(BeNil())
			Expect(config).To(Equal(sut))
		})

		It("should succeed with empty config", func() {
			// Given
			sut := &config.Config{}

			// When
			config := sut.GetData()

			// Then
			Expect(config).NotTo(BeNil())
			Expect(config).To(Equal(sut))
		})

		It("should succeed with nil config", func() {
			// Given
			var sut *config.Config

			// When
			config := sut.GetData()

			// Then
			Expect(config).To(BeNil())
		})
	})

	t.Describe("ToBytes", func() {
		It("should succeed", func() {
			// Given
			// When
			res, err := sut.ToBytes()

			// Then
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
		})
	})
})
