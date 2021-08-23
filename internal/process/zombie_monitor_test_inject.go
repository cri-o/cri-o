//go:build test
// +build test

// All *_inject.go files are meant to be used by tests only. Purpose of this
// files is to provide a way to inject mocked data into the current setup.

package process

import "time"

// defaultZombieChildReapPeriod reduces the time waited to reap the children
// for testing purposes
var defaultZombieChildReapPeriod = time.Second * 5
