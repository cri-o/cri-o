package config_test

import (
	"context"
	"os"
	"strings"

	"github.com/containers/common/pkg/apparmor"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/pkg/config"
)

// The actual test suite.
var _ = t.Describe("Config", func() {
	BeforeEach(beforeEach)

	t.Describe("Reload", func() {
		modifyDefaultConfig := func(old, new string) {
			filePath := t.MustTempFile("config")
			Expect(sut.ToFile(filePath)).To(Succeed())
			Expect(sut.UpdateFromFile(context.Background(), filePath)).To(Succeed())

			read, err := os.ReadFile(filePath)
			Expect(err).ToNot(HaveOccurred())

			newContents := strings.ReplaceAll(string(read), old, new)
			err = os.WriteFile(filePath, []byte(newContents), 0)
			Expect(err).ToNot(HaveOccurred())
		}

		It("should succeed without any config change", func() {
			// Given
			filePath := t.MustTempFile("config")
			Expect(sut.ToFile(filePath)).To(Succeed())
			Expect(sut.UpdateFromFile(context.Background(), filePath)).To(Succeed())

			// When
			err := sut.Reload(context.Background())

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail with invalid log_level", func() {
			// Given
			modifyDefaultConfig(
				`log_level = "info"`,
				`log_level = "invalid"`,
			)

			// When
			err := sut.Reload(context.Background())

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail with invalid pause_image_auth_file", func() {
			// Given
			modifyDefaultConfig(
				`pause_image_auth_file = ""`,
				`pause_image_auth_file = "`+invalidPath+`"`,
			)

			// When
			err := sut.Reload(context.Background())

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should not fail with invalid seccomp_profile path", func() {
			// Given
			modifyDefaultConfig(
				`seccomp_profile = ""`,
				`seccomp_profile = "`+invalidPath+`"`,
			)

			// When
			err := sut.Reload(context.Background())

			// Then
			Expect(err).ToNot(HaveOccurred())
		})
	})

	t.Describe("ReloadLogLevel", func() {
		It("should succeed without any config change", func() {
			// Given
			// When
			err := sut.ReloadLogLevel(sut)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should succeed with config change", func() {
			// Given
			const newLogLevel = "fatal"
			newConfig := defaultConfig()
			newConfig.LogLevel = newLogLevel

			// When
			err := sut.ReloadLogLevel(newConfig)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.LogLevel).To(Equal(newLogLevel))
		})

		It("should fail with invalid log_level", func() {
			// Given
			newConfig := defaultConfig()
			newConfig.LogLevel = invalid

			// When
			err := sut.ReloadLogLevel(newConfig)

			// Then
			Expect(err).To(HaveOccurred())
		})
	})

	t.Describe("ReloadLogFilter", func() {
		It("should succeed without any config change", func() {
			// Given
			// When
			err := sut.ReloadLogFilter(sut)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should succeed with config change", func() {
			// Given
			const newLogFilter = "fatal"
			newConfig := defaultConfig()
			newConfig.LogFilter = newLogFilter

			// When
			err := sut.ReloadLogFilter(newConfig)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.LogFilter).To(Equal(newLogFilter))
		})

		It("should fail with invalid log_filter", func() {
			// Given
			newConfig := defaultConfig()
			newConfig.LogFilter = "("

			// When
			err := sut.ReloadLogFilter(newConfig)

			// Then
			Expect(err).To(HaveOccurred())
		})
	})

	t.Describe("ReloadPauseImage", func() {
		It("should succeed without any config change", func() {
			// Given
			// When
			err := sut.ReloadPauseImage(sut)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should succeed with pause_image change", func() {
			// Given
			const newPauseImage = "my-pause"
			newConfig := defaultConfig()
			newConfig.PauseImage = newPauseImage

			// When
			err := sut.ReloadPauseImage(newConfig)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.PauseImage).To(Equal(newPauseImage))
		})

		It("should fail with invalid pause_image change", func() {
			// Given
			const newPauseImage = "//THIS=is!invalid"
			newConfig := defaultConfig()
			newConfig.PauseImage = newPauseImage

			// When
			err := sut.ReloadPauseImage(newConfig)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should succeed with pause_command change", func() {
			// Given
			const newPauseCommand = "/new-pause"
			newConfig := defaultConfig()
			newConfig.PauseCommand = newPauseCommand

			// When
			err := sut.ReloadPauseImage(newConfig)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.PauseCommand).To(Equal(newPauseCommand))
		})

		It("should succeed with pause_image_auth_file change", func() {
			// Given
			newConfig := defaultConfig()
			newConfig.PauseImageAuthFile = validFilePath

			// When
			err := sut.ReloadPauseImage(newConfig)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.PauseImageAuthFile).To(Equal(validFilePath))
		})

		It("should fail with invalid pause_image_auth_file", func() {
			// Given
			newConfig := defaultConfig()
			newConfig.PauseImageAuthFile = invalidPath

			// When
			err := sut.ReloadPauseImage(newConfig)

			// Then
			Expect(err).To(HaveOccurred())
		})
	})

	t.Describe("ReloadRegistries", func() {
		It("should succeed to reload registries", func() {
			// Given
			// When
			err := sut.ReloadRegistries()

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail if registries file does not exist", func() {
			// Given
			sut.SystemContext.SystemRegistriesConfPath = invalidPath

			// When
			err := sut.ReloadRegistries()

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should fail if registries file is invalid", func() {
			// Given
			regConf := t.MustTempFile("reload-registries")
			Expect(os.WriteFile(regConf, []byte("invalid"), 0o755)).To(Succeed())
			sut.SystemContext.SystemRegistriesConfPath = regConf

			// When
			err := sut.ReloadRegistries()

			// Then
			Expect(err).To(HaveOccurred())
		})
	})

	t.Describe("ReloadSeccompProfile", func() {
		It("should succeed without any config change", func() {
			// Given
			// When
			err := sut.ReloadSeccompProfile(sut)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should succeed with config change", func() {
			// Given
			filePath := t.MustTempFile("seccomp")
			Expect(os.WriteFile(filePath, []byte(`{}`), 0o644)).To(Succeed())

			newConfig := defaultConfig()
			newConfig.SeccompProfile = filePath

			// When
			err := sut.ReloadSeccompProfile(newConfig)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.SeccompProfile).To(Equal(filePath))
		})

		It("should not fail with invalid seccomp_profile path", func() {
			// Given
			newConfig := defaultConfig()
			newConfig.SeccompProfile = invalidPath

			// When
			err := sut.ReloadSeccompProfile(newConfig)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})
	})

	t.Describe("ReloadAppArmorProfile", func() {
		BeforeEach(func() {
			if !apparmor.IsEnabled() {
				Skip("AppArmor is disabled")
			}
		})

		It("should succeed without any config change", func() {
			// Given
			// When
			err := sut.ReloadAppArmorProfile(sut)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should succeed with config change", func() {
			// Given
			const profile = "unconfined"
			newConfig := defaultConfig()
			newConfig.ApparmorProfile = profile

			// When
			err := sut.ReloadAppArmorProfile(newConfig)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.ApparmorProfile).To(Equal(profile))
		})
	})

	t.Describe("ReloadRuntimes", func() {
		var existingRuntimePath string
		BeforeEach(func() {
			existingRuntimePath = t.MustTempFile("runc")
		})

		It("should succeed without any config change", func() {
			// Given
			// When
			err := sut.ReloadRuntimes(sut)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})

		It("should fail for invalid default_runtime", func() {
			// Given
			newConfig := &config.Config{}
			newConfig.DefaultRuntime = "invalid"

			// When
			err := sut.ReloadRuntimes(newConfig)

			// Then
			Expect(err).To(HaveOccurred())
		})

		It("should add a new runtime", func() {
			// Given
			newRuntimeHandler := &config.RuntimeHandler{
				RuntimePath:                  existingRuntimePath,
				PrivilegedWithoutHostDevices: true,
			}
			newConfig := &config.Config{}
			newConfig.Runtimes = make(config.Runtimes)
			newConfig.Runtimes["new"] = newRuntimeHandler

			// When
			err := sut.ReloadRuntimes(newConfig)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.Runtimes).To(HaveKeyWithValue("new", newRuntimeHandler))
		})

		It("should change the default runtime", func() {
			// Given
			sut.Runtimes["existing"] = &config.RuntimeHandler{
				RuntimePath: existingRuntimePath,
			}
			newConfig := &config.Config{}
			newConfig.Runtimes = sut.Runtimes
			newConfig.DefaultRuntime = "existing"

			// When
			err := sut.ReloadRuntimes(newConfig)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.DefaultRuntime).To(Equal("existing"))
		})

		It("should overwrite existing runtime", func() {
			// Given
			existingRuntime := &config.RuntimeHandler{
				RuntimePath: existingRuntimePath,
			}
			sut.Runtimes["existing"] = existingRuntime

			newRuntime := &config.RuntimeHandler{
				RuntimePath:                  existingRuntimePath,
				PrivilegedWithoutHostDevices: true,
			}
			newConfig := &config.Config{}
			newConfig.Runtimes = make(config.Runtimes)
			newConfig.Runtimes["existing"] = newRuntime

			// When
			err := sut.ReloadRuntimes(newConfig)

			// Then
			Expect(err).ToNot(HaveOccurred())
			Expect(sut.Runtimes).To(HaveKeyWithValue("existing", newRuntime))
			Expect(sut.Runtimes["existing"].PrivilegedWithoutHostDevices).To(BeTrue())
		})
	})

	t.Describe("ReloadPinnedImages", func() {
		It("should update PinnedImages with newConfig's PinnedImages if they are different", func() {
			sut.PinnedImages = []string{"image1", "image4", "image3"}
			newConfig := &config.Config{}
			newConfig.PinnedImages = []string{"image5"}
			sut.ReloadPinnedImages(newConfig)
			Expect(sut.PinnedImages).To(Equal([]string{"image5"}))
		})

		It("should not update PinnedImages if they are the same as newConfig's PinnedImages", func() {
			sut.PinnedImages = []string{"image1", "image2", "image3"}
			newConfig := &config.Config{}
			newConfig.PinnedImages = []string{"image1", "image2", "image3"}
			sut.ReloadPinnedImages(newConfig)
			Expect(sut.PinnedImages).To(Equal([]string{"image1", "image2", "image3"}))
		})
	})
})
