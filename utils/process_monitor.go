package utils

import (
	"context"
	"errors"
	"fmt"
	"github.com/cri-o/cri-o/internal/log"
	"sync"
	"syscall"
)

// ProcessMonitor handles monitoring multiple processes using a single epoll instance
type ProcessMonitor struct {
	epfd      int
	processes map[int]int
	pidfds    map[int32]int
	eventChan chan int32
	mu        sync.Mutex
	stopChan  chan struct{}
	wg        sync.WaitGroup
}

type ProcessInfo struct {
	pid   int
	pidfd int
}

// NewProcessMonitor creates a new process monitor
func NewProcessMonitor() (*ProcessMonitor, error) {
	epfd, err := syscall.EpollCreate1(0)
	if err != nil {
		return nil, fmt.Errorf("failed to create epoll instance: %w", err)
	}

	pm := &ProcessMonitor{
		epfd: epfd,
	}

	go pm.monitor()

	return pm, nil
}

// monitor is the only goroutine that blocks on epoll_wait
func (pm *ProcessMonitor) monitor() {
	// Buffer for epoll events
	events := make([]syscall.EpollEvent, 10)

	for {
		// Use a reasonable timeout instead of blocking indefinitely
		n, err := syscall.EpollWait(pm.epfd, events, 1000) // 1 second timeout

		// Check if we should stop
		select {
		case <-pm.stopChan:
			return
		default:
			// Continue processing
		}

		if err != nil {
			if errors.Is(err, syscall.EINTR) {
				continue // Ignore interrupted syscalls
			}
			log.Errorf(context.TODO(), "epoll_wait error: %v", err)
			continue
		}

		// Process events without blocking
		for i := 0; i < n; i++ {
			select {
			case pm.eventChan <- events[i].Fd:
				// Event sent to channel
			default:
				// Channel buffer is full, log and continue
			}
		}
	}
}

// AddProcess adds a process to be monitored
func (pm *ProcessMonitor) AddProcess(id int, pidfd int) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	// Add the pidfd to the epoll instance
	event := &syscall.EpollEvent{
		Events: syscall.EPOLLIN,
		Fd:     int32(pidfd),
	}
	if err := syscall.EpollCtl(pm.epfd, syscall.EPOLL_CTL_ADD, pidfd, event); err != nil {
		syscall.Close(pidfd)
		return fmt.Errorf("failed to add pidfd to epoll: %w", err)
	}
	pm.processes[id] = pidfd

	return nil
}

func (pm *ProcessMonitor) DeleteProcess(id int) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
}

// ProcessEvents returns a channel that will receive process termination events
func (pm *ProcessMonitor) ProcessEvents() <-chan *ProcessInfo {
	eventsChan := make(chan *ProcessInfo)

	// Start a goroutine to process events
	go func() {
		for fd := range pm.eventChan {
			pm.mu.Lock()
			process, exists := pm.processes[fd]
			if exists {
				// Make a copy to safely send
				processCopy := *process

				// Remove it from our tracking
				delete(pm.processes, fd)

				// Remove from epoll
				syscall.EpollCtl(pm.epfd, syscall.EPOLL_CTL_DEL, int(fd), nil)

				// Close the pidfd
				syscall.Close(int(fd))

				// Send the event
				eventsChan <- &processCopy
			}
			pm.mu.Unlock()
		}
		close(eventsChan)
	}()

	return eventsChan
}

// Close stops the monitor and releases resources
func (pm *ProcessMonitor) Close() error {
	// Signal the monitoring goroutine to stop
	close(pm.stopChan)

	// Wait for it to finish
	pm.wg.Wait()

	// Close the event channel
	close(pm.eventChan)

	// Close all pidfd's
	pm.mu.Lock()
	for fd, process := range pm.processes {
		syscall.Close(process.pidfd)
		delete(pm.processes, fd)
	}
	pm.mu.Unlock()

	// Close the epoll instance
	return syscall.Close(pm.epfd)
}
