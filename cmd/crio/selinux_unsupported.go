// +build !linux

package main

import "github.com/sirupsen/logrus"

func disableSELinux() {
	logrus.Debugf("there is no selinux to disable")
}
