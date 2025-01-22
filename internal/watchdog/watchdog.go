package watchdog

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/coreos/go-systemd/v22/daemon"
	"k8s.io/apimachinery/pkg/util/wait"

	"github.com/cri-o/cri-o/internal/log"
)

// Watchdog is the main structure for this package.
type Watchdog struct {
	systemd        Systemd
	backoff        wait.Backoff
	healthCheckers []HealthCheckFn
	notifications  atomic.Uint64
}

const minInterval = time.Second

// HealthCheckFn is the health checker function type definition.
type HealthCheckFn func(context.Context, time.Duration) error

// New creates a new systemd Watchdog instance.
func New(healthCheckers ...HealthCheckFn) *Watchdog {
	return &Watchdog{
		systemd: DefaultSystemd(),
		backoff: wait.Backoff{
			Duration: time.Second,
			Factor:   2.0,
			Jitter:   0.1,
			Steps:    2,
		},
		healthCheckers: healthCheckers,
	}
}

// Start runs the watchdog.
func (w *Watchdog) Start(ctx context.Context) error {
	interval, err := w.systemd.WatchdogEnabled()
	if err != nil {
		return fmt.Errorf("configure watchdog: %w", err)
	}

	if interval == 0 {
		log.Infof(ctx, "No systemd watchdog enabled")
		return nil
	}

	if interval <= minInterval {
		return fmt.Errorf("watchdog timeout of %v should be at least %v", interval, minInterval)
	}
	interval /= 2

	log.Infof(ctx, "Starting systemd watchdog using interval: %v", interval)

	go wait.Until(func() {
		if err := w.runHealthCheckers(ctx, interval); err != nil {
			log.Errorf(ctx, "Will not notify watchdog because CRI-O is unhealthy: %v", err)
			return
		}

		if err := wait.ExponentialBackoff(w.backoff, func() (bool, error) {
			gotAck, err := w.systemd.Notify(daemon.SdNotifyWatchdog)
			w.notifications.Add(1)
			if err != nil {
				log.Warnf(ctx, "Failed to notify systemd watchdog, retrying: %v", err)
				return false, nil
			}
			if !gotAck {
				return false, errors.New("notification not supported (NOTIFY_SOCKET is unset)")
			}

			log.Debugf(ctx, "Systemd watchdog successfully notified")
			return true, nil
		}); err != nil {
			log.Errorf(ctx, "Failed to notify watchdog: %v", err)
		}
	}, interval, ctx.Done())

	return nil
}

// Notifications returns the amount of done systemd notifications.
func (w *Watchdog) Notifications() uint64 {
	return w.notifications.Load()
}

func (w *Watchdog) runHealthCheckers(ctx context.Context, timeout time.Duration) error {
	for _, hc := range w.healthCheckers {
		if err := hc(ctx, timeout); err != nil {
			return fmt.Errorf("health checker failed: %w", err)
		}
	}
	return nil
}
