package oci_test

import (
	"context"
	"math/rand"
	"os"
	"os/exec"
	"time"

	"github.com/cri-o/cri-o/internal/oci"
	libconfig "github.com/cri-o/cri-o/pkg/config"
	runnerMock "github.com/cri-o/cri-o/test/mocks/cmdrunner"
	"github.com/cri-o/cri-o/utils/cmdrunner"
	"github.com/golang/mock/gomock"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	shortTimeout  int64 = 1
	mediumTimeout int64 = 5
	longTimeout   int64 = 15
)

// The actual test suite
var _ = t.Describe("Oci", func() {
	Context("StopContainer", func() {
		var (
			sut          *oci.Container
			sleepProcess *exec.Cmd
			runner       *runnerMock.MockCommandRunner
			runtime      oci.RuntimeOCI
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

			runner = runnerMock.NewMockCommandRunner(mockCtrl)
			cmdrunner.SetMocked(runner)

			cfg, err := libconfig.DefaultConfig()
			Expect(err).To(BeNil())
			r, err := oci.New(cfg)
			Expect(err).To(BeNil())
			runtime = oci.NewRuntimeOCI(r, &libconfig.RuntimeHandler{})
		})
		AfterEach(func() {
			// nolint:errcheck
			oci.Kill(sleepProcess.Process.Pid)
			// make sure the entry in the process table is cleaned up
			// nolint:errcheck
			sleepProcess.Wait()
			cmdrunner.ResetPrependedCmd()
		})

		It("should fail to stop if container paused", func() {
			state := &oci.ContainerState{}
			state.Status = oci.ContainerStatePaused
			sut.SetState(state)

			Expect(sut.ShouldBeStopped()).NotTo(BeNil())
		})
		It("should fail to stop if container stopped", func() {
			state := &oci.ContainerState{}
			state.Status = oci.ContainerStateStopped
			sut.SetState(state)

			Expect(sut.ShouldBeStopped()).To(Equal(oci.ErrContainerStopped))
		})
		It("should return early if runtime command fails and process stopped", func() {
			// Given
			gomock.InOrder(
				runner.EXPECT().Command(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ string, _ ...string) interface{} {
						Expect(oci.Kill(sleepProcess.Process.Pid)).To(BeNil())
						waitForKillToComplete(sleepProcess)
						return exec.Command("/bin/false")
					},
				),
			)

			// When
			sut.SetAsStopping()
			runtime.StopLoopForContainer(sut)

			// Then
			Expect(sut.State().Finished).NotTo(BeZero())
			verifyContainerStopped(sut, sleepProcess)
		})
		It("should stop container before timeout", func() {
			// Given
			gomock.InOrder(
				runner.EXPECT().Command(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ string, _ ...string) interface{} {
						Expect(oci.Kill(sleepProcess.Process.Pid)).To(BeNil())
						waitForKillToComplete(sleepProcess)
						return exec.Command("/bin/true")
					},
				),
			)
			sut.SetAsStopping()
			go runtime.StopLoopForContainer(sut)

			// Then
			waitOnContainerTimeout(sut, longTimeout, mediumTimeout, sleepProcess)
		})
		It("should fall back to KILL after timeout", func() {
			// Given
			containerIgnoreSignalCmdrunnerMock(sleepProcess, runner)
			sut.SetAsStopping()
			go runtime.StopLoopForContainer(sut)

			// Then
			waitOnContainerTimeout(sut, shortTimeout, mediumTimeout, sleepProcess)
		})
		It("should interrupt longer stop timeout", func() {
			// Given
			containerIgnoreSignalCmdrunnerMock(sleepProcess, runner)
			sut.SetAsStopping()
			go runtime.StopLoopForContainer(sut)
			go sut.WaitOnStopTimeout(context.Background(), longTimeout)

			// Then
			waitOnContainerTimeout(sut, shortTimeout, mediumTimeout, sleepProcess)
		})

		It("should not update time if chronologically after", func() {
			// Given
			containerIgnoreSignalCmdrunnerMock(sleepProcess, runner)
			sut.SetAsStopping()
			go runtime.StopLoopForContainer(sut)

			// When
			shortStopChan := stopTimeoutWithChannel(context.Background(), sut, shortTimeout)

			// Then
			waitOnContainerTimeout(sut, mediumTimeout, longTimeout, sleepProcess)
			<-shortStopChan
		})
		It("should handle many updates", func() {
			// Given
			containerIgnoreSignalCmdrunnerMock(sleepProcess, runner)
			sut.SetAsStopping()
			go runtime.StopLoopForContainer(sut)
			// very long timeout
			stoppedChan := stopTimeoutWithChannel(context.Background(), sut, longTimeout*10)

			// When
			for i := 0; i < 10; i++ {
				go sut.WaitOnStopTimeout(context.Background(), int64(rand.Intn(100)+20))
				time.Sleep(time.Second)
			}
			sut.WaitOnStopTimeout(context.Background(), mediumTimeout)

			// Then
			<-stoppedChan
			verifyContainerStopped(sut, sleepProcess)
		})
		It("should handle context timeout", func() {
			// Given
			ctx, cancel := context.WithCancel(context.Background())
			stoppedChan := stopTimeoutWithChannel(ctx, sut, shortTimeout)

			// When
			cancel()

			// Then
			// unconditionally expect the container was not stopped
			<-stoppedChan
			verifyContainerNotStopped(sut)
		})
	})
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
				Expect(os.WriteFile(fileName, test.contents, 0o644)).To(BeNil())
				found, err := oci.TruncateAndReadFile(context.Background(), fileName, test.size)
				Expect(err).To(BeNil())
				Expect(found).To(Equal(test.expected))
			})
		}
	})
})

func containerIgnoreSignalCmdrunnerMock(sleepProcess *exec.Cmd, runner *runnerMock.MockCommandRunner) {
	gomock.InOrder(
		runner.EXPECT().Command(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ string, _ ...string) interface{} {
				return exec.Command("/bin/true")
			},
		),
		runner.EXPECT().Command(gomock.Any(), gomock.Any()).DoAndReturn(
			func(_ string, _ ...string) interface{} {
				Expect(oci.Kill(sleepProcess.Process.Pid)).To(BeNil())
				waitForKillToComplete(sleepProcess)
				return exec.Command("/bin/true")
			},
		),
	)
}

func waitOnContainerTimeout(sut *oci.Container, stopTimeout, waitTimeout int64, sleepProcess *exec.Cmd) {
	stoppedChan := stopTimeoutWithChannel(context.Background(), sut, stopTimeout)

	select {
	case <-stoppedChan:
	case <-time.After(time.Second * time.Duration(waitTimeout)):
		Fail("did not timeout quickly enough")
	}
	verifyContainerStopped(sut, sleepProcess)
}

func stopTimeoutWithChannel(ctx context.Context, sut *oci.Container, timeout int64) chan struct{} {
	stoppedChan := make(chan struct{}, 1)
	go func() {
		sut.WaitOnStopTimeout(ctx, timeout)
		close(stoppedChan)
	}()
	return stoppedChan
}

func verifyContainerStopped(sut *oci.Container, sleepProcess *exec.Cmd) {
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

func verifyContainerNotStopped(sut *oci.Container) {
	pid, err := sut.Pid()
	Expect(pid).NotTo(Equal(0))
	Expect(err).To(BeNil())
}

func inSeconds(d int64) time.Duration {
	return time.Duration(d) * time.Second
}
