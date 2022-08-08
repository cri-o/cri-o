package log_test

import (
	"context"

	"github.com/cri-o/cri-o/internal/log"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
)

// The actual test suite
var _ = t.Describe("Log", func() {
	var ctx context.Context

	const (
		msg  = "Hello world"
		id   = "some-id"
		name = "some-name"
	)

	idEntry := "id=" + id
	nameEntry := "name=" + name

	BeforeEach(func() {
		ctx = context.WithValue(
			context.WithValue(context.Background(), log.ID{}, id),
			log.Name{}, name,
		)
	})

	t.Describe("Debugf", func() {
		BeforeEach(func() { beforeEach(logrus.DebugLevel) })

		It("should succeed to debug log", func() {
			// Given
			// When
			log.Debugf(ctx, msg)

			// Then
			Expect(buf.String()).To(ContainSubstring(msg))
			Expect(buf.String()).To(ContainSubstring(idEntry))
			Expect(buf.String()).To(ContainSubstring(nameEntry))
		})

		It("should succeed to debug on empty context", func() {
			// Given
			// When
			log.Debugf(context.Background(), msg)

			// Then
			Expect(buf.String()).To(ContainSubstring(msg))
			Expect(buf.String()).ToNot(ContainSubstring(idEntry))
			Expect(buf.String()).ToNot(ContainSubstring(nameEntry))
		})

		It("should succeed to debug on nil context", func() {
			// Given
			// When
			// nolint: staticcheck
			log.Debugf(nil, msg)

			// Then
			Expect(buf.String()).To(ContainSubstring(msg))
			Expect(buf.String()).ToNot(ContainSubstring(idEntry))
			Expect(buf.String()).ToNot(ContainSubstring(nameEntry))
		})
	})

	t.Describe("Infof", func() {
		BeforeEach(func() { beforeEach(logrus.InfoLevel) })

		It("should succeed to info log", func() {
			// Given
			// When
			log.Infof(ctx, msg)

			// Then
			Expect(buf.String()).To(ContainSubstring(msg))
			Expect(buf.String()).To(ContainSubstring(idEntry))
			Expect(buf.String()).To(ContainSubstring(nameEntry))
		})

		It("should not debug log", func() {
			// Given
			// When
			log.Debugf(ctx, msg)

			// Then
			Expect(buf.String()).To(BeEmpty())
		})
	})

	t.Describe("Warnf", func() {
		BeforeEach(func() { beforeEach(logrus.WarnLevel) })

		It("should succeed to warn log", func() {
			// Given
			// When
			log.Warnf(ctx, msg)

			// Then
			Expect(buf.String()).To(ContainSubstring(msg))
			Expect(buf.String()).To(ContainSubstring(idEntry))
			Expect(buf.String()).To(ContainSubstring(nameEntry))
		})

		It("should not info log", func() {
			// Given
			// When
			log.Infof(ctx, msg)

			// Then
			Expect(buf.String()).To(BeEmpty())
		})
	})

	t.Describe("Errorf", func() {
		BeforeEach(func() { beforeEach(logrus.ErrorLevel) })

		It("should succeed to error log", func() {
			// Given
			// When
			log.Errorf(ctx, msg)

			// Then
			Expect(buf.String()).To(ContainSubstring(msg))
			Expect(buf.String()).To(ContainSubstring(idEntry))
			Expect(buf.String()).To(ContainSubstring(nameEntry))
		})

		It("should not warn log", func() {
			// Given
			// When
			log.Warnf(ctx, msg)

			// Then
			Expect(buf.String()).To(BeEmpty())
		})
	})

	t.Describe("Fatalf", func() {
		BeforeEach(func() { beforeEach(logrus.FatalLevel) })

		It("should not error log", func() {
			// Given
			// When
			log.Errorf(ctx, msg)

			// Then
			Expect(buf.String()).To(BeEmpty())
		})
	})
})
