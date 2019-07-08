package lib_test

import (
	"io/ioutil"
	"os"

	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("ContainerServer", func() {
	// Prepare the sut
	BeforeEach(beforeEach)

	const configFile = "config.json"

	t.Describe("ContainerRename", func() {
		It("should succeed", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			addContainerAndSandbox()
			Expect(ioutil.WriteFile(configFile, []byte("{}"), 0644)).To(BeNil())
			defer os.RemoveAll(configFile)
			gomock.InOrder(
				storeMock.EXPECT().SetNames(gomock.Any(), gomock.Any()).
					Return(nil),
			)

			// When
			err := sut.ContainerRename(containerID, "newID")

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail when set names errors", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			addContainerAndSandbox()
			Expect(ioutil.WriteFile(configFile, []byte("{}"), 0644)).To(BeNil())
			defer os.RemoveAll(configFile)
			gomock.InOrder(
				storeMock.EXPECT().SetNames(gomock.Any(), gomock.Any()).
					Return(t.TestError),
			)

			// When
			err := sut.ContainerRename(containerID, "newID")

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail when config not available", func() {
			// Given
			sut.SetRuntime(ociRuntimeMock)
			addContainerAndSandbox()

			// When
			err := sut.ContainerRename(containerID, "newID")

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail on invalid container ID", func() {
			// Given
			// When
			err := sut.ContainerRename("", "")

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
