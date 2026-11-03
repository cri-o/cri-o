package shimio_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"sync"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/cri-o/cri-o/internal/shimio"
)

// syncBuffer is a goroutine safe write closer recording what was written.
type syncBuffer struct {
	mu     sync.Mutex
	buf    bytes.Buffer
	closed bool
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.Write(p)
}

func (b *syncBuffer) Close() error {
	b.mu.Lock()
	defer b.mu.Unlock()

	b.closed = true

	return nil
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.buf.String()
}

func (b *syncBuffer) Closed() bool {
	b.mu.Lock()
	defer b.mu.Unlock()

	return b.closed
}

// writeFIFO acts as the shim side and writes data into a FIFO.
func writeFIFO(path, data string) {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	Expect(err).NotTo(HaveOccurred())
	_, err = f.WriteString(data)
	Expect(err).NotTo(HaveOccurred())
	Expect(f.Close()).To(Succeed())
}

var _ = Describe("FIFOSet", func() {
	It("should create FIFO paths in a temporary directory and remove it on close", func() {
		root := GinkgoT().TempDir()

		fifos, err := shimio.NewFIFOSetInDir(root, "id", true)
		Expect(err).NotTo(HaveOccurred())
		Expect(fifos.Terminal).To(BeTrue())

		dir := filepath.Dir(fifos.Stdin)
		Expect(filepath.Dir(dir)).To(Equal(root))
		Expect(fifos.Stdin).To(Equal(filepath.Join(dir, "id-stdin")))
		Expect(fifos.Stdout).To(Equal(filepath.Join(dir, "id-stdout")))
		Expect(fifos.Stderr).To(Equal(filepath.Join(dir, "id-stderr")))
		Expect(dir).To(BeADirectory())

		Expect(fifos.Close()).To(Succeed())
		Expect(dir).NotTo(BeAnExistingFile())
	})

	It("should call the provided close function", func() {
		called := false
		fifos := shimio.NewFIFOSet(shimio.Config{}, func() error {
			called = true

			return nil
		})

		Expect(fifos.Close()).To(Succeed())
		Expect(called).To(BeTrue())
	})
})

var _ = Describe("ContainerIO", func() {
	It("should fail without FIFOs", func() {
		_, err := shimio.NewContainerIO("id")
		Expect(err).To(HaveOccurred())
	})

	It("should pipe container output to added outputs", func() {
		cio, err := shimio.NewContainerIO(
			"id",
			shimio.WithNewFIFOs(GinkgoT().TempDir(), false, false),
		)
		Expect(err).NotTo(HaveOccurred())

		cfg := cio.Config()
		Expect(cfg.Stdin).To(BeEmpty())
		Expect(cfg.Stdout).To(BeAnExistingFile())
		Expect(cfg.Stderr).To(BeAnExistingFile())

		stdout, stderr := &syncBuffer{}, &syncBuffer{}
		oldStdout, oldStderr := cio.AddOutput("log", stdout, stderr)
		Expect(oldStdout).To(BeNil())
		Expect(oldStderr).To(BeNil())

		cio.Pipe()
		writeFIFO(cfg.Stdout, "out")
		writeFIFO(cfg.Stderr, "err")
		cio.Wait()

		Expect(stdout.String()).To(Equal("out"))
		Expect(stderr.String()).To(Equal("err"))
		Expect(stdout.Closed()).To(BeTrue())
		Expect(stderr.Closed()).To(BeTrue())
		Expect(cio.Close()).To(Succeed())
		Expect(filepath.Dir(cfg.Stdout)).NotTo(BeAnExistingFile())
	})

	It("should return the replaced outputs", func() {
		cio, err := shimio.NewContainerIO(
			"id",
			shimio.WithNewFIFOs(GinkgoT().TempDir(), true, false),
		)
		Expect(err).NotTo(HaveOccurred())

		defer cio.Close()

		first := &syncBuffer{}
		cio.AddOutput("log", first, nil)

		oldStdout, oldStderr := cio.AddOutput("log", &syncBuffer{}, nil)
		Expect(oldStdout).To(BeIdenticalTo(first))
		Expect(oldStderr).To(BeNil())
	})

	It("should attach to container output until the context is done", func() {
		cio, err := shimio.NewContainerIO(
			"id",
			shimio.WithNewFIFOs(GinkgoT().TempDir(), false, false),
		)
		Expect(err).NotTo(HaveOccurred())

		defer cio.Close()

		// Keep the writer group non-empty, otherwise piping stops on the
		// first write.
		logOutput := &syncBuffer{}
		cio.AddOutput("log", logOutput, nil)
		cio.Pipe()

		shim, err := os.OpenFile(cio.Config().Stdout, os.O_WRONLY, 0)
		Expect(err).NotTo(HaveOccurred())

		defer shim.Close()

		ctx, cancel := context.WithCancel(context.Background())
		stdout := &syncBuffer{}
		done := make(chan struct{})

		go func() {
			defer GinkgoRecover()

			cio.Attach(ctx, shimio.AttachOptions{Stdout: stdout})
			close(done)
		}()

		Eventually(func() string {
			_, err := shim.WriteString("x")
			Expect(err).NotTo(HaveOccurred())

			return stdout.String()
		}).Should(ContainSubstring("x"))

		cancel()
		Eventually(done).Should(BeClosed())
		Expect(stdout.Closed()).To(BeTrue())
		Expect(logOutput.Closed()).To(BeFalse())
	})
})

var _ = Describe("ExecIO", func() {
	It("should pipe exec output to the attached streams", func() {
		eio, err := shimio.NewExecIO("id", GinkgoT().TempDir(), false, false)
		Expect(err).NotTo(HaveOccurred())

		cfg := eio.Config()
		Expect(cfg.Stdin).To(BeEmpty())

		stdout, stderr := &syncBuffer{}, &syncBuffer{}
		done := eio.Attach(shimio.AttachOptions{Stdout: stdout, Stderr: stderr})

		writeFIFO(cfg.Stdout, "out")
		writeFIFO(cfg.Stderr, "err")
		Eventually(done).Should(BeClosed())
		eio.Wait()

		Expect(stdout.String()).To(Equal("out"))
		Expect(stderr.String()).To(Equal("err"))
		Expect(eio.Close()).To(Succeed())
		Expect(filepath.Dir(cfg.Stdout)).NotTo(BeAnExistingFile())
	})
})

var _ = Describe("NewSerialWriteCloser", func() {
	It("should pass writes and close through", func() {
		buf := &syncBuffer{}
		wc := shimio.NewSerialWriteCloser(buf)

		_, err := wc.Write([]byte("data"))
		Expect(err).NotTo(HaveOccurred())
		Expect(wc.Close()).To(Succeed())
		Expect(buf.String()).To(Equal("data"))
		Expect(buf.Closed()).To(BeTrue())
	})
})
