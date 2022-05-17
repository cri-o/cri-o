package oci_test

import (
	"context"
	"os"
	"os/exec"
	"time"

	"github.com/cri-o/cri-o/internal/oci"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"github.com/pkg/errors"
)

const (
	shortTimeout  int64 = 1
	mediumTimeout int64 = 3
	longTimeout   int64 = 15
)

// The actual test suite
var _ = t.Describe("Oci", func() {
	Context("TruncateAndReadFile", func() {
		tests := []struct {
			title    string
			contents []byte
			expected []byte
			fail     bool
			size     int64
		}{
			{
				title:    "should read file if size is smaller than limit",
				contents: []byte("abcd"),
				expected: []byte("abcd"),
				size:     5,
			},
			{
				title:    "should read only size if size is same as limit",
				contents: []byte("abcd"),
				expected: []byte("abcd"),
				size:     4,
			},
			{
				title:    "should read only size if size is larger than limit",
				contents: []byte("abcd"),
				expected: []byte("abc"),
				size:     3,
			},
		}
		for _, test := range tests {
			test := test
			It(test.title, func() {
				fileName := t.MustTempFile("to-read")
				Expect(os.WriteFile(fileName, test.contents, 0644)).To(BeNil())
				found, err := oci.TruncateAndReadFile(context.Background(), fileName, test.size)
				Expect(err).To(BeNil())
				Expect(found).To(Equal(test.expected))
			})
		}
	})
})

func waitContainerStopAndFailAfterTimeout(ctx context.Context,
	stoppedChan chan error,
	sut *oci.Container,
	waitContainerStopTimeout int64,
	failAfterTimeout int64,
	ignoreKill bool,
) {
	select {
	case stoppedChan <- oci.WaitContainerStop(ctx, sut, inSeconds(waitContainerStopTimeout), ignoreKill):
	case <-time.After(inSeconds(failAfterTimeout)):
		stoppedChan <- errors.Errorf("%d seconds passed, container kill should have been recognized", failAfterTimeout)
	}
	close(stoppedChan)
}

func verifyContainerStopped(sut *oci.Container, sleepProcess *exec.Cmd, waitError error) {
	Expect(waitError).To(BeNil())
	waitForKillToComplete(sleepProcess)
	pid, err := sut.Pid()
	Expect(pid).To(Equal(0))
	Expect(err).NotTo(BeNil())
}

func waitForKillToComplete(sleepProcess *exec.Cmd) {
	Expect(sleepProcess.Wait()).NotTo(BeNil())
	// this fixes a race with the kernel cleaning up the /proc entry
	// even adding a Kill() in the call to Pid() doesn't fix
	time.Sleep(inSeconds(shortTimeout))
}

func verifyContainerNotStopped(sut *oci.Container, _ *exec.Cmd, waitError error) {
	Expect(waitError).NotTo(BeNil())
	pid, err := sut.Pid()
	Expect(pid).NotTo(Equal(0))
	Expect(err).To(BeNil())
}

func inSeconds(d int64) time.Duration {
	return time.Duration(d) * time.Second
}
