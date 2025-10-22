package nri

import (
	"fmt"
	"os"
	goruntime "runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	onlineCpus = "/sys/devices/system/cpu/online"
	normalMems = "/sys/devices/system/node/has_normal_memory"
)

var (
	availableCpuset []string
	availableMemset []string
)

// getAvailableCpuset returns the set of online CPUs.
func getAvailableCpuset(t *testing.T) []string {
	if availableCpuset == nil {
		availableCpuset = getXxxset(t, "cpuset", onlineCpus)
	}

	return availableCpuset
}

// getAvailableMemset returns the set of usable NUMA nodes.
func getAvailableMemset(t *testing.T) []string {
	if availableMemset == nil {
		availableMemset = getXxxset(t, "memset", normalMems)
	}

	return availableMemset
}

// getXxxset reads/parses a CPU/memory set into a slice.
func getXxxset(t *testing.T, kind, path string) []string {
	var (
		data []byte
		set  []string
		one  uint64
		err  error
	)

	data, err = os.ReadFile(path)
	if err != nil {
		t.Logf("failed to read %s: %v", path, err)

		return nil
	}

	for rng := range strings.SplitSeq(strings.TrimSpace(string(data)), ",") {
		var (
			lo int
			hi = -1
		)

		loHi := strings.Split(rng, "-")
		switch len(loHi) {
		case 2:
			one, err = strconv.ParseUint(loHi[1], 10, 32)
			if err != nil {
				t.Errorf("failed to parse %s range %q: %v", kind, rng, err)

				return nil
			}

			hi = int(one) + 1

			fallthrough
		case 1:
			one, err = strconv.ParseUint(loHi[0], 10, 32)
			if err != nil {
				t.Errorf("failed to parse %s range %q: %v", kind, rng, err)

				return nil
			}

			lo = int(one)
		default:
			t.Errorf("invalid %s range %q", kind, rng)

			return nil
		}

		if hi == -1 {
			hi = lo + 1
		}

		for i := lo; i < hi; i++ {
			set = append(set, strconv.Itoa(i))
		}
	}

	return set
}

func getTestNamespace() string {
	pcs := make([]uintptr, 32)
	cnt := goruntime.Callers(2, pcs)

	for _, pc := range pcs[:cnt] {
		name := goruntime.FuncForPC(pc).Name()
		modAndName := strings.Split(name, ".")

		if cnt := len(modAndName); cnt >= 2 {
			name = modAndName[cnt-1]
		}

		if after, ok := strings.CutPrefix(name, "Test"); ok {
			name = after

			return name
		}
	}

	return "unknown-test-namespace"
}

// Wait for a file to show up in the filesystem then read its content.
func waitForFileAndRead(path string) ([]byte, error) {
	var (
		deadline = time.After(5 * time.Second)
		slack    = 100 * time.Millisecond
	)

	for {
		if _, err := os.Stat(path); err == nil {
			break
		}

		select {
		case <-deadline:
			return nil, fmt.Errorf("waiting for %s timed out", path)
		default:
			time.Sleep(slack)
		}
	}

	time.Sleep(slack)

	return os.ReadFile(path)
}
