package server_test

import (
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"go.uber.org/mock/gomock"
	"golang.org/x/sys/unix"
)

// reloadCompletionHook captures the "Configuration reload completed" log
// message emitted by the SIGHUP reload watcher, which allows to wait
// deterministically for the completion of the configuration reload.
type reloadCompletionHook struct {
	mutex sync.Mutex
	fired bool
}

func (h *reloadCompletionHook) Fire(entry *logrus.Entry) error {
	if strings.HasPrefix(entry.Message, "Configuration reload completed") {
		h.mutex.Lock()
		defer h.mutex.Unlock()

		h.fired = true
	}

	return nil
}

func (h *reloadCompletionHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *reloadCompletionHook) hasFired() bool {
	h.mutex.Lock()
	defer h.mutex.Unlock()

	return h.fired
}

// The actual test suite.
var _ = t.Describe("Server", func() {
	BeforeEach(beforeEach)
	AfterEach(afterEach)

	t.Describe("ReloadConfig", func() {
		It(
			"should make runtime handlers added by SIGHUP reload available to RunPodSandbox",
			func() {
				// Given
				// Reset all SIGHUP channel registrations of this process: every
				// Server created in this suite registers a reload watcher which
				// would otherwise also receive the signal below and operate on
				// already torn down mocks. After the reset, the Server created
				// in this spec is the only SIGHUP watcher.
				signal.Reset(unix.SIGHUP)

				setupSUT()

				// The handler must not be available before the reload
				_, errBefore := sut.Runtime().ValidateRuntimeHandler("kata")
				Expect(errBefore).To(HaveOccurred())

				reloadConfig := `
[crio.runtime.runtimes.crun]
runtime_path = "/bin/echo"

[crio.runtime.runtimes.kata]
runtime_path = "/bin/echo"
`
				configFile := filepath.Join(t.MustTempDir("reload"), "config.conf")
				Expect(os.WriteFile(configFile, []byte(reloadConfig), 0o644)).
					To(Succeed())

				serverConfig.SetSingleConfigPath(configFile)

				imageServerMock.EXPECT().UpdatePinnedImagesList(gomock.Any())
				imageServerMock.EXPECT().PinnedImageRegexps().Return(nil)

				hook := &reloadCompletionHook{}
				logrus.AddHook(hook)

				// The watcher logs the reload completion on info level, which
				// is filtered out by the suite's default panic level
				logrus.SetLevel(logrus.InfoLevel)

				// When: sending SIGHUP, like `systemctl reload crio` does
				Expect(unix.Kill(os.Getpid(), unix.SIGHUP)).To(Succeed())

				// Then: wait for the reload watcher to complete before asserting
				// on the reloaded configuration
				Eventually(hook.hasFired).
					WithTimeout(10 * time.Second).
					WithPolling(100 * time.Millisecond).
					Should(BeTrue())

				// The added handler is now available for sandbox creation,
				// which is the path used by RunPodSandbox
				_, errAfter := sut.Runtime().ValidateRuntimeHandler("kata")
				Expect(errAfter).ToNot(HaveOccurred())
			},
		)
	})
})
