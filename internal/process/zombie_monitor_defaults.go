//go:build !test
// +build !test

package process

import "time"

// defaultZombieChildReapPeriod is the period
// the zombie monitor will reap defunct children
var defaultZombieChildReapPeriod = time.Minute * 5
