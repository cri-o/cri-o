// +build !linux

package main

import "github.com/sirupsen/logrus"

func disableSELinux() {
	logrus.Infof("there is no selinux to disable")
}
