package oci_test

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	types "k8s.io/cri-api/pkg/apis/runtime/v1"

	"github.com/cri-o/cri-o/internal/oci"
	libconfig "github.com/cri-o/cri-o/pkg/config"
)

type discardWriteCloser struct{ io.Writer }

func (discardWriteCloser) Close() error { return nil }

type attachWorkerState struct {
	workers int
	senders int
}

// attachWorkers finds AttachContainer's result workers by function name rather
// than closure number, which shifts when unrelated literals are added.
func attachWorkers() attachWorkerState {
	bufSize := 1 << 20

	for {
		buf := make([]byte, bufSize)

		n := runtime.Stack(buf, true)
		if n == len(buf) {
			bufSize *= 2

			continue
		}

		state := attachWorkerState{}

		for stack := range strings.SplitSeq(string(buf[:n]), "\n\n") {
			if !strings.Contains(stack, "AttachContainer.func") {
				continue
			}

			state.workers++
			if strings.Contains(stack, "chan send") {
				state.senders++
			}
		}

		return state
	}
}

func expectAttachWorkersExit(before attachWorkerState) {
	GinkgoHelper()

	deadline := time.Now().Add(5 * time.Second)

	for {
		current := attachWorkers()
		if current.senders > before.senders {
			Fail(fmt.Sprintf("AttachContainer stranded %d sender goroutine(s)", current.senders-before.senders))
		}

		if current.workers <= before.workers {
			return
		}

		if time.Now().After(deadline) {
			Fail(fmt.Sprintf("AttachContainer left %d worker goroutine(s) running", current.workers-before.workers))
		}

		time.Sleep(10 * time.Millisecond)
	}
}

func newAttachTestFixture() (*net.UnixListener, oci.RuntimeOCI, *oci.Container) {
	GinkgoHelper()

	sockDir := t.MustTempDir("attach-socket")
	bundleDir := t.MustTempDir("attach-bundle")

	Expect(os.WriteFile(filepath.Join(bundleDir, "ctl"), nil, 0o600)).To(Succeed())

	const testContainerID = "attach-leak-test"

	Expect(os.MkdirAll(filepath.Join(sockDir, testContainerID), 0o750)).To(Succeed())

	listener, err := net.ListenUnix("unixpacket", &net.UnixAddr{
		Name: filepath.Join(sockDir, testContainerID, "attach"),
		Net:  "unixpacket",
	})
	Expect(err).NotTo(HaveOccurred())
	DeferCleanup(func() {
		Expect(listener.Close()).To(Succeed())
	})

	cfg, err := libconfig.DefaultConfig()
	Expect(err).NotTo(HaveOccurred())

	cfg.ContainerAttachSocketDir = sockDir

	r, err := oci.New(cfg)
	Expect(err).NotTo(HaveOccurred())

	container, err := oci.NewContainer(testContainerID, "name", bundleDir, "logPath",
		map[string]string{}, map[string]string{}, map[string]string{},
		"image", nil, nil, "", &types.ContainerMetadata{}, "sandbox",
		false, true, false, "", t.MustTempDir("attach-dir"), time.Now(), "")
	Expect(err).NotTo(HaveOccurred())

	return listener, oci.NewRuntimeOCI(r, &libconfig.RuntimeHandler{}), container
}

func runAttach(runtimeOCI oci.RuntimeOCI, container *oci.Container, input io.Reader) <-chan error {
	done := make(chan error, 1)

	go func() {
		done <- runtimeOCI.AttachContainer(context.Background(), container, input,
			discardWriteCloser{io.Discard}, discardWriteCloser{io.Discard}, false, nil)
	}()

	return done
}

func drainAttachPeer(listener *net.UnixListener) <-chan error {
	done := make(chan error, 1)

	go func() {
		conn, err := listener.AcceptUnix()
		if err != nil {
			done <- err

			return
		}
		defer conn.Close()

		var buf [512]byte
		for {
			if _, err := conn.Read(buf[:]); err != nil {
				done <- nil

				return
			}
		}
	}()

	return done
}

func closeAttachPeer(listener *net.UnixListener) <-chan error {
	done := make(chan error, 1)

	go func() {
		conn, err := listener.AcceptUnix()
		if err != nil {
			done <- err

			return
		}

		done <- conn.Close()
	}()

	return done
}

var _ = t.Describe("Oci", func() {
	Context("AttachContainer result workers", func() {
		It("does not strand the stdout sender when stdin completes first", func() {
			listener, runtimeOCI, container := newAttachTestFixture()
			before := attachWorkers()
			peerDone := drainAttachPeer(listener)
			attachDone := runAttach(runtimeOCI, container, nil)

			var attachErr error
			Eventually(attachDone, 30*time.Second).Should(Receive(&attachErr))
			Expect(attachErr).NotTo(HaveOccurred())
			Eventually(peerDone, 5*time.Second).Should(Receive(Succeed()))
			expectAttachWorkersExit(before)
		})

		It("does not strand the stdin sender when stdout completes first", func() {
			listener, runtimeOCI, container := newAttachTestFixture()
			before := attachWorkers()
			inputReader, inputWriter := io.Pipe()

			DeferCleanup(func() {
				_ = inputReader.Close()
				_ = inputWriter.Close()
			})

			peerDone := closeAttachPeer(listener)
			attachDone := runAttach(runtimeOCI, container, inputReader)

			Eventually(peerDone, 5*time.Second).Should(Receive(Succeed()))
			Eventually(attachDone, 30*time.Second).Should(Receive())
			Expect(inputWriter.Close()).To(Succeed())
			expectAttachWorkersExit(before)
		})
	})
})
