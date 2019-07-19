package oci_test

import (
	"testing"
	"time"

	"github.com/cri-o/cri-o/pkg/oci"
	. "github.com/cri-o/cri-o/test/framework"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	pb "k8s.io/cri-api/pkg/apis/runtime/v1alpha2"
)

// TestOci runs the created specs
func TestOci(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "Public OCI")
}

var (
	t *TestFramework
)

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()
})

func getTestContainer() *oci.Container {
	container, err := oci.NewContainer("id", "name", "bundlePath", "logPath",
		"netns", map[string]string{"key": "label"},
		map[string]string{"key": "crioAnnotation"},
		map[string]string{"key": "annotation"},
		"image", "imageName", "imageRef", &pb.ContainerMetadata{}, "sandbox",
		false, false, false, false, "", "dir", time.Now(), "")
	Expect(err).To(BeNil())
	Expect(container).NotTo(BeNil())
	return container
}

var _ = AfterSuite(func() {
	t.Teardown()
})
