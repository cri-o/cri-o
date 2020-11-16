package lib_test

import (
	"io/ioutil"
	"os"
	"path"
	"time"

	"github.com/cri-o/cri-o/v1/lib"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
)

// The actual test suite
var _ = t.Describe("ContainerServer", func() {
	// Prepare the sut
	BeforeEach(beforeEach)

	t.Describe("GetLogs", func() {
		It("should succeed", func() {
			// Given
			c := make(chan string)
			e := make(chan error)

			// Prepare the container
			addContainerAndSandbox()

			// Prepare the log file
			logString := []byte("2007-01-02T15:04:05.0-07:00 Log\n" +
				"1996-01-02T15:04:05.0-07:00 Before\n" +
				"WRONG_DATE Invalid\n" +
				"WRONG\n")
			logFile := path.Join(sandboxID, containerID+".log")
			Expect(os.MkdirAll(sandboxID, 0o755)).To(BeNil())
			Expect(ioutil.WriteFile(logFile, logString, 0o644)).To(BeNil())
			defer os.RemoveAll(sandboxID)

			// When
			go func() {
				e <- sut.GetLogs(containerID, c, lib.LogOptions{
					SinceTime: time.Date(2000, 0, 0, 0, 0, 0, 0, time.UTC),
				})
			}()

			// Then
			Expect(<-c).To(ContainSubstring("Log"))
			Expect(<-e).To(BeNil())
		})

		It("should succeed with seek info", func() {
			// Given
			c := make(chan string)
			addContainerAndSandbox()

			// When
			err := sut.GetLogs(containerID, c, lib.LogOptions{Tail: 1})

			// Then
			Expect(<-c).To(BeEmpty())
			Expect(err).To(BeNil())
		})

		It("should fail on invalid container ID", func() {
			// Given

			// When
			err := sut.GetLogs("", make(chan string), lib.LogOptions{})

			// Then
			Expect(err).NotTo(BeNil())
		})
	})
})
