package log_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"

	"github.com/cri-o/cri-o/internal/log"
)

var _ = t.Describe("HookFilename", func() {
	t.Describe("NewFilenameHook", func() {
		It("should succeed to create", func() {
			// Given
			// When
			res := log.NewFilenameHook()

			// Then
			Expect(res).NotTo(BeNil())
		})
	})

	t.Describe("Levels", func() {
		It("should work for debug log level", func() {
			// Given
			hook := log.NewFilenameHook()

			// When
			res := hook.Levels()

			// Then
			Expect(res).To(Equal([]logrus.Level{logrus.DebugLevel}))
		})
	})

	t.Describe("Fire", func() {
		It("should succeed", func() {
			// Given
			hook := log.NewFilenameHook()
			entry := &logrus.Entry{
				Message: "a log message",
				Logger: &logrus.Logger{
					Formatter: &logrus.JSONFormatter{},
				},
			}

			// When
			err := hook.Fire(entry)

			// Then
			Expect(err).ToNot(HaveOccurred())
		})
	})

	t.Describe("Format", func() {
		It("should succeed", func() {
			// Given
			hook := log.NewFilenameHook()

			// When
			res := hook.Formatter("file", "function", 0)

			// Then
			Expect(res).To(Equal("file:0"))
		})
	})
})
