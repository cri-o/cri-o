package log_test

import (
	"github.com/cri-o/cri-o/internal/log"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
)

var _ = t.Describe("Hook", func() {
	t.Describe("RemoveHook", func() {
		var (
			logger       *logrus.Logger
			filterHook   *log.FilterHook
			fileNameHook *log.FileNameHook
		)

		// Setup the hooks
		BeforeEach(func() {
			logger = logrus.New()
			filterHook, err := log.NewFilterHook("")
			Expect(err).To(BeNil())
			Expect(filterHook).NotTo(BeNil())
			fileNameHook := log.NewFilenameHook()
			Expect(fileNameHook).NotTo(BeNil())
		})

		It("should succeed to remove", func() {
			// Given
			logger.AddHook(filterHook)
			logger.AddHook(fileNameHook)

			// When
			log.RemoveHook(logger, "FilterHook")

			// Then
			Expect(logger.Hooks).To(HaveLen(1))
		})

		It("should succeed to replace", func() {
			// Given
			logger.AddHook(filterHook)

			// When
			log.RemoveHook(logger, "FileNameHook")
			logger.AddHook(fileNameHook)

			// Then
			Expect(logger.Hooks).To(HaveLen(7))
		})
	})
})
