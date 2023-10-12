package config_test

import (
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"github.com/containers/storage"
	crioann "github.com/cri-o/cri-o/pkg/annotations"
	"github.com/cri-o/cri-o/pkg/config"
	"github.com/cri-o/cri-o/utils/cmdrunner"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	alwaysPresentPath = "/tmp"
	invalid           = "invalid"
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
		sut.Conmon = validConmonPath()
		tmpDir := t.MustTempDir("cni-test")
		sut.NetworkConfig.PluginDirs = []string{tmpDir}
		sut.NetworkDir = os.TempDir()
		sut.LogDir = "/"
		sut.Listen = t.MustTempFile("crio.sock")
		sut.HooksDir = []string{}
		return sut
	}

	isRootless := func() bool {
		return os.Geteuid() != 0
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
			if isRootless() {
				Skip("this test does not work rootless")
			}

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
			sut.Conmon = validConmonPath()
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
			sut.Conmon = validConmonPath()
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
			sut.Conmon = validConmonPath()
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
			sut.Conmon = validConmonPath()
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
			sut.DefaultSysctls = []string{invalid}

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
	t.Describe("TranslateMonitorFields", func() {
		It("should fail on invalid conmon cgroup", func() {
			// Given
			handler := &config.RuntimeHandler{}
			sut.ConmonCgroup = "wrong"

			// When
			err := sut.RuntimeConfig.TranslateMonitorFieldsForHandler(handler, true)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on invalid conmon cgroup", func() {
			// Given
			handler := &config.RuntimeHandler{}
			sut.ConmonCgroup = invalid

			// When
			err := sut.RuntimeConfig.TranslateMonitorFieldsForHandler(handler, true)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on invalid InfraCtrCPUSet", func() {
			// Given
			sut.RuntimeConfig.InfraCtrCPUSet = "unparsable"

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should inherit from .Conmon even if bogus", func() {
			// Given
			sut.Conmon = invalidPath
			handler := &config.RuntimeHandler{}

			// When
			err := sut.RuntimeConfig.TranslateMonitorFieldsForHandler(handler, true)

			// Then
			Expect(err).NotTo(BeNil())
		})
		It("should inherit from .Conmon", func() {
			// Given
			sut.Conmon = validConmonPath()
			handler := &config.RuntimeHandler{}

			// When
			err := sut.RuntimeConfig.TranslateMonitorFieldsForHandler(handler, true)

			// Then
			Expect(err).To(BeNil())
			Expect(handler.MonitorPath).To(Equal(sut.Conmon))
		})
		It("should inherit from .ConmonEnv", func() {
			// Given
			sut.ConmonEnv = []string{"PATH=/usr/bin"}
			handler := &config.RuntimeHandler{}

			// When
			err := sut.RuntimeConfig.TranslateMonitorFieldsForHandler(handler, true)

			// Then
			Expect(err).To(BeNil())
			Expect(handler.MonitorEnv).To(Equal(sut.ConmonEnv))
		})
		It("should inherit from .ConmonCgroup", func() {
			// Given
			sut.ConmonCgroup = "system.slice"
			handler := &config.RuntimeHandler{}

			// When
			err := sut.RuntimeConfig.TranslateMonitorFieldsForHandler(handler, true)

			// Then
			Expect(err).To(BeNil())
			Expect(handler.MonitorCgroup).To(Equal(sut.ConmonCgroup))
		})

		It("should configure a taskset prefix for cmdrunner for a valid InfraCtrCPUSet", func() {
			executable, err := exec.LookPath("taskset")
			if err != nil {
				Skip("this test relies on 'taskset' being present")
			}

			// Given
			cmdrunner.ResetPrependedCmd()
			sut.RuntimeConfig.InfraCtrCPUSet = "0"

			// When
			err = sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).To(BeNil())
			Expect(cmdrunner.GetPrependedCmd()).To(Equal(executable))
		})

		It("should not configure a taskset prefix for cmdrunner for an empty InfraCtrCPUSet", func() {
			// Given
			cmdrunner.ResetPrependedCmd()
			sut.RuntimeConfig.InfraCtrCPUSet = ""

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).To(BeNil())
			Expect(cmdrunner.GetPrependedCmd()).To(Equal(""))
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

		It("should fail if default executable not in $PATH", func() {
			// Given
			sut.Runtimes[invalidPath] = &config.RuntimeHandler{RuntimePath: ""}
			sut.DefaultRuntime = invalidPath

			// When
			err := sut.RuntimeConfig.ValidateRuntimes()

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should not fail if non-default executable not in $PATH", func() {
			// Given
			sut.Runtimes[invalidPath] = &config.RuntimeHandler{RuntimePath: ""}
			sut.DefaultRuntime = "runc"

			// When
			err := sut.RuntimeConfig.ValidateRuntimes()

			// Then
			Expect(err).To(BeNil())
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

		It("should fail with wrong allowed_annotation", func() {
			// Given
			sut.Runtimes["runc"] = &config.RuntimeHandler{
				RuntimePath:        validFilePath,
				AllowedAnnotations: []string{"wrong"},
			}

			// When
			err := sut.RuntimeConfig.ValidateRuntimes()

			// Then
			Expect(err).NotTo(BeNil())
		})
		It("should have allowed and disallowed annotation", func() {
			// Given
			sut.Runtimes["runc"] = &config.RuntimeHandler{
				RuntimePath:        validFilePath,
				AllowedAnnotations: []string{crioann.DevicesAnnotation},
			}

			// When
			err := sut.RuntimeConfig.ValidateRuntimes()

			// Then
			Expect(err).To(BeNil())
			Expect(sut.Runtimes["runc"].AllowedAnnotations).To(ContainElement(crioann.DevicesAnnotation))
			Expect(sut.Runtimes["runc"].DisallowedAnnotations).NotTo(ContainElement(crioann.DevicesAnnotation))
		})
	})

	t.Describe("ValidateConmonPath", func() {
		It("should succeed with valid file in $PATH", func() {
			// Given
			sut.RuntimeConfig.Conmon = ""
			handler := &config.RuntimeHandler{MonitorPath: ""}

			// When
			err := sut.RuntimeConfig.ValidateConmonPath(validConmonPath(), handler)

			// Then
			Expect(err).To(BeNil())
			Expect(handler.MonitorPath).To(Equal(validConmonPath()))
		})

		It("should fail with invalid file in $PATH", func() {
			// Given
			handler := &config.RuntimeHandler{MonitorPath: ""}

			// When
			err := sut.RuntimeConfig.ValidateConmonPath(invalidPath, handler)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should succeed with valid file outside $PATH", func() {
			// Given
			handler := &config.RuntimeHandler{MonitorPath: validConmonPath()}

			// When
			err := sut.RuntimeConfig.ValidateConmonPath("", handler)

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail with invalid file outside $PATH", func() {
			// Given
			handler := &config.RuntimeHandler{MonitorPath: invalidPath}

			// When
			err := sut.RuntimeConfig.ValidateConmonPath("", handler)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("ValidateImageConfig", func() {
		It("should succeed with default config", func() {
			// Given
			// When
			err := sut.ImageConfig.Validate(false)

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed on execution and writing permissions", func() {
			// Given
			sut.ImageConfig.SignaturePolicyDir = os.TempDir()

			// When
			err := sut.ImageConfig.Validate(true)

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail when SignaturePolicyDir is not absolute", func() {
			// Given
			sut.ImageConfig.SignaturePolicyDir = "./wrong/path"

			// When
			err := sut.ImageConfig.Validate(false)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail when PauseImage is invalid", func() {
			// Given
			sut.ImageConfig.PauseImage = "//NOT:a valid image reference!"

			// When
			err := sut.ImageConfig.Validate(false)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("ImageConfig.ParsePauseImage", func() {
		It("should succeed with the default value", func() {
			// Given
			sut.ImageConfig.PauseImage = config.DefaultPauseImage

			// When
			ref, err := sut.ImageConfig.ParsePauseImage()

			// Then
			Expect(err).To(BeNil())
			// DefaultPauseImage is using a canonical form where this comparison is expected to work.
			Expect(ref.String()).To(Equal(config.DefaultPauseImage))
		})

		It("should succeed with a name-only value", func() {
			// Given
			sut.ImageConfig.PauseImage = "registry.k8s.io/pause"

			// When
			ref, err := sut.ImageConfig.ParsePauseImage()

			// Then
			Expect(err).To(BeNil())
			Expect(ref.String()).To(Equal("registry.k8s.io/pause:latest"))
		})

		It("should succeed with a short name", func() {
			// NOTE: This behavior is undocumented. Users are expected to provide a
			// name with a registry

			// Given
			sut.ImageConfig.PauseImage = "short:notlatest"

			// When
			ref, err := sut.ImageConfig.ParsePauseImage()

			// Then
			Expect(err).To(BeNil())
			Expect(ref.String()).To(Equal("docker.io/library/short:notlatest"))
		})

		It("should fail with an invalid value", func() {
			// Given
			sut.ImageConfig.PauseImage = "//THIS is:very!invalid="

			// When
			_, err := sut.ImageConfig.ParsePauseImage()

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
			if isRootless() {
				Skip("this test does not work rootless")
			}

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
			if isRootless() {
				Skip("this test does not work rootless")
			}

			// Given
			defaultStore, err := storage.GetStore(storage.StoreOptions{})
			Expect(err).To(BeNil())

			sut.RootConfig.RunRoot = ""
			sut.RootConfig.Root = ""
			sut.RootConfig.Storage = ""
			sut.RootConfig.StorageOptions = make([]string, 0)
			// this must be set in case pinns isn't downloaded to the $PATH
			sut.RuntimeConfig.PinnsPath = alwaysPresentPath

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
			if isRootless() {
				Skip("this test does not work rootless")
			}

			// Given
			defaultStore, err := storage.GetStore(storage.StoreOptions{})
			Expect(err).To(BeNil())

			sut.RootConfig.RunRoot = alwaysPresentPath
			sut.RootConfig.Root = alwaysPresentPath
			// this must be set in case pinns isn't downloaded to the $PATH
			sut.RuntimeConfig.PinnsPath = alwaysPresentPath

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
			tmpfile := t.MustTempFile("config")

			// When
			err := sut.ToFile(tmpfile)

			// Then
			Expect(err).To(BeNil())
			_, err = os.Stat(tmpfile)
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
			Expect(os.WriteFile(f,
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

		It("should inherit storage_options from storage.conf and remove duplicates", func() {
			f := t.MustTempFile("config")
			// Given
			Expect(os.WriteFile(f,
				[]byte(`
					[crio]
					storage_option = [
						"foo=bar",
					]`,
				), 0),
			).To(BeNil())
			for _, tc := range []struct {
				opts   []string
				expect []string
			}{
				{[]string{"option1=v1", "option2=v2", "option3=v3"}, []string{"option1=v1", "option2=v2", "option3=v3", "foo=bar"}},
				{[]string{"option1=v1", "option3=v3", "option2=v2", "option3=v3"}, []string{"option1=v1", "option2=v2", "option3=v3", "foo=bar"}},
				{[]string{"option1=v1", "option2=v2", "option3=v3", "option1=v1"}, []string{"option2=v2", "option3=v3", "option1=v1", "foo=bar"}},
				{[]string{"option1=v1", "option2=v2", "option3=v3", "option4=v4", "option3=v3", "option1=v1"}, []string{"option2=v2", "option4=v4", "option3=v3", "option1=v1", "foo=bar"}},
			} {
				// When
				defaultcfg := defaultConfig()
				defaultcfg.StorageOptions = tc.opts
				err := defaultcfg.UpdateFromFile(f)

				// Then
				Expect(err).To(BeNil())
				Expect(defaultcfg.RootConfig.StorageOptions).To(Equal(tc.expect))
			}
		})

		It("should inherit graphroot from storage.conf if crio root is empty", func() {
			f := t.MustTempFile("config")
			for _, tc := range []struct {
				criocfg   []byte
				graphRoot string
				expect    string
			}{
				{[]byte(`
				[crio]
				root = ""
				`,
				), "/test/storage", "/test/storage"},
				{[]byte(`
				[crio]
				root = "/test/crio/storage"
				`,
				), "/test/storage", "/test/crio/storage"},
			} {
				Expect(os.WriteFile(f, tc.criocfg, 0)).To(BeNil())
				// When
				defaultcfg := defaultConfig()
				defaultcfg.Root = tc.graphRoot
				err := defaultcfg.UpdateFromFile(f)

				// Then
				Expect(err).To(BeNil())
				Expect(defaultcfg.Root).To(Equal(tc.expect))
			}
		})

		It("should inherit runroot from storage.conf if crio runroot is empty", func() {
			f := t.MustTempFile("config")
			for _, tc := range []struct {
				criocfg []byte
				runRoot string
				expect  string
			}{
				{[]byte(`
				[crio]
				runroot = ""
				`,
				), "/test/storage", "/test/storage"},
				{[]byte(`
				[crio]
				runroot = "/test/crio/storage"
				`,
				), "/test/storage", "/test/crio/storage"},
			} {
				Expect(os.WriteFile(f, tc.criocfg, 0)).To(BeNil())
				// When
				defaultcfg := defaultConfig()
				defaultcfg.RunRoot = tc.runRoot
				err := defaultcfg.UpdateFromFile(f)

				// Then
				Expect(err).To(BeNil())
				Expect(defaultcfg.RunRoot).To(Equal(tc.expect))
			}
		})

		It("should inherit runroot from storage.conf if crio runroot is empty", func() {
			f := t.MustTempFile("config")
			for _, tc := range []struct {
				criocfg       []byte
				storageDriver string
				expect        string
			}{
				{[]byte(`
				[crio]
				storage_driver = ""
				`,
				), "/test/storage", "/test/storage"},
				{[]byte(`
				[crio]
				storage_driver = "/test/crio/storage"
				`,
				), "/test/storage", "/test/crio/storage"},
			} {
				Expect(os.WriteFile(f, tc.criocfg, 0)).To(BeNil())
				// When
				defaultcfg := defaultConfig()
				defaultcfg.Storage = tc.storageDriver
				err := defaultcfg.UpdateFromFile(f)

				// Then
				Expect(err).To(BeNil())
				Expect(defaultcfg.Storage).To(Equal(tc.expect))
			}
		})

		It("should succeed with custom runtime", func() {
			// Given
			f := t.MustTempFile("config")
			Expect(os.WriteFile(f,
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
			Expect(os.WriteFile(f,
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
			Expect(os.WriteFile(
				filepath.Join(configDir, "00-default"),
				[]byte("[crio.runtime]\nlog_level = \"debug\"\n"),
				0o644,
			)).To(BeNil())
			Expect(os.WriteFile(
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
			Expect(os.WriteFile(
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

	t.Describe("ValidateRuntimeVMBinaryPattern", func() {
		It("should succeed when using RuntimeTypeVM and runtime_path follows the containerd pattern", func() {
			// Given
			sut.Runtimes["kata"] = &config.RuntimeHandler{
				RuntimePath: "containerd-shim-kata-qemu-v2", RuntimeType: config.RuntimeTypeVM,
			}

			// When
			ok := sut.Runtimes["kata"].ValidateRuntimeVMBinaryPattern()

			// Then
			Expect(ok).To(BeTrue())
		})

		It("should fail when using RuntimeTypeVM and runtime_path does not follow the containerd pattern", func() {
			// Given
			sut.Runtimes["kata"] = &config.RuntimeHandler{
				RuntimePath: "kata-runtime", RuntimeType: config.RuntimeTypeVM,
			}

			// When
			ok := sut.Runtimes["kata"].ValidateRuntimeVMBinaryPattern()

			// Then
			Expect(ok).To(BeFalse())
		})
	})

	t.Describe("ValidateRuntimeConfigPath", func() {
		It("should fail with OCI runtime type when runtime_config_path is used", func() {
			// Given
			sut.Runtimes["runc"] = &config.RuntimeHandler{
				RuntimeConfigPath: validFilePath, RuntimeType: config.DefaultRuntimeType,
			}

			// When
			err := sut.Runtimes["runc"].ValidateRuntimeConfigPath("runc")

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with VM runtime type and runtime_config_path points to an invalid path", func() {
			// Given
			sut.Runtimes["kata"] = &config.RuntimeHandler{
				RuntimeConfigPath: invalidPath, RuntimeType: config.RuntimeTypeVM,
			}

			// When
			err := sut.Runtimes["kata"].ValidateRuntimeConfigPath("kata")

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should succeed with VM runtime type and runtime_config_path points to a valid path", func() {
			// Given
			sut.Runtimes["kata"] = &config.RuntimeHandler{
				RuntimeConfigPath: validFilePath, RuntimeType: config.RuntimeTypeVM,
			}

			// When
			err := sut.Runtimes["kata"].ValidateRuntimeConfigPath("kata")

			// Then
			Expect(err).To(BeNil())
		})
	})
})
