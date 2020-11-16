package container_test

import (
	"context"
	"testing"

	. "github.com/cri-o/cri-o/test/framework"
	"github.com/cri-o/cri-o/v1alpha2/container"
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
	var err error
	sut, err = container.New(context.Background())
	Expect(err).To(BeNil())
})
