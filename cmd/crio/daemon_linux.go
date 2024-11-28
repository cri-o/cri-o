package main

import (
	"github.com/coreos/go-systemd/v22/daemon"
	"github.com/sirupsen/logrus"

	"github.com/cri-o/cri-o/internal/watchdog"
)

func sdNotify() {
	if _, err := watchdog.DefaultSystemd().Notify(daemon.SdNotifyReady); err != nil {
		logrus.Warnf("Failed to sd_notify systemd: %v", err)
	}
}

// notifySystem sends a message to the host when the server is ready to be used.
func notifySystem() {
	// Tell the init daemon we are accepting requests
	go sdNotify()
}
