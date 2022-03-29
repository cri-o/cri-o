package storage_test

import (
	"testing"

	. "github.com/cri-o/cri-o/test/framework"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// TestStorage runs the created specs
func TestStorage(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "Storage")
}

var (
	t            *TestFramework
	testManifest []byte
)

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()

	// Setup test data
	testManifest = []byte(`{"schemaVersion": 1,"fsLayers":[{"blobSum": ""}],` +
		`"history": [{"v1Compatibility": "{\"id\":\"e45a5af57b00862e5ef57` +
		`82a9925979a02ba2b12dff832fd0991335f4a11e5c5\",\"parent\":\"\"}\n"}]}`)
})

var _ = AfterSuite(func() {
	t.Teardown()
})
