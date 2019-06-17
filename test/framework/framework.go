package framework

import (
	"fmt"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/onsi/ginkgo"
	"github.com/onsi/ginkgo/reporters"
	"github.com/onsi/gomega"
)

// TestFramework is used to support commonnly used test features
type TestFramework struct {
	setup     func(*TestFramework) error
	teardown  func(*TestFramework) error
	TestError error

	tempDirs  []string
	tempFiles []string
}

// NewTestFramework creates a new test framework instance for a given `setup`
// and `teardown` function
func NewTestFramework(setup, teardown func(*TestFramework) error) *TestFramework {
	return &TestFramework{
		setup,
		teardown,
		fmt.Errorf("error"),
		nil,
		nil,
	}
}

// NilFunc is a convenience function which simply does nothing
func NilFunc(f *TestFramework) error {
	return nil
}

// Setup is the global initialization function which runs before each test
// suite
func (t *TestFramework) Setup() {
	// Global initialization for the whole framework goes in here

	// Setup the actual test suite
	gomega.Expect(t.setup(t)).To(gomega.Succeed())
}

// Teardown is the global deinitialization function which runs after each test
// suite
func (t *TestFramework) Teardown() {
	// Global deinitialization for the whole framework goes in here

	// Teardown the actual test suite
	gomega.Expect(t.teardown(t)).To(gomega.Succeed())

	// Clean up any temporary directories and files the test suite created.
	for _, d := range t.tempDirs {
		os.RemoveAll(d)
	}
	for _, d := range t.tempFiles {
		os.RemoveAll(d)
	}
}

// Describe is a convenience wrapper around the `ginkgo.Describe` function
func (t *TestFramework) Describe(text string, body func()) bool {
	return ginkgo.Describe("cri-o: "+text, body)
}

// MustTempDir uses ioutil.TempDir to create a temporary directory
// with the given prefix.  It panics on any error.
func (t *TestFramework) MustTempDir(prefix string) string {
	path, err := ioutil.TempDir("", prefix)
	if err != nil {
		panic(err)
	}

	t.tempDirs = append(t.tempDirs, path)
	return path
}

// MustTempFile uses ioutil.TempFile to create a temporary file
// with the given pattern.  It panics on any error.
func (t *TestFramework) MustTempFile(pattern string) string {
	path, err := ioutil.TempFile("", pattern)
	if err != nil {
		panic(err)
	}

	t.tempFiles = append(t.tempFiles, path.Name())
	return path.Name()
}

// RunFrameworkSpecs is a convenience wrapper for running tests
func RunFrameworkSpecs(t *testing.T, suiteName string) {
	ginkgo.RunSpecsWithDefaultAndCustomReporters(t, suiteName,
		[]ginkgo.Reporter{reporters.NewJUnitReporter(
			fmt.Sprintf("%v_junit.xml", strings.ToLower(suiteName)))})
}
