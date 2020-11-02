package config_test

import (
	"io/ioutil"
	"os"
	"path"
	"path/filepath"

	"github.com/containers/storage"
	"github.com/cri-o/cri-o/pkg/config"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("Config", func() {
	BeforeEach(beforeEach)

	runtimeValidConfig := func() *config.Config {
		sut.Runtimes["runc"] = &config.RuntimeHandler{
			RuntimePath: validFilePath, RuntimeType: config.DefaultRuntimeType,
		}
		sut.PinnsPath = validFilePath
		sut.NamespacesDir = os.TempDir()
		sut.Conmon = validConmonPath
		tmpDir := t.MustTempDir("cni-test")
		sut.NetworkConfig.PluginDirs = []string{tmpDir}
		sut.NetworkDir = os.TempDir()
		sut.LogDir = "/"
		sut.Listen = t.MustTempFile("crio.sock")
		sut.HooksDir = []string{}
		return sut
	}

	t.Describe("ValidateConfig", func() {
		It("should succeed with default config", func() {
			// Given
			// When
			err := sut.Validate(false)

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed with runtime checks", func() {
			// Given
			sut = runtimeValidConfig()

			// When
			err := sut.Validate(true)

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail with invalid log_dir", func() {
			// Given
			sut.RootConfig.LogDir = "/dev/null"

			// When
			err := sut.Validate(true)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with invalid runtime config", func() {
			// Given
			sut = runtimeValidConfig()
			sut.Listen = "/proc"

			// When
			err := sut.Validate(true)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with invalid api config", func() {
			// Given
			sut.AdditionalDevices = []string{invalidPath}

			// When
			err := sut.Validate(true)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with invalid network config", func() {
			// Given
			sut.Runtimes["runc"] = &config.RuntimeHandler{RuntimePath: validDirPath}
			sut.Conmon = validConmonPath
			sut.NetworkConfig.NetworkDir = invalidPath

			// When
			err := sut.Validate(true)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on unrecognized image volume type", func() {
			// Given
			sut.ImageVolumes = invalidPath

			// When
			err := sut.Validate(false)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on wrong default ulimits", func() {
			// Given
			sut.DefaultUlimits = []string{"invalid=-1:-1"}

			// When
			err := sut.Validate(false)

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
			sut = runtimeValidConfig()

			// When
			err := sut.RuntimeConfig.Validate(nil, true)

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed with additional devices", func() {
			// Given
			sut = runtimeValidConfig()
			sut.AdditionalDevices = []string{"/dev/null:/dev/null:rw"}

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
			sut.PinnsPath = validFilePath
			sut.NamespacesDir = os.TempDir()
			sut.Conmon = validConmonPath
			sut.HooksDir = []string{validDirPath, validDirPath, validDirPath}

			// When
			err := sut.RuntimeConfig.Validate(nil, true)

			// Then
			Expect(err).To(BeNil())
			Expect(sut.HooksDir).To(HaveLen(3))
		})

		It("should sort out invalid hooks directories", func() {
			// Given
			sut.Runtimes["runc"] = &config.RuntimeHandler{RuntimePath: validFilePath}
			sut.Conmon = validConmonPath
			sut.PinnsPath = validFilePath
			sut.NamespacesDir = os.TempDir()
			sut.HooksDir = []string{invalidPath, validDirPath, validDirPath}

			// When
			err := sut.RuntimeConfig.Validate(nil, true)

			// Then
			Expect(err).To(BeNil())
			Expect(sut.HooksDir).To(HaveLen(2))
		})

		It("should create non-existent hooks directory", func() {
			// Given
			sut.Runtimes["runc"] = &config.RuntimeHandler{RuntimePath: validFilePath}
			sut.Conmon = validConmonPath
			sut.PinnsPath = validFilePath
			sut.NamespacesDir = os.TempDir()
			sut.HooksDir = []string{filepath.Join(validDirPath, "new")}

			// When
			err := sut.RuntimeConfig.Validate(nil, true)

			// Then
			Expect(err).To(BeNil())
			Expect(sut.HooksDir).To(HaveLen(1))
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

		It("should fail wrong max log size", func() {
			// Given
			sut.LogSizeMax = 1

			// When
			err := sut.Validate(false)

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

		It("should fail on invalid default_sysctls", func() {
			// Given
			sut.DefaultSysctls = []string{"invalid"}

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on invalid conmon cgroup", func() {
			// Given
			sut.ConmonCgroup = "invalid"

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).NotTo(BeNil())
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
			err := sut.RuntimeConfig.ValidateConmonPath(validConmonPath)

			// Then
			Expect(err).To(BeNil())
			Expect(sut.RuntimeConfig.Conmon).To(Equal(validConmonPath))
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
			sut.RuntimeConfig.Conmon = validConmonPath

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
			sut = runtimeValidConfig()

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
			err := sut.Validate(true)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should get default storage options when options are empty", func() {
			// Given
			defaultStore, err := storage.GetStore(storage.StoreOptions{})
			Expect(err).To(BeNil())

			sut.RootConfig.RunRoot = ""
			sut.RootConfig.Root = ""
			sut.RootConfig.Storage = ""
			sut.RootConfig.StorageOptions = make([]string, 0)

			// When
			err = sut.Validate(true)

			// Then
			Expect(err).To(BeNil())
			Expect(sut.RootConfig.RunRoot).To(Equal(defaultStore.RunRoot()))
			Expect(sut.RootConfig.Root).To(Equal(defaultStore.GraphRoot()))
			Expect(sut.RootConfig.Storage).To(Equal(defaultStore.GraphDriverName()))
			Expect(sut.RootConfig.StorageOptions).To(Equal(defaultStore.GraphOptions()))
		})

		It("should override default storage options", func() {
			// Given
			defaultStore, err := storage.GetStore(storage.StoreOptions{})
			Expect(err).To(BeNil())

			sut.RootConfig.RunRoot = "/tmp"
			sut.RootConfig.Root = "/tmp"

			// When
			err = sut.Validate(true)

			// Then
			Expect(err).To(BeNil())
			Expect(sut.RootConfig.RunRoot).NotTo(Equal(defaultStore.RunRoot()))
			Expect(sut.RootConfig.Root).NotTo(Equal(defaultStore.GraphRoot()))
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
		It("should succeed with custom config", func() {
			// Given
			f := t.MustTempFile("config")
			Expect(ioutil.WriteFile(f,
				[]byte(`
					[crio]
					storage_driver = "overlay2"
					[crio.runtime]
					pids_limit = 2048`,
				), 0),
			).To(BeNil())

			// When
			err := sut.UpdateFromFile(f)

			// Then
			Expect(err).To(BeNil())
			Expect(sut.Storage).To(Equal("overlay2"))
			Expect(sut.Runtimes).To(HaveLen(1))
			Expect(sut.Runtimes).To(HaveKey("runc"))
			Expect(sut.PidsLimit).To(BeEquivalentTo(2048))
		})

		It("should succeed with custom runtime", func() {
			// Given
			f := t.MustTempFile("config")
			Expect(ioutil.WriteFile(f,
				[]byte("[crio.runtime.runtimes.crun]"), 0),
			).To(BeNil())

			// When
			err := sut.UpdateFromFile(f)

			// Then
			Expect(err).To(BeNil())
			Expect(sut.Runtimes).To(HaveLen(2))
			Expect(sut.Runtimes).To(HaveKey("crun"))
		})

		It("should succeed with additional runtime", func() {
			// Given
			f := t.MustTempFile("config")
			Expect(ioutil.WriteFile(f,
				[]byte(`
					[crio.runtime.runtimes.runc]
					[crio.runtime.runtimes.crun]
				`), 0),
			).To(BeNil())

			// When
			err := sut.UpdateFromFile(f)

			// Then
			Expect(err).To(BeNil())
			Expect(sut.Runtimes).To(HaveLen(2))
			Expect(sut.Runtimes).To(HaveKey("crun"))
			Expect(sut.Runtimes).To(HaveKey("runc"))
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

	t.Describe("UpdateFromPath", func() {
		It("should succeed with the correct priority", func() {
			// Given
			Expect(sut.LogLevel).To(Equal("info"))

			configDir := t.MustTempDir("config-dir")
			Expect(ioutil.WriteFile(
				filepath.Join(configDir, "00-default"),
				[]byte("[crio.runtime]\nlog_level = \"debug\"\n"),
				0o644,
			)).To(BeNil())
			Expect(ioutil.WriteFile(
				filepath.Join(configDir, "01-my-config"),
				[]byte("[crio.runtime]\nlog_level = \"warning\"\n"),
				0o644,
			)).To(BeNil())

			// When
			err := sut.UpdateFromPath(configDir)

			// Then
			Expect(err).To(BeNil())
			Expect(sut.LogLevel).To(Equal("warning"))
		})

		It("should fail with invalid config", func() {
			// Given
			configDir := t.MustTempDir("config-dir")
			Expect(ioutil.WriteFile(
				filepath.Join(configDir, "00-default"),
				[]byte("[crio.runtime]\nlog_level = true\n"),
				0o644,
			)).To(BeNil())

			// When
			err := sut.UpdateFromPath(configDir)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should succeed with not existing path", func() {
			// Given
			// When
			err := sut.UpdateFromPath("not-existing")

			// Then
			Expect(err).To(BeNil())
		})
	})
})
