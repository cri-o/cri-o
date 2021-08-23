package process

import (
	"syscall"
	"time"

	"github.com/sirupsen/logrus"
)

// ZombieMonitor is a structure for watching and cleaning up zombies on the node.
// It is responsible for cleaning up zombies that are children of the currently running process.
// It does so by occasionally polling for the zombie processes.
// If any zombies are found, there is a delay between when they're identified and when they're cleaned.
// This is to ensure ZombieMonitor doesn't interfere with the go runtime's own child management.
type ZombieMonitor struct {
	closeChan chan struct{}
}

// NewZombieMonitor creates and starts the zombie monitor.
func NewZombieMonitor() *ZombieMonitor {
	monitor := &ZombieMonitor{
		closeChan: make(chan struct{}, 1),
	}
	go monitor.Start()
	return monitor
}

// Shutdown instructs the zombie monitor to stop listening and exit.
func (zm *ZombieMonitor) Shutdown() {
	zm.closeChan <- struct{}{}
}

// Start begins the zombie monitor. It will populate the zombie count,
// as well as begin the zombie cleaning process.
func (zm *ZombieMonitor) Start() {
	for {
		_, zombieChildren, err := ParseDefunctProcesses()
		if err != nil {
			logrus.Warnf("Failed to get defunct process information: %v", err)
		}
		select {
		case <-zm.closeChan:
			// Since the process will soon shutdown, and its children will be reparented, no need to delay the shutdown to cleanup.
			return
		case <-time.After(defaultZombieChildReapPeriod):
		}
		for _, child := range zombieChildren {
			if _, err := syscall.Wait4(child, nil, syscall.WNOHANG, nil); err != nil {
				logrus.Errorf("Failed to reap child process %d: %v", child, err)
			}
		}
	}
}
