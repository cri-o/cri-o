package utils_test

import (
	"bytes"
	"os"
	"strings"

	"github.com/kubernetes-sigs/cri-o/utils"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

type errorReaderWriter struct {
}

func (m *errorReaderWriter) Write(p []byte) (n int, err error) {
	return 0, t.TestError
}

func (m *errorReaderWriter) Read(p []byte) (n int, err error) {
	return 0, t.TestError
}

// The actual test suite
var _ = t.Describe("Utils", func() {
	t.Describe("ExecCmd", func() {
		It("should succeed", func() {
			// Given
			// When
			res, err := utils.ExecCmd("ls")

			// Then
			Expect(err).To(BeNil())
			Expect(res).NotTo(BeEmpty())
		})

		It("should fail on wrong command", func() {
			// Given
			// When
			res, err := utils.ExecCmd("not-existing")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(res).To(BeEmpty())
		})
	})

	t.Describe("ExecCmdWithStdStreams", func() {
		It("should succeed", func() {
			// Given
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}

			// When
			err := utils.ExecCmdWithStdStreams(nil, stdout, stderr, "ls")

			// Then
			Expect(err).To(BeNil())
			Expect(stdout.String()).NotTo(BeEmpty())
			Expect(stderr.String()).To(BeEmpty())
		})

		It("should fail on wrong command", func() {
			// Given
			stdout := &bytes.Buffer{}
			stderr := &bytes.Buffer{}

			// When
			err := utils.ExecCmdWithStdStreams(nil, stdout, stderr, "not-existing")

			// Then
			Expect(err).NotTo(BeNil())
			Expect(stdout.String()).To(BeEmpty())
			Expect(stderr.String()).To(BeEmpty())
		})
	})

	t.Describe("StatusToExitCode", func() {
		It("should succeed", func() {
			// Given
			// When
			code := utils.StatusToExitCode(20000)

			// Then
			Expect(code).To(Equal(78))
		})
	})

	t.Describe("DetachError", func() {
		It("should return an error", func() {
			// Given
			err := &utils.DetachError{}

			// When
			str := err.Error()

			// Then
			Expect(str).To(Equal("detached from container"))
		})
	})

	t.Describe("CopyDetachable", func() {
		It("should succeed", func() {
			// Given
			reader := strings.NewReader("test")
			writer := &bytes.Buffer{}
			keys := []byte{}

			// When
			written, err := utils.CopyDetachable(writer, reader, keys)

			// Then
			Expect(err).To(BeNil())
			Expect(written).To(SatisfyAll(BeNumerically(">", 0)))
		})

		It("should succeed with keys", func() {
			// Given
			reader := strings.NewReader("x")
			writer := &bytes.Buffer{}
			keys := []byte("xe")

			// When
			written, err := utils.CopyDetachable(writer, reader, keys)

			// Then
			Expect(err).To(BeNil())
			Expect(written).To(SatisfyAll(BeNumerically(">", 0)))
		})

		It("should fail with nil reader/writer", func() {
			// Given
			// When
			written, err := utils.CopyDetachable(nil, nil, nil)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(written).To(BeEquivalentTo(0))
		})

		It("should fail with error reader", func() {
			// Given
			reader := &errorReaderWriter{}
			writer := &bytes.Buffer{}
			keys := []byte{}

			// When
			written, err := utils.CopyDetachable(writer, reader, keys)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(written).To(BeEquivalentTo(0))
		})

		It("should fail with error writer", func() {
			// Given
			reader := strings.NewReader("x")
			writer := &errorReaderWriter{}
			keys := []byte{}

			// When
			written, err := utils.CopyDetachable(writer, reader, keys)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(written).To(BeEquivalentTo(0))
		})

		It("should fail on detach error", func() {
			// Given
			reader := strings.NewReader("x")
			writer := &bytes.Buffer{}
			keys := []byte("x")

			// When
			written, err := utils.CopyDetachable(writer, reader, keys)

			// Then
			Expect(err).NotTo(BeNil())
			Expect(written).To(BeEquivalentTo(0))
		})
	})

	t.Describe("WriteGoroutineStacksToFile", func() {
		It("should succeed", func() {
			// Given
			const testFile = "testFile"

			// When
			err := utils.WriteGoroutineStacksToFile(testFile)

			// Then
			Expect(err).To(BeNil())
			os.Remove(testFile)
		})

		It("should fail on invalid file path", func() {
			// Given

			// When
			err := utils.WriteGoroutineStacksToFile("")

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("WriteGoroutineStacks", func() {
		It("should succeed", func() {
			// Given
			writer := &bytes.Buffer{}

			// When
			err := utils.WriteGoroutineStacks(writer)

			// Then
			Expect(err).To(BeNil())
		})

		It("should fail on invalid writer", func() {
			// Given
			writer := &errorReaderWriter{}

			// When
			err := utils.WriteGoroutineStacks(writer)

			// Then
			Expect(err).NotTo(BeNil())
		})

		It("should fail with nil reader/writer", func() {
			// Given
			// When
			err := utils.WriteGoroutineStacks(nil)

			// Then
			Expect(err).NotTo(BeNil())
		})
	})

	t.Describe("RunUnderSystemdScope", func() {
		It("should fail unauthenticated", func() {
			// Given
			// When
			err := utils.RunUnderSystemdScope(1, "", "")

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
