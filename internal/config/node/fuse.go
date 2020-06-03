package node

import (
	"bufio"
	"os"
	"strings"
	"sync"
)

var (
	hasFuseOnce sync.Once
	hasFuse     bool
	hasFuseErr  error
)

func HasFuse() bool {
	hasFuseOnce.Do(func() {
		f, err := os.Open("/proc/mounts")
		if err != nil {
			hasFuseErr = err
			return
		}
		defer f.Close()

		s := bufio.NewScanner(f)

		for s.Scan() {
			if strings.Contains(s.Text(), "/dev/fuse") {
				hasFuse = true
				break
			}
		}
	})
	return hasFuse
}
