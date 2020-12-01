package log_test

import (
	"bytes"
	"testing"

	. "github.com/cri-o/cri-o/test/framework"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
)

// TestLog runs the created specs
func TestLog(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "Log")
}

var (
	t   *TestFramework
	sut *logrus.Logger
	buf *bytes.Buffer
)

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()
})

var _ = AfterSuite(func() {
	t.Teardown()
})

func beforeEach(level logrus.Level) {
	buf = &bytes.Buffer{}
	sut = logrus.StandardLogger()
	sut.Level = level
	sut.Out = buf
}
