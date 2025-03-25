package config_test

import (
	"context"
	"os"
	"os/exec"
	"path"
	"path/filepath"

	"github.com/containers/storage"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/internal/config/cgmgr"
	crioann "github.com/cri-o/cri-o/pkg/annotations"
	"github.com/cri-o/cri-o/pkg/config"
	"github.com/cri-o/cri-o/utils/cmdrunner"
)

const (
	alwaysPresentPath = "/tmp"
	invalid           = "invalid"
)

// The actual test suite.
var _ = t.Describe("Config", func() {
	BeforeEach(beforeEach)

	runtimeValidConfig := func() *config.Config {
		sut.Runtimes[config.DefaultRuntime] = &config.RuntimeHandler{
			RuntimePath: validFilePath, RuntimeType: config.DefaultRuntimeType, ContainerMinMemory: "12MiB",
		}
		sut.PinnsPath = validFilePath
		sut.NamespacesDir = os.TempDir()
		sut.Conmon = validConmonPath()
		tmpDir := t.MustTempDir("cni-test")
		sut.PluginDirs = []string{tmpDir}
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
			Expect(err).ToNot(HaveOccurred())
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
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail with invalid log_dir", func() {
			// Given
			sut.LogDir = "/dev/null"

			// When
			err := sut.Validate(true)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail with invalid runtime config", func() {
			// Given
			sut = runtimeValidConfig()
			sut.Listen = "/proc"

			// When
			err := sut.Validate(true)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail with invalid api config", func() {
			// Given
			sut.AdditionalDevices = []string{invalidPath}

			// When
			err := sut.Validate(true)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail with invalid network config", func() {
			// Given
			sut.Runtimes[config.DefaultRuntime] = &config.RuntimeHandler{RuntimePath: validDirPath}
			sut.Conmon = validConmonPath()
			sut.NetworkDir = invalidPath

			// When
			err := sut.Validate(true)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail on unrecognized image volume type", func() {
			// Given
			sut.ImageVolumes = invalidPath

			// When
			err := sut.Validate(false)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail on wrong default ulimits", func() {
			// Given
			sut.DefaultUlimits = []string{"invalid=-1:-1"}

			// When
			err := sut.Validate(false)

			// Then
			Expect(err).To(HaveOccurred())
		})
	})

	t.Describe("ValidateAPIConfig", func() {
		It("should succeed with negative GRPCMaxSendMsgSize", func() {
			// Given
			sut.GRPCMaxSendMsgSize = -100

			// When
			err := sut.APIConfig.Validate(false)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should succeed with negative GRPCMaxRecvMsgSize", func() {
			// Given
			sut.GRPCMaxRecvMsgSize = -100

			// When
			err := sut.APIConfig.Validate(false)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail on invalid Listen directory", func() {
			// Given
			sut = runtimeValidConfig()
			sut.Listen = "/proc/dir/crio.sock"

			// When
			err := sut.APIConfig.Validate(true)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail if socket removal fails", func() {
			// Given
			sut = runtimeValidConfig()
			sut.Listen = "/proc"

			// When
			err := sut.APIConfig.Validate(true)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should succeed if stream server TLS enabled", func() {
			// Given
			sut = runtimeValidConfig()
			sut.StreamEnableTLS = true
			sut.StreamTLSCert = "cert"
			sut.StreamTLSKey = "key"
			sut.StreamTLSCA = "ca"

			// When
			err := sut.APIConfig.Validate(false)

			// Then
			Expect(err).NotTo(HaveOccurred())
		})

		It("should fail if stream server TLS enabled and cert is empty", func() {
			// Given
			sut = runtimeValidConfig()
			sut.StreamEnableTLS = true
			sut.StreamTLSCert = ""
			sut.StreamTLSKey = "key"
			sut.StreamTLSCA = "ca"

			// When
			err := sut.APIConfig.Validate(false)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail if stream server TLS enabled and key is empty", func() {
			// Given
			sut = runtimeValidConfig()
			sut.StreamEnableTLS = true
			sut.StreamTLSCert = "cert"
			sut.StreamTLSKey = ""
			sut.StreamTLSCA = "ca"

			// When
			err := sut.APIConfig.Validate(false)

			// Then
			Expect(err).To(HaveOccurred())
		})
	})

	t.Describe("ValidateRuntimeConfig", func() {
		It("should succeed with default config", func() {
			// Given
			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should succeed during runtime", func() {
			// Given
			sut = runtimeValidConfig()

			// When
			err := sut.RuntimeConfig.Validate(nil, true)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should succeed with additional devices", func() {
			// Given
			sut = runtimeValidConfig()
			sut.AdditionalDevices = []string{"/dev/null:/dev/null:rw"}

			// When
			err := sut.RuntimeConfig.Validate(nil, true)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should succeed with hooks directories", func() {
			// Given
			sut.Runtimes[config.DefaultRuntime] = &config.RuntimeHandler{
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
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.HooksDir).To(HaveLen(3))
		})

		It("should sort out invalid hooks directories", func() {
			// Given
			sut.Runtimes[config.DefaultRuntime] = &config.RuntimeHandler{RuntimePath: validFilePath}
			sut.Conmon = validConmonPath()
			sut.PinnsPath = validFilePath
			sut.NamespacesDir = os.TempDir()
			sut.HooksDir = []string{invalidPath, validDirPath, validDirPath}

			// When
			err := sut.RuntimeConfig.Validate(nil, true)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.HooksDir).To(HaveLen(2))
		})

		It("should create non-existent hooks directory", func() {
			// Given
			sut.Runtimes[config.DefaultRuntime] = &config.RuntimeHandler{RuntimePath: validFilePath}
			sut.Conmon = validConmonPath()
			sut.PinnsPath = validFilePath
			sut.NamespacesDir = os.TempDir()
			sut.HooksDir = []string{filepath.Join(validDirPath, "new")}

			// When
			err := sut.RuntimeConfig.Validate(nil, true)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.HooksDir).To(HaveLen(1))
		})

		It("should fail on invalid conmon path", func() {
			// Given
			sut.Runtimes[config.DefaultRuntime] = &config.RuntimeHandler{RuntimePath: validFilePath}
			sut.Conmon = invalidPath
			sut.HooksDir = []string{validDirPath}

			// When
			err := sut.RuntimeConfig.Validate(nil, true)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail on wrong DefaultUlimits", func() {
			// Given
			sut.DefaultUlimits = []string{invalidPath}

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should inherit default value if invalid runtime container minimum memory limit is set", func() {
			// Given
			sut.Runtimes[config.DefaultRuntime].ContainerMinMemory = "123invalid"

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail on wrong invalid device specification", func() {
			// Given
			sut.AdditionalDevices = []string{"::::"}

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail on invalid device", func() {
			// Given
			sut.AdditionalDevices = []string{invalidPath}

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail on invalid device mode", func() {
			// Given
			sut.AdditionalDevices = []string{"/dev/null:/dev/null:abc"}

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail on invalid first device", func() {
			// Given
			sut.AdditionalDevices = []string{"wrong:/dev/null:rw"}

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail on invalid second device", func() {
			// Given
			sut.AdditionalDevices = []string{"/dev/null:wrong:rw"}

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail on no default runtime", func() {
			// Given
			sut.Runtimes = config.Runtimes{}

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail on non existing runtime binary", func() {
			// Given
			sut.Runtimes[config.DefaultRuntime] = &config.RuntimeHandler{RuntimePath: "not-existing"}

			// When
			err := sut.RuntimeConfig.Validate(nil, true)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail wrong max log size", func() {
			// Given
			sut.LogSizeMax = 1

			// When
			err := sut.Validate(false)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should succeed without defaultRuntime set", func() {
			// Given
			sut.DefaultRuntime = ""

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.DefaultRuntime).To(Equal(config.DefaultRuntime))
		})

		It("should succeed without Runtimes and DefaultRuntime set", func() {
			// Given
			sut.DefaultRuntime = ""
			sut.Runtimes = config.Runtimes{}

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.DefaultRuntime).To(Equal(config.DefaultRuntime))
		})

		It("should fail on invalid default_sysctls", func() {
			// Given
			sut.DefaultSysctls = []string{invalid}

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).To(HaveOccurred())
		})
		It("should pass for valid Timezone", func() {
			// Set a valid Timezone
			sut.Timezone = "America/New_York"

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail for invalid Timezone", func() {
			// Set an invalid Timezone
			sut.Timezone = "InvalidTimezone"

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).To(HaveOccurred())
			Expect(err.Error()).To(ContainSubstring("invalid timezone: InvalidTimezone"))
		})

		It("should pass for 'Local' Timezone", func() {
			// Set Timezone to 'Local'
			sut.Timezone = "Local"

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})
	})
	t.Describe("TranslateMonitorFields", func() {
		It("should fail on invalid conmon cgroup", func() {
			// Given
			handler := &config.RuntimeHandler{}
			sut.ConmonCgroup = "wrong"

			// When
			err := sut.TranslateMonitorFieldsForHandler(handler, true)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail on invalid conmon cgroup", func() {
			// Given
			handler := &config.RuntimeHandler{}
			sut.ConmonCgroup = invalid

			// When
			err := sut.TranslateMonitorFieldsForHandler(handler, true)

			// Then
			Expect(err).To(HaveOccurred())
		})
		It("should not fail on cgroupfs as cgroup manager and conmon_cgroup as empty", func() {
			handler := &config.RuntimeHandler{}

			// Given
			cgm, _ := cgmgr.SetCgroupManager("cgroupfs") //nolint:errcheck
			runtimeConfig := *config.DefaultRuntimeConfig(cgm)

			// When
			err := runtimeConfig.TranslateMonitorFieldsForHandler(handler, true)

			// Then
			Expect(handler.MonitorCgroup).To(Equal(""))
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail on invalid InfraCtrCPUSet", func() {
			// Given
			sut.InfraCtrCPUSet = "unparsable"

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should inherit from .Conmon even if bogus", func() {
			// Given
			sut.Conmon = invalidPath
			handler := &config.RuntimeHandler{}

			// When
			err := sut.TranslateMonitorFieldsForHandler(handler, true)

			// Then
			Expect(err).To(HaveOccurred())
		})
		It("should inherit from .Conmon", func() {
			// Given
			sut.Conmon = validConmonPath()
			handler := &config.RuntimeHandler{}

			// When
			err := sut.TranslateMonitorFieldsForHandler(handler, true)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(handler.MonitorPath).To(Equal(sut.Conmon))
		})
		It("should inherit from .ConmonEnv", func() {
			// Given
			sut.ConmonEnv = []string{"PATH=/foo/bar/baz"}
			handler := &config.RuntimeHandler{}

			// When
			err := sut.TranslateMonitorFieldsForHandler(handler, false)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(handler.MonitorEnv).To(Equal(sut.ConmonEnv))
		})
		It("should inherit from .ConmonCgroup", func() {
			// Given
			sut.ConmonCgroup = "system.slice"
			handler := &config.RuntimeHandler{}

			// When
			err := sut.TranslateMonitorFieldsForHandler(handler, false)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(handler.MonitorCgroup).To(Equal(sut.ConmonCgroup))
		})

		It("should configure a taskset prefix for cmdrunner for a valid InfraCtrCPUSet", func() {
			executable, err := exec.LookPath("taskset")
			if err != nil {
				Skip("this test relies on 'taskset' being present")
			}

			// Given
			cmdrunner.ResetPrependedCmd()
			sut.InfraCtrCPUSet = "0"

			// When
			err = sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(cmdrunner.GetPrependedCmd()).To(Equal(executable))
		})

		It("should not configure a taskset prefix for cmdrunner for an empty InfraCtrCPUSet", func() {
			// Given
			cmdrunner.ResetPrependedCmd()
			sut.InfraCtrCPUSet = ""

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(cmdrunner.GetPrependedCmd()).To(Equal(""))
		})
	})

	t.Describe("ValidateRuntimes", func() {
		It("should succeed with default config", func() {
			// Given
			// When
			err := sut.ValidateRuntimes()

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should succeed with empty runtime_type", func() {
			// Given
			sut.Runtimes[config.DefaultRuntime] = &config.RuntimeHandler{
				RuntimePath: validFilePath,
			}

			// When
			err := sut.ValidateRuntimes()

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail if default executable not in $PATH", func() {
			// Given
			sut.Runtimes[invalidPath] = &config.RuntimeHandler{RuntimePath: ""}
			sut.DefaultRuntime = invalidPath

			// When
			err := sut.ValidateRuntimes()

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should not fail if non-default executable not in $PATH", func() {
			// Given
			sut.Runtimes[invalidPath] = &config.RuntimeHandler{RuntimePath: ""}
			sut.DefaultRuntime = config.DefaultRuntime

			// When
			err := sut.ValidateRuntimes()

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail with wrong but set runtime_path", func() {
			// Given
			sut.Runtimes[config.DefaultRuntime] = &config.RuntimeHandler{RuntimePath: invalidPath}

			// When
			err := sut.ValidateRuntimes()

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail with wrong runtime_type", func() {
			// Given
			sut.Runtimes[config.DefaultRuntime] = &config.RuntimeHandler{
				RuntimePath: validFilePath,
				RuntimeType: "wrong",
			}

			// When
			err := sut.ValidateRuntimes()

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail with wrong allowed_annotation", func() {
			// Given
			sut.Runtimes[config.DefaultRuntime] = &config.RuntimeHandler{
				RuntimePath:        validFilePath,
				AllowedAnnotations: []string{"wrong"},
			}

			// When
			err := sut.ValidateRuntimes()

			// Then
			Expect(err).To(HaveOccurred())
		})
		It("should have allowed and disallowed annotation", func() {
			// Given
			sut.Runtimes[config.DefaultRuntime] = &config.RuntimeHandler{
				RuntimePath:        validFilePath,
				AllowedAnnotations: []string{crioann.DevicesAnnotation},
			}

			// When
			err := sut.ValidateRuntimes()

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.Runtimes[config.DefaultRuntime].AllowedAnnotations).To(ContainElement(crioann.DevicesAnnotation))
			Expect(sut.Runtimes[config.DefaultRuntime].DisallowedAnnotations).NotTo(ContainElement(crioann.DevicesAnnotation))
		})

		It("should allow no_sync_log for implicit default runtime", func() {
			sut.Runtimes[config.DefaultRuntime] = &config.RuntimeHandler{
				RuntimePath: validFilePath,
			}
			sut.Runtimes[config.DefaultRuntime].NoSyncLog = true

			err := sut.Runtimes[config.DefaultRuntime].Validate(config.DefaultRuntime)

			Expect(err).ToNot(HaveOccurred())
			Expect(sut.Runtimes[config.DefaultRuntime].NoSyncLog).To(BeTrue())
		})

		It("should allow no_sync_log for the 'oci' runtime", func() {
			sut.Runtimes[config.DefaultRuntime] = &config.RuntimeHandler{
				RuntimePath: validFilePath,
				RuntimeType: "oci",
			}
			sut.Runtimes[config.DefaultRuntime].NoSyncLog = true

			err := sut.Runtimes[config.DefaultRuntime].Validate(config.DefaultRuntime)

			Expect(err).ToNot(HaveOccurred())
			Expect(sut.Runtimes[config.DefaultRuntime].NoSyncLog).To(BeTrue())
		})

		It("should disallow no_sync_log for the 'vm' runtime", func() {
			sut.Runtimes["kata"] = &config.RuntimeHandler{
				RuntimePath: "containerd-shim-kata-qemu-v2", RuntimeType: config.RuntimeTypeVM,
			}
			sut.Runtimes["kata"].NoSyncLog = true

			err := sut.Runtimes["kata"].ValidateNoSyncLog()

			Expect(err).To(HaveOccurred())
			Expect(err).To(MatchError("no_sync_log is only allowed with runtime type 'oci', runtime type is 'vm'"))
		})
	})

	t.Describe("ValidateConmonPath", func() {
		It("should succeed with valid file in $PATH", func() {
			// Given
			sut.Conmon = ""
			handler := &config.RuntimeHandler{MonitorPath: ""}

			// When
			err := sut.ValidateConmonPath(validConmonPath(), handler)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(handler.MonitorPath).To(Equal(validConmonPath()))
		})

		It("should fail with invalid file in $PATH", func() {
			// Given
			handler := &config.RuntimeHandler{MonitorPath: ""}

			// When
			err := sut.ValidateConmonPath(invalidPath, handler)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should succeed with valid file outside $PATH", func() {
			// Given
			handler := &config.RuntimeHandler{MonitorPath: validConmonPath()}

			// When
			err := sut.ValidateConmonPath("", handler)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail with invalid file outside $PATH", func() {
			// Given
			handler := &config.RuntimeHandler{MonitorPath: invalidPath}

			// When
			err := sut.ValidateConmonPath("", handler)

			// Then
			Expect(err).To(HaveOccurred())
		})
	})

	t.Describe("ValidateImageConfig", func() {
		It("should succeed with default config", func() {
			// Given
			// When
			err := sut.ImageConfig.Validate(false)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should succeed on execution and writing permissions", func() {
			// Given
			sut.SignaturePolicyDir = os.TempDir()

			// When
			err := sut.ImageConfig.Validate(true)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail when SignaturePolicyDir is not absolute", func() {
			// Given
			sut.SignaturePolicyDir = "./wrong/path"

			// When
			err := sut.ImageConfig.Validate(false)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail when PauseImage is invalid", func() {
			// Given
			sut.PauseImage = "//NOT:a valid image reference!"

			// When
			err := sut.ImageConfig.Validate(false)

			// Then
			Expect(err).To(HaveOccurred())
		})
	})

	t.Describe("ImageConfig.ParsePauseImage", func() {
		It("should succeed with the default value", func() {
			// Given
			sut.PauseImage = config.DefaultPauseImage

			// When
			ref, err := sut.ParsePauseImage()

			// Then
			Expect(err).ToNot(HaveOccurred())
			// DefaultPauseImage is using a canonical form where this comparison is expected to work.
			Expect(ref.StringForOutOfProcessConsumptionOnly()).To(Equal(config.DefaultPauseImage))
		})

		It("should succeed with a name-only value", func() {
			// Given
			sut.PauseImage = "registry.k8s.io/pause"

			// When
			ref, err := sut.ParsePauseImage()

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(ref.StringForOutOfProcessConsumptionOnly()).To(Equal("registry.k8s.io/pause:latest"))
		})

		It("should succeed with a short name", func() {
			// NOTE: This behavior is undocumented. Users are expected to provide a
			// name with a registry

			// Given
			sut.PauseImage = "short:notlatest"

			// When
			ref, err := sut.ParsePauseImage()

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(ref.StringForOutOfProcessConsumptionOnly()).To(Equal("docker.io/library/short:notlatest"))
		})

		It("should fail with an invalid value", func() {
			// Given
			sut.PauseImage = "//THIS is:very!invalid="

			// When
			_, err := sut.ParsePauseImage()

			// Then
			Expect(err).To(HaveOccurred())
		})
	})

	t.Describe("ValidateNetworkConfig", func() {
		It("should succeed with default config", func() {
			// Given
			// When
			err := sut.NetworkConfig.Validate(false)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should succeed during runtime", func() {
			// Given
			sut = runtimeValidConfig()

			// When
			err := sut.NetworkConfig.Validate(true)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should create the NetworkDir", func() {
			// Given
			tmpDir := t.MustTempDir("network")
			Expect(os.RemoveAll(tmpDir)).ToNot(HaveOccurred())

			sut.NetworkDir = tmpDir
			sut.PluginDirs = []string{validDirPath}

			// When
			err := sut.NetworkConfig.Validate(true)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail on invalid NetworkDir", func() {
			// Given
			tmpfile := path.Join(os.TempDir(), "wrong-file")
			file, err := os.Create(tmpfile)
			Expect(err).ToNot(HaveOccurred())
			file.Close()
			defer os.Remove(tmpfile)
			sut.NetworkDir = tmpfile
			sut.PluginDirs = []string{}

			// When
			err = sut.NetworkConfig.Validate(true)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail on invalid PluginDirs", func() {
			// Given
			sut.NetworkDir = validDirPath
			sut.PluginDirs = []string{invalidPath}

			// When
			err := sut.NetworkConfig.Validate(true)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should succeed on having PluginDir", func() {
			// Given
			sut.NetworkDir = validDirPath
			sut.PluginDir = validDirPath
			sut.PluginDirs = []string{}

			// When
			err := sut.NetworkConfig.Validate(true)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should succeed in appending PluginDir to PluginDirs", func() {
			// Given
			sut.NetworkDir = validDirPath
			sut.PluginDir = validDirPath
			sut.PluginDirs = []string{}

			// When
			err := sut.NetworkConfig.Validate(true)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.NetworkConfig.PluginDirs[0]).To(Equal(validDirPath))
		})

		It("should fail in validating invalid PluginDir", func() {
			// Given
			sut.NetworkDir = validDirPath
			sut.PluginDir = invalidPath
			sut.PluginDirs = []string{}

			// When
			err := sut.NetworkConfig.Validate(true)

			// Then
			Expect(err).To(HaveOccurred())
		})
	})

	t.Describe("ValidateRootConfig", func() {
		It("should succeed with default config", func() {
			// Given
			// When
			err := sut.RootConfig.Validate(false)

			// Then
			Expect(err).ToNot(HaveOccurred())
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
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail on invalid LogDir", func() {
			// Given
			sut.LogDir = "/dev/null"

			// When
			err := sut.RootConfig.Validate(true)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail with non absolute log_dir", func() {
			// Given
			sut.LogDir = "test"

			// When
			err := sut.Validate(true)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should get default storage options when options are empty", func() {
			if isRootless() {
				Skip("this test does not work rootless")
			}

			// Given
			defaultStore, err := storage.GetStore(storage.StoreOptions{})
			Expect(err).ToNot(HaveOccurred())

			sut.RunRoot = ""
			sut.Root = ""
			sut.Storage = ""
			sut.StorageOptions = make([]string, 0)
			// this must be set in case pinns isn't downloaded to the $PATH
			sut.PinnsPath = alwaysPresentPath

			// When
			err = sut.Validate(true)

			// Then
			Expect(err).ToNot(HaveOccurred())
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
			Expect(err).ToNot(HaveOccurred())

			sut.RunRoot = alwaysPresentPath
			sut.Root = alwaysPresentPath
			// this must be set in case pinns isn't downloaded to the $PATH
			sut.PinnsPath = alwaysPresentPath

			// When
			err = sut.Validate(true)

			// Then
			Expect(err).ToNot(HaveOccurred())
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
			Expect(err).ToNot(HaveOccurred())
			_, err = os.Stat(tmpfile)
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail with invalid path", func() {
			// Given
			// When
			err := sut.ToFile(invalidPath)

			// Then
			Expect(err).To(HaveOccurred())
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
			).To(Succeed())

			// When
			err := sut.UpdateFromFile(context.Background(), f)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.Storage).To(Equal("overlay2"))
			Expect(sut.Runtimes).To(HaveLen(1))
			Expect(sut.Runtimes).To(HaveKey(config.DefaultRuntime))
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
			).To(Succeed())
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
				err := defaultcfg.UpdateFromFile(context.Background(), f)

				// Then
				Expect(err).ToNot(HaveOccurred())
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
				Expect(os.WriteFile(f, tc.criocfg, 0)).To(Succeed())
				// When
				defaultcfg := defaultConfig()
				defaultcfg.Root = tc.graphRoot
				err := defaultcfg.UpdateFromFile(context.Background(), f)

				// Then
				Expect(err).ToNot(HaveOccurred())
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
				Expect(os.WriteFile(f, tc.criocfg, 0)).To(Succeed())
				// When
				defaultcfg := defaultConfig()
				defaultcfg.RunRoot = tc.runRoot
				err := defaultcfg.UpdateFromFile(context.Background(), f)

				// Then
				Expect(err).ToNot(HaveOccurred())
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
				Expect(os.WriteFile(f, tc.criocfg, 0)).To(Succeed())
				// When
				defaultcfg := defaultConfig()
				defaultcfg.Storage = tc.storageDriver
				err := defaultcfg.UpdateFromFile(context.Background(), f)

				// Then
				Expect(err).ToNot(HaveOccurred())
				Expect(defaultcfg.Storage).To(Equal(tc.expect))
			}
		})

		It("should succeed with custom runtime", func() {
			// Given
			f := t.MustTempFile("config")
			Expect(os.WriteFile(f,
				[]byte("[crio.runtime.runtimes.foo]"), 0),
			).To(Succeed())

			// When
			err := sut.UpdateFromFile(context.Background(), f)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.Runtimes).To(HaveLen(2))
			Expect(sut.Runtimes).To(HaveKey("foo"))
		})

		It("should succeed with additional runtime", func() {
			// Given
			f := t.MustTempFile("config")
			Expect(os.WriteFile(f,
				[]byte(`
					[crio.runtime.runtimes.crun]
					[crio.runtime.runtimes.foo]
				`), 0),
			).To(Succeed())

			// When
			err := sut.UpdateFromFile(context.Background(), f)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.Runtimes).To(HaveLen(2))
			Expect(sut.Runtimes).To(HaveKey("foo"))
			Expect(sut.Runtimes).To(HaveKey(config.DefaultRuntime))
		})

		It("should succeed with additional runtime with inheritance", func() {
			// Given
			f := t.MustTempFile("config")
			Expect(os.WriteFile(f,
				[]byte(`
					[crio.runtime.runtimes.crun]
					[crio.runtime.runtimes.foo]
					inherit_default_runtime = true
				`), 0),
			).To(Succeed())

			// When
			err := sut.UpdateFromFile(context.Background(), f)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.Runtimes).To(HaveLen(2))
			Expect(sut.Runtimes).To(HaveKey("foo"))
			Expect(sut.Runtimes).To(HaveKey(config.DefaultRuntime))
			Expect(sut.Runtimes["foo"].InheritDefaultRuntime).To(BeTrue())
		})

		It("should fail when file does not exist", func() {
			// Given
			// When
			err := sut.UpdateFromFile(context.Background(), "/invalid/file")

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail when toml decode fails", func() {
			// Given
			// When
			err := sut.UpdateFromFile(context.Background(), "config.go")

			// Then
			Expect(err).To(HaveOccurred())
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
			Expect(err).ToNot(HaveOccurred())
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
			)).To(Succeed())
			Expect(os.WriteFile(
				filepath.Join(configDir, "01-my-config"),
				[]byte("[crio.runtime]\nlog_level = \"warning\"\n"),
				0o644,
			)).To(Succeed())

			// When
			err := sut.UpdateFromPath(context.Background(), configDir)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.LogLevel).To(Equal("warning"))
		})

		It("should fail with invalid config", func() {
			// Given
			configDir := t.MustTempDir("config-dir")
			Expect(os.WriteFile(
				filepath.Join(configDir, "00-default"),
				[]byte("[crio.runtime]\nlog_level = true\n"),
				0o644,
			)).To(Succeed())

			// When
			err := sut.UpdateFromPath(context.Background(), configDir)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should succeed with not existing path", func() {
			// Given
			// When
			err := sut.UpdateFromPath(context.Background(), "not-existing")

			// Then
			Expect(err).ToNot(HaveOccurred())
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
			sut.Runtimes[config.DefaultRuntime] = &config.RuntimeHandler{
				RuntimeConfigPath: validFilePath, RuntimeType: config.DefaultRuntimeType,
			}

			// When
			err := sut.Runtimes[config.DefaultRuntime].ValidateRuntimeConfigPath(config.DefaultRuntime)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail with VM runtime type and runtime_config_path points to an invalid path", func() {
			// Given
			sut.Runtimes["kata"] = &config.RuntimeHandler{
				RuntimeConfigPath: invalidPath, RuntimeType: config.RuntimeTypeVM,
			}

			// When
			err := sut.Runtimes["kata"].ValidateRuntimeConfigPath("kata")

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should succeed with VM runtime type and runtime_config_path points to a valid path", func() {
			// Given
			sut.Runtimes["kata"] = &config.RuntimeHandler{
				RuntimeConfigPath: validFilePath, RuntimeType: config.RuntimeTypeVM,
			}

			// When
			err := sut.Runtimes["kata"].ValidateRuntimeConfigPath("kata")

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should succeed with empty runtime type and runtime_config_path when inheriting from default", func() {
			// Given
			sut.Runtimes["inherited"] = &config.RuntimeHandler{
				RuntimeConfigPath: invalidPath, RuntimeType: "invalid", InheritDefaultRuntime: true,
			}
			// When
			err := sut.ValidateRuntimes()

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.Runtimes).To(HaveKey(sut.DefaultRuntime))
			Expect(sut.Runtimes).To(HaveKey("inherited"))

			// When
			Expect(sut.Runtimes["inherited"].RuntimePath).To(Equal(sut.Runtimes[sut.DefaultRuntime].RuntimePath))
		})
	})

	t.Describe("RuntimeHandlerFeatures", func() {
		It("should fail to load runtime features with nothing to load", func() {
			// Given
			handler := &config.RuntimeHandler{}

			err := handler.LoadRuntimeFeatures([]byte(``))

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail to load runtime features with an empty document", func() {
			// Given
			handler := &config.RuntimeHandler{}

			err := handler.LoadRuntimeFeatures([]byte(`{}`))

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail to load OCI runtime features when required attributes are missing", func() {
			// Given
			handler := &config.RuntimeHandler{}

			err := handler.LoadRuntimeFeatures(
				[]byte(`
					{
					  "ociVersionMin": "1.0.0",
					  "mountOptions": ["ro"]
					}
				`),
			)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail to load OCI runtime features when malformed document is used", func() {
			// Given
			handler := &config.RuntimeHandler{}

			err := handler.LoadRuntimeFeatures(
				[]byte(`
					{
					  "ociVersionMin": "1.0.0",
					  "ociVersionMax": "1.2.0",
					  "mountOptions": ["ro",]
					}
				`),
			)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should succeed to load OCI runtime features with support for RRO mounts", func() {
			// Given
			handler := &config.RuntimeHandler{}

			err := handler.LoadRuntimeFeatures(
				[]byte(`
					{
					  "ociVersionMin": "1.0.0",
					  "ociVersionMax": "1.2.0",
					  "mountOptions": ["ro", "rro"]
					}
				`),
			)

			// Then
			Expect(err).ToNot(HaveOccurred())

			// When
			ok := handler.RuntimeSupportsMountFlag("rro")

			// Then
			Expect(ok).To(BeTrue())
		})

		It("should succeed to load OCI runtime features with support for ID-mapping", func() {
			// Given
			handler := &config.RuntimeHandler{}

			err := handler.LoadRuntimeFeatures(
				[]byte(`
					{
					  "ociVersionMin": "1.0.0",
					  "ociVersionMax": "1.2.0",
					  "linux": {
					    "mountExtensions": {
					      "idmap": {
					        "enabled": true
					      }
					    }
					  }
					}
				`),
			)

			// Then
			Expect(err).ToNot(HaveOccurred())

			// When
			ok := handler.RuntimeSupportsIDMap()

			// Then
			Expect(ok).To(BeTrue())
		})
	})
})
