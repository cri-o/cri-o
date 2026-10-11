package annotations_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/internal/annotations"
	. "github.com/cri-o/cri-o/test/framework"
)

func TestAnnotations(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Annotations")
}

//nolint:gochecknoglobals // test framework requires global state
var t *TestFramework

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()
})

var _ = AfterSuite(func() {
	t.Teardown()
})

var _ = t.Describe("IsInternal", func() {
	DescribeTable("should return true for internal annotations",
		func(key string) {
			Expect(annotations.IsInternal(key)).To(BeTrue())
		},
		Entry("ShmPath", annotations.ShmPath),
		Entry("ContainerID", annotations.ContainerID),
		Entry("MountPoint", annotations.MountPoint),
		Entry("CNIResult", annotations.CNIResult),
		Entry("Volumes", annotations.Volumes),
		Entry("SandboxID", annotations.SandboxID),
		Entry("ContainerManager", annotations.ContainerManager),
		Entry("NetworkIsolated", annotations.NetworkIsolated),
	)
}
