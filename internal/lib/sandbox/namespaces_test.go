package sandbox_test

import (
	"os"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("SandboxManagedNamespaces", func() {
	// Setup the SUT
	BeforeEach(beforeEach)
	t.Describe("CreateSandboxNamespaces", func() {
		It("should succeed if empty", func() {
			// Given
			managedNamespaces := make([]string, 0)

			// When
			ns, err := testSandbox.CreateManagedNamespaces(managedNamespaces, "pinns")

			// Then
			Expect(err).To(BeNil())
			Expect(len(ns)).To(Equal(0))
		})

		It("should fail on invalid namespace", func() {
			// Given
			managedNamespaces := []string{"invalid"}

			// When
			_, err := testSandbox.CreateManagedNamespaces(managedNamespaces, "pinns")

			// Then
			Expect(err).To(Not(BeNil()))
		})
	})
	t.Describe("RemoveManagedNamespaces", func() {
		It("should succeed when namespaces nil", func() {
			// Given
			// When
			err := testSandbox.RemoveManagedNamespaces()

			// Then
			Expect(err).To(BeNil())
		})
		It("should succeed when ipc not nil", func() {
			// Given
			tmpFile, tmpDir := createTmpFileInTmpDir()

			gomock.InOrder(
				namespaceIfaceMock.EXPECT().Path().Return(tmpFile),
				namespaceIfaceMock.EXPECT().Remove().Return(nil),
			)

			testSandbox.IpcNsSet(namespaceIfaceMock)

			// When
			err := testSandbox.RemoveManagedNamespaces()

			// Then
			Expect(err).To(BeNil())
			_, err = os.Stat(tmpFile)
			Expect(os.IsNotExist(err)).To(Equal(true))

			_, err = os.Stat(tmpDir)
			Expect(os.IsNotExist(err)).To(Equal(true))
		})
	})
})
