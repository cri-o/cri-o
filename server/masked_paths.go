package server

import (
	"fmt"
	"os"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/containers/common/pkg/config"
)

// appendDefaultMaskedPaths is retrieving the default masked paths and appends
// the existing ones to it.
func appendDefaultMaskedPaths(additionalPaths []string) []string {
	paths := slices.Concat(defaultLinuxMaskedPaths(), additionalPaths)
	slices.Sort(paths)

	return slices.Compact(paths)
}

// defaultLinuxMaskedPaths will be used to evaluate the default masked paths once.
var defaultLinuxMaskedPaths = sync.OnceValue(func() []string {
	maskedPaths := slices.Concat(
		config.DefaultMaskedPaths,
		[]string{"/proc/asound", "/proc/interrupts"},
	)

	for _, cpu := range possibleCPUs() {
		path := fmt.Sprintf("/sys/devices/system/cpu/cpu%d/thermal_throttle", cpu)
		if _, err := os.Stat(path); err == nil {
			maskedPaths = append(maskedPaths, path)
		}
	}

	return maskedPaths
})

// possibleCPUs returns the number of possible CPUs on this host.
func possibleCPUs() (cpus []int) {
	if ncpu := possibleCPUsParsed(); ncpu != nil {
		return ncpu
	}

	for i := range runtime.NumCPU() {
		cpus = append(cpus, i)
	}

	return cpus
}

// possibleCPUsParsed is parsing the amount of possible CPUs on this host from
// /sys/devices.
var possibleCPUsParsed = sync.OnceValue(func() (cpus []int) {
	data, err := os.ReadFile("/sys/devices/system/cpu/possible")
	if err != nil {
		return nil
	}

	for r := range strings.SplitSeq(strings.TrimSpace(string(data)), ",") {
		if rStart, rEnd, ok := strings.Cut(r, "-"); !ok {
			cpu, err := strconv.Atoi(rStart)
			if err != nil {
				return nil
			}
			cpus = append(cpus, cpu)
		} else {
			var start, end int
			start, err := strconv.Atoi(rStart)
			if err != nil {
				return nil
			}
			end, err = strconv.Atoi(rEnd)
			if err != nil {
				return nil
			}
			for i := start; i <= end; i++ {
				cpus = append(cpus, i)
			}
		}
	}

	return cpus
})
