package lib_test

import (
	"io/ioutil"
	"os"
	"path"

	"github.com/cri-o/cri-o/lib"
	"github.com/cri-o/cri-o/oci"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("Config", func() {
	// The system under test
	var sut *lib.Config

	BeforeEach(func() {
		var err error
		sut, err = lib.DefaultConfig(nil)
		Expect(err).To(BeNil())
		Expect(sut).NotTo(BeNil())
	})

	const (
		validPath = "/bin/sh"
		wrongPath = "/wrong"
	)

	t.Describe("ValidateConfig", func() {
		It("should succeed with default config", func() {
			// Given
			// When
			err := sut.Validate(nil, false)

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail with invalid root config", func() {
			// Given
			sut.RootConfig.LogDir = "/dev/null"

			// When
			err := sut.Validate(nil, true)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with invalid runtime config", func() {
			// Given
			sut.RootConfig.LogDir = "."
			sut.AdditionalDevices = []string{wrongPath}

			// When
			err := sut.Validate(nil, true)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with invalid network config", func() {
			// Given
			sut.RootConfig.LogDir = "."
			sut.Runtimes["runc"] = oci.RuntimeHandler{RuntimePath: validPath}
			sut.Conmon = validPath
			sut.NetworkConfig.NetworkDir = wrongPath

			// When
			err := sut.Validate(nil, true)

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
			sut.Runtimes["runc"] = oci.RuntimeHandler{RuntimePath: validPath}
			sut.Conmon = validPath

			// When
			err := sut.RuntimeConfig.Validate(nil, true)

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed with additional devices", func() {
			// Given
			sut.AdditionalDevices = []string{"/dev/null:/dev/null:rw"}
			sut.Runtimes["runc"] = oci.RuntimeHandler{RuntimePath: validPath}
			sut.Conmon = validPath

			// When
			err := sut.RuntimeConfig.Validate(nil, true)

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed with hooks directories", func() {
			// Given
			sut.Runtimes["runc"] = oci.RuntimeHandler{RuntimePath: validPath}
			sut.Conmon = validPath
			sut.HooksDir = []string{validPath}

			// When
			err := sut.RuntimeConfig.Validate(nil, true)

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail on invalid hooks directory", func() {
			// Given
			sut.Runtimes["runc"] = oci.RuntimeHandler{RuntimePath: validPath}
			sut.Conmon = validPath
			sut.HooksDir = []string{wrongPath}

			// When
			err := sut.RuntimeConfig.Validate(nil, true)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on invalid conmon path", func() {
			// Given
			sut.Runtimes["runc"] = oci.RuntimeHandler{RuntimePath: validPath}
			sut.Conmon = wrongPath
			sut.HooksDir = []string{validPath}

			// When
			err := sut.RuntimeConfig.Validate(nil, true)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on wrong DefaultUlimits", func() {
			// Given
			sut.DefaultUlimits = []string{wrongPath}

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
			sut.AdditionalDevices = []string{wrongPath}

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
			sut.Runtimes = make(map[string]oci.RuntimeHandler)

			// When
			err := sut.RuntimeConfig.Validate(nil, false)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on non existing runtime binary", func() {
			// Given
			sut.Runtimes["runc"] = oci.RuntimeHandler{RuntimePath: "not-existing"}

			// When
			err := sut.RuntimeConfig.Validate(nil, true)

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
			sut.NetworkConfig.NetworkDir = validPath
			tmpDir := path.Join(os.TempDir(), "cni-test")
			sut.NetworkConfig.PluginDir = []string{tmpDir}
			defer os.RemoveAll(tmpDir)

			// When
			err := sut.NetworkConfig.Validate(true)

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail on invalid NetworkDir", func() {
			// Given
			sut.NetworkConfig.NetworkDir = wrongPath

			// When
			err := sut.NetworkConfig.Validate(true)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on PluginDir without permissions", func() {
			// Given
			sut.NetworkConfig.NetworkDir = validPath
			sut.NetworkConfig.PluginDir = []string{wrongPath}

			// When
			err := sut.NetworkConfig.Validate(true)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on invalid PluginDir", func() {
			// Given
			sut.NetworkConfig.NetworkDir = validPath
			sut.NetworkConfig.PluginDir = []string{validPath}

			// When
			err := sut.NetworkConfig.Validate(true)

			// Then
			Expect(err).NotTo(BeNil())
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
			sut.RootConfig.LogDir = "."

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
			err := sut.ToFile("/proc/invalid")

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
			sut := &lib.Config{}

			// When
			config := sut.GetData()

			// Then
			Expect(config).NotTo(BeNil())
			Expect(config).To(Equal(sut))
		})

		It("should succeed with nil config", func() {
			// Given
			var sut *lib.Config

			// When
			config := sut.GetData()

			// Then
			Expect(config).To(BeNil())
		})
	})
})
