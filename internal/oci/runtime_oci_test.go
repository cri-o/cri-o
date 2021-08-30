package oci_test

import (
	"context"
	"math/rand"
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
	Context("ContainerStop", func() {
		var (
			sut          *oci.Container
			sleepProcess *exec.Cmd
		)

		BeforeEach(func() {
			sleepProcess = exec.Command("sleep", "100000")
			Expect(sleepProcess.Start()).To(BeNil())

			Expect(sleepProcess.Process.Pid).NotTo(Equal(0))

			sut = getTestContainer()
			state := &oci.ContainerState{}
			state.Pid = sleepProcess.Process.Pid
			Expect(state.SetInitPid(sleepProcess.Process.Pid)).To(BeNil())
			sut.SetState(state)
		})
		AfterEach(func() {
			// nolint:errcheck
			oci.Kill(sleepProcess.Process.Pid)
			// make sure the entry in the process table is cleaned up
			// nolint:errcheck
			sleepProcess.Wait()
		})
		tests := []struct {
			ignoreKill             bool
			verifyCorrectlyStopped func(*oci.Container, *exec.Cmd, error)
			name                   string
		}{
			{
				ignoreKill:             true,
				verifyCorrectlyStopped: verifyContainerNotStopped,
				name:                   "ignoring kill",
			},
			{
				ignoreKill:             false,
				verifyCorrectlyStopped: verifyContainerStopped,
				name:                   "not ignoring kill",
			},
		}
		for _, test := range tests {
			test := test
			It("should stop container after timeout if "+test.name, func() {
				// Given
				sut.SetAsStopping(shortTimeout)

				// When
				err := oci.WaitContainerStop(context.Background(), sut, inSeconds(shortTimeout), test.ignoreKill)

				// Then
				test.verifyCorrectlyStopped(sut, sleepProcess, err)
			})
			It("should interrupt longer stop timeout if "+test.name, func() {
				// Given
				stoppedChan := make(chan error, 1)
				sut.SetAsStopping(longTimeout)
				go waitContainerStopAndFailAfterTimeout(context.Background(), stoppedChan, sut, longTimeout, longTimeout, test.ignoreKill)

				// When
				sut.SetAsStopping(shortTimeout)

				// Then
				test.verifyCorrectlyStopped(sut, sleepProcess, <-stoppedChan)
			})
			It("should handle being killed mid-timeout if "+test.name, func() {
				// Given
				stoppedChan := make(chan error, 1)
				sut.SetAsStopping(longTimeout)
				go waitContainerStopAndFailAfterTimeout(context.Background(), stoppedChan, sut, longTimeout, mediumTimeout, test.ignoreKill)

				// When
				// nolint:errcheck
				oci.Kill(sleepProcess.Process.Pid)
				waitForKillToComplete(sleepProcess)

				// Then
				// unconditionally expect the container was stopped
				verifyContainerStopped(sut, sleepProcess, <-stoppedChan)
			})
			It("should handle context timeout if "+test.name, func() {
				// Given
				ctx, cancel := context.WithCancel(context.Background())
				stoppedChan := make(chan error, 1)
				sut.SetAsStopping(longTimeout)
				go waitContainerStopAndFailAfterTimeout(ctx, stoppedChan, sut, longTimeout, mediumTimeout, test.ignoreKill)

				// When
				cancel()

				// Then
				// unconditionally expect the container was not stopped
				verifyContainerNotStopped(sut, sleepProcess, <-stoppedChan)
			})
			It("should not update time if chronologically after if "+test.name, func() {
				// Given
				stoppedChan := make(chan error, 1)
				sut.SetAsStopping(mediumTimeout)
				go waitContainerStopAndFailAfterTimeout(context.Background(), stoppedChan, sut, mediumTimeout, mediumTimeout, test.ignoreKill)

				// When
				sut.SetAsStopping(longTimeout)

				// Then
				test.verifyCorrectlyStopped(sut, sleepProcess, <-stoppedChan)
			})
			It("should handle many updates if "+test.name, func() {
				// Given
				stoppedChan := make(chan error, 1)
				sut.SetAsStopping(longTimeout)
				go waitContainerStopAndFailAfterTimeout(context.Background(), stoppedChan, sut, longTimeout, longTimeout, test.ignoreKill)

				// When
				for i := 0; i < 5; i++ {
					go sut.SetAsStopping(int64(rand.Intn(10)))
				}

				// Then
				test.verifyCorrectlyStopped(sut, sleepProcess, <-stoppedChan)
			})
		}
	})
})

func waitContainerStopAndFailAfterTimeout(ctx context.Context,
	stoppedChan chan error,
	sut *oci.Container,
	waitContainerStopTimeout int64,
	failAfterTimeout int64,
	ignoreKill bool) {
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
