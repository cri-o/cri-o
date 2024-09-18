package oci_test

import (
	"context"
	"math/rand"
	"os"
	"os/exec"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"go.uber.org/mock/gomock"
	kwait "k8s.io/apimachinery/pkg/util/wait"
	kclock "k8s.io/utils/clock"

	"github.com/cri-o/cri-o/internal/oci"
	libconfig "github.com/cri-o/cri-o/pkg/config"
	runnerMock "github.com/cri-o/cri-o/test/mocks/cmdrunner"
	"github.com/cri-o/cri-o/utils/cmdrunner"
)

const (
	shortTimeout  int64 = 1
	mediumTimeout int64 = 5
	longTimeout   int64 = 15
)

// The actual test suite.
var _ = t.Describe("Oci", func() {
	Context("StopContainer", func() {
		var (
			sut          *oci.Container
			sleepProcess *exec.Cmd
			runner       *runnerMock.MockCommandRunner
			runtime      oci.RuntimeOCI
			bm           kwait.BackoffManager
		)
		BeforeEach(func() {
			sleepProcess = exec.Command("sleep", "100000")
			Expect(sleepProcess.Start()).To(Succeed())

			Expect(sleepProcess.Process.Pid).NotTo(Equal(0))

			sut = getTestContainer()
			state := &oci.ContainerState{}
			state.Pid = sleepProcess.Process.Pid
			Expect(state.SetInitPid(sleepProcess.Process.Pid)).To(Succeed())
			sut.SetState(state)

			runner = runnerMock.NewMockCommandRunner(mockCtrl)
			cmdrunner.SetMocked(runner)

			cfg, err := libconfig.DefaultConfig()
			Expect(err).ToNot(HaveOccurred())
			cfg.ContainerAttachSocketDir = t.MustTempDir("attach-socket")
			r, err := oci.New(cfg)
			Expect(err).ToNot(HaveOccurred())
			runtime = oci.NewRuntimeOCI(r, &libconfig.RuntimeHandler{})
			bm = kwait.NewExponentialBackoffManager( //nolint:staticcheck
				1.0, // Initial backoff.
				10,  // Maximum backoff.
				10,  // Reset backoff.
				2.0, // Backoff factor.
				0.0, // Backoff jitter.
				kclock.RealClock{},
			)
		})
		AfterEach(func() {
			//nolint:errcheck
			oci.Kill(sleepProcess.Process.Pid)
			// make sure the entry in the process table is cleaned up
			//nolint:errcheck
			sleepProcess.Wait()
			cmdrunner.ResetPrependedCmd()
		})

		It("should return early if runtime command fails and process stopped", func() {
			// Given
			gomock.InOrder(
				runner.EXPECT().Command(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ string, _ ...string) interface{} {
						Expect(oci.Kill(sleepProcess.Process.Pid)).To(Succeed())
						waitForKillToComplete(sleepProcess)
						return exec.Command("/bin/false")
					},
				),
			)

			// When
			sut.SetAsStopping()
			go runtime.StopLoopForContainer(sut, bm)
			stoppedChan := stopTimeoutWithChannel(context.Background(), sut, shortTimeout)
			<-stoppedChan

			// Then
			Expect(sut.State().Finished).NotTo(BeZero())
			verifyContainerStopped(sut, sleepProcess)
		})
		It("should stop container before timeout", func() {
			// Given
			gomock.InOrder(
				runner.EXPECT().Command(gomock.Any(), gomock.Any()).DoAndReturn(
					func(_ string, _ ...string) interface{} {
						Expect(oci.Kill(sleepProcess.Process.Pid)).To(Succeed())
						waitForKillToComplete(sleepProcess)
						return exec.Command("/bin/true")
					},
				),
			)
			sut.SetAsStopping()
			go runtime.StopLoopForContainer(sut, bm)

			// Then
			waitOnContainerTimeout(sut, shortTimeout, mediumTimeout, sleepProcess)
		})
		It("should fall back to KILL after timeout", func() {
			// Given
			containerIgnoreSignalCmdrunnerMock(sleepProcess, runner)
			sut.SetAsStopping()
			go runtime.StopLoopForContainer(sut, bm)

			// Then
			waitOnContainerTimeout(sut, shortTimeout, mediumTimeout, sleepProcess)
		})
		It("should interrupt longer stop timeout", func() {
			// Given
			containerIgnoreSignalCmdrunnerMock(sleepProcess, runner)
			sut.SetAsStopping()
			go runtime.StopLoopForContainer(sut, bm)
			go sut.WaitOnStopTimeout(context.Background(), longTimeout)

			// Then
			waitOnContainerTimeout(sut, shortTimeout, mediumTimeout, sleepProcess)
		})

		It("should not update time if chronologically after", func() {
			// Given
			containerIgnoreSignalCmdrunnerMock(sleepProcess, runner)
			sut.SetAsStopping()
			go runtime.StopLoopForContainer(sut, bm)

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
			go runtime.StopLoopForContainer(sut, bm)
			// very long timeout
			stoppedChan := stopTimeoutWithChannel(context.Background(), sut, longTimeout*10)

			// When
			for range 10 {
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
			It(test.title, func() {
				fileName := t.MustTempFile("to-read")
				Expect(os.WriteFile(fileName, test.contents, 0o644)).To(Succeed())
				found, err := oci.TruncateAndReadFile(context.Background(), fileName, test.size)
				Expect(err).ToNot(HaveOccurred())
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
				Expect(oci.Kill(sleepProcess.Process.Pid)).To(Succeed())
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
	Expect(err).To(HaveOccurred())
}

func waitForKillToComplete(sleepProcess *exec.Cmd) {
	Expect(sleepProcess.Wait()).NotTo(Succeed())
	// this fixes a race with the kernel cleaning up the /proc entry
	// even adding a Kill() in the call to Pid() doesn't fix
	time.Sleep(inSeconds(shortTimeout))
}

func verifyContainerNotStopped(sut *oci.Container) {
	pid, err := sut.Pid()
	Expect(pid).NotTo(Equal(0))
	Expect(err).ToNot(HaveOccurred())
}

func inSeconds(d int64) time.Duration {
	return time.Duration(d) * time.Second
}
