package oci

import (
	"runtime"
	"strings"
)

// goroutineStacks returns one entry per goroutine from a full stack dump,
// growing the buffer until the dump is not truncated.
func goroutineStacks() []string {
	bufSize := 1 << 20

	for {
		buf := make([]byte, bufSize)

		n := runtime.Stack(buf, true)
		if n == len(buf) {
			bufSize *= 2

			continue
		}

		return strings.Split(string(buf[:n]), "\n\n")
	}
}

// countGoroutines counts goroutines whose stack contains every needle.
func countGoroutines(needles ...string) int {
	count := 0

	for _, stack := range goroutineStacks() {
		matched := true

		for _, needle := range needles {
			if !strings.Contains(stack, needle) {
				matched = false

				break
			}
		}

		if matched {
			count++
		}
	}

	return count
}
