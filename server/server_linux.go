package server

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"runtime/debug"
	"strconv"
	"strings"

	"github.com/opencontainers/runtime-spec/specs-go"
	"github.com/sirupsen/logrus"

	"github.com/cri-o/cri-o/internal/config/seccomp"
	"github.com/cri-o/cri-o/internal/log"
	"github.com/cri-o/cri-o/server/metrics"
)

func (s *Server) startSeccompNotifierWatcher(ctx context.Context) error {
	logrus.Info("Starting seccomp notifier watcher")

	s.seccompNotifierChan = make(chan seccomp.Notification)

	// Restore or cleanup
	notifierPath := s.config.Seccomp().NotifierPath()
	info, err := os.Stat(notifierPath)

	if err == nil && info.IsDir() {
		if err := filepath.Walk(notifierPath, func(path string, info fs.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}

			id := info.Name()

			if err := os.RemoveAll(path); err != nil {
				logrus.Error("Unable to remove path: %w", err)

				return nil
			}

			ctr, err := s.ContainerServer.GetContainerFromShortID(ctx, id)
			if err != nil {
				logrus.Warnf("Skipping not existing seccomp notifier container ID: %s", id)

				return nil
			}

			if ctr.State().Status != specs.StateRunning {
				logrus.Warnf("Skipping container %s because it is not running any more", id)

				return nil
			}

			// Restart the notifier
			notifier, err := seccomp.NewNotifier(ctx, s.seccompNotifierChan, id, path, ctr.Annotations())
			if err != nil {
				logrus.Errorf("Unable to run restored notifier: %v", err)

				return nil
			}

			s.seccompNotifiers.Store(id, notifier)

			return nil
		}); err != nil {
			return fmt.Errorf("unable to walk seccomp listener dir: %w", err)
		}
	} else {
		if err := os.RemoveAll(notifierPath); err != nil {
			return fmt.Errorf("unable to remove default seccomp listener dir: %w", err)
		}

		if err := os.MkdirAll(notifierPath, 0o700); err != nil {
			return fmt.Errorf("unable to create default seccomp listener dir: %w", err)
		}
	}

	// Start the notifier watcher
	go func() {
		for {
			msg := <-s.seccompNotifierChan
			ctx := msg.Ctx()
			id := msg.ContainerID()
			syscall := msg.Syscall()

			log.Infof(ctx, "Got seccomp notifier message for container ID: %s (syscall = %s)", id, syscall)

			result, ok := s.seccompNotifiers.Load(id)
			if !ok {
				log.Errorf(ctx, "Unable to get notifier for container ID")

				continue
			}

			notifier, ok := result.(*seccomp.Notifier)
			if !ok {
				log.Errorf(ctx, "Notifier is not a seccomp notifier type")

				continue
			}

			notifier.AddSyscall(syscall)

			ctr := s.ContainerServer.GetContainer(ctx, id)
			usedSyscalls := notifier.UsedSyscalls()

			if notifier.StopContainers() {
				// Stop the container only if the notifier timer has expired
				// The timer will be refreshed after each call to OnExpired.
				notifier.OnExpired(func() {
					log.Infof(ctx, "Seccomp notifier timer expired, stopping container %s", id)

					state := ctr.StateNoLock()
					state.SeccompKilled = true
					state.Error = "Used forbidden syscalls: " + usedSyscalls

					if err := s.stopContainer(context.Background(), ctr, 0); err != nil {
						log.Errorf(ctx, "Unable to stop container %s: %v", id, err)
					}
				})
			}

			metrics.Instance().MetricContainersSeccompNotifierCountTotalInc(ctr.Name(), syscall)
		}
	}()

	return nil
}

// configureMaxThreads sets the Go runtime max threads threshold
// which is 90% of the kernel setting from /proc/sys/kernel/threads-max.
func configureMaxThreads() error {
	mt, err := os.ReadFile("/proc/sys/kernel/threads-max")
	if err != nil {
		return fmt.Errorf("read max threads file: %w", err)
	}

	mtint, err := strconv.Atoi(strings.TrimSpace(string(mt)))
	if err != nil {
		return err
	}

	maxThreads := (mtint / 100) * 90
	debug.SetMaxThreads(maxThreads)
	logrus.Debugf("Golang's threads limit set to %d", maxThreads)

	return nil
}
