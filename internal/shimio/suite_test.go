package shimio_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
)

// TestShimIO runs the created specs.
func TestShimIO(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "ShimIO")
}

var _ = BeforeSuite(func() {
	logrus.SetLevel(logrus.PanicLevel)
})
