package log_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"k8s.io/klog/v2"

	"github.com/cri-o/cri-o/internal/log"
)

// The actual test suite.
var _ = t.Describe("Log", func() {
	const msg = "Hello world"
	BeforeEach(func() {
		beforeEach(logrus.DebugLevel)
		log.InitKlogShim()
	})

	It("should convert info klog to debug log", func() {
		// Given
		// When
		klog.InfoS(msg)

		// Then
		Expect(buf.String()).To(ContainSubstring(msg))
		Expect(buf.String()).To(ContainSubstring("debug"))
		Expect(buf.String()).NotTo(ContainSubstring("("))
	})

	It("should convert info klog with keys and values to debug log", func() {
		// Given
		// When
		klog.InfoS(msg, "key1", "val1", "key2", "val2")

		// Then
		Expect(buf.String()).To(ContainSubstring(msg))
		Expect(buf.String()).To(ContainSubstring("debug"))
		Expect(buf.String()).To(ContainSubstring("(key1=\\\"val1\\\" key2=\\\"val2\\\")"))
	})

	It("should fill missing value", func() {
		// Given
		// When
		klog.InfoS(msg, "key1", "val1", "key2") //nolint:loggercheck

		// Then
		Expect(buf.String()).To(ContainSubstring(msg))
		Expect(buf.String()).To(ContainSubstring("debug"))
		Expect(buf.String()).To(ContainSubstring("(key1=\\\"val1\\\" key2=\\\"[MISSING]\\\")"))
	})
})
