package metrics_test

import (
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/server/metrics"
	. "github.com/cri-o/cri-o/test/framework"
)

// TestMetrics runs the created specs.
func TestMetrics(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Metrics")
}

//nolint:gochecknoglobals
var t *TestFramework

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()
})

var _ = AfterSuite(func() {
	t.Teardown()
})

// The actual test suite.
var _ = t.Describe("Metrics", func() {
	t.Describe("SinceInMicroseconds", func() {
		It("should succeed", func() {
			// Given
			// When
			res := metrics.SinceInMicroseconds(
				time.Now().Add(-time.Millisecond))

			// Then
			Expect(res).NotTo(BeZero())
		})

		It("should be zero at time.Now()", func() {
			// Given
			// When
			res := metrics.SinceInMicroseconds(time.Now())

			// Then
			Expect(res).To(BeZero())
		})
	})
})
