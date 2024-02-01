package server

import (
	"runtime/debug"

	"github.com/sirupsen/logrus"
	"golang.org/x/sys/unix"
)

// configureMaxThreads sets the Go runtime max threads threshold
// which is 90% of the kernel setting from sysctl kern.threads.max_threads_per_proc
func configureMaxThreads() error {
	maxThreadsPerProc, err := unix.SysctlUint32("kern.threads.max_threads_per_proc")
	if err != nil {
		return nil
	}
	maxThreads := (int(maxThreadsPerProc) / 100) * 90
	debug.SetMaxThreads(maxThreads)
	logrus.Debugf("Golang's threads limit set to %d", maxThreads)

	return nil
}
