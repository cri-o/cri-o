package container_test

import (
	"context"
	"testing"

	"github.com/cri-o/cri-o/pkg/container"
	. "github.com/cri-o/cri-o/test/framework"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// TestContainer runs the specs
func TestContainer(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "Container")
}

var (
	t   *TestFramework
	sut container.Container
)

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()
})

var _ = AfterSuite(func() {
	t.Teardown()
})

var _ = BeforeEach(func() {
	sut = container.New(context.Background())
})
