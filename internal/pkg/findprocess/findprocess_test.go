package findprocess_test

import (
	"os/exec"
	"testing"

	"github.com/cri-o/cri-o/internal/pkg/findprocess"
	. "github.com/cri-o/cri-o/test/framework"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// TestFindprocess runs the created specs
func TestFindprocess(t *testing.T) {
	RegisterFailHandler(Fail)
	RunFrameworkSpecs(t, "Findprocess")
}

var t *TestFramework

var _ = BeforeSuite(func() {
	t = NewTestFramework(NilFunc, NilFunc)
	t.Setup()
})

var _ = AfterSuite(func() {
	t.Teardown()
})

// The actual test suite
var _ = t.Describe("Findprocess", func() {
	It("should succeed to find an existing process", func() {
		// Given
		cmd := exec.Command("sleep", "1")
		Expect(cmd.Start()).To(BeNil())

		// When
		process, err := findprocess.FindProcess(cmd.Process.Pid)

		// Then
		Expect(err).To(BeNil())
		Expect(process).NotTo(BeNil())
	})

	It("should fail to find an already released process", func() {
		// Given
		// When
		process, err := findprocess.FindProcess(-1)

		// Then
		Expect(err).NotTo(BeNil())
		Expect(err.Error()).To(ContainSubstring("process already released"))
		Expect(process).To(BeNil())
	})

	It("should fail to find an already finished process", func() {
		// Given
		cmd := exec.Command("echo")
		Expect(cmd.Start()).To(BeNil())
		Expect(cmd.Wait()).To(BeNil())

		// When
		process, err := findprocess.FindProcess(cmd.Process.Pid)

		// Then
		Expect(err).NotTo(BeNil())
		Expect(err).To(Equal(findprocess.ErrNotFound))
		Expect(process).To(BeNil())
	})
})
