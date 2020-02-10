package config_test

import (
	"io/ioutil"
	"strings"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("Config", func() {
	BeforeEach(beforeEach)

	t.Describe("Reload", func() {
		var modifyDefaultConfig = func(old, new string) {
			filePath := t.MustTempFile("config")
			Expect(sut.ToFile(filePath)).To(BeNil())
			Expect(sut.UpdateFromFile(filePath)).To(BeNil())

			read, err := ioutil.ReadFile(filePath)
			Expect(err).To(BeNil())

			newContents := strings.ReplaceAll(string(read), old, new)
			err = ioutil.WriteFile(filePath, []byte(newContents), 0)
			Expect(err).To(BeNil())
		}

		It("should succeed without any config change", func() {
			// Given
			filePath := t.MustTempFile("config")
			Expect(sut.ToFile(filePath)).To(BeNil())
			Expect(sut.UpdateFromFile(filePath)).To(BeNil())

			// When
			err := sut.Reload()

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail with invalid log_level", func() {
			// Given
			modifyDefaultConfig(
				`log_level = "info"`,
				`log_level = "invalid"`,
			)

			// When
			err := sut.Reload()

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with invalid pause_image_auth_file", func() {
			// Given
			modifyDefaultConfig(
				`pause_image_auth_file = ""`,
				`pause_image_auth_file = "`+invalidPath+`"`,
			)

			// When
			err := sut.Reload()

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

	t.Describe("ReloadLogFilter", func() {
		It("should succeed without any config change", func() {
			// Given
			// When
			err := sut.ReloadLogFilter(sut)

			// Then
			Expect(err).To(BeNil())
		})

		It("should succeed with config change", func() {
			// Given
			const newLogFilter = "fatal"
			newConfig := defaultConfig()
			newConfig.LogFilter = newLogFilter

			// When
			err := sut.ReloadLogFilter(newConfig)

			// Then
			Expect(err).To(BeNil())
			Expect(sut.LogFilter).To(Equal(newLogFilter))
		})

		It("should fail with invalid log_filter", func() {
			// Given
			newConfig := defaultConfig()
			newConfig.LogFilter = "("

			// When
			err := sut.ReloadLogFilter(newConfig)

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
			newConfig.PauseImageAuthFile = validFilePath

			// When
			err := sut.ReloadPauseImage(newConfig)

			// Then
			Expect(err).To(BeNil())
			Expect(sut.PauseImageAuthFile).To(Equal(validFilePath))
		})

		It("should fail with invalid pause_image_auth_file", func() {
			// Given
			newConfig := defaultConfig()
			newConfig.PauseImageAuthFile = invalidPath

			// When
			err := sut.ReloadPauseImage(newConfig)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("ReloadRegistries", func() {
		It("should succeed to reload registries", func() {
			// Given
			// When
			err := sut.ReloadRegistries()

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail if registries file does not exist", func() {
			// Given
			sut.SystemContext.SystemRegistriesConfPath = invalidPath

			// When
			err := sut.ReloadRegistries()

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail if registries file is invalid", func() {
			// Given
			regConf := t.MustTempFile("reload-registries")
			Expect(ioutil.WriteFile(regConf, []byte("invalid"), 0755)).To(BeNil())
			sut.SystemContext.SystemRegistriesConfPath = regConf

			// When
			err := sut.ReloadRegistries()

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
