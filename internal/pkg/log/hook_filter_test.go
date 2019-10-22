package log_test

import (
	"fmt"

	"github.com/cri-o/cri-o/internal/pkg/log"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
)

var _ = t.Describe("HookFilter", func() {
	t.Describe("NewFilterHook", func() {
		It("should succeed to create", func() {
			// Given
			// When
			res, err := log.NewFilterHook("")

			// Then
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeNil())
		})

		It("should fail to create with invalid filter", func() {
			// Given
			// When
			res, err := log.NewFilterHook("(")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeNil())
		})
	})

	t.Describe("Levels", func() {
		It("should work for all log levels", func() {
			// Given
			hook, err := log.NewFilterHook("")
			Expect(err).To(BeNil())

			// When
			res := hook.Levels()

			// Then
			Expect(res).To(Equal(logrus.AllLevels))
		})
	})

	t.Describe("Fire", func() {
		It("should succeed to filter", func() {
			// Given
			hook, err := log.NewFilterHook("none")
			Expect(err).To(BeNil())
			entry := &logrus.Entry{
				Message: "this message will be filtered out",
			}

			// When
			res := hook.Fire(entry)

			// Then
			Expect(res).To(BeNil())
			Expect(entry.Message).To(BeEmpty())
		})

		It("should succeed to filter byte slice", func() {
			// Given
			hook, err := log.NewFilterHook("")
			Expect(err).To(BeNil())
			entry := &logrus.Entry{
				Message: fmt.Sprintf("a slice: %v", []byte{1, 2, 3, 4}),
				Level:   logrus.DebugLevel,
			}

			// When
			res := hook.Fire(entry)

			// Then
			Expect(res).To(BeNil())
			Expect(entry.Message).To(Equal("a slice: [FILTERED]"))
		})
	})
})
