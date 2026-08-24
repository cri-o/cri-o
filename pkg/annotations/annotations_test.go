package annotations_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/pkg/annotations"
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
		Entry("ShmPath", "io.kubernetes.cri-o.ShmPath"),
		Entry("ContainerID", "io.kubernetes.cri-o.ContainerID"),
		Entry("MountPoint", "io.kubernetes.cri-o.MountPoint"),
		Entry("CNIResult", "io.kubernetes.cri-o.CNIResult"),
		Entry("Volumes", "io.kubernetes.cri-o.Volumes"),
		Entry("SandboxID", "io.kubernetes.cri-o.SandboxID"),
		Entry("ContainerManager", "io.container.manager"),
		Entry("indexed IP.0", "io.kubernetes.cri-o.IP.0"),
		Entry("indexed IP.1", "io.kubernetes.cri-o.IP.1"),
	)

	DescribeTable("should return false for non-internal annotations",
		func(key string) {
			Expect(annotations.IsInternal(key)).To(BeFalse())
		},
		Entry("V1 ShmSize", "io.kubernetes.cri-o.ShmSize"),
		Entry("V1 Devices", "io.kubernetes.cri-o.Devices"),
		Entry("V1 UnifiedCgroup", "io.kubernetes.cri-o.UnifiedCgroup"),
		Entry("V1 container-specific with slash", "io.kubernetes.cri-o.Devices/container1"),
		Entry("V1 container-specific with dot", "io.kubernetes.cri-o.ShmSize.something"),
		Entry("unknown key under prefix", "io.kubernetes.cri-o.FutureField"),
		Entry("V1 prefix without separator", "io.kubernetes.cri-o.ShmSizeEvil"),
		Entry("unrelated annotation", "kubectl.kubernetes.io/last-applied-configuration"),
		Entry("workload annotation", "target.workload.openshift.io/management"),
		Entry("V1 SeccompProfile (reverse-domain prefix)", "seccomp-profile.kubernetes.cri-o.io"),
		Entry("empty string", ""),
	)
})
